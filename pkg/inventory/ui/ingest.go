// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"context"
	"io"
	"strings"
	"time"
)

// 03의 이력 적재를 막는 것.
const (
	IngestViewOnly    = "view-only"
	IngestNoResults   = "no-results"
	IngestNoVerifyKey = "no-verify-key"
	IngestPolicy      = "policy"
	IngestInputs      = "inputs"
	IngestObserving   = "observing"
	// IngestNoCandidate — 활성화할 확정 정책으로 적재할 수 없다: 02에서 확정한 정책이 없거나, 확정한 뒤 바뀌었거나, 활성 정책과 같다.
	IngestNoCandidate = "no-candidate"
)

// 결과 파일 하나가 적재되지 못하는 까닭. 하나라도 있으면 묶음 전체를 적재하지 않는다.
const (
	IngestWhyNotFile    = "not-file"
	IngestWhyUnreadable = "unreadable"
	IngestWhyNotResult  = "not-result"
	IngestWhyUnsigned   = "unsigned"
	IngestWhyNoKey      = "no-key"
	IngestWhyOtherKey   = "other-key"
	IngestWhyMismatch   = "mismatch"
)

// IngestProblem — 파일 · 수집기 ID · 판정 사유.
type IngestProblem struct {
	File, Collector, Why string
}

// Text — 사유의 문구.
func (p IngestProblem) Text(l Lang) string { return ingestWhyText[p.Why].In(l) }

// IngestNode — 노드 하나에 대해 이력이 말하는 것. Want는 이 결과와 정책으로 기대하는 참조, Before는 적재 전의 최신 참조,
// Latest는 지금의 최신 참조다. Body(스냅샷 본체가 있는가)와 Current(최신 참조가 그것인가)는 따로다. Unknown이면 이력을 읽지
// 못했다: 확인할 수 없는 것을 확인한 것처럼 보이지 않는다.
type IngestNode struct {
	Node, Want, Before, Latest string
	Body, Current, Unknown     bool
}

// State — 노드 상태의 코드.
func (n IngestNode) State() string {
	switch {
	case n.Unknown:
		return "unknown"
	case n.Current:
		return "current"
	case n.Body:
		return "body-only"
	}
	return "absent"
}

// StateText — 노드 상태의 문구.
func (n IngestNode) StateText(l Lang) string { return ingestStateText[n.State()].In(l) }

// short — 참조를 앞 열두 자로. 없으면 「없음」이다.
func shortRef(l Lang, d string) string {
	switch {
	case d == "":
		return tIngestNone.In(l)
	case len(d) > 12:
		return d[:12]
	}
	return d
}

// shortPolicy — 정책 지문을 알아볼 수 있게 줄인다. 지문은 「형식 이름:해시」 꼴이라 앞에서 자르면 형식 이름만 남는다.
// 그래서 형식 이름은 두고 해시만 앞 열두 자로 줄인다.
func shortPolicy(l Lang, d string) string {
	i := strings.LastIndex(d, ":")
	if i < 0 {
		return shortRef(l, d)
	}
	return d[:i+1] + shortRef(l, d[i+1:])
}

// IngestLast — 가장 최근 적재의 기록. Nodes는 **지금 이력을 다시 읽은** 상태다.
type IngestLast struct {
	ID             string
	Started, Ended time.Time
	Files          int
	PolicyDigest   string
	// Candidate — 02에서 확정한 새 정책으로 적재한 실행이다(그때 활성 정책이 아니었다). 거짓이면 활성 정책으로 적재했다.
	Candidate bool
	// Running — 적재 프로세스가 아직 돈다. HasResult — 프로세스가 결과를 남겼다. 남기지 못했으면 종료를 알 수 없다.
	Running, HasResult bool
	// Latest — 가장 최근 적재다. 거짓이면 주소로 요청한 옛 실행을 보이고 있다.
	Latest bool
	// Unchanged — 노드마다 본체 · 최신 참조 · 관측 기록 수가 적재 전과 같다고 **확인했다.** 확인하지 못했으면 거짓이다.
	Unchanged          bool
	Reason, Detail     string
	Accepted, Rejected int
	OffScope, Changed  int
	Excluded           int
	Nodes              []IngestNode
}

// Outcome — 적재 하나의 판정 코드. **이력을 다시 읽은 것으로 말한다.**
func (r IngestLast) Outcome() string {
	all, any, unknown := len(r.Nodes) > 0, false, false
	for _, n := range r.Nodes {
		unknown = unknown || n.Unknown
		all = all && n.Current
		any = any || n.Current
	}
	switch {
	case r.Running:
		return "running"
	case unknown:
		return "unknown"
	case r.Unchanged:
		return "unchanged"
	case all:
		return "recorded"
	case any:
		return "partial"
	}
	return "nothing"
}

// OutcomeText — 판정의 문구.
func (r IngestLast) OutcomeText(l Lang) string { return ingestOutcomeText[r.Outcome()].In(l) }

// Failed — 적재가 끝까지 가지 못했다(실패했거나, 결과를 남기지 못하고 끝났다).
func (r IngestLast) Failed() bool { return !r.Running && (!r.HasResult || r.Reason != "") }

