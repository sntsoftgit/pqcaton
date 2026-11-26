// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 설정 화면. 화면을 띄울 때 정해진 값과 그 상태를 모아 보인다. **값을 바꾸는 요청은 받지 않는다**(표시 언어는 다른 화면과
// 같은 쿠키로 바뀐다). 비밀은 모으지 않는다: pqcota 연결은 출처만, 키는 자리와 상태만 넘긴다.

import (
	"io"
	"net/http"
	"runtime/debug"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// settingsHref — 설정으로 가는 자리. 복구 · 진단 상태에서는 모든 화면이 그 상태 화면이라 링크를 두지 않는다.
func (s *server) settingsHref() string {
	if s.recovering || len(s.diag) > 0 {
		return ""
	}
	return ui.ScreenSettings
}

// buildVersion — 이 바이너리의 판. 릴리스로 설치한 것은 태그이고, 체크아웃에서 빌드한 것은 그렇게 적힌다.
func buildVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return ""
}

// settingsData — 지금 값. 키는 점검 결과의 자리와 상태만 쓴다(공개 키 · 지문 · 개인 키는 넘기지 않는다).
func (s *server) settingsData(mode ui.Mode) ui.SettingsData {
	d := ui.SettingsData{
		Version: buildVersion(), Addr: s.addr, Mode: mode,
		Org: s.orgName(), OrgFromDecl: s.orgFromDecl, Workspace: s.workspace,
		UpstreamOrg: s.upstreamOrgName, DSNFromEnv: s.dsnFromEnv, DSNReadVar: s.dsnReadVar,
		CollectorKeysDir: s.collectorKeys, CollectorVerify: s.collectorVerify,
		Collectors: s.collectors, CollectorPin: s.collectorPin, AllowUnverified: s.allowUnverified,
	}
	if cfg, err := s.loadDSNs(); err == nil {
		d.IngestOn, d.IngestFromEnv = cfg.ingest != "", cfg.ingestFromEnv
	}
	if s.st != nil {
		d.StateDir = s.st.Dir
	}
	add := func(kind, path, flag string) {
		if path != "" {
			d.Files = append(d.Files, ui.SettingFile{Kind: kind, Path: path, Flag: flag})
		}
	}
	add(ui.FileDecl, s.decl, "-decl")
	for _, l := range s.layers {
		add(ui.FileLayer, l, "-layers")
	}
	add(ui.FileScopeActive, s.scopeAssets, "-scope-assets")
	add(ui.FileScopeOut, s.scopeOut, "-scope-out")
	add(ui.FileScopeSess, s.scope, "-scope")
	add(ui.FileSession, s.path, "<session.json>")
	add(ui.FilePlan, s.planOut, "-plan")
	add(ui.FileJudgments, s.judgments, "-judgments")
	add(ui.FileResults, s.results, "-results")
	add(ui.FileHosts, s.hosts, "-workspace")

	key, _ := s.readKey()
	verify := s.readVerify()
	d.ApprovalKey = ui.KeyCheck{State: key.State, Where: key.Where}
	d.VerifyKey = ui.KeyCheck{State: verify.State, Where: verify.Where}
	home := s.keyHome()
	d.CollectorKeysState = ui.KeyNotGiven
	if s.collectorKeys != "" {
		d.CollectorKeysState = dirProblem(home, s.collectorKeys)
	}
	d.CollectorVerifyState = ui.KeyNotGiven
	if s.collectorVerify != "" {
		d.CollectorVerifyState = readCollectorVerify(home, s.collectorVerify).State
	}
	return d
}

// settings — 설정 화면. 선언이 아직 없어도 열린다(무엇으로 띄웠는지는 그때도 볼 수 있어야 한다).
func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	wd := s.load()
	page := s.nextPage(r, ui.ScreenSettings, "", s.steps(wd))
	v := ui.NewSettingsView(s.settingsData(page.Mode), page)
	html(w, r, func(out io.Writer) error { return ui.RenderSettings(out, v) })
}
