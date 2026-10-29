// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// browser: 화면을 쓰는 사람의 브라우저. 쿠키를 들고 다니고, 양식은 같은 출처(Origin)로 보낸다. 리다이렉트는
// 따라가지 않고 돌려준다(어디로 보냈는지 재기 위해).
type browser struct {
	t    *testing.T
	base string
	c    *http.Client
	lang string
}

func newBrowser(t *testing.T, h http.Handler, lang string) *browser {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return browserAt(t, srv.URL, lang)
}

// browserAt: 이미 떠 있는 화면(base)을 쓰는 브라우저.
func browserAt(t *testing.T, base, lang string) *browser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &browser{t: t, base: base, lang: lang, c: &http.Client{Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// serving: 실제 서버를 띄운다(run). 끝내는 함수는 Ctrl+C와 같은 신호를 보내고 끝날 때까지 기다린다.
func serving(t *testing.T, args ...string) (base string, stop func()) {
	t.Helper()
	sig := make(chan os.Signal, 1)
	oldStop, oldOpen := stopSignal, openBrowser
	t.Cleanup(func() { stopSignal, openBrowser = oldStop, oldOpen })
	stopSignal = func() <-chan os.Signal { return sig }
	opened := make(chan string, 1)
	openBrowser = func(u string, _ io.Writer) { opened <- u }
	done := make(chan int, 1)
	var errb bytes.Buffer
	go func() { done <- run(append(append([]string(nil), args...), "-addr", "127.0.0.1:0"), io.Discard, &errb) }()
	select {
	case u := <-opened:
		base = strings.TrimSuffix(u, "/")
	case code := <-done:
		t.Fatalf("the screen did not start: %d\n%s", code, errb.String())
	case <-time.After(5 * time.Second):
		t.Fatal("the screen did not start in time")
	}
	stopped := false
	stop = func() {
		if stopped {
			return
		}
		stopped = true
		sig <- os.Interrupt
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the screen did not stop")
		}
	}
	t.Cleanup(stop)
	return base, stop
}

func (b *browser) url(path string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return b.base + path + sep + "lang=" + b.lang
}

// open: 화면을 연다. 200이 아니면 멈춘다. 영어 화면에 한글이 있으면 알린다.
func (b *browser) open(path string) string {
	b.t.Helper()
	res, err := b.c.Get(b.url(path))
	if err != nil {
		b.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		b.t.Fatalf("GET %s = %d\n%s", path, res.StatusCode, body)
	}
	if b.lang == "en" && hasHangul(visible(string(body))) {
		b.t.Errorf("GET %s (en): Korean on the English screen", path)
	}
	return string(body)
}

// send: 양식을 보낸다. 303이어야 하고, 알림에 막힌 까닭(problem)이 없어야 한다. 돌아간 화면의 경로를 돌려준다.
func (b *browser) send(path string, form url.Values) string {
	b.t.Helper()
	req, err := http.NewRequest(http.MethodPost, b.url(path), strings.NewReader(form.Encode()))
	if err != nil {
		b.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", b.base)
	res, err := b.c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusSeeOther {
		b.t.Fatalf("POST %s = %d\n%s", path, res.StatusCode, body)
	}
	loc, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		b.t.Fatal(err)
	}
	if p := loc.Query().Get(ui.ProblemParam); p != "" {
		b.t.Fatalf("POST %s was refused: %s", path, p)
	}
	return loc.Path
}

// post: 양식을 보내고 응답 본문을 돌려준다(확인 페이지처럼 200으로 그리는 것).
func (b *browser) post(path string, form url.Values) string {
	b.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, b.url(path), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", b.base)
	res, err := b.c.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		b.t.Fatalf("POST %s = %d\n%s", path, res.StatusCode, body)
	}
	return string(body)
}

var fieldName = regexp.MustCompile(`name="((?:policy|plan|kind|target|level):[^"]+)"`)

