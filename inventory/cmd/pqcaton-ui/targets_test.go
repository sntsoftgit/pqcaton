// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// targetState: 02의 「대상 노드의 접속 준비」에서 그 노드 줄의 상태 코드. 줄이 없으면 빈 문자열이다.
func targetState(page, node string) string {
	i := strings.Index(page, `data-target-node="`+node+`" data-target-state="`)
	if i < 0 {
		return ""
	}
	rest := page[i+len(`data-target-node="`+node+`" data-target-state="`):]
	return rest[:strings.Index(rest, `"`)]
}

// runsStarted: 관측 실행 자리의 수. 거절된 시작 요청은 자리를 만들지 않는다(노드에 접속하지 않았다).
func runsStarted(t *testing.T, s *server) int {
	t.Helper()
	ents, err := os.ReadDir(s.runsDir())
	if err != nil {
		return 0
	}
	return len(ents)
}

// IC-U256 — **관측 대상은 준비된 선언 노드이고, 접근 정보의 노드 ID는 지금 선언에 있어야 한다.** 줄은 선언의 노드다.
//
//   - 밖에서 만든 hosts.csv의 ID가 모두 선언에 있으면 다시 저장하지 않고 관측을 시작할 수 있고, 파일은 바이트까지 그대로다.
//   - 선언에는 있고 접근 정보가 없는 노드는 「접속 정보 미입력」 줄로 남고, 시작 양식이 대상과 빠지는 노드를 갈라 적는다.
//     준비된 노드만으로 관측한다.
//   - 접근 정보에 선언에 없는 ID가 있으면 그 행과 ID가 따로 보이고 시작 전 점검이 막는다. 직접 보낸 시작 요청은 409
//     `run-hosts-undeclared`이고 실행 자리가 생기지 않는다(노드에 접속하지 않았다). 대소문자만 다른 ID도, `observed_as`에 적힌
//     ID도 선언의 노드로 치지 않는다.
//   - 화면을 연 뒤 선언에서 노드 이름을 바꾸면, 예전 화면이 보낸 시작 요청은 지금 선언으로 다시 검사돼 같은 까닭으로 막힌다.
//     어느 경우에도 hosts.csv는 바이트까지 그대로다.
func TestObservationTargetsAreTheDeclaredNodes(t *testing.T) {
	t.Run("an external hosts.csv that matches runs without saving", func(t *testing.T) {
		s := runReady(t, "ok")
		before := readFile(t, s.hosts)
		page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
		if targetState(page, "web") != ui.TargetReady || targetState(page, "db") != ui.TargetReady || strings.Contains(page, `data-targets="orphans"`) {
			t.Fatalf("the matching rows are not ready, or an orphan is shown:\n%s", section(page, "observe-targets"))
		}
		// 선언에만 있는 노드(web-gw · app)는 줄로 남고, 시작 양식이 대상과 빠지는 노드를 갈라 적는다.
		if targetState(page, "app") != ui.TargetUnready || targetState(page, "web-gw") != ui.TargetUnready ||
			!strings.Contains(page, "이번 관측의 대상 2곳: web, db") || !strings.Contains(page, "이번에 빠지는 노드 2곳: web-gw, app") ||
			!strings.Contains(page, "접속 정보 미입력") {
			t.Errorf("the nodes without access details are not shown as left out:\n%s", section(page, "observe-targets"))
		}
		if c := checkOf(s.preflight(), ui.RunHostsDeclared); c == nil || !c.OK {
			t.Fatalf("the declared check = %+v", c)
		}
		if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
			t.Fatalf("start = %d\n%s", w.Code, w.Body.String())
		}
		if st := waitRun(t, s); st.State != ui.RunDone {
			t.Fatalf("state %s\n%s", st.State, st.Log)
		}
		if readFile(t, s.hosts) != before {
			t.Error("hosts.csv changed although nothing was saved")
		}
		page = get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
		if targetState(page, "web") != ui.TargetObserved || targetState(page, "app") != ui.TargetUnready || !strings.Contains(page, "관측됨: 지금 결과는") {
			t.Errorf("after the run the observed and the left-out nodes:\n%s", section(page, "observe-targets"))
		}
	})

	for name, row := range map[string]string{
		"an ID that is not declared":      "db-old,Old,10.0.0.77,22,deploy,,,linux,ssh\n",
		"an ID that differs only in case": "App,App,10.0.0.5,22,deploy,,,linux,ssh\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := runReady(t, "ok")
			before := readFile(t, s.hosts) + row
			if err := os.WriteFile(s.hosts, []byte(before), 0o600); err != nil {
				t.Fatal(err)
			}
			id := row[:strings.Index(row, ",")]
			page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
			// 줄 번호는 파일의 줄이다(머리글이 1행, 그 뒤 두 행, 넷째 줄이 이 행).
			if !strings.Contains(page, `data-orphan-row="4" data-orphan-id="`+id+`"`) || strings.Contains(page, `action="`+ui.RunPath+`"`) ||
				!strings.Contains(page, "접속 정보 4행: 선언에 없는 노드 <code>"+id+"</code>") ||
				!strings.Contains(page, "선언에 없는 행 1개") {
				t.Fatalf("the row is not shown as outside the declaration, or the start form is still there:\n%s", section(page, "observe-targets"))
			}
			if c := checkOf(s.preflight(), ui.RunHostsDeclared); c == nil || c.OK || !c.Blocking || !strings.Contains(c.Detail, "line 4: "+id) {
				t.Errorf("the declared check = %+v", c)
			}
			w := postFormLang(t, s, ui.RunPath, "ko", url.Values{})
			if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedRunHostsUndeclared) ||
				!strings.Contains(w.Body.String(), "노드에는 접속하지 않았고 파일은 그대로입니다") {
				t.Fatalf("start = %d, want 409 %s\n%s", w.Code, ui.RefusedRunHostsUndeclared, w.Body.String())
			}
			if n := runsStarted(t, s); n != 0 {
				t.Errorf("%d run directories exist after a refused start", n)
			}
			if readFile(t, s.hosts) != before {
				t.Error("hosts.csv changed")
			}
		})
	}

	t.Run("observed_as does not tie access details to a node", func(t *testing.T) {
		s := runReady(t, "ok")
		d := strings.Replace(readFile(t, s.decl), `{"name":"db","ips":["10.0.0.9"]}`, `{"name":"db-01","ips":["10.0.0.9"],"observed_as":["db"]}`, 1)
		if err := os.WriteFile(s.decl, []byte(d), 0o644); err != nil {
			t.Fatal(err)
		}
		page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
		if !strings.Contains(page, `data-orphan-id="db"`) || targetState(page, "db-01") != ui.TargetUnready {
			t.Errorf("the row db was tied to db-01 through observed_as or the IP:\n%s", section(page, "observe-targets"))
		}
	})

	t.Run("a node renamed after the screen was drawn", func(t *testing.T) {
		s := runReady(t, "ok")
		before := readFile(t, s.hosts)
		page := get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
		if !strings.Contains(page, `action="`+ui.RunPath+`"`) {
			t.Fatal("no start form before the rename")
		}
		// 화면을 연 뒤 선언에서 web을 web-01로 바꿨다. 접근 정보에는 옛 ID가 남는다.
		d := strings.Replace(readFile(t, s.decl), `{"name":"web","ips":["10.0.0.2"]}`, `{"name":"web-01","ips":["10.0.0.2"]}`, 1)
		if err := os.WriteFile(s.decl, []byte(d), 0o644); err != nil {
			t.Fatal(err)
		}
		// 예전 화면이 보내는 그대로의 시작 요청.
		w := postFormLang(t, s, ui.RunPath, "ko", url.Values{"window": {"7"}})
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedRunHostsUndeclared) {
			t.Fatalf("start from the earlier screen = %d, want 409 %s\n%s", w.Code, ui.RefusedRunHostsUndeclared, w.Body.String())
		}
		if n := runsStarted(t, s); n != 0 {
			t.Errorf("%d run directories exist: remote work may have begun", n)
		}
		if readFile(t, s.hosts) != before {
			t.Error("hosts.csv changed")
		}
		// 처리기를 거치지 않고 실행을 띄워도, 인벤토리를 쓰기 직전의 검사가 막는다(아무것도 실행하지 않은 「실패」).
		if err := s.startRun(5); err != nil {
			t.Fatal(err)
		}
		if st := waitRun(t, s); st.State != ui.RunFailed || !strings.Contains(st.Log, "not in the declaration") || strings.Contains(st.Log, "PLAY RECAP") {
			t.Errorf("a run started past the handler: state %s\n%s", st.State, st.Log)
		}
		page = get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
		if !strings.Contains(page, `data-orphan-id="web"`) || targetState(page, "web-01") != ui.TargetUnready {
			t.Errorf("after the rename: want the old row outside the declaration and the new name without access:\n%s", section(page, "observe-targets"))
		}
		if readFile(t, s.hosts) != before {
			t.Error("hosts.csv changed")
		}
	})
}

