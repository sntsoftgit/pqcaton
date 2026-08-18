// Command pqcaton-ui — 리뷰 큐를 사람이 다루는 화면.
//
// 세션 파일을 열어 **정책 단위로** 결론을 채우고, 확정 버튼이 `pqcaton-decide close` 와
// **같은 게이트**(`review.Finalize`)를 태운다. 게이트가 두 벌이면 언젠가 한쪽만 고쳐진다.
//
//	pqcaton-ui <session.json> [-addr 127.0.0.1:8765] [-judgments 파일] [-org 이름]
//
// **의존성이 없다.** net/http 와 html/template 만 쓴다 — 링크되는 모듈이 늘지 않는다.
//
// **기본은 127.0.0.1 이다.** 리뷰 큐에는 어느 노드가 무엇을 쓰는지가 그대로 있다 — 곧 그
// 조직의 공격면이다. 실수로 밖에 열리는 쪽이 편의보다 훨씬 비싸므로, 밖으로 열려면
// -addr 를 명시적으로 바꿔야 하고 그때 경고한다.
//
// **산출물은 여전히 파일이다.** 화면이 생겨도 무엇을 근거로 무엇을 정했는지가 사라지지
// 않는다 — 편집한 세션 파일과 확정 계획, 판정 원장이 그대로 남는다.
package main

import (
	"flag"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8765", "들을 주소")
	judgments := flag.String("judgments", "", "확정 시 판정을 남길 파일(JSONL, append-only)")
	orgName := flag.String("org", "local", "판정을 묶을 조직")
	planOut := flag.String("plan", "plan.json", "확정 계획을 쓸 파일")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: pqcaton-ui <session.json> [-addr 127.0.0.1:8765] [-judgments 파일] [-org 이름]")
		os.Exit(2)
	}
	path := flag.Arg(0)
	if _, err := review.Load(path); err != nil {
		fmt.Fprintln(os.Stderr, "❌ 세션 파일을 읽을 수 없다:", err)
		os.Exit(1)
	}

	s := &server{path: path, judgments: *judgments, org: *orgName, planOut: *planOut}
	mux := http.NewServeMux()
	s.routes(mux)

	if !loopback(*addr) {
		// **조용히 열지 않는다.** 리뷰 큐는 그 조직의 공격면이다.
		fmt.Fprintf(os.Stderr,
			"⚠ %s 는 루프백이 아닙니다 — 리뷰 큐가 네트워크에 열립니다. 앞에 인증을 두십시오.\n", *addr)
	}
	fmt.Fprintf(os.Stderr, "리뷰 화면: http://%s  (세션 %s)\n", *addr, path)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
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
	judgments string
	org       string
	planOut   string
}

func (s *server) routes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/save", s.save)
	mux.HandleFunc("/finalize", s.finalize)
}

// view — 화면이 보는 것. 세션을 **정책으로 묶어** 보여 준다 — 그것이 판정의 기본 단위다.
type view struct {
	Path      string
	Scope     string
	Reviewer  string
	Signature string
	Policies  []policyView
	Autopass  int
	// Message · Problem — 방금 무엇이 됐고 무엇이 막았나. 막힌 이유를 안 보여 주면
	// 사람은 화면에서도 고칠 수 없다.
	Message string
	Problem string
	Plan    string
}

type policyView struct {
	Name       string
	Conclusion string
	Items      []review.Item
	Mandatory  int
}

func toView(sf review.Session, path string) view {
	byPolicy := map[string]*policyView{}
	var order []string
	for _, it := range sf.Items {
		p, ok := byPolicy[it.Policy]
		if !ok {
			p = &policyView{Name: it.Policy, Conclusion: sf.PolicyDecisions[it.Policy]}
			byPolicy[it.Policy] = p
			order = append(order, it.Policy)
		}
		p.Items = append(p.Items, it)
		if it.Mandatory {
			p.Mandatory++
		}
	}
	sort.Strings(order)
	v := view{Path: path, Scope: sf.Scope, Reviewer: sf.Reviewer,
		Signature: sf.Signature, Autopass: len(sf.Autopass)}
	for _, name := range order {
		v.Policies = append(v.Policies, *byPolicy[name])
	}
	return v
}

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
	v := toView(sf, s.path)
	v.Message = r.URL.Query().Get("msg")
	v.Problem = r.URL.Query().Get("problem")
	render(w, v)
}

