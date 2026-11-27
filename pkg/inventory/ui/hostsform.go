// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"bytes"
	"encoding/csv"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"
)

// 02 관측의 접근 정보 양식. **줄은 선언의 노드가 정한다.**
//
//   - 선언의 노드마다 한 줄이 있다. 노드 ID는 선언의 이름이고 고칠 수 없다. 접속 정보가 없는 노드의 줄은 비어 있고, 빈 줄은
//     저장해도 hosts.csv에 생기지 않는다.
//   - 접근 정보에는 있고 선언에는 없는 행은 「선언에 없는 행」으로 따로 보인다. 운영자가 행마다 고른다: 그대로 둠 · 지움 ·
//     선언의 어느 노드로 옮김. 옮길 대상은 운영자가 직접 고른다(IP나 observed_as로 골라 주지 않는다).
//   - 화면을 여는 것으로는 아무것도 바뀌지 않는다. 「접속 정보 저장」을 눌렀을 때만 파일을 쓴다.

// 선언에 없는 행에 대해 운영자가 고르는 것(양식의 host.move.N).
const (
	HostMoveKeep   = ""  // 그대로 둔다(파일에 남는다. 그 행이 있는 동안 관측은 시작되지 않는다)
	HostMoveRemove = "-" // 지운다
	hostMoveTo     = "=" // 뒤에 선언 노드의 이름이 온다
)

// HostMoveTo — 그 선언 노드로 옮긴다는 선택의 값.
func HostMoveTo(node string) string { return hostMoveTo + node }

// Blank — 접속 정보를 하나도 적지 않은 줄. 선언 노드의 빈 줄은 저장하지 않는다. OS와 접속 방법은 고르는 칸이라 늘 값이
// 있으므로 세지 않는다.
func (r HostRow) Blank() bool {
	return r.Name == "" && r.IP == "" && r.Port == "" && r.User == "" && r.Key == ""
}

// HostFormRows — 양식의 줄. 선언의 노드가 순서대로 오고(접속 정보가 있으면 그 값, 없으면 빈 줄), 그 뒤에 선언에 없는 행이
// 파일의 순서대로 온다. lines는 hosts의 행마다 파일의 줄 번호다(모르면 nil). declKnown이 거짓이면(선언이 없거나 읽지 못했다)
// 견줄 기준이 없어 모든 행이 「선언에 없는 행」이다.
func HostFormRows(declared []string, declKnown bool, hosts []inventory.Host, lines []int) []HostRow {
	row := func(i int, h inventory.Host) HostRow {
		r := HostRow{NodeID: h.NodeID, Name: h.Name, IP: h.IP, User: h.SSHUser, Key: h.SSHKey, OS: h.OS, Conn: h.Conn,
			Was: h.NodeID, HasPass: h.SSHPass != ""}
		if h.Port > 0 {
			r.Port = strconv.Itoa(h.Port)
		}
		if i < len(lines) {
			r.Line = lines[i]
		}
		return r
	}
	byID := map[string]int{}
	for i, h := range hosts {
		if _, dup := byID[h.NodeID]; !dup {
			byID[h.NodeID] = i
		}
	}
	isDeclared := map[string]bool{}
	var out []HostRow
	if declKnown {
		for _, n := range declared {
			isDeclared[n] = true
			if i, ok := byID[n]; ok {
				r := row(i, hosts[i])
				r.Declared = true
				out = append(out, r)
			} else {
				out = append(out, HostRow{NodeID: n, Declared: true})
			}
		}
	}
	for i, h := range hosts {
		if !isDeclared[h.NodeID] || byID[h.NodeID] != i {
			r := row(i, h)
			r.Orphan = true
			out = append(out, r)
		}
	}
	return out
}

