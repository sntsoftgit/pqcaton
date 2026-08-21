// Command checklicenses — 링크되는 의존성의 라이선스를 허용 목록으로 막는다.
//
// 왜 관문인가: pqcaton은 BUSL-1.1로 내고 Change Date에 Apache-2.0으로 전환한다.
// 카피레프트 라이브러리를 하나라도 링크하면 파생물 전체가 그 조건을 따라야 해서,
// 둘 다 막힌다. 라이선스가 약속한 것을 지킬 수 없게 된다.
//
// 사람 기억에 맡기면 새어 든다 — go get 한 번이면 들어온다. 그래서 빌드에서 막는다.
//
// 검사 대상은 **함께 실려 나가는 것 전부**다. 두 갈래다:
//
//   - `go list -deps ./...`가 내는 **실제 링크되는 모듈**
//   - 바이너리에 박히는 **웹 자산**(`.js`·`.css`) — 화면이 브라우저로 내보내는 것
//
// 웹 자산을 뒤늦게 넣은 이유: Go 쪽만 보면 관문이 절반만 지킨다. 화면에 프런트
// 라이브러리를 하나 받아 넣는 순간, **이 리포가 배포하는 코드의 일부가 목록 밖에**
// 생긴다 — 「무엇을 쓰는지 모르면 이관도 못 한다」고 말하는 리포에서.
//
// 테스트 전용이나 별도 프로세스로 부르는 도구는 대상이 아니다(링크되지 않으므로
// 전염되지 않고, 실려 나가지도 않는다).
//
// usage: go run ./tools/checklicenses
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// 허용 — 허용적(permissive) 라이선스만. 저작권 고지 유지 의무만 있고 파생물의
// 라이선스를 강제하지 않는다.
var allowed = map[string]bool{
	"MIT":          true,
	"Apache-2.0":   true,
	"BSD-2-Clause": true,
	"BSD-3-Clause": true,
	"ISC":          true,
	"Unlicense":    true,
	"0BSD":         true, // 고지 의무조차 없다 — BSD 계열 중 가장 느슨하다
	// BUSL-1.1 은 **이 리포 자신의 라이선스**다. 우리가 쓴 웹 자산을 목록에 적을 때
	// 쓴다. 남의 코드에 이 값이 붙어 있으면 그건 검토 대상이다 — 그때는 손으로 본다.
	"BUSL-1.1": true,
}

// 금지 — 카피레프트. 링크하면 파생물도 같은 조건이 되어 BUSL로 낼 수 없다.
// (별도 프로세스로 호출하는 것은 링크가 아니라 여기 걸리지 않는다.)
var forbidden = map[string]string{
	"GPL-2.0":  "copyleft — linking makes the derivative GPL too",
	"GPL-3.0":  "copyleft — linking makes the derivative GPL too",
	"LGPL-2.1": "weak copyleft, but static linking still infects (Go links statically by default)",
	"LGPL-3.0": "weak copyleft, but static linking still infects (Go links statically by default)",
	"AGPL-3.0": "copyleft — blocks both shipping under BUSL and the Apache-2.0 change",
	"MPL-2.0":  "file-level copyleft — commercial distribution needs a terms review",
	"SSPL-1.0": "not OSI-approved — heavy obligations when offering a service",
}

type mod struct {
	Path    string
	Version string
	Main    bool
}

func main() {
	mods, err := linkedModules()
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ could not list dependencies:", err)
		os.Exit(1)
	}

	// **빈 목록은 통과가 아니다.** 리포 루트가 아닌 곳에서 돌리면 `go list -deps ./...`가
	// 그 디렉터리만 보므로 외부 모듈이 하나도 안 나온다 - 그대로 두면 관문이 초록으로
	// 통과하고 아무것도 검사하지 않은 것이 된다.
	if len(mods) == 0 {
		fmt.Fprintln(os.Stderr, "✗ no external module is linked at all — run this from the repo root")
		os.Exit(1)
	}

	assets, err := webAssets(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ could not walk the web assets:", err)
		os.Exit(1)
	}

	known, err := loadAllowlist("licenses.txt")
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ could not read licenses.txt:", err)
		os.Exit(1)
	}

	missing, bad := verdict(append(mods, assets...), known)

	if len(missing) > 0 {
		fmt.Fprintln(os.Stderr, "✗ dependencies with an unknown license — check and record them in licenses.txt:")
		for _, p := range missing {
			fmt.Fprintln(os.Stderr, "   ", p)
		}
	}
	if len(bad) > 0 {
		fmt.Fprintln(os.Stderr, "✗ this license cannot be linked — it would break the promised Apache-2.0 change:")
		for _, s := range bad {
			fmt.Fprintln(os.Stderr, "   ", s)
		}
	}
	if len(missing) > 0 || len(bad) > 0 {
		fmt.Fprintln(os.Stderr, "\nFind an alternative, or discuss in an issue whether it can be split out into a separate process.")
		os.Exit(1)
	}
	fmt.Printf("✓ license check passed (%d linked modules, %d shipped web assets — all permissive)\n",
		len(mods), len(assets))
}

