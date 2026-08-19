// 화면에 뜨는 모든 문구. **두 말이 한 줄 건너 나란히 있다.**
//
// 파일을 갈라 두면 한쪽만 고쳐지는 날이 오고, 그날 어느 쪽이 최신인지 아무도 모른다.
// 이름 있는 변수라 새 문구를 빠뜨리면 컴파일이 막는다.
//
// **문구 안에 태그가 있다.** 한국어와 영어는 어순이 달라서, 강조할 자리를 조각내 두면
// 한쪽 말이 반드시 어색해진다. 여기 있는 것은 전부 우리가 쓴 상수이지 밖에서 온 값이
// 아니므로, 화면이 `templ.Raw` 로 그대로 낸다.
package ui

// ── 이동 · 화면 이름 ────────────────────────────────────────────────────────
var (
	tNavDecl   = T{KO: "① 선언", EN: "① Declaration"}
	tNavScope  = T{KO: "② 자산 스코프", EN: "② Asset scope"}
	tNavSurvey = T{KO: "③ 대조", EN: "③ Reconciliation"}
	tNavReview = T{KO: "④ 리뷰 큐", EN: "④ Review queue"}

	tTitleDecl   = T{KO: "선언", EN: "Declaration"}
	tTitleScope  = T{KO: "자산 스코프", EN: "Asset scope"}
	tTitleSurvey = T{KO: "대조", EN: "Reconciliation"}
	tTitleReview = T{KO: "리뷰 큐", EN: "Review queue"}

	tSubOrg     = T{KO: "조직", EN: "org"}
	tSubSession = T{KO: "세션", EN: "session"}
	tSubResults = T{KO: "결과", EN: "results"}

	// 저장·확정 뒤에 뜨는 알림. **화면 글이라 말을 탄다.**
	tSavedNotFinal = T{
		KO: "세션 파일에 저장했습니다 — 아직 확정하지 않았습니다",
		EN: "Saved to the session file — not finalized yet"}
	tFinalizedPlan = T{
		KO: "확정했습니다 — 조치 %d건을 %s 에 썼습니다",
		EN: "Finalized — %d actions written to %s"}
	tFinalizedPolicy = T{
		KO: "확정했습니다 — 규칙 %d개를 %s 에 썼습니다. 이 파일이 `pqcota-ingest -scope-assets` 의 입력입니다",
		EN: "Finalized — %d rules written to %s. That file is the input to `pqcota-ingest -scope-assets`"}
	tJudgmentsSaved = T{
		KO: " · 판정 %d건을 %s 에 남겼습니다",
		EN: " · %d judgments appended to %s"}
	tRulesSaved = T{
		KO: "계층 %d개에 규칙 %d개를 썼습니다 — 판정할 변경 %d건, 그중 왜 뺐는지를 적어야 하는 것 %d건",
		EN: "Wrote %d rules across %d layers — %d changes to judge, %d of them needing a recorded reason"}
	tSignatureCleared = T{
		KO: " · 정책이 달라져 서명을 지웠습니다",
		EN: " · the policy changed, so the signature was cleared"}
	tDeclSaved = T{
		KO: "선언을 저장했습니다 — 노드 %d · 자산 %d · 엣지 %d",
		EN: "Declaration saved — %d nodes · %d assets · %d edges"}
	tDeclStillOff = T{
		KO: " · 맞지 않는 자리 %d곳이 남아 있습니다",
		EN: " · %d places still do not add up"}

	tRefused = T{KO: "하지 않았습니다 — 이유는 이렇습니다.",
		EN: "Not done — here is why."}
	tAddRow = T{KO: "행 추가", EN: "Add row"}
)

