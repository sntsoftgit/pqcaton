// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// 화면에 뜨는 모든 문구. **두 말이 한 줄 건너 나란히 있다.**
//
// 파일을 갈라 두면 한쪽만 고쳐지는 날이 오고, 그날 어느 쪽이 최신인지 아무도 모른다.
// 이름 있는 변수라 새 문구를 빠뜨리면 컴파일이 막는다.
//
// **문구 안에 태그가 있다.** 한국어와 영어는 어순이 달라서, 강조할 자리를 조각내 두면
// 한쪽 말이 반드시 어색해진다. 여기 있는 것은 전부 우리가 쓴 상수이지 밖에서 온 값이
// 아니므로, `templ.Raw`로 그대로 화면에 나간다.
package ui

// ── 이동 · 화면 이름 ────────────────────────────────────────────────────────
var (
	tTitleDecl    = T{KO: "대상 노드와 기대 상태", EN: "Target nodes and expected state"}
	tTitleScope   = T{KO: "암호 자산 스코프", EN: "Crypto asset scope"}
	tTitleObserve = T{KO: "관측", EN: "Observation"}
	tTitleSurvey  = T{KO: "대조", EN: "Reconciliation"}
	tTitleReview  = T{KO: "판정(리뷰 큐)", EN: "Judgment (review queue)"}
	tTitleApprove = T{KO: "프로비저닝", EN: "Provisioning"}

	tSubOrg     = T{KO: "조직", EN: "org"}
	tSubSession = T{KO: "세션", EN: "session"}
	tSubResults = T{KO: "결과", EN: "results"}

	// 저장·확정 뒤에 뜨는 알림. **화면 글이라 보는 사람의 말로 뜬다.**
	tSavedNotFinal = T{
		KO: "세션 파일에 저장했습니다. 아직 확정하지 않았습니다",
		EN: "Saved to the session file — not finalized yet"}
	tFinalizedPlan = T{
		KO: "판정을 확정했습니다. 조치 %d건을 %s에 썼습니다. 실행 승인은 pqcota-approve가 합니다",
		EN: "Judging finalized — %d actions written to %s. Execution approval is done by pqcota-approve"}
	tFinalizedPolicy = T{
		KO: "확정했습니다. 규칙 %d개를 %s에 썼습니다. 이 파일이 `pqcota-ingest -scope-assets`의 입력입니다",
		EN: "Finalized — %d rules written to %s. That file is the input to `pqcota-ingest -scope-assets`"}
	tJudgmentsSaved = T{
		KO: " · 판정 %d건을 %s에 남겼습니다",
		EN: " · %d judgments appended to %s"}
	tRulesSaved = T{
		KO: "규칙 %d개를 계층 %d개에 썼습니다. 판정할 변경이 %d건이고, 그중 %d건은 왜 뺐는지를 적어야 합니다",
		EN: "Wrote %d rules across %d layers — %d changes to judge, %d of them needing a recorded reason"}
	tSignatureCleared = T{
		KO: " · 정책이 달라져 서명을 지웠습니다",
		EN: " · the policy changed, so the signature was cleared"}
	tDeclSaved = T{
		KO: "선언을 저장했습니다. 노드 %d · 자산 %d · 엣지 %d",
		EN: "Declaration saved — %d nodes · %d assets · %d edges"}
	tDeclStillOff = T{
		KO: " · 맞지 않는 자리 %d곳이 남아 있습니다",
		EN: " · still not adding up: %d places"}

	// 저장하지 않은 까닭(409 · 503 페이지). 내부 경로와 오류 원문은 싣지 않는다.
	tNotSaved      = T{KO: "저장하지 않았습니다", EN: "Not saved"}
	tBackToScreen  = T{KO: "화면으로 돌아가기", EN: "Back to the screen"}
	tFilesInvolved = T{KO: "관련 파일", EN: "Files involved"}
	tRefusedStale  = T{
		KO: "화면을 연 뒤에 파일이 바뀌어 저장하지 않았습니다. 화면을 새로 고친 뒤 다시 적용하십시오.",
		EN: "The files changed after this screen was opened, so nothing was saved. Reload the screen and apply your changes again."}
	tRefusedFinalized = T{
		KO: "이 세션은 이미 확정되었습니다. 바꾸려면 새 세션을 여십시오.",
		EN: "This session is already finalized. Open a new session to make changes."}
	tRefusedRulesetChanged = T{
		KO: "이 세션은 지금과 다른 상류 규칙 판으로 열렸거나 규칙 판이 적혀 있지 않습니다. 두 규칙 판이 섞이지 않도록 저장하거나 확정하지 않았고, 세션 파일은 그대로입니다. 판정 화면 위쪽의 「지금 규칙으로 새 회차 시작」으로 넘어가십시오.",
		EN: "This session was opened under a different upstream ruleset, or records none. To keep the two rulesets from mixing, nothing was saved or finalized, and the session file is unchanged. Move on with \"Start a new round under the current rules\" at the top of the review screen."}
	tRefusedPlanRuleset = T{
		KO: "이 계획은 지금과 다른 상류 규칙 판으로 판정됐거나, 근거의 규칙 판을 확인할 수 없습니다. 서명하거나 플레이북을 만들지 않았고, 있는 파일은 그대로입니다. 지금 규칙의 새 회차에서 다시 확정하십시오. 상류를 v0.10.5 이하로 운영한다면 pqcaton v0.24.x나 상류 pqcota-provision v0.10.5로 생성하십시오.",
		EN: "This plan was judged under a different upstream ruleset, or the ruleset of its evidence cannot be confirmed. Nothing was signed or generated, and the existing files are unchanged. Finalize it again in a new round under the current rules. If upstream still runs v0.10.5 or earlier, generate with pqcaton v0.24.x or upstream pqcota-provision v0.10.5."}
	tPlanRulesetTop          = T{KO: "이 계획은 지금과 다른 상류 규칙 판으로 판정됐습니다. 서명 · 생성하지 않습니다. 열람과 보관은 할 수 있습니다.", EN: "This plan was judged under a different upstream ruleset. It is not signed or generated here; it can still be viewed and archived."}
	tPlanRulesetEvidence     = T{KO: "이 계획의 근거 가운데 다른 상류 규칙 판의 것이 있습니다(규칙 판이 섞인 계획). 서명 · 생성하지 않습니다.", EN: "Some evidence in this plan is under a different upstream ruleset (a plan that mixes rulesets). It is not signed or generated."}
	tPlanRulesetUnverifiable = T{KO: "이 계획의 관측 근거 가운데 스냅샷 참조나 규칙 판이 없는 것이 있어 지금 이력에서 되짚을 수 없습니다. 서명 · 생성하지 않습니다.", EN: "Some observed evidence in this plan has no snapshot reference or ruleset, so it cannot be traced in the current history. It is not signed or generated."}
	tSignedRulesetProblem    = T{KO: "서명한 계획(plan.signed.json)이 지금과 다른 상류 규칙 판으로 판정됐거나 근거의 규칙 판을 확인할 수 없습니다. 그 계획으로는 다시 서명하거나 플레이북을 만들지 않습니다. 열람과 보관은 할 수 있습니다.", EN: "The signed plan (plan.signed.json) was judged under a different upstream ruleset, or the ruleset of its evidence cannot be confirmed. It is not signed again or used to generate playbooks; it can still be viewed and archived."}
	tRefusedPlanScope        = T{
		KO: "이 계획은 이 작업 폴더에서 확정한 기록과 이어지지 않거나, 지금 활성 자산 스코프 정책과 다른 정책으로 만들어졌습니다. 서명하거나 플레이북을 만들지 않았고, 있는 파일은 그대로입니다. 현재 정책에서 03의 새 판정 회차를 열어 다시 확정하십시오. 다른 곳에서 옮겨 온 계획은 pqcota-approve와 pqcota-provision으로 승인하고 생성하십시오.",
		EN: "This plan does not lead back to a finalize recorded in this workspace, or it was made under an asset-scope policy other than the active one. Nothing was signed or generated, and the existing files are unchanged. Open a new review round in 03 under the current policy and finalize again. A plan brought in from elsewhere is approved and generated with pqcota-approve and pqcota-provision."}
	tRefusedPlanText = T{
		KO: "현재 계획(plan.json)의 문자열에 NUL 문자가 있어 안전하게 승인할 수 없습니다. pqcota는 이런 계획의 서명과 생성을 거절합니다. 새 판정 회차를 열어 해당 값을 고친 뒤 다시 확정하십시오.",
		EN: "The current plan (plan.json) contains a NUL character in a string and cannot be approved safely. pqcota refuses to sign or generate such a plan. Open a new review round, correct the affected value, and finalize it again."}
	tSignedPlanText = T{
		KO: "서명한 계획(plan.signed.json)의 문자열에 NUL 문자가 있어 안전하게 승인할 수 없습니다. pqcota는 이런 계획의 서명과 생성을 거절합니다. 서명한 계획을 보관하고, 새 판정 회차를 열어 해당 값을 고친 뒤 다시 확정하십시오.",
		EN: "The signed plan (plan.signed.json) contains a NUL character in a string and cannot be approved safely. pqcota refuses to sign or generate such a plan. Archive the signed plan, open a new review round, correct the affected value, and finalize it again."}
	tRefusedPlanFields = T{
		KO: "현재 계획(plan.json)에 소스 근거나 승인 가이드가 들어 있습니다. 계획이 손상된 것은 아닙니다. pqcaton은 아직 그 내용을 04에서 검토하거나 플레이북에 싣지 않으므로 이 계획에 서명하지 않습니다. pqcota-approve --review로 내용을 확인한 뒤 pqcota-approve와 pqcota-provision으로 승인하고 생성하십시오.",
		EN: "The current plan (plan.json) carries source evidence or an approval guide. The plan is not damaged: pqcaton does not yet review that content on 04 or put it into playbooks, so it does not sign this plan. Check the content with pqcota-approve --review, then approve and generate with pqcota-approve and pqcota-provision."}
	tSignedPlanFields = T{
		KO: "서명한 계획(plan.signed.json)에 소스 근거나 승인 가이드가 들어 있습니다. 계획이 손상된 것은 아닙니다. pqcaton은 아직 그 내용을 검토하거나 플레이북에 싣지 않으므로 이 계획으로 플레이북을 만들지 않습니다. 서명한 계획은 보관할 수 있고, 생성은 pqcota-provision으로 하십시오.",
		EN: "The signed plan (plan.signed.json) carries source evidence or an approval guide. The plan is not damaged: pqcaton does not yet review that content or put it into playbooks, so it does not generate from this plan. The signed plan can be archived; generate with pqcota-provision."}
	tGenFieldsBlocked = T{
		KO: "계획에 소스 근거나 승인 가이드가 있어 pqcaton은 플레이북을 만들지 않습니다. pqcota-provision으로 만드십시오.",
		EN: "The plan carries source evidence or an approval guide, so pqcaton does not generate playbooks. Generate them with pqcota-provision."}
	tGenTextBlocked = T{
		KO: "계획의 문자열에 NUL 문자가 있어 플레이북을 만들지 않습니다.",
		EN: "The plan contains a NUL character in a string, so no playbooks are generated."}
	tRefusedSessionText = T{
		KO: "확정하면 만들어질 계획의 문자열에 NUL 문자가 있어 확정하지 않았습니다. 세션 · 계획 · 영수증은 그대로입니다. pqcota는 이런 계획의 서명과 생성을 거절합니다. 해당 값을 고친 뒤 다시 확정하십시오.",
		EN: "The plan this finalize would write contains a NUL character in a string, so nothing was finalized. The session, plan and receipt are unchanged. pqcota refuses to sign or generate such a plan. Correct the affected value and finalize again."}
	tRefusedScopePolicy = T{
		KO: "이 판정 세션을 세울 때 쓴 자산 스코프 정책이 지금 정책과 다르거나, 세션에 정책 기록이 없습니다(이 판보다 먼저 만든 세션). 확정하지 않았고 파일은 그대로입니다. 세션을 세울 때와 같은 정책으로 확정하거나, 지금 정책으로 새 세션을 여십시오.",
		EN: "The asset-scope policy this review session was built under differs from the current one, or the session records no policy (it was made before this version). Nothing was finalized and the files are unchanged. Finalize under the policy the session was built with, or open a new session under the current policy."}
	tRefusedScopeBaseline = T{
		KO: "이 판정 세션에는 자산 스코프 정책 기록이 없습니다(이 판보다 먼저 만든 세션). 저장하거나 확정하지 않았고 파일은 그대로입니다. 판정 화면 위쪽에서 지금 정책을 기준선으로 채택하는 새 회차를 여십시오.",
		EN: "This review session records no asset-scope policy (it was made before this version). Nothing was saved or finalized and the files are unchanged. Open the new round that adopts the current policy as the baseline, at the top of the review screen."}
	tRefusedScopeUnactivated = T{
		KO: "자산 스코프 정책(scope-assets.csv)이 활성화 기록 없이 바뀌었습니다. 저장하거나 확정하지 않았고 파일은 그대로입니다. 이전 정책 파일을 복원하거나, pqcota를 새 정책으로 다시 적재한 뒤 정식 정책 활성화를 거치십시오.",
		EN: "The asset-scope policy (scope-assets.csv) changed without an activation record. Nothing was saved or finalized and the files are unchanged. Restore the previous policy file, or re-ingest pqcota with the new policy and go through the policy activation."}
	tRefusedUpstreamUnreachable = T{
		KO: "pqcota 이력에 닿지 못했거나 조회가 10초 안에 끝나지 않아 정책을 활성화하거나 기준선을 채택하지 않았습니다. 파일은 그대로입니다. pqcota 연결을 확인한 뒤 다시 하십시오.",
		EN: "The pqcota history could not be reached, or the lookup did not finish within 10 seconds, so no policy was activated or adopted as the baseline. The files are unchanged. Check the pqcota connection and try again."}
	tRefusedUpstreamOrg = T{
		KO: "연결한 pqcota 이력의 조직이 판정 조직이나 판정 세션의 조직과 달라, 조회하지 않고 정책을 활성화하거나 기준선을 채택하지 않았습니다. 파일은 그대로입니다. 다른 조직의 이력으로는 진행할 수 없습니다.",
		EN: "The organization of the connected pqcota history differs from the organization of the judgments or of the review session, so nothing was looked up and no policy was activated or adopted as the baseline. The files are unchanged. The history of another organization cannot be used."}
	tRefusedUpstreamMissing = T{
		KO: "pqcota 이력에서 일부 노드의 최신 적재 결과가 없거나 이 정책으로 만든 결과와 달라 정책을 활성화하거나 기준선을 채택하지 않았습니다. 파일은 그대로입니다. 01에서 pqcota를 이 정책으로 다시 적재한 뒤 하십시오.",
		EN: "In the pqcota history, the latest ingest of some nodes is missing or differs from the results under this policy, so no policy was activated or adopted as the baseline. The files are unchanged. Re-ingest pqcota with this policy on screen 01, then try again."}
	tRefusedUpstreamEmpty = T{
		KO: "결과 폴더에 pqcota 이력과 대조할 관측 결과가 없어 정책을 활성화하거나 기준선을 채택하지 않았습니다. 파일은 그대로입니다.",
		EN: "The results folder holds no observations to reconcile with the pqcota history, so no policy was activated or adopted as the baseline. The files are unchanged."}
	tRefusedUpstreamRevert = T{
		KO: "pqcota의 최신 적재 결과가 이 정책과 다릅니다. 이 정책으로 결과를 다시 적재한 뒤 활성화하십시오. 파일은 그대로입니다.",
		EN: "The latest ingest in the pqcota history differs from this policy. Re-ingest the results under this policy, then activate it. The files are unchanged."}
	tRefusedScopeSame           = T{KO: "자산 스코프 화면에서 확정한 정책이 이미 활성 정책과 같습니다(note · 공백만 다를 수 있습니다). 활성화할 것이 없어 아무것도 쓰지 않았습니다.", EN: "The policy finalized on the asset scope screen is already the active policy (only notes or spacing may differ). There is nothing to activate, so nothing was written."}
	tRefusedScopeNotFinalized   = T{KO: "자산 스코프 화면에서 확정한 정책이 없거나, 확정한 뒤 asset-scope.csv가 바뀌었거나, 읽을 수 없어 활성화하지 않았습니다. 자산 스코프 화면에서 확정하십시오.", EN: "There is no policy finalized on the asset scope screen, asset-scope.csv changed after it was finalized, or it cannot be read, so nothing was activated. Finalize it on the asset scope screen."}
	tActivateBody               = T{KO: "자산 스코프 화면에서 확정한 정책(asset-scope.csv)이 지금 대조에 쓰는 활성 정책(scope-assets.csv)과 다릅니다. 새 정책으로 다시 적재하고 활성화하는 일은 01 관측 화면에서 합니다. 활성화하면 결과에서 세션을 다시 세운 새 회차가 열리고, 지난 결론은 「지난 판정」으로만 옮겨집니다.", EN: "The policy finalized on the asset scope screen (asset-scope.csv) differs from the active policy used for reconciliation (scope-assets.csv). Re-ingesting under the new policy and activating it are done on screen 01. Activation opens a new round rebuilt from the results; earlier conclusions are carried only as previous values."}
	tActivateStep1Body          = T{KO: "자산 스코프 화면에서 확정한 정책으로 pqcota를 다시 적재합니다. 먼저 첫 명령으로 결과의 수집기 서명을 확인하고(저장하지 않습니다, 거절될 결과가 있으면 종료 코드 1), 통과하면 둘째 명령으로 적재합니다. 두 명령 모두 PQCOTA_VERIFY_KEY를 01이 신뢰하는 공개 키(verify.pub)와 같은 값으로 주고, 연결 정보는 PQCOTA_DSN으로만 넘깁니다.", EN: "Ingest pqcota again with the policy finalized on the asset scope screen. First check the collector signatures of the results with the first command (it stores nothing and exits 1 if any result would be refused); when it passes, ingest with the second. Give both PQCOTA_VERIFY_KEY with the same value as the trusted public keys of 01 (verify.pub), and pass the connection only as PQCOTA_DSN."}
	tActivateStep1Hint          = T{KO: "작업 폴더에서 실행합니다. 경로는 작업 폴더 기준의 상대 경로입니다.", EN: "Run it in the workspace folder; the paths are relative to it."}
	tSurveyActivateHead         = T{KO: "자산 스코프 화면에서 확정한 정책이 아직 활성화되지 않았습니다", EN: "The policy finalized on the asset scope screen is not active yet"}
	tSurveyActivateBody         = T{KO: "이 화면은 현재 활성 정책(scope-assets.csv)으로 대조했습니다. 자산 스코프 화면에서 확정한 정책(asset-scope.csv)은 아직 대조에 반영되지 않았습니다. 새 정책을 반영하려면 01에서 그 정책으로 다시 적재한 뒤 활성화하십시오.", EN: "This comparison uses the active policy (scope-assets.csv). The policy finalized on the asset scope screen (asset-scope.csv) has not yet been applied to reconciliation. To apply it, re-ingest under that policy on screen 01, then activate it there."}
	tSurveyActivatePolicy       = T{KO: "활성 정책 → 자산 스코프 화면에서 확정한 정책:", EN: "Active policy → policy finalized on the asset scope screen:"}
	tSurveyActivateGo           = T{KO: "01에서 재적재 · 활성화 보기", EN: "View re-ingest and activation in 01"}
	tScopeNextHead              = T{KO: "다음 단계", EN: "Next"}
	tScopeNextBody              = T{KO: "이 정책은 아직 대조에 반영되지 않았습니다. 먼저 02에서 현재 활성 정책으로 계산한 대조 결과를 확인하십시오.", EN: "This policy has not yet been applied to reconciliation. First review the reconciliation produced with the currently active policy in 02."}
	tScopeNextSurvey            = T{KO: "02에서 현재 대조 보기", EN: "View current reconciliation in 02"}
	tScopeNextSteps             = T{KO: "01에서 재적재 · 활성화 보기", EN: "View re-ingest and activation in 01"}
	tActivateNext               = T{KO: "정책 활성화 확인", EN: "Confirm policy activation"}
	tActivateConfirmTitle       = T{KO: "정책 활성화 전 확인", EN: "Before activating the policy"}
	tActivateConfirmBody        = T{KO: "아래 내용으로 자산 스코프 정책을 활성화합니다. 아직 아무것도 쓰지 않았습니다.", EN: "The asset-scope policy will be activated as below. Nothing has been written yet."}
	tActivateDo                 = T{KO: "새 정책 활성화", EN: "Activate the new policy"}
	tActivateLineFrom           = T{KO: "지금 활성 정책: %s", EN: "Active policy now: %s"}
	tActivateLineTo             = T{KO: "새 활성 정책: %s (%s)", EN: "New active policy: %s (%s)"}
	tActivateLineFiles          = T{KO: "%s를 새 정책으로 바꾸고, 옛 정책은 %s로 보관합니다.", EN: "%s is replaced by the new policy; the old policy is archived as %s."}
	tActivateLineRound          = T{KO: "판정 세션은 보관되고, 새 정책으로 다시 세운 새 회차가 열립니다. 지난 결론은 「지난 판정」으로만 옮겨지고 새 정책에서 빠진 항목은 변환 보고서에만 남습니다. 옛 정책으로 만든 계획은 04에서 서명 · 생성되지 않습니다.", EN: "The review session is archived and a new round rebuilt under the new policy opens. Earlier conclusions are carried only as previous values, and items the new policy no longer manages stay only in the change report. Plans made under the old policy are not signed or generated in 04."}
	tActivateLineUpstream       = T{KO: "확인 시점에 pqcota 이력과 일치했습니다(노드 %d개, %s 기준). 활성화를 누르면 처음부터 다시 대조합니다.", EN: "At the time of checking, the pqcota history matched (%d nodes, as of %s). Activating reconciles again from the start."}
	tActivateLineDowngrade      = T{KO: "이 작업을 하면 이 작업 폴더를 v0.25.x 이하로 다시 열 수 없습니다. 필요하면 먼저 작업 폴더를 백업하십시오(운영자 안내서 7절).", EN: "After this, the workspace cannot be opened with v0.25.x or earlier; back it up first if you may need to (operator guide section 7)."}
	tScopeBaselineHead          = T{KO: "정책 기준선을 채택해야 합니다", EN: "Adopt a policy baseline"}
	tScopeBaselineBody          = T{KO: "이 세션은 이 판보다 먼저 만들어져 어느 자산 스코프 정책으로 판정했는지 기록이 없습니다. 지금 scope-assets.csv를 기준선으로 채택하면 결과에서 세션을 다시 세운 새 회차가 열리고, 지난 결론은 「지난 판정」으로만 옮겨집니다. 채택할 때 pqcota 이력이 이 정책으로 적재돼 있는지 대조하고, 일치하지 않으면 채택하지 않습니다. 옛 세션 파일은 보관됩니다. 이 작업을 하면 이 작업 폴더를 v0.25.x 이하로 다시 열 수 없습니다. 필요하면 먼저 작업 폴더를 백업하십시오(운영자 안내서 7절).", EN: "This session was made before this version and records no asset-scope policy. Adopting the current scope-assets.csv as the baseline opens a new round rebuilt from the results; earlier conclusions are carried only as previous values. Adopting reconciles the pqcota history against this policy first and stops if it does not match. The old session file is archived. After this, the workspace cannot be opened with v0.25.x or earlier; back it up first if you may need to (operator guide section 7)."}
	tScopeBaselineDo            = T{KO: "지금 정책을 기준선으로 새 회차 시작", EN: "Start a new round with the current policy as baseline"}
	tScopeUnactivatedHead       = T{KO: "자산 스코프 정책이 활성화 없이 바뀌었습니다", EN: "The asset-scope policy changed without activation"}
	tScopeUnactivatedBody       = T{KO: "이 세션을 판정한 정책과 지금 scope-assets.csv가 다르고, 그 변경을 잇는 활성화 기록이 없습니다. 저장과 확정은 막혀 있습니다. 이전 정책 파일을 복원하거나, pqcota를 새 정책으로 다시 적재한 뒤 자산 스코프 화면에서 확정한 정책을 정식으로 활성화하십시오.", EN: "The policy this session was judged under differs from the current scope-assets.csv, and no activation records that change. Saving and finalizing are blocked. Restore the previous policy file, or re-ingest pqcota with the new policy and activate the policy finalized on the asset scope screen."}
	tStepReviewScopeBaseline    = T{KO: "정책 기준선을 채택해야 합니다", EN: "A policy baseline must be adopted"}
	tStepReviewScopeUnactivated = T{KO: "정책이 활성화 없이 바뀌었습니다", EN: "The policy changed without activation"}
	tPlanScopeNotHere           = T{KO: "이 계획(plan.json)은 이 작업 폴더에서 확정한 기록과 이어지지 않습니다. 다른 곳에서 옮겨 왔을 수 있습니다. 서명 · 생성하지 않습니다. 열람과 보관은 할 수 있습니다. 옮겨 온 계획은 pqcota-approve와 pqcota-provision으로 승인하고 생성하십시오.", EN: "This plan (plan.json) does not lead back to a finalize recorded in this workspace; it may have been brought in from elsewhere. It is not signed or generated here; it can still be viewed and archived. Approve and generate a plan brought in from elsewhere with pqcota-approve and pqcota-provision."}
	tPlanScopeChanged           = T{KO: "이 계획(plan.json)은 확정한 뒤에 내용이 바뀌었습니다. 서명 · 생성하지 않습니다. 열람과 보관은 할 수 있습니다. 03에서 다시 확정하십시오.", EN: "This plan (plan.json) changed after it was finalized. It is not signed or generated here; it can still be viewed and archived. Finalize again in 03."}
	tPlanScopeUnverifiable      = T{KO: "이 계획(plan.json)은 이 판보다 먼저 확정되어 어느 자산 스코프 정책으로 만들었는지 기록이 없습니다. 서명 · 생성하지 않습니다. 열람과 보관은 할 수 있습니다. 현재 정책에서 03의 새 판정 회차를 열어 다시 확정하십시오.", EN: "This plan (plan.json) was finalized before this version, so there is no record of the asset-scope policy it was made under. It is not signed or generated here; it can still be viewed and archived. Open a new review round in 03 under the current policy and finalize again."}
	tPlanScopeMismatch          = T{KO: "이 계획(plan.json)은 지금 활성 정책(scope-assets.csv)과 다른 자산 스코프 정책으로 만들었습니다. 서명 · 생성하지 않습니다. 열람과 보관은 할 수 있습니다. 현재 정책에서 03의 새 판정 회차를 열어 다시 확정하십시오.", EN: "This plan (plan.json) was made under an asset-scope policy other than the active one (scope-assets.csv). It is not signed or generated here; it can still be viewed and archived. Open a new review round in 03 under the current policy and finalize again."}
	tGenScopeBlocked            = T{KO: "이 작업 폴더의 확정 기록과 이어지지 않거나 정책이 맞지 않는 계획으로는 새 플레이북을 만들지 않습니다. 이미 만든 플레이북은 그대로 둡니다.", EN: "No new playbook is generated from a plan that does not lead back to this workspace's finalize record or whose policy does not match. Playbooks already made are left as they are."}
	tGenRulesetBlocked          = T{KO: "규칙 판이 맞지 않는 계획으로는 새 플레이북을 만들지 않습니다. 이미 만든 플레이북은 그대로 둡니다.", EN: "No new playbook is generated from a plan whose ruleset does not match. Playbooks already made are left as they are."}
	tJCAUnconfirmed             = T{KO: "v0.10.6 JCA 수집 결과임을 확인할 수 없는 노드가 있습니다. 수집기와 collector.jar의 판을 확인하십시오:", EN: "For some nodes, the JCA results cannot be confirmed as v0.10.6 collection results. Check the version of the collectors and collector.jar:"}
	tRefusedBusy                = T{
		KO: "다른 저장이 아직 끝나지 않았습니다. 잠시 뒤 다시 시도하십시오.",
		EN: "Another save has not finished yet. Try again in a moment."}
	tRefusedRecovery = T{
		KO: "파일이 저장 기록과 맞지 않아 멈췄습니다. 운영자가 운영자 안내서의 「복구」 절대로 확인해야 합니다.",
		EN: "Stopped: the files do not match the save record. An operator needs to check them as the \"Recovery\" section of the operator guide says."}
	tRefusedViewOnly = T{
		KO: "이 플랫폼에서는 화면을 보기만 할 수 있습니다. 저장은 Linux와 Intel Mac(검증함), Apple Silicon Mac(검증하지 않음)에서 합니다.",
		EN: "On this platform the screen is view-only. Saving runs on Linux and Intel Macs (verified) and Apple Silicon Macs (not verified)."}
	tRefusedExpired = T{
		KO: "확인 화면이 만료되었습니다(10분이 지났습니다). 확정을 처음부터 다시 하십시오.",
		EN: "The confirmation page has expired (10 minutes passed). Start the finalize again."}
	tRefusedConfirmBad = T{
		KO: "확인 정보가 맞지 않아 확정하지 않았습니다. 서버를 다시 시작했거나 확인 페이지를 거치지 않았습니다. 확정을 처음부터 다시 하십시오.",
		EN: "Not finalized: the confirmation does not match. The server may have restarted, or the request skipped the confirmation page. Start the finalize again."}
	// 확정 전 확인 페이지.
	tConfirmTitle = T{KO: "확정 전 확인", EN: "Confirm before finalizing"}
	tConfirmBody  = T{
		KO: "아래 내용으로 확정합니다. 확정한 세션은 다시 고칠 수 없고, 바꾸려면 새 세션을 열어야 합니다. 아직 아무것도 저장하지 않았습니다.",
		EN: "This finalizes the content below. A finalized session cannot be edited again; open a new session to change it. Nothing has been saved yet."}
	tConfirmDo        = T{KO: "확정합니다", EN: "Finalize"}
	tConfirmSaveOnly  = T{KO: "확정하지 않고 저장만 합니다", EN: "Save without finalizing"}
	tConfirmBack      = T{KO: "확정하지 않고 화면으로 돌아가기(저장하지 않은 편집은 남지 않습니다)", EN: "Back to the screen without finalizing (unsaved edits are not kept)"}
	tLinePlanActions  = T{KO: "계획에 넣는 조치 %d건", EN: "%d actions in the plan"}
	tLineJudgments    = T{KO: "판정 원장에 남길 기록 %d줄", EN: "%d records added to the judgment ledger"}
	tLinePolicyRules  = T{KO: "정책 규칙 %d개", EN: "%d policy rules"}
	tFinalizedAlready = T{
		KO: "같은 내용으로 이미 확정되어 있어 다시 쓰지 않았습니다.",
		EN: "Already finalized with the same content; nothing was written again."}
	tFinalizedSuperseded = T{
		KO: "이 세션이 만든 출력 경로에는 현재 다른 세션의 확정 결과가 있습니다. 이전 결과를 다시 쓰지 않았습니다.",
		EN: "The output paths this session produced now hold another session's finalized result. The earlier result was not written again."}
	tRefused = T{KO: "요청하신 것을 하지 않았습니다. 이유는 이렇습니다.",
		EN: "Not done — here is why."}
	// 제거는 **묻고 지운다.** 잘못 누르면 적어 둔 것이 한 번에 사라지는 자리다.
	tRemove        = T{KO: "제거", EN: "Remove"}
	tRemoveNodeAsk = T{
		KO: "이 노드와 그 안의 암호 자산을 표에서 지웁니다. 저장해야 파일에 반영됩니다. 지울까요?",
		EN: "This removes the node and the crypto assets inside it from the table. It reaches the file only when you save. Remove?"}
	tRemoveAssetAsk = T{
		KO: "이 암호 자산을 표에서 지웁니다. 저장해야 파일에 반영됩니다. 지울까요?",
		EN: "This removes the crypto asset from the table. It reaches the file only when you save. Remove?"}
	tRemoveEdgeAsk = T{
		KO: "이 통신 엣지를 표에서 지웁니다. 저장해야 파일에 반영됩니다. 지울까요?",
		EN: "This removes the communication edge from the table. It reaches the file only when you save. Remove?"}

	// 다시 불러오기 — 편집을 버리고 파일에 있는 것으로 되돌린다.
	tReload    = T{KO: "다시 불러오기", EN: "Reload"}
	tReloadAsk = T{
		KO: "저장하지 않고 고친 것을 버리고, 마지막으로 저장된 선언을 다시 불러옵니다. 계속할까요?",
		EN: "This discards edits you have not saved and loads the declaration as it was last saved. Continue?"}

	tAddNode  = T{KO: "노드 추가", EN: "Add node"}
	tAddAsset = T{KO: "자산 추가", EN: "Add asset"}
	tAddEdge  = T{KO: "엣지 추가", EN: "Add edge"}
	tAddRow   = T{KO: "행 추가", EN: "Add row"}
)

