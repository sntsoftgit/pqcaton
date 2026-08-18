// Command pqcaton-reconcile — 인벤토리 종단 데모(Phase 1).
// 관측(호스트 Discovery) vs 선언(CSV)을 대조해 3-상태 + 리뷰 큐를 낸다.
// usage: pqcaton-reconcile <declaration.csv> [node] [org]
//
// org 를 주지 않으면 `local` 이다 — 한 조직만 다루는 자리라도 대조는 조직에 묶인다.
package main

import (
	"fmt"
	"os"

	"github.com/randyinthedev-hash/pqcota/discovery/collectors/openssl"
	discoveryv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota/pkg/discovery/normalize"
	"github.com/randyinthedev-hash/pqcota/pkg/inventory/declaration"
	"github.com/randyinthedev-hash/pqcota/pkg/kernel/registry"
	"github.com/randyinthedev-hash/pqcota/pkg/org"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pqcaton-reconcile <declaration.csv> [node] [org]")
		os.Exit(2)
	}
	declPath := os.Args[1]
	node := "host://local"
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

	// 관측(observed): 호스트 스캔 → CollectionResult → Normalize → 자산.
	dets, st := openssl.ScanHost(registry.DefaultForkSignatures)
	res := openssl.BuildResult(node, dets)
	snap, err2 := normalize.Normalize([]*discoveryv1.CollectionResult{res}, "snap-1", node, "ruleset-1", nil, nil)
	if err2 != nil {
		fmt.Fprintln(os.Stderr, "normalize:", err2)
		os.Exit(1)
	}
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
		orgName, node, len(observed), len(declared), st.Accessible, st.Denied)
	fmt.Print(reconcile.RenderView(recs))
}
