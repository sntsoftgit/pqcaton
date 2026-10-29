// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 06 승인 · 생성 화면. 서명한 계획과 플레이북은 plan.json 옆에 쓴다(작업 폴더 모드면 작업 폴더다).
//
// **서명 · 확인 · 생성은 상류 함수다.** pqcota-approve와 pqcota-provision이 하는 일을 같은 함수로 한다
// (provisioning.PrepareApproval · sign.SignApproval · sign.VerifyApprovals · provisioning.Generate…Playbook).
// 그래서 화면이 서명한 계획은 상류 명령이 그대로 확인하고, 화면이 만든 플레이북은 상류 명령이 만드는 것과 같다.
//
// **승인 키는 작업 폴더 밖에 둔다.** `-approval-key`로 받은 파일을 서명할 때마다 다시 읽고, 소유자 전용이
// 아니거나 작업 폴더 안에 있으면 쓰지 않는다. 작업 폴더는 통째로 백업하는 자리라, 거기 두면 키가 함께 퍼진다.
//
// 서명한 계획은 `plan.signed.json`에 따로 쓴다. plan.json을 덮으면 무엇에 서명했는지 되짚을 수 없다(상류와 같다).

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/sign"
	"github.com/randyinthedev-hash/pqcota-provisioning/pkg/provisioning"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const (
	kindApprove   = "approve"
	kindProvision = "provision"
	kindArchive   = "archive"
)

// 06 화면이 쓰는 자리. 계획 파일 옆이고 이름은 계획 파일을 따른다(plan.json → plan.signed.json · plan.provision.yml ·
// plan.rollback.yml). 명시 모드에서 한 폴더에 계획을 여럿 두어도 서로 덮지 않는다. 계획 출력이 없으면 모두 비어 있다.
func (s *server) signedPath() string    { return s.besidePlan(".signed.json") }
func (s *server) provisionPath() string { return s.besidePlan(".provision.yml") }
func (s *server) rollbackPath() string  { return s.besidePlan(".rollback.yml") }

func (s *server) besidePlan(suffix string) string {
	if s.planOut == "" {
		return ""
	}
	return strings.TrimSuffix(s.planOut, filepath.Ext(s.planOut)) + suffix
}

// approveInputs: 06 화면이 그려질 때 읽는 것. 서명 · 생성 · 보관 양식의 rev가 덮는다. **승인 키도 넣는다** — 화면이
// 보인 키와 서명하는 키가 다르면(그 사이 파일이 바뀌면) 오래된 화면으로 거절한다.
func (s *server) approveInputs() []store.Input {
	in := []store.Input{{Name: "plan", Path: s.planOut}, {Name: "signed", Path: s.signedPath()},
		{Name: "provision", Path: s.provisionPath()}, {Name: "rollback", Path: s.rollbackPath()}}
	if s.approvalKey != "" {
		in = append(in, store.Input{Name: "approval-key", Path: s.approvalKey})
	}
	return in
}

// keyHome: 승인 키가 있어서는 안 되는 폴더. 작업 폴더(명시 모드면 계획이 놓이는 폴더)다.
func (s *server) keyHome() string {
	if s.workspace != "" {
		return s.workspace
	}
	return filepath.Dir(s.planOut)
}

