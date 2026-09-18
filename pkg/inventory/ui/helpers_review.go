package ui

import "strings"

// remediationKinds — 계약의 통제 어휘(`RemediationKind`). **여기서 지어내지 않는다** — 상류
// 계약에 있는 값만 고를 수 있어야, 오타가 UNSPECIFIED로 표시 없이 떨어져 아무 일도 안 하는
// 조치가 되는 것을 막는다. 비워 두면 확정이 막히고(RequireDecisions), 상류도 UNSPECIFIED를 거절한다.
var remediationKinds = []string{
	"REMEDIATION_KIND_CONFIG_ONLY",
	"REMEDIATION_KIND_PROVIDER_INJECT",
	"REMEDIATION_KIND_JDK_UPGRADE",
	"REMEDIATION_KIND_FORK_REPLACE",
	"REMEDIATION_KIND_PROXY_FRONT",
	"REMEDIATION_KIND_APP_RECONFIG",
	"REMEDIATION_KIND_REBUILD",
	"REMEDIATION_KIND_DECOMMISSION",
}

// deployLevels — 위임 수준. **자산별로 고른다**(상류 규정서 §4.3). 계획에 넣는 조치는 반드시 골라야
// 한다 — 비면 확정이 막힌다(review.RequireDecisions). 상류에는 미지정을 실행할 때 준 `--level`로 채우는
// 경로가 있지만 그 플래그는 승인 서명 밖이라, pqcaton은 확정 전에 미지정을 거부한다.
var deployLevels = []string{"L1", "L2", "L3"}

// shortKind — 화면에 길게 늘어놓지 않는다. 값은 계약 어휘 그대로 보내고 표시만 줄인다.
func shortKind(k string) string {
	return strings.ToLower(strings.TrimPrefix(k, "REMEDIATION_KIND_"))
}
