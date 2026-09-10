package ui

import "strings"

// remediationKinds — 계약의 통제 어휘(`RemediationKind`). **여기서 지어내지 않는다** — 상류
// 계약에 있는 값만 고를 수 있어야, 오타가 UNSPECIFIED로 조용히 떨어져 아무 일도 안 하는
// 조치가 되는 것을 막는다. 비워 두면 상류가 PROVIDER_INJECT로 본다.
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

// deployLevels — 위임 수준. **자산별로 고른다**(상류 규정서 §4.3). 비워 두면 상류가 실행할 때
// 준 `--level`을 쓰는데, 그 플래그는 승인 서명 밖이라 승인한 수준과 실행 수준이 갈릴 수 있다.
var deployLevels = []string{"L1", "L2", "L3"}

// shortKind — 화면에 길게 늘어놓지 않는다. 값은 계약 어휘 그대로 보내고 표시만 줄인다.
func shortKind(k string) string {
	return strings.ToLower(strings.TrimPrefix(k, "REMEDIATION_KIND_"))
}