// inside: p가 dir 안에 있는가. 둘 다 실제 경로(링크를 푼 것)로 견준다.
func inside(dir, p string) bool {
	rd, err := filepath.EvalSymlinks(dir)
	if err != nil {
		rd = dir
	}
	rp, err := filepath.EvalSymlinks(p)
	if err != nil {
		rp = p
	}
	rel, err := filepath.Rel(rd, rp)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// readKey: 승인 키를 점검하고 쓸 만하면 개인 키(base64)도 돌려준다.
//
// **링크를 따라가지 않는다.** 키 파일 자체가 심볼릭 링크면 거부하고(Lstat), 하드 링크가 하나 더 있거나(그 이름으로
// 다른 곳에서 바뀔 수 있다) 이 사용자의 것이 아니면 거부한다. 「작업 폴더 밖」은 문자열이 아니라 실제 경로로 본다
// (작업 폴더를 가리키는 링크 폴더를 거쳐도 걸린다).
func (s *server) readKey() (ui.KeyCheck, string) {
	k := ui.KeyCheck{Path: s.approvalKey}
	if s.approvalKey == "" {
		k.State = ui.KeyNotGiven
		return k, ""
	}
	fi, err := os.Lstat(s.approvalKey)
	switch {
	case err != nil:
		k.State = ui.KeyUnreadable
		return k, ""
	case fi.Mode()&fs.ModeSymlink != 0:
		k.State = ui.KeyLink
		return k, ""
	case !fi.Mode().IsRegular():
		k.State = ui.KeyUnreadable
		return k, ""
	}
	if home := s.keyHome(); home != "" && inside(home, s.approvalKey) {
		k.State = ui.KeyInside
		return k, ""
	}
	if !ownedByMe(fi) {
		k.State = ui.KeyOwner
		return k, ""
	}
	if links(fi) > 1 {
		k.State = ui.KeyLinks
		return k, ""
	}
	if fi.Mode().Perm()&0o077 != 0 {
		k.State = ui.KeyOpen
		return k, ""
	}
	b, err := os.ReadFile(s.approvalKey)
	if err != nil {
		k.State = ui.KeyUnreadable
		return k, ""
	}
	priv := privateKeyIn(b)
	raw, err := base64.StdEncoding.DecodeString(priv)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		k.State = ui.KeyBad
		return k, ""
	}
	k.State = ui.KeyOK
	k.Public = base64.StdEncoding.EncodeToString(ed25519.PrivateKey(raw).Public().(ed25519.PublicKey))
	k.Fingerprint = keyFingerprint(k.Public)
	return k, priv
}

// keyFingerprint: 공개 키의 지문(SHA256, base64). 확인 페이지가 보이고 확인 토큰이 덮는다.
func keyFingerprint(public string) string {
	sum := sha256.Sum256([]byte(public))
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// privateKeyIn: pqcota-keygen의 출력(`PQCOTA_SIGN_KEY=…` 줄)이면 그 값을, 아니면 주석과 빈 줄을 뺀 첫 줄을 키로 본다.
func privateKeyIn(b []byte) string {
	var first string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, "PQCOTA_SIGN_KEY="); ok {
			return strings.TrimSpace(v)
		}
		if first == "" && line != "" && !strings.HasPrefix(line, "#") {
			first = line
		}
	}
	return first
}

