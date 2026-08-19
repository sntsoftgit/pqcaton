// Command pqcaton-ui — 리뷰 큐와 선언을 사람이 다루는 화면.
//
// **파일에서 읽고 파일에 쓰는 껍데기다.** 화면 자체는 `pkg/inventory/ui` 에 있고, 확정
// 게이트는 `pkg/inventory/review` 에 있다 — 컨트롤 플레인이 같은 화면과 같은 게이트를
// 쓰고, 다른 것은 「어디서 읽고 누가 들어오나」뿐이다.
//
//	pqcaton-ui <session.json> [-decl declaration.json] [-scope scope-session.json]
//	           [-results 디렉터리] [-addr 127.0.0.1:8765] [-judgments 파일] [-org 이름]
//	           [-plan 파일] [-scope-out 파일]
//
// **탭 순서가 절차 순서다** — 선언 → 스코프 → 대조 → 리뷰 큐. 쓰는 사람이 다음에 무엇을
// 할지 화면이 말해 준다. 재료를 주지 않은 자리는 만들지 않는다.
//
// **의존성이 없다.** net/http 와 html/template 만 쓴다 — 링크되는 모듈이 늘지 않는다.
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
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func main() {
	fs := flag.NewFlagSet("pqcaton-ui", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8765", "들을 주소")
	declPath := fs.String("decl", "", "선언 파일(declaration.json). 주면 선언 편집 화면이 열린다")
	resultsDir := fs.String("results", "", "관측 결과 디렉터리. -decl 과 함께 주면 대조 화면이 열린다")
	scopePath := fs.String("scope", "", "자산 스코프 세션(pqcaton-scope open 산출물). 주면 스코프 화면이 열린다")
	scopeOut := fs.String("scope-out", "asset-scope.csv", "확정된 스코프 정책을 쓸 파일")
	judgments := fs.String("judgments", "", "확정 시 판정을 남길 파일(JSONL, append-only)")
	orgName := fs.String("org", "local", "판정을 묶을 조직")
	planOut := fs.String("plan", "plan.json", "확정 계획을 쓸 파일")

	// **위치 인자를 먼저 걷고 나머지를 플래그로 넘긴다.** 표준 flag 는 첫 비플래그에서
	// 파싱을 멈추므로, 그냥 두면 `pqcaton-ui session.json -addr ...` 의 -addr 이 조용히
	// 무시되고 기본 주소로 뜬다. 다른 명령들과 같은 규칙이다.
	pos, flags := splitArgs(os.Args[1:])
	if err := fs.Parse(flags); err != nil {
		os.Exit(2)
	}
	if len(pos) < 1 {
		fmt.Fprintln(os.Stderr, "usage: pqcaton-ui <session.json> [-decl declaration.json] [-results 디렉터리] [-addr 127.0.0.1:8765] [-judgments 파일] [-org 이름] [-plan 파일]")
		os.Exit(2)
	}
	path := pos[0]
	if _, err := review.Load(path); err != nil {
		fmt.Fprintln(os.Stderr, "❌ 리뷰 세션을 읽을 수 없다:", err)
		fmt.Fprintln(os.Stderr, "   `pqcaton-decide open <declaration.json> -results <results-dir> > "+path+"` 로 먼저 만드십시오")
		os.Exit(1)
	}
	if *declPath != "" {
		if _, err := decl.Load(*declPath); err != nil {
			// 선언은 사람이 처음 쓰는 것이라 만들어 줄 명령이 없다 — 빈 파일에서
			// 시작할 수 있다는 것을 말한다.
			fmt.Fprintln(os.Stderr, "❌ 선언 파일을 읽을 수 없다:", err)
			fmt.Fprintln(os.Stderr, `   빈 선언으로 시작하려면: echo '{"scope":[],"nodes":[],"assets":[],"edges":[]}' > `+*declPath)
			os.Exit(1)
		}
	}

	if *resultsDir != "" && *declPath == "" {
		// **선언 없이는 대조할 것이 없다.** 조용히 빈 화면을 주면 사람이 무엇이 빠졌는지 모른다.
		fmt.Fprintln(os.Stderr, "❌ -results 는 -decl 과 함께 주십시오 — 대조는 선언과 맞대는 일입니다")
		os.Exit(2)
	}
	if *scopePath != "" {
		if _, err := scope.LoadSession(*scopePath); err != nil {
			// **어디서 나는 파일인지 말한다.** 화면은 세션을 열지 못한다 — 여는 일은
			// 관측·대조가 필요하고, 화면은 그 결과를 채우는 자리다.
			fmt.Fprintln(os.Stderr, "❌ 스코프 세션을 읽을 수 없다:", err)
			fmt.Fprintln(os.Stderr, "   `pqcaton-scope open <계층.csv>... -base <현재정책.csv> > "+*scopePath+"` 로 먼저 만드십시오")
			os.Exit(1)
		}
	}
	s := &server{path: path, decl: *declPath, results: *resultsDir, scope: *scopePath,
		judgments: *judgments, org: *orgName, planOut: *planOut, scopeOut: *scopeOut}
	mux := http.NewServeMux()
	s.routes(mux)

	if !loopback(*addr) {
		// **조용히 열지 않는다.** 리뷰 큐는 그 조직의 공격면이다.
		fmt.Fprintf(os.Stderr,
			"⚠ %s 는 루프백이 아닙니다 — 화면이 네트워크에 열립니다. 앞에 인증을 두십시오.\n", *addr)
	}
	fmt.Fprintf(os.Stderr, "화면: http://%s  (세션 %s", *addr, path)
	if *declPath != "" {
		fmt.Fprintf(os.Stderr, " · 선언 %s", *declPath)
	}
	fmt.Fprintln(os.Stderr, ")")
	if err := http.ListenAndServe(*addr, mux); err != nil {
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

type server struct {
	path      string
	decl      string
	results   string
	scope     string
	judgments string
	org       string
	planOut   string
	scopeOut  string
}

func (s *server) routes(mux *http.ServeMux) {
	// **첫 화면은 절차의 첫 자리다.** 선언이 있으면 거기서 시작하고, 없으면 리뷰 큐다.
	mux.HandleFunc("/", s.home)
	mux.HandleFunc("/decl", s.declEdit)
	mux.HandleFunc("/decl/save", s.declSave)
	mux.HandleFunc("/scope", s.scopeEdit)
	mux.HandleFunc("/scope/save", s.scopeSave)
	mux.HandleFunc("/scope/finalize", s.scopeFinalize)
	mux.HandleFunc("/survey", s.survey)
	mux.HandleFunc("/review", s.review)
	mux.HandleFunc("/save", s.save)
	mux.HandleFunc("/finalize", s.finalize)
}

// home — 절차의 첫 자리로 보낸다.
func (s *server) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	to := "/review"
	if s.decl != "" {
		to = "/decl"
	}
	http.Redirect(w, r, to+"?"+r.URL.RawQuery, http.StatusSeeOther)
}

// nav — 위쪽 이동 링크. **절차 순서로 둔다** — 선언 → 대조 → 판정. 쓰는 사람이 다음에
// 무엇을 할지 순서 자체가 말해 준다.
//
// 재료를 주지 않은 자리는 만들지 않는다 — **없는 것을 눌러 보게 하지 않는다.**
func (s *server) nav(here string) []ui.Link {
	var links []ui.Link
	if s.decl != "" {
		links = append(links, ui.Link{Href: "/decl", Text: "① 선언", Here: here == "/decl"})
	}
	// **스코프는 대조 앞이다.** 무엇을 계속 볼지가 정해져야 관측이 적재되고, 그 뒤에
	// 대조한다 — 절차가 그 순서다.
	if s.scope != "" {
		links = append(links, ui.Link{Href: "/scope", Text: "② 스코프", Here: here == "/scope"})
	}
	if s.results != "" {
		links = append(links, ui.Link{Href: "/survey", Text: "③ 대조", Here: here == "/survey"})
	}
	links = append(links, ui.Link{Href: "/review", Text: "④ 리뷰 큐", Here: here == "/review"})
	return links
}

func (s *server) page(r *http.Request, title, subtitle, here string) ui.Page {
	return ui.Page{
		Title: title, Subtitle: subtitle, Nav: s.nav(here),
		Message: r.URL.Query().Get("msg"), Problem: r.URL.Query().Get("problem"),
	}
}

// ── 리뷰 큐 ────────────────────────────────────────────────────────────────

func (s *server) review(w http.ResponseWriter, r *http.Request) {
	sf, err := review.Load(s.path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := ui.NewReviewView(sf, s.page(r, "리뷰 큐", sf.Scope+" · 세션 "+s.path, "/review"))
	html(w, func() error { return ui.RenderReview(w, v) })
}

// applyReview — 폼 값을 얹어 파일에 쓴다. **읽고 얹고 쓴다** — 화면이 자기 사본을 들고
// 있으면 그 사이 파일을 고친 사람의 편집이 사라진다.
func (s *server) applyReview(r *http.Request) (review.Session, error) {
	sf, err := review.Load(s.path)
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
	if !post(w, r) {
		return
	}
	if _, err := s.applyReview(r); err != nil {
		redirect(w, r, "/review", "", err.Error())
		return
	}
	redirect(w, r, "/review", "세션 파일에 저장했습니다 — 아직 확정하지 않았습니다", "")
}

// finalize — **명령과 같은 게이트를 탄다.** 여기서 따로 판정하지 않는다.
func (s *server) finalize(w http.ResponseWriter, r *http.Request) {
	if !post(w, r) {
		return
	}
	sf, err := s.applyReview(r)
	if err != nil {
		redirect(w, r, "/review", "", err.Error())
		return
	}
	res, err := review.Finalize(sf)
	if err != nil {
		// 확정되지 않은 이유를 그대로 보여 준다 — 무엇이 남았는지가 거기 적혀 있다.
		redirect(w, r, "/review", "", err.Error())
		return
	}
	raw, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(res.Plan)
	if err != nil {
		redirect(w, r, "/review", "", err.Error())
		return
	}
	if err := os.WriteFile(s.planOut, append(raw, '\n'), 0o644); err != nil {
		redirect(w, r, "/review", "", err.Error())
		return
	}
	msg := fmt.Sprintf("확정했습니다 — 조치 %d건을 %s 에 썼습니다", len(res.Plan.GetActions()), s.planOut)
	// **게이트를 지난 뒤에만 남긴다.**
	if s.judgments != "" {
		n, err := review.SaveJudgments(s.judgments, s.org, sf, res.Decided)
		if err != nil {
			redirect(w, r, "/review", msg, "판정 기록: "+err.Error())
			return
		}
		msg += fmt.Sprintf(" · 판정 %d건을 %s 에 남겼습니다", n, s.judgments)
	}
	redirect(w, r, "/review", msg, "")
}

// ── 자산 스코프 ────────────────────────────────────────────────────────────

func (s *server) scopeEdit(w http.ResponseWriter, r *http.Request) {
	if s.scope == "" {
		http.Error(w, "스코프 세션을 주지 않았습니다 — -scope 로 지정하십시오", http.StatusNotFound)
		return
	}
	sf, err := scope.LoadSession(s.scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := ui.NewScopeView(sf, s.page(r, "자산 스코프", "조직 "+sf.Org+" · "+s.scope, "/scope"))
	html(w, func() error { return ui.RenderScope(w, v) })
}

// applyScope — 폼 값을 얹어 파일에 쓴다. 읽고 얹고 쓴다.
func (s *server) applyScope(r *http.Request) (scope.Session, error) {
	sf, err := scope.LoadSession(s.scope)
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
	if !post(w, r) {
		return
	}
	if s.scope == "" {
		http.Error(w, "스코프 세션을 주지 않았습니다", http.StatusNotFound)
		return
	}
	if _, err := s.applyScope(r); err != nil {
		redirect(w, r, "/scope", "", err.Error())
		return
	}
	redirect(w, r, "/scope", "세션 파일에 저장했습니다 — 아직 확정하지 않았습니다", "")
}

// scopeFinalize — **명령과 같은 게이트를 탄다.**
func (s *server) scopeFinalize(w http.ResponseWriter, r *http.Request) {
	if !post(w, r) {
		return
	}
	if s.scope == "" {
		http.Error(w, "스코프 세션을 주지 않았습니다", http.StatusNotFound)
		return
	}
	sf, err := s.applyScope(r)
	if err != nil {
		redirect(w, r, "/scope", "", err.Error())
		return
	}
	res, err := scope.Finalize(sf, s.org)
	if err != nil {
		redirect(w, r, "/scope", "", err.Error())
		return
	}
	f, err := os.Create(s.scopeOut)
	if err != nil {
		redirect(w, r, "/scope", "", err.Error())
		return
	}
	err = scope.WriteCSV(f, res.Policy)
	f.Close()
	if err != nil {
		redirect(w, r, "/scope", "", err.Error())
		return
	}
	msg := fmt.Sprintf("확정했습니다 — 규칙 %d개를 %s 에 썼습니다 (`pqcota-ingest -scope-assets` 의 입력)",
		len(res.Policy.Rules), s.scopeOut)
	// **게이트를 지난 뒤에만 남긴다.**
	if s.judgments != "" {
		n, err := scope.SaveJudgments(s.judgments, s.org, sf, res.Decided)
		if err != nil {
			redirect(w, r, "/scope", msg, "판정 기록: "+err.Error())
			return
		}
		msg += fmt.Sprintf(" · 판정 %d건을 %s 에 남겼습니다", n, s.judgments)
	}
	redirect(w, r, "/scope", msg, "")
}

// ── 대조 ───────────────────────────────────────────────────────────────────

// survey — 관측을 모아 선언과 대조한 것을 보여 준다. **계산은 report 패키지가 한다** —
// 명령(`pqcaton-report`)이 글로 찍는 것과 같은 것을 표로 그린다.
func (s *server) survey(w http.ResponseWriter, r *http.Request) {
	if s.results == "" {
		http.Error(w, "관측 결과를 주지 않았습니다 — -results 로 지정하십시오", http.StatusNotFound)
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
	v := ui.NewSurveyView(res, s.page(r, "대조", "조직 "+res.Org+" · 결과 "+s.results, "/survey"))
	v.SVG = renderDOT(v.DOT)
	html(w, func() error { return ui.RenderSurvey(w, v) })
}

// renderDOT — `dot` 이 있으면 SVG 로 그린다. 없으면 빈 값을 돌려주고 화면이 원문을 보인다.
//
// **의존성이 아니라 있으면 좋은 것이다.** 없다고 화면이 깨지면 표준 라이브러리만 쓴다는
// 약속이 무의미해진다.
func renderDOT(dot string) template.HTML {
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
	return template.HTML(out)
}

// ── 선언 ───────────────────────────────────────────────────────────────────

func (s *server) declEdit(w http.ResponseWriter, r *http.Request) {
	if s.decl == "" {
		http.Error(w, "선언 파일을 주지 않았습니다 — -decl 로 지정하십시오", http.StatusNotFound)
		return
	}
	d, err := decl.Load(s.decl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := ui.NewDeclView(d, s.page(r, "선언", "조직 "+d.OrgOrDefault()+" · "+s.decl, "/decl"))
	html(w, func() error { return ui.RenderDecl(w, v) })
}

func (s *server) declSave(w http.ResponseWriter, r *http.Request) {
	if !post(w, r) {
		return
	}
	if s.decl == "" {
		http.Error(w, "선언 파일을 주지 않았습니다", http.StatusNotFound)
		return
	}
	prev, err := decl.Load(s.decl)
	if err != nil {
		redirect(w, r, "/decl", "", err.Error())
		return
	}
	if err := r.ParseForm(); err != nil {
		redirect(w, r, "/decl", "", err.Error())
		return
	}
	d := ui.ApplyDecl(prev, r.PostForm)
	if err := decl.Save(s.decl, d); err != nil {
		redirect(w, r, "/decl", "", err.Error())
		return
	}
	msg := fmt.Sprintf("선언을 저장했습니다 — 노드 %d · 자산 %d · 엣지 %d",
		len(d.Nodes), len(d.Assets), len(d.Edges))
	// **저장은 됐지만 앞뒤가 안 맞으면 그 사실을 함께 말한다.** 막지는 않는다.
	if p := decl.Check(d); len(p) > 0 {
		msg += fmt.Sprintf(" · 맞지 않는 자리 %d곳이 남아 있습니다", len(p))
	}
	redirect(w, r, "/decl", msg, "")
}

// ── 공통 ───────────────────────────────────────────────────────────────────

func post(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "POST 만 받는다", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

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
