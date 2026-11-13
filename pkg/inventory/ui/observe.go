// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

// 03 관측 화면의 접근 정보(`hosts.csv`).
//
// **파일 형식은 상류의 것이다.** 해석과 Ansible 인벤토리 생성은 `pqcota-inventory`의 ParseHosts ·
// RenderAnsibleInventory가 하고, 여기서는 화면의 칸과 그 형식을 오간다. 형식을 따로 정하면 같은 파일을
// 상류 명령(`pqcota-hosts`)과 화면이 다르게 읽는다.
//
// **비밀번호는 화면에 오가지 않는다.** `ssh_pass`는 칸이 없고 「있음」만 보인다. 저장할 때 그 줄이 원래
// 어느 노드였는지(`host.was.N`)로 옛 값을 찾아 그대로 쓴다. 노드 ID를 고쳐도 비밀번호는 따라간다.

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"
)

// HostsHeader — 저장할 때 쓰는 머리글. 상류 예제(`examples/discovery/hosts.csv`)와 같은 순서다.
var HostsHeader = []string{"node_id", "name", "ip", "port", "ssh_user", "ssh_key", "ssh_pass", "os", "connection"}

// HostRow — 접근 정보 한 줄. 비밀번호는 싣지 않는다(HasPass뿐이다).
type HostRow struct {
	NodeID, Name, IP, Port, User, Key, OS, Conn string
	// Was — 이 줄이 저장된 파일에서 어느 노드였나. 새 줄은 비어 있다.
	Was     string
	HasPass bool
}

// HostProblem — 저장을 막은 자리 하나. 그 줄의 노드 ID와 무엇이 틀렸는지.
type HostProblem struct {
	Node string
	What T
}

// ObserveView — 03 관측 화면.
type ObserveView struct {
	Page
	Hosts []HostRow
	// Problems — 저장을 막은 자리. 있으면 화면은 받은 그대로의 값을 다시 그린다(적던 것을 잃지 않는다).
	Problems []HostProblem
	// Run — 관측 실행. 저장하지 않는 서버(보기 전용 · 복구)에서는 Checks가 비고 시작 단추가 없다.
	Run ObserveRun
}

// ObserveRun — 관측 실행 구역.
type ObserveRun struct {
	Checks []RunCheck
	// Writable — 이 서버가 저장하는가. 아니면 시작하지 않는다.
	Writable bool
	// Window — 시작 양식에 채울 네트워크 관측 구간(초).
	Window int
	// Status — 마지막(또는 지금) 실행. 한 번도 돌리지 않았으면 nil이다.
	Status *RunStatus
	// Current — results/가 지금 어느 실행의 결과인가. 화면에서 관측을 돌려 바꾼 적이 없으면 nil이다.
	Current *RunCurrent
	// JCAUnconfirmed — 지금 결과에서 JCA 서비스 목록을 확인할 수 없는 노드(report.Result의 같은 칸). 시작을 막지 않는다.
	JCAUnconfirmed []string
}

// RunCurrent — 지금 결과 세대.
type RunCurrent struct {
	ID           string
	Ended        time.Time
	Nodes, Files int
}

// CanStart — 시작 단추를 낼 수 있는가. 막는 점검이 남았거나 이미 돌고 있으면 낼 수 없다.
func (r ObserveRun) CanStart() bool {
	if !r.Writable || (r.Status != nil && (r.Status.State == RunRunning || r.Status.State == RunFinishing)) {
		return false
	}
	for _, c := range r.Checks {
		if c.Blocking && !c.OK {
			return false
		}
	}
	return true
}

// RunItem — 시작 전에 보는 것 하나.
type RunItem string

