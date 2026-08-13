// Package runner — 고객망에서 도는 러너.
//
// 하는 일은 **바깥과 이야기하는 것 하나뿐**이다. 대상 노드에 붙어 collector를 반입·실행·
// 회수하는 일은 pqcota의 참조 플레이북이 한다 — 자체 원격 실행 엔진을 두지 않는다는 그쪽
// 규정을 그대로 따른다.
//
// 상태를 갖지 않는다. 스케줄에 깨어나 [RunOnce]를 한 번 돌고 끝난다 — 죽으면 다음 스케줄에
// 다시 뜨면 그만이다. **컨트롤 플레인에 할 일을 묻지 않는다**: 스케줄이 곧 관측 주기이고,
// 무엇을 볼지는 운영자가 채운 인벤토리가 정한다.
package runner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Version — 러너 버전. 컨트롤 플레인이 옛 러너를 거를 때 본다.
const Version = "0.1.0"

// 결과 디렉터리 아래 두 자리. 성격이 반대라 보존 기간도 다르다.
const (
	// sentDir — 올린 것. 정말 들어갔는지의 **원본 근거는 컨트롤 플레인에 있다**(적재됐거나
	// 거절 사유가 남는다). 여기 사본은 며칠짜리 확인용이라 짧게 둔다.
	sentDir = "sent"
	// badDir — 못 올린 것. 올라간 적이 없어 **여기 사본이 유일한 증거다** — 왜 깨졌는지
	// 보려면 그것이 남아 있어야 한다. 그래서 더 길게 둔다.
	badDir = "bad"
)

// 보존 기간 기본값(일). 0이면 지우지 않는다.
const (
	DefaultSentKeepDays = 7
	DefaultBadKeepDays  = 30
)

// Config — 러너 설정. 설치 플레이북이 파일로 놓고, 운영자가 대상 목록을 채운다.
type Config struct {
	// API — 컨트롤 플레인 주소. 러너가 **밖으로 거는** 유일한 상대다.
	API string
	// Token — 러너 토큰. **조직과 영역이 여기서 유도된다** — 러너는 둘 다 주장하지 않는다.
	Token string
	// RunnerID — 누가 가져갔나. 비면 호스트이름을 쓴다.
	//
	// 지금은 러너가 스스로 대는 값이다. 토큰이 조직·영역 단위라 유도할 수 없다 —
	// `register`가 러너를 토큰에 묶으면 그때 닫힌다.
	RunnerID string
	// ResultsDir — 플레이북이 결과 JSON을 모아 두는 곳.
	ResultsDir string
	// SentKeepDays — 올린 결과를 며칠 두나.
	//
	// **짧게 둔다.** 고객 노드의 암호 자산 정보라 러너에 쌓일수록 반출 위험이 커지고,
	// 디스크가 차면 플레이북이 결과를 못 쓴다.
	SentKeepDays int
	// BadKeepDays — 못 올린 결과를 며칠 두나. `sent`보다 길다(위 [badDir]).
	BadKeepDays int

	// Playbook — 스케줄마다 돌릴 플레이북. pqcota의 참조 플레이북을 그대로 쓴다.
	// 비면 돌리지 않는다 — 결과를 올리는 일만 한다.
	Playbook string
	// Inventory — 그 플레이북에 넘길 대상 목록. **운영자가 채운 파일이다.**
	Inventory string
	// Ansible — 부를 명령. 비면 [DefaultAnsible].
	Ansible string

	// AddrKey — 주소를 토큰으로 바꿀 조직 단위 키(§6.3.1).
	//
	// **이 키는 러너에만 있다.** 우리 쪽에서는 토큰을 주소로 되돌릴 수 없고, 같은
	// 주소는 늘 같은 토큰이 되므로 영역 간에 같은 상대를 이어 붙일 수 있다.
	// 없으면 토큰이 안 붙는다 — 그 대가는 [RunOnce]가 로그로 알린다.
	AddrKey string
}

