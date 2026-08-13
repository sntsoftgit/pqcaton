package intake_test

import (
	"fmt"
	"sync"
	"testing"

	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"

	"github.com/sntsoftgit/pqcaton/saas/internal/intake"
)

// CP-INTAKE-10 — 서로 다른 결과를 동시에 올려도 전부 들어간다.
//
// 저장소는 HTTP 요청 사이에서 공유된다. 단일 고루틴 테스트만 있으면 `-race`가 볼 것이
// 없어서, 검출기가 깨끗한 것이 아무것도 증명하지 않는다.
func TestConcurrentDistinctResults(t *testing.T) {
	f := newFixture(t)
	const n = 24

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			node := fmt.Sprintf("web-%02d", i)
			rep, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{
				signed(t, f.priv, result(node, node, t0)),
			})
			if err != nil {
				errs <- err
				return
			}
			if rep.Accepted != 1 {
				errs <- fmt.Errorf("%s: 통과하지 못했다 %+v", node, rep)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	for i := 0; i < n; i++ {
		node := fmt.Sprintf("web-%02d", i)
		if snap, _ := f.store.Latest(node); snap == nil {
			t.Fatalf("%s가 쌓이지 않았다", node)
		}
	}
}

// CP-INTAKE-11 — **같은 결과를 동시에 올려도 한 번만 쌓인다.**
//
// 러너가 응답을 못 받고 재시도하는 것이 정상 동작이고, 그 재시도가 앞의 요청과 겹칠 수
// 있다. 「봤나 확인 → 적재 → 표시」가 원자적이지 않으면 둘 다 통과해 관측 횟수가
// 부풀려진다 — `pqcota_observations`가 지키기로 한 "언제·몇 번 봤나"가 틀어진다.
func TestConcurrentResendIsCountedOnce(t *testing.T) {
	f := newFixture(t)
	res := signed(t, f.priv, result("web-01", "same", t0))
	const n = 16

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted int
		dup      int
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rep, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{res})
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			accepted += rep.Accepted
			dup += rep.Duplicate
			mu.Unlock()
		}()
	}
	wg.Wait()

	if accepted != 1 {
		t.Fatalf("같은 결과가 %d번 적재됐다 (중복 %d) — 멱등이 동시 요청에서 깨진다", accepted, dup)
	}
	if dup != n-1 {
		t.Fatalf("중복 수가 맞지 않는다: %d (기대 %d)", dup, n-1)
	}
}
