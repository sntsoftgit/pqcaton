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
	"io"
	"net/url"
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
}

// RunCurrent — 지금 결과 세대.
type RunCurrent struct {
	ID           string
	Ended        time.Time
	Nodes, Files int
}

// CanStart — 시작 단추를 낼 수 있는가. 막는 점검이 남았거나 이미 돌고 있으면 낼 수 없다.
func (r ObserveRun) CanStart() bool {
	if !r.Writable || (r.Status != nil && r.Status.State == RunRunning) {
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
)

// RunCheck — 점검 하나. Blocking이 아닌 것은 빠져도 시작하지만 그 관측이 빠진다는 것을 알린다.
type RunCheck struct {
	Item     RunItem
	OK       bool
	Blocking bool
	// Detail — 찾은 자리(명령의 경로 · 폴더). 운영자 자신의 기계 경로라 보인다.
	Detail string
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

// RunState — 실행의 상태.
type RunState string

const (
	RunRunning RunState = "running"
	RunDone    RunState = "done"
	RunFailed  RunState = "failed"
	RunStopped RunState = "stopped"
	// RunIncomplete — ansible-playbook은 0으로 끝났지만 어느 노드의 결과가 빠졌다.
	RunIncomplete RunState = "incomplete"
	// RunInterrupted — 화면이 실행 도중에 끝났다. 다음 시작이 남은 것을 정리했다.
	RunInterrupted RunState = "interrupted"
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
	// Missing — 결과가 오지 않은 노드(와 무엇이 빠졌는지).
	Missing []string
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

// hasControl — 값에 제어 문자(줄바꿈 · 탭 같은 것)가 있는가. 접근 정보에서는 정당한 쓰임이 없고, 인벤토리의
// 줄을 갈라 다른 설정을 끼워 넣는 길이 된다.
func hasControl(vs ...string) bool {
	for _, v := range vs {
		if strings.IndexFunc(v, unicode.IsControl) >= 0 {
			return true
		}
	}
	return false
}
