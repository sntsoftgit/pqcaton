// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type revWorld struct {
	session, layer, results string
}

func newRevWorld(t *testing.T) revWorld {
	t.Helper()
	dir := t.TempDir()
	w := revWorld{session: filepath.Join(dir, "session.json"), layer: filepath.Join(dir, "org.csv"),
		results: filepath.Join(dir, "results")}
	if err := os.MkdirAll(filepath.Join(w.results, "node-b"), 0o700); err != nil {
		t.Fatal(err)
	}
	for p, c := range map[string]string{
		w.session: "{\"items\":[]}\n", w.layer: "rule,keep\n",
		filepath.Join(w.results, "node-a.json"):           "{\"a\":1}\n",
		filepath.Join(w.results, "node-b", "result.json"): "{\"b\":1}\n",
	} {
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func (w revWorld) inputs() []Input {
	return []Input{{Name: "session", Path: w.session}, {Name: "layer:0", Path: w.layer}, {Name: "results", Path: w.results, Dir: true}}
}

func mustRev(t *testing.T, in []Input) string {
	t.Helper()
	r, err := Rev(in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// IC-W27: 입력이 같으면 rev가 같고, 세션 · 레이어 · 관측 파일 한 바이트, 관측 파일의 추가 · 이름 바꿈,
// 입력의 자리, 「없음」과 「빈 파일」 가운데 하나라도 다르면 rev가 다르다.
func TestRevCoversEveryInput(t *testing.T) {
	w := newRevWorld(t)
	base := mustRev(t, w.inputs())
	if again := mustRev(t, w.inputs()); again != base {
		t.Fatalf("rev changed with nothing changed: %s → %s", base, again)
	}
	changes := map[string]func(t *testing.T, w revWorld) []Input{
		"session byte": func(t *testing.T, w revWorld) []Input {
			os.WriteFile(w.session, []byte("{\"items\":[1]}\n"), 0o600)
			return w.inputs()
		},
		"layer byte": func(t *testing.T, w revWorld) []Input {
			os.WriteFile(w.layer, []byte("rule,drop\n"), 0o600)
			return w.inputs()
		},
		"nested result byte": func(t *testing.T, w revWorld) []Input {
			os.WriteFile(filepath.Join(w.results, "node-b", "result.json"), []byte("{\"b\":2}\n"), 0o600)
			return w.inputs()
		},
		"result added": func(t *testing.T, w revWorld) []Input {
			os.WriteFile(filepath.Join(w.results, "node-c.json"), []byte("{}"), 0o600)
			return w.inputs()
		},
		"result renamed": func(t *testing.T, w revWorld) []Input {
			os.Rename(filepath.Join(w.results, "node-a.json"), filepath.Join(w.results, "node-z.json"))
			return w.inputs()
		},
		"inputs swapped": func(t *testing.T, w revWorld) []Input {
			in := w.inputs()
			in[0].Name, in[1].Name = in[1].Name, in[0].Name
			return in
		},
		"absent versus empty": func(t *testing.T, w revWorld) []Input {
			os.WriteFile(w.layer, nil, 0o600)
			empty := mustRev(t, w.inputs())
			os.Remove(w.layer)
			if mustRev(t, w.inputs()) == empty {
				t.Error("an absent layer and an empty layer gave the same rev")
			}
			return w.inputs()
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			w := newRevWorld(t)
			before := mustRev(t, w.inputs())
			if before != base {
				t.Fatalf("the same starting files gave another rev")
			}
			if after := mustRev(t, change(t, w)); after == before {
				t.Errorf("rev did not change")
			}
		})
	}
}

// IC-W28: 읽는 동안 입력이 바뀌면 다시 읽어 바뀐 뒤의 rev를 낸다. 계속 바뀌면 ErrChanged로 끝난다.
// 반쯤 바뀐 상태의 rev를 내지 않는다.
func TestRevRetriesWhileFilesChange(t *testing.T) {
	w := newRevWorld(t)
	n := 0
	got, err := rev(w.inputs(), func() {
		if n == 0 {
			os.WriteFile(w.session, []byte("{\"items\":[\"moved\"]}\n"), 0o600)
		}
		n++
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := mustRev(t, w.inputs()); got != want {
		t.Errorf("rev after one change while reading = %s, want the rev of the new content %s", got, want)
	}
	if n != 2 {
		t.Errorf("read %d times, want 2", n)
	}

	i := 0
	_, err = rev(w.inputs(), func() {
		i++
		// 크기를 매번 바꾼다. 수정 시각의 단위가 굵은 파일시스템에서도 바뀐 것이 보이게 한다.
		os.WriteFile(filepath.Join(w.results, "node-a.json"), []byte(strings.Repeat("x", 10+i)), 0o600)
	})
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("err = %v, want ErrChanged", err)
	}
	if i != revAttempts {
		t.Errorf("tried %d times, want %d", i, revAttempts)
	}
}

// IC-W29: rev는 캐시하지 않는다. 크기와 수정 시각을 그대로 두고 내용만 바꿔도 rev가 바뀐다.
func TestRevReadsContent(t *testing.T) {
	w := newRevWorld(t)
	before := mustRev(t, w.inputs())
	fi, err := os.Stat(w.layer)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.layer, []byte("rule,KEEP\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mt := fi.ModTime()
	if err := os.Chtimes(w.layer, mt, mt); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(w.layer)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != fi.Size() || !after.ModTime().Equal(fi.ModTime()) {
		t.Fatal("could not keep size and mtime the same")
	}
	if mustRev(t, w.inputs()) == before {
		t.Error("rev stayed the same though the content changed")
	}
}
