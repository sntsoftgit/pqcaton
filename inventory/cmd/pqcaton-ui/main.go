// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Command pqcaton-ui — 리뷰 큐와 선언을 사람이 다루는 화면.
//
// **파일에서 읽고 파일에 쓰는 껍데기다.** 화면 자체는 `pkg/inventory/ui`에 있고, 확정
// 관문은 `pkg/inventory/review`에 있다 — 컨트롤 플레인이 같은 화면과 같은 관문을
// 쓰고, 다른 것은 「어디서 읽고 누가 들어오나」뿐이다.
//
//	pqcaton-ui <session.json> [-decl declaration.json] [-results 디렉터리]
//	           [-layers corp.csv,prod.csv] [-base asset-scope.csv] [-scope scope-session.json]
//	           [-addr 127.0.0.1:8765] [-judgments 파일] [-org 이름]
//	           [-plan 파일] [-scope-out 파일]
//
// **탭 순서가 절차 순서다** — 선언 → 스코프 → 대조 → 리뷰 큐. 쓰는 사람이 다음에 무엇을
// 할지 화면이 알려 준다. 입력을 주지 않은 자리는 만들지 않는다.
//
// **라우팅은 chi, 화면은 templ, 부분 갱신은 htmx다.** 셋 다 허용적 라이선스이고
// 링크되는 모듈은 둘만 는다(전이 의존이 없다). htmx는 바이너리에 담겨 나가므로 망이
// 끊긴 기계에서도 그대로 뜬다 — 그리고 우리 라이선스 관문이 그 파일까지 본다.
//
// **루프백에서만 열린다.** 리뷰 큐에는 어느 노드가 무엇을 쓰는지가 그대로 있다 — 곧 그
// 조직의 공격면이다. 이 화면에는 인증이 없으므로 -addr가 루프백이 아니면 시작하지 않는다
// (checkBind). 다른 기계에서 쓰려면 SSH로 포트를 넘긴다. 열린 뒤에도 요청은 출처를 본다
// (guard.go): Host는 모든 요청에서, Origin과 CSRF 토큰은 바꾸는 요청에서 본다.
//
// **산출물은 여전히 파일이다.** 화면이 생겨도 무엇을 근거로 무엇을 정했는지가 사라지지
// 않는다 — 편집한 세션 파일과 선언, 확정 계획, 판정 원장이 그대로 남는다.
package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	kscope "github.com/randyinthedev-hash/pqcota-common/pkg/kernel/scope"
	"github.com/randyinthedev-hash/pqcota-common/pkg/org"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// serveHTTP: 열린 포트로 화면을 낸다. 시험이 바꿔 끼워 처리기를 받아 간다.
var serveHTTP = http.Serve

// run: 시작부터 끝까지. 종료 코드를 돌려준다.
func run(args []string, stdout, stderr io.Writer) int {
	c, err := parseStart(args, stderr)
	if errors.Is(err, errUsage) {
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "❌", err)
		return 1
	}
	// 복구 상태 보기는 화면을 열지 않는다. 서버가 떠 있어도 볼 수 있다(읽기만 한다).
	if c.recoverStatus {
		dir := c.stateDir
		if dir == "" && c.session != "" {
			dir = c.stateDirOf()
		}
		if dir == "" {
			fmt.Fprintln(stderr, "usage: pqcaton-ui [<session.json> | -workspace <dir>] -recover-status [-state-dir <dir>] [-diagnostics <file>]")
			return 2
		}
		if err := printRecoverStatus(stdout, dir, c.diagnostics); err != nil {
			fmt.Fprintln(stderr, "❌", err)
			return 1
		}
		return 0
	}
	// **주소부터 본다.** 파일을 읽기 전에 막아야 열리지 않을 화면 때문에 입력을 건드리지 않는다.
	if err := checkBind(c.addr); err != nil {
		fmt.Fprintln(stderr, "❌", err)
		return 2
	}
	s := c.server()
	if c.workspace == "" {
		if code := checkExplicit(&c, stderr); code != 0 {
			return code
		}
		s = c.server()
	} else {
		// 작업 폴더 모드: 깨진 입력은 시작을 막지 않고 진단 전용 화면으로 연다. 데스크톱에서 띄운 사람은
		// 표준오류를 볼 수 없다.
		s.diag = s.diagInputs()
		if s.orgFromDecl && len(s.diag) == 0 {
			if d, err := decl.Load(s.decl); err == nil {
				s.adoptOrg(d.Org)
			}
		}
	}
	if err := s.openState(c.stateDir); err != nil {
		if errors.Is(err, store.ErrBusy) {
			if err := openRunning(c.stateDirOf(), c.noBrowser, stderr); err != nil {
				fmt.Fprintln(stderr, "❌", err)
				return 1
			}
			return 0
		}
		fmt.Fprintln(stderr, "❌", err)
		return 1
	}
	if len(s.diag) > 0 {
		ref := newRef()
		for _, p := range s.diag {
			fmt.Fprintf(stderr, "error ref=%s cannot read the %s %s: %v\n", ref, p.role, p.path, p.err)
		}
		fmt.Fprintln(stderr, "   The screen opens for diagnostics only: saving is stopped until the files are fixed and the screen is restarted.")
		s.diagRef = ref
	}
	// **포트를 먼저 연다.** 그 뒤에 주소를 적고 브라우저를 연다 — 브라우저가 먼저 뜨면 빈 페이지를 본다.
	ln, addr, err := listen(c.addr)
	if err != nil {
		fmt.Fprintln(stderr, "❌", err)
		return 1
	}
	if s.st != nil {
		if err := recordAddr(s.st.Dir, addr); err != nil {
			fmt.Fprintln(stderr, "warning: could not record the screen address:", err)
		}
	}
	url := "http://" + addr + "/"
	where := "session " + c.session
	if c.workspace != "" {
		where = "workspace " + c.workspace
	}
	fmt.Fprintf(stderr, "screen: %s  (%s)\n", url, where)
	if !c.noBrowser {
		openBrowser(url, stderr)
	}
	// **끝낼 때(Ctrl+C · systemctl stop) 적어 둔 주소를 지운다.** 남겨 두면 다시 띄울 때 실행기가 아직 열리지
	// 않은 낡은 주소를 먼저 연다. 실행 잠금은 프로세스가 끝나면 풀린다.
	stopped := make(chan struct{})
	sig := stopSignal()
	go func() {
		if _, ok := <-sig; !ok {
			return
		}
		if s.st != nil {
			_ = os.Remove(filepath.Join(s.st.Dir, addrFile))
		}
		close(stopped)
		_ = ln.Close()
	}()
	err = serveHTTP(ln, s.handler())
	select {
	case <-stopped:
		if s.st != nil {
			s.st.Close()
		}
		fmt.Fprintln(stderr, "screen stopped")
		return 0
	default:
	}
	if err != nil {
		fmt.Fprintln(stderr, "❌", err)
		return 1
	}
	return 0
}

// stopSignal: 화면을 끝내는 신호(Ctrl+C · SIGTERM). 시험이 바꿔 낀다.
var stopSignal = func() <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	return c
}

