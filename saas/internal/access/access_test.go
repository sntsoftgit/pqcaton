package access_test

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pqcota/pqcota/pkg/org"
	"github.com/sntsoftgit/pqcaton/saas/internal/access"
)

var t0 = time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)

// issued — 조직 하나에 토큰 하나를 발급해 저장한 상태.
func issued(t *testing.T, s access.Store, o org.ID) access.Token {
	t.Helper()
	tok, err := access.NewToken()
	if err != nil {
		t.Fatalf("발급: %v", err)
	}
	if err := s.PutToken(access.TokenRecord{
		Lookup: tok.Lookup, Org: o, Digest: tok.Digest(), IssuedAt: t0,
	}); err != nil {
		t.Fatalf("저장: %v", err)
	}
	return tok
}

// CP-TOKEN-1 — 발급된 토큰의 모양과, 저장소에 평문이 남지 않는다는 것.
func TestTokenShapeAndNoPlaintextStored(t *testing.T) {
	tok, err := access.NewToken()
	if err != nil {
		t.Fatalf("발급: %v", err)
	}
	if !strings.HasPrefix(tok.Plaintext, access.Prefix+"_") {
		t.Fatalf("접두어가 없다: %q", tok.Plaintext)
	}
	lookup, secret, err := access.SplitToken(tok.Plaintext)
	if err != nil {
		t.Fatalf("가르기: %v", err)
	}
	if lookup != tok.Lookup {
		t.Fatalf("조회키가 다르다: %q ≠ %q", lookup, tok.Lookup)
	}
	if len(lookup) != 8 || len(secret) != 32 {
		t.Fatalf("길이가 다르다: 조회키 %d, 비밀 %d", len(lookup), len(secret))
	}
	// 저장되는 것은 해시뿐이다 — 평문을 되돌릴 수 없어야 유출 경로가 하나 줄어든다.
	if strings.Contains(string(tok.Digest()), secret) {
		t.Fatal("해시에 비밀이 그대로 들어 있다")
	}
	if len(tok.Digest()) != 32 {
		t.Fatalf("SHA-256이 아니다: %d바이트", len(tok.Digest()))
	}
}

// CP-TOKEN-2 — 발급된 토큰은 자기 조직을 돌려준다. **조직은 여기서만 나온다.**
func TestAuthenticateDerivesOrg(t *testing.T) {
	s := access.NewMemStore()
	tok := issued(t, s, "acme")

	got, rec, err := access.Authenticate(s, tok.Plaintext, t0)
	if err != nil {
		t.Fatalf("인증: %v", err)
	}
	if got != "acme" {
		t.Fatalf("조직이 다르다: %q", got)
	}
	if rec.Lookup != tok.Lookup {
		t.Fatalf("다른 레코드가 나왔다: %q", rec.Lookup)
	}
}

// CP-TOKEN-3 — 모양이 아닌 것은 조회하지 않고 끊는다.
// 저장소를 때리지 않아야 아무 문자열이나 던지는 쪽에 비용을 주지 않는다.
func TestMalformedTokenNeverReachesStore(t *testing.T) {
	for _, bad := range []string{
		"", "pqcrt", "pqcrt_short_x", "wrong_aaaaaaaa_" + strings.Repeat("b", 32),
		"pqcrt_aaaaaaaa", "pqcrt_aaaaaaaa_" + strings.Repeat("b", 31),
	} {
		if _, _, err := access.SplitToken(bad); !errors.Is(err, access.ErrMalformed) {
			t.Fatalf("%q: ErrMalformed가 아니라 %v", bad, err)
		}
	}
}

