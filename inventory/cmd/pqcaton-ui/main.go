// Command pqcaton-ui — 리뷰 큐와 선언을 사람이 다루는 화면.
//
// **파일에서 읽고 파일에 쓰는 껍데기다.** 화면 자체는 `pkg/inventory/ui` 에 있고, 확정
// 관문은 `pkg/inventory/review` 에 있다 — 컨트롤 플레인이 같은 화면과 같은 관문을
// 쓰고, 다른 것은 「어디서 읽고 누가 들어오나」뿐이다.
//
//	pqcaton-ui <session.json> [-decl declaration.json] [-results 디렉터리]
//	           [-layers corp.csv,prod.csv] [-base asset-scope.csv] [-scope scope-session.json]
//	           [-addr 127.0.0.1:8765] [-judgments 파일] [-org 이름]
//	           [-plan 파일] [-scope-out 파일]
//
// **탭 순서가 절차 순서다** — 선언 → 스코프 → 대조 → 리뷰 큐. 쓰는 사람이 다음에 무엇을
// 할지 화면이 말해 준다. 재료를 주지 않은 자리는 만들지 않는다.
//
// **라우팅은 chi, 화면은 templ, 부분 갱신은 htmx 다.** 셋 다 허용적 라이선스이고
// 링크되는 모듈은 둘만 는다(전이 의존이 없다). htmx 는 바이너리에 박혀 나가므로 망이
// 끊긴 기계에서도 그대로 뜬다 — 그리고 우리 라이선스 관문이 그 파일까지 본다.
//
// **기본은 127.0.0.1 이다.** 리뷰 큐에는 어느 노드가 무엇을 쓰는지가 그대로 있다 — 곧 그
// 조직의 공격면이다. 밖으로 열려면 -addr 를 명시적으로 바꿔야 하고 그때 경고한다.
//
// **산출물은 여전히 파일이다.** 화면이 생겨도 무엇을 근거로 무엇을 정했는지가 사라지지
// 않는다 — 편집한 세션 파일과 선언, 확정 계획, 판정 원장이 그대로 남는다.
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
	publicsite "github.com/sntsoftgit/pqcaton/site"
)

