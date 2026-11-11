// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review_test

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
)

// ★ 화면이 고르게 하는 조치 종류는 **계약에 있는 값이어야 한다.**
//
// 어휘의 단일 출처는 상류 계약이다. 화면에서 지어낸 값은 상류가 통제 어휘로 받지 않아 거절되고,
// 사용자는 화면이 준 선택지 때문에 거절당한다. 실제로 초안에서 `PROXY_TERMINATE`를 넣었는데
// 계약에는 `PROXY_FRONT`가 있었다. 눈으로는 그럴듯해서 걸리지 않는 종류의 오타다.
//
// 계약은 **링크된 생성 패키지의 enum descriptor**에서 읽는다. 예전에는 나란히 체크아웃된 상류의
// plan.proto를 경로로 찾았는데, 상류가 리포를 나눈 뒤로 그 경로가 없어져 이 시험이 늘 건너뛰었고
// 아무도 몰랐다. descriptor는 빌드가 실제로 쓰는 계약이라 모듈 캐시 · 체크아웃 위치 · replace와
// 상관없이 같은 것을 본다. 그래서 어느 쪽 목록이든 비면 건너뛰지 않고 실패한다.
func TestRemediationKindsComeFromTheContract(t *testing.T) {
	values := provisioningv1.RemediationKind(0).Descriptor().Values()
	want := map[string]bool{}
	for i := 0; i < values.Len(); i++ {
		if v := values.Get(i); v.Number() != 0 {
			want[string(v.Name())] = true
		}
	}
	if len(want) == 0 {
		t.Fatal("the contract enum RemediationKind has no values besides UNSPECIFIED")
	}
	t.Logf("contract: %s from %s (%s), %d kinds",
		values.Get(0).Parent().FullName(), values.Get(0).ParentFile().Path(), contractModule(), len(want))

	raw, err := os.ReadFile("../ui/helpers_review.go")
	if err != nil {
		t.Fatal(err)
	}
	offered := regexp.MustCompile(`"(REMEDIATION_KIND_[A-Z_]+)"`).FindAllStringSubmatch(string(raw), -1)
	if len(offered) == 0 {
		t.Fatal("found no remediation kinds in ../ui/helpers_review.go")
	}
	for _, k := range offered {
		if !want[k[1]] {
			t.Errorf("화면이 계약에 없는 조치 종류를 고르게 한다: %s", k[1])
		}
	}
	t.Logf("screen offers %d kinds, all in the contract", len(offered))
}

// contractModule — 계약 패키지를 담은 모듈의 경로와 판(replace가 있으면 그것까지). 로그로 무엇과
// 대조했는지 남긴다. 시험 바이너리에는 의존 모듈 정보가 실리지 않아 go 명령에 묻는다. 판을 못 읽어도
// 대조 자체는 descriptor로 이미 했으므로 실패로 보지 않고 그 사정을 적는다.
func contractModule() string {
	out, err := exec.Command("go", "list", "-m", "-f",
		"{{.Path}} {{.Version}}{{with .Replace}} => {{.Path}} {{.Version}}{{end}}",
		"github.com/randyinthedev-hash/pqcota-common").Output()
	if err != nil {
		return "module version unknown: " + err.Error()
	}
	return strings.TrimSpace(string(out))
}
