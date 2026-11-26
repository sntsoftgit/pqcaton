// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build unix

package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/provrun"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// fakeProvAnsible: 06 실행에서 ansible-playbook 대신 부를 스크립트. 전용 callback이 하는 일을 흉내 낸다: 감독이 만든 기록에
// 「준비됨」을 덧붙이고, 허용 파일을 기다리고, 넘겨받은 플레이북의 play와 작업 이름대로 사건을 쓴다. FAKE_PROV가 가른다.
//
//	(없음) 모든 작업이 changed · nochange 모두 ok · skip 첫 노드의 둘째 작업을 건너뜀 · fail2 둘째 노드의 둘째 작업이 실패
//	hang2 둘째 노드의 둘째 작업을 시작하고 멈춤 · cut 첫 노드의 둘째 작업 앞에서 말없이 끝남 · noload 준비 신호 없이 250으로 끝남
//	replay 앞 실행(FAKE_PROV_OLD)의 ID로 사건을 씀 · recfail2 둘째 노드의 둘째 작업을 시작한 뒤 기록을 쓰지 못해 끝남(251)
//	garble2 둘째 노드의 둘째 작업 앞에 읽을 수 없는 줄을 쓰고 계속함
const fakeProvAnsible = `#!/bin/sh
{ echo "ARGS=$*"; echo "ANSIBLE_ENV=$(env | grep -c '^ANSIBLE_')"; echo "INVENTORY=$(test -f inventory.yml && echo yes || echo no)"
  echo "GO_AT_START=$(test -e "$PQCATON_GO" && echo yes || echo no)"; } >> "$FAKE_OUT/prov.txt"
[ "$FAKE_PROV" = noload ] && exit 250
ev="$PQCATON_EVENTS"; run="$PQCATON_RUN_ID"; seq=0
[ "$FAKE_PROV" = replay ] && run="$FAKE_PROV_OLD"
check=false; for a in "$@"; do [ "$a" = --check ] && check=true; book="$a"; done
emit() { seq=$((seq+1)); printf '{"seq":%d,"run":"%s",%s}\n' "$seq" "$run" "$1" >> "$ev"; }
emit '"ev":"ready"'
i=0; while ! { [ -f "$PQCATON_GO" ] && grep -q "\"$PQCATON_RUN_ID\"" "$PQCATON_GO" && grep -q "\"$PQCATON_GO_TOKEN\"" "$PQCATON_GO"; }; do i=$((i+1)); [ $i -gt 60 ] && exit 250; sleep 0.05; done
emit "\"ev\":\"begin\",\"check\":$check"
node=""; n=0; t=0; k=0
task() {
  k=$((k+1)); emit "\"ev\":\"task\",\"task\":\"t$k\",\"name\":\"$1\""
  emit "\"ev\":\"start\",\"host\":\"$node\",\"task\":\"t$k\""
  if [ "$2" = hang ]; then "$FAKE_HELPER" -test-sleep; fi
  emit "\"ev\":\"result\",\"host\":\"$node\",\"task\":\"t$k\",\"status\":\"$2\""
}
while IFS= read -r line; do
  case "$line" in
    "  hosts: ["*) node=${line#*\[\"}; node=${node%\"\]}; n=$((n+1)); t=0
      emit "\"ev\":\"play\",\"hosts\":[\"$node\"]"; task "Gathering Facts" ok ;;
    "    - name: "*) name=${line#"    - name: "}; name=${name#\"}; name=${name%\"}; t=$((t+1)); st=changed
      case "$FAKE_PROV" in
        nochange) st=ok ;;
        skip) if [ $n = 1 ] && [ $t = 2 ]; then st=skipped; fi ;;
        fail2) if [ $n = 2 ] && [ $t = 2 ]; then task "$name" failed; exit 2; fi ;;
        hang2) if [ $n = 2 ] && [ $t = 2 ]; then st=hang; fi ;;
        cut) if [ $n = 1 ] && [ $t = 2 ]; then exit 0; fi ;;
        recfail2) if [ $n = 2 ] && [ $t = 2 ]; then k=$((k+1)); emit "\"ev\":\"task\",\"task\":\"t$k\",\"name\":\"$name\""
            emit "\"ev\":\"start\",\"host\":\"$node\",\"task\":\"t$k\""; exit 251; fi ;;
        garble2) if [ $n = 2 ] && [ $t = 2 ]; then echo "not a record line" >> "$ev"; fi ;;
      esac
      task "$name" "$st" ;;
  esac
done < "$book"
emit '"ev":"end"'
`

// provHosts: 세 노드의 접근 정보. db에 비밀번호가 있다.
const provHosts = "node_id,name,ip,port,ssh_user,ssh_key,ssh_pass,os,connection\n" +
	"web,Web,10.0.0.2,22,deploy,/home/me/.ssh/id_ed25519,,linux,ssh\n" +
	"app,App,10.0.0.5,22,deploy,/home/me/.ssh/id_ed25519,,linux,ssh\n" +
	"db,DB,10.0.0.9,22,deploy,,s3cret-pw,linux,ssh\n"

// planOn: 노드마다 설정만 놓는 L2 조치가 하나씩인 계획. extra는 조치마다 덧붙일 JSON 칸이다.
func planOn(extra string, nodes ...string) string {
	var acts []string
	for i, n := range nodes {
		acts = append(acts, fmt.Sprintf(`{"id":"a%d","targetNodeId":%q,"findingId":"f%d","kind":"REMEDIATION_KIND_CONFIG_ONLY",`+
			`"cryptoRuntime":"CRYPTO_RUNTIME_OPENSSL","automationLevel":"DEPLOY_AUTOMATION_LEVEL_L2_STAGE_INSTALL","targetAlgorithm":"ML-KEM (FIPS 203)",%s`+
			`"evidenceSources":[{"findingId":"f%d","snapshot":{"sourceNodeId":%q,"content":{"formatVersion":"pqcota-snapshot-content/v1",`+
			`"digest":%q,"rulesetVersion":%q}}}]}`, i+1, n, i+1, extra, i+1, n, planDigest, normalizeRuleset))
	}
	return `{"id":"plan-1","status":"PLAN_STATUS_IN_REVIEW","scope":"acme","rulesetVersion":"` + review.RulesetVersion + `","actions":[` +
		strings.Join(acts, ",") + `]}`
}

