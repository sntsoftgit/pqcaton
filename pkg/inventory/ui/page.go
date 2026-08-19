package ui

import "html/template"

// 화면 두 장이 **같은 껍데기**를 쓴다. 머리글·이동 링크·알림 상자가 화면마다 달라지면
// 쓰는 사람이 다른 프로그램으로 여긴다.
const shell = `{{define "head"}}<!doctype html>
<html lang="ko"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — pqcaton</title>
<style>
 :root { color-scheme: light dark; --line:#8883; --warn:#b3261e; --ok:#146c2e; --dim:#7a7a7a; }
 body { font: 15px/1.6 system-ui, -apple-system, "Apple SD Gothic Neo", sans-serif;
        max-width: 64rem; margin: 2rem auto; padding: 0 1rem; }
 h1 { font-size: 1.3rem; margin-bottom: .2rem; }
 h2 { font-size: 1rem; margin: 1.6rem 0 .4rem; }
 .sub { color: var(--dim); margin-top: 0; }
 nav { display: flex; gap: .8rem; margin: 0 0 1.2rem; padding-bottom: .6rem;
       border-bottom: 1px solid var(--line); font-size: .9rem; }
 nav a { color: inherit; text-decoration: none; opacity: .65; }
 nav a.here { opacity: 1; font-weight: 600; }
 fieldset { border: 1px solid var(--line); border-radius: 8px; margin: 1rem 0; padding: 1rem; }
 legend { font-weight: 600; padding: 0 .4rem; }
 legend .n { font-weight: 400; color: var(--dim); }
 input[type=text], textarea { width: 100%; padding: .45rem .6rem; border: 1px solid var(--line);
        border-radius: 6px; background: transparent; color: inherit; font: inherit; }
 textarea { min-height: 5rem; }
 table { border-collapse: collapse; width: 100%; margin-top: .7rem; }
 th, td { text-align: left; padding: .35rem .5rem; border-top: 1px solid var(--line); font-size: .9rem; }
 th { color: var(--dim); font-weight: 500; }
 td input[type=text] { padding: .3rem .45rem; }
 .must { color: var(--warn); font-weight: 600; }
 .msg { padding: .7rem 1rem; border-radius: 8px; border: 1px solid var(--line); }
 .msg.ok { border-color: var(--ok); }
 .msg.bad { border-color: var(--warn); }
 pre { white-space: pre-wrap; margin: .4rem 0 0; font-size: .85rem; }
 .actions { display: flex; gap: .6rem; margin-top: 1.2rem; }
 button { font: inherit; padding: .5rem 1rem; border-radius: 6px; border: 1px solid var(--line);
          background: transparent; color: inherit; cursor: pointer; }
 button.primary { border-color: var(--ok); font-weight: 600; }
 .hint { color: var(--dim); font-size: .85rem; }
 .warn-list { border: 1px solid var(--warn); border-radius: 8px; padding: .6rem 1rem; margin: 1rem 0; }
 .warn-list li { margin: .35rem 0; font-size: .9rem; }
 .warn-list code { color: var(--dim); }
</style></head><body>

<h1>{{.Title}}</h1>
<p class="sub">{{.Subtitle}}</p>
{{if .Nav}}<nav>{{range .Nav}}<a href="{{.Href}}"{{if .Here}} class="here"{{end}}>{{.Text}}</a>{{end}}</nav>{{end}}
{{if .Message}}<p class="msg ok">{{.Message}}</p>{{end}}
{{if .Problem}}<div class="msg bad"><strong>하지 않았습니다.</strong><pre>{{.Problem}}</pre></div>{{end}}
{{end}}

{{define "foot"}}</body></html>{{end}}`

// reviewPage — 리뷰 큐. 정책으로 묶고, 정책마다 결론 칸 하나를 준다.
var reviewPage = template.Must(template.New("review").Funcs(funcs).Parse(shell + `
{{template "head" .Page}}
<form method="post">
{{range .Policies}}
 <fieldset>
  <legend>{{.Name}} <span class="n">— 항목 {{len .Items}}개{{if .Mandatory}}, 필수 {{.Mandatory}}{{end}}</span></legend>
  <label>이 정책의 결론 <span class="hint">(적으면 아래 항목이 한 번에 판정됩니다)</span>
   <input type="text" name="policy:{{.Name}}" value="{{.Conclusion}}" placeholder="예: PQC 라이브러리로 교체한다"></label>
  <table>
   <tr><th>대상</th><th>상태</th><th>확신</th><th>계획</th><th>개별 결론(예외)</th></tr>
   {{range .Items}}
   <tr>
    <td><code>{{.ID}}</code>{{if .Rescan}} <span class="hint">재수집 후보</span>{{end}}</td>
    <td{{if .Mandatory}} class="must"{{end}}>{{.State}}</td>
    <td>{{printf "%.2f" .Conf}}</td>
    <td><input type="checkbox" name="plan:{{.ID}}" {{if .Plan}}checked{{end}}></td>
    <td><input type="text" name="item:{{.ID}}" value="{{.Conclusion}}"></td>
   </tr>
   {{end}}
  </table>
 </fieldset>
{{else}}
 <p>판정할 것이 없습니다.</p>
{{end}}
{{if .Autopass}}<p class="hint">자동통과 후보 {{.Autopass}}개는 판정 대상이 아닙니다.</p>{{end}}

<fieldset>
 <legend>승인</legend>
 <p class="hint">서명 없이는 확정되지 않습니다.</p>
 <label>승인자 <input type="text" name="reviewer" value="{{.Reviewer}}"></label>
 <label style="display:block;margin-top:.6rem">서명 <input type="text" name="signature" value="{{.Signature}}"></label>
</fieldset>

<div class="actions">
 <button formaction="/save">저장만</button>
 <button formaction="/finalize" class="primary">확정하고 계획 내기</button>
</div>
<p class="hint">확정은 <code>pqcaton-decide close</code> 와 같은 게이트를 탑니다 — 필수 항목의 결론과 서명이 모두 있어야 통과합니다.</p>
</form>
{{template "foot"}}`))

