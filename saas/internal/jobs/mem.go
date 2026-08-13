package jobs

import (
	"sort"
	"sync"
	"time"

	"github.com/pqcota/pqcota/pkg/org"
)

// MemStore — 인메모리. **Pg판과 같은 규칙을 지킨다** — 테스트가 느슨한 경로를 타면
// 실제와 어긋난다.
type MemStore struct {
	mu sync.Mutex
	m  map[string]Job // key: org\x00id
}

// NewMemStore — 빈 저장소.
func NewMemStore() *MemStore { return &MemStore{m: map[string]Job{}} }

func mkey(o org.ID, id string) string { return string(o) + "\x00" + id }

func (s *MemStore) Put(j Job) error {
	if j.Org == "" {
		return org.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.State == "" {
		j.State = Pending
	}
	s.m[mkey(j.Org, j.ID)] = j
	return nil
}

func (s *MemStore) Get(o org.ID, id string) (Job, error) {
	if o == "" {
		return Job{}, org.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.m[mkey(o, id)]
	if !ok {
		return Job{}, ErrNotFound
	}
	return j, nil
}

func (s *MemStore) Lease(o org.ID, runnerID string, till, now time.Time) (Job, bool, error) {
	if o == "" {
		return Job{}, false, org.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// 오래 기다린 것부터. 같은 시각이면 id로 갈라 순서를 고정한다 —
	// 순서가 흔들리면 재현이 안 되고, 굶는 작업이 생겨도 안 보인다.
	var keys []string
	for k, j := range s.m {
		if j.Org == o && j.State == Pending {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return Job{}, false, nil
	}
	sort.Slice(keys, func(a, b int) bool {
		x, y := s.m[keys[a]], s.m[keys[b]]
		if !x.Created.Equal(y.Created) {
			return x.Created.Before(y.Created)
		}
		return x.ID < y.ID
	})

	j := s.m[keys[0]]
	j.State, j.RunnerID, j.LeaseTill = Leased, runnerID, till
	j.Attempts++
	j.Updated = now
	s.m[keys[0]] = j
	return j, true, nil
}

func (s *MemStore) Extend(o org.ID, id, runnerID string, till time.Time) error {
	if o == "" {
		return org.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.m[mkey(o, id)]
	if !ok {
		return ErrNotFound
	}
	if j.State != Leased || j.RunnerID != runnerID {
		return ErrNotLeased
	}
	j.LeaseTill = till
	s.m[mkey(o, id)] = j
	return nil
}

func (s *MemStore) Complete(o org.ID, id, runnerID string, now time.Time) error {
	if o == "" {
		return org.ErrEmpty
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.m[mkey(o, id)]
	if !ok {
		return ErrNotFound
	}
	if j.State != Leased || j.RunnerID != runnerID {
		return ErrNotLeased
	}
	j.State, j.LeaseTill, j.Updated = Done, time.Time{}, now
	s.m[mkey(o, id)] = j
	return nil
}

func (s *MemStore) Sweep(now time.Time) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var back, review int
	for k, j := range s.m {
		if j.State != Leased || j.LeaseTill.After(now) {
			continue
		}
		if j.Kind.Redeployable() {
			j.State, j.RunnerID, j.LeaseTill = Pending, "", time.Time{}
			back++
		} else {
			// 쓰기 작업은 자동으로 다시 주지 않는다 — 두 번 적용될 수 있고,
			// 두 번째 before 캡처는 이미 바뀐 상태를 찍는다(§6.2.1).
			j.State, j.LeaseTill = NeedsReview, time.Time{}
			j.Note = "점유가 만료됐다 — 러너가 이미 적용했을 수 있어 자동으로 다시 주지 않는다"
			review++
		}
		j.Updated = now
		s.m[k] = j
	}
	return back, review, nil
}
