// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

// 알림 · 거절 · 오류의 문구 목록. 화면 위 알림은 요청 사이를 코드와 인자로만 오가고(notice.go), 문장은
// 여기서 그 말로 만든다. 인자 종류: n 숫자, r 파일 역할(그 서버의 파일 이름으로 바뀐다), i 식별자(항목 ID 등).

type noticeSpec struct {
	t     T
	kinds string
}

var noticeText = map[string]noticeSpec{
	// 성공 알림. 「 · 」로 시작하는 것은 앞 알림에 이어 붙는다.
	NoticeSaved:               {tSavedNotFinal, ""},
	NoticeFinalizedPlan:       {tFinalizedPlan, "nr"},
	NoticeFinalizedPolicy:     {tFinalizedPolicy, "nr"},
	NoticeJudgmentsSaved:      {tJudgmentsSaved, "nr"},
	NoticeRulesSaved:          {tRulesSaved, "nnnn"},
	NoticeSignatureCleared:    {tSignatureCleared, ""},
	NoticeDeclSaved:           {tDeclSaved, "nnn"},
	NoticeHostsSaved:          {tHostsSaved, "n"},
	NoticeDeclDropped:         {tDeclDroppedNoIP, "n"},
	NoticeDeclStillOff:        {tDeclStillOff, "n"},
	NoticeFinalizedAlready:    {tFinalizedAlready, ""},
	NoticeFinalizedSuperseded: {tFinalizedSuperseded, ""},
	NoticeDeclCreated: {T{
		KO: "빈 선언을 만들었습니다. 아래에서 노드와 자산을 적으십시오",
		EN: "An empty declaration was created. Write the nodes and assets below"}, ""},
	NoticeRoundOpened: {T{
		KO: "새 검토를 시작했습니다. 지난 결론과 계획 칸을 옮겨 왔고, 승인 서명은 비웠습니다",
		EN: "A new review was started: earlier conclusions and plan fields were carried over, and the signature was cleared"}, ""},
	NoticeScopeRoundOpened: {T{
		KO: "새 스코프 검토를 시작했습니다. 지난 결론을 옮겨 왔고, 승인 서명은 비웠습니다",
		EN: "A new scope review was started: earlier conclusions were carried over, and the signature was cleared"}, ""},
	NoticeLayerCreated: {T{
		KO: "빈 계층을 만들었습니다: %s. 아래에서 규칙을 적으십시오",
		EN: "An empty layer was created: %s. Write its rules below"}, "i"},

	// 확정 관문이 막은 까닭. 머리 하나에 빠진 것들이 「 · 」 줄로 붙는다.
	ProblemCannotFinalize: {T{
		KO: "확정할 수 없습니다. 아래 칸을 채우십시오",
		EN: "Cannot finalize: fill in the fields below"}, ""},
	ProblemMandatory: {T{
		KO: "확정할 수 없습니다. 아직 판정하지 않은 필수 항목이 남아 있습니다. 필수 항목은 하나도 빠짐없이 판정해야 합니다",
		EN: "Cannot finalize: mandatory items are still unjudged. Every mandatory item must be judged"}, ""},
	ProblemNoSignature: {T{
		KO: "확정할 수 없습니다. 승인 서명이 없습니다",
		EN: "Cannot finalize: there is no reviewer's signature"}, ""},
	ProblemNotInReview: {T{
		KO: "확정할 수 없습니다. 세션이 검토 중(in-review) 상태가 아닙니다",
		EN: "Cannot finalize: the session is not in review"}, ""},
	ProblemNotDraft: {T{
		KO: "리뷰를 시작할 수 없습니다. 세션이 초안(draft) 상태가 아닙니다",
		EN: "Cannot start the review: the session is not in draft"}, ""},
	ProblemMissingSignature: {T{
		KO: "   · 서명을 채우십시오",
		EN: "   · fill in the signature"}, ""},
	ProblemMissingApproval: {T{
		KO: "   · 승인자와 서명을 채우십시오",
		EN: "   · fill in the approver and the signature"}, ""},
	ProblemMissingConclusion: {T{
		KO: "   · 왜 이렇게 정했는지를 적지 않았습니다: %s",
		EN: "   · no record of why this was decided: %s"}, "i"},
	ProblemMissingConclusionOf: {T{
		KO: "   · 왜 이렇게 정했는지를 적지 않았습니다: %s (%s)",
		EN: "   · no record of why this was decided: %s (%s)"}, "ii"},
	ProblemPlanField: {T{
		KO: "   · 계획에 넣은 대로는 낼 수 없습니다: %s (%s)",
		EN: "   · cannot be planned as selected: %s (%s)"}, "ip"},
	// ProblemInternal — 까닭을 화면 말로 옮길 수 없는 실패. 원문은 서버 표준오류에 참조 번호와 함께 있다.
	ProblemInternal: {T{
		KO: "처리하지 못했습니다. 운영자가 서버 기록에서 참조 번호 %s를 찾아 원인을 확인할 수 있습니다",
		EN: "This could not be done. An operator can find reference %s in the server log to see why"}, "i"},
}

