package decision

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/randyinthedev-hash/pqcota/pkg/org"
)

// FileJudgmentStore — 파일 한 줄에 판정 하나(JSONL). append-only다.
//
// **Postgres 없이도 판정이 남아야 한다.** 이 리포를 체크아웃해서 한 바퀴 돌아 보는 사람에게
// DB를 세우게 하면 거기서 멈춘다. 그렇다고 판정을 안 남기면 *"누가 언제 무엇을 근거로
// 정했는지가 감사 근거로 남는다"* 는 말이 명령줄에서는 거짓이 된다.
//
// **append-only를 파일이 강제한다** — `O_APPEND`로만 연다. 고치려면 새 줄을 쌓고, 최신 상태는
// [LatestPerSubject]가 파생한다(Mem·Pg판과 같은 규칙).
type FileJudgmentStore struct {
	mu   sync.Mutex
	org  org.ID
	path string
}

// ErrOrgMismatch — 다른 조직의 판정이 그 파일에 섞여 있다.
//
// **읽는 쪽에서 거른다.** 파일은 누구나 이어 쓸 수 있어, 한 파일에 두 조직이 섞이면
// 격리가 파일 권한에만 기대게 된다.
var ErrOrgMismatch = errors.New("그 파일에 다른 조직의 판정이 있다")

// NewFileJudgmentStore — 조직을 지정해 연다. 빈 조직은 열리지 않는다(Mem·Pg판과 같은 규칙).
func NewFileJudgmentStore(o org.ID, path string) (*FileJudgmentStore, error) {
	if o == "" {
		return nil, org.ErrEmpty
	}
	if path == "" {
		return nil, errors.New("판정 파일 경로가 비었다")
	}
	return &FileJudgmentStore{org: o, path: path}, nil
}

// Org — 이 핸들이 묶인 조직.
func (f *FileJudgmentStore) Org() org.ID { return f.org }

// record — 파일에 실제로 쓰는 모양. 조직을 함께 적는다 — 읽을 때 거르기 위해서다.
type record struct {
	Org string   `json:"org"`
	J   Judgment `json:"judgment"`
}

func (f *FileJudgmentStore) Save(j *Judgment) error {
	if j == nil {
		return errors.New("판정이 비었다")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	// **O_APPEND만 쓴다.** 덮어쓸 방법을 두지 않는 것이 append-only를 지키는 자리다.
	fh, err := os.OpenFile(f.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer fh.Close()
	line, err := json.Marshal(record{Org: string(f.org), J: *j})
	if err != nil {
		return err
	}
	_, err = fh.Write(append(line, '\n'))
	return err
}

func (f *FileJudgmentStore) All() ([]*Judgment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fh, err := os.Open(f.path)
	if os.IsNotExist(err) {
		return nil, nil // 아직 아무것도 안 쌓였다. 오류가 아니다
	}
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	var out []*Judgment
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r record
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("%s:%d 읽을 수 없다: %w", f.path, n, err)
		}
		if r.Org != string(f.org) {
			return nil, fmt.Errorf("%w: %s:%d", ErrOrgMismatch, f.path, n)
		}
		j := r.J
		out = append(out, &j)
	}
	return out, sc.Err()
}

func (f *FileJudgmentStore) Get(id string) (*Judgment, error) {
	all, err := f.All()
	if err != nil {
		return nil, err
	}
	// **뒤에서 찾는다** — append-only라 같은 id가 여러 줄일 수 있고, 최신이 뒤에 있다.
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].ID == id {
			return all[i], nil
		}
	}
	return nil, nil
}

func (f *FileJudgmentStore) BySubject(subject string) ([]*Judgment, error) {
	all, err := f.All()
	if err != nil {
		return nil, err
	}
	var out []*Judgment
	for _, j := range all {
		if j.Subject == subject {
			out = append(out, j)
		}
	}
	return out, nil
}
