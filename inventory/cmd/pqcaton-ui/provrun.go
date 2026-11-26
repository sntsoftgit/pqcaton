// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 06 프로비저닝의 미리 보기(`--check`)와 결과 화면.
//
// **이 판은 미리 보기까지만 돌린다.** 적용을 시작하는 처리기는 없다. 미리 보기가 「바뀔 것이 있고 모두 확인했다」로 끝나면
// 적용 권한(provrun.Grant)을 실행 자리에 남기고 화면은 그 상태(유효 · 만료 · 입력이 바뀜)를 보이기만 한다.
//
// 화면이 돌리는 플레이북은 한 종류다: **지금 계획에 서명한, 이 키로 확인되는 계획에서 L2로 생성한 것**이고 설정 파일만
// 놓는다. 파일이 생성기의 출력과 한 글자라도 다르면 돌리지 않는다. provider 모듈 · L3 훅 · 수동 단계가 있으면 그 노드와
// 작업을 밝히고 시작하지 않는다.
//
// 실행 자리에는 그 실행이 읽는 것을 모두 둔다(플레이북 사본 · 인벤토리 · 변수 · 설정 · callback). 미리 보기와 적용이 같은
// 것을 읽는지는 그 묶음의 해시(provrun.Binding)로 가린다. 인벤토리와 변수는 실행이 끝나면 지운다.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	provisioningv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/provisioning/v1"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory"
	"github.com/randyinthedev-hash/pqcota-provisioning/pkg/provisioning"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/provrun"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

const (
	wsProvisionRuns  = "provision-runs"
	provManifestFile = "run.json"
	provResultFile   = "result.json"
	provEventsFile   = "events.jsonl"
	provGoFile       = "go"
	provInventory    = "inventory.yml"
	provPlaybook     = "playbook.yml"
	provCallbackDir  = "callback"
)

// provStall: 작업을 시작한 뒤 결과를 기다리는 상한(초). 설정 파일 배치의 첫 운영 기본값이다. 시험이 줄인다.
var provStall = 120

// provManifest: 실행 하나의 기록. Ansible을 띄우기 전에 쓴다. Plays는 실행 전에 플레이북에서 읽은, 기대하는 작업이다.
type provManifest struct {
	ID      string          `json:"id"`
	Check   bool            `json:"check"`
	Started time.Time       `json:"started"`
	Stall   int             `json:"stall"`
	Plays   []provrun.Play  `json:"plays"`
	Binding provrun.Binding `json:"binding"`
	// RecheckOf: 이 실행이 재확인하는 적용 실행의 ID. 비면 재확인이 아니다. 재확인은 미리 보기와 같은 실행(--check)이지만 적용
	// 권한을 남기지 않는다.
	RecheckOf string `json:"recheck_of,omitempty"`
}

type provRunner struct {
	mu  sync.Mutex
	job *provJob
}

type provJob struct {
	m    provManifest
	dir  string
	cmd  *exec.Cmd
	done chan struct{}
}

func (j *provJob) closed() bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

// provDir: 실행 자리들. 계획 파일 옆이다.
func (s *server) provDir() string { return filepath.Join(filepath.Dir(s.planOut), wsProvisionRuns) }

// provReady: 지금 미리 볼 수 있는가. 막는 것이 없으면 실행에 들어갈 것을 모두 돌려준다.
type provReady struct {
	plays              []provrun.Play
	playbook, inv, cfg []byte
	vars               []byte
	ansible            string
	binding            provrun.Binding
}

