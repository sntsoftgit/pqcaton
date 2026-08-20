package ui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func sample() decl.Declaration {
	return decl.Declaration{
		Org: "acme", Scope: []string{"web"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1", "10.0.1.1"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "openssl", Component: "libssl"}},
		Edges:  []decl.Edge{{Src: "web", Dst: "web", Port: 443, Proto: "TLS"}},
	}
}

// IC-UI1 — **화면이 있는 것을 다 보여 준다.** 표에 안 나온 줄은 폼으로 돌아오지 않으므로
// 저장하는 순간 사라진다 — 화면이 곧 데이터 손실의 자리가 된다.
func TestDeclRenderShowsEverything(t *testing.T) {
	var b strings.Builder
	if err := ui.RenderDecl(&b, ui.NewDeclView(sample(), ui.Page{Title: "선언"})); err != nil {
		t.Fatal(err)
	}
	body := b.String()
	for _, want := range []string{
		`name="org"`,
		`name="node.name.0"`, "10.0.0.1, 10.0.1.1", // 여러 IP를 한 칸에
		`name="asset.component.0"`, "libssl",
		`name="edge.port.0"`, "443",
		`name="node.name.1"`, // 빈 줄이 열려 있어야 새로 넣을 수 있다
	} {
		if !strings.Contains(body, want) {
			t.Errorf("화면에 %q 가 없다", want)
		}
	}
}

// IC-UI2 — **폼을 다시 읽으면 같은 선언이 나온다.** 그리기와 읽기가 어긋나면 저장할 때마다
// 조용히 달라진다.
func TestApplyDeclRoundTrip(t *testing.T) {
	in := sample()
	f := url.Values{
		"org":         {in.Org},
		"node.name.0": {"web"}, "node.ips.0": {"10.0.0.1, 10.0.1.1"},
		"asset.node.0": {"web"}, "asset.runtime.0": {"openssl"}, "asset.component.0": {"libssl"},
		"edge.src.0": {"web"}, "edge.dst.0": {"web"}, "edge.port.0": {"443"}, "edge.proto.0": {"TLS"},
	}
	got := ui.ApplyDecl(in, f)
	if got.Org != "acme" || len(got.Scope) != 1 {
		t.Fatalf("머리 부분이 다르다: %+v", got)
	}
	if len(got.Nodes) != 1 || len(got.Nodes[0].IPs) != 2 || got.Nodes[0].IPs[1] != "10.0.1.1" {
		t.Fatalf("노드가 다르다: %+v", got.Nodes)
	}
	if len(got.Assets) != 1 || len(got.Edges) != 1 || got.Edges[0].Port != 443 {
		t.Fatalf("자산·엣지가 다르다: %+v %+v", got.Assets, got.Edges)
	}
	if len(decl.Check(got)) != 0 {
		t.Errorf("맞는 선언인데 짚는다: %+v", decl.Check(got))
	}
}

// IC-UI3 — **IP는 쉼표로도 공백으로도 받는다.** 사람이 어느 쪽으로 적는지 외우게 하지 않는다.
func TestApplyDeclSplitsIPsEitherWay(t *testing.T) {
	for _, in := range []string{"10.0.0.1, 10.0.1.1", "10.0.0.1 10.0.1.1", "10.0.0.1,10.0.1.1"} {
		got := ui.ApplyDecl(decl.Declaration{}, url.Values{
			"node.name.0": {"web"}, "node.ips.0": {in},
		})
		if len(got.Nodes) != 1 || len(got.Nodes[0].IPs) != 2 {
			t.Errorf("%q → %+v", in, got.Nodes)
		}
	}
}

// IC-UI4 — **머리말은 사람이 편집할 자리가 아니다.** 생성 도구(declare.py)가 남긴 것이라
// 폼에 없다 — 그래도 저장에서 사라지면 안 된다.
func TestApplyDeclKeepsComment(t *testing.T) {
	prev := decl.Declaration{Comment: "declare.py 가 생성했습니다"}
	got := ui.ApplyDecl(prev, url.Values{"org": {"acme"}})
	if got.Comment != prev.Comment {
		t.Errorf("머리말이 사라졌다: %q", got.Comment)
	}
}

// IC-UI5 — 리뷰 큐는 **정책으로 묶여** 그려진다(§3.4). 한 줄씩 늘어놓으면 화면이 있어도
// 수천 대에서 리뷰가 끝나지 않는다.
func TestReviewViewGroupsByPolicy(t *testing.T) {
	sf := review.Session{Scope: "host://local",
		PolicyDecisions: map[string]string{"openssl/libssl": "교체한다"},
		Items: []review.Item{
			{ID: "a", Policy: "openssl/libssl", Mandatory: true},
			{ID: "b", Policy: "openssl/libssl"},
			{ID: "c", Policy: "jca/provider"},
		},
	}
	v := ui.NewReviewView(sf, ui.Page{Title: "리뷰 큐"})
	if len(v.Policies) != 2 {
		t.Fatalf("정책 묶음 %d개: %+v", len(v.Policies), v.Policies)
	}
	// 정렬해야 실행마다 순서가 흔들리지 않는다.
	if v.Policies[0].Name != "jca/provider" {
		t.Errorf("정렬되지 않았다: %s", v.Policies[0].Name)
	}
	for _, g := range v.Policies {
		if g.Name == "openssl/libssl" {
			if len(g.Items) != 2 || g.Mandatory != 1 || g.Conclusion != "교체한다" {
				t.Errorf("묶음이 다르다: %+v", g)
			}
		}
	}
}

// IC-UI6 — 리뷰 폼을 읽으면 세션에 그대로 얹힌다.
func TestApplyReview(t *testing.T) {
	sf := review.Session{
		PolicyDecisions: map[string]string{"p": ""},
		Items:           []review.Item{{ID: "a", Policy: "p"}},
	}
	got := ui.ApplyReview(sf, url.Values{
		"reviewer": {"보안팀"}, "signature": {"sig"},
		"policy:p": {"교체한다"}, "item:a": {"예외"}, "plan:a": {"on"},
	})
	if got.Reviewer != "보안팀" || got.Signature != "sig" {
		t.Errorf("승인 정보가 안 얹혔다: %+v", got)
	}
	if got.PolicyDecisions["p"] != "교체한다" || got.Items[0].Conclusion != "예외" || !got.Items[0].Plan {
		t.Errorf("판정이 안 얹혔다: %+v", got)
	}
}

// IC-UI7 — 스코프 변경은 **계층으로 묶여** 그려진다(§3.4). 규칙 한 줄씩 승인하는 리뷰는
// 수천 대에서 끝나지 않는다.
func TestScopeViewGroupsByLayer(t *testing.T) {
	sf := scope.Session{
		Org: "acme", LayerDecisions: map[string]string{"corp": "뺀다", "pay": ""},
		Changes: []scope.ChangeItem{
			{ID: "a", Layer: "corp", Kind: scope.KindAdded, Audited: true},
			{ID: "b", Layer: "corp", Kind: scope.KindRemoved},
			{ID: "c", Layer: "pay", Kind: scope.KindAdded, Audited: true},
		},
	}
	v := ui.NewScopeView(sf, ui.Page{Title: "자산 스코프"})
	if len(v.Layers) != 2 {
		t.Fatalf("계층 %d개", len(v.Layers))
	}
	if v.Layers[0].Name != "corp" {
		t.Errorf("정렬되지 않았다: %s", v.Layers[0].Name)
	}
	if v.Layers[0].Audited != 1 || len(v.Layers[0].Changes) != 2 {
		t.Errorf("묶음이 다르다: %+v", v.Layers[0])
	}
	if v.Audited != 2 {
		t.Errorf("근거 필수 %d건, want 2", v.Audited)
	}
}

// IC-UI8 — 스코프 폼을 읽으면 세션에 그대로 얹힌다.
func TestApplyScope(t *testing.T) {
	sf := scope.Session{
		LayerDecisions: map[string]string{"corp": ""},
		Changes:        []scope.ChangeItem{{ID: "a", Layer: "corp"}},
	}
	got := ui.ApplyScope(sf, url.Values{
		"reviewer": {"보안팀"}, "signature": {"sig"},
		"layer:corp": {"뺀다"}, "change:a": {"예외로 둔다"},
	})
	if got.Reviewer != "보안팀" || got.Signature != "sig" {
		t.Errorf("승인 정보가 안 얹혔다: %+v", got)
	}
	if got.LayerDecisions["corp"] != "뺀다" || got.Changes[0].Conclusion != "예외로 둔다" {
		t.Errorf("판정이 안 얹혔다: %+v", got)
	}
}

// IC-UI9 — **「행 추가」가 내는 줄과 화면이 그리는 줄이 같은 폼 이름을 쓴다.**
//
// 폼 이름이 곧 저장 경로입니다. 두 벌이 되어 어긋나면 화면은 멀쩡히 그려지고 새로 넣은
// 줄만 조용히 저장되지 않습니다 — 오류도 나지 않습니다. 그래서 같은 조각을 쓰는지 잰다.
func TestAddedRowUsesSameFormNames(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want []string
	}{
		{ui.KindNode, []string{`name="node.name.7"`, `name="node.ips.7"`}},
		{ui.KindAsset, []string{`name="asset.node.7"`, `name="asset.runtime.7"`, `name="asset.component.7"`}},
		{ui.KindEdge, []string{`name="edge.src.7"`, `name="edge.dst.7"`, `name="edge.port.7"`, `name="edge.proto.7"`}},
	} {
		var b strings.Builder
		if err := ui.RenderRow(&b, ui.KO, tc.kind, 7); err != nil {
			t.Fatal(err)
		}
		for _, w := range tc.want {
			if !strings.Contains(b.String(), w) {
				t.Errorf("%s: 새 줄에 %q 가 없다", tc.kind, w)
			}
		}
	}
}