// HostRowsFrom — 폼의 줄을 그대로 읽는다(검증하지 않는다). 저장이 막혔을 때 다시 그릴 값이다. 비밀번호는 양식에 없고,
// 「있음」 표시만 저장된 파일에서 가져온다.
func HostRowsFrom(prev []inventory.Host, f url.Values) []HostRow {
	pass := map[string]bool{}
	for _, h := range prev {
		pass[h.NodeID] = h.SSHPass != ""
	}
	var out []HostRow
	for _, i := range formRows(f, "host.id.", "") {
		get := func(field string) string { return strings.TrimSpace(f.Get("host." + field + "." + strconv.Itoa(i))) }
		r := HostRow{NodeID: get("id"), Name: get("name"), IP: get("ip"), Port: get("port"), User: get("user"),
			Key: get("key"), OS: get("os"), Conn: get("conn"), Was: get("was"), Move: f.Get("host.move." + strconv.Itoa(i)),
			Declared: get("row") == hostRowDeclared, Orphan: get("row") == hostRowOrphan}
		r.Line, _ = strconv.Atoi(get("line"))
		if r.NodeID == "" {
			continue // 노드 ID를 비우면 지운 줄이다(이 판 이전의 양식이 그렇게 보낸다)
		}
		r.HasPass = r.Was != "" && pass[r.Was]
		out = append(out, r)
	}
	return out
}

// 양식의 host.row.N: 그 줄이 선언의 노드인가, 선언에 없는 행인가. 다시 그릴 때만 쓴다(판정은 저장할 때 지금의 선언으로 한다).
const (
	hostRowDeclared = "declared"
	hostRowOrphan   = "orphan"
)

// MoveTargets — 선언에 없는 행을 옮길 수 있는 선언 노드(아직 접속 정보가 없는 노드).
func MoveTargets(rows []HostRow) []string {
	var out []string
	for _, r := range rows {
		if r.Declared && r.Blank() && r.Was == "" {
			out = append(out, r.NodeID)
		}
	}
	return out
}

