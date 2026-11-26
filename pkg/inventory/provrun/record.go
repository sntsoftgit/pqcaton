// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package provrun

import (
	_ "embed"
	"encoding/json"
	"strings"
	"time"
)

// CallbackPlugin — 실행 자리에 놓는 전용 callback. 표준 출력용이라 불러오지 못하면 Ansible이 첫 작업 전에 멈춘다.
//
//go:embed callback/pqcaton_events.py
var CallbackPlugin []byte

// CallbackName — 설정의 stdout_callback에 적는 이름. 파일 이름도 이것이다.
const CallbackName = "pqcaton_events"

// GatherTask — play마다 Ansible이 먼저 돌리는 작업의 이름. 읽기만 한다.
const GatherTask = "Gathering Facts"

// 감독이 실행을 끝낸 까닭. 비면 Ansible이 스스로 끝났다.
const (
	EndStall    = "stall"     // 어떤 노드가 작업을 시작한 뒤 상한 동안 결과가 없었다
	EndStopped  = "stopped"   // 운영자가 중지했다
	EndOrphaned = "orphaned"  // 화면이 사라졌다
	EndNotReady = "not-ready" // callback의 준비 신호를 받지 못해 원격 작업을 허용하지 않았다
	EndLost     = "lost"      // 감독이 결과를 남기지 못하고 끝났다(다음 시작이 적는다)
	EndRecord   = "record"    // callback이 기록을 더 쓰지 못해 실행을 끝냈다(ExitRecord)
	EndNoGrant  = "no-grant"  // 적용 권한을 소비하지 못해 원격 작업을 허용하지 않았다(없음 · 이미 쓰임 · 만료 · 입력이 다름)
)

// ExitRecord — callback이 기록을 쓰지 못해 Ansible을 끝낼 때의 종료 코드. 감독이 이것을 EndRecord로 적는다.
const ExitRecord = 251

// Result — 감독이 실행 끝에 적는 것.
type Result struct {
	Exit   int       `json:"exit"`
	Reason string    `json:"reason,omitempty"`
	Ended  time.Time `json:"ended"`
}

// 작업 하나의 결과.
const (
	TaskOK          = "ok"
	TaskChanged     = "changed"
	TaskFailed      = "failed"
	TaskSkipped     = "skipped"
	TaskUnreachable = "unreachable"
	TaskNoResult    = "no-result"   // 시작은 기록됐고 결과는 없다
	TaskNotStarted  = "not-started" // 시작이 기록되지 않았다
)

// NodeState — 실행 기록이 말하는 노드의 상태. 재확인 결과로는 바뀌지 않는다.
type NodeState string

const (
	NodeNotRun      NodeState = "not-run"     // 이 실행이 이 노드에 아무 작업도 시작하지 않았다
	NodeDone        NodeState = "done"        // 모든 작업의 성공이 기록됐다
	NodePartial     NodeState = "partial"     // 끝난 변경 작업이 있고 뒤가 실패했거나 시작되지 않았다
	NodeFailed      NodeState = "failed"      // 실패했고 끝난 변경 작업이 없다
	NodeUnreachable NodeState = "unreachable" // 변경 작업 전에 접속하지 못했다
	NodeUnknown     NodeState = "unknown"     // 시작한 작업의 결과를 받지 못했다
	NodeSkipped     NodeState = "skipped"     // 건너뛴 작업이 있다
)

// 노드 상태에 붙는 이유. 기록에서 근거를 댈 수 있는 것만이다. 모듈의 메시지는 어디에도 없다.
const (
	WhyUnreachable = "unreachable"
	WhyTaskFailed  = "task-failed"
	WhyStalled     = "stalled"
	WhyStopped     = "stopped"
	WhyRecordCut   = "record-cut"
	WhySkipped     = "skipped"
)

// Outcome — 실행 전체의 판정.
type Outcome string

const (
	// 미리 보기
	PreviewReady    Outcome = "ready"      // 기대한 작업이 모두 ok · changed이고 바뀔 것이 있다. 이때만 적용 권한이 생긴다
	PreviewNoChange Outcome = "no-change"  // 기대한 작업이 빠짐없이 ok다. 적용할 변경이 없다
	PreviewPartial  Outcome = "incomplete" // 빠지거나 건너뛰었거나 실패했거나 기록이 온전하지 않다
	// 적용
	ApplyDone    Outcome = "applied"
	ApplyPartial Outcome = "not-applied"
)

// TaskVerdict · NodeVerdict · Verdict — 화면이 그리는 판정.
type TaskVerdict struct {
	Name   string
	Status string
}

type NodeVerdict struct {
	Node  string
	State NodeState
	Why   string
	Tasks []TaskVerdict
}

type Verdict struct {
	Check   bool
	Outcome Outcome
	// RecordWhole — 기록이 이 실행의 것이고 처음부터 끝(end)까지 이어진다.
	RecordWhole bool
	// End — 감독이 끝낸 까닭(Result.Reason).
	End string
	// Began — 기록에 실행의 시작(begin)이 있다. callback은 감독의 허용을 받은 뒤에만 이것을 쓴다. 없으면 원격 작업이 없었다.
	Began bool
	Nodes []NodeVerdict
}

