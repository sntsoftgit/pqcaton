package decision

import (
	"errors"
	"sync"

	"github.com/randyinthedev-hash/pqcota/pkg/org"
)

// JudgmentStore — 판정 영속화(§3.6, §7). append-only — Save는 언제나 새 레코드를 쌓는다(§0.2).
// All()은 판정 순서(오래된→최신)로 돌려준다. 최신 상태는 LatestPerSubject로 파생.
// ErrNoSessionID — 빈 세션 id 로 원장을 찾으려 했다. 옛 행이 빈 값을 갖고 있어, 그것으로 찾으면
// 세션이 아니라 「세션을 모르는 판정 전부」가 나온다. 세션이 아닌 것을 세션이라고 돌려주지 않는다.
var ErrNoSessionID = errors.New("cannot look up judgments by an empty session id")

type JudgmentStore interface {
	Save(j *Judgment) error
	Get(id string) (*Judgment, error)
	BySubject(subject string) ([]*Judgment, error)
	// BySessionID — 한 리뷰 세션에서 난 판정 전부. 계획 id 가 담은 세션 id 로 원장을 되짚는 길이다.
	// 빈 id 는 아무것도 가리키지 않는다 — 옛 행이 빈 값을 갖고 있어, 빈 값으로 찾으면 세션이
	// 아니라 「세션을 모르는 판정 전부」가 나온다.
	BySessionID(sessionID string) ([]*Judgment, error)
	All() ([]*Judgment, error)
}

// MemJudgmentStore — 인메모리 append-only 로그(테스트·데모용).
//
// Pg판과 같은 모양으로 조직에 묶인다 — 테스트가 격리 없는 경로를 타면 실제 동작과 어긋난다.
type MemJudgmentStore struct {
	mu  sync.Mutex
	org org.ID
	log []*Judgment
}

// NewMemJudgmentStore — 조직을 지정해 연다. 빈 조직은 열리지 않는다(Pg판과 같은 규칙).
func NewMemJudgmentStore(o org.ID) (*MemJudgmentStore, error) {
	if o == "" {
		return nil, org.ErrEmpty
	}
	return &MemJudgmentStore{org: o}, nil
}

// Org — 이 핸들이 묶인 조직.
func (m *MemJudgmentStore) Org() org.ID { return m.org }

func (m *MemJudgmentStore) Save(j *Judgment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *j
	m.log = append(m.log, &cp)
	return nil
}

func (m *MemJudgmentStore) Get(id string) (*Judgment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.log {
		if j.ID == id {
			cp := *j
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *MemJudgmentStore) BySubject(subject string) ([]*Judgment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Judgment
	for _, j := range m.log {
		if j.Subject == subject {
			cp := *j
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *MemJudgmentStore) BySessionID(sessionID string) ([]*Judgment, error) {
	if sessionID == "" {
		return nil, ErrNoSessionID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Judgment
	for _, j := range m.log {
		if j.SessionID == sessionID {
			cp := *j
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *MemJudgmentStore) All() ([]*Judgment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Judgment, len(m.log))
	for i, j := range m.log {
		cp := *j
		out[i] = &cp
	}
	return out, nil
}