// IC-UI10 — **새 줄을 내줄 때마다 다음 번호가 하나 오른다.**
//
// `ApplyDecl` 은 번호가 끊기는 자리에서 읽기를 멈춥니다. 버튼이 같은 번호를 계속 주면
// 새 줄이 앞의 것을 덮고, 번호를 건너뛰면 그 뒤가 통째로 저장되지 않습니다 — 둘 다
// 오류 없이 틀리는 자리입니다.
func TestAddedRowAdvancesTheButton(t *testing.T) {
	var b strings.Builder
	if err := ui.RenderRow(&b, ui.KO, ui.KindNode, 7); err != nil {
		t.Fatal(err)
	}
	body := b.String()
	if !strings.Contains(body, "i=8") {
		t.Errorf("다음 번호가 8이 아니다:\n%s", body)
	}
	if !strings.Contains(body, "hx-swap-oob") {
		t.Error("버튼이 자기 자신을 갈아 끼우지 않는다 — 다음 줄이 같은 번호로 나온다")
	}
}

// IC-UI11 — **모르는 표는 받지 않는다.** 주소는 밖에서 오는 값이다.
func TestValidKindRefusesUnknown(t *testing.T) {
	for _, k := range []string{ui.KindNode, ui.KindAsset, ui.KindEdge} {
		if !ui.ValidKind(k) {
			t.Errorf("%q 를 막았다", k)
		}
	}
	for _, k := range []string{"", "policy", "node ", "NODE", "../etc"} {
		if ui.ValidKind(k) {
			t.Errorf("%q 를 통과시켰다", k)
		}
	}
}