// apply — 폼에서 온 값을 세션에 얹는다. **읽고 얹고 쓴다** — 화면이 자기 사본을 들고 있으면
// 그 사이 파일을 고친 사람의 편집이 사라진다.
func (s *server) apply(r *http.Request) (review.Session, error) {
	sf, err := review.Load(s.path)
	if err != nil {
		return sf, err
	}
	if err := r.ParseForm(); err != nil {
		return sf, err
	}
	sf.Reviewer = strings.TrimSpace(r.PostFormValue("reviewer"))
	sf.Signature = strings.TrimSpace(r.PostFormValue("signature"))
	if sf.PolicyDecisions == nil {
		sf.PolicyDecisions = map[string]string{}
	}
	for pol := range sf.PolicyDecisions {
		sf.PolicyDecisions[pol] = strings.TrimSpace(r.PostFormValue("policy:" + pol))
	}
	for i, it := range sf.Items {
		sf.Items[i].Conclusion = strings.TrimSpace(r.PostFormValue("item:" + it.ID))
		sf.Items[i].Plan = r.PostFormValue("plan:"+it.ID) != ""
	}
	return sf, review.Save(s.path, sf)
}

func (s *server) save(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST 만 받는다", http.StatusMethodNotAllowed)
		return
	}
	if _, err := s.apply(r); err != nil {
		redirect(w, r, "", err.Error())
		return
	}
	redirect(w, r, "세션 파일에 저장했습니다 — 아직 확정하지 않았습니다", "")
}

// finalize — **명령과 같은 게이트를 탄다.** 여기서 따로 판정하지 않는다.
func (s *server) finalize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST 만 받는다", http.StatusMethodNotAllowed)
		return
	}
	sf, err := s.apply(r)
	if err != nil {
		redirect(w, r, "", err.Error())
		return
	}
	res, err := review.Finalize(sf)
	if err != nil {
		// 확정되지 않은 이유를 그대로 보여 준다 — 무엇이 남았는지가 거기 적혀 있다.
		redirect(w, r, "", err.Error())
		return
	}
	raw, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(res.Plan)
	if err != nil {
		redirect(w, r, "", err.Error())
		return
	}
	if err := os.WriteFile(s.planOut, append(raw, '\n'), 0o644); err != nil {
		redirect(w, r, "", err.Error())
		return
	}
	msg := fmt.Sprintf("확정했습니다 — 조치 %d건을 %s 에 썼습니다", len(res.Plan.GetActions()), s.planOut)
	// **게이트를 지난 뒤에만 남긴다.**
	if s.judgments != "" {
		n, err := review.SaveJudgments(s.judgments, s.org, sf, res.Decided)
		if err != nil {
			redirect(w, r, msg, "판정 기록: "+err.Error())
			return
		}
		msg += fmt.Sprintf(" · 판정 %d건을 %s 에 남겼습니다", n, s.judgments)
	}
	redirect(w, r, msg, "")
}

// redirect — POST 뒤에는 GET 으로 보낸다. 새로고침이 같은 확정을 다시 태우지 않게 한다.
func redirect(w http.ResponseWriter, r *http.Request, msg, problem string) {
	u := "/?msg=" + template.URLQueryEscaper(msg)
	if problem != "" {
		u += "&problem=" + template.URLQueryEscaper(problem)
	}
	http.Redirect(w, r, u, http.StatusSeeOther)
}

