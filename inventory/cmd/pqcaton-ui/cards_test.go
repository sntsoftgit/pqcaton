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
	if len(out) != 5 {
		t.Fatalf("found %d step cards, want 5:\n%s", len(out), body)
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
	if !strings.HasSuffix(first["02"], " | 미확정") || !strings.HasSuffix(first["05"], " | 미확정") {
		t.Errorf("the finalizing steps do not show their state: 02 %q, 05 %q", first["02"], first["05"])
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
		line := cards(t, w.Body.String())["05"]
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
	for _, num := range []string{"01", "03", "04", "05"} {
		if strings.HasPrefix(got[num], "확인할 수 없음") {
			t.Errorf("card %s went unavailable with the scope session", num)
		}
	}
}

// IC-U85 — **카드만 읽는 파일이 모델을 만드는 사이에 바뀌면 다시 만든다.** 양식의 rev는 그 화면
// 입력만 덮으므로 지금 파일의 rev와 같다.
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

}

// IC-U86 — **조회 본문도 스냅샷 안에서 만든다.** 스냅샷이 짝을 지은 뒤에 원장을 망가뜨려도 화면은
// 이미 만든 본문을 그린다. 본문이 정책 · 원장을 묶음 밖에서 다시 읽으면 카드와 다른 시점을 보이고,
// 이 경우에는 500이 된다.
func TestInventoryBodyIsInsideTheSnapshot(t *testing.T) {
	s := everyScreen(t)
	calls := 0
	s.afterSnapshot = func() {
		calls++
		if err := os.WriteFile(s.judgments, []byte("{not json\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w := get(t, s, ui.ScreenInventory+"?lang=ko")
	if calls == 0 {
		t.Fatal("the snapshot hook did not run")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("GET /inventory after the snapshot = %d: the body read the ledger again outside the snapshot\n%s", w.Code, w.Body.String())
	}
}

// IC-U87 — **확정 상태를 정하는 영수증도 스냅샷이 잰다.** 모델을 만든 직후에 영수증만 사라지면(출력은
// 그대로) 모델을 다시 만들고, 카드는 지금 상태(미확정)를 적는다. 영수증을 재지 않으면 카드가 사라진
// 영수증으로 판정한 「확정됨」을 싣고 나간다.
func TestSnapshotCoversTheReceipt(t *testing.T) {
	s, _ := newServer(t)
	if q := location(t, finalizeVia(t, s, "", "/finalize/confirm", signed())); q.Get("problem") != "" {
		t.Fatalf("finalize: %s", q.Get("problem"))
	}
	var receipt string
	for _, in := range s.st.StateInputs(s.reviewID()) {
		if !in.Dir {
			receipt = in.Path
		}
	}
	if _, err := os.Stat(receipt); err != nil {
		t.Fatalf("no receipt after finalizing: %v", err)
	}
	calls := 0
	s.afterModel = func() {
		calls++
		if calls == 1 {
			if err := os.Remove(receipt); err != nil {
				t.Fatal(err)
			}
		}
	}
	body := get(t, s, ui.ScreenReview+"?lang=ko").Body.String()
	if calls != 2 {
		t.Errorf("the model was built %d times, want 2 (the receipt went away under the first)", calls)
	}
	line := cards(t, body)["05"]
	if got := line[strings.LastIndex(line, " | ")+3:]; got != "미확정" {
		t.Errorf("review card = %q, want the state after the receipt went away (미확정)", got)
	}
}
