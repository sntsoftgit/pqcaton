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
var ErrOrgMismatch = errors.New("that file holds judgments from another organization")

// NewFileJudgmentStore — 조직을 지정해 연다. 빈 조직은 열리지 않는다(Mem·Pg판과 같은 규칙).
func NewFileJudgmentStore(o org.ID, path string) (*FileJudgmentStore, error) {
	if o == "" {
		return nil, org.ErrEmpty
	}
	if path == "" {
		return nil, errors.New("the judgment file path is empty")
	}
	return &FileJudgmentStore{org: o, path: path}, nil
}

// Org — 이 핸들이 묶인 조직.
func (f *FileJudgmentStore) Org() org.ID { return f.org }

// record — 파일에 실제로 쓰는 모양. 조직을 함께 적는다 — 읽을 때 거르기 위해서다.
type record struct {
	Org string       `json:"org"`
	J   judgmentWire `json:"judgment"`
}

// judgmentWire — Judgment 의 파일 모양. **ConfidenceEvaluated 를 포인터로 받는다.** Judgment 에는
// json 태그가 없어 Go 이름으로 직렬화되는데, bool 로 두면 옛 줄의 칸 부재가 false 로 읽힌다. 옛 행은
// 전부 평가된 값이었다(그때는 미평가라는 개념이 없었다). nil 이면 참, 명시적 false 만 미평가다.
// 쓸 때는 언제나 명시적으로 쓴다 - 새 파일에는 부재가 없게. 나머지 칸은 Judgment 와 같은 이름이다.
type judgmentWire struct {
	ID                  string
	Subject             string
	Conclusion          string
	Reviewer            string
	Signature           string
	BasisHash           string
	Confidence          float64
	ConfidenceEvaluated *bool `json:",omitempty"`
	DecidedAt           int64
	SessionID           string
	RecordKind          RecordKind `json:",omitempty"`
	// 파생 플래그는 저장하지 않는다. 옛 줄에 남아 있어도 읽을 때 버린다.
	NeedsReReview bool `json:",omitempty"`
	Stale         bool `json:",omitempty"`
}

func toWire(j Judgment) judgmentWire {
	ev := j.ConfidenceEvaluated
	return judgmentWire{ID: j.ID, Subject: j.Subject, Conclusion: j.Conclusion, Reviewer: j.Reviewer,
		Signature: j.Signature, BasisHash: j.BasisHash, Confidence: j.Confidence, ConfidenceEvaluated: &ev,
		DecidedAt: j.DecidedAt, SessionID: j.SessionID, RecordKind: j.RecordKind}
}

func fromWire(w judgmentWire) Judgment {
	ev := true // 칸이 없으면 옛 행이고, 옛 행은 전부 평가된 값이다
	if w.ConfidenceEvaluated != nil {
		ev = *w.ConfidenceEvaluated
	}
	return Judgment{ID: w.ID, Subject: w.Subject, Conclusion: w.Conclusion, Reviewer: w.Reviewer,
		Signature: w.Signature, BasisHash: w.BasisHash, Confidence: w.Confidence, ConfidenceEvaluated: ev,
		DecidedAt: w.DecidedAt, SessionID: w.SessionID, RecordKind: w.RecordKind}
}

func (f *FileJudgmentStore) Save(j *Judgment) error {
	if j == nil {
		return errors.New("the judgment is empty")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	// **O_APPEND만 쓴다.** 덮어쓸 방법을 두지 않는 것이 append-only를 지키는 자리다.
	fh, err := os.OpenFile(f.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer fh.Close()
	line, err := json.Marshal(record{Org: string(f.org), J: toWire(*j)})
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
			return nil, fmt.Errorf("%s:%d cannot be read: %w", f.path, n, err)
		}
		if r.Org != string(f.org) {
			// **무엇이 어긋났는지 적는다.** 대개 -org 를 저장할 때와 다르게 준 것인데,
			// "다른 조직의 판정이 있다"만 보면 파일이 오염된 줄 안다.
			return nil, fmt.Errorf("%w: %s:%d - this handle is %q but that line is %q",
				ErrOrgMismatch, f.path, n, f.org, r.Org)
		}
		j := fromWire(r.J)
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

func (f *FileJudgmentStore) BySessionID(sessionID string) ([]*Judgment, error) {
	if sessionID == "" {
		return nil, ErrNoSessionID
	}
	all, err := f.All()
	if err != nil {
		return nil, err
	}
	var out []*Judgment
	for _, j := range all {
		if j.SessionID == sessionID {
			out = append(out, j)
		}
	}
	return out, nil
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
