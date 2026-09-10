package review_test

import (
	"strings"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
)

// ★ 계획에 넣는 항목은 **검토자가 골랐어야 할 것을 다 골라야** 확정된다.
//
// 전에는 위임 수준이 비면 L2, 조치 종류가 비면 PROVIDER_INJECT로 채웠다. 그러면 사용자가 하지
// 않은 정책 결정을 도구가 대신 내리고 **그 결과에 승인 서명이 붙는다.** 서명은 조치의 내용을
// 덮으므로, 승인자는 자기가 고르지 않은 수준과 조치에 책임을 지게 된다.
//
// 상류(pqcota)도 빈칸을 알리고 종료 3으로 끝내지만 그것은 직접 쓴 계획을 위한 최후 안전장치다.
// 승인·확정 계층인 여기서 통과시키고 거기서 걸리게 두면 계층의 역할이 뒤바뀐다.
func TestPlannedItemsNeedTheirDecisions(t *testing.T) {
	base := review.Item{
		ID: "n1/openssl/libssl", Node: "n1", Runtime: "openssl", Plan: true,
		Level: "L2", Kind: "REMEDIATION_KIND_CONFIG_ONLY", TargetAlgorithm: "ML-KEM (FIPS 203)",
	}
	if err := review.RequireDecisions(base); err != nil {
		t.Fatalf("다 고른 항목이 막혔다: %v", err)
	}

	for _, tc := range []struct {
		name string
		fix  func(*review.Item)
		want string
	}{
		{"위임 수준 미선택", func(i *review.Item) { i.Level = "" }, "no deploy level"},
		{"위임 수준 오타", func(i *review.Item) { i.Level = "L4" }, "unknown deploy level"},
		{"조치 종류 미선택", func(i *review.Item) { i.Kind = "" }, "no remediation kind"},
		{"조치 종류 오타", func(i *review.Item) { i.Kind = "REMEDIATION_KIND_PROXY_TERMINATE" }, "unknown remediation_kind"},
		{"config 조치인데 목표 없음", func(i *review.Item) { i.TargetAlgorithm = "" }, "target_algorithm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := base
			tc.fix(&it)
			err := review.RequireDecisions(it)
			if err == nil {
				t.Fatal("확정을 막지 않았다")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("무엇이 모자란지 말하지 않으면 고칠 수 없다: %v", err)
			}
		})
	}

	// config 로 배달하지 않는 조치에는 목표 알고리즘을 적을 자리가 없다 — 요구하지 않는다.
	fork := base
	fork.Kind, fork.TargetAlgorithm = "REMEDIATION_KIND_FORK_REPLACE", ""
	if err := review.RequireDecisions(fork); err != nil {
		t.Errorf("포크 교체에 목표 알고리즘을 요구했다: %v", err)
	}
}

// ★ L3 는 활성화까지 간다. 훅이 비면 무엇이 **일어나지 않는지**를 여기서 먼저 막는다.
func TestL3NeedsItsHooks(t *testing.T) {
	full := review.Item{
		ID: "n1/openssl/libssl", Node: "n1", Runtime: "openssl", Plan: true,
		Level: "L3", Kind: "REMEDIATION_KIND_CONFIG_ONLY", TargetAlgorithm: "ML-KEM (FIPS 203)",
		Activate: "a", Restart: "r", Deactivate: "d",
	}
	if err := review.RequireDecisions(full); err != nil {
		t.Fatalf("훅을 다 적었는데 막혔다: %v", err)
	}
	for _, tc := range []struct{ name, want string }{
		{"activate", "the fragment is placed but never referenced"},
		{"restart", "the new provider may never be loaded"},
		{"deactivate", "rollback cannot undo the activation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := full
			switch tc.name {
			case "activate":
				it.Activate = ""
			case "restart":
				it.Restart = ""
			case "deactivate":
				it.Deactivate = ""
			}
			err := review.RequireDecisions(it)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("L3의 %s 누락을 막지 않거나 이유를 말하지 않았다: %v", tc.name, err)
			}
		})
	}
	// L2 는 활성화하지 않으므로 훅을 요구하지 않는다.
	l2 := full
	l2.Level, l2.Activate, l2.Restart, l2.Deactivate = "L2", "", "", ""
	if err := review.RequireDecisions(l2); err != nil {
		t.Errorf("L2에 훅을 요구했다: %v", err)
	}
}