// 설정 파일의 키. 이름은 설치 문서와 같은 것을 쓴다 — 두 벌이 되면 어긋난다.
const (
	keyAPI        = "PQCATON_API"
	keyToken      = "PQCATON_TOKEN"
	keyRunnerID   = "PQCATON_RUNNER_ID"
	keyResultsDir = "PQCATON_RESULTS_DIR"
	keySentKeep   = "PQCATON_SENT_KEEP_DAYS"
	keyBadKeep    = "PQCATON_BAD_KEEP_DAYS"
	keyPlaybook   = "PQCATON_PLAYBOOK"
	keyInventory  = "PQCATON_INVENTORY"
	keyAnsible    = "PQCATON_ANSIBLE"
	keyAddrKey    = "PQCATON_ADDR_KEY"
)

var (
	// ErrNoToken — 토큰이 없다. 조직을 유도할 수 없으므로 아무것도 못 한다.
	ErrNoToken = errors.New("토큰이 없다")
	// ErrNoAPI — 컨트롤 플레인 주소가 없다.
	ErrNoAPI = errors.New("컨트롤 플레인 주소가 없다")
)

// LoadConfig — `KEY=value` 파일을 읽는다.
//
// **토큰을 명령줄에 두지 않기 위한 형식이다.** 명령줄에 넣으면 셸 히스토리와 `ps`에 남는다.
// 파일은 0600으로 두고 이 함수가 읽는다.
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	c := Config{SentKeepDays: DefaultSentKeepDays, BadKeepDays: DefaultBadKeepDays}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)

		var perr error
		switch k {
		case keyAPI:
			c.API = v
		case keyToken:
			c.Token = v
		case keyRunnerID:
			c.RunnerID = v
		case keyResultsDir:
			c.ResultsDir = v
		case keySentKeep:
			c.SentKeepDays, perr = days(v)
		case keyBadKeep:
			c.BadKeepDays, perr = days(v)
		case keyPlaybook:
			c.Playbook = v
		case keyInventory:
			c.Inventory = v
		case keyAnsible:
			c.Ansible = v
		case keyAddrKey:
			c.AddrKey = v
		}
		if perr != nil {
			return Config{}, fmt.Errorf("%s: %w", k, perr)
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, err
	}
	return c, c.check()
}

// days — 보존 기간. 음수는 거절한다 — 오타를 "지우지 않음"으로 삼키면 디스크가 조용히 찬다.
func days(v string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("일수가 아니다: %q", v)
	}
	if n < 0 {
		return 0, fmt.Errorf("음수다: %d", n)
	}
	return n, nil
}

func (c *Config) check() error {
	if c.Token == "" {
		return ErrNoToken
	}
	if c.API == "" {
		return ErrNoAPI
	}
	if c.RunnerID == "" {
		h, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("러너 id를 정할 수 없다: %w", err)
		}
		c.RunnerID = h
	}
	return nil
}

// Report — 한 번 돈 결과.
type Report struct {
	Files    int  // 올린 결과 파일 수
	Bad      int  // 읽을 수 없어 치운 파일 수
	Played   bool // 플레이북을 돌렸나
	Accepted int  // 컨트롤 플레인이 적재한 수

	// 연결확인. 올린 수와 그 판정이다(§6.3).
	Enrollments int
	Enrolled    int
	Held        int
}