// ApplyHosts — 폼에서 온 값으로 접근 정보를 다시 만든다. 막는 자리가 있으면 그 목록을 돌려주고 아무것도 만들지 않는다.
// from은 saved의 줄마다 그것이 저장된 파일의 몇째 행이었는지다(새 줄은 -1).
//
// **지금의 선언으로 판정한다**(declared · declKnown은 저장 직전에 다시 읽은 것이다). 양식을 연 뒤 선언이 바뀌었으면 여기서
// 드러난다.
//
//   - 선언 노드의 빈 줄은 건너뛴다(파일에 생기지 않는다).
//   - 새로 적은 줄과 옮긴 줄의 노드 ID는 지금 선언에 있어야 한다. 대소문자까지 그대로 견준다.
//   - 옮길 대상에 이미 접속 정보가 있으면 덮어쓰지 않고 막는다(저장된 파일에 그 ID의 행이 있거나, 같은 저장에서 그 줄에도
//     적었다).
//   - 선언에 없는 행을 「그대로 둠」으로 두면 고치지 않고 남긴다. 값이 바뀌어 오면 막는다.
//
// **고르는 칸은 상류 값만 받는다.** OS · 접속 방법은 화면에서 목록으로 고르지만 주소는 밖에서 오는 값이라 여기서 다시 본다.
// WinRM의 두 조건(Windows 노드 · 키 없음)은 상류 ParseHosts가 거절하는 것과 같다.
func ApplyHosts(prev []inventory.Host, f url.Values, declared []string, declKnown bool) (saved []inventory.Host, from []int, problems []HostProblem) {
	prevAt := map[string]int{}
	for i, h := range prev {
		if _, dup := prevAt[h.NodeID]; !dup {
			prevAt[h.NodeID] = i
		}
	}
	isDeclared := map[string]bool{}
	for _, n := range declared {
		isDeclared[n] = true
	}
	type kept struct {
		h    inventory.Host
		from int
	}
	var (
		rows []kept
		seen = map[string]bool{}
	)
	for _, r := range HostRowsFrom(prev, f) {
		was, hadRow := prevAt[r.Was]
		if r.Was == "" {
			hadRow = false
		}
		bad := func(t T) { problems = append(problems, HostProblem{Node: r.NodeID, What: t}) }
		orphan := hadRow && !(declKnown && isDeclared[r.Was])
		id := r.NodeID
		switch {
		case orphan && r.Move == HostMoveRemove:
			continue
		case orphan && strings.HasPrefix(r.Move, hostMoveTo):
			id = strings.TrimPrefix(r.Move, hostMoveTo)
			if !declKnown || !isDeclared[id] {
				problems = append(problems, HostProblem{Node: r.Was + " → " + id, What: tHostMoveNotDeclared})
				continue
			}
			if _, taken := prevAt[id]; taken {
				problems = append(problems, HostProblem{Node: r.Was + " → " + id, What: tHostMoveTaken})
				continue
			}
		case orphan:
			// 그대로 둔다. 고친 값이 함께 왔으면 그 행을 고치려는 것이라 막는다: 선언에 없는 노드의 접속 정보는 고치지 않는다.
			if id != r.Was || !sameHostFields(prev[was], r) {
				bad(tHostOrphanEdit)
				continue
			}
		default:
			if !hadRow && r.Blank() {
				continue // 선언 노드의 빈 줄
			}
			if hadRow && id == r.Was && r.Blank() {
				continue // 접속 정보를 모두 지운 줄: 그 노드의 행을 없앤다
			}
			if !declKnown {
				bad(tHostDeclUnknown)
				continue
			}
			if !isDeclared[id] {
				bad(tHostNotDeclared)
				continue
			}
		}
		if seen[id] {
			if orphan && id != r.Was {
				problems = append(problems, HostProblem{Node: r.Was + " → " + id, What: tHostMoveTaken})
			} else {
				problems = append(problems, HostProblem{Node: id, What: tHostDup})
			}
			continue
		}
		if hasControl(id, r.Name, r.IP, r.Port, r.User, r.Key) {
			bad(tHostControl)
			continue
		}
		seen[id] = true
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
		h := inventory.Host{NodeID: id, Name: r.Name, IP: r.IP, Port: port, SSHUser: r.User, SSHKey: r.Key, OS: osName, Conn: conn}
		k := kept{h: h, from: -1}
		if hadRow {
			k.h.SSHPass, k.from = prev[was].SSHPass, was
		}
		rows = append(rows, k)
	}
	if len(problems) > 0 {
		return nil, nil, problems
	}
	// 파일의 순서를 지킨다: 있던 행은 제자리에(옮긴 행도), 새 줄은 그 뒤에 양식의 순서로.
	byFrom := map[int]kept{}
	for _, k := range rows {
		if k.from >= 0 {
			byFrom[k.from] = k
		}
	}
	for i := range prev {
		if k, ok := byFrom[i]; ok {
			saved, from = append(saved, k.h), append(from, i)
		}
	}
	for _, k := range rows {
		if k.from < 0 {
			saved, from = append(saved, k.h), append(from, -1)
		}
	}
	return saved, from, nil
}

// sameHostFields — 양식의 줄이 저장된 행과 같은 값인가(노드 ID와 비밀번호는 보지 않는다).
func sameHostFields(h inventory.Host, r HostRow) bool {
	port := ""
	if h.Port > 0 {
		port = strconv.Itoa(h.Port)
	}
	osName, conn := strings.ToLower(r.OS), strings.ToLower(r.Conn)
	if osName == "" {
		osName = inventory.OSLinux
	}
	if conn == "" {
		conn = inventory.ConnSSH
	}
	return h.Name == r.Name && h.IP == r.IP && port == r.Port && h.SSHUser == r.User && h.SSHKey == r.Key && h.OS == osName && h.Conn == conn
}

// sameHost — 두 행이 같은 접근 정보인가. 비워 둔 OS · 접속 방법은 기본값(linux · ssh)과 같게 본다.
func sameHost(a, b inventory.Host) bool {
	norm := func(h inventory.Host) inventory.Host {
		h.OS, h.Conn = strings.ToLower(h.OS), strings.ToLower(h.Conn)
		if h.OS == "" {
			h.OS = inventory.OSLinux
		}
		if h.Conn == "" {
			h.Conn = inventory.ConnSSH
		}
		return h
	}
	return norm(a) == norm(b)
}