// IC-U104 — **빈 작업 폴더에서 확정까지 터미널 없이 간다(두 말).** 화면을 실제 HTTP로만 쓴다(쿠키 · 요청
// 보호 토큰 · 같은 출처). 선언을 만들고 노드를 적어 저장하고, 관측 결과가 결과 폴더에 들어오면(pqcota 수집의
// 자리) 판정 화면에 그 자산이 오른다. 결론 · 계획 칸 · 서명을 채워 확인을 거쳐 확정하면 04 카드가
// 「확정됨」이 되고, 계획 파일은 pqcota가 읽는 계약 형식(FinalizedPlan, IN_REVIEW)이다. 판정 원장도 남는다.
func TestEmptyWorkspaceToFinalizedPlan(t *testing.T) {
	for _, c := range []struct{ lang, empty, finalized string }{
		{"ko", "아직 관측 결과가 없습니다", "확정됨"}, {"en", "No observed results yet", "Finalized"},
	} {
		t.Run(c.lang, func(t *testing.T) {
			ws := t.TempDir()
			h, code, _, stderr := started(t, "-workspace", ws, "-no-browser")
			if code != 0 {
				t.Fatalf("code %d\n%s", code, stderr)
			}
			b := newBrowser(t, h, c.lang)

			// 1. 선언 만들기.
			page := b.open(ui.ScreenDecl)
			form := hiddenFields(page)
			form.Set("org", "acme")
			if got := b.send(ui.DeclCreatePath, form); got != ui.ScreenDecl {
				t.Fatalf("create landed on %s", got)
			}

			// 2. 노드 한 대를 적어 저장한다.
			form = hiddenFields(b.open(ui.ScreenDecl))
			form.Set("org", "acme")
			form.Set("node.name.0", "web")
			form.Set("node.ips.0", "10.0.0.1")
			if got := b.send("/decl/save", form); got != ui.ScreenDecl {
				t.Fatalf("save landed on %s", got)
			}

			// 3. 관측 결과가 들어온다. 선언하지 않은 openssl이 web에서 관측됐다.
			if !strings.Contains(b.open(ui.ScreenReview), c.empty) {
				t.Error("the review screen does not point at the empty results folder")
			}
			writeResult(t, filepath.Join(ws, "results", "web.json"), "web")

			// 4 · 5. 판정하고 확인을 거쳐 확정한다.
			b.judgeAndFinalize()
			if got := cards(t, b.open(ui.ScreenReview))["05"]; !strings.HasSuffix(got, "| "+c.finalized) {
				t.Errorf("the 04 card is %q, want it %s", got, c.finalized)
			}

			raw, err := os.ReadFile(filepath.Join(ws, "plan.json"))
			if err != nil {
				t.Fatal(err)
			}
			var plan provisioningv1.FinalizedPlan
			if err := protojson.Unmarshal(raw, &plan); err != nil {
				t.Fatalf("the plan is not in the contract format: %v", err)
			}
			if plan.GetStatus() != provisioningv1.PlanStatus_PLAN_STATUS_IN_REVIEW || len(plan.GetActions()) != 1 {
				t.Errorf("plan: status %v, %d actions", plan.GetStatus(), len(plan.GetActions()))
			}
			ledgerOrg(t, ws, "acme")
		})
	}
}

