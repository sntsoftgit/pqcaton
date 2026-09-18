// Package review — 사람이 판정하는 세션의 파일 형식과 **확정 관문**.
//
// 명령에서 떼어 둔 것은 관문이 둘이 되면 안 되기 때문이다. `pqcaton-decide close`와
// 화면의 확정 버튼이 각자 관문을 들고 있으면 언젠가 한쪽만 고쳐지고, 그날 화면으로는
// 확정되는데 명령으로는 안 되는(또는 그 반대의) 계획이 생긴다.
//
// **파일 형식이 곧 감사 기록이다.** 화면이 생겨도 산출물은 파일이다 — 무엇을 근거로 무엇을
// 정했는지가 화면에서 사라지지 않는다.
package review

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	provisioningv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/history"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/randyinthedev-hash/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

// Session — 사람이 편집하는 파일. **라이브러리 타입을 그대로 쓰지 않는다** — 편집하는
// 사람에게 필요한 것과 상태기계가 필요한 것이 다르다.
type Session struct {
	Note      string `json:"_how_to_read"`
	Scope     string `json:"scope"`
	Reviewer  string `json:"reviewer"`
	Signature string `json:"signature"`
	// PolicyDecisions — 정책 하나에 결론 하나. **이것이 기본 단위다**(§3.4) — 수천 대를
	// 한 건씩 보는 리뷰는 끝나지 않는다. 개별 항목의 Conclusion은 예외를 위한 자리다.
	PolicyDecisions map[string]string `json:"policy_decisions"`
	// RulesetVersion — 이 세션을 연 시점의 규칙 판. **여는 순간 박고 확정할 때 다시 찍지
	// 않는다.** 검토 도중 도구가 올라가도 실제 검토 근거가 보존되어야 하기 때문이다.
	RulesetVersion string `json:"ruleset_version,omitempty"`
	// SessionID — 이 세션의 동일성. **열 때 만들고, 다시 열어도 그대로다.** 계약으로 나가는 계획의
	// id가 이 값을 담고 판정 원장 행도 이 값을 들고 가서, 계획에서 원장으로 되짚는 식별자가 된다.
	// 「어느 계획 사건인가」를 답하는 값이라 내용 해시가 아니다 — 같은 내용이 두 번 승인·실행될
	// 수 있고, 그때 감사 기록이 겹치면 안 된다. 근거 해시([BasisOf])에는 넣지 않는다 — 동일성이지
	// 판정의 근거가 아니다.
	SessionID string `json:"session_id,omitempty"`
	Items     []Item `json:"items"`
	// Autopass — 기계가 답한 항목(CONFIRMED + 고신뢰). **판정을 다시 받지 않지만 계획에는 들어갈 수
	// 있다.** 자동통과는 「사람이 볼 필요가 없다」이지 「바꿀 필요가 없다」가 아니다 - 선언과 맞고
	// 확인된 자산이야말로 가장 자연스러운 조치 대상이다. Item과 같은 형이라 계획 칸을 든다.
	// 결론 칸은 쓰지 않는다.
	Autopass []Item `json:"autopass_candidates"`
	// LegacyAutopass — 옛 빌드가 남긴 식별자 문자열. **표시용이고 계획에 넣을 수 없다.** ID 밖에
	// 없어 노드·근거·관리 판정을 복원할 방법이 없고, 빈 칸을 채워 일반 항목으로 올리면 사람이 새
	// 후보로 읽는다. 관측에서 같은 ID를 찾으면 Carry가 치환한다. 이름을 새로 둔 것은 같은 이름에
	// 두 형을 허용하면 읽는 코드가 형을 살펴야 하고 그 분기는 한 번 쓰면 지워지지 않기 때문이다.
	LegacyAutopass []string `json:"autopass_candidates_legacy,omitempty"`
}