// RunOnce — 한 번 돈다. 스케줄이 이것을 부른다.
//
// **스케줄이 곧 관측 주기다.** 컨트롤 플레인에 할 일을 묻지 않는다 — 무엇을 볼지는 운영자가
// 채운 인벤토리가 정하고, 언제 볼지는 이 프로세스를 깨우는 스케줄이 정한다. 그래서 러너는
// **말할 줄만 알면 된다.**
//
// 올리는 자리는 둘이고 **서로 독립이다.** 하나가 실패해도 다른 하나는 올라간다 — 연결확인이
// 안 올라갔다고 이미 끝난 관측까지 묵힐 이유가 없다.
func RunOnce(c Config, cl *Client, log *slog.Logger) (Report, error) {
	var rep Report

	// **같은 결과 디렉터리를 쓰는 실행은 하나뿐이다.** 스케줄이 관측 시간보다 촘촘하면
	// 이전 실행이 아직 도는 채로 다음이 뜬다(cron은 그것을 보지 않는다).
	release, err := lock(c.ResultsDir)
	if err != nil {
		return rep, err // ErrAlreadyRunning이면 부르는 쪽이 실패로 세지 않는다
	}
	defer release()
	defer sweepBoth(c, log) // 실패해도 청소는 한다 — 디스크가 차면 관측이 멈춘다

	var playErr error
	if c.Playbook != "" {
		if playErr = runPlaybook(c, log); playErr == nil {
			rep.Played = true
		}
		// 실패해도 계속한다. **반쯤 나온 결과라도 올린다** — 버리면 그 관측은 사라지고,
		// 무엇이 왜 안 됐는지는 컨트롤 플레인의 완전성 맵에서 봐야 한다.
	}

	if err := sendEnrollments(c, cl, &rep, log); err != nil {
		log.Error("연결확인을 올리지 못했다 — 다음 실행이 다시 올린다", "err", err)
	}
	if err := sendResults(c, cl, &rep, log); err != nil {
		return rep, err
	}
	return rep, playErr
}

// sendResults — 관측 결과를 올리고 옮긴다.
func sendResults(c Config, cl *Client, rep *Report, log *slog.Logger) error {
	files, err := jsonFiles(c.ResultsDir)
	if err != nil {
		return fmt.Errorf("결과 디렉터리: %w", err)
	}
	payloads, good, bad := read(files)
	if len(bad) > 0 {
		// **하나가 깨졌다고 나머지를 버리지 않는다.** 다만 조용히 넘기지도 않는다 —
		// 그대로 두면 다음 실행마다 같은 파일에 걸려 그 디렉터리가 영영 안 올라간다.
		rep.Bad += len(bad)
		log.Warn("읽을 수 없는 결과를 치운다 — 왜 깨졌는지 봐야 한다",
			"files", bad, "moved_to", filepath.Join(c.ResultsDir, badDir))
		if err := move(c.ResultsDir, badDir, bad); err != nil {
			log.Error("치우지 못했다 — 다음 실행에서 또 걸린다", "err", err)
		}
	}
	if len(payloads) == 0 {
		return nil
	}

	res, err := cl.SendResults(c.RunnerID, payloads)
	if err != nil {
		// **파일을 그대로 둔다.** 다음 실행이 다시 올린다 — 같은 결과는 멱등이 접는다.
		return fmt.Errorf("결과 전송: %w", err)
	}
	rep.Files, rep.Accepted = len(good), res.Accepted

	// 올린 것은 옮긴다. 안 옮기면 매번 다시 올리게 되고, 멱등이 접어 주더라도
	// 그만큼 러너와 경계가 헛일을 한다.
	if err := move(c.ResultsDir, sentDir, good); err != nil {
		log.Error("올린 결과를 옮기지 못했다 — 다음 실행에 다시 올라간다", "err", err)
	}
	// **`off_scope`를 함께 찍는다.** 이 값이 없으면 `accepted:0`만 보이고, 왜 안
	// 들어왔는지는 컨트롤 플레인 DB를 열어야 안다 — 운영자 눈에는 아무 일도 안 일어난
	// 것으로 보인다.
	log.Info("결과를 올렸다", "files", rep.Files, "accepted", rep.Accepted,
		"duplicate", res.Duplicate, "rejected", res.Rejected,
		"unverified", res.Unverified, "off_scope", res.OffScope)
	return nil
}

