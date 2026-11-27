// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// 03에서 하는 이력 적재. 지금 `results/`의 서명된 결과를 **활성 정책으로** pqcota 이력에 적재한다. 관측이 끝났다고 스스로
// 적재하지 않는다: 운영자가 확인 화면을 거쳐 누른다.
//
// **적재는 상류 함수다**(ingest.IngestWith). `pqcota-ingest`가 하는 것과 같은 적재다. 다른 점은 하나다: 상류 명령은 서명이
// 맞지 않는 결과만 빼고 나머지를 적재하지만, 화면은 **하나라도 서명이 없거나 맞지 않으면 묶음 전체를 적재 전에 거절한다.**
//
// 화면 프로세스는 쓰기 연결을 갖지 않는다. 적재는 자기 바이너리를 적재 모드(`-ingest-run <실행 자리>`)로 띄운 별도 프로세스가
// 하고, 적재용 연결 정보(PQCOTA_DSN의 값)는 환경이 아니라 표준 입력으로 받는다. 화면에서 적재하려면 조회용
// PQCOTA_READONLY_DSN과 적재용 PQCOTA_DSN이 둘 다 있어야 한다(loadDSNs).
//
// **운영자가 확인한 묶음과 적재하는 묶음이 같아야 한다.**
//
//   - 확인 화면은 묶음(결과 파일들 · 활성 정책 · 신뢰할 공개 키 · 조직 · 규칙 판)의 해시를 확인 토큰에 묶는다.
//   - 적재를 시작할 때 묶음을 다시 재어 같아야 하고, 그때 읽은 바이트를 실행 자리에 사본으로 둔다.
//   - 적재 프로세스는 그 사본만 읽고, 읽기 전에 사본의 해시를 다시 견준다. 서명도 사본으로 다시 확인한다.
//   - 확인 토큰은 한 번만 쓴다(쓴 표시를 디스크에 새로 만든다). 더블클릭이나 같은 요청의 재전송으로 두 번째 적재가 시작되지
//     않는다.
//
// **적재 프로세스가 도중에 끝나면 종료 코드로 말하지 않는다.** 이력을 다시 읽어, 노드마다 스냅샷 본체가 있는지와 최신 참조가
// 그것인지를 따로 보인다(upstream.States). 조회하지 못하면 「확인할 수 없음」이다.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/ingest"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/normalize"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/resultio"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/report"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

const (
	ingestRunFlag      = "-ingest-run"
	kindIngest         = "ingest"
	wsIngestRuns       = "ingest-runs"
	ingestManifestFile = "run.json"
	ingestResultFile   = "result.json"
	ingestPidFile      = "pid"
	ingestResultsCopy  = "results"
	ingestPolicyCopy   = "policy.csv"
	ingestTokens       = ".confirmed"
)

// ingestFile — 적재할 결과 파일 하나.
type ingestFile struct {
	Name       string   `json:"name"`
	SHA        string   `json:"sha256"`
	Collectors []string `json:"collectors"`
	Nodes      []string `json:"nodes"`
}

// ingestManifest — 적재 하나의 기록. 적재 프로세스를 띄우기 전에 쓴다. 비밀이 없다(공개 키는 비밀이 아니다).
type ingestManifest struct {
	ID      string       `json:"id"`
	Started time.Time    `json:"started"`
	Org     string       `json:"org"`
	Files   []ingestFile `json:"files"`
	// NoPolicy: 활성 정책 파일이 없다(모든 자산이 관리 대상). PolicySHA: 정책 사본의 해시. PolicyDigest: 정책의 지문.
	NoPolicy     bool   `json:"no_policy,omitempty"`
	PolicySHA    string `json:"policy_sha256"`
	PolicyDigest string `json:"policy_digest"`
	// Keys: 신뢰할 수집기 공개 키(수집기 ID → 키들). 03의 verify.pub에서 읽은 것이다.
	Keys    map[string][]string `json:"keys"`
	Binding string              `json:"binding"`
	// Refs: 이 결과와 정책으로 기대하는 노드별 참조. Before: 적재 전의 이력 상태.
	Refs     []upstream.Ref       `json:"refs"`
	Before   []upstream.NodeState `json:"before"`
	Observed int                  `json:"observed"`
	Managed  int                  `json:"managed"`
	// ReadDB: 화면이 조회하는 데이터베이스의 식별. 적재 프로세스가 적재용 연결의 것과 견준다.
	ReadDB string `json:"read_db"`
}

// 적재 프로세스가 끝난 까닭. 비면 적재가 끝까지 갔다.
const (
	ingestInputsChanged = "inputs-changed" // 사본이 기록과 다르다
	ingestDecode        = "decode"         // 해독하지 못한 결과가 있다
	ingestSignature     = "signature"      // 서명이 없거나 맞지 않는 결과가 있다
	ingestOtherDB       = "other-db"       // 적재용 연결이 조회하는 데이터베이스가 아니다
	ingestStore         = "store"          // 이력을 열지 못했다
	ingestFailed        = "ingest-failed"  // 적재하다 실패했다
)