// checkExplicit: 명시 모드의 입력 검사. v0.22까지와 같다 — 터미널에서 띄우는 길이라 표준오류로 알리고 멈춘다.
func checkExplicit(c *startConfig, stderr io.Writer) int {
	if c.session == "" {
		fmt.Fprintln(stderr, "usage: pqcaton-ui [-workspace <dir>] | <session.json> [-decl declaration.json] [-results <dir>] [-layers <layer.csv>,...] [-base <in-force.csv>] [-addr 127.0.0.1:8765] [-judgments <file>] [-org <name>] [-plan <file>]")
		return 2
	}
	if _, err := review.Load(c.session); err != nil {
		// **입력이 있으면 화면이 직접 연다.** 선언과 관측 결과가 곧 리뷰 큐의 입력이다.
		if c.decl == "" || c.results == "" {
			fmt.Fprintln(stderr, "❌ cannot read the review session:", err)
			fmt.Fprintln(stderr, "   Give it the declaration and the results and the screen raises one itself: -decl declaration.json -results <results-dir>")
			fmt.Fprintln(stderr, "   Or start with a workspace folder instead: pqcaton-ui -workspace <dir>")
			return 1
		}
	}
	if c.decl != "" {
		if _, err := decl.Load(c.decl); err != nil {
			fmt.Fprintln(stderr, "❌ cannot read the declaration file:", err)
			fmt.Fprintln(stderr, "   A workspace folder lets the screen create an empty declaration: pqcaton-ui -workspace <dir>")
			return 1
		}
	}
	if c.results != "" && c.decl == "" {
		// **선언 없이는 대조할 것이 없다.**
		fmt.Fprintln(stderr, "❌ -results needs -decl — reconciliation means matching against a declaration")
		return 2
	}
	if len(c.layers) > 0 && c.scope == "" {
		// **계층을 줬으면 세션은 화면이 만든다.**
		c.scope = defaultScopeSession
	}
	if c.scope != "" && len(c.layers) == 0 {
		if _, err := scope.LoadSession(c.scope); err != nil {
			fmt.Fprintln(stderr, "❌ cannot read the scope session:", err)
			fmt.Fprintln(stderr, "   Give it the layer CSVs and the screen raises one itself: -layers corp.csv,prod.csv -base asset-scope.csv")
			return 1
		}
	}
	if _, err := scope.LoadLayers(c.layers); err != nil {
		fmt.Fprintln(stderr, "❌ cannot read the layer CSVs:", err)
		return 1
	}
	// 정책은 읽을 수 있는지만 본다. **보관하지 않는다** - 쓸 때마다 읽는다(assetPolicy).
	if _, err := review.LoadAssetPolicy(c.scopeAssets); err != nil {
		fmt.Fprintln(stderr, "❌", err)
		return 2
	}
	return 0
}

// openState — 상태 디렉토리를 열고 이 서버가 유일한 작성자가 된다. 그다음 남은 준비 파일을 보고,
// 남은 트랜잭션을 복구하고, 입력에서 세운 세션을 파일로 남긴다.
//
// 저장을 지원하지 않는 플랫폼이면 보기 전용으로 연다. 네트워크 파일시스템 같은 다른 거절은 열지
// 않는다. **복구 필요가 남아 있으면 복구 상태로 연다.** 실행 잠금은 쥔 채로(명령도 막힌다) 정적
// 파일을 뺀 모든 요청에 503 복구 페이지를 낸다(recoveryGate). 어긋난 파일을 읽어 그린 화면은 믿을 수
// 없고, 그 위의 저장은 더 나쁘다. 세션도 파일로 남기지 않는다.
//
// 세션을 파일로 남기지 못하면 **시작하지 않는다.** 그대로 열면 읽을 때마다 세션 ID가 새로 나서
// 확정의 동일성이 서지 않는다.
func (s *server) openState(dir string) error {
	if dir == "" {
		dir = store.DefaultDir(s.path)
	}
	st, err := store.Open(dir, s.storeOpts)
	if errors.Is(err, store.ErrUnsupported) && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		fmt.Fprintln(os.Stderr, "warning: saving is not supported on this platform; the screen is view-only")
		return nil
	}
	if err != nil {
		return err
	}
	if err := st.AcquireRun(); err != nil {
		st.Close()
		return err
	}
	if err := st.CheckOrphans(s.targetDirs()); err != nil {
		st.Close()
		return err
	}
	if err := st.Recover(lockWait); err != nil {
		if !errors.Is(err, store.ErrNeedsRecovery) {
			st.Close()
			return err
		}
		fmt.Fprintln(os.Stderr, "❌", err)
		fmt.Fprintln(os.Stderr, "   The screen opens in recovery mode: every page answers 503 until this is resolved and the screen is restarted.")
		s.st, s.recovering = st, true
		return nil
	}
	s.st = st
	// 진단 전용이면 깨진 입력으로 세션을 세우지 않는다(화면이 까닭을 보인다).
	if len(s.diag) > 0 {
		return nil
	}
	if err := s.materialize(); err != nil {
		st.Close()
		s.st = nil
		return fmt.Errorf("could not write the session raised from the inputs, so the screen does not start: %w", err)
	}
	fmt.Fprintf(os.Stderr, "state: %s\n", st.Dir)
	return nil
}

// recoveryGate — 복구 상태면 정적 파일(CSS · JS)을 뺀 모든 요청이 처리기에 닿지 않는다. 화면(GET)은 복구 상태
// 화면(503)이고, 진단 파일만 내려받을 수 있다. 저장 요청은 503 거절이다.
func (s *server) recoveryGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case !s.recovering || strings.HasPrefix(r.URL.Path, ui.StaticPath):
			next.ServeHTTP(w, r)
		case r.Method == http.MethodGet && r.URL.Path == ui.RecoveryDiagPath:
			s.recoveryDiagnostics(w, r)
		case r.Method == http.MethodGet:
			s.recoveryPage(w, r)
		default:
			s.refuse(w, r, http.StatusServiceUnavailable, ui.RefusedRecovery, "/", nil)
		}
	})
}

// recoveryPage — 남은 기록마다 파일 이름 · 하려던 일 · 지금 상태를 보인다. 전체 경로와 해시는 싣지 않는다
// (진단 파일에 있다). 고치는 버튼은 없다.
func (s *server) recoveryPage(w http.ResponseWriter, r *http.Request) {
	v := ui.NewRecovery(s.page(r, "", ""))
	if s.st != nil {
		rep, err := s.st.Status()
		if err != nil {
			v.Ref = logRef(r, err)
		} else {
			v.DiagHref = ui.RecoveryDiagPath
			for _, rec := range rep.Records {
				rr := ui.RecoveryRecord{Kind: rec.Record.Kind, Started: rec.Record.Created.UTC().Format("2006-01-02 15:04 UTC")}
				for _, it := range rec.Items {
					rr.Items = append(rr.Items, ui.RecoveryItem{File: filepath.Base(it.AbsPath), Kind: it.Kind, Verdict: it.Verdict})
				}
				v.Records = append(v.Records, rr)
			}
		}
	}
	writePage(w, http.StatusServiceUnavailable, func(out io.Writer) error { return ui.RenderRecovery(out, v) })
}

// recoveryDiagnostics — 진단 파일(-recover-status -diagnostics와 같은 JSON)을 내려받게 한다. 아무것도 쓰지 않는다.
func (s *server) recoveryDiagnostics(w http.ResponseWriter, r *http.Request) {
	if s.st == nil {
		notFound(w, r)
		return
	}
	b, err := s.st.Diagnostics()
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, err)
		return
	}
	name := "pqcaton-diagnostics-" + time.Now().UTC().Format("20060102-150405") + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

// splitArgs — 위치 인자와 플래그를 가른다. 첫 플래그부터 뒤는 전부 플래그로 넘긴다.
func splitArgs(args []string) (pos, flags []string) {
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			return pos, args[i:]
		}
		pos = append(pos, args[i])
	}
	return pos, nil
}

