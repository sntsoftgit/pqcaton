package jobs_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pqcota/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/saas/internal/jobs"
)

const dsnEnv = "PQCATON_TEST_DSN"

// pgJobs — 실 테이블을 쓰는 저장소와, 이 실행에만 쓰는 조직.
//
// **스킵은 통과가 아니다.** 아래 케이스는 한 테이블을 공유하는 쪽에서만 잴 수 있다 —
// 인메모리는 저장소 객체가 애초에 달라 격리를 증명하지 않는다.
func pgJobs(t *testing.T) (*jobs.PgStore, org.ID) {
	t.Helper()
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("%s가 없어 스킵한다 — 격리와 동시 점유를 확인하지 못한 것이다", dsnEnv)
	}
	ctx := context.Background()
	if err := jobs.Migrate(ctx, dsn); err != nil {
		t.Fatalf("스키마: %v", err)
	}
	s, err := jobs.NewPgStore(ctx, dsn)
	if err != nil {
		t.Fatalf("열기: %v", err)
	}
	t.Cleanup(s.Close)

	// 같은 DB를 여러 번 돌려도 서로 밟지 않게 조직 이름을 매번 다르게 둔다.
	o := org.ID(fmt.Sprintf("t-%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		// Sweep은 조직을 가리지 않는다(배경 정리자다). 남겨 두면 다음 실행의 정리에
		// 걸려 세는 수가 흔들린다 — 자기가 만든 것은 자기가 치운다.
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return
		}
		defer pool.Close()
		_, _ = pool.Exec(ctx, `DELETE FROM pqcaton_job WHERE org LIKE $1`, string(o)+"%")
	})
	return s, o
}

func pgPut(t *testing.T, s jobs.Store, o org.ID, id string, k jobs.Kind, created time.Time) {
	t.Helper()
	if err := s.Put(jobs.Job{
		ID: id, Org: o, Kind: k, State: jobs.Pending, Created: created,
		Targets: []string{"web-01"},
	}); err != nil {
		t.Fatalf("넣기: %v", err)
	}
}

// CP-PG-8 — **한 테이블을 공유하는 상태에서** 작업이 조직으로 갈린다.
//
// `CP-JOB-2`는 인메모리에서만 통과했다. 거기서는 저장소 객체가 달라 아무것도 증명하지
// 않는다 — 조직 조건이 질의에 실제로 붙었는지는 여기서만 잴 수 있다.
func TestPgLeaseIsolatesOrg(t *testing.T) {
	s, a := pgJobs(t)
	b := a + "-other"
	pgPut(t, s, a, "j1", jobs.Observe, t0)

	if _, ok, err := s.Lease(b, "r1", t0.Add(time.Minute), t0); err != nil || ok {
		t.Fatalf("다른 조직의 작업이 나갔다: %v %v", ok, err)
	}
	if _, err := s.Get(b, "j1"); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("다른 조직에서 조회됐다: %v", err)
	}
	// 그리고 원래 조직에서는 멀쩡히 나간다 — 격리가 정상 경로까지 막으면 안 된다.
	if _, ok, err := s.Lease(a, "r1", t0.Add(time.Minute), t0); err != nil || !ok {
		t.Fatalf("자기 조직의 작업이 안 나갔다: %v %v", ok, err)
	}
}

// CP-PG-9 — **같은 작업이 두 러너에게 나가지 않는다.**
//
// 인메모리는 뮤텍스 하나로 자명하지만, Pg판은 `FOR UPDATE SKIP LOCKED`에 기댄다.
// 고르는 것과 표시하는 것이 나뉘면 두 러너가 같은 행을 골라 둘 다 점유한다 —
// 관측은 중복되고 적용은 두 번 된다. 그 전제는 **실 테이블에서만** 확인된다.
func TestPgLeaseIsExclusiveUnderConcurrency(t *testing.T) {
	s, o := pgJobs(t)
	const njobs, nrunners = 8, 24
	for i := 0; i < njobs; i++ {
		pgPut(t, s, o, fmt.Sprintf("j%02d", i), jobs.Observe, t0.Add(time.Duration(i)*time.Second))
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		got   = map[string]string{} // 작업 id → 가져간 러너
		dup   []string
		fails []error
	)
	for i := 0; i < nrunners; i++ {
		runner := fmt.Sprintf("r%02d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, ok, err := s.Lease(o, runner, t0.Add(time.Minute), t0)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				fails = append(fails, err)
			case !ok:
			default:
				if prev, seen := got[j.ID]; seen {
					dup = append(dup, fmt.Sprintf("%s → %s·%s", j.ID, prev, runner))
				}
				got[j.ID] = runner
			}
		}()
	}
	wg.Wait()

	for _, e := range fails {
		t.Error(e)
	}
	if len(dup) > 0 {
		t.Fatalf("같은 작업이 여러 러너에게 나갔다: %v", dup)
	}
	if len(got) != njobs {
		t.Fatalf("나간 작업이 %d개다 — %d개여야 한다", len(got), njobs)
	}
	// 점유 표시가 실제로 남았는지. 남지 않으면 다음 폴에서 또 나간다.
	for id, runner := range got {
		j, err := s.Get(o, id)
		if err != nil {
			t.Fatalf("조회: %v", err)
		}
		if j.State != jobs.Leased || j.RunnerID != runner || j.Attempts != 1 {
			t.Fatalf("%s의 점유가 남지 않았다: %+v", id, j)
		}
	}
}

