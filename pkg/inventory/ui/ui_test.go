package ui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
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
		`name="org"`, `name="scope"`,
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
		"org": {in.Org}, "scope": {"web"},
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
			{ID: "a", Layer: "corp", Kind: "추가", Audited: true},
			{ID: "b", Layer: "corp", Kind: "제거"},
			{ID: "c", Layer: "pay", Kind: "추가", Audited: true},
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
		if err := ui.RenderRow(&b, tc.kind, 7); err != nil {
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
	if err := ui.RenderRow(&b, ui.KindNode, 7); err != nil {
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
			{Exclude: true, Runtime: "openssl", Lib: "libcrypto.so.*", AppKey: "/usr/bin/python*", Note: "python 런타임"},
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
		"rule.0.0.note": {"python 런타임"},
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
		"rule.0.0.lib": {""}, "rule.0.0.app_key": {""}, "rule.0.0.note": {"python 런타임"},
	}
	if n := len(ui.ApplyLayers(layerFiles(), f)[0].Layer.Rules); n != 0 {
		t.Fatalf("비운 줄이 남았다: %d개", n)
	}
}

// IC-UI17 — 「행 추가」가 낸 규칙 줄이 화면과 같은 폼 이름을 쓰고, 다음 번호가 오른다.
func TestRuleRowFragment(t *testing.T) {
	var b strings.Builder
	if err := ui.RenderRuleRow(&b, 1, 4); err != nil {
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