const (
	RunHosts       RunItem = "hosts"
	RunAnsible     RunItem = "ansible"
	RunCollectors  RunItem = "collectors"
	RunLinuxBins   RunItem = "linux-bins"
	RunWindowsBins RunItem = "windows-bins"
	RunJar         RunItem = "jar"
	// RunStuckItem — 끝내지 못한 지난 실행의 프로세스가 있다.
	RunStuckItem RunItem = "stuck"
	// 수집기 판(collectorver). 모두 확인되면 RunCollectorVersion 하나만 통과로 보인다.
	RunCollectorVersion RunItem = "collector-version"
	// RunCollectorMismatch — 고정 판이 아닌 판이 보이거나 수집기끼리 판이 섞였다. 어떤 옵션으로도 넘기지 못한다.
	RunCollectorMismatch RunItem = "collector-mismatch"
	// RunCollectorUnverified — 판을 확인할 수 없다. -allow-unverified-collectors 로만 넘긴다.
	RunCollectorUnverified RunItem = "collector-unverified"
	// RunCollectorAllowed — 확인할 수 없는 수집기를 -allow-unverified-collectors 로 허용했다(경고로 보인다).
	RunCollectorAllowed RunItem = "collector-allowed"
	// RunJarMismatch — collector.jar 의 정식 판이 다르다(막는다, 넘길 수 없다).
	RunJarMismatch RunItem = "jar-mismatch"
	// RunJarUnverified — collector.jar 를 확인할 수 없다(판 정보 없음 · devel · 태그 뒤 커밋 · dirty · 커밋 정보 없음). 수집기의
	// 확인 불가처럼 막고, -allow-unverified-collectors 로 허용하면 RunJarAllowed(경고)다. v0.30.0 전에는 판 정보가 없으면 경고만 했다.
	RunJarUnverified RunItem = "jar-unverified"
	RunJarAllowed    RunItem = "jar-allowed"
)

// CollectorFile — 수집기 파일 하나의 판 판정(collectorver.File 을 화면에 옮긴 것).
type CollectorFile struct {
	Path         string
	State        string // verified · mismatch · unverified
	Reason       string // collectorver.Reason
	Declared     string
	Revision     string
	Sum          string // 빌드 정보의 주 모듈 h1(확인됨이면 고정한 값과 같다)
	DepsReplaced bool
}

// commitHex — 커밋으로 보이는 값: git 의 SHA-1(40자) · SHA-256(64자) 16진수. 이 꼴일 때만 줄여 보이고 전체를 title 에 둔다.
var commitHex = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Title — 접근 가능한 설명(title)에 둘 전체 커밋. 화면 줄에는 앞 12자만 보인다. 커밋 꼴이 아니면 빈 값이다.
func (f CollectorFile) Title(l Lang) string {
	if commitHex.MatchString(f.Revision) {
		return fmt.Sprintf(tCVFullCommit.In(l), f.Revision)
	}
	return ""
}

// RunCheck — 점검 하나. Blocking이 아닌 것은 빠져도 시작하지만 그 관측이 빠진다는 것을 알린다.
type RunCheck struct {
	Item     RunItem
	OK       bool
	Blocking bool
	// Detail — 찾은 자리(명령의 경로 · 폴더). 운영자 자신의 기계 경로라 보인다.
	Detail string
	// Files — 수집기 판 점검이면 파일마다의 판정. Procs — 끝내지 못한 지난 관측이면 남긴 프로세스의 지금 상태.
	Files []CollectorFile
	Procs []ProcInfo
}

// Text — 점검의 이름.
func (c RunCheck) Text(l Lang) string {
	if t, ok := runItemText[c.Item]; ok {
		return t.In(l)
	}
	return string(c.Item)
}

// Missing — 빠졌을 때 무엇을 하면 되는지.
func (c RunCheck) Missing(l Lang) string {
	if t, ok := runItemMissing[c.Item]; ok {
		return t.In(l)
	}
	return ""
}

