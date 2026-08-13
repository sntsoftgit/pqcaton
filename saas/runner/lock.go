package runner

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrAlreadyRunning — 이전 실행이 아직 돌고 있다.
//
// **오류지만 고장은 아니다.** 스케줄이 관측 시간보다 촘촘하면 정상적으로 생긴다. 부르는
// 쪽은 이것을 실패로 세지 않는다 — 다음 스케줄에 어차피 다시 온다.
var ErrAlreadyRunning = errors.New("이전 실행이 아직 돈다")

// lockName — 결과 디렉터리에 두는 잠금 파일. `*.json`이 아니라 결과로 읽히지 않는다.
const lockName = ".lock"

// lock — 이 결과 디렉터리를 쓰는 실행을 **하나로 묶는다.**
//
// 러너는 상주하지 않고 스케줄이 매번 새로 띄운다. cron은 이전 실행이 끝났는지 보지 않으므로,
// 관측이 스케줄 간격보다 길면 **두 프로세스가 같은 노드에 동시에 붙는다** — collector가 같은
// 자리를 쓰기 때문에 서로 덮어쓰고, 하나가 정리하는 동안 다른 하나가 실행된다.
//
// **기다리지 않는다.** 잡혀 있으면 [ErrAlreadyRunning]으로 곧 끝낸다 — 기다리게 하면 밀린
// 프로세스가 줄줄이 쌓인다. 다음 스케줄에 다시 온다.
//
// 이것이 막는 것은 **한 노드 안**이다. 러너들 사이(같은 작업이 둘에게 나가는 것)는 컨트롤
// 플레인의 작업 점유가 막는다 — 층이 둘이고 각각 다른 것을 막는다.
func lock(dir string) (func(), error) {
	if dir == "" {
		return func() {}, nil // 결과 디렉터리가 없으면 지킬 것도 없다
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := flock(f); err != nil {
		f.Close()
		return nil, err
	}
	// 잠금은 파일을 닫으면 풀린다. **프로세스가 죽어도 커널이 풀어 준다** — 그래서
	// 락 파일이 남아 영영 못 도는 일이 없다(`O_EXCL` 방식의 약점이 여기 없다).
	return func() { f.Close() }, nil
}