// provCheck: 미리 보기의 조건을 본다. 막는 것(blocks)이 비면 ready가 채워진다.
func (s *server) provCheck(v ui.ApproveView) (provReady, []ui.ProvBlock) {
	var rd provReady
	block := func(code, node, task string) []ui.ProvBlock {
		return []ui.ProvBlock{{Code: code, Node: node, Task: task}}
	}
	if !v.Writable {
		return rd, block(ui.ProvViewOnly, "", "")
	}
	if v.Signed.State != ui.SignValid || v.TextProblem || v.FieldsProblem || v.RulesetProblem != "" || v.ScopeProblem != "" {
		return rd, block(ui.ProvNotApproved, "", "")
	}
	if !v.Playbooks.Made {
		return rd, block(ui.ProvNoPlaybook, "", "")
	}
	signedBytes, err := os.ReadFile(s.signedPath())
	if err != nil {
		return rd, block(ui.ProvNotApproved, "", "")
	}
	signed, err := loadPlanFile(s.signedPath())
	if err != nil || signed == nil {
		return rd, block(ui.ProvNotApproved, "", "")
	}
	book, err := os.ReadFile(s.provisionPath())
	if err != nil {
		return rd, block(ui.ProvNoPlaybook, "", "")
	}
	// 파일이 이 서명한 계획에서 L2로 생성한 것과 한 글자도 다르지 않아야 한다. 다른 수준으로 만들었거나 손으로 고친 것,
	// 서명이 바뀐 뒤 다시 만들지 않은 것은 여기서 걸린다.
	want := provisioning.GenerateProvisioningPlaybook(signed, provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L2_STAGE_INSTALL)
	if !bytes.Equal(book, []byte(want)) {
		if v.Playbooks.Level != "" && v.Playbooks.Level != "L2" {
			return rd, block(ui.ProvLevel, "", "")
		}
		return rd, block(ui.ProvNotGenerated, "", "")
	}
	pb := provrun.ParsePlaybook(book)
	if pb.Malformed {
		return rd, block(ui.ProvUnknownForm, "", "")
	}
	var blocks []ui.ProvBlock
	for _, u := range pb.Unsupported() {
		blocks = append(blocks, ui.ProvBlock{Code: ui.ProvUnsupported + "-" + u.Why, Node: u.Node, Task: u.Task})
	}
	// 계획이 L3로 확정한 조치는 생성기가 훅을 넣는다. 훅이 비어 작업이 안 생겼어도 화면은 그 조치를 돌리지 않는다.
	for _, a := range signed.GetActions() {
		if a.GetAutomationLevel() == provisioningv1.DeployAutomationLevel_DEPLOY_AUTOMATION_LEVEL_L3_FULL_AUTO {
			blocks = append(blocks, ui.ProvBlock{Code: ui.ProvUnsupported + "-" + provrun.WhyL3, Node: a.GetTargetNodeId(), Task: a.GetId()})
		}
	}
	if len(blocks) > 0 {
		return rd, blocks
	}
	hosts, err := s.loadHosts()
	if err != nil {
		return rd, block(ui.ProvNoHosts, "", "")
	}
	byNode := map[string]inventory.Host{}
	for _, h := range hosts {
		byNode[h.NodeID] = h
	}
	var targets []inventory.Host
	for _, n := range pb.Nodes() {
		h, ok := byNode[n]
		switch {
		case !ok:
			blocks = append(blocks, ui.ProvBlock{Code: ui.ProvNoHosts, Node: n})
		case h.OS == inventory.OSWindows || h.Conn == inventory.ConnWinRM:
			blocks = append(blocks, ui.ProvBlock{Code: ui.ProvWindows, Node: n})
		default:
			targets = append(targets, h)
		}
	}
	if len(blocks) > 0 {
		return rd, blocks
	}
	inv, err := renderProvTargets(targets)
	if err != nil {
		return rd, block(ui.ProvNoHosts, "", "")
	}
	ansible, err := exec.LookPath(ansibleCommand)
	if err != nil {
		return rd, block(ui.ProvNoAnsible, "", "")
	}
	fi, err := os.Stat(ansible)
	if err != nil {
		return rd, block(ui.ProvNoAnsible, "", "")
	}
	vars, _ := json.Marshal(runVars())
	// 설정은 실행 자리의 경로를 담는다. 묶음에는 자리를 뺀 꼴의 해시를 넣는다.
	cfg := ansibleConfigWith(".", provCallbackDir, provrun.CallbackName)
	rd = provReady{plays: pb.Plays, playbook: book, inv: inv, cfg: cfg, vars: vars, ansible: ansible, binding: provrun.Binding{
		"plan": provrun.Hash(signedBytes), "playbook": provrun.Hash(book), "inventory": provrun.Hash(inv),
		"vars": provrun.Hash(vars), "config": provrun.Hash(cfg), "callback": provrun.Hash(provrun.CallbackPlugin),
		"ansible": fmt.Sprintf("%s|%d|%d", ansible, fi.Size(), fi.ModTime().UnixNano()),
		"stall":   fmt.Sprint(provStall),
	}}
	return rd, nil
}