// loopback — 이 주소가 이 기계 안에서만 닿는가.
func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return false // 빈 호스트는 전 인터페이스다
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// defaultScopeSession — 계층만 주고 세션 파일을 안 줬을 때 쓸 이름.
const defaultScopeSession = "scope-session.json"

// splitPaths — 쉼표로 준 경로 목록. **순서를 지킨다** — 계층 상속이 그 순서다.
func splitPaths(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type server struct {
	path    string
	decl    string
	results string
	scope   string
	// layers — 자산 스코프 계층 CSV들, 준 순서대로. 있으면 화면에서 규칙을 고친다.
	layers    []string
	base      string
	judgments string
	// org — 판정이 묶이는 조직. 요청이 동시에 읽으므로 orgName으로 읽는다. orgFromDecl이면 선언의 조직을
	// 따른다(작업 폴더 모드에서 -org를 주지 않았을 때). 명시 모드는 -org 그대로다(v0.22와 같다).
	org         string
	orgMu       sync.RWMutex
	orgFromDecl bool
	planOut     string
	scopeOut    string
	// scopeAssets: 자산 스코프 정책 파일의 경로. rev가 덮는 입력이다.
	scopeAssets string
	// st: 상태 디렉토리. nil이면 보기 전용이다(저장을 지원하지 않는 플랫폼).
	st *store.State
	// recovering: 시작할 때 복구 필요가 남아 있었다. 다시 띄울 때까지 모든 화면이 503이다.
	recovering bool
	// workspace: 작업 폴더 모드면 그 폴더(아니면 빈 값). diag · diagRef: 작업 폴더 모드에서 시작할 때 읽지 못한
	// 입력과 그 참조 번호. 있으면 진단 전용 모드다 — 모든 화면이 503이고 저장하지 않는다.
	workspace string
	diag      []diagProblem
	diagRef   string
	// storeOpts: 상태 디렉토리를 열 때의 선택 사항. 테스트가 실패 주입 훅을 단다.
	storeOpts store.Options
	// afterModel: 화면 모델을 만든 직후에 불린다(snapshot). 테스트가 그 틈에 파일을 바꾼다.
	afterModel func()
	// afterSnapshot: 스냅샷이 짝을 지은 직후(화면을 그리기 전)에 불린다. 테스트가 그 뒤에 파일을 바꿔,
	// 화면이 묶음 밖에서 다시 읽지 않는지 본다.
	afterSnapshot func()
	// wait: 대상 잠금을 기다리는 시간. 0이면 lockWait.
	wait time.Duration
	// key: 확인 토큰의 서명 키(confirm.go). clock: 지금 시각. 테스트가 바꾼다.
	keyOnce sync.Once
	key     []byte
	clock   func() time.Time
}

// handler — 주소와 처리기를 잇는다.
//
// **메서드를 라우터가 가른다.** 예전에는 처리기마다 `POST 인가`를 손으로 물었고, 그
// 물음을 빠뜨린 자리는 GET으로도 확정이 돌 수 있었다 — 새로고침 한 번에 확정이 다시
// 타는 길이다. 여기서는 `r.Post`로 등록하지 않은 주소에 POST가 닿지 않는다.
//
// Recoverer를 두는 이유: 화면 하나가 터져도 나머지 절차는 계속 다뤄야 한다. 리뷰 중에
// 서버가 죽으면 적던 판정이 통째로 날아간다.
func (s *server) handler() http.Handler {
	r := chi.NewRouter()
	// **순서가 곧 보호다**(guard.go). 머리글이 맨 바깥이라 403과 panic 응답에도 붙는다. 쿠키
	// 둘(CSRF · 말)은 Host 검사를 지난 요청에만 낸다.
	r.Use(securityHeaders)
	r.Use(recoverPanic)
	r.Use(hostGuard)
	r.Use(csrfCookie)
	r.Use(postGuard)
	// 고른 말을 기억한다. 화면을 옮길 때마다 다시 고르게 하지 않는다.
	r.Use(rememberLang)
	// 복구 상태면 정적 파일 말고는 처리기에 닿지 않는다.
	r.Use(s.recoveryGate)
	r.Use(s.diagGate)

	// 라우터에 없는 주소와 방식도 새 껍데기의 오류 페이지로 그린다.
	r.NotFound(notFound)
	r.MethodNotAllowed(methodNotAllowed)

	// 화면이 브라우저로 내보내는 것(스타일 · htmx). 바이너리에 담겨 있다.
	r.Handle(ui.StaticPath+"*", ui.Static())

	// **첫 화면은 절차의 첫 자리다.** 선언이 있으면 거기서 시작하고, 없으면 리뷰 큐다.
	r.Get("/", s.home)
	r.Get(ui.ScreenDecl, s.declEdit)
	r.Get(ui.RowPath, s.declRow)
	r.Get(ui.RemovePath, s.declRemove)
	r.Post("/decl/save", s.declSave)
	r.Post(ui.DeclCreatePath, s.declCreate)
	r.Post(ui.LayerCreatePath, s.layerCreate)
	r.Get(ui.ScreenScope, s.scopeEdit)
	r.Get(ui.ScopeRowPath, s.scopeRow)
	r.Post("/scope/rules", s.scopeRules)
	r.Post("/scope/save", s.scopeSave)
	r.Post("/scope/finalize/confirm", s.scopeConfirm)
	r.Post("/scope/finalize", s.scopeFinalize)
	r.Get(ui.ScreenSurvey, s.survey)
	r.Get(ui.ScreenInventory, s.inventory)
	r.Get(ui.ScreenReview, s.review)
	r.Post("/save", s.save)
	r.Post("/finalize/confirm", s.finalizeConfirm)
	r.Post("/finalize", s.finalize)

	// 옛 주소(v0.21의 `-next`, v0.22의 `/classic/…`). 옛 화면은 v0.23.0에서 없어졌고, 북마크가 죽지 않게
	// 301로 지금 화면에 넘긴다.
	for from, to := range ui.MovedScreens() {
		r.Get(from, moved(to))
	}
	return r
}

// moved — 옮긴 주소. **질의 문자열은 그대로 붙인다** — `?lang=en`으로 저장한 북마크가 말을 잃지 않게.
func moved(to string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dst := to
		if r.URL.RawQuery != "" {
			dst += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, dst, http.StatusMovedPermanently)
	}
}