// ── 선언 ───────────────────────────────────────────────────────────────────
var (
	// **설명은 접어 둔다.** 이 화면은 날마다 쓰는 자리지 읽는 자리가 아니다 — 문장을
	// 늘어놓으면 적을 칸이 밀린다. 필요한 사람만 「도움말」을 펴서 본다.
	tTip = T{KO: "도움말", EN: "Help"}

	tDeclOrg     = T{KO: "조직", EN: "Organization"}
	tDeclOrgHint = T{
		KO: "대조와 판정이 이 조직에 묶입니다. 비우면 <code>local</code>입니다.",
		EN: "Reconciliation and judgments are bound to this org. Empty means <code>local</code>."}

	tDeclScope     = T{KO: "관리 대상 노드와 그 안의 암호 자산", EN: "Nodes under management, and the crypto assets inside them"}
	tDeclScopeHint = T{
		KO: "<b>여기 적은 노드가 대조 대상입니다.</b> 여기 없는 노드와 통신한 것이 관측되면 " +
			"대조 화면에 「등재 판정 요청」으로 올라옵니다. 노드 <b>안의</b> 무엇을 볼지는 " +
			"「암호 자산 스코프」 탭에서 정합니다.",
		EN: "<b>The nodes listed here are what gets reconciled.</b> If traffic to a node that " +
			"is not listed here is observed, it shows up on the reconciliation screen as " +
			"<b>needs an enrollment decision</b>. What to look at <b>inside</b> a node is " +
			"decided on the Asset scope tab."}
	tDeclNodesHint = T{
		KO: "IP는 <b>관측에 찍힌 주소를 이 이름과 잇는 근거</b>입니다. 한 노드가 망 둘에 " +
			"걸치면 IP도 둘이니 쉼표나 공백으로 나눠 적으십시오. <b>이름이나 IP를 비운 줄은 " +
			"저장하지 않습니다</b>. 이을 근거가 없으면 대조 결과가 오류 없이 " +
			"틀리기 때문입니다. 그 칸을 채우십시오. 노드를 지우려면 「제거」를 누르십시오.<br>" +
			"<b>「관측 이름」은 관측이 이 노드를 부르는 이름입니다.</b> 자산 대조는 노드 " +
			"이름이 글자 그대로 같아야 맞는데, collector는 자기가 붙인 " +
			"id(<code>node:1a2b…</code>)나 호스트명으로 보내는 일이 흔합니다. 이름이 서로 다르면 " +
			"<b>그 노드의 자산이 통째로 UNDECLARED로</b> 올라옵니다. 선언이 틀려서가 아니라 " +
			"이름이 서로 달라서입니다. 호스트명(짧은 이름 포함)이 위 이름과 같으면 비워 두십시오. " +
			"그때는 저절로 이어집니다. 아직 어느 노드에도 붙지 않은 관측 이름은 " +
			"이 칸에서 고를 수 있습니다.",
		EN: "The IP is <b>what ties an observed address back to this name</b>. A node on two " +
			"networks has two IPs — separate them with commas or spaces. <b>A row with an empty " +
			"name or IP is not saved</b> — without that tie, reconciliation comes out wrong " +
			"without erroring. Fill the field in. To delete a node, use Remove.<br>" +
			"<b>“Observed as” is what observation calls this node.</b> Asset reconciliation " +
			"matches node names literally, but a collector often reports its own " +
			"id (<code>node:1a2b…</code>) or a hostname. When the names diverge, <b>every asset " +
			"on that node comes up as UNDECLARED</b> — not because the declaration is wrong but " +
			"because the names differ. Leave it empty when the hostname (or its short form) " +
			"equals the name above; those tie themselves. Names observed but not yet tied to " +
			"any node are offered here as candidates."}

	tDeclAssets     = T{KO: "암호 자산", EN: "Crypto assets"}
	tDeclAssetsHint = T{
		KO: "노드 안의 <b>암호 자산</b>은 그 노드에서 쓰인다고 선언한 런타임과 " +
			"컴포넌트입니다. 관측에 나와야 할 모듈입니다. 런타임은 " +
			"<b>목록에서 고릅니다</b>. 관측 결과에 나올 수 있는 이름만 들어 있습니다.<br>" +
			"<b>컴포넌트는 관측된 이름과 글자 그대로 같아야 맞습니다.</b> 앞부분만 같거나 " +
			"뒷부분만 같은 것은 맞지 않고, <code>*</code> 같은 것도 쓸 수 없습니다. 적을 때는 " +
			"관측 이름에서 <code>.so</code>부터 뒤를 뗍니다. <code>libssl.so.3</code>은 " +
			"<code>libssl</code>로 적습니다. <b>벤더링 해시는 떼지 않습니다</b>. " +
			"<code>libcrypto-fbc9a285.so.3</code>은 <code>libcrypto-fbc9a285</code>여야 하고, " +
			"<code>libcrypto</code>는 다른 자산입니다. <code>jca</code>의 컴포넌트는 " +
			"<code>jca-provider-chain</code> 하나, <code>cng</code>는 " +
			"<code>cng-providers</code> 하나입니다. 기계에 하나뿐이라 관측 결과에 늘 그 " +
			"이름으로 적힙니다.<br>맞지 않아도 막히지 않습니다. " +
			"<b>선언한 것은 미관측으로, 관측된 것은 UNDECLARED로</b> 구분됩니다. 그래서 " +
			"<b>그 노드에서 관측된 이름이 컴포넌트 칸에 후보로 뜹니다</b>. 칸을 " +
			"누르면 뜨고, 거기서 고르면 옮겨 적다 틀릴 일이 없습니다. 관측이 아직 없으면 " +
			"후보도 없으니 위 규칙대로 적으십시오.",
		EN: "The <b>crypto assets</b> inside a node are the runtime and component you declare " +
			"it uses — the modules you expect observation to find. The runtime is " +
			"<b>picked from a list</b>: only names observation can produce are in it.<br>" +
			"<b>The component must equal the observed name exactly.</b> A shared prefix or " +
			"suffix does not match, and there are no wildcards. Write the observed name with " +
			"everything from <code>.so</code> onwards removed — <code>libssl.so.3</code> is " +
			"written <code>libssl</code>. <b>A vendoring hash stays</b>: " +
			"<code>libcrypto-fbc9a285.so.3</code> must be <code>libcrypto-fbc9a285</code>, and " +
			"<code>libcrypto</code> is a different asset. Under <code>jca</code> the component " +
			"is always <code>jca-provider-chain</code>, and under <code>cng</code> always " +
			"<code>cng-providers</code> — one per machine, so observation reports that " +
			"name.<br>A mismatch does not stop you — " +
			"<b>what you declared goes UNOBSERVED and what was observed goes UNDECLARED</b>. So the " +
			"component field <b>offers the names observed on that node</b> — click the field and " +
			"pick one, and there is nothing to copy wrongly. With no observations yet there are " +
			"no candidates, so write them by the rule above."}
	tDeclEdges     = T{KO: "통신 엣지", EN: "Communication edges"}
	tDeclEdgesHint = T{
		KO: "<b>이 노드가 저 노드와 이렇게 통신한다</b>는 선언입니다. 포트까지 같아야 관측된 엣지와 맞습니다.",
		EN: "This node talks to that node like this — the port must match too."}

	tDeclSave     = T{KO: "저장", EN: "Save"}
	tDeclSaveHint = T{
		KO: "저장하면 선언 파일에 씁니다. 파일 이름은 버튼 옆에 있고, 전체 경로는 설정에서 볼 수 있습니다.",
		EN: "Saving writes to the declaration file. Its name is next to the buttons, and the full path is in the settings."}
	tSaveTarget = T{KO: "저장하는 파일:", EN: "Saves to:"}

	// 관측 결과 폴더가 비어 있을 때. 결과를 넣으면 화면을 다시 불러오는 것만으로 대조에 들어간다(세션은 읽을 때마다 다시 세운다).
	tEmptyResultsHead = T{KO: "대조할 성공 관측 결과가 아직 없습니다", EN: "No successful observation result to reconcile with yet"}
	tEmptyResultsBody = T{
		KO: "01 관측이 성공으로 끝나거나 pqcota 수집이 이 폴더에 결과를 쓰면, 이 화면을 다시 불러왔을 때 대조와 판정에 들어갑니다.",
		EN: "Once an observation on 01 finishes successfully, or pqcota collection writes its results to this folder, reloading this screen takes them into reconciliation and review."}

	// 선언 문제 목록(decl.Check). 막지 않고 짚는다.
	tDeclProblemsHead = T{KO: "앞뒤가 맞지 않는 자리", EN: "Places that do not add up"}
	tDeclProblemsLede = T{
		KO: "형식이 틀린 값은 저장할 때 거절합니다. 그 밖의 자리는 저장을 막지 않지만, 그대로 두면 대조 결과가 오류 없이 틀리므로 아래 칸을 고친 뒤 저장하십시오.",
		EN: "Malformed values are refused when you save. The other places do not block saving, but left as is, reconciliation is wrong without any error, so fix these fields and save."}
	// 저장을 거절했을 때(declform.go). 무엇이 틀렸는지와 함께 **파일과 적은 값이 어떻게 됐는지**를 말한다.
	tDeclRefusedHead = T{KO: "저장하지 않았습니다", EN: "Not saved"}
	tDeclRefusedLede = T{
		KO: "선언 파일은 그대로이고, 적으신 값은 이 화면에 남아 있습니다. 아래 칸을 고친 뒤 다시 저장하십시오.",
		EN: "The declaration file is unchanged, and what you typed is still on this screen. Fix the fields below and save again."}
	tFieldNameMissing = T{
		KO: "이름이 비어 있습니다. 이 노드와 그 안의 암호 자산을 지우려면 「제거」를 누르십시오.",
		EN: "The name is empty. To delete this node and the crypto assets inside it, use Remove."}
	tFieldIPMissing = T{
		KO: "IP가 비어 있습니다. 관측된 주소를 이 노드로 이으려면 IP가 있어야 합니다. 이 노드와 그 안의 암호 자산을 지우려면 「제거」를 누르십시오.",
		EN: "The IP is empty. An IP is needed to link observed addresses to this node. To delete this node and the crypto assets inside it, use Remove."}
	tFieldIPMalformed = T{
		KO: "IP 주소가 아닙니다: %s. 주소만 적고, 여럿이면 쉼표나 공백으로 나눕니다. 호스트명과 포트는 적지 않습니다.",
		EN: "Not an IP address: %s. Enter addresses only, separated by commas or spaces. Do not enter hostnames or ports."}
	tFieldComponentMissing = T{
		KO: "컴포넌트가 비어 있습니다. 이 자산을 지우려면 「제거」를 누르십시오.",
		EN: "The component is empty. To delete this asset, use Remove."}
	tFieldEdgeSrcMissing = T{
		KO: "보내는 쪽이 비어 있습니다. 이 엣지를 지우려면 「제거」를 누르십시오.",
		EN: "The sending side is empty. To delete this edge, use Remove."}
	tFieldEdgeDstMissing = T{
		KO: "받는 쪽이 비어 있습니다. 이 엣지를 지우려면 「제거」를 누르십시오.",
		EN: "The receiving side is empty. To delete this edge, use Remove."}
	tFieldPortMalformed = T{
		KO: "포트가 아닙니다: %s. 0부터 65535까지의 숫자를 적습니다.",
		EN: "Not a port: %s. Enter a number from 0 to 65535."}
	// 자산 스코프 화면: 규칙을 고친 채 「저장만」이나 「확정」을 눌렀을 때. 그 버튼들은 규칙을 쓰지 않는다.
	tRulesUnsavedBody = T{
		KO: "고친 규칙이 아직 계층 파일에 저장되지 않았습니다. 먼저 「규칙 저장」을 누르십시오. 「규칙 저장」은 아래에 적은 결론도 함께 저장합니다. 적으신 값은 이 화면에 남아 있습니다.",
		EN: "The rules you changed are not saved to the layer files yet. Use “Save rules” first. It saves the conclusions you wrote below as well. What you typed is still on this screen."}
	tRulesUnsavedGo = T{KO: "「규칙 저장」 버튼으로 가기", EN: "Go to the “Save rules” button"}
	// 편집 양식의 버튼 줄에 서는 표시(app.js가 넣는다). 떠날 때 묻는 창의 글은 브라우저가 정한다.
	tUnsaved = T{KO: "저장하지 않은 변경이 있습니다", EN: "There are unsaved changes"}
	// 칸의 title. 브라우저가 pattern에 맞지 않는 값을 막을 때 함께 보인다.
	tFieldIPTitle = T{
		KO: "IP 주소만 적습니다. 여럿이면 쉼표나 공백으로 나눕니다.",
		EN: "IP addresses only, separated by commas or spaces."}
	tFieldPortTitle = T{KO: "0부터 65535까지의 숫자", EN: "A number from 0 to 65535"}
	tDeclProblemsIf = T{KO: "그대로 두면", EN: "If left as is"}

	tColName = T{KO: "이름", EN: "Name"}
	tColIP   = T{KO: "IP", EN: "IP"}
	// **관측이 이 노드를 뭐라고 부르는가.** 자산 대조는 노드 이름이 글자 그대로 같아야
	// 맞는데, collector는 자기가 붙인 id나 호스트명으로 보낸다.
	tColObservedAs       = T{KO: "관측 이름", EN: "Observed as"}
	tColObservedAsHolder = T{KO: "호스트명이 이름과 같으면 비워 둡니다", EN: "leave empty if the hostname matches the name"}
	tColNode             = T{KO: "노드", EN: "Node"}
	tColRuntime          = T{KO: "런타임", EN: "Runtime"}
	tColComponent        = T{KO: "컴포넌트", EN: "Component"}
	tColSrc              = T{KO: "보내는 쪽", EN: "From"}
	tColDst              = T{KO: "받는 쪽", EN: "To"}
	tColPort             = T{KO: "포트", EN: "Port"}
	tColProto            = T{KO: "프로토콜", EN: "Protocol"}
)

// ── 리뷰 큐 ────────────────────────────────────────────────────────────────
var (
	tReviewItems            = T{KO: "항목", EN: "items"}
	tReviewMandatory        = T{KO: "필수", EN: "mandatory"}
	tReviewPolicyConclusion = T{KO: "이 정책의 결론", EN: "Conclusion for this policy"}
	tReviewQueueHint        = T{
		KO: "정책마다 결론을 하나 적으면 그 아래 항목이 한 번에 판정됩니다. 수천 대를 " +
			"한 건씩 보는 리뷰는 끝나지 않습니다. 「개별 결론(예외)」은 그 정책에서 " +
			"어긋나는 것만 따로 적는 자리입니다.<br><b>「플랫폼 조치」가 붙은 항목</b>은 " +
			"provider를 갈아 끼우는 자리가 아닙니다. <code>cng</code>가 그렇습니다. " +
			"쓸 수 있는 알고리즘은 Windows 빌드가, FIPS는 OS 정책이 정하므로 계획의 " +
			"provider 칸이 빕니다. 무엇을 할지는 여기 적는 결론이 담습니다.",
		EN: "One conclusion per policy judges every item under it at once — a review that " +
			"looks at thousands of machines one by one never ends. The per-item column is " +
			"for the exceptions that do not follow the policy.<br><b>Items marked “platform " +
			"action”</b> are not answered by swapping a provider — <code>cng</code> is one. " +
			"Which algorithms are available is set by the Windows build and FIPS by OS " +
			"policy, so the plan's provider field stays empty. What to do goes in the " +
			"conclusion here."}
	tReviewPlaceholder = T{
		KO: "예: PQC 라이브러리로 교체한다", EN: "e.g. replace with a PQC library"}

	tColTarget          = T{KO: "대상", EN: "Target"}
	tColState           = T{KO: "상태", EN: "State"}
	tColConfidence      = T{KO: "신뢰도", EN: "Confidence"}
	tColInPlan          = T{KO: "계획에 넣기", EN: "In plan"}
	tColException       = T{KO: "개별 결론(예외)", EN: "Per-item conclusion (exception)"}
	tColKind            = T{KO: "조치 종류", EN: "Remediation kind"}
	tColTarget2         = T{KO: "목표 알고리즘", EN: "Target algorithm"}
	tColLevel           = T{KO: "위임 수준", EN: "Delegation level"}
	tKindAuto           = T{KO: "고르지 않음", EN: "not chosen"}
	tHooks              = T{KO: "활성화 훅 (L3 전용)", EN: "Activation hooks (used at L3 only)"}
	tHooksKept          = T{KO: "이 항목에는 활성화 훅에 적어 둔 값이 있습니다. 위임 수준이 L3일 때만 쓰이고, L3를 고르면 다시 보입니다.", EN: "This item has activation hooks filled in. They are used only at delegation level L3, and show again when you pick L3."}
	tInstallsHead       = T{KO: "같은 자산 키 아래 설치(경로를 확인하지 못한 설치 후보 포함)가 둘 이상입니다. 이 판정과 조치는 아래 설치 전부에 적용됩니다.", EN: "There is more than one installation under this asset key (counting candidates whose path could not be confirmed). This judgment and its action apply to all of the installations below."}
	tPrevHead           = T{KO: "지난 판정(이전 회차의 참고값이며 현재 값이 아닙니다):", EN: "Earlier judgment (from the previous round; a reference value, not the current one):"}
	tPrevConclusion     = T{KO: "결론", EN: "conclusion"}
	tPrevInPlan         = T{KO: "· 계획에 넣었음:", EN: "· was in the plan:"}
	tPrevPolicy         = T{KO: "지난 정책 결론(이전 회차의 참고값):", EN: "Earlier policy conclusion (from the previous round; a reference value):"}
	tAdopt              = T{KO: "다시 채택", EN: "Adopt again"}
	tRulesetRoundHead   = T{KO: "이 세션은 지금과 다른 상류 규칙 판으로 열렸습니다", EN: "This session was opened under a different upstream ruleset"}
	tRulesetRoundBody   = T{KO: "자산 ID와 스냅샷 지문이 다른 규칙으로 셈해져 그대로는 저장 · 확정하지 않습니다. 새 회차를 열면 옛 세션을 보관하고, 지난 결론은 참고값으로만 옮기며, 변환 보고서를 씁니다.", EN: "Asset ids and snapshot fingerprints were computed by different rules, so it is not saved or finalized as it is. A new round archives the old session, carries earlier conclusions only as reference values, and writes a change report."}
	tRulesetRoundDo     = T{KO: "지금 규칙으로 새 회차 시작", EN: "Start a new round under the current rules"}
	tReportHead         = T{KO: "규칙 전환 보고서", EN: "Ruleset change report"}
	tReportNote         = T{KO: "상류 규칙 판이 바뀌었습니다. 아래에 사라짐 · 생김 · 바뀜으로 보이는 것은 규칙이 바뀐 것만으로 생겼을 수 있고, 그것만으로 설치가 없어지거나 생기거나 바뀐 것은 아닙니다.", EN: "The upstream ruleset changed. What shows below as removed, added or changed may come from the rule change alone, and is not by itself an installation removed, added or changed."}
	tReportFrom         = T{KO: "옛 규칙 판과 보고서:", EN: "Old ruleset and report:"}
	tScopeRoundHead     = T{KO: "이 회차를 연 정책 변경", EN: "The policy change that opened this round"}
	tScopeRoundPolicy   = T{KO: "자산 스코프 정책:", EN: "Asset-scope policy:"}
	tScopeBasisUpstream = T{KO: "근거: pqcota 이력 대조. 확인 시점에 pqcota 이력과 일치했습니다(노드마다 최신 적재 결과가 이 정책으로 만든 결과와 같았습니다).", EN: "Basis: pqcota history reconciliation. At the time of checking, the pqcota history matched (every node's latest ingest equalled the results under this policy)."}
	tScopeBasisOperator = T{KO: "근거: 운영자 진술(v0.27.x 이하에서 연 회차의 과거 기록). pqcota 이력과 대조하지 않았습니다.", EN: "Basis: an operator statement (a past record of a round opened by v0.27.x or earlier). It was not reconciled with the pqcota history."}
	tReportInstalls     = T{KO: "설치 %d개", EN: "%d installations"}
	tReportMissing      = T{KO: "보고서 파일을 읽지 못했습니다.", EN: "The report file could not be read."}
	tChangeOnlyOld      = T{KO: "지금 규칙에서는 없음", EN: "not under the current rules"}
	tChangeOnlyNew      = T{KO: "지금 규칙에서 새로 생김", EN: "new under the current rules"}
	tChangeChanged      = T{KO: "근거가 바뀜", EN: "evidence changed"}
	tInstallUnknownPath = T{KO: "경로를 확인하지 못한 설치 후보:", EN: "Installation candidate with no confirmed path:"}
	tInstallSeenBy      = T{KO: "관측 출처:", EN: "Observed from:"}
	tInstallsAll        = T{KO: "이 조치가 아래 설치 전부에 적용됨을 확인합니다", EN: "I confirm this action applies to all of the installations below"}
	tInstallsEach       = T{KO: "설치마다 다른 조치가 필요합니다 (이 판에서는 계획에 넣지 못합니다)", EN: "Each installation needs a different action (this version cannot put that in a plan)"}
	tHookPre            = T{KO: "내리기", EN: "pre"}
	tHookActivate       = T{KO: "활성화", EN: "activate"}
	tHookDeact          = T{KO: "되돌리기", EN: "deactivate"}
	tHookRestart        = T{KO: "재시작", EN: "restart"}
	tRescan             = T{KO: "재수집 후보", EN: "rescan candidate"}
	// **provider를 갈아 끼우는 조치가 아닌 자리.** 계획의 provider 칸이 비는데, 화면이
	// 말하지 않으면 빠뜨린 것으로 읽힌다.
	tPlatformFix = T{KO: "플랫폼 조치", EN: "platform action"}

	tReviewEmpty    = T{KO: "판정할 것이 없습니다.", EN: "Nothing to judge."}
	tReviewAutopass = T{
		KO: "자동통과 후보 %d개는 이 큐에 올리지 않았습니다. 선언과 맞고 신뢰도가 높은 " +
			"항목이라 하나씩 볼 필요가 없습니다.",
		EN: "%d auto-pass candidates are not in this queue — they match the declaration " +
			"with high confidence, so they are not worth looking at one by one."}

	tAutopassLegend = T{KO: "기계가 답한 항목 (계획에는 넣을 수 있습니다)", EN: "Items the machine answered (they can still go into the plan)"}
	tAutopassHint   = T{
		KO: "선언과 맞고 신뢰도가 높아 하나씩 판정할 필요가 없는 항목입니다. 판정 칸은 없습니다. " +
			"다만 「볼 필요가 없다」는 「바꿀 필요가 없다」가 아닙니다. 확인된 자산이야말로 조치 대상이 " +
			"되기 쉬우므로, 계획에 넣고 조치 종류·목표·위임 수준을 고를 수 있습니다.",
		EN: "These match the declaration with high confidence, so they need no per-item judgment - there is " +
			"no conclusion field. But not needing a look is not the same as not needing a change: confirmed " +
			"assets are the most natural targets, so they can be put in the plan with a kind, target and level."}
	tLegacyAutopass = T{
		KO: "옛 세션의 자동통과 후보 %d개는 식별자만 남아 있어 계획에 넣을 수 없습니다. 결과에서 세션을 다시 열면 고를 수 있게 됩니다.",
		EN: "Listed by id only, from an older session: %d auto-pass candidates. They cannot be put in a plan; reopen the session from the results to make them selectable."}
	tRollbackNote = T{KO: "되돌림 메모", EN: "Rollback note"}
	// 원장의 계획 선택 행. 판정이 아니다 - 결론 칸 대신 이 말이 선다.
	tPlanSelectionRow = T{KO: "(계획에 넣기로 한 기록입니다. 판정이 아닙니다)", EN: "(recorded as put in a plan — not a judgment)"}

	tApproval           = T{KO: "승인", EN: "Approval"}
	tReviewApprovalHint = T{
		KO: "정책마다 <b>판정</b>(결론)을 적고, 여기에 <b>승인</b>자와 서명을 채운 뒤, " +
			"<b>확정</b>하면 계획이 만들어집니다. 셋이 다 있어야 확정됩니다.",
		EN: "Write a <b>judgment</b> for each policy, fill in the <b>approver</b> and " +
			"signature here, then <b>finalize</b> to emit the plan. All three are required."}
	tReviewer  = T{KO: "승인자", EN: "Approver"}
	tSignature = T{KO: "서명", EN: "Signature"}

	tSaveOnly       = T{KO: "저장만", EN: "Save only"}
	tFinalizePlan   = T{KO: "확정하고 계획 내보내기", EN: "Finalize and emit the plan"}
	tReviewGateHint = T{
		KO: "확정은 <code>pqcaton-decide close</code>와 같은 검사를 거칩니다. " +
			"필수 항목의 결론과 서명이 모두 있어야 통과합니다.",
		EN: "Finalizing goes through the same gate as <code>pqcaton-decide close</code> — " +
			"every mandatory item needs a conclusion, and the signature must be present."}
)

