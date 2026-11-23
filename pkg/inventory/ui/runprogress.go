// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

// 관측 실행의 진행: 지금 어느 단계인가, 어느 노드가 시작했고 끝났나.
//
// **근거는 실행 로그 하나다.** 화면은 Ansible의 출력을 그대로 받아 두므로(RunStatus.Log), 그 안의 `PLAY [...]` ·
// `TASK [...]` 줄과 노드별 결과 줄(`ok: [n1]` · `changed: [n2]` · `fatal: [n3]`)에서 읽는다. 추측으로 채우지 않는다:
// 단계 이름을 모르는 줄이면 단계를 비우고, 노드는 그 노드의 시작 · 완료 · 실패를 말하는 줄이 있을 때만 싣는다. 로그의
// 앞부분을 버렸으면(RunStatus.Dropped) 그만큼 근거도 없다.
//
// 플레이북의 단계 이름이 바뀌면 여기와 어긋난다. 임베드된 플레이북 셋의 모든 단계가 어느 단계로 읽히는지는 시험이 잰다
// (inventory/cmd/pqcaton-ui의 IC-U226).

import (
	"regexp"
	"strings"
)

// RunPhase — 관측 실행의 단계. 순서가 진행 순서다(runPhases).
type RunPhase string

const (
	PhaseCheck   RunPhase = "check"   // 노드 접속과 권한 점검(observe-check.yml)
	PhaseStaging RunPhase = "staging" // 수집기를 노드에 올린다
	PhaseOpenSSL RunPhase = "openssl" // OpenSSL 런타임 관측
	PhaseJCA     RunPhase = "jca"     // JCA 프로바이더 관측
	PhaseNetwork RunPhase = "network" // TLS · SSH 핸드셰이크 관측(관측 구간만큼 기다린다)
	PhaseCleanup RunPhase = "cleanup" // 노드의 스테이징 폴더 정리
)

var runPhases = []RunPhase{PhaseCheck, PhaseStaging, PhaseOpenSSL, PhaseJCA, PhaseNetwork, PhaseCleanup}

// 플레이의 이름(observe-check.yml · observe.yml · observe-cleanup.yml의 `- name:`).
const (
	playCheck   = "pqcaton observation check"
	playObserve = "pqcaton observation"
	playCleanup = "pqcaton observation cleanup"
)

// 노드에서 일이 시작되고 끝나는 단계. 이 둘의 결과 줄이 노드별 상태의 근거다.
const (
	taskNodeStart = "Make the staging directory"
	taskNodeEnd   = "Remove the staging directory"
)

// PhaseOfTask — 플레이와 단계 이름이 어느 단계인가. 모르는 이름이면 빈 값이다.
func PhaseOfTask(play, task string) RunPhase {
	switch play {
	case playCheck:
		return PhaseCheck
	case playCleanup:
		return PhaseCleanup
	case playObserve:
	default:
		return ""
	}
	switch {
	case task == taskNodeEnd:
		return PhaseCleanup
	case strings.Contains(task, "OpenSSL"):
		return PhaseOpenSSL
	case strings.Contains(task, "JCA"):
		return PhaseJCA
	case strings.Contains(task, "network") || strings.Contains(task, "TLS and SSH"):
		return PhaseNetwork
	}
	switch task {
	case "Gathering Facts", "Check the collectors for this node's architecture", "Stop here when there are no collectors for it",
		taskNodeStart, "Copy the collectors", "Look for JVMs", "Copy the JVM attach agent where a JVM runs":
		return PhaseStaging
	}
	return ""
}

var (
	playLine   = regexp.MustCompile(`^PLAY \[(.*)\] \*+\s*$`)
	taskLine   = regexp.MustCompile(`^TASK \[(.*)\] \*+\s*$`)
	resultLine = regexp.MustCompile(`^(ok|changed|fatal|failed|unreachable): \[([A-Za-z0-9_.-]+)(?: -> [^\]]*)?\]`)
)

// RunNode — 이 실행의 노드 하나: 인벤토리의 별칭(로그에 찍히는 이름)과 노드 ID.
type RunNode struct {
	Alias, Node string
}

// 노드의 상태. 근거가 있는 것만 있다.
const (
	NodeStarted = "started" // 노드에서 수집을 시작했다(스테이징 폴더를 만들었다)
	NodeDone    = "done"    // 노드에서 끝났다(스테이징 폴더를 지웠다)
	NodeFailed  = "failed"  // 그 노드의 단계가 실패했거나 노드에 닿지 못했다
)

// NodeProgress — 근거가 확인된 노드 하나의 상태.
type NodeProgress struct {
	Node  string
	State string
}