// rememberLang — 주소로 말을 고르면 쿠키에 남긴다.
//
// **주소에 실린 말이 쿠키보다 우선한다**(ui.PickLang). 여기서는 그 선택을 기억만 한다 —
// 「행 추가」처럼 조각만 받아 오는 요청도 같은 말로 오게 하려는 것이다.
func rememberLang(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.URL.Query().Get(ui.LangParam); v != "" {
			if l := ui.PickLang(r); string(l) == strings.ToLower(v) {
				ui.SetLang(w, l)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// home — 절차의 첫 자리로 보낸다.
func (s *server) home(w http.ResponseWriter, r *http.Request) {
	to := "/review"
	if s.decl != "" {
		to = "/decl"
	}
	http.Redirect(w, r, to+"?"+r.URL.RawQuery, http.StatusSeeOther)
}

// page — 화면 한 장의 공통 부분. **말은 요청이 정한다** — 화면만 두 말을 쓰고,
// 이 명령이 표준오류로 찍는 것은 영어 하나다.
func (s *server) page(r *http.Request, here, subtitle string) ui.Page {
	l := ui.PickLang(r)
	mode := ui.ModeWrite
	switch {
	case s.recovering:
		mode = ui.ModeRecovery
	case len(s.diag) > 0:
		mode = ui.ModeDiagnostic
	case s.st == nil:
		mode = ui.ModeViewOnly
	}
	return ui.Page{
		Title: ui.ScreenTitle(here, l), Subtitle: subtitle,
		Lang: l, LangHref: ui.SwitchHref(r, l.Other()),
		// 알림은 코드로 온다. 문장은 여기서 그 말로 만든다(주소에 실려 온 글을 그대로 보이지 않는다).
		Message: ui.NoticeLines(l, ui.DecodeNotices(r.URL.Query().Get(ui.MsgParam)), s.fileName, ""),
		Problem: ui.NoticeLines(l, ui.DecodeNotices(r.URL.Query().Get(ui.ProblemParam)), s.fileName, "\n"),
		CSRF:    csrfToken(r),
		Self:    here, Mode: mode,
		// rev는 여기서 재지 않는다. 화면 모델과 같은 시점에 잰 값(snapshot)을 처리기가 넣는다.
	}
}

// sub — 부제. 「무엇을 보고 있나」라 값은 그대로 두고 이름표만 그 말로 옮긴다.
func sub(parts ...string) string { return strings.Join(parts, " · ") }

// ── 리뷰 큐 ────────────────────────────────────────────────────────────────

// reviewSession — 리뷰 세션을 읽는다. **관측 결과가 있으면 그것이 정답지다.**
//
// 큐는 관측에서 파생된 것이라, 결과가 늘었는데 세션이 그대로면 화면이 옛 큐를 보여
// 준다 — 방금 나타난 UNDECLARED가 판정 대상에 없는 상태다. 그래서 읽을 때마다 다시 세우고,
// 사람이 적은 판정은 [review.Carry]가 들고 간다. 파일에 쓰는 것은 저장·확정할 때뿐이다.
func (s *server) reviewSession() (review.Session, []review.Warning, error) {
	prev, err := review.Load(s.path)
	if err != nil && (s.results == "" || !os.IsNotExist(err)) {
		return prev, nil, err
	}
	if s.results == "" {
		return prev, nil, nil
	}
	d, err := decl.Load(s.decl)
	if err != nil {
		return prev, nil, err
	}
	policy, err := s.assetPolicy()
	if err != nil {
		return prev, nil, err
	}
	b, err := review.FromResultsWith(s.results, d, s.orgName(), policy)
	if err != nil {
		return prev, nil, err
	}
	return review.Carry(prev, b.Session), b.Warnings, nil
}

func (s *server) review(w http.ResponseWriter, r *http.Request) {
	if s.needsDecl(w, r) {
		return
	}
	var wd *world
	rev, err := s.snapshotScreen(ui.ScreenReview, func() error {
		wd = s.load()
		return wd.reviewErr
	})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	sf, warn := wd.review, wd.reviewWarn
	page := s.nextPage(r, ui.ScreenReview, s.path, s.steps(wd))
	// 세션의 스코프는 `org://이름`이다. 위쪽 맥락 줄에는 다른 화면과 같은 모양으로
	// 조직 이름만 적는다 — 한 껍데기에서 같은 자리가 화면마다 다르면 다른 값으로 읽힌다.
	page.Org = strings.TrimPrefix(sf.Scope, "org://")
	page.Warnings = ui.Warnings(page.Lang, warn)
	page.Rev = rev
	v := ui.NewReviewView(sf, page)
	html(w, r, func(out io.Writer) error { return ui.RenderReview(out, v) })
}

// applyReview — 폼 값을 지금 세션에 얹는다. **읽고 얹는다** — 화면이 자기 사본을 들고 있으면 그 사이
// 파일을 고친 사람의 편집이 사라진다. 저장소의 잠금 안에서 불린다.
func (s *server) applyReview(r *http.Request) (review.Session, error) {
	sf, _, err := s.reviewSession()
	if err != nil {
		return sf, err
	}
	return ui.ApplyReview(sf, r.PostForm), nil
}

// reviewID: 리뷰 세션의 동일성. 파일에 적힌 세션 ID다.
func (s *server) reviewID() store.Identity {
	sf, _ := review.Load(s.path)
	return store.Identity{Kind: kindReview, Org: s.orgName(), SessionID: sessionName(sf.SessionID)}
}

func (s *server) save(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenReview
	if !s.writable(w, r, back) {
		return
	}
	err := s.st.SaveChecked(s.reviewID(), []string{s.path}, s.check(r), s.waitTime(), func() ([]store.Change, error) {
		sf, err := s.applyReview(r)
		if err != nil {
			return nil, err
		}
		b, err := review.Encode(sf)
		return []store.Change{{Path: s.path, Content: b}}, err
	})
	if err != nil {
		s.fail(w, r, back, err, file(ui.PickLang(r), "session", s.path))
		return
	}
	redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeSaved)}, nil)
}

// finalize — **명령과 같은 검사를 거친다.** 여기서 따로 판정하지 않는다(review.Outputs가
// review.Finalize를 부른다). 확인 페이지(confirm.go)를 거친 요청만 받는다.
//
// 세션 · 계획(앞 판은 .prev로) · 판정 원장 · 영수증이 한 트랜잭션이다. 같은 세션을 같은 내용으로
// 다시 확정하면 아무것도 다시 쓰지 않는다.
func (s *server) finalize(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenReview
	l := ui.PickLang(r)
	files := []ui.FileRef{file(l, "session", s.path), file(l, "plan", s.planOut)}
	if !s.writable(w, r, back) {
		return
	}
	withJudg := s.judgments != ""
	targets := s.reviewTargets()
	if withJudg {
		files = append(files, file(l, "ledger", s.judgments))
	}
	// 확인 페이지가 서명한 지문. 잠금 안에서 다시 만들어 견준다 - 그 사이 파일이 바뀌었으면 쓰지 않는다.
	id := s.reviewID()
	payload, ok := s.confirmed(w, r, kindReview, back, id)
	if !ok {
		return
	}
	var written review.Output
	out, err := s.st.FinalizeChecked(id, targets, s.check(r), payload, s.waitTime(), func() (store.Parts, error) {
		sf, err := s.applyReview(r)
		if err != nil {
			return store.Parts{}, err
		}
		o, err := review.Outputs(sf, s.orgName(), withJudg, time.Now().Unix())
		if err != nil {
			return store.Parts{}, err
		}
		p, err := review.PayloadSHA256(sf, s.orgName(), withJudg)
		if err != nil {
			return store.Parts{}, err
		}
		written = o
		parts := store.Parts{PayloadSHA256: p, Replace: []store.Change{
			{Path: s.path, Content: o.Session}, {Path: s.planOut, Content: o.Plan, Backup: true}}}
		if withJudg && len(o.Judgments) > 0 {
			parts.Append = []store.Addition{{Path: s.judgments, Data: o.Judgments}}
		}
		return parts, nil
	})
	if err != nil {
		s.fail(w, r, back, err, files...)
		return
	}
	switch out.Status {
	case store.OutcomeAlready:
		redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeFinalizedAlready)}, nil)
	case store.OutcomeSuperseded:
		redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeFinalizedSuperseded)}, nil)
	default:
		msg := []ui.Notice{ui.N(ui.NoticeFinalizedPlan, len(written.Result.Plan.GetActions()), "plan")}
		if withJudg {
			msg = append(msg, ui.N(ui.NoticeJudgmentsSaved, written.Judged, "ledger"))
		}
		redirect(w, r, back, msg, nil)
	}
}

// ── 자산 스코프 ────────────────────────────────────────────────────────────