// Line — 「선언된 판: … · 실제 소스: … · 상태」. 수집기 자신은 맞고 의존만 replace 면
// 「수집기 소스 v0.10.7 확인 · replace 의존 소스 확인 불가」다.
func (f CollectorFile) Line(l Lang) string {
	if f.Reason == "deps-replaced" {
		return fmt.Sprintf(tCVDepsReplaced.In(l), f.Declared)
	}
	declared := f.Declared
	if declared == "" {
		declared = tCVNone.In(l)
	}
	var source string
	switch {
	case f.Reason == "" && commitHex.MatchString(f.Revision):
		source = fmt.Sprintf(tCVTagCommit.In(l), f.Revision[:12])
	case f.Reason == "" && f.Sum != "":
		source = fmt.Sprintf(tCVModuleSum.In(l), declared)
	case f.Reason == "":
		source = declared
	default:
		source = collectorReasonText[f.Reason].In(l)
	}
	parts := []string{fmt.Sprintf(tCVDeclared.In(l), declared), fmt.Sprintf(tCVSource.In(l), source), collectorStateText[f.State].In(l)}
	return strings.Join(parts, " · ")
}

// RunState — 실행의 상태.
type RunState string

const (
	RunRunning RunState = "running"
	RunDone    RunState = "done"
	RunFailed  RunState = "failed"
	RunStopped RunState = "stopped"
	// RunIncomplete — ansible-playbook은 0으로 끝났지만 어느 노드의 결과가 빠졌다.
	RunIncomplete RunState = "incomplete"
	// RunInterrupted — 화면이 실행 도중에 끝났다. 감독이나 다음 시작이 남은 것을 정리했다.
	RunInterrupted RunState = "interrupted"
	// RunFinishing — 프로세스는 끝났고 노드 정리 · 프로세스 회수 · 마지막 기록을 하는 중이다. 화면에만 보이고 기록에는 쓰지
	// 않는다. 이 동안은 새 관측을 받지 않는다.
	RunFinishing RunState = "finishing"
	// RunStuck — 이 실행의 프로세스를 끝내지 못했다. 아무것도 지우거나 옮기지 않았고, 새 관측을 시작하지 않는다.
	RunStuck RunState = "stuck"
)

// RunStatus — 실행 하나의 지금 모습.
type RunStatus struct {
	ID             string
	State          RunState
	Started, Ended time.Time
	Log            string
	// Dropped — 화면에 남기지 못하고 앞에서 버린 로그 줄 수.
	Dropped  int
	Received int
	Exit     int
	Window   int
	// Missing — 결과가 오지 않았거나 맞지 않는 자리(와 그 까닭).
	Missing []string
	// NodeCleanup — 끝까지 돌지 못한 실행 뒤 노드 정리의 결과(done · failed). 비면 정리할 일이 없었다.
	NodeCleanup string
	// Stuck · StuckWhy — 끝내지 못했거나 확인하지 못한 이 실행의 프로세스와 그 까닭.
	Stuck    []int
	StuckWhy string
	// StuckProcs — Stuck 을 남길 때 찾은 PID와 시작 시각. 지금 상태는 시작 전 점검(RunStuckItem)의 Procs 가 보인다.
	StuckProcs []ProcRecord
}

// ProcRecord — 「회수 필요」로 남긴 프로세스 하나(찾을 때의 PID와 시작 시각).
type ProcRecord struct {
	PID   int
	Start string
}

// ProcState — 남긴 프로세스의 지금 상태.
type ProcState string

const (
	ProcSame        ProcState = "same"        // 남긴 그 프로세스가 아직 있다(PID와 시작 시각이 같다)
	ProcGone        ProcState = "gone"        // 이미 종료됨
	ProcReused      ProcState = "reused"      // 같은 PID를 다른 프로세스가 쓴다. 회수 대상이 아니다
	ProcDenied      ProcState = "denied"      // 조회 권한이 없거나 이 사용자의 프로세스가 아니다
	ProcFailed      ProcState = "failed"      // 조회하지 못했다
	ProcUnconfirmed ProcState = "unconfirmed" // 시작 시각을 남기지 않은 옛 기록이라 같은 프로세스인지 확인할 수 없다
)

