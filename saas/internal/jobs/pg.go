package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pqcota/pqcota/pkg/org"
)

// Schema — 작업 표. [Migrate]가 올린다.
//
// 기본키에 조직이 든다. 작업 id는 조직 안에서만 뜻이 있고, 모든 질의가 조직으로 좁혀지는
// 것을 키가 강제한다 — id 하나를 키로 두면 조직 조건을 빠뜨린 질의가 남의 작업을 집는다.
const Schema = `
CREATE TABLE IF NOT EXISTS pqcaton_job (
    org        TEXT NOT NULL,
    id         TEXT NOT NULL,
    kind       TEXT NOT NULL,
    state      TEXT NOT NULL,
    payload    BYTEA,
    targets    TEXT[] NOT NULL DEFAULT '{}',
    runner_id  TEXT NOT NULL DEFAULT '',
    lease_till TIMESTAMPTZ,
    attempts   INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    note       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (org, id)
);

-- 배포는 "그 조직의 대기 중 작업을 오래된 것부터"가 전부다. 그 모양 그대로 색인한다.
CREATE INDEX IF NOT EXISTS idx_pqcaton_job_pending ON pqcaton_job(org, created_at, id) WHERE state = 'pending';
-- 정리는 조직을 가리지 않고 만료된 점유만 훑는다.
CREATE INDEX IF NOT EXISTS idx_pqcaton_job_lease ON pqcaton_job(lease_till) WHERE state = 'leased';
`

// ErrSchemaMissing — 표가 없다.
var ErrSchemaMissing = errors.New("작업 표가 없다 — Migrate를 먼저 돌려야 한다")

// PgStore — Postgres 구현.
type PgStore struct{ pool *pgxpool.Pool }

// Migrate — 표를 올린다. 저장소를 여는 것과 나눠 둔다(access.Migrate와 같은 이유).
func Migrate(ctx context.Context, dsn string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, Schema)
	return err
}