// ── 선언 ───────────────────────────────────────────────────────────────────
var (
	tDeclProblems = T{
		KO: "선언이 앞뒤가 맞지 않는 자리",
		EN: "places where the declaration contradicts itself"}
	tDeclProblemsHint = T{
		KO: "저장은 됩니다. 다만 그대로 두면 대조 결과가 <b>오류 없이 틀립니다.</b>",
		EN: "Saving still works. But left as is, reconciliation will be <b>wrong without erroring.</b>"}

	tDeclOrg     = T{KO: "조직", EN: "Organization"}
	tDeclOrgHint = T{
		KO: "대조와 판정이 이 조직에 묶입니다. 비우면 <code>local</code> 입니다.",
		EN: "Reconciliation and judgments are bound to this org. Empty means <code>local</code>."}

	tDeclScope     = T{KO: "관리 대상 노드", EN: "Nodes under management"}
	tDeclScopeNote = T{KO: "어느 노드를 볼 것인가", EN: "which nodes to look at"}
	tDeclScopeHint = T{
		KO: "한 줄에 하나. 여기 없는 노드와 통신한 것이 관측되면, 대조 화면에 " +
			"「등재 판정 요청」으로 올라옵니다. <b>노드 안에서 무엇을 볼지</b>는 " +
			"「자산 스코프」 탭에서 정합니다.",
		EN: "One per line. If traffic to a node that is not listed here is observed, it shows " +
			"up on the reconciliation screen as <b>needs an enrollment decision</b>. " +
			"<b>What to look at inside a node</b> is decided on the Asset scope tab."}

	tDeclNodes     = T{KO: "노드 주소", EN: "Node addresses"}
	tDeclNodesNote = T{
		KO: "관측에 찍힌 IP를 노드 이름과 잇는 근거",
		EN: "how observed IPs are tied back to node names"}
	tDeclNodesHint = T{
		KO: "한 노드가 망 둘에 걸치면 IP도 둘입니다. 쉼표나 공백으로 나눠 적으십시오. " +
			"이름을 비우면 그 줄은 지워집니다.",
		EN: "A node on two networks has two IPs. Separate them with commas or spaces. " +
			"Clearing the name deletes that row."}

	tDeclAssets     = T{KO: "자산", EN: "Assets"}
	tDeclAssetsNote = T{
		KO: "「이 노드에서 이것을 쓴다」", EN: "“this node uses this”"}
	tDeclEdges     = T{KO: "통신 엣지", EN: "Communication edges"}
	tDeclEdgesNote = T{
		KO: "「이 노드가 저 노드와 이렇게 통신한다」",
		EN: "“this node talks to that node like this”"}

	tDeclSave     = T{KO: "저장", EN: "Save"}
	tDeclSaveHint = T{
		KO: "저장하면 <code>pqcaton-report</code> 가 읽는 바로 그 파일에 씁니다 — " +
			"화면 위에 경로가 적혀 있습니다.",
		EN: "Saving writes to the very file <code>pqcaton-report</code> reads — " +
			"its path is shown at the top of this page."}

	tColName      = T{KO: "이름", EN: "Name"}
	tColIP        = T{KO: "IP", EN: "IP"}
	tColNode      = T{KO: "노드", EN: "Node"}
	tColRuntime   = T{KO: "런타임", EN: "Runtime"}
	tColComponent = T{KO: "컴포넌트", EN: "Component"}
	tColSrc       = T{KO: "보내는 쪽", EN: "From"}
	tColDst       = T{KO: "받는 쪽", EN: "To"}
	tColPort      = T{KO: "포트", EN: "Port"}
	tColProto     = T{KO: "프로토콜", EN: "Protocol"}
)