// IC-UI12 — **스타일과 htmx 는 같은 바이너리에서 나온다.**
//
// CDN 을 걸면 망이 끊긴 기계에서 화면이 깨지고, 남의 서버에서 오는 스크립트는 우리
// 라이선스 게이트가 볼 수도 없습니다. 이 리포를 쓰는 곳에서 바깥으로 못 나가는 망은
// 예외가 아니라 흔한 조건입니다.
func TestStaticIsServedFromTheBinary(t *testing.T) {
	srv := httptest.NewServer(ui.Static())
	defer srv.Close()

	for _, name := range []string{"htmx.min.js", "app.css"} {
		res, err := http.Get(srv.URL + ui.StaticPath + name)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: %d", name, res.StatusCode)
		}
		if len(body) == 0 {
			t.Errorf("%s 가 비었다", name)
		}
	}
}

// IC-UI13 — 화면이 그 둘을 실제로 부른다. 박아 두고 부르지 않으면 아무 일도 일어나지 않는다.
func TestPageLoadsStatic(t *testing.T) {
	var b strings.Builder
	if err := ui.RenderDecl(&b, ui.NewDeclView(sample(), ui.Page{Title: "선언"})); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{ui.StaticPath + "app.css", ui.StaticPath + "htmx.min.js"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("화면이 %q 를 부르지 않는다", want)
		}
	}
}