var normalizeRuleset = regexp.MustCompile(`"rulesetVersion":"([^"]+)"\}\}\}\]`).FindStringSubmatch(planJSON)[1]

// provServer: 서명하고 L2 플레이북까지 만든 서버와 가짜 ansible-playbook. 계획은 web · app · db에 조치가 하나씩이다.
func provServer(t *testing.T, mode, plan string) *server {
	t.Helper()
	s, _ := keyReady(t)
	bindPlan(t, s, plan)
	if w := signVia(t, s, "보안팀장"); w.Code != http.StatusSeeOther {
		t.Fatalf("sign = %d\n%s", w.Code, w.Body.String())
	}
	if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l2"}}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("generate = %d\n%s", w.Code, w.Body.String())
	}
	if err := os.WriteFile(s.hosts, []byte(provHosts), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(t.TempDir(), "ansible-playbook")
	if err := os.WriteFile(fake, []byte(fakeProvAnsible), 0o755); err != nil {
		t.Fatal(err)
	}
	old, oldStall := ansibleCommand, provStall
	ansibleCommand = fake
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_HELPER", helper)
	t.Setenv("FAKE_PROV", mode)
	t.Setenv("FAKE_OUT", t.TempDir())
	t.Cleanup(func() {
		s.shutdownProv()
		ansibleCommand, provStall = old, oldStall
	})
	return s
}

// preview: 화면이 하는 그대로 미리 보기를 시작하고 끝나기를 기다린 뒤 06 화면을 돌려준다.
func preview(t *testing.T, s *server) string {
	t.Helper()
	if w := postForm(t, s, ui.ProvPreviewPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("preview = %d\n%s", w.Code, w.Body.String())
	}
	waitProv(t, s)
	return get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
}

func waitProv(t *testing.T, s *server) {
	t.Helper()
	s.prov.mu.Lock()
	j := s.prov.job
	s.prov.mu.Unlock()
	if j == nil {
		t.Fatal("nothing ran")
	}
	select {
	case <-j.done:
	case <-time.After(60 * time.Second):
		t.Fatal("the provisioning run did not finish")
	}
}

// lastProv: 가장 최근 실행 자리.
func lastProv(t *testing.T, s *server) string {
	t.Helper()
	all := s.provAll()
	if len(all) == 0 {
		t.Fatal("no run folder")
	}
	return all[len(all)-1].dir
}

var provAttr = regexp.MustCompile(`data-prov-node="([^"]+)" data-prov-state="([^"]+)"`)

// provStates: 미리 보기 구역의 노드 줄에서 「노드=상태」를 읽는다. applyStates는 적용 실행 구역의 것이다.
func provStates(page string) string  { return statesIn(page, "prov-result") }
func applyStates(page string) string { return statesIn(page, "prov-apply") }

func statesIn(page, id string) string {
	i := strings.Index(page, `<section id="`+id+`"`)
	if i < 0 {
		return ""
	}
	page = page[i:]
	if j := strings.Index(page, "</section>"); j >= 0 {
		page = page[:j]
	}
	var out []string
	for _, m := range provAttr.FindAllStringSubmatch(page, -1) {
		out = append(out, m[1]+"="+m[2])
	}
	return strings.Join(out, " ")
}

var threeNodes = planOn("", "web", "app", "db")

