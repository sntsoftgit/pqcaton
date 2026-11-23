// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

func judgeSession() review.Session {
	return review.Session{Scope: "host://local",
		PolicyDecisions: map[string]string{"openssl/libssl": ""},
		Items: []review.Item{
			{ID: "n1/openssl/libssl", Policy: "openssl/libssl", Mandatory: true, State: "UNDECLARED",
				Kind: "REMEDIATION_KIND_PROVIDER_INJECT", Level: "L3", Activate: "systemctl reload nginx", RollbackNote: "note-l3"},
			{ID: "n2/openssl/libssl", Policy: "openssl/libssl", State: "UNDECLARED", Level: "L2", Restart: "kept-restart", RollbackNote: "note-l2"},
			{ID: "n3/openssl/libssl", Policy: "openssl/libssl", State: "UNDECLARED", Level: "L2"},
		},
		Autopass: []review.Item{{ID: "n4/openssl/libcrypto", Policy: "openssl/libcrypto", State: "CONFIRMED", Conf: 0.9}},
	}
}

// rowOf: 그 항목의 줄 하나(여는 태그부터 닫는 태그까지).
func rowOf(t *testing.T, body, attr, id string) string {
	t.Helper()
	m := regexp.MustCompile(`(?s)<tr[^>]*` + attr + `="level:` + regexp.QuoteMeta(id) + `"[^>]*>.*?</tr>`).FindString(body)
	if m == "" {
		t.Fatalf("no row with %s for %s", attr, id)
	}
	return m
}

// IC-UI69 — **05의 활성화 훅은 위임 수준이 L3인 항목에서만 펼쳐진다.** 훅 넷의 줄은 L3가 아니면 `hidden`으로 접히되
// 칸과 값은 양식에 그대로 있다(L3로 돌아오면 다시 보인다). 접힌 채로 값이 있는 항목은 그 사실을 알리는 줄이 보이고,
// 값이 없는 항목과 L3인 항목은 그 줄이 접혀 있다. 되돌림 메모는 L3 전용이 아니라 어느 수준에서든 보인다.
func TestHooksUnfoldOnlyAtL3(t *testing.T) {
	var b strings.Builder
	if err := ui.RenderReview(&b, ui.NewReviewView(judgeSession(), ui.Page{Lang: ui.KO})); err != nil {
		t.Fatal(err)
	}
	body := b.String()
	open := func(row string) bool { return !strings.Contains(row[:strings.Index(row, ">")], " hidden") }
	for _, c := range []struct {
		id          string
		hooks, kept bool
	}{
		{"n1/openssl/libssl", true, false},  // L3
		{"n2/openssl/libssl", false, true},  // L2인데 훅에 적어 둔 값이 있다
		{"n3/openssl/libssl", false, false}, // L2이고 훅이 비었다
		{"n4/openssl/libcrypto", false, false},
	} {
		if got := open(rowOf(t, body, "data-l3-for", c.id)); got != c.hooks {
			t.Errorf("%s: the hooks row is open = %v, want %v", c.id, got, c.hooks)
		}
		if got := open(rowOf(t, body, "data-l3-kept-for", c.id)); got != c.kept {
			t.Errorf("%s: the kept-values row is open = %v, want %v", c.id, got, c.kept)
		}
	}
	for _, want := range []string{
		`name="restart:n2/openssl/libssl" value="kept-restart"`, // 접혀 있어도 값은 양식에 있다
		`name="activate:n1/openssl/libssl" value="systemctl reload nginx"`,
		`name="rollback:n1/openssl/libssl" value="note-l3"`, `name="rollback:n2/openssl/libssl" value="note-l2"`,
		`name="level:n2/openssl/libssl"`, ` data-level`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the review form has no %s", want)
		}
	}
	// 되돌림 메모는 훅 줄 안에 있지 않다(훅 줄과 함께 접히지 않는다).
	if strings.Contains(rowOf(t, body, "data-l3-for", "n2/openssl/libssl"), "rollback:") {
		t.Error("the rollback note folds away with the L3 hooks")
	}
}