// HostLines — 접근 정보 파일의 행마다(머리글 다음부터) 그 행이 시작하는 파일의 줄 번호. 운영자가 편집기에서 찾을 자리다.
// 행의 수가 want와 맞지 않으면(상류와 다르게 읽혔다) nil이다.
func HostLines(raw []byte, want int) []int {
	recs := hostRecords(raw)
	if len(recs) != want+1 {
		return nil
	}
	out := make([]int, 0, want)
	for _, r := range recs[1:] {
		out = append(out, r.line)
	}
	return out
}

type hostRecord struct {
	raw    []byte
	line   int
	fields []string
}

// hostRecords — 파일을 CSV 레코드 단위의 원문 조각으로 나눈다(머리글 포함). 읽지 못하면 nil이다.
func hostRecords(raw []byte) []hostRecord {
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	r.Comment = '#' // 상류 형식의 주석 줄. 그 줄의 원문은 바로 뒤 행의 조각에 붙어 함께 남는다.
	var (
		out  []hostRecord
		prev int64
	)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil
		}
		line, _ := r.FieldPos(0)
		end := r.InputOffset()
		if len(rec) > 0 {
			out = append(out, hostRecord{raw: raw[prev:end], line: line, fields: append([]string(nil), rec...)})
		}
		prev = end
	}
	return out
}

// EncodeHostsKeeping — 저장할 접근 정보를 상류 형식의 CSV로 쓰되, **값이 바뀌지 않은 행은 저장된 파일의 바이트를 그대로
// 둔다.** 그래서 한 행의 ID만 옮기면 그 행만 다시 쓰이고, 머리글과 다른 행은 밖에서 만든 모양 그대로 남는다. from은
// saved의 줄마다 그것이 prev의 몇째 행이었는지다(새 줄은 -1). 파일을 행 단위로 나누지 못하면 전부 새로 쓴다(EncodeHosts).
func EncodeHostsKeeping(raw []byte, prev, saved []inventory.Host, from []int) ([]byte, error) {
	recs := hostRecords(raw)
	if len(recs) != len(prev)+1 || len(from) != len(saved) {
		return EncodeHosts(saved)
	}
	var b bytes.Buffer
	line := func(p []byte) {
		b.Write(p)
		if !bytes.HasSuffix(p, []byte("\n")) {
			b.WriteByte('\n')
		}
	}
	line(recs[0].raw)
	for i, h := range saved {
		if j := from[i]; j >= 0 && j < len(prev) && sameHost(prev[j], h) {
			// 파일의 마지막 행이 줄바꿈 없이 끝났고 지금도 마지막이면 그대로 둔다(아무것도 고치지 않은 저장이 바이트를 바꾸지 않게).
			if i == len(saved)-1 && j == len(prev)-1 {
				b.Write(recs[j+1].raw)
			} else {
				line(recs[j+1].raw)
			}
			continue
		}
		// 노드 ID만 바뀐 행(옮긴 행): 저장된 행의 칸을 그대로 두고 첫 칸만 바꾼다. 비워 둔 포트 · OS · 접속 방법도 비운 채로
		// 남는다(읽을 때 채워지는 기본값을 파일에 적지 않는다).
		if j := from[i]; j >= 0 && j < len(prev) && len(recs[j+1].fields) > 0 {
			same := prev[j]
			same.NodeID = h.NodeID
			if sameHost(same, h) {
				fields := append([]string(nil), recs[j+1].fields...)
				fields[0] = h.NodeID
				w := csv.NewWriter(&b)
				if err := w.Write(fields); err != nil {
					return nil, err
				}
				w.Flush()
				if err := w.Error(); err != nil {
					return nil, err
				}
				continue
			}
		}
		one, err := encodeHostRows([]inventory.Host{h})
		if err != nil {
			return nil, err
		}
		b.Write(one)
	}
	return b.Bytes(), nil
}