// IC-U257 — **대조는 어느 결과로 했는지를 밝힌다.** 성공한 관측이 한 번도 없으면 03이 「대조할 성공 관측 결과가 아직
// 없습니다」를 먼저 보이고, 선언 자산의 미관측이 「찾지 못했다」가 아니라고 적으며, 실패한 실행의 시각을 적는다. 성공한 관측
// 뒤에는 그 결과의 시각과 노드 수를 적고, 선언에는 있고 그 결과에는 없는 노드를 이름으로 적으며 그 노드의 미관측 자산에
// 표시를 단다. 그 뒤 실행이 실패하면 결과는 그대로이고, 대조에 쓴 결과의 시각과 실패한 실행의 시각이 따로 보인다.
func TestReconciliationSaysWhichResultsItUsed(t *testing.T) {
	s := runReady(t, "fail")
	if err := os.Remove(filepath.Join(s.results, oldResult)); err != nil {
		t.Fatal(err)
	}
	run := func(mode string, want ui.RunState) {
		t.Helper()
		t.Setenv("FAKE_MODE", mode)
		if w := postForm(t, s, ui.RunPath, url.Values{}); w.Code != http.StatusSeeOther {
			t.Fatalf("%s: start = %d\n%s", mode, w.Code, w.Body.String())
		}
		if st := waitRun(t, s); st.State != want {
			t.Fatalf("%s: state %s\n%s", mode, st.State, st.Log)
		}
	}
	survey := func() string { return get(t, s, ui.ScreenSurvey+"?lang=ko").Body.String() }

	run("fail", ui.RunFailed)
	page := survey()
	for _, want := range []string{"대조할 성공 관측 결과가 아직 없습니다", `data-survey-source="empty"`, "성공한 관측에서 찾지 못했다는 뜻이 아닙니다",
		"data-survey-last-failed", "「실패」로 끝나 결과를 바꾸지 않았습니다"} {
		if !strings.Contains(page, want) {
			t.Errorf("with no successful observation yet 03 lacks %q", want)
		}
	}
	if !strings.Contains(page, "<code>web-gw</code>") {
		t.Error("the declared assets are not shown while there is no result")
	}

	run("ok", ui.RunDone)
	page = survey()
	for _, want := range []string{`data-survey-source="run"`, "data-survey-source-at", "에 끝난 관측의 결과로 했습니다(노드 2곳)",
		"대조에 쓴 결과에 없는 선언 노드 2곳: web-gw, app", "data-node-absent"} {
		if !strings.Contains(page, want) {
			t.Errorf("after a successful observation 03 lacks %q", want)
		}
	}
	if strings.Contains(page, "data-survey-last-failed") || strings.Contains(page, "대조할 성공 관측 결과가 아직 없습니다") {
		t.Error("03 still reports a failure or no result after a successful observation")
	}

	run("fail", ui.RunFailed)
	page = survey()
	if !strings.Contains(page, "data-survey-source-at") || !strings.Contains(page, "data-survey-last-failed") {
		t.Errorf("after a later failure 03 should show both the time of the results used and the time of the failed run")
	}
}
