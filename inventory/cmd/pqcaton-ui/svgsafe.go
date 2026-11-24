// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// `dot`이 낸 SVG를 화면에 싣기 전의 검사.
//
// 토폴로지의 DOT에는 관측에서 온 문자열이 라벨로 들어간다: 노드 이름, 상대 주소, 그리고 이력에서 그리는 그림에는 실행
// 파일 경로(앱 키)와 라이브러리 이름까지. 화면은 SVG를 문서 안에 그대로 넣으므로, SVG 안의 스크립트 · 이벤트 속성 · 링크 ·
// 바깥 자원은 이 화면의 권한으로 돈다. 그래서 **Graphviz가 그림을 그릴 때 쓰는 요소 · 속성 · 그 값의 꼴만** 받고, 그 밖의 것이 하나라도
// 있으면 그림을 싣지 않는다(화면은 DOT 원문과 내려받기를 보인다). 관측에서 온 문자열은 글자(text · title의 내용)로만 남는다.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// 그림의 상태. 화면이 까닭을 가려 알린다.
const (
	svgDrawn    = ""          // 그렸다
	svgNoDot    = "no-dot"    // `dot`이 없다(선택 사항이다)
	svgFailed   = "failed"    // `dot`이 실패했다
	svgRefused  = "refused"   // 그렸지만 허용하지 않는 요소 · 속성 · 값이 있어 싣지 않는다
	svgTimeout  = "timeout"   // `dot`이 정한 시간 안에 끝나지 않았다
	svgTooLarge = "too-large" // 그림이 정한 크기를 넘었다
)

// svgElements — Graphviz가 그림에 쓰는 요소. 링크(a) · 그림(image) · 스크립트 · 스타일 · foreignObject · use는 없다.
var svgElements = map[string]bool{
	"svg": true, "g": true, "title": true, "path": true, "polygon": true, "polyline": true, "ellipse": true, "text": true,
}

// 속성값의 꼴. **이름뿐 아니라 값도 정한 꼴만 받는다.** 색 · 수 · 좌표 · 정해진 낱말 말고는 들어올 자리가 없으므로, 다른
// 문서나 주소를 가리키는 값(`url(…)` 같은 것)은 어느 꼴에도 맞지 않아 그림 전체가 거절된다.
const svgNum = `-?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][-+]?[0-9]+)?`

var (
	reNum       = regexp.MustCompile(`^` + svgNum + `$`)
	reLength    = regexp.MustCompile(`^` + svgNum + `(?:pt|px)?$`)
	reNumList   = regexp.MustCompile(`^(?:` + svgNum + `[ ,]*)+$`)
	reColor     = regexp.MustCompile(`^(?:none|transparent|#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6}|#[0-9a-fA-F]{8}|[a-zA-Z]{1,30})$`)
	reID        = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
	reClass     = regexp.MustCompile(`^[a-z]{1,20}(?: [a-z]{1,20}){0,3}$`)
	reTransform = regexp.MustCompile(`^(?:(?:scale|rotate|translate)\((?:` + svgNum + `[ ,]*){1,3}\) ?){1,4}$`)
	rePath      = regexp.MustCompile(`^[MmLlCcZzHhVvSsQqTtAa0-9eE+\-., ]+$`)
	reFont      = regexp.MustCompile(`^[A-Za-z0-9 ,-]{1,80}$`)
	reWeight    = regexp.MustCompile(`^(?:normal|bold|bolder|lighter|[1-9]00)$`)
	reStyle     = regexp.MustCompile(`^(?:normal|italic|oblique)$`)
	reDecor     = regexp.MustCompile(`^(?:none|underline|overline|line-through)$`)
	reAnchor    = regexp.MustCompile(`^(?:start|middle|end)$`)
	reLinecap   = regexp.MustCompile(`^(?:butt|round|square)$`)
)

