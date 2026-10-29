// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 시작.
//
// 두 시작 모드가 있고 **섞지 않는다**.
//
//   - 작업 폴더 모드: `-workspace <dir>` 또는 인자 없음(기본 `~/pqcaton`). 파일 위치가 정해져 있다
//     (workspaceLayout). 백업은 그 폴더 하나를 통째로 복사하면 된다. 경로 플래그를 함께 주면 시작하지
//     않는다 — 폴더 밖 파일을 가리키면 「폴더 하나가 전부」라는 약속이 깨진다.
//   - 명시 모드: 위치 인자 세션과 경로 플래그. v0.22까지와 같다.
//
// 동작 플래그(`-addr` · `-no-browser` · `-org`)는 두 모드 모두 받는다. 포트를 연 뒤 브라우저를 열고(실패해도
// 서버는 계속), 이미 같은 상태 디렉토리로 떠 있으면 새로 띄우지 않고 떠 있는 화면을 연다.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// 작업 폴더 안의 이름. 바꾸면 운영자 안내서와 백업 절차가 함께 바뀐다.
const (
	wsSession      = "session.json"
	wsDecl         = "declaration.json"
	wsResults      = "results"
	wsLayers       = "layers"
	wsPolicy       = "asset-scope.csv"
	wsScopeAssets  = "scope-assets.csv"
	wsScopeSession = "scope-session.json"
	wsPlan         = "plan.json"
	wsJudgments    = "judgments.jsonl"
	wsHosts        = "hosts.csv"
	wsCollectors   = "collectors"
	wsObservations = "observations"
	defaultWsName  = "pqcaton"
	addrFile       = "screen.addr"
)

// pathFlags: 경로를 정하는 플래그. 작업 폴더 모드와 함께 줄 수 없다.
var pathFlags = []string{"decl", "results", "layers", "base", "scope", "scope-assets", "scope-out", "judgments", "plan", "state-dir"}

// startConfig: 시작 설정. parseStart가 만들고 main이 쓴다.
type startConfig struct {
	addr, org, stateDir  string
	noBrowser            bool
	open                 bool // 떠 있는 화면을 열기만 한다(실행기)
	orgGiven             bool // -org를 직접 주었나
	recoverStatus        bool
	diagnostics          string
	workspace            string // 작업 폴더 모드면 그 절대 경로
	session, decl        string
	results, scopeAssets string
	scope, base          string
	layers               []string
	scopeOut, judgments  string
	plan                 string
	hosts                string // 접근 정보. 작업 폴더 모드에서만 있다
	collectors           string // 수집기 폴더. 작업 폴더 모드에서 주지 않으면 그 안의 collectors
	approvalKey          string // 승인 서명 키 파일(작업 폴더 밖)
}

// errUsage: 사용법이 틀렸다(종료 코드 2).
var errUsage = errors.New("usage")

