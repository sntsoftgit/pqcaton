// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"strconv"
	"strings"
	"time"
)

// 선언의 노드 하나가 관측에 대해 어디에 있는가. **대조의 범위는 이것과 무관하다**: 접속 정보가 없거나 관측에 실패한 노드도
// 선언에 있으면 대조에 남는다. 이 상태는 그 노드를 이번에 관측하는가와, 지금 결과가 그 노드를 담고 있는가를 말할 뿐이다.
const (
	// TargetUnready — 선언에는 있고 접근 정보(hosts.csv)에 그 ID의 행이 없다. 「아직 관측하지 않는 노드」다.
	TargetUnready = "unready"
	// TargetReady — 접근 정보에 그 ID의 행이 있다. 다음 관측의 대상이다.
	TargetReady = "ready"
	// TargetObserved — 지금 결과에 이 노드가 있다.
	TargetObserved = "observed"
	// TargetFailed — 가장 최근 실행이 이 노드에 닿지 못했거나 이 노드의 단계가 실패했다(지금 결과는 그 실행의 것이 아니다).
	TargetFailed = "failed"
)

// TargetRow — 선언의 노드 한 줄.
type TargetRow struct {
	Node, State string
	// HasAccess — 접근 정보에 이 노드의 행이 있다.
	HasAccess bool
	// InResults — 지금 결과에 이 노드가 있다. ObservedAt은 그 결과를 낸 실행이 끝난 때이고, 화면 밖에서 받은 결과면 비어 있다.
	InResults  bool
	ObservedAt time.Time
	// FailedAt — State가 TargetFailed일 때 그 실행이 끝난 때.
	FailedAt time.Time
}

// StateText — 상태의 문구. 시각은 근거가 있을 때만 적는다.
func (r TargetRow) StateText(l Lang) string {
	at := func(t time.Time) string { return t.Local().Format("2006-01-02 15:04 MST") }
	observed := func() string {
		if r.ObservedAt.IsZero() {
			return tTargetObservedOutside.In(l)
		}
		return tTargetObserved.F(l, at(r.ObservedAt))
	}
	switch r.State {
	case TargetUnready:
		if r.InResults {
			return tTargetUnready.In(l) + " · " + observed()
		}
		return tTargetUnready.In(l)
	case TargetFailed:
		s := tTargetFailed.F(l, at(r.FailedAt))
		if r.InResults {
			s += " · " + observed()
		}
		return s
	case TargetObserved:
		return observed()
	}
	return tTargetReady.In(l)
}

// OrphanHost — 접근 정보에는 있고 선언에는 없는 행. Row는 그 행이 시작하는 파일의 줄 번호다(머리글이 1행이다). 운영자가
// 편집기에서 찾을 자리다.
type OrphanHost struct {
	Row int
	ID  string
}

// ObserveTargets — 02 관측의 「대상 노드의 접속 준비」. 줄은 **선언의 노드**이고, 접근 정보의 노드 ID는 선언의 노드 이름과
// 글자 그대로(대소문자까지) 같아야 그 노드의 것이다. `observed_as`나 IP로 잇지 않는다.
type ObserveTargets struct {
	// DeclUnknown — 선언이 없거나 읽지 못했다. 접근 정보의 ID를 견줄 기준이 없어 관측을 시작하지 않는다.
	DeclUnknown bool
	Rows        []TargetRow
	Orphans     []OrphanHost
}

// Ready — 이번 관측의 대상(접근 정보가 있는 선언 노드). Unready — 접근 정보가 없어 이번에 빠지는 선언 노드.
func (t ObserveTargets) Ready() []string   { return t.pick(true) }
func (t ObserveTargets) Unready() []string { return t.pick(false) }

func (t ObserveTargets) pick(access bool) []string {
	var out []string
	for _, r := range t.Rows {
		if r.HasAccess == access {
			out = append(out, r.Node)
		}
	}
	return out
}

// OrphanDetail — 「line 4: db-old」를 이은 것. 점검 줄과 거절 기록에 적는다.
func OrphanDetail(orphans []OrphanHost) string {
	parts := make([]string, 0, len(orphans))
	for _, o := range orphans {
		parts = append(parts, "line "+strconv.Itoa(o.Row)+": "+o.ID)
	}
	return strings.Join(parts, " · ")
}

// SurveySource — 03 대조가 **어느 결과로** 대조했는가.
type SurveySource struct {
	// Empty — 결과 폴더에 대조할 결과가 없다(성공한 관측이 아직 없거나 결과를 받지 않았다).
	Empty bool
	// At — 대조에 쓴 결과를 낸 실행이 끝난 때. 화면 밖에서 받은 결과면 비어 있고 Outside가 참이다. Nodes는 그 결과의 노드 수다.
	At      time.Time
	Outside bool
	Nodes   int
	// LastFailedAt · LastFailedState — 가장 최근 실행이 결과를 바꾸지 못하고 끝났으면 그 실행이 끝난 때와 상태. 아니면 비어 있다.
	LastFailedAt    time.Time
	LastFailedState RunState
	// Absent — 선언에는 있고 대조에 쓴 결과에는 없는 노드. 그 노드의 미관측은 「찾지 못했다」가 아니라 「그 노드를 보지 않았다」다.
	Absent []string
}

// nodeNames — 노드 이름을 쉼표로 잇는다. 많으면 앞의 스무 개와 나머지의 수만 적는다.
func nodeNames(names []string) string {
	const max = 20
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + " … +" + strconv.Itoa(len(names)-max)
}

// surveySourceKind — 대조에 쓴 결과의 갈래(시험과 스타일이 읽는다).
func surveySourceKind(s SurveySource) string {
	switch {
	case s.Empty:
		return "empty"
	case s.Outside:
		return "outside"
	}
	return "run"
}

// Short — 실행 상태의 짧은 이름. 결과를 바꾸지 못하고 끝난 상태에만 문구가 있다.
func (s RunState) Short(l Lang) string {
	if t, ok := tRunStateShort[s]; ok {
		return t.In(l)
	}
	return string(s)
}