// 계획 칸 거절의 사유(decision.Plan*). ProblemPlanField의 둘째 인자로 들어간다.
var planReasonText = map[string]T{
	"excluded":                 {KO: "자산 스코프 정책이 관리하지 않는 항목입니다. 정책이나 선언을 먼저 고치십시오", EN: "the asset-scope policy does not manage this item; change the policy or the declaration first"},
	"no-level":                 {KO: "위임 수준(L1 · L2 · L3)을 고르십시오", EN: "pick a delegation level (L1, L2 or L3)"},
	"bad-level":                {KO: "위임 수준이 L1 · L2 · L3 가운데 하나가 아닙니다", EN: "the delegation level is not one of L1, L2 or L3"},
	"no-kind":                  {KO: "조치 종류를 고르십시오", EN: "pick a remediation kind"},
	"bad-kind":                 {KO: "조치 종류가 계약에 없는 값입니다", EN: "the remediation kind is not one the contract knows"},
	"no-target":                {KO: "이 조치 종류는 목표 알고리즘이 있어야 합니다", EN: "this remediation kind needs a target algorithm"},
	"l3:activation.activate":   {KO: "L3는 활성화 명령(activate)이 있어야 합니다", EN: "L3 needs an activate command"},
	"l3:activation.restart":    {KO: "L3는 재시작 명령(restart)이 있어야 합니다", EN: "L3 needs a restart command"},
	"l3:activation.deactivate": {KO: "L3는 비활성화 명령(deactivate)이 있어야 합니다", EN: "L3 needs a deactivate command"},
}

var tPlanReasonUnknown = T{KO: "계획 칸을 다시 확인하십시오", EN: "check the plan fields again"}

// tNoticeUnknown — 모르는 코드나 맞지 않는 인자. 주소에 실려 온 값을 그대로 보이지 않는다.
var tNoticeUnknown = T{KO: "알 수 없는 알림입니다", EN: "Unknown notice"}