func render(w http.ResponseWriter, v view) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(w, v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// page — 화면 한 장. **자바스크립트가 없다** — 폼과 링크만으로 되는 일이라 넣지 않았다.
var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="ko"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>리뷰 큐 — pqcaton</title>
<style>
 :root { color-scheme: light dark; --line:#8883; --warn:#b3261e; --ok:#146c2e; }
 body { font: 15px/1.6 system-ui, -apple-system, "Apple SD Gothic Neo", sans-serif;
        max-width: 60rem; margin: 2rem auto; padding: 0 1rem; }
 h1 { font-size: 1.3rem; margin-bottom: .2rem; }
 .sub { color: #7a7a7a; margin-top: 0; }
 fieldset { border: 1px solid var(--line); border-radius: 8px; margin: 1rem 0; padding: 1rem; }
 legend { font-weight: 600; padding: 0 .4rem; }
 legend .n { font-weight: 400; color: #7a7a7a; }
 input[type=text] { width: 100%; padding: .45rem .6rem; border: 1px solid var(--line);
                    border-radius: 6px; background: transparent; color: inherit; font: inherit; }
 table { border-collapse: collapse; width: 100%; margin-top: .7rem; }
 th, td { text-align: left; padding: .35rem .5rem; border-top: 1px solid var(--line); font-size: .9rem; }
 th { color: #7a7a7a; font-weight: 500; }
 .must { color: var(--warn); font-weight: 600; }
 .msg { padding: .7rem 1rem; border-radius: 8px; border: 1px solid var(--line); }
 .msg.ok { border-color: var(--ok); }
 .msg.bad { border-color: var(--warn); }
 pre { white-space: pre-wrap; margin: .4rem 0 0; font-size: .85rem; }
 .actions { display: flex; gap: .6rem; margin-top: 1.2rem; }
 button { font: inherit; padding: .5rem 1rem; border-radius: 6px; border: 1px solid var(--line);
          background: transparent; color: inherit; cursor: pointer; }
 button.primary { border-color: var(--ok); font-weight: 600; }
 .hint { color: #7a7a7a; font-size: .85rem; }
</style></head><body>

<h1>리뷰 큐</h1>
<p class="sub">{{.Scope}} · 세션 <code>{{.Path}}</code>{{if .Autopass}} · 자동통과 후보 {{.Autopass}}개{{end}}</p>

{{if .Message}}<p class="msg ok">{{.Message}}</p>{{end}}
{{if .Problem}}<div class="msg bad"><strong>확정하지 않았습니다.</strong><pre>{{.Problem}}</pre></div>{{end}}

<form method="post">
{{range .Policies}}
 <fieldset>
  <legend>{{.Name}} <span class="n">— 항목 {{len .Items}}개{{if .Mandatory}}, 필수 {{.Mandatory}}{{end}}</span></legend>
  <label>이 정책의 결론 <span class="hint">(적으면 아래 항목이 한 번에 판정됩니다)</span>
   <input type="text" name="policy:{{.Name}}" value="{{.Conclusion}}" placeholder="예: PQC 라이브러리로 교체한다"></label>
  <table>
   <tr><th>대상</th><th>상태</th><th>확신</th><th>계획</th><th>개별 결론(예외)</th></tr>
   {{range .Items}}
   <tr>
    <td><code>{{.ID}}</code>{{if .Rescan}} <span class="hint">재수집 후보</span>{{end}}</td>
    <td{{if .Mandatory}} class="must"{{end}}>{{.State}}</td>
    <td>{{printf "%.2f" .Conf}}</td>
    <td><input type="checkbox" name="plan:{{.ID}}" {{if .Plan}}checked{{end}}></td>
    <td><input type="text" name="item:{{.ID}}" value="{{.Conclusion}}"></td>
   </tr>
   {{end}}
  </table>
 </fieldset>
{{else}}
 <p>판정할 것이 없습니다.</p>
{{end}}

<fieldset>
 <legend>승인</legend>
 <p class="hint">서명 없이는 확정되지 않습니다.</p>
 <label>승인자 <input type="text" name="reviewer" value="{{.Reviewer}}"></label>
 <label style="display:block;margin-top:.6rem">서명 <input type="text" name="signature" value="{{.Signature}}"></label>
</fieldset>

<div class="actions">
 <button formaction="/save">저장만</button>
 <button formaction="/finalize" class="primary">확정하고 계획 내기</button>
</div>
<p class="hint">확정은 <code>pqcaton-decide close</code> 와 같은 게이트를 탑니다 — 필수 항목의 결론과 서명이 모두 있어야 통과합니다.</p>
</form>
</body></html>`))
