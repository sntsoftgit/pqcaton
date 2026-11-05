// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
)

// writeJVM: JVM 수집기의 결과 하나. services면 상류 v0.10.6이 싣는 PQC 서비스 목록을 든다.
func writeJVM(t *testing.T, path, node string, services bool) {
	t.Helper()
	props := `{"name":"pqcota:crypto_runtime","value":"jca"},{"name":"pqcota:jca.provider_set","value":"SUN,SunEC"}`
	if services {
		props += `,{"name":"pqcota:jca.pqc_services","value":"SunEC="}`
	}
	r := &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{TargetNodeId: node, CollectorId: "jvm-collector", CollectedAt: timestamppb.Now(),
			DetectionMethod: commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION},
		CbomCyclonedx: []byte(`{"bomFormat":"CycloneDX","specVersion":"1.6","components":[{"type":"cryptographic-asset",` +
			`"name":"jca-provider-chain","properties":[` + props + `]}]}`), CyclonedxSpecVersion: "1.6",
		Completeness: &commonv1.Completeness{LayersCovered: []commonv1.CollectionLayer{commonv1.CollectionLayer_COLLECTION_LAYER_PROCESS}},
	}
	b, err := protojson.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// IC-U143 — **JCA 서비스 목록을 확인하지 못한 노드는 03 관측과 04 대조 화면에 두 말로 알린다.** 노드 이름과 정해진 문구가
// 그대로 보인다. 서비스 목록이 있거나(비어 있어도) JCA 결과가 없으면 경고가 없다. 결과가 화면의 관측에서 오지 않았어도 같다.
func TestJCAWarningReachesBothScreens(t *testing.T) {
	text := map[string]string{
		"ko": "v0.10.6 JCA 수집 결과임을 확인할 수 없는 노드가 있습니다",
		"en": "the JCA results cannot be confirmed as v0.10.6 collection results",
	}
	for name, c := range map[string]struct {
		write func(t *testing.T, dir string)
		warn  bool
	}{
		"jca without services": {func(t *testing.T, dir string) { writeJVM(t, filepath.Join(dir, "jvm.json"), "web-gw", false) }, true},
		"jca with services":    {func(t *testing.T, dir string) { writeJVM(t, filepath.Join(dir, "jvm.json"), "web-gw", true) }, false},
		"no jca":               {func(t *testing.T, dir string) { writeResult(t, filepath.Join(dir, "web.json"), "web-gw") }, false},
	} {
		t.Run(name, func(t *testing.T) {
			s := everyScreen(t)
			c.write(t, s.results)
			for _, lang := range []string{"ko", "en"} {
				for _, screen := range []string{ui.ScreenObserve, ui.ScreenSurvey} {
					body := get(t, s, screen+"?lang="+lang).Body.String()
					has := strings.Contains(body, text[lang]) && strings.Contains(body, "<code>web-gw</code>")
					if has != c.warn {
						t.Errorf("%s (%s): warning shown = %v, want %v", screen, lang, has, c.warn)
					}
					if lang == "en" && strings.Contains(body, text["ko"]) {
						t.Errorf("%s: the Korean warning is on the English screen", screen)
					}
				}
			}
		})
	}
}
