package access

import (
	"errors"
	"sync"
	"time"

	"github.com/pqcota/pqcota/pkg/org"
)

// ── 저장되는 것 ────────────────────────────────────────────────────────────

// TokenRecord — 러너 토큰. 평문은 없다(§6.4.1).
type TokenRecord struct {
	Lookup   string // 평문 색인
	Org      org.ID
	Digest   []byte // 비밀의 SHA-256
	Label    string // "본사 러너" 같은 사람용 이름
	IssuedAt time.Time
	LastUsed time.Time // 제로값이면 아직 안 쓰임
	Revoked  time.Time // 제로값이면 유효
}

// CollectorKey — 조직에 등록된 collector 공개키(§6.4.2).
//
// 같은 (조직, collector_id)에 유효한 키가 **둘 이상인 구간**이 있어야 키를 바꿀 수 있다 —
// 새 키를 등록하고, 러너들이 갈아탄 뒤, 옛 키를 폐기한다. 그래서 키 자체가 식별자에 든다.
type CollectorKey struct {
	Org         org.ID
	CollectorID string
	PublicKey   string // base64 ed25519
	Label       string
	Registered  time.Time
	Revoked     time.Time // 제로값이면 유효
}

// Runner — 등록된 러너. 어느 토큰으로 등록했는지 남긴다 — 토큰을 폐기하면
// **그 토큰을 쓴 러너만** 끊긴다.
type Runner struct {
	ID          string
	Org         org.ID
	TokenLookup string
	Label       string
	Version     string
	Registered  time.Time
	LastSeen    time.Time
}

// ── 저장소 ─────────────────────────────────────────────────────────────────

// ErrNotFound — 그런 러너가 없다.
var ErrNotFound = errors.New("없다")

// TokenStore — 러너 토큰. 발급은 관리자 경로에서만 부른다(§6.4.1).
type TokenStore interface {
	PutToken(TokenRecord) error
	TokenByLookup(lookup string) (TokenRecord, error) // 없으면 ErrUnknownToken
	RevokeToken(lookup string, at time.Time) error
	TouchToken(lookup string, at time.Time) error
}

// KeyStore — collector 공개키 등록소. 등록도 관리자 경로에서만 부른다(§6.2).
type KeyStore interface {
	RegisterKey(CollectorKey) error
	// ActiveKeys — 그 조직의 그 collector에 **유효한** 키 전부. 폐기된 것은 빠진다.
	//
	// 조직 조건이 함께 붙는 것이 요점이다 — 조직 경계와 collector 경계가 같은 자리에서
	// 걸린다. 이 목록을 sign.Verify에 넘긴다(§6.4.2).
	ActiveKeys(o org.ID, collectorID string) ([]string, error)
	RevokeKey(o org.ID, collectorID, publicKey string, at time.Time) error
}

// RunnerStore — 러너 등록.
type RunnerStore interface {
	PutRunner(Runner) error
	Runner(id string) (Runner, error) // 없으면 ErrNotFound
	TouchRunner(id, version string, at time.Time) error
}

// Store — 셋 다.
type Store interface {
	TokenStore
	KeyStore
	RunnerStore
}

// ── 검증 ───────────────────────────────────────────────────────────────────

