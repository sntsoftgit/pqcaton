package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mods(paths ...string) []mod {
	out := make([]mod, 0, len(paths))
	for _, p := range paths {
		out = append(out, mod{Path: p, Version: "v1.0.0"})
	}
	return out
}

// IC-X1 — **카피레프트는 하나도 통과하지 못한다.**
//
// 링크하면 파생물 전체가 그 조건을 따라야 해서 BUSL로 내는 것도 Change Date의 Apache-2.0
// 전환도 막힙니다. 금지 목록에 적어 두고 한 종류만 재면 나머지가 새는지 알 수 없어,
// 목록에 있는 것을 **전부** 돌린다.
func TestForbiddenLicensesAllBlocked(t *testing.T) {
	for lic, why := range forbidden {
		known := map[string]string{"example.com/dep": lic}
		missing, bad := verdict(mods("example.com/dep"), known)
		if len(missing) != 0 {
			t.Errorf("%s: 라이선스를 아는데 모름으로 셌다", lic)
		}
		if len(bad) != 1 {
			t.Fatalf("%s(%s)가 통과했다", lic, why)
		}
		// 왜 막혔는지 말해야 한다 — 이름만 찍으면 대안을 찾을 수 없다.
		if !strings.Contains(bad[0], why) {
			t.Errorf("%s: 사유를 말하지 않는다: %s", lic, bad[0])
		}
	}
}

// IC-X2 — **모르는 라이선스도 막는다.**
//
// 금지 목록에 없다고 통과시키면 게이트가 아니라 블랙리스트가 된다 — 새 라이선스가 나오면
// 그때마다 목록을 고쳐야 하고, 고치기 전까지는 조용히 통과한다.
func TestUnknownLicenseIsBlocked(t *testing.T) {
	_, bad := verdict(mods("example.com/dep"), map[string]string{"example.com/dep": "WTFPL"})
	if len(bad) != 1 {
		t.Fatalf("허용 목록에 없는 라이선스가 통과했다: %v", bad)
	}
	if !strings.Contains(bad[0], "검토") {
		t.Errorf("무엇을 하라는지 말하지 않는다: %s", bad[0])
	}
}

// IC-X3 — **licenses.txt에 없는 모듈은 막힌다.**
//
// `go get` 한 번이면 새 의존성이 들어온다. 적히지 않은 것을 통과시키면 그 순간 게이트가
// 뚫린다 — 이것이 이 도구를 빌드에 둔 이유다.
func TestUnlistedModuleIsBlocked(t *testing.T) {
	missing, bad := verdict(mods("example.com/new-dep"), map[string]string{})
	if len(missing) != 1 || missing[0] != "example.com/new-dep" {
		t.Fatalf("적히지 않은 모듈이 통과했다: missing=%v bad=%v", missing, bad)
	}
}

// IC-X4 — 허용적 라이선스는 통과한다. 막는 것만 재면 전부 막아도 케이스는 통과한다.
func TestAllowedLicensesPass(t *testing.T) {
	known := map[string]string{}
	var paths []string
	for lic := range allowed {
		p := "example.com/" + strings.ToLower(lic)
		known[p] = lic
		paths = append(paths, p)
	}
	missing, bad := verdict(mods(paths...), known)
	if len(missing) != 0 || len(bad) != 0 {
		t.Fatalf("허용적 라이선스가 막혔다: missing=%v bad=%v", missing, bad)
	}
}

// IC-X5 — 허용 목록 파일 형식. 주석과 빈 줄은 건너뛰고, 두 칸이 안 되는 줄은 **조용히
// 넘기지 않고 끊는다** — 넘기면 그 모듈이 「모름」으로 빠지는 대신 아예 사라진다.
func TestLoadAllowlist(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "licenses.txt")
	if err := os.WriteFile(p, []byte(
		"# 주석\n\nexample.com/a MIT\nexample.com/b Apache-2.0  # 뒤에 붙은 것\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadAllowlist(p)
	if err != nil {
		t.Fatal(err)
	}
	if got["example.com/a"] != "MIT" || got["example.com/b"] != "Apache-2.0" {
		t.Fatalf("읽은 값이 다르다: %v", got)
	}

	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("example.com/only-path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAllowlist(bad); err == nil {
		t.Error("형식이 깨진 줄을 그냥 넘겼다")
	}
}

// IC-X6 — **파일이 없어도 열리되, 그래서 통과하지는 않는다.**
//
// 빈 목록을 돌려주는 것이 위험해 보이지만 그 반대다 — 모든 모듈이 「모름」이 되어 전부
// 막힌다. 파일이 없다고 게이트가 열리는 일은 없다.
func TestMissingAllowlistBlocksEverything(t *testing.T) {
	known, err := loadAllowlist(filepath.Join(t.TempDir(), "없는파일.txt"))
	if err != nil {
		t.Fatalf("파일이 없다고 멈췄다: %v", err)
	}
	missing, _ := verdict(mods("example.com/a", "example.com/b"), known)
	if len(missing) != 2 {
		t.Fatalf("허용 목록이 없는데 %d개만 막혔다", len(missing))
	}
}

// IC-X7 — **검사 대상은 실제로 링크되는 모듈이다.**
//
// 이 리포 자신을 대상으로 돈다. 메인 모듈이 섞이면 자기 자신을 「라이선스 모름」으로 막는다.
//
// **리포 루트에서 재는 것이 이 케이스의 절반이다.** `go list -deps ./...` 는 지금 디렉터리를
// 기준으로 도므로, 이 도구가 있는 폴더에서 돌리면 stdlib 뿐이라 외부 모듈이 0개로 나온다 —
// 그래서 main 은 0개를 통과로 보지 않는다.
func TestLinkedModulesExcludesMain(t *testing.T) {
	chdirRepoRoot(t)
	got, err := linkedModules()
	if err != nil {
		t.Skipf("go list 를 돌릴 수 없다: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("링크되는 모듈이 하나도 없다 — 게이트가 빈 목록을 보고 있다")
	}
	for _, m := range got {
		if m.Main {
			t.Errorf("메인 모듈이 섞였다: %s", m.Path)
		}
		if m.Path == "github.com/sntsoftgit/pqcaton" {
			t.Error("자기 자신을 의존성으로 셌다")
		}
	}
}

// chdirRepoRoot — go.mod 가 있는 곳까지 올라간다. 테스트가 끝나면 되돌린다.
func chdirRepoRoot(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod 를 찾지 못했다 (%s 에서 시작)", cwd)
		}
		dir = parent
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}
