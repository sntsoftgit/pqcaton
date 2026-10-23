// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

var stepState = regexp.MustCompile(`<span class="step-num">(\d\d)</span>\s*<span class="step-title">[^<]*</span>\s*<span class="step-state">([^<]*)</span>\s*(?:<span class="step-final">([^<]*)</span>)?`)

// cards: 단계 카드의 번호 → 상태 줄(확정 상태가 있으면 「상태 | 확정 상태」).
func cards(t *testing.T, body string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range stepState.FindAllStringSubmatch(body, -1) {
		out[m[1]] = m[2]
		if m[3] != "" {
			out[m[1]] += " | " + m[3]
		}
	}
	if len(out) != 4 {
		t.Fatalf("found %d step cards, want 4:\n%s", len(out), body)
	}
	return out
}

// IC-U82 — **네 카드를 모두 채우고, 어느 화면에서 보든 같다.** 본문과 카드가 같은 입력을 쓰므로
// 판정 화면의 「04」와 선언 화면의 「04」가 같다. 「확인할 수 없음」은 읽지 못했을 때만이다.
func TestEveryCardIsFilledTheSameWay(t *testing.T) {
	s := everyScreen(t)
	var first map[string]string
	for _, sc := range ui.AllScreens() {
		got := cards(t, get(t, s, sc+"?lang=ko").Body.String())
		for num, line := range got {
			if strings.HasPrefix(line, "확인할 수 없음") {
				t.Errorf("%s: card %s is unavailable", sc, num)
			}
		}
		if first == nil {
			first = got
			continue
		}
		for num := range first {
			if got[num] != first[num] {
				t.Errorf("%s: card %s = %q, on %s it was %q", sc, num, got[num], ui.ScreenDecl, first[num])
			}
		}
	}
	if !strings.HasSuffix(first["02"], " | 미확정") || !strings.HasSuffix(first["04"], " | 미확정") {
		t.Errorf("the finalizing steps do not show their state: 02 %q, 04 %q", first["02"], first["04"])
	}
}

// IC-U83 — **판정 카드의 확정 상태는 출력을 견준 결과다.** 확정 전 미확정, 확정 뒤 확정됨, 계획을 손으로
// 고치면 복구 필요다. 다른 요청이 대상 잠금을 쥐고 있으면 기다리다 「확인할 수 없음」이고 화면은 선다.
func TestReviewCardShowsTheVerifiedState(t *testing.T) {
	s, _ := newServer(t)
	s.wait = 50 * time.Millisecond
	final := func() string {
		w := get(t, s, ui.ScreenReview+"?lang=ko")
		if w.Code != http.StatusOK {
			t.Fatalf("GET /review = %d", w.Code)
		}
		line := cards(t, w.Body.String())["04"]
		return line[strings.LastIndex(line, " | ")+3:]
	}
	if got := final(); got != "미확정" {
		t.Fatalf("before finalizing: %q", got)
	}
	if q := location(t, finalizeVia(t, s, "", "/finalize/confirm", signed())); q.Get("problem") != "" {
		t.Fatalf("finalize: %s", q.Get("problem"))
	}
	if got := final(); got != "확정됨" {
		t.Errorf("after finalizing: %q", got)
	}

	release, err := s.st.LockTargets([]string{filepath.Dir(s.planOut)}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := final(); got != "확인할 수 없음" {
		t.Errorf("while another request holds the lock: %q", got)
	}
	release()

	if err := os.WriteFile(s.planOut, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := final(); got != "복구 필요" {
		t.Errorf("plan edited by hand: %q", got)
	}
}

// IC-U84 — **한 단계의 입력을 읽지 못하면 그 카드만 「확인할 수 없음」이다.** 다른 화면은 그대로 선다.
func TestOneBrokenInputDimsOneCard(t *testing.T) {
	s := everyScreen(t)
	if err := os.WriteFile(s.scope, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := get(t, s, ui.ScreenReview+"?lang=ko")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /review with a broken scope session = %d", w.Code)
	}
	got := cards(t, w.Body.String())
	if !strings.HasPrefix(got["02"], "확인할 수 없음") {
		t.Errorf("scope card = %q, want unavailable", got["02"])
	}
	for _, num := range []string{"01", "03", "04"} {
		if strings.HasPrefix(got[num], "확인할 수 없음") {
			t.Errorf("card %s went unavailable with the scope session", num)
		}
	}
}

// IC-U85 — **카드만 읽는 파일이 모델을 만드는 사이에 바뀌면 다시 만든다.** 양식의 rev는 그 화면
// 입력만 덮으므로 지금 파일의 rev와 같다. 옛 화면(카드 없음)은 카드만 읽는 파일을 재지 않는다.
func TestSnapshotCoversCardInputs(t *testing.T) {
	s, dir := newServer(t)
	s.judgments = filepath.Join(dir, "judgments.jsonl")
	calls := 0
	s.afterModel = func() {
		calls++
		if calls == 1 {
			if err := os.WriteFile(s.judgments, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	body := get(t, s, ui.ScreenReview).Body.String()
	if calls != 2 {
		t.Errorf("the model was built %d times, want 2 (the ledger changed under the first)", calls)
	}
	if got := hiddenFields(body).Get(ui.RevField); got != s.revFor(ui.ScreenReview) {
		t.Errorf("rev = %q, want the rev of the review inputs", got)
	}

	calls = 0
	s.afterModel = func() {
		calls++
		os.WriteFile(s.judgments, []byte(strings.Repeat("{}\n", calls+1)), 0o600)
	}
	get(t, s, ui.Classic(ui.ScreenReview))
	if calls != 1 {
		t.Errorf("the old screen built its model %d times, want 1", calls)
	}
}
