// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package scope

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/randyinthedev-hash/pqcota-common/pkg/org"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
)

// Output — 스코프 확정이 쓸 바이트 전부. 화면과 명령이 같은 것을 쓴다.
type Output struct {
	Session   []byte
	Policy    []byte // 확정 정책 CSV
	Judgments []byte // 원장에 더할 줄. 원장을 쓰지 않으면 비어 있다
	Judged    int
	Result    *FinalizeResult
}

// Outputs — 세션을 확정하고 쓸 바이트를 만든다. 관문은 Finalize 하나다.
func Outputs(sf Session, orgName string, withJudgments bool, now int64) (Output, error) {
	res, err := Finalize(sf, orgName)
	if err != nil {
		return Output{}, err
	}
	var pol bytes.Buffer
	if err := WriteCSV(&pol, res.Policy); err != nil {
		return Output{}, err
	}
	sess, err := EncodeSession(sf)
	if err != nil {
		return Output{}, err
	}
	out := Output{Session: sess, Policy: pol.Bytes(), Result: res}
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

// PayloadSHA256 — 확정 내용의 지문. 판정 행의 시각은 넣지 않는다(review.PayloadSHA256과 같은 규칙).
func PayloadSHA256(sf Session, orgName string, withJudgments bool) (string, error) {
	o, err := Outputs(sf, orgName, withJudgments, 0)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "pqcaton scope finalize/1\norg %q\njudgments %t\n", orgName, withJudgments)
	for _, part := range [][]byte{o.Session, o.Policy, o.Judgments} {
		fmt.Fprintf(h, "%d\n", len(part))
		h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
