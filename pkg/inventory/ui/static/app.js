// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// 행 추가 · 제거 뒤의 포커스.
//
// 「행 추가」를 누르면 새 행의 첫 칸으로, 「제거」를 누르면 다음 행의 첫 칸으로 포커스를 옮긴다.
// 다음 행이 없거나 그 사이 사라졌으면 그 표의 「행 추가」 버튼으로 간다. 옮기지 않으면 포커스가
// 지워진 버튼과 함께 사라져 키보드로 쓰는 사람은 문서 처음부터 다시 Tab을 눌러야 한다.
//
// 옮길 곳은 바꾸기 직전(htmx:beforeSwap)에 기억하고, 바꾼 뒤(htmx:afterSettle)에 옮긴다. 지워진
// 행과 갈아 끼운 버튼에서 나는 이벤트는 문서까지 올라오지 않으므로, 같은 일을 짧은 시간 뒤에 한
// 번 더 시도한다(먼저 된 쪽만 옮긴다).
//
// 양수 tabindex를 쓰지 않고 :focus-visible을 지우지 않는다. 버튼은 data-focus로 고른다:
// "new"는 행 추가, "next"는 제거다. 파일은 아무것도 바꾸지 않는다.
(function () {
  "use strict";

  var controls = "input:not([type=hidden]), select, textarea, button";
  var pending = null;

  function first(el) {
    return el ? el.querySelector(controls) : null;
  }

  // addButton: 그 행이 속한 표의 「행 추가」 버튼. 자산 표는 노드마다 따로라 가장 가까운 묶음부터 본다.
  function addButton(from) {
    var box = from.closest(".assets") || from.closest("fieldset");
    return box ? box.querySelector(":scope > .addrow button") : null;
  }

  function apply() {
    var p = pending;
    pending = null;
    if (!p) {
      return;
    }
    var to = null;
    if (p.kind === "new") {
      var added = p.last ? p.last.nextElementSibling : p.target.firstElementChild;
      to = first(added);
    } else {
      to = p.next && p.next.isConnected ? p.next : null;
    }
    if (!to || !to.isConnected) {
      to = p.add && p.add.isConnected ? p.add : null;
    }
    if (!to && p.box) {
      to = p.box.querySelector(":scope > .addrow button");
    }
    if (to) {
      to.focus();
    }
  }

  document.addEventListener("htmx:beforeSwap", function (evt) {
    var d = evt.detail;
    var trigger = d.requestConfig && d.requestConfig.elt;
    var kind = trigger && trigger.getAttribute("data-focus");
    if (kind === "new") {
      pending = { kind: kind, target: d.target, last: d.target.lastElementChild,
        box: d.target.closest(".assets") || d.target.closest("fieldset") };
    } else if (kind === "next") {
      var row = d.target;
      pending = { kind: kind, next: first(row.nextElementSibling), add: addButton(row),
        box: row.closest(".assets") || row.closest("fieldset") };
    } else {
      return;
    }
    setTimeout(apply, 150);
  });

  document.addEventListener("htmx:afterSettle", apply);
})();

// 선언 양식: 적다 만 줄과 오류 목록.
//
// 줄에 무엇이든 적었으면 그 줄의 이름과 IP(자산은 컴포넌트, 엣지는 보내는 쪽 · 받는 쪽)가 있어야 한다. 자산 줄의 런타임을
// 기본값에서 바꾼 것도 적은 것으로 본다. 서버가 같은 것을 보고 저장을 거절하므로
// (pkg/inventory/ui/declform.go), 여기서는 보내기 전에 브라우저가 그 칸을 짚게 required만 건다. 기존 줄의 required는
// 서버가 그릴 때 이미 달려 있고 여기서 떼지 않는다. 아무것도 적지 않은 새 줄은 걸지 않는다(서버도 그 줄을 버린다).
//
// 오류 목록의 링크는 그 칸으로 포커스를 옮긴다. 스크립트가 막혀도 링크는 그 칸으로 간다.
(function () {
  "use strict";

  function need(el) {
    var scope = el.closest(el.getAttribute("data-need-scope"));
    if (!scope) {
      return;
    }
    if (el.getAttribute("data-need-kept") === null) {
      el.setAttribute("data-need-kept", el.required ? "1" : "0");
    }
    var filled = el.getAttribute("data-need-kept") === "1";
    var others = scope.querySelectorAll("input[type=text]");
    for (var i = 0; i < others.length; i++) {
      if (others[i] !== el && others[i].value.trim() !== "") {
        filled = true;
      }
    }
    var picked = scope.querySelectorAll("select");
    for (var j = 0; j < picked.length; j++) {
      if (picked[j].selectedIndex > 0) {
        filled = true;
      }
    }
    el.required = filled;
  }

  function needAll() {
    var all = document.querySelectorAll("[data-need-scope]");
    for (var i = 0; i < all.length; i++) {
      need(all[i]);
    }
  }

  document.addEventListener("input", needAll);
  document.addEventListener("change", needAll);
  document.addEventListener("htmx:afterSettle", needAll);
  needAll();

  document.addEventListener("click", function (evt) {
    var a = evt.target.closest ? evt.target.closest("a[data-focus-field]") : null;
    var to = a ? document.getElementById(a.getAttribute("href").slice(1)) : null;
    if (to) {
      evt.preventDefault();
      to.focus();
      to.scrollIntoView({ block: "center" });
    }
  });
})();
