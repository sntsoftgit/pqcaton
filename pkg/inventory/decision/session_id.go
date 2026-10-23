// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package decision

import (
	"crypto/rand"
	"fmt"
)

// NewSessionID — 판정 세션의 동일성. UUID v4다. 리뷰 세션과 스코프 세션이 같은 것을 쓴다.
// 표준 라이브러리만 쓴다 - 식별자 하나를 위해 의존성을 들이지 않는다.
func NewSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand가 실패하는 환경은 이 도구가 돌 수 없는 환경이다. 알리지 않고 약한 값을
		// 내느니 여기서 멈춘다 - 식별자가 겹치면 감사 기록이 겹친다.
		panic("session id: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
