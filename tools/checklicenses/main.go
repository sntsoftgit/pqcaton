// Command checklicenses — 링크되는 의존성의 라이선스를 허용 목록으로 막는다.
//
// 왜 게이트인가: pqcaton은 BUSL-1.1로 내고 Change Date에 Apache-2.0으로 전환한다.
// 카피레프트 라이브러리를 하나라도 링크하면 파생물 전체가 그 조건을 따라야 해서,
// 둘 다 막힌다. 라이선스가 약속한 것을 지킬 수 없게 된다.
//
// 사람 기억에 맡기면 새어 든다 — go get 한 번이면 들어온다. 그래서 빌드에서 막는다.
//
// 검사 대상은 `go list -deps ./...`가 내는 **실제 링크되는 모듈**이다. 테스트 전용이나
// 별도 프로세스로 부르는 도구는 대상이 아니다(링크되지 않으므로 전염되지 않는다).
//
// usage: go run ./tools/checklicenses
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

	known, err := loadAllowlist("licenses.txt")
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ licenses.txt를 읽지 못했다:", err)
		os.Exit(1)
	}

	missing, bad := verdict(mods, known)

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
	fmt.Printf("✓ 라이선스 검사 통과 (링크되는 모듈 %d개 — 전부 허용적)\n", len(mods))
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