func main() {
	fs := flag.NewFlagSet("pqcaton-ui", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8765", "address to listen on")
	declPath := fs.String("decl", "", "declaration file (declaration.json). Given this, the declaration screen opens")
	resultsDir := fs.String("results", "", "directory of collected results. With -decl, the reconciliation screen opens")
	scopePath := fs.String("scope", "", "asset scope session file. With -layers, the screen raises one itself")
	layerList := fs.String("layers", "", "asset scope layer CSVs, comma separated. They stack in the order given (org, environment, node group), and **the later layer wins** when rules clash. Given these, rules can be edited on the screen")
	basePath := fs.String("base", "", "the policy CSV in force. Given this, only changed rules come up for review")
	scopeOut := fs.String("scope-out", "asset-scope.csv", "file to write the finalized scope policy to")
	judgments := fs.String("judgments", "", "file to append judgments to on finalize (JSONL, append-only)")
	orgName := fs.String("org", "local", "organization the judgments are bound to")
	planOut := fs.String("plan", "plan.json", "file to write the finalized plan to")

	// **위치 인자를 먼저 걷고 나머지를 플래그로 넘긴다.** 표준 flag 는 첫 비플래그에서
	// 파싱을 멈추므로, 그냥 두면 `pqcaton-ui session.json -addr ...` 의 -addr 이 조용히
	// 무시되고 기본 주소로 뜬다. 다른 명령들과 같은 규칙이다.
	pos, flags := splitArgs(os.Args[1:])
	if err := fs.Parse(flags); err != nil {
		os.Exit(2)
	}
	if len(pos) < 1 {
		fmt.Fprintln(os.Stderr, "usage: pqcaton-ui <session.json> [-decl declaration.json] [-results <dir>] [-layers <layer.csv>,...] [-base <in-force.csv>] [-addr 127.0.0.1:8765] [-judgments <file>] [-org <name>] [-plan <file>]")
		os.Exit(2)
	}
	path := pos[0]
	if _, err := review.Load(path); err != nil {
		// **재료가 있으면 화면이 직접 연다.** 선언과 관측 결과가 곧 리뷰 큐의 재료다 —
		// 그것을 손에 들고도 명령을 한 번 돌려야 화면이 열리는 것은, 화면을 두는 이유와
		// 어긋난다.
		if *declPath == "" || *resultsDir == "" {
			fmt.Fprintln(os.Stderr, "❌ cannot read the review session:", err)
			fmt.Fprintln(os.Stderr, "   Give it the declaration and the results and the screen raises one itself: -decl declaration.json -results <results-dir>")
			fmt.Fprintln(os.Stderr, "   To make one with a command: `pqcaton-decide open <declaration.json> -results <results-dir> > "+path+"`")
			os.Exit(1)
		}
	}
	if *declPath != "" {
		if _, err := decl.Load(*declPath); err != nil {
			// 선언은 사람이 처음 쓰는 것이라 만들어 줄 명령이 없다 — 빈 파일에서
			// 시작할 수 있다는 것을 말한다.
			fmt.Fprintln(os.Stderr, "❌ cannot read the declaration file:", err)
			fmt.Fprintln(os.Stderr, `   To start from an empty declaration: echo '{"scope":[],"nodes":[],"assets":[],"edges":[]}' > `+*declPath)
			os.Exit(1)
		}
	}

	if *resultsDir != "" && *declPath == "" {
		// **선언 없이는 대조할 것이 없다.** 조용히 빈 화면을 주면 사람이 무엇이 빠졌는지 모른다.
		fmt.Fprintln(os.Stderr, "❌ -results needs -decl — reconciliation means matching against a declaration")
		os.Exit(2)
	}
	layers := splitPaths(*layerList)
	if len(layers) > 0 && *scopePath == "" {
		// **계층을 줬으면 세션은 화면이 만든다.** 사람이 먼저 명령을 돌려야 화면이
		// 열리는 것은, 화면을 두는 이유와 어긋난다.
		*scopePath = defaultScopeSession
	}
	if *scopePath != "" && len(layers) == 0 {
		if _, err := scope.LoadSession(*scopePath); err != nil {
			// 계층을 주지 않았으면 화면은 세션을 만들 재료가 없다 — 어디서 나는지 말한다.
			fmt.Fprintln(os.Stderr, "❌ cannot read the scope session:", err)
			fmt.Fprintln(os.Stderr, "   Give it the layer CSVs and the screen raises one itself: -layers corp.csv,prod.csv -base asset-scope.csv")
			fmt.Fprintln(os.Stderr, "   To make one with a command: `pqcaton-scope open <layer.csv>... -base <in-force.csv> > "+*scopePath+"`")
			os.Exit(1)
		}
	}
	if _, err := scope.LoadLayers(layers); err != nil {
		fmt.Fprintln(os.Stderr, "❌ cannot read the layer CSVs:", err)
		os.Exit(1)
	}
	s := &server{path: path, decl: *declPath, results: *resultsDir, scope: *scopePath,
		layers: layers, base: *basePath,
		judgments: *judgments, org: *orgName, planOut: *planOut, scopeOut: *scopeOut}
	h := s.handler()

	if !loopback(*addr) {
		// **조용히 열지 않는다.** 리뷰 큐는 그 조직의 공격면이다.
		fmt.Fprintf(os.Stderr,
			"⚠ %s is not loopback — the screen is open on the network. Put authentication in front of it.\n", *addr)
	}
	fmt.Fprintf(os.Stderr, "screen: http://%s  (session %s", *addr, path)
	if *declPath != "" {
		fmt.Fprintf(os.Stderr, " · declaration %s", *declPath)
	}
	fmt.Fprintln(os.Stderr, ")")
	if err := http.ListenAndServe(*addr, h); err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
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

// loopback — 이 주소가 이 기계 밖에서 닿는가.
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
	org       string
	planOut   string
	scopeOut  string
}

// handler — 주소와 처리기를 잇는다.
//
// **메서드를 라우터가 가른다.** 예전에는 처리기마다 `POST 인가`를 손으로 물었고, 그
// 물음을 빠뜨린 자리는 GET 으로도 확정이 돌 수 있었다 — 새로고침 한 번에 확정이 다시
// 타는 길이다. 여기서는 `r.Post` 로 등록하지 않은 주소에 POST 가 닿지 않는다.
//
// Recoverer 를 두는 이유: 화면 하나가 터져도 나머지 절차는 계속 다뤄야 한다. 리뷰 중에
// 서버가 죽으면 적던 판정이 통째로 날아간다.
func (s *server) handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	// 고른 말을 기억한다. 화면을 옮길 때마다 다시 고르게 하지 않는다.
	r.Use(rememberLang)

	// 화면이 브라우저로 내보내는 것(스타일 · htmx). 바이너리에 박혀 있다.
	r.Handle(ui.StaticPath+"*", ui.Static())

	// **첫 화면은 절차의 첫 자리다.** 선언이 있으면 거기서 시작하고, 없으면 리뷰 큐다.
	r.Get("/", s.home)
	// 비교 화면은 제품의 파일을 읽거나 쓰지 않는다. 같은 정적 원본을 Pages와 이 바이너리에
	// 함께 싣는다. 현행 절차 화면과 헷갈리지 않게 별도 주소로만 연다.
	r.Get("/ui-next.html", uiNext)
	r.Get("/decl", s.declEdit)
	r.Get(ui.ScreenDeclNext, s.declNext)
	r.Get(ui.RowPath, s.declRow)
	r.Get(ui.RemovePath, s.declRemove)
	r.Post("/decl/save", s.declSave)
	r.Get("/scope", s.scopeEdit)
	r.Get(ui.ScopeRowPath, s.scopeRow)
	r.Post("/scope/rules", s.scopeRules)
	r.Post("/scope/save", s.scopeSave)
	r.Post("/scope/finalize", s.scopeFinalize)
	r.Get("/survey", s.survey)
	r.Get(ui.ScreenInventory, s.inventory)
	r.Get("/review", s.review)
	r.Post("/save", s.save)
	r.Post("/finalize", s.finalize)
	return r
}

