// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 02가 Ansible을 부르는 조건. **운영자의 ansible.cfg와 ANSIBLE_* 환경을 쓰지 않는다.**
//
// 서명 키는 노드 표준 입력으로 넘어간다. 그 길에 운영자 설정이 끼면 키가 샌다는 것을 실험으로 확인했다(pqcaton-aai
// reports/2026-11-18-v0.31.0-ansible-isolation.md): 운영자 폴더의 콜백 플러그인은 `no_log`여도 작업 인자로 키를 받았고, 설정이나
// host var로 바꾼 sudo 실행 파일은 키를 표준 입력으로 받았고, 2.16은 내장과 같은 이름의 연결 · 권한 상승 플러그인을 운영자
// 폴더에서 불러 썼다. 그래서
//
//   - 환경은 허용 목록만 물려준다(ansibleEnv). ANSIBLE_CONFIG는 실행 자리에 쓴 이 설정 하나를 가리킨다. 이때 Ansible은
//     ./ansible.cfg · ~/.ansible.cfg · /etc/ansible/ansible.cfg를 읽지 않는다.
//   - 설정은 모든 플러그인 경로와 컬렉션 경로를 빈 폴더로 둔다. 내장(ansible.builtin)만 쓴다.
//   - 연결은 인벤토리의 host var(ansible.builtin.ssh)로 고정한다. 추가 변수로 고정하면 delegate_to: localhost 작업까지 ssh로 붙어
//     결과를 컨트롤러로 옮기지 못한다. 권한 상승과 pipelining은 추가 변수(runVars)로 고정한다. 추가 변수는 host var까지 이긴다.
//   - pipelining을 켠다. 끄면 키가 든 모듈 꾸러미가 노드에 파일로 생기고, 컨트롤러가 죽으면 남는다.
//
// ssh 클라이언트 설정(~/.ssh/config)은 ssh가 읽으므로 그대로 듣는다. 그래서 키는 환경 변수로 넘기지 않는다(SendEnv가 넘긴다).

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	runConfigName  = "ansible.cfg"
	runPluginsNone = "plugins-none"
	runCheckBook   = "check.yml"
)

// ansibleConfig: 실행 자리에 쓰는 설정. dir은 실행 자리다.
func ansibleConfig(dir string) []byte {
	return ansibleConfigWith(dir, "", "ansible.builtin.default")
}

// ansibleConfigWith: callbackDir가 있으면 callback 플러그인만 그 폴더에서 읽고(05의 전용 callback), stdout은 표준 출력용
// callback의 이름이다. 나머지 플러그인 경로는 02와 같이 빈 폴더다.
func ansibleConfigWith(dir, callbackDir, stdout string) []byte {
	none := filepath.Join(dir, runPluginsNone)
	var b strings.Builder
	b.WriteString("# pqcaton: the only Ansible configuration an observation reads. Written for one run and removed when it ends.\n[defaults]\n")
	for _, k := range []string{"action_plugins", "become_plugins", "cache_plugins", "callback_plugins", "cliconf_plugins",
		"connection_plugins", "doc_fragment_plugins", "filter_plugins", "httpapi_plugins", "inventory_plugins", "library",
		"lookup_plugins", "module_utils", "netconf_plugins", "strategy_plugins", "terminal_plugins", "test_plugins", "vars_plugins",
		"collections_path"} {
		if k == "callback_plugins" && callbackDir != "" {
			b.WriteString(k + " = " + callbackDir + "\n")
			continue
		}
		b.WriteString(k + " = " + none + "\n")
	}
	// callbacks_enabled는 적지 않는다: 이 설정 밖은 읽지 않으므로 기본값(없음)이고, 2.21은 빈 값을 주면 예외로 멈춘다.
	b.WriteString(`collections_scan_sys_path = False
stdout_callback = ` + stdout + `
display_args_to_stdout = False
transport = ansible.builtin.ssh
keep_remote_files = False
verbosity = 0
retry_files_enabled = False
nocolor = True
[privilege_escalation]
become_allow_same_user = False
[connection]
pipelining = True
[ssh_connection]
pipelining = True
ssh_args = ` + sshArgs + "\n")
	return []byte(b.String())
}