// parseStart: 인자를 읽어 시작 설정을 만든다. 작업 폴더 모드면 폴더를 만들고(0700) 정해진 위치를 채운다.
func parseStart(args []string, stderr io.Writer) (startConfig, error) {
	fs := flag.NewFlagSet("pqcaton-ui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c startConfig
	fs.StringVar(&c.addr, "addr", "127.0.0.1:8765", "address to listen on (loopback only)")
	fs.BoolVar(&c.noBrowser, "no-browser", false, "do not open a browser after the screen starts")
	fs.BoolVar(&c.open, "open", false, "open the screen already running for this workspace in a browser, waiting up to 15s for it to answer; never starts one (the desktop launcher uses this). With -no-browser, only print its address")
	ws := fs.String("workspace", "", "workspace folder: every file sits at a fixed name inside it (default with no arguments: ~/pqcaton). Cannot be combined with the path flags")
	fs.StringVar(&c.decl, "decl", "", "declaration file (declaration.json). Given this, the declaration screen opens")
	fs.StringVar(&c.results, "results", "", "directory of collected results. With -decl, the reconciliation screen opens")
	fs.StringVar(&c.scopeAssets, "scope-assets", "", "asset-scope policy (scope-assets.csv) — the SAME file pqcota-ingest was given, or snapshot fingerprints will not match the central history")
	fs.StringVar(&c.scope, "scope", "", "asset scope session file. With -layers, the screen raises one itself")
	layerList := fs.String("layers", "", "asset scope layer CSVs, comma separated. They stack in the order given (org, environment, node group), and **the later layer wins** when rules clash. Given these, rules can be edited on the screen")
	fs.StringVar(&c.base, "base", "", "the policy CSV in force. Given this, only changed rules come up for review")
	fs.StringVar(&c.scopeOut, "scope-out", "asset-scope.csv", "file to write the finalized scope policy to")
	fs.StringVar(&c.judgments, "judgments", "", "file to append judgments to on finalize (JSONL, append-only)")
	fs.StringVar(&c.org, "org", "local", "organization the judgments are bound to")
	fs.StringVar(&c.plan, "plan", "plan.json", "file to write the finalized plan to")
	fs.BoolVar(&c.recoverStatus, "recover-status", false, "print what unfinished transactions left behind (files, expected and current hashes, prepared copies) and exit. Changes nothing")
	fs.StringVar(&c.diagnostics, "diagnostics", "", "with -recover-status: also write the report as JSON to this new file (0600)")
	fs.StringVar(&c.collectors, "collectors", "", "collector folder for running observations from the screen: linux-amd64/, linux-arm64/, windows-amd64/ and collector.jar as pqcota-discovery builds them (default in a workspace: its collectors folder)")
	fs.StringVar(&c.approvalKey, "approval-key", "", "approval signing key for the 06 screen: the output of pqcota-keygen saved to a file outside the workspace, readable only by its owner (chmod 600)")
	fs.StringVar(&c.stateDir, "state-dir", "", "state directory holding transaction records and receipts (default: .pqcaton next to the session file). Commands that change the same files must use the same one")

	// **위치 인자를 먼저 걷고 나머지를 플래그로 넘긴다.** 표준 flag는 첫 비플래그에서 파싱을 멈추므로, 그냥
	// 두면 `pqcaton-ui session.json -addr ...`의 -addr이 표시 없이 무시된다. 다른 명령들과 같은 규칙이다.
	pos, flags := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		return c, errUsage
	}
	// **플래그 뒤에 남은 인자는 받지 않는다.** 표준 flag는 첫 비플래그에서 멈추므로 `-addr x session.json
	// -decl …`의 세션과 그 뒤는 버려진다. 그대로 두면 인자가 없는 것으로 보고 작업 폴더로 뜬다.
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "❌ %q comes after a flag; put the session file first: pqcaton-ui <session.json> -decl …\n", fs.Arg(0))
		return c, errUsage
	}
	c.layers = splitPaths(*layerList)
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	c.orgGiven = set["org"]

	if *ws == "" && len(pos) > 0 {
		c.session = pos[0]
		return c, nil
	}
	// 작업 폴더 모드.
	if len(pos) > 0 {
		fmt.Fprintln(stderr, "❌ give either a session file or -workspace, not both")
		return c, errUsage
	}
	var given []string
	for _, name := range pathFlags {
		if set[name] {
			given = append(given, "-"+name)
		}
	}
	if len(given) > 0 {
		fmt.Fprintf(stderr, "❌ a workspace fixes where every file is; %s cannot be combined with it.\n", strings.Join(given, " "))
		fmt.Fprintln(stderr, "   Either drop them, or start with a session file and the path flags instead of a workspace.")
		return c, errUsage
	}
	dir := *ws
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return c, fmt.Errorf("cannot find the home folder for the default workspace: %w", err)
		}
		dir = filepath.Join(home, defaultWsName)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return c, err
	}
	if err := ensureWorkspace(abs, stderr); err != nil {
		return c, err
	}
	c.workspace = abs
	return c.inWorkspace()
}

// ensureWorkspace: 작업 폴더와 그 안의 results · layers를 만든다(소유자 전용). 이미 있으면 그대로 두고, 남이
// 읽을 수 있으면 경고만 한다(권한을 바꾸지 않는다).
func ensureWorkspace(dir string, stderr io.Writer) error {
	for _, d := range []string{dir, filepath.Join(dir, wsResults), filepath.Join(dir, wsLayers)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("cannot create the workspace %s: %w", d, err)
		}
	}
	if fi, err := os.Stat(dir); err == nil && fi.Mode().Perm()&0o077 != 0 && runtime.GOOS != "windows" {
		fmt.Fprintf(stderr, "warning: the workspace %s is open to other users (%#o); chmod 700 %s keeps it to you\n", dir, fi.Mode().Perm(), dir)
	}
	return nil
}