// sendEnrollments — 연결확인을 올리고 옮긴다.
func sendEnrollments(c Config, cl *Client, rep *Report, log *slog.Logger) error {
	enr, err := readEnrollments(c.ResultsDir, c.AddrKey)
	if err != nil {
		return fmt.Errorf("등재 디렉터리: %w", err)
	}
	if enr.SawAddr && c.AddrKey == "" {
		// 조용히 넘기면, 영역 간 엣지를 이어 붙일 표가 없다는 것을 **몇 달 뒤에**
		// 안다. 그때는 전 노드를 다시 등재해야 한다(§6.3.1).
		log.Warn("주소는 있는데 "+keyAddrKey+"가 없다 — 주소 토큰 없이 등재한다",
			"dir", filepath.Join(c.ResultsDir, enrollDir))
	}
	if len(enr.Bad) > 0 {
		rep.Bad += len(enr.Bad)
		log.Warn("읽을 수 없는 연결확인을 치운다", "files", enr.Bad)
		if err := move(filepath.Join(c.ResultsDir, enrollDir), badDir, enr.Bad); err != nil {
			log.Error("치우지 못했다 — 다음 실행에서 또 걸린다", "err", err)
		}
	}
	if len(enr.Items) == 0 {
		return nil
	}

	res, err := cl.SendEnrollments(c.RunnerID, enr.Items)
	if err != nil {
		return fmt.Errorf("연결확인 전송: %w", err)
	}
	rep.Enrollments, rep.Enrolled, rep.Held = len(enr.Items), res.Enrolled, res.Held
	if err := move(filepath.Join(c.ResultsDir, enrollDir), sentDir, enr.Good); err != nil {
		log.Error("올린 연결확인을 옮기지 못했다 — 다음 실행에 다시 올라간다", "err", err)
	}
	log.Info("연결확인을 올렸다", "sent", rep.Enrollments, "enrolled", res.Enrolled,
		"held", res.Held, "failed", res.FailedNodes,
		"refused", res.Refused, "refused_reason", res.RefusedReason)
	return nil
}

// read — 파일을 읽어 보낼 것과 치울 것으로 가른다.
//
// 러너는 **내용을 해석하지 않는다.** JSON인지만 본다 — 계약을 아는 쪽은 collector와 수신
// API이고, 러너가 중간에서 해석하면 버전이 어긋날 때 러너가 먼저 깨진다.
func read(files []string) (payloads []json.RawMessage, good, bad []string) {
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil || !json.Valid(raw) {
			bad = append(bad, f)
			continue
		}
		payloads = append(payloads, json.RawMessage(raw))
		good = append(good, f)
	}
	return payloads, good, bad
}

// jsonFiles — 그 디렉터리의 `*.json`. 하위 디렉터리는 보지 않는다(`sent`·`bad`·`enroll`이 거기 있다).
func jsonFiles(dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // 아직 한 번도 안 돌았다. 오류가 아니다
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out, nil
}

func move(dir, sub string, files []string) error {
	dst := filepath.Join(dir, sub)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Rename(f, filepath.Join(dst, filepath.Base(f))); err != nil {
			return err
		}
	}
	return nil
}

// sweepBoth — 두 자리를 각자의 보존 기간으로 청소한다. **등재 쪽도 같이 본다** —
// 빠뜨리면 그 디렉터리만 조용히 쌓여 디스크가 찬다.
func sweepBoth(c Config, log *slog.Logger) {
	for _, base := range []string{c.ResultsDir, filepath.Join(c.ResultsDir, enrollDir)} {
		sweep(filepath.Join(base, sentDir), c.SentKeepDays, log)
		sweep(filepath.Join(base, badDir), c.BadKeepDays, log)
	}
}

// sweep — 보존 기간이 지난 것을 지운다. 0이면 지우지 않는다.
//
// **지운 사실을 남긴다.** 조용히 사라지면, 나중에 그 결과를 찾는 사람이 *"올라가지 않았나"*
// 와 *"보존 기간이 지났나"* 를 구분할 수 없다.
func sweep(dir string, keepDays int, log *slog.Logger) {
	if keepDays <= 0 || dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // 아직 없다. 오류가 아니다
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	var gone int
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			log.Error("지우지 못했다", "file", e.Name(), "err", err)
			continue
		}
		gone++
	}
	if gone > 0 {
		log.Info("보존 기간이 지난 결과를 지웠다", "dir", dir, "files", gone, "keep_days", keepDays)
	}
}