// loadPlanFile: protojson으로 쓴 확정 계획. 파일이 없으면 nil이다.
func loadPlanFile(p string) (*provisioningv1.FinalizedPlan, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	plan := &provisioningv1.FinalizedPlan{}
	if err := protojson.Unmarshal(b, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func planSummary(p *provisioningv1.FinalizedPlan) *ui.PlanSummary {
	out := &ui.PlanSummary{ID: p.GetId(), Status: p.GetStatus().String()}
	for _, a := range p.GetActions() {
		out.Actions = append(out.Actions, ui.PlanAction{ID: a.GetId(), Node: a.GetTargetNodeId(), Kind: a.GetKind().String(),
			Provider: a.GetProviderChoice(), Target: a.GetTargetAlgorithm(), Level: a.GetAutomationLevel().String()})
	}
	return out
}

// sameContent: 서명한 계획이 지금 계획에 서명한 것인가. 승인이 바꾸는 셋(상태 · 확정 시각 · 서명)만 빼고 견준다.
func sameContent(plan, signed *provisioningv1.FinalizedPlan) bool {
	a, b := proto.Clone(plan).(*provisioningv1.FinalizedPlan), proto.Clone(signed).(*provisioningv1.FinalizedPlan)
	for _, p := range []*provisioningv1.FinalizedPlan{a, b} {
		p.Status, p.FinalizedAt, p.ApprovalSignatures = provisioningv1.PlanStatus_PLAN_STATUS_UNSPECIFIED, nil, nil
	}
	return proto.Equal(a, b)
}

// verifyWith: 서명한 계획의 승인을 이 키 하나로 확인한다. 승인자 이름은 서명 문자열 앞머리에 있다.
func verifyWith(public string, signed *provisioningv1.FinalizedPlan) sign.ApprovalCheck {
	keys := map[string]string{}
	for _, sig := range signed.GetApprovalSignatures() {
		if name, _, ok := strings.Cut(sig, ":"); ok && name != "" {
			keys[name] = public
		}
	}
	return sign.VerifyApprovals(keys, signed)
}

// approveModel: 06 화면 모델. 카드도 같은 것을 쓴다.
func (s *server) approveModel(page ui.Page) (ui.ApproveView, error) {
	v := ui.ApproveView{Page: page, Writable: s.st != nil && !s.recovering && len(s.diag) == 0}
	v.Key, _ = s.readKey()
	v.Signed.State = ui.SignNone
	plan, err := loadPlanFile(s.planOut)
	if err != nil {
		return v, err
	}
	if plan == nil {
		return v, nil
	}
	v.Plan = planSummary(plan)
	signed, err := loadPlanFile(s.signedPath())
	if err != nil {
		v.Signed.State = ui.SignInvalid
		return v, nil
	}
	if signed == nil {
		return v, nil
	}
	if t := signed.GetFinalizedAt(); t != nil {
		v.Signed.At = t.AsTime().Local()
	}
	for _, sig := range signed.GetApprovalSignatures() {
		if name, _, ok := strings.Cut(sig, ":"); ok && v.Approver == "" {
			v.Approver = name
		}
	}
	switch {
	case !sameContent(plan, signed):
		v.Signed.State = ui.SignStale
	case v.Key.State != ui.KeyOK:
		// 확인할 키가 없으면 맞는지 말할 수 없다. 서명은 있지만 확인하지 못한 것으로 둔다.
		v.Signed.State = ui.SignInvalid
	default:
		chk := verifyWith(v.Key.Public, signed)
		if len(chk.Rejected) > 0 || len(chk.Approved) == 0 {
			v.Signed.State = ui.SignInvalid
		} else {
			v.Signed.State, v.Signed.Approvers = ui.SignValid, chk.Approved
		}
	}
	v.Playbooks = s.playbookState(signed)
	return v, nil
}

var levelLine = regexp.MustCompile(`(?m)^# default level: (L[123])\b`)

// playbookState: 만든 플레이북. 빈 곳은 서명한 계획과 파일 머리의 기본 수준으로 다시 센다.
func (s *server) playbookState(signed *provisioningv1.FinalizedPlan) ui.PlaybookState {
	var st ui.PlaybookState
	fi, err := os.Stat(s.provisionPath())
	if err != nil {
		return st
	}
	b, err := os.ReadFile(s.provisionPath())
	if err != nil {
		return st
	}
	st.Made, st.At = true, fi.ModTime()
	st.Files = []string{filepath.Base(s.provisionPath()), filepath.Base(s.rollbackPath())}
	level := provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L2_STAGE_INSTALL
	if m := levelLine.FindSubmatch(b); m != nil {
		st.Level = string(m[1])
		level = levels[strings.ToLower(st.Level)]
	}
	if si, err := os.Stat(s.signedPath()); err == nil && si.ModTime().After(fi.ModTime()) {
		st.Stale = true
	}
	st.Warnings = genWarnings(signed, level)
	return st
}

var levels = map[string]provisioningv1.DeployAutomationLevel{
	"l1": provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L1_STAGE_ONLY,
	"l2": provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L2_STAGE_INSTALL,
	"l3": provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L3_FULL_AUTO,
}

// genWarnings: 생성기가 짚는 빈 곳. pqcota-provision이 종료 상태 3을 내며 세는 것과 같은 것을 센다
// (provider 자리 경고는 세지 않는 것까지 같다).
func genWarnings(plan *provisioningv1.FinalizedPlan, level provisioningv1.DeployAutomationLevel) []string {
	var out []string
	out = append(out, provisioning.ProviderClassWarnings(plan)...)
	out = append(out, provisioning.ConfigConflictWarnings(plan)...)
	out = append(out, provisioning.TargetAlgorithmWarnings(plan)...)
	out = append(out, provisioning.ActivationWarnings(plan, level)...)
	out = append(out, provisioning.AutomationLevelWarnings(plan)...)
	out = append(out, provisioning.TraceabilityWarnings(plan)...)
	for _, a := range plan.GetActions() {
		for _, r := range provisioning.Unresolved(provisioning.ResolveAction(nil, plan, a)) {
			out = append(out, fmt.Sprintf("action %s (node=%s): %s", a.GetId(), a.GetTargetNodeId(), r.GetReason()))
		}
	}
	return out
}

func (s *server) approve(w http.ResponseWriter, r *http.Request) {
	if s.planOut == "" {
		notFound(w, r)
		return
	}
	var v ui.ApproveView
	rev, err := s.snapshotScreen(ui.ScreenApprove, func() error {
		var err error
		v, err = s.approveModel(ui.Page{})
		return err
	})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	v.Page = s.nextPage(r, ui.ScreenApprove, s.planOut, s.steps(s.load()))
	v.Page.Rev = rev
	html(w, r, func(out io.Writer) error { return ui.RenderApprove(out, v) })
}

// approverName: 승인자 이름. 서명 문자열의 구분자(:)와 제어 문자는 받지 않는다.
func approverName(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 100 || strings.Contains(v, ":") || strings.IndexFunc(v, unicode.IsControl) >= 0 {
		return "", false
	}
	return v, true
}

// signPayload: 확인 토큰이 덮는 것. 계획 파일의 해시 · 승인 키의 지문 · 승인자. 확인 페이지에서 본 것과 서명하는
// 순간의 것이 하나라도 다르면 서명하지 않는다.
func signPayload(planBytes []byte, fingerprint, approver string) string {
	sum := sha256.Sum256(planBytes)
	return hex.EncodeToString(sum[:]) + "|" + fingerprint + "|" + approver
}

// signReady: 서명 양식을 읽고 지금 상태로 서명할 수 있는지 본다. 막히면 거절을 그리고 false.
func (s *server) signReady(w http.ResponseWriter, r *http.Request, back string) (approver string, key ui.KeyCheck, priv string, ok bool) {
	approver, okName := approverName(r.PostForm.Get("approver"))
	key, priv = s.readKey()
	if !okName || r.PostForm.Get("reviewed") != "on" || key.State != ui.KeyOK {
		s.refuse(w, r, http.StatusConflict, ui.RefusedSignNotReady, back, nil)
		return "", key, "", false
	}
	if _, err := os.Lstat(s.signedPath()); err == nil {
		s.refuse(w, r, http.StatusConflict, ui.RefusedSignedExists, back, []ui.FileRef{file(ui.PickLang(r), "signed-plan", s.signedPath())})
		return "", key, "", false
	}
	return approver, key, priv, true
}

// approveSignConfirm: 서명 전 확인(POST). 아무것도 쓰지 않는다. 무엇에(계획 · 해시 · 조치 수) 누가(승인자) 어느 키로
// (지문) 서명하는지 보이고, 그 셋과 rev · 만료 시각을 서버만 아는 키로 묶은 확인 토큰을 싣는다.
func (s *server) approveSignConfirm(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenApprove
	if s.planOut == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	approver, key, _, ok := s.signReady(w, r, back)
	if !ok {
		return
	}
	var (
		payload string
		plan    *provisioningv1.FinalizedPlan
	)
	cur, err := s.snapshot(ui.SignConfirmPath, func() error {
		b, err := os.ReadFile(s.planOut)
		if err != nil {
			return err
		}
		if plan, err = loadPlanFile(s.planOut); err != nil {
			return err
		}
		payload = signPayload(b, key.Fingerprint, approver)
		return nil
	})
	if err != nil || plan == nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	l := ui.PickLang(r)
	lines := ui.SignConfirmLines(l, plan.GetId(), len(plan.GetActions()), strings.SplitN(payload, "|", 2)[0], approver, key.Fingerprint)
	files := []ui.FileRef{file(l, "plan", s.planOut), file(l, "signed-plan", s.signedPath())}
	s.confirmPageAs(w, r, kindApprove, ui.SignPath, back, store.Identity{SessionID: plan.GetId()}, cur, payload, lines, files, ui.SignConfirmDo(l))
}

// approveSign: 확인을 거친 서명(POST). 확인 토큰이 맞고 만료 전이어야 하고, 잠금 안에서 계획 · 키 · 승인자를 **다시
// 읽어** 확인한 것과 같아야 한다. 서명한 계획 파일이 이미 있으면 어떤 경우에도 덮지 않는다. 상태 전이와 서명은 상류
// 함수다. 서명한 계획은 계획 파일 옆에 따로 쓰고 계획 파일은 그대로 둔다.
func (s *server) approveSign(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenApprove
	if s.planOut == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	plan0, err := loadPlanFile(s.planOut)
	if err != nil || plan0 == nil {
		s.refuse(w, r, http.StatusConflict, ui.RefusedStale, back, nil)
		return
	}
	payload, ok := s.confirmed(w, r, kindApprove, back, store.Identity{SessionID: plan0.GetId()})
	if !ok {
		return
	}
	approver, _, priv, ok := s.signReady(w, r, back)
	if !ok {
		return
	}
	var (
		errNotApprovable = errors.New("not approvable")
		errChanged       = errors.New("changed since the confirmation")
		errExists        = errors.New("signed plan exists")
	)
	id := store.Identity{Kind: kindApprove, Org: s.orgName(), SessionID: "approval"}
	err = s.st.SaveChecked(id, []string{s.signedPath()}, s.check(r), s.waitTime(), func() ([]store.Change, error) {
		// 잠금 안에서 다시 본다. 확인한 뒤 누가 서명 파일을 만들었으면 덮지 않는다.
		if _, err := os.Lstat(s.signedPath()); err == nil {
			return nil, errExists
		}
		b, err := os.ReadFile(s.planOut)
		if err != nil {
			return nil, err
		}
		key, priv2 := s.readKey()
		if key.State != ui.KeyOK || priv2 != priv || signPayload(b, key.Fingerprint, approver) != payload {
			return nil, errChanged
		}
		plan, err := loadPlanFile(s.planOut)
		if err != nil {
			return nil, err
		}
		if err := provisioning.PrepareApproval(plan, s.now()); err != nil {
			return nil, fmt.Errorf("%w: %v", errNotApprovable, err)
		}
		sig, err := sign.SignApproval(priv, approver, plan)
		if err != nil {
			return nil, err
		}
		plan.ApprovalSignatures = append(plan.ApprovalSignatures, sig)
		out, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(plan)
		return []store.Change{{Path: s.signedPath(), Content: append(out, '\n')}}, err
	})
	l := ui.PickLang(r)
	switch {
	case errors.Is(err, errExists):
		s.refuse(w, r, http.StatusConflict, ui.RefusedSignedExists, back, []ui.FileRef{file(l, "signed-plan", s.signedPath())})
		return
	case errors.Is(err, errChanged):
		s.refuse(w, r, http.StatusConflict, ui.RefusedStale, back, nil)
		return
	case errors.Is(err, errNotApprovable):
		fmt.Fprintln(os.Stderr, "approve:", err)
		s.refuse(w, r, http.StatusConflict, ui.RefusedPlanNotApprovable, back, nil)
		return
	case err != nil:
		s.fail(w, r, back, err, file(l, "signed-plan", s.signedPath()))
		return
	}
	redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeSigned)}, nil)
}

