// Command pqcaton-ui — 리뷰 큐와 선언을 사람이 다루는 화면.
//
// **파일에서 읽고 파일에 쓰는 껍데기다.** 화면 자체는 `pkg/inventory/ui` 에 있고, 확정
// 게이트는 `pkg/inventory/review` 에 있다 — 컨트롤 플레인이 같은 화면과 같은 게이트를
// 쓰고, 다른 것은 「어디서 읽고 누가 들어오나」뿐이다.
//
//	pqcaton-ui <session.json> [-decl declaration.json] [-addr 127.0.0.1:8765]
//	           [-judgments 파일] [-org 이름] [-plan 파일]
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
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func main() {
	fs := flag.NewFlagSet("pqcaton-ui", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8765", "들을 주소")
	declPath := fs.String("decl", "", "선언 파일(declaration.json). 주면 선언 편집 화면이 열린다")
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
		fmt.Fprintln(os.Stderr, "usage: pqcaton-ui <session.json> [-decl declaration.json] [-addr 127.0.0.1:8765] [-judgments 파일] [-org 이름] [-plan 파일]")
		os.Exit(2)
	}
	path := pos[0]
	if _, err := review.Load(path); err != nil {
		fmt.Fprintln(os.Stderr, "❌ 세션 파일을 읽을 수 없다:", err)
		os.Exit(1)
	}
	if *declPath != "" {
		if _, err := decl.Load(*declPath); err != nil {
			fmt.Fprintln(os.Stderr, "❌ 선언 파일을 읽을 수 없다:", err)
			os.Exit(1)
		}
	}

	s := &server{path: path, decl: *declPath, judgments: *judgments, org: *orgName, planOut: *planOut}
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
	judgments string
	org       string
	planOut   string
}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/save", s.save)
	mux.HandleFunc("/finalize", s.finalize)
	mux.HandleFunc("/decl", s.declEdit)
	mux.HandleFunc("/decl/save", s.declSave)
}

// nav — 위쪽 이동 링크. 선언 파일을 주지 않았으면 그 자리를 만들지 않는다 —
// **없는 것을 눌러 보게 하지 않는다.**
func (s *server) nav(here string) []ui.Link {
	links := []ui.Link{{Href: "/", Text: "리뷰 큐", Here: here == "/"}}
	if s.decl != "" {
		links = append(links, ui.Link{Href: "/decl", Text: "선언", Here: here == "/decl"})
	}
	return links
}

func (s *server) page(r *http.Request, title, subtitle, here string) ui.Page {
	return ui.Page{
		Title: title, Subtitle: subtitle, Nav: s.nav(here),
		Message: r.URL.Query().Get("msg"), Problem: r.URL.Query().Get("problem"),
	}
}

// ── 리뷰 큐 ────────────────────────────────────────────────────────────────

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	sf, err := review.Load(s.path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := ui.NewReviewView(sf, s.page(r, "리뷰 큐", sf.Scope+" · 세션 "+s.path, "/"))
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
		redirect(w, r, "/", "", err.Error())
		return
	}
	redirect(w, r, "/", "세션 파일에 저장했습니다 — 아직 확정하지 않았습니다", "")
}

// finalize — **명령과 같은 게이트를 탄다.** 여기서 따로 판정하지 않는다.
func (s *server) finalize(w http.ResponseWriter, r *http.Request) {
	if !post(w, r) {
		return
	}
	sf, err := s.applyReview(r)
	if err != nil {
		redirect(w, r, "/", "", err.Error())
		return
	}
	res, err := review.Finalize(sf)
	if err != nil {
		// 확정되지 않은 이유를 그대로 보여 준다 — 무엇이 남았는지가 거기 적혀 있다.
		redirect(w, r, "/", "", err.Error())
		return
	}
	raw, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(res.Plan)
	if err != nil {
		redirect(w, r, "/", "", err.Error())
		return
	}
	if err := os.WriteFile(s.planOut, append(raw, '\n'), 0o644); err != nil {
		redirect(w, r, "/", "", err.Error())
		return
	}
	msg := fmt.Sprintf("확정했습니다 — 조치 %d건을 %s 에 썼습니다", len(res.Plan.GetActions()), s.planOut)
	// **게이트를 지난 뒤에만 남긴다.**
	if s.judgments != "" {
		n, err := review.SaveJudgments(s.judgments, s.org, sf, res.Decided)
		if err != nil {
			redirect(w, r, "/", msg, "판정 기록: "+err.Error())
			return
		}
		msg += fmt.Sprintf(" · 판정 %d건을 %s 에 남겼습니다", n, s.judgments)
	}
	redirect(w, r, "/", msg, "")
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
