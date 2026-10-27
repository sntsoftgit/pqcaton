// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"strings"
	"testing"
)

// IC-W48: macOS에서 Intel(amd64)은 검증한 지원 플랫폼이라 시작 경고가 없다. Apple Silicon(arm64)은 검증하지 않았다는
// 경고가 남는다. 네트워크 파일시스템 거부(IC-W8)는 아키텍처와 무관하게 그대로다.
func TestIntelMacIsVerifiedAppleSiliconIsNot(t *testing.T) {
	if w := darwinWarning("amd64"); w != "" {
		t.Errorf("Intel macOS still warns: %q", w)
	}
	if w := darwinWarning("arm64"); !strings.Contains(w, "not a verified platform") || !strings.Contains(w, "Apple Silicon") {
		t.Errorf("Apple Silicon does not warn: %q", w)
	}
}