// approveArchive: 지난 서명(과 그 서명으로 만든 플레이북)을 보관한다(POST). **지우지 않고 이름만 바꾼다** —
// `<계획>.signed.<시각>.json`처럼 시각을 붙여 옆에 둔다. 세 파일은 **한 트랜잭션**이다(store.MoveChecked): 서명 ·
// 생성과 같은 대상 잠금 안에서 rev를 확인하고, 새 이름이 하나라도 이미 있으면 아무것도 옮기지 않고, 도중에 끊기면
// 다음 시작이 끝까지 옮기거나 되돌린다. 지금 계획과 맞는, 이 키로 확인되는 서명은 보관하지 않는다.
func (s *server) approveArchive(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenApprove
	if s.planOut == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	v, err := s.approveModel(ui.Page{})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	if !v.CanArchive() {
		s.refuse(w, r, http.StatusConflict, ui.RefusedStale, back, nil)
		return
	}
	stamp := s.now().UTC().Format("20060102T150405Z")
	var moves []store.Move
	for _, p := range []string{s.signedPath(), s.provisionPath(), s.rollbackPath()} {
		ext := filepath.Ext(p)
		moves = append(moves, store.Move{From: p, To: strings.TrimSuffix(p, ext) + "." + stamp + ext})
	}
	id := store.Identity{Kind: kindArchive, Org: s.orgName(), SessionID: "approval"}
	n, err := s.st.MoveChecked(id, moves, s.check(r), s.waitTime())
	if err != nil {
		l := ui.PickLang(r)
		if errors.Is(err, store.ErrExists) {
			s.refuse(w, r, http.StatusConflict, ui.RefusedExists, back, []ui.FileRef{file(l, "signed-plan", s.signedPath())})
			return
		}
		s.fail(w, r, back, err, file(l, "signed-plan", s.signedPath()))
		return
	}
	redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeArchived, n)}, nil)
}