// ReasonText — 적재 프로세스가 끝난 까닭의 문구. 실패면 「적재에 실패했습니다.」로 시작하고, 까닭과 갈래의 안내가 잇는다.
// **오류의 문장은 싣지 않는다**: 갈래(코드)로 고른 문구만이다.
func (r IngestLast) ReasonText(l Lang) string {
	switch {
	case r.Running:
		return ""
	case !r.HasResult:
		return tIngestFailed.In(l) + " " + tIngestNoResult.In(l)
	case r.Reason == "":
		return tIngestCounts.F(l, r.Accepted, r.Changed, r.Excluded)
	}
	s := tIngestFailed.In(l)
	if t, ok := ingestReasonText[r.Reason]; ok {
		s += " " + t.In(l)
	}
	if t, ok := ingestDetailText[r.Detail]; ok {
		s += " " + t.In(l)
	}
	return s
}

// IngestView — 03의 적재 구역.
type IngestView struct {
	// Configured — 화면에서 적재하도록 설정됐다(조회용 PQCOTA_READONLY_DSN과 적재용 PQCOTA_DSN이 둘 다 있다).
	Configured bool
	// ReadVar — 화면이 조회에 쓰는 변수의 이름.
	ReadVar      string
	Block        string
	Problems     []IngestProblem
	Files        int
	PolicyDigest string
	NoPolicy     bool
	Observed     int
	Managed      int
	Nodes        []IngestNode
	// StateUnknown — 이력의 지금 상태를 읽지 못했다. AllCurrent — 모든 노드의 최신 참조가 이미 이 결과 · 정책의 것이다.
	StateUnknown, AllCurrent, Running bool
	Last                              *IngestLast
}

// CanIngest — 적재 확인으로 가는 버튼을 낼 수 있는가.
func (v IngestView) CanIngest() bool {
	return v.Configured && v.Block == "" && len(v.Problems) == 0 && len(v.Nodes) > 0 && !v.StateUnknown && !v.AllCurrent && !v.Running
}

// BlockText — 막는 것의 문구.
func (v IngestView) BlockText(l Lang) string { return ingestBlockText[v.Block].In(l) }

// IngestConfirmLines: 적재 확인 페이지에 적을 것. 조직, 활성 정책의 지문, 검증된 결과 파일 수, 정책으로 제외되는 자산 수, 노드마다
// 기대하는 참조와 지금의 최신 참조, 그리고 되돌릴 수 없다는 것.
//
// candidate면 02에서 확정한 새 정책으로 적재한다. 그때는 적재에 쓸 정책과 현재 활성 정책(active)의 지문을 나란히 적고, 적재가
// 끝나도 활성 정책이 바뀌지 않는다는 것을 밝힌다.
func IngestConfirmLines(l Lang, org, policy string, noPolicy, candidate bool, active string, files, observed, managed int, nodes []IngestNode) []string {
	out := []string{tIngestLineOrg.F(l, org)}
	switch {
	case candidate:
		out = append(out, tIngestLineCandidate.F(l, policy), tIngestLineActiveStays.F(l, active))
	case noPolicy:
		out = append(out, tIngestLineNoPolicy.In(l))
	default:
		out = append(out, tIngestLinePolicy.F(l, policy))
	}
	out = append(out, tIngestLineFiles.F(l, files), tIngestLineExcluded.F(l, observed-managed, observed))
	for _, n := range nodes {
		out = append(out, tIngestLineNode.F(l, n.Node, shortRef(l, n.Want), shortRef(l, n.Latest), n.StateText(l)))
	}
	return append(out, tIngestLineSameDB.In(l), tIngestLineAppendOnly.In(l))
}

// ObservePolicy — 03의 「활성화할 확정 정책: 재적재와 활성화」. Offer의 From · To가 현재 활성 정책과 활성화할 확정 정책의 지문이다.
// Ingest는 **활성화할 확정 정책으로** 적재한다면의 구역이다(노드별 기대 참조와 이력의 상태).
type ObservePolicy struct {
	Offer                     ActivationOffer
	ActiveFile, CandidateFile string
	Ingest                    IngestView
	// ActivateRev — 활성화 양식에 실을 rev. 활성화는 판정 세션 · 두 정책 파일 · 결과 폴더를 함께 재므로 03의 rev와 다르다.
	ActivateRev string
}

// Reingested — 모든 노드의 최신 참조가 활성화할 확정 정책의 것이라고 **확인했다.** 이력을 읽지 못했거나 기대 참조를 계산하지
// 못했으면 거짓이다.
func (p ObservePolicy) Reingested() bool {
	return p.Ingest.Block == "" && len(p.Ingest.Problems) == 0 && !p.Ingest.StateUnknown && p.Ingest.AllCurrent
}

// CanActivate — 활성화 확인으로 가는 단추를 낼 수 있는가. 재적재가 확인됐고 적재가 돌고 있지 않을 때다. 단추를 눌러도 확인
// 페이지를 거치고, 활성화 직전에 이력을 처음부터 다시 대조한다.
func (p ObservePolicy) CanActivate() bool {
	return p.Reingested() && !p.Ingest.Running && p.ActivateRev != ""
}

// IngestConfirmTitle: 적재 확인 페이지의 제목.
func IngestConfirmTitle(l Lang) string { return tIngestConfirmTitle.In(l) }

// RenderIngestRunning — 도는 동안 다시 받는 조각.
func RenderIngestRunning(w io.Writer, l Lang) error {
	return ingestRunning(l).Render(context.Background(), w)
}
