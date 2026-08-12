package access_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/pqcota/pqcota/pkg/org"
	"github.com/sntsoftgit/pqcaton/saas/internal/access"
)

// DSNEnv — 실 Postgres로 돌릴 때 쓰는 환경변수. 없으면 스킵한다.
// **스킵은 통과가 아니다** — 아래 케이스는 한 테이블을 공유하는 쪽에서만 잴 수 있다.
const dsnEnv = "PQCATON_TEST_DSN"

func pgStore(t *testing.T) (*access.PgStore, org.ID) {
	t.Helper()
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("%s가 없어 스킵한다 — 격리를 확인하지 못한 것이다", dsnEnv)
	}
	ctx := context.Background()
	if err := access.Migrate(ctx, dsn); err != nil {
		t.Fatalf("스키마: %v", err)
	}
	s, err := access.NewPgStore(ctx, dsn)
	if err != nil {
		t.Fatalf("열기: %v", err)
	}
	t.Cleanup(s.Close)
	// 같은 DB를 여러 번 돌려도 서로 밟지 않게 조직 이름을 매번 다르게 둔다.
	return s, org.ID(fmt.Sprintf("t-%d", time.Now().UnixNano()))
}

// CP-PG-1 — 토큰 왕복: 저장 → 인증 → 마지막 사용 → 폐기.
func TestPgTokenRoundTrip(t *testing.T) {
	s, o := pgStore(t)
	tok := issued(t, s, o)

	got, _, err := access.Authenticate(s, tok.Plaintext, t0)
	if err != nil {
		t.Fatalf("인증: %v", err)
	}
	if got != o {
		t.Fatalf("조직이 다르다: %q ≠ %q", got, o)
	}
	rec, err := s.TokenByLookup(tok.Lookup)
	if err != nil {
		t.Fatalf("조회: %v", err)
	}
	if rec.LastUsed.IsZero() {
		t.Fatal("마지막 사용이 남지 않았다")
	}

	if err := s.RevokeToken(tok.Lookup, t0); err != nil {
		t.Fatalf("폐기: %v", err)
	}
	if _, _, err := access.Authenticate(s, tok.Plaintext, t0); !errors.Is(err, access.ErrRevoked) {
		t.Fatalf("폐기 뒤에도 통과했다: %v", err)
	}
}

// CP-ORG-1 — **한 테이블을 공유하는 상태에서** 조직이 갈리는지.
//
// 인메모리 케이스는 저장소 객체가 애초에 다르므로 통과해도 격리를 증명하지 않는다.
// 여기서만 잴 수 있다.
func TestPgActiveKeysIsolatesOrg(t *testing.T) {
	s, a := pgStore(t)
	b := a + "-other"

	for _, k := range []struct {
		o   org.ID
		pub string
	}{{a, "key-a"}, {b, "key-b"}} {
		if err := s.RegisterKey(access.CollectorKey{
			Org: k.o, CollectorID: "openssl-collector", PublicKey: k.pub, Registered: t0,
		}); err != nil {
			t.Fatalf("등록: %v", err)
		}
	}

	got, err := s.ActiveKeys(a, "openssl-collector")
	if err != nil {
		t.Fatalf("조회: %v", err)
	}
	if len(got) != 1 || got[0] != "key-a" {
		t.Fatalf("다른 조직의 키가 보인다: %v", got)
	}
}

// CP-PG-2 — 키 교체 구간이 실제 테이블에서도 성립한다.
// 기본키에 public_key가 들어 있지 않으면 두 번째 등록이 첫 번째를 덮어쓴다.
func TestPgRotationWindowAndRevoke(t *testing.T) {
	s, o := pgStore(t)
	for _, k := range []string{"old", "new"} {
		if err := s.RegisterKey(access.CollectorKey{
			Org: o, CollectorID: "netcap", PublicKey: k, Registered: t0,
		}); err != nil {
			t.Fatalf("등록: %v", err)
		}
	}
	got, _ := s.ActiveKeys(o, "netcap")
	sort.Strings(got)
	if len(got) != 2 {
		t.Fatalf("교체 구간이 없다 — 덮어쓰였다: %v", got)
	}

	if err := s.RevokeKey(o, "netcap", "old", t0); err != nil {
		t.Fatalf("폐기: %v", err)
	}
	got, _ = s.ActiveKeys(o, "netcap")
	if len(got) != 1 || got[0] != "new" {
		t.Fatalf("폐기가 반영되지 않았다: %v", got)
	}
}

// CP-PG-3 — 러너 등록과 갱신.
func TestPgRunnerRoundTrip(t *testing.T) {
	s, o := pgStore(t)
	id := string(o) + "-r1"
	if err := s.PutRunner(access.Runner{
		ID: id, Org: o, TokenLookup: "abcdefgh", Version: "0.1.0", Registered: t0,
	}); err != nil {
		t.Fatalf("등록: %v", err)
	}
	seen := t0.Add(time.Hour)
	if err := s.TouchRunner(id, "0.2.0", seen); err != nil {
		t.Fatalf("갱신: %v", err)
	}
	got, err := s.Runner(id)
	if err != nil {
		t.Fatalf("조회: %v", err)
	}
	if got.Org != o || got.Version != "0.2.0" || !got.LastSeen.Equal(seen.UTC()) {
		t.Fatalf("갱신되지 않았다: %+v", got)
	}
	// 빈 버전은 기존 값을 지우지 않는다 — status가 버전을 안 보낼 때 지워지면 안 된다.
	if err := s.TouchRunner(id, "", seen.Add(time.Hour)); err != nil {
		t.Fatalf("갱신: %v", err)
	}
	got, _ = s.Runner(id)
	if got.Version != "0.2.0" {
		t.Fatalf("빈 버전이 기존 값을 지웠다: %q", got.Version)
	}
}

// CP-PG-4 — 스키마가 없으면 만들지 않고 끊는다.
//
// 생성자가 말없이 DDL을 돌면 가리키는 곳이 어긋났을 때 빈 테이블이 새로 생기고 거기에
// 쓴다 — 데이터가 사라진 것처럼 보인다.
func TestPgRefusesMissingSchema(t *testing.T) {
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("%s가 없어 스킵한다", dsnEnv)
	}
	// search_path를 빈 스키마로 돌려 테이블이 안 보이게 한다.
	ctx := context.Background()
	if _, err := access.NewPgStore(ctx, dsn+"&options=-c%20search_path%3Dpg_temp"); !errors.Is(err, access.ErrSchemaMissing) {
		t.Fatalf("스키마 없이 열렸다: %v", err)
	}
}