// approveGenerate: 서명한 계획으로 적용 · 되돌리기 플레이북을 만들어 작업 폴더에 쓴다(POST). 이 키로 확인되는
// 승인이 지금 계획에 있어야 한다. 빈 곳이 있어도 쓰고, 그 수를 알린다(상류의 종료 상태 3과 같은 자리).
func (s *server) approveGenerate(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenApprove
	if s.planOut == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	level, ok := levels[r.PostForm.Get("level")]
	if !ok {
		errorPage(w, r, http.StatusBadRequest, ui.ErrBadRequest, nil)
		return
	}
	var warnings int
	errNotReady := errors.New("not ready")
	id := store.Identity{Kind: kindProvision, Org: s.orgName(), SessionID: "playbooks"}
	err := s.st.SaveChecked(id, []string{s.provisionPath(), s.rollbackPath()}, s.check(r), s.waitTime(), func() ([]store.Change, error) {
		v, err := s.approveModel(ui.Page{})
		if err != nil {
			return nil, err
		}
		if v.Signed.State != ui.SignValid {
			return nil, errNotReady
		}
		signed, err := loadPlanFile(s.signedPath())
		if err != nil {
			return nil, err
		}
		if err := provisioning.Executable(signed); err != nil {
			return nil, fmt.Errorf("%w: %v", errNotReady, err)
		}
		warnings = len(genWarnings(signed, level))
		return []store.Change{
			{Path: s.provisionPath(), Content: []byte(provisioning.GenerateProvisioningPlaybook(signed, level))},
			{Path: s.rollbackPath(), Content: []byte(provisioning.GenerateRollbackPlaybook(signed, level))},
		}, nil
	})
	if errors.Is(err, errNotReady) {
		fmt.Fprintln(os.Stderr, "generate:", err)
		s.refuse(w, r, http.StatusConflict, ui.RefusedGenNotReady, back, nil)
		return
	}
	if err != nil {
		s.fail(w, r, back, err, file(ui.PickLang(r), "playbook", s.provisionPath()))
		return
	}
	redirect(w, r, back, []ui.Notice{ui.N(ui.NoticeGenerated, warnings)}, nil)
}