type event struct {
	Seq    int      `json:"seq"`
	Run    string   `json:"run"`
	Ev     string   `json:"ev"`
	Check  bool     `json:"check"`
	Hosts  []string `json:"hosts"`
	Task   string   `json:"task"`
	Name   string   `json:"name"`
	Host   string   `json:"host"`
	Status string   `json:"status"`
}

// Header — 감독이 기록 파일을 만들며 쓰는 첫 줄. callback은 이 파일에 덧붙이기만 한다.
func Header(run string) []byte {
	b, _ := json.Marshal(map[string]any{"seq": 0, "run": run, "ev": "open"})
	return append(b, '\n')
}

// Ready — 기록에 이 실행의 준비 신호(둘째 줄)가 있는가. 감독은 이것을 본 뒤에만 실행을 허용한다.
func Ready(record []byte, run string) bool {
	lines := strings.Split(string(record), "\n")
	if len(lines) < 3 { // 두 줄과 끝의 빈 조각
		return false
	}
	var e event
	return json.Unmarshal([]byte(lines[1]), &e) == nil && e.Seq == 1 && e.Run == run && e.Ev == "ready"
}

// Pending — 시작은 기록됐고 결과가 없는 (노드, 작업 순번)들. 감독이 상한을 잴 때 쓴다. 키는 "노드\x00작업 ID"다.
func Pending(record []byte) []string {
	open := map[string]bool{}
	var order []string
	for _, line := range strings.Split(string(record), "\n") {
		var e event
		if line == "" || json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		k := e.Host + "\x00" + e.Task
		switch e.Ev {
		case "start":
			if !open[k] {
				open[k] = true
				order = append(order, k)
			}
		case "result":
			delete(open, k)
		}
	}
	var out []string
	for _, k := range order {
		if open[k] {
			out = append(out, k)
		}
	}
	return out
}