// scopeSession — 스코프 세션을 읽는다. **계층 파일이 있으면 그것이 정답지다.**
//
// 세션은 계층에서 파생된 것이라, 파일을 고쳐 놓고 세션을 안 고치면 화면이 옛 변경을
// 보여 준다. 그래서 읽을 때마다 계층에서 다시 세운다 — 사람이 적은 판정은 [scope.Reopen]
// 이 들고 간다. 파일에 쓰는 것은 저장·확정할 때뿐이다.
func (s *server) scopeSession() (scope.Session, []scope.LayerFile, error) {
	layers := s.layerFiles()
	sf, err := scope.LoadSession(s.scope)
	if err != nil && (len(layers) == 0 || !os.IsNotExist(err)) {
		return sf, nil, err
	}
	if len(layers) == 0 {
		return sf, nil, nil
	}
	files, err := scope.LoadLayers(layers)
	if err != nil {
		return sf, nil, err
	}
	var base *kscope.AssetPolicy
	if s.base != "" {
		if base, err = scope.LoadPolicyFile(s.base); err != nil {
			return sf, nil, err
		}
	}
	return scope.Reopen(sf, scope.Layers(files), base, s.orgName()), files, nil
}

func (s *server) scopeEdit(w http.ResponseWriter, r *http.Request) {
	if s.scope == "" {
		missingInput(w, r, "scope-session")
		return
	}
	if s.scopeMissing() {
		// **GET은 만들지 않는다.** 만들기 폼만 보이고, 만드는 것은 그 POST다(layerCreate).
		wd := s.load()
		p := s.nextPage(r, ui.ScreenScope, s.scope, s.steps(wd))
		html(w, r, func(out io.Writer) error { return ui.RenderLayerCreate(out, ui.LayerCreateView{Page: p}) })
		return
	}
	var wd *world
	rev, err := s.snapshotScreen(ui.ScreenScope, func() error {
		wd = s.load()
		return wd.scopeErr
	})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	sf, files := wd.scope, wd.scopeFiles
	page := s.nextPage(r, ui.ScreenScope, s.scope, s.steps(wd))
	page.Org = sf.Org
	page.Rev = rev
	v := ui.NewScopeView(sf, page)
	if len(files) > 0 {
		v = v.Editable(files)
	}
	html(w, r, func(out io.Writer) error { return ui.RenderScope(out, v) })
}

// scopeRules — 화면에서 고친 규칙을 계층 파일에 쓴다.
//
// **판정을 먼저 얹고 규칙을 쓴다.** 같은 폼에서 온 값이라, 규칙만 저장하면 그 사이에
// 적어 둔 결론이 날아간다. 계층 파일들과 세션이 한 트랜잭션이다.
func (s *server) scopeRules(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenScope
	layers := s.layerFiles()
	if len(layers) == 0 {
		missingInput(w, r, "layer")
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	l := ui.PickLang(r)
	targets := append([]string{s.scope}, layers...)
	files := []ui.FileRef{file(l, "scope-session", s.scope)}
	for _, p := range layers {
		files = append(files, file(l, "layer", p))
	}
	var (
		was, next scope.Session
		edited    []scope.LayerFile
	)
	err := s.st.SaveChecked(s.scopeID(), targets, s.check(r), s.waitTime(), func() ([]store.Change, error) {
		sf, files, err := s.scopeSession()
		if err != nil {
			return nil, err
		}
		was = sf
		sf = ui.ApplyScope(sf, r.PostForm)
		edited = ui.ApplyLayers(files, r.PostForm)
		var changes []store.Change
		for _, lf := range edited {
			b, err := scope.EncodeLayer(lf)
			if err != nil {
				return nil, err
			}
			changes = append(changes, store.Change{Path: lf.Path, Content: b})
		}
		// 규칙이 달라졌으니 변경을 다시 센다. 적어 둔 판정은 규칙 동일성으로 따라온다.
		var base *kscope.AssetPolicy
		if s.base != "" {
			if base, err = scope.LoadPolicyFile(s.base); err != nil {
				return nil, err
			}
		}
		next = scope.Reopen(sf, scope.Layers(edited), base, s.orgName())
		b, err := scope.EncodeSession(next)
		if err != nil {
			return nil, err
		}
		return append(changes, store.Change{Path: s.scope, Content: b}), nil
	})
	if err != nil {
		s.fail(w, r, back, err, files...)
		return
	}
	n := 0
	for _, lf := range edited {
		n += len(lf.Layer.Rules)
	}
	msg := []ui.Notice{ui.N(ui.NoticeRulesSaved, n, len(edited), len(next.Changes), next.AuditedCount())}
	if next.Signature == "" && was.Signature != "" {
		// **말하지 않으면 사람은 서명이 남아 있다고 여긴다.**
		msg = append(msg, ui.N(ui.NoticeSignatureCleared))
	}
	redirect(w, r, back, msg, nil)
}

// scopeRow — 규칙 표의 「행 추가」.
func (s *server) scopeRow(w http.ResponseWriter, r *http.Request) {
	n := len(s.layerFiles())
	if n == 0 {
		missingInput(w, r, "layer")
		return
	}
	layer, err := strconv.Atoi(r.URL.Query().Get("layer"))
	if err != nil || layer < 0 || layer >= n {
		errorPage(w, r, http.StatusBadRequest, ui.ErrBadRequest, nil)
		return
	}
	i, err := strconv.Atoi(r.URL.Query().Get("i"))
	if err != nil || i < 0 || i > maxRows {
		errorPage(w, r, http.StatusBadRequest, ui.ErrBadRequest, nil)
		return
	}
	html(w, r, func(out io.Writer) error { return ui.RenderRuleRow(out, ui.PickLang(r), layer, i) })
}

// applyScope — 폼 값을 지금 세션에 얹는다. 저장소의 잠금 안에서 불린다.
func (s *server) applyScope(r *http.Request) (scope.Session, error) {
	sf, _, err := s.scopeSession()
	if err != nil {
		return sf, err
	}
	return ui.ApplyScope(sf, r.PostForm), nil
}

// scopeID: 스코프 세션의 동일성.
func (s *server) scopeID() store.Identity {
	sf, _ := scope.LoadSession(s.scope)
	return store.Identity{Kind: kindScope, Org: s.orgName(), SessionID: sessionName(sf.SessionID)}
}

func (s *server) scopeSave(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenScope
	if s.scope == "" || s.scopeMissing() {
		missingInput(w, r, "scope-session")
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	err := s.st.SaveChecked(s.scopeID(), []string{s.scope}, s.check(r), s.waitTime(), func() ([]store.Change, error) {
		sf, err := s.applyScope(r)
		if err != nil {
			return nil, err
		}
		b, err := scope.EncodeSession(sf)
		return []store.Change{{Path: s.scope, Content: b}}, err
	})
	if err != nil {
		s.fail(w, r, back, err, file(ui.PickLang(r), "scope-session", s.scope))
		return
	}
	redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeSaved)}, nil)
}

