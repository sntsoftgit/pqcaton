// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

import (
	"context"
	"io"
	"time"
)

// 05의 미리 보기를 막는 것의 종류. ProvUnsupported 뒤에는 까닭(provider · l3 · manual · unknown)이 붙는다.
const (
	ProvViewOnly     = "view-only"
	ProvNotApproved  = "not-approved"
	ProvNoPlaybook   = "no-playbook"
	ProvLevel        = "level"
	ProvNotGenerated = "not-generated"
	ProvUnknownForm  = "unknown-form"
	ProvUnsupported  = "unsupported"
	ProvNoHosts      = "no-hosts"
	// ProvHostsUndeclared — 접근 정보(hosts.csv)에 선언에 없는 노드 ID가 있다. 그 행이 이번 플레이북의 대상이 아니어도 원격
	// 실행을 시작하지 않는다(관측과 같은 규칙). Node가 그 ID다.
	ProvHostsUndeclared = "hosts-undeclared"
	ProvWindows         = "windows"
	ProvNoAnsible       = "no-ansible"
	// ProvAnsibleVersion — 부를 ansible-playbook의 core 판이 05가 지원하는 판이 아니거나 읽을 수 없다.
	ProvAnsibleVersion = "ansible-version"
)

// ProvBlock — 화면에서 실행하지 않는 까닭 하나. Node · Task는 해당하는 노드와 작업(또는 조치 ID)이다.
type ProvBlock struct {
	Code, Node, Task string
	// Have · Want — ProvAnsibleVersion일 때 감지한 core 판(읽지 못했으면 빈 값)과 지원하는 판.
	Have, Want string
}

// ProvTask · ProvNode · ProvResult — 실행 하나의 판정. 값은 provrun 패키지의 것 그대로다(상태 · 이유 · 결과의 코드).
// **모듈의 메시지는 어디에도 없다.** 화면은 이 코드에 정해 둔 문구만 보인다.
type ProvTask struct {
	Name, Status string
}

type ProvNode struct {
	Node, State, Why string
	Tasks            []ProvTask
	// Recheck — 이 실행에 연결한 재확인이 이 노드에 대해 낸 것(RecheckMatch · RecheckDiffers · RecheckUnreached). 재확인이
	// 없으면 비어 있다. **실행 기록의 상태(State)는 재확인으로 바뀌지 않는다.**
	Recheck string
}

// 재확인이 노드 하나에 대해 내는 것.
const (
	RecheckMatch     = "match"     // 그 시각에 대상 파일이 기대 상태와 같다
	RecheckDiffers   = "differs"   // 바뀔 작업이 있다
	RecheckUnreached = "unreached" // 재확인이 이 노드의 작업을 모두 확인하지 못했다
)

type ProvResult struct {
	ID             string
	Check          bool
	Started, Ended time.Time
	Outcome, End   string
	RecordWhole    bool
	Stall          int
	Nodes          []ProvNode
	// Grant — 적용 권한의 지금 상태(valid · expired · changed · used). 비면 권한이 없다.
	Grant        string
	GrantExpires time.Time
	// Began — 기록에 실행의 시작이 있다(감독이 원격 작업을 허용했다). Consumed — 이 적용 실행이 적용 권한을 소비했다.
	// 둘은 따로다: 소비했는데 시작하지 못한 실행이 있을 수 있다(그때 원격 작업은 0건이고 새 미리 보기가 필요하다).
	Began, Consumed bool
	// RecheckID · RecheckAt — 이 적용 실행에 연결한 가장 최근 재확인과 그 시각. 없으면 비어 있다.
	RecheckID string
	RecheckAt time.Time
}

// RecheckText — 노드의 재확인 칸. 재확인 시각(UTC)과 결과다.
func (r ProvResult) RecheckText(l Lang, n ProvNode) string {
	switch n.Recheck {
	case RecheckMatch:
		return tProvRecheckMatch.F(l, r.RecheckAt.UTC().Format("15:04 UTC"))
	case RecheckDiffers:
		return tProvRecheckDiffers.F(l, r.RecheckAt.UTC().Format("15:04 UTC"))
	case RecheckUnreached:
		return tProvRecheckUnreached.In(l)
	}
	return ""
}

// ProvRun — 05의 실행 구역.
type ProvRun struct {
	Blocks    []ProvBlock
	Targets   []string
	Running   bool
	RunningID string
	// Apply — 마지막 적용 실행과 그 재확인. Last — 마지막 미리 보기(재확인이 아닌 것).
	Apply *ProvResult
	Last  *ProvResult
	// RecheckSame — 지금의 입력이 그 적용 실행의 것과 같다. 같을 때만 재확인할 수 있다.
	RecheckSame bool
	// CanApply — 가장 최근 미리 보기의 권한이 지금 유효하다. 이때만 적용 확인으로 가는 버튼을 낸다.
	CanApply bool
}

