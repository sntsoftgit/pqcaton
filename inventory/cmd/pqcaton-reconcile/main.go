// Command pqcaton-reconcile — 인벤토리 종단 데모(Phase 1).
// 관측(호스트 Discovery) vs 선언(CSV)을 대조해 3-상태 + 리뷰 큐를 낸다.
// usage: pqcaton-reconcile <declaration.csv> [이-기계에-붙일-이름] [org]
//
// **이 기계를 스캔한다.** 두 번째 인자는 결과에 붙이는 이름표이지 관측 대상이 아니다 —
// 다른 노드를 관측하려면 pqcota 의 collector 를 그 노드에서 돌리고 `pqcaton-report` 로
// 모은다. `/proc` 이 없으면(비-리눅스) 끊는다.
//
// org 를 주지 않으면 `local` 이다 — 한 조직만 다루는 자리라도 대조는 조직에 묶인다.
package main

import (
	"fmt"
	"os"

	"github.com/randyinthedev-hash/pqcota/pkg/inventory/declaration"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/localscan"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pqcaton-reconcile <declaration.csv> [이-기계에-붙일-이름] [org]")
		os.Exit(2)
	}
	declPath := os.Args[1]
	node := localscan.DefaultNode
	if len(os.Args) > 2 {
		node = os.Args[2]
	}
	orgName := "local"
	if len(os.Args) > 3 {
		orgName = os.Args[3]
	}
	eng, err := reconcile.For(org.ID(orgName))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// 관측(observed): **이 기계를** 스캔한다. 노드 이름은 결과에 붙이는 이름표일 뿐이고,
	// /proc 을 못 열면 끊는다 - 그 상태로 대조하면 「못 본 것」이 「없는 것」으로 읽힌다.
	scan, err := localscan.Scan(node)
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
	for _, w := range scan.Warnings {
		fmt.Fprintln(os.Stderr, "⚠", w)
	}
	snap := scan.Snapshot
	observed := eng.AssetsFromSnapshot(snap)
	gapLayers := reconcile.GapLayers(snap)

	// 선언(declared): CSV 임포트 → 자산.
	f, err := os.Open(declPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open declaration:", err)
		os.Exit(1)
	}
	defer f.Close()
	declResults, err := declaration.ImportCSV(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "import csv:", err)
		os.Exit(1)
	}
	declared, err := eng.AssetsFromResults(declResults)
	if err != nil {
		fmt.Fprintln(os.Stderr, "declared assets:", err)
		os.Exit(1)
	}

	recs, err := eng.Reconcile(declared, observed, gapLayers)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("== Inventory Reconciliation (Phase 1) ==\n")
	fmt.Printf("조직 %s · 노드 %s · 관측 자산 %d · 선언 자산 %d (스캔: 접근가능 %d · 거부 %d)\n\n",
		orgName, node, len(observed), len(declared), scan.Accessible, scan.Denied)
	fmt.Print(reconcile.RenderView(recs))
}