// scopeFinalize — **명령과 같은 검사를 거친다.** 세션 · 정책 CSV(앞 판은 .prev로) · 판정 원장 ·
// 영수증이 한 트랜잭션이다.
func (s *server) scopeFinalize(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenScope
	if s.scope == "" || s.scopeMissing() {
		missingInput(w, r, "scope-session")
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	l := ui.PickLang(r)
	withJudg := s.judgments != ""
	targets := s.scopeTargets()
	files := []ui.FileRef{file(l, "scope-session", s.scope), file(l, "policy", s.scopeOut)}
	if withJudg {
		files = append(files, file(l, "ledger", s.judgments))
	}
	id := s.scopeID()
	payload, ok := s.confirmed(w, r, kindScope, back, id)
	if !ok {
		return
	}
	var written scope.Output
	out, err := s.st.FinalizeChecked(id, targets, s.check(r), payload, s.waitTime(), func() (store.Parts, error) {
		sf, err := s.applyScope(r)
		if err != nil {
			return store.Parts{}, err
		}
		o, err := scope.Outputs(sf, s.orgName(), withJudg, time.Now().Unix())
		if err != nil {
			return store.Parts{}, err
		}
		p, err := scope.PayloadSHA256(sf, s.orgName(), withJudg)
		if err != nil {
			return store.Parts{}, err
		}
		written = o
		parts := store.Parts{PayloadSHA256: p, Replace: []store.Change{
			{Path: s.scope, Content: o.Session}, {Path: s.scopeOut, Content: o.Policy, Backup: true}}}
		if withJudg && len(o.Judgments) > 0 {
			parts.Append = []store.Addition{{Path: s.judgments, Data: o.Judgments}}
		}
		return parts, nil
	})
	if err != nil {
		s.fail(w, r, back, err, files...)
		return
	}
	switch out.Status {
	case store.OutcomeAlready:
		redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeFinalizedAlready)}, nil)
	case store.OutcomeSuperseded:
		redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeFinalizedSuperseded)}, nil)
	default:
		msg := []ui.Notice{ui.N(ui.NoticeFinalizedPolicy, len(written.Result.Policy.Rules), "policy")}
		if withJudg {
			msg = append(msg, ui.N(ui.NoticeJudgmentsSaved, written.Judged, "ledger"))
		}
		redirect(w, r, back, msg, nil)
	}
}

// ── 대조 ───────────────────────────────────────────────────────────────────

// survey — 관측을 모아 선언과 대조한 것을 보여 준다. **계산은 report 패키지가 한다** —
// 명령(`pqcaton-report`)이 글로 찍는 것과 같은 것을 표로 그린다.
func (s *server) survey(w http.ResponseWriter, r *http.Request) {
	if s.needsDecl(w, r) {
		return
	}
	if s.results == "" {
		missingInput(w, r, "results")
		return
	}
	var wd *world
	if _, err := s.snapshotScreen(ui.ScreenSurvey, func() error {
		wd = s.load()
		return wd.resErr
	}); err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	res := wd.res
	page := s.nextPage(r, ui.ScreenSurvey, s.results, s.steps(wd))
	page.Org = res.Org
	v := ui.NewSurveyView(res, page)
	v.SVG = renderDOT(v.DOT)
	html(w, r, func(out io.Writer) error { return ui.RenderSurvey(out, v) })
}

// renderDOT — `dot`이 있으면 SVG로 그린다. 없으면 빈 값을 돌려주고 화면이 원문을 보인다.
//
// **의존성이 아니라 있으면 좋은 것이다.** 없다고 화면이 깨지면 표준 라이브러리만 쓴다는
// 약속이 무의미해진다.
func renderDOT(dot string) string {
	bin, err := exec.LookPath("dot")
	if err != nil {
		return ""
	}
	cmd := exec.Command(bin, "-Tsvg")
	cmd.Stdin = strings.NewReader(dot)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// `dot`이 낸 SVG다. 우리가 만든 DOT에서 나온 것이라 밖에서 온 값이 아니다.
	return string(out)
}

// ── 인벤토리 조회 ──────────────────────────────────────────────────────────

// inventory — 찾아보는 자리. **절차의 한 단계가 아니다.**
//
// 대조 화면은 지금 대조한 것을 전부 늘어놓는다 — 수천 대에서는 그 자체가 못 쓰는
// 화면이다. 여기서는 조건을 걸어 찾고, 자산 하나의 판정 이력을 열고, 정책이 뺀 것과 근거가
// 바뀐 판정을 본다. **전부 손에 든 파일에서 나온다.**
func (s *server) inventory(w http.ResponseWriter, r *http.Request) {
	if s.needsDecl(w, r) {
		return
	}
	if s.results == "" {
		missingInput(w, r, "results")
		return
	}
	var (
		wd *world
		v  ui.InventoryView
	)
	// **본문도 묶음 안에서 만든다.** 정책 · 원장은 조회 본문만 읽는 파일이라, 묶음 밖에서 읽으면 카드와
	// 본문이 다른 시점을 보인다.
	if _, err := s.snapshotScreen(ui.ScreenInventory, func() (err error) {
		wd = s.load()
		if wd.resErr != nil {
			return wd.resErr
		}
		// **조회는 절차 카드에 넣지 않는다** — 단계가 아니라 아무 때나 들어오는 자리다.
		// 그래도 위쪽 카드는 그대로 세워 둔다. 어디서 들어왔든 절차가 어디까지 왔는지는
		// 보여야 한다.
		page := s.nextPage(r, ui.ScreenInventory, s.results, s.steps(wd))
		page.Org = wd.res.Org
		v = ui.NewInventoryView(wd.res, ui.FilterFrom(r.URL.Query()), page)
		if v, err = s.withPolicy(v); err != nil {
			return err
		}
		v, err = s.withLedger(v, wd.res, r.URL.Query().Get("subject"))
		return err
	}); err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	html(w, r, func(out io.Writer) error { return ui.RenderInventory(out, v) })
}

// withPolicy — 「안 보고 있는 것」. 확정된 정책 CSV가 있어야 셀 수 있다.
func (s *server) withPolicy(v ui.InventoryView) (ui.InventoryView, error) {
	pol, err := scope.LoadPolicyFile(s.scopeOut)
	if err != nil {
		// **없는 것은 오류가 아니다.** 아직 정책을 확정하지 않았을 뿐이고, 화면은 그
		// 절에서 무엇을 주면 열리는지 알린다.
		if os.IsNotExist(err) {
			return v, nil
		}
		return v, err
	}
	results, _, err := report.LoadResults(s.results)
	if err != nil {
		return v, err
	}
	// **명령(`pqcaton-scope review`)과 같은 계산이다.**
	ex, err := scope.ExcludedFromResults(pol, results)
	if err != nil {
		return v, err
	}
	prior, err := s.ledger()
	if err != nil {
		return v, err
	}
	return v.WithUnseen(ex, scope.Review(ex, prior, time.Now().Unix(), defaultTTLDays*24*60*60)), nil
}

// withLedger — 판정 이력과 「근거가 바뀐 판정」.
func (s *server) withLedger(v ui.InventoryView, res *report.Result, subject string) (ui.InventoryView, error) {
	if s.judgments == "" {
		return v, nil
	}
	all, err := s.ledger()
	if err != nil {
		return v, err
	}
	// 지금 관측이 만드는 근거. **명령의 delta와 같은 계산이라야** 화면과 명령이 같은
	// 것을 「바뀌었다」고 알린다.
	basis := map[string]string{}
	for _, rec := range res.Assets {
		basis[review.Key(rec.Key)] = decision.HashBasis(string(rec.State), rec.Key.Runtime)
	}
	return v.WithLedger(all, basis, subject), nil
}

// ledger — 원장을 통째로 읽는다. 없으면 빈 목록이다.
func (s *server) ledger() ([]decision.Judgment, error) {
	if s.judgments == "" {
		return nil, nil
	}
	store, err := decision.NewFileJudgmentStore(org.ID(s.orgName()), s.judgments)
	if err != nil {
		return nil, err
	}
	saved, err := store.All()
	if err != nil {
		return nil, err
	}
	out := make([]decision.Judgment, 0, len(saved))
	for _, j := range saved {
		out = append(out, *j)
	}
	return out, nil
}

