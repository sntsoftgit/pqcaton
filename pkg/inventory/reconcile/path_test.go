// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package reconcile

import (
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
)

// IC-R26 — **같은 이름의 OpenSSL 둘은 한 키 아래 근거 둘이 되고, 각 근거가 설치 경로를 든다.** 자산 키에는 경로가 없어
// 둘이 한 항목으로 모인다. 경로를 근거에서 잃으면 사람이 두 설치를 가르지 못한다. 경로를 모르는 관측은 빈 값이다.
func TestSameNamedInstallationsKeepTheirPaths(t *testing.T) {
	lib := func(id, path string) *discoveryv1.Finding {
		return &discoveryv1.Finding{Id: id, CryptoRuntime: commonv1.CryptoRuntime_CRYPTO_RUNTIME_OPENSSL,
			RuntimeAxes: &discoveryv1.Finding_Openssl{Openssl: &discoveryv1.OpensslAxes{Lib: "libssl", Path: path}}}
	}
	obs := observedFrom("web", []*discoveryv1.Finding{
		lib("f-sys", "/usr/lib/x86_64-linux-gnu/libssl.so.3"),
		lib("f-app", "/opt/app/lib/libssl.so.3"),
		lib("f-old", ""),
	})
	if len(obs) != 3 || obs[0].Key != obs[1].Key {
		t.Fatalf("the installations did not land under one key: %+v", obs)
	}
	rec := reconcileAssets([]AssetKey{obs[0].Key}, obs, nil, nil)
	if len(rec) != 1 {
		t.Fatalf("want one reconciled asset, got %d", len(rec))
	}
	got := map[string]string{}
	for _, s := range rec[0].Sources {
		got[s.FindingID] = s.Path
	}
	want := map[string]string{"f-sys": "/usr/lib/x86_64-linux-gnu/libssl.so.3", "f-app": "/opt/app/lib/libssl.so.3", "f-old": ""}
	for id, p := range want {
		if v, ok := got[id]; !ok || v != p {
			t.Errorf("source %s path = %q (present %t), want %q", id, v, ok, p)
		}
	}
}
