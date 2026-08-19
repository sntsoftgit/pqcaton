// Command checklicenses — 링크되는 의존성의 라이선스를 허용 목록으로 막는다.
//
// 왜 게이트인가: pqcaton은 BUSL-1.1로 내고 Change Date에 Apache-2.0으로 전환한다.
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
// 웹 자산을 뒤늦게 넣은 이유: Go 쪽만 보면 게이트가 절반만 지킨다. 화면에 프런트
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
	"GPL-2.0":  "카피레프트 — 링크 시 파생물도 GPL",
	"GPL-3.0":  "카피레프트 — 링크 시 파생물도 GPL",
	"LGPL-2.1": "약한 카피레프트지만 정적 링크는 전염된다(Go는 정적 링크가 기본)",
	"LGPL-3.0": "약한 카피레프트지만 정적 링크는 전염된다(Go는 정적 링크가 기본)",
	"AGPL-3.0": "카피레프트 — BUSL로 내는 것도 Apache-2.0 전환도 막힌다",
	"MPL-2.0":  "파일 단위 카피레프트 — 상업 배포 시 조건 검토가 필요하다",
	"SSPL-1.0": "OSI 미승인 — 서비스 제공 시 요구가 크다",
}

type mod struct {
	Path    string
	Version string
	Main    bool
}

func main() {
	mods, err := linkedModules()
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ 의존성 목록을 얻지 못했다:", err)
		os.Exit(1)
	}

	// **빈 목록은 통과가 아니다.** 리포 루트가 아닌 곳에서 돌리면 `go list -deps ./...`가
	// 그 디렉터리만 보므로 외부 모듈이 하나도 안 나온다 - 그대로 두면 게이트가 초록으로
	// 통과하고 아무것도 검사하지 않은 것이 된다.
	if len(mods) == 0 {
		fmt.Fprintln(os.Stderr, "✗ 링크되는 외부 모듈이 하나도 없다 — 리포 루트에서 돌리십시오")
		os.Exit(1)
	}

	assets, err := webAssets(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ 웹 자산을 훑지 못했다:", err)
		os.Exit(1)
	}

	known, err := loadAllowlist("licenses.txt")
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ licenses.txt를 읽지 못했다:", err)
		os.Exit(1)
	}

	missing, bad := verdict(append(mods, assets...), known)

	if len(missing) > 0 {
		fmt.Fprintln(os.Stderr, "✗ 라이선스를 모르는 의존성이 있다 — licenses.txt에 확인해 적을 것:")
		for _, p := range missing {
			fmt.Fprintln(os.Stderr, "   ", p)
		}
	}
	if len(bad) > 0 {
		fmt.Fprintln(os.Stderr, "✗ 링크할 수 없는 라이선스다 — Apache-2.0 전환 약속을 지킬 수 없게 된다:")
		for _, s := range bad {
			fmt.Fprintln(os.Stderr, "   ", s)
		}
	}
	if len(missing) > 0 || len(bad) > 0 {
		fmt.Fprintln(os.Stderr, "\n대안을 찾거나, 별도 프로세스로 분리해 호출할 수 있는지 이슈에서 상의할 것.")
		os.Exit(1)
	}
	fmt.Printf("✓ 라이선스 검사 통과 (링크되는 모듈 %d개, 실려 나가는 웹 자산 %d개 — 전부 허용적)\n",
		len(mods), len(assets))
}

// verdict — 링크되는 모듈과 허용 목록을 견줘 **무엇이 막는지** 가른다.
//
// main 에서 떼어 둔 것은 이 판정이 게이트 그 자체이기 때문이다 — 여기가 조용히 통과하면
// 빌드는 초록이고 라이선스 약속만 깨진다. 순수 함수라 케이스가 실제 판정을 잰다.
//
// **모르는 것은 통과시키지 않는다.** 허용 목록에 없으면 금지 목록에 없어도 막는다 —
// go get 한 번이면 새 의존성이 들어오는데, 모르는 것을 통과시키면 게이트가 아니다.
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
			bad = append(bad, fmt.Sprintf("%s = %s — 허용 목록에 없다(검토 필요)", m.Path, lic))
		}
	}
	sort.Strings(missing)
	sort.Strings(bad)
	return missing, bad
}

// webAssets — 바이너리에 박혀 브라우저로 나가는 자산을 전부 모은다.
//
// **확장자로 훑지, 디렉터리 규약을 믿지 않는다.** `vendor/` 아래만 본다면 그 밖에
// 놓은 파일은 검사되지 않는다 — 규약을 어긴 사람이 아니라 모르고 놓은 사람이 게이트를
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
			return nil, fmt.Errorf("형식이 맞지 않는 줄: %q (모듈경로 SPDX식별자)", line)
		}
		out[parts[0]] = parts[1]
	}
	return out, sc.Err()
}
