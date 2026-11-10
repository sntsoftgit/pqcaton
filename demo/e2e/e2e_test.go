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

// IC-U152 — **데모 종단: 상류 재적재부터 활성화까지.** demo/scripts/e2e-scope.sh가 pqcota 데모를 처음부터 띄우고 다음을 잰다.
//   - 화면이 떠 있으면 apply-scope가 상류 적재도 시작하지 않고 실패한다.
//   - 손으로 고친 후보는 상류 적재 전에 거절한다.
//   - 상류를 다시 적재하지 않으면(노드의 최신 스냅샷이 옛 정책) 확인이 거절한다.
//   - 실패 경로마다 정책 · 세션 · 계획 · 영수증의 바이트와 상류 스냅샷 수가 그대로다.
//   - 성공하면 활성 정책 · 새 세션 · 활성화 영수증의 지문이 같고, 영수증의 basis가 upstream-verified다.
//   - 06은 활성화 전 계획의 서명을 막고, 05는 지난 결론을 참고값으로만 보인다.
//
// 필요한 것(PQCOTA_DIR과 상류 다섯 리포, docker, go, python3)이 없으면 건너뛰지 않고 실패한다. 스크립트는 이번 실행만의
// dind 컨테이너 안에서 상류 데모를 띄우므로, 호스트에 pqcota 쪽 데모가 떠 있어도 이름이 부딪히지 않고 그것을 건드리지도
// 않는다. 스크립트가 `e2e-dind:` 줄로 밝힌 dind 컨테이너와 그 볼륨이 끝난 뒤(성공이든 실패든) 호스트에 남지 않아야 한다.
func TestScopeActivationEndToEnd(t *testing.T) {
	var missing []string
	dir := os.Getenv("PQCOTA_DIR")
	if dir == "" {
		missing = append(missing, "PQCOTA_DIR (the upstream pqcota v0.10.6 checkout)")
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