// inWorkspace: 정해진 위치를 채운다. 있는 파일만 그 화면을 연다.
func (c startConfig) inWorkspace() (startConfig, error) {
	at := func(name string) string { return filepath.Join(c.workspace, name) }
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	c.session = at(wsSession)
	c.decl = at(wsDecl)
	c.results = at(wsResults)
	c.plan = at(wsPlan)
	c.judgments = at(wsJudgments)
	c.hosts = at(wsHosts)
	if c.collectors == "" {
		c.collectors = at(wsCollectors)
	}
	c.scopeOut = at(wsPolicy)
	c.stateDir = ""
	if exists(at(wsScopeAssets)) {
		c.scopeAssets = at(wsScopeAssets)
	}
	ents, err := os.ReadDir(at(wsLayers))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return c, err
	}
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".csv") {
			c.layers = append(c.layers, filepath.Join(at(wsLayers), e.Name()))
		}
	}
	sort.Strings(c.layers)
	// 스코프 화면은 늘 연다. 계층이 없으면 화면이 계층 만들기를 낸다.
	c.scope = at(wsScopeSession)
	if exists(at(wsPolicy)) {
		c.base = at(wsPolicy)
	}
	return c, nil
}

// server: 설정으로 서버를 만든다(아직 상태 디렉토리는 열지 않는다).
func (c startConfig) server() *server {
	return &server{path: c.session, decl: c.decl, results: c.results, scope: c.scope,
		layers: c.layers, base: c.base, scopeAssets: c.scopeAssets, judgments: c.judgments,
		org: c.org, planOut: c.plan, scopeOut: c.scopeOut, workspace: c.workspace, hosts: c.hosts, collectors: absPath(c.collectors),
		approvalKey: absPath(c.approvalKey),
		orgFromDecl: c.workspace != "" && !c.orgGiven}
}

// orgName: 판정이 묶이는 조직.
func (s *server) orgName() string {
	s.orgMu.RLock()
	defer s.orgMu.RUnlock()
	return s.org
}

// adoptOrg: 작업 폴더 모드에서 -org를 주지 않았으면 선언의 조직을 서버의 조직으로 삼는다. 세션이 쓰는 규칙
// (선언에 적힌 조직)과 원장 · 계획 출력이 같은 조직을 말하게 한다. 그 밖에는 아무것도 하지 않는다.
func (s *server) adoptOrg(org string) {
	if !s.orgFromDecl || strings.TrimSpace(org) == "" {
		return
	}
	s.orgMu.Lock()
	s.org = strings.TrimSpace(org)
	s.orgMu.Unlock()
}

// diagInputs: 작업 폴더 모드에서 입력 파일을 읽어 본다. 읽지 못한 것을 역할과 함께 돌려준다. **있는데 깨진 것만**
// 문제다 — 없는 것은 화면이 상태로 보인다(선언 만들기 · 결과 폴더 안내).
func (s *server) diagInputs() []diagProblem {
	var out []diagProblem
	check := func(role, path string, load func(string) error) {
		if path == "" {
			return
		}
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return
		}
		if err := load(path); err != nil {
			out = append(out, diagProblem{role: role, path: path, err: err})
		}
	}
	check("declaration", s.decl, func(p string) error { _, err := decl.Load(p); return err })
	check("session", s.path, func(p string) error { _, err := review.Load(p); return err })
	check("scope-session", s.scope, func(p string) error { _, err := scope.LoadSession(p); return err })
	check("policy", s.base, func(p string) error { _, err := scope.LoadPolicyFile(p); return err })
	check("scope-assets", s.scopeAssets, func(p string) error { _, err := review.LoadAssetPolicy(p); return err })
	for _, l := range s.layerFiles() {
		check("layer", l, func(p string) error { _, err := scope.LoadLayers([]string{p}); return err })
	}
	return out
}

// diagProblem: 진단 전용 모드의 까닭 하나.
type diagProblem struct {
	role, path string
	err        error
}

// layerFiles: 지금의 계층 CSV. 작업 폴더 모드에서는 화면이 계층을 만들 수 있으므로 읽을 때마다 `layers/`를 다시
// 훑는다(이름 순). 명시 모드는 시작할 때 받은 그대로다.
func (s *server) layerFiles() []string {
	if s.workspace == "" {
		return s.layers
	}
	dir := filepath.Join(s.workspace, wsLayers)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".csv") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// scopeMissing: 작업 폴더에 계층도 스코프 세션도 아직 없다. 스코프 화면은 계층 만들기를 낸다.
