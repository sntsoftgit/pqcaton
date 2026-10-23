// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package ui

// 접근 가능한 이름.
//
// 표 칸 안의 입력에는 보이는 이름이 없다. 머리글은 표 위에 있어서 눈으로는 어느 칸인지 알지만,
// 화면 읽기 프로그램에는 「편집 칸」으로만 들린다. 그래서 몇 번째 무엇의 어느 칸인지를 이름으로
// 단다(「노드 2 IP」). 칸 이름은 머리글과 같은 문구를 쓴다 — 둘이 다르면 같은 칸이 두 이름을 갖는다.

import (
	"fmt"

	"github.com/a-h/templ"
)

// rowLabel — 「노드 2 IP」. n은 1부터 센다.
func rowLabel(l Lang, row T, n int, col string) string {
	return fmt.Sprintf("%s %d %s", row.In(l), n, col)
}

// assetLabel — 「노드 1 · 자산 2 런타임」. 자산 표는 노드마다 따로라 노드 번호까지 단다.
func assetLabel(l Lang, node, i int, col string) string {
	return fmt.Sprintf("%s %d · %s %d %s", tRowNode.In(l), node+1, tRowAsset.In(l), i+1, col)
}

// ruleLabel — 「계층 1 · 규칙 2 lib」.
func ruleLabel(l Lang, layer, i int, col string) string {
	return fmt.Sprintf("%s %d · %s %d %s", tLayer.In(l), layer+1, tRowRule.In(l), i+1, col)
}

// itemLabel — 「목표 알고리즘 · host://local/openssl/libssl」. 판정 표의 줄은 번호보다 항목 ID가
// 사람이 찾는 이름이다.
func itemLabel(col, id string) string { return col + " · " + id }

// ariaCurrent — 지금 자리면 aria-current를 단다. 아니면 속성을 싣지 않는다.
func ariaCurrent(here bool, what string) templ.Attributes {
	if !here {
		return nil
	}
	return templ.Attributes{"aria-current": what}
}
