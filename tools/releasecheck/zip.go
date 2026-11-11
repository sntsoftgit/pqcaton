// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	"golang.org/x/mod/zip"
)

// makeZip 은 repo 의 rev 를 모듈 zip 으로 out 에 쓰고 h1 해시를 돌려준다. 실패하면 out 을 남기지 않는다.
func makeZip(repo, rev, mod, version, out string) (string, error) {
	if err := module.Check(mod, version); err != nil {
		return "", err
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	err = zip.CreateFromVCS(f, module.Version{Path: mod, Version: version}, repo, rev, "")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(out)
		return "", fmt.Errorf("the go command cannot create this module's zip: %w", err)
	}
	return dirhash.HashZip(out, dirhash.Hash1)
}

// modHash 는 go.mod 내용의 h1 해시다. 체크섬 DB 의 `<ver>/go.mod` 줄과 같은 방식으로 센다.
func modHash(data []byte) (string, error) {
	return dirhash.Hash1([]string{"go.mod"}, func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})
}

func cmdZip(args []string) error {
	fs := flag.NewFlagSet("zip", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository root")
	rev := fs.String("rev", "HEAD", "revision to archive")
	version := fs.String("version", "", "module version")
	out := fs.String("out", "", "zip file to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" || *out == "" {
		return errors.New("zip needs -version and -out")
	}
	mod, err := modulePath(*repo)
	if err != nil {
		return err
	}
	h1, err := makeZip(*repo, *rev, mod, *version, *out)
	if err != nil {
		return err
	}
	fmt.Println(h1)
	return nil
}

func cmdHashZip(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: hashzip <file.zip>")
	}
	h1, err := dirhash.HashZip(args[0], dirhash.Hash1)
	if err != nil {
		return err
	}
	fmt.Println(h1)
	return nil
}

func cmdHashMod(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: hashmod <go.mod>")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	h1, err := modHash(data)
	if err != nil {
		return err
	}
	fmt.Println(h1)
	return nil
}

func cmdEscape(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: escape <module path>")
	}
	p, err := module.EscapePath(args[0])
	if err != nil {
		return err
	}
	fmt.Println(p)
	return nil
}

// proxyInfo 는 프록시 .info 의 필요한 칸이다.
type proxyInfo struct {
	Version string
	Origin  *struct {
		VCS, URL, Hash, Ref string
	}
}

// checkInfo 는 .info 가 그 판 · 그 커밋을 가리키는지 본다.
func checkInfo(data []byte, version, commit string) error {
	var in proxyInfo
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("cannot read .info: %w", err)
	}
	if in.Version != version {
		return fmt.Errorf(".info Version is %q, want %q", in.Version, version)
	}
	if in.Origin == nil || in.Origin.Hash == "" {
		return errors.New(".info has no Origin.Hash; cannot tell which commit the proxy used")
	}
	if in.Origin.Hash != commit {
		return fmt.Errorf(".info Origin.Hash is %s, want %s", in.Origin.Hash, commit)
	}
	return nil
}

func cmdInfo(args []string) error {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	version := fs.String("version", "", "module version")
	commit := fs.String("commit", "", "full commit hash")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *version == "" || *commit == "" {
		return errors.New("usage: info -version V -commit C <file.info>")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := checkInfo(data, *version, *commit); err != nil {
		return err
	}
	fmt.Printf("✓ .info points at %s %s\n", *version, *commit)
	return nil
}
