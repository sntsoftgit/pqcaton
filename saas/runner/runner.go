// Package runner — 고객망에서 도는 러너.
//
// 하는 일은 **바깥과 이야기하는 것 하나뿐**이다. 대상 노드에 붙어 collector를 반입·실행·
// 회수하는 일은 pqcota의 참조 플레이북이 한다 — 자체 원격 실행 엔진을 두지 않는다는 그쪽
// 규정을 그대로 따른다.
//
// 상태를 갖지 않는다. 스케줄에 깨어나 [RunOnce]를 한 번 돌고 끝난다 — 죽으면 다음 스케줄에
// 다시 뜨면 그만이고, 진행 중이던 작업은 컨트롤 플레인의 점유 만료가 회수한다.
package runner

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Version — 러너 버전. 컨트롤 플레인이 옛 러너를 거를 때 본다.
const Version = "0.1.0"

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
}

// 설정 파일의 키. 이름은 설치 문서와 같은 것을 쓴다 — 두 벌이 되면 어긋난다.
const (
	keyAPI        = "PQCATON_API"
	keyToken      = "PQCATON_TOKEN"
	keyRunnerID   = "PQCATON_RUNNER_ID"
	keyResultsDir = "PQCATON_RESULTS_DIR"
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

	var c Config
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
		switch k {
		case keyAPI:
			c.API = v
		case keyToken:
			c.Token = v
		case keyRunnerID:
			c.RunnerID = v
		case keyResultsDir:
			c.ResultsDir = v
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, err
	}
	return c, c.check()
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
	JobID    string // 받아 온 작업. 없었으면 빈 값
	Kind     string
	Files    int    // 올린 결과 파일 수
	Accepted int    // 컨트롤 플레인이 적재한 수
	Job      string // 작업 처리 결과 — closed · not-found · not-leased …
}

// RunOnce — 한 번 돈다. 스케줄이 이것을 부른다.
//
// 순서가 이렇다는 것이 요점이다 — **작업을 먼저 묻고, 결과를 올리며 그 작업을 닫는다.**
// 올리기와 닫기가 한 왕복인 이유는 §6.2.1에 있다: 나누면 "결과는 올렸는데 닫지 못한"
// 구간이 생기고, 그 작업은 만료돼 한 번 더 배포된다.
//
// **작업이 없어도 결과는 올린다.** 스케줄이 돌린 관측이 결과 디렉터리에 남아 있을 수 있고,
// 그것을 다음 작업이 올 때까지 묵혀 둘 이유가 없다.
func RunOnce(c Config, cl *Client, log *slog.Logger) (Report, error) {
	var rep Report

	job, ok, err := cl.NextJob(c.RunnerID)
	if err != nil {
		return rep, fmt.Errorf("작업 조회: %w", err)
	}
	if ok {
		rep.JobID, rep.Kind = job.ID, job.Kind
		log.Info("작업을 받았다", "job", job.ID, "kind", job.Kind, "targets", len(job.Targets))
	}

	files, err := resultFiles(c.ResultsDir)
	if err != nil {
		return rep, fmt.Errorf("결과 디렉터리: %w", err)
	}
	if len(files) == 0 {
		if ok {
			// 작업은 받았는데 올릴 것이 없다. 닫지 않는다 — 만료가 회수한다.
			log.Warn("작업을 받았는데 결과가 없다", "job", job.ID, "dir", c.ResultsDir)
		}
		return rep, nil
	}

	res, err := cl.SendResults(c.RunnerID, rep.JobID, files)
	if err != nil {
		// **파일을 그대로 둔다.** 다음 실행이 다시 올린다 — 같은 결과는 멱등이 접는다.
		return rep, fmt.Errorf("결과 전송: %w", err)
	}
	rep.Files, rep.Accepted, rep.Job = len(files), res.Accepted, res.Job

	// 올린 것은 옮긴다. 안 옮기면 매번 다시 올리게 되고, 멱등이 접어 주더라도
	// 그만큼 러너와 경계가 헛일을 한다.
	if err := archive(c.ResultsDir, files); err != nil {
		log.Error("올린 결과를 옮기지 못했다 — 다음 실행에 다시 올라간다", "err", err)
	}
	log.Info("결과를 올렸다", "files", rep.Files, "accepted", rep.Accepted,
		"duplicate", res.Duplicate, "rejected", res.Rejected, "job", res.Job)
	return rep, nil
}

// resultFiles — 결과 디렉터리의 `*.json`. 하위 디렉터리는 보지 않는다(`sent/`가 거기 있다).
func resultFiles(dir string) ([]string, error) {
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

// sentDir — 올린 결과를 옮겨 두는 곳. 지우지 않는 이유는 하나다 — **올린 것이 정말
// 들어갔는지 나중에 확인할 근거가 남아야 한다.** 정리는 운영자 몫이다.
const sentDir = "sent"

func archive(dir string, files []string) error {
	dst := filepath.Join(dir, sentDir)
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