// ── 리뷰 큐 ────────────────────────────────────────────────────────────────
var (
	tReviewItems            = T{KO: "항목", EN: "items"}
	tReviewMandatory        = T{KO: "필수", EN: "mandatory"}
	tReviewPolicyConclusion = T{KO: "이 정책의 결론", EN: "Conclusion for this policy"}
	tReviewConclusionHint   = T{
		KO: "— 왜 이렇게 정했는지 적습니다. 적으면 아래 항목이 한 번에 판정됩니다",
		EN: "— write down why you decided this. One entry judges every item below at once"}
	tReviewPlaceholder = T{
		KO: "예: PQC 라이브러리로 교체한다", EN: "e.g. replace with a PQC library"}

	tColTarget     = T{KO: "대상", EN: "Target"}
	tColState      = T{KO: "상태", EN: "State"}
	tColConfidence = T{KO: "확신", EN: "Confidence"}
	tColInPlan     = T{KO: "계획에 넣기", EN: "In plan"}
	tColException  = T{KO: "개별 결론(예외)", EN: "Per-item conclusion (exception)"}
	tRescan        = T{KO: "재수집 후보", EN: "rescan candidate"}

	tReviewEmpty    = T{KO: "판정할 것이 없습니다.", EN: "Nothing to judge."}
	tReviewAutopass = T{
		KO: "자동통과 후보 %d개는 이 큐에 올리지 않았습니다 — 선언과 맞고 확신도 높은 " +
			"것들이라, 하나씩 볼 것이 아닙니다.",
		EN: "%d auto-pass candidates are not in this queue — they match the declaration " +
			"with high confidence, so they are not worth looking at one by one."}

	tApproval           = T{KO: "승인", EN: "Approval"}
	tReviewApprovalHint = T{
		KO: "정책마다 <b>판정</b>(결론)을 적고, 여기에 <b>승인</b>자와 서명을 채운 뒤, " +
			"<b>확정</b>하면 계획이 나갑니다. 셋이 다 있어야 통과합니다.",
		EN: "Write a <b>judgment</b> for each policy, fill in the <b>approver</b> and " +
			"signature here, then <b>finalize</b> to emit the plan. All three are required."}
	tReviewer  = T{KO: "승인자", EN: "Approver"}
	tSignature = T{KO: "서명", EN: "Signature"}

	tSaveOnly       = T{KO: "저장만", EN: "Save only"}
	tFinalizePlan   = T{KO: "확정하고 계획 내보내기", EN: "Finalize and emit the plan"}
	tReviewGateHint = T{
		KO: "확정은 <code>pqcaton-decide close</code> 와 같은 게이트를 탑니다 — " +
			"필수 항목의 결론과 서명이 모두 있어야 통과합니다.",
		EN: "Finalizing goes through the same gate as <code>pqcaton-decide close</code> — " +
			"every mandatory item needs a conclusion, and the signature must be present."}
)

