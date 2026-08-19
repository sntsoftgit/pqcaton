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
 <p class="hint">정책마다 <b>판정</b>(결론)을 적고, 여기에 <b>승인</b>자와 서명을 채운 뒤,
  <b>확정</b>하면 계획이 나갑니다. 셋이 다 있어야 통과합니다.</p>
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
 <legend>관리 대상 노드 <span class="n">— 어느 노드를 볼 것인가</span></legend>
 <p class="hint">한 줄에 하나. 여기 없는 상대와의 통신은 「등재 판정 요청」으로 표기됩니다.
  <b>노드 안에서 무엇을 볼지</b>는 「자산 스코프」 탭에서 정합니다.</p>
 <textarea name="scope">{{range .Decl.Scope}}{{.}}
{{end}}</textarea>
</fieldset>

<fieldset>
 <legend>노드 주소 <span class="n">— 관측 IP를 노드 이름으로 잇는 근거</span></legend>
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

// surveyPage — 대조 결과. **관측을 먼저 보인다** — 그것 없이는 UNOBSERVED가 「없다」인지
// 「못 봤다」인지 읽는 사람이 가를 수 없다(§2.7).
var surveyPage = template.Must(template.New("survey").Funcs(funcs).Parse(shell + `
{{template "head" .Page}}

<fieldset>
 <legend>관측 <span class="n">— pqcota가 무엇을 보았나</span></legend>
 <p class="hint">대상 노드에 collector를 반입·실행·회수했습니다. 노드에는 아무것도 남지 않습니다.</p>
 <table>
  <tr><th style="width:14rem">노드</th><th>본 collector</th></tr>
  {{range .R.SeenNodes}}<tr><td><code>{{.}}</code></td><td>{{range $k, $c := index $.R.SeenBy .}}{{if $k}}, {{end}}{{$c}}{{end}}</td></tr>{{end}}
 </table>
 <p style="margin-top:.8rem">관측 자산 <b>{{.R.ObservedAssets}}</b> · 협상된 통신 <b>{{.R.ObservedEdges}}</b>건</p>

 <div class="warn-list" style="border-color:var(--line)">
  <strong>못 본 것</strong>
  {{if and (not .R.GapLayers) (not .R.UncoveredNodes)}}
   <p class="hint">없습니다 — 이 범위에서는 관측이 완전합니다.</p>
  {{else}}
   <ul>
   {{range .R.GapLayers}}<li>계층 <code>{{.}}</code></li>{{end}}
   {{range .R.UncoveredNodes}}<li>통신 미관측 노드 <code>{{.}}</code></li>{{end}}
   </ul>
   <p class="hint"><b>못 본 것과 없는 것은 다릅니다.</b> 아래 UNOBSERVED가 어느 쪽인지는 이 줄이 가릅니다 —
    갭이면 재수집이 먼저이고, 아니면 사람이 판정합니다.</p>
  {{end}}
 </div>
 {{if .R.Skipped}}<div class="warn-list"><strong>읽지 못한 결과 파일</strong>
  <ul>{{range .R.Skipped}}<li>{{.}}</li>{{end}}</ul>
  <p class="hint">빠진 노드를 모르면 「관측 안 됨」과 「못 읽음」이 뒤섞입니다.</p></div>{{end}}
</fieldset>

<fieldset>
 <legend>자산 <span class="n">— 선언과 맞댄 3-상태</span></legend>
 <p>CONFIRMED <b>{{.Confirmed}}</b> · UNDECLARED(shadow) <b>{{.Undeclared}}</b> · UNOBSERVED <b>{{.Unobserved}}</b></p>
 <table>
  <tr><th>상태</th><th>확신</th><th>노드</th><th>런타임</th><th>컴포넌트</th></tr>
  {{range .Assets}}
  <tr><td{{if ne .State "CONFIRMED"}} class="must"{{end}}>{{.State}}</td>
      <td>{{printf "%.2f" .Conf}}</td>
      <td><code>{{.Node}}</code></td><td>{{.Runtime}}</td>
      <td>{{.Component}}{{if .Rescan}} <span class="hint">재수집 후보</span>{{end}}</td></tr>
  {{else}}<tr><td colspan="5">대조할 자산이 없습니다.</td></tr>{{end}}
 </table>
 <p class="hint"><b>UNDECLARED가 이 도구의 첫 값입니다</b> — 선언에 없는데 실제로 쓰이고 있는 것입니다.
  판정은 <a href="/review">리뷰 큐</a>에서 합니다.</p>
</fieldset>

<fieldset>
 <legend>통신 엣지 <span class="n">— 선언과 맞댄 3-상태 · 양자내성 등급</span></legend>
 <p>🟢 PQC <b>{{.PQC}}</b> · 🔴 고전 <b>{{.Classical}}</b> · ⚪ 불명 <b>{{.Unknown}}</b></p>
 <table>
  <tr><th style="width:2rem"></th><th>보내는 쪽</th><th>받는 쪽</th><th>포트</th><th>프로토콜</th><th>협상 그룹</th><th>대조</th></tr>
  {{range .Edges}}
  <tr><td>{{.Mark}}</td>
      <td><code>{{.Src}}</code>{{if .Uncovered}} <span class="hint">미관측</span>{{end}}</td>
      <td><code>{{.Dst}}</code>{{if .OffScope}} <span class="hint">등재 판정 요청</span>{{end}}</td>
      <td>{{.Port}}</td><td>{{.Proto}}</td>
      <td>{{if .Group}}{{.Group}}{{else}}<span class="hint">{{.Grade}}</span>{{end}}</td>
      <td{{if ne .State "CONFIRMED"}} class="must"{{end}}>{{.State}}</td></tr>
  {{else}}<tr><td colspan="7">대조할 엣지가 없습니다.</td></tr>{{end}}
 </table>
</fieldset>

<fieldset>
 <legend>토폴로지 <span class="n">— 색은 등급, 선형은 대조 상태</span></legend>
 {{if .SVG}}
  <div style="overflow-x:auto">{{.SVG}}</div>
  <p class="hint">색은 양자내성 등급, 선형은 대조 상태입니다. <b>미관측은 점선</b>이라 부재와 구분됩니다.</p>
 {{else}}
  <p class="hint"><code>dot</code>(Graphviz)이 이 기계에 없어 그리지 못했습니다.
   <b>선택 사항입니다</b> — 설치하지 않아도 나머지는 그대로 돕니다(README 「사전 준비」).
   <code>apt install graphviz</code> · <code>brew install graphviz</code> ·
   <code>winget install graphviz</code>.<br>
   설치하지 않고 그리려면 아래를 저장해 아무 데서나
   <code>dot -Tsvg topology.dot -o topology.svg</code> 로 그리십시오.</p>
  <textarea readonly style="min-height:12rem">{{.DOT}}</textarea>
 {{end}}
</fieldset>
{{template "foot"}}`))