// renderProvTargets: 06 실행의 인벤토리. 플레이북이 노드 ID로 겨누므로 호스트 이름이 노드 ID다(03은 별칭을 쓴다). 이름은
// provrun.NodeNameOK를 지난 것뿐이고 값은 모두 !unsafe다. 플레이북이 겨누는 노드만 싣는다.
func renderProvTargets(hosts []inventory.Host) ([]byte, error) {
	if err := hostValuesOK(hosts); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("# pqcaton: runtime-only inventory for one provisioning run. Removed when the run ends.\n")
	b.WriteString("targets:\n  hosts:\n")
	for _, h := range hosts {
		if !provrun.NodeNameOK(h.NodeID) {
			return nil, errHostValue
		}
		fmt.Fprintf(&b, "    %q:\n", h.NodeID)
		put := func(k, v string) {
			if v != "" {
				fmt.Fprintf(&b, "      %s: %s\n", k, yamlUnsafe(v))
			}
		}
		put("ansible_host", h.IP)
		fmt.Fprintf(&b, "      ansible_port: %d\n", h.Port)
		put("ansible_user", h.SSHUser)
		b.WriteString("      ansible_connection: ansible.builtin.ssh\n")
		put("ansible_ssh_private_key_file", h.SSHKey)
		put("ansible_ssh_pass", h.SSHPass)
	}
	return b.Bytes(), nil
}

var errProvBusy = errors.New("a provisioning run is already going")

// startProv: 미리 보기를 띄운다. 끝나기를 기다리지 않는다. 띄우기 전에 실패하면 실행 자리에 결과(not-ready)를 남긴다.
func (s *server) startProv(rd provReady, check bool, recheckOf string) error {
	p := &s.prov
	p.mu.Lock()
	if p.job != nil && !p.job.closed() {
		p.mu.Unlock()
		return errProvBusy
	}
	id, err := runID()
	if err != nil {
		p.mu.Unlock()
		return err
	}
	// 시작 시각은 실제 시각이다(화면의 시계를 시험이 바꿔도 실행의 순서는 실제 순서다).
	j := &provJob{m: provManifest{ID: id, Check: check, Started: time.Now(), Stall: provStall, Plays: rd.plays, Binding: rd.binding, RecheckOf: recheckOf},
		dir: filepath.Join(s.provDir(), id), done: make(chan struct{})}
	p.job = j
	p.mu.Unlock()

	fail := func(err error) error {
		fmt.Fprintln(s.stderr, "provisioning run "+id+":", err)
		if isDir(j.dir) {
			_ = writeDurable(filepath.Join(j.dir, provResultFile), provrun.Result{Exit: 3, Reason: provrun.EndNotReady, Ended: time.Now()})
			removeProvSecrets(j.dir)
		}
		close(j.done)
		return nil
	}
	if err := os.MkdirAll(filepath.Join(j.dir, provCallbackDir), 0o700); err != nil {
		return fail(err)
	}
	// 기록을 먼저 쓴다. 인벤토리를 쓴 뒤 화면이 죽어도 다음 시작이 이 자리를 알아보고 지운다.
	if err := writeDurable(filepath.Join(j.dir, provManifestFile), j.m); err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(filepath.Join(j.dir, runPluginsNone), 0o700); err != nil {
		return fail(err)
	}
	for _, f := range []struct {
		name string
		body []byte
	}{{provInventory, rd.inv}, {"vars.json", rd.vars}, {provPlaybook, rd.playbook},
		{runConfigName, ansibleConfigWith(j.dir, filepath.Join(j.dir, provCallbackDir), provrun.CallbackName)},
		{filepath.Join(provCallbackDir, provrun.CallbackName+".py"), provrun.CallbackPlugin}} {
		if err := os.WriteFile(filepath.Join(j.dir, f.name), f.body, 0o600); err != nil {
			return fail(err)
		}
	}
	exe, err := supervisorExe()
	if err != nil {
		return fail(err)
	}
	cmd := exec.Command(exe, provSuperviseFlag, j.dir, rd.ansible)
	cmd.Dir = j.dir
	cmd.Env = append(childEnv(), runMarker(id))
	ownGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fail(err)
	}
	if err := cmd.Start(); err != nil {
		return fail(err)
	}
	p.mu.Lock()
	j.cmd = cmd
	p.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		_ = stdin.Close()
		s.finishProv(j.dir, j.m, cmd.Process.Pid)
		close(j.done)
	}()
	return nil
}