// CP-TOKEN-4·5·6 — 거절 사유를 나눠 돌려준다.
//
// 폐기된 토큰을 계속 쓰는 러너와 아무 토큰이나 넣어 보는 쪽은 다른 일이다. 기록에서
// 갈라야 무엇에 대응할지가 정해진다(HTTP 응답은 어느 쪽이든 같다).
func TestAuthenticateDistinguishesRejections(t *testing.T) {
	s := access.NewMemStore()
	tok := issued(t, s, "acme")

	// 모르는 조회키
	unknown, _ := access.NewToken()
	if _, _, err := access.Authenticate(s, unknown.Plaintext, t0); !errors.Is(err, access.ErrUnknownToken) {
		t.Fatalf("모르는 토큰: %v", err)
	}

	// 조회키는 맞고 비밀만 다르다
	_, _, _ = access.SplitToken(tok.Plaintext)
	forged := access.Prefix + "_" + tok.Lookup + "_" + strings.Repeat("z", 32)
	if _, _, err := access.Authenticate(s, forged, t0); !errors.Is(err, access.ErrSecret) {
		t.Fatalf("비밀 불일치: %v", err)
	}

	// 폐기된 토큰은 비밀이 맞아도 거절한다
	if err := s.RevokeToken(tok.Lookup, t0); err != nil {
		t.Fatalf("폐기: %v", err)
	}
	if _, _, err := access.Authenticate(s, tok.Plaintext, t0); !errors.Is(err, access.ErrRevoked) {
		t.Fatalf("폐기된 토큰: %v", err)
	}
}

// CP-TOKEN-7 — 인증이 성공하면 마지막 사용 시각이 남는다.
// 만료를 두지 않는 대신 이것으로 안 쓰이는 토큰을 찾아 거둔다(§6.4.1).
func TestAuthenticateRecordsLastUsed(t *testing.T) {
	s := access.NewMemStore()
	tok := issued(t, s, "acme")

	used := t0.Add(3 * time.Hour)
	if _, _, err := access.Authenticate(s, tok.Plaintext, used); err != nil {
		t.Fatalf("인증: %v", err)
	}
	rec, err := s.TokenByLookup(tok.Lookup)
	if err != nil {
		t.Fatalf("조회: %v", err)
	}
	if !rec.LastUsed.Equal(used) {
		t.Fatalf("마지막 사용이 남지 않았다: %v", rec.LastUsed)
	}
}

// CP-TOKEN-8 — 두 번 발급하면 조회키도 비밀도 겹치지 않는다.
func TestTokensAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		tok, err := access.NewToken()
		if err != nil {
			t.Fatalf("발급: %v", err)
		}
		if seen[tok.Lookup] {
			t.Fatalf("조회키가 겹쳤다: %q", tok.Lookup)
		}
		if seen[tok.Plaintext] {
			t.Fatalf("평문이 겹쳤다")
		}
		seen[tok.Lookup], seen[tok.Plaintext] = true, true
	}
}

// CP-TOKEN-9 — 조직 없는 토큰은 저장되지 않는다.
// 빈 조직을 품는 경로가 하나라도 있으면 그 경로로 데이터가 섞인다.
func TestTokenWithoutOrgIsRejected(t *testing.T) {
	s := access.NewMemStore()
	tok, _ := access.NewToken()
	err := s.PutToken(access.TokenRecord{Lookup: tok.Lookup, Digest: tok.Digest()})
	if !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("빈 조직이 저장됐다: %v", err)
	}
}

