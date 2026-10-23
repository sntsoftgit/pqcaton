// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build linux || darwin

package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// guideStep: 안내서에서 시험이 돌리는 것 하나.
type guideStep struct {
	kind string // run · background · fails · until · setup
	code string
	line int
}

var guideMarkerLine = regexp.MustCompile(`^<!-- (run(?:: ([a-z]+))?|setup: (.+)) -->$`)

func guideSteps(t *testing.T, path string) []guideStep {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	var out []guideStep
	for i := 0; i < len(lines); i++ {
		m := guideMarkerLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		if m[3] != "" {
			out = append(out, guideStep{kind: "setup", code: m[3], line: i + 1})
			continue
		}
		kind := "run"
		if m[2] != "" {
			kind = m[2]
		}
		if i+1 >= len(lines) || lines[i+1] != "```bash" {
			t.Fatalf("%s:%d: a run marker not followed by a bash block", path, i+1)
		}
		var code []string
		for i += 2; i < len(lines) && lines[i] != "```"; i++ {
			code = append(code, lines[i])
		}
		out = append(out, guideStep{kind: kind, code: strings.Join(code, "\n"), line: i})
	}
	return out
}

// IC-U108 — **운영자 안내서의 명령은 그대로 돌아간다.** 실행 표시가 붙은 명령을 빈 홈 폴더에서 차례대로 실제로 돌린다.
// 띄우기 · 떠 있는 화면 다시 열기 · 끝내기 · 루프백이 아닌 주소 거부 · 끈 상태의 백업과 복원 · 저장 도중 멈추고 밖에서
// 바뀐 파일의 복구(보존 → 준비 파일을 대상 자리에 → 다시 띄움)가 안내서에 적힌 명령으로 끝난다. 두 판 모두 돈다.
// 저장 도중 멈춘 상태는 제품 바이너리가 아니라 시험 도구(testdata/guidetool)가 저장소의 실패 주입으로 만든다.
func TestOperatorGuideCommandsRun(t *testing.T) {
	for _, tool := range []string{"bash", "pgrep", "pkill", "curl", "cp", "diff", "date"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("the guide needs %s: %v", tool, err)
		}
	}
	user := os.Getenv("USER")
	if out, err := exec.Command("pgrep", "-u", user, "-x", "pqcaton-ui").Output(); err == nil {
		t.Fatalf("a pqcaton-ui of yours is already running (%s); the guide stops it by name, so stop it first", strings.TrimSpace(string(out)))
	}
	bin := t.TempDir()
	for _, b := range [][]string{
		{"build", "-o", filepath.Join(bin, "pqcaton-ui"), "."},
		{"build", "-o", filepath.Join(bin, "guidetool"), "./testdata/guidetool"},
	} {
		if out, err := exec.Command("go", b...).CombinedOutput(); err != nil {
			t.Fatalf("go %v: %v\n%s", b, err, out)
		}
	}
	for _, guide := range []string{"operator-guide.md", "operator-guide.en.md"} {
		t.Run(guide, func(t *testing.T) {
			runGuide(t, filepath.Join("..", "..", "..", "docs", guide), bin, user)
		})
	}
}

func runGuide(t *testing.T, path, bin, user string) {
	home := t.TempDir()
	env := map[string]string{"HOME": home, "USER": user, "PATH": bin + ":" + os.Getenv("PATH")}
	environ := func() []string {
		out := []string{"LANG=C"}
		for k, v := range env {
			out = append(out, k+"="+v)
		}
		return out
	}
	// 띄운 화면은 곧바로 거둔다(Wait). 거두지 않으면 끝난 프로세스가 좀비로 남아 pgrep에 잡힌다.
	type bg struct {
		c    *exec.Cmd
		done chan struct{}
	}
	var background []bg
	t.Cleanup(func() {
		for _, b := range background {
			_ = syscall.Kill(-b.c.Process.Pid, syscall.SIGKILL)
			<-b.done
		}
	})
	shell := func(ctx context.Context, code string) *exec.Cmd {
		c := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail", "-c", code)
		c.Dir, c.Env = home, environ()
		return c
	}
	addr := filepath.Join(home, "pqcaton", ".pqcaton", "screen.addr")
	steps := guideSteps(t, path)
	if len(steps) == 0 {
		t.Fatal("no runnable commands in the guide")
	}
	for _, st := range steps {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		where := filepath.Base(path) + ":" + strings.TrimSpace(strings.SplitN(st.code, "\n", 2)[0])
		switch st.kind {
		case "run", "setup":
			c := shell(ctx, st.code)
			var out, errb bytes.Buffer
			c.Stdout, c.Stderr = &out, &errb
			if err := c.Run(); err != nil {
				cancel()
				t.Fatalf("%s (%s): %v\n%s%s", where, st.kind, err, out.String(), errb.String())
			}
			if st.kind == "setup" {
				sc := bufio.NewScanner(&out)
				for sc.Scan() {
					if k, v, ok := strings.Cut(sc.Text(), "="); ok && k == strings.ToUpper(k) {
						env[k] = v
					}
				}
			}
		case "fails":
			c := shell(ctx, st.code)
			if out, err := c.CombinedOutput(); err == nil || ctx.Err() != nil {
				cancel()
				t.Fatalf("%s: want it to fail at once, got %v (timeout %v)\n%s", where, err, ctx.Err(), out)
			}
		case "until":
			deadline := time.Now().Add(10 * time.Second)
			for {
				out, err := shell(ctx, st.code).CombinedOutput()
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					cancel()
					t.Fatalf("%s: still failing after 10s: %v\n%s", where, err, out)
				}
				time.Sleep(200 * time.Millisecond)
			}
		case "background":
			_ = os.Remove(addr)
			c := shell(context.Background(), st.code)
			var log bytes.Buffer
			c.Stdout, c.Stderr = &log, &log
			c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := c.Start(); err != nil {
				cancel()
				t.Fatalf("%s: %v", where, err)
			}
			done := make(chan struct{})
			go func() { _ = c.Wait(); close(done) }()
			background = append(background, bg{c, done})
			deadline := time.Now().Add(10 * time.Second)
			for {
				if b, err := os.ReadFile(addr); err == nil && len(b) > 0 {
					break
				}
				if time.Now().After(deadline) {
					cancel()
					t.Fatalf("%s: the screen did not come up\n%s", where, log.String())
				}
				time.Sleep(50 * time.Millisecond)
			}
		default:
			cancel()
			t.Fatalf("%s: unknown marker %q", where, st.kind)
		}
		cancel()
	}
}