// Item — 판정 대상 하나.
type Item struct {
	ID string `json:"id"`
	// Node · Runtime — **id에서 되찾지 않고 여기 적어 둔다.** 노드가 `host://local` 같은
	// URI면 `/`로 쪼개 복원할 수 없다 — 조치가 엉뚱한 노드를 겨누고, 런타임이 비어
	// 기본값으로 표시 없이 떨어진다. 대조할 때 이미 알던 값이므로 그대로 들고 간다.
	Node    string `json:"node"`
	Runtime string `json:"runtime"`
	// FindingID — 이 항목을 낸 관측의 id. **조치의 근거로 상류까지 간다**(계약의 `finding_id`).
	// 사람이 채우는 자리가 아니라 대조가 들고 온 사실이다. UNOBSERVED는 관측이 없어 빈다.
	FindingID string `json:"finding_id,omitempty"`
	// Fingerprint — 근거 관측의 **내용** 지문(`reconcile.Fingerprint`). 사람이 채우는 자리가
	// 아니라 대조가 들고 온 사실이다. `finding_id`는 자산이 같으면 같으므로, 버전이 오르거나
	// 강화 판정이 달라진 것을 id 로는 알 수 없다 — 그 자리를 이 값이 맡는다.
	Fingerprint string `json:"evidence_fingerprint,omitempty"`
	// Sources — 근거 전부. 같은 자산을 원천 노드 여럿이 봤으면 여럿이고 주 근거가 앞이다
	// (`FindingID`·`Fingerprint`는 주 근거의 것). 계약의 `evidence_sources`로 나간다. 근거 해시에는
	// 정렬된 (finding, 지문, 원천 노드) 묶음이 들어간다 — 주 근거만 보면 보조 근거가 바뀌어도 판정
	// 서명이 살아남는다. 스냅샷 지문은 근거 해시에 넣지 않는다 — 위치이지 근거가 아니다.
	Sources []EvidenceSource `json:"evidence_sources,omitempty"`
	// Managed — 관리 축(대조의 ManagedState). **세션 파일에 적는다.** BasisOf가 이 값을 덮고,
	// 계획 차단이 이 값을 본다. 파일에 없으면 다시 열 때 사라져, 해시가 관리 판정을 덮지 못하고
	// 손으로 고친 파일을 막을 수도 없다. 옛 파일(v2 이하)에는 이 칸이 없다 - 큐에 오른 항목은
	// 전부 관리 대상이었으므로 빈 값을 MANAGED로 읽는다.
	Managed string `json:"managed,omitempty"`
	// ExcludedSources — 정책이 뺀 근거. **계약으로 나가지 않는다**(형이 다르다). 해시에는 들어간다 -
	// 근거 구성이 바뀌면 서명이 무효가 돼야 하기 때문이다.
	ExcludedSources []ExcludedSource `json:"excluded_sources,omitempty"`
	// Policy — 같은 정책의 항목은 한 번에 판정한다(§3.4).
	Policy string  `json:"policy"`
	State  string  `json:"state"`
	Conf   float64 `json:"confidence"`
	// ConfEvaluated — Conf가 잰 값인가. 거짓이면 UNOBSERVED의 상태 기본값 같은 것이다. 옛 파일(v2
	// 이하)에는 이 칸이 없어 거짓으로 읽히는데, 그 판에는 미평가라는 개념이 없었으므로 규칙 판을
	// 보고 참으로 읽는다([Item.ConfidenceEvaluated]).
	ConfEvaluated bool `json:"confidence_evaluated,omitempty"`
	// Mandatory — 이 항목은 결론 없이 확정할 수 없다(§3.3②).
	Mandatory bool `json:"mandatory"`
	// Rescan — UNOBSERVED인데 커버리지 갭으로 설명된다. **「없다」가 아니라 「못 봤다」**이므로
	// 재수집이 먼저다(§2.6).
	Rescan bool `json:"rescan_candidate,omitempty"`

	// ── 사람이 채우는 자리 ──
	Conclusion string `json:"conclusion"`
	Plan       bool   `json:"include_in_plan"`
	Level      string `json:"deploy_level,omitempty"` // L1 | L2 | L3
	FIPS       bool   `json:"fips_required,omitempty"`
	// Kind — 조치 종류. 계약의 통제 어휘다(`REMEDIATION_KIND_*`). **비우면 확정되지 않는다.**
	Kind string `json:"remediation_kind,omitempty"`
	// TargetAlgorithm — 무엇으로 바꾸는가. 비우면 상류가 낸 config 조각의 `Groups` 줄이 주석으로
	// 나가 **배치해도 아무것도 켜지지 않는다.** 도구가 고르지 않는 값이라 사람이 적는다.
	TargetAlgorithm string `json:"target_algorithm,omitempty"`
	// Config — provider 설정 조각. **도구가 지어내지 않는다.**
	Config string `json:"config_artifact,omitempty"`
	// RollbackNote — 계약의 rollback_note. **계획 축의 값이다.** 전에는 판정 결론을 그대로 넣었는데,
	// 결론은 「어떻게 판정했나」이고 이것은 「어떻게 되돌리나」다. 자동통과 항목은 결론을 요구하지
	// 않으므로 결론에 기대면 빈 값이 계약으로 나간다. 옛 판(v2 이하)의 세션에서 비어 있으면 결론으로
	// 물러선다 - 그 자리에 있던 값이 실제로 되돌림 메모 구실을 해 왔다. v3부터 빈 값은 비우기로 한 것이다.
	RollbackNote string `json:"rollback_note,omitempty"`
	// 활성화 훅 — L3에서만 쓰인다. **도구가 추측하지 않는다**(상류 §2.5): 활성화 지점은 앱
	// 기동 방식에 달려 있어 관측으로 알 수 없다. 비면 상류가 무엇이 일어나지 않는지 알린다.
	Pre        string `json:"activation_pre,omitempty"`
	Activate   string `json:"activation_activate,omitempty"`
	Deactivate string `json:"activation_deactivate,omitempty"`
	Restart    string `json:"activation_restart,omitempty"`
}

// EvidenceSource — 근거 하나. 대조의 Source를 세션 파일에 적는 꼴이다.
type EvidenceSource struct {
	FindingID   string `json:"finding_id"`
	Fingerprint string `json:"fingerprint"`
	Evidence    string `json:"evidence,omitempty"`
	// 스냅샷 위치. 원천 노드는 봉투의 이름이라 항목의 Node(선언 이름)와 다를 수 있다.
	SourceNodeID    string `json:"source_node_id"`
	SnapshotDigest  string `json:"snapshot_digest"`
	SnapshotRuleset string `json:"snapshot_ruleset"`
}

// All — 이 세션의 항목 전부(Items + Autopass). **읽기 전용이다.**
//
// 돌려주는 것은 값의 복사본이므로 여기에 쓰면 세션은 바뀌지 않는다. 해시·비교·선택 수집처럼
// **읽는** 자리에만 쓴다. 값을 고치는 자리(Carry·화면 저장)는 [update]로 실제 컬렉션의 자리를
// 찾아 쓴다 - 복사본에 쓰면 쓰고 나서 아무 일도 일어나지 않는 코드가 된다.
//
// **판정 생성·결론 검사·판정 원장은 이 함수를 쓰지 않는다.** 그 자리는 Items만 본다 - 자동통과는
// 판정을 다시 받지 않는다. 여기에 넣으면 결론을 요구하게 된다.
func All(sf Session) []Item {
	out := make([]Item, 0, len(sf.Items)+len(sf.Autopass))
	out = append(out, sf.Items...)
	out = append(out, sf.Autopass...)
	return out
}

// Selected — 계획에 고른 것(Plan=true). 리뷰 항목과 자동통과 양쪽에서. 읽기 전용이다.
func Selected(sf Session) []Item {
	var out []Item
	for _, it := range All(sf) {
		if it.Plan {
			out = append(out, it)
		}
	}
	return out
}

// update — ID로 실제 컬렉션의 자리를 찾아 그 항목을 고친다. 찾았으면 참이다.
func update(sf *Session, id string, fn func(*Item)) bool {
	for i := range sf.Items {
		if sf.Items[i].ID == id {
			fn(&sf.Items[i])
			return true
		}
	}
	for i := range sf.Autopass {
		if sf.Autopass[i].ID == id {
			fn(&sf.Autopass[i])
			return true
		}
	}
	return false
}

// checkNoDuplicateIDs — 한 자산은 한 컬렉션에만 있다. 양쪽에 같은 ID가 있으면 대조의 결함이므로
// 오류로 중단한다 - 그대로 두면 Carry가 한쪽에만 쓰고 다른 쪽이 그것을 덮는다.
func checkNoDuplicateIDs(sf Session) error {
	seen := map[string]string{}
	for _, it := range sf.Items {
		seen[it.ID] = "items"
	}
	for _, it := range sf.Autopass {
		if where, dup := seen[it.ID]; dup {
			return fmt.Errorf("item %s appears in both %s and autopass_candidates — one asset belongs in exactly one of them", it.ID, where)
		}
	}
	return nil
}