// uiNext — 현재 UI와 나란히 검토할 읽기 전용 정적 프로토타입.
//
// ServeFile로 소스 트리를 읽으면 설치된 바이너리에서는 404가 된다. site.UINext를 쓰면
// Pages의 원본과 같은 바이트를 망이 끊긴 기계에서도 낸다.
func uiNext(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(publicsite.UINext)
}

// rememberLang — 주소로 말을 고르면 쿠키에 남긴다.
//
// **주소에 실린 말이 쿠키를 이긴다**(ui.PickLang). 여기서는 그 선택을 기억만 한다 —
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
	return ui.Page{
		Title: ui.ScreenTitle(here, l), Subtitle: subtitle,
		Nav: ui.NavFor(l, here, ui.Screens{
			Decl: s.decl != "", Scope: s.scope != "", Survey: s.results != "",
			// 조회는 관측 결과가 있어야 볼 것이 있다. 원장과 정책은 있으면 절이 더 열린다.
			Inventory: s.results != "",
		}),
		Lang: l, LangHref: ui.SwitchHref(r, l.Other()),
		Message: r.URL.Query().Get("msg"), Problem: r.URL.Query().Get("problem"),
	}
}

// sub — 부제. 「무엇을 보고 있나」라 값은 그대로 두고 이름표만 그 말로 옮긴다.
func sub(parts ...string) string { return strings.Join(parts, " · ") }

