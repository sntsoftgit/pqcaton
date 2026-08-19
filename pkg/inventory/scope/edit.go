package scope

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kscope "github.com/randyinthedev-hash/pqcota/pkg/kernel/scope"
)

// LayerFile — 계층 하나와 **그것이 사는 파일**.
//
// 화면이 규칙을 고치면 이 파일에 쓴다. 합본(확정될 정책)을 고치지 않는 이유는 계층이
// 판정의 단위이기 때문이다(§3.4) — 합본에 쓰면 그 규칙이 조직에서 온 것인지 노드군에서
// 온 것인지가 사라지고, 다음 리뷰에서 누구에게 물어야 할지 알 수 없게 된다.
type LayerFile struct {
	Path  string
	Layer Layer
}

// LayerName — 파일 이름이 곧 계층 이름이다. `corp.csv` → `corp`.
func LayerName(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// LoadLayers — 계층 파일을 **준 순서대로** 읽는다. 순서가 곧 상속이다 — 같은 자산에 규칙이
// 여럿 걸리면 뒤 계층의 것이 적용된다.
func LoadLayers(paths []string) ([]LayerFile, error) {
	var out []LayerFile
	for _, p := range paths {
		pol, err := LoadPolicyFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, LayerFile{Path: p, Layer: Layer{Name: LayerName(p), Rules: pol.Rules}})
	}
	return out, nil
}

// SaveLayer — 계층 하나를 그 파일에 CSV 로 쓴다.
//
// **덮어쓰기 전에 임시 파일에 쓰고 옮긴다.** 계층 파일은 사람이 손으로도 고치는 것이고,
// 쓰다 만 CSV 가 남으면 다음에 열 때 규칙이 통째로 사라진 것처럼 보인다.
func SaveLayer(lf LayerFile) error {
	tmp := lf.Path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	err = WriteCSV(f, &kscope.AssetPolicy{Rules: lf.Layer.Rules})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("%s: %w", lf.Path, err)
	}
	return os.Rename(tmp, lf.Path)
}

// Layers — 파일 목록에서 계층만 순서대로 뽑는다.
func Layers(files []LayerFile) []Layer {
	out := make([]Layer, 0, len(files))
	for _, f := range files {
		out = append(out, f.Layer)
	}
	return out
}

// Reopen — 계층이 바뀐 뒤 세션을 **다시 만들되, 사람이 적은 것은 들고 간다.**
//
// 규칙을 고칠 때마다 판정을 처음부터 다시 적게 하면 아무도 화면에서 고치지 않는다. 그래서
// 규칙의 동일성([RuleID])을 열쇠로 결론을 옮긴다 — note 는 동일성에 넣지 않으므로, 설명을
// 다듬은 것만으로는 판정이 날아가지 않는다.
//
// **두 가지는 일부러 버린다:**
//
//   - 계층에 **못 보던 변경이 생겼으면 그 계층의 일괄 결론을 지운다.** 일괄 판정은 「이
//     계층의 변경들을 보고 내린 결론」인데, 새 변경은 사람이 본 적이 없다. 그대로 두면
//     방금 넣은 exclude 가 **누가 승인한 적 없는 근거를 달고** 확정을 통과한다.
//   - 정책 전문이 달라졌으면 **서명을 지운다.** 서명은 그 정책에 대한 것이다. 승인자
//     이름은 남긴다 — 그건 사람이지 확인이 아니다.
func Reopen(prev Session, layers []Layer, base *kscope.AssetPolicy, orgName string) Session {
	next := NewSession(layers, base, orgName)

	was := map[string]string{}
	for _, c := range prev.Changes {
		was[c.ID] = c.Conclusion
	}
	gained := map[string]bool{}
	for i, c := range next.Changes {
		conc, seen := was[c.ID]
		if !seen {
			gained[c.Layer] = true
			continue
		}
		next.Changes[i].Conclusion = conc
	}
	for layer := range next.LayerDecisions {
		if gained[layer] {
			continue
		}
		if v, ok := prev.LayerDecisions[layer]; ok {
			next.LayerDecisions[layer] = v
		}
	}

	next.Reviewer = prev.Reviewer
	if samePolicy(prev.Merged, next.Merged) {
		next.Signature = prev.Signature
	}
	return next
}

// samePolicy — 나갈 CSV 가 같은가. note 까지 본다 — 나가는 파일의 한 칸이다.
func samePolicy(a, b []Rule) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