// GrantNote — 적용 실행이 원격 작업을 시작하지 못했을 때 권한이 어떻게 됐는지의 한 줄. 시작했으면 비어 있다.
func (r ProvResult) GrantNote(l Lang) string {
	switch {
	case r.Check || r.Began:
		return ""
	case r.Consumed:
		return tApplyNotBegunConsumed.In(l)
	}
	return tApplyNotBegunUnused.In(l)
}

// CanRecheck — 재확인 버튼을 낼 수 있는가.
func (p ProvRun) CanRecheck() bool { return p.Apply != nil && p.RecheckSame && !p.Running }

// CanPreview — 미리 보기 버튼을 낼 수 있는가.
func (p ProvRun) CanPreview() bool { return len(p.Blocks) == 0 && len(p.Targets) > 0 && !p.Running }

// Text — 막는 까닭의 문구.
func (b ProvBlock) Text(l Lang) string {
	if b.Code == ProvAnsibleVersion {
		have := b.Have
		if have == "" {
			have = tProvVersionUnread.In(l)
		}
		return tProvVersionBlock.F(l, have, b.Want, b.Want)
	}
	return provBlockText[b.Code].In(l)
}

// EndText — 실행 전체에 붙는 한 줄. 감독이 끝낸 까닭이 있으면 그것이고, 없는데 기록이 끝까지 이어지지 않으면 「실행 기록
// 중단」이다. 비면 붙일 말이 없다.
func (r ProvResult) EndText(l Lang) string {
	switch {
	case r.End == "stall":
		return provEndText["stall"].F(l, r.Stall)
	case r.End == "orphaned":
		return provEndText["stopped"].In(l)
	case r.End == "lost" && !r.Began:
		// 원격 작업이 시작된 적이 없는 실행이다. 「이미 시작한 원격 작업」을 말하지 않는다.
		return provEndText["lost-idle"].In(l)
	case r.End != "":
		return provEndText[r.End].In(l)
	case !r.RecordWhole:
		return provEndText["cut"].In(l)
	}
	return ""
}

// OutcomeText — 판정의 문구.
func (r ProvResult) OutcomeText(l Lang) string { return provOutcomeText[r.Outcome].In(l) }

// Good — 판정이 좋은 쪽인가(알림의 색).
func (r ProvResult) Good() bool {
	return r.Outcome == "ready" || r.Outcome == "no-change" || r.Outcome == "applied"
}

// StateText — 노드 상태의 문구. 「끝남」은 미리 보기에서는 「확인함」, 적용에서는 「L2 배치 완료」다.
func (r ProvResult) StateText(l Lang, n ProvNode) string {
	if n.State == "done" {
		if r.Check {
			return provStateText["done-check"].In(l)
		}
		return provStateText["done-apply"].In(l)
	}
	return provStateText[n.State].In(l)
}

// WhyText — 노드 상태의 이유. 기록에서 근거를 댈 수 있는 것뿐이다.
func (r ProvResult) WhyText(l Lang, n ProvNode) string {
	if n.Why == "stalled" {
		return provWhyText["stalled"].F(l, r.Stall)
	}
	return provWhyText[n.Why].In(l)
}

// TaskText — 작업 결과의 문구.
func (r ProvResult) TaskText(l Lang, t ProvTask) string {
	if t.Status == "changed" {
		if r.Check {
			return provTaskText["changed-check"].In(l)
		}
		return provTaskText["changed-apply"].In(l)
	}
	return provTaskText[t.Status].In(l)
}

// GrantText — 적용 권한의 상태 문구.
func (r ProvResult) GrantText(l Lang) string {
	switch r.Grant {
	case "valid":
		return tProvGrantValid.F(l, r.GrantExpires.Local().Format("15:04:05"))
	case "expired":
		return tProvGrantExpired.In(l)
	case "changed":
		return tProvGrantChanged.In(l)
	case "used":
		return tProvGrantUsed.In(l)
	}
	return ""
}

// RenderProvRunning — 도는 동안 다시 받는 조각.
func RenderProvRunning(w io.Writer, l Lang, csrf string) error {
	return provRunning(l, csrf).Render(context.Background(), w)
}

// provSectionID — 결과 구역의 id. 적용 실행과 미리 보기가 한 화면에 함께 있다.
func provSectionID(r ProvResult) string {
	if r.Check {
		return "prov-result"
	}
	return "prov-apply"
}