// ── 리뷰 큐 ────────────────────────────────────────────────────────────────

// reviewSession — 리뷰 세션을 읽는다. **관측 결과가 있으면 그것이 정답지다.**
//
// 큐는 관측에서 파생된 것이라, 결과가 늘었는데 세션이 그대로면 화면이 옛 큐를 보여
// 준다 — 방금 나타난 UNDECLARED 가 판정 대상에 없는 상태다. 그래서 읽을 때마다 다시 세우고,
// 사람이 적은 판정은 [review.Carry] 가 들고 간다. 파일에 쓰는 것은 저장·확정할 때뿐이다.
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
	b, err := review.FromResults(s.results, d, s.org)
	if err != nil {
		return prev, nil, err
	}
	return review.Carry(prev, b.Session), b.Warnings, nil
}

func (s *server) review(w http.ResponseWriter, r *http.Request) {
	sf, warn, err := s.reviewSession()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := s.page(r, ui.ScreenReview, sub(sf.Scope, ui.LabelSession(ui.PickLang(r))+" "+s.path))
	page.Warnings = ui.Warnings(ui.PickLang(r), warn)
	html(w, func() error { return ui.RenderReview(w, ui.NewReviewView(sf, page)) })
}

// applyReview — 폼 값을 얹어 파일에 쓴다. **읽고 얹고 쓴다** — 화면이 자기 사본을 들고
// 있으면 그 사이 파일을 고친 사람의 편집이 사라진다.
func (s *server) applyReview(r *http.Request) (review.Session, error) {
	sf, _, err := s.reviewSession()
	if err != nil {
		return sf, err
	}
	if err := r.ParseForm(); err != nil {
		return sf, err
	}
	sf = ui.ApplyReview(sf, r.PostForm)
	return sf, review.Save(s.path, sf)
}