// ── 자산 스코프 ────────────────────────────────────────────────────────────
var (
	tScopeIntro = T{
		KO: "<b>노드 안에서 무엇을 계속 볼 것인가</b>를 정하는 자리입니다. " +
			"인벤토리에서 뺀 자산은 나중에 「왜 이건 안 봤나」에 답해야 하므로, " +
			"<b>왜 뺐는지를 적어야 확정됩니다.</b>",
		EN: "This is where you decide <b>what to keep looking at inside a node</b>. " +
			"An asset you drop from the inventory is something you will have to account " +
			"for later, so <b>you must record why you dropped it before it can be finalized.</b>"}
	tScopeDelta = T{
		KO: "판정할 것으로는 <b>지금 쓰는 정책과 달라진 규칙만</b> 올라옵니다. " +
			"바뀌지 않은 규칙까지 매번 다시 승인하게 하면 아무도 들여다보지 않습니다.",
		EN: "Only <b>rules that differ from the policy in force</b> come up for judgment. " +
			"If unchanged rules had to be re-approved every time, nobody would read them."}

	tRuleLegend = T{KO: "규칙을 적는 법", EN: "How to write a rule"}
	tColMeaning = T{KO: "칸", EN: "Column"}
	tColMeans   = T{KO: "뜻", EN: "Meaning"}

	tRuleAction = T{
		KO: "<b>exclude</b> — 이 자산을 인벤토리에서 뺍니다. 나중에 「왜 이건 안 봤나」에 " +
			"답해야 하므로, 아래에서 <b>왜 뺐는지를 적어야 확정됩니다.</b> " +
			"<b>include</b> — 다시 넣습니다. 위 계층이 뺀 것을 되돌릴 때 씁니다.",
		EN: "<b>exclude</b> — drop this asset from the inventory. You will have to account " +
			"for it later, so <b>you must record below why you dropped it</b> before it can " +
			"be finalized. <b>include</b> — put it back, to undo an exclusion from a layer above."}
	tRuleRuntime = T{
		KO: "어느 런타임인가. <code>openssl</code> · <code>jca</code> 같은 것. 비우면 <b>전부</b>입니다",
		EN: "Which runtime — <code>openssl</code>, <code>jca</code> and the like. Empty means <b>all</b>"}
	tRuleLib = T{
		KO: "라이브러리 이름. <code>*</code> 를 쓸 수 있습니다 — <code>libcrypto.so.*</code>",
		EN: "Library name. <code>*</code> is allowed — <code>libcrypto.so.*</code>"}
	tRuleAppKey = T{
		KO: "그것을 쓰는 실행 파일. <code>/usr/bin/python*</code> · <code>/usr/sbin/sshd</code>",
		EN: "The executable using it. <code>/usr/bin/python*</code>, <code>/usr/sbin/sshd</code>"}
	tRuleNote = T{
		KO: "사람이 읽는 설명입니다. <b>판정의 근거가 아닙니다</b> — 근거는 아래 결론 칸에 적습니다",
		EN: "A note for people to read. <b>Not the reason for a judgment</b> — that goes in the conclusion field below"}

	tRuleEmptyWarn = T{
		KO: "<b>빈 칸은 「전부」입니다.</b> 그래서 <code>runtime</code>·<code>lib</code>·" +
			"<code>app_key</code> 가 모두 빈 줄은 규칙으로 만들지 않습니다 — 그대로 두면 " +
			"<code>exclude</code> 하나로 <b>인벤토리가 통째로 빕니다.</b> 「전부」를 뜻하려면 " +
			"<code>*</code> 를 적으십시오.",
		EN: "<b>An empty cell means “all”.</b> That is why a row with " +
			"<code>runtime</code>, <code>lib</code> and <code>app_key</code> all empty is not " +
			"turned into a rule — as written, a single <code>exclude</code> would " +
			"<b>empty the entire inventory.</b> Write <code>*</code> if you really mean all."}
	tRuleLayerOrder = T{
		KO: "<b>같은 자산에 규칙이 여럿 걸리면 아래 계층의 것이 적용됩니다.</b> " +
			"계층은 위에서 아래로 겹치기 때문입니다(조직 → 환경 → 노드군). 그래서 위 계층이 " +
			"뺀 자산을 아래 계층에서 <code>include</code> 로 되돌릴 수 있고, 그 되돌림도 승인을 거칩니다.",
		EN: "<b>When several rules match one asset, the lower layer wins.</b> Layers stack " +
			"top to bottom (org → environment → node group). So a lower layer can put back " +
			"an asset an upper layer excluded, and that reversal goes through approval too."}
	tScopeTableOrder = T{
		KO: "아래 표의 순서가 곧 계층 순서입니다 — <b>겹치면 아래쪽이 적용됩니다.</b>",
		EN: "The order of the tables below is the layer order — <b>on a clash, the lower one wins.</b>"}

	tLayer         = T{KO: "계층", EN: "Layer"}
	tLayerRules    = T{KO: "규칙", EN: "rules"}
	tSaveRules     = T{KO: "규칙 저장", EN: "Save rules"}
	tSaveRulesHint = T{
		KO: "저장하면 계층 파일에 그대로 쓰고, <b>그래서 무엇이 달라지는지를 아래에 다시 " +
			"보여 줍니다.</b> 적어 둔 판정은 그대로 남습니다 — 규칙 자체가 달라진 것만 " +
			"다시 판정하면 됩니다.",
		EN: "Saving writes straight to the layer files and then <b>shows below what that " +
			"changes.</b> Judgments you already wrote stay — only rules that actually " +
			"changed need judging again."}
	tScopeEditResult = T{KO: "고친 결과 — 승인할 변경", EN: "What your edits change — for approval"}
	tRemoveRuleHint  = T{
		KO: "줄을 지우려면 <code>runtime</code>·<code>lib</code>·<code>app_key</code> 를 모두 비우십시오.",
		EN: "To delete a row, clear <code>runtime</code>, <code>lib</code> and <code>app_key</code>."}

	tChanges          = T{KO: "변경", EN: "changes"}
	tReasonNeeded     = T{KO: "왜 뺐는지가 필요한 것", EN: "need a recorded reason"}
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
		KO: "바뀐 규칙이 없습니다 — 승인할 것이 없습니다.",
		EN: "No rules changed — nothing to approve."}

	tScopeApprovalHint = T{
		KO: "계층마다 <b>판정</b>(결론)을 적고, 여기에 <b>승인</b>자와 서명을 채운 뒤, " +
			"<b>확정</b>하면 정책이 나갑니다. 셋이 다 있어야 통과합니다.",
		EN: "Write a <b>judgment</b> for each layer, fill in the <b>approver</b> and " +
			"signature here, then <b>finalize</b> to emit the policy. All three are required."}
	tPolicyOnFinalize = T{KO: "확정될 정책", EN: "Policy to be finalized"}
	tPolicyRulesAll   = T{KO: "규칙 %d개 전부", EN: "all %d rules"}
	tPolicyWholeHint  = T{
		KO: "<b>판정은 달라진 규칙만 보지만, 확정하면 정책 전체가 나갑니다.</b> " +
			"pqcota의 집행기가 일부가 아니라 정책 전체를 받기 때문입니다.",
		EN: "<b>You judge only what changed, but finalizing emits the whole policy.</b> " +
			"pqcota's enforcer takes the entire policy, not a fragment."}
	tFinalizePolicy = T{KO: "확정하고 정책 내보내기", EN: "Finalize and emit the policy"}
	tScopeGateHint  = T{
		KO: "확정은 <code>pqcaton-scope close</code> 와 같은 게이트를 탑니다. 나온 CSV 가 " +
			"<code>pqcota-ingest -scope-assets</code> 의 입력입니다.",
		EN: "Finalizing goes through the same gate as <code>pqcaton-scope close</code>. " +
			"The CSV it emits is the input to <code>pqcota-ingest -scope-assets</code>."}
)