// ProcInfo — 화면에 보일 프로세스 정보. **Args 는 서버에서 이미 가린 값이다.** 원문 인자는 템플릿에 넘기지 않는다(접힌
// 내용도 HTML 에 들어가므로). 환경 변수는 읽지도 싣지도 않는다. ProcSame 이 아니면 PID 밖의 칸은 비어 있다.
type ProcInfo struct {
	PID, PPID, PGID int
	State           ProcState
	User, Exe       string
	Started         string
	Args            string
	// ExeDenied · ArgsDenied — 그 칸만 읽을 권한이 없었다.
	ExeDenied, ArgsDenied bool
}

// StateText — 상태 한 줄과 할 일.
func (p ProcInfo) StateText(l Lang) string { return procStateText[p.State].In(l) }

// StagingPaths — 노드에 남을 수 있는 스테이징 폴더(실행 ID가 붙는다).
func (s RunStatus) StagingPaths() string {
	return "/tmp/pqcaton-observe-" + s.ID + " (Windows: C:\\Windows\\Temp\\pqcaton-observe-" + s.ID + ")"
}

// Text — 상태 한 줄.
func (s RunStatus) Text(l Lang) string {
	switch s.State {
	case RunRunning:
		return tRunRunning.F(l, s.Started.Format("15:04:05"))
	case RunDone:
		return tRunDone.F(l, s.Received)
	case RunStopped:
		return tRunStopped.F(l, s.Received)
	case RunIncomplete:
		return tRunIncomplete.F(l, len(s.Missing), s.Received)
	case RunInterrupted:
		return tRunInterrupted.In(l)
	case RunFinishing:
		return tRunFinishing.In(l)
	case RunStuck:
		return tRunStuck.F(l, s.StuckWhy, s.StagingPaths())
	}
	return tRunFailed.F(l, s.Exit, s.Received)
}

// ObserveSummary — 관측 카드에 적을 것.
type ObserveSummary struct {
	Hosts int
}

// Summary — 카드의 숫자.
func (v ObserveView) Summary() ObserveSummary { return ObserveSummary{Hosts: len(v.Hosts)} }

// NewObserveView — 파일에서 읽은 접근 정보로 화면을 만든다.
func NewObserveView(hosts []inventory.Host, p Page) ObserveView {
	v := ObserveView{Page: p}
	for _, h := range hosts {
		v.Hosts = append(v.Hosts, HostRow{NodeID: h.NodeID, Name: h.Name, IP: h.IP, Port: strconv.Itoa(h.Port),
			User: h.SSHUser, Key: h.SSHKey, OS: h.OS, Conn: h.Conn, Was: h.NodeID, HasPass: h.SSHPass != ""})
	}
	return v
}

// HostRowsFrom — 폼의 줄을 그대로 읽는다(검증하지 않는다). 저장이 막혔을 때 다시 그릴 값이다.
func HostRowsFrom(prev []inventory.Host, f url.Values) []HostRow {
	pass := map[string]bool{}
	for _, h := range prev {
		pass[h.NodeID] = h.SSHPass != ""
	}
	var out []HostRow
	for _, i := range formRows(f, "host.id.", "") {
		get := func(field string) string { return strings.TrimSpace(f.Get("host." + field + "." + strconv.Itoa(i))) }
		r := HostRow{NodeID: get("id"), Name: get("name"), IP: get("ip"), Port: get("port"), User: get("user"),
			Key: get("key"), OS: get("os"), Conn: get("conn"), Was: get("was")}
		if r.NodeID == "" {
			continue // 노드 ID를 비우면 지운 줄이다
		}
		r.HasPass = r.Was != "" && pass[r.Was]
		out = append(out, r)
	}
	return out
}