// NewPgStore — 연다. 표가 없으면 만들지 않고 끊는다.
func NewPgStore(ctx context.Context, dsn string) (*PgStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	var exists *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('pqcaton_job')::text`).Scan(&exists); err != nil {
		pool.Close()
		return nil, err
	}
	if exists == nil {
		pool.Close()
		return nil, ErrSchemaMissing
	}
	return &PgStore{pool: pool}, nil
}

// Close — 연결을 닫는다.
func (p *PgStore) Close() { p.pool.Close() }

// redeployableKinds — 자동 재배포해도 되는 종류의 이름들.
//
// 질의에 `kind <> 'provision'`을 적지 않는다. 그러면 정책이 Go와 SQL 두 곳에 생기고,
// 한쪽만 고치는 날이 온다(CP-JOB-10).
func redeployableKinds() []string {
	out := make([]string, 0, len(AllKinds))
	for _, k := range AllKinds {
		if k.Redeployable() {
			out = append(out, string(k))
		}
	}
	return out
}

func nullable(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func fromNull(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// jobCols — [scanJob]이 읽는 순서. 질의마다 다시 적지 않는다.
const jobCols = `id,org,kind,state,payload,targets,runner_id,lease_till,attempts,created_at,updated_at,note`

type scanner interface{ Scan(...any) error }

func scanJob(row scanner) (Job, error) {
	var (
		j    Job
		o, k string
		st   string
		till *time.Time
	)
	err := row.Scan(&j.ID, &o, &k, &st, &j.Payload, &j.Targets, &j.RunnerID, &till,
		&j.Attempts, &j.Created, &j.Updated, &j.Note)
	if err != nil {
		return Job{}, err
	}
	j.Org, j.Kind, j.State, j.LeaseTill = org.ID(o), Kind(k), State(st), fromNull(till)
	return j, nil
}

func (p *PgStore) Put(j Job) error {
	if j.Org == "" {
		return org.ErrEmpty
	}
	if j.State == "" {
		j.State = Pending
	}
	if j.Targets == nil {
		j.Targets = []string{}
	}
	_, err := p.pool.Exec(context.Background(),
		`INSERT INTO pqcaton_job(org,id,kind,state,payload,targets,runner_id,lease_till,attempts,created_at,updated_at,note)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,COALESCE($10,now()),COALESCE($11,now()),$12)
		 ON CONFLICT(org,id) DO UPDATE SET
		   kind=$3, state=$4, payload=$5, targets=$6, runner_id=$7, lease_till=$8,
		   attempts=$9, updated_at=COALESCE($11,now()), note=$12`,
		string(j.Org), j.ID, string(j.Kind), string(j.State), j.Payload, j.Targets,
		j.RunnerID, nullable(j.LeaseTill), j.Attempts,
		nullable(j.Created), nullable(j.Updated), j.Note)
	return err
}

func (p *PgStore) Get(o org.ID, id string) (Job, error) {
	if o == "" {
		return Job{}, org.ErrEmpty
	}
	j, err := scanJob(p.pool.QueryRow(context.Background(),
		`SELECT `+jobCols+` FROM pqcaton_job WHERE org=$1 AND id=$2`, string(o), id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}

// Lease — 한 문장으로 고르고 점유까지 한다.
//
// **`FOR UPDATE SKIP LOCKED`가 이 저장소의 핵심이다.** 고르는 것과 표시하는 것이 나뉘면
// 두 러너가 같은 행을 골라 둘 다 점유한다 — 관측은 중복되고 적용은 두 번 된다(CP-JOB-1).
// 잠긴 행을 건너뛰므로 러너가 늘어도 서로 기다리지 않는다.
func (p *PgStore) Lease(o org.ID, runnerID string, till, now time.Time) (Job, bool, error) {
	if o == "" {
		return Job{}, false, org.ErrEmpty
	}
	j, err := scanJob(p.pool.QueryRow(context.Background(),
		`WITH next AS (
		     SELECT org,id FROM pqcaton_job
		      WHERE org=$1 AND state='pending'
		      ORDER BY created_at, id
		      FOR UPDATE SKIP LOCKED
		      LIMIT 1
		 )
		 UPDATE pqcaton_job j
		    SET state='leased', runner_id=$2, lease_till=$3, attempts=j.attempts+1, updated_at=$4
		   FROM next n
		  WHERE j.org=n.org AND j.id=n.id
		 RETURNING j.id,j.org,j.kind,j.state,j.payload,j.targets,j.runner_id,j.lease_till,
		           j.attempts,j.created_at,j.updated_at,j.note`,
		string(o), runnerID, till, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, true, nil
}

func (p *PgStore) Extend(o org.ID, id, runnerID string, till time.Time) error {
	if o == "" {
		return org.ErrEmpty
	}
	tag, err := p.pool.Exec(context.Background(),
		`UPDATE pqcaton_job SET lease_till=$4
		  WHERE org=$1 AND id=$2 AND state='leased' AND runner_id=$3`,
		string(o), id, runnerID, till)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return p.whyNotLeased(o, id)
	}
	return nil
}

func (p *PgStore) Complete(o org.ID, id, runnerID string, now time.Time) error {
	if o == "" {
		return org.ErrEmpty
	}
	tag, err := p.pool.Exec(context.Background(),
		`UPDATE pqcaton_job SET state='done', lease_till=NULL, updated_at=$4
		  WHERE org=$1 AND id=$2 AND state='leased' AND runner_id=$3`,
		string(o), id, runnerID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return p.whyNotLeased(o, id)
	}
	return nil
}

// whyNotLeased — 안 고쳐진 이유를 가른다. 없는 것과 남의 점유는 다른 일이다 —
// 운영자가 보는 화면에서 "그런 작업 없음"과 "네 것이 아님"이 같은 말이 되면 안 된다.
func (p *PgStore) whyNotLeased(o org.ID, id string) error {
	var one int
	err := p.pool.QueryRow(context.Background(),
		`SELECT 1 FROM pqcaton_job WHERE org=$1 AND id=$2`, string(o), id).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return ErrNotLeased
}

// Sweep — 만료된 점유를 한 문장으로 정리한다.
//
// 두 갈래를 따로 돌리면 그 사이에 정리 대상이 바뀐다. 한 문장 안의 두 UPDATE는 같은
// 스냅숏을 보고, 종류로 갈려 서로 겹치지 않는다.
//
// **모르는 종류는 사람에게 넘긴다.** 재배포 가능한 목록에 있는 것만 돌려보낸다 —
// 목록에 없는 종류가 쓰기 작업이면, 자동 재배포는 되돌릴 수 없는 실수가 된다.
func (p *PgStore) Sweep(now time.Time) (int, int, error) {
	var redeployed, review int64
	err := p.pool.QueryRow(context.Background(),
		`WITH expired AS (
		     SELECT org,id,kind FROM pqcaton_job
		      WHERE state='leased' AND lease_till <= $1
		      FOR UPDATE SKIP LOCKED
		 ), back AS (
		     UPDATE pqcaton_job j
		        SET state='pending', runner_id='', lease_till=NULL, updated_at=$1
		       FROM expired e
		      WHERE j.org=e.org AND j.id=e.id AND e.kind = ANY($2)
		     RETURNING 1
		 ), held AS (
		     UPDATE pqcaton_job j
		        SET state='needs-review', lease_till=NULL, updated_at=$1, note=$3
		       FROM expired e
		      WHERE j.org=e.org AND j.id=e.id AND NOT (e.kind = ANY($2))
		     RETURNING 1
		 )
		 SELECT (SELECT count(*) FROM back), (SELECT count(*) FROM held)`,
		now, redeployableKinds(), NoteExpired).Scan(&redeployed, &review)
	if err != nil {
		return 0, 0, err
	}
	return int(redeployed), int(review), nil
}