// layerFiles — 계층 하나짜리 편집 재료.
func layerFiles() []scope.LayerFile {
	return []scope.LayerFile{{Path: "/tmp/corp.csv", Layer: scope.Layer{Name: "corp",
		Rules: []kscope.AssetRule{
			{Exclude: true, Runtime: "openssl", Lib: "libcrypto.so.*", AppKey: "/usr/bin/python*", Note: "python runtime"},
		}}}}
}

// IC-UI14 — **규칙 표를 그리고 다시 읽으면 같은 규칙이 나온다.**
//
// 그리기와 읽기가 어긋나면 저장할 때마다 규칙이 조용히 달라집니다. 그 규칙은 pqcota 가
// 집행하는 것이라, 어긋난 만큼 인벤토리에서 무엇이 빠지는지가 달라집니다.
func TestApplyLayersRoundTrip(t *testing.T) {
	files := layerFiles()
	f := url.Values{
		"rule.0.0.action": {"exclude"}, "rule.0.0.runtime": {"openssl"},
		"rule.0.0.lib": {"libcrypto.so.*"}, "rule.0.0.app_key": {"/usr/bin/python*"},
		"rule.0.0.note": {"python runtime"},
	}
	got := ui.ApplyLayers(files, f)
	if len(got) != 1 || len(got[0].Layer.Rules) != 1 {
		t.Fatalf("규칙 수가 다르다: %+v", got)
	}
	if got[0].Layer.Rules[0] != files[0].Layer.Rules[0] {
		t.Errorf("규칙이 달라졌다:\n  got  %+v\n  want %+v", got[0].Layer.Rules[0], files[0].Layer.Rules[0])
	}
	if got[0].Path != files[0].Path || got[0].Layer.Name != "corp" {
		t.Error("어느 파일의 어느 계층인지가 날아갔다 — 저장할 곳을 잃는다")
	}
}

// IC-UI15 — **세 칸이 모두 빈 줄은 규칙이 아니다.**
//
// pqcota 는 빈 칸을 `*`로 읽습니다. 그대로 만들면 `exclude,*,*,*` — **인벤토리가 통째로
// 빕니다.** 미리 열어 둔 빈 줄에서 action 만 잘못 골라도 그렇게 됩니다. 「전부」를
// 뜻하려면 `*`를 적어야 합니다.
func TestApplyLayersDropsEmptyRows(t *testing.T) {
	f := url.Values{
		"rule.0.0.action": {"exclude"}, "rule.0.0.runtime": {"openssl"},
		"rule.0.0.lib": {"libssl.so.3"}, "rule.0.0.app_key": {"*"},
		// 빈 줄인데 action 만 exclude 로 남았다
		"rule.0.1.action": {"exclude"}, "rule.0.1.runtime": {""},
		"rule.0.1.lib": {""}, "rule.0.1.app_key": {""}, "rule.0.1.note": {"적다 만 줄"},
	}
	got := ui.ApplyLayers(layerFiles(), f)
	if n := len(got[0].Layer.Rules); n != 1 {
		t.Fatalf("규칙 %d개 — 빈 줄이 규칙이 됐다: %+v", n, got[0].Layer.Rules)
	}
	// 「전부」를 뜻하려면 * 를 적는다. 그건 그대로 규칙이 된다.
	f.Set("rule.0.1.runtime", "*")
	if n := len(ui.ApplyLayers(layerFiles(), f)[0].Layer.Rules); n != 2 {
		t.Fatalf("`*` 를 적었는데 규칙이 되지 않았다: %d개", n)
	}
}

