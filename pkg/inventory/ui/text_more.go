// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"fmt"
	"strings"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

// 관측 계층 이름의 한국어. **영어는 `report`에 있다.**
var koLayer = map[string]string{
	"COLLECTION_LAYER_SOURCE":            "소스 코드(SOURCE)",
	"COLLECTION_LAYER_ARTIFACT":          "빌드 산출물(바이너리·패키지, ARTIFACT)",
	"COLLECTION_LAYER_PROCESS":           "돌고 있는 프로세스(PROCESS)",
	"COLLECTION_LAYER_NETWORK":           "실제 통신(NETWORK)",
	"COLLECTION_LAYER_JVM_INTROSPECTION": "JVM 내부(JCA, JVM_INTROSPECTION)",
}

func layerLabel(l Lang, name string) string {
	if l == KO {
		if t, ok := koLayer[name]; ok {
			return t
		}
	}
	return report.LayerLabel(name)
}

// layerName — 계층 이름. 사라진 규칙이 묶이는 자리만 코드라 옮긴다.
func layerName(l Lang, name string) string {
	if l == KO && name == scope.LayerRemoved {
		return "(제거)"
	}
	return name
}

// 세션을 세우며 나온 경고의 한국어. **영어는 `review`에 있다.**
func warningText(l Lang, w review.Warning) string {
	if l != KO {
		// 명령의 영어는 자리를 `pqcaton-report`로 보낸다. 화면은 01 선언 화면이 그 자리를 보인다.
		if w.Code == review.WarnDeclProblems {
			return fmt.Sprintf("%d places where the declaration does not add up. Left as is, reconciliation "+
				"is wrong without any error. The 01 declaration screen shows where", w.Count)
		}
		return w.English()
	}
	switch w.Code {
	case review.WarnDeclProblems:
		return fmt.Sprintf("선언에 앞뒤가 맞지 않는 자리가 %d곳 있습니다. 그대로 두면 대조 결과가 오류 없이 "+
			"틀립니다. 어느 자리인지는 01 선언 화면이 보여 줍니다", w.Count)
	case review.WarnUnreadableResult:
		return "건너뜀(읽을 수 없음): " + w.Detail
	}
	return w.English()
}

// Warnings — 경고들을 그 말로. 부르는 쪽이 Page.Warnings에 넣는다.
func Warnings(l Lang, ws []review.Warning) []string {
	if len(ws) == 0 {
		return nil
	}
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, warningText(l, w))
	}
	return out
}

// 선언 문제의 한국어. **영어는 `decl`에 있다**(Problem.What · Problem.Why).
var (
	declWhatKO = map[decl.Code]string{
		decl.NodeNotInScope:  "이 노드는 관리 대상이 아닙니다",
		decl.NodeHasNoIP:     "이 노드에 IP가 없습니다",
		decl.IPMalformed:     "IP 주소가 아닙니다: %s",
		decl.IPClaimedTwice:  "여러 노드가 같은 IP(%s)를 적었습니다",
		decl.ObservedAsTwice: "여러 노드가 같은 관측 이름(%s)을 적었습니다",
		decl.NodeMissingIP:   "IP 표에 없습니다",
		decl.AssetOffScope:   "관리 대상이 아닌 노드를 가리킵니다",
		decl.EdgeSrcOffScope: "보내는 쪽 노드가 관리 대상이 아닙니다",
		decl.EdgePortZero:    "포트가 0입니다",
	}
	declWhyKO = map[decl.Code]string{
		decl.NodeNotInScope: "대조하지 않는 노드라서 이 IP 줄은 쓰이지 않습니다",
		decl.NodeHasNoIP: "이 노드로 가는 관측 통신을 노드에 이을 수 없습니다. 그래서 선언한 엣지는 UNOBSERVED로, " +
			"관측된 엣지는 UNDECLARED로 나옵니다",
		decl.IPMalformed:    "IP는 글자가 그대로 같아야 이어집니다. 포트나 호스트명을 붙이면 맞지 않습니다",
		decl.IPClaimedTwice: "나중에 적은 노드로 이어지므로, 통신이 다른 노드에 잘못 붙습니다",
		decl.ObservedAsTwice: "한 노드만 그 이름을 차지하므로, 그 기계의 자산이 다른 노드에 잘못 붙습니다. 다른 노드에 " +
			"선언한 자산은 UNOBSERVED로 남습니다",
		decl.NodeMissingIP: "이 노드로 가는 관측 통신을 노드에 이을 수 없습니다. 그래서 선언한 엣지는 UNOBSERVED로, " +
			"관측된 엣지는 UNDECLARED로 나옵니다",
		decl.AssetOffScope:   "선언은 했지만 관측이 붙지 않으므로 계속 UNOBSERVED로 남습니다",
		decl.EdgeSrcOffScope: "관측이 붙지 않으므로 계속 UNOBSERVED로 남습니다",
		decl.EdgePortZero:    "포트까지 같아야 관측된 엣지와 맞으므로, 포트가 0이면 맞는 엣지가 없습니다",
	}
)

// declProblemText: 선언 문제 하나를 그 말로. 영어 문장의 강조 표시(**)는 화면에서 뺀다.
func declProblemText(l Lang, p decl.Problem) (what, why string) {
	if l != KO {
		return p.What(), strings.ReplaceAll(p.Why(), "**", "")
	}
	what = declWhatKO[p.Code]
	if strings.Contains(what, "%s") {
		what = fmt.Sprintf(what, p.Detail)
	}
	return what, declWhyKO[p.Code]
}
