// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"context"
	"io"
)

// 설정 화면. **늘 볼 필요가 없는 것을 여기에 모은다**: 작업 폴더와 파일의 자리, pqcota 연결의 출처, 키와 수집기의 자리와
// 상태, 화면의 판과 주소. 상단에는 조직과 지금 단계만 남긴다. 값 대부분은 화면을 띄울 때 플래그와 파일로 정해지므로 여기서는
// 보이기만 하고, 값마다 무엇으로 바꾸는지와 다시 띄워야 하는지를 적는다. 화면에서 바로 바꾸는 것은 표시 언어뿐이다.
//
// **비밀은 싣지 않는다.** pqcota 연결은 출처만, 키는 자리와 상태만 보인다. 연결 문자열 · 비밀번호 · 키 원문은 이 화면의 모델에
// 들어오지 않는다(SettingsData에 그런 칸이 없다).

// 설정 화면이 보이는 파일의 종류.
const (
	FileDecl        = "decl"
	FileLayer       = "layer"
	FileScopeActive = "scope-active"
	FileScopeOut    = "scope-out"
	FileScopeSess   = "scope-session"
	FileSession     = "session"
	FilePlan        = "plan"
	FileJudgments   = "judgments"
	FileResults     = "results"
	FileHosts       = "hosts"
)

// SettingFile — 화면이 읽고 쓰는 파일이나 폴더 하나. Flag는 작업 폴더 없이 띄울 때 그 자리를 주는 플래그다.
type SettingFile struct {
	Kind, Path, Flag string
}

// SettingsData — 처리기가 모은 지금 값. 비밀은 없다.
type SettingsData struct {
	// Version — 이 바이너리의 판(빌드 정보). Addr — 듣는 주소.
	Version, Addr string
	Mode          Mode
	// Org — 판정 조직. OrgFromDecl이면 선언의 조직을 따른다.
	Org         string
	OrgFromDecl bool
	// Workspace — 작업 폴더로 띄웠으면 그 폴더. 비었으면 파일마다 플래그로 준 것이다.
	Workspace string
	Files     []SettingFile
	StateDir  string
	// UpstreamOrg — pqcota 이력을 연 조직. DSNFromEnv — 연결 정보를 환경 변수에서 읽었는가(아니면 pqcota.env).
	UpstreamOrg string
	DSNFromEnv  bool
	// 키와 수집기: 자리와 상태만.
	ApprovalKey, VerifyKey                   KeyCheck
	CollectorKeysDir, CollectorVerify        string
	CollectorKeysState, CollectorVerifyState KeyState
	Collectors, CollectorPin                 string
	AllowUnverified                          bool
}

// SettingRow — 설정 한 줄. Value는 지금 값, State는 그 상태(키처럼 값에 상태가 따르는 것), How는 무엇으로 바꾸는지다.
// Restart는 바꾼 뒤 화면을 다시 띄워야 하는가. Live면 이 화면에서 바로 바꾼다(표시 언어).
type SettingRow struct {
	ID          string
	Name, Value string
	State       string
	How         string
	Restart     bool
	Live        bool
	// Plain — 값이 경로나 식별자가 아니라 문장이다(저장 상태 · 연결의 출처 · 켬/끔). code로 그리지 않는다.
	Plain bool
}

// SettingGroup — 설정 한 묶음.
type SettingGroup struct {
	ID, Title string
	Rows      []SettingRow
}

// SettingsView — 설정 화면.
type SettingsView struct {
	Page
	Groups []SettingGroup
}

// keyStateLabel — 키 자리의 상태를 짧게. 쓸 수 없는 까닭은 그 키를 쓰는 화면(승인 키는 06, 수집기 키는 03)이 자세히 적으므로
// 여기서는 쓸 수 있는지만 가른다. 키의 내용은 다루지 않는다.
func keyStateLabel(l Lang, st KeyState) string {
	switch st {
	case KeyOK:
		return tSetKeyOK.In(l)
	case KeyNotGiven, KeyMissing:
		return tSetKeyNone.In(l)
	case KeyDisabled:
		return tSetKeyDisabled.In(l)
	}
	return tSetKeyUnusable.In(l)
}

// modeNext — 지금 저장 상태에서 할 일.
func modeNext(l Lang, m Mode) string {
	switch m {
	case ModeViewOnly:
		return tSetModeViewOnlyNext.In(l)
	case ModeRecovery:
		return tSetModeRecoveryNext.In(l)
	case ModeDiagnostic:
		return tSetModeDiagnosticNext.In(l)
	}
	return tSetModeWriteNext.In(l)
}

func fileName(l Lang, kind string) string {
	switch kind {
	case FileDecl:
		return tSetFileDecl.In(l)
	case FileLayer:
		return tSetFileLayer.In(l)
	case FileScopeActive:
		return tSetFileScopeActive.In(l)
	case FileScopeOut:
		return tSetFileScopeOut.In(l)
	case FileScopeSess:
		return tSetFileScopeSess.In(l)
	case FileSession:
		return tSetFileSession.In(l)
	case FilePlan:
		return tSetFilePlan.In(l)
	case FileJudgments:
		return tSetFileJudgments.In(l)
	case FileResults:
		return tSetFileResults.In(l)
	}
	return tSetFileHosts.In(l)
}

