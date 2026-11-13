// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testMod = `module example.com/x

require (
	github.com/randyinthedev-hash/pqcota-common v0.14.0
	github.com/randyinthedev-hash/pqcota-discovery v0.14.0
	github.com/randyinthedev-hash/pqcota-inventory v0.14.0
	github.com/randyinthedev-hash/pqcota-provisioning v0.14.0
)
`

func sumLines(discovery string) string {
	var b strings.Builder
	for _, m := range modules {
		h := "h1:" + strings.TrimPrefix(m, "github.com/randyinthedev-hash/") + "="
		if strings.HasSuffix(m, "pqcota-discovery") {
			h = discovery
		}
		// /go.mod 줄을 먼저 둔다. 생성기가 이것을 모듈 zip 의 h1 로 고르면 안 된다.
		b.WriteString(m + " v0.14.0/go.mod h1:gomod-" + strings.TrimPrefix(m, "github.com/randyinthedev-hash/") + "=\n")
		b.WriteString(m + " v0.14.0 " + h + "\n")
	}
	return b.String()
}

func repo(t *testing.T, mod, sum string) string {
	t.Helper()
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "go.sum"), []byte(sum), 0o644); err != nil {
		t.Fatal(err)
	}
	return d
}

// IC-CV14 — 생성기는 go.sum 에서 모듈 zip 의 h1 만 고르고(`/go.mod` 줄의 해시가 아니다), 생성물에 판과 h1 을 함께 적는다.
// 모듈의 h1 이 없거나, 같은 모듈 · 판에 h1 이 둘로 모순되거나, go.mod 가 그 모듈을 요구하지 않으면 생성하지 않는다.
func TestGenpinReadsTheZipHash(t *testing.T) {
	p, err := read(repo(t, testMod, sumLines("h1:discovery-zip=")))
	if err != nil {
		t.Fatal(err)
	}
	d := p["github.com/randyinthedev-hash/pqcota-discovery"]
	if d.version != "v0.14.0" || d.sum != "h1:discovery-zip=" {
		t.Fatalf("want v0.14.0 and the zip h1, got %+v", d)
	}
	src, err := render(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `{Version: "v0.14.0", Sum: "h1:discovery-zip="}`) || strings.Contains(string(src), "gomod-") {
		t.Fatalf("the generated code should carry the version with the zip h1 and no go.mod hash:\n%s", src)
	}

	noZip := strings.Replace(sumLines("h1:discovery-zip="), "github.com/randyinthedev-hash/pqcota-discovery v0.14.0 h1:discovery-zip=\n", "", 1)
	if _, err := read(repo(t, testMod, noZip)); err == nil || !strings.Contains(err.Error(), "no h1") {
		t.Errorf("a missing zip h1 (only the /go.mod line) should fail, got %v", err)
	}
	twice := sumLines("h1:discovery-zip=") + "github.com/randyinthedev-hash/pqcota-discovery v0.14.0 h1:other=\n"
	if _, err := read(repo(t, testMod, twice)); err == nil || !strings.Contains(err.Error(), "two h1 values") {
		t.Errorf("two h1 values for one module version should fail, got %v", err)
	}
	noReq := strings.Replace(testMod, "\tgithub.com/randyinthedev-hash/pqcota-discovery v0.14.0\n", "", 1)
	if _, err := read(repo(t, noReq, sumLines("h1:discovery-zip="))); err == nil || !strings.Contains(err.Error(), "does not require") {
		t.Errorf("go.mod without the module should fail, got %v", err)
	}
}