// finishProv: 끝난 실행을 마무리한다. 남은 프로세스를 끝내고, 감독이 결과를 남기지 못했으면 그렇게 적고, 비밀을 지우고,
// 미리 보기가 통과했으면 적용 권한을 남긴다. 시작 때의 복구도 이것을 쓴다(pgid 0).
func (s *server) finishProv(dir string, m provManifest, pgid int) {
	mark := runMark{Marker: runMarker(m.ID)}
	if pgid > 0 {
		mark.Suspect = []int{pgid}
	}
	if why := reaped(reapMarked(mark, stopGrace)); why != "" {
		fmt.Fprintln(s.stderr, "provisioning run "+m.ID+":", why)
	}
	if !isFile(filepath.Join(dir, provResultFile)) {
		_ = writeDurable(filepath.Join(dir, provResultFile), provrun.Result{Exit: -1, Reason: provrun.EndLost, Ended: time.Now()})
	}
	removeProvSecrets(dir)
	v, res := provVerdict(dir, m)
	if v.Outcome == provrun.PreviewReady && res != nil && m.RecheckOf == "" {
		if err := provrun.WriteGrant(dir, provrun.NewGrant(m.ID, m.Binding.Digest(), res.Ended)); err != nil {
			fmt.Fprintln(s.stderr, "provisioning run "+m.ID+": the apply grant could not be written:", writeFailure(err))
		}
	}
}