// IC-U234 — **06은 승인된 L2 설정 배치 플레이북을 대상 노드에 미리 본다.** 미리 보기 버튼은 대상 노드 수를 적는다. 실행은
// `--check`로 돌고, 운영자의 ANSIBLE_* 환경은 설정 파일 하나를 가리키는 것 말고 넘어가지 않는다. 감독은 callback의 준비 신호를
// 본 뒤에만 허용 파일을 둔다(시작할 때는 없다). 끝나면 화면은 「바뀔 것이 있다」와 노드마다 「확인함」을 보이고, 미리 본 내용이
// 언제까지 유효한지 적고, 적용 확인으로 가는 버튼을 낸다(대상 수를 적는다). 접속 정보가 실린 인벤토리와 변수는 실행 자리에
// 남지 않는다. 확인 토큰 없이 적용 주소로 직접 보낸 요청은 거절된다.
func TestPreviewRunsTheApprovedPlaybookInCheckMode(t *testing.T) {
	s := provServer(t, "", threeNodes)
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if !strings.Contains(page, "3개 노드 미리 보기") || strings.Contains(page, "data-prov-blocked") {
		t.Fatalf("the screen should offer a preview on 3 nodes:\n%s", section(page, "approve-run"))
	}
	page = preview(t, s)
	if got := provStates(page); got != "web=done app=done db=done" {
		t.Errorf("node states = %q", got)
	}
	for _, want := range []string{`data-prov-outcome="ready"`, `data-prov-grant="valid"`, "확인함", "바뀜 예정", "한 번의 적용에만 유효합니다", "노드의 상태는 바뀔 수 있습니다"} {
		if !strings.Contains(page, want) {
			t.Errorf("the screen lacks %q", want)
		}
	}
	out := fakeOut(t, "prov.txt")
	for _, want := range []string{"--check playbook.yml", "ANSIBLE_ENV=1", "INVENTORY=yes", "GO_AT_START=no"} {
		if !strings.Contains(out, want) {
			t.Errorf("the run: want %q in\n%s", want, out)
		}
	}
	dir := lastProv(t, s)
	for _, gone := range []string{provInventory, "vars.json", provGoFile} {
		if isFile(filepath.Join(dir, gone)) {
			t.Errorf("%s was left in the run folder", gone)
		}
	}
	if readFile(t, filepath.Join(dir, provPlaybook)) != readFile(t, s.provisionPath()) {
		t.Error("the run folder's playbook is not the generated one")
	}
	if strings.Contains(readFile(t, filepath.Join(dir, provEventsFile)), "s3cret-pw") {
		t.Error("the run record holds an access secret")
	}
	if !strings.Contains(page, "data-prov-apply") || !strings.Contains(page, "3개 노드에 적용: 확인으로") {
		t.Error("after a ready preview the screen should offer the way to the apply confirmation")
	}
	if w := postForm(t, s, ui.ProvApplyPath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusForbidden {
		t.Errorf("POST apply without a confirmation token = %d, want 403", w.Code)
	}
	if len(applyRuns(s)) != 0 || provrun.Used(dir) {
		t.Error("a refused apply started a run or used the grant")
	}
}

// section: 화면에서 그 id의 제목부터 뒤쪽 일부. 실패 메시지를 줄인다.
func section(page, id string) string {
	i := strings.Index(page, `id="`+id+`"`)
	if i < 0 {
		return "(no #" + id + ")"
	}
	if len(page) > i+1500 {
		return page[i : i+1500]
	}
	return page[i:]
}

// IC-U235 — **화면이 실행하지 않는 것은 까닭과 함께 밝히고 시작하지 않는다.** L3로 생성한 플레이북, 생성 뒤 손으로 고친
// 플레이북, 접근 정보에 없는 노드, 계획이 L3로 확정한 조치, provider 모듈을 놓는 조치가 저마다의 까닭으로 막힌다(노드와 작업
// 이름이 함께 나온다). 그때 미리 보기 버튼은 없고 직접 보낸 요청은 409이며 실행 자리가 생기지 않는다. 생성된 플레이북은
// 그대로다.
func TestUnsupportedRunsAreNamedAndNotStarted(t *testing.T) {
	blocked := func(t *testing.T, s *server, code string, also ...string) {
		t.Helper()
		page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
		if !strings.Contains(page, `data-prov-block="`+code+`"`) || strings.Contains(page, "data-prov-preview") {
			t.Errorf("want block %s and no preview button:\n%s", code, section(page, "approve-run"))
		}
		for _, a := range also {
			if !strings.Contains(page, a) {
				t.Errorf("block %s: the screen lacks %q", code, a)
			}
		}
		if w := postForm(t, s, ui.ProvPreviewPath, url.Values{}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedProvNotReady) {
			t.Errorf("block %s: preview = %d, want 409 %s", code, w.Code, ui.RefusedProvNotReady)
		}
		if len(s.provRuns()) != 0 {
			t.Errorf("block %s: a run folder was made", code)
		}
	}
	t.Run("generated at L3", func(t *testing.T) {
		s := provServer(t, "", threeNodes)
		if w := postForm(t, s, ui.GeneratePath, withRev(url.Values{"level": {"l3"}}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
			t.Fatalf("generate l3 = %d", w.Code)
		}
		blocked(t, s, ui.ProvLevel)
	})
	t.Run("edited after generation", func(t *testing.T) {
		s := provServer(t, "", threeNodes)
		book := readFile(t, s.provisionPath())
		if err := os.WriteFile(s.provisionPath(), []byte(strings.Replace(book, "/opt/pqcota", "/opt/other", 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		blocked(t, s, ui.ProvNotGenerated)
		if !strings.Contains(readFile(t, s.provisionPath()), "/opt/other") {
			t.Error("the screen changed the playbook file")
		}
	})
	t.Run("a node missing from the access details", func(t *testing.T) {
		s := provServer(t, "", threeNodes)
		if err := os.WriteFile(s.hosts, []byte(strings.Replace(provHosts, "app,App", "other,App", 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		blocked(t, s, ui.ProvNoHosts, "<code>app</code>")
	})
	t.Run("an action the plan finalized at L3", func(t *testing.T) {
		s := provServer(t, "", strings.Replace(threeNodes, `"targetNodeId":"db","findingId":"f3","kind":"REMEDIATION_KIND_CONFIG_ONLY","cryptoRuntime":"CRYPTO_RUNTIME_OPENSSL","automationLevel":"DEPLOY_AUTOMATION_LEVEL_L2_STAGE_INSTALL"`,
			`"targetNodeId":"db","findingId":"f3","kind":"REMEDIATION_KIND_CONFIG_ONLY","cryptoRuntime":"CRYPTO_RUNTIME_OPENSSL","automationLevel":"DEPLOY_AUTOMATION_LEVEL_L3_FULL_AUTO"`, 1))
		blocked(t, s, ui.ProvUnsupported+"-"+provrun.WhyL3, "<code>db</code>")
	})
	t.Run("a provider module", func(t *testing.T) {
		s := provServer(t, "", strings.Replace(threeNodes, `"targetNodeId":"app","findingId":"f2","kind":"REMEDIATION_KIND_CONFIG_ONLY"`,
			`"targetNodeId":"app","findingId":"f2","kind":"REMEDIATION_KIND_PROVIDER_INJECT","providerChoice":"oqsprovider"`, 1))
		blocked(t, s, ui.ProvUnsupported+"-"+provrun.WhyProvider, "<code>app</code>", "stage the provider module (oqsprovider)")
	})
}

// IC-U236 — **미리 보기의 세 판정과 적용 권한의 일생.** 바뀔 것이 있으면 권한이 생기고 유효하다. 끝난 지 10분이 지나면
// 「만료」다. 그 전이라도 접근 정보가 바뀌면 「입력이 바뀜」이고 되돌리면 다시 유효하다. 한 번 쓰면 「이미 쓰임」이다. 모든
// 작업이 ok이면 「적용할 변경이 없습니다」이고 권한이 없다. 작업 하나를 건너뛰면 「미리 보기 미완료」이고 권한이 없다.
func TestPreviewVerdictsAndTheGrant(t *testing.T) {
	s := provServer(t, "", threeNodes)
	grant := func() string {
		m := regexp.MustCompile(`data-prov-grant="([^"]+)"`).FindStringSubmatch(get(t, s, ui.ScreenApprove+"?lang=ko").Body.String())
		if m == nil {
			return ""
		}
		return m[1]
	}
	preview(t, s)
	if got := grant(); got != "valid" {
		t.Fatalf("after a ready preview the grant is %q", got)
	}
	s.clock = func() time.Time { return time.Now().Add(provrun.GrantTTL + time.Second) }
	if got := grant(); got != "expired" {
		t.Errorf("after ten minutes the grant is %q", got)
	}
	s.clock = nil
	if err := os.WriteFile(s.hosts, []byte(strings.Replace(provHosts, "10.0.0.9", "10.0.0.10", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := grant(); got != "changed" {
		t.Errorf("after the access details changed the grant is %q", got)
	}
	if err := os.WriteFile(s.hosts, []byte(provHosts), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := grant(); got != "valid" {
		t.Errorf("with the inputs back the grant is %q", got)
	}
	dir := lastProv(t, s)
	m, _ := readProvManifest(dir)
	if err := provrun.Consume(dir, time.Now(), m.Binding.Digest(), "test"); err != nil {
		t.Fatal(err)
	}
	if got := grant(); got != "used" {
		t.Errorf("after one use the grant is %q", got)
	}

	t.Setenv("FAKE_PROV", "nochange")
	page := preview(t, s)
	if !strings.Contains(page, `data-prov-outcome="no-change"`) || !strings.Contains(page, "적용할 변경이 없습니다") || strings.Contains(page, "data-prov-grant") {
		t.Errorf("an all-ok preview:\n%s", section(page, "prov-result"))
	}
	if g, _ := provrun.ReadGrant(lastProv(t, s)); g != nil {
		t.Error("an all-ok preview left a grant")
	}
	t.Setenv("FAKE_PROV", "skip")
	page = preview(t, s)
	if !strings.Contains(page, `data-prov-outcome="incomplete"`) || !strings.Contains(page, "미리 보기 미완료") || strings.Contains(page, "data-prov-grant") {
		t.Errorf("a preview with a skipped task:\n%s", section(page, "prov-result"))
	}
	if got := provStates(page); got != "web=skipped app=done db=done" {
		t.Errorf("node states = %q", got)
	}
}

// IC-U237 — **앞 실행의 기록으로 뒤 실행을 승인하지 못하고, 기록이 준비되지 않으면 아무것도 시작하지 않는다.** 같은 계획 ·
// 같은 노드 · 같은 작업을 이어서 미리 보되 둘째 실행이 첫 실행의 ID로 사건을 쓰면, 둘째는 「미완료」이고 권한이 없다(첫 실행의
// 권한도 더는 보이지 않는다). callback이 준비 신호 없이 끝나면 감독은 허용 파일을 두지 않고, 화면은 「원격 작업을 시작하지
// 않았습니다」와 모든 노드 「실행하지 않음」을 보인다.
func TestAnEarlierRecordOrNoRecordNeverApproves(t *testing.T) {
	s := provServer(t, "", threeNodes)
	preview(t, s)
	first := filepath.Base(lastProv(t, s))
	t.Setenv("FAKE_PROV", "replay")
	t.Setenv("FAKE_PROV_OLD", first)
	page := preview(t, s)
	if filepath.Base(lastProv(t, s)) == first {
		t.Fatal("the second preview reused the first run's folder")
	}
	if !strings.Contains(page, `data-prov-outcome="incomplete"`) || strings.Contains(page, "data-prov-grant") {
		t.Errorf("a run that wrote the earlier run's id:\n%s", section(page, "prov-result"))
	}
	if g, _ := provrun.ReadGrant(lastProv(t, s)); g != nil {
		t.Error("the replayed record left a grant")
	}

	t.Setenv("FAKE_PROV", "noload")
	page = preview(t, s)
	if !strings.Contains(page, `data-prov-end="not-ready"`) || !strings.Contains(page, "원격 작업을 시작하지 않았습니다") {
		t.Errorf("a callback that never got ready:\n%s", section(page, "prov-result"))
	}
	if got := provStates(page); got != "web=not-run app=not-run db=not-run" {
		t.Errorf("node states = %q", got)
	}
	if out := fakeOut(t, "prov.txt"); strings.Contains(out, "GO_AT_START=yes") {
		t.Errorf("the go-ahead file existed before a ready signal:\n%s", out)
	}
}

// IC-U238 — **결과가 오지 않으면 상한에서 끝내고, 그 노드는 「결과를 알 수 없음」이다.** 둘째 노드가 작업을 시작하고 답이
// 없으면 감독이 상한(시험에서는 1초) 뒤 실행을 끝낸다. 첫째 노드는 「확인함」, 둘째는 「결과를 알 수 없음」(N초 동안 결과를 받지
// 못함), 셋째는 「실행하지 않음」이다. 화면은 「이미 시작한 원격 작업은 뒤늦게 끝날 수 있습니다」를 보인다. 운영자가 중지해도
// 같은 문구이고 이유는 「컨트롤러에서 중단」이다. 어느 쪽도 이 실행의 프로세스를 남기지 않는다.
func TestStallAndStopLeaveTheNodeUnknown(t *testing.T) {
	s := provServer(t, "hang2", threeNodes)
	provStall = 1
	page := preview(t, s)
	if got := provStates(page); got != "web=done app=unknown db=not-run" {
		t.Errorf("after a stall: %q", got)
	}
	for _, want := range []string{`data-prov-end="stall"`, "1초 동안 결과를 받지 못해 컨트롤러에서 실행을 중단했습니다", "이미 시작한 원격 작업은 뒤늦게 끝날 수 있습니다", "1초 동안 결과를 받지 못함", `data-prov-outcome="incomplete"`} {
		if !strings.Contains(page, want) {
			t.Errorf("after a stall the screen lacks %q", want)
		}
	}
	if ps, err := markedProcs(runMark{Marker: runMarker(filepath.Base(lastProv(t, s)))}); err != nil || len(ps) != 0 {
		t.Errorf("processes left after the stall: %v %v", ps, err)
	}

	provStall = 120
	if w := postForm(t, s, ui.ProvPreviewPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("preview = %d", w.Code)
	}
	rec := filepath.Join(lastProv(t, s), provEventsFile)
	for i := 0; i < 200 && len(provrun.Pending([]byte(readOr(rec)))) == 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if running := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String(); !strings.Contains(running, `id="prov-running"`) || strings.Contains(running, "data-prov-preview") {
		t.Error("while a run is going the screen should show it and no preview button")
	}
	if w := postForm(t, s, ui.ProvPreviewPath, url.Values{}); w.Code != http.StatusConflict {
		t.Errorf("a second preview while one runs = %d, want 409", w.Code)
	}
	if w := postForm(t, s, ui.ProvStopPath, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("stop = %d", w.Code)
	}
	waitProv(t, s)
	page = get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if got := provStates(page); got != "web=done app=unknown db=not-run" {
		t.Errorf("after a stop: %q", got)
	}
	for _, want := range []string{`data-prov-end="stopped"`, "컨트롤러에서 실행을 중단했습니다. 이미 시작한 원격 작업은 뒤늦게 끝날 수 있습니다.", "컨트롤러에서 중단"} {
		if !strings.Contains(page, want) {
			t.Errorf("after a stop the screen lacks %q", want)
		}
	}
	if ps, err := markedProcs(runMark{Marker: runMarker(filepath.Base(lastProv(t, s)))}); err != nil || len(ps) != 0 {
		t.Errorf("processes left after the stop: %v %v", ps, err)
	}
}

func readOr(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

// IC-U239 — **화면이 다시 떠도 실행 기록이 말하는 만큼만 말한다.** 감독이 결과를 남기지 못하고 끝난 실행 자리(기록이 첫 노드의
// 작업 도중에서 끊겼고 인벤토리가 남아 있다)를 둔 채 시작하면, 시작 때의 복구가 인벤토리를 지우고 결과를 「감독이 남기지 못함」
// 으로 적는다. 화면은 「실행 기록 중단」과 그 노드 「결과를 알 수 없음」, 나머지 「실행하지 않음」을 보인다. 실행 기록은 고치지
// 않는다. 말없이 끝난 실행(끝 표시가 없는 기록)도 「실행 기록 중단」이다.
func TestRestartSaysOnlyWhatTheRecordSays(t *testing.T) {
	s := provServer(t, "cut", threeNodes)
	page := preview(t, s)
	if !strings.Contains(page, "실행 기록 중단") || !strings.Contains(page, `data-prov-outcome="incomplete"`) {
		t.Errorf("a record with no end:\n%s", section(page, "prov-result"))
	}
	if got := provStates(page); got != "web=unknown app=not-run db=not-run" {
		t.Errorf("a record cut between tasks: %q", got)
	}

	// 감독이 죽은 자리를 만든다: 앞 실행의 자리를 새 ID로 베끼고, 결과를 지우고, 기록을 작업 도중에서 끊고, 인벤토리를 남긴다.
	old := lastProv(t, s)
	m, _ := readProvManifest(old)
	m.ID, m.Check = "29991231T000000-aaaaaaaaaaaa", false
	dir := filepath.Join(s.provDir(), m.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeDurable(filepath.Join(dir, provManifestFile), m); err != nil {
		t.Fatal(err)
	}
	record := string(provrun.Header(m.ID)) + `{"seq":1,"run":"` + m.ID + `","ev":"ready"}` + "\n" + `{"seq":2,"run":"` + m.ID + `","ev":"begin","check":false}` + "\n" +
		`{"seq":3,"run":"` + m.ID + `","ev":"play","hosts":["web"]}` + "\n" + `{"seq":4,"run":"` + m.ID + `","ev":"task","task":"t1","name":"Gathering Facts"}` + "\n" +
		`{"seq":5,"run":"` + m.ID + `","ev":"start","host":"web","task":"t1"}` + "\n"
	for name, body := range map[string]string{provEventsFile: record, provInventory: "secret", "vars.json": "{}"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s.recoverProv()
	if isFile(filepath.Join(dir, provInventory)) || isFile(filepath.Join(dir, "vars.json")) {
		t.Error("the recovery left the inventory in the run folder")
	}
	if readFile(t, filepath.Join(dir, provEventsFile)) != record {
		t.Error("the recovery changed the run record")
	}
	page = get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if got := applyStates(page); got != "web=unknown app=not-run db=not-run" {
		t.Errorf("after a restart: %q", got)
	}
	for _, want := range []string{`data-prov-end="lost"`, "실행 기록 중단", "마지막 적용 실행"} {
		if !strings.Contains(page, want) {
			t.Errorf("after a restart the screen lacks %q", want)
		}
	}
}

// IC-U240 — **적용 실행의 고정 사례: 첫째 노드 완료 → 둘째 노드 실패 → 셋째 노드 미시작.** 06의 적용 확인을 거쳐 시작한다.
// 화면은 첫째 「L2 배치 완료」(서비스 활성화와 운영 상태: 확인하지 않음), 둘째 「일부 작업만 끝남」
// (작업 실패), 셋째 「실행하지 않음」을 보인다. 실패의 까닭은 「작업 실패」뿐이고, 적용 실행은 적용 권한을 남기지 않는다.
func TestApplyFixedCaseFirstDoneSecondFailedThirdNotStarted(t *testing.T) {
	s := provServer(t, "", threeNodes)
	page := applyVia(t, s, "fail2")
	if got := applyStates(page); got != "web=done app=partial db=not-run" {
		t.Errorf("node states = %q", got)
	}
	for _, want := range []string{"마지막 적용 실행", "L2 배치 완료", "서비스 활성화와 운영 상태: 확인하지 않음", "일부 작업만 끝남", "작업 실패", "실행하지 않음", `data-prov-outcome="not-applied"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the screen lacks %q", want)
		}
	}
	// 상태와 이유의 문구가 같으면 이유를 따로 적지 않는다(「작업 실패 작업 실패」가 되지 않게).
	if strings.Contains(page, `<span class="why">일부 작업만 끝남</span>`) || !strings.Contains(page, `<span class="why">작업 실패</span>`) {
		t.Error("the reason should appear once, beside a state it differs from")
	}
	if out := fakeOut(t, "prov.txt"); strings.Count(out, "ARGS=") != 2 || strings.Count(out, "--check") != 1 {
		t.Errorf("want one preview with --check and one apply without:\n%s", out)
	}
	if g, _ := provrun.ReadGrant(lastProv(t, s)); g != nil {
		t.Error("an apply run left a grant of its own")
	}
}

// applyRuns: 적용 실행의 자리들(시작한 순서대로).
func applyRuns(s *server) []provRun {
	var out []provRun
	for _, r := range s.provAll() {
		if !r.m.Check {
			out = append(out, r)
		}
	}
	return out
}

// applyVia: 화면이 하는 그대로 적용한다. 미리 보기(모든 작업이 바뀜 예정)로 권한을 받고, 적용 확인 페이지를 거쳐 그 양식을 보낸다.
// 적용 실행의 가짜 Ansible은 mode로 돈다. 끝난 뒤의 06 화면을 돌려준다.
func applyVia(t *testing.T, s *server, mode string) string {
	t.Helper()
	t.Setenv("FAKE_PROV", "")
	if page := preview(t, s); !strings.Contains(page, "data-prov-apply") {
		t.Fatalf("no way to the apply confirmation after a preview:\n%s", section(page, "approve-run"))
	}
	t.Setenv("FAKE_PROV", mode)
	if w := finalizeVia(t, s, "ko", ui.ProvApplyConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("apply = %d\n%s", w.Code, w.Body.String())
	}
	waitProv(t, s)
	return get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
}

// IC-U241 — **준비 신호를 주고받은 뒤에 기록이 끊기면, 기록된 성공까지만 인정한다.** 둘째 노드가 작업을 시작한 뒤 callback이
// 기록을 쓰지 못해 실행을 끝내면(종료 251) 감독은 그것을 「기록 중단」으로 적는다(운영자 중지와 다른 사유다). 미리 보기는
// 「미완료」이고 적용 권한이 없다. 적용 실행에서는 첫째 노드 「L2 배치 완료」, 시작만 기록된 둘째 노드 「결과를 알 수 없음(실행 기록
// 중단)」, 셋째 노드는 기록이 읽을 수 있는 데까지가 전부이므로 「실행하지 않음」이다. **기록 가운데에 읽을 수 없는 줄이 있으면**
// 그 뒤를 알 수 없으므로 셋째 노드도 「실행하지 않음」이 아니라 「결과를 알 수 없음」이다.
func TestRecordCutAfterTheHandshake(t *testing.T) {
	s := provServer(t, "recfail2", threeNodes)
	page := preview(t, s)
	for _, want := range []string{`data-prov-outcome="incomplete"`, `data-prov-end="record"`, "기록을 더 쓰지 못해 실행을 끝냈습니다"} {
		if !strings.Contains(page, want) {
			t.Errorf("preview: the screen lacks %q", want)
		}
	}
	if strings.Contains(page, "data-prov-grant") || strings.Contains(page, "컨트롤러에서 실행을 중단했습니다") {
		t.Error("preview: a cut record must leave no grant and must not read as an operator stop")
	}
	if got := provStates(page); got != "web=done app=unknown db=not-run" {
		t.Errorf("preview: %q", got)
	}
	if g, _ := provrun.ReadGrant(lastProv(t, s)); g != nil {
		t.Error("preview: a grant was written")
	}
	page = applyVia(t, s, "recfail2")
	if got := applyStates(page); got != "web=done app=unknown db=not-run" {
		t.Errorf("apply: %q", got)
	}
	if !strings.Contains(page, `<span class="why">실행 기록 중단</span>`) || !strings.Contains(page, "결과 없음") {
		t.Errorf("apply: the node with a start and no result should read unknown, record cut:\n%s", section(page, "prov-apply"))
	}

	page = applyVia(t, s, "garble2")
	if got := applyStates(page); got != "web=done app=unknown db=unknown" {
		t.Errorf("a record with an unreadable line: %q", got)
	}
	if !strings.Contains(page, `data-prov-outcome="not-applied"`) || !strings.Contains(page, "실행 기록 중단") {
		t.Errorf("a record with an unreadable line:\n%s", section(page, "prov-apply"))
	}
}

var recheckAttr = regexp.MustCompile(`data-prov-recheck-node="([^"]*)"`)

// rechecks: 적용 실행 구역의 노드별 재확인 칸.
func rechecks(page string) string {
	i := strings.Index(page, `<section id="prov-apply"`)
	if i < 0 {
		return ""
	}
	page = page[i:]
	if j := strings.Index(page, "</section>"); j >= 0 {
		page = page[:j]
	}
	var out []string
	for _, m := range recheckAttr.FindAllStringSubmatch(page, -1) {
		out = append(out, m[1])
	}
	return strings.Join(out, " ")
}

// IC-U242 — **적용 기록과 재확인을 한 화면에서 견준다.** 적용 실행(첫째 완료 → 둘째 실패 → 셋째 미시작) 뒤 화면은 그 실행의
// ID와 노드별 상태, 「이 실행 재확인」을 보인다. 재확인하면 노드마다 재확인 시각(UTC)과 결과가 상태 옆에 붙는다: 모든 작업이 ok면
// 「파일 일치」, 바뀔 작업이 있으면 「다름」, 재확인이 앞 노드에서 멈춰 닿지 못했으면 「재확인하지 못함」. **실행 기록의 상태는
// 그대로다**(셋째 노드는 「파일 일치」여도 「실행하지 않음」이다). 재확인은 적용 권한을 남기지 않는다. 관계는 실행 자리의 기록에
// 있어, 화면의 실행 상태를 비운 뒤(다시 띄운 것과 같다)에도 같은 실행과 재확인이 이어진다. 입력이 바뀌면 재확인 버튼 대신 까닭이
// 보이고 직접 보낸 요청은 409다.
func TestApplyRecordAndRecheckSideBySide(t *testing.T) {
	s := provServer(t, "", threeNodes)
	page := applyVia(t, s, "fail2")
	apply := filepath.Base(lastProv(t, s))
	if !strings.Contains(page, "실행 ID "+apply) || !strings.Contains(page, "data-prov-recheck>") || rechecks(page) != "" {
		t.Fatalf("after the apply run:\n%s", section(page, "prov-apply"))
	}
	recheck := func(mode string) string {
		t.Helper()
		t.Setenv("FAKE_PROV", mode)
		if w := postForm(t, s, ui.ProvRecheckPath, url.Values{}); w.Code != http.StatusSeeOther {
			t.Fatalf("recheck = %d\n%s", w.Code, w.Body.String())
		}
		waitProv(t, s)
		return get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	}
	page = recheck("nochange")
	first := filepath.Base(lastProv(t, s))
	if got := rechecks(page); got != "match match match" {
		t.Errorf("rechecks = %q", got)
	}
	if got := applyStates(page); got != "web=done app=partial db=not-run" {
		t.Errorf("the run record changed after a recheck: %q", got)
	}
	for _, want := range []string{`data-prov-recheck-run="` + first + `"`, "UTC · 파일 일치", "실행 ID " + apply, "실행하지 않음"} {
		if !strings.Contains(page, want) {
			t.Errorf("after a recheck the screen lacks %q", want)
		}
	}
	if g, _ := provrun.ReadGrant(lastProv(t, s)); g != nil || strings.Contains(page, `data-prov-run="`+first+`"`) {
		t.Error("a recheck must leave no grant and must not show as a preview")
	}

	page = recheck("fail2")
	second := filepath.Base(lastProv(t, s))
	if got := rechecks(page); got != "differs unreached unreached" {
		t.Errorf("a recheck that stopped at the second node: %q", got)
	}
	if !strings.Contains(page, "재확인하지 못함") || !strings.Contains(page, "UTC · 다름") {
		t.Errorf("the recheck column:\n%s", section(page, "prov-apply"))
	}
	// 화면을 다시 띄운 것처럼: 메모리의 실행 상태를 비우고 다시 그린다.
	s.prov.mu.Lock()
	s.prov.job = nil
	s.prov.mu.Unlock()
	again := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if !strings.Contains(again, "실행 ID "+apply) || !strings.Contains(again, `data-prov-recheck-run="`+second+`"`) || rechecks(again) != "differs unreached unreached" ||
		applyStates(again) != "web=done app=partial db=not-run" {
		t.Errorf("after a restart the run and its recheck are not the same pair:\n%s", section(again, "prov-apply"))
	}

	if err := os.WriteFile(s.hosts, []byte(strings.Replace(provHosts, "10.0.0.9", "10.0.0.10", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if strings.Contains(changed, "data-prov-recheck>") || !strings.Contains(changed, "data-prov-recheck-blocked") {
		t.Error("with changed inputs the screen should explain instead of offering a recheck")
	}
	if w := postForm(t, s, ui.ProvRecheckPath, url.Values{}); w.Code != http.StatusConflict {
		t.Errorf("recheck with changed inputs = %d, want 409", w.Code)
	}
}

// IC-U243 — **적용은 확인을 거치고, 권한은 한 번만 쓰이고, 두 번째 실행은 시작되지 않는다.** 확인 페이지는 조직 · 대상 노드
// 이름과 수 · 승인된 계획 · 생성된 플레이북 · 미리 보기의 실행과 만료 · 노드별 변경 예정 작업을 보이고, 마지막 버튼은 「3개 노드에
// 적용」이다. **확인 페이지를 여는 것만으로는 권한이 쓰이지 않는다.** 적용을 누르면 실행 자리에 입력 사본(서명한 계획 · 플레이북)
// 을 두고 `--check` 없이 돈다. 같은 양식을 도는 동안 다시 보내면 409(이미 돌고 있음)이고, 끝난 뒤에 다시 보내도, 화면의 실행
// 상태를 비운 뒤(다시 띄운 것과 같다) 다시 보내도 409(유효한 미리 보기 없음)다. 적용 실행은 하나뿐이고, 권한의 소비 기록은 그
// 실행의 ID를 적는다. 그 뒤 화면에는 적용으로 가는 버튼이 없다.
func TestApplyGoesThroughTheConfirmationOnce(t *testing.T) {
	s := provServer(t, "", threeNodes)
	provStall = 3 // 상한은 미리 본 입력의 하나다. 미리 보기 전에 정한다(뒤에 바꾸면 입력이 바뀐 것이다)
	preview(t, s)
	grantDir := lastProv(t, s)
	w := postFormLang(t, s, ui.ProvApplyConfirmPath, "ko", withRev(url.Values{}, s.revFor(ui.ScreenApprove)))
	if w.Code != http.StatusOK {
		t.Fatalf("confirm = %d\n%s", w.Code, w.Body.String())
	}
	confirm := w.Body.String()
	signed, _ := loadPlanFile(s.signedPath())
	for _, want := range []string{"조직: " + s.orgName(), "대상 노드 3개: web, app, db", signed.GetId(), "plan.provision.yml", filepath.Base(grantDir),
		"web의 변경 예정", "db의 변경 예정", ">3개 노드에 적용<", "까지 유효", "L2 배치까지입니다"} {
		if !strings.Contains(confirm, want) {
			t.Errorf("the confirmation page lacks %q", want)
		}
	}
	if provrun.Used(grantDir) || len(applyRuns(s)) != 0 {
		t.Fatal("opening the confirmation page used the grant or started a run")
	}
	form := hiddenFields(confirm)
	t.Setenv("FAKE_PROV", "hang2")
	if w := postForm(t, s, ui.ProvApplyPath, form); w.Code != http.StatusSeeOther {
		t.Fatalf("apply = %d\n%s", w.Code, w.Body.String())
	}
	if w := postForm(t, s, ui.ProvApplyPath, form); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedProvBusy) {
		t.Errorf("the same form again while the run goes = %d, want 409 %s", w.Code, ui.RefusedProvBusy)
	}
	waitProv(t, s)
	runs := applyRuns(s)
	if len(runs) != 1 {
		t.Fatalf("%d apply runs, want 1", len(runs))
	}
	if u := provrun.UsedBy(grantDir); u == nil || u.By != runs[0].m.ID {
		t.Errorf("the use record = %+v, want by %s", u, runs[0].m.ID)
	}
	if out := fakeOut(t, "prov.txt"); strings.Count(out, "ARGS=") != 2 || strings.Count(out, "--check") != 1 {
		t.Errorf("want one preview and one apply without --check:\n%s", out)
	}
	for _, name := range []string{provPlanCopy, provPlaybook} {
		want := s.signedPath()
		if name == provPlaybook {
			want = s.provisionPath()
		}
		if readFile(t, filepath.Join(runs[0].dir, name)) != readFile(t, want) {
			t.Errorf("the run folder's %s is not a copy of the input", name)
		}
	}
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
	if got := applyStates(page); got != "web=done app=unknown db=not-run" || strings.Contains(page, "data-prov-apply") || !strings.Contains(page, `data-prov-grant="used"`) {
		t.Errorf("after the apply: %q\n%s", got, section(page, "approve-run"))
	}
	again := func(what string) {
		t.Helper()
		if w := postForm(t, s, ui.ProvApplyPath, form); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedProvGrant) {
			t.Errorf("%s: the same form = %d, want 409 %s", what, w.Code, ui.RefusedProvGrant)
		}
		if w := postForm(t, s, ui.ProvApplyConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusConflict {
			t.Errorf("%s: a new confirmation = %d, want 409", what, w.Code)
		}
		if len(applyRuns(s)) != 1 {
			t.Errorf("%s: a second apply run was started", what)
		}
	}
	again("after the run ended")
	s.prov.mu.Lock()
	s.prov.job = nil
	s.prov.mu.Unlock()
	s.recoverProv()
	again("after a restart")
}

// IC-U244 — **권한은 준비 신호 뒤, 원격 작업을 허용하기 직전에 소비한다. 그 앞뒤에서 끊겨도 원격 작업은 0건이다.**
//   - 소비 직전에 감독이 죽으면: 권한은 미사용이고 기록에 실행의 시작이 없다. 화면은 「원격 작업을 시작하지 않았고 미리 보기를
//     쓰지 않았습니다」를 보이고, 같은 미리 보기로 다시 적용할 수 있다.
//   - 소비 직전에 권한이 만료됐거나, 실행 자리의 입력 사본이 미리 본 것과 달라졌으면: 소비하지 않고 허용하지 않는다.
//   - **소비 직후, 허용하기 전에 감독이 죽으면: 권한은 그 실행의 ID로 소비된 채이고 기록에 실행의 시작이 없다.** 화면은 「미리
//     보기를 썼지만 원격 작업을 시작하지 않았습니다」와 새 미리 보기 안내를 보인다. 다시 보낸 요청과, 다시 띄운 뒤의 요청은
//     409다. 두 번째 적용 실행은 시작되지 않는다.
//   - 허용은 감독이 넘긴 토큰과 실행 ID를 적은 파일이다. 허용 파일 자리에 폴더가 있으면 감독은 소비한 뒤 허용을 전하지 못하고,
//     callback은 그것을 허용으로 받아들이지 않는다: 원격 작업 0건 · 권한 소비 1건(사유 「허용하지 못함」).
func TestGrantIsConsumedJustBeforeRemoteWorkIsAllowed(t *testing.T) {
	s := provServer(t, "", threeNodes)
	began := func(dir string) bool {
		return strings.Contains(readOr(filepath.Join(dir, provEventsFile)), `"ev":"begin"`)
	}
	try := func(hook string) (string, provRun, string) {
		t.Helper()
		t.Setenv("FAKE_PROV_HOOK", "")
		t.Setenv("FAKE_PROV", "")
		preview(t, s)
		grantDir := lastProv(t, s)
		t.Setenv("FAKE_PROV_HOOK", hook)
		if w := finalizeVia(t, s, "ko", ui.ProvApplyConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
			t.Fatalf("%s: apply = %d\n%s", hook, w.Code, w.Body.String())
		}
		waitProv(t, s)
		runs := applyRuns(s)
		return get(t, s, ui.ScreenApprove+"?lang=ko").Body.String(), runs[len(runs)-1], grantDir
	}
	for _, hook := range []string{"before-consume:die", "before-consume:expire", "before-consume:swap"} {
		page, run, grantDir := try(hook)
		if began(run.dir) || provrun.Used(grantDir) {
			t.Errorf("%s: began=%v used=%v, want no remote work and an unused grant", hook, began(run.dir), provrun.Used(grantDir))
		}
		if got := applyStates(page); got != "web=not-run app=not-run db=not-run" || !strings.Contains(page, `data-prov-begun="no"`) || strings.Contains(page, "data-prov-consumed") ||
			!strings.Contains(page, "미리 보기를 쓰지 않았습니다") {
			t.Errorf("%s: %q\n%s", hook, got, section(page, "prov-apply"))
		}
		if hook != "before-consume:die" && !strings.Contains(page, `data-prov-end="no-grant"`) {
			t.Errorf("%s: the run should end for want of a usable grant", hook)
		}
	}
	// 소비 직전에 죽은 뒤에는 같은 미리 보기로 다시 적용할 수 있다.
	t.Setenv("FAKE_PROV_HOOK", "")
	t.Setenv("FAKE_PROV", "")
	preview(t, s)
	grantDir := lastProv(t, s)
	t.Setenv("FAKE_PROV_HOOK", "before-consume:die")
	if w := finalizeVia(t, s, "ko", ui.ProvApplyConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("apply = %d", w.Code)
	}
	waitProv(t, s)
	t.Setenv("FAKE_PROV_HOOK", "")
	if page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String(); !strings.Contains(page, "data-prov-apply") {
		t.Fatalf("after a run that died before consuming, the same preview should still allow an apply:\n%s", section(page, "approve-run"))
	}
	if w := finalizeVia(t, s, "ko", ui.ProvApplyConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusSeeOther {
		t.Fatalf("the retry = %d\n%s", w.Code, w.Body.String())
	}
	waitProv(t, s)
	runs := applyRuns(s)
	if u := provrun.UsedBy(grantDir); u == nil || u.By != runs[len(runs)-1].m.ID || !began(runs[len(runs)-1].dir) {
		t.Errorf("the retry should consume the grant and begin: %+v", u)
	}

	// 허용 파일 자리에 폴더가 있으면 감독은 소비한 뒤 허용을 전하지 못한다. **그 자리에 무엇이 있다는 것만으로는 허용이 아니다.**
	blocked, brun, bgrant := try("before-consume:block")
	if began(brun.dir) {
		t.Fatal("a directory at the go-ahead path was taken for a go-ahead")
	}
	if u := provrun.UsedBy(bgrant); u == nil || u.By != brun.m.ID || !strings.Contains(blocked, `data-prov-end="not-allowed"`) || !strings.Contains(blocked, "data-prov-consumed") {
		t.Errorf("blocked go-ahead: use=%+v\n%s", u, section(blocked, "prov-apply"))
	}

	before := len(applyRuns(s))
	page, run, grantDir := try("after-consume:die")
	if began(run.dir) {
		t.Fatal("after-consume: remote work began although the go-ahead was never given")
	}
	if u := provrun.UsedBy(grantDir); u == nil || u.By != run.m.ID {
		t.Errorf("after-consume: the use record = %+v, want by %s", u, run.m.ID)
	}
	if got := applyStates(page); got != "web=not-run app=not-run db=not-run" || !strings.Contains(page, "data-prov-consumed") || !strings.Contains(page, "미리 보기를 썼지만 원격 작업을 시작하지 않았습니다") ||
		strings.Contains(page, "data-prov-apply") || !strings.Contains(page, `data-prov-grant="used"`) {
		t.Errorf("after-consume: %q\n%s", got, section(page, "approve-run"))
	}
	t.Setenv("FAKE_PROV_HOOK", "")
	refused := func(what string) {
		t.Helper()
		if w := postForm(t, s, ui.ProvApplyConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenApprove))); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedProvGrant) {
			t.Errorf("%s: a new apply = %d, want 409 %s", what, w.Code, ui.RefusedProvGrant)
		}
		if len(applyRuns(s)) != before+1 {
			t.Errorf("%s: another apply run was started", what)
		}
	}
	refused("after the run")
	s.prov.mu.Lock()
	s.prov.job = nil
	s.prov.mu.Unlock()
	s.recoverProv()
	refused("after a restart")
	if began(run.dir) {
		t.Error("after a restart the record should still show no remote work")
	}
}