// IC-UI16 — 줄을 지우는 방법은 **세 칸을 비우는 것** 하나다. 그 방법이 먹지 않으면
// 화면에서 규칙을 뺄 길이 없다.
func TestApplyLayersRemovesRuleWhenCleared(t *testing.T) {
	f := url.Values{
		"rule.0.0.action": {"exclude"}, "rule.0.0.runtime": {""},
		"rule.0.0.lib": {""}, "rule.0.0.app_key": {""}, "rule.0.0.note": {"python runtime"},
	}
	if n := len(ui.ApplyLayers(layerFiles(), f)[0].Layer.Rules); n != 0 {
		t.Fatalf("비운 줄이 남았다: %d개", n)
	}
}

// IC-UI17 — 「행 추가」가 낸 규칙 줄이 화면과 같은 폼 이름을 쓰고, 다음 번호가 오른다.
func TestRuleRowFragment(t *testing.T) {
	var b strings.Builder
	if err := ui.RenderRuleRow(&b, ui.KO, 1, 4); err != nil {
		t.Fatal(err)
	}
	body := b.String()
	for _, want := range []string{
		`name="rule.1.4.action"`, `name="rule.1.4.runtime"`, `name="rule.1.4.lib"`,
		`name="rule.1.4.app_key"`, `name="rule.1.4.note"`, "layer=1&amp;i=5", "hx-swap-oob",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("새 규칙 줄에 %q 가 없다:\n%s", want, body)
		}
	}
	// **빈 줄의 기본은 include 다.** exclude 가 기본이면 실수 한 번이 인벤토리를 지운다.
	if !strings.Contains(body, `<option value="include" selected`) {
		t.Error("빈 줄의 기본이 include 가 아니다")
	}
}

// IC-UI18 — 계층 파일을 주지 않으면 편집 표를 그리지 않는다. **저장할 곳이 없는 칸을
// 사람이 채우게 하지 않는다.**
func TestScopeScreenIsReadOnlyWithoutLayers(t *testing.T) {
	sf := scope.Session{Org: "acme", LayerDecisions: map[string]string{}}
	var b strings.Builder
	if err := ui.RenderScope(&b, ui.NewScopeView(sf, ui.Page{Title: "자산 스코프"})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), `name="rule.0.0.action"`) {
		t.Error("계층 파일 없이 편집 표를 그렸다")
	}
	var c strings.Builder
	if err := ui.RenderScope(&c, ui.NewScopeView(sf, ui.Page{Title: "자산 스코프"}).Editable(layerFiles())); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.String(), `name="rule.0.0.action"`) {
		t.Error("계층 파일을 줬는데 편집 표가 없다")
	}
}

// langToggle — 말 바꾸기 링크. **여기만 다른 말로 적는 것이 맞다** — 지금 말을 못 읽는
// 사람도 자기 말은 알아봐야 하므로, 영어 화면의 토글은 「한국어」라고 적힌다.
var langToggle = regexp.MustCompile(`<a class="lang"[^>]*>[^<]*</a>`)

// hasHangul — 화면에 한글이 남아 있나. 토글은 덜어내고 본다.
func hasHangul(s string) string {
	s = langToggle.ReplaceAllString(s, "")
	for i, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 {
			from := max(0, i-70)
			return s[from:min(len(s), i+70)]
		}
	}
	return ""
}