// removeProvSecrets: 실행 자리에서 접속 정보가 실린 것을 지운다. 플레이북 사본과 기록은 남긴다.
func removeProvSecrets(dir string) {
	for _, name := range []string{provInventory, "vars.json", provGoFile} {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// provVerdict: 실행 자리의 기록으로 판정을 낸다. 결과 파일이 없으면 감독이 남기지 못한 것이다.
func provVerdict(dir string, m provManifest) (provrun.Verdict, *provrun.Result) {
	var res *provrun.Result
	if b, err := os.ReadFile(filepath.Join(dir, provResultFile)); err == nil {
		var r provrun.Result
		if json.Unmarshal(b, &r) == nil {
			res = &r
		}
	}
	rec, _ := os.ReadFile(filepath.Join(dir, provEventsFile))
	return provrun.Evaluate(m.ID, m.Check, m.Plays, rec, res), res
}

// provRuns: 실행 자리의 ID들(오래된 것부터). ID는 시각으로 시작한다.
func (s *server) provRuns() []string {
	es, err := os.ReadDir(s.provDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range es {
		if e.IsDir() && isFile(filepath.Join(s.provDir(), e.Name(), provManifestFile)) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func readProvManifest(dir string) (provManifest, bool) {
	var m provManifest
	b, err := os.ReadFile(filepath.Join(dir, provManifestFile))
	return m, err == nil && json.Unmarshal(b, &m) == nil && m.ID != ""
}

// recoverProv: 시작할 때 부른다. 결과가 없는 실행 자리는 화면이나 감독이 도중에 끝난 것이다. 그 실행의 프로세스를 끝내고,
// 「감독이 결과를 남기지 못함」으로 적고, 비밀을 지운다. 실행 기록은 그대로 둔다: 어디까지 기록됐는지가 화면이 말할 수 있는
// 전부다.
func (s *server) recoverProv() {
	if s.planOut == "" {
		return
	}
	for _, id := range s.provRuns() {
		dir := filepath.Join(s.provDir(), id)
		if isFile(filepath.Join(dir, provResultFile)) {
			removeProvSecrets(dir)
			continue
		}
		if m, ok := readProvManifest(dir); ok {
			s.finishProv(dir, m, 0)
		}
	}
}

// stopProv: 도는 실행을 멈춘다. 감독에게 끝내라고 알린다. 끝났음을 알리는 채널을 돌려준다(돌고 있지 않으면 nil).
func (s *server) stopProv() <-chan struct{} {
	p := &s.prov
	p.mu.Lock()
	defer p.mu.Unlock()
	j := p.job
	if j == nil || j.closed() || j.cmd == nil || j.cmd.Process == nil {
		return nil
	}
	terminate(j.cmd)
	return j.done
}

func (s *server) shutdownProv() {
	if done := s.stopProv(); done != nil {
		select {
		case <-done:
		case <-time.After(3*stopGrace + 5*time.Second):
		}
	}
}

// provRun: 실행 자리 하나를 읽은 것.
type provRun struct {
	dir string
	m   provManifest
}

// provPicks: 화면이 보일 실행 셋. 가장 최근의 적용 실행, 그 실행에 연결한 가장 최근의 재확인, 가장 최근의 미리 보기(재확인이
// 아닌 것). 도는 실행(skip)은 뺀다. **관계는 실행 자리의 기록(run.json의 recheck_of)에 있다.** 화면을 새로 받거나 다시 띄워도
// 같은 실행과 재확인이 이어진다.
func (s *server) provPicks(skip string) (apply, recheck, preview *provRun, newest string) {
	all := s.provAll()
	for k := len(all) - 1; k >= 0; k-- {
		r := &all[k]
		if r.m.ID == skip {
			continue
		}
		if newest == "" && r.m.RecheckOf == "" {
			newest = r.m.ID
		}
		switch {
		case !r.m.Check && apply == nil:
			apply = r
		case r.m.Check && r.m.RecheckOf == "" && preview == nil:
			preview = r
		}
	}
	for k := len(all) - 1; k >= 0 && apply != nil && recheck == nil; k-- {
		if r := &all[k]; r.m.ID != skip && r.m.Check && r.m.RecheckOf == apply.m.ID {
			recheck = r
		}
	}
	return apply, recheck, preview, newest
}

// provAll: 읽을 수 있는 실행 자리 모두, 시작한 순서대로. **ID의 글자 순서로 세지 않는다**: ID의 시각은 초까지라, 같은 초에
// 시작한 두 실행은 뒤의 무작위 부분으로 순서가 뒤바뀐다. 기록의 시작 시각으로 센다.
func (s *server) provAll() []provRun {
	var out []provRun
	for _, id := range s.provRuns() {
		dir := filepath.Join(s.provDir(), id)
		if m, ok := readProvManifest(dir); ok {
			out = append(out, provRun{dir: dir, m: m})
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if !out[a].m.Started.Equal(out[b].m.Started) {
			return out[a].m.Started.Before(out[b].m.Started)
		}
		return out[a].m.ID < out[b].m.ID
	})
	return out
}

// provResult: 실행 하나의 판정을 화면 모델로.
func provResult(r *provRun) (*ui.ProvResult, provrun.Verdict) {
	vd, res := provVerdict(r.dir, r.m)
	out := &ui.ProvResult{ID: r.m.ID, Check: r.m.Check, Started: r.m.Started, Outcome: string(vd.Outcome), End: vd.End,
		RecordWhole: vd.RecordWhole, Stall: r.m.Stall}
	if res != nil {
		out.Ended = res.Ended
	}
	for _, n := range vd.Nodes {
		pn := ui.ProvNode{Node: n.Node, State: string(n.State), Why: n.Why}
		for _, t := range n.Tasks {
			pn.Tasks = append(pn.Tasks, ui.ProvTask{Name: t.Name, Status: t.Status})
		}
		out.Nodes = append(out.Nodes, pn)
	}
	return out, vd
}

// recheckOf: 재확인이 한 노드에 대해 말하는 것. 모든 작업이 ok면 「파일 일치」, 바뀔 작업이 있으면 「다름」이다. 재확인이 그 노드의
// 모든 작업을 확인하지 못했으면(앞 노드에서 멈췄거나, 접속하지 못했거나, 결과를 받지 못했으면) 「재확인하지 못함」이다.
func recheckOf(n provrun.NodeVerdict) string {
	if n.State != provrun.NodeDone {
		return ui.RecheckUnreached
	}
	for _, t := range n.Tasks {
		if t.Status != provrun.TaskOK {
			return ui.RecheckDiffers
		}
	}
	return ui.RecheckMatch
}

// provModel: 06의 실행 구역. 조건, 도는 실행, 마지막 적용 실행과 그 재확인, 마지막 미리 보기와 적용 권한의 상태.
func (s *server) provModel(v ui.ApproveView) ui.ProvRun {
	out := ui.ProvRun{}
	rd, blocks := s.provCheck(v)
	out.Blocks = blocks
	for _, pl := range rd.plays {
		out.Targets = append(out.Targets, pl.Node)
	}
	s.prov.mu.Lock()
	if j := s.prov.job; j != nil && !j.closed() {
		out.Running, out.RunningID = true, j.m.ID
	}
	s.prov.mu.Unlock()
	apply, recheck, preview, newest := s.provPicks(out.RunningID)
	if apply != nil {
		out.Apply, _ = provResult(apply)
		// 재확인은 적용한 것과 같은 묶음으로만 한다. 입력이 바뀌었으면 그 실행의 기대 상태를 다시 잴 수 없다.
		out.RecheckSame = len(blocks) == 0 && rd.binding.Digest() == apply.m.Binding.Digest()
		if recheck != nil {
			rr, vd := provResult(recheck)
			out.Apply.RecheckID, out.Apply.RecheckAt = rr.ID, rr.Ended
			if rr.Ended.IsZero() {
				out.Apply.RecheckAt = rr.Started
			}
			by := map[string]string{}
			for _, n := range vd.Nodes {
				by[n.Node] = recheckOf(n)
			}
			for k := range out.Apply.Nodes {
				out.Apply.Nodes[k].Recheck = by[out.Apply.Nodes[k].Node]
				if out.Apply.Nodes[k].Recheck == "" {
					out.Apply.Nodes[k].Recheck = ui.RecheckUnreached
				}
			}
		}
	}
	if preview != nil {
		out.Last, _ = provResult(preview)
		// 적용 권한은 가장 최근 실행이 이 미리 보기일 때만 본다(그 뒤에 적용 실행이 있었으면 이미 지난 미리 보기다). 묶음은 지금 다시 잰다.
		if g, err := provrun.ReadGrant(preview.dir); err == nil && g != nil && preview.m.ID == newest {
			digest := ""
			if len(blocks) == 0 {
				digest = rd.binding.Digest()
			}
			out.Last.Grant, out.Last.GrantExpires = string(g.State(s.now(), digest, provrun.Used(preview.dir))), g.Expires
		}
	}
	return out
}

// provPreview: 미리 보기를 시작한다(POST). 조건이 안 맞으면 409다.
func (s *server) provPreview(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenApprove
	if s.planOut == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	v, err := s.approveModel(ui.Page{})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	rd, blocks := s.provCheck(v)
	if len(blocks) > 0 {
		s.refuse(w, r, http.StatusConflict, ui.RefusedProvNotReady, back, nil)
		return
	}
	if err := s.startProv(rd, true, ""); err != nil {
		if errors.Is(err, errProvBusy) {
			s.refuse(w, r, http.StatusConflict, ui.RefusedProvBusy, back, nil)
			return
		}
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, err)
		return
	}
	http.Redirect(w, r, back+"?"+ui.LangParam+"="+string(ui.PickLang(r))+"#approve-run", http.StatusSeeOther)
}

// provRecheck: 마지막 적용 실행을 재확인한다(POST). 그 실행과 같은 묶음일 때만 한다. 미리 보기와 같은 실행이고, 그 적용
// 실행의 ID를 기록에 남긴다.
func (s *server) provRecheck(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenApprove
	if s.planOut == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	v, err := s.approveModel(ui.Page{})
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	rd, blocks := s.provCheck(v)
	apply, _, _, _ := s.provPicks("")
	if len(blocks) > 0 || apply == nil || rd.binding.Digest() != apply.m.Binding.Digest() {
		s.refuse(w, r, http.StatusConflict, ui.RefusedProvNotReady, back, nil)
		return
	}
	if err := s.startProv(rd, true, apply.m.ID); err != nil {
		if errors.Is(err, errProvBusy) {
			s.refuse(w, r, http.StatusConflict, ui.RefusedProvBusy, back, nil)
			return
		}
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, err)
		return
	}
	http.Redirect(w, r, back+"?"+ui.LangParam+"="+string(ui.PickLang(r))+"#approve-run", http.StatusSeeOther)
}

// provStop: 도는 실행을 멈춘다(POST).
func (s *server) provStop(w http.ResponseWriter, r *http.Request) {
	if s.planOut == "" {
		notFound(w, r)
		return
	}
	s.stopProv()
	http.Redirect(w, r, ui.ScreenApprove+"?"+ui.LangParam+"="+string(ui.PickLang(r))+"#approve-run", http.StatusSeeOther)
}

// provStatus: 도는 동안 화면이 몇 초마다 받는 조각(GET). 끝났으면 화면을 통째로 새로 받게 한다.
func (s *server) provStatus(w http.ResponseWriter, r *http.Request) {
	if s.planOut == "" {
		notFound(w, r)
		return
	}
	s.prov.mu.Lock()
	running := s.prov.job != nil && !s.prov.job.closed()
	s.prov.mu.Unlock()
	if !running {
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	html(w, r, func(out io.Writer) error { return ui.RenderProvRunning(out, ui.PickLang(r), csrfToken(r)) })
}
