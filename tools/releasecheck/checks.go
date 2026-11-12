// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

// checks: 후보 커밋의 GitHub 워크플로가 모두 성공했는가. release-check가 GitHub API의 실행 목록
// (GET /repos/{owner}/{repo}/actions/runs?head_sha=<commit>)을 받아 넘긴다.
//
// 필수 워크플로는 표시 이름이 아니라 **파일 경로**로 가리고, head_sha가 후보 커밋과 정확히 같은 **push** 실행만 받는다.
// 그 가운데 가장 나중 실행의 가장 높은 run_attempt가 completed/success여야 한다. **없거나, 끝나지 않았거나, 성공이 아니거나,
// API 오류 · 호출 제한 · 잘린 목록이면 실패다.** release-check가 통과해도 GitHub의 ci가 실패한 커밋을 내보낸 일(v0.29.0)이 계기다.

type workflowRun struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	HeadSHA    string `json:"head_sha"`
	Event      string `json:"event"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Attempt    int    `json:"run_attempt"`
	CreatedAt  string `json:"created_at"`
	URL        string `json:"html_url"`
}

// checkRuns 는 필수 워크플로마다 한 줄씩 결과를 돌려준다. 하나라도 통과하지 못하면 오류도 돌려준다.
func checkRuns(raw []byte, commit string, required []string) ([]string, error) {
	var page struct {
		Total *int          `json:"total_count"`
		Runs  []workflowRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("workflow runs JSON: %w", err)
	}
	// 오류 응답(호출 제한 등)은 workflow_runs가 없다. 목록이 잘렸으면(다음 쪽이 있으면) 판단하지 않는다.
	if page.Total == nil {
		return nil, errors.New("the response is not a workflow runs list (an API error or rate limit?)")
	}
	if *page.Total > len(page.Runs) {
		return nil, fmt.Errorf("the runs list is cut (%d of %d); refusing to judge from a partial list", len(page.Runs), *page.Total)
	}
	if len(required) == 0 {
		return nil, errors.New("no required workflow given")
	}
	var lines, bad []string
	for _, path := range required {
		var runs []workflowRun
		for _, r := range page.Runs {
			if r.Path == path && r.HeadSHA == commit && r.Event == "push" {
				runs = append(runs, r)
			}
		}
		if len(runs) == 0 {
			bad = append(bad, path)
			lines = append(lines, fmt.Sprintf("✗ %s: no run for %s", path, commit))
			continue
		}
		sort.Slice(runs, func(i, j int) bool {
			if runs[i].CreatedAt != runs[j].CreatedAt {
				return runs[i].CreatedAt < runs[j].CreatedAt
			}
			return runs[i].Attempt < runs[j].Attempt
		})
		r := runs[len(runs)-1]
		switch {
		case r.Status != "completed":
			bad = append(bad, path)
			lines = append(lines, fmt.Sprintf("✗ %s: %s (not finished) %s", path, r.Status, r.URL))
		case r.Conclusion != "success":
			bad = append(bad, path)
			lines = append(lines, fmt.Sprintf("✗ %s: %s (attempt %d) %s", path, r.Conclusion, r.Attempt, r.URL))
		default:
			lines = append(lines, fmt.Sprintf("✓ %s: success (attempt %d) %s", path, r.Attempt, r.URL))
		}
	}
	if len(bad) > 0 {
		return lines, fmt.Errorf("required workflows did not succeed for %s: %s", commit, strings.Join(bad, ", "))
	}
	return lines, nil
}

func cmdChecks(args []string) error {
	fs := flag.NewFlagSet("checks", flag.ContinueOnError)
	runs := fs.String("runs", "", "GitHub API workflow runs JSON for the commit")
	commit := fs.String("commit", "", "candidate commit (40 hex)")
	require := fs.String("require", "", "comma-separated workflow paths that must succeed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(*commit) != 40 {
		return fmt.Errorf("commit %q is not a full hash", *commit)
	}
	raw, err := os.ReadFile(*runs)
	if err != nil {
		return err
	}
	var req []string
	for _, p := range strings.Split(*require, ",") {
		if p = strings.TrimSpace(p); p != "" {
			req = append(req, p)
		}
	}
	lines, err := checkRuns(raw, *commit, req)
	for _, l := range lines {
		fmt.Println(l)
	}
	return err
}