// 오류 페이지(거절 페이지와 같은 틀)의 머리와 본문. 본문은 「다음에 할 일」을 적는다. 플래그 이름이나 경로를
// 적지 않는다.
var (
	tErrNotFoundHead = T{KO: "이 주소에는 화면이 없습니다", EN: "There is no screen at this address"}
	tErrNotFound     = T{
		KO: "아래 「화면으로 돌아가기」로 첫 화면에 가서 단계 카드에서 화면을 고르십시오.",
		EN: "Use \"Back to the screen\" below to go to the first screen, and pick a screen from the step cards there."}
	tErrMethodHead = T{KO: "이 주소는 이 방식의 요청을 받지 않습니다", EN: "This address does not take this kind of request"}
	tErrMethod     = T{
		KO: "화면의 버튼과 링크로 다시 하십시오.",
		EN: "Use the buttons and links on the screen instead."}
	tErrBadRequestHead = T{KO: "요청을 처리할 수 없습니다", EN: "The request cannot be handled"}
	tErrBadRequest     = T{
		KO: "화면을 새로 고친 뒤 다시 하십시오.",
		EN: "Reload the screen and try again."}
	tErrInputMissingHead = T{KO: "이 화면에 필요한 파일이 없습니다", EN: "A file this screen needs is not there"}
	tErrInputMissing     = T{
		KO: "이 화면은 아래 파일이 있어야 열립니다. 운영자 안내서의 「작업 폴더」 절을 보십시오.",
		EN: "This screen opens only with the file below. See the \"Workspace\" section of the operator guide."}
	tErrInputBrokenHead = T{KO: "파일을 읽지 못했습니다", EN: "A file could not be read"}
	tErrInputBroken     = T{
		KO: "아래 파일을 읽지 못해 화면을 그리지 않았습니다. 파일이 망가졌거나 형식이 맞지 않습니다. 운영자가 서버 기록에서 참조 번호를 찾아 원인을 확인할 수 있습니다.",
		EN: "The screen was not drawn because the file below could not be read: it is damaged or not in the expected format. An operator can find the reference in the server log to see why."}
	tErrForbiddenHead = T{KO: "이 요청을 받지 않았습니다", EN: "This request was refused"}
	tErrForbidden     = T{
		KO: "요청 보호에 걸렸습니다. 화면을 띄운 기계의 브라우저에서 화면을 처음부터 다시 여십시오. 다른 기계에서 보려면 운영자 안내서의 「다른 기계에서 보기」 절을 따르십시오.",
		EN: "The request protection refused it. Open the screen again from the start in a browser on the machine running it. To view it from another machine, follow the \"Viewing from another machine\" section of the operator guide."}
	tErrInternalHead = T{KO: "처리하지 못했습니다", EN: "This could not be done"}
	tErrInternal     = T{
		KO: "예상하지 못한 문제로 멈췄습니다. 파일은 바꾸지 않았습니다. 운영자가 서버 기록에서 참조 번호를 찾아 원인을 확인할 수 있습니다.",
		EN: "Stopped by an unexpected problem. No file was changed. An operator can find the reference in the server log to see why."}
	tErrRef  = T{KO: "참조 번호", EN: "Reference"}
	tErrCode = T{KO: "오류 코드(운영자 안내서의 오류 표)", EN: "Error code (the error table in the operator guide)"}

	// 저장소가 막은 까닭 가운데 운영자가 손봐야 하는 것.
	tRefusedUnsafe = T{
		KO: "파일의 권한이나 링크가 안전하지 않아 멈췄습니다. 운영자 안내서의 「오류 표」 절대로 바로잡은 뒤 다시 하십시오.",
		EN: "Stopped: a file's permissions or links are not safe. Fix them as the \"Error table\" section of the operator guide says, then try again."}
	tRefusedMismatch = T{
		KO: "이 파일들은 다른 상태 디렉토리가 관리하고 있어 저장하지 않았습니다. 운영자 안내서의 「오류 표」 절을 보십시오.",
		EN: "Not saved: these files are managed by another state directory. See the \"Error table\" section of the operator guide."}
	tRefusedDiagnostic = T{
		KO: "작업 폴더의 아래 파일을 읽지 못해 화면이 진단 전용으로 열렸습니다. 저장은 하지 않습니다. 파일을 고치거나 백업에서 되돌린 뒤 화면을 다시 띄우십시오. 운영자가 서버 기록에서 참조 번호를 찾아 원인을 확인할 수 있습니다.",
		EN: "The screen opened for diagnostics only because the workspace files below could not be read. Nothing is saved. Fix them or restore them from a backup, then start the screen again. An operator can find the reference in the server log to see why."}
	tRefusedExists = T{
		KO: "만들려는 파일이 이미 있어 덮어쓰지 않았습니다. 화면을 새로 고치십시오.",
		EN: "The file already exists, so it was not overwritten. Reload the screen."}
	tRefusedDamaged = T{
		KO: "상태 디렉토리의 기록을 읽지 못해 멈췄습니다. 운영자 안내서의 「복구」 절대로 확인해야 합니다.",
		EN: "Stopped: a record in the state directory could not be read. An operator needs to check it as the \"Recovery\" section of the operator guide says."}
)

