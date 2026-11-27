// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const stars = " ***********************************************"

// runLog: Ansible이 찍는 꼴의 로그. 점검 → 스테이징 → OpenSSL 관측 도중이다. n1은 시작했고, n2는 시작한 뒤 실패했고,
// n3은 아직 아무 줄도 없다.
func runLog(extra ...string) string {
	lines := []string{
		"pqcaton: /usr/bin/ansible-playbook -i targets.yml -e @vars.json observe.yml (3 nodes, network window 60s)",
		"", "PLAY [pqcaton observation check]" + stars, "",
		"TASK [Read the account the node is reached with]" + stars, "ok: [n1]", "ok: [n2]", "ok: [n3]", "",
		"pqcaton: node web-gw: passed: the node is reached as root, so the key does not go through sudo",
		"", "PLAY [pqcaton observation]" + stars, "",
		"TASK [Gathering Facts]" + stars, "ok: [n1]", "ok: [n2]", "",
		"TASK [Make the staging directory]" + stars, "changed: [n1]", "changed: [n2]", "",
		"TASK [Observe the OpenSSL runtime]" + stars, "fatal: [n2]: FAILED! => {\"msg\": \"x\"}",
	}
	return strings.Join(append(lines, extra...), "\n")
}

func runAt(state ui.RunState, log string) ui.RunStatus {
	started := time.Date(2026, 11, 23, 7, 8, 4, 0, time.UTC)
	st := ui.RunStatus{ID: "20261123T070804-aaaaaaaaaaaa", State: state, Started: started, Window: 60, Log: log,
		Nodes: []ui.RunNode{{Alias: "n1", Node: "web-gw"}, {Alias: "n2", Node: "pay-app"}, {Alias: "n3", Node: "pay-db"}}}
	if state != ui.RunRunning {
		st.Ended = started.Add(95 * time.Second)
	}
	return st
}

// IC-UI72 — **관측의 지금 단계와 노드별 상태는 실행 로그에 근거가 있는 것만 읽는다.** 단계는 마지막으로 시작한 플레이 ·
// 단계 줄에서 읽고(여섯 단계 가운데 몇 번째인지와 함께), 모르는 단계 이름이면 앞의 단계를 그대로 둔다. 노드는 그 노드의
// 시작(스테이징 폴더를 만든 줄) · 완료(스테이징 폴더를 지운 줄) · 실패(fatal 줄)가 로그에 있을 때만 싣고, 아무 줄도 없는
// 노드는 싣지 않는다. 다른 노드로 넘긴 단계(`n1 -> localhost`)의 결과도 그 노드의 것으로 읽는다. 로그가 비면 둘 다 비어 있다.
func TestRunProgressReadsOnlyWhatTheLogShows(t *testing.T) {
	names := func(p ui.RunProgress) string {
		var out []string
		for _, n := range p.Nodes {
			out = append(out, n.Node+"="+n.State)
		}
		return strings.Join(out, " ")
	}
	st := runAt(ui.RunRunning, runLog())
	p := st.Progress()
	if p.Phase != ui.PhaseOpenSSL || names(p) != "web-gw=started pay-app=failed" {
		t.Fatalf("phase %q, nodes %q", p.Phase, names(p))
	}
	if got := st.PhaseText(ui.KO); got != "지금 단계 3/6: OpenSSL 런타임 관측" {
		t.Errorf("ko phase text: %q", got)
	}
	if got := st.PhaseText(ui.EN); got != "Step 3 of 6 now: observing the OpenSSL runtime" {
		t.Errorf("en phase text: %q", got)
	}
	// 네트워크 단계는 관측 구간을 함께 적는다. 모르는 단계 이름은 단계를 바꾸지 않는다.
	st = runAt(ui.RunRunning, runLog("", "TASK [Observe TLS and SSH handshakes]"+stars, "", "TASK [A task this build does not know]"+stars))
	if got := st.PhaseText(ui.KO); !strings.Contains(got, "5/6") || !strings.Contains(got, "60초") {
		t.Errorf("network phase text: %q", got)
	}
	// 끝까지 간 노드와, 다른 노드로 넘긴 단계.
	st = runAt(ui.RunRunning, runLog("", "TASK [Keep the OpenSSL result]"+stars, "changed: [n1 -> localhost]", "",
		"TASK [Remove the staging directory]"+stars, "changed: [n1]", "changed: [n2]"))
	if p = st.Progress(); p.Phase != ui.PhaseCleanup || names(p) != "web-gw=done pay-app=failed" {
		t.Errorf("after the cleanup task: phase %q, nodes %q", p.Phase, names(p))
	}
	if p = runAt(ui.RunRunning, "").Progress(); p.Phase != "" || len(p.Nodes) != 0 {
		t.Errorf("an empty log gives a phase or nodes: %+v", p)
	}
	if p = runAt(ui.RunRunning, "PLAY [pqcaton observation cleanup]"+stars+"\n\nTASK [Remove the staging directory]"+stars+"\nchanged: [n1]").Progress(); p.Phase != ui.PhaseCleanup || len(p.Nodes) != 0 {
		t.Errorf("the cleanup play is read as nodes finishing: %+v", p)
	}
}