// ── 대조 ───────────────────────────────────────────────────────────────────
var (
	tSurveyObserved     = T{KO: "관측", EN: "Observation"}
	tSurveyObservedNote = T{KO: "pqcota가 무엇을 보았나", EN: "what pqcota saw"}
	tSurveyObservedHint = T{
		KO: "대상 노드에 collector를 반입·실행·회수했습니다. 노드에는 아무것도 남지 않습니다.",
		EN: "The collector was carried to the target node, run, and taken back. " +
			"Nothing is left behind on the node."}
	tColCollector  = T{KO: "그 노드에서 돈 collector", EN: "Collectors that ran on it"}
	tObservedCount = T{
		KO: "관측 자산 <b>%d</b> · 협상된 통신 <b>%d</b>건",
		EN: "<b>%d</b> assets observed · <b>%d</b> negotiated connections"}

	tNotSeen     = T{KO: "못 본 것", EN: "What was not seen"}
	tNotSeenNone = T{
		KO: "없습니다 — 이 범위에서는 관측이 완전합니다.",
		EN: "Nothing — observation is complete for this scope."}
	tNotSeenLayer = T{
		KO: "이 계층을 아예 보지 못했습니다 —", EN: "This layer was never observed —"}
	tNotSeenEdges = T{
		KO: "이 노드의 통신을 보지 못했습니다 —", EN: "This node's traffic was not observed —"}
	tNotSeenHint = T{
		KO: "<b>못 본 것과 없는 것은 다릅니다.</b> 아래 UNOBSERVED 가 「쓰지 않는 것」인지 " +
			"「보지 못한 것」인지는 위 목록이 가릅니다 — <b>위에 적힌 것이면 재수집이 먼저</b>이고, " +
			"아니면 사람이 판정할 차례입니다.",
		EN: "<b>Not seen is not the same as not there.</b> Whether an UNOBSERVED below means " +
			"“not used” or “not looked at” is settled by the list above — " +
			"<b>if it is listed there, collect again first</b>; otherwise it is for a person to judge."}
	tSkipped     = T{KO: "읽지 못한 결과 파일", EN: "Result files that could not be read"}
	tSkippedHint = T{
		KO: "빠진 노드를 모르면 「관측 안 됨」과 「못 읽음」이 뒤섞입니다.",
		EN: "If you do not know which nodes are missing, “not observed” and " +
			"“could not read” get mixed up."}

	tSurveyAssets     = T{KO: "자산", EN: "Assets"}
	tSurveyAssetsNote = T{KO: "선언과 맞댄 3-상태", EN: "three states against the declaration"}
	tSurveyNoAssets   = T{KO: "대조할 자산이 없습니다.", EN: "No assets to reconcile."}
	tShadowHint       = T{
		KO: "<b>UNDECLARED 를 찾아내는 것이 이 도구의 첫 번째 쓸모입니다</b> — 선언에 없는데 " +
			"실제로 쓰이고 있는 것입니다. 판정은 <a href=\"/review\">리뷰 큐</a>에서 합니다.",
		EN: "<b>Finding UNDECLARED is the first thing this tool is good for</b> — things in " +
			"actual use that nobody declared. Judge them in the " +
			"<a href=\"/review\">review queue</a>."}

	tSurveyEdges     = T{KO: "통신 엣지", EN: "Communication edges"}
	tSurveyEdgesNote = T{
		KO: "선언과 맞댄 3-상태 · 양자내성 등급",
		EN: "three states against the declaration · quantum-resistance grade"}
	tSurveyNoEdges = T{KO: "대조할 엣지가 없습니다.", EN: "No edges to reconcile."}
	tPostures      = T{KO: "🟢 PQC <b>%d</b> · 🔴 고전 <b>%d</b> · ⚪ 불명 <b>%d</b>",
		EN: "🟢 PQC <b>%d</b> · 🔴 classical <b>%d</b> · ⚪ unknown <b>%d</b>"}
	tColGroup       = T{KO: "협상 그룹", EN: "Negotiated group"}
	tColReconcile   = T{KO: "대조", EN: "Reconciled"}
	tGradeClassical = T{KO: "고전 — 양자 취약", EN: "classical — quantum-vulnerable"}
	tGradeUnknown   = T{KO: "불명", EN: "unknown"}
	tUncoveredSrc   = T{KO: "관측 안 된 노드", EN: "node not observed"}
	tOffScope       = T{KO: "등재 판정 요청", EN: "needs enrollment decision"}

	tTopology     = T{KO: "토폴로지", EN: "Topology"}
	tTopologyNote = T{
		KO: "색은 양자내성 등급, 선 모양은 대조 상태",
		EN: "colour is the quantum-resistance grade, line style is the reconciliation state"}
	tTopologyHint = T{
		KO: "색은 양자내성 등급, 선 모양은 대조 상태입니다. <b>보지 못한 것은 점선</b>이라 " +
			"없는 것과 구분됩니다.",
		EN: "Colour is the quantum-resistance grade, line style the reconciliation state. " +
			"<b>What was not seen is dashed</b>, so it is distinct from what is absent."}
	tNoDot = T{
		KO: "<code>dot</code>(Graphviz)이 이 기계에 없어 그리지 못했습니다. " +
			"<b>선택 사항입니다</b> — 설치하지 않아도 나머지는 그대로 돕니다(README 「사전 준비」). " +
			"<code>apt install graphviz</code> · <code>brew install graphviz</code> · " +
			"<code>winget install graphviz</code>.<br>설치하지 않고 그리려면 아래를 저장해 " +
			"아무 데서나 <code>dot -Tsvg topology.dot -o topology.svg</code> 로 그리십시오.",
		EN: "<code>dot</code> (Graphviz) is not on this machine, so the graph was not drawn. " +
			"<b>It is optional</b> — everything else works without it (see README, " +
			"“Prerequisites”). <code>apt install graphviz</code>, " +
			"<code>brew install graphviz</code>, <code>winget install graphviz</code>.<br>" +
			"To draw it without installing, save the source below and run " +
			"<code>dot -Tsvg topology.dot -o topology.svg</code> anywhere."}
)
