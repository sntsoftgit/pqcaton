// Package org — 조직 격리의 축.
//
// 인벤토리는 한 조직의 암호 자산 지도다. 다른 조직의 것이 섞이면 그 자체로 사고다 —
// 계열사 분리든 호스팅이든, 경계는 데이터에 박혀 있어야 하지 질의를 쓰는 사람의
// 기억에 있으면 안 된다.
//
// 그래서 이 패키지는 타입 하나와 규칙 하나만 갖는다: **빈 조직은 만들 수 없다.**
// 저장소 핸들이 조직에 묶이므로(decision.NewPgJudgmentStore), 한 번 만들고 나면
// 질의마다 조건을 붙이는 일이 없다 — 붙이는 것을 잊을 수도 없다.
package org

import (
	"errors"
	"regexp"
)

// ID — 조직 식별자. 저장소·질의·리포트의 격리 단위다.
type ID string

// ErrEmpty — 조직 없이 저장소를 열려 한 경우. 격리가 없는 상태를 만들지 않는다.
var ErrEmpty = errors.New("조직 식별자가 비었다 — 격리 없는 저장소는 열지 않는다")

// ErrShape — 식별자 모양이 규칙에 맞지 않는 경우.
var ErrShape = errors.New("조직 식별자는 소문자·숫자·하이픈 2~64자여야 한다")

// 소문자·숫자·하이픈만. 대소문자를 섞으면 같은 조직이 둘로 갈린다.
var shape = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// Parse — 식별자를 검증해 돌려준다. 여기를 통과하지 않은 값은 저장소에 닿지 않는다.
func Parse(s string) (ID, error) {
	if s == "" {
		return "", ErrEmpty
	}
	if !shape.MatchString(s) {
		return "", ErrShape
	}
	return ID(s), nil
}

func (o ID) String() string { return string(o) }