// defaultTTLDays — 제외 승인의 유효기간. `pqcaton-scope`와 같은 값이라야 화면과 명령이
// 같은 것을 「오래됐다」고 알린다.
const defaultTTLDays = 180

// ── 선언 ───────────────────────────────────────────────────────────────────

// declEdit — 선언 화면.
func (s *server) declEdit(w http.ResponseWriter, r *http.Request) {
	// 작업 폴더에 선언이 아직 없으면 만들기를 낸다. GET은 아무것도 만들지 않는다.
	if s.workspace != "" && !s.declReady() {
		p := s.nextPage(r, ui.ScreenDecl, s.decl, ui.StepState{DeclMissing: true})
		html(w, r, func(out io.Writer) error {
			return ui.RenderDeclCreate(out, ui.DeclCreateView{Page: p, Org: s.orgName()})
		})
		return
	}
	s.declScreen(w, r)
}

// nextPage — 다음 판 껍데기가 쓰는 값을 얹는다.
//
// **절차 카드는 여기서 만든다.** 상태를 아는 화면부터 채우는데, 지금은 선언 하나다 —
// 카드 하나를 채우려면 그 화면의 계산을 매 요청마다 돌려야 하고, 대조는 계산이 무겁다.
func (s *server) nextPage(r *http.Request, here, subtitle string, st ui.StepState) ui.Page {
	p := s.page(r, here, subtitle)
	p.Org = s.orgName()
	p.Results = s.results
	p.Steps = ui.StepsFor(p.Lang, here, s.screens(), st)
	if s.screens().Inventory {
		p.InventoryHref = ui.ScreenInventory
		p.InventoryText = ui.InventoryLabel(p.Lang)
	}
	if (here == ui.ScreenSurvey || here == ui.ScreenReview) && s.resultsEmpty() {
		p.EmptyResults = s.results
	}
	return p
}

// resultsEmpty: 관측 결과 폴더에 파일이 하나도 없는가(숨은 파일은 세지 않는다). 읽지 못하면 비었다고 하지 않는다.
func (s *server) resultsEmpty() bool {
	if s.results == "" {
		return false
	}
	ents, err := os.ReadDir(s.results)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), ".") {
			return false
		}
	}
	return true
}

func (s *server) declScreen(w http.ResponseWriter, r *http.Request) {
	if s.decl == "" {
		missingInput(w, r, "declaration")
		return
	}
	var wd *world
	rev, err := s.snapshotScreen(ui.ScreenDecl, func() error {
		wd = s.load()
		return wd.declErr
	})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	d, seen, unmatched := wd.decl, wd.seen, wd.unmatched
	// **부제에는 파일만 적는다.** 조직은 위쪽 맥락 줄에 이미 있어서, 그대로 두면
	// 같은 값이 한 화면에 두 번 나온다.
	page := s.nextPage(r, ui.ScreenDecl, s.decl, s.steps(wd))
	page.Org = d.OrgOrDefault()
	page.Rev = rev
	v := ui.NewDeclView(d, page).WithObserved(seen, unmatched)
	html(w, r, func(out io.Writer) error { return ui.RenderDecl(out, v) })
}

// observedFrom — 노드마다 **관측된** 자산. 컴포넌트 칸의 후보다. 대조 결과는 부르는 쪽이 이미 만든 것을 받는다.
//
// **관측 결과에 적힌 이름이 곧 맞는 이름이다.** 대조는 글자 그대로 같을 때만 맞는데, 관측 이름은
// `.so` 뒤가 떼인 채로 온다 — 사람이 대조 화면을 보고 옮겨 적으면 거기서 틀린다.
//
// 결과를 읽지 못해도 선언 화면은 열려야 한다. 후보는 거들 뿐이고, 없으면 손으로 적는다.
// unmatched — 관측에는 있는데 어느 선언 노드에도 붙지 않은 노드 이름. 「관측 이름」 칸의
// 후보다. 붙지 않았다는 것은 그 노드의 자산이 통째로 UNDECLARED로 오른다는 뜻이고, 그것은
// 선언이 틀려서가 아니라 이름이 서로 달라서다.
func observedFrom(d decl.Declaration, res *report.Result) (map[string][]ui.DeclAsset, []string) {
	declared := map[string]bool{}
	for _, n := range d.Nodes {
		declared[n.Name] = true
	}
	out := map[string][]ui.DeclAsset{}
	odd := map[string]bool{}
	for _, a := range res.Assets {
		if a.State == reconcile.Unobserved {
			continue // 선언만 있고 관측되지 않은 것은 후보가 아니다
		}
		out[a.Key.NodeID] = append(out[a.Key.NodeID],
			ui.DeclAsset{Runtime: a.Key.Runtime, Component: a.Key.Component})
		if !declared[a.Key.NodeID] {
			odd[a.Key.NodeID] = true
		}
	}
	for node := range res.SeenBy {
		if !declared[node] {
			odd[node] = true
		}
	}
	names := make([]string, 0, len(odd))
	for n := range odd {
		names = append(names, n)
	}
	return out, names
}

// declRow — 「행 추가」. 빈 줄 하나와, 번호가 하나 오른 버튼을 돌려준다.
//
// **페이지를 다시 띄우지 않는 이유가 전부다** — 다시 띄우면 아직 저장하지 않은 편집이
// 날아간다. 줄이 몇 개 필요한지는 쓰기 전에는 알 수 없으니, 미리 열어 둔 빈 줄로는
// 모자란 날이 온다.
//
// 번호는 밖에서 오는 값이라 받는 대로 믿지 않는다.
func (s *server) declRow(w http.ResponseWriter, r *http.Request) {
	if s.decl == "" {
		missingInput(w, r, "declaration")
		return
	}
	kind := r.URL.Query().Get("kind")
	if !ui.ValidKind(kind) {
		errorPage(w, r, http.StatusBadRequest, ui.ErrBadRequest, nil)
		return
	}
	i, err := strconv.Atoi(r.URL.Query().Get("i"))
	if err != nil || i < 0 || i > maxRows {
		errorPage(w, r, http.StatusBadRequest, ui.ErrBadRequest, nil)
		return
	}
	// 자산은 어느 노드의 것인지가 함께 온다 — 그 번호도 밖에서 오는 값이다.
	node := 0
	if v := r.URL.Query().Get("node"); v != "" {
		if node, err = strconv.Atoi(v); err != nil || node < 0 || node > maxRows {
			errorPage(w, r, http.StatusBadRequest, ui.ErrBadRequest, nil)
			return
		}
	}
	html(w, r, func(out io.Writer) error { return ui.RenderRow(out, ui.PickLang(r), kind, node, i) })
}

