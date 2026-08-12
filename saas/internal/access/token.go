// Package access — 누가 우리에게 말할 수 있나.
//
// 러너 토큰(조직을 유도한다) · collector 공개키 등록소(관측을 누가 냈나) · 러너 등록.
// 셋 다 [러너 설계 §6.4](../../design.md)가 정한 것이고, 여기는 그 저장·검증만 한다.
// HTTP는 이 위에 얹힌다 — 이 패키지는 네트워크를 모른다.
package access

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
)

// Prefix — 토큰 접두어. 로그에서 눈에 띄고 시크릿 스캐너에 걸린다.
// 실수로 붙여 넣은 것을 찾을 수 있어야 한다.
const Prefix = "pqcrt"

const (
	lookupBytes = 5  // → base32 8자. 조회키는 비밀이 아니라 색인이다
	secretBytes = 20 // → base32 32자. 160비트면 추측이 성립하지 않는다
)

// enc — 패딩 없는 소문자 base32. `=`가 붙으면 URL·설정 파일에서 잘리는 자리가 생긴다.
var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

var (
	// ErrMalformed — 토큰 모양이 아니다. 조회할 것도 없다.
	ErrMalformed = errors.New("토큰 모양이 아니다")
	// ErrUnknownToken — 그런 조회키가 없다.
	ErrUnknownToken = errors.New("등록되지 않은 토큰이다")
	// ErrRevoked — 폐기된 토큰이다.
	ErrRevoked = errors.New("폐기된 토큰이다")
	// ErrSecret — 조회키는 맞는데 비밀이 다르다.
	ErrSecret = errors.New("토큰 비밀이 맞지 않는다")
)

// Token — 발급된 평문 토큰. **저장하지 않는다.** 발급 순간에만 존재하고, 저장소에는
// 조회키와 비밀의 해시만 남는다. 다시 보여 줄 방법이 없어야 유출 경로가 하나 줄어든다.
type Token struct {
	Plaintext string // pqcrt_<조회키>_<비밀>
	Lookup    string
	digest    [32]byte
}

// NewToken — 새 토큰을 만든다. 저장은 호출자가 [TokenStore.PutToken]으로 한다.
func NewToken() (Token, error) {
	l, err := randB32(lookupBytes)
	if err != nil {
		return Token{}, err
	}
	s, err := randB32(secretBytes)
	if err != nil {
		return Token{}, err
	}
	return Token{
		Plaintext: Prefix + "_" + l + "_" + s,
		Lookup:    l,
		digest:    sha256.Sum256([]byte(s)),
	}, nil
}

// Digest — 저장할 비밀의 해시.
//
// SHA-256을 쓴다. bcrypt·argon2를 쓰지 않는 이유는 이 비밀이 사람이 고른 것이 아니라
// 160비트 랜덤이라서다 — 사전 공격의 대상이 아니고, 매 요청 검증이라 느린 해시는
// 비용만 된다. 느린 해시는 엔트로피가 낮은 비밀을 지키는 장치다.
func (t Token) Digest() []byte { return t.digest[:] }

func randB32(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("난수: %w", err)
	}
	return strings.ToLower(enc.EncodeToString(b)), nil
}

// SplitToken — 평문 토큰을 조회키와 비밀로 가른다.
//
// 조회키를 따로 두는 이유: 해시만 저장하면 어느 행인지 찾으려고 전부 훑어야 한다.
// 조회키는 비밀이 아니므로 평문 색인으로 두고, 비밀만 해시로 지킨다.
func SplitToken(plaintext string) (lookup, secret string, err error) {
	parts := strings.Split(plaintext, "_")
	if len(parts) != 3 || parts[0] != Prefix {
		return "", "", ErrMalformed
	}
	lookup, secret = parts[1], parts[2]
	if len(lookup) != enc.EncodedLen(lookupBytes) || len(secret) != enc.EncodedLen(secretBytes) {
		return "", "", ErrMalformed
	}
	return lookup, secret, nil
}

// matches — 비밀이 저장된 해시와 맞는가. 상수시간으로 비교한다.
func matches(secret string, digest []byte) bool {
	sum := sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(sum[:], digest) == 1
}
