// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"strings"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

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

// kindName — 조치 종류를 고르는 자리에 보일 이름: 그 말로 풀어 쓴 뜻과 계약 어휘의 짧은 꼴.
// 「설정만 변경 (config_only)」. **값은 바꾸지 않는다.** 보내고 저장하는 것은 계약 어휘(`REMEDIATION_KIND_*`) 그대로다.
// 짧은 꼴을 함께 적는 것은 계획 파일과 04 화면이 그 꼴로 적기 때문이다. 풀어 쓴 뜻이 없는 값은 짧은 꼴만 보인다.
func kindName(l Lang, k string) string {
	if t, ok := kindNames[k]; ok {
		return t.In(l) + " (" + shortKind(k) + ")"
	}
	return shortKind(k)
}

// hasHooks — 활성화 훅 넷 가운데 적은 것이 있는가.
func hasHooks(it review.Item) bool {
	return it.Pre != "" || it.Activate != "" || it.Deactivate != "" || it.Restart != ""
}

// shortKind — 화면에 길게 늘어놓지 않는다. 값은 계약 어휘 그대로 보내고 표시만 줄인다.
func shortKind(k string) string {
	return strings.ToLower(strings.TrimPrefix(k, "REMEDIATION_KIND_"))
}