// declRemove — 「제거」. **빈 응답을 돌려준다** — 지우는 것은 브라우저가 하고(htmx가
// 그 줄을 빈 것으로 갈아 끼운다), 파일이 달라지는 것은 저장할 때뿐이다.
//
// 서버가 아무것도 바꾸지 않으므로 GET이다. 새로고침으로 무엇이 다시 일어나지 않는다.
func (s *server) declRemove(w http.ResponseWriter, r *http.Request) {
	if s.decl == "" {
		missingInput(w, r, "declaration")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// maxRows — 한 표에 열어 줄 줄 수의 상한. 사람이 화면에서 손으로 넣는 자리라 이보다
// 많아질 일이 없다. 많다면 파일을 만들어 넣을 일이다.
const maxRows = 10000

func (s *server) declSave(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenDecl
	if s.decl == "" {
		missingInput(w, r, "declaration")
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	var (
		d       decl.Declaration
		dropped []string
	)
	id := store.Identity{Kind: kindDecl, Org: s.orgName(), SessionID: "declaration"}
	err := s.st.SaveChecked(id, []string{s.decl}, s.check(r), s.waitTime(), func() ([]store.Change, error) {
		prev, err := decl.Load(s.decl)
		if err != nil {
			return nil, err
		}
		d, dropped = ui.ApplyDecl(prev, r.PostForm)
		b, err := decl.Encode(d)
		return []store.Change{{Path: s.decl, Content: b}}, err
	})
	if err != nil {
		s.fail(w, r, back, err, file(ui.PickLang(r), "declaration", s.decl))
		return
	}
	s.adoptOrg(d.Org)
	msg := []ui.Notice{ui.N(ui.NoticeDeclSaved, len(d.Nodes), len(d.Assets), len(d.Edges))}
	// **뺀 줄은 알린다.** IP 없는 줄은 관리 대상이 아니라 저장에서 빠지는데, 표에서
	// 사라진 것만 보이면 지워진 것으로 읽힌다.
	if len(dropped) > 0 {
		msg = append(msg, ui.N(ui.NoticeDeclDropped, len(dropped)))
	}
	// **저장은 됐지만 앞뒤가 안 맞으면 그 사실을 함께 알린다.** 막지는 않는다.
	if p := decl.Check(d); len(p) > 0 {
		msg = append(msg, ui.N(ui.NoticeDeclStillOff, len(p)))
	}
	redirect(w, r, back, msg, nil)
}

// layerCreate: 작업 폴더에 빈 계층 CSV를 만든다. 같은 이름의 파일이 있으면 덮어쓰지 않고 409다. 만든 뒤 같은
// 요청에서 스코프 세션을 세운다(materialize).
func (s *server) layerCreate(w http.ResponseWriter, r *http.Request) {
	if s.workspace == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, ui.ScreenScope) {
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("name"))
	if !layerName.MatchString(name) {
		errorPage(w, r, http.StatusBadRequest, ui.ErrBadRequest, nil)
		return
	}
	l := ui.PickLang(r)
	p := filepath.Join(s.workspace, wsLayers, name+".csv")
	b, err := scope.EncodeLayer(scope.LayerFile{Path: p, Layer: scope.Layer{Name: name}})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, err)
		return
	}
	if err := s.st.Create([]store.Change{{Path: p, Content: b}}, s.waitTime()); err != nil {
		if errors.Is(err, store.ErrExists) {
			s.refuse(w, r, http.StatusConflict, ui.RefusedExists, ui.ScreenScope, []ui.FileRef{file(l, "layer", p)})
			return
		}
		s.fail(w, r, ui.ScreenScope, err, file(l, "layer", p))
		return
	}
	if err := s.materialize(); err != nil {
		s.fail(w, r, ui.ScreenScope, err, file(l, "scope-session", s.scope))
		return
	}
	redirect(w, r, ui.ScreenScope, []ui.Notice{ui.N(ui.NoticeLayerCreated, name)}, nil)
}

// layerName: 계층 이름. 화면의 입력 칸과 같은 식이다(ui.LayerNamePattern).
var layerName = regexp.MustCompile("^" + ui.LayerNamePattern + "$")

// ── 공통 ───────────────────────────────────────────────────────────────────

// redirect — POST 뒤에는 GET으로 보낸다. 새로고침이 같은 확정을 다시 태우지 않게 한다.

// fileName: 알림 · 거절에 적을 파일 이름(역할 → 이 서버의 파일). 전체 경로는 싣지 않는다.
func (s *server) fileName(role string) string {
	p := map[string]string{"plan": s.planOut, "ledger": s.judgments, "policy": s.scopeOut, "session": s.path,
		"scope-session": s.scope, "declaration": s.decl, "results": s.results}[role]
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

// missingInput: 이 화면에 필요한 파일을 주지 않았다(404). 역할만 보인다.
func missingInput(w http.ResponseWriter, r *http.Request, role string) {
	errorPage(w, r, http.StatusNotFound, ui.ErrInputMissing, nil, ui.FileRef{Role: ui.RoleLabel(ui.PickLang(r), role)})
}

// screens: 열리는 화면. 작업 폴더에 선언이 아직 없으면 대조 · 판정 · 조회는 닫는다(판정할 것이 없다).
func (s *server) screens() ui.Screens {
	declOK := s.workspace == "" || s.declReady()
	return ui.Screens{
		Decl: s.decl != "", Scope: s.scope != "",
		Survey: s.results != "" && declOK, Inventory: s.results != "" && declOK,
		NoReview: !declOK,
	}
}

// needsDecl: 작업 폴더에 선언이 아직 없으면 그 화면 대신 「필요한 파일 없음」을 낸다.
func (s *server) needsDecl(w http.ResponseWriter, r *http.Request) bool {
	if s.workspace == "" || s.declReady() {
		return false
	}
	missingInput(w, r, "declaration")
	return true
}

// diagGate: 진단 전용 모드면 정적 파일 말고는 모두 503 진단 페이지다. 읽지 못한 파일의 역할과 이름, 참조 번호를 보인다.
func (s *server) diagGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.diag) == 0 || strings.HasPrefix(r.URL.Path, ui.StaticPath) {
			next.ServeHTTP(w, r)
			return
		}
		l := ui.PickLang(r)
		var files []ui.FileRef
		for _, p := range s.diag {
			files = append(files, file(l, p.role, p.path))
		}
		v := ui.NewRefused(s.page(r, "", ""), ui.RefusedDiagnostic, "/", files)
		v.Ref = s.diagRef
		writePage(w, http.StatusServiceUnavailable, func(out io.Writer) error { return ui.RenderRefused(out, v) })
	})
}

// declCreate: 작업 폴더에 빈 선언을 만든다(POST). 이미 있으면 덮어쓰지 않고 409다. 만든 뒤 관측 결과가 있으면
// 리뷰 세션도 같은 요청에서 세운다 — GET이 파일을 만들지 않게.
func (s *server) declCreate(w http.ResponseWriter, r *http.Request) {
	if s.decl == "" {
		missingInput(w, r, "declaration")
		return
	}
	if !s.writable(w, r, ui.ScreenDecl) {
		return
	}
	org := strings.TrimSpace(r.PostForm.Get("org"))
	// -org를 준 서버는 그 조직의 선언만 만든다. 다르면 세션이 서지 않는다(선언과 -org가 다른 조직을 말한다).
	if org == "" || len(org) > 100 || strings.ContainsAny(org, "\r\n\t/\\") || (!s.orgFromDecl && org != s.orgName()) {
		errorPage(w, r, http.StatusBadRequest, ui.ErrBadRequest, nil)
		return
	}
	b, err := decl.Encode(decl.Declaration{Org: org, Scope: []string{}, Nodes: []decl.Node{}, Assets: []decl.Asset{}, Edges: []decl.Edge{}})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, err)
		return
	}
	l := ui.PickLang(r)
	if err := s.st.Create([]store.Change{{Path: s.decl, Content: b}}, s.waitTime()); err != nil {
		if errors.Is(err, store.ErrExists) {
			s.refuse(w, r, http.StatusConflict, ui.RefusedExists, ui.ScreenDecl, []ui.FileRef{file(l, "declaration", s.decl)})
			return
		}
		s.fail(w, r, ui.ScreenDecl, err, file(l, "declaration", s.decl))
		return
	}
	s.adoptOrg(org)
	if err := s.materialize(); err != nil {
		s.fail(w, r, ui.ScreenDecl, err, file(l, "session", s.path))
		return
	}
	redirect(w, r, ui.ScreenDecl, []ui.Notice{ui.N(ui.NoticeDeclCreated)}, nil)
}