// IC-UI29 — **화면마다 설명이 접혀 있다.**
//
// 여기는 날마다 여는 자리지 읽는 자리가 아닙니다. 절마다 설명을 펼쳐 두면 그만큼
// 표가 아래로 밀리고, 그 표를 보러 온 사람은 문장부터 지나쳐야 합니다. 설명을 없애는
// 대신 「도움말」로 접고, 필요한 사람만 폅니다.
func TestEveryScreenFoldsItsExplanations(t *testing.T) {
	page := ui.Page{Title: "t", Lang: ui.KO}

	d := decl.Declaration{Org: "acme", Scope: []string{"web"},
		Nodes: []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}}}}
	rv := review.Session{Scope: "org://acme", PolicyDecisions: map[string]string{"p": ""},
		Items: []review.Item{{ID: "a", Policy: "p", State: "UNDECLARED", Mandatory: true}}}
	sc := scope.Session{Org: "acme", LayerDecisions: map[string]string{"corp": ""}}

	for name, render := range map[string]func(w io.Writer) error{
		"decl":   func(w io.Writer) error { return ui.RenderDecl(w, ui.NewDeclView(d, page)) },
		"review": func(w io.Writer) error { return ui.RenderReview(w, ui.NewReviewView(rv, page)) },
		"scope": func(w io.Writer) error {
			return ui.RenderScope(w, ui.NewScopeView(sc, page).Editable(layerFiles()))
		},
	} {
		var b strings.Builder
		if err := render(&b); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(b.String(), `<details class="tip">`) {
			t.Errorf("%s 화면에 접어 둔 설명이 없다", name)
		}
	}
}

// IC-UI19 — **영어 화면에 한글이 남아 있지 않다.**
//
// 문구를 하나 옮기지 않으면 그 자리만 한국어로 뜹니다. 눈으로는 못 찾습니다 — 화면
// 넷에 문구가 200개 가까이 있고, 새 문구는 계속 늘어납니다. 그래서 **화면을 통째로
// 영어로 그려 놓고 한글이 한 글자라도 있으면 막습니다.**
//
// 재료는 전부 아스키로 둡니다 — 노드 이름 같은 값까지 잡으면 이 케이스가 거짓으로 웁니다.
func TestEnglishScreensHaveNoKorean(t *testing.T) {
	page := ui.Page{Title: "t", Subtitle: "s", Lang: ui.EN, LangHref: "/?lang=ko",
		Nav: ui.NavFor(ui.EN, ui.ScreenScope, ui.Screens{Decl: true, Scope: true, Survey: true})}

	d := decl.Declaration{Org: "acme", Scope: []string{"web"},
		Nodes:  []decl.Node{{Name: "web", IPs: []string{"10.0.0.1"}}},
		Assets: []decl.Asset{{Node: "web", Runtime: "openssl", Component: "libssl"}},
		Edges:  []decl.Edge{{Src: "web", Dst: "web", Port: 443, Proto: "TLS"}}}

	rv := review.Session{Scope: "org://acme", PolicyDecisions: map[string]string{"p": ""},
		Items:    []review.Item{{ID: "a", Policy: "p", State: "UNDECLARED", Mandatory: true}},
		Autopass: []string{"b"}}

	sc := scope.Session{Org: "acme", LayerDecisions: map[string]string{"corp": ""},
		Changes: []scope.ChangeItem{{ID: "r1", Layer: "corp", Kind: scope.KindAdded,
			Rule: "exclude:openssl/lib/app", Audited: true}},
		Merged: []scope.Rule{{Action: "exclude", Runtime: "openssl", Lib: "lib", AppKey: "app"}}}

	for name, render := range map[string]func(w io.Writer) error{
		"decl":   func(w io.Writer) error { return ui.RenderDecl(w, ui.NewDeclView(d, page)) },
		"review": func(w io.Writer) error { return ui.RenderReview(w, ui.NewReviewView(rv, page)) },
		"scope": func(w io.Writer) error {
			return ui.RenderScope(w, ui.NewScopeView(sc, page).Editable(layerFiles()))
		},
		"row":     func(w io.Writer) error { return ui.RenderRow(w, ui.EN, ui.KindNode, 0) },
		"ruleRow": func(w io.Writer) error { return ui.RenderRuleRow(w, ui.EN, 0, 0) },
	} {
		var b strings.Builder
		if err := render(&b); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if around := hasHangul(b.String()); around != "" {
			t.Errorf("%s 화면을 영어로 그렸는데 한글이 남았다:\n   …%s…", name, around)
		}
	}
}