func (s *server) scopeMissing() bool {
	if s.workspace == "" || len(s.layerFiles()) > 0 {
		return false
	}
	_, err := os.Stat(s.scope)
	return errors.Is(err, fs.ErrNotExist)
}

// declReady: 선언 파일이 있는가. 작업 폴더 모드에서는 없을 수 있다(화면이 만들기를 낸다).
func (s *server) declReady() bool {
	if s.decl == "" {
		return false
	}
	_, err := os.Stat(s.decl)
	return err == nil
}

// openRunning: 같은 상태 디렉토리로 떠 있는 화면을 연다. **적힌 주소를 믿지 않는다**: 그 주소의 서버가 같은
// 상태 디렉토리의 ID를 답할 때까지 기다린 뒤에 연다(runningAt). 비정상 종료가 남긴 낡은 주소, 아직 포트를 열지
// 않은 서버, 같은 포트의 다른 프로그램을 열지 않는다. 기다려도 답이 없으면 열지 않고 실패한다.
func openRunning(stateDir string, noBrowser bool, stderr io.Writer) error {
	deadline := time.Now().Add(openWait)
	for {
		url, err := runningAt(stateDir)
		if err == nil {
			fmt.Fprintf(stderr, "the screen is running: %s\n", url)
			if !noBrowser {
				openBrowser(url, stderr)
			}
			return nil
		}
		if errors.Is(err, errNotLoopback) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(openPoll)
	}
}

// openWait · openPoll: 떠 있는 화면이 답하기를 기다리는 시간과 간격. 시험이 줄인다.
var (
	openWait = 15 * time.Second
	openPoll = 200 * time.Millisecond
)

var errNotLoopback = errors.New("not a loopback address")

// healthClient: 떠 있는 화면에 묻는 클라이언트. 답이 없는 주소에서 오래 머물지 않는다.
var healthClient = &http.Client{Timeout: time.Second}

// runningAt: 적힌 주소의 서버가 이 상태 디렉토리의 화면이면 그 주소를 돌려준다.
func runningAt(stateDir string) (string, error) {
	id, err := store.ReadStateID(stateDir)
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("no screen has used %s yet", stateDir)
	}
	b, err := os.ReadFile(filepath.Join(stateDir, addrFile))
	if err != nil {
		return "", fmt.Errorf("no running screen has recorded its address in %s", stateDir)
	}
	addr := strings.TrimSpace(string(b))
	if !loopback(addr) {
		return "", fmt.Errorf("%w: the recorded address of the running screen (%s); not opening it", errNotLoopback, addr)
	}
	res, err := healthClient.Get("http://" + addr + ui.HealthPath)
	if err != nil {
		return "", fmt.Errorf("the recorded address %s does not answer: %w", addr, err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 256))
	if res.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != id {
		return "", fmt.Errorf("the recorded address %s answers for another screen", addr)
	}
	return "http://" + addr + "/", nil
}

// recordAddr: 떠 있는 화면의 주소를 상태 디렉토리에 적는다(0600). 다시 실행하면 이 주소를 연다.
func recordAddr(stateDir, addr string) error {
	p := filepath.Join(stateDir, addrFile)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(addr+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// openBrowser: 브라우저를 연다. 실패해도 서버는 계속 돈다(경고만).
var openBrowser = func(url string, stderr io.Writer) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		fmt.Fprintf(stderr, "open %s in a browser\n", url)
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "warning: could not open a browser (%v); open %s yourself\n", err, url)
		return
	}
	go func() { _ = cmd.Wait() }()
}

// listen: 포트를 연다. 화면 주소를 돌려준다.
func listen(addr string) (net.Listener, string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, "", err
	}
	return ln, ln.Addr().String(), nil
}

// stateDirOf: 이 설정이 쓰는 상태 디렉토리.
func (c startConfig) stateDirOf() string {
	if c.stateDir != "" {
		return c.stateDir
	}
	return store.DefaultDir(c.session)
}

// absPath: 상대 경로를 지금 자리에서 절대 경로로 바꾼다. 관측은 작업 자리에서 돌아서 상대 경로가 다른 곳을 가리킨다.
func absPath(p string) string {
	if p == "" {
		return ""
	}
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