// Evaluate — 실행 기록에서 판정을 낸다. 기대하는 것(plays)은 실행 전에 플레이북에서 읽어 둔 것이다.
//
// **기록이 이 실행의 것인지부터 본다.** 줄마다 run이 이 실행의 ID이고 순번이 0부터 이어져야 한다. 어긋난 줄부터는 읽지
// 않는다. 그래서 같은 계획 · 같은 노드 · 같은 작업을 앞서 미리 본 기록으로는 이 실행을 승인할 수 없다.
//
// res가 nil이면 감독이 결과를 남기지 못한 것이다(EndLost).
func Evaluate(run string, check bool, plays []Play, record []byte, res *Result) Verdict {
	v := Verdict{Check: check}
	if res == nil {
		res = &Result{Exit: -1, Reason: EndLost}
	}
	v.End = res.Reason

	status := make([][]string, len(plays))
	for i, p := range plays {
		status[i] = make([]string, len(p.Tasks)+1)
		for j := range status[i] {
			status[i][j] = TaskNotStarted
		}
	}
	name := func(pi, ti int) string {
		if ti == 0 {
			return GatherTask
		}
		return plays[pi].Tasks[ti-1].Name
	}

	// damaged — 읽을 수 없는 줄이 있다(깨졌거나, 순번이 끊겼거나, 다른 실행의 것이거나, 기대와 다른 내용이다). 그 뒤에 무엇이
	// 있었는지 알 수 없으므로, 사건이 없는 노드도 「실행하지 않음」이라고 하지 못한다. 줄 끝에서 끊긴 것(쓰다 만 끝줄)은 damaged가
	// 아니다: 거기까지가 기록의 전부다.
	whole, sawEnd, began, checkSeen, damaged := true, false, false, false, false
	pi, ti := -1, -1
	lines := strings.Split(string(record), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	} else {
		whole = false // 끝줄이 줄바꿈으로 끝나지 않았다
		if n > 0 {
			lines = lines[:n-1]
		}
	}
	for i, line := range lines {
		var e event
		if json.Unmarshal([]byte(line), &e) != nil || e.Seq != i || e.Run != run || sawEnd {
			whole, damaged = false, true
			break
		}
		bad := false
		switch e.Ev {
		case "open":
			bad = i != 0
		case "ready":
			bad = i != 1
		case "begin":
			bad = i != 2
			began, checkSeen = true, e.Check
		case "play":
			pi, ti = pi+1, -1
			bad = !began || pi >= len(plays) || len(e.Hosts) != 1 || e.Hosts[0] != plays[pi].Node
		case "task":
			// **작업을 알린 줄부터 「보냈을 수 있다」로 본다.** Ansible은 작업을 알린 뒤(v2_playbook_on_task_start)에야 노드에
			// 보낼 차례에 넣는다. callback은 이 줄을 디스크에 내린 뒤에 돌아오고, 쓰지 못하면 실행을 끝낸다. 그래서 이 줄이 없는
			// 작업은 보내지지 않았다. 이 줄 뒤에 기록이 끊겼으면 보내졌는지 알 수 없다.
			ti++
			bad = pi < 0 || pi >= len(plays) || ti >= len(status[pi]) || e.Name != name(pi, ti)
			if !bad {
				status[pi][ti] = TaskNoResult
			}
		case "start":
			bad = pi < 0 || ti < 0 || e.Host != plays[pi].Node || status[pi][ti] != TaskNoResult
		case "result":
			bad = pi < 0 || ti < 0 || e.Host != plays[pi].Node || status[pi][ti] != TaskNoResult
			if !bad {
				switch e.Status {
				case TaskOK, TaskChanged, TaskFailed, TaskSkipped, TaskUnreachable:
					status[pi][ti] = e.Status
				default:
					bad = true
				}
			}
		case "end":
			sawEnd = true
		default:
			bad = true
		}
		if bad {
			whole, damaged = false, true
			break
		}
	}
	v.RecordWhole = whole && sawEnd && began && checkSeen == check
	v.Began = began
	// 「실행하지 않음」의 근거: 기록이 읽을 수 있는 데까지가 전부이고(damaged가 아니고) 한 줄이라도 있거나, 감독이 원격 작업을
	// 허용하지 않았다. 근거는 순서다: callback은 감독의 허용을 받기 전에는 아무것도 시작하지 않고, play와 작업을 알리는 줄을
	// 디스크에 내린 뒤에야 Ansible이 그 작업을 노드에 보내며, 쓰지 못하면 보내기 전에 실행이 끝난다. 그래서 온전히 읽히는 기록에
	// 알린 줄이 없는 작업은 보내지지 않았다. 읽을 수 없는 줄이 있으면 이 근거가 서지 않는다.
	evidence := (!damaged && len(lines) > 0) || res.Reason == EndNotReady || res.Reason == EndNoGrant

	allDone, anyChanged := true, false
	for i, p := range plays {
		nv := NodeVerdict{Node: p.Node}
		has := map[string]bool{}
		doneChange := false
		for j, st := range status[i] {
			nv.Tasks = append(nv.Tasks, TaskVerdict{Name: name(i, j), Status: st})
			has[st] = true
			if j > 0 && (st == TaskOK || st == TaskChanged) {
				doneChange = true
			}
			anyChanged = anyChanged || st == TaskChanged
		}
		onlyGood := !has[TaskFailed] && !has[TaskSkipped] && !has[TaskUnreachable] && !has[TaskNoResult]
		switch {
		case onlyGood && !has[TaskNotStarted]:
			nv.State = NodeDone
		case onlyGood && !has[TaskOK] && !has[TaskChanged] && evidence:
			nv.State = NodeNotRun
		case onlyGood && !has[TaskOK] && !has[TaskChanged]:
			nv.State, nv.Why = NodeUnknown, WhyRecordCut
		case has[TaskNoResult]:
			nv.State, nv.Why = NodeUnknown, cutWhy(res.Reason)
		case has[TaskUnreachable] && !doneChange:
			nv.State, nv.Why = NodeUnreachable, WhyUnreachable
		case has[TaskUnreachable]:
			nv.State, nv.Why = NodePartial, WhyUnreachable
		case has[TaskFailed] && doneChange:
			nv.State, nv.Why = NodePartial, WhyTaskFailed
		case has[TaskFailed]:
			nv.State, nv.Why = NodeFailed, WhyTaskFailed
		case has[TaskSkipped]:
			nv.State, nv.Why = NodeSkipped, WhySkipped
		case evidence && (res.Reason == EndStall || res.Reason == EndStopped || res.Reason == EndOrphaned):
			// 작업 사이에서 감독이 끝냈다. 시작이 기록되지 않은 작업은 시작되지 않았다.
			nv.State, nv.Why = NodePartial, WhyStopped
		case evidence && res.Reason == EndRecord:
			// 작업 사이에서 callback이 기록을 쓰지 못해 끝냈다. 기록된 성공까지만 인정한다.
			nv.State, nv.Why = NodePartial, WhyRecordCut
		default:
			// 기록이 작업 사이에서 끊겼다. 뒤 작업이 기록 없이 돌았는지 알 수 없다.
			nv.State, nv.Why = NodeUnknown, WhyRecordCut
		}
		allDone = allDone && nv.State == NodeDone
		v.Nodes = append(v.Nodes, nv)
	}

	good := v.RecordWhole && res.Exit == 0 && res.Reason == "" && allDone && len(plays) > 0
	switch {
	case check && good && anyChanged:
		v.Outcome = PreviewReady
	case check && good:
		v.Outcome = PreviewNoChange
	case check:
		v.Outcome = PreviewPartial
	case good:
		v.Outcome = ApplyDone
	default:
		v.Outcome = ApplyPartial
	}
	return v
}

func cutWhy(end string) string {
	switch end {
	case EndStall:
		return WhyStalled
	case EndStopped, EndOrphaned:
		return WhyStopped
	}
	return WhyRecordCut
}