// CP-PG-10 — 실 테이블에서도 **만료된 `provision`은 자동으로 다시 주지 않는다.**
//
// 정리 질의에 `kind <> 'provision'`을 손으로 적으면 정책이 Go와 SQL 두 곳에 생긴다.
// 한쪽만 고치는 날이 오고, 그때 쓰기 작업이 조용히 두 번 적용된다(§6.2.1).
func TestPgSweepFollowsKindPolicy(t *testing.T) {
	s, o := pgJobs(t)
	pgPut(t, s, o, "obs", jobs.Observe, t0)
	pgPut(t, s, o, "prov", jobs.Provision, t0.Add(time.Second))
	pgPut(t, s, o, "live", jobs.Observe, t0.Add(2*time.Second))

	for _, id := range []string{"obs", "prov"} {
		j, ok, err := s.Lease(o, "r1", t0.Add(time.Minute), t0)
		if err != nil || !ok || j.ID != id {
			t.Fatalf("점유(%s): %+v %v %v", id, j, ok, err)
		}
	}
	// 살아 있는 점유 하나 — 만료 전 것을 뺏으면 두 러너가 같은 일을 한다.
	if _, ok, err := s.Lease(o, "r2", t0.Add(time.Hour), t0); err != nil || !ok {
		t.Fatalf("점유(live): %v %v", ok, err)
	}

	back, review, err := s.Sweep(t0.Add(2 * time.Minute))
	if err != nil {
		t.Fatalf("정리: %v", err)
	}
	// Sweep은 조직을 가리지 않으므로 다른 실행이 남긴 것이 함께 세어질 수 있다.
	// 그래서 수는 최소선만 보고, **무엇이 어떻게 됐는지는 우리 작업으로 본다.**
	if back < 1 || review < 1 {
		t.Fatalf("정리가 돌지 않았다: 재배포 %d, 확인필요 %d", back, review)
	}

	obs, _ := s.Get(o, "obs")
	if obs.State != jobs.Pending || obs.RunnerID != "" || !obs.LeaseTill.IsZero() {
		t.Fatalf("읽기 작업이 대기로 돌아가지 않았다: %+v", obs)
	}
	prov, _ := s.Get(o, "prov")
	if prov.State != jobs.NeedsReview {
		t.Fatalf("적용 작업이 자동 재배포됐다: %+v", prov)
	}
	if prov.Note == "" {
		t.Fatal("왜 멈췄는지가 남지 않았다 — 조용히 멈추면 잊힌다")
	}
	live, _ := s.Get(o, "live")
	if live.State != jobs.Leased || live.RunnerID != "r2" {
		t.Fatalf("살아 있는 점유를 뺏었다: %+v", live)
	}
	// 확인 필요로 간 것은 다시 나가지 않는다.
	for {
		j, ok, err := s.Lease(o, "r3", t0.Add(time.Hour), t0.Add(3*time.Minute))
		if err != nil {
			t.Fatalf("점유: %v", err)
		}
		if !ok {
			break
		}
		if j.ID == "prov" {
			t.Fatal("확인 필요 상태인데 다시 나갔다")
		}
	}
}

// CP-PG-11 — 작업의 일생이 실 테이블에서도 그대로다: 오래된 것부터 → 연장 → 완료.
// 남의 점유는 미루지도 닫지도 못한다.
func TestPgJobLifecycle(t *testing.T) {
	s, o := pgJobs(t)
	pgPut(t, s, o, "new", jobs.Observe, t0.Add(time.Hour))
	pgPut(t, s, o, "old", jobs.Observe, t0)

	j, ok, err := s.Lease(o, "r1", t0.Add(time.Minute), t0)
	if err != nil || !ok {
		t.Fatalf("점유: %v %v", ok, err)
	}
	if j.ID != "old" {
		t.Fatalf("오래된 것이 먼저 나가지 않았다: %s", j.ID)
	}
	if len(j.Targets) != 1 || j.Targets[0] != "web-01" {
		t.Fatalf("대상이 돌아오지 않았다: %v", j.Targets)
	}

	if err := s.Extend(o, "old", "r2", t0.Add(time.Hour)); !errors.Is(err, jobs.ErrNotLeased) {
		t.Fatalf("남의 점유가 미뤄졌다: %v", err)
	}
	if err := s.Extend(o, "없는것", "r1", t0.Add(time.Hour)); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("없는 작업이 미뤄졌다: %v", err)
	}
	if err := s.Extend(o, "old", "r1", t0.Add(time.Hour)); err != nil {
		t.Fatalf("연장: %v", err)
	}
	if err := s.Complete(o, "old", "r2", t0); !errors.Is(err, jobs.ErrNotLeased) {
		t.Fatalf("남의 작업을 끝냈다: %v", err)
	}
	if err := s.Complete(o, "old", "r1", t0); err != nil {
		t.Fatalf("완료: %v", err)
	}
	done, _ := s.Get(o, "old")
	if done.State != jobs.Done || !done.LeaseTill.IsZero() {
		t.Fatalf("완료가 반영되지 않았다: %+v", done)
	}
	if _, _, err := s.Lease("", "r1", t0, t0); !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("조직 없이 점유했다: %v", err)
	}
}

// CP-PG-12 — 표가 없으면 만들지 않고 끊는다.
//
// 생성자가 말없이 DDL을 돌면 가리키는 곳이 어긋났을 때 빈 표가 새로 생기고 거기에 쓴다 —
// 러너가 받아 갈 작업이 통째로 사라진 것처럼 보인다.
func TestPgJobsRefuseMissingSchema(t *testing.T) {
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("%s가 없어 스킵한다", dsnEnv)
	}
	ctx := context.Background()
	if _, err := jobs.NewPgStore(ctx, dsn+"&options=-c%20search_path%3Dpg_temp"); !errors.Is(err, jobs.ErrSchemaMissing) {
		t.Fatalf("표 없이 열렸다: %v", err)
	}
}
