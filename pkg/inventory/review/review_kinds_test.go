package review_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ★ 화면이 고르게 하는 조치 종류는 **계약에 있는 값이어야 한다.**
//
// 어휘의 단일 출처는 상류 계약이다. 화면에서 지어낸 값은 상류가 통제 어휘로 받지 않아 거절되고,
// 사용자는 화면이 준 선택지 때문에 거절당한다. 실제로 초안에서 `PROXY_TERMINATE`를 넣었는데
// 계약에는 `PROXY_FRONT`가 있었다. 눈으로는 그럴듯해서 걸리지 않는 종류의 오타다.
func TestRemediationKindsComeFromTheContract(t *testing.T) {
	raw, err := os.ReadFile("../ui/helpers_review.go")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, m := range regexp.MustCompile(`REMEDIATION_KIND_[A-Z_]+`).FindAllString(contractProto(t), -1) {
		want[m] = true
	}
	if len(want) == 0 {
		t.Skip("상류 계약 proto를 찾지 못했다 — 모듈 캐시에만 있으면 건너뛴다")
	}
	for _, k := range regexp.MustCompile(`"(REMEDIATION_KIND_[A-Z_]+)"`).FindAllStringSubmatch(string(raw), -1) {
		if !want[k[1]] {
			t.Errorf("화면이 계약에 없는 조치 종류를 고르게 한다: %s", k[1])
		}
	}
}

// contractProto — 상류 plan.proto를 읽는다. 나란히 체크아웃돼 있을 때만 본다.
func contractProto(t *testing.T) string {
	t.Helper()
	for _, p := range []string{
		"../../../../pqcota/contracts/proto/pqcota/provisioning/v1/plan.proto",
	} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	return strings.Join(nil, "")
}