// Authenticate — 평문 토큰으로 조직을 유도한다.
//
// **조직은 러너가 주장하는 것이 아니라 여기서 나온다**(§6.4). 요청 본문에 조직이 적혀
// 있어도 보지 않는다.
//
// 실패 사유를 나눠 돌려주는 것은 **기록을 위해서다** — 폐기된 토큰을 계속 쓰는 러너와
// 아무 토큰이나 넣어 보는 쪽은 다른 일이다. 다만 **응답은 구분하지 않는다**: HTTP 계층은
// 어느 쪽이든 같은 거절을 낸다.
func Authenticate(s TokenStore, plaintext string, now time.Time) (org.ID, TokenRecord, error) {
	lookup, secret, err := SplitToken(plaintext)
	if err != nil {
		return "", TokenRecord{}, err
	}
	rec, err := s.TokenByLookup(lookup)
	if err != nil {
		return "", TokenRecord{}, err
	}
	if !rec.Revoked.IsZero() {
		return "", TokenRecord{}, ErrRevoked
	}
	if !matches(secret, rec.Digest) {
		return "", TokenRecord{}, ErrSecret
	}
	if rec.Org == "" {
		// 여기까지 왔는데 조직이 비어 있으면 발급 경로가 깨진 것이다. 품지 않는다 —
		// 상류의 org.Resolve를 쓰지 않는 이유와 같다(docs/design.md §2.1).
		return "", TokenRecord{}, org.ErrEmpty
	}
	_ = s.TouchToken(lookup, now) // 마지막 사용 시각은 실패해도 인증을 막지 않는다
	return rec.Org, rec, nil
}

// ── 인메모리 ───────────────────────────────────────────────────────────────

// MemStore — 인메모리 구현. **Pg판과 같은 규칙을 지킨다** — 테스트가 느슨한 경로를
// 타면 실제와 어긋난다(docs/design.md §2.1).
type MemStore struct {
	mu      sync.Mutex
	tokens  map[string]TokenRecord
	keys    []CollectorKey
	runners map[string]Runner
}

// NewMemStore — 빈 저장소.
func NewMemStore() *MemStore {
	return &MemStore{tokens: map[string]TokenRecord{}, runners: map[string]Runner{}}
}

func (m *MemStore) PutToken(r TokenRecord) error {
	if r.Org == "" {
		return org.ErrEmpty
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[r.Lookup] = r
	return nil
}

func (m *MemStore) TokenByLookup(lookup string) (TokenRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.tokens[lookup]
	if !ok {
		return TokenRecord{}, ErrUnknownToken
	}
	return r, nil
}

func (m *MemStore) RevokeToken(lookup string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.tokens[lookup]
	if !ok {
		return ErrUnknownToken
	}
	r.Revoked = at
	m.tokens[lookup] = r
	return nil
}

func (m *MemStore) TouchToken(lookup string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.tokens[lookup]
	if !ok {
		return ErrUnknownToken
	}
	r.LastUsed = at
	m.tokens[lookup] = r
	return nil
}

func (m *MemStore) RegisterKey(k CollectorKey) error {
	if k.Org == "" {
		return org.ErrEmpty
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.keys {
		if e.Org == k.Org && e.CollectorID == k.CollectorID && e.PublicKey == k.PublicKey {
			m.keys[i] = k // 같은 키 재등록은 폐기를 되돌린다
			return nil
		}
	}
	m.keys = append(m.keys, k)
	return nil
}

func (m *MemStore) ActiveKeys(o org.ID, collectorID string) ([]string, error) {
	if o == "" {
		return nil, org.ErrEmpty
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, k := range m.keys {
		if k.Org == o && k.CollectorID == collectorID && k.Revoked.IsZero() {
			out = append(out, k.PublicKey)
		}
	}
	return out, nil
}

func (m *MemStore) RevokeKey(o org.ID, collectorID, publicKey string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, k := range m.keys {
		if k.Org == o && k.CollectorID == collectorID && k.PublicKey == publicKey {
			m.keys[i].Revoked = at
			return nil
		}
	}
	return ErrNotFound
}

func (m *MemStore) PutRunner(r Runner) error {
	if r.Org == "" {
		return org.ErrEmpty
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runners[r.ID] = r
	return nil
}

func (m *MemStore) Runner(id string) (Runner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runners[id]
	if !ok {
		return Runner{}, ErrNotFound
	}
	return r, nil
}

func (m *MemStore) TouchRunner(id, version string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runners[id]
	if !ok {
		return ErrNotFound
	}
	r.LastSeen = at
	if version != "" {
		r.Version = version
	}
	m.runners[id] = r
	return nil
}
