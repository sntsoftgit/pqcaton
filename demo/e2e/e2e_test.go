// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build e2edemo

package e2e

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// IC-U152 — **데모 종단: pqcota 재적재부터 두 길의 활성화, 06의 서명 · 생성까지.** demo/scripts/e2e-scope.sh가 pqcota 데모를
// 처음부터 띄우고 다음을 잰다.
//   - pqcota를 새 정책으로 다시 적재하기 전에는 화면(05)의 활성화가 `upstream-missing`으로 막힌다. 한 노드의 관리 자산을
//     0개로 만든 정책이라 그 노드도 대조 대상이고 반드시 거절한다. 활성화 상자에 운영자 진술 칸이 없다.
//   - 명령도 재적재 전 · `-confirm-ingest` · PQCOTA_DSN 없음 · 손으로 고친 후보를 모두 거절한다.
//   - 실패 경로마다 정책 · 세션 · 계획 · 처리 기록의 바이트와 pqcota 스냅샷 수가 그대로다. apply-scope는 재적재만 하고 작업
//     폴더를 바꾸지 않는다.
//   - 재적재 뒤 작업 폴더 복사본에서 명령으로, 작업 폴더에서 화면으로 활성화한다. 둘 다 활성 정책 · 새 세션 · 활성화 처리
//     기록의 지문이 같고 basis가 upstream-verified이며, 보고서에 upstream_check가 있고 DSN은 기록에 없다.
//   - 06은 활성화 전 계획의 서명을 막고, 05는 지난 결론을 참고값으로만, 회차의 근거를 pqcota 이력 대조로 보인다.
//   - 활성화 뒤 새 회차를 확정한 계획은 06 화면 요청으로 서명되고(FINALIZED · 승인 하나) 플레이북 둘이 만들어진다.
//   - v0.25.0 태그로 빌드한 옛 바이너리는 작업 폴더 복사본에서 시작하지 않고(상태 문서가 형식 2) 아무것도 바꾸지 않는다.
//
// 필요한 것(PQCOTA_DIR과 상류 다섯 리포, docker, go, python3)이 없으면 건너뛰지 않고 실패한다. 상류 다섯 리포가 go.mod가
// 고정한 판의 태그에 있지 않으면 실패한다. 스크립트는 이번 실행만의
// dind 컨테이너 안에서 상류 데모를 띄우므로, 호스트에 pqcota 쪽 데모가 떠 있어도 이름이 부딪히지 않고 그것을 건드리지도
// 않는다. 스크립트가 `e2e-dind:` 줄로 밝힌 dind 컨테이너와 그 볼륨이 끝난 뒤(성공이든 실패든) 호스트에 남지 않아야 한다.
func TestScopeActivationEndToEnd(t *testing.T) {
	var missing []string
	dir := os.Getenv("PQCOTA_DIR")
	if dir == "" {
		missing = append(missing, "PQCOTA_DIR (the upstream pqcota checkout at the version go.mod pins)")
	} else {
		if _, err := os.Stat(filepath.Join(dir, "demo", "scripts", "up.sh")); err != nil {
			missing = append(missing, "PQCOTA_DIR/demo/scripts/up.sh")
		}
		for _, sib := range []string{"pqcota-common", "pqcota-inventory", "pqcota-discovery", "pqcota-provisioning"} {
			if _, err := os.Stat(filepath.Join(dir, "..", sib)); err != nil {
				missing = append(missing, "the upstream repository "+sib+" next to PQCOTA_DIR")
			}
		}
	}
	for _, prog := range []string{"docker", "go", "python3", "bash"} {
		if _, err := exec.LookPath(prog); err != nil {
			missing = append(missing, prog)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("the demo end-to-end check cannot run; missing: %s", strings.Join(missing, ", "))
	}
	// 상류 다섯 리포는 go.mod가 고정한 판의 태그에 있어야 한다. 다른 판으로 돌린 데모는 이 판을 검증한 것이 아니다.
	want := pinnedUpstream(t)
	for _, r := range []string{"pqcota", "pqcota-common", "pqcota-inventory", "pqcota-discovery", "pqcota-provisioning"} {
		out, err := exec.Command("git", "-C", filepath.Join(dir, "..", r), "describe", "--tags", "--exact-match", "HEAD").Output()
		if got := strings.TrimSpace(string(out)); err != nil || got != want {
			t.Fatalf("the upstream repository %s is at %q, not at the tag %s that go.mod pins (git describe: %v)", r, got, want, err)
		}
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("docker is not usable: %v\n%s", err, out)
	}
	if err := exec.Command("docker", "image", "inspect", "docker:dind").Run(); err != nil {
		missing = append(missing, "the docker:dind image (docker pull docker:dind)")
		t.Fatalf("the demo end-to-end check cannot run; missing: %s", strings.Join(missing, ", "))
	}
	script, err := filepath.Abs(filepath.Join("..", "scripts", "e2e-scope.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := exec.Command("bash", script)
	cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	cmd.Stderr = io.MultiWriter(os.Stderr, &out)
	cmd.Env = append(os.Environ(), "PQCOTA_DIR="+dir)
	runErr := cmd.Run()

	// 끝난 뒤(성공이든 실패든) 이번 실행의 dind 컨테이너와 볼륨이 남지 않는다.
	id, vol := dindOf(out.String())
	if id == "" {
		t.Errorf("the script did not report its dind container (no e2e-dind: line)")
	} else {
		if exec.Command("docker", "container", "inspect", id).Run() == nil {
			t.Errorf("the dind container %s is left behind", id)
		}
		if vol != "" && exec.Command("docker", "volume", "inspect", vol).Run() == nil {
			t.Errorf("the dind volume %s is left behind", vol)
		}
	}
	if runErr != nil {
		t.Fatalf("e2e-scope.sh failed: %v", runErr)
	}
}

// dindOf: 스크립트가 밝힌 dind 컨테이너 ID와 볼륨 이름(`e2e-dind: container <id> volume <name>`). 없으면 빈 문자열이다.
func dindOf(out string) (id, vol string) {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) >= 3 && f[0] == "e2e-dind:" && f[1] == "container" {
			id = f[2]
			if len(f) >= 5 && f[3] == "volume" {
				vol = f[4]
			}
		}
	}
	return id, vol
}

// pinnedUpstream: go.mod가 고정한 pqcota 계열의 판(pqcota-common의 판). 모듈끼리 같은 판인지는 판별 표 시험이 잰다.
func pinnedUpstream(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && f[0] == "github.com/randyinthedev-hash/pqcota-common" {
			return f[1]
		}
	}
	t.Fatal("go.mod pins no github.com/randyinthedev-hash/pqcota-common")
	return ""
}