// envKeep: Ansible과 ssh에 물려주는 환경 변수. 접속(ssh-agent) · 언어 · 임시 폴더만이다. ANSIBLE_* · PQCOTA_*(DSN 포함)는
// 물려주지 않는다.
var envKeep = []string{"PATH", "HOME", "USER", "LOGNAME", "LANG", "LANGUAGE", "TZ", "TMPDIR", "SSH_AUTH_SOCK",
	"OBJC_DISABLE_INITIALIZE_FORK_SAFETY"}

// envKeepPrefixes: 접두어로 물려주는 환경 변수(언어 설정). 시험 바이너리는 가짜 ansible-playbook의 변수(FAKE_)를 더한다.
var envKeepPrefixes = []string{"LC_"}

// ansibleEnv: 감독이 Ansible을 띄울 때의 환경. 허용 목록 · LC_* · 실행 표지 · 이 설정 파일.
func ansibleEnv(dir, marker string) []string {
	var out []string
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		keep := contains(envKeep, k)
		for _, p := range envKeepPrefixes {
			keep = keep || strings.HasPrefix(k, p)
		}
		if keep {
			out = append(out, e)
		}
	}
	return append(out, marker, "ANSIBLE_CONFIG="+filepath.Join(dir, runConfigName))
}

// runVars: 추가 변수(vars.json)에 싣는 실행 조건. 권한 상승은 내장 sudo, 실행 파일 · 옵션 · 사용자를 고정하고 pipelining을 켠다.
func runVars() map[string]any {
	v := map[string]any{
		"ansible_become_method": "ansible.builtin.sudo",
		"ansible_become_exe":    "sudo",
		"ansible_become_flags":  "-H -S -n",
		"ansible_become_user":   "root",
		"ansible_pipelining":    true,
	}
	if p, err := exec.LookPath("ssh"); err == nil {
		v["ansible_ssh_executable"] = p
	}
	return v
}

// writeRunConfig: 설정과 빈 플러그인 폴더를 실행 자리에 둔다. 정리 플레이북만 다시 돌릴 때(복구)도 쓴다.
func writeRunConfig(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, runPluginsNone), 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, runConfigName), ansibleConfig(dir), 0o600)
}

// provAnsibleCore: 05가 원격 실행(미리 보기 · 적용 · 재확인)을 허용하는 ansible-core 판. **하나뿐이다.**
//
// 05는 「이 노드에는 실행하지 않았다」고 말한다. 그 근거는 Ansible 안의 순서다: 작업을 알린 뒤(callback이 그 줄을 디스크에
// 내린 뒤)에야 그 작업을 노드에 보낸다. 이 순서를 확인한 판이 2.16.3이다(pqcaton-aai
// reports/2026-11-26-provisioning-apply-path.md). 다른 판을 더할 때는 그 판에서 callback 호출 순서와 기록 중단 시의 동작을
// 확인한 뒤에 넓힌다. 02 관측의 지원 범위는 이것과 무관하다.
const provAnsibleCore = "2.16.3"

var coreVersionLine = regexp.MustCompile(`^ansible-playbook \[core ([0-9][0-9A-Za-z.+-]*)\]`)

// ansibleCoreVersion: 그 `ansible-playbook`이 스스로 밝히는 core 판(`--version`의 첫 줄). 읽지 못하면 빈 값이다. 운영자의
// ANSIBLE_* 환경은 넘기지 않는다(ansibleEnv와 같은 허용 목록).
var ansibleCoreVersion = func(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Dir = os.TempDir()
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		keep := contains(envKeep, k)
		for _, p := range envKeepPrefixes {
			keep = keep || strings.HasPrefix(k, p)
		}
		if keep {
			cmd.Env = append(cmd.Env, e)
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(out), "\n")
	if m := coreVersionLine.FindStringSubmatch(strings.TrimSpace(first)); m != nil {
		return m[1]
	}
	return ""
}