// svgAttrs — 그 요소들에 붙는 속성과 그 값의 꼴. 이벤트(on…) · 주소(href) · style은 없다.
var svgAttrs = map[string]*regexp.Regexp{
	"id": reID, "class": reClass, "transform": reTransform, "width": reLength, "height": reLength, "viewBox": reNumList,
	"fill": reColor, "stroke": reColor, "fill-opacity": reNum, "stroke-opacity": reNum, "stroke-width": reNum,
	"stroke-dasharray": reNumList, "stroke-linecap": reLinecap, "points": reNumList, "d": rePath,
	"cx": reNum, "cy": reNum, "rx": reNum, "ry": reNum, "x": reNum, "y": reNum,
	"text-anchor": reAnchor, "font-family": reFont, "font-size": reNum, "font-weight": reWeight, "font-style": reStyle,
	"text-decoration": reDecor,
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
				re := svgAttrs[a.Name.Local]
				if a.Name.Space != "" || re == nil {
					return "", fmt.Errorf("the SVG has an attribute that is not allowed: %q on %q", a.Name.Local, name)
				}
				if !re.MatchString(a.Value) {
					return "", fmt.Errorf("the SVG has an attribute value that is not allowed: %q on %q", a.Name.Local, name)
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

// 그림을 그리는 데 두는 상한. 조직 전체를 그리는 그림은 노드가 많으면 오래 걸리고 커진다. 상한을 넘으면 그리지 않고
// 까닭을 알린다. 요약 표와 DOT 내려받기는 그림과 따로 돈다.
var (
	dotTimeout = 10 * time.Second
	dotMaxSVG  = 4 << 20 // 4 MiB
)

var errDotTooLarge = errors.New("the drawing is larger than the limit")

// runDot — `dot -Tsvg`를 돌려 SVG를 받는다. 상한(시간 · 출력 크기)을 넘으면 프로세스를 끝낸다. 시험이 바꾼다.
var runDot = func(ctx context.Context, dot string) ([]byte, error) {
	bin, err := exec.LookPath("dot")
	if err != nil {
		return nil, exec.ErrNotFound
	}
	cmd := exec.CommandContext(ctx, bin, "-Tsvg")
	cmd.Stdin = strings.NewReader(dot)
	cmd.WaitDelay = time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	raw, rerr := io.ReadAll(io.LimitReader(stdout, int64(dotMaxSVG)+1))
	if len(raw) > dotMaxSVG {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, errDotTooLarge
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return raw, rerr
}

// 마지막으로 그린 그림 하나. 같은 DOT을 다시 그리지 않는다(같은 이력에서 화면을 다시 열거나 표의 노드만 바꿀 때). 그리는
// 동안에는 잠가 두어 같은 그림을 동시에 여러 번 그리지 않는다.
var dotCache struct {
	sync.Mutex
	key        [sha256.Size]byte
	svg, state string
	filled     bool
}

// renderDOT — `dot`이 있으면 SVG로 그리고, safeSVG를 지난 것만 돌려준다. 그리지 못했으면 빈 값과 까닭을 돌려주고 화면이
// 원문을 보인다. 04와 이력 화면이 함께 쓴다.
//
// **의존성이 아니라 있으면 좋은 것이다.** 없다고 화면이 깨지면 표준 라이브러리만 쓴다는 약속이 무의미해진다.
func renderDOT(dot string) (svg, state string) {
	key := sha256.Sum256([]byte(dot))
	dotCache.Lock()
	defer dotCache.Unlock()
	if dotCache.filled && dotCache.key == key {
		return dotCache.svg, dotCache.state
	}
	svg, state = drawDOT(dot)
	// `dot`이 없는 것은 담아 두지 않는다. 설치한 뒤 화면을 다시 열면 바로 그려야 한다.
	if state != svgNoDot {
		dotCache.key, dotCache.svg, dotCache.state, dotCache.filled = key, svg, state, true
	}
	return svg, state
}

func drawDOT(dot string) (svg, state string) {
	ctx, cancel := context.WithTimeout(context.Background(), dotTimeout)
	defer cancel()
	raw, err := runDot(ctx, dot)
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return "", svgNoDot
	case errors.Is(err, errDotTooLarge):
		return "", svgTooLarge
	case ctx.Err() != nil:
		return "", svgTimeout
	case err != nil:
		return "", svgFailed
	}
	safe, err := safeSVG(string(raw))
	if err != nil {
		return "", svgRefused
	}
	return safe, svgDrawn
}