// IC-UI70 — **조치 종류는 그 말로 풀어 쓴 이름으로 보이고, 보내는 값은 계약 어휘 그대로다.** 여덟 종류 모두 한국어 ·
// 영어 이름이 있고 짧은 꼴(`config_only`)을 함께 적는다. 영어 화면의 이름에 한글이 없다. 고른 값이 골라진 채로 그려진다.
func TestRemediationKindsAreNamed(t *testing.T) {
	kinds := []string{"CONFIG_ONLY", "PROVIDER_INJECT", "JDK_UPGRADE", "FORK_REPLACE", "PROXY_FRONT", "APP_RECONFIG", "REBUILD", "DECOMMISSION"}
	option := regexp.MustCompile(`<option value="(REMEDIATION_KIND_[A-Z_]+)"( selected)?>([^<]*)</option>`)
	names := map[ui.Lang]map[string]string{}
	for _, l := range []ui.Lang{ui.KO, ui.EN} {
		var b strings.Builder
		if err := ui.RenderReview(&b, ui.NewReviewView(judgeSession(), ui.Page{Lang: l})); err != nil {
			t.Fatal(err)
		}
		names[l] = map[string]string{}
		selected := 0
		for _, m := range option.FindAllStringSubmatch(b.String(), -1) {
			names[l][m[1]] = m[3]
			if m[2] != "" {
				selected++
				if m[1] != "REMEDIATION_KIND_PROVIDER_INJECT" {
					t.Errorf("%s: %s is drawn as selected", l, m[1])
				}
			}
		}
		if selected != 1 {
			t.Errorf("%s: %d kinds drawn as selected, want the one the session holds", l, selected)
		}
		for _, k := range kinds {
			name := names[l]["REMEDIATION_KIND_"+k]
			short := "(" + strings.ToLower(k) + ")"
			if !strings.HasSuffix(name, short) || strings.TrimSpace(strings.TrimSuffix(name, short)) == "" {
				t.Errorf("%s: %s is shown as %q, want a name followed by %s", l, k, name, short)
			}
			if l == ui.EN {
				for _, r := range name {
					if r >= 0xAC00 && r <= 0xD7A3 {
						t.Errorf("en: Korean in the name of %s: %q", k, name)
					}
				}
			}
		}
	}
	for _, k := range kinds {
		if names[ui.KO]["REMEDIATION_KIND_"+k] == names[ui.EN]["REMEDIATION_KIND_"+k] && k != "JDK_UPGRADE" {
			t.Errorf("%s has the same name in both languages: %q", k, names[ui.KO]["REMEDIATION_KIND_"+k])
		}
	}
}

// IC-UI71 — **05의 판정 표는 항목 하나가 tbody 하나이고 칸마다 이름표가 있다.** 좁은 화면에서 항목이 카드 하나로 접히는
// 자리다(읽고 판정하고 저장하기까지 표를 옆으로 밀지 않는다). 표는 `editable judge`이고, 항목 수만큼 `tbody.item`이
// 있으며, 값이 든 칸은 모두 `data-label`로 그 칸의 이름을 든다.
func TestJudgeTableFoldsIntoOneCardPerItem(t *testing.T) {
	var b strings.Builder
	if err := ui.RenderReview(&b, ui.NewReviewView(judgeSession(), ui.Page{Lang: ui.EN})); err != nil {
		t.Fatal(err)
	}
	body := b.String()
	if n := strings.Count(body, `<table class="editable judge" role="table">`); n != 2 {
		t.Fatalf("%d judge tables, want 2 (the policy group and the machine-answered items)", n)
	}
	if n := strings.Count(body, `<tbody role="rowgroup" class="item">`); n != 4 {
		t.Errorf("%d item bodies, want one for each of the 4 items", n)
	}
	for _, label := range []string{"Target", "State", "Confidence", "In plan", "Remediation kind", "Target algorithm", "Delegation level", "Per-item conclusion (exception)"} {
		if !strings.Contains(body, `data-label="`+label+`"`) {
			t.Errorf("no cell is labelled %q", label)
		}
	}
	for _, td := range regexp.MustCompile(`<td\b[^>]*>`).FindAllString(body, -1) {
		if !strings.Contains(td, `role="cell"`) || !strings.Contains(td, `data-label="`) {
			t.Errorf("a cell of the judge table has no role or label: %s", td)
		}
	}
}
