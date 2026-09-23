// Package site — 공개 UI 비교 페이지를 pqcaton-ui에 담는다.
//
// ui-next.html은 GitHub Pages에서 쓰는 독립된 HTML 파일로 유지한다. 로컬 UI 서버도 이 파일을
// 내장하므로, 별도 사본을 관리하지 않고 같은 프로토타입을 낸다.
package site

import _ "embed"

// UINext is the read-only workflow comparison prototype.
//
//go:embed ui-next.html
var UINext []byte
