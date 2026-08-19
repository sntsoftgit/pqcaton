package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
)

// static — 화면이 브라우저로 내보내는 것. **바이너리에 박아 나간다.**
//
// CDN 을 걸지 않는 이유: 이 화면은 망이 끊긴 기계에서도 떠야 한다. 관측 대상 망이
// 바깥으로 나가지 못하는 것은 이 도구를 쓰는 곳에서 오히려 흔한 조건이다. 그리고
// 남의 서버에서 받아 오는 스크립트는 **우리 라이선스 게이트가 볼 수 없다** — 여기 두면
// `checklicenses` 가 licenses.txt 와 맞대 본다.
//
//go:embed static
var static embed.FS

// StaticPath · RowPath — 화면이 스스로를 가리키는 주소.
//
// 템플릿에 문자열로 박아 두면 라우터에서 경로를 옮기는 날 조용히 404 가 된다. 한 자리에
// 두어 **옮기면 같이 옮겨지게** 한다.
const (
	StaticPath = "/static/"
	RowPath    = "/decl/row"
	// ScopeRowPath — 자산 스코프의 규칙 한 줄. 선언의 것과 나눈 것은 계층 번호가
	// 하나 더 붙기 때문이다 — 한 주소에 둘을 밀어 넣으면 둘 다 읽기 어려워진다.
	ScopeRowPath = "/scope/row"
)

// 표의 종류. 「행 추가」가 무엇을 한 줄 더 낼지 고르는 값이다.
const (
	KindNode  = "node"
	KindAsset = "asset"
	KindEdge  = "edge"
)

// ValidKind — 모르는 종류는 받지 않는다. 주소는 밖에서 오는 값이다.
func ValidKind(kind string) bool {
	switch kind {
	case KindNode, KindAsset, KindEdge:
		return true
	}
	return false
}

// Static — `/static/` 아래를 내주는 핸들러.
func Static() http.Handler {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		// embed 는 빌드 시점에 정해진다 — 여기서 실패하면 프로그램이 잘못 만들어진 것이다.
		panic(err)
	}
	return http.StripPrefix(StaticPath, http.FileServer(http.FS(sub)))
}

func rowsID(kind string) string { return "rows-" + kind }

// ruleRowsID · ruleField — 계층마다 표가 따로 있으므로 번호가 이름에 들어간다.
func ruleRowsID(layer int) string { return "rows-rule-" + strconv.Itoa(layer) }

func ruleField(layer, i int, field string) string {
	return "rule." + strconv.Itoa(layer) + "." + strconv.Itoa(i) + "." + field
}

// portText — 포트를 칸에 넣을 문자열로. 0 도 그대로 보인다 — 「안 적었다」와 「0 이라고
// 적었다」를 화면이 대신 판단하지 않는다. 그 판정은 decl.Check 가 한다.
func portText(p uint32) string { return strconv.FormatUint(uint64(p), 10) }

// oobAttr — 「행 추가」 버튼이 자기 자신을 갈아 끼울 때만 붙는 표시.
func oobAttr(oob bool) templ.Attributes {
	if !oob {
		return nil
	}
	return templ.Attributes{"hx-swap-oob": "true"}
}
