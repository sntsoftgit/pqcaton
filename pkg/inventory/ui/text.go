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
	tNavScope  = T{KO: "② 암호 자산 스코프", EN: "② Crypto asset scope"}
	tNavSurvey = T{KO: "③ 대조", EN: "③ Reconciliation"}
	tNavReview = T{KO: "④ 판정(리뷰 큐)", EN: "④ Judgment (review queue)"}

	tTitleDecl   = T{KO: "선언", EN: "Declaration"}
	tTitleScope  = T{KO: "암호 자산 스코프", EN: "Crypto asset scope"}
	tTitleSurvey = T{KO: "대조", EN: "Reconciliation"}
	tTitleReview = T{KO: "판정(리뷰 큐)", EN: "Judgment (review queue)"}

	tSubOrg     = T{KO: "조직", EN: "org"}
	tSubSession = T{KO: "세션", EN: "session"}
	tSubResults = T{KO: "결과", EN: "results"}

	// 저장·확정 뒤에 뜨는 알림. **화면 글이라 보는 사람의 말로 뜬다.**
	tSavedNotFinal = T{
		KO: "세션 파일에 저장했습니다. 아직 확정하지 않았습니다",
		EN: "Saved to the session file — not finalized yet"}
	tFinalizedPlan = T{
		KO: "확정했습니다. 조치 %d건을 %s 에 썼습니다",
		EN: "Finalized — %d actions written to %s"}
	tFinalizedPolicy = T{
		KO: "확정했습니다. 규칙 %d개를 %s 에 썼습니다. 이 파일이 `pqcota-ingest -scope-assets` 의 입력입니다",
		EN: "Finalized — %d rules written to %s. That file is the input to `pqcota-ingest -scope-assets`"}
	tJudgmentsSaved = T{
		KO: " · 판정 %d건을 %s 에 남겼습니다",
		EN: " · %d judgments appended to %s"}
	tRulesSaved = T{
		KO: "계층 %d개에 규칙 %d개를 썼습니다. 판정할 변경이 %d건이고, 그중 %d건은 왜 뺐는지를 적어야 합니다",
		EN: "Wrote %d rules across %d layers — %d changes to judge, %d of them needing a recorded reason"}
	tSignatureCleared = T{
		KO: " · 정책이 달라져 서명을 지웠습니다",
		EN: " · the policy changed, so the signature was cleared"}
	tDeclSaved = T{
		KO: "선언을 저장했습니다. 노드 %d · 자산 %d · 엣지 %d",
		EN: "Declaration saved — %d nodes · %d assets · %d edges"}
	tDeclDroppedNoIP = T{
		KO: " · IP를 적지 않은 노드 %d개는 관리 대상에서 뺐습니다",
		EN: " · %d nodes with no IP were left out of management"}
	tDeclStillOff = T{
		KO: " · 맞지 않는 자리 %d곳이 남아 있습니다",
		EN: " · %d places still do not add up"}

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
		KO: "대조와 판정이 이 조직에 묶입니다. 비우면 <code>local</code> 입니다.",
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
			"걸치면 IP도 둘이니 쉼표나 공백으로 나눠 적으십시오. <b>IP를 적지 않은 줄은 " +
			"관리 대상이 되지 않습니다</b>. 이을 근거가 없으면 대조 결과가 오류 없이 " +
			"틀리기 때문입니다. 저장하면 그 줄은 표에서 사라집니다. 이름을 비워도 지워집니다.<br>" +
			"<b>「관측 이름」은 관측이 이 노드를 부르는 이름입니다.</b> 자산 대조는 노드 " +
			"이름이 글자 그대로 같아야 맞는데, collector 는 자기가 붙인 " +
			"id(<code>node:1a2b…</code>)나 호스트명으로 보내는 일이 흔합니다. 이름이 서로 다르면 " +
			"<b>그 노드의 자산이 통째로 UNDECLARED 로</b> 올라옵니다. 선언이 틀려서가 아니라 " +
			"이름이 서로 달라서입니다. 호스트명(짧은 이름 포함)이 위 이름과 같으면 비워 두십시오. " +
			"그때는 저절로 이어집니다. 아직 어느 노드에도 붙지 않은 관측 이름은 " +
			"이 칸에서 고를 수 있습니다.",
		EN: "The IP is <b>what ties an observed address back to this name</b>. A node on two " +
			"networks has two IPs — separate them with commas or spaces. <b>A row with no IP " +
			"is not under management</b> — without that tie, reconciliation comes out wrong " +
			"without erroring. Saving drops the row. Clearing the name drops it too.<br>" +
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
			"관측 이름에서 <code>.so</code> 부터 뒤를 뗍니다. <code>libssl.so.3</code> 은 " +
			"<code>libssl</code> 로 적습니다. <b>벤더링 해시는 떼지 않습니다</b>. " +
			"<code>libcrypto-fbc9a285.so.3</code> 은 <code>libcrypto-fbc9a285</code> 여야 하고, " +
			"<code>libcrypto</code> 는 다른 자산입니다. <code>jca</code> 의 컴포넌트는 " +
			"<code>jca-provider-chain</code> 하나, <code>cng</code> 는 " +
			"<code>cng-providers</code> 하나입니다. 기계에 하나뿐이라 관측 결과에 늘 그 " +
			"이름으로 적힙니다.<br>맞지 않아도 막히지 않습니다. " +
			"<b>선언한 것은 미관측으로, 관측된 것은 UNDECLARED 로</b> 구분됩니다. 그래서 " +
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
		KO: "저장하면 <code>pqcaton-report</code> 가 읽는 바로 그 파일에 씁니다. " +
			"화면 위에 경로가 적혀 있습니다.",
		EN: "Saving writes to the very file <code>pqcaton-report</code> reads — " +
			"its path is shown at the top of this page."}

	tColName = T{KO: "이름", EN: "Name"}
	tColIP   = T{KO: "IP", EN: "IP"}
	// **관측이 이 노드를 뭐라고 부르는가.** 자산 대조는 노드 이름이 글자 그대로 같아야
	// 맞는데, collector 는 자기가 붙인 id 나 호스트명으로 보낸다.
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
			"provider 를 갈아 끼우는 자리가 아닙니다. <code>cng</code> 가 그렇습니다. " +
			"쓸 수 있는 알고리즘은 Windows 빌드가, FIPS 는 OS 정책이 정하므로 계획의 " +
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

	tColTarget     = T{KO: "대상", EN: "Target"}
	tColState      = T{KO: "상태", EN: "State"}
	tColConfidence = T{KO: "신뢰도", EN: "Confidence"}
	tColInPlan     = T{KO: "계획에 넣기", EN: "In plan"}
	tColException  = T{KO: "개별 결론(예외)", EN: "Per-item conclusion (exception)"}
	tRescan        = T{KO: "재수집 후보", EN: "rescan candidate"}
	// **provider 를 갈아 끼우는 조치가 아닌 자리.** 계획의 provider 칸이 비는데, 화면이
	// 말하지 않으면 빠뜨린 것으로 읽힌다.
	tPlatformFix = T{KO: "플랫폼 조치", EN: "platform action"}

	tReviewEmpty    = T{KO: "판정할 것이 없습니다.", EN: "Nothing to judge."}
	tReviewAutopass = T{
		KO: "자동통과 후보 %d개는 이 큐에 올리지 않았습니다. 선언과 맞고 신뢰도가 높은 " +
			"것들이라 하나씩 볼 필요가 없습니다.",
		EN: "%d auto-pass candidates are not in this queue — they match the declaration " +
			"with high confidence, so they are not worth looking at one by one."}

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
		KO: "확정은 <code>pqcaton-decide close</code> 와 같은 검사를 거칩니다. " +
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
		KO: "라이브러리 이름. <code>*</code> 를 쓸 수 있습니다. <code>libcrypto.so.*</code>",
		EN: "Library name. <code>*</code> is allowed — <code>libcrypto.so.*</code>"}
	tRuleAppKey = T{
		KO: "그 라이브러리를 쓰는 실행 파일. <code>/usr/bin/python*</code> · <code>/usr/sbin/sshd</code>",
		EN: "The executable using it. <code>/usr/bin/python*</code>, <code>/usr/sbin/sshd</code>"}
	tRuleNote = T{
		KO: "사람이 읽는 설명입니다. <b>판정의 근거가 아닙니다.</b> 근거는 아래 결론 칸에 적습니다",
		EN: "A note for people to read. <b>Not the reason for a judgment</b> — that goes in the conclusion field below"}

	tRuleEmptyWarn = T{
		KO: "<b>빈 칸은 「전부」입니다.</b> 그래서 <code>runtime</code>·<code>lib</code>·" +
			"<code>app_key</code> 세 칸이 모두 빈 줄은 규칙으로 만들지 않습니다. 그대로 두면 " +
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
		KO: "줄을 지우려면 <code>runtime</code>·<code>lib</code>·<code>app_key</code> 를 모두 비우십시오.",
		EN: "To delete a row, clear <code>runtime</code>, <code>lib</code> and <code>app_key</code>."}

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
		KO: "확정은 <code>pqcaton-scope close</code> 와 같은 검사를 거칩니다. 나온 CSV 가 " +
			"<code>pqcota-ingest -scope-assets</code> 의 입력입니다.",
		EN: "Finalizing goes through the same gate as <code>pqcaton-scope close</code>. " +
			"The CSV it emits is the input to <code>pqcota-ingest -scope-assets</code>."}
)