// ExcludedSource — 정책이 뺀 근거 하나. 대조의 ExcludedSource를 세션 파일에 적는 꼴이다.
// **EvidenceSource와 형이 다르다** - 계약 변환(evidenceOf)이 이 값을 받지 못한다. 스냅샷 위치가
// 없는 것은 이 finding이 중앙 이력에 없기 때문이다.
type ExcludedSource struct {
	FindingID    string   `json:"finding_id"`
	Fingerprint  string   `json:"fingerprint"`
	Evidence     string   `json:"evidence,omitempty"`
	SourceNodeID string   `json:"source_node_id"`
	AppKeys      []string `json:"app_keys,omitempty"`
}

// ExcludedSourcesOf — 대조 결과의 제외 근거를 세션 항목의 꼴로. SourcesOf와 짝이다.
func ExcludedSourcesOf(r reconcile.Reconciled) []ExcludedSource {
	out := make([]ExcludedSource, 0, len(r.ExcludedSources))
	for _, x := range r.ExcludedSources {
		out = append(out, ExcludedSource{FindingID: x.FindingID, Fingerprint: x.Fingerprint, Evidence: x.Evidence,
			SourceNodeID: x.SourceNodeID, AppKeys: append([]string(nil), x.AppKeys...)})
	}
	return out
}

// ItemOf — 대조 결과 하나를 세션 항목으로. **항목을 만드는 자리는 여기 하나다.** 명령과 화면이
// 따로 만들면 한쪽만 칸을 더하는 날이 오고, 그날 그 칸은 한쪽 세션 파일에만 있다.
func ItemOf(r reconcile.Reconciled, mandatory bool) Item {
	return Item{
		ID: Key(r.Key), Policy: PolicyOf(r.Key),
		Node: r.Key.NodeID, Runtime: r.Key.Runtime,
		FindingID: r.FindingID, Fingerprint: r.Fingerprint, Sources: SourcesOf(r),
		Managed: string(r.Managed), ExcludedSources: ExcludedSourcesOf(r),
		// 위임 수준은 **실제 값으로 저장한다.** 화면에만 기본으로 보여 주고 비워 두면
		// 검토자가 고르지 않은 값이 나중에 기본값으로 채워지고, 그 결과에 승인 서명이
		// 붙는다. 저장해 두면 검토자에게 보이고 승인 대상에 들어간다. 바꾸는 것은 화면에서 한다.
		Level: "L2",
		State: string(r.State), Conf: r.Confidence, ConfEvaluated: r.ConfidenceEvaluated,
		Mandatory: mandatory, Rescan: r.RescanCandidate,
	}
}

// ManagedOf — 항목의 관리 축. 옛 파일(빈 값)은 MANAGED 다 - 그 판에서 큐에 오른 항목은 전부
// 관리 대상이었다.
func ManagedOf(it Item) reconcile.ManagedState {
	if it.Managed == "" {
		return reconcile.Managed
	}
	return reconcile.ManagedState(it.Managed)
}

// ConfidenceEvaluated — 항목의 신뢰도가 잰 값인가. 규칙 판이 v2 이하이면 칸의 부재를 평가됨으로
// 읽는다 - 그 판에는 미평가라는 개념이 없었다. v3 부터는 칸의 값 그대로다.
func ConfidenceEvaluated(it Item, rulesetVersion string) bool {
	if it.ConfEvaluated {
		return true
	}
	return !rulesetAtLeastV3(rulesetVersion)
}

// SourcesOf — 대조 결과의 근거를 세션 항목의 꼴로 옮긴다.
func SourcesOf(r reconcile.Reconciled) []EvidenceSource {
	out := make([]EvidenceSource, 0, len(r.Sources))
	for _, s := range r.Sources {
		out = append(out, EvidenceSource{FindingID: s.FindingID, Fingerprint: s.Fingerprint, Evidence: s.Evidence,
			SourceNodeID: s.Snapshot.SourceNodeID, SnapshotDigest: s.Snapshot.Digest, SnapshotRuleset: s.Snapshot.RulesetVersion})
	}
	return out
}

// RulesetVersion — **이 판정의 근거가 된 규칙 판.** 상류의 강화 규칙과 이 리포의 대조·계획
// 규칙을 합친 것이다. 파생 결과를 바꿀 수 있는 규칙이 어느 쪽에서든 바뀌면 값이 달라진다.
//
// **릴리스 버전과 같지 않다.** 화면을 고치거나 문구를 다듬는다고 같은 관측에서 다른 판정이
// 나오지 않는다. 반대로 대조 규칙이나 계획 변환이 바뀌면, 릴리스를 내지 않아도 이 값은 올라야
// 한다 — 그때 옛 판정의 근거가 지금 규칙과 다르다는 사실이 이 문자열로 드러난다.
//
// **사용자가 입력하지 않는다.** 도구가 부여하고, 세션을 **열 때** 넣어 둔다(확정할 때가 아니다).
// 검토 도중 도구가 올라가도 실제로 검토한 근거가 그대로 남아야 하기 때문이다.
//
// v2 — 주 근거를 입력 순서가 아니라 가장 강한 증거로 고르고, 계약 변환이 근거 여럿을 낸다. 같은 관측에서
// 다른 판정·다른 계획이 나오므로 올렸다. 상류도 같은 이유(병합 규칙)로 v2 다.
//
// v3 — 판정 축과 계획 축, 대조 축과 관리 축을 나눴다. 자동통과 자산이 계획에 들어올 수 있고, 정책이
// 뺀 선언 자산이 리뷰 큐에서 빠진다 - 같은 관측에서 다른 계획이 나온다. 근거 해시가 관리 판정·제외
// 근거·미평가 여부를 덮으므로 **v3으로 다시 열어 Carry 한 세션의 서명은 무효**다. 저장된 옛 세션을
// 그대로 확정하면 서명은 살아 있고 경고만 난다([Finalize]) - 검토 중인 세션이 도구 교체로 버려지면
// 사람이 한 일이 사라진다.
const RulesetVersion = normalize.RulesetVersion + "+pqcaton-plan/v3"