// ── 자산 스코프 ────────────────────────────────────────────────────────────
var (
	tScopeIntro = T{
		KO: "이 화면에서는 <b>노드 안의 무엇을 계속 볼지</b>를 정합니다. " +
			"인벤토리에서 뺀 자산은 나중에 「왜 이것은 안 봤나」에 답해야 하므로, " +
			"<b>왜 뺐는지를 적어야 확정됩니다.</b>",
		EN: "This is where you decide <b>what to keep looking at inside a node</b>. " +
			"An asset you drop from the inventory is something you will have to account " +
			"for later, so <b>you must record why you dropped it before it can be finalized.</b>"}
	tScopeDelta = T{
		KO: "여기에는 <b>지금 쓰는 정책과 달라진 규칙만</b> 올라옵니다. " +
			"바뀌지 않은 규칙까지 매번 다시 승인하게 하면 아무도 들여다보지 않습니다.",
		EN: "Only <b>rules that differ from the policy in force</b> come up for judgment. " +
			"If unchanged rules had to be re-approved every time, nobody would read them."}

	tRuleLegend = T{KO: "규칙을 적는 법", EN: "How to write a rule"}
	tColMeaning = T{KO: "칸", EN: "Column"}
	tColMeans   = T{KO: "뜻", EN: "Meaning"}

	tRuleAction = T{
		KO: "<b>exclude</b>: 이 자산을 인벤토리에서 뺍니다. 나중에 「왜 이것은 안 봤나」에 " +
			"답해야 하므로, 아래에서 <b>왜 뺐는지를 적어야 확정됩니다.</b> " +
			"<b>include</b>: 다시 넣습니다. 위 계층이 뺀 것을 되돌릴 때 씁니다.",
		EN: "<b>exclude</b> — drop this asset from the inventory. You will have to account " +
			"for it later, so <b>you must record below why you dropped it</b> before it can " +
			"be finalized. <b>include</b> — put it back, to undo an exclusion from a layer above."}
	tRuleRuntime = T{
		KO: "런타임 이름. <code>openssl</code> · <code>jca</code> · <code>cng</code> 같은 것. 비우면 <b>전부</b>입니다",
		EN: "Which runtime — <code>openssl</code>, <code>jca</code>, <code>cng</code>. Empty means <b>all</b>"}
	tRuleLib = T{
		KO: "라이브러리 이름. <code>*</code>를 쓸 수 있습니다. <code>libcrypto.so.*</code>",
		EN: "Library name. <code>*</code> is allowed — <code>libcrypto.so.*</code>"}
	tRuleAppKey = T{
		KO: "그 라이브러리를 쓰는 실행 파일. <code>/usr/bin/python*</code> · <code>/usr/sbin/sshd</code>",
		EN: "The executable using it. <code>/usr/bin/python*</code>, <code>/usr/sbin/sshd</code>"}
	tRuleNote = T{
		KO: "사람이 읽는 설명입니다. <b>판정의 근거가 아닙니다.</b> 근거는 아래 결론 칸에 적습니다",
		EN: "A note for people to read. <b>Not the reason for a judgment</b> — that goes in the conclusion field below"}

	tRuleEmptyWarn = T{
		KO: "<b>빈 칸은 「전부」입니다.</b> 그래서 <code>runtime</code>·<code>lib</code>·" +
			"<code>app_key</code> 세 칸이 모두 빈 줄은 저장하지 않고 그 줄을 짚습니다. 그대로 쓰면 " +
			"<code>exclude</code> 하나로 <b>인벤토리가 통째로 빕니다.</b> 「전부」를 뜻하려면 " +
			"<code>*</code>를 적으십시오. 아무것도 적지 않은 새 줄은 버립니다.",
		EN: "<b>An empty cell means “all”.</b> That is why a row with " +
			"<code>runtime</code>, <code>lib</code> and <code>app_key</code> all empty is not " +
			"saved, and the screen points at it — as written, a single <code>exclude</code> would " +
			"<b>empty the entire inventory.</b> Write <code>*</code> if you really mean all. " +
			"A new row with nothing typed is dropped."}
	tRuleLayerOrder = T{
		KO: "<b>같은 자산에 규칙이 여럿 걸리면 아래 계층의 것이 적용됩니다.</b> " +
			"계층은 위에서 아래로 겹치기 때문입니다(조직 → 환경 → 노드군). 그래서 위 계층이 " +
			"뺀 자산을 아래 계층에서 <code>include</code>로 되돌릴 수 있고, 그 되돌림도 승인을 거칩니다.",
		EN: "<b>When several rules match one asset, the lower layer wins.</b> Layers stack " +
			"top to bottom (org → environment → node group). So a lower layer can put back " +
			"an asset an upper layer excluded, and that reversal goes through approval too."}
	tScopeTableOrder = T{
		KO: "아래 표의 순서가 곧 계층 순서입니다. <b>겹치면 아래쪽이 적용됩니다.</b>",
		EN: "The order of the tables below is the layer order — <b>on a clash, the lower one wins.</b>"}

	tLayer         = T{KO: "계층", EN: "Layer"}
	tLayerRules    = T{KO: "규칙", EN: "rules"}
	tSaveRules     = T{KO: "규칙 저장", EN: "Save rules"}
	tSaveRulesHint = T{
		KO: "저장하면 계층 파일에 그대로 쓰고, <b>그 결과 무엇이 달라지는지를 아래에 다시 " +
			"보여 줍니다.</b> 적어 둔 판정은 그대로 남습니다. 규칙 자체가 달라진 것만 " +
			"다시 판정하면 됩니다.",
		EN: "Saving writes straight to the layer files and then <b>shows below what that " +
			"changes.</b> Judgments you already wrote stay — only rules that actually " +
			"changed need judging again."}
	tScopeEditResult = T{KO: "고친 결과: 승인할 변경", EN: "What your edits change — for approval"}
	tRemoveRuleHint  = T{
		KO: "규칙을 지우려면 그 줄의 「제거」를 누르십시오. 「삭제 예정」으로 표시되고, 「규칙 저장」을 눌러야 파일에서 빠집니다.",
		EN: "To delete a rule, use Remove on its row. It is marked “To be removed” and leaves the file only when you use Save rules."}
	tRemoveRuleAsk = T{
		KO: "이 규칙을 삭제 예정으로 표시합니다. 「규칙 저장」을 눌러야 파일에 반영됩니다. 표시할까요?",
		EN: "This marks the rule to be removed. It reaches the file only when you use Save rules. Mark it?"}
	tRuleRemoving   = T{KO: "삭제 예정", EN: "To be removed"}
	tRuleUndo       = T{KO: "되돌리기", EN: "Undo"}
	tFieldRuleEmpty = T{
		KO: "조건(runtime · lib · app_key)이 모두 비어 있습니다. 전체에 적용하려면 별표(*)를 적으십시오. 이 규칙을 지우려면 「제거」를 누르십시오.",
		EN: "The conditions (runtime · lib · app_key) are all empty. To apply to everything, write *. To delete this rule, use Remove."}
	tRulesRefusedLede = T{
		KO: "계층 파일과 세션 파일은 그대로이고, 적으신 값은 이 화면에 남아 있습니다. 아래 줄을 고친 뒤 「규칙 저장」을 누르십시오.",
		EN: "The layer files and the session file are unchanged, and what you typed is still on this screen. Fix the rows below and use Save rules."}

	tChanges          = T{KO: "변경", EN: "changes"}
	tReasonNeeded     = T{KO: "근거를 적어야 하는 것", EN: "need a recorded reason"}
	tColChange        = T{KO: "변경", EN: "Change"}
	tColRule          = T{KO: "규칙", EN: "Rule"}
	tColNote          = T{KO: "설명", EN: "Note"}
	tKindAdded        = T{KO: "추가", EN: "added"}
	tKindRemoved      = T{KO: "제거", EN: "removed"}
	tLayerConclusion  = T{KO: "이 계층의 결론", EN: "Conclusion for this layer"}
	tLayerPlaceholder = T{
		KO: "예: OS 패치로 관리하므로 인벤토리에서 뺀다",
		EN: "e.g. managed by OS patching, so kept out of the inventory"}
	tScopeEmpty = T{
		KO: "바뀐 규칙이 없습니다. 승인할 것이 없습니다.",
		EN: "No rules changed — nothing to approve."}

	tScopeConclusionHint = T{
		KO: "계층마다 결론을 하나 적으면 그 계층의 변경이 한 번에 판정됩니다. " +
			"「개별 결론(예외)」은 그 계층에서 어긋나는 것만 따로 적는 자리입니다.",
		EN: "One conclusion per layer judges every change in it at once. The per-item " +
			"column is for the exceptions that do not follow the layer's conclusion."}

	tScopeApprovalHint = T{
		KO: "계층마다 <b>판정</b>(결론)을 적고, 여기에 <b>승인</b>자와 서명을 채운 뒤, " +
			"<b>확정</b>하면 정책이 만들어집니다. 셋이 다 있어야 확정됩니다.",
		EN: "Write a <b>judgment</b> for each layer, fill in the <b>approver</b> and " +
			"signature here, then <b>finalize</b> to emit the policy. All three are required."}
	tPolicyOnFinalize = T{KO: "확정될 정책", EN: "Policy to be finalized"}
	tPolicyRulesAll   = T{KO: "규칙 %d개 전부", EN: "all %d rules"}
	tPolicyWholeHint  = T{
		KO: "<b>판정은 달라진 규칙만 보지만, 확정하면 정책 전체가 만들어집니다.</b> " +
			"pqcota의 집행기가 일부가 아니라 정책 전체를 받기 때문입니다.",
		EN: "<b>You judge only what changed, but finalizing emits the whole policy.</b> " +
			"pqcota's enforcer takes the entire policy, not a fragment."}
	tFinalizePolicy = T{KO: "확정하고 정책 내보내기", EN: "Finalize and emit the policy"}
	tScopeGateHint  = T{
		KO: "확정은 <code>pqcaton-scope close</code>와 같은 검사를 거칩니다. 나온 CSV가 " +
			"<code>pqcota-ingest -scope-assets</code>의 입력입니다.",
		EN: "Finalizing goes through the same gate as <code>pqcaton-scope close</code>. " +
			"The CSV it emits is the input to <code>pqcota-ingest -scope-assets</code>."}
)

// ── 대조 ───────────────────────────────────────────────────────────────────
var (
	tSurveyObserved     = T{KO: "관측", EN: "Observation"}
	tSurveyObservedHint = T{
		KO: "pqcota가 무엇을 보았는지 보여 줍니다. 대상 노드에 collector를 가져가 실행하고 " +
			"회수했습니다. 노드에는 아무것도 남지 않습니다.",
		EN: "What pqcota saw. The collector was carried to the target node, run, and taken " +
			"back — nothing is left behind on the node."}
	tColCollector  = T{KO: "그 노드에서 실행된 collector", EN: "Collectors that ran on it"}
	tObservedCount = T{
		KO: "관측 자산 <b>%d</b> · 협상된 통신 <b>%d</b>건",
		EN: "<b>%d</b> assets observed · <b>%d</b> negotiated connections"}

	tNotSeen     = T{KO: "못 본 것", EN: "What was not seen"}
	tNotSeenNone = T{
		KO: "없습니다. 이 범위에서는 관측이 완전합니다.",
		EN: "Nothing — observation is complete for this scope."}
	tNotSeenLayer = T{
		KO: "이 계층을 아예 보지 못했습니다: ", EN: "This layer was never observed —"}
	tNotSeenEdges = T{
		KO: "이 노드의 통신을 보지 못했습니다: ", EN: "This node's traffic was not observed —"}
	tNotSeenHint = T{
		KO: "<b>못 본 것과 없는 것은 다릅니다.</b> 아래 UNOBSERVED가 「쓰지 않는 것」인지 " +
			"「보지 못한 것」인지는 위 목록에서 구분합니다. <b>위에 적힌 것이면 재수집이 먼저</b>이고, " +
			"아니면 사람이 판정할 차례입니다.",
		EN: "<b>Not seen is not the same as not there.</b> Whether an UNOBSERVED below means " +
			"“not used” or “not looked at” is settled by the list above — " +
			"<b>if it is listed there, collect again first</b>; otherwise it is for a person to judge."}
	tSkipped     = T{KO: "읽지 못한 결과 파일", EN: "Result files that could not be read"}
	tSkippedHint = T{
		KO: "빠진 노드를 모르면 「관측 안 됨」과 「못 읽음」이 뒤섞입니다.",
		EN: "If you do not know which nodes are missing, “not observed” and " +
			"“could not read” get mixed up."}

	tSurveyAssets     = T{KO: "암호 자산", EN: "Crypto assets"}
	tSurveyAssetsHint = T{
		KO: "노드 안의 암호 런타임·컴포넌트를 선언과 대조해 세 가지 상태로 구분한 결과입니다.",
		EN: "The crypto runtimes and components inside each node, in three states against the declaration."}
	tSurveyNoAssets = T{KO: "대조할 자산이 없습니다.", EN: "No assets to reconcile."}
	tUndeclaredHint = T{
		KO: "<b>UNDECLARED를 찾아내는 것이 이 도구의 첫 번째 쓸모입니다</b>. 선언에 없는데 " +
			"실제로 쓰이고 있는 것입니다. 판정은 <a href=\"/review\">판정(리뷰 큐)</a> 탭에서 합니다.",
		EN: "<b>Finding UNDECLARED is the first thing this tool is good for</b> — things in " +
			"actual use that nobody declared. Judge them in the " +
			"<a href=\"/review\">judgment (review queue)</a> tab."}

	tSurveyEdges     = T{KO: "통신 엣지", EN: "Communication edges"}
	tSurveyEdgesHint = T{
		KO: "선언과 대조해 구분한 세 가지 상태, 그리고 협상된 통신의 양자내성 등급입니다.",
		EN: "Three states against the declaration, and the quantum-resistance grade of " +
			"each negotiated connection."}
	tSurveyNoEdges = T{KO: "대조할 엣지가 없습니다.", EN: "No edges to reconcile."}
	tPostures      = T{KO: "🟢 PQC <b>%d</b> · 🔴 고전 <b>%d</b> · ⚪ 불명 <b>%d</b>",
		EN: "🟢 PQC <b>%d</b> · 🔴 classical <b>%d</b> · ⚪ unknown <b>%d</b>"}
	tColGroup       = T{KO: "협상 그룹", EN: "Negotiated group"}
	tColReconcile   = T{KO: "대조", EN: "Reconciled"}
	tGradeClassical = T{KO: "고전(양자 취약)", EN: "classical — quantum-vulnerable"}
	tGradeUnknown   = T{KO: "불명", EN: "unknown"}
	tUncoveredSrc   = T{KO: "관측 안 된 노드", EN: "node not observed"}
	tOffScope       = T{KO: "등재 판정 요청", EN: "needs enrollment decision"}
	tServerOnly     = T{KO: "받은 노드에서만 관측", EN: "observed on the accepting node only"}
	tColPeer        = T{KO: "상대", EN: "Peer"}

	tUnconfirmedEdges = T{
		KO: "방향을 확인하지 못한 연결: %d건",
		EN: "Connections whose direction was not confirmed: %d"}
	tUnconfirmedEdgesHint = T{
		KO: "수집기가 어느 쪽이 연결을 열었는지 확인하지 못한 기록입니다. 포트와 협상 그룹이 없어서 선언과 대조하지 않았고, 위 표와 " +
			"수에도 넣지 않았습니다. 다시 관측하면 확인될 수 있습니다.",
		EN: "The collector could not confirm which side opened these connections. They carry no port and no negotiated " +
			"group, so they are not reconciled with the declaration and are not counted above. Observing again may confirm them."}

	// 이력에서 앱별 통신 보기(02에서 여는 조회 화면).
	tHistTopoTitle    = T{KO: "이력에서 앱별 통신 보기", EN: "App connections from the history"}
	tHistTopoLinkHint = T{
		KO: "pqcota 이력에 적재된 관측에서 노드마다 어느 앱이 무엇과 통신했는지 봅니다. 위 그림과 달리 선언과 대조하지 않습니다.",
		EN: "See which app on each node talked to what, from the observations loaded into the pqcota history. Unlike the picture above, it is not reconciled with the declaration."}
	tHistBack       = T{KO: "02 대조로 돌아가기", EN: "Back to 02 reconciliation"}
	tHistSource     = T{KO: "출처", EN: "Source"}
	tHistSourceOrg  = T{KO: "조회한 조직(판정 조직)", EN: "Organization queried (the judgment organization)"}
	tHistSourceBody = T{
		KO: "이 조직의 pqcota 이력에서 읽었습니다. 노드마다 그 노드의 현재 스냅샷이고, 적재 시각은 노드마다 다릅니다. 작업 폴더의 관측 결과와 같은지는 같은 노드의 스냅샷 지문으로 견줍니다. 시각으로는 판단하지 않습니다.",
		EN: "Read from this organization's pqcota history. Each node shows its current snapshot, and load times differ per node. Whether it is the same as the observation results in the workspace is decided by the snapshot digest of the same node, never by the time."}
	tColHistSnap   = T{KO: "이력 스냅샷", EN: "History snapshot"}
	tColDigest     = T{KO: "스냅샷 지문", EN: "Snapshot digest"}
	tColHistTime   = T{KO: "적재 시각(UTC)", EN: "Loaded at (UTC)"}
	tColCompare    = T{KO: "작업 폴더와 비교", EN: "Against the workspace"}
	tHistCreated   = T{KO: "적재 기록이 없어 스냅샷 생성 시각입니다", EN: "no load record: this is when the snapshot was created"}
	tCmpSame       = T{KO: "일치", EN: "same"}
	tCmpDiffer     = T{KO: "다름", EN: "different"}
	tCmpNone       = T{KO: "비교 불가", EN: "cannot compare"}
	tCmpNoLocal    = T{KO: "작업 폴더에 이 노드의 결과가 없습니다", EN: "the workspace has no results for this node"}
	tCmpUnreadable = T{KO: "작업 폴더의 결과를 읽지 못했습니다", EN: "the results in the workspace could not be read"}
	tCmpDifferHint = T{
		KO: "작업 폴더의 결과가 이 스냅샷과 다릅니다. 적재한 뒤에 다시 관측했거나 다른 정책으로 적재한 경우입니다",
		EN: "the results in the workspace differ from this snapshot: observed again after the load, or loaded under another policy"}
	tHistOnlyLocal = T{
		KO: "작업 폴더에는 결과가 있고 이 조직의 이력에는 스냅샷이 없는 노드",
		EN: "Nodes with results in the workspace and no snapshot in this organization's history"}
	tHistEmpty     = T{KO: "이 조직의 pqcota 이력에 스냅샷이 없습니다.", EN: "The pqcota history holds no snapshot for this organization."}
	tHistEmptyNext = T{
		KO: "01에서 관측한 결과를 이 조직(<code>PQCOTA_ORG</code>)으로 <code>pqcota-ingest</code>에 적재한 뒤 이 화면을 다시 여십시오. 다른 조직으로 적재한 이력은 여기에 보이지 않습니다.",
		EN: "Load the results observed on 01 with <code>pqcota-ingest</code> under this organization (<code>PQCOTA_ORG</code>), then open this screen again. A history loaded under another organization does not show here."}
	tHistUnreachable = T{
		KO: "pqcota 이력을 읽지 못했습니다. 연결과 화면 계정의 조회 권한을 확인한 뒤 다시 여십시오.",
		EN: "The pqcota history could not be read. Check the connection and the read privileges of the screen's account, then open this screen again."}
	tHistEndpoints = T{
		KO: "이 조직에 등록된 엔드포인트: %d개. 상대의 IP가 등록된 엔드포인트 하나에만 속할 때 그 노드로 적고, 그렇지 않으면 주소로 적습니다. 선언이나 다른 조직의 등록 정보로 이름을 채우지 않습니다.",
		EN: "Endpoints registered for this organization: %d. A peer is written as a node only when its IP belongs to exactly one registered endpoint; otherwise it is written as an address. Names are not filled in from the declaration or from another organization's registrations."}
	tHistNoEndpoints = T{
		KO: "등록된 엔드포인트가 없어 모든 상대가 주소로 나옵니다. 노드로 이어 보려면 이 조직으로 <code>pqcota-hosts</code>를 돌려 엔드포인트를 등록하십시오.",
		EN: "No endpoint is registered, so every peer shows as an address. To tie peers to nodes, register the endpoints with <code>pqcota-hosts</code> under this organization."}
	tHistSummary = T{KO: "노드별 요약", EN: "Summary by node"}
	tHistCounts  = T{
		KO: "노드 %d대 · 저장된 연결 기록 %d건 · 노드로 잇지 못한 상대 %d곳",
		EN: "Nodes: %d · stored connection records: %d · peers not tied to a node: %d"}
	tHistRecordsNote = T{
		KO: "기록은 관측한 노드마다 하나씩 저장됩니다. 같은 연결을 두 노드가 보면 두 줄이므로, 이 수는 02의 「협상된 통신」 건수와 다릅니다. 횟수는 그 기록의 것이고 더하지 않습니다.",
		EN: "A record is stored per observing node. A connection seen by both nodes is two rows, so this number is not the “negotiated connections” count of 02. A count belongs to its record and is not added up."}
	tHistPick    = T{KO: "노드 고르기", EN: "Choose a node"}
	tHistAll     = T{KO: "모든 노드", EN: "All nodes"}
	tHistShow    = T{KO: "보기", EN: "Show"}
	tColApp      = T{KO: "앱", EN: "App"}
	tColLibs     = T{KO: "같은 앱 키로 관측된 라이브러리", EN: "Libraries observed with the same app key"}
	tHistLibNote = T{
		KO: "같은 스냅샷에서 같은 앱 키로 관측됐다는 뜻입니다. 그 연결이 이 라이브러리를 썼다는 근거가 아닙니다.",
		EN: "They were observed in the same snapshot with the same app key. That is not evidence that the connection used them."}
	tHistApps        = T{KO: "앱과 라이브러리", EN: "Apps and libraries"}
	tHistRecords     = T{KO: "저장된 연결 기록", EN: "Stored connection records"}
	tHistNoApps      = T{KO: "앱으로 귀속된 연결 기록이 없습니다.", EN: "No connection record is attributed to an app."}
	tHistNoLibs      = T{KO: "없음", EN: "none"}
	tHistNoRecords   = T{KO: "저장된 연결 기록이 없습니다.", EN: "No connection record is stored."}
	tColDirection    = T{KO: "방향", EN: "Direction"}
	tDirOpened       = T{KO: "→ 열었음", EN: "→ opened"}
	tDirAccepted     = T{KO: "← 받았음", EN: "← accepted"}
	tDirUnknown      = T{KO: "↔ 방향 미확인", EN: "↔ not confirmed"}
	tColWire         = T{KO: "통신", EN: "Connection"}
	tColCount        = T{KO: "횟수", EN: "Count"}
	tNoApp           = T{KO: "(앱 미귀속)", EN: "(no app)"}
	tGroupUnknown    = T{KO: "그룹 불명", EN: "group unknown"}
	tCountNone       = T{KO: "기록 없음", EN: "not recorded"}
	tPeerUnreg       = T{KO: "등록되지 않은 주소", EN: "unregistered address"}
	tPeerShared      = T{KO: "등록 노드 %d대가 함께 쓰는 IP", EN: "IP shared by registered nodes: %d"}
	tPeerNotIP       = T{KO: "IP가 아닌 주소", EN: "not an IP address"}
	tPeerNoSnap      = T{KO: "이력에 스냅샷 없음", EN: "no snapshot in the history"}
	tHistPicture     = T{KO: "그림(조직 전체)", EN: "Picture (the whole organization)"}
	tHistPictureNote = T{
		KO: "그림은 위에서 고른 노드와 관계없이 이 조직의 모든 노드를 그립니다. 넓으면 이 상자 안에서 좌우로 움직입니다. 선 하나는 저장된 기록 하나이고, 라벨은 상류 도구가 쓴 영어 그대로입니다. 그림은 표와 따로 그리며, 정한 시간이나 크기를 넘으면 그리지 않습니다.",
		EN: "The picture draws every node of this organization, whatever node is chosen above. When it is wide, scroll sideways inside this box. One line is one stored record, and the labels are the English the upstream tool writes. The picture is drawn apart from the table, and is not drawn when it exceeds the time or size limit."}
	tSVGRefused = T{
		KO: "그림에 화면이 싣지 않는 요소가 있어 보이지 않습니다. DOT을 내려받아 따로 그리십시오.",
		EN: "The drawing holds something this screen does not embed, so it is not shown. Download the DOT and draw it elsewhere."}
	tSVGFailed = T{
		KO: "<code>dot</code>이 그림을 그리지 못했습니다. DOT을 내려받아 확인하십시오.",
		EN: "<code>dot</code> could not draw the picture. Download the DOT to check it."}
	tSVGTimeout = T{
		KO: "그림을 그리는 데 정한 시간을 넘겨 그리지 않았습니다. DOT을 내려받아 따로 그리십시오.",
		EN: "Drawing the picture took longer than the limit, so it is not drawn. Download the DOT and draw it elsewhere."}
	tSVGTooLarge = T{
		KO: "그림이 화면에 싣는 크기 상한을 넘어 그리지 않았습니다. DOT을 내려받아 따로 그리십시오.",
		EN: "The picture is larger than this screen embeds, so it is not drawn. Download the DOT and draw it elsewhere."}
	tPicturePlace = T{
		KO: "그림은 이 화면과 따로 그려 이 자리에 들어옵니다. 그림이 없어도 위의 「DOT 내려받기」로 받아 다른 곳에서 그릴 수 있습니다.",
		EN: "The picture is drawn apart from this screen and arrives in this place. Without it, “Download DOT” above gives the source to draw elsewhere."}
	tSurveyPicturePending = T{
		KO: "그림을 그리고 있습니다. 위의 대조 결과와 표는 그림과 관계없이 볼 수 있습니다.",
		EN: "The picture is being drawn. The reconciliation and the tables above can be read regardless of the picture."}
	tSurveyPictureUnreadable = T{
		KO: "대조 결과를 읽지 못해 그림을 그리지 못했습니다. 02를 다시 여십시오.",
		EN: "The reconciliation could not be read, so the picture is not drawn. Open 02 again."}
	tHistPicturePending = T{
		KO: "그림을 그리고 있습니다. 위의 표와 DOT 내려받기는 그림과 관계없이 쓸 수 있습니다.",
		EN: "The picture is being drawn. The table above and the DOT download work regardless of the picture."}
	tDotDownload = T{KO: "DOT 내려받기", EN: "Download DOT"}

	tTopology     = T{KO: "토폴로지", EN: "Topology"}
	tTopologyHint = T{
		KO: "색은 양자내성 등급, 선 모양은 대조 상태입니다. <b>보지 못한 것은 점선</b>이라 " +
			"없는 것과 구분됩니다.",
		EN: "Colour is the quantum-resistance grade, line style the reconciliation state. " +
			"<b>What was not seen is dashed</b>, so it is distinct from what is absent."}
	tNoDot = T{
		KO: "<code>dot</code>(Graphviz)이 이 기계에 없어 그리지 못했습니다. " +
			"<b>선택 사항입니다.</b>",
		EN: "<code>dot</code> (Graphviz) is not on this machine, so the graph was not " +
			"drawn — <b>it is optional</b>."}
	tNoDotHelp = T{
		KO: "설치하지 않아도 나머지는 그대로 됩니다(README 「사전 준비」). " +
			"<code>apt install graphviz</code> · <code>brew install graphviz</code> · " +
			"<code>winget install graphviz</code>.<br>설치하지 않고 그리려면 아래를 저장해 " +
			"아무 데서나 <code>dot -Tsvg topology.dot -o topology.svg</code>로 그리십시오.",
		EN: "Everything else works without it (see README, “Prerequisites”). " +
			"<code>apt install graphviz</code>, <code>brew install graphviz</code>, " +
			"<code>winget install graphviz</code>.<br>To draw it without installing, save " +
			"the source below and run <code>dot -Tsvg topology.dot -o topology.svg</code> anywhere."}
)

