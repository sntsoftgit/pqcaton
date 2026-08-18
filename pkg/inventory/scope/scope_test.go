package scope_test

import (
	"bytes"
	"strings"
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

func ex(runtime, lib, app, note string) kscope.AssetRule {
	return kscope.AssetRule{Exclude: true, Runtime: runtime, Lib: lib, AppKey: app, Note: note}
}
func inc(runtime, lib, app, note string) kscope.AssetRule {
	return kscope.AssetRule{Runtime: runtime, Lib: lib, AppKey: app, Note: note}
}

func finding(lib, app string) *discoveryv1.Finding {
	return &discoveryv1.Finding{
		CryptoRuntime:    commonv1.CryptoRuntime_CRYPTO_RUNTIME_OPENSSL,
		EvidenceStrength: commonv1.EvidenceStrength_EVIDENCE_STRENGTH_CONFIRMED,
		Openssl:          &discoveryv1.OpenSSLFinding{Lib: lib},
		AppKeys:          []string{app},
	}
}

// IC-S1 — **상속은 규칙 이어붙이기다.** 상류의 「뒤 규칙이 이긴다」를 그대로 쓰므로 판정
// 규칙이 세상에 하나만 존재한다. 우리가 잠금을 따로 두면 내려보낸 CSV 를 상류가 집행한
// 결과와 우리 화면이 갈라진다.
func TestMergeLetsLowerLayerWin(t *testing.T) {
	조직 := scope.Layer{Name: "corp", Rules: []kscope.AssetRule{ex("openssl", "libcrypto*", "", "전사 제외")}}
	노드군 := scope.Layer{Name: "pay", Rules: []kscope.AssetRule{inc("openssl", "libcrypto*", "", "결제는 본다")}}

	p := scope.Merge(조직, 노드군)
	if !p.Managed(finding("libcrypto.so.3", "/opt/pay")) {
		t.Error("하위 계층의 include 가 상위 exclude 를 되돌리지 못했다")
	}
	// 순서를 뒤집으면 결과도 뒤집힌다 — 「뒤가 이긴다」가 그대로 상속 규칙이라는 증거다.
	if scope.Merge(노드군, 조직).Managed(finding("libcrypto.so.3", "/opt/pay")) {
		t.Error("순서를 뒤집었는데도 include 가 이겼다")
	}
}

// IC-S2 — **바뀐 것만 리뷰에 올린다.** 매번 전부 다시 승인하게 하면 아무도 안 본다.
func TestDiffOnlyChanges(t *testing.T) {
	base := &kscope.AssetPolicy{Rules: []kscope.AssetRule{ex("openssl", "libssl*", "", "옛 규칙")}}
	layers := []scope.Layer{{Name: "corp", Rules: []kscope.AssetRule{
		ex("openssl", "libssl*", "", "설명만 고쳤다"), // 그대로 — 올라오면 안 된다
		ex("jca", "*", "/usr/bin/java", "새 제외"),   // 추가
	}}}

	got := scope.Diff(base, layers)
	if len(got) != 1 {
		t.Fatalf("변경 %d건: %+v", len(got), got)
	}
	if !got[0].Added || got[0].Layer != "corp" {
		t.Errorf("추가로 잡히지 않았다: %+v", got[0])
	}
	if !got[0].Audited {
		t.Error("exclude 추가는 근거 필수여야 한다 — 「안 본다」는 감사 대상이다")
	}
}

// IC-S3 — **사라진 규칙도 리뷰 대상이다.** 다만 근거 필수는 아니다 — 제외를 거두는 것은
// 인벤토리가 넓어지는 방향이라 무게가 다르다.
func TestDiffReportsRemoval(t *testing.T) {
	base := &kscope.AssetPolicy{Rules: []kscope.AssetRule{ex("openssl", "libssl*", "", "")}}
	got := scope.Diff(base, nil)
	if len(got) != 1 || got[0].Added {
		t.Fatalf("제거가 잡히지 않았다: %+v", got)
	}
	if got[0].Audited {
		t.Error("제외를 거두는 것까지 근거 필수로 두면 넓히는 방향이 막힌다")
	}
}

// IC-S4 — **note 는 동일성이 아니다.** 사람이 읽으라고 붙인 설명이라 문구를 다듬었다고
// 재승인을 받게 하면 리뷰가 잡음으로 찬다.
func TestRuleIDIgnoresNote(t *testing.T) {
	a := scope.RuleID(ex("openssl", "libssl*", "", "처음 쓴 설명"))
	b := scope.RuleID(ex("openssl", "libssl*", "", "다듬은 설명"))
	if a != b {
		t.Errorf("note 가 동일성에 섞였다: %q vs %q", a, b)
	}
	if a == scope.RuleID(inc("openssl", "libssl*", "", "")) {
		t.Error("exclude 와 include 가 같은 규칙으로 보인다")
	}
}

// IC-S5 — **나가는 형식은 상류 것 그대로다.** 우리 형식을 만들면 「거버넌스가 확정한 정책을
// 상류가 집행한다」가 코드로는 거짓이 된다. 써서 다시 읽어 같은 판정이 나오는지로 잰다.
func TestWriteCSVRoundTripsThroughUpstream(t *testing.T) {
	p := &kscope.AssetPolicy{Rules: []kscope.AssetRule{
		ex("openssl", "libcrypto*", "/usr/bin/python*", "python 런타임"),
		inc("openssl", "libssl.so.3", "", "결제 게이트웨이 — 위 제외의 예외"),
	}}
	var buf bytes.Buffer
	if err := scope.WriteCSV(&buf, p); err != nil {
		t.Fatal(err)
	}
	back, err := kscope.LoadAssetPolicy(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("상류가 우리 CSV 를 읽지 못했다: %v", err)
	}
	if len(back.Rules) != len(p.Rules) {
		t.Fatalf("규칙 %d개로 돌아왔다, want %d\n%s", len(back.Rules), len(p.Rules), buf.String())
	}
	for i := range p.Rules {
		if scope.RuleID(back.Rules[i]) != scope.RuleID(p.Rules[i]) {
			t.Errorf("%d번 규칙이 달라졌다: %s → %s", i,
				scope.RuleID(p.Rules[i]), scope.RuleID(back.Rules[i]))
		}
	}
}

// IC-S6 — **뺀 것을 이름으로 낸다.** 상류는 수만 센다(`ExcludedByScope`) — 잡음을 거르는
// 것이 목적이라 그것으로 충분하다. 사고 뒤에 "왜 이게 인벤토리에 없었나"에 답하려면
// 무엇이 빠졌는지 말할 수 있어야 한다.
func TestExcludedFromNamesWhatWasDropped(t *testing.T) {
	p := &kscope.AssetPolicy{Rules: []kscope.AssetRule{ex("openssl", "libcrypto*", "", "")}}
	got := scope.ExcludedFrom(p, "web-gw", []*discoveryv1.Finding{
		finding("libcrypto.so.3", "/usr/bin/python"), // 빠진다
		finding("libssl.so.3", "/opt/pay"),           // 남는다
	})
	if len(got) != 1 {
		t.Fatalf("제외 %d건: %+v", len(got), got)
	}
	if got[0].Subject() != "web-gw/openssl/libcrypto.so.3" {
		t.Errorf("대상 키 = %q", got[0].Subject())
	}
	if !got[0].StillObserved {
		t.Error("관측된 finding 을 걸러낸 것이므로 지금도 관측되는 것이다")
	}
}

// IC-S7 — **제외는 영구 면제가 아니다.** 승인이 아예 없는 것과 오래된 것은 다시 올리고,
// 살아 있는 승인은 조용히 둔다 — 매번 전부 올리면 아무도 안 본다.
func TestReviewRaisesUnjudgedAndStale(t *testing.T) {
	now := int64(1_000_000)
	ttl := int64(100)
	ex := []scope.Excluded{
		{Node: "n", Runtime: "openssl", Asset: "없음"},
		{Node: "n", Runtime: "openssl", Asset: "오래됨"},
		{Node: "n", Runtime: "openssl", Asset: "살아있음"},
	}
	prior := []decision.Judgment{
		{Subject: "n/openssl/오래됨", Conclusion: "제외 승인", DecidedAt: now - ttl - 1},
		{Subject: "n/openssl/살아있음", Conclusion: "제외 승인", DecidedAt: now - 1},
	}

	got := scope.Review(ex, prior, now, ttl)
	if len(got) != 2 {
		t.Fatalf("다시 볼 것 %d건: %+v", len(got), got)
	}
	bySubject := map[string]string{}
	for _, r := range got {
		bySubject[r.Subject()] = r.Reason
	}
	if bySubject["n/openssl/없음"] != scope.ReasonNeverJudged {
		t.Errorf("승인 없는 제외의 사유 = %q", bySubject["n/openssl/없음"])
	}
	if bySubject["n/openssl/오래됨"] != scope.ReasonStale {
		t.Errorf("만료된 제외의 사유 = %q", bySubject["n/openssl/오래됨"])
	}
	if _, raised := bySubject["n/openssl/살아있음"]; raised {
		t.Error("승인이 살아 있는 것까지 올리면 리뷰가 잡음으로 찬다")
	}
}