// planRulesetNumber — 결합 판 문자열에서 이 리포의 계획 규칙 판 번호를 꺼낸다. 없거나 못 읽으면
// 0 이다(옛 빌드가 연 세션, 또는 규칙 판을 안 적은 것).
func planRulesetNumber(rulesetVersion string) int {
	const tag = "+pqcaton-plan/v"
	i := strings.LastIndex(rulesetVersion, tag)
	if i < 0 {
		return 0
	}
	n, err := strconv.Atoi(rulesetVersion[i+len(tag):])
	if err != nil {
		return 0
	}
	return n
}

// rulesetAtLeastV3 — 이 세션이 v3 이후 규칙으로 열렸나. 세션 파일의 옛 칸(관리 축 · 미평가 ·
// 되돌림 메모)을 어떻게 읽을지가 여기에 달린다. **세션이 들고 있는 규칙 판을 본다** - 지금 실행
// 파일의 상수가 아니다. 검토 도중 도구가 올라갈 수 있다.
func rulesetAtLeastV3(rulesetVersion string) bool { return planRulesetNumber(rulesetVersion) >= 3 }

// NewSessionID — 세션을 **열 때** 한 번 만든다. UUID v4 다. 표준 라이브러리만 쓴다 — 식별자
// 하나를 위해 의존성을 들이지 않는다.
//
// 세션을 다시 열면 [Carry]가 앞 세션의 것을 옮기므로 새로 만들지 않는다. 새 세션이면 내용이
// 같아도 다른 값이다.
func NewSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand가 실패하는 환경은 이 도구가 돌 수 없는 환경이다. 알리지 않고 약한 값을
		// 내느니 여기서 멈춘다 — 식별자가 겹치면 감사 기록이 겹친다.
		panic("session id: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Note — 세션 파일 첫 줄에 적히는 사용법.
const Note = "Write one conclusion per policy under policy_decisions and every item in that " +
	"policy is judged at once (recommended). Use an item's own conclusion only for exceptions. " +
	"Fill in reviewer and signature, then feed this to `pqcaton-decide close`. " +
	"Set include_in_plan to true for items that go into the finalized plan — each of those also needs " +
	"deploy_level and remediation_kind chosen, target_algorithm when the kind delivers through config, " +
	"and the activation hooks when the level is L3. New items start at deploy_level L2 for you to confirm " +
	"or change; nothing else is filled in, and nothing empty is corrected at finalize time. Your signature " +
	"here records who judged; it is not the execution approval. The plan goes out as IN_REVIEW and " +
	"`pqcota-approve` (upstream, with the approver's own key) raises it to FINALIZED. That approval " +
	"covers every field, so a silent default would put an approver's name on a decision nobody made."

// Load — 세션 파일을 읽는다.
func Load(path string) (Session, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}
	return Decode(raw)
}

// sessionWire — 세션 파일의 표면. autopass_candidates가 판에 따라 객체 배열이거나 문자열 배열이라,
// 형을 정하지 않고 먼저 받는다. **Session에 직접 붙이지 않는다** - 형이 안 맞으면 Unmarshal이
// 그 자리에서 실패해 변환 코드에 닿지 못한다. 나머지 칸은 Session을 그대로 판다.
type sessionWire struct {
	Session
	Autopass json.RawMessage `json:"autopass_candidates"`
}

// Decode — 세션 파일의 바이트를 읽는다. 옛 형식의 자동통과 목록(문자열 배열)은 **표시용으로
// 보존**하고 계획에는 넣지 못하게 둔다 - ID 밖에 없어 노드·근거·관리 판정을 복원할 수 없고,
// 빈 칸을 채워 일반 항목으로 올리면 사람이 새 후보로 읽는다. 모르는 형식은 오류로 중단한다 -
// 「모르니 비워 둔다」로 넘기면 사람이 검토한 목록이 표시 없이 사라진다.
func Decode(raw []byte) (Session, error) {
	var w sessionWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return Session{}, fmt.Errorf("session file: %w", err)
	}
	sf := w.Session
	sf.Autopass = nil
	if len(w.Autopass) == 0 || string(w.Autopass) == "null" {
		return sf, nil
	}
	var items []Item
	if err := json.Unmarshal(w.Autopass, &items); err == nil {
		sf.Autopass = items
		return sf, nil
	}
	var legacy []string
	if err := json.Unmarshal(w.Autopass, &legacy); err != nil {
		return Session{}, fmt.Errorf("session file: autopass_candidates is neither a list of items nor a list of ids: %w", err)
	}
	sf.LegacyAutopass = append(sf.LegacyAutopass, legacy...)
	return sf, nil
}