// IC-UI73 — **01은 상세 로그를 펼치지 않고도 지금 무엇을 하는지, 언제 시작했는지, 끝난 뒤 무엇을 할지를 보인다.** 도는
// 동안: 상태 줄에 시작 시각과 시간대, 지금 단계, 근거가 있는 노드의 상태, 중지 버튼이 있고 로그는 접힌 `details` 안에
// 있다. 알림 영역은 요약뿐이고 다시 받는 상자 전체가 아니다. 성공: 02로 가는 링크와 시작 · 종료 시각. 실패 · 중지: 멈춘
// 단계, 시작 · 종료 시각, 정리 결과, 다시 하는 법. 두 말 모두.
func TestRunStatusShowsProgressAboveTheLog(t *testing.T) {
	render := func(l ui.Lang, st ui.RunStatus) string {
		var b strings.Builder
		if err := ui.RenderRunStatus(&b, l, "tok", &st); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	for _, c := range []struct {
		lang                                  ui.Lang
		running, phase, node, stop, log, zone string
		goSurvey, stoppedAt, again, cleaned   string
	}{
		{ui.KO, "관측 중입니다(2026-11-23 07:08:04 UTC 시작)", "지금 단계 3/6: OpenSSL 런타임 관측", "pay-app: 실패", "중지", "상세 로그(", "UTC",
			"02 대조에서 보기", "멈춘 단계: 3/6 OpenSSL 런타임 관측", "다시 실행합니다", "스테이징 폴더를 지웠습니다"},
		{ui.EN, "Observing (started at 2026-11-23 07:08:04 UTC)", "Step 3 of 6 now: observing the OpenSSL runtime", "pay-app: failed", "Stop", "Detailed log (", "UTC",
			"See them in 02 Reconciliation", "Stopped at: 3 of 6, observing the OpenSSL runtime", "run it again", "staging folders on the nodes were removed"},
	} {
		body := render(c.lang, runAt(ui.RunRunning, runLog()))
		for _, want := range []string{c.running, c.phase, c.node, "web-gw: ", ">" + c.stop + "<", c.log,
			`<div class="run-summary" role="status" aria-live="polite">`, `<details class="run-log-box" data-keep-open="run-log">`, `id="run-live" hx-get="/observe/run?lang=` + string(c.lang) + `&amp;part=live&amp;state=running" hx-trigger="every 2s"`, `id="run-log" hx-get=`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s running: no %s", c.lang, want)
			}
		}
		if strings.Contains(body, "pay-db") {
			t.Errorf("%s running: a node with no line in the log is shown", c.lang)
		}
		if strings.Contains(body, "<details class=\"run-log-box\" data-keep-open=\"run-log\" open") || strings.Contains(body[:strings.Index(body, "run-summary")], `role="status"`) {
			t.Errorf("%s running: the log is drawn open, or the whole polled box is a live region", c.lang)
		}
		if summary := body[:strings.Index(body, "<details")]; strings.Contains(summary, "PLAY [") {
			t.Errorf("%s running: log lines show above the folded log", c.lang)
		}

		done := runAt(ui.RunDone, runLog())
		done.Received = 7
		body = render(c.lang, done)
		for _, want := range []string{c.goSurvey, "2026-11-23 07:08:04 UTC", "2026-11-23 07:09:39 UTC"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s done: no %s", c.lang, want)
			}
		}
		if strings.Contains(body, "hx-trigger") || strings.Contains(body, ">"+c.stop+"<") {
			t.Errorf("%s done: a finished run still polls or offers Stop", c.lang)
		}

		for _, state := range []ui.RunState{ui.RunFailed, ui.RunStopped} {
			// 멈춘 뒤에는 정리 플레이가 돈다. 그것은 「멈춘 단계」가 아니다.
			over := runAt(state, runLog("", "PLAY [pqcaton observation cleanup]"+stars, "", "TASK [Remove the staging directory]"+stars, "changed: [n1]"))
			over.Exit, over.NodeCleanup = 2, "done"
			body = render(c.lang, over)
			if strings.Contains(body, "6/6") || strings.Contains(body, "6 of 6") || strings.Contains(body, "web-gw: "+map[ui.Lang]string{ui.KO: "수집 중", ui.EN: "collecting"}[c.lang]) {
				t.Errorf("%s %s: the cleanup after the stop is shown as where it stopped, or an unfinished node as still collecting", c.lang, state)
			}
			if !strings.Contains(body, "web-gw: "+map[ui.Lang]string{ui.KO: "끝나지 않음", ui.EN: "did not finish"}[c.lang]) {
				t.Errorf("%s %s: the node that started and did not finish is not shown as unfinished", c.lang, state)
			}
			for _, want := range []string{c.stoppedAt, c.again, c.cleaned, "2026-11-23 07:09:39 UTC", c.node} {
				if !strings.Contains(body, want) {
					t.Errorf("%s %s: no %s", c.lang, state, want)
				}
			}
			if strings.Contains(body, c.goSurvey) {
				t.Errorf("%s %s: a run that did not finish points at 02", c.lang, state)
			}
		}
	}
}
