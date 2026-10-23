// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// observed: 노드 하나가 라이브러리 하나를 앱 하나로 로드하고 있다는 관측 결과.
func observed(t *testing.T, dir, node, lib, app string) {
	t.Helper()
	cbom := fmt.Sprintf(`{"bomFormat":"CycloneDX","specVersion":"1.6","components":[
      {"type":"cryptographic-asset","name":%q,"properties":[
        {"name":"pqcota:crypto_runtime","value":"openssl"},
        {"name":"pqcota:openssl.version","value":"3.0.2"},
        {"name":"pqcota:openssl.fork","value":"OpenSSL"},
        {"name":"pqcota:app_keys","value":%q}]}]}`, lib, app)
	res := &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{TargetNodeId: node, CollectorId: "openssl-collector",
			CollectedAt:     timestamppb.New(time.Unix(100, 0)),
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(cbom), CyclonedxSpecVersion: "1.6",
		Completeness: &commonv1.Completeness{LayersCovered: []commonv1.CollectionLayer{commonv1.CollectionLayer_COLLECTION_LAYER_PROCESS}},
	}
	raw, err := protojson.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, node+".json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// IC-U73 — **자산 스코프 정책(-scope-assets)은 쓸 때마다 읽는다.** 서버가 떠 있는 동안 정책 파일이
// 바뀌면 화면 모델 · rev · 확인 결과가 모두 새 정책을 쓴다. 시작할 때 읽은 사본을 계속 쓰면 화면의
// 대조가 지금 정책과 달라지고, 상류 적재와 근거 지문이 어긋난다.
func TestScopeAssetsAreReadEachTime(t *testing.T) {
	s, dir := newServer(t)
	if err := os.Remove(s.path); err != nil {
		t.Fatal(err)
	}
	s.results = filepath.Join(dir, "results")
	if err := os.Mkdir(s.results, 0o700); err != nil {
		t.Fatal(err)
	}
	observed(t, s.results, "pay-db", "libcrypto.so.3", "/usr/sbin/sshd")
	observed(t, s.results, "web-gw", "libssl.so.3", "/usr/sbin/nginx")
	s.decl = filepath.Join(dir, "declaration.json")
	d := decl.Declaration{Org: "acme", Scope: []string{"pay-db", "web-gw"},
		Nodes: []decl.Node{{Name: "pay-db", IPs: []string{"10.0.0.2"}}, {Name: "web-gw", IPs: []string{"10.0.0.1"}}},
		Assets: []decl.Asset{{Node: "pay-db", Runtime: "openssl", Component: "libcrypto"},
			{Node: "web-gw", Runtime: "openssl", Component: "libssl"}}}
	if err := decl.Save(s.decl, d); err != nil {
		t.Fatal(err)
	}
	s.scopeAssets = filepath.Join(dir, "scope-assets.csv")
	if err := os.WriteFile(s.scopeAssets, []byte("action,runtime,lib,app_key,note\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 시작할 때처럼 세션을 파일로 남긴다(세션 ID가 서게).
	if err := s.materialize(); err != nil {
		t.Fatal(err)
	}

	ids := func() []string {
		sf, _, err := s.reviewSession()
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, it := range sf.Autopass {
			out = append(out, it.ID)
		}
		return out
	}
	before := ids()
	var sshd, nginx string
	for _, id := range before {
		switch {
		case strings.Contains(id, "libcrypto"):
			sshd = id
		case strings.Contains(id, "libssl"):
			nginx = id
		}
	}
	if sshd == "" || nginx == "" {
		t.Fatalf("both assets should be auto-pass candidates before the policy: %v", before)
	}
	if body := get(t, s, ui.ScreenReview).Body.String(); !strings.Contains(body, `name="plan:`+sshd+`"`) {
		t.Fatal("before the policy the screen should offer the sshd asset for the plan")
	}

	// 서버가 떠 있는 동안 정책이 바뀐다: sshd가 쓰는 것은 관리하지 않는다.
	if err := os.WriteFile(s.scopeAssets, []byte("action,runtime,lib,app_key,note\nexclude,*,*,/usr/sbin/sshd*,\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := get(t, s, ui.ScreenReview)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /review = %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, `name="plan:`+sshd+`"`) {
		t.Error("the screen model still uses the policy read at start")
	}
	if !strings.Contains(body, `name="`+ui.RevField+`" value="`+s.revFor(ui.ScreenReview)+`"`) {
		t.Error("the form does not carry the rev of the current policy")
	}

	// 확인 결과(지문)도 지금 정책으로 계산돼야 한다.
	form := url.Values{"reviewer": {"보안팀"}, "signature": {"sig"},
		"plan:" + nginx: {"on"}, "kind:" + nginx: {"REMEDIATION_KIND_CONFIG_ONLY"},
		"target:" + nginx: {"ML-KEM (FIPS 203)"}, "level:" + nginx: {"L2"}}
	c := postForm(t, s, "/finalize/confirm", form)
	if c.Code != http.StatusOK {
		t.Fatalf("confirm: status %d\n%s", c.Code, c.Body.String())
	}
	// 기대값은 서버를 거치지 않고 지금 파일로 직접 세운다.
	pol, err := review.LoadAssetPolicy(s.scopeAssets)
	if err != nil || pol == nil {
		t.Fatalf("policy: %v", err)
	}
	prev, err := review.Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	built, err := review.FromResultsWith(s.results, d, s.org, pol)
	if err != nil {
		t.Fatal(err)
	}
	sf := review.Carry(prev, built.Session)
	want, err := review.PayloadSHA256(ui.ApplyReview(sf, form), s.org, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := hiddenFields(c.Body.String()).Get(ui.ConfirmPayloadField); got != want {
		t.Errorf("the confirmation fingerprint was computed with another policy: %s, want %s", got, want)
	}
	if q := location(t, postForm(t, s, "/finalize", hiddenFields(c.Body.String()))); q.Get("problem") != "" {
		t.Errorf("finalize after the policy change: %s", q.Get("problem"))
	}
}