// declPage — 선언 편집. **틀린 자리를 위에 먼저 보여 준다** — 저장은 막지 않되, 그대로
// 두면 대조가 조용히 틀린다는 것을 사람이 알아야 한다.
var declPage = template.Must(template.New("decl").Funcs(funcs).Parse(shell + `
{{template "head" .Page}}

{{if .Problems}}
<div class="warn-list">
 <strong>선언이 앞뒤가 맞지 않는 자리 {{len .Problems}}곳</strong>
 <p class="hint">저장은 됩니다. 다만 그대로 두면 대조 결과가 <b>오류 없이 틀립니다.</b></p>
 <ul>{{range .Problems}}<li><code>{{.Where}}</code> — {{.What}}<br><span class="hint">{{.Why}}</span></li>{{end}}</ul>
</div>
{{end}}

<form method="post" action="/decl/save">
<fieldset>
 <legend>조직</legend>
 <p class="hint">대조 엔진이 이 값으로 열립니다. 비우면 <code>local</code> 입니다.</p>
 <input type="text" name="org" value="{{.Decl.Org}}" placeholder="local">
</fieldset>

<fieldset>
 <legend>스코프 <span class="n">— 등재된 노드</span></legend>
 <p class="hint">한 줄에 하나. 여기 없는 상대와의 통신은 「등재 판정 요청」으로 표기됩니다.</p>
 <textarea name="scope">{{range .Decl.Scope}}{{.}}
{{end}}</textarea>
</fieldset>

<fieldset>
 <legend>노드 주소 <span class="n">— 관측 IP를 노드로 해소하는 근거</span></legend>
 <p class="hint">한 노드가 망 둘에 걸치면 IP도 둘입니다. 쉼표나 공백으로 나눠 적으십시오.
  이름을 비우면 그 줄은 지워집니다.</p>
 <table>
  <tr><th style="width:14rem">이름</th><th>IP</th></tr>
  {{range $i, $n := .Decl.Nodes}}
  <tr><td><input type="text" name="node.name.{{$i}}" value="{{$n.Name}}"></td>
      <td><input type="text" name="node.ips.{{$i}}" value="{{range $k, $ip := $n.IPs}}{{if $k}}, {{end}}{{$ip}}{{end}}"></td></tr>
  {{end}}
  {{range $j := seq .Blank}}
  <tr><td><input type="text" name="node.name.{{add (len $.Decl.Nodes) $j}}" value=""></td>
      <td><input type="text" name="node.ips.{{add (len $.Decl.Nodes) $j}}" value=""></td></tr>
  {{end}}
 </table>
</fieldset>

<fieldset>
 <legend>자산 <span class="n">— 「이 노드에서 이것을 쓴다」</span></legend>
 <table>
  <tr><th style="width:12rem">노드</th><th style="width:8rem">런타임</th><th>컴포넌트</th></tr>
  {{range $i, $a := .Decl.Assets}}
  <tr><td><input type="text" name="asset.node.{{$i}}" value="{{$a.Node}}"></td>
      <td><input type="text" name="asset.runtime.{{$i}}" value="{{$a.Runtime}}"></td>
      <td><input type="text" name="asset.component.{{$i}}" value="{{$a.Component}}"></td></tr>
  {{end}}
  {{range $j := seq .Blank}}
  <tr><td><input type="text" name="asset.node.{{add (len $.Decl.Assets) $j}}" value=""></td>
      <td><input type="text" name="asset.runtime.{{add (len $.Decl.Assets) $j}}" value="" placeholder="openssl | jca"></td>
      <td><input type="text" name="asset.component.{{add (len $.Decl.Assets) $j}}" value=""></td></tr>
  {{end}}
 </table>
</fieldset>

<fieldset>
 <legend>통신 엣지 <span class="n">— 「이 노드가 저 노드와 이렇게 통신한다」</span></legend>
 <table>
  <tr><th style="width:11rem">보내는 쪽</th><th style="width:11rem">받는 쪽</th><th style="width:6rem">포트</th><th>프로토콜</th></tr>
  {{range $i, $e := .Decl.Edges}}
  <tr><td><input type="text" name="edge.src.{{$i}}" value="{{$e.Src}}"></td>
      <td><input type="text" name="edge.dst.{{$i}}" value="{{$e.Dst}}"></td>
      <td><input type="text" name="edge.port.{{$i}}" value="{{$e.Port}}"></td>
      <td><input type="text" name="edge.proto.{{$i}}" value="{{$e.Proto}}"></td></tr>
  {{end}}
  {{range $j := seq .Blank}}
  <tr><td><input type="text" name="edge.src.{{add (len $.Decl.Edges) $j}}" value=""></td>
      <td><input type="text" name="edge.dst.{{add (len $.Decl.Edges) $j}}" value=""></td>
      <td><input type="text" name="edge.port.{{add (len $.Decl.Edges) $j}}" value=""></td>
      <td><input type="text" name="edge.proto.{{add (len $.Decl.Edges) $j}}" value="" placeholder="TLS | SSH"></td></tr>
  {{end}}
 </table>
</fieldset>

<div class="actions"><button class="primary">저장</button></div>
<p class="hint">저장하면 <code>pqcaton-report</code> 가 읽는 그 파일에 그대로 씁니다.</p>
</form>
{{template "foot"}}`))
