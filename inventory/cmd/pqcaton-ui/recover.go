// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// -recover-status: 끝나지 않은 트랜잭션이 남긴 것을 보인다. **아무것도 고치지 않는다.**
//
// 화면은 「복구 필요」만 알리고 전체 경로와 해시는 싣지 않는다. 그것을 보는 자리가 여기다. 무엇을
// 할지는 운영자가 정한다. 일반적인 강제 옵션은 두지 않는다 - 밖에서 바뀐 파일을 덮어쓰는 길이 된다.

import (
	"fmt"
	"io"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/store"
)

func short(h string) string {
	switch {
	case h == "":
		return "(absent)"
	case len(h) > 12:
		return h[:12]
	}
	return h
}

// printRecoverStatus: 상태 디렉토리의 남은 기록을 사람이 읽는 글로. diag가 있으면 JSON도 낸다.
func printRecoverStatus(w io.Writer, dir, diag string) error {
	st, err := store.Open(dir, store.Options{})
	if err != nil {
		return err
	}
	rep, err := st.Status()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "state directory: %s (state id %s)\n", rep.StateDir, rep.StateID)
	if len(rep.Records) == 0 {
		fmt.Fprintln(w, "no unfinished transactions")
	}
	for _, r := range rep.Records {
		rec := r.Record
		fmt.Fprintf(w, "\ntransaction %s  %s  %s/%s/%s  started %s\n", rec.TxID, rec.State, rec.Kind, rec.Org,
			rec.SessionID, rec.Created.Format("2006-01-02 15:04:05Z"))
		for _, it := range r.Items {
			prep := "missing"
			if it.PreparedPresent {
				prep = "present, differs"
				if it.PreparedMatches {
					prep = "present, matches"
				}
			}
			fmt.Fprintf(w, "  %-8s %-7s %s\n", it.Verdict, it.Kind, it.AbsPath)
			fmt.Fprintf(w, "           expected %s  before %s  now %s  prepared copy: %s\n",
				short(it.After), short(it.Before), short(it.Current), prep)
			if it.PreparedPresent {
				fmt.Fprintf(w, "           prepared copy at %s\n", it.AbsPrepared)
			}
		}
	}
	if len(rep.Records) > 0 {
		fmt.Fprintln(w, "\ndone: already applied · pending: not applied yet, the next start finishes it · "+
			"partial: the ledger entry is half written, the next start writes the rest · "+
			"mismatch: changed outside pqcaton; nothing will overwrite it")
		fmt.Fprintln(w, "For a mismatch, keep a copy of the file as it is now before doing anything else.")
	}
	if diag != "" {
		if err := st.ExportDiagnostics(diag); err != nil {
			return err
		}
		fmt.Fprintf(w, "\ndiagnostics written to %s (0600)\n", diag)
	}
	return nil
}