// ── 대조 ───────────────────────────────────────────────────────────────────
var (
	tSurveyObserved     = T{KO: "관측", EN: "Observation"}
	tSurveyObservedHint = T{
		KO: "pqcota 가 무엇을 보았는지 보여 줍니다. 대상 노드에 collector를 가져가 실행하고 " +
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
		KO: "<b>못 본 것과 없는 것은 다릅니다.</b> 아래 UNOBSERVED 가 「쓰지 않는 것」인지 " +
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
		KO: "노드 안의 암호 런타임·컴포넌트를 선언과 맞댄 3-상태입니다.",
		EN: "The crypto runtimes and components inside each node, in three states against the declaration."}
	tSurveyNoAssets = T{KO: "대조할 자산이 없습니다.", EN: "No assets to reconcile."}
	tUndeclaredHint = T{
		KO: "<b>UNDECLARED 를 찾아내는 것이 이 도구의 첫 번째 쓸모입니다</b>. 선언에 없는데 " +
			"실제로 쓰이고 있는 것입니다. 판정은 <a href=\"/review\">판정(리뷰 큐)</a> 탭에서 합니다.",
		EN: "<b>Finding UNDECLARED is the first thing this tool is good for</b> — things in " +
			"actual use that nobody declared. Judge them in the " +
			"<a href=\"/review\">judgment (review queue)</a> tab."}

	tSurveyEdges     = T{KO: "통신 엣지", EN: "Communication edges"}
	tSurveyEdgesHint = T{
		KO: "선언과 맞댄 3-상태, 그리고 협상된 통신의 양자내성 등급입니다.",
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
			"아무 데서나 <code>dot -Tsvg topology.dot -o topology.svg</code> 로 그리십시오.",
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
		KO: "이 화면이 읽는 것은 <b>지금 가지고 있는 파일뿐입니다</b>. 관측 결과, 판정 원장, " +
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
	tReasonStaleKO = T{KO: "승인한 지 오래됐습니다. 빼둔 사이 달라졌을 수 있습니다", EN: "the approval is stale — things may have changed while it was set aside"}
	tColEvidence   = T{KO: "관측 근거", EN: "Evidence"}

	tStale     = T{KO: "근거가 바뀐 판정", EN: "Judgments whose basis changed"}
	tStaleHint = T{
		KO: "재관측 뒤 <b>근거가 달라진 판정</b>입니다. 전면 재리뷰가 아니라 이것만 " +
			"다시 봅니다. <code>pqcaton-decide delta</code> 와 같은 계산입니다.",
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
		KO: "판정 원장을 주지 않았습니다. <code>-judgments</code> 로 지정하면 이력과 " +
			"「근거가 바뀐 판정」이 열립니다.",
		EN: "No judgment ledger was given — pass <code>-judgments</code> to open the history " +
			"and the “basis changed” section."}
	tNoPolicy = T{
		KO: "정책 CSV를 주지 않았습니다. <code>-scope-out</code>(확정된 정책)을 주면 " +
			"「안 보고 있는 것」이 열립니다.",
		EN: "No policy CSV was given — pass <code>-scope-out</code> (the finalized policy) " +
			"to open “what is not being looked at”."}
)