// verdict — 링크되는 모듈과 허용 목록을 견줘 **무엇이 막는지** 가른다.
//
// main 에서 떼어 둔 것은 이 판정이 관문 그 자체이기 때문이다 — 여기가 조용히 통과하면
// 빌드는 초록이고 라이선스 약속만 깨진다. 순수 함수라 케이스가 실제 판정을 잰다.
//
// **모르는 것은 통과시키지 않는다.** 허용 목록에 없으면 금지 목록에 없어도 막는다 —
// go get 한 번이면 새 의존성이 들어오는데, 모르는 것을 통과시키면 관문이 아니다.
func verdict(mods []mod, known map[string]string) (missing, bad []string) {
	for _, m := range mods {
		lic, ok := known[m.Path]
		if !ok {
			missing = append(missing, m.Path)
			continue
		}
		if why, no := forbidden[lic]; no {
			bad = append(bad, fmt.Sprintf("%s = %s — %s", m.Path, lic, why))
			continue
		}
		if !allowed[lic] {
			bad = append(bad, fmt.Sprintf("%s = %s — not on the allowlist (needs review)", m.Path, lic))
		}
	}
	sort.Strings(missing)
	sort.Strings(bad)
	return missing, bad
}

// webAssets — 바이너리에 박혀 브라우저로 나가는 자산을 전부 모은다.
//
// **확장자로 훑지, 디렉터리 규약을 믿지 않는다.** `vendor/` 아래만 본다면 그 밖에
// 놓은 파일은 검사되지 않는다 — 규약을 어긴 사람이 아니라 모르고 놓은 사람이 관문을
// 지나가게 된다. 우리가 쓴 파일도 목록에 적어야 하지만, 그 값은 BUSL-1.1 한 줄이다.
//
// 경로는 리포 상대 경로 그대로 쓴다. 같은 파일을 두 곳에 두면 두 줄이 필요하고,
// 그건 사본이 있다는 사실이 드러나는 것이니 손해가 아니다.
func webAssets(root string) ([]mod, error) {
	var out []mod
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// `.git`·`node_modules`는 배포물이 아니다.
			if n := d.Name(); n == ".git" || n == "node_modules" || n == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".js", ".mjs", ".css":
			out = append(out, mod{Path: filepath.ToSlash(strings.TrimPrefix(p, "./"))})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// linkedModules — 실제로 링크되는 모듈만. 테스트 전용 의존성은 제외된다.
func linkedModules() ([]mod, error) {
	out, err := exec.Command("go", "list", "-deps", "-json", "./...").Output()
	if err != nil {
		return nil, err
	}
	seen := map[string]mod{}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var pkg struct{ Module *mod }
		if err := dec.Decode(&pkg); err != nil {
			break
		}
		if pkg.Module == nil || pkg.Module.Main || pkg.Module.Path == "" {
			continue
		}
		seen[pkg.Module.Path] = *pkg.Module
	}
	var mods []mod
	for _, m := range seen {
		mods = append(mods, m)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Path < mods[j].Path })
	return mods, nil
}

// loadAllowlist — "모듈경로 SPDX식별자" 한 줄씩. `#`로 시작하면 주석.
func loadAllowlist(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil, fmt.Errorf("malformed line: %q (want: <module path> <SPDX id>)", line)
		}
		out[parts[0]] = parts[1]
	}
	return out, sc.Err()
}
