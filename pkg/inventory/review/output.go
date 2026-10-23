// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/randyinthedev-hash/pqcota-common/pkg/org"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// Output — 확정이 쓸 바이트 전부. 화면과 명령이 같은 것을 쓴다.
//
// **파일로 쓰기 전에 다 만든다.** 계획 · 세션 · 판정 원장을 한 트랜잭션으로 바꾸려면, 무엇을 쓸지가
// 쓰기 시작하기 전에 정해져 있어야 한다.
type Output struct {
	Session   []byte
	Plan      []byte
	Judgments []byte // 원장에 더할 줄. 원장을 쓰지 않으면 비어 있다
	Judged    int
	Result    *Result
}

// Outputs — 세션을 확정하고 쓸 바이트를 만든다. 관문은 Finalize 하나다.
//
// now는 판정 행의 시각이다. 같은 세션 · 같은 now면 같은 바이트가 나온다.
func Outputs(sf Session, orgName string, withJudgments bool, now int64) (Output, error) {
	res, err := Finalize(sf)
	if err != nil {
		return Output{}, err
	}
	plan, err := PlanJSON(res)
	if err != nil {
		return Output{}, err
	}
	sess, err := Encode(sf)
	if err != nil {
		return Output{}, err
	}
	out := Output{Session: sess, Plan: plan, Result: res}
	if withJudgments {
		for _, j := range judgmentsOf(sf, res.Decided, now) {
			line, err := decision.EncodeJudgment(org.ID(orgName), *j)
			if err != nil {
				return Output{}, err
			}
			out.Judgments = append(out.Judgments, line...)
			out.Judged++
		}
	}
	return out, nil
}

// PlanJSON — 확정 계획 파일의 바이트. 명령이 표준출력으로 내던 것과 같은 모양이다.
func PlanJSON(res *Result) ([]byte, error) {
	raw, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(res.Plan)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// PayloadSHA256 — 확정 내용의 지문. 확인 화면이 보여 준 것과 실제로 쓰는 것이 같은지 견줄 때 쓴다.
//
// **판정 행의 시각은 넣지 않는다.** 확인과 확정 사이에 시각이 흐르므로, 시각까지 넣으면 늘 다르다.
// 그래서 now를 0으로 둔 출력으로 잰다. 시각 말고 무엇이든 다르면 지문이 다르다.
//
// **계획은 JSON 글이 아니라 결정적 바이너리로 잰다.** protojson은 바이트 비교를 막으려고 실행 파일마다
// 공백을 흔든다. 글로 재면 화면과 명령(서로 다른 실행 파일)이 같은 확정을 다른 내용으로 본다.
func PayloadSHA256(sf Session, orgName string, withJudgments bool) (string, error) {
	o, err := Outputs(sf, orgName, withJudgments, 0)
	if err != nil {
		return "", err
	}
	plan, err := proto.MarshalOptions{Deterministic: true}.Marshal(o.Result.Plan)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "pqcaton review finalize/1\norg %q\njudgments %t\n", orgName, withJudgments)
	for _, part := range [][]byte{o.Session, plan, o.Judgments} {
		fmt.Fprintf(h, "%d\n", len(part))
		h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