// CP-KEY-1·2 — 등록한 키가 나오고, **같은 collector에 키가 둘이어도 둘 다 유효하다.**
//
// 이 구간이 없으면 키를 바꾸는 순간 그 조직의 관측이 전부 거절된다 — 새 키를 등록하고
// 러너들이 갈아탄 뒤 옛 키를 폐기해야 한다(§6.4.2).
func TestActiveKeysAllowsRotationWindow(t *testing.T) {
	s := access.NewMemStore()
	for _, k := range []string{"old-key", "new-key"} {
		if err := s.RegisterKey(access.CollectorKey{
			Org: "acme", CollectorID: "openssl-collector", PublicKey: k, Registered: t0,
		}); err != nil {
			t.Fatalf("등록: %v", err)
		}
	}
	got, err := s.ActiveKeys("acme", "openssl-collector")
	if err != nil {
		t.Fatalf("조회: %v", err)
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != "new-key" || got[1] != "old-key" {
		t.Fatalf("교체 구간에 둘이 아니다: %v", got)
	}
}

// CP-KEY-3 — 폐기한 키는 빠진다.
func TestRevokedKeyDisappears(t *testing.T) {
	s := access.NewMemStore()
	_ = s.RegisterKey(access.CollectorKey{Org: "acme", CollectorID: "c", PublicKey: "k1", Registered: t0})
	_ = s.RegisterKey(access.CollectorKey{Org: "acme", CollectorID: "c", PublicKey: "k2", Registered: t0})
	if err := s.RevokeKey("acme", "c", "k1", t0); err != nil {
		t.Fatalf("폐기: %v", err)
	}
	got, _ := s.ActiveKeys("acme", "c")
	if len(got) != 1 || got[0] != "k2" {
		t.Fatalf("폐기가 반영되지 않았다: %v", got)
	}
}

// CP-KEY-4·5 — 조직과 collector가 다르면 안 나온다.
//
// 조직 조건이 조회에 함께 붙는 것이 요점이다 — 이 목록을 그대로 sign.Verify에 넘기므로,
// 여기서 새면 다른 조직의 collector가 서명한 결과가 통과한다.
func TestActiveKeysIsolatesOrgAndCollector(t *testing.T) {
	s := access.NewMemStore()
	_ = s.RegisterKey(access.CollectorKey{Org: "acme", CollectorID: "openssl", PublicKey: "acme-openssl", Registered: t0})
	_ = s.RegisterKey(access.CollectorKey{Org: "beta", CollectorID: "openssl", PublicKey: "beta-openssl", Registered: t0})
	_ = s.RegisterKey(access.CollectorKey{Org: "acme", CollectorID: "jvm", PublicKey: "acme-jvm", Registered: t0})

	got, _ := s.ActiveKeys("acme", "openssl")
	if len(got) != 1 || got[0] != "acme-openssl" {
		t.Fatalf("격리가 새고 있다: %v", got)
	}
}

// CP-KEY-6 — 조직 없이 조회하지 않는다.
func TestActiveKeysRequiresOrg(t *testing.T) {
	s := access.NewMemStore()
	if _, err := s.ActiveKeys("", "openssl"); !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("조직 없이 열렸다: %v", err)
	}
}

// CP-RUNNER-1·2 — 등록하고, 접속할 때마다 버전과 시각이 갱신된다.
func TestRunnerRegisterAndTouch(t *testing.T) {
	s := access.NewMemStore()
	if err := s.PutRunner(access.Runner{
		ID: "r1", Org: "acme", TokenLookup: "abcdefgh", Version: "0.1.0", Registered: t0,
	}); err != nil {
		t.Fatalf("등록: %v", err)
	}
	seen := t0.Add(time.Hour)
	if err := s.TouchRunner("r1", "0.2.0", seen); err != nil {
		t.Fatalf("갱신: %v", err)
	}
	got, err := s.Runner("r1")
	if err != nil {
		t.Fatalf("조회: %v", err)
	}
	if got.Version != "0.2.0" || !got.LastSeen.Equal(seen) {
		t.Fatalf("갱신되지 않았다: %+v", got)
	}
	// 어느 토큰으로 등록했는지가 남아야 그 토큰만 폐기했을 때 누가 끊기는지 안다.
	if got.TokenLookup != "abcdefgh" {
		t.Fatalf("토큰 출처가 없다: %+v", got)
	}
}

// CP-RUNNER-3 — 없는 러너를 갱신하지 않는다.
func TestTouchUnknownRunner(t *testing.T) {
	s := access.NewMemStore()
	if err := s.TouchRunner("없다", "0.1.0", t0); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("없는 러너가 갱신됐다: %v", err)
	}
}