// ── 인벤토리 조회 ──────────────────────────────────────────────────────────
var (
	tNavInventory   = T{KO: "인벤토리·판정 이력", EN: "Inventory & judgment history"}
	tTitleInventory = T{KO: "인벤토리·판정 이력", EN: "Inventory & judgment history"}

	tInventoryIntro = T{
		KO: "인벤토리에 지금 무엇이 있는지 <b>검색하는 화면</b>입니다. 선언 → 자산 스코프 → " +
			"대조 → 판정 순서를 거쳐 올 필요는 없습니다. 언제든 열어서 조건을 걸어 찾으면 됩니다. " +
			"<b>지난 판정과 그 근거도 여기서 봅니다</b>. 아래 「판정 이력」과 「근거가 바뀐 판정」입니다.",
		EN: "This screen <b>searches what is in the inventory right now</b>. You do not have to " +
			"arrive through the declaration → asset scope → reconciliation → judgment sequence — " +
			"open it whenever you like and narrow things down. <b>Past judgments and their basis " +
			"are here too</b> — see “judgment history” and “judgments whose basis changed” below."}
	tInventoryBounds = T{
		KO: "이 화면이 읽는 것은 <b>지금 있는 파일뿐입니다</b>. 관측 결과, 판정 원장, " +
			"정책 CSV. 여러 시점의 스냅샷을 쌓아 두고 시간에 따라 비교하는 기능은 없습니다.",
		EN: "This screen reads <b>only the files you have right now</b> — collected results, " +
			"the judgment ledger, the policy CSV. It does not keep snapshots from many points " +
			"in time and compare them."}

	tSearch      = T{KO: "찾기", EN: "Search"}
	tSearchQ     = T{KO: "노드 · 런타임 · 컴포넌트 · 상태로 찾습니다", EN: "node, runtime, component, state — any of them"}
	tSearchState = T{KO: "상태", EN: "State"}
	tSearchAll   = T{KO: "전부", EN: "all"}
	tSearchGo    = T{KO: "이 조건으로 찾기", EN: "Search"}
	tSearchClear = T{KO: "지우기", EN: "Clear"}
	// **자리를 번호로 고정한다.** 한국어는 「전체 중 몇 개」, 영어는 「몇 개 of 전체」로
	// 어순이 뒤집힌다 — 번호가 없으면 한쪽 말에서 숫자 둘이 자리를 바꿔 뜬다.
	tShowingOf = T{KO: "%[1]d개 중 <b>%[2]d개</b>", EN: "<b>%[2]d</b> of %[1]d"}
	tNoMatch   = T{KO: "찾은 것이 없습니다.", EN: "Nothing matched."}

	tUnseen     = T{KO: "안 보고 있는 것", EN: "What is not being looked at"}
	tUnseenNote = T{
		KO: "암호 자산 스코프가 뺀 것입니다.", EN: "What the crypto asset scope dropped."}
	tUnseenHint = T{
		KO: "<b>뺐다고 없어진 것이 아닙니다.</b> 지금도 관측되는 것에는 표시가 붙습니다. " +
			"승인이 없거나 오래된 것은 다시 봐야 합니다.",
		EN: "<b>Dropped is not gone.</b> Anything still observed is marked — " +
			"if the approval is missing or stale, it needs looking at again."}
	tUnseenNone    = T{KO: "정책이 뺀 자산이 없습니다.", EN: "The policy drops nothing."}
	tColStillSeen  = T{KO: "지금도 관측됨", EN: "Still observed"}
	tColWhyAgain   = T{KO: "다시 볼 이유", EN: "Why look again"}
	tReasonSettled = T{KO: "승인이 아직 유효합니다", EN: "the approval is still valid"}
	tReasonNever   = T{KO: "이 제외를 승인한 판정이 없습니다", EN: "no judgment ever approved this exclusion"}
	tReasonStaleKO = T{KO: "승인한 지 오래됐습니다. 빼 둔 사이 달라졌을 수 있습니다", EN: "the approval is stale — things may have changed while it was set aside"}
	tColEvidence   = T{KO: "관측 근거", EN: "Evidence"}

	tStale     = T{KO: "근거가 바뀐 판정", EN: "Judgments whose basis changed"}
	tStaleHint = T{
		KO: "재관측 뒤 <b>근거가 달라진 판정</b>입니다. 전면 재리뷰가 아니라 이것만 " +
			"다시 봅니다. <code>pqcaton-decide delta</code>와 같은 계산입니다.",
		EN: "These are <b>judgments resting on a basis that changed</b> after re-collection. " +
			"Not a full re-review — only these need looking at again, the same computation as " +
			"<code>pqcaton-decide delta</code>."}
	tStaleNone = T{KO: "근거가 바뀐 판정이 없습니다. 관측이 그대로입니다.", EN: "No judgment's basis changed — the observation has not moved."}

	tHistory     = T{KO: "판정 이력", EN: "Judgment history"}
	tHistoryHint = T{
		KO: "이 자산을 <b>누가 언제 무엇으로 정했는지</b> 봅니다. 원장에는 덧붙이기만 하므로 " +
			"한 번 남긴 판정은 고쳐 쓸 수 없습니다. 아래가 그 기록 그대로입니다.",
		EN: "<b>When, by whom, and as what</b> this asset was decided. The ledger is " +
			"append-only, so nothing is overwritten — what is below is the record itself."}
	tHistoryNone   = T{KO: "이 자산에 내려진 판정이 없습니다.", EN: "No judgment was ever recorded for this asset."}
	tOpenHistory   = T{KO: "이력", EN: "history"}
	tBackToAll     = T{KO: "전체로 돌아가기", EN: "Back to all"}
	tColDecided    = T{KO: "판정 시각(UTC)", EN: "Decided at (UTC)"}
	tColConclusion = T{KO: "결론", EN: "Conclusion"}
	tColBasis      = T{KO: "근거 해시", EN: "Basis hash"}

	tNoLedger = T{
		KO: "판정 원장을 주지 않았습니다. <code>-judgments</code>로 지정하면 이력과 " +
			"「근거가 바뀐 판정」이 열립니다.",
		EN: "No judgment ledger was given — pass <code>-judgments</code> to open the history " +
			"and the “basis changed” section."}
	tNoPolicy = T{
		KO: "정책 CSV를 주지 않았습니다. <code>-scope-out</code>(확정된 정책)을 주면 " +
			"「안 보고 있는 것」이 열립니다.",
		EN: "No policy CSV was given — pass <code>-scope-out</code> (the finalized policy) " +
			"to open “what is not being looked at”."}
)

// ── 선언 요약 (`/decl-next`) ────────────────────────────────────────────────
//
// **적는 자리 위에 「지금 무엇을 할지」를 얹는다.** 선언 편집은 칸이 많아서, 처음 여는
// 사람은 어디부터 손대야 하는지 알기 어렵다. 관측에 있는데 어느 노드에도 붙지 않은
// 이름이 그 답이고, 그것이 곧 대조가 통째로 어긋나는 자리다.
var (
	tNextDeclEyebrow = T{KO: "01 관측 · 대상 노드와 기대 상태", EN: "01 Observation · Target nodes and expected state"}
	tNextDeclHead    = T{
		KO: "관측한 노드를 선언과 연결합니다",
		EN: "Link observed nodes to the declaration"}
	tNextDeclLede = T{
		KO: "대조 전에 이름을 맞추면, 실제 자산을 다른 노드의 UNDECLARED로 오해하지 않습니다.",
		EN: "Match the names before reconciling and a real asset will not be read as " +
			"another node's UNDECLARED."}

	tNextUnlinked = T{
		KO: "선언 노드에 연결되지 않은 관측 이름이 %d개 있습니다",
		EN: "%d observed names are not linked to a declared node"}
	tNextUnlinkedBody = T{
		KO: "이 이름으로 관측된 자산은 어느 노드에도 붙지 않아 통째로 UNDECLARED로 오릅니다.",
		EN: "Assets observed under these names attach to no node, so they all come up as UNDECLARED."}
	tNextAllLinked = T{
		KO: "연결할 관측 이름이 없습니다",
		EN: "There is no observed name left to link"}
	tNextAllLinkedBody = T{
		KO: "관측에 나온 이름이 모두 선언 노드에 붙었습니다. 대조로 넘어가도 됩니다.",
		EN: "Every observed name is attached to a declared node. You can move on to reconciliation."}
	tNextReviewLink = T{KO: "연결 검토", EN: "Review the link"}

	tNextMetricNodes    = T{KO: "관리 대상 노드", EN: "nodes in scope"}
	tNextMetricUnlinked = T{KO: "연결할 관측 이름", EN: "observed names to link"}
	tNextMetricAssets   = T{KO: "선언한 암호 자산", EN: "declared crypto assets"}

	tNextLinkSection = T{KO: "노드와 관측 연결", EN: "Linking nodes to observations"}
	tNextLinkNote    = T{
		KO: "원본 선언은 그대로 유지합니다",
		EN: "the original declaration stays as it is"}

	tNextObservedAs = T{KO: "관측 이름", EN: "observed name"}
	tNextEmpty      = T{KO: "비어 있음", EN: "empty"}
	tNextLinked     = T{KO: "연결됨", EN: "linked"}
	tNextCandidate  = T{KO: "연결 후보 있음", EN: "candidate found"}
	tNextAwaiting   = T{KO: "관측 대기", EN: "awaiting observation"}
	tNextGoEdit     = T{KO: "고치러 가기", EN: "Go and edit"}

	tNextEditHead = T{KO: "선언 고치기", EN: "Edit the declaration"}
	tNextEditLede = T{
		KO: "위에서 본 것을 여기서 고칩니다. 저장하면 선언 파일에 그대로 씁니다.",
		EN: "What you saw above gets edited here. Saving writes it straight to the declaration file."}
)

// ── 다음 판의 껍데기 ────────────────────────────────────────────────────────
var (
	// 화면 머리의 모드. 실제로 저장하는지를 적는다.
	tModeWrite      = T{KO: "저장 가능", EN: "Saving enabled"}
	tModeViewOnly   = T{KO: "보기 전용: 이 플랫폼에서는 저장하지 않습니다", EN: "View only: this platform does not save"}
	tModeRecovery   = T{KO: "복구 필요: 저장을 멈췄습니다", EN: "Recovery required: saving is stopped"}
	tModeDiagnostic = T{KO: "진단 전용: 입력 파일을 읽지 못해 저장을 멈췄습니다", EN: "Diagnostics only: an input file could not be read, so saving is stopped"}

	// 작업 폴더에 선언이 아직 없을 때.
	tStepDeclMissing = T{KO: "아직 선언이 없습니다", EN: "no declaration yet"}
	tDeclCreateHead  = T{KO: "아직 선언이 없습니다", EN: "There is no declaration yet"}
	tDeclCreateLede  = T{
		KO: "조직 이름을 적고 만들면 작업 폴더에 빈 선언이 생깁니다. 노드 · 자산 · 엣지는 만든 뒤에 이 화면에서 적습니다.",
		EN: "Enter the organization name and create it: an empty declaration appears in the workspace. Write the nodes, assets and edges on this screen afterwards."}
	// 작업 폴더에 계층이 아직 없을 때.
	tLayerCreateHead = T{KO: "아직 계층이 없습니다", EN: "There is no layer yet"}
	tLayerCreateLede = T{
		KO: "자산 스코프 규칙은 계층 CSV에 적습니다. 이름을 정해 만들면 작업 폴더의 layers 폴더에 빈 계층이 생기고, 그다음 이 화면에서 규칙을 적습니다. 계층이 여럿이면 이름 순으로 겹치고, 같은 자산에 규칙이 겹치면 뒤의 계층을 따릅니다.",
		EN: "Asset scope rules are written in layer CSVs. Name one and create it: an empty layer appears in the layers folder of the workspace, and you write its rules on this screen. Several layers stack in name order, and where rules clash on the same asset the later layer applies."}
	tLayerCreateName   = T{KO: "계층 이름", EN: "Layer name"}
	tLayerCreateHint   = T{KO: "영문 소문자와 숫자, 「-」, 「_」를 써서 40자까지 적습니다. 이 이름이 파일 이름이 됩니다(예: corp → corp.csv).", EN: "Lowercase letters, digits, - and _, up to 40. It becomes the file name (corp → corp.csv)."}
	tLayerCreateDo     = T{KO: "계층 만들기", EN: "Create the layer"}
	tStepLayersMissing = T{KO: "아직 계층이 없습니다", EN: "no layer yet"}

	tDeclCreateOrg = T{KO: "조직 이름", EN: "Organization name"}
	tDeclCreateDo  = T{KO: "선언 만들기", EN: "Create the declaration"}
	// 설정 화면.
	tTitleSettings = T{KO: "설정", EN: "Settings"}
	tSetLede       = T{
		KO: "화면을 띄울 때 정해진 값과 그 상태입니다. 대부분은 플래그나 파일로 정하므로 여기서는 보기만 하고, 값마다 무엇으로 바꾸는지 적었습니다. 연결 문자열 · 비밀번호 · 키의 내용은 보이지 않습니다.",
		EN: "The values fixed when the screen started, and their state. Most are set by a flag or a file, so they are only shown here, each with how to change it. Connection strings, passwords and the contents of keys are not shown."}
	tSetColName  = T{KO: "항목", EN: "Item"}
	tSetColValue = T{KO: "지금 값", EN: "Current value"}
	tSetColHow   = T{KO: "바꾸는 법", EN: "How to change it"}
	tSetRestart  = T{KO: "바꾼 뒤 화면을 다시 띄웁니다", EN: "restart the screen after changing it"}
	tSetLive     = T{KO: "다시 띄우지 않아도 됩니다", EN: "no restart needed"}
	tSetNone     = T{KO: "(없음)", EN: "(none)"}
	tSetYes      = T{KO: "켬", EN: "on"}
	tSetNo       = T{KO: "끔", EN: "off"}

	tSetGroupScreen     = T{KO: "화면", EN: "The screen"}
	tSetGroupWork       = T{KO: "조직과 파일", EN: "Organization and files"}
	tSetGroupHistory    = T{KO: "pqcota 이력", EN: "The pqcota history"}
	tSetGroupKeys       = T{KO: "키", EN: "Keys"}
	tSetGroupCollectors = T{KO: "수집기", EN: "Collectors"}

	tSetLang             = T{KO: "표시 언어", EN: "Display language"}
	tSetLangHow          = T{KO: "여기서 고릅니다. 고른 말은 다른 화면에서도 쓰입니다.", EN: "Choose it here. The choice carries over to the other screens."}
	tSetMode             = T{KO: "저장 상태", EN: "Saving state"}
	tSetModeWriteNext    = T{KO: "할 일이 없습니다. 저장하고 확정할 수 있습니다.", EN: "Nothing to do. Saving and finalizing work."}
	tSetModeViewOnlyNext = T{
		KO: "이 플랫폼에서는 화면이 파일을 쓰지 않습니다. 저장하려면 지원 플랫폼(운영자 안내서 1절)에서 같은 작업 폴더로 띄웁니다.",
		EN: "On this platform the screen writes no file. To save, start it on a supported platform (operator guide, section 1) with the same workspace."}
	tSetModeRecoveryNext = T{
		KO: "끝나지 않은 저장이 남아 있습니다. 화면이 보이는 복구 안내대로 정리한 뒤 다시 띄웁니다(운영자 안내서 8절).",
		EN: "An unfinished save is left. Resolve it as the recovery screen says, then restart (operator guide, section 8)."}
	tSetModeDiagnosticNext = T{
		KO: "읽지 못한 입력 파일이 있습니다. 화면이 보이는 진단대로 파일을 고친 뒤 다시 띄웁니다(운영자 안내서 8절).",
		EN: "An input file could not be read. Fix it as the diagnostics screen says, then restart (operator guide, section 8)."}
	tSetVersion    = T{KO: "화면의 판", EN: "Version of the screen"}
	tSetVersionHow = T{KO: "다른 판을 설치합니다(운영자 안내서 9절).", EN: "Install another version (operator guide, section 9)."}
	tSetAddr       = T{KO: "듣는 주소", EN: "Listening address"}
	tSetAddrHow    = T{KO: "`-addr` 플래그로 줍니다.", EN: "Give it with the `-addr` flag."}

	tSetOrg        = T{KO: "조직(판정 조직)", EN: "Organization (of the judgments)"}
	tSetOrgHowDecl = T{
		KO: "선언 파일의 조직입니다. 다른 조직은 그 조직의 작업 폴더로 화면을 따로 띄웁니다.",
		EN: "It is the organization of the declaration file. For another organization, start a screen with that organization's workspace."}
	tSetOrgHowFlag       = T{KO: "`-org` 플래그로 줍니다.", EN: "Give it with the `-org` flag."}
	tSetWorkspace        = T{KO: "작업 폴더", EN: "Workspace"}
	tSetWorkspaceHow     = T{KO: "`-workspace` 플래그로 줍니다.", EN: "Give it with the `-workspace` flag."}
	tSetFileHowWorkspace = T{KO: "작업 폴더 안의 정해진 이름입니다. 따로 바꾸지 않습니다.", EN: "A fixed name inside the workspace. It is not changed on its own."}
	tSetFileHowFlag      = T{KO: "`%s` 플래그로 줍니다.", EN: "Give it with the `%s` flag."}
	tSetFileDecl         = T{KO: "선언", EN: "Declaration"}
	tSetFileLayer        = T{KO: "자산 스코프 계층", EN: "Asset scope layer"}
	tSetFileScopeActive  = T{KO: "활성 정책(대조에 쓰는 것)", EN: "Active policy (used for reconciliation)"}
	tSetFileScopeOut     = T{KO: "자산 스코프 화면에서 확정한 정책", EN: "Policy finalized on the asset scope screen"}
	tSetFileScopeSess    = T{KO: "자산 스코프 세션", EN: "Asset scope session"}
	tSetFileSession      = T{KO: "판정 세션", EN: "Judgment session"}
	tSetFilePlan         = T{KO: "계획", EN: "Plan"}
	tSetFileJudgments    = T{KO: "판정 원장", EN: "Judgment ledger"}
	tSetFileResults      = T{KO: "관측 결과 폴더", EN: "Observation results folder"}
	tSetFileHosts        = T{KO: "접근 정보", EN: "Access information"}
	tSetStateDir         = T{KO: "상태 폴더", EN: "State folder"}
	tSetStateDirHow      = T{KO: "`-state-dir` 플래그로 줍니다. 같은 파일을 바꾸는 명령은 같은 폴더를 써야 합니다.", EN: "Give it with the `-state-dir` flag. Commands that change the same files must use the same folder."}

	tSetUpstreamOrg    = T{KO: "조회하는 조직", EN: "Organization queried"}
	tSetUpstreamOrgHow = T{
		KO: "판정 조직과 같습니다. `pqcota-ingest`도 `PQCOTA_ORG`를 이 값으로 주고 적재해야 여기서 보입니다.",
		EN: "It is the organization of the judgments. `pqcota-ingest` must load with `PQCOTA_ORG` set to this value for the data to show here."}
	tSetDSN       = T{KO: "연결 정보의 출처", EN: "Where the connection comes from"}
	tSetDSNEnv    = T{KO: "환경 변수 %s", EN: "the environment variable %s"}
	tSetDSNFile   = T{KO: "사용자 설정 폴더의 pqcaton/pqcota.env (%s)", EN: "pqcaton/pqcota.env in the user configuration folder (%s)"}
	tSetIngest    = T{KO: "화면에서 이력 적재", EN: "Ingesting into the history from the screen"}
	tSetIngestOff = T{KO: "하지 않음(적재용 연결 정보가 설정되지 않았습니다)", EN: "off (no connection for ingesting is set)"}
	tSetIngestHow = T{
		KO: "조회용 연결 정보를 PQCOTA_READONLY_DSN에, 같은 데이터베이스의 적재 계정 연결 정보를 PQCOTA_DSN에 둡니다. 둘 다 있을 때만 화면에서 적재합니다. 적재용 연결 정보를 설정한 화면 서버는 그 값을 읽을 수 있습니다.",
		EN: "Put the read connection in PQCOTA_READONLY_DSN and the ingest account's connection to the same database in PQCOTA_DSN. The screen ingests only when both are set. A screen server with the ingest connection set can read that value."}
	tSetDSNHow = T{
		KO: "환경 변수 `PQCOTA_DSN`이 먼저이고, 없으면 사용자 설정 폴더의 `pqcaton/pqcota.env`를 읽습니다. 값은 화면에 보이지 않습니다.",
		EN: "The environment variable `PQCOTA_DSN` comes first; without it the screen reads `pqcaton/pqcota.env` in the user configuration folder. The value is not shown on the screen."}

	tSetApprovalKey    = T{KO: "승인 서명 키", EN: "Approval signing key"}
	tSetApprovalKeyHow = T{
		KO: "자리는 `-approval-key` 플래그로 바꿉니다. 같은 자리의 파일을 바꾸면 다음 서명부터 쓰입니다.",
		EN: "Change the place with the `-approval-key` flag. Replacing the file in the same place takes effect from the next signature."}
	tSetVerifyKey          = T{KO: "승인 확인용 공개 키", EN: "Approval verification key"}
	tSetVerifyKeyHow       = T{KO: "자리는 `-approval-verify-key` 플래그로 바꿉니다.", EN: "Change the place with the `-approval-verify-key` flag."}
	tSetCollectorKeys      = T{KO: "수집기 서명 키 폴더", EN: "Collector signing key folder"}
	tSetCollectorKeysHow   = T{KO: "자리는 `-collector-keys` 플래그로 바꿉니다.", EN: "Change the place with the `-collector-keys` flag."}
	tSetCollectorVerify    = T{KO: "수집기 신뢰 공개 키", EN: "Trusted collector public keys"}
	tSetCollectorVerifyHow = T{KO: "자리는 `-collector-verify-keys` 플래그로 바꿉니다.", EN: "Change the place with the `-collector-verify-keys` flag."}
	tSetKeyOK              = T{KO: "쓸 수 있음", EN: "usable"}
	tSetKeyNone            = T{KO: "없음", EN: "not there"}
	tSetKeyDisabled        = T{KO: "찾지 않음(기본 위치 검색을 껐습니다)", EN: "not looked for (the default location is turned off)"}
	tSetKeyUnusable        = T{
		KO: "쓸 수 없음. 까닭은 그 키를 쓰는 화면에 있습니다(승인 키는 04, 수집기 키는 01)",
		EN: "not usable. The reason is on the screen that uses the key (04 for approval keys, 01 for collector keys)"}

	tSetCollectors         = T{KO: "수집기 폴더", EN: "Collector folder"}
	tSetCollectorsHow      = T{KO: "`-collectors` 플래그로 줍니다. 작업 폴더로 띄우면 그 안의 collectors 폴더입니다.", EN: "Give it with the `-collectors` flag. With a workspace it is the collectors folder inside it."}
	tSetCollectorPin       = T{KO: "맞춰야 하는 상류 판", EN: "Upstream version to match"}
	tSetCollectorPinHow    = T{KO: "화면의 판이 정합니다. 화면의 판을 바꾸면 달라집니다.", EN: "The version of the screen decides it. It changes with the version of the screen."}
	tSetAllowUnverified    = T{KO: "확인할 수 없는 수집기 허용", EN: "Allowing collectors that cannot be verified"}
	tSetAllowUnverifiedHow = T{KO: "`-allow-unverified-collectors` 플래그로 켭니다.", EN: "Turn it on with the `-allow-unverified-collectors` flag."}
	tWbStepsAria           = T{KO: "거버넌스 절차", EN: "Governance steps"}

	// 접근성: 건너뛰기 링크와 표 칸의 이름에 쓰는 행 이름.
	tSkipToMain = T{KO: "본문으로 건너뛰기", EN: "Skip to main content"}
	tRowNode    = T{KO: "노드", EN: "Node"}
	tRowAsset   = T{KO: "자산", EN: "Asset"}
	tRowEdge    = T{KO: "엣지", EN: "Edge"}
	tRowRule    = T{KO: "규칙", EN: "Rule"}

	tStepDeclOpen = T{
		KO: "연결할 관측 이름 %d개",
		EN: "%d observed names to link"}
	tStepDeclDone     = T{KO: "연결할 이름 없음", EN: "nothing left to link"}
	tStepDeclProblems = T{KO: "앞뒤가 맞지 않는 자리 %d곳", EN: "does not add up in %d places"}
	tStepClosed       = T{KO: "입력 파일을 주지 않았습니다", EN: "no input was given"}
	tStepUnknown      = T{KO: "확인할 수 없음", EN: "Unavailable"}

	// 확정 상태. 영수증과 지금 파일을 견준 결과다(store.VerifyFinalized).
	tFinalNone       = T{KO: "미확정", EN: "Not finalized"}
	tFinalDone       = T{KO: "확정됨", EN: "Finalized"}
	tFinalSuperseded = T{KO: "대체됨", EN: "Superseded"}
	tFinalRecovery   = T{KO: "복구 필요", EN: "Recovery required"}
)