// Text — 「web-gw: 수집 중」. over는 실행이 더 돌지 않는다는 뜻이다(마무리 중이거나 끝났다). 그때 시작만 하고 끝난 줄이
// 없는 노드는 「수집 중」이 아니라 「끝나지 않음」이다.
func (n NodeProgress) Text(l Lang, over bool) string {
	switch {
	case n.State == NodeDone:
		return n.Node + ": " + tNodeDone.In(l)
	case n.State == NodeFailed:
		return n.Node + ": " + tNodeFailed.In(l)
	case over:
		return n.Node + ": " + tNodeUnfinished.In(l)
	}
	return n.Node + ": " + tNodeStarted.In(l)
}

// RunProgress — 로그에서 읽은 진행.
type RunProgress struct {
	// Phase — 마지막으로 시작한 단계. 읽지 못했으면 빈 값이다.
	Phase RunPhase
	// Reached — 점검 · 관측 플레이에서 마지막으로 시작한 단계. 실패하거나 멈춘 실행이 어디까지 갔는지다. 그 뒤에 도는 정리
	// 플레이(observe-cleanup.yml)는 세지 않는다: 정리는 멈춘 뒤의 일이라, 그것을 「멈춘 단계」로 적으면 틀린다.
	Reached RunPhase
	// Nodes — 시작 · 완료 · 실패의 근거가 있는 노드. 실행의 노드 순서를 따른다. 근거가 없는 노드는 싣지 않는다.
	Nodes []NodeProgress
}

// Progress — 이 실행의 로그에서 진행을 읽는다.
func (s RunStatus) Progress() RunProgress {
	var (
		p          RunProgress
		play, task string
		state      = map[string]string{}
	)
	for _, line := range strings.Split(s.Log, "\n") {
		if m := playLine.FindStringSubmatch(line); m != nil {
			play, task = m[1], ""
			if ph := PhaseOfTask(play, ""); ph != "" {
				p.Phase = ph
				if play != playCleanup {
					p.Reached = ph
				}
			}
			continue
		}
		if m := taskLine.FindStringSubmatch(line); m != nil {
			task = m[1]
			if ph := PhaseOfTask(play, task); ph != "" {
				p.Phase = ph
				if play != playCleanup {
					p.Reached = ph
				}
			}
			continue
		}
		m := resultLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		kind, alias := m[1], m[2]
		switch {
		case kind == "fatal" || kind == "failed" || kind == "unreachable":
			state[alias] = NodeFailed
		case play == playObserve && task == taskNodeEnd && state[alias] != NodeFailed:
			state[alias] = NodeDone
		case play == playObserve && task == taskNodeStart && state[alias] == "":
			state[alias] = NodeStarted
		}
	}
	for _, n := range s.Nodes {
		if st := state[n.Alias]; st != "" {
			p.Nodes = append(p.Nodes, NodeProgress{Node: n.Node, State: st})
		}
	}
	return p
}

// PhaseText — 도는 실행의 지금 단계: 「지금 단계 3/6: OpenSSL 런타임 관측」. 단계를 읽지 못했으면 빈 값이다.
func (s RunStatus) PhaseText(l Lang) string {
	ph := s.Progress().Phase
	for i, x := range runPhases {
		if x != ph {
			continue
		}
		name := phaseNames[ph].In(l)
		if ph == PhaseNetwork && s.Window > 0 {
			name = tPhaseNetworkWindow.F(l, s.Window)
		}
		return tPhaseOf.F(l, i+1, len(runPhases), name)
	}
	return ""
}

// ReachedText — 실패하거나 멈춘 실행이 어디까지 갔는가: 「멈춘 단계: 5/6 네트워크 핸드셰이크 관측」. 읽지 못했으면 빈 값이다.
func (s RunStatus) ReachedText(l Lang) string {
	ph := s.Progress().Reached
	for i, x := range runPhases {
		if x == ph {
			return tRunStoppedAt.F(l, i+1, len(runPhases), phaseNames[ph].In(l))
		}
	}
	return ""
}

// timeWithZone — 시각과 그 시간대. 화면이 도는 기계의 시간대다(운영자의 브라우저와 다를 수 있으므로 밝혀 적는다).
const timeWithZone = "2006-01-02 15:04:05 MST"

// StartedText · EndedText — 시작 · 종료 시각(시간대 포함). 없으면 빈 값이다.
func (s RunStatus) StartedText() string {
	if s.Started.IsZero() {
		return ""
	}
	return s.Started.Format(timeWithZone)
}

func (s RunStatus) EndedText() string {
	if s.Ended.IsZero() {
		return ""
	}
	return s.Ended.Format(timeWithZone)
}

// Over — 끝난 실행인가(돌거나 마무리하는 중이 아니다).
func (s RunStatus) Over() bool { return s.State != RunRunning && s.State != RunFinishing }

// LogLines — 화면에 든 로그의 줄 수.
func (s RunStatus) LogLines() int {
	if s.Log == "" {
		return 0
	}
	return strings.Count(s.Log, "\n") + 1
}