// IC-UI20 — **같은 화면이 두 말로 뜬다.** 한쪽만 그려지면 토글이 장식이 된다.
func TestScreenRendersInBothLanguages(t *testing.T) {
	sc := scope.Session{Org: "acme", LayerDecisions: map[string]string{}}
	for _, tc := range []struct {
		lang ui.Lang
		want string
	}{
		{ui.KO, "규칙을 적는 법"},
		{ui.EN, "How to write a rule"},
	} {
		var b strings.Builder
		page := ui.Page{Title: "t", Lang: tc.lang}
		if err := ui.RenderScope(&b, ui.NewScopeView(sc, page).Editable(layerFiles())); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), tc.want) {
			t.Errorf("%s 화면에 %q 가 없다", tc.lang, tc.want)
		}
	}
}

// IC-UI24 — **좁혀 놓은 것을 전부로 읽게 하지 않는다.**
//
// 「5개 중 2개」의 두 숫자는 한국어와 영어에서 자리가 뒤집힙니다(「전체 중 몇 개」 ↔
// 「몇 개 of 전체」). 자리를 번호로 고정하지 않으면 **한쪽 말에서만 숫자 둘이 바뀌어**
// 뜨고, 그 화면을 보는 사람은 좁혀 놓은 것을 전부로 읽습니다.
func TestShowingCountIsRightInBothLanguages(t *testing.T) {
	r := &report.Result{
		Assets: []reconcile.Reconciled{
			{Key: reconcile.AssetKey{NodeID: "web", Runtime: "openssl", Component: "libssl"}, State: "UNDECLARED"},
			{Key: reconcile.AssetKey{NodeID: "web", Runtime: "openssl", Component: "libcrypto"}, State: "CONFIRMED"},
			{Key: reconcile.AssetKey{NodeID: "db", Runtime: "jca", Component: "provider"}, State: "CONFIRMED"},
		},
	}
	for _, lang := range []ui.Lang{ui.KO, ui.EN} {
		var b strings.Builder
		v := ui.NewInventoryView(r, ui.Filter{Q: "libssl"}, ui.Page{Title: "t", Lang: lang})
		if err := ui.RenderInventory(&b, v); err != nil {
			t.Fatal(err)
		}
		if len(v.Assets) != 1 || v.TotalAssets != 3 {
			t.Fatalf("%s: 좁힌 결과가 %d/%d", lang, len(v.Assets), v.TotalAssets)
		}
		body := b.String()
		// 전체 3, 보이는 것 1 — 두 말 모두 그렇게 읽혀야 한다.
		want := "3개 중 <b>1개</b>"
		if lang == ui.EN {
			want = "<b>1</b> of 3"
		}
		if !strings.Contains(body, want) {
			t.Errorf("%s 화면에 %q 가 없다 — 숫자가 뒤집혔을 수 있다", lang, want)
		}
	}
}