// ── 스코프 요약 (`/scope-next`) ─────────────────────────────────────────────
var (
	tNextScopeEyebrow = T{KO: "01 관측 · 암호 자산 스코프", EN: "01 Observation · Crypto asset scope"}
	tNextScopeHead    = T{
		KO: "관리하지 않을 자산을 승인합니다",
		EN: "Approve what will not be managed"}
	tNextScopeLede = T{
		KO: "규칙 원본은 계층별 CSV에 남고, 제외 변경만 판정과 서명을 거쳐 확정합니다.",
		EN: "The rules themselves stay in the per-layer CSVs; only exclusions go through " +
			"judgment and a signature."}

	tNextScopeTodo = T{
		KO: "승인할 규칙 변경이 %d개 있습니다",
		EN: "%d rule changes are waiting for approval"}
	tNextScopeAudited = T{
		KO: "그 가운데 %d개는 왜 뺐는지를 적어야 확정됩니다.",
		EN: "%d of them need a recorded reason before finalizing."}
	tNextScopeDone     = T{KO: "승인할 규칙 변경이 없습니다", EN: "No rule change is waiting"}
	tNextScopeDoneBody = T{
		KO: "지금 정책은 규칙 %d개이고, 새로 뺀 자산이 없습니다.",
		EN: "The policy in force has %d rules, and nothing new was excluded."}

	tNextScopeGo = T{KO: "승인하러 가기", EN: "Go and approve"}

	tNextScopeMetricLayers  = T{KO: "적용 중인 계층", EN: "layers in force"}
	tNextScopeMetricRules   = T{KO: "확정된 규칙", EN: "finalized rules"}
	tNextScopeMetricChanges = T{KO: "판정할 변경", EN: "changes to judge"}

	tNextLayerSection = T{KO: "적용 중인 계층", EN: "Layers in force"}
	tNextLayerNote    = T{KO: "아래 계층일수록 우선합니다", EN: "the lower the layer, the higher it wins"}
	tNextLayerRules   = T{KO: " · 규칙 %d개", EN: " · %d rules"}

	tNextScopeEditHead = T{KO: "규칙과 판정", EN: "Rules and judgments"}
	tNextScopeEditLede = T{
		KO: "위에서 본 것을 여기서 승인합니다. 확정하면 pqcota 집행기가 읽는 CSV 그대로 나갑니다.",
		EN: "What you saw above gets approved here. Finalizing writes the very CSV the " +
			"pqcota enforcer reads."}
)

var (
	tStepScopeOpen    = T{KO: "승인할 변경 %d건", EN: "%d changes to approve"}
	tStepScopeAudited = T{
		KO: "승인할 변경 %d건 · 근거 필요 %d건",
		EN: "%d changes to approve · a reason needed for %d"}
	tStepScopeDone = T{KO: "승인할 변경 없음", EN: "no change to approve"}
)

// ── 대조 요약 (`/survey-next`) ──────────────────────────────────────────────
var (
	tNextSurveyEyebrow = T{KO: "02 · 대조", EN: "02 · Reconciliation"}
	tNextSurveyHead    = T{
		KO: "대조 결과에는 사람이 확인할 항목이 %d개 있습니다",
		EN: "%d items need a person to look at the reconciliation"}
	tNextSurveyHeadDone = T{
		KO: "사람이 확인할 항목이 없습니다",
		EN: "Nothing in the reconciliation needs a person"}
	tNextSurveyLede = T{
		KO: "관측이 부족한지, 선언이 빠진 것인지 먼저 가른 뒤 판정으로 보냅니다.",
		EN: "Sort out whether the observation fell short or the declaration is missing, " +
			"then send it on to judgment."}

	tNextSurveyRescan     = T{KO: "관측 범위가 부족합니다", EN: "The observation did not reach far enough"}
	tNextSurveyRescanBody = T{
		KO: "다시 관측해 봐야 하는 자산이 %d개입니다. UNOBSERVED를 없는 것으로 확정하지 마십시오.",
		EN: "%d assets need to be observed again. Do not finalize UNOBSERVED as absent."}
	tNextSurveyJudge = T{
		KO: "판정할 항목이 %d개입니다",
		EN: "%d items are waiting for a judgment"}
	tNextSurveyJudgeBody = T{
		KO: "관측과 선언이 어긋난 자리입니다. 판정 큐에서 결론을 남깁니다.",
		EN: "These are where observation and declaration diverge. Record a conclusion in the review queue."}
	tNextSurveyDone     = T{KO: "대조가 다 맞았습니다", EN: "Everything reconciled"}
	tNextSurveyDoneBody = T{
		KO: "선언과 관측이 모두 맞물렸습니다. 판정으로 넘어갈 것이 없습니다.",
		EN: "Declaration and observation line up. Nothing is waiting for judgment."}
	tNextSurveyGoReview = T{KO: "판정 큐 열기", EN: "Open the review queue"}
	tNextSurveyExcluded = T{
		KO: "선언과 어긋난 자산 가운데 %d개는 암호 자산 스코프 정책이 관리 대상에서 뺀 것이어서 판정 큐에 오르지 않고, 위 숫자에도 세지 않았습니다. 아래 표에 「정책 제외」로 적혀 있습니다.",
		EN: "Assets that diverge from the declaration but that the crypto asset scope policy excludes: %d. Such assets do not go to the review queue and are not counted above; the table below marks them as excluded by policy."}
	tNextSurveyNoneToJudge = T{KO: "판정할 항목이 없습니다", EN: "Nothing is waiting for a judgment"}
	tSurveyExcluded        = T{KO: "정책 제외", EN: "excluded by policy"}

	tNextSurveyDetail     = T{KO: "자산과 엣지 원본", EN: "The assets and edges themselves"}
	tNextSurveyDetailLede = T{
		KO: "위의 숫자가 어느 줄에서 나왔는지 여기서 봅니다.",
		EN: "This is where the numbers above come from."}
)

var (
	tStepSurveyOpen   = T{KO: "판정할 항목 %d개", EN: "%d items to judge"}
	tStepSurveyRescan = T{
		KO: "판정할 항목 %d개 · 재관측 필요 %d개",
		EN: "%d items to judge · re-observing needed for %d"}
	tStepSurveyDone = T{KO: "확인할 항목 없음", EN: "nothing to check"}
)

// ── 판정 요약 (`/review-next`) ──────────────────────────────────────────────
var (
	tNextReviewEyebrow = T{KO: "03 · 판정", EN: "03 · Judgment"}
	tNextReviewHead    = T{
		KO: "정책별로 결론을 남기고, 예외만 개별로 봅니다",
		EN: "Record a conclusion per policy, and look at exceptions one by one"}
	tNextReviewLede = T{
		KO: "확정은 모든 필수 결론과 승인 서명이 채워진 뒤에만 가능합니다.",
		EN: "Finalizing is possible only after every required conclusion and the approval " +
			"signature are filled in."}

	tNextReviewTodo = T{
		KO: "필수 판정 %d개가 남았습니다",
		EN: "%d required judgments are left"}
	tNextReviewTodoSign = T{
		KO: "필수 판정 %d개와 승인 서명이 남았습니다",
		EN: "%d required judgments and the approval signature are left"}
	tNextReviewSignOnly = T{KO: "승인 서명이 남았습니다", EN: "The approval signature is left"}
	tNextReviewTodoBody = T{
		KO: "정책 %d개에 결론을 남긴 뒤, 필요한 항목만 계획에 넣습니다.",
		EN: "Record a conclusion on %d policies, then put only what is needed into the plan."}
	tNextReviewSignBody = T{
		KO: "결론은 다 찼습니다. 승인자와 서명을 채우면 확정할 수 있습니다.",
		EN: "Every conclusion is in. Fill in the approver and the signature to finalize."}
	tNextReviewReady     = T{KO: "확정할 수 있습니다", EN: "Ready to finalize"}
	tNextReviewReadyBody = T{
		KO: "필수 결론과 서명이 모두 찼습니다.",
		EN: "Every required conclusion and the signature are filled in."}
	tNextReviewGo = T{KO: "확정 준비", EN: "Get ready to finalize"}

	tNextReviewMetricPolicies = T{KO: "결론을 남길 정책", EN: "policies to conclude"}
	tNextReviewMetricOpen     = T{KO: "결론이 빈 정책", EN: "policies without a conclusion"}
	tNextReviewMetricAutopass = T{KO: "기계가 답한 항목", EN: "items the machine answered"}

	tNextReviewEditHead = T{KO: "정책별 결론과 확정", EN: "Conclusions and finalization"}
	tNextReviewEditLede = T{
		KO: "위에서 본 것을 여기서 남깁니다. 확정하면 판정 원장에 append-only로 쌓입니다.",
		EN: "What you saw above gets recorded here. Finalizing appends to the judgment ledger."}
)

var (
	tStepReviewOpen  = T{KO: "남은 필수 판정 %d개", EN: "%d required judgments left"}
	tStepReviewSign  = T{KO: "승인 서명이 남음", EN: "signature left"}
	tStepReviewReady = T{KO: "확정할 수 있음", EN: "ready to finalize"}
	// tStepReviewRuleset — 다른 상류 규칙 판의 세션. 잠겨 있어 확정할 수 없다.
	tStepReviewRuleset = T{KO: "규칙 전환 회차가 필요함", EN: "needs a ruleset change round"}
)

// ── 조회 요약 (`/inventory-next`) ───────────────────────────────────────────
var (
	tNextInvEyebrow = T{KO: "조회 · 절차 밖", EN: "Lookup · outside the steps"}
	tNextInvHead    = T{
		KO: "인벤토리와 판정 이력을 살펴봅니다",
		EN: "Look through the inventory and the judgment history"}
	tNextInvLede = T{
		KO: "이 화면은 단계가 아니라 언제든 근거를 확인하는 자리입니다.",
		EN: "This screen is not a step; it is where you check the evidence at any time."}

	tNextInvMetricAssets = T{KO: "관리 중인 자산", EN: "assets under management"}
	tNextInvMetricUnseen = T{KO: "제외분 재검토", EN: "exclusions to re-examine"}
	tNextInvMetricStale  = T{KO: "근거가 바뀐 판정", EN: "judgments whose basis changed"}

	tNextInvAttention     = T{KO: "주의가 필요한 항목", EN: "What needs attention"}
	tNextInvAttentionNote = T{
		KO: "원본 전체는 아래에서 확인합니다",
		EN: "the full records are below"}
	tNextInvUnseenCard = T{KO: "제외분 재검토", EN: "Exclusions to re-examine"}
	tNextInvUnseenBody = T{
		KO: "승인이 없거나 만료된 제외 %d개",
		EN: "%d exclusions with no approval or an expired one"}
	tNextInvStaleCard = T{KO: "근거가 바뀐 판정", EN: "Judgments whose basis changed"}
	tNextInvStaleBody = T{
		KO: "다시 봐야 하는 결론 %d개",
		EN: "%d conclusions to look at again"}
	tNextInvQuiet     = T{KO: "지금 걸리는 것이 없습니다", EN: "Nothing is flagged right now"}
	tNextInvQuietBody = T{
		KO: "제외분과 판정 원장 모두 다시 볼 것이 없습니다.",
		EN: "Neither the exclusions nor the judgment ledger has anything to revisit."}
	tNextInvNeedCheck = T{KO: "검토 필요", EN: "needs a look"}

	tNextInvDetail     = T{KO: "인벤토리 원본", EN: "The inventory itself"}
	tNextInvDetailLede = T{
		KO: "노드·런타임·컴포넌트·상태로 좁혀 봅니다. 좁힌 조건은 주소에 실리므로 그대로 남에게 보낼 수 있습니다.",
		EN: "Narrow by node, runtime, component or state. The filter rides in the URL, so " +
			"you can send the screen as it is."}
)

// ── 01 관측 ─────────────────────────────────────────────────────────────────
var (
	tObserveEyebrow = T{KO: "01 · 관측", EN: "01 · Observation"}
	tObserveHead    = T{
		KO: "관측할 노드와 접속 방법을 적습니다",
		EN: "Write down the nodes to observe and how to reach them"}
	tObserveLede = T{
		KO: "관측은 이 표에 적힌 노드에 SSH나 WinRM으로 접속해 수집기를 실행합니다. 결과는 작업 폴더의 results에 들어오고 02 대조로 이어집니다.",
		EN: "Observation reaches each node in this table over SSH or WinRM and runs the collectors there. " +
			"The results land in the workspace's results folder and go on to 02 Reconciliation."}
	tObserveMetricHosts = T{KO: "관측할 노드", EN: "nodes to observe"}

	tHostsHead = T{KO: "접근 정보", EN: "Access"}
	tHostsLede = T{
		KO: "작업 폴더의 hosts.csv에 저장합니다. 형식은 pqcota-hosts가 읽는 것과 같습니다.",
		EN: "This is saved to hosts.csv in the workspace, in the same format pqcota-hosts reads."}
	tHostsLegend = T{KO: "노드", EN: "Nodes"}
	tHostsHint   = T{
		KO: "<b>줄은 「대상 노드와 기대 상태」에 등록한 노드입니다.</b> 노드 ID는 선언의 노드 이름이고 여기서 고치지 않습니다. 접속 정보를 적지 않은 줄은 저장되지 않고, 그 노드는 관측에서 빠집니다.<br>" +
			"<b>SSH 키는 개인 키 파일의 경로를 적습니다.</b> 키 자체는 화면에 들어오지 않습니다. 포트를 비우면 SSH는 22, WinRM은 5985입니다.<br>" +
			"<b>비밀번호는 이 화면에서 적지 않습니다.</b> 파일에 있으면 「있음」으로만 보이고 저장해도 그대로 남습니다. " +
			"WinRM 노드처럼 비밀번호가 필요하면 hosts.csv의 ssh_pass 열에 직접 적습니다.<br>" +
			"고치지 않은 행은 저장해도 파일의 글자 그대로 남습니다. 행을 고치거나 옮겨도 그 앞의 주석 줄(#)은 그대로 남습니다.",
		EN: "<b>The rows are the nodes registered on the target nodes screen.</b> The node ID is the declared node name and is not edited here. A row without access details is not saved, and that node is left out of the observation.<br>" +
			"<b>For an SSH key, write the path to the private key file.</b> The key itself never enters the screen. A blank port means 22 for SSH and 5985 for WinRM.<br>" +
			"<b>Passwords are not written on this screen.</b> If the file has one, it shows only as 「set」 and is kept as is when you save. " +
			"When a node needs a password, as WinRM nodes do, write it in the ssh_pass column of hosts.csv directly.<br>" +
			"A row you did not change stays in the file exactly as it was. Comment lines (#) before a row stay when that row is changed or moved."}
	tColNodeID = T{KO: "노드 ID", EN: "Node ID"}
	tColUser   = T{KO: "계정", EN: "User"}
	tColKey    = T{KO: "SSH 키 경로", EN: "SSH key path"}
	tColOS     = T{KO: "OS", EN: "OS"}
	tColConn   = T{KO: "접속", EN: "Connection"}
	tColPass   = T{KO: "비밀번호", EN: "Password"}
	tPassSet   = T{KO: "있음", EN: "set"}
	tRowHost   = T{KO: "노드", EN: "Node"}
	tAddHost   = T{KO: "노드 추가", EN: "Add node"}
	tHostsSave = T{KO: "저장", EN: "Save"}

	tRemoveHostAsk = T{
		KO: "이 노드를 표에서 지웁니다. 저장해야 파일에 반영되고, 그 노드의 비밀번호도 함께 지워집니다. 지울까요?",
		EN: "This removes the node from the table. It reaches the file only when you save, and the node's password goes with it. Remove?"}

	tHostsSaved = T{
		KO: "접근 정보를 저장했습니다. 노드 %d",
		EN: "Access saved — %d nodes"}

	tHostProblemsHead = T{KO: "저장하지 않았습니다. 아래 자리를 고치십시오", EN: "Not saved: fix the places below"}
	tHostDup          = T{KO: "같은 노드 ID가 두 번 적혔습니다", EN: "the same node ID is written twice"}
	tHostOS           = T{KO: "OS는 Linux와 Windows 가운데 하나입니다", EN: "the OS must be Linux or Windows"}
	tHostConn         = T{KO: "접속 방법은 SSH와 WinRM 가운데 하나입니다", EN: "the connection must be SSH or WinRM"}
	tHostWinRMOS      = T{KO: "WinRM은 Windows 노드에만 씁니다", EN: "WinRM is only for Windows nodes"}
	tHostWinRMKey     = T{KO: "WinRM은 SSH 키로 접속하지 않습니다. 키 경로를 비우십시오", EN: "WinRM does not connect with an SSH key: clear the key path"}
	tHostPort         = T{KO: "포트는 1부터 65535까지의 숫자입니다", EN: "the port must be a number from 1 to 65535"}
	tHostNoIP         = T{KO: "IP가 없습니다", EN: "there is no IP"}
	tHostControl      = T{KO: "줄바꿈 · 탭 · 줄 구분자 같은 인쇄할 수 없는 문자가 들어 있습니다", EN: "it holds a character that cannot be printed, such as a line break, a tab or a line separator"}

	tStepObserveNoHosts  = T{KO: "아직 접근 정보가 없습니다", EN: "no access written yet"}
	tStepObserveNoRemote = T{KO: "대상 노드와 기대 상태만(작업 폴더 없이 열림)", EN: "target nodes and expected state only (opened without a workspace)"}
	tObsPartDecl         = T{KO: "1. 대상 노드와 기대 상태", EN: "1. Target nodes and expected state"}
	tObsPartAccess       = T{KO: "2. 접속 준비와 관측", EN: "2. Access and observation"}
	tObsPartScope        = T{KO: "3. 자산 스코프와 이력 적재", EN: "3. Asset scope and ingest into the history"}
	tObsDeclLede         = T{
		KO: "관측할 노드를 한 번 등록하고, 그 노드에 있어야 할 암호 자산과 통신을 적습니다. 접속 정보는 같은 노드에 아래에서 채웁니다. 대조는 접속 준비나 관측의 성공과 무관하게 여기 등록한 전부를 대상으로 합니다.",
		EN: "Register each node to observe once, and write the crypto assets and communications that should be on it. Access details are filled in below for the same node. Reconciliation covers everything registered here, whether or not a node is ready or was observed."}
	tObsDeclCounts = T{KO: "대상 노드 %d곳 · 기대 자산 %d개 · 기대 통신 %d개", EN: "%d target nodes · %d expected assets · %d expected communications"}
	tObsDeclEdit   = T{KO: "대상 노드와 기대 상태 고치기", EN: "Edit the target nodes and expected state"}
	tObsNoRemote   = T{
		KO: "접속 준비와 원격 관측을 쓰려면 작업 폴더로 시작하십시오(pqcaton-ui -workspace <폴더>). 지금은 대상 노드와 기대 상태만 볼 수 있습니다.",
		EN: "To prepare access and observe remotely, start with a workspace (pqcaton-ui -workspace <folder>). Right now only the target nodes and expected state can be viewed."}
	tStepObserveHosts   = T{KO: "관측할 노드 %d대", EN: "%d nodes to observe"}
	tStepObserveRunning = T{KO: "관측 중", EN: "observing now"}
)

// ── 01 관측 · 실행 ──────────────────────────────────────────────────────────
var (
	tRunHead = T{KO: "관측 실행", EN: "Run the observation"}
	tRunLede = T{
		KO: "위 표의 노드마다 수집기를 올려 실행하고, 결과를 받은 뒤 노드에서 지웁니다. 운영 기계의 설정은 바꾸지 않습니다.",
		EN: "Copies the collectors to each node in the table above, runs them, takes the results, and removes them from the node. " +
			"Nothing in the machines' configuration is changed."}
	tRunWindow    = T{KO: "네트워크 관측 구간(초)", EN: "Network observation window (seconds)"}
	tRunStart     = T{KO: "관측 시작", EN: "Start the observation"}
	tRunStartHint = T{
		KO: "<b>Linux 노드에서는 root 권한(become)으로 실행합니다.</b> 다른 사용자의 프로세스와 네트워크 패킷을 보려면 필요합니다.<br>" +
			"네트워크 관측은 노드의 기본 경로 인터페이스에서 이 구간만큼 TLS · SSH 핸드셰이크를 기다립니다. " +
			"구간 동안 오간 통신이 없으면 「없음」이 아니라 「관측하지 못함」으로 남습니다.<br>" +
			"받은 결과는 작업 폴더의 results에 들어가고, 같은 노드의 이전 결과를 갈아 끼웁니다.",
		EN: "<b>On Linux nodes the collectors run as root (become).</b> Seeing other users' processes and network packets needs it.<br>" +
			"The network observation waits this long for TLS and SSH handshakes on the node's default-route interface. " +
			"If nothing passes in that window, it is recorded as 「not observed」, not as 「none」.<br>" +
			"The results go into the workspace's results folder and replace the earlier results of the same node."}
	tRunNotWritable = T{
		KO: "이 화면은 저장하지 않는 모드라 관측을 시작하지 않습니다",
		EN: "This screen does not save in its current mode, so it does not start an observation"}
	tRunStop     = T{KO: "중지", EN: "Stop"}
	tRunStopHint = T{
		KO: "멈추면 그때까지 받은 결과만 옮깁니다. 실행 중이던 노드에는 스테이징 폴더(/tmp/pqcaton-observe-…)가 남을 수 있습니다.",
		EN: "Stopping moves only the results received so far. A node that was mid-run may keep its staging folder (/tmp/pqcaton-observe-…)."}
	tRunGoSurvey = T{KO: "02 대조에서 보기", EN: "See them in 02 Reconciliation"}
	tRunDropped  = T{KO: "화면에 남기지 않은 앞의 로그: %d줄", EN: "Log lines not kept from the start: %d"}
	tRunLog      = T{KO: "관측 실행 로그", EN: "Observation log"}

	tRunRefusedHead     = T{KO: "관측을 시작하지 않았습니다", EN: "The observation was not started"}
	tRunRefusedNotReady = T{
		KO: "시작하기 전에 갖출 것이 빠졌습니다. 01 관측 화면의 점검 목록에서 ✗ 표시된 것을 갖추십시오.",
		EN: "Something needed before starting is missing. Provide the items marked ✗ in the checklist on the 01 Observation screen."}
	tRunRefusedHostsUndeclared = T{
		KO: "접근 정보(hosts.csv)에 지금 선언에 없는 노드 ID가 있어 관측을 시작하지 않았습니다. 노드에는 접속하지 않았고 파일은 그대로입니다. 01 관측 화면의 「선언에 없는 행」에서 그 행과 ID를 확인하십시오.",
		EN: "The access details (hosts.csv) hold a node ID that is not in the current declaration, so no observation was started. No node was contacted and the files are unchanged. See the row and the ID under \"Rows not in the declaration\" on the 01 Observation screen."}
	tTargetsHead = T{KO: "대상 노드의 접속 준비", EN: "Access readiness of the target nodes"}
	tTargetsLede = T{
		KO: "줄은 「대상 노드와 기대 상태」에 등록한 노드입니다. 접근 정보의 노드 ID가 선언의 노드 이름과 대소문자까지 같으면 그 노드의 접속 정보입니다. 접속 정보가 없는 노드는 이번 관측에서 빠질 뿐이고, 대조에는 선언의 모든 노드가 남습니다.",
		EN: "The rows are the nodes registered on the target nodes screen. Access details belong to a node when their node ID equals the declared node name, including case. A node without access details is only left out of this observation; reconciliation keeps every declared node."}
	tTargetsDeclUnknown = T{
		KO: "선언이 아직 없거나 읽을 수 없어 접근 정보의 노드 ID를 견줄 수 없습니다. 「대상 노드와 기대 상태」에서 대상 노드를 먼저 등록하십시오.",
		EN: "There is no declaration yet, or it cannot be read, so the node IDs of the access details cannot be checked. Register the target nodes in the target nodes screen first."}
	tTargetsColState       = T{KO: "접속 준비와 관측", EN: "Access and observation"}
	tTargetUnready         = T{KO: "접속 정보 미입력(아직 관측하지 않는 노드)", EN: "no access details (not observed yet)"}
	tTargetReady           = T{KO: "준비됨", EN: "ready"}
	tTargetObserved        = T{KO: "관측됨: 지금 결과는 %s에 끝난 관측의 것", EN: "observed: the current results come from the observation that finished at %s"}
	tTargetObservedOutside = T{KO: "지금 결과에 있음(화면 밖에서 받은 결과)", EN: "in the current results (received outside the screen)"}
	tTargetFailed          = T{KO: "관측 실패: %s에 끝난 가장 최근 실행이 이 노드에 접속하지 못했거나 이 노드의 단계가 실패함", EN: "observation failed: the most recent run, which ended at %s, could not reach this node or a step failed on it"}
	tOrphansHead           = T{KO: "선언에 없는 행 %d개", EN: "Rows not in the declaration: %d"}
	tOrphansLede           = T{
		KO: "접근 정보(hosts.csv)에는 있고 지금 선언에는 없는 노드 ID입니다. 이 행이 있는 동안에는 관측과 04의 원격 실행을 시작하지 않습니다. 선언에서 노드 이름을 바꿨다면 옛 ID의 행을 새 이름의 노드로 옮길 수 있습니다. 옮길 대상은 직접 고르고, 「저장」을 눌러야 파일이 바뀝니다.",
		EN: "These node IDs are in the access details (hosts.csv) but not in the current declaration. While such a row exists, no observation and no remote run of 04 is started. If you renamed a node in the declaration, you can move the row with the old ID to the node with the new name. You choose the target yourself, and the file changes only when you press Save."}
	tOrphanLine          = T{KO: "접속 정보 %d행: 선언에 없는 노드", EN: "Access details, line %d: a node that is not in the declaration,"}
	tOrphanNoLine        = T{KO: "접속 정보: 선언에 없는 노드", EN: "Access details: a node that is not in the declaration,"}
	tOrphanWhat          = T{KO: "주소 %s · 계정 %s", EN: "address %s · account %s"}
	tOrphanChoose        = T{KO: "이 행을 어떻게 할지 고르십시오", EN: "Choose what to do with this row"}
	tOrphanKeep          = T{KO: "그대로 둠(이 행이 있는 동안 관측을 시작할 수 없습니다)", EN: "Leave it (no observation can start while this row exists)"}
	tOrphanRemove        = T{KO: "이 행을 지움", EN: "Remove this row"}
	tOrphanMoveTo        = T{KO: "선언 노드 %s로 옮김(옛 ID %s → 새 ID %s. 주소 · 계정 · 인증 정보는 그대로)", EN: "Move to the declared node %s (old ID %s → new ID %s; address, account and credentials are kept)"}
	tHostEmpty           = T{KO: "접속 정보 미입력", EN: "no access details"}
	tHostNotDeclared     = T{KO: "지금 선언에 없는 노드입니다. 저장하지 않았고 파일은 그대로입니다. 「대상 노드와 기대 상태」에서 노드 이름을 확인하십시오(이 화면을 연 뒤 선언이 바뀌었을 수 있습니다)", EN: "is not a node of the current declaration. Nothing was saved and the file is unchanged. Check the node name in the target nodes screen (the declaration may have changed after this screen was opened)"}
	tHostDeclUnknown     = T{KO: "선언이 없거나 읽을 수 없어 이 노드가 선언에 있는지 확인하지 못했습니다. 저장하지 않았습니다", EN: "the declaration is missing or cannot be read, so this node could not be checked against it. Nothing was saved"}
	tHostMoveNotDeclared = T{KO: "옮길 대상이 지금 선언에 없습니다. 저장하지 않았고 파일은 그대로입니다", EN: "the target is not in the current declaration. Nothing was saved and the file is unchanged"}
	tHostMoveTaken       = T{KO: "옮길 대상 노드에 이미 접속 정보가 있습니다. 덮어쓰지 않았고 파일은 그대로입니다", EN: "the target node already has access details. Nothing was overwritten and the file is unchanged"}
	tHostOrphanEdit      = T{KO: "선언에 없는 노드의 접속 정보는 고칠 수 없습니다. 그대로 두거나, 지우거나, 선언의 노드로 옮기십시오", EN: "the access details of a node that is not in the declaration cannot be edited. Leave them, remove them or move them to a declared node"}
	tRunTargets          = T{KO: "이번 관측의 대상 %d곳: %s", EN: "Targets of this observation (%d): %s"}
	tRunOmitted          = T{KO: "접속 정보가 없어 이번에 빠지는 노드 %d곳: %s. 대조에는 그대로 남습니다.", EN: "Left out this time for lack of access details (%d): %s. They stay in reconciliation."}
	tSurveySrcAt         = T{KO: "이 대조는 %s에 끝난 관측의 결과로 했습니다(노드 %d곳).", EN: "This reconciliation used the results of the observation that finished at %s (%d nodes)."}
	tSurveySrcOutside    = T{KO: "이 대조는 화면 밖에서 받은 결과로 했습니다(노드 %d곳). 그 결과의 관측 시각은 화면이 알지 못합니다.", EN: "This reconciliation used results received outside the screen (%d nodes). The screen does not know when they were observed."}
	tSurveySrcEmpty      = T{KO: "아래의 선언 자산은 관측과 견준 것이 아닙니다. 성공한 관측에서 찾지 못했다는 뜻이 아닙니다.", EN: "The declared assets below were not compared with an observation. This does not mean that a successful observation failed to find them."}
	tSurveySrcFailed     = T{KO: "가장 최근 실행은 %s에 「%s」로 끝나 결과를 바꾸지 않았습니다.", EN: "The most recent run ended at %s as \"%s\" and did not replace the results."}
	tSurveySrcAbsent     = T{KO: "대조에 쓴 결과에 없는 선언 노드 %d곳: %s. 이 노드들의 미관측은 그 노드를 관측하지 않았다는 뜻입니다.", EN: "Declared nodes that are not in the results used (%d): %s. Their unobserved assets mean the node itself was not observed."}
	tRunStateShort       = map[RunState]T{
		RunFailed: {KO: "실패", EN: "failed"}, RunStopped: {KO: "중지됨", EN: "stopped"},
		RunIncomplete: {KO: "결과가 빠짐", EN: "incomplete"}, RunInterrupted: {KO: "중단됨", EN: "interrupted"},
		RunStuck: {KO: "회수 필요", EN: "needs reclaiming"},
	}
	tSurveyAbsentNode = T{KO: "대조에 쓴 결과에 없는 노드", EN: "node not in the results used"}
	tRunRefusedBusy   = T{
		KO: "이미 관측이 돌고 있습니다. 끝나거나 멈춘 뒤에 다시 시작하십시오.",
		EN: "An observation is already running. Start again after it finishes or is stopped."}
	tRunRunning = T{KO: "관측 중입니다(%s 시작)", EN: "Observing (started at %s)"}
	// 진행(runprogress.go). 단계와 노드별 상태는 실행 로그에서 읽은 것만 적는다.
	tPhaseOf            = T{KO: "지금 단계 %d/%d: %s", EN: "Step %d of %d now: %s"}
	tPhaseNetworkWindow = T{KO: "네트워크 핸드셰이크 관측(관측 구간 %d초 동안 기다립니다)", EN: "observing TLS and SSH handshakes (waits for the %d-second window)"}
	tRunStoppedAt       = T{KO: "멈춘 단계: %d/%d %s", EN: "Stopped at: %d of %d, %s"}
	tRunTimes           = T{KO: "시작 %s · 종료 %s", EN: "Started %s · ended %s"}
	tRunNodesHead       = T{KO: "노드별 상태(로그에서 확인된 것만)", EN: "Per node (only what the log confirms)"}
	tNodeStarted        = T{KO: "수집 중", EN: "collecting"}
	tNodeDone           = T{KO: "끝남", EN: "finished"}
	tNodeUnfinished     = T{KO: "끝나지 않음", EN: "did not finish"}
	tNodeFailed         = T{KO: "실패", EN: "failed"}
	tRunLogDetails      = T{KO: "상세 로그(%d줄)", EN: "Detailed log (%d lines)"}
	tRunAgain           = T{KO: "원인을 고친 뒤 「관측 시작」으로 다시 실행합니다. 지금 결과는 바뀌지 않았습니다.", EN: "Fix the cause, then run it again with “Start observation”. The current results were not changed."}
	tRunAgainStopped    = T{KO: "이어서 하려면 「관측 시작」으로 다시 실행합니다. 지금 결과는 바뀌지 않았습니다.", EN: "To go on, run it again with “Start observation”. The current results were not changed."}
	tRunDone            = T{
		KO: "관측을 마쳤습니다. 받은 결과 파일 %d개가 지금 결과가 됐습니다",
		EN: "The observation finished, and what it received is now the current results (files: %d)"}
	tRunStopped = T{
		KO: "관측을 멈췄습니다. 지금 결과는 그대로이고, 받은 파일 %d개는 진단용으로 따로 두었습니다",
		EN: "The observation was stopped. The current results are unchanged; what arrived is kept aside for diagnosis (files: %d)"}
	tRunFailed = T{
		KO: "관측이 실패했습니다(종료 코드 %d). 지금 결과는 그대로이고, 받은 파일 %d개는 진단용으로 따로 두었습니다. 자세한 오류는 아래 실행 로그에서 확인할 수 있습니다",
		EN: "The observation failed (exit code %d). The current results are unchanged; what arrived is kept aside for diagnosis (files: %d). The detailed errors are in the run log below"}
	tRunIncomplete = T{
		KO: "결과가 오지 않았거나 맞지 않는 자리 %d곳이 있어 지금 결과로 쓰지 않았습니다. 받은 파일 %d개는 진단용으로 따로 두었습니다",
		EN: "Some results did not arrive or do not check out (places: %d), so this run was not used as the current results; what arrived is kept aside for diagnosis (files: %d)"}
	tRunFinishing = T{
		KO: "관측은 끝났고, 노드 정리와 프로세스 회수를 마무리하고 있습니다. 끝나면 결과가 보이고 새 관측을 시작할 수 있습니다.",
		EN: "The observation has ended and the node cleanup and process reclaim are finishing. The result shows, and a new observation can start, when this is done."}
	tRunInterrupted = T{
		KO: "지난 관측은 화면이 도중에 끝나 중단됐습니다. 남은 실행을 끝냈고, 접근 비밀이 담긴 인벤토리를 지웠고, 지금 결과는 그대로입니다",
		EN: "The last observation was interrupted when the screen stopped. What was still running was stopped, the inventory with the access secrets was removed, and the current results are unchanged"}
	tRunStagingLeft = T{
		KO: "노드 정리를 마치지 못했습니다. 노드에 스테이징 폴더 %s가 남았을 수 있으니 확인해 지우십시오",
		EN: "The node cleanup did not finish. Nodes may keep the staging folder %s: check and remove it"}
	tRunStuck = T{
		KO: "이 관측의 프로세스를 끝내지 못했거나 확인하지 못했습니다(%s). 인벤토리와 받던 결과를 지우거나 옮기지 않았고, 새 관측을 시작하지 않습니다. 그 프로세스를 끝낸 뒤 화면을 다시 띄우십시오. 노드의 스테이징 폴더: %s",
		EN: "Processes of this observation did not end or could not be inspected (%s). The inventory and what was arriving were neither removed nor moved, and no new observation starts. Stop those processes, then restart the screen. The staging folder on the nodes: %s"}
	tRunStagingCleaned   = T{KO: "노드의 스테이징 폴더를 지웠습니다", EN: "The staging folders on the nodes were removed"}
	tRunStagingNotNeeded = T{
		KO: "관측이 노드에서 시작되지 않아 노드에서 정리할 것이 없습니다. 노드에서는 점검만 돌았고, 점검은 노드에 아무것도 만들지 않습니다",
		EN: "The observation did not start on the nodes, so there is nothing to clean up on them. Only the node check ran there, and it leaves nothing"}
	tRunMissing = T{KO: "결과가 오지 않았거나 맞지 않는 자리", EN: "Results that did not arrive or do not check out"}
	tRunCurrent = T{
		KO: "지금 결과는 %s에 끝난 관측입니다(노드 %d곳, 파일 %d개)",
		EN: "The current results come from the observation that finished at %s (%d nodes, %d files)"}
)