// ingestResult — 적재 프로세스가 끝에 적는 것. Detail은 분류뿐이다(연결 정보나 서버의 문구를 싣지 않는다).
type ingestResult struct {
	Exit      int       `json:"exit"`
	Reason    string    `json:"reason,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Ended     time.Time `json:"ended"`
	Accepted  int       `json:"accepted"`
	Rejected  int       `json:"rejected"`
	OffScope  int       `json:"off_scope"`
	Snapshots int       `json:"snapshots"`
	Changed   int       `json:"changed"`
	Excluded  int       `json:"excluded"`
	Nodes     []string  `json:"nodes,omitempty"`
}

type ingestRunner struct {
	mu   sync.Mutex
	done chan struct{} // 도는 적재가 끝나면 닫힌다. 돌고 있지 않으면 nil이거나 닫혀 있다
}

func (r *ingestRunner) running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done == nil {
		return false
	}
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}

func (s *server) ingestDir() string { return filepath.Join(filepath.Dir(s.results), wsIngestRuns) }

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ingestPlan — 지금 적재한다면 무엇을 적재하는가. 화면 · 확인 · 시작이 같은 함수로 잰다.
type ingestPlan struct {
	// block: 적재를 막는 것의 코드(ui.Ingest…). 비면 막는 것이 없다.
	block    string
	problems []ui.IngestProblem
	files    []ingestFile
	bodies   map[string][]byte
	policy   []byte
	noPolicy bool
	digest   string
	keys     map[string][]string
	keysSHA  string
	filesSHA string
	org      string
	refs     []upstream.Ref
	observed int
	managed  int
	binding  string
}

// ingestPlan: 결과 폴더 · 활성 정책 · 신뢰할 공개 키를 읽어 묶음을 잰다. 서명은 여기서 전부 확인한다.
func (s *server) ingestPlan() ingestPlan {
	p := ingestPlan{org: s.orgName(), bodies: map[string][]byte{}}
	if s.st == nil || s.recovering || len(s.diag) > 0 {
		p.block = ui.IngestViewOnly
		return p
	}
	if s.results == "" {
		p.block = ui.IngestNoResults
		return p
	}
	trusted := readCollectorVerify(s.keyHome(), s.collectorVerify)
	if trusted.State != ui.KeyOK {
		p.block = ui.IngestNoVerifyKey
		return p
	}
	p.keys = trusted.Keys
	kb, _ := json.Marshal(sortedKeys(trusted.Keys))
	p.keysSHA = sha(kb)

	if s.scopeAssets != "" {
		b, err := os.ReadFile(s.scopeAssets)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			p.noPolicy = true
		case err != nil:
			p.block = ui.IngestPolicy
			return p
		default:
			p.policy = b
		}
	} else {
		p.noPolicy = true
	}
	var err error
	if p.noPolicy {
		p.digest, err = scope.PolicyFileDigest("")
	} else {
		p.digest, err = scope.PolicyFileDigest(s.scopeAssets)
	}
	if err != nil {
		p.block = ui.IngestPolicy
		return p
	}

	ents, err := os.ReadDir(s.results)
	if err != nil {
		p.block = ui.IngestNoResults
		return p
	}
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == runManifestFile || (!strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".jsonl")) {
			continue
		}
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() {
			p.problems = append(p.problems, ui.IngestProblem{File: name, Why: ui.IngestWhyNotFile})
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.results, name))
		if err != nil {
			p.problems = append(p.problems, ui.IngestProblem{File: name, Why: ui.IngestWhyUnreadable})
			continue
		}
		res, flaws := resultio.Decode(b)
		if len(flaws) > 0 || len(res) == 0 {
			p.problems = append(p.problems, ui.IngestProblem{File: name, Why: ui.IngestWhyNotResult})
			continue
		}
		f := ingestFile{Name: name, SHA: sha(b)}
		for _, r := range res {
			id, node := r.GetEnvelope().GetCollectorId(), r.GetEnvelope().GetTargetNodeId()
			if !contains(f.Collectors, id) {
				f.Collectors = append(f.Collectors, id)
			}
			if !contains(f.Nodes, node) {
				f.Nodes = append(f.Nodes, node)
			}
			if why, _ := signatureProblem(trusted.Keys, r); why != "" {
				p.problems = append(p.problems, ui.IngestProblem{File: name, Collector: id, Why: signatureWhy(why)})
			}
		}
		p.files = append(p.files, f)
		p.bodies[name] = b
	}
	sort.Slice(p.files, func(a, b int) bool { return p.files[a].Name < p.files[b].Name })
	if len(p.files) == 0 && len(p.problems) == 0 {
		p.block = ui.IngestNoResults
		return p
	}
	h, fh := sha256.New(), sha256.New()
	fmt.Fprintf(h, "pqcaton ingest/1\n%s\n%s\n%v\n%s\n%s\n", p.org, normalize.RulesetVersion, p.noPolicy, sha(p.policy), p.keysSHA)
	for _, f := range p.files {
		fmt.Fprintf(h, "%s\x00%s\n", f.Name, f.SHA)
		fmt.Fprintf(fh, "%s\x00%s\n", f.Name, f.SHA)
	}
	// 서명이 맞지 않게 된 파일도 「결과 파일이 바뀌었다」에 들어야 한다. 걸린 파일의 이름과 까닭을 함께 넣는다.
	for _, pr := range p.problems {
		fmt.Fprintf(fh, "!%s\x00%s\x00%s\n", pr.File, pr.Collector, pr.Why)
	}
	p.filesSHA = hex.EncodeToString(fh.Sum(nil))
	fmt.Fprintf(h, "%s\n", p.filesSHA)
	p.binding = hex.EncodeToString(h.Sum(nil))
	return p
}

// parts: 묶음의 세 조각(결과 파일 · 활성 정책 · 신뢰할 공개 키)의 지문. 확인 뒤에 무엇이 바뀌었는지 가르는 데 쓴다.
func (p ingestPlan) parts() [3]string {
	return [3]string{p.filesSHA, fmt.Sprint(p.noPolicy) + ":" + sha(p.policy), p.keysSHA}
}

// ingestChanged: 확인한 조각(confirmed)과 지금의 조각을 견줘 거절 종류를 고른다. 하나만 바뀌었으면 그것을 짚고, 둘 이상이거나
// 셋이 그대로인데 묶음이 다르면(조직 · 규칙 판) 뭉뚱그린 종류다.
func ingestChanged(confirmed []string, now [3]string) string {
	kinds := [3]string{ui.RefusedIngestChangedResults, ui.RefusedIngestChangedPolicy, ui.RefusedIngestChangedKeys}
	var changed []string
	for i := range now {
		if i >= len(confirmed) || confirmed[i] != now[i] {
			changed = append(changed, kinds[i])
		}
	}
	if len(changed) == 1 {
		return changed[0]
	}
	return ui.RefusedIngestChanged
}

// sortedKeys: 공개 키 지도를 순서가 정해진 꼴로(해시에 쓴다).
func sortedKeys(m map[string][]string) [][]string {
	var out [][]string
	for id, ks := range m {
		row := append([]string{id}, ks...)
		sort.Strings(row[1:])
		out = append(out, row)
	}
	sort.Slice(out, func(a, b int) bool { return out[a][0] < out[b][0] })
	return out
}

// signatureWhy: signatureProblem의 까닭을 화면의 코드로. 문구의 앞머리로 가른다(같은 파일의 함수다).
func signatureWhy(why string) string {
	switch {
	case why == "not signed":
		return ui.IngestWhyUnsigned
	case strings.HasPrefix(why, "no trusted public key"):
		return ui.IngestWhyNoKey
	case strings.HasPrefix(why, "signed with the key listed for"):
		return ui.IngestWhyOtherKey
	}
	return ui.IngestWhyMismatch
}

// expected: 묶음의 결과와 정책으로 기대하는 노드별 참조와 자산 수. dir은 결과가 있는 폴더(원본 또는 사본)다.
func (s *server) ingestExpected(p *ingestPlan, dir, policyPath string) error {
	wd := s.load()
	if wd.declErr != nil {
		return wd.declErr
	}
	policy, err := review.LoadAssetPolicy(policyPath)
	if err != nil {
		return err
	}
	r, err := report.BuildWith(dir, wd.decl, policy)
	if err != nil {
		return err
	}
	p.refs, p.observed, p.managed = upstream.Refs(r), r.ObservedAssets, r.ManagedAssets
	return nil
}

func (s *server) activePolicyPath(p ingestPlan) string {
	if p.noPolicy {
		return ""
	}
	return s.scopeAssets
}

// ingestView: 03의 적재 구역.
func (s *server) ingestView(runID string) ui.IngestView {
	v := ui.IngestView{Running: s.ingest.running()}
	cfg, err := s.loadDSNs()
	v.Configured = err == nil && cfg.ingest != ""
	v.ReadVar = s.dsnReadVar
	p := s.ingestPlan()
	v.Block, v.Problems, v.Files, v.PolicyDigest, v.NoPolicy = p.block, p.problems, len(p.files), p.digest, p.noPolicy
	if s.run.running() {
		v.Block = ui.IngestObserving
	}
	if v.Block == "" && len(p.problems) == 0 {
		if err := s.ingestExpected(&p, s.results, s.activePolicyPath(p)); err != nil {
			v.Block = ui.IngestInputs
		} else {
			v.Observed, v.Managed = p.observed, p.managed
			states, err := upstream.States(s.upstream, p.refs)
			if err != nil {
				v.StateUnknown = true
				for _, r := range p.refs {
					v.Nodes = append(v.Nodes, ui.IngestNode{Node: r.Node, Want: r.Digest, Unknown: true})
				}
			} else {
				v.AllCurrent = upstream.AllCurrent(states)
				for _, st := range states {
					v.Nodes = append(v.Nodes, ui.IngestNode{Node: st.Node, Want: st.Want, Latest: st.Latest, Body: st.Body, Current: st.Current})
				}
			}
		}
	}
	v.Last = s.ingestLast(runID)
	return v
}

// ingestLast: 가장 최근 적재의 기록과, **지금 이력을 다시 읽은** 노드별 상태.
//
// runID가 있고 그 실행이 있으면 그 실행을 보인다(이미 쓴 확인으로 온 사람을 그 확인이 시작한 실행으로 데려간다). 없으면 가장
// 최근 것이다.
func (s *server) ingestLast(runID string) *ui.IngestLast {
	runs := s.ingestRuns()
	if len(runs) == 0 {
		return nil
	}
	dir := runs[len(runs)-1]
	m, ok := readIngestManifest(dir)
	for _, d := range runs {
		if mm, found := readIngestManifest(d); found && runID != "" && mm.ID == runID {
			dir, m, ok = d, mm, true
		}
	}
	if !ok {
		return nil
	}
	out := &ui.IngestLast{ID: m.ID, Started: m.Started, Files: len(m.Files), PolicyDigest: m.PolicyDigest, Latest: dir == runs[len(runs)-1]}
	if alive(dir) {
		out.Running = true
	}
	var res ingestResult
	if b, err := os.ReadFile(filepath.Join(dir, ingestResultFile)); err == nil && json.Unmarshal(b, &res) == nil {
		out.HasResult, out.Reason, out.Detail, out.Ended = true, res.Reason, res.Detail, res.Ended
		out.Accepted, out.Rejected, out.OffScope, out.Changed, out.Excluded = res.Accepted, res.Rejected, res.OffScope, res.Changed, res.Excluded
	}
	before := map[string]upstream.NodeState{}
	for _, b := range m.Before {
		before[b.Node] = b
	}
	// 끝까지 가지 못한 적재는 관측 기록 수까지 읽는다. 「이력 변화가 없습니다」는 노드마다 본체 · 최신 참조 · 관측 기록 수가
	// 적재 전과 같다고 **확인했을 때만** 적는다.
	clean := out.HasResult && out.Reason == ""
	read := upstream.StatesWithObs
	if clean || out.Running {
		read = upstream.States
	}
	now, err := read(s.upstream, m.Refs)
	if err != nil {
		for _, r := range m.Refs {
			out.Nodes = append(out.Nodes, ui.IngestNode{Node: r.Node, Want: r.Digest, Before: before[r.Node].Latest, Unknown: true})
		}
		return out
	}
	same := len(now) > 0 && len(now) == len(m.Before) && !clean && !out.Running
	for _, st := range now {
		out.Nodes = append(out.Nodes, ui.IngestNode{Node: st.Node, Want: st.Want, Before: before[st.Node].Latest, Latest: st.Latest, Body: st.Body, Current: st.Current})
		b, had := before[st.Node]
		same = same && had && b.ObsKnown && st.ObsKnown && b.Obs == st.Obs && b.Body == st.Body && b.Latest == st.Latest
	}
	out.Unchanged = same
	return out
}

// ingestRuns: 적재 실행 자리들, 시작한 순서대로.
func (s *server) ingestRuns() []string {
	es, err := os.ReadDir(s.ingestDir())
	if err != nil {
		return nil
	}
	type run struct {
		dir string
		at  time.Time
	}
	var rs []run
	for _, e := range es {
		dir := filepath.Join(s.ingestDir(), e.Name())
		if m, ok := readIngestManifest(dir); ok && e.IsDir() {
			rs = append(rs, run{dir, m.Started})
		}
	}
	sort.SliceStable(rs, func(a, b int) bool { return rs[a].at.Before(rs[b].at) })
	var out []string
	for _, r := range rs {
		out = append(out, r.dir)
	}
	return out
}

func readIngestManifest(dir string) (ingestManifest, bool) {
	var m ingestManifest
	b, err := os.ReadFile(filepath.Join(dir, ingestManifestFile))
	return m, err == nil && json.Unmarshal(b, &m) == nil && m.ID != ""
}

// alive: 그 실행 자리의 적재 프로세스가 아직 살아 있는가(프로세스가 남긴 PID 파일로 본다). 화면이 다시 뜬 뒤에도, 앞서 띄운
// 적재가 아직 돌고 있으면 새 적재를 받지 않는다.
func alive(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, ingestPidFile))
	if err != nil {
		return false
	}
	var pid int
	if _, err := fmt.Sscan(strings.TrimSpace(string(b)), &pid); err != nil || pid <= 0 {
		return false
	}
	return pidAlive(pid)
}

// writeSync: 파일을 쓰고 디스크에 내린다(임시 파일 → fsync → 이름 바꾸기).
func writeSync(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(b, '\n'))
	serr := f.Sync()
	if cerr := f.Close(); werr != nil || serr != nil || cerr != nil {
		return errors.Join(werr, serr, cerr)
	}
	return os.Rename(p+".tmp", p)
}

// ── 확인과 시작 ─────────────────────────────────────────────────────────────

// ingestReady: 지금 적재할 수 있는가. 막히면 거절을 그리고 false다. 기대 참조와 이력 상태까지 채운 묶음을 돌려준다.
func (s *server) ingestReady(w http.ResponseWriter, r *http.Request) (ingestPlan, dsnConfig, []upstream.NodeState, bool) {
	back := ui.ScreenObserve
	cfg, err := s.loadDSNs()
	if err != nil || cfg.ingest == "" {
		s.refuse(w, r, http.StatusConflict, ui.RefusedIngestNotReady, back, nil)
		return ingestPlan{}, cfg, nil, false
	}
	return s.ingestReadyWith(w, r, s.ingestPlan(), cfg)
}

// ingestReadyWith: 이미 잰 묶음으로 ingestReady의 나머지를 본다.
func (s *server) ingestReadyWith(w http.ResponseWriter, r *http.Request, p ingestPlan, cfg dsnConfig) (ingestPlan, dsnConfig, []upstream.NodeState, bool) {
	back := ui.ScreenObserve
	if p.block != "" || len(p.problems) > 0 || s.run.running() {
		s.refuse(w, r, http.StatusConflict, ui.RefusedIngestNotReady, back, nil)
		return p, cfg, nil, false
	}
	if err := s.ingestExpected(&p, s.results, s.activePolicyPath(p)); err != nil || len(p.refs) == 0 {
		s.refuse(w, r, http.StatusConflict, ui.RefusedIngestNotReady, back, nil)
		return p, cfg, nil, false
	}
	states, err := upstream.States(s.upstream, p.refs)
	if err != nil {
		s.refuse(w, r, http.StatusConflict, ui.RefusedIngestNotReady, back, nil)
		return p, cfg, nil, false
	}
	// 모든 노드의 최신 참조가 이미 이 결과 · 정책의 것이면 적재할 것이 없다. 과거 이력에 같은 본체가 있다는 것만으로는
	// 건너뛰지 않는다(최신 참조로만 본다).
	if upstream.AllCurrent(states) {
		s.refuse(w, r, http.StatusConflict, ui.RefusedIngestNotReady, back, nil)
		return p, cfg, nil, false
	}
	return p, cfg, states, true
}

// ingestConfirm: 적재 전 확인(POST). 아무것도 쓰지 않는다.
func (s *server) ingestConfirm(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenObserve
	if s.hosts == "" || s.results == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	cur, err := s.snapshot(ui.IngestConfirmPath, func() error { return nil })
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInputBroken, err)
		return
	}
	p, _, states, ok := s.ingestReady(w, r)
	if !ok {
		return
	}
	l := ui.PickLang(r)
	var nodes []ui.IngestNode
	for _, st := range states {
		nodes = append(nodes, ui.IngestNode{Node: st.Node, Want: st.Want, Latest: st.Latest, Body: st.Body, Current: st.Current})
	}
	lines := ui.IngestConfirmLines(l, p.org, p.digest, p.noPolicy, len(p.files), p.observed, p.managed, nodes)
	// 확인마다 다른 값을 붙인다. 확인 토큰은 한 번만 쓰는데, 같은 묶음을 같은 초에 다시 확인하면 토큰이 같아져 정당한 두 번째
	// 확인이 「이미 쓴 확인」으로 거절된다.
	nonce, err := runID()
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, err)
		return
	}
	parts := p.parts()
	s.confirmPageWith(w, r, kindIngest, ui.IngestPath, "", back, store.Identity{SessionID: p.org}, cur,
		strings.Join(append([]string{p.binding, nonce}, parts[:]...), "|"), lines, nil, "ingest")
}

var errIngestBusy = errors.New("an ingest is already running")

// ingestStart: 확인을 거친 적재(POST).
func (s *server) ingestStart(w http.ResponseWriter, r *http.Request) {
	back := ui.ScreenObserve
	if s.hosts == "" || s.results == "" {
		notFound(w, r)
		return
	}
	if !s.writable(w, r, back) {
		return
	}
	// 거절의 까닭을 정확히 적으려고 순서를 이렇게 둔다.
	//  1. 이미 쓴 확인인가. 그사이 조건이 달라졌어도(이미 적재됨 · 입력이 바뀜) 까닭은 「이미 쓴 확인」이다.
	//  2. 확인 토큰이 맞고 만료 전인가.
	//  3. 확인한 묶음과 지금의 묶음이 같은가. 다르면 무엇이 바뀌었는지 짚는다.
	//  4. 이미 돌고 있는가.  5. 지금 적재할 조건이 맞는가.
	token := sha([]byte(r.PostForm.Get(ui.ConfirmTokenField)))
	if id, used := s.ingestUsed(token); used {
		s.ingestReplayed(w, r, id)
		return
	}
	payload, ok := s.confirmed(w, r, kindIngest, back, store.Identity{SessionID: s.orgName()})
	if !ok {
		return
	}
	cfg, err := s.loadDSNs()
	if err != nil || cfg.ingest == "" {
		s.refuse(w, r, http.StatusConflict, ui.RefusedIngestNotReady, back, nil)
		return
	}
	p := s.ingestPlan()
	// 확인한 묶음과 지금의 묶음이 같아야 한다(결과 파일 · 활성 정책 · 신뢰할 공개 키 · 조직 · 규칙 판). 묶음을 잴 수 없게 된
	// 경우(p.block)는 5에서 거절한다.
	if f := strings.Split(payload, "|"); p.block == "" && f[0] != p.binding {
		confirmedParts := []string{}
		if len(f) >= 5 {
			confirmedParts = f[2:5]
		}
		s.refuse(w, r, http.StatusConflict, ingestChanged(confirmedParts, p.parts()), back, nil)
		return
	}
	if s.ingestBusy() {
		s.refuse(w, r, http.StatusConflict, ui.RefusedIngestBusy, back, nil)
		return
	}
	p, cfg, states, ok := s.ingestReadyWith(w, r, p, cfg)
	if !ok {
		return
	}
	switch err := s.runIngest(p, cfg, states, token); {
	case errors.Is(err, errIngestBusy):
		s.refuse(w, r, http.StatusConflict, ui.RefusedIngestBusy, back, nil)
	case errors.Is(err, errIngestReplay):
		id, _ := s.ingestUsed(token)
		s.ingestReplayed(w, r, id)
	case errors.Is(err, errIngestOtherDB):
		s.refuse(w, r, http.StatusConflict, ui.RefusedIngestNotReady, back, nil)
	case err != nil:
		errorPage(w, r, http.StatusInternalServerError, ui.ErrInternal, err)
	default:
		http.Redirect(w, r, back+"?"+ui.LangParam+"="+string(ui.PickLang(r))+"#observe-ingest", http.StatusSeeOther)
	}
}

// ingestUsed: 그 확인 토큰으로 이미 적재를 시작했는가. 시작했으면 그 실행의 ID다(쓴 표시에 적혀 있다).
func (s *server) ingestUsed(token string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(s.ingestDir(), ingestTokens, token))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// ingestReplayed: 이미 쓴 확인의 거절. 돌아가는 주소가 그 확인이 시작한 실행의 기록이다.
func (s *server) ingestReplayed(w http.ResponseWriter, r *http.Request, id string) {
	back := ui.ScreenObserve + "?" + ui.LangParam + "=" + string(ui.PickLang(r))
	if runIDPattern.MatchString(id) {
		back += "&" + ui.IngestRunParam + "=" + id
	}
	s.refuse(w, r, http.StatusConflict, ui.RefusedIngestReplay, back+"#ingest-last", nil)
}

// runIDPattern: 실행 ID의 꼴(주소에 싣기 전에 본다).
var runIDPattern = regexp.MustCompile(`^[0-9A-Za-z-]{1,64}$`)

// ingestBusy: 적재가 돌고 있는가. 이 화면이 띄운 것과, 화면이 다시 뜨기 전에 띄운 것(PID 파일)을 함께 본다.
func (s *server) ingestBusy() bool {
	if s.ingest.running() {
		return true
	}
	for _, dir := range s.ingestRuns() {
		if alive(dir) {
			return true
		}
	}
	return false
}

var (
	errIngestReplay  = errors.New("this confirmation was already used to start an ingest")
	errIngestOtherDB = errors.New("the read connection cannot be identified")
)

// dbIdentity: 그 연결이 닿는 데이터베이스의 식별. 조회용과 적재용이 같은 데이터베이스인지 견주는 데 쓴다. 값에 연결 정보가 없다.
// 시험이 바꿔 낀다.
var dbIdentity = func(dsn string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), upstream.Timeout)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return "", err
	}
	defer conn.Close(context.Background())
	var db, oid, sys string
	if err := conn.QueryRow(ctx, `SELECT current_database(), (SELECT oid::text FROM pg_database WHERE datname = current_database())`).Scan(&db, &oid); err != nil {
		return "", err
	}
	// 서버의 식별자(클러스터마다 다르다). 읽을 권한이 없으면 주소와 포트로 대신한다. 두 연결 모두 같은 규칙으로 잰다.
	if err := conn.QueryRow(ctx, `SELECT system_identifier::text FROM pg_control_system()`).Scan(&sys); err != nil {
		var addr, port *string
		if err := conn.QueryRow(ctx, `SELECT host(inet_server_addr()), inet_server_port()::text`).Scan(&addr, &port); err != nil {
			return "", err
		}
		sys = "addr"
		if addr != nil {
			sys += ":" + *addr
		}
		if port != nil {
			sys += ":" + *port
		}
	}
	return sys + "/" + db + "/" + oid, nil
}

// ingestSpawn: 적재 프로세스를 띄우고 끝나기를 기다린다. 적재용 연결 정보는 표준 입력으로 준다(환경 · 인자에 싣지 않는다).
// 시험이 바꿔 낀다(같은 프로세스에서 ingestRun을 부른다).
var ingestSpawn = func(dir, dsn string) {
	exe, err := supervisorExe()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, ingestRunFlag, dir)
	cmd.Dir = dir
	cmd.Env = append(childEnv(), "PQCOTA_AUTO_DDL=0")
	cmd.Stdin = strings.NewReader(dsn + "\n")
	_ = cmd.Run()
}

// runIngest: 실행 자리를 만들고 적재 프로세스를 띄운다. 끝나기를 기다리지 않는다.
func (s *server) runIngest(p ingestPlan, cfg dsnConfig, before []upstream.NodeState, token string) error {
	run := &s.ingest
	run.mu.Lock()
	if run.done != nil {
		select {
		case <-run.done:
		default:
			run.mu.Unlock()
			return errIngestBusy
		}
	}
	done := make(chan struct{})
	run.done = done
	run.mu.Unlock()
	started := false
	defer func() {
		if !started {
			close(done)
		}
	}()

	// 확인 토큰은 한 번만 쓴다. 쓴 표시를 새로 만들 수 있어야 한다(같은 요청이 함께 와도 하나만 만든다).
	if err := os.MkdirAll(filepath.Join(s.ingestDir(), ingestTokens), 0o700); err != nil {
		return err
	}
	id, err := runID()
	if err != nil {
		return err
	}
	tf, err := os.OpenFile(filepath.Join(s.ingestDir(), ingestTokens, token), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return errIngestReplay
	}
	if err != nil {
		return err
	}
	// 쓴 표시에 실행 ID를 적는다. 같은 확인이 다시 오면 그 실행의 기록으로 데려간다.
	_, werr := tf.WriteString(id + "\n")
	if cerr := tf.Close(); werr != nil || cerr != nil {
		return errors.Join(werr, cerr)
	}

	readDB, err := dbIdentity(cfg.read)
	if err != nil {
		return errIngestOtherDB
	}
	// 적재 전의 상태를 관측 기록 수까지 적어 둔다. 적재가 실패했을 때 「이력이 그대로다」를 근거 있게 적으려면 필요하다.
	// 세지 못하면 앞서 읽은 상태를 그대로 둔다(그러면 그 문장은 적지 않는다).
	if withObs, err := upstream.StatesWithObs(s.upstream, p.refs); err == nil {
		before = withObs
	}
	dir := filepath.Join(s.ingestDir(), id)
	if err := os.MkdirAll(filepath.Join(dir, ingestResultsCopy), 0o700); err != nil {
		return err
	}
	// 확인할 때 읽은 바로 그 바이트를 사본으로 둔다.
	for _, f := range p.files {
		if err := os.WriteFile(filepath.Join(dir, ingestResultsCopy, f.Name), p.bodies[f.Name], 0o600); err != nil {
			return err
		}
	}
	policyCopy := ""
	if !p.noPolicy {
		policyCopy = filepath.Join(dir, ingestPolicyCopy)
		if err := os.WriteFile(policyCopy, p.policy, 0o600); err != nil {
			return err
		}
	}
	// 기대 참조는 사본으로 다시 잰다(적재 프로세스가 읽는 것과 같은 것이다).
	if err := s.ingestExpected(&p, filepath.Join(dir, ingestResultsCopy), policyCopy); err != nil {
		return err
	}
	m := ingestManifest{ID: id, Started: time.Now(), Org: p.org, Files: p.files, NoPolicy: p.noPolicy, PolicySHA: sha(p.policy),
		PolicyDigest: p.digest, Keys: p.keys, Binding: p.binding, Refs: p.refs, Before: before, Observed: p.observed, Managed: p.managed, ReadDB: readDB}
	if err := writeSync(filepath.Join(dir, ingestManifestFile), m); err != nil {
		return err
	}
	started = true
	go func() {
		defer close(done)
		ingestSpawn(dir, cfg.ingest)
	}()
	return nil
}

// ingestStatus: 도는 동안 화면이 몇 초마다 받는 조각(GET). 끝났으면 화면을 통째로 새로 받게 한다.
func (s *server) ingestStatus(w http.ResponseWriter, r *http.Request) {
	if s.hosts == "" {
		notFound(w, r)
		return
	}
	if !s.ingestBusy() {
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	html(w, r, func(out io.Writer) error { return ui.RenderIngestRunning(out, ui.PickLang(r)) })
}

// ── 적재 프로세스 ────────────────────────────────────────────────────────────

// openIngestStore: 적재용으로 이력을 연다(DDL을 돌리지 않는다: PQCOTA_AUTO_DDL=0). 시험이 바꿔 낀다.
var openIngestStore = func(dsn, org string) (history.Store, func(), error) {
	_ = os.Setenv("PQCOTA_AUTO_DDL", "0")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := history.NewPgStoreIn(ctx, dsn, org)
	if err != nil {
		return nil, nil, err
	}
	return st, st.Close, nil
}

// ingestDetail: 적재 오류의 갈래. **코드만이다.** 화면은 코드로 고른 자기 문구만 보이고 오류의 문장은 어디에도 싣지 않는다
// (연결 정보가 섞일 수 있다).
func ingestDetail(err error) string {
	switch c := upstream.Classify(err); {
	case strings.HasPrefix(c, upstream.ClassNoPrivilege):
		return ui.IngestDetailPermission
	case c == upstream.ClassSchemaMissing:
		return ui.IngestDetailSchema
	case c == "timed out":
		return ui.IngestDetailTimeout
	}
	return ""
}

// ingestMain: 적재 모드의 시작. 적재용 연결 정보를 표준 입력의 첫 줄로 받는다.
func ingestMain(dir string) int {
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return ingestRun(dir, strings.TrimRight(line, "\r\n"))
}

// ingestRun: 실행 자리의 사본을 적재한다. 종료 코드는 0(끝까지 갔다) · 3(쓰기 전에 거절) · 1(적재하다 실패)이다. **종료 코드는
// 이력의 상태를 말하지 않는다**: 화면은 이력을 다시 읽어 보인다.
func ingestRun(dir, dsn string) int {
	m, ok := readIngestManifest(dir)
	if !ok {
		return 3
	}
	_ = os.WriteFile(filepath.Join(dir, ingestPidFile), []byte(fmt.Sprint(os.Getpid())), 0o600)
	res := ingestResult{}
	end := func(exit int, reason, detail string) int {
		res.Exit, res.Reason, res.Detail, res.Ended = exit, reason, detail, time.Now()
		_ = writeSync(filepath.Join(dir, ingestResultFile), res)
		_ = os.Remove(filepath.Join(dir, ingestPidFile))
		return exit
	}

	// 1. 사본이 기록과 같은가. 기록에 없는 파일이 끼어 있어도 안 된다.
	copyDir := filepath.Join(dir, ingestResultsCopy)
	want := map[string]string{}
	for _, f := range m.Files {
		want[f.Name] = f.SHA
	}
	ents, err := os.ReadDir(copyDir)
	if err != nil || len(ents) != len(want) {
		return end(3, ingestInputsChanged, "")
	}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(copyDir, e.Name()))
		if err != nil || want[e.Name()] == "" || sha(b) != want[e.Name()] {
			return end(3, ingestInputsChanged, "")
		}
	}
	policyPath := ""
	if !m.NoPolicy {
		policyPath = filepath.Join(dir, ingestPolicyCopy)
		if b, err := os.ReadFile(policyPath); err != nil || sha(b) != m.PolicySHA {
			return end(3, ingestInputsChanged, "")
		}
	}
	policy, err := review.LoadAssetPolicy(policyPath)
	if err != nil {
		return end(3, ingestInputsChanged, "")
	}

	// 2. 해독하고 서명을 전부 확인한다. 하나라도 걸리면 아무것도 적재하지 않는다.
	results, flaws := resultio.LoadDir(copyDir)
	if len(flaws) > 0 || len(results) == 0 {
		return end(3, ingestDecode, "")
	}
	for _, r := range results {
		if why, _ := signatureProblem(m.Keys, r); why != "" {
			return end(3, ingestSignature, "")
		}
	}

	// 3. 적재용 연결이 화면이 조회하는 그 데이터베이스인가. 아니면 쓰지 않는다.
	db, err := dbIdentity(dsn)
	if err != nil {
		return end(3, ingestStore, ingestDetail(err))
	}
	if db != m.ReadDB {
		return end(3, ingestOtherDB, "")
	}

	// 4. 적재. 판정 조직으로 열고, 서명 검증은 수집기 ID에 묶은 것을 그대로 건다.
	st, closeStore, err := openIngestStore(dsn, m.Org)
	if err != nil {
		return end(3, ingestStore, ingestDetail(err))
	}
	defer closeStore()
	opts := ingest.IngestOptions{
		SnapshotPrefix: "ingest-" + time.Now().UTC().Format("20060102T150405.000Z"),
		RulesetVersion: normalize.RulesetVersion, Store: st, AssetPolicy: policy,
		VerifyCollector: func(r *discoveryv1.CollectionResult) error {
			if why, _ := signatureProblem(m.Keys, r); why != "" {
				return errors.New(why)
			}
			return nil
		},
	}
	if rs, ok := st.(ingest.RejectionStore); ok {
		opts.Rejections = rs
	}
	rep, err := ingest.IngestWith(results, opts)
	if rep != nil {
		res.Accepted, res.Rejected, res.OffScope, res.Snapshots, res.Changed, res.Excluded, res.Nodes =
			rep.Accepted, rep.Rejected, rep.OffScope, rep.Snapshots, rep.Changed, rep.ExcludedByScope, rep.Nodes
	}
	if err != nil {
		return end(1, ingestFailed, ingestDetail(err))
	}
	return end(0, "", "")
}
