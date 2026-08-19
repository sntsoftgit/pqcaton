package scope_test

import (
	"os"
	"path/filepath"
	"testing"

	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

func rule(exclude bool, runtime, lib, appKey, note string) kscope.AssetRule {
	return kscope.AssetRule{Exclude: exclude, Runtime: runtime, Lib: lib, AppKey: appKey, Note: note}
}

// IC-S8 — **계층 파일을 쓰고 다시 읽으면 같은 규칙이 나온다.**
//
// 화면이 고쳐 쓰는 파일이 곧 다음 리뷰의 입력입니다. 여기가 어긋나면 저장할 때마다
// 규칙이 조금씩 달라지고, 아무도 그것을 눈치채지 못합니다.
func TestSaveLayerRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corp.csv")
	in := []kscope.AssetRule{
		rule(true, "openssl", "libcrypto.so.*", "/usr/bin/python*", "python 런타임 — 관리 대상 아님"),
		rule(false, "openssl", "libssl.so.3", "*", "결제 게이트웨이 — 위 제외의 예외"),
	}
	if err := scope.SaveLayer(scope.LayerFile{Path: path, Layer: scope.Layer{Name: "corp", Rules: in}}); err != nil {
		t.Fatal(err)
	}
	files, err := scope.LoadLayers([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Layer.Name != "corp" {
		t.Fatalf("계층 이름이 파일 이름에서 오지 않았다: %+v", files)
	}
	got := files[0].Layer.Rules
	if len(got) != len(in) {
		t.Fatalf("규칙 수가 다르다: %d != %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("%d번 규칙이 달라졌다:\n  got  %+v\n  want %+v", i, got[i], in[i])
		}
	}
}

// IC-S9 — **쓰다 만 파일을 남기지 않는다.** 계층 파일은 사람이 손으로도 고치는 것이라,
// 잘린 CSV 가 남으면 다음에 열 때 규칙이 통째로 사라진 것처럼 보인다.
func TestSaveLayerLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corp.csv")
	if err := scope.SaveLayer(scope.LayerFile{Path: path,
		Layer: scope.Layer{Name: "corp", Rules: []kscope.AssetRule{rule(true, "openssl", "*", "*", "")}}}); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != "corp.csv" {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Fatalf("남은 파일: %v", names)
	}
}

// 판정이 채워진 세션 하나. 계층 corp 에 exclude 두 개.
func judgedSession(t *testing.T) (scope.Session, []scope.Layer) {
	t.Helper()
	layers := []scope.Layer{{Name: "corp", Rules: []kscope.AssetRule{
		rule(true, "openssl", "libcrypto.so.*", "/usr/bin/python*", "python 런타임"),
		rule(true, "*", "*", "/usr/sbin/sshd", "sshd 는 OS 패치로 관리"),
	}}}
	sf := scope.NewSession(layers, nil, "acme")
	sf.Reviewer, sf.Signature = "보안팀", "sig-1"
	sf.LayerDecisions["corp"] = "OS 패치로 관리하므로 인벤토리에서 뺀다"
	for i := range sf.Changes {
		sf.Changes[i].Conclusion = "판정 " + sf.Changes[i].ID
	}
	return sf, layers
}

// IC-S10 — **규칙을 고쳐도 적어 둔 판정은 남는다.**
//
// 고칠 때마다 판정을 처음부터 다시 적게 하면 아무도 화면에서 고치지 않습니다. 규칙의
// 동일성(RuleID)이 열쇠이고, note 는 동일성에 넣지 않으므로 설명을 다듬은 것만으로는
// 판정이 날아가지 않습니다.
func TestReopenKeepsJudgmentsAcrossNoteEdit(t *testing.T) {
	sf, layers := judgedSession(t)
	edited := []scope.Layer{{Name: "corp", Rules: []kscope.AssetRule{
		rule(true, "openssl", "libcrypto.so.*", "/usr/bin/python*", "설명을 다듬었다"),
		layers[0].Rules[1],
	}}}

	next := scope.Reopen(sf, edited, nil, "acme")
	if len(next.Changes) != 2 {
		t.Fatalf("변경 %d건", len(next.Changes))
	}
	for _, c := range next.Changes {
		if c.Conclusion != "판정 "+c.ID {
			t.Errorf("%s 의 판정이 날아갔다: %q", c.ID, c.Conclusion)
		}
	}
	if next.LayerDecisions["corp"] == "" {
		t.Error("계층 결론이 날아갔다 — 못 보던 변경이 생긴 것이 아니다")
	}
	if next.Reviewer != "보안팀" {
		t.Error("승인자 이름이 날아갔다 — 사람은 그대로다")
	}
	// note 는 나가는 CSV 의 한 칸이라 정책은 달라졌다.
	if next.Signature != "" {
		t.Error("정책이 달라졌는데 서명이 남았다")
	}
}

// IC-S11 — **계층에 못 보던 변경이 생기면 그 계층의 일괄 결론을 지운다.**
//
// 일괄 판정은 「이 계층의 변경들을 보고 내린 결론」입니다. 새 변경은 사람이 본 적이
// 없는데 그대로 두면, 방금 넣은 exclude 가 **누가 승인한 적 없는 근거를 달고** 확정을
// 통과합니다 — 오류 없이 틀리는 자리입니다.
func TestReopenClearsLayerDecisionOnNewChange(t *testing.T) {
	sf, layers := judgedSession(t)
	edited := []scope.Layer{{Name: "corp", Rules: append(append([]kscope.AssetRule{}, layers[0].Rules...),
		rule(true, "openssl", "libssl.so.*", "/usr/bin/curl", "방금 넣었다"))}}

	next := scope.Reopen(sf, edited, nil, "acme")
	if next.LayerDecisions["corp"] != "" {
		t.Fatalf("못 보던 변경이 생겼는데 계층 결론이 남았다: %q", next.LayerDecisions["corp"])
	}
	// 이미 판정한 것까지 지우지는 않는다 — 그러면 아무도 화면을 쓰지 않는다.
	kept := 0
	for _, c := range next.Changes {
		if c.Conclusion != "" {
			kept++
		}
	}
	if kept != 2 {
		t.Errorf("이미 적은 판정 %d건만 남았다 — 2건이어야 한다", kept)
	}
	if _, err := scope.Finalize(next, "acme"); err == nil {
		t.Error("근거 없는 exclude 가 확정을 통과했다")
	}
}

// IC-S12 — 정책이 그대로면 서명도 그대로다. 아무것도 안 바꾼 화면 새로고침마다 서명을
// 지우면, 사람은 서명 칸을 계속 다시 채우게 된다.
func TestReopenKeepsSignatureWhenPolicyUnchanged(t *testing.T) {
	sf, layers := judgedSession(t)
	next := scope.Reopen(sf, layers, nil, "acme")
	if next.Signature != "sig-1" {
		t.Fatalf("정책이 같은데 서명이 지워졌다: %q", next.Signature)
	}
}
