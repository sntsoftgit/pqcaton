// Command pqcota-reconcile — 인벤토리 종단 데모(Phase 1).
// 관측(호스트 Discovery) vs 선언(CSV)을 대조해 3-상태 + 리뷰 큐를 낸다.
// usage: pqcota-reconcile <declaration.csv> [node]
package main

import (
	"fmt"
	"os"

	"github.com/pqcota/pqcota/discovery/collectors/openssl"
	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"
	"github.com/pqcota/pqcota/pkg/discovery/normalize"
	"github.com/pqcota/pqcota/pkg/inventory/declaration"
	"github.com/pqcota/pqcota/pkg/kernel/registry"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/reconcile"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pqcota-reconcile <declaration.csv> [node]")
		os.Exit(2)
	}
	declPath := os.Args[1]
	node := "host://local"
	if len(os.Args) > 2 {
		node = os.Args[2]
	}

	// 관측(observed): 호스트 스캔 → CollectionResult → Normalize → 자산.
	dets, st := openssl.ScanHost(registry.DefaultForkSignatures)
	res := openssl.BuildResult(node, dets)
	snap, err := normalize.Normalize([]*discoveryv1.CollectionResult{res}, "snap-1", node, "ruleset-1", nil, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "normalize:", err)
		os.Exit(1)
	}
	observed := reconcile.AssetsFromSnapshot(snap)
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
	declared, err := reconcile.AssetsFromResults(declResults)
	if err != nil {
		fmt.Fprintln(os.Stderr, "declared assets:", err)
		os.Exit(1)
	}

	recs := reconcile.Reconcile(declared, observed, gapLayers)
	fmt.Printf("== Inventory Reconciliation (Phase 1) ==\n")
	fmt.Printf("노드 %s · 관측 자산 %d · 선언 자산 %d (스캔: 접근가능 %d · 거부 %d)\n\n",
		node, len(observed), len(declared), st.Accessible, st.Denied)
	fmt.Print(reconcile.RenderView(recs))
}
