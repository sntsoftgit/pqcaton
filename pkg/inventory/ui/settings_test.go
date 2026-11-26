// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui_test

import (
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// IC-UI77 — **설정의 줄마다 지금 값 · 바꾸는 법 · 다시 띄워야 하는지가 있고, 표시 언어만 여기서 바꾼다.**
//
// 작업 폴더로 띄웠으면 파일은 「작업 폴더 안의 정해진 이름」이고, 파일마다 플래그로 띄웠으면 그 플래그를 적는다. 키는 자리와
// 상태만 보인다. 저장 상태는 정상이 아닐 때 다음 할 일을 적는다.
func TestSettingsRowsSayHowToChangeThem(t *testing.T) {
	d := ui.SettingsData{
		Version: "v0.33.0", Addr: "127.0.0.1:8765", Mode: ui.ModeWrite, Org: "demo-corp", OrgFromDecl: true, Workspace: "/srv/pqcaton",
		Files:    []ui.SettingFile{{Kind: ui.FileDecl, Path: "/srv/pqcaton/declaration.json", Flag: "-decl"}, {Kind: ui.FileResults, Path: "/srv/pqcaton/results", Flag: "-results"}},
		StateDir: "/srv/pqcaton/.pqcaton", UpstreamOrg: "demo-corp",
		ApprovalKey: ui.KeyCheck{State: ui.KeyOK, Where: "pqcaton/approval.key"}, VerifyKey: ui.KeyCheck{State: ui.KeyNotGiven},
		CollectorKeysDir: "/home/op/.config/pqcaton/collector-keys", CollectorKeysState: ui.KeyOpen, CollectorVerifyState: ui.KeyNotGiven,
		Collectors: "/srv/pqcaton/collectors", CollectorPin: "v0.20.0",
	}
	render := func(d ui.SettingsData, l ui.Lang) (ui.SettingsView, string) {
		v := ui.NewSettingsView(d, ui.Page{Title: "설정", Lang: l, Org: d.Org, SettingsHref: ui.ScreenSettings, Mode: d.Mode})
		var b strings.Builder
		if err := ui.RenderSettings(&b, v); err != nil {
			t.Fatal(err)
		}
		return v, b.String()
	}
	v, body := render(d, ui.KO)
	rows := map[string]ui.SettingRow{}
	for _, g := range v.Groups {
		for _, r := range g.Rows {
			if r.Name == "" || r.How == "" || (r.Value == "" && !r.Live) {
				t.Errorf("the row %q lacks a name, a value or how to change it: %+v", r.ID, r)
			}
			rows[r.ID] = r
		}
	}
	if !rows["lang"].Live || rows["lang"].Restart || !rows["addr"].Restart || rows["mode"].Restart {
		t.Errorf("only the language is changed here: lang %+v, addr %+v, mode %+v", rows["lang"], rows["addr"], rows["mode"])
	}
	for id, want := range map[string]string{
		"file-decl": "작업 폴더 안의 정해진 이름", "org": "선언 파일의 조직", "dsn": "pqcaton/pqcota.env", "approval-key": "쓸 수 있음",
		"verify-key": "없음", "collector-keys": "쓸 수 없음", "collector-pin": "v0.20.0", "allow-unverified": "끔",
	} {
		r := rows[id]
		if !strings.Contains(r.Value+" "+r.State+" "+r.How, want) {
			t.Errorf("the row %q does not say %q: %+v", id, want, r)
		}
	}
	for _, sub := range []string{`id="settings-keys"`, `data-setting-state`, `data-setting-restart`, `data-lang-current="ko"`, `data-lang-switch="en"`,
		`href="` + ui.ScreenSettings + `?lang=en"`, "(없음)"} {
		if !strings.Contains(body, sub) {
			t.Errorf("the settings screen lacks %q", sub)
		}
	}

	// 파일마다 플래그로 띄웠으면 그 플래그를 적고, 조직은 -org다.
	d.Workspace, d.OrgFromDecl = "", false
	v, _ = render(d, ui.EN)
	for _, g := range v.Groups {
		for _, r := range g.Rows {
			switch r.ID {
			case "file-decl":
				if !strings.Contains(r.How, "`-decl`") {
					t.Errorf("without a workspace the declaration row says %q", r.How)
				}
			case "org":
				if !strings.Contains(r.How, "`-org`") {
					t.Errorf("without a workspace the organization row says %q", r.How)
				}
			case "workspace":
				t.Error("a workspace row shows although the screen was started without one")
			}
		}
	}

	// 정상이 아닌 저장 상태는 다음 할 일을 적는다.
	for mode, word := range map[ui.Mode]string{ui.ModeViewOnly: "지원 플랫폼", ui.ModeRecovery: "복구 안내", ui.ModeDiagnostic: "진단"} {
		d.Mode = mode
		if _, b := render(d, ui.KO); !strings.Contains(b, word) {
			t.Errorf("%s: the settings do not say what to do next (%q)", mode, word)
		}
	}
}
