package decision

import "sync"

// JudgmentStore — 판정 영속화(§3.6, §7). append-only — Save는 언제나 새 레코드를 쌓는다(§0.2).
// All()은 판정 순서(오래된→최신)로 돌려준다. 최신 상태는 LatestPerSubject로 파생.
type JudgmentStore interface {
	Save(j *Judgment) error
	Get(id string) (*Judgment, error)
	BySubject(subject string) ([]*Judgment, error)
	All() ([]*Judgment, error)
}

// MemJudgmentStore — 인메모리 append-only 로그(테스트·데모용).
type MemJudgmentStore struct {
	mu  sync.Mutex
	log []*Judgment
}

func NewMemJudgmentStore() *MemJudgmentStore { return &MemJudgmentStore{} }

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
