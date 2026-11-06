// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package scope

// 정책 지문. 「두 정책이 같은 자산을 관리 대상으로 고르는가」를 값 하나로 잰다.
//
// 대조(04 · 05)가 쓰는 활성 정책과 02에서 확정한 정책을 견주고, 05 확정이 그 계획을 만든 정책으로 영수증에
// 적는다. 06은 그 값과 지금 활성 정책을 견준다. 그래서 **판정에 영향을 주는 것만** 넣는다.
//
//   - 규칙의 순서는 넣는다. 상류는 매치되는 마지막 규칙이 결정한다(뒤 규칙이 이긴다). 순서가 바뀌면 고르는
//     자산이 바뀐다.
//   - 빈 칸과 `*`는 같게 둔다. 상류가 둘을 똑같이 「모두」로 본다(`star`).
//   - 패턴의 대소문자는 그대로 둔다. 상류의 glob이 대소문자를 가린다.
//   - note는 넣지 않는다. 사람이 읽는 설명이고 판정에 쓰이지 않는다(IC-S4와 같은 선).
//   - 주석 줄 · 빈 줄 · 머리글 · 칸 앞 공백은 상류 파서가 이미 버리므로 지문에도 남지 않는다.
//
// **파일이 없는 것과 규칙이 0개인 파일은 같은 지문이다.** 둘 다 상류에서 「전부 관리 대상」인 같은 동작이다.
// 읽을 수 없는 파일과 형식이 틀린 파일은 빈 정책으로 다루지 않고 오류를 돌려준다. 그것을 빈 정책으로 읽으면
// 정책이 깨졌는데도 「제한 없음」의 지문이 나와 비교를 통과한다.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	kscope "github.com/randyinthedev-hash/pqcota-common/pkg/kernel/scope"
)

// PolicyDigestPrefix: 지문의 판. 정규화 규칙을 바꾸면 판을 올린다. 옛 값과 새 값이 우연히 같아 보이면 안 된다.
const PolicyDigestPrefix = "scope-policy-v1:"

// PolicyDigest — 정책의 지문. nil은 규칙 0개와 같다.
func PolicyDigest(p *kscope.AssetPolicy) string {
	rules := [][4]string{}
	if p != nil {
		for _, r := range p.Rules {
			action := "include"
			if r.Exclude {
				action = "exclude"
			}
			rules = append(rules, [4]string{action, anyIfEmpty(r.Runtime), anyIfEmpty(r.Lib), anyIfEmpty(r.AppKey)})
		}
	}
	// 칸마다 따옴표로 감싸는 JSON이라 칸 경계가 섞이지 않는다(「a,b」+「c」와 「a」+「b,c」가 다르다).
	b, err := json.Marshal(rules)
	if err != nil {
		panic(err) // 문자열 배열은 실패하지 않는다
	}
	sum := sha256.Sum256(b)
	return PolicyDigestPrefix + hex.EncodeToString(sum[:])
}

func anyIfEmpty(s string) string {
	if s == "" {
		return "*"
	}
	return s
}

// PolicyFileDigest — 정책 파일의 지문. 빈 경로나 없는 파일은 규칙 0개의 지문이다. 읽기 실패와 형식 오류는 오류다.
func PolicyFileDigest(path string) (string, error) {
	if path == "" {
		return PolicyDigest(nil), nil
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return PolicyDigest(nil), nil
	}
	if err != nil {
		return "", fmt.Errorf("reading the asset scope policy: %w", err)
	}
	defer f.Close()
	p, err := kscope.LoadAssetPolicy(f)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return PolicyDigest(p), nil
}