// 복구 필요 화면. 푸는 버튼은 없고, 안내서의 「복구」 절과 진단 파일로 보낸다.
var (
	tRecoveryHead = T{KO: "복구가 필요합니다", EN: "Recovery required"}
	tRecoveryLede = T{
		KO: "저장하다 멈춘 기록이 남아 있는데, 그 기록의 파일 가운데 pqcaton 밖에서 바뀐 것이 있어 저장을 멈췄습니다. 어느 파일이 옳은지는 화면이 정하지 않으므로 자동으로 고치지 않습니다.",
		EN: "A save stopped part way left a record, and a file in it was changed outside pqcaton, so saving is stopped. The screen does not decide which file is right, so it does not fix anything by itself."}
	tRecoveryStep1 = T{
		KO: "아래 「진단 파일 내려받기」로 지금 상태를 받아 둡니다. 파일의 전체 경로와 해시는 이 파일에 있습니다.",
		EN: "Download the diagnostics file below and keep it. The full paths and hashes of the files are in it."}
	tRecoveryStep2 = T{
		KO: "「pqcaton 밖에서 바뀜」인 파일은 지금 상태 그대로 따로 복사해 보존합니다.",
		EN: "Copy each file marked \"changed outside pqcaton\" somewhere else, as it is now."}
	tRecoveryStep3 = T{
		KO: "운영자 안내서의 「복구」 절을 따라, 기록이 기대하는 상태로 파일을 맞춥니다.",
		EN: "Follow the \"Recovery\" section of the operator guide to put the files in the state the record expects."}
	tRecoveryStep4 = T{
		KO: "화면을 끄고 다시 띄웁니다. 시작할 때 남은 기록을 끝까지 적용합니다.",
		EN: "Stop the screen and start it again. On start it carries the remaining record through."}
	tRecoveryDiag       = T{KO: "진단 파일 내려받기", EN: "Download the diagnostics file"}
	tRecoveryUnreadable = T{KO: "복구 상태를 읽지 못했습니다. 서버 로그에 같은 참조 번호로 원인이 남아 있습니다.", EN: "The recovery state could not be read. The server log has the cause under the same reference."}
	tRecoveryColFile    = T{KO: "파일", EN: "File"}
	tRecoveryColChange  = T{KO: "하려던 일", EN: "Intended change"}
	tRecoveryColNow     = T{KO: "지금 상태", EN: "Now"}
)

// recordKindText · itemKindText · verdictText: 저장소 코드를 그 말로. 모르는 코드는 그대로 보인다.
func recordKindText(l Lang, kind string) string {
	switch kind {
	case "review":
		return T{KO: "판정(리뷰 세션)", EN: "Review"}.In(l)
	case "scope":
		return T{KO: "자산 스코프", EN: "Asset scope"}.In(l)
	case "create":
		return T{KO: "새 파일 만들기", EN: "Creating a file"}.In(l)
	}
	return kind
}

func itemKindText(l Lang, kind string) string {
	switch kind {
	case "replace":
		return T{KO: "내용 바꾸기", EN: "Replace the content"}.In(l)
	case "append":
		return T{KO: "원장에 덧붙이기", EN: "Append to the ledger"}.In(l)
	case "create":
		return T{KO: "새로 만들기", EN: "Create"}.In(l)
	}
	return kind
}

func verdictText(l Lang, v string) string {
	switch v {
	case VerdictDone:
		return T{KO: "적용됨", EN: "applied"}.In(l)
	case VerdictPending:
		return T{KO: "아직 적용하지 않음: 다시 띄우면 끝까지 적용합니다", EN: "not applied yet: the next start finishes it"}.In(l)
	case VerdictPartial:
		return T{KO: "원장에 일부만 씀: 다시 띄우면 나머지를 씁니다", EN: "half written to the ledger: the next start writes the rest"}.In(l)
	case VerdictMismatch:
		return T{KO: "pqcaton 밖에서 바뀜: 덮어쓰지 않습니다", EN: "changed outside pqcaton: nothing will overwrite it"}.In(l)
	}
	return v
}

// 확정된 세션의 화면. 읽기 전용이고, 새 회차를 연다.
var (
	tRoundReviewHead = T{KO: "이 판정 세션은 확정되었습니다", EN: "This review session is finalized"}
	tRoundReviewBody = T{
		KO: "확정한 세션은 고칠 수 없어 읽기 전용으로 보입니다. 바꾸려면 새 검토를 시작하십시오. 결론과 계획 칸은 새 세션으로 옮겨 가고, 승인 서명은 새로 받습니다.",
		EN: "A finalized session cannot be changed, so it is shown read-only. To change it, start a new review: conclusions and plan fields carry over, and a new signature is needed."}
	tRoundReviewDo  = T{KO: "새 검토 시작", EN: "Start a new review"}
	tRoundScopeHead = T{KO: "이 스코프 검토는 확정되었습니다", EN: "This scope review is finalized"}
	tRoundScopeBody = T{
		KO: "확정한 스코프 검토는 고칠 수 없어 읽기 전용으로 보입니다. 규칙이나 결론을 바꾸려면 새 스코프 검토를 시작하십시오. 결론은 옮겨 가고, 승인 서명은 새로 받습니다.",
		EN: "A finalized scope review cannot be changed, so it is shown read-only. To change rules or conclusions, start a new scope review: conclusions carry over, and a new signature is needed."}
	tRoundScopeDo = T{KO: "새 스코프 검토 시작", EN: "Start a new scope review"}
)
