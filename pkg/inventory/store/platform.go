// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// darwinWarning: macOS에서 상태 디렉토리를 열 때 남길 경고. Intel(amd64)은 macOS 26.6.2의 로컬 파일시스템에서
// 저장 · 확정 · 복구를 검증한 지원 플랫폼이라 경고하지 않는다(v0.23.1). Apple Silicon(arm64)과 그 밖의 아키텍처는
// 돌지만 검증하지 않았다. 빌드 태그 없이 두어 어느 플랫폼에서든 시험한다.
func darwinWarning(arch string) string {
	if arch == "amd64" {
		return ""
	}
	return "macOS on " + arch + " (Apple Silicon) is not a verified platform for saving; Linux and Intel macOS on a local file system are"
}
