package ui

import (
	"fmt"
	"strings"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
)

// 선언 문제의 한국어. **영어는 여기 없다** — `decl` 패키지가 갖는다.
//
// 영어를 두 곳에 두면 한쪽만 고쳐지는 날이 오고, 그날 명령과 화면이 같은 문제를 다르게
// 설명한다. 한국어가 빠진 코드는 영어로 떨어진다 — 빈 칸보다는 낫다.
var (
	koDeclWhat = map[decl.Code]string{
		decl.NodeNotInScope:  "스코프에 없는 노드입니다",
		decl.NodeHasNoIP:     "IP가 없습니다",
		decl.IPMalformed:     "IP 형식이 아닙니다: %s",
		decl.IPClaimedTwice:  "%s 를 여러 노드가 함께 주장합니다",
		decl.NodeMissingIP:   "IP가 없습니다",
		decl.AssetOffScope:   "스코프에 없는 노드를 가리킵니다",
		decl.EdgeSrcOffScope: "보내는 쪽이 스코프에 없습니다",
		decl.EdgePortZero:    "포트가 0입니다",
	}
	koDeclWhy = map[decl.Code]string{
		decl.NodeNotInScope: "대조 대상이 아니므로 이 IP 표는 쓰이지 않습니다",
		decl.NodeHasNoIP: "이 노드로 오는 관측 통신이 어느 노드인지 이어지지 않아 " +
			"<b>선언한 엣지는 미관측으로, 관측된 엣지는 shadow 로</b> 갈립니다",
		decl.IPMalformed:    "잇기는 문자열이 정확히 맞을 때만 됩니다 — 포트나 호스트명을 적으면 맞지 않습니다",
		decl.IPClaimedTwice: "IP가 뒤에 오는 노드로 이어져 **통신이 엉뚱한 노드에 붙습니다**",
		decl.NodeMissingIP: "이 노드로 오는 관측 통신이 어느 노드인지 이어지지 않아 " +
			"<b>선언한 엣지는 미관측으로, 관측된 엣지는 shadow 로</b> 갈립니다",
		decl.AssetOffScope:   "선언만 있고 관측이 이어지지 않아 <b>늘 미관측(UNOBSERVED)</b> 으로 남습니다",
		decl.EdgeSrcOffScope: "관측이 이어지지 않아 늘 미관측으로 남습니다",
		decl.EdgePortZero:    "엣지 동일성에 포트가 들어가므로 관측된 엣지와 맞지 않습니다",
	}
)

func problemWhat(l Lang, p decl.Problem) string {
	if l == KO {
		if t, ok := koDeclWhat[p.Code]; ok {
			return fill(t, p.Detail)
		}
	}
	return p.What()
}

func problemWhy(l Lang, p decl.Problem) string {
	if l == KO {
		if t, ok := koDeclWhy[p.Code]; ok {
			return t
		}
	}
	return p.Why()
}

// whereLabel — 어느 자리인지. **파일 안쪽 표기가 아니라 화면의 절 이름으로 말한다** —
// `scope/pay-db` 를 보여 줘 봐야, 읽는 사람은 그 자리를 화면에서 찾지 못한다.
func whereLabel(l Lang, where string) (section, id string) {
	head, rest, ok := strings.Cut(where, "/")
	if !ok {
		return "", where
	}
	switch head {
	case "nodes", "scope":
		return tWhereNodes.In(l), rest
	case "assets":
		return tWhereAssets.In(l), rest
	case "edges":
		return tWhereEdges.In(l), rest
	}
	return "", where
}

// fill — 값 자리가 있는 문장에만 값을 넣는다.
func fill(tmpl, detail string) string {
	if !strings.Contains(tmpl, "%s") {
		return tmpl
	}
	return fmt.Sprintf(tmpl, detail)
}