var runItemText = map[RunItem]T{
	RunHosts:               {KO: "관측할 노드", EN: "Nodes to observe"},
	RunHostsDeclared:       {KO: "접근 정보의 노드 ID가 모두 선언에 있음", EN: "Every node ID in the access details is in the declaration"},
	RunAnsible:             {KO: "ansible-playbook", EN: "ansible-playbook"},
	RunCollectors:          {KO: "수집기 폴더", EN: "Collector folder"},
	RunLinuxBins:           {KO: "Linux 수집기", EN: "Linux collectors"},
	RunWindowsBins:         {KO: "Windows 수집기", EN: "Windows collectors"},
	RunJar:                 {KO: "JVM 관측 에이전트(collector.jar)", EN: "JVM observation agent (collector.jar)"},
	RunStuckItem:           {KO: "끝내지 못한 지난 관측", EN: "Earlier observation not stopped"},
	RunCollectorVersion:    {KO: "수집기 판", EN: "Collector versions"},
	RunCollectorMismatch:   {KO: "수집기 판이 맞지 않음", EN: "Collector versions do not match"},
	RunCollectorUnverified: {KO: "수집기 판을 확인할 수 없음", EN: "Collector versions cannot be verified"},
	RunCollectorAllowed:    {KO: "판을 확인하지 못한 수집기를 허용함", EN: "Unverified collectors allowed"},
	RunJarMismatch:         {KO: "collector.jar 판이 맞지 않음", EN: "collector.jar version does not match"},
	RunJarUnverified:       {KO: "collector.jar를 확인할 수 없음", EN: "collector.jar cannot be verified"},
	RunJarAllowed:          {KO: "확인하지 못한 collector.jar를 허용함", EN: "Unverified collector.jar allowed"},
	RunWindowsUnsupported:  {KO: "Windows 노드(이번 판의 01은 지원하지 않음)", EN: "Windows nodes (not supported by 01 in this version)"},
	RunCollectorKeys:       {KO: "수집기 서명 키", EN: "Collector signing keys"},
	RunCollectorVerify:     {KO: "신뢰할 수집기 공개 키", EN: "Trusted collector public keys"},
}

var runItemMissing = map[RunItem]T{
	RunHosts: {KO: "위 표에 노드를 적고 저장하십시오", EN: "Write the nodes in the table above and save"},
	RunHostsDeclared: {
		KO: "선언에 없는 행을 지워 저장하거나, 그 노드를 「대상 노드와 기대 상태」에 등록하십시오. 노드 ID는 선언의 노드 이름과 대소문자까지 같아야 합니다",
		EN: "Remove the rows that are not in the declaration and save, or register those nodes in the target nodes screen. A node ID must equal the declared node name, including case"},
	RunAnsible: {
		KO: "이 기계에 Ansible을 설치하십시오. PATH에서 ansible-playbook을 찾습니다",
		EN: "Install Ansible on this machine: ansible-playbook is looked up on the PATH"},
	RunCollectors: {
		KO: "작업 폴더에 collectors 폴더를 두거나 -collectors <폴더>로 시작하십시오",
		EN: "Put a collectors folder in the workspace, or start with -collectors <folder>"},
	RunLinuxBins: {
		KO: "linux-amd64(또는 linux-arm64)에 pqcota-nodescan · pqcota-netcap · pqcota-jvmscan을 두십시오",
		EN: "Put pqcota-nodescan, pqcota-netcap and pqcota-jvmscan in linux-amd64 (or linux-arm64)"},
	RunWindowsBins: {
		KO: "windows-amd64에 pqcota-cngscan.exe · pqcota-jvmscan.exe를 두십시오",
		EN: "Put pqcota-cngscan.exe and pqcota-jvmscan.exe in windows-amd64"},
	RunStuckItem: {
		KO: "지난 관측의 프로세스가 아직 남았습니다. 아래 상태의 PID를 끝낸 뒤 화면을 다시 띄우십시오",
		EN: "Processes of an earlier observation are still there. Stop the PIDs shown below, then restart the screen"},
	RunJar: {
		KO: "없어도 시작하지만, 실행 중인 JVM의 provider 체인은 관측하지 않습니다",
		EN: "The observation starts without it, but the provider chains of running JVMs are not observed"},
	RunCollectorMismatch: {
		KO: "옆에 적힌 판의 상류 릴리스 자산으로 바꾸십시오. 판이 다르거나, 판은 같아도 내용 해시(h1)가 다른 수집기는 -allow-unverified-collectors로도 쓸 수 없습니다",
		EN: "Replace them with the upstream release assets of the version shown. Collectors of another version, or of the same version with another content hash (h1), cannot be used even with -allow-unverified-collectors"},
	RunCollectorUnverified: {
		KO: "옆에 적힌 판의 상류 릴리스 자산을 두십시오. 판을 확인하지 못한 채로 쓰려면 화면을 -allow-unverified-collectors로 시작하십시오",
		EN: "Put the upstream release assets of the version shown. To use them without a verified version, start the screen with -allow-unverified-collectors"},
	RunCollectorAllowed: {
		KO: "-allow-unverified-collectors로 시작해, 판을 확인하지 못한 수집기로 관측합니다. 이 허용은 화면을 끄면 사라지고 작업 폴더에 남지 않습니다",
		EN: "Started with -allow-unverified-collectors: the observation uses collectors whose version is not verified. The allowance ends with the screen and is not kept in the workspace"},
	RunJarMismatch: {
		KO: "collector.jar의 판이 옆에 적힌 판과 다릅니다. 같은 판으로 빌드한 것으로 바꾸십시오",
		EN: "The version of collector.jar differs from the version shown. Replace it with one built at that version"},
	RunJarUnverified: {
		KO: "collector.jar의 판 · 커밋 · 수정 여부를 확인할 수 없습니다(판 정보 없음 · 직접 빌드 · 고친 트리 · 커밋 정보 없음). 옆에 적힌 판의 상류 릴리스 collector.jar로 바꾸십시오. 그대로 쓰려면 화면을 -allow-unverified-collectors로 시작하십시오",
		EN: "The version, commit or modified flag of collector.jar cannot be verified (no version, a local build, a modified tree or no commit). Replace it with the upstream release collector.jar of the version shown. To use it as it is, start the screen with -allow-unverified-collectors"},
	RunWindowsUnsupported: {
		KO: "이번 판의 01은 Windows 노드에 서명 키를 넘기지 않아, 관측해도 그 결과가 서명 검증을 지나지 못합니다. 위 표에서 옆에 적힌 노드를 빼십시오. 그 노드는 pqcota-cngscan · pqcota-jvmscan을 서명 키와 함께 직접 돌려 관측합니다",
		EN: "In this version 01 does not pass signing keys to Windows nodes, so their results would not pass the signature check. Remove the nodes shown from the table above, and observe them by running pqcota-cngscan and pqcota-jvmscan yourself with their signing keys"},
	RunCollectorKeys: {
		KO: "관측은 수집기 ID마다 서명 키가 있어야 시작합니다. 옆에 적힌 폴더(작업 폴더 밖, chmod 700)에 ID마다 pqcota-keygen -collector <ID>의 출력을 <ID>.key로, 소유자만 읽게(umask 077) 저장하십시오. 옆에 빠진 ID와 그 까닭이 있습니다",
		EN: "An observation starts only with a signing key for each collector ID. In the folder shown (outside the workspace, chmod 700), save the output of pqcota-keygen -collector <ID> as <ID>.key for each ID, readable only by its owner (umask 077). The IDs missing and why are shown"},
	RunCollectorVerify: {
		KO: "신뢰할 공개 키 파일에 적재(pqcota-ingest)에 주는 것과 같은 PQCOTA_VERIFY_KEY 한 줄을 두십시오. <ID>:<공개 키>를 쉼표로 이은 결합 형식이고, 서명 키마다 그 공개 키가 자기 ID 아래 있어야 합니다. 한 공개 키를 두 ID에 두거나 승인 키를 서명 키로 쓰면 시작하지 않습니다",
		EN: "Put in the trusted public key file the same PQCOTA_VERIFY_KEY line you give the ingest (pqcota-ingest): <ID>:<public key> entries, comma-separated, with each signing key's public key under its own ID. One public key under two IDs, or the approval key used as a signing key, keeps the observation from starting"},
	RunJarAllowed: {
		KO: "-allow-unverified-collectors로 시작해, 확인하지 못한 collector.jar로 관측합니다. 이 허용은 화면을 끄면 사라지고 작업 폴더에 남지 않습니다",
		EN: "Started with -allow-unverified-collectors: the observation uses a collector.jar that is not verified. The allowance ends with the screen and is not kept in the workspace"},
}

// 「회수 필요」 프로세스 정보(01).
var (
	tProcHead        = T{KO: "남은 프로세스", EN: "Processes left behind"}
	tProcRecheck     = T{KO: "다시 확인", EN: "Check again"}
	tProcRecheckHint = T{KO: "다시 읽기만 합니다. 파일 · 기록은 바꾸지 않고, 프로세스를 끝내지도 않습니다.", EN: "This only reads again. It changes no file or record and stops no process."}
	tProcPID         = T{KO: "PID", EN: "PID"}
	tProcPPID        = T{KO: "부모 PID", EN: "Parent PID"}
	tProcPGID        = T{KO: "프로세스 묶음", EN: "Process group"}
	tProcUser        = T{KO: "사용자", EN: "User"}
	tProcExe         = T{KO: "실행 파일", EN: "Executable"}
	tProcStarted     = T{KO: "시작 시각", EN: "Started"}
	tProcArgs        = T{KO: "인자 보기(비밀값은 가림)", EN: "Show arguments (secrets masked)"}
	tProcNoAccess    = T{KO: "읽을 권한 없음", EN: "no permission to read"}
)

var procStateText = map[ProcState]T{
	ProcSame: {KO: "이 관측이 남긴 프로세스입니다. 아래 정보로 확인한 뒤 운영자가 정상 종료하고 화면을 다시 시작하십시오.",
		EN: "A process this observation left behind. Check it with the details below, stop it normally, then restart the screen."},
	ProcGone: {KO: "이미 종료됨. 화면을 다시 시작하면 남은 정리를 이어서 합니다.",
		EN: "Already ended. Restart the screen and it carries on with the cleanup that is left."},
	ProcReused: {KO: "다른 프로세스로 바뀜: 같은 PID를 다른 프로세스가 쓰고 있습니다. 회수 대상이 아니니 끝내지 마십시오.",
		EN: "Replaced by another process: another process now uses this PID. It is not to be reclaimed; do not stop it."},
	ProcDenied: {KO: "조회 권한 없음: 이 화면을 띄운 사용자의 프로세스가 아니거나 읽을 수 없습니다. 소속을 확인할 수 없으니 끝내지 마십시오.",
		EN: "No permission: the process is not this user's or cannot be read. Its belonging cannot be checked; do not stop it."},
	ProcFailed: {KO: "조회 실패: 프로세스 정보를 읽지 못했습니다. 잠시 뒤 다시 확인하십시오.",
		EN: "Lookup failed: the process details could not be read. Check again in a moment."},
	ProcUnconfirmed: {KO: "같은 프로세스인지 확인할 수 없음: 시작 시각을 남기지 않은 이전 판의 기록입니다. 끝내기 전에 운영자 안내서의 확인 명령으로 직접 확인하십시오.",
		EN: "Cannot tell whether it is the same process: an earlier version did not record its start time. Before stopping it, check it yourself with the command in the operator guide."},
}

// 수집기 파일 한 줄: 「선언된 판: v0.10.7 · 실제 소스: 로컬 replace(devel) · 확인 불가」.
var (
	tCVDeclared     = T{KO: "선언된 판: %s", EN: "declared version: %s"}
	tCVSource       = T{KO: "실제 소스: %s", EN: "actual source: %s"}
	tCVNone         = T{KO: "없음", EN: "none"}
	tCVTagCommit    = T{KO: "태그 커밋 %s", EN: "tag commit %s"}
	tCVModuleSum    = T{KO: "%s 모듈 · 내용 해시(h1) 일치", EN: "%s module · content hash (h1) matches"}
	tCVFullCommit   = T{KO: "커밋 %s", EN: "commit %s"}
	tCVDepsReplaced = T{KO: "수집기 소스 %s 확인 · replace 의존 소스 확인 불가", EN: "collector source %s verified · replaced dependency sources cannot be verified"}
)

var collectorStateText = map[string]T{
	"verified":   {KO: "확인됨", EN: "verified"},
	"mismatch":   {KO: "맞지 않음", EN: "does not match"},
	"unverified": {KO: "확인 불가", EN: "cannot be verified"},
}

var collectorReasonText = map[string]T{
	"no-build-info":  {KO: "Go 빌드 정보 없음", EN: "no Go build information"},
	"other-module":   {KO: "상류 수집기가 아닌 모듈", EN: "a module other than the upstream collectors"},
	"local-replace":  {KO: "로컬 replace(devel)", EN: "local replace (devel)"},
	"devel":          {KO: "판 없음(devel)", EN: "no version (devel)"},
	"dirty":          {KO: "고친 트리(dirty)", EN: "modified tree (dirty)"},
	"pseudo-version": {KO: "태그가 아닌 커밋", EN: "a commit that is not a tag"},
	"old-version":    {KO: "다른 판", EN: "another version"},
	"no-version":     {KO: "판 정보 없음", EN: "no version information"},
	"unreadable":     {KO: "읽을 수 없음", EN: "unreadable"},
	"no-pin":         {KO: "이 화면이 고정한 판을 알 수 없음", EN: "the version this screen pins is unknown"},
	"deps-replaced":  {KO: "replace 의존 소스 확인 불가", EN: "replaced dependency sources cannot be verified"},
	"other-content":  {KO: "판은 같지만 내용 해시(h1)가 다름", EN: "same version, another content hash (h1)"},
	"no-sum":         {KO: "모듈 내용 해시(h1) 없음(직접 빌드)", EN: "no module content hash (h1) (built locally)"},
	"no-commit":      {KO: "커밋 · 수정 여부 정보 없음", EN: "no commit or modified flag"},
}

