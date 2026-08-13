// Package jobs — 러너에게 줄 일.
//
// [러너 설계 §6.2.1](../../design.md)의 작업 일생을 담는다. HTTP는 이 위에 얹힌다 —
// 이 패키지는 네트워크를 모르고, 조직은 이미 정해진 것을 받는다.
package jobs

import (
	"errors"
	"time"

	"github.com/pqcota/pqcota/pkg/org"
)

// Kind — 작업 종류(§6.2).
type Kind string

const (
	// Enroll — 대상에 붙어 머신 지문만 확인한다.
	Enroll Kind = "enroll"
	// Observe — collector를 반입·실행·회수한다.
	Observe Kind = "observe"
	// Provision — 확정된 계획을 적용하거나 되돌린다.
	Provision Kind = "provision"
)

// AllKinds — 아는 종류 전부.
//
// Pg판이 재배포 정책을 질의 파라미터로 넘겨야 해서 목록이 필요하다. 정책 자체는
// [Kind.Redeployable] 하나에만 두고, 이 목록은 그것을 훑는 데만 쓴다.
var AllKinds = []Kind{Enroll, Observe, Provision}

// State — 작업의 상태.
type State string

const (
	// Pending — 아직 아무도 안 가져갔다.
	Pending State = "pending"
	// Leased — 러너가 가져갔다. 만료시각이 있다.
	Leased State = "leased"
	// Done — 끝났다.
	Done State = "done"
	// NeedsReview — **사람이 봐야 한다.** 자동으로 다시 주지 않는다.
	NeedsReview State = "needs-review"
)

// Redeployable — 만료됐을 때 자동으로 다시 배포해도 되는 종류인가.
//
// **읽기만 열거한다.** `k != Provision`으로 쓰면 모르는 종류가 자동 재배포로 떨어진다 —
// 새 종류가 쓰기일 때 아무도 손대지 않아도 조용히 두 번 실행되는 쪽으로 기운다.
// 러너가 플레이북을 이미 적용했는데 응답만 못 준 것일 수 있어, 다시 주면 두 번 적용되고
// 두 번째 before 캡처가 이미 바뀐 상태를 찍는다(§6.2.1).
func (k Kind) Redeployable() bool { return k == Enroll || k == Observe }

// NoteExpired — 자동 재배포를 하지 않고 사람에게 넘길 때 남기는 사유.
//
// 저장소마다 다른 문구를 두지 않는다 — 화면에서 같은 일이 두 가지로 보인다.
const NoteExpired = "점유가 만료됐다 — 러너가 이미 적용했을 수 있어 자동으로 다시 주지 않는다"

// Job — 작업 하나.
type Job struct {
	ID      string
	Org     org.ID
	Kind    Kind
	State   State
	Payload []byte // 종류가 정하는 내용. 이 패키지는 열어 보지 않는다

	// Targets — 대상 노드 식별자. **접속 정보가 아니다** — 그것은 러너에만 있다(§6.3).
	Targets []string

	RunnerID  string    // 점유한 러너
	LeaseTill time.Time // 점유 만료. Leased가 아니면 제로값
	Attempts  int       // 배포된 횟수
	Created   time.Time
	Updated   time.Time
	Note      string // NeedsReview로 간 이유 같은 것
}

var (
	// ErrNotFound — 그런 작업이 없다.
	ErrNotFound = errors.New("작업이 없다")
	// ErrNotLeased — 그 러너가 점유한 작업이 아니다.
	ErrNotLeased = errors.New("점유한 작업이 아니다")
)

// Store — 작업 저장소.
//
// 조직이 모든 질의에 든다. 러너는 자기 조직의 작업만 받아야 하고, 그 조직은 토큰에서
// 유도된 것이다(§6.4).
type Store interface {
	// Put — 작업을 넣는다.
	Put(Job) error
	// Lease — 그 조직의 대기 중 작업 하나를 러너에게 준다. 없으면 (Job{}, false, nil).
	Lease(o org.ID, runnerID string, till time.Time, now time.Time) (Job, bool, error)
	// Extend — 진행 중임을 알린 러너의 점유를 미룬다. 긴 작업에 큰 만료값을 잡지 않기 위한 것이다.
	Extend(o org.ID, id, runnerID string, till time.Time) error
	// Complete — 끝냈다고 표시한다.
	Complete(o org.ID, id, runnerID string, now time.Time) error
	// Sweep — 만료된 점유를 정리한다.
	//
	// 다시 줄 수 있는 종류는 대기로 돌리고, `provision`은 **사람이 볼 자리**로 보낸다.
	// 돌려준 수와 사람에게 넘긴 수를 알려준다.
	Sweep(now time.Time) (redeployed, review int, err error)
	// Get — 한 건.
	Get(o org.ID, id string) (Job, error)
}