// ApplyHosts — 폼에서 온 값으로 접근 정보를 다시 만든다. 막는 자리가 있으면 그 목록을 돌려주고 아무것도
// 만들지 않는다.
//
// **고르는 칸은 상류 값만 받는다.** OS · 접속 방법은 화면에서 목록으로 고르지만 주소는 밖에서 오는
// 값이라 여기서 다시 본다. WinRM의 두 조건(Windows 노드 · 키 없음)은 상류 ParseHosts가 거절하는 것과
// 같고, 저장한 뒤에야 관측이 실패하지 않게 저장에서 막는다.
func ApplyHosts(prev []inventory.Host, f url.Values) ([]inventory.Host, []HostProblem) {
	pass := map[string]string{}
	for _, h := range prev {
		pass[h.NodeID] = h.SSHPass
	}
	var (
		out      []inventory.Host
		problems []HostProblem
		seen     = map[string]bool{}
	)
	for _, r := range HostRowsFrom(prev, f) {
		bad := func(t T) { problems = append(problems, HostProblem{Node: r.NodeID, What: t}) }
		if seen[r.NodeID] {
			bad(tHostDup)
			continue
		}
		if hasControl(r.NodeID, r.Name, r.IP, r.Port, r.User, r.Key) {
			bad(tHostControl)
			continue
		}
		seen[r.NodeID] = true
		osName, conn := strings.ToLower(r.OS), strings.ToLower(r.Conn)
		if osName == "" {
			osName = inventory.OSLinux
		}
		if conn == "" {
			conn = inventory.ConnSSH
		}
		switch {
		case osName != inventory.OSLinux && osName != inventory.OSWindows:
			bad(tHostOS)
			continue
		case conn != inventory.ConnSSH && conn != inventory.ConnWinRM:
			bad(tHostConn)
			continue
		case conn == inventory.ConnWinRM && osName != inventory.OSWindows:
			bad(tHostWinRMOS)
			continue
		case conn == inventory.ConnWinRM && r.Key != "":
			bad(tHostWinRMKey)
			continue
		}
		port := 0
		if r.Port != "" {
			p, err := strconv.Atoi(r.Port)
			if err != nil || p < 1 || p > 65535 {
				bad(tHostPort)
				continue
			}
			port = p
		}
		if r.IP == "" {
			bad(tHostNoIP)
			continue
		}
		out = append(out, inventory.Host{NodeID: r.NodeID, Name: r.Name, IP: r.IP, Port: port, SSHUser: r.User,
			SSHKey: r.Key, SSHPass: pass[r.Was], OS: osName, Conn: conn})
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return out, nil
}

// EncodeHosts — 접근 정보를 상류 형식의 CSV로. 포트가 0이면 비워 둔다(상류가 접속 방법의 기본값을 쓴다).
func EncodeHosts(hosts []inventory.Host) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if err := w.Write(HostsHeader); err != nil {
		return nil, err
	}
	for _, h := range hosts {
		port := ""
		if h.Port > 0 {
			port = strconv.Itoa(h.Port)
		}
		if err := w.Write([]string{h.NodeID, h.Name, h.IP, port, h.SSHUser, h.SSHKey, h.SSHPass, h.OS, h.Conn}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

// RenderRunStatus — 관측 실행 상태 조각. 화면이 돌고 있는 동안 몇 초마다 다시 받는다.
func RenderRunStatus(w io.Writer, l Lang, csrf string, st *RunStatus) error {
	return runStatus(l, csrf, st).Render(context.Background(), w)
}

// RenderObserve — 03 관측 화면.
func RenderObserve(w io.Writer, v ObserveView) error {
	return observePage(v).Render(context.Background(), w)
}

// hasControl — 값에 인쇄할 수 없는 문자가 있는가. 줄바꿈 · 탭 같은 제어 문자와, YAML이 줄바꿈으로 읽는 U+2028 ·
// U+2029, 보이지 않는 서식 문자까지다. 접근 정보에서는 정당한 쓰임이 없고, 인벤토리의 줄을 갈라 다른 설정을 끼워
// 넣는 길이 된다. 실행 전 점검(hostValuesOK)과 같은 규칙이다.
func hasControl(vs ...string) bool {
	for _, v := range vs {
		if strings.IndexFunc(v, func(r rune) bool { return r != ' ' && !unicode.IsPrint(r) }) >= 0 {
			return true
		}
	}
	return false
}