// IC-UI25 — **좁히기는 아무 칸에나 걸린다.** 어느 칸인지 미리 고르게 하면 찾는 사람이
// 그 칸을 알아야 합니다 — 무엇을 찾는지 모를 때 여는 화면인데.
func TestInventoryFilterMatchesAnyColumn(t *testing.T) {
	r := &report.Result{
		Assets: []reconcile.Reconciled{
			{Key: reconcile.AssetKey{NodeID: "pay-db", Runtime: "openssl", Component: "libssl"}, State: "CONFIRMED"},
			{Key: reconcile.AssetKey{NodeID: "web-gw", Runtime: "jca", Component: "provider"}, State: "UNDECLARED"},
		},
	}
	page := ui.Page{Title: "t", Lang: ui.EN}
	for _, tc := range []struct {
		q    string
		want int
	}{
		{"pay-db", 1}, // 노드
		{"jca", 1},    // 런타임
		{"libssl", 1}, // 컴포넌트
		{"UNDECL", 1}, // 상태
		{"", 2},       // 안 좁히면 전부
		{"없는것", 0},    // 없는 것은 없다고 한다
	} {
		got := len(ui.NewInventoryView(r, ui.Filter{Q: tc.q}, page).Assets)
		if got != tc.want {
			t.Errorf("q=%q → %d개 (want %d)", tc.q, got, tc.want)
		}
	}
	// 상태로 좁히는 것은 자유 문자열과 따로다 — 둘 다 주면 둘 다 걸린다.
	v := ui.NewInventoryView(r, ui.Filter{Q: "web", State: "CONFIRMED"}, page)
	if len(v.Assets) != 0 {
		t.Errorf("상태와 문자열이 함께 걸리지 않는다: %+v", v.Assets)
	}
}

// IC-UI26 — **한 표가 두 목록을 대신한다.**
//
// 선언 파일에는 「관리 대상 노드」(`scope`)와 「노드 주소」(`nodes`)가 따로 있었고,
// 화면도 그렇게 둘로 물었습니다. 그래서 **같은 것을 두 번 적게 하고**, 어긋나면 짚어
// 줬습니다 — 한 곳에 적었으면 애초에 생기지 않을 어긋남입니다.
//
// 합친 표에서: 스코프에만 있던 노드는 IP가 빈 줄로, 주소 표에만 있던 노드도 한 줄로
// 올라옵니다. **아직 IP를 모르는 노드**를 적는 길은 그대로 남습니다 — IP 칸을 비우면
// 그것이 예전의 「스코프에만 있는 노드」입니다.
func TestDeclMergesScopeAndAddressesIntoOneTable(t *testing.T) {
	d := decl.Declaration{
		Scope: []string{"web", "db"}, // db 는 주소 표에 없다
		Nodes: []decl.Node{
			{Name: "web", IPs: []string{"10.0.0.1"}},
			{Name: "cache", IPs: []string{"10.0.0.9"}}, // 스코프에 없다
		},
	}
	v := ui.NewDeclView(d, ui.Page{Title: "선언", Lang: ui.KO})
	var names []string
	for _, n := range v.Decl.Nodes {
		names = append(names, n.Name+"="+strings.Join(n.IPs, ","))
	}
	// 스코프 순서가 앞이고, 주소 표에만 있던 것이 뒤에 붙는다.
	if got := strings.Join(names, " "); got != "web=10.0.0.1 db= cache=10.0.0.9" {
		t.Fatalf("합쳐진 표가 다르다: %s", got)
	}
}

// IC-UI27 — **저장하면 이름이 곧 관리 대상이 된다.** IP를 채운 줄만 주소 표에 들어간다 —
// IP 없는 주소 줄은 파일에 아무 뜻도 더하지 않는다.
func TestApplyDeclDerivesScopeFromTheTable(t *testing.T) {
	got := ui.ApplyDecl(decl.Declaration{}, url.Values{
		"node.name.0": {"web"}, "node.ips.0": {"10.0.0.1"},
		"node.name.1": {"db"}, "node.ips.1": {""}, // IP는 아직 모른다
		"node.name.2": {""}, "node.ips.2": {"10.0.0.9"}, // 이름이 없으면 지운 줄이다
	})
	if strings.Join(got.Scope, ",") != "web,db" {
		t.Fatalf("관리 대상이 다르다: %v", got.Scope)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].Name != "web" {
		t.Fatalf("주소 표에 IP 없는 줄이 들어갔다: %+v", got.Nodes)
	}
	// 한 바퀴 돌려도 같은 표가 나온다.
	back := ui.NewDeclView(got, ui.Page{Lang: ui.KO}).Decl.Nodes
	if len(back) != 2 || back[0].Name != "web" || back[1].Name != "db" || len(back[1].IPs) != 0 {
		t.Fatalf("다시 그리면 달라진다: %+v", back)
	}
}

