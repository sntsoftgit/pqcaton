package intake_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pqcota/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/saas/internal/intake"
)

const dsnEnv = "PQCATON_TEST_DSN"

func pgSeen(t *testing.T) (*intake.PgSeen, org.ID) {
	t.Helper()
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		t.Skipf("%s가 없어 스킵한다 — 멱등의 원자성을 확인하지 못한 것이다", dsnEnv)
	}
	ctx := context.Background()
	if err := intake.MigrateSeen(ctx, dsn); err != nil {
		t.Fatalf("스키마: %v", err)
	}
	s, err := intake.NewPgSeen(ctx, dsn)
	if err != nil {
		t.Fatalf("열기: %v", err)
	}
	t.Cleanup(s.Close)
	return s, org.ID(fmt.Sprintf("t-%d", time.Now().UnixNano()))
}

// CP-PG-5 — **실 테이블에서도 동시 확보는 하나만 성공한다.**
//
// 인메모리는 뮤텍스 하나로 원자성이 자명하지만, Pg판은 `ON CONFLICT DO NOTHING`의
// 영향 행 수에 기대고 있다. 그 전제는 **실제 테이블에서만** 확인된다 — 인메모리 케이스가
// 통과해도 이쪽은 아무것도 증명하지 않는다.
func TestPgClaimIsAtomicUnderConcurrency(t *testing.T) {
	s, o := pgSeen(t)
	const n = 24

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		won   int
		fails []error
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.Claim(o, "같은-지문")
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fails = append(fails, err)
				return
			}
			if ok {
				won++
			}
		}()
	}
	wg.Wait()

	for _, e := range fails {
		t.Error(e)
	}
	if won != 1 {
		t.Fatalf("동시 확보에서 %d개가 성공했다 — 하나만 성공해야 한다", won)
	}
}

// CP-PG-6 — 놓은 뒤에는 다시 확보된다.
// 이것이 없으면 거절이 굳어, 키를 고쳐 등록해도 그 결과가 영영 못 들어온다.
func TestPgReleaseAllowsReclaim(t *testing.T) {
	s, o := pgSeen(t)

	ok, err := s.Claim(o, "fp")
	if err != nil || !ok {
		t.Fatalf("첫 확보: %v %v", ok, err)
	}
	if again, _ := s.Claim(o, "fp"); again {
		t.Fatal("이미 확보한 것이 다시 확보됐다")
	}
	if err := s.Release(o, "fp"); err != nil {
		t.Fatalf("반환: %v", err)
	}
	if again, err := s.Claim(o, "fp"); err != nil || !again {
		t.Fatalf("놓은 뒤에 다시 확보되지 않는다: %v %v", again, err)
	}
}

// CP-PG-7 — 멱등 표도 조직으로 갈린다.
// 한 테이블을 공유하므로 여기서만 잴 수 있다 — 섞이면 한 조직의 결과가 다른 조직에서
// "이미 받았다"로 사라진다.
func TestPgSeenIsolatesOrg(t *testing.T) {
	s, a := pgSeen(t)
	b := a + "-other"

	if ok, err := s.Claim(a, "fp"); err != nil || !ok {
		t.Fatalf("확보: %v %v", ok, err)
	}
	ok, err := s.Claim(b, "fp")
	if err != nil {
		t.Fatalf("확보: %v", err)
	}
	if !ok {
		t.Fatal("다른 조직이 확보한 지문에 막혔다")
	}
	if _, err := s.Claim("", "fp"); !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("조직 없이 확보됐다: %v", err)
	}
}
