package probe_test

import (
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
	"github.com/sntsoftgit/pqcaton/internal/probe"
)

// 계약을 실제로 소비할 수 있는지가 이 리포의 첫 전제다. 타입을 import하는 것만으로는
// 부족하고, enum 값이 우리가 기대한 어휘인지까지 봐야 한다 — 상류가 이름을 바꾸면
// 여기서 끊긴다.
func TestRuntimeName(t *testing.T) {
	got := probe.RuntimeName(commonv1.CryptoRuntime_CRYPTO_RUNTIME_OPENSSL)
	if want := "CRYPTO_RUNTIME_OPENSSL"; got != want {
		t.Fatalf("계약 어휘가 달라졌다: got %q, want %q", got, want)
	}
}
