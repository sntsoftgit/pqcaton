package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

// 세션을 만들고 사람이 편집한 뒤 확정하는 왕복을 파일로 흉내 낸다. 실제 조작이 파일 왕복이라
// 케이스도 그렇게 재야 한다 — 함수만 부르면 파일 형식이 어긋나도 통과한다.
func writeCSV(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// capture — 표준출력으로 나가는 산출물을 받는다. open·close 가 stdout 으로 내므로,
// 그것을 잡지 않으면 무엇이 나왔는지 잴 수 없다.
func capture(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = old

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return b.String(), runErr
}

func openSession(t *testing.T, layers []string, base, orgName string) scope.Session {
	t.Helper()
	out, err := capture(t, func() error { return open(layers, base, orgName) })
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var sf scope.Session
	if err := json.Unmarshal([]byte(out), &sf); err != nil {
		t.Fatalf("세션 파일이 JSON 이 아니다: %v\n%s", err, out)
	}
	return sf
}

func saveSession(t *testing.T, dir string, sf scope.Session) string {
	t.Helper()
	p := filepath.Join(dir, "session.json")
	raw, err := json.Marshal(sf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const corpCSV = `action,runtime,lib,app_key,note
exclude,openssl,libcrypto.so.*,/usr/bin/python*,python 런타임
`

// IC-S8 — **exclude 추가는 결론 없이 확정되지 않는다.**
//
// 「이 자산은 안 본다」는 사고 뒤에 근거를 대야 하는 결정이다. 게이트가 명령에서 실제로
// 닫히는지는 여기서만 잴 수 있다 — 상태기계 케이스는 상태기계가 옳은 것만 말한다.
func TestCloseRefusesExcludeWithoutConclusion(t *testing.T) {
	dir := t.TempDir()
	corp := writeCSV(t, dir, "corp.csv", corpCSV)

	sf := openSession(t, []string{corp}, "", "acme")
	if len(sf.Changes) != 1 || !sf.Changes[0].Audited {
		t.Fatalf("근거 필수 변경이 잡히지 않았다: %+v", sf.Changes)
	}
	sf.Reviewer, sf.Signature = "보안팀", "sig" // 결론만 비워 둔다

	_, err := capture(t, func() error { return closeSession(saveSession(t, dir, sf), "", "acme") })
	if err == nil {
		t.Fatal("결론 없이 확정됐다 — 게이트가 열려 있다")
	}
	// **무엇이 남았는지 말해야 한다.** 모르면 사람은 파일을 고칠 수 없다.
	if !strings.Contains(err.Error(), sf.Changes[0].ID) {
		t.Errorf("어느 변경이 막았는지 말하지 않는다: %v", err)
	}
}

// IC-S9 — **서명이 없으면 확정하지 않는다.** 결론만 채우고 승인자를 비우는 것이 가장 흔한
// 빠뜨림이다.
func TestCloseRefusesWithoutSignature(t *testing.T) {
	dir := t.TempDir()
	sf := openSession(t, []string{writeCSV(t, dir, "corp.csv", corpCSV)}, "", "acme")
	sf.LayerDecisions["corp"] = "OS 패치로 관리한다"

	_, err := capture(t, func() error { return closeSession(saveSession(t, dir, sf), "", "acme") })
	if err == nil {
		t.Fatal("서명 없이 확정됐다")
	}
}

// IC-S10 — **승인하면 pqcota의 집행기가 읽는 CSV 가 나온다.**
//
// 계층 판정 하나로 그 계층의 규칙이 한 번에 판정되는 것(§3.4)도 여기서 함께 잰다.
func TestCloseEmitsPolicyForUpstream(t *testing.T) {
	dir := t.TempDir()
	corp := writeCSV(t, dir, "corp.csv", corpCSV)
	pay := writeCSV(t, dir, "pay.csv",
		"action,runtime,lib,app_key,note\ninclude,openssl,libcrypto.so.*,/usr/bin/python*,결제는 본다\n")

	sf := openSession(t, []string{corp, pay}, "", "acme")
	sf.Reviewer, sf.Signature = "보안팀", "sig"
	for l := range sf.LayerDecisions {
		sf.LayerDecisions[l] = "계층 일괄 승인"
	}

	out, err := capture(t, func() error { return closeSession(saveSession(t, dir, sf), "", "acme") })
	if err != nil {
		t.Fatalf("확정되지 않았다: %v", err)
	}
	// pqcota가 그대로 읽어야 한다 — 우리 형식을 만들면 「pqcota가 집행한다」가 거짓이 된다.
	p, err := kscope.LoadAssetPolicy(strings.NewReader(out))
	if err != nil {
		t.Fatalf("pqcota가 우리 CSV 를 읽지 못했다: %v\n%s", err, out)
	}
	if len(p.Rules) != 2 {
		t.Fatalf("규칙 %d개: %s", len(p.Rules), out)
	}
	// 계층 순서가 살아 있어야 상속이 성립한다 — 뒤가 이긴다.
	if !p.Rules[0].Exclude || p.Rules[1].Exclude {
		t.Errorf("계층 순서가 뒤집혔다: %+v", p.Rules)
	}
}

// IC-S11 — **남의 조직 세션을 확정하지 않는다.**
//
// 세션 파일은 건네받는 것이라 어느 조직 것인지 파일이 말한다. 지금 준 조직과 다르면 끊는다 —
// 대조 엔진·판정 원장과 같은 규칙이다.
func TestCloseRefusesAnotherOrgSession(t *testing.T) {
	dir := t.TempDir()
	sf := openSession(t, []string{writeCSV(t, dir, "corp.csv", corpCSV)}, "", "acme")
	sf.Reviewer, sf.Signature = "보안팀", "sig"
	sf.LayerDecisions["corp"] = "승인"

	_, err := capture(t, func() error { return closeSession(saveSession(t, dir, sf), "", "beta") })
	if err == nil {
		t.Fatal("남의 조직 세션이 확정됐다")
	}
	if !strings.Contains(err.Error(), "acme") || !strings.Contains(err.Error(), "beta") {
		t.Errorf("어긋난 두 조직을 함께 말하지 않는다: %v", err)
	}
}

// IC-S12 — **-base 를 주면 이미 쓰는 규칙은 올라오지 않는다.** 매번 전부 다시 승인하게 하면
// 아무도 안 본다.
func TestOpenWithBaseRaisesOnlyDelta(t *testing.T) {
	dir := t.TempDir()
	base := writeCSV(t, dir, "base.csv", corpCSV)
	corp := writeCSV(t, dir, "corp.csv", corpCSV+
		"exclude,*,*,/usr/sbin/sshd,sshd 는 OS 패치로 관리\n")

	sf := openSession(t, []string{corp}, base, "acme")
	if len(sf.Changes) != 1 {
		t.Fatalf("변경 %d건 — 이미 쓰는 규칙까지 올라왔다: %+v", len(sf.Changes), sf.Changes)
	}
	if !strings.Contains(sf.Changes[0].ID, "sshd") {
		t.Errorf("올라온 것이 새 규칙이 아니다: %s", sf.Changes[0].ID)
	}
	// 바뀐 것만 리뷰하되 나가는 것은 전문이다 — pqcota의 집행기는 정책 전체를 받는다.
	if len(sf.Merged) != 2 {
		t.Errorf("확정될 정책이 %d개 — 전문이 아니다", len(sf.Merged))
	}
}

// IC-S13 — **빈 조직으로는 열리지 않는다.** 저장소들과 같은 규칙이다.
func TestOpenRefusesEmptyOrg(t *testing.T) {
	dir := t.TempDir()
	_, err := capture(t, func() error {
		return open([]string{writeCSV(t, dir, "corp.csv", corpCSV)}, "", "")
	})
	if err == nil {
		t.Fatal("빈 조직으로 세션이 열렸다")
	}
}

// IC-S14 — 계층 이름은 파일 이름에서 온다. 이름이 곧 일괄 판정의 열쇠라 여기가 어긋나면
// 승인 단위가 흩어진다.
func TestLayerNameFromPath(t *testing.T) {
	for path, want := range map[string]string{
		"/etc/pqcaton/prod.csv": "prod",
		"corp.csv":              "corp",
		"a/b/pay-tier.csv":      "pay-tier",
	} {
		if got := layerName(path); got != want {
			t.Errorf("layerName(%q) = %q, want %q", path, got, want)
		}
	}
}