// ── 04 프로비저닝(승인 서명 · 플레이북 생성) ──────────────────────────────────────────────────────────
var (
	tApproveEyebrow = T{KO: "04 · 프로비저닝", EN: "04 · Provisioning"}
	tApproveHead    = T{
		KO: "확정한 계획에 승인 서명을 하고 플레이북을 만듭니다",
		EN: "Sign the approval of the finalized plan and generate the playbooks"}
	tApproveLede = T{
		KO: "서명하면 승인된 계획(plan.signed.json)이 FINALIZED로 따로 생기고, 원본 계획(plan.json)은 그대로입니다. 플레이북은 승인된 계획으로 적용과 되돌리기를 함께 만들고, 운영 기계에 적용하는 일은 운영자가 Ansible로 합니다.",
		EN: "Signing writes the approved plan (plan.signed.json) as FINALIZED beside the original plan (plan.json), which stays as it is. " +
			"The apply and rollback playbooks are generated together from the approved plan; " +
			"applying them to the machines is the operator's own Ansible run."}
	tApproveNoPlan     = T{KO: "아직 확정한 계획이 없습니다", EN: "There is no finalized plan yet"}
	tApproveNoPlanBody = T{
		KO: "03 판정에서 확정하면 plan.json이 생기고 여기서 승인할 수 있습니다.",
		EN: "Finalizing in 03 Judgment writes plan.json, which can then be approved here."}
	tPlanHead       = T{KO: "승인할 계획", EN: "The plan to approve"}
	tPlanOriginal   = T{KO: "원본 계획(plan.json): %s", EN: "Original plan (plan.json): %s"}
	tPlanApproved   = T{KO: "승인된 계획(plan.signed.json): %s", EN: "Approved plan (plan.signed.json): %s"}
	tPlanSignedOnly = T{
		KO: "서명한 계획(plan.signed.json): %s. 지금 계획과 맞지 않거나 이 키로 확인되지 않아 승인된 계획으로 보지 않습니다",
		EN: "Signed plan (plan.signed.json): %s. It does not match the current plan or does not check out with this key, so it is not treated as the approved plan"}
	tColProvider = T{KO: "provider", EN: "Provider"}

	tSignHead    = T{KO: "승인 서명", EN: "Approval signature"}
	tKeyItem     = T{KO: "승인 키", EN: "Approval key"}
	tSignedValid = T{
		KO: "승인했습니다: %s(%s). 이 키로 확인했습니다",
		EN: "Approved by %s (%s), checked with this key"}
	tSignedStale = T{
		KO: "서명한 계획(plan.signed.json)이 지금 계획과 다릅니다. 03에서 다시 확정했다면 새 계획에 다시 서명하십시오.",
		EN: "The signed plan (plan.signed.json) differs from the current plan. If 03 was finalized again, sign the new plan."}
	tSignedInvalid = T{
		KO: "서명한 계획의 서명을 이 키로 확인할 수 없습니다. 다른 키로 서명했거나 서명 뒤에 파일이 바뀌었습니다.",
		EN: "The signed plan's signature does not check out with this key: it was signed with another key, or the file changed after signing."}
	tVerifyKeyHint = T{
		KO: "pqcota-provision으로 직접 생성할 때는 아래 줄을 환경 변수로 줍니다. 공개 키라 비밀이 아닙니다.",
		EN: "When generating with pqcota-provision yourself, pass the line below as an environment variable. It is a public key, not a secret."}
	tApprover    = T{KO: "승인자", EN: "Approver"}
	tReviewedAsk = T{KO: "위 조치를 검토했고 승인합니다", EN: "I have reviewed the actions above and approve them"}
	tSign        = T{KO: "승인 서명", EN: "Sign the approval"}
	tSignHint    = T{
		KO: "<b>서명은 계획의 내용 전부를 덮습니다.</b> 서명한 뒤에 계획을 고치면 승인은 무효가 됩니다.<br>" +
			"서명한 계획은 plan.signed.json에 따로 남고 plan.json은 그대로입니다. 무엇에 서명했는지 되짚을 수 있게 하려는 것입니다.<br>" +
			"승인자 이름은 서명 안에 들어가고, 확인은 그 이름의 키로만 합니다. 콜론(:)은 쓸 수 없습니다.",
		EN: "<b>The signature covers the whole content of the plan.</b> Editing the plan after signing voids the approval.<br>" +
			"The signed plan is kept separately in plan.signed.json and plan.json stays as it is, so that what was signed can be traced.<br>" +
			"The approver's name goes inside the signature, and it is checked only with that name's key. A colon (:) cannot be used."}

	tGenHead   = T{KO: "플레이북 생성", EN: "Playbook generation"}
	tGenSource = T{KO: "생성 대상: 승인된 계획(plan.signed.json)", EN: "Generated from: the approved plan (plan.signed.json)"}
	tGenStale  = T{
		KO: "만든 플레이북이 지금 서명한 계획보다 오래됐습니다. 다시 만드십시오.",
		EN: "The playbooks are older than the signed plan. Generate them again."}
	tGenMade = T{
		KO: "플레이북을 만들었습니다(기본 수준 %s, %s)",
		EN: "The playbooks were generated (default level %s, %s)"}
	tGenWarningsHead = T{KO: "생성기가 짚은 빈 곳 %d곳", EN: "Gaps the generator flagged: %d"}
	tGenWarningsLede = T{
		KO: "플레이북은 만들어졌지만 아래 자리가 비어 있습니다. pqcota-provision이라면 종료 상태 3으로 끝나는 경우입니다. 03에서 채워 다시 확정하거나, 알고 넘어가십시오.",
		EN: "The playbooks were generated, but the places below are empty; pqcota-provision would end with exit status 3. " +
			"Fill them in at 03 and finalize again, or go on knowingly."}
	tApplyHint = T{
		KO: "<b>화면이 적용하지 않는 플레이북은 운영자가 실행합니다.</b> 관측에 쓴 것과 같은 인벤토리로 실행합니다. " +
			"<code>pqcota-hosts --ansible-out targets.ini hosts.csv</code>로 만든 뒤 " +
			"<code>ansible-playbook -i targets.ini plan.provision.yml</code>을 실행하고, 되돌릴 때는 plan.rollback.yml을 실행합니다. " +
			"targets.ini에는 접근 비밀이 들어가니 끝나면 지우십시오.<br>" +
			"provider 모듈 파일은 도구가 주지 않습니다. 플레이북이 컨트롤러의 files/ 아래에서 찾습니다.",
		EN: "<b>What the screen does not apply, the operator runs.</b> Run it with the same inventory the observation used: make it with " +
			"<code>pqcota-hosts --ansible-out targets.ini hosts.csv</code>, then run " +
			"<code>ansible-playbook -i targets.ini plan.provision.yml</code>; to undo, run plan.rollback.yml. " +
			"targets.ini holds access secrets, so remove it afterwards.<br>" +
			"The tool does not supply provider module files; the playbook looks for them under files/ on the controller."}
	tGenLevel = T{KO: "기본 위임 수준", EN: "Default delegation level"}
	tLevelL1  = T{KO: "L1 · 모듈만 둔다", EN: "L1 · stage the module only"}
	tLevelL2  = T{KO: "L2 · 설정까지 둔다", EN: "L2 · stage the module and the configuration"}
	tLevelL3  = T{KO: "L3 · 활성화하고 재시작한다", EN: "L3 · activate and restart"}
	tGenerate = T{KO: "플레이북 생성", EN: "Generate the playbooks"}
	tGenHint  = T{
		KO: "<b>기본 수준은 계획이 수준을 밝히지 않은 조치에만 씁니다.</b> 조치가 수준을 밝혔으면 그것을 따릅니다. 그 값은 승인 서명이 덮습니다.<br>" +
			"계획 파일 옆에 plan.provision.yml과 plan.rollback.yml을 씁니다(계획 파일 이름을 따릅니다). 다시 만들면 갈아 끼웁니다.",
		EN: "<b>The default level applies only to actions whose level the plan does not state.</b> An action that states its level keeps it; that value is covered by the approval signature.<br>" +
			"plan.provision.yml and plan.rollback.yml are written beside the plan file (named after it), and generating again replaces them."}
	tGenNeedsSign = T{
		KO: "지금 계획에 이 키로 확인되는 승인 서명이 있어야 만들 수 있습니다.",
		EN: "Generation needs an approval on the current plan that checks out with this key."}

	tStepApproveNoPlan     = T{KO: "아직 확정한 계획이 없습니다", EN: "no finalized plan yet"}
	tStepApproveSign       = T{KO: "승인 서명이 필요합니다", EN: "approval needed"}
	tStepApproveStale      = T{KO: "서명이 지금 계획과 맞지 않습니다", EN: "the signature does not match the plan"}
	tStepApproveGenerate   = T{KO: "플레이북을 만들 수 있습니다", EN: "ready to generate"}
	tStepApproveIncomplete = T{KO: "빈 곳 %d곳", EN: "%d gaps"}
	tStepApproveDone       = T{KO: "플레이북을 만들었습니다", EN: "playbooks generated"}

	tSignLinePlan     = T{KO: "계획 %s · 조치 %d개", EN: "Plan %s, actions: %d"}
	tSignLineHash     = T{KO: "계획 파일 SHA-256 %s", EN: "Plan file SHA-256 %s"}
	tSignLineApprover = T{KO: "승인자 %s", EN: "Approver %s"}
	tSignLineKey      = T{KO: "승인 키 지문 %s", EN: "Approval key fingerprint %s"}
	tSignDo           = T{KO: "이 내용으로 서명", EN: "Sign this"}
	tSignConfirmTitle = T{KO: "서명 전 확인", EN: "Confirm before signing"}
	tSignConfirmBody  = T{
		KO: "아래 계획에 이 승인자와 이 키로 승인 서명을 합니다. 서명한 계획은 따로 남고 덮지 않습니다. 아직 아무것도 쓰지 않았습니다.",
		EN: "This signs the approval of the plan below as this approver with this key. The signed plan is kept separately and never overwritten. Nothing has been written yet."}
	tSignConfirmBack = T{KO: "서명하지 않고 화면으로 돌아가기", EN: "Back to the screen without signing"}
	tSignedExists    = T{
		KO: "서명한 계획 파일이 이미 있어 덮지 않았습니다. 지금 계획과 맞지 않으면 04 화면에서 지난 서명을 보관한 뒤 서명하십시오.",
		EN: "A signed plan file already exists and was not overwritten. If it does not match the current plan, archive the earlier signature on the 04 screen, then sign."}
	tArchive    = T{KO: "지난 서명 보관", EN: "Archive the earlier signature"}
	tArchiveAsk = T{
		KO: "서명한 계획과 그 플레이북을 지우지 않고, 이름에 지금 시각을 붙여 옆에 둡니다. 그 뒤 새 계획에 서명할 수 있습니다.",
		EN: "The signed plan and its playbooks are not deleted: they are kept beside it with the current time in their names. Then the new plan can be signed."}
	tArchivedNotice = T{KO: "지난 서명을 보관했습니다. 파일 %d개", EN: "The earlier signature was archived (files: %d)"}
	tSingleKey      = T{
		KO: "04 화면은 승인 키 하나로 서명하고 확인합니다. 서명마다 승인자 이름이 들어가지만, 확인은 이 화면에 준 키 하나로 합니다(단일 키 · 단일 승인자).",
		EN: "The 04 screen signs and checks with a single approval key. Each signature carries the approver's name, but checking uses the one key given to this screen (single key, single approver)."}

	tSignedNotice    = T{KO: "승인 서명을 했습니다. 승인된 계획(plan.signed.json)이 FINALIZED이고, 원본 계획(plan.json)은 그대로 둡니다", EN: "The approval was signed: the approved plan (plan.signed.json) is FINALIZED, and the original plan (plan.json) stays as it is"}
	tGeneratedNotice = T{KO: "플레이북을 만들었습니다. 빈 곳 %d곳", EN: "The playbooks were generated: %d gaps"}

	tSignRefusedHead = T{KO: "서명하지 않았습니다", EN: "Not signed"}
	tSignNotReady    = T{
		KO: "승인 키를 쓸 수 없거나, 승인자 이름이 비었거나 콜론을 담았거나, 검토 확인을 하지 않았습니다. 04 화면의 점검을 보십시오.",
		EN: "The approval key cannot be used, the approver's name is empty or holds a colon, or the review was not confirmed. See the checks on the 04 screen."}
	tPlanNotApprovable = T{
		KO: "이 계획은 승인할 수 없습니다. 검토 중(IN_REVIEW) 상태가 아니거나, 조치가 없거나, 조치에 대상 노드 · 종류가 비었거나, 이미 이 키로 서명했습니다.",
		EN: "This plan cannot be approved: it is not in review (IN_REVIEW), has no actions, an action lacks its target node or kind, " +
			"or it is already signed with this key."}
	tGenRefusedHead = T{KO: "플레이북을 만들지 않았습니다", EN: "The playbooks were not generated"}
	tGenNotReady    = T{
		KO: "지금 계획에 이 키로 확인되는 승인 서명이 없습니다. 먼저 승인 서명을 하십시오.",
		EN: "The current plan has no approval that checks out with this key. Sign the approval first."}
)

var keyText = map[KeyState]T{
	KeyOK: {KO: "쓸 수 있습니다", EN: "usable"},
	KeyNotGiven: {
		KO: "-approval-key <파일>로 시작하십시오. 키는 pqcota-keygen의 출력을 그대로 파일에 저장해 만듭니다",
		EN: "Start with -approval-key <file>. Make the key by saving the output of pqcota-keygen to a file"},
	KeyUnreadable: {KO: "파일을 읽을 수 없습니다", EN: "the file cannot be read"},
	KeyOpen: {
		KO: "다른 사용자도 읽을 수 있습니다. chmod 600으로 소유자만 읽게 하십시오",
		EN: "other users can read it: make it owner-only with chmod 600"},
	KeyInside: {
		KO: "작업 폴더 안에 있습니다. 작업 폴더 밖으로 옮기십시오. 작업 폴더를 백업하면 키가 함께 퍼집니다",
		EN: "it is inside the workspace: move it out. Backing up the workspace would spread the key"},
	KeyBad: {
		KO: "pqcota-keygen이 낸 개인 키(PQCOTA_SIGN_KEY)를 찾지 못했습니다",
		EN: "no private key from pqcota-keygen (PQCOTA_SIGN_KEY) was found in it"},
	KeyLink: {
		KO: "심볼릭 링크입니다. 링크가 아닌 파일 자체를 주십시오",
		EN: "it is a symbolic link: give the file itself, not a link"},
	KeyOwner: {
		KO: "이 화면을 띄운 사용자의 파일이 아닙니다",
		EN: "it does not belong to the user running this screen"},
	KeyLinks: {
		KO: "하드 링크가 더 있습니다. 다른 이름으로 바뀔 수 있으니 링크가 하나인 파일로 두십시오",
		EN: "it has more hard links, so it can change under another name: keep it as a file with a single link"},
	KeyHasPrivate: {
		KO: "확인용 공개 키 파일에 개인 키가 들어 있습니다. PQCOTA_VERIFY_KEY 줄만 두십시오",
		EN: "the verification key file holds a private key: keep only the PQCOTA_VERIFY_KEY line"},
	KeyUnwritable: {
		KO: "다른 사용자도 고칠 수 있습니다. chmod go-w로 소유자만 고치게 하십시오",
		EN: "other users can change it: make it writable by its owner only with chmod go-w"},
	KeyNotCollector: {
		KO: "수집기 ID가 없는 키입니다. pqcota-keygen -collector <ID>로 만드십시오",
		EN: "the key names no collector ID: make it with pqcota-keygen -collector <ID>"},
	KeyOtherCollector: {
		KO: "파일 이름과 다른 수집기 ID의 키입니다",
		EN: "the key is for another collector ID than its file name"},
	KeyLegacyList: {
		KO: "수집기 ID에 묶이지 않은 옛 목록 꼴입니다. <ID>:<공개 키>를 쉼표로 이어 적으십시오",
		EN: "it is the old list that binds no key to a collector ID: write <ID>:<public key> entries, comma-separated"},
	KeyCollector: {
		KO: "수집기 서명용 키입니다(PQCOTA_VERIFY_KEY 줄에 수집기 ID가 붙어 있습니다). 승인 키는 pqcota-keygen을 플래그 없이 실행해 만드십시오",
		EN: "it is a collector signing key (its PQCOTA_VERIFY_KEY line names a collector ID): make the approval key with pqcota-keygen and no flags"},
}

// 04 의 승인 키 문구(서명용 개인 키와 확인용 공개 키).
var (
	tKeyDefaultOK      = T{KO: "기본 승인 키를 사용합니다.", EN: "The default approval key is used."}
	tKeyExplicitOK     = T{KO: "지정한 승인 키를 사용합니다.", EN: "The approval key given at start is used."}
	tKeyWhere          = T{KO: "위치: 사용자 설정 폴더의 %s", EN: "Location: %s in the user configuration folder"}
	tKeyFingerprint    = T{KO: "키 지문: %s", EN: "Key fingerprint: %s"}
	tKeyMissingText    = T{KO: "승인 키가 없습니다. 새 승인에 서명하려면 기본 위치에 키를 만들거나 -approval-key로 지정하십시오.", EN: "There is no approval key. To sign new approvals, create the key at the default location or give it with -approval-key."}
	tKeyDefaultBad     = T{KO: "기본 승인 키를 사용할 수 없습니다: %s. 새 승인 서명은 막혔습니다. 파일의 소유자 · 권한 · 링크 · 형식을 확인하십시오.", EN: "The default approval key cannot be used: %s. New approval signatures are blocked. Check the file's owner, permissions, links and format."}
	tKeyDisabledText   = T{KO: "기본 승인 키 자동 검색을 껐습니다. 새 승인에 서명하려면 -approval-key로 키를 지정하십시오.", EN: "The automatic search for the default approval key is off. To sign new approvals, give the key with -approval-key."}
	tKeyMismatchText   = T{KO: "승인 키와 확인용 공개 키가 서로 다른 키입니다. 새 승인 서명과 서명 확인을 막았습니다. 두 파일을 같은 pqcota-keygen 출력으로 맞추십시오.", EN: "The approval key and the verification key are different keys. New signatures and signature checks are blocked. Make both files come from the same pqcota-keygen output."}
	tKeyConflictText   = T{KO: "고른 확인용 공개 키를 쓸 수 없어 구성이 맞지 않습니다. 새 승인 서명과 플레이북 생성을 막았습니다. 아래 확인용 공개 키 줄을 보십시오.", EN: "The verification key chosen cannot be used, so the configuration is inconsistent. New signatures and playbook generation are blocked. See the verification key line below."}
	tKeyVerifyOnlyText = T{KO: "확인용 공개 키만 지정했습니다. 새 승인에 서명하려면 -approval-key로 개인 키를 지정하십시오.", EN: "Only a verification key was given. To sign new approvals, give the private key with -approval-key."}
	tKeyCanVerify      = T{KO: "이미 서명된 계획은 확인용 공개 키로 계속 확인하고 생성할 수 있습니다.", EN: "Plans already signed can still be checked and generated with the verification key."}
	tKeyNoVerify       = T{KO: "이미 서명된 계획을 확인하고 생성하려면 확인용 공개 키(pqcaton/approval.pub)를 두십시오.", EN: "To check and generate plans already signed, put the verification key (pqcaton/approval.pub)."}
	tVerifyKeyOK       = T{KO: "확인용 공개 키: %s · 키 지문: %s", EN: "Verification key: %s · key fingerprint: %s"}
	tVerifyKeyBad      = T{KO: "확인용 공개 키를 사용할 수 없습니다(%s): %s", EN: "The verification key cannot be used (%s): %s"}
)

// kindNames — 조치 종류를 그 말로 풀어 쓴 뜻. 상류 계약(plan.proto의 RemediationKind)의 설명을 따른다. 값은 바꾸지 않는다.
var kindNames = map[string]T{
	"REMEDIATION_KIND_CONFIG_ONLY":     {KO: "설정만 변경", EN: "Config change only"},
	"REMEDIATION_KIND_PROVIDER_INJECT": {KO: "프로바이더 추가 및 설정", EN: "Add and configure a provider"},
	"REMEDIATION_KIND_JDK_UPGRADE":     {KO: "JDK 업그레이드", EN: "JDK upgrade"},
	"REMEDIATION_KIND_FORK_REPLACE":    {KO: "포크로 교체", EN: "Replace with a fork"},
	"REMEDIATION_KIND_PROXY_FRONT":     {KO: "앞단에 프록시 배치", EN: "Place a proxy in front"},
	"REMEDIATION_KIND_APP_RECONFIG":    {KO: "앱 코드 또는 설정 변경", EN: "Change app code or configuration"},
	"REMEDIATION_KIND_REBUILD":         {KO: "다시 빌드", EN: "Rebuild"},
	"REMEDIATION_KIND_DECOMMISSION":    {KO: "폐기 또는 위험 수용", EN: "Decommission or accept the risk"},
}

// phaseNames — 관측 실행의 단계 이름.
var phaseNames = map[RunPhase]T{
	PhaseCheck:   {KO: "노드 접속과 권한 점검", EN: "checking access to the nodes"},
	PhaseStaging: {KO: "수집기를 노드에 올리는 중", EN: "copying the collectors to the nodes"},
	PhaseOpenSSL: {KO: "OpenSSL 런타임 관측", EN: "observing the OpenSSL runtime"},
	PhaseJCA:     {KO: "JCA 프로바이더 관측", EN: "observing the JCA providers"},
	PhaseNetwork: {KO: "네트워크 핸드셰이크 관측", EN: "observing TLS and SSH handshakes"},
	PhaseCleanup: {KO: "노드의 스테이징 폴더 정리", EN: "removing the staging folders on the nodes"},
}

// 04 프로비저닝: 대상 노드 미리 보기와 실행 결과(provrun.go · provrun.templ).
var (
	tProvHead = T{KO: "대상 노드 미리 보기", EN: "Preview on the target nodes"}
	tProvLede = T{
		KO: "승인된 계획에서 L2로 생성한, 설정 파일만 놓는 플레이북을 대상 노드에 --check로 실행해 무엇이 바뀔지 확인합니다. 노드를 바꾸지 않습니다.",
		EN: "Runs the playbook generated at L2 from the approved plan (it only places configuration files) against the target nodes with --check, to see what would change. It does not change the nodes."}
	tProvScope = T{
		KO: "화면은 별도 확인을 거쳐 L2 플레이북을 대상 노드에 적용합니다. 활성화와 되돌리기는 운영자가 수행합니다.",
		EN: "After a separate confirmation the screen applies the L2 playbook to the target nodes. Activating and rolling back are the operator's job."}
	tApplyGo           = T{KO: "%d개 노드에 적용: 확인으로", EN: "Apply to %d nodes: go to the confirmation"}
	tApplyDo           = T{KO: "%d개 노드에 적용", EN: "Apply to %d nodes"}
	tApplyConfirmTitle = T{KO: "적용 확인", EN: "Confirm the apply"}
	tApplyConfirmBody  = T{
		KO: "아래 내용을 대상 노드에 적용합니다. 이 화면을 여는 것만으로는 아무것도 바뀌지 않고 미리 보기도 쓰이지 않습니다. 미리 보기는 적용이 원격 작업을 시작하기 직전에 한 번 쓰입니다.",
		EN: "What is below will be applied to the target nodes. Opening this page changes nothing and does not use up the preview. The preview is used once, just before the apply starts its remote work."}
	tApplyLineOrg      = T{KO: "조직: %s", EN: "Organization: %s"}
	tApplyLineNodes    = T{KO: "대상 노드 %d개: %s", EN: "Target nodes (%d): %s"}
	tApplyLinePlan     = T{KO: "승인된 계획: %s (SHA-256 %s)", EN: "Approved plan: %s (SHA-256 %s)"}
	tApplyLinePlaybook = T{KO: "생성된 플레이북: %s (SHA-256 %s)", EN: "Generated playbook: %s (SHA-256 %s)"}
	tApplyLinePreview  = T{KO: "미리 보기: 실행 %s, %s에 끝남, %s까지 유효", EN: "Preview: run %s, ended %s, good until %s"}
	tApplyLineChange   = T{KO: "%s의 변경 예정 %d건: %s", EN: "%s, %d to change: %s"}
	tApplyLineNoChange = T{KO: "%s: 바뀔 것 없음", EN: "%s: nothing to change"}
	tApplyLineDrift    = T{
		KO: "미리 본 뒤에도 노드의 상태는 바뀔 수 있습니다. 적용은 미리 본 것과 같은 입력으로만 시작합니다.",
		EN: "The nodes can still have changed since the preview. The apply only starts with the same inputs that were previewed."}
	tApplyLineScope = T{
		KO: "적용은 L2 배치까지입니다. 서비스 활성화와 되돌리기는 하지 않습니다.",
		EN: "The apply goes as far as the L2 placement. It does not activate services or roll back."}
	tApplyRefusedHead  = T{KO: "적용을 시작하지 않았습니다", EN: "The apply was not started"}
	tApplyRefusedGrant = T{
		KO: "유효한 미리 보기가 없습니다. 미리 보기가 없거나, 끝난 지 10분이 지났거나, 미리 본 뒤 입력이 바뀌었거나, 이미 한 번 쓰였습니다. 다시 미리 보십시오.",
		EN: "There is no valid preview: there is none, it ended more than 10 minutes ago, an input changed after it, or it was already used once. Preview again."}
	tApplyNotBegunConsumed = T{
		KO: "이 실행은 미리 보기를 썼지만 원격 작업을 시작하지 않았습니다. 노드는 바뀌지 않았습니다. 다시 적용하려면 새로 미리 보십시오.",
		EN: "This run used up the preview but started no remote work. The nodes were not changed. To apply again, preview again."}
	tApplyNotBegunUnused = T{
		KO: "이 실행은 원격 작업을 시작하지 않았고 미리 보기를 쓰지 않았습니다. 노드는 바뀌지 않았습니다. 미리 보기가 아직 유효하면 다시 적용할 수 있습니다.",
		EN: "This run started no remote work and did not use up the preview. The nodes were not changed. If the preview is still valid, you can apply again."}
	tProvBlockedHead = T{KO: "화면에서 실행하지 않는 까닭", EN: "Why the screen does not run this"}
	tProvBlockedFoot = T{
		KO: "생성된 플레이북 파일은 그대로 있습니다. 운영자가 직접 실행할 수 있습니다.",
		EN: "The generated playbook file is untouched. The operator can run it directly."}
	tProvPreview     = T{KO: "%d개 노드 미리 보기", EN: "Preview on %d nodes"}
	tProvRunningText = T{KO: "미리 보기를 실행하고 있습니다.", EN: "The preview is running."}
	tProvStop        = T{KO: "중지", EN: "Stop"}
	tProvLastPreview = T{KO: "마지막 미리 보기: %s", EN: "Last preview: %s"}
	tProvLastApply   = T{KO: "마지막 적용 실행: %s", EN: "Last apply run: %s"}
	tProvColNode     = T{KO: "노드", EN: "Node"}
	tProvColState    = T{KO: "실행 기록의 상태", EN: "State in the run record"}
	tProvColTasks    = T{KO: "작업", EN: "Tasks"}
	tProvNotVerified = T{
		KO: "서비스 활성화와 운영 상태: 확인하지 않음",
		EN: "Service activation and the running state: not checked"}
	tProvGrantValid = T{
		KO: "미리 본 내용은 %s까지, 한 번의 적용에만 유효합니다. 그 사이에도 노드의 상태는 바뀔 수 있습니다.",
		EN: "What was previewed is good until %s, for one apply. The nodes can still change in the meantime."}
	tProvGrantExpired = T{
		KO: "미리 보기가 만료됐습니다(끝난 뒤 10분). 다시 미리 보십시오.",
		EN: "The preview has expired (10 minutes after it ended). Preview again."}
	tProvGrantChanged = T{
		KO: "미리 본 뒤 입력이 바뀌었습니다(계획 · 플레이북 · 접근 정보 · 실행 설정 가운데 하나). 다시 미리 보십시오.",
		EN: "An input changed after the preview (the plan, the playbook, the access details or the run settings). Preview again."}
	tProvGrantUsed = T{KO: "이 미리 보기는 이미 한 번 쓰였습니다. 다시 미리 보십시오.", EN: "This preview was already used once. Preview again."}

	tProvColRecheck  = T{KO: "재확인", EN: "Recheck"}
	tProvRecheck     = T{KO: "이 실행 재확인", EN: "Recheck this run"}
	tProvRecheckHint = T{
		KO: "재확인은 그 시각에 대상 파일이 기대 상태와 같은지를 봅니다. 적용 실행이 어디까지 실행했는지를 알려 주지는 않습니다. 실행 기록의 상태는 재확인으로 바뀌지 않습니다.",
		EN: "A recheck tells whether the target files match the expected state at that moment. It does not tell how far the apply run got. The state in the run record is not changed by a recheck."}
	tProvRecheckChanged = T{
		KO: "이 실행 뒤에 입력이 바뀌어 재확인할 수 없습니다. 재확인은 적용한 것과 같은 계획 · 플레이북 · 접근 정보 · 실행 설정으로만 합니다.",
		EN: "An input changed after this run, so it cannot be rechecked. A recheck only runs with the same plan, playbook, access details and run settings as the apply run."}
	tProvRecheckMatch     = T{KO: "재확인 %s · 파일 일치", EN: "Rechecked %s · files match"}
	tProvRecheckDiffers   = T{KO: "재확인 %s · 다름", EN: "Rechecked %s · differs"}
	tProvRecheckUnreached = T{KO: "재확인하지 못함", EN: "Not rechecked"}
	tProvRunID            = T{KO: "실행 ID %s", EN: "Run ID %s"}

	tProvVersionBlock = T{
		KO: "감지한 ansible-core 버전: %s. 04의 미리 보기 · 적용 · 재확인은 ansible-core %s에서만 실행합니다. 화면을 띄운 환경의 PATH에서 먼저 찾아지는 ansible-playbook이 %s이 되게 한 뒤 이 화면을 다시 여십시오. 승인 서명, 플레이북 생성, 지난 실행 기록 조회는 그대로 할 수 있습니다.",
		EN: "Detected ansible-core version: %s. The 04 preview, apply and recheck only run with ansible-core %s. Make the ansible-playbook found first on the PATH of the environment that started the screen be %s, then open this screen again. Signing the approval, generating the playbooks and reading earlier run records still work."}
	tProvVersionUnread = T{KO: "판독할 수 없음", EN: "could not be read"}

	tProvRefusedHead     = T{KO: "04의 원격 실행을 시작하지 않았습니다", EN: "The 04 remote run was not started"}
	tProvRefusedNotReady = T{
		KO: "화면에서 실행할 조건이 맞지 않습니다. 04 화면의 「화면에서 실행하지 않는 까닭」을 확인하십시오.",
		EN: "The conditions for running from the screen are not met. See \"Why the screen does not run this\" on screen 04."}
	tProvRefusedBusy = T{KO: "이미 실행이 돌고 있습니다. 끝난 뒤 다시 하십시오.", EN: "A run is already going. Try again when it ends."}
)

var provBlockText = map[string]T{
	ProvViewOnly:        {KO: "보기 전용으로 열려 있어 실행하지 않습니다.", EN: "The screen is open view-only, so nothing is run."},
	ProvNotApproved:     {KO: "지금 계획에 대한, 이 키로 확인되는 승인이 있어야 합니다.", EN: "The current plan needs an approval this key can check."},
	ProvNoPlaybook:      {KO: "플레이북을 먼저 생성하십시오.", EN: "Generate the playbooks first."},
	ProvLevel:           {KO: "화면은 L2로 생성한 플레이북만 실행합니다. L2로 다시 생성하십시오.", EN: "The screen only runs a playbook generated at L2. Generate it again at L2."},
	ProvNotGenerated:    {KO: "플레이북 파일이 승인된 계획에서 생성한 것과 다릅니다. 다시 생성하십시오.", EN: "The playbook file differs from what the approved plan generates. Generate it again."},
	ProvUnknownForm:     {KO: "플레이북에 이 판이 알지 못하는 형식의 줄이 있습니다.", EN: "The playbook has a line in a form this version does not know."},
	ProvNoHosts:         {KO: "접근 정보(hosts.csv)에서 이 노드를 읽지 못했습니다.", EN: "This node could not be read from the access details (hosts.csv)."},
	ProvHostsUndeclared: {KO: "접근 정보(hosts.csv)에 선언에 없는 노드 ID가 있습니다. 01 관측에서 그 행을 지우거나 그 노드를 선언에 등록하십시오.", EN: "The access details (hosts.csv) hold a node ID that is not in the declaration. On 01 Observation, remove that row or register the node in the declaration."},
	ProvWindows:         {KO: "Windows 노드는 화면에서 실행하지 않습니다.", EN: "Windows nodes are not run from the screen."},
	ProvNoAnsible:       {KO: "ansible-playbook을 찾지 못했습니다.", EN: "ansible-playbook was not found."},
	ProvUnsupported + "-provider": {
		KO: "provider 모듈을 놓거나 확인하는 작업입니다. 미리 보기로 확인할 수 없어 화면에서 실행하지 않습니다.",
		EN: "This task stages or verifies a provider module. A preview cannot check it, so the screen does not run it."},
	ProvUnsupported + "-l3":      {KO: "활성화 · 재시작(L3) 조치입니다. 화면에서 실행하지 않습니다.", EN: "An activation or restart (L3) action. The screen does not run it."},
	ProvUnsupported + "-manual":  {KO: "설정으로 전달할 수 없는 수동 단계가 있습니다.", EN: "There is a manual step that configuration cannot deliver."},
	ProvUnsupported + "-unknown": {KO: "화면이 실행하지 않는 종류의 작업이거나, 그대로 쓸 수 없는 노드 이름입니다.", EN: "A kind of task the screen does not run, or a node name it cannot use as is."},
}