// Save — 세션 파일을 쓴다. 화면이 판정을 채워 넣는 자리다.
//
// **덮어쓰되 형식은 그대로다.** 화면으로 채운 파일을 명령이 그대로 읽을 수 있어야 한다 —
// 아니면 두 길이 갈린다.
func Save(path string, sf Session) error {
	raw, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// Result — 확정이 통과한 결과.
type Result struct {
	Plan *provisioningv1.FinalizedPlan
	// Decided — 실제로 판정된 항목의 결론. 정책 일괄로 붙은 것도 들어온다.
	Decided map[string]string
	// Batched — 정책별 일괄 판정 건수. 무엇이 한 번에 정해졌는지 사람에게 보여 주는 값이다.
	Batched map[string]int
	Scope   string
}

// Finalize — **이 리포에서 반드시 거쳐야 하는 관문**(§3.7).
//
// 필수 항목의 결론과 승인 서명이 모두 있어야 통과한다. 통과하지 못하면 왜 안 되는지
// [Pending]이 알린다 — 무엇을 더 채워야 하는지 모르면 사람은 파일도 화면도 고칠 수 없다.
//
// **명령과 화면이 이 함수 하나를 쓴다.** 관문이 둘이면 언젠가 한쪽만 고쳐진다.
func Finalize(sf Session) (*Result, error) {
	items := make([]decision.Item, 0, len(sf.Items))
	for _, it := range sf.Items {
		items = append(items, decision.Item{ID: it.ID, Policy: it.Policy, Mandatory: it.Mandatory})
	}
	s := decision.NewSession(sf.Scope, items)
	if err := s.StartReview(); err != nil {
		return nil, err
	}

	batched := map[string]int{}
	// **정책이 먼저다**(§3.4). 개별 결론은 그 뒤에 얹혀 예외를 만든다.
	for pol, c := range sf.PolicyDecisions {
		if strings.TrimSpace(c) != "" {
			if n := s.DecidePolicy(pol, c); n > 0 {
				batched[pol] = n
			}
		}
	}
	for _, it := range sf.Items {
		if strings.TrimSpace(it.Conclusion) != "" {
			s.Decide(it.ID, it.Conclusion)
		}
	}
	if sf.Reviewer != "" || sf.Signature != "" {
		s.Sign(sf.Reviewer, sf.Signature)
	}
	if err := s.Finalize(); err != nil {
		return nil, &decision.NotFinalized{Err: err, Missing: Pending(sf)}
	}

	// **계획은 리뷰 항목과 자동통과 양쪽에서 고른다.** 판정은 위에서 Items만 봤다 - 자동통과는
	// 판정을 다시 받지 않는다. 계획에 넣는 문턱(RequireNode·RequireDecisions)은 양쪽에 같은 것 하나다.
	if err := checkNoDuplicateIDs(sf); err != nil {
		return nil, err
	}
	plan := make([]decision.PlanItem, 0)
	picked := make([]Item, 0)
	for _, it := range Selected(sf) {
		if err := RequireNode(it); err != nil {
			return nil, err
		}
		if err := RequireDecisions(it); err != nil {
			return nil, err
		}
		plan = append(plan, decision.PlanItem{
			NodeID: it.Node, RemediationClass: it.Conclusion,
			DeployAutomationLevel: it.Level,
			ProviderChoice:        decision.RouteProvider(it.Runtime, it.FIPS),
		})
		picked = append(picked, it)
	}
	// 옛 목록의 항목은 계획에 넣을 수 없다. 파일을 고쳐 넣으려 해도 ID 뿐이라 여기까지 오지 않지만,
	// 그 목록이 남아 있는 세션에서 계획을 내는 것은 사람이 「저 후보들은?」을 물을 자리라 알린다.
	if len(sf.LegacyAutopass) > 0 {
		fmt.Fprintf(os.Stderr, "note: %d auto-pass candidate(s) from an older session are listed by id only and cannot be put in a plan — "+
			"reopen the session from the results (`pqcaton-decide open -results …`) to make them selectable\n", len(sf.LegacyAutopass))
	}
	// 규칙 판은 **세션이 들고 있던 것**을 쓴다. 여기서 지금 실행 파일의 것을 찍으면, 검토 도중
	// 도구가 올라갔을 때 실제로 검토한 근거가 아닌 판이 계획에 박힌다.
	if sf.RulesetVersion == "" {
		return nil, fmt.Errorf("this session records no ruleset_version — it was raised by an older build. " +
			"raise it again from the results (`pqcaton-decide open`) rather than stamping today's rules onto " +
			"a judgement made under rules we cannot name")
	}

	// 세션 id도 **세션이 들고 있던 것**을 쓴다. 없으면 옛 빌드가 연 세션이다. 여기서 새로
	// 찍으면 그 세션의 판정 원장 행들은 세션 id가 비어 있어 계획에서 되짚을 수 없다 —
	// 계획은 세션을 가리키는데 원장에는 그 세션이 없는 상태가 된다.
	if sf.SessionID == "" {
		return nil, fmt.Errorf("this session records no session_id — it was raised by an older build. " +
			"raise it again from the results (`pqcaton-decide open`) so its judgments and its plan " +
			"share one session id; finalizing it as-is would leave a plan nothing in the ledger points back to")
	}

	// 규칙 판이 지금 도구보다 낮은 세션이다. 막지는 않는다 - 검토 중인 세션이 도구 교체로 버려지면
	// 사람이 한 일이 사라진다. 다만 그 세션의 근거 해시는 옛 규칙으로 계산됐고 서명도 그것을 덮은
	// 것이라, 무엇으로 판정됐는지를 값으로 알린다.
	if have, now := planRulesetNumber(sf.RulesetVersion), planRulesetNumber(RulesetVersion); have < now {
		fmt.Fprintf(os.Stderr, "warning: this session was judged under %s; the tool now runs %s. "+
			"Its basis hashes and signature are the old rules' — reopen it from the results to judge under the current rules\n",
			sf.RulesetVersion, RulesetVersion)
	}

	// **관문은 여기다.** 판정이 끝나지 않은 세션에서는 계획 자체가 만들어지지 않는다.
	p, err := decision.BuildPlan(s, plan)
	if err != nil {
		return nil, err
	}
	if err := decision.ReadyForApproval(p); err != nil {
		return nil, err
	}
	out, err := ToContract(p, picked, sf.RulesetVersion, sf.SessionID)
	if err != nil {
		return nil, err
	}
	return &Result{Plan: out, Decided: decidedOf(s), Batched: batched, Scope: p.Scope}, nil
}

// Pending — 무엇이 남았는지. 「안 된다」만 말하면 사람은 고칠 수 없다.
func Pending(sf Session) []decision.Missing {
	var out []decision.Missing
	if sf.Signature == "" {
		out = append(out, decision.Missing{Code: decision.MissingSignature})
	}
	for _, it := range sf.Items {
		if it.Mandatory && strings.TrimSpace(it.Conclusion) == "" &&
			strings.TrimSpace(sf.PolicyDecisions[it.Policy]) == "" {
			out = append(out, decision.Missing{
				Code: decision.MissingConclusion, Subject: it.ID, Detail: it.State,
			})
		}
	}
	// 계획 칸은 **고른 것만** 본다. 자동통과에는 결론을 요구하지 않는다.
	for _, it := range Selected(sf) {
		if err := RequireDecisions(it); err != nil {
			out = append(out, decision.Missing{Code: decision.MissingPlanField, Subject: it.ID, Detail: err.Error()})
		}
	}
	return out
}

// RequireNode — **지어내지 않고 중단한다.** node가 비면 겨눌 곳을 모르는 것이고, 빈 채로
// 내보내면 pqcota가 이름 없는 노드에 조치를 건다. v0.1.0이 낸 세션 파일이 여기 걸린다.
func RequireNode(it Item) error {
	if it.Node == "" {
		return fmt.Errorf("item %s has no node — this is a v0.1.0 format session. "+
			"run `pqcaton-decide open` again, or give the screen -results and let it raise a new one", it.ID)
	}
	return nil
}

// RequireDecisions — 계획에 넣는 항목이 **검토자가 골랐어야 할 것을 다 골랐는지** 본다.
//
// pqcaton은 승인·확정 계층이다. 여기서 비운 값을 기본값으로 채우면 사용자가 하지 않은 정책
// 결정을 도구가 대신 내리고 **그 결과에 승인 서명이 붙는다.** 서명은 조치의 내용을 덮으므로,
// 승인자는 자기가 고르지 않은 수준과 조치에 책임을 지게 된다.
//
// 상류(pqcota)도 빈칸을 이름으로 알리고 종료 상태 3으로 끝내지만, 그것은 **직접 쓴 계획을 위한
// 최후 안전장치**다. 여기서 통과시키고 거기서 걸리게 두면 계층의 역할이 뒤바뀐다.
//
// 필수의 범위는 상류의 완결성 기준을 따른다. 목표 알고리즘은 **config로 배달하는 조치**에만
// 필요하다 — 그 조치의 조각에만 `Groups` 줄이 있고, 비면 배치해도 아무것도 켜지지 않는다.
// 포크 교체나 폐기에는 적을 자리가 없으므로 요구하지 않는다.
func RequireDecisions(it Item) error {
	// **관리 대상이 아닌 자산에 조치를 세우지 않는다.** 이미 한 번 잡힌 결함이다 - 정책을 무시하던
	// 동안 뺀 자산에 조치 계획을 세우고 있었다. 화면에서 고를 수 없고, 세션 파일을 손으로 고쳐
	// 켜도 여기서 막힌다. 관리 축이 세션 파일에 있어야 이 검사가 가능하다.
	if ManagedOf(it) == reconcile.ExcludedByPolicy {
		return fmt.Errorf("item %s is excluded by the asset-scope policy — it was observed, but it is not managed, "+
			"so no action can be planned for it. Change the policy or the declaration first", it.ID)
	}
	if it.Level == "" {
		return fmt.Errorf("item %s: no deploy level — pick L1, L2 or L3. "+
			"an older session has none recorded, so it has to be chosen again", it.ID)
	}
	if _, err := levelOf(it.Level); err != nil {
		return fmt.Errorf("item %s: %w", it.ID, err)
	}
	if it.Kind == "" {
		return fmt.Errorf("item %s: no remediation kind — what to do is a review decision, not a default", it.ID)
	}
	if _, err := kindOf(it.Kind); err != nil {
		return fmt.Errorf("item %s: %w", it.ID, err)
	}
	switch it.Kind {
	case "REMEDIATION_KIND_CONFIG_ONLY", "REMEDIATION_KIND_PROVIDER_INJECT":
		if strings.TrimSpace(it.TargetAlgorithm) == "" {
			return fmt.Errorf("item %s: %s delivers its change through a config fragment, "+
				"so it needs a target_algorithm — without one the fragment turns nothing on", it.ID, it.Kind)
		}
	}
	// L3는 활성화까지 간다. 상류가 무엇이 **일어나지 않는지** 알리는 자리를 여기서 먼저 막는다:
	// activate가 없으면 조각이 놓이기만 하고 참조되지 않으며, restart가 없으면 새 provider가
	// 로드되지 않고, deactivate가 없으면 롤백이 활성화를 되돌리지 못한다.
	if strings.ToUpper(it.Level) == "L3" {
		for _, h := range []struct {
			name, cmd, why string
		}{
			{"activation.activate", it.Activate, "the fragment is placed but never referenced"},
			{"activation.restart", it.Restart, "the new provider may never be loaded"},
			{"activation.deactivate", it.Deactivate, "rollback cannot undo the activation"},
		} {
			if strings.TrimSpace(h.cmd) == "" {
				return fmt.Errorf("item %s is L3 but has no %s — %s", it.ID, h.name, h.why)
			}
		}
	}
	return nil
}

// SaveJudgments — 판정을 append-only로 남긴다.
//
// **근거 해시를 함께 적는다.** 그것이 없으면 나중에 「근거가 바뀌었나」를 물을 수 없고,
// 델타 리뷰가 성립하지 않는다(§3.6).
func SaveJudgments(path, orgName string, sf Session, decided map[string]string) (int, error) {
	store, err := decision.NewFileJudgmentStore(org.ID(orgName), path)
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	n := 0
	// **판정 행은 리뷰 항목에서만 난다.** 자동통과는 판정을 다시 받지 않는다.
	for _, it := range sf.Items {
		c, ok := decided[it.ID]
		if !ok {
			continue
		}
		j := &decision.Judgment{
			ID: fmt.Sprintf("%s@%d", it.ID, now), Subject: it.ID, Conclusion: c,
			Reviewer: sf.Reviewer, Signature: sf.Signature,
			BasisHash: BasisOf(it, sf.RulesetVersion), Confidence: it.Conf,
			ConfidenceEvaluated: ConfidenceEvaluated(it, sf.RulesetVersion), DecidedAt: now,
			SessionID: sf.SessionID, RecordKind: decision.RecordJudgment,
		}
		if err := store.Save(j); err != nil {
			return n, err
		}
		n++
	}
	// **계획 선택 행은 계획에 고른 항목마다 난다** - 리뷰 항목이든 자동통과든. 「이 자산을 이렇게
	// 판정했다」와 「이 자산을 이 계획에 넣기로 했다」는 다른 사실이라, 결론이 있는 리뷰 항목을
	// 계획에 넣으면 두 행이 다 생긴다. 결론 칸은 비운다 - 사람이 내린 결론이 없거나(자동통과),
	// 있어도 이 행의 사실이 아니다. id에 접미를 붙여 같은 초의 판정 행과 부딪히지 않게 한다.
	//
	// Reviewer·Signature는 세션에 적힌 자유 문자열이다. 이 행이 답하는 것은 「누가 넣었나」가
	// 아니라 「누가 넣었다고 기록됐나」다. 검증된 신원은 상류의 실행 승인에만 있다.
	for _, it := range Selected(sf) {
		j := &decision.Judgment{
			ID: fmt.Sprintf("%s@%d#plan", it.ID, now), Subject: it.ID,
			Reviewer: sf.Reviewer, Signature: sf.Signature,
			BasisHash: BasisOf(it, sf.RulesetVersion), Confidence: it.Conf,
			ConfidenceEvaluated: ConfidenceEvaluated(it, sf.RulesetVersion), DecidedAt: now,
			SessionID: sf.SessionID, RecordKind: decision.RecordPlanSelection,
		}
		if err := store.Save(j); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// BasisOf — 이 판정이 무엇을 보고 내려졌나. **근거를 세는 자리는 여기 하나다.**
//
// 델타 리뷰(무엇을 다시 봐야 하나)와 서명 무효화(승인이 아직 유효한가)는 같은 물음의 두
// 얼굴이다. 전에는 둘이 따로 셌다 — 원장은 상태·신뢰도·정책을 보고, 서명은 id와 상태만
// 봤다. 그래서 신뢰도가 움직이면 델타는 걸리는데 승인 서명은 그대로 살아남았다.
//
// 근거에 넣는 것:
//
//   - **규칙 판**. 같은 관측이어도 규칙이 달라지면 다른 판정이 나온다. 인자로 받는 것은
//     세션이 **열릴 때** 박은 값을 써야 하기 때문이다(지금 상수가 아니다).
//   - **대조 상태와 신뢰도, 정책**. 판정을 요구하는 이유 자체다.
//   - **관측 내용 지문**. `finding_id`는 자산 동일성이라 버전이 오르고 검출 방법이 바뀌고
//     강화 판정이 달라져도 그대로다. 지문이 없으면 그 변화를 통째로 놓친다.
//   - **재수집 후보 여부**. 「없다」와 「못 봤다」가 갈리는 자리라 결론이 달라진다.
//   - **근거 전부**. 정렬된 (finding, 관측 지문, 원천 노드) 묶음. 주 근거만 보면 보조 근거가
//     더해지거나 빠지거나 바뀌어도 판정 서명이 살아남는다. 스냅샷 지문은 넣지 않는다 — 관측
//     지문이 내용을 이미 덮고, 그것은 그 관측이 속한 스냅샷의 위치다.
//
// **관측이 그대로면 몇 번을 다시 돌려도 걸리지 않는다**(§3.6, IC-D2/D3). 그래서 지문은
// 재수집마다 흔들리는 스냅샷 id를 빼고 만든다(`reconcile.Fingerprint`).
func BasisOf(it Item, rulesetVersion string) string {
	parts := []string{
		"ruleset=" + rulesetVersion,
		"state=" + it.State,
		fmt.Sprintf("conf=%.2f", it.Conf),
		"policy=" + it.Policy,
		"evidence=" + it.Fingerprint,
		fmt.Sprintf("rescan=%t", it.Rescan),
	}
	// v3부터 관리 축과 미평가 여부도 근거다. 관리 판정이 MANAGED에서 EXCLUDED로 바뀌면 다른
	// 판정 대상이고, 미평가와 0.00을 해시가 가르지 못하면 정책이 바뀌어 평가 대상에서 빠진 것이
	// 서명을 살려 둔다. 옛 판(v2 이하)의 해시는 그대로 두어야 그 세션의 원장 행과 델타 비교가
	// 어긋나지 않는다 - 그 판의 세션에는 이 칸들이 없고, 있어도 뜻이 없다.
	if rulesetAtLeastV3(rulesetVersion) {
		parts = append(parts,
			"managed="+string(ManagedOf(it)),
			fmt.Sprintf("conf_evaluated=%t", ConfidenceEvaluated(it, rulesetVersion)))
		for _, x := range it.ExcludedSources {
			parts = append(parts, "excluded="+x.FindingID+"|"+x.Fingerprint+"|"+x.SourceNodeID)
		}
	}
	for _, s := range it.Sources {
		parts = append(parts, "source="+s.FindingID+"|"+s.Fingerprint+"|"+s.SourceNodeID)
	}
	return decision.HashBasis(parts...) // HashBasis가 정렬한다 — 근거의 순서는 근거가 아니다
}

// evidenceOf — 항목의 근거를 계약의 꼴로. 주 근거가 앞이다. 스냅샷 참조는 **내용 지문**이다 — 이
// 리포는 상류 이력의 id를 모른다. 원천 노드는 봉투의 이름이고, 규칙 판은 **스냅샷의** 것이다
// (계획의 결합 판이 아니다). 상류는 (org, 원천 노드, 규칙 판, 지문)으로 찾고, 찾은 스냅샷에 그
// finding이 실제로 있는지까지 본다. 지문이 비면(옛 세션) 참조 없이 finding만 낸다 — 상류가
// 모양이 틀렸다고 알린다. 알리지 않고 빼지 않는다.
func evidenceOf(it Item) []*provisioningv1.ActionEvidenceSource {
	out := make([]*provisioningv1.ActionEvidenceSource, 0, len(it.Sources))
	for _, s := range it.Sources {
		e := &provisioningv1.ActionEvidenceSource{FindingId: s.FindingID}
		if s.SnapshotDigest != "" {
			e.Snapshot = &provisioningv1.SnapshotReference{
				SourceNodeId: s.SourceNodeID,
				Reference: &provisioningv1.SnapshotReference_Content{Content: &provisioningv1.SnapshotContentReference{
					FormatVersion: history.SnapshotContentFormatV1, Digest: s.SnapshotDigest, RulesetVersion: s.SnapshotRuleset}},
			}
		}
		out = append(out, e)
	}
	return out
}

// ToContract — 판정이 끝난 계획을 pqcota 계약(`provisioningv1.FinalizedPlan`)으로 옮긴다.
//
// 어휘의 단일 출처는 계약이다 — 이 리포는 그 어휘로 적고 자기 형식을 새로 만들지 않는다.
//
// **판정과 실행 승인은 다른 단계다.** 이 함수가 내는 것은 판정이 끝난 계획이지 실행 근거가
// 아니다. 그래서 상태는 IN_REVIEW이고, approval_signatures와 finalized_at은 **비운다** — 둘 다
// 실행 승인의 자리라 상류의 pqcota-approve가 채운다. 전에는 판정자의 자유 문자열을
// approval_signatures에 넣고 FINALIZED를 달았는데, 그 이름표는 검증되지 않는 것이라 아무것도
// 증명하지 않으면서 상류 구조 관문의 「승인 항목 있음」을 충족하는 모양이 됐다.
//
// 계획 id는 세션 id를 담는다. 판정 원장 행도 같은 값을 들고 있어, 계획에서 원장으로 되짚는
// 식별자다. 검토자가 적은 문자열은 id에 넣지 않는다 — 전에는 그 앞 여덟 글자가 id에 들어갔다.
func ToContract(p *decision.JudgedPlan, items []Item, rulesetVersion, sessionID string) (*provisioningv1.FinalizedPlan, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("a plan needs the session id it came from — without it nothing in the ledger points back to this plan")
	}
	// 되짚을 근거 가운데 **도구가 아는 것은 도구가 채운다.** 계획 id와 규칙 판이 그렇다 —
	// 사람에게 물을 값이 아니다. 관측 스냅샷 id는 계획이 여러 노드에 걸치는데 계약의 그 칸이
	// 계획마다 하나라 비운다.
	out := &provisioningv1.FinalizedPlan{
		Id:             "pqcaton:" + p.Scope + ":" + sessionID,
		Scope:          p.Scope,
		Status:         provisioningv1.PlanStatus_PLAN_STATUS_IN_REVIEW,
		RulesetVersion: rulesetVersion,
	}
	for i, it := range items {
		kind, err := kindOf(it.Kind)
		if err != nil {
			return nil, err
		}
		lvl, err := levelOf(p.Items[i].DeployAutomationLevel)
		if err != nil {
			return nil, err
		}
		out.Actions = append(out.Actions, &provisioningv1.RemediationAction{
			Id:              fmt.Sprintf("a%d", i+1),
			TargetNodeId:    p.Items[i].NodeID,
			CryptoRuntime:   runtimeOf(it.Runtime),
			Kind:            kind,
			AutomationLevel: lvl,
			TargetAlgorithm: it.TargetAlgorithm,
			ProviderChoice:  p.Items[i].ProviderChoice,
			FindingId:       it.FindingID,
			EvidenceSources: evidenceOf(it),
			Activation:      hooksOf(it),
			ConfigArtifact:  it.Config,
			RollbackNote:    rollbackNoteOf(it, rulesetVersion),
		})
	}
	return out, nil
}

// rollbackNoteOf — 계약의 rollback_note. v3 부터는 계획 칸 그대로이고 빈 값은 빈 값이다. v2 이하의
// 세션에서만 빈 값을 결론으로 채운다 - 그 판에는 이 칸이 없었다. 판단 기준은 **세션이 들고 있는
// 규칙 판**이다. 지금 실행 파일의 상수로 보면 v3 사용자가 일부러 비운 자리에 결론이 다시 들어간다.
func rollbackNoteOf(it Item, rulesetVersion string) string {
	if it.RollbackNote != "" || rulesetAtLeastV3(rulesetVersion) {
		return it.RollbackNote
	}
	return it.Conclusion
}

// Key — 자산 식별자의 문자열 표현. 판정 원장의 대상 id가 된다.
func Key(k reconcile.AssetKey) string {
	return k.NodeID + "/" + k.Runtime + "/" + k.Component
}

// PolicyOf — 같은 정책으로 묶는 기준. 런타임과 컴포넌트 이름에서 만든다.
//
// 컴포넌트에 붙는 **버전 해시만** 뗀다(`libssl-e2f2d68a` → `libssl`). 같은 라이브러리의 여러
// 판이 한 묶음이 되는 것이 정책 단위 리뷰가 뜻하는 것이다(§3.4).
//
// **해시처럼 생긴 것만 뗀다.** 길이로만 자르면 `jca-provider-chain`의 `-chain`까지 떼어
// 이름이 다른 컴포넌트가 한 정책으로 묶인다.
func PolicyOf(k reconcile.AssetKey) string {
	c := k.Component
	if i := strings.LastIndex(c, "-"); i > 0 && isHex(c[i+1:]) {
		c = c[:i]
	}
	return k.Runtime + "/" + c
}

func isHex(s string) bool {
	if len(s) < 8 {
		return false
	}
	for _, r := range s {
		if !('0' <= r && r <= '9' || 'a' <= r && r <= 'f') {
			return false
		}
	}
	return true
}

func decidedOf(s *decision.Session) map[string]string {
	out := map[string]string{}
	for _, it := range s.Items {
		if it.Decided {
			out[it.ID] = it.Conclusion
		}
	}
	return out
}

func runtimeOf(s string) commonv1.CryptoRuntime {
	if s == "jca" {
		return commonv1.CryptoRuntime_CRYPTO_RUNTIME_JCA
	}
	return commonv1.CryptoRuntime_CRYPTO_RUNTIME_OPENSSL
}

// kindOf — **모르는 값도 빈 값도 지어내지 않고 중단한다.** 계약의 통제 어휘라 오타가 표시 없이
// UNSPECIFIED로 떨어지면 그 조치는 아무것도 하지 않고, 빈 값을 기본 조치로 채우면 사용자가
// 하지 않은 결정에 승인 서명이 붙는다.
func kindOf(s string) (provisioningv1.RemediationKind, error) {
	if s == "" {
		return 0, fmt.Errorf("no remediation_kind — what to do is a review decision, not a default")
	}
	if v, ok := provisioningv1.RemediationKind_value[s]; ok && v != 0 {
		return provisioningv1.RemediationKind(v), nil
	}
	return 0, fmt.Errorf("unknown remediation_kind: %q — must be one of the contract's REMEDIATION_KIND_*", s)
}

// levelOf — 위임 수준. **모르는 값을 L2로 삼키지 않는다.** 전에는 미지정도 오타도 모두 L2가
// 되어, 사용자가 하지 않은 정책 결정을 이 도구가 대신 내리고 그 결과에 승인 서명이 붙었다.
// 확정 전에 RequireDecisions가 막으므로 여기까지 빈 값이 오면 그것은 배선이 샌 것이다.
func levelOf(s string) (provisioningv1.DeployAutomationLevel, error) {
	switch strings.ToUpper(s) {
	case "L1":
		return provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L1_STAGE_ONLY, nil
	case "L2":
		return provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L2_STAGE_INSTALL, nil
	case "L3":
		return provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L3_FULL_AUTO, nil
	default:
		return 0, fmt.Errorf("unknown deploy level %q — it has to be L1, L2 or L3", s)
	}
}

// hooksOf — 사람이 적은 활성화 명령. **하나도 없으면 nil을 준다** — 빈 훅 묶음을 붙이면
// 상류가 「훅이 있는데 명령이 비었다」와 「훅 자체가 없다」를 가리지 못한다.
func hooksOf(it Item) *provisioningv1.ActivationHooks {
	if it.Pre == "" && it.Activate == "" && it.Deactivate == "" && it.Restart == "" {
		return nil
	}
	return &provisioningv1.ActivationHooks{
		Pre: it.Pre, Activate: it.Activate, Deactivate: it.Deactivate, Restart: it.Restart,
	}
}
