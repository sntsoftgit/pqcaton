// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package provrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// GrantTTL — 미리 보기가 낸 적용 권한이 사는 시간. 미리 보기가 성공으로 끝난 때부터 잰다.
const GrantTTL = 10 * time.Minute

// Binding — 실행에 들어가는 것의 묶음. 이름 → 그 내용의 해시다(승인된 계획 · 플레이북 · 인벤토리와 접속 정보 · 변수 ·
// 실행 설정 · callback · Ansible 실행 파일). 미리 보기 때와 적용 때의 묶음이 하나라도 다르면 미리 본 것과 실행할 것이 다르다.
type Binding map[string]string

// Hash — 묶음에 넣을 내용의 해시.
func Hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Digest — 묶음 전체의 해시. 이름 순으로 잇는다.
func (b Binding) Digest() string {
	keys := make([]string, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k + "\x00" + b[k] + "\x00"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Grant — 적용 권한. 미리 보기 하나가 「바뀔 것이 있고 모두 확인했다」로 끝났을 때만 생긴다. 그 미리 보기의 묶음에 묶이고,
// 10분 동안, 한 번만 쓸 수 있다.
type Grant struct {
	Run     string    `json:"run"`
	Digest  string    `json:"digest"`
	Issued  time.Time `json:"issued"`
	Expires time.Time `json:"expires"`
}

// NewGrant — ended는 미리 보기가 끝난 시각이다.
func NewGrant(run, digest string, ended time.Time) Grant {
	return Grant{Run: run, Digest: digest, Issued: ended, Expires: ended.Add(GrantTTL)}
}

// GrantState — 적용 권한의 지금 상태.
type GrantState string

const (
	GrantNone    GrantState = ""
	GrantValid   GrantState = "valid"
	GrantUsed    GrantState = "used"
	GrantChanged GrantState = "changed" // 묶음이 달라졌다. 남은 시간과 무관하다
	GrantExpired GrantState = "expired"
)

const (
	grantFile = "grant.json"
	usedFile  = "grant.used"
)

// State — digest는 지금 다시 잰 묶음이다. 쓴 것 · 달라진 것 · 만료 순으로 본다.
func (g Grant) State(now time.Time, digest string, used bool) GrantState {
	switch {
	case g.Run == "":
		return GrantNone
	case used:
		return GrantUsed
	case digest != g.Digest:
		return GrantChanged
	case !now.Before(g.Expires):
		return GrantExpired
	}
	return GrantValid
}

// WriteGrant · ReadGrant — 실행 자리에 둔다. 없으면 nil이다.
func WriteGrant(dir string, g Grant) error {
	b, err := json.Marshal(g)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, grantFile), append(b, '\n'), 0o600)
}

func ReadGrant(dir string) (*Grant, error) {
	b, err := os.ReadFile(filepath.Join(dir, grantFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var g Grant
	if err := json.Unmarshal(b, &g); err != nil || g.Run == "" {
		return nil, errors.New("the apply grant cannot be read")
	}
	return &g, nil
}

// Use — 권한을 소비한 기록. 어느 적용 실행이 언제 소비했는지다.
type Use struct {
	By string    `json:"by"`
	At time.Time `json:"at"`
}

// UsedBy — 소비 기록. 소비하지 않았으면 nil이다. 표시는 있는데 읽을 수 없으면(쓰다 끊겼으면) 빈 기록이다: 소비한 것으로 본다.
func UsedBy(dir string) *Use {
	b, err := os.ReadFile(filepath.Join(dir, usedFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	var u Use
	_ = json.Unmarshal(b, &u)
	return &u
}

// Used — 이 권한을 이미 썼는가.
func Used(dir string) bool { return UsedBy(dir) != nil }

// ErrGrant — 권한을 쓸 수 없다. State가 까닭이다.
type ErrGrant struct{ State GrantState }

func (e ErrGrant) Error() string { return "the apply grant cannot be used: " + string(e.State) }

// Consume — 권한을 소비한다. 유효할 때만, 한 번만 성공한다. 소비 기록(어느 실행이 언제)을 새로 만들 수 있어야 하고(O_EXCL: 두
// 요청이 함께 와도 하나만 만든다), **그 기록이 디스크에 닿은 뒤에야 성공을 돌려준다**(파일과 폴더를 fsync한다). 부르는 쪽은
// 성공을 받은 뒤에만 원격 작업을 허용한다. 기록을 만들다 실패하면 표시가 남을 수 있다: 그때 권한은 소비된 것이다(다시 쓰지 못한다).
func Consume(dir string, now time.Time, digest, by string) error {
	g, err := ReadGrant(dir)
	if err != nil {
		return err
	}
	if g == nil {
		return ErrGrant{GrantNone}
	}
	if st := g.State(now, digest, Used(dir)); st != GrantValid {
		return ErrGrant{st}
	}
	f, err := os.OpenFile(filepath.Join(dir, usedFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return ErrGrant{GrantUsed}
	}
	if err != nil {
		return err
	}
	b, _ := json.Marshal(Use{By: by, At: now})
	_, werr := f.Write(append(b, '\n'))
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil {
		return errors.Join(werr, serr, cerr)
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
