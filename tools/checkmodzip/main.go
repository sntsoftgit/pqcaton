// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

// Command checkmodzip — 이 리포를 Go 모듈로 받을 수 있는지 잰다.
//
// 왜 관문인가: v0.26.2는 시험 픽스처를 `testdata/nul` 에 두었다. NUL 은 Windows 예약어라 go 명령이
// 그 판의 모듈 zip 을 만들지 않았고, 프록시로도 직접으로도 `go install ...@v0.26.2` 가 실패했다.
// 빌드 · 시험 · 종단 검증은 모두 체크아웃에서 돌아 아무도 못 잡았고, 발행한 뒤 프록시에서 처음 드러났다.
//
// 그래서 go 명령이 모듈 zip 을 만들 때 쓰는 검사(golang.org/x/mod/zip)를 발행 전에 그대로 돌린다.
// 예약어 말고도 대소문자만 다른 경로, 크기 한도, 잘못된 파일 이름을 함께 본다.
//
// 대상은 `git ls-files` 가 내놓는 파일(추적하는 것과 무시되지 않은 새 파일)이다. 작업 트리를 통째로
// 보면 빌드 산출물처럼 실리지 않는 파일까지 잡아서 틀린다. 하위 모듈(이 도구 자신)과 vendor 는
// go 명령처럼 빼고 본다.
//
// 이 도구만 별도 모듈이다. x/mod 를 위해 본 모듈의 go.mod 에 의존성을 늘리지 않으려는 것이다.
//
// usage: go run . <repo-root>
package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/zip"
)

// file 은 zip.File 을 작업 트리의 파일로 채운다.
type file struct {
	root, rel string
}

func (f file) Path() string { return f.rel }
func (f file) Lstat() (fs.FileInfo, error) {
	return os.Lstat(filepath.Join(f.root, filepath.FromSlash(f.rel)))
}
func (f file) Open() (io.ReadCloser, error) {
	return os.Open(filepath.Join(f.root, filepath.FromSlash(f.rel)))
}

// listFiles 는 모듈 zip 에 들어갈 후보를 돌려준다. 추적은 되는데 작업 트리에서 지운 파일은 뺀다.
func listFiles(root string) ([]zip.File, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []zip.File
	for _, rel := range bytes.Split(out, []byte{0}) {
		if len(rel) == 0 {
			continue
		}
		f := file{root: root, rel: string(rel)}
		if _, err := f.Lstat(); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// check 는 문제를 한 줄씩 돌려준다. 비어 있으면 모듈 zip 을 만들 수 있다.
func check(files []zip.File) []string {
	cf, err := zip.CheckFiles(files)
	var problems []string
	for _, fe := range cf.Invalid {
		problems = append(problems, fmt.Sprintf("%s: %v", fe.Path, fe.Err))
	}
	if cf.SizeError != nil {
		problems = append(problems, cf.SizeError.Error())
	}
	if len(problems) == 0 && err != nil {
		problems = append(problems, err.Error())
	}
	return problems
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: checkmodzip <repo-root>")
		os.Exit(2)
	}
	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	files, err := listFiles(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if problems := check(files); len(problems) > 0 {
		fmt.Println("✗ the go command cannot create this module's zip; go install would fail for this version:")
		fmt.Println("  " + strings.Join(problems, "\n  "))
		os.Exit(1)
	}
	fmt.Printf("✓ module zip check passed (%d files)\n", len(files))
}
