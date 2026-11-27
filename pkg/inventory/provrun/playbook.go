// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Package provrun — 04가 플레이북을 노드에 돌릴 때 쓰는 판정. 화면도 Ansible도 모른다: 생성된 플레이북에서 기대하는 작업을
// 읽고, 실행 기록에서 노드별 상태를 정하고, 미리 보기가 낸 적용 권한의 유효 여부를 가린다.
package provrun

import (
	"regexp"
	"strings"
)

// 화면에서 돌리지 않는 작업의 까닭.
const (
	WhyProvider = "provider" // provider 모듈을 놓거나 그 무결성을 확인하는 작업. 미리 보기가 확인하지 못한다
	WhyL3       = "l3"       // 활성화 · 재시작 훅
	WhyManual   = "manual"   // 설정만으로 전달하지 못하는 조치(생성기가 주석으로 남긴다)
	WhyUnknown  = "unknown"  // 이 판이 아는 꼴이 아니다
)

// Task — 플레이북의 작업 하나. Why가 비면 화면에서 돌릴 수 있는 작업이다(디렉터리, 내용을 적어 넣는 설정 조각).
type Task struct {
	Name   string `json:"name"`
	Module string `json:"module"`
	Why    string `json:"why,omitempty"`
}

// Play — 노드 하나의 작업들. 생성기는 노드마다 play 하나를 낸다.
type Play struct {
	Node  string `json:"node"`
	Tasks []Task `json:"tasks"`
}

// Playbook — 생성된 플레이북에서 읽은 것.
type Playbook struct {
	Plays []Play `json:"plays"`
	// Manual — 수동 단계 주석이 붙은 노드(주석마다 한 번).
	Manual []string `json:"manual,omitempty"`
	// Malformed — 생성기가 내는 꼴이 아닌 줄이 있다. 그러면 아무것도 돌리지 않는다.
	Malformed bool `json:"malformed,omitempty"`
}

// Unsupported — 화면에서 돌리지 않는 것 하나.
type Unsupported struct {
	Node, Task, Why string
}

// nodeName — 인벤토리의 호스트 이름과 `hosts:`에 그대로 쓸 수 있는 노드 이름. 패턴 문자(`*` · `!` · `:` · `,` · `&`)가 들면
// Ansible이 한 노드가 아닌 것으로 읽는다.
var nodeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// NodeNameOK — 화면이 돌릴 수 있는 노드 이름인가.
func NodeNameOK(n string) bool { return nodeName.MatchString(n) }

var moduleLine = regexp.MustCompile(`^      (ansible\.builtin\.[a-z_]+):`)

// ParsePlaybook — 상류 생성기(provisioning.GenerateProvisioningPlaybook)가 내는 꼴만 읽는다. YAML을 풀이하지 않는다: 줄의
// 들여쓰기와 머리로 가리고, 모르는 줄이 하나라도 있으면 Malformed다. 부르는 쪽은 그 전에 파일이 생성기의 출력과 한 글자도
// 다르지 않은지 견준다. 여기서 읽는 것은 「무엇을 기대할지」와 「돌릴 수 있는 종류인지」뿐이다.
func ParsePlaybook(b []byte) Playbook {
	var pb Playbook
	var play *Play
	var task *Task
	hasSrc, hasContent := false, false
	closeTask := func() {
		if task == nil {
			return
		}
		switch task.Module {
		case "ansible.builtin.file":
		case "ansible.builtin.copy":
			switch {
			case hasSrc:
				task.Why = WhyProvider
			case !hasContent:
				task.Why = WhyUnknown
			}
		case "ansible.builtin.stat", "ansible.builtin.assert":
			task.Why = WhyProvider
		case "ansible.builtin.shell":
			task.Why = WhyL3
		default:
			task.Why = WhyUnknown
		}
		task, hasSrc, hasContent = nil, false, false
	}
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "        "): // 작업의 몸(모듈 인자 · 내용 · 명령)
			if task == nil || task.Module == "" {
				pb.Malformed = true
				continue
			}
			if task.Module == "ansible.builtin.copy" && !strings.HasPrefix(line, "         ") {
				hasSrc = hasSrc || strings.HasPrefix(line, "        src:")
				hasContent = hasContent || strings.HasPrefix(line, "        content:")
			}
		case strings.TrimSpace(line) == "", strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "- name: "):
			closeTask()
			pb.Plays = append(pb.Plays, Play{})
			play = &pb.Plays[len(pb.Plays)-1]
		case play == nil:
			pb.Malformed = true
		case strings.HasPrefix(line, "  hosts: [") && strings.HasSuffix(line, "]"):
			play.Node = unquote(line[len("  hosts: [") : len(line)-1])
		case line == "  become: true", line == "  tasks:":
		case strings.HasPrefix(line, "    # "):
			closeTask()
			pb.Manual = append(pb.Manual, play.Node)
		case strings.HasPrefix(line, "    - name: "):
			closeTask()
			play.Tasks = append(play.Tasks, Task{Name: unquote(strings.TrimPrefix(line, "    - name: "))})
			task = &play.Tasks[len(play.Tasks)-1]
		case moduleLine.MatchString(line):
			if task == nil || task.Module != "" {
				pb.Malformed = true
				continue
			}
			task.Module = moduleLine.FindStringSubmatch(line)[1]
		case strings.HasPrefix(line, "      register: "), strings.HasPrefix(line, "      when: "):
			if task == nil {
				pb.Malformed = true
			}
		default:
			pb.Malformed = true
		}
	}
	closeTask()
	if len(pb.Plays) == 0 {
		pb.Malformed = true
	}
	for _, p := range pb.Plays {
		if p.Node == "" || len(p.Tasks) == 0 {
			pb.Malformed = true
		}
	}
	return pb
}

// unquote — 생성기의 yamlScalar가 싼 것을 푼다(`\\` · `\"` · `\n`). 따옴표가 없으면 그대로다.
func unquote(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	var b strings.Builder
	in := s[1 : len(s)-1]
	for i := 0; i < len(in); i++ {
		if in[i] == '\\' && i+1 < len(in) {
			i++
			if in[i] == 'n' {
				b.WriteByte('\n')
			} else {
				b.WriteByte(in[i])
			}
			continue
		}
		b.WriteByte(in[i])
	}
	return b.String()
}

// Unsupported — 화면에서 돌리지 않는 작업과 수동 단계. 비고 Malformed가 아니면 이 플레이북은 화면이 돌릴 수 있는 종류다.
func (pb Playbook) Unsupported() []Unsupported {
	var out []Unsupported
	for _, p := range pb.Plays {
		if !NodeNameOK(p.Node) {
			out = append(out, Unsupported{Node: p.Node, Why: WhyUnknown})
		}
		for _, t := range p.Tasks {
			if t.Why != "" {
				out = append(out, Unsupported{Node: p.Node, Task: t.Name, Why: t.Why})
			}
		}
	}
	for _, n := range pb.Manual {
		out = append(out, Unsupported{Node: n, Why: WhyManual})
	}
	return out
}

// Nodes — 플레이북이 겨누는 노드(순서대로).
func (pb Playbook) Nodes() []string {
	var out []string
	for _, p := range pb.Plays {
		out = append(out, p.Node)
	}
	return out
}
