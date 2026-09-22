// Package site — 공개 UI 비교 페이지를 pqcaton-ui에 담는다.
//
// ui-next.html은 그대로 GitHub Pages의 독립 파일이다. 담긴 바이트를 그 옆에 두면 로컬 UI 서버와
// 공개 사이트가 사본 없이 같은 프로토타입을 그린다.
package site

import _ "embed"

// UINext is the read-only workflow comparison prototype.
//
//go:embed ui-next.html
var UINext []byte
