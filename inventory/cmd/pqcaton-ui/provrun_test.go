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
i=0; while [ ! -e "$PQCATON_GO" ]; do i=$((i+1)); [ $i -gt 60 ] && exit 250; sleep 0.05; done
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
// 언제까지 유효한지 적는다. 접속 정보가 실린 인벤토리와 변수는 실행 자리에 남지 않는다. **적용 버튼과 적용 주소는 없다.**
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
	if strings.Contains(page, "/approve/apply") || strings.Contains(page, "노드에 적용") {
		t.Error("this version must not offer an apply button")
	}
	if w := postForm(t, s, "/approve/apply", url.Values{}); w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /approve/apply = %d, want no such route", w.Code)
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
	if err := provrun.Consume(dir, time.Now(), m.Binding.Digest()); err != nil {
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

// IC-U240 — **적용 실행의 고정 사례: 첫째 노드 완료 → 둘째 노드 실패 → 셋째 노드 미시작.** 이 판에는 적용을 시작하는 주소가
// 없어 실행을 직접 띄운다. 화면은 첫째 「L2 배치 완료」(서비스 활성화와 운영 상태: 확인하지 않음), 둘째 「일부 작업만 끝남」
// (작업 실패), 셋째 「실행하지 않음」을 보인다. 실패의 까닭은 「작업 실패」뿐이고, 적용 실행은 적용 권한을 남기지 않는다.
func TestApplyFixedCaseFirstDoneSecondFailedThirdNotStarted(t *testing.T) {
	s := provServer(t, "fail2", threeNodes)
	v, err := s.approveModel(ui.Page{})
	if err != nil {
		t.Fatal(err)
	}
	rd, blocks := s.provCheck(v)
	if len(blocks) != 0 {
		t.Fatalf("blocks: %v", blocks)
	}
	if err := s.startProv(rd, false, ""); err != nil {
		t.Fatal(err)
	}
	waitProv(t, s)
	page := get(t, s, ui.ScreenApprove+"?lang=ko").Body.String()
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
	if out := fakeOut(t, "prov.txt"); strings.Contains(out, "--check") {
		t.Errorf("an apply run was started with --check:\n%s", out)
	}
	if g, _ := provrun.ReadGrant(lastProv(t, s)); g != nil || strings.Contains(page, "data-prov-grant") {
		t.Error("an apply run left a grant")
	}
}

// applyDirect: 이 판에는 적용을 시작하는 주소가 없어 적용 실행을 직접 띄우고 끝나기를 기다린다.
func applyDirect(t *testing.T, s *server) string {
	t.Helper()
	v, err := s.approveModel(ui.Page{})
	if err != nil {
		t.Fatal(err)
	}
	rd, blocks := s.provCheck(v)
	if len(blocks) != 0 {
		t.Fatalf("blocks: %v", blocks)
	}
	if err := s.startProv(rd, false, ""); err != nil {
		t.Fatal(err)
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
	page = applyDirect(t, s)
	if got := applyStates(page); got != "web=done app=unknown db=not-run" {
		t.Errorf("apply: %q", got)
	}
	if !strings.Contains(page, `<span class="why">실행 기록 중단</span>`) || !strings.Contains(page, "결과 없음") {
		t.Errorf("apply: the node with a start and no result should read unknown, record cut:\n%s", section(page, "prov-apply"))
	}

	t.Setenv("FAKE_PROV", "garble2")
	page = applyDirect(t, s)
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
	s := provServer(t, "fail2", threeNodes)
	page := applyDirect(t, s)
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
	if g, _ := provrun.ReadGrant(lastProv(t, s)); g != nil || strings.Contains(page, "data-prov-grant") || strings.Contains(page, `id="prov-result"`) {
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