// NewSettingsView — 지금 값으로 설정 화면을 세운다.
func NewSettingsView(d SettingsData, page Page) SettingsView {
	l := page.Lang
	v := SettingsView{Page: page}
	none := tSetNone.In(l)
	val := func(s string) string {
		if s == "" {
			return none
		}
		return s
	}

	screen := SettingGroup{ID: "screen", Title: tSetGroupScreen.In(l), Rows: []SettingRow{
		{ID: "lang", Name: tSetLang.In(l), Value: l.Label(), How: tSetLangHow.In(l), Live: true},
		{ID: "mode", Name: tSetMode.In(l), Value: d.Mode.text(l), How: modeNext(l, d.Mode), Plain: true},
		{ID: "version", Name: tSetVersion.In(l), Value: val(d.Version), How: tSetVersionHow.In(l), Restart: true},
		{ID: "addr", Name: tSetAddr.In(l), Value: val(d.Addr), How: tSetAddrHow.In(l), Restart: true},
	}}

	orgHow := tSetOrgHowFlag.In(l)
	if d.OrgFromDecl {
		orgHow = tSetOrgHowDecl.In(l)
	}
	work := SettingGroup{ID: "workspace", Title: tSetGroupWork.In(l), Rows: []SettingRow{
		{ID: "org", Name: tSetOrg.In(l), Value: val(d.Org), How: orgHow, Restart: true},
	}}
	if d.Workspace != "" {
		work.Rows = append(work.Rows, SettingRow{ID: "workspace", Name: tSetWorkspace.In(l), Value: d.Workspace, How: tSetWorkspaceHow.In(l), Restart: true})
	}
	for _, f := range d.Files {
		how := tSetFileHowWorkspace.In(l)
		if d.Workspace == "" {
			how = tSetFileHowFlag.F(l, f.Flag)
		}
		work.Rows = append(work.Rows, SettingRow{ID: "file-" + f.Kind, Name: fileName(l, f.Kind), Value: val(f.Path), How: how, Restart: true})
	}
	work.Rows = append(work.Rows, SettingRow{ID: "state-dir", Name: tSetStateDir.In(l), Value: val(d.StateDir), How: tSetStateDirHow.In(l), Restart: true})

	src := tSetDSNFile.In(l)
	if d.DSNFromEnv {
		src = tSetDSNEnv.In(l)
	}
	history := SettingGroup{ID: "history", Title: tSetGroupHistory.In(l), Rows: []SettingRow{
		{ID: "upstream-org", Name: tSetUpstreamOrg.In(l), Value: val(d.UpstreamOrg), How: tSetUpstreamOrgHow.In(l), Restart: true},
		{ID: "dsn", Name: tSetDSN.In(l), Value: src, How: tSetDSNHow.In(l), Restart: true, Plain: true},
	}}

	keys := SettingGroup{ID: "keys", Title: tSetGroupKeys.In(l), Rows: []SettingRow{
		{ID: "approval-key", Name: tSetApprovalKey.In(l), Value: val(d.ApprovalKey.Where), State: keyStateLabel(l, d.ApprovalKey.State), How: tSetApprovalKeyHow.In(l), Restart: true},
		{ID: "verify-key", Name: tSetVerifyKey.In(l), Value: val(d.VerifyKey.Where), State: keyStateLabel(l, d.VerifyKey.State), How: tSetVerifyKeyHow.In(l), Restart: true},
		{ID: "collector-keys", Name: tSetCollectorKeys.In(l), Value: val(d.CollectorKeysDir), State: keyStateLabel(l, d.CollectorKeysState), How: tSetCollectorKeysHow.In(l), Restart: true},
		{ID: "collector-verify", Name: tSetCollectorVerify.In(l), Value: val(d.CollectorVerify), State: keyStateLabel(l, d.CollectorVerifyState), How: tSetCollectorVerifyHow.In(l), Restart: true},
	}}

	allow := tSetNo.In(l)
	if d.AllowUnverified {
		allow = tSetYes.In(l)
	}
	collectors := SettingGroup{ID: "collectors", Title: tSetGroupCollectors.In(l), Rows: []SettingRow{
		{ID: "collectors", Name: tSetCollectors.In(l), Value: val(d.Collectors), How: tSetCollectorsHow.In(l), Restart: true},
		{ID: "collector-pin", Name: tSetCollectorPin.In(l), Value: val(d.CollectorPin), How: tSetCollectorPinHow.In(l), Restart: true},
		{ID: "allow-unverified", Name: tSetAllowUnverified.In(l), Value: allow, How: tSetAllowUnverifiedHow.In(l), Restart: true, Plain: true},
	}}

	v.Groups = []SettingGroup{screen, work, history, keys, collectors}
	return v
}

// RenderSettings — 화면을 그린다.
func RenderSettings(w io.Writer, v SettingsView) error {
	return settingsPage(v).Render(context.Background(), w)
}