func (s *server) save(w http.ResponseWriter, r *http.Request) {
	if _, err := s.applyReview(r); err != nil {
		redirect(w, r, "/review", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	redirect(w, r, "/review", ui.MsgSavedNotFinal(ui.PickLang(r)), "")
}

// finalize — **명령과 같은 같은 검사를 거친다.** 여기서 따로 판정하지 않는다.
func (s *server) finalize(w http.ResponseWriter, r *http.Request) {
	sf, err := s.applyReview(r)
	if err != nil {
		redirect(w, r, "/review", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	res, err := review.Finalize(sf)
	if err != nil {
		// 확정되지 않은 이유를 그대로 보여 준다 — 무엇이 남았는지가 거기 적혀 있다.
		redirect(w, r, "/review", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	raw, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(res.Plan)
	if err != nil {
		redirect(w, r, "/review", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	if err := os.WriteFile(s.planOut, append(raw, '\n'), 0o644); err != nil {
		redirect(w, r, "/review", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	msg := ui.MsgFinalizedPlan(ui.PickLang(r), len(res.Plan.GetActions()), s.planOut)
	// **관문을 지난 뒤에만 남긴다.**
	if s.judgments != "" {
		n, err := review.SaveJudgments(s.judgments, s.org, sf, res.Decided)
		if err != nil {
			redirect(w, r, "/review", msg, "recording judgments: "+err.Error())
			return
		}
		msg += ui.MsgJudgmentsSaved(ui.PickLang(r), n, s.judgments)
	}
	redirect(w, r, "/review", msg, "")
}

// ── 자산 스코프 ────────────────────────────────────────────────────────────

// scopeSession — 스코프 세션을 읽는다. **계층 파일이 있으면 그것이 정답지다.**
//
// 세션은 계층에서 파생된 것이라, 파일을 고쳐 놓고 세션을 안 고치면 화면이 옛 변경을
// 보여 준다. 그래서 읽을 때마다 계층에서 다시 세운다 — 사람이 적은 판정은 [scope.Reopen]
// 이 들고 간다. 파일에 쓰는 것은 저장·확정할 때뿐이다.
func (s *server) scopeSession() (scope.Session, []scope.LayerFile, error) {
	sf, err := scope.LoadSession(s.scope)
	if err != nil && (len(s.layers) == 0 || !os.IsNotExist(err)) {
		return sf, nil, err
	}
	if len(s.layers) == 0 {
		return sf, nil, nil
	}
	files, err := scope.LoadLayers(s.layers)
	if err != nil {
		return sf, nil, err
	}
	var base *kscope.AssetPolicy
	if s.base != "" {
		if base, err = scope.LoadPolicyFile(s.base); err != nil {
			return sf, nil, err
		}
	}
	return scope.Reopen(sf, scope.Layers(files), base, s.org), files, nil
}

func (s *server) scopeEdit(w http.ResponseWriter, r *http.Request) {
	if s.scope == "" {
		http.Error(w, "no scope session was given — pass -scope or -layers", http.StatusNotFound)
		return
	}
	sf, files, err := s.scopeSession()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := s.page(r, ui.ScreenScope, sub(ui.LabelOrg(ui.PickLang(r))+" "+sf.Org, s.scope))
	v := ui.NewScopeView(sf, page)
	if len(files) > 0 {
		v = v.Editable(files)
	}
	html(w, func() error { return ui.RenderScope(w, v) })
}

// scopeRules — 화면에서 고친 규칙을 계층 파일에 쓴다.
//
// **판정을 먼저 얹고 규칙을 쓴다.** 같은 폼에서 온 값이라, 규칙만 저장하면 그 사이에
// 적어 둔 결론이 날아간다.
func (s *server) scopeRules(w http.ResponseWriter, r *http.Request) {
	if len(s.layers) == 0 {
		http.Error(w, "no layer CSVs were given — pass -layers", http.StatusNotFound)
		return
	}
	sf, files, err := s.scopeSession()
	if err != nil {
		redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	if err := r.ParseForm(); err != nil {
		redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	sf = ui.ApplyScope(sf, r.PostForm)

	edited := ui.ApplyLayers(files, r.PostForm)
	for _, lf := range edited {
		if err := scope.SaveLayer(lf); err != nil {
			redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
			return
		}
	}
	// 규칙이 달라졌으니 변경을 다시 센다. 적어 둔 판정은 규칙 동일성으로 따라온다.
	var base *kscope.AssetPolicy
	if s.base != "" {
		if base, err = scope.LoadPolicyFile(s.base); err != nil {
			redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
			return
		}
	}
	next := scope.Reopen(sf, scope.Layers(edited), base, s.org)
	if err := scope.SaveSession(s.scope, next); err != nil {
		redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	n := 0
	for _, lf := range edited {
		n += len(lf.Layer.Rules)
	}
	msg := ui.MsgRulesSaved(ui.PickLang(r), len(edited), n, len(next.Changes), next.AuditedCount())
	if next.Signature == "" && sf.Signature != "" {
		// **말하지 않으면 사람은 서명이 남아 있다고 여긴다.**
		msg += ui.MsgSignatureCleared(ui.PickLang(r))
	}
	redirect(w, r, "/scope", msg, "")
}

// scopeRow — 규칙 표의 「행 추가」.
func (s *server) scopeRow(w http.ResponseWriter, r *http.Request) {
	if len(s.layers) == 0 {
		http.Error(w, "no layer CSVs were given", http.StatusNotFound)
		return
	}
	layer, err := strconv.Atoi(r.URL.Query().Get("layer"))
	if err != nil || layer < 0 || layer >= len(s.layers) {
		http.Error(w, "no such layer", http.StatusBadRequest)
		return
	}
	i, err := strconv.Atoi(r.URL.Query().Get("i"))
	if err != nil || i < 0 || i > maxRows {
		http.Error(w, "the row number is out of range", http.StatusBadRequest)
		return
	}
	html(w, func() error { return ui.RenderRuleRow(w, ui.PickLang(r), layer, i) })
}

// applyScope — 폼 값을 얹어 파일에 쓴다. 읽고 얹고 쓴다.
func (s *server) applyScope(r *http.Request) (scope.Session, error) {
	sf, _, err := s.scopeSession()
	if err != nil {
		return sf, err
	}
	if err := r.ParseForm(); err != nil {
		return sf, err
	}
	sf = ui.ApplyScope(sf, r.PostForm)
	return sf, scope.SaveSession(s.scope, sf)
}

func (s *server) scopeSave(w http.ResponseWriter, r *http.Request) {
	if s.scope == "" {
		http.Error(w, "no scope session was given", http.StatusNotFound)
		return
	}
	if _, err := s.applyScope(r); err != nil {
		redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	redirect(w, r, "/scope", ui.MsgSavedNotFinal(ui.PickLang(r)), "")
}

// scopeFinalize — **명령과 같은 같은 검사를 거친다.**
func (s *server) scopeFinalize(w http.ResponseWriter, r *http.Request) {
	if s.scope == "" {
		http.Error(w, "no scope session was given", http.StatusNotFound)
		return
	}
	sf, err := s.applyScope(r)
	if err != nil {
		redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	res, err := scope.Finalize(sf, s.org)
	if err != nil {
		redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	f, err := os.Create(s.scopeOut)
	if err != nil {
		redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	err = scope.WriteCSV(f, res.Policy)
	f.Close()
	if err != nil {
		redirect(w, r, "/scope", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	msg := ui.MsgFinalizedPolicy(ui.PickLang(r), len(res.Policy.Rules), s.scopeOut)
	// **관문을 지난 뒤에만 남긴다.**
	if s.judgments != "" {
		n, err := scope.SaveJudgments(s.judgments, s.org, sf, res.Decided)
		if err != nil {
			redirect(w, r, "/scope", msg, "recording judgments: "+err.Error())
			return
		}
		msg += ui.MsgJudgmentsSaved(ui.PickLang(r), n, s.judgments)
	}
	redirect(w, r, "/scope", msg, "")
}

// ── 대조 ───────────────────────────────────────────────────────────────────

// survey — 관측을 모아 선언과 대조한 것을 보여 준다. **계산은 report 패키지가 한다** —
// 명령(`pqcaton-report`)이 글로 찍는 것과 같은 것을 표로 그린다.
func (s *server) survey(w http.ResponseWriter, r *http.Request) {
	if s.results == "" {
		http.Error(w, "no results were given — pass -results", http.StatusNotFound)
		return
	}
	d, err := decl.Load(s.decl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res, err := report.Build(s.results, d)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := ui.NewSurveyView(res, s.page(r, ui.ScreenSurvey,
		sub(ui.LabelOrg(ui.PickLang(r))+" "+res.Org, ui.LabelResults(ui.PickLang(r))+" "+s.results)))
	v.SVG = renderDOT(v.DOT)
	html(w, func() error { return ui.RenderSurvey(w, v) })
}

// renderDOT — `dot` 이 있으면 SVG 로 그린다. 없으면 빈 값을 돌려주고 화면이 원문을 보인다.
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
	// `dot` 이 낸 SVG 다. 우리가 만든 DOT 에서 나온 것이라 밖에서 온 값이 아니다.
	return string(out)
}

// ── 인벤토리 조회 ──────────────────────────────────────────────────────────

// inventory — 찾아보는 자리. **절차의 한 단계가 아니다.**
//
// 대조 화면은 지금 대조한 것을 전부 늘어놓는다 — 수천 대에서는 그 자체가 못 쓰는
// 화면이다. 여기서는 조건을 걸어 찾고, 자산 하나의 판정 이력을 열고, 정책이 뺀 것과 근거가
// 바뀐 판정을 본다. **전부 손에 든 파일에서 나온다.**
func (s *server) inventory(w http.ResponseWriter, r *http.Request) {
	if s.results == "" {
		http.Error(w, "no results were given — pass -results", http.StatusNotFound)
		return
	}
	d, err := decl.Load(s.decl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	res, err := report.Build(s.results, d)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	l := ui.PickLang(r)
	page := s.page(r, ui.ScreenInventory, sub(ui.LabelOrg(l)+" "+res.Org, ui.LabelResults(l)+" "+s.results))
	v := ui.NewInventoryView(res, ui.FilterFrom(r.URL.Query()), page)

	if v, err = s.withPolicy(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if v, err = s.withLedger(v, res, r.URL.Query().Get("subject")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	html(w, func() error { return ui.RenderInventory(w, v) })
}

// withPolicy — 「안 보고 있는 것」. 확정된 정책 CSV 가 있어야 셀 수 있다.
func (s *server) withPolicy(v ui.InventoryView) (ui.InventoryView, error) {
	pol, err := scope.LoadPolicyFile(s.scopeOut)
	if err != nil {
		// **없는 것은 오류가 아니다.** 아직 정책을 확정하지 않았을 뿐이고, 화면은 그
		// 절에서 무엇을 주면 열리는지 말한다.
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
	// 지금 관측이 만드는 근거. **명령의 delta 와 같은 계산이라야** 화면과 명령이 같은
	// 것을 「바뀌었다」고 말한다.
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
	store, err := decision.NewFileJudgmentStore(org.ID(s.org), s.judgments)
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

// defaultTTLDays — 제외 승인의 유효기간. `pqcaton-scope` 와 같은 값이라야 화면과 명령이
// 같은 것을 「오래됐다」고 말한다.
const defaultTTLDays = 180

// ── 선언 ───────────────────────────────────────────────────────────────────

// declEdit · declNext — 같은 값을 서로 다른 판으로 그린다.
//
// **뷰를 하나만 만든다.** 두 화면이 각자 세면 숫자가 갈리는 날이 오고, 그날 어느 쪽이
// 맞는지 아무도 모른다. 갈리는 것은 그리는 방식뿐이다.
func (s *server) declEdit(w http.ResponseWriter, r *http.Request) {
	s.declScreen(w, r, ui.RenderDecl)
}

func (s *server) declNext(w http.ResponseWriter, r *http.Request) {
	s.declScreen(w, r, ui.RenderDeclNext)
}

func (s *server) declScreen(w http.ResponseWriter, r *http.Request, render func(io.Writer, ui.DeclView) error) {
	if s.decl == "" {
		http.Error(w, "no declaration file was given — pass -decl", http.StatusNotFound)
		return
	}
	d, err := decl.Load(s.decl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := ui.NewDeclView(d, s.page(r, ui.ScreenDecl, sub(ui.LabelOrg(ui.PickLang(r))+" "+d.OrgOrDefault(), s.decl)))
	seen, unmatched := s.observedAssets(d)
	v = v.WithObserved(seen, unmatched)
	html(w, func() error { return render(w, v) })
}

// observedAssets — 노드마다 **관측된** 자산. 컴포넌트 칸의 후보다.
//
// **관측 결과에 적힌 이름이 곧 맞는 이름이다.** 대조는 글자 그대로 같을 때만 맞는데, 관측 이름은
// `.so` 뒤가 떼인 채로 온다 — 사람이 대조 화면을 보고 옮겨 적으면 거기서 틀린다.
//
// 결과를 읽지 못해도 선언 화면은 열려야 한다. 후보는 거들 뿐이고, 없으면 손으로 적는다.
// unmatched — 관측에는 있는데 어느 선언 노드에도 붙지 않은 노드 이름. 「관측 이름」 칸의
// 후보다. 붙지 않았다는 것은 그 노드의 자산이 통째로 UNDECLARED 로 오른다는 뜻이고, 그것은
// 선언이 틀려서가 아니라 이름이 서로 달라서다.
func (s *server) observedAssets(d decl.Declaration) (map[string][]ui.DeclAsset, []string) {
	if s.results == "" {
		return nil, nil
	}
	res, err := report.Build(s.results, d)
	if err != nil {
		return nil, nil
	}
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
		http.Error(w, "no declaration file was given", http.StatusNotFound)
		return
	}
	kind := r.URL.Query().Get("kind")
	if !ui.ValidKind(kind) {
		http.Error(w, "unknown table name: "+kind, http.StatusBadRequest)
		return
	}
	i, err := strconv.Atoi(r.URL.Query().Get("i"))
	if err != nil || i < 0 || i > maxRows {
		http.Error(w, "the row number is out of range", http.StatusBadRequest)
		return
	}
	// 자산은 어느 노드의 것인지가 함께 온다 — 그 번호도 밖에서 오는 값이다.
	node := 0
	if v := r.URL.Query().Get("node"); v != "" {
		if node, err = strconv.Atoi(v); err != nil || node < 0 || node > maxRows {
			http.Error(w, "the node number is out of range", http.StatusBadRequest)
			return
		}
	}
	html(w, func() error { return ui.RenderRow(w, ui.PickLang(r), kind, node, i) })
}

// declRemove — 「제거」. **빈 응답을 돌려준다** — 지우는 것은 브라우저가 하고(htmx 가
// 그 줄을 빈 것으로 갈아 끼운다), 파일이 달라지는 것은 저장할 때뿐이다.
//
// 서버가 아무것도 바꾸지 않으므로 GET 이다. 새로고침으로 무엇이 다시 일어나지 않는다.
func (s *server) declRemove(w http.ResponseWriter, r *http.Request) {
	if s.decl == "" {
		http.Error(w, "no declaration file was given", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// maxRows — 한 표에 열어 줄 줄 수의 상한. 사람이 화면에서 손으로 넣는 자리라 이보다
// 많아질 일이 없다. 많다면 파일을 만들어 넣을 일이다.
const maxRows = 10000

func (s *server) declSave(w http.ResponseWriter, r *http.Request) {
	if s.decl == "" {
		http.Error(w, "no declaration file was given", http.StatusNotFound)
		return
	}
	prev, err := decl.Load(s.decl)
	if err != nil {
		redirect(w, r, "/decl", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	if err := r.ParseForm(); err != nil {
		redirect(w, r, "/decl", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	d, dropped := ui.ApplyDecl(prev, r.PostForm)
	if err := decl.Save(s.decl, d); err != nil {
		redirect(w, r, "/decl", "", ui.Refusal(ui.PickLang(r), err))
		return
	}
	msg := ui.MsgDeclSaved(ui.PickLang(r), len(d.Nodes), len(d.Assets), len(d.Edges))
	// **뺀 줄은 말한다.** IP 없는 줄은 관리 대상이 아니라 저장에서 빠지는데, 표에서
	// 사라진 것만 보이면 지워진 것으로 읽힌다.
	if len(dropped) > 0 {
		msg += ui.MsgDeclDroppedNoIP(ui.PickLang(r), len(dropped))
	}
	// **저장은 됐지만 앞뒤가 안 맞으면 그 사실을 함께 말한다.** 막지는 않는다.
	if p := decl.Check(d); len(p) > 0 {
		msg += ui.MsgDeclStillOff(ui.PickLang(r), len(p))
	}
	redirect(w, r, "/decl", msg, "")
}

// ── 공통 ───────────────────────────────────────────────────────────────────

// redirect — POST 뒤에는 GET 으로 보낸다. 새로고침이 같은 확정을 다시 태우지 않게 한다.
func redirect(w http.ResponseWriter, r *http.Request, to, msg, problem string) {
	u := to + "?msg=" + url.QueryEscape(msg)
	if problem != "" {
		u += "&problem=" + url.QueryEscape(problem)
	}
	http.Redirect(w, r, u, http.StatusSeeOther)
}

func html(w http.ResponseWriter, render func() error) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
