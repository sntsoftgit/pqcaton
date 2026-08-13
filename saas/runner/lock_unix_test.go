//go:build unix

package runner_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/sntsoftgit/pqcaton/saas/runner"
)

// RUN-15 — **이전 실행이 도는 동안 두 번째는 아무것도 하지 않는다.**
//
// 러너는 상주하지 않고 스케줄이 매번 새로 띄운다. cron은 이전 실행이 끝났는지 보지 않으므로,
// 관측이 스케줄 간격보다 길면 두 프로세스가 **같은 노드에 동시에 붙는다** — collector가 같은
// 자리를 쓰기 때문에 서로 덮어쓴다.
//
// 다른 프로세스가 잡은 상태를 흉내 내려고 잠금 파일을 직접 건다. 실제로 겪는 것이 그것이다.
func TestSecondRunDoesNothingWhileFirstHoldsTheLock(t *testing.T) {
	p := &plane{}
	srv := p.start(t)
	cfg, cl := setup(t, srv, "web-01-openssl.json")

	// 앞선 실행이 잡고 있는 상황.
	f, err := os.OpenFile(filepath.Join(cfg.ResultsDir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("잠금: %v", err)
	}

	_, err = runner.RunOnce(cfg, cl, quiet())
	if err != runner.ErrAlreadyRunning {
		t.Fatalf("겹쳐 돌았다: %v", err)
	}
	if p.gotBody != nil {
		t.Fatal("두 번째 실행이 결과를 올렸다 — 같은 파일을 둘이 다룬다")
	}
	if _, err := os.Stat(filepath.Join(cfg.ResultsDir, "web-01-openssl.json")); err != nil {
		t.Fatalf("두 번째 실행이 파일을 건드렸다: %v", err)
	}

	// 앞선 실행이 끝나면 다음 차례가 돈다.
	f.Close()
	rep, err := runner.RunOnce(cfg, cl, quiet())
	if err != nil || rep.Files != 1 {
		t.Fatalf("잠금이 풀렸는데 안 돌았다: %+v %v", rep, err)
	}
}
