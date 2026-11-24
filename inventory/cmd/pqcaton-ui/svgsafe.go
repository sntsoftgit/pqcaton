// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// `dot`이 낸 SVG를 화면에 싣기 전의 검사.
//
// 토폴로지의 DOT에는 관측에서 온 문자열이 라벨로 들어간다: 노드 이름, 상대 주소, 그리고 이력에서 그리는 그림에는 실행
// 파일 경로(앱 키)와 라이브러리 이름까지. 화면은 SVG를 문서 안에 그대로 넣으므로, SVG 안의 스크립트 · 이벤트 속성 · 링크 ·
// 바깥 자원은 이 화면의 권한으로 돈다. 그래서 **Graphviz가 그림을 그릴 때 쓰는 요소와 속성만** 받고, 그 밖의 것이 하나라도
// 있으면 그림을 싣지 않는다(화면은 DOT 원문과 내려받기를 보인다). 관측에서 온 문자열은 글자(text · title의 내용)로만 남는다.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// 그림의 상태. 화면이 까닭을 가려 알린다.
const (
	svgDrawn   = ""        // 그렸다
	svgNoDot   = "no-dot"  // `dot`이 없다(선택 사항이다)
	svgFailed  = "failed"  // `dot`이 실패했다
	svgRefused = "refused" // 그렸지만 허용하지 않는 요소나 속성이 있어 싣지 않는다
)

// svgElements — Graphviz가 그림에 쓰는 요소. 링크(a) · 그림(image) · 스크립트 · 스타일 · foreignObject · use는 없다.
var svgElements = map[string]bool{
	"svg": true, "g": true, "title": true, "path": true, "polygon": true, "polyline": true, "ellipse": true, "text": true,
}

// svgAttrs — 그 요소들에 붙는 속성. 이벤트(on…) · 주소(href) · style은 없다.
var svgAttrs = map[string]bool{
	"id": true, "class": true, "transform": true, "width": true, "height": true, "viewBox": true,
	"fill": true, "fill-opacity": true, "stroke": true, "stroke-width": true, "stroke-dasharray": true, "stroke-opacity": true,
	"stroke-linecap": true, "points": true, "d": true, "cx": true, "cy": true, "rx": true, "ry": true, "x": true, "y": true,
	"text-anchor": true, "font-family": true, "font-size": true, "font-weight": true, "font-style": true, "text-decoration": true,
}

// safeSVG — SVG를 읽어 허용한 요소와 속성만으로 다시 쓴다. 허용하지 않는 것이 있으면 오류다(지우고 싣지 않는다: 무엇이
// 빠졌는지 모르는 그림을 보이지 않는다). XML 선언 · DOCTYPE · 주석 · 처리 지시는 버린다. 글자는 다시 이스케이프해 쓴다.
func safeSVG(svg string) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(svg))
	dec.Strict = true
	var out bytes.Buffer
	depth := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("the SVG is not well-formed XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if !svgElements[name] || (t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg") {
				return "", fmt.Errorf("the SVG has an element that is not allowed: %q", name)
			}
			if depth == 0 && name != "svg" {
				return "", fmt.Errorf("the SVG does not start with an svg element")
			}
			depth++
			out.WriteString("<" + name)
			if name == "svg" && depth == 1 {
				out.WriteString(` xmlns="http://www.w3.org/2000/svg"`)
			}
			for _, a := range t.Attr {
				// 이름공간 선언(xmlns · xmlns:xlink)은 버린다. 다시 쓸 때 svg 요소에 하나만 적는다.
				if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
					continue
				}
				if a.Name.Space != "" || !svgAttrs[a.Name.Local] {
					return "", fmt.Errorf("the SVG has an attribute that is not allowed: %q on %q", a.Name.Local, name)
				}
				if strings.Contains(strings.ToLower(a.Value), "url(") || strings.Contains(strings.ToLower(a.Value), "javascript:") {
					return "", fmt.Errorf("the SVG has an attribute value that refers elsewhere: %q on %q", a.Name.Local, name)
				}
				out.WriteString(" " + a.Name.Local + `="`)
				_ = xml.EscapeText(&out, []byte(a.Value))
				out.WriteString(`"`)
			}
			out.WriteString(">")
		case xml.EndElement:
			depth--
			out.WriteString("</" + t.Name.Local + ">")
		case xml.CharData:
			if depth > 0 {
				_ = xml.EscapeText(&out, []byte(t))
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			// 버린다.
		}
	}
	if depth != 0 || out.Len() == 0 {
		return "", fmt.Errorf("the SVG is incomplete")
	}
	return out.String(), nil
}

// renderDOT — `dot`이 있으면 SVG로 그리고, safeSVG를 지난 것만 돌려준다. 없으면 빈 값과 까닭을 돌려주고 화면이 원문을 보인다.
//
// **의존성이 아니라 있으면 좋은 것이다.** 없다고 화면이 깨지면 표준 라이브러리만 쓴다는 약속이 무의미해진다.
func renderDOT(dot string) (svg, state string) {
	bin, err := exec.LookPath("dot")
	if err != nil {
		return "", svgNoDot
	}
	cmd := exec.Command(bin, "-Tsvg")
	cmd.Stdin = strings.NewReader(dot)
	raw, err := cmd.Output()
	if err != nil {
		return "", svgFailed
	}
	safe, err := safeSVG(string(raw))
	if err != nil {
		return "", svgRefused
	}
	return safe, svgDrawn
}