// scopePage — 자산 스코프. **exclude 추가만 근거 필수**라, 그 무게 차이가 화면에 보여야 한다.
var scopePage = template.Must(template.New("scope").Funcs(funcs).Parse(shell + `
{{template "head" .Page}}

<p class="hint">「이 자산은 안 본다」는 <b>사고 뒤에 근거를 대야 하는 결정</b>입니다.
 계층은 준 순서대로 이기고(조직 · 환경 · 노드군), 바뀐 규칙만 올라옵니다.</p>

<form method="post" action="/scope/finalize">
{{range .Layers}}
 <fieldset>
  <legend>{{.Name}} <span class="n">— 변경 {{len .Changes}}건{{if .Audited}}, 근거 필수 {{.Audited}}{{end}}</span></legend>
  <label>이 계층의 결론 <span class="hint">(적으면 아래 변경이 한 번에 판정됩니다)</span>
   <input type="text" name="layer:{{.Name}}" value="{{.Conclusion}}" placeholder="예: OS 패치로 관리하므로 인벤토리에서 뺀다"></label>
  <table>
   <tr><th style="width:4rem">변경</th><th>규칙</th><th>설명</th><th>개별 결론(예외)</th></tr>
   {{range .Changes}}
   <tr>
    <td{{if .Audited}} class="must"{{end}}>{{.Kind}}</td>
    <td><code>{{.Rule}}</code></td>
    <td class="hint">{{.Note}}</td>
    <td><input type="text" name="change:{{.ID}}" value="{{.Conclusion}}"></td>
   </tr>
   {{end}}
  </table>
 </fieldset>
{{else}}
 <p>바뀐 규칙이 없습니다 — 승인할 것이 없습니다.</p>
{{end}}

<fieldset>
 <legend>승인</legend>
 <p class="hint">계층마다 <b>판정</b>(결론)을 적고, 여기에 <b>승인</b>자와 서명을 채운 뒤,
  <b>확정</b>하면 정책이 나갑니다. 셋이 다 있어야 통과합니다.</p>
 <label>승인자 <input type="text" name="reviewer" value="{{.Session.Reviewer}}"></label>
 <label style="display:block;margin-top:.6rem">서명 <input type="text" name="signature" value="{{.Session.Signature}}"></label>
</fieldset>

<fieldset>
 <legend>확정될 정책 <span class="n">— 전문 {{len .Session.Merged}}줄</span></legend>
 <p class="hint"><b>바뀐 것만 리뷰하되 나가는 것은 전문입니다</b> — pqcota의 집행기는 정책 전체를 받습니다.</p>
 <table>
  <tr><th style="width:6rem">action</th><th style="width:6rem">runtime</th><th>lib</th><th>app_key</th><th>note</th></tr>
  {{range .Session.Merged}}
  <tr><td{{if eq .Action "exclude"}} class="must"{{end}}>{{.Action}}</td>
      <td>{{.Runtime}}</td><td><code>{{.Lib}}</code></td><td><code>{{.AppKey}}</code></td>
      <td class="hint">{{.Note}}</td></tr>
  {{end}}
 </table>
</fieldset>

<div class="actions">
 <button formaction="/scope/save">저장만</button>
 <button formaction="/scope/finalize" class="primary">확정하고 정책 내기</button>
</div>
<p class="hint">확정은 <code>pqcaton-scope close</code> 와 같은 게이트를 탑니다. 나온 CSV 가
 <code>pqcota-ingest -scope-assets</code> 의 입력입니다.</p>
</form>
{{template "foot"}}`))