// writeResult: 상류 수집기가 내는 모양 그대로의 결과 하나(openssl libcrypto).
func writeResult(t *testing.T, path, node string) {
	t.Helper()
	cbom := `{"bomFormat":"CycloneDX","specVersion":"1.6","components":[
      {"type":"cryptographic-asset","name":"libcrypto","properties":[
        {"name":"pqcota:crypto_runtime","value":"openssl"},
        {"name":"pqcota:openssl.version","value":"3.0.2"},
        {"name":"pqcota:openssl.fork","value":"OpenSSL"}]}]}`
	r := &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{TargetNodeId: node, CollectorId: "openssl-collector",
			CollectedAt:     timestamppb.Now(),
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(cbom), CyclonedxSpecVersion: "1.6",
		Completeness: &commonv1.Completeness{LayersCovered: []commonv1.CollectionLayer{commonv1.CollectionLayer_COLLECTION_LAYER_PROCESS}},
	}
	b, err := protojson.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mapKeys(v url.Values) []string {
	out := make([]string, 0, len(v))
	for k := range v {
		out = append(out, k)
	}
	return out
}

// judgeAndFinalize: 판정 화면에 오른 칸을 채우고 서명한 뒤, 확인 페이지를 거쳐 확정한다. 확인 페이지를 돌려준다.
func (b *browser) judgeAndFinalize() string {
	t := b.t
	t.Helper()
	page := b.open(ui.ScreenReview)
	form := hiddenFields(page)
	for _, m := range fieldName.FindAllStringSubmatch(page, -1) {
		switch name := m[1]; strings.SplitN(name, ":", 2)[0] {
		case "policy":
			form.Set(name, "replace with a PQC-ready library")
		case "plan":
			form.Set(name, "on")
		case "kind":
			form.Set(name, "REMEDIATION_KIND_CONFIG_ONLY")
		case "target":
			form.Set(name, "ML-KEM (FIPS 203)")
		case "level":
			form.Set(name, "L2")
		}
	}
	form.Set("reviewer", "security")
	form.Set("signature", "sig")
	if !strings.Contains(strings.Join(mapKeys(form), " "), "plan:") {
		t.Fatalf("the observed asset did not come up for review:\n%s", page)
	}
	confirm := b.post("/finalize/confirm", form)
	if got := b.send("/finalize", hiddenFields(confirm)); got != ui.ScreenReview {
		t.Fatalf("finalize landed on %s", got)
	}
	return confirm
}

// ledgerOrg: 판정 원장의 기록이 모두 그 조직에 묶였나.
func ledgerOrg(t *testing.T, ws, org string) {
	t.Helper()
	led, err := os.ReadFile(filepath.Join(ws, "judgments.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(led)), "\n")
	for _, l := range lines {
		if !strings.Contains(l, `"org":"`+org+`"`) {
			t.Errorf("a ledger entry is not bound to %s: %s", org, l)
		}
	}
}

// IC-U105 — **작업 폴더 모드에서 -org를 주지 않으면 판정은 선언의 조직에 묶인다.** 선언이 이미 있는 작업
// 폴더로 시작해 확정해도 원장 · 계획이 세션과 같은 조직(선언의 조직)을 말한다. -org를 준 서버는 그 조직의
// 선언만 만든다(다르면 400). 명시 모드는 -org 그대로다(v0.22와 같다).
func TestWorkspaceJudgmentsFollowTheDeclarationOrg(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "declaration.json"), []byte(`{"org":"acme","scope":["web"],
 "nodes":[{"name":"web","ips":["10.0.0.1"]}],"assets":[],"edges":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "results"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeResult(t, filepath.Join(ws, "results", "web.json"), "web")
	h, code, _, stderr := started(t, "-workspace", ws, "-no-browser")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, stderr)
	}
	b := newBrowser(t, h, "en")
	b.judgeAndFinalize()
	ledgerOrg(t, ws, "acme")

	other := t.TempDir()
	h, code, _, stderr = started(t, "-workspace", other, "-no-browser", "-org", "acme")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, stderr)
	}
	if w := do(h, posted(ui.DeclCreatePath, url.Values{"org": {"globex"}})); w.Code != http.StatusBadRequest {
		t.Errorf("a declaration for another org on a -org acme server: %d, want 400", w.Code)
	}
	if w := do(h, posted(ui.DeclCreatePath, url.Values{"org": {"acme"}})); w.Code != http.StatusSeeOther {
		t.Errorf("a declaration for the -org org: %d, want 303", w.Code)
	}

	// 명시 모드는 v0.22와 같다: -org를 주지 않으면 기본 조직(local)이다.
	ex := t.TempDir()
	for _, f := range []string{"declaration.json", "results/web.json"} {
		b, err := os.ReadFile(filepath.Join(ws, f))
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(filepath.Dir(filepath.Join(ex, f)), 0o700)
		if err := os.WriteFile(filepath.Join(ex, f), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h, code, _, stderr = started(t, filepath.Join(ex, "session.json"), "-decl", filepath.Join(ex, "declaration.json"),
		"-results", filepath.Join(ex, "results"), "-judgments", filepath.Join(ex, "judgments.jsonl"),
		"-plan", filepath.Join(ex, "plan.json"), "-no-browser")
	if code != 0 {
		t.Fatalf("explicit mode: code %d\n%s", code, stderr)
	}
	eb := newBrowser(t, h, "en")
	form := hiddenFields(eb.open(ui.ScreenDecl)) // 선언을 저장해도 명시 모드의 조직은 바뀌지 않는다
	form.Set("org", "acme")
	form.Set("node.name.0", "web")
	form.Set("node.ips.0", "10.0.0.1")
	eb.send("/decl/save", form)
	eb.judgeAndFinalize()
	ledgerOrg(t, ex, "local")
}

// openNewRound: 확정된 화면 위쪽의 새 회차 단추를 누른다.
func (b *browser) openNewRound(screen, action string) {
	b.t.Helper()
	page := b.open(screen)
	i := strings.Index(page, `action="`+action+`"`)
	if i < 0 {
		b.t.Fatalf("%s: no new-round button:\n%s", screen, page)
	}
	form := hiddenFields(page[i : i+strings.Index(page[i:], "</form>")])
	if got := b.send(action, form); got != screen {
		b.t.Fatalf("the new round landed on %s", got)
	}
}

var sessionIDField = regexp.MustCompile(`"session_id":\s*"([^"]+)"`)

func sessionIDOf(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	m := sessionIDField.FindSubmatch(b)
	if m == nil {
		t.Fatalf("%s has no session id", p)
	}
	return string(m[1])
}

// IC-U109 — **확정한 세션은 읽기 전용이고, 화면에서 새 회차를 연다.** 확정한 판정 화면은 칸이 모두 잠기고
// (`fieldset disabled`) 저장 · 확정 단추가 없으며 「새 검토 시작」 단추(요청 보호 · rev)가 있다. 누르면 새 ID의
// 세션이 서고, 결론은 옮겨 오고, 승인 서명은 비고, 04 카드는 「미확정」으로 돌아가 다시 고칠 수 있다. 옛 세션의
// 확정 요청을 다시 보내도 「복구 필요」(503)가 아니다.
func TestFinalizedSessionOpensANewRound(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "declaration.json"), []byte(`{"org":"acme","scope":["web"],
 "nodes":[{"name":"web","ips":["10.0.0.1"]}],"assets":[],"edges":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(ws, "results"), 0o700)
	writeResult(t, filepath.Join(ws, "results", "web.json"), "web")
	h, code, _, stderr := started(t, "-workspace", ws, "-no-browser")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, stderr)
	}
	b := newBrowser(t, h, "ko")
	session := filepath.Join(ws, "session.json")
	first := sessionIDOf(t, session)
	confirm := b.judgeAndFinalize()

	page := b.open(ui.ScreenReview)
	for _, want := range []string{"이 판정 세션은 확정되었습니다", `<fieldset class="locked" disabled>`, `action="` + ui.ReviewNewPath + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the finalized review screen has no %q", want)
		}
	}
	if strings.Contains(page, `formaction="/finalize/confirm"`) || strings.Contains(page, `formaction="/save"`) {
		t.Error("the finalized review screen still offers save or finalize")
	}

	b.openNewRound(ui.ScreenReview, ui.ReviewNewPath)
	second := sessionIDOf(t, session)
	if second == first {
		t.Fatal("the new round kept the session id")
	}
	page = b.open(ui.ScreenReview)
	if strings.Contains(page, `fieldset class="locked" disabled`) || !strings.Contains(page, `formaction="/finalize/confirm"`) {
		t.Error("the new round is not editable")
	}
	if !strings.Contains(page, `value="replace with a PQC-ready library"`) {
		t.Error("the conclusion was not carried over")
	}
	if !strings.Contains(page, `name="signature" value=""`) {
		t.Error("the signature was carried over")
	}
	if got := cards(t, page)["05"]; !strings.HasSuffix(got, "| 미확정") {
		t.Errorf("the 04 card is %q after the new round, want 미확정", got)
	}

	// 옛 세션의 확인 페이지를 다시 보낸다.
	req, _ := http.NewRequest(http.MethodPost, b.url("/finalize"), strings.NewReader(hiddenFields(confirm).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", b.base)
	res, err := b.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode == http.StatusServiceUnavailable {
		t.Error("re-sending the old finalize reads as recovery needed")
	}
	if got := sessionIDOf(t, session); got != second {
		t.Errorf("re-sending the old finalize rewrote the session (%s)", got)
	}
}

// IC-U110 — **같은 작업 폴더에서 확정 → 새 회차 → 다시 확정을 두 번, 사이에 재시작을 끼워서 한다.** 판정과
// 자산 스코프 둘 다. 회차마다 새 세션 ID이고, 확정할 때마다 카드가 「확정됨」, 새 회차 뒤에는 「미확정」이다.
// 원장에는 세 판정 세션이 모두 남고, 계획은 마지막 세션의 것이다. 재시작한 화면은 「저장 가능」이며
// 남은 기록이 없다(옛 영수증이 복구 필요로 읽히지 않는다).
func TestRoundsAcrossARestart(t *testing.T) {
	ws := t.TempDir()
	base, stop := serving(t, "-workspace", ws)
	b := browserAt(t, base, "en")
	form := hiddenFields(b.open(ui.ScreenDecl))
	form.Set("org", "acme")
	b.send(ui.DeclCreatePath, form)
	form = hiddenFields(b.open(ui.ScreenDecl))
	form.Set("org", "acme")
	form.Set("node.name.0", "web")
	form.Set("node.ips.0", "10.0.0.1")
	b.send("/decl/save", form)
	writeResult(t, filepath.Join(ws, "results", "web.json"), "web")
	form = hiddenFields(b.open(ui.ScreenScope))
	form.Set("name", "corp")
	b.send(ui.LayerCreatePath, form)

	session := filepath.Join(ws, "session.json")
	scopeSession := filepath.Join(ws, "scope-session.json")
	var reviews, scopes []string
	finalizeBoth := func() {
		b.judgeAndFinalize()
		b.scopeFinalize()
		reviews = append(reviews, sessionIDOf(t, session))
		scopes = append(scopes, sessionIDOf(t, scopeSession))
		page := b.open(ui.ScreenReview)
		if c := cards(t, page); !strings.HasSuffix(c["05"], "| Finalized") || !strings.HasSuffix(c["02"], "| Finalized") {
			t.Fatalf("after finalizing: %v", c)
		}
	}
	newRounds := func() {
		b.openNewRound(ui.ScreenReview, ui.ReviewNewPath)
		b.openNewRound(ui.ScreenScope, ui.ScopeNewPath)
		if c := cards(t, b.open(ui.ScreenReview)); !strings.HasSuffix(c["05"], "| Not finalized") || !strings.HasSuffix(c["02"], "| Not finalized") {
			t.Fatalf("after the new rounds: %v", c)
		}
	}

	finalizeBoth()
	newRounds()
	stop()
	base, _ = serving(t, "-workspace", ws)
	b = browserAt(t, base, "en")
	if page := b.open(ui.ScreenReview); !strings.Contains(page, "Saving enabled") {
		t.Fatal("the restarted screen is not in the saving mode")
	}
	finalizeBoth()
	newRounds()
	finalizeBoth()

	for _, ids := range [][]string{reviews, scopes} {
		if len(ids) != 3 || ids[0] == ids[1] || ids[1] == ids[2] || ids[0] == ids[2] {
			t.Errorf("each round should have its own session id: %v", ids)
		}
	}
	led, err := os.ReadFile(filepath.Join(ws, "judgments.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range reviews {
		if !strings.Contains(string(led), `"SessionID":"`+id+`"`) {
			t.Errorf("the ledger has nothing from session %s", id)
		}
	}
	if plan, _ := os.ReadFile(filepath.Join(ws, "plan.json")); !strings.Contains(string(plan), reviews[2]) {
		t.Error("the plan is not the last session's")
	}
	var out bytes.Buffer
	if code := run([]string{"-workspace", ws, "-recover-status"}, &out, io.Discard); code != 0 || !strings.Contains(out.String(), "no unfinished transactions") {
		t.Errorf("recover-status: %d\n%s", code, out.String())
	}
}

// scopeFinalize: 스코프 화면에서 바뀐 규칙마다 결론을 적고 서명해 확인을 거쳐 확정한다.
func (b *browser) scopeFinalize() {
	b.t.Helper()
	page := b.open(ui.ScreenScope)
	form := hiddenFields(page)
	for _, m := range regexp.MustCompile(`name="(layer:[^"]+|change:[^"]+)"`).FindAllStringSubmatch(page, -1) {
		form.Set(m[1], "keep it")
	}
	form.Set("reviewer", "security")
	form.Set("signature", "sig")
	confirm := b.post("/scope/finalize/confirm", form)
	if got := b.send("/scope/finalize", hiddenFields(confirm)); got != ui.ScreenScope {
		b.t.Fatalf("scope finalize landed on %s", got)
	}
}
