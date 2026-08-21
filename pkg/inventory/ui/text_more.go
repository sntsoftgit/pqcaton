package ui

import (
	"fmt"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
)

// 관측 계층 이름의 한국어. **영어는 `report` 가 갖는다.**
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

// 세션을 세우며 나온 경고의 한국어. **영어는 `review` 가 갖는다.**
func warningText(l Lang, w review.Warning) string {
	if l != KO {
		return w.English()
	}
	switch w.Code {
	case review.WarnDeclProblems:
		return fmt.Sprintf("선언에 맞지 않는 자리 %d곳 — 그대로 두면 대조 결과가 오류 없이 "+
			"틀립니다. 어느 자리인지는 `pqcaton-report` 가 말합니다", w.Count)
	case review.WarnUnreadableResult:
		return "건너뜀(읽을 수 없음): " + w.Detail
	}
	return w.English()
}

// Warnings — 경고들을 그 말로. 부르는 쪽이 Page.Warnings 에 넣는다.
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