var provOutcomeText = map[string]T{
	"ready":       {KO: "미리 보기를 마쳤습니다. 기대한 작업을 모두 확인했고 바뀔 것이 있습니다.", EN: "The preview is complete. Every expected task was checked, and there are changes to make."},
	"no-change":   {KO: "적용할 변경이 없습니다. 기대한 작업이 빠짐없이 「바뀔 것 없음」으로 확인됐습니다.", EN: "There is nothing to apply. Every expected task was checked and none would change anything."},
	"incomplete":  {KO: "미리 보기 미완료. 확인하지 못한 작업이 있어 적용 권한을 만들지 않았습니다.", EN: "The preview is incomplete. Some tasks were not checked, so nothing may be applied from it."},
	"applied":     {KO: "모든 노드에서 L2 배치의 성공이 기록됐습니다.", EN: "The run record shows the L2 placement succeeded on every node."},
	"not-applied": {KO: "L2 배치가 끝까지 기록되지 않은 노드가 있습니다.", EN: "On some nodes the L2 placement is not recorded to the end."},
}

var provEndText = map[string]T{
	"stall": {
		KO: "%d초 동안 결과를 받지 못해 컨트롤러에서 실행을 중단했습니다. 이미 시작한 원격 작업은 뒤늦게 끝날 수 있습니다.",
		EN: "No result came for %d seconds, so the run was stopped on the controller. A remote task that had already started may still finish later."},
	"stopped": {
		KO: "컨트롤러에서 실행을 중단했습니다. 이미 시작한 원격 작업은 뒤늦게 끝날 수 있습니다.",
		EN: "The run was stopped on the controller. A remote task that had already started may still finish later."},
	"not-ready": {
		KO: "실행 기록을 준비하지 못해 원격 작업을 시작하지 않았습니다.",
		EN: "The run record could not be made ready, so no remote task was started."},
	"lost": {
		KO: "실행 기록 중단. 감독이 결과를 남기지 못하고 끝났습니다. 이미 시작한 원격 작업은 뒤늦게 끝날 수 있습니다.",
		EN: "The run record was cut short: the supervisor ended without leaving a result. A remote task that had already started may still finish later."},
	"record": {
		KO: "실행 기록 중단. 기록을 더 쓰지 못해 실행을 끝냈습니다. 결과가 기록되지 않은 작업은 노드에서 끝났을 수 있습니다.",
		EN: "The run record was cut short: it could not be written any more, so the run was ended. A task whose result is not recorded may have finished on the node."},
	"no-grant": {
		KO: "유효한 미리 보기를 쓸 수 없어 원격 작업을 시작하지 않았습니다(없음 · 만료 · 입력이 바뀜 · 이미 쓰임).",
		EN: "No valid preview could be used, so no remote work was started (none, expired, inputs changed, or already used)."},
	"not-allowed": {
		KO: "원격 작업을 허용하지 못해 시작하지 않았습니다.",
		EN: "Remote work could not be allowed, so none was started."},
	"lost-idle": {
		KO: "감독이 결과를 남기지 못하고 끝났습니다. 기록에 실행의 시작이 없습니다.",
		EN: "The supervisor ended without leaving a result. The record has no start of the run."},
	"ansible-version": {
		KO: "실행하려던 ansible-playbook이 지원하는 버전이 아니거나 버전을 읽을 수 없어 시작하지 않았습니다. 원격 작업은 없었습니다.",
		EN: "The ansible-playbook about to be run was not a supported version, or its version could not be read, so nothing was started. There was no remote work."},
	"cut": {
		KO: "실행 기록 중단. 기록이 끝까지 이어지지 않습니다.",
		EN: "The run record was cut short: it does not run to the end."},
}

var provStateText = map[string]T{
	"not-run": {KO: "실행하지 않음", EN: "Not run"}, "partial": {KO: "일부 작업만 끝남", EN: "Only some tasks finished"},
	"failed": {KO: "작업 실패", EN: "Task failed"}, "unreachable": {KO: "접속하지 못함", EN: "Could not connect"},
	"unknown": {KO: "결과를 알 수 없음", EN: "Result unknown"}, "skipped": {KO: "건너뛴 작업 있음", EN: "Some tasks were skipped"},
	"done-check": {KO: "확인함", EN: "Checked"}, "done-apply": {KO: "L2 배치 완료", EN: "L2 placement done"},
}

var provWhyText = map[string]T{
	"unreachable": {KO: "접속하지 못함", EN: "could not connect"}, "task-failed": {KO: "작업 실패", EN: "task failed"},
	"stalled": {KO: "%d초 동안 결과를 받지 못함", EN: "no result for %d seconds"}, "stopped": {KO: "컨트롤러에서 중단", EN: "stopped on the controller"},
	"record-cut": {KO: "실행 기록 중단", EN: "run record cut short"}, "skipped": {KO: "건너뜀", EN: "skipped"},
}

var provTaskText = map[string]T{
	"ok": {KO: "바뀔 것 없음", EN: "no change"}, "changed-check": {KO: "바뀜 예정", EN: "would change"},
	"changed-apply": {KO: "바꿈", EN: "changed"}, "failed": {KO: "실패", EN: "failed"}, "skipped": {KO: "건너뜀", EN: "skipped"},
	"unreachable": {KO: "접속 실패", EN: "could not connect"}, "no-result": {KO: "결과 없음", EN: "no result"},
	"not-started": {KO: "시작 안 함", EN: "not started"},
}

// 01 관측: 이력에 적재(ingest.go · ingest.templ).
var (
	tIngestHead = T{KO: "이력에 적재", EN: "Ingest into the history"}
	tIngestLede = T{
		KO: "지금 결과 폴더의 서명된 결과를 활성 정책으로 pqcota 이력에 적재합니다. 관측이 끝나도 스스로 적재하지 않습니다.",
		EN: "Ingests the signed results in the results folder into the pqcota history under the active policy. Nothing is ingested on its own when an observation ends."}
	tIngestNotConfigured = T{
		KO: "화면에서 적재하도록 설정되지 않았습니다. 화면은 조회만 합니다.",
		EN: "The screen is not set up to ingest. It only reads."}
	tIngestTerminal = T{
		KO: "<b>화면에서 적재하려면</b> 조회용 연결 정보를 <code>PQCOTA_READONLY_DSN</code>에, 같은 데이터베이스의 적재 계정 연결 정보를 <code>PQCOTA_DSN</code>에 두고 화면을 다시 띄웁니다.<br>" +
			"<b>터미널에서 적재하려면</b> 운영자 안내서 6절대로 <code>pqcota-ingest -verify-keys</code>로 확인한 뒤 <code>pqcota-ingest -scope-assets</code>로 적재합니다.",
		EN: "<b>To ingest from the screen</b>, put the read connection in <code>PQCOTA_READONLY_DSN</code> and the ingest account's connection to the same database in <code>PQCOTA_DSN</code>, then restart the screen.<br>" +
			"<b>To ingest from a terminal</b>, follow section 6 of the operator guide: check with <code>pqcota-ingest -verify-keys</code>, then ingest with <code>pqcota-ingest -scope-assets</code>."}
	tIngestProblemsHead = T{KO: "적재하지 않는 까닭 %d건", EN: "Reasons nothing is ingested: %d"}
	tIngestProblemsLede = T{
		KO: "결과 하나라도 서명이 없거나 확인되지 않으면 묶음 전체를 적재하지 않습니다.",
		EN: "When even one result is unsigned or cannot be verified, the whole set is not ingested."}
	tIngestSummary = T{
		KO: "결과 파일 %d개 · 활성 정책 %s · 정책으로 제외되는 자산 %d개(관측 %d개 가운데)",
		EN: "%d result files · active policy %s · %d assets excluded by the policy (of %d observed)"}
	tIngestSummaryNoPolicy = T{
		KO: "결과 파일 %d개 · 활성 정책 없음(관측한 자산 %d개가 모두 관리 대상)",
		EN: "%d result files · no active policy (all %d observed assets are managed)"}
	tIngestStateUnknown = T{
		KO: "이력의 지금 상태를 읽지 못했습니다. 확인할 수 없어 적재를 시작하지 않습니다.",
		EN: "The current state of the history could not be read. It cannot be checked, so no ingest is started."}
	tIngestAlready = T{
		KO: "이미 적재됨. 모든 노드의 최신 참조가 이 결과와 정책의 것입니다.",
		EN: "Already ingested. Every node's latest reference is the one for these results and this policy."}
	tIngestGo     = T{KO: "이력에 적재: 확인으로", EN: "Ingest into the history: go to the confirmation"}
	tIngestGoHint = T{
		KO: "확인 화면에서 조직 · 활성 정책 · 결과 파일 수 · 노드별 참조를 본 뒤 적재합니다. 이력은 덧붙이기만 됩니다.",
		EN: "The confirmation shows the organization, the active policy, the number of result files and the reference per node before anything is ingested. The history is append-only."}
	tIngestRunning     = T{KO: "이력에 적재하고 있습니다.", EN: "Ingesting into the history."}
	tIngestLastHead    = T{KO: "마지막 적재: %s", EN: "Last ingest: %s"}
	tIngestColBefore   = T{KO: "적재 전 최신 참조", EN: "Latest reference before"}
	tIngestColWant     = T{KO: "기대하는 참조", EN: "Expected reference"}
	tIngestColLatest   = T{KO: "지금 최신 참조", EN: "Latest reference now"}
	tIngestColState    = T{KO: "이력의 상태", EN: "State in the history"}
	tIngestUnknownCell = T{KO: "확인할 수 없음", EN: "cannot be checked"}
	tIngestNone        = T{KO: "없음", EN: "none"}
	tIngestNoResult    = T{
		KO: "적재 프로세스가 결과를 남기지 못하고 끝났습니다.",
		EN: "The ingest process ended without leaving a result."}
	tIngestFailed   = T{KO: "적재에 실패했습니다.", EN: "The ingest failed."}
	tIngestReadBack = T{KO: "이력을 다시 읽은 결과: ", EN: "Read back from the history: "}
	tIngestShownRun = T{
		KO: "가장 최근 적재가 아닙니다. 요청한 실행의 기록을 보이고 있습니다.",
		EN: "This is not the most recent ingest. The record of the requested run is shown."}
	tIngestCounts = T{
		KO: "받은 결과 %d건 · 내용이 바뀐 노드 %d개 · 정책으로 제외한 자산 %d건",
		EN: "%d results accepted · %d nodes with changed content · %d assets excluded by the policy"}
	tIngestConfirmTitle = T{KO: "이력 적재 확인", EN: "Confirm the ingest"}
	tIngestConfirmBody  = T{
		KO: "아래 묶음을 pqcota 이력에 적재합니다. 이 화면을 여는 것만으로는 아무것도 적재되지 않습니다.",
		EN: "The set below will be ingested into the pqcota history. Opening this page ingests nothing."}
	tIngestDo              = T{KO: "이력에 적재", EN: "Ingest into the history"}
	tIngestLineOrg         = T{KO: "조직: %s", EN: "Organization: %s"}
	tIngestLinePolicy      = T{KO: "활성 정책의 지문: %s", EN: "Fingerprint of the active policy: %s"}
	tIngestLineNoPolicy    = T{KO: "활성 정책 없음: 관측한 자산이 모두 관리 대상으로 적재됩니다", EN: "No active policy: every observed asset is ingested as managed"}
	tIngestLineFiles       = T{KO: "서명을 확인한 결과 파일: %d개", EN: "Result files with a verified signature: %d"}
	tIngestLineExcluded    = T{KO: "정책으로 제외되는 자산: %d개(관측 %d개 가운데)", EN: "Assets excluded by the policy: %d (of %d observed)"}
	tIngestLineNode        = T{KO: "%s: 기대하는 참조 %s · 지금 최신 참조 %s(%s)", EN: "%s: expected reference %s · latest reference now %s (%s)"}
	tIngestLineCandidate   = T{KO: "적재에 쓸 정책: 활성화할 확정 정책 %s", EN: "Policy for this ingest: the finalized policy to activate, %s"}
	tIngestLineActiveStays = T{
		KO: "현재 활성 정책: %s. 적재가 끝나도 활성 정책은 바뀌지 않습니다. 활성화는 따로 확인합니다.",
		EN: "Active policy now: %s. The active policy does not change when the ingest ends. Activation is confirmed separately."}
	tIngestLastActive    = T{KO: "적재에 쓴 정책: 적재할 때의 활성 정책 %s", EN: "Policy used: the active policy at the time, %s"}
	tIngestLastCandidate = T{KO: "적재에 쓴 정책: 자산 스코프 화면에서 확정한 정책 %s. 이 실행은 이력에만 적재했습니다.", EN: "Policy used: the policy finalized on the asset scope screen, %s. This run only ingested into the history."}
	tPolicyHead          = T{KO: "활성화할 확정 정책: 재적재와 활성화", EN: "Finalized policy to activate: re-ingest and activate"}
	tPolicyLede          = T{
		KO: "자산 스코프 화면에서 확정한 정책이 현재 활성 정책과 다릅니다. 활성화할 확정 정책으로 이력에 다시 적재한 뒤 활성화합니다. 재적재가 끝나도 활성 정책은 바뀌지 않습니다. 활성화는 따로 확인합니다.",
		EN: "The policy finalized on the asset scope screen differs from the active policy. Re-ingest into the history under the finalized policy to activate, then activate it. The active policy does not change when the re-ingest ends. Activation is confirmed separately."}
	tPolicyColPolicy = T{KO: "정책", EN: "Policy"}
	tPolicyColFile   = T{KO: "파일", EN: "File"}
	tPolicyColDigest = T{KO: "지문", EN: "Fingerprint"}
	tPolicyActive    = T{KO: "현재 활성 정책", EN: "Active policy now"}
	tPolicyCandidate = T{KO: "활성화할 확정 정책", EN: "Finalized policy to activate"}
	tPolicyStep1     = T{KO: "활성화할 확정 정책으로 재적재", EN: "Re-ingest under the finalized policy to activate"}
	tPolicyStep1Body = T{
		KO: "지금 결과 폴더의 서명된 결과를 활성화할 확정 정책으로 이력에 적재합니다. 활성 정책 파일은 건드리지 않습니다.",
		EN: "Ingests the signed results in the results folder into the history under the finalized policy to activate. The active policy file is not touched."}
	tPolicySummary = T{
		KO: "결과 파일 %d개 · 활성화할 확정 정책으로 제외되는 자산 %d개(관측 %d개 가운데)",
		EN: "%d result files · %d assets excluded by the finalized policy to activate (of %d observed)"}
	tPolicyReingested = T{
		KO: "모든 노드의 최신 참조가 활성화할 확정 정책의 것입니다. 다시 적재할 것이 없습니다.",
		EN: "Every node's latest reference is the one for the finalized policy to activate. There is nothing to re-ingest."}
	tPolicySeeBelow = T{
		KO: "적재하지 못하는 결과가 있습니다. 아래 「이력에 적재」에 파일마다 까닭이 적혀 있습니다.",
		EN: "Some results cannot be ingested. The reason per file is under \"Ingest into the history\" below."}
	tPolicyGo       = T{KO: "이 정책으로 재적재", EN: "Re-ingest under this policy"}
	tPolicyTerminal = T{
		KO: "화면에서 적재하도록 설정되지 않았습니다. 작업 폴더의 터미널에서 다음 두 명령으로 다시 적재합니다.",
		EN: "The screen is not set up to ingest. Re-ingest with the two commands below, in a terminal in the workspace folder."}
	tPolicyActiveNow = T{KO: "현재 활성 정책: ", EN: "Active policy now: "}
	tObsScopeHead    = T{KO: "자산 스코프", EN: "Asset scope"}
	tObsScopeLede    = T{
		KO: "관측한 자산 가운데 무엇을 관리 대상에서 뺄지 정하는 규칙입니다. 대조 · 판정 · 이력 적재는 현재 활성 정책으로 합니다. 규칙을 고치고 서명해 확정하는 일은 자산 스코프 화면에서 합니다.",
		EN: "The rules that decide which observed assets are left out of management. Reconciliation, judgments and the ingest into the history use the active policy. Editing, signing and finalizing the rules are done on the asset scope screen."}
	tObsScopeUnknown   = T{KO: "현재 활성 정책 파일을 읽을 수 없습니다.", EN: "The active policy file cannot be read."}
	tObsScopeFile      = T{KO: "파일 %s · 규칙 %d개", EN: "file %s · %d rules"}
	tObsScopeState     = T{KO: "규칙 편집: %s · 확정 상태: %s", EN: "Rule editing: %s · finalization: %s"}
	tObsScopeEdit      = T{KO: "규칙 고치기", EN: "Edit the rules"}
	tScopeBackObserve  = T{KO: "01 관측으로 돌아가기", EN: "Back to 01 Observation"}
	tIngestFoldSummary = T{KO: "현재 활성 정책으로 다시 적재", EN: "Ingest again under the active policy"}
	tIngestFoldWarn    = T{
		KO: "여기서 적재하면 이력의 최신 참조가 현재 활성 정책의 것으로 돌아갑니다. 그 뒤에 활성화할 확정 정책을 활성화하려면 그 정책으로 다시 적재해야 합니다.",
		EN: "Ingesting here turns the latest references in the history back to the ones for the active policy. To activate the finalized policy after that, you must re-ingest under that policy again."}
	tIngestFoldGo           = T{KO: "현재 활성 정책으로 다시 적재: 확인으로", EN: "Ingest again under the active policy: go to the confirmation"}
	tActivateRefusedHead    = T{KO: "정책을 활성화하지 않았습니다", EN: "The policy was not activated"}
	tActivateHistoryChanged = T{
		KO: "확인 후 이력의 최신 참조가 달라졌습니다. 현재 활성 정책은 그대로입니다. 아래 링크로 돌아가 01의 「활성화할 확정 정책: 재적재와 활성화」에서 노드별 대조 결과를 확인하십시오.",
		EN: "The latest references in the history changed after the confirmation. The active policy is unchanged. Follow the link below and check the per-node comparison under \"Finalized policy to activate: re-ingest and activate\" on screen 01."}
	tPolicyStep2 = T{KO: "정책 활성화", EN: "Activate the policy"}
	tPolicyReady = T{
		KO: "재적재가 확인됐습니다. 현재 활성 정책은 아직 그대로입니다. 확인 페이지에서 바뀌는 파일과 되돌리기 제한을 본 뒤 활성화합니다. 활성화 직전에 이력을 처음부터 다시 대조하고, 최신 참조가 달라졌으면 활성화하지 않습니다.",
		EN: "The re-ingest is confirmed. The active policy is still the same. Review the files that change and the rollback limit on the confirmation page, then activate. Right before activating, the history is reconciled again from the start; if a latest reference has changed, nothing is activated."}
	tPolicyNotYet = T{
		KO: "모든 노드의 최신 참조가 활성화할 확정 정책의 것이 되면 활성화할 수 있습니다. 그 전에는 현재 활성 정책이 유지됩니다.",
		EN: "Activation becomes possible once every node's latest reference is the one for the finalized policy to activate. Until then the active policy stays as it is."}
	tIngestLineSameDB = T{
		KO: "화면이 조회하는 것과 같은 데이터베이스에만 적재합니다. 다르면 쓰기 전에 멈춥니다.",
		EN: "The ingest only goes to the same database the screen reads. If they differ it stops before writing."}
	tIngestLineAppendOnly = T{
		KO: "이력은 덧붙이기만 됩니다. 적재한 것은 되돌릴 수 없습니다.",
		EN: "The history is append-only. What is ingested cannot be undone."}
	tIngestRefusedHead    = T{KO: "적재를 시작하지 않았습니다", EN: "No ingest was started"}
	tIngestChangedResults = T{
		KO: "확인한 뒤 결과 파일이 바뀌었습니다. 다시 점검하고 적재할 내용을 확인하십시오.",
		EN: "The result files changed after the confirmation. Check again and confirm what will be ingested."}
	tIngestChangedPolicy = T{
		KO: "확인한 뒤 활성 정책이 바뀌었습니다. 다시 점검하고 적재할 내용을 확인하십시오.",
		EN: "The active policy changed after the confirmation. Check again and confirm what will be ingested."}
	tIngestChangedKeys = T{
		KO: "확인한 뒤 검증 키(신뢰할 수집기 공개 키)가 바뀌었습니다. 다시 점검하고 적재할 내용을 확인하십시오.",
		EN: "The verification keys (trusted collector public keys) changed after the confirmation. Check again and confirm what will be ingested."}
	tIngestChangedInputs = T{
		KO: "확인한 뒤 적재할 입력이 둘 이상 바뀌었습니다(결과 파일 · 활성 정책 · 검증 키 · 조직 · 규칙 판 가운데). 다시 점검하고 적재할 내용을 확인하십시오.",
		EN: "More than one input to the ingest changed after the confirmation (among the result files, the active policy, the verification keys, the organization and the ruleset). Check again and confirm what will be ingested."}
	tIngestRefusedNotReady = T{
		KO: "적재할 조건이 맞지 않습니다. 01 화면의 「이력에 적재」에서 까닭을 확인하십시오(설정 · 서명 · 이미 적재됨 · 이력을 읽지 못함 · 조회와 적재의 데이터베이스가 다름).",
		EN: "The conditions for ingesting are not met. See \"Ingest into the history\" on screen 01 for the reason (setup, signatures, already ingested, the history could not be read, or the read and ingest databases differ)."}
	tIngestRefusedBusy   = T{KO: "이미 적재가 돌고 있습니다. 끝난 뒤 다시 하십시오.", EN: "An ingest is already running. Try again when it ends."}
	tIngestRefusedReplay = T{
		KO: "이미 사용한 적재 확인입니다. 새 적재를 시작하지 않았습니다. 아래 링크로 돌아가면 이 확인으로 시작한 적재의 기록이 보입니다.",
		EN: "This ingest confirmation was already used. No new ingest was started. The link below goes to the record of the ingest this confirmation started."}
)

var ingestBlockText = map[string]T{
	IngestViewOnly:    {KO: "보기 전용으로 열려 있어 적재하지 않습니다.", EN: "The screen is open view-only, so nothing is ingested."},
	IngestNoResults:   {KO: "결과 폴더에 적재할 결과가 없습니다.", EN: "The results folder has nothing to ingest."},
	IngestNoVerifyKey: {KO: "신뢰할 수집기 공개 키(verify.pub)를 쓸 수 없어 서명을 확인할 수 없습니다.", EN: "The trusted collector public keys (verify.pub) cannot be used, so signatures cannot be checked."},
	IngestPolicy:      {KO: "활성 정책 파일을 읽을 수 없습니다.", EN: "The active policy file cannot be read."},
	IngestInputs:      {KO: "선언이나 결과를 읽을 수 없어 기대하는 참조를 계산하지 못했습니다.", EN: "The declaration or the results could not be read, so the expected references could not be worked out."},
	IngestObserving:   {KO: "관측이 돌고 있습니다. 끝난 뒤에 적재합니다.", EN: "An observation is running. Ingest after it ends."},
	IngestNoCandidate: {KO: "활성화할 확정 정책이 없습니다. 자산 스코프 화면에서 확정한 정책이 없거나, 확정한 뒤 바뀌었거나, 이미 활성 정책과 같습니다.", EN: "There is no finalized policy to activate. No policy was finalized on the asset scope screen, it changed after it was finalized, or it is already the active policy."},
}

var ingestWhyText = map[string]T{
	IngestWhyNotFile:    {KO: "일반 파일이 아닙니다.", EN: "is not a plain file."},
	IngestWhyUnreadable: {KO: "읽을 수 없습니다.", EN: "cannot be read."},
	IngestWhyNotResult:  {KO: "관측 결과로 해독되지 않습니다.", EN: "does not decode as a collection result."},
	IngestWhyUnsigned:   {KO: "서명이 없습니다.", EN: "is not signed."},
	IngestWhyNoKey:      {KO: "이 수집기 ID에 등록된 신뢰할 공개 키가 없습니다.", EN: "has no trusted public key listed for this collector ID."},
	IngestWhyOtherKey:   {KO: "다른 수집기 ID에 등록된 키로 서명됐습니다.", EN: "is signed with a key listed for another collector ID."},
	IngestWhyMismatch:   {KO: "서명이 등록된 키와 맞지 않습니다(서명 뒤에 바뀌었거나 등록되지 않은 키).", EN: "has a signature that does not match the listed keys (changed after signing, or an unlisted key)."},
}

var ingestStateText = map[string]T{
	"current":   {KO: "최신 참조가 기대와 같음", EN: "latest reference is the expected one"},
	"body-only": {KO: "본체는 있으나 최신 참조가 아님", EN: "the body exists but is not the latest reference"},
	"absent":    {KO: "기록되지 않음", EN: "not recorded"},
	"unknown":   {KO: "확인할 수 없음", EN: "cannot be checked"},
}

var ingestOutcomeText = map[string]T{
	"running":  {KO: "적재가 돌고 있습니다.", EN: "The ingest is running."},
	"recorded": {KO: "모든 노드가 기록됐습니다. 최신 참조가 기대하는 것과 같습니다.", EN: "Every node is recorded: its latest reference is the expected one."},
	"partial":  {KO: "일부 노드만 기록됐습니다. 아래 표가 노드마다 이력의 상태를 보입니다.", EN: "Only some nodes are recorded. The table below shows the state in the history per node."},
	"nothing":  {KO: "기록된 노드가 없습니다. 어느 노드의 최신 참조도 기대하는 것이 아닙니다.", EN: "No node is recorded: no node's latest reference is the expected one."},
	"unknown":  {KO: "기록 여부를 확인하지 못했습니다. 이력을 읽지 못했습니다.", EN: "Whether anything was recorded could not be checked: the history could not be read."},
	// unchanged — 노드마다 본체 · 최신 참조 · 관측 기록 수가 적재 전과 같다고 확인했을 때만이다.
	"unchanged": {KO: "이력 변화가 없습니다. 노드마다 본체 · 최신 참조 · 관측 기록 수가 적재 전과 같습니다.", EN: "The history did not change: per node, the body, the latest reference and the number of observation records are what they were before."},
}

// 적재 오류의 갈래(코드)로 고르는 안내. 오류의 문장은 싣지 않는다.
const (
	IngestDetailPermission = "permission"
	IngestDetailSchema     = "schema"
	IngestDetailTimeout    = "timeout"
)

var ingestDetailText = map[string]T{
	IngestDetailPermission: {KO: "적재 계정에 권한이 없습니다. PQCOTA_DSN의 계정이 이력 표에 쓸 수 있는지 확인하십시오.", EN: "The ingest account lacks privileges. Check that the account in PQCOTA_DSN can write to the history tables."},
	IngestDetailSchema:     {KO: "이력 스키마가 없거나 완전하지 않습니다.", EN: "The history schema is missing or incomplete."},
	IngestDetailTimeout:    {KO: "시간 안에 응답을 받지 못했습니다.", EN: "No answer came in time."},
}

var ingestReasonText = map[string]T{
	"inputs-changed": {KO: "적재할 사본이 확인한 것과 달라 쓰기 전에 멈췄습니다.", EN: "The copies to ingest differed from what was confirmed, so it stopped before writing."},
	"decode":         {KO: "해독하지 못한 결과가 있어 쓰기 전에 멈췄습니다.", EN: "A result could not be decoded, so it stopped before writing."},
	"signature":      {KO: "서명이 없거나 맞지 않는 결과가 있어 쓰기 전에 멈췄습니다.", EN: "A result was unsigned or did not verify, so it stopped before writing."},
	"other-db":       {KO: "적재용 연결이 화면이 조회하는 데이터베이스가 아니어서 쓰기 전에 멈췄습니다.", EN: "The ingest connection is not the database the screen reads, so it stopped before writing."},
	"store":          {KO: "적재용으로 이력을 열지 못해 쓰기 전에 멈췄습니다.", EN: "The history could not be opened for ingesting, so it stopped before writing."},
	"ingest-failed":  {KO: "적재하는 도중에 오류가 났습니다.", EN: "An error came up while ingesting."},
}
