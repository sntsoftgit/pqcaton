// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

// pqcota 이력. 화면은 시작할 때 PQCOTA_DSN으로 연결하고 필수 조회를 통과해야 뜬다. 정책 회차(05의 활성화 · 기준선)는 명령과
// 같은 upstream.Reconcile로 대조한다. 연결 정보는 환경 변수로만 받고 작업 폴더 · 처리 기록 · 로그 · 화면에 남기지 않는다.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decl"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/review"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// openUpstream — pqcota 이력(조회용). 시험이 메모리 이력으로 바꾼다.
var openUpstream = upstream.OpenPg

// dsnFile — PQCOTA_DSN이 환경에 없을 때 읽는 파일. 승인 키와 같은 사용자 설정 폴더(pqcaton/ 아래)에 둔다.
const dsnFile = "pqcota.env"

// errDSNFile — pqcota.env를 쓸 수 없다. 까닭만 싣고 내용은 싣지 않는다.
var errDSNFile = errors.New(dsnFile + " cannot be used")

// loadDSN — pqcota 이력의 연결 정보. **환경 변수 PQCOTA_DSN이 먼저다.** 비었으면 사용자 설정 폴더의 pqcota.env를 읽는다.
// 파일은 링크가 아닌 보통 파일이어야 하고, 이 사용자의 것이어야 하며, 그룹 · 다른 사용자에게 아무 권한이 없어야 하고(0600),
// 하드 링크가 하나뿐이어야 하며, 작업 폴더 밖에 있어야 한다. 내용은 셸로 평가하지 않고 데이터로만 읽는다: 빈 줄과 # 주석
// 말고는 `PQCOTA_DSN=<값>` 한 줄만 받는다(다른 변수 · 따옴표 · 두 번 적기는 거절). 파일이 없으면 ErrNoDSN이다. 돌려주는
// where는 값이 아니라 출처(환경 변수 또는 사용자 설정 폴더 기준의 위치)다.
func (s *server) loadDSN() (dsn, where string, err error) {
	if v := os.Getenv(upstream.DSNEnv); v != "" {
		return v, "the environment (" + upstream.DSNEnv + ")", nil
	}
	dir, err := userConfigDir()
	if err != nil {
		return "", "", upstream.ErrNoDSN
	}
	p := filepath.Join(dir, ui.KeyDefaultFolder, dsnFile)
	where = filepath.Join(ui.KeyDefaultFolder, dsnFile) + " in the user configuration folder"
	fi, err := os.Lstat(p)
	switch {
	case err != nil && errors.Is(err, fs.ErrNotExist):
		return "", "", fmt.Errorf("%w (and there is no %s)", upstream.ErrNoDSN, where)
	case err != nil:
		return "", "", fmt.Errorf("%w: %s cannot be read", errDSNFile, where)
	case fi.Mode()&fs.ModeSymlink != 0:
		return "", "", fmt.Errorf("%w: %s is a symbolic link", errDSNFile, where)
	case !fi.Mode().IsRegular():
		return "", "", fmt.Errorf("%w: %s is not a regular file", errDSNFile, where)
	case !ownedByMe(fi):
		return "", "", fmt.Errorf("%w: %s is not owned by this user", errDSNFile, where)
	case fi.Mode().Perm()&0o077 != 0:
		return "", "", fmt.Errorf("%w: %s is open to other users (%04o); chmod 600 it", errDSNFile, where, fi.Mode().Perm())
	case links(fi) > 1:
		return "", "", fmt.Errorf("%w: %s has more than one hard link", errDSNFile, where)
	}
	if home := s.keyHome(); home != "" && inside(home, p) {
		return "", "", fmt.Errorf("%w: %s is inside the workspace", errDSNFile, where)
	}
	if fi.Size() > 64<<10 {
		return "", "", fmt.Errorf("%w: %s is too large", errDSNFile, where)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", "", fmt.Errorf("%w: %s cannot be read", errDSNFile, where)
	}
	v, err := parseDSNFile(b)
	if err != nil {
		return "", "", fmt.Errorf("%w: %s %v", errDSNFile, where, err)
	}
	return v, where, nil
}

// parseDSNFile — pqcota.env의 내용을 데이터로 읽는다. 오류에는 줄 번호만 싣고 값은 싣지 않는다.
func parseDSNFile(b []byte) (string, error) {
	var dsn string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if t := strings.TrimSpace(line); t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		switch {
		case !ok || key != upstream.DSNEnv:
			return "", fmt.Errorf("line %d: only %s=<value> is allowed", n, upstream.DSNEnv)
		case dsn != "":
			return "", fmt.Errorf("line %d: %s is given twice", n, upstream.DSNEnv)
		case val == "" || strings.TrimSpace(val) != val:
			return "", fmt.Errorf("line %d: the value is empty or has surrounding spaces", n)
		case strings.ContainsAny(val[:1], `"'`) || strings.ContainsAny(val[len(val)-1:], `"'`):
			return "", fmt.Errorf("line %d: write the value without quotes", n)
		}
		dsn = val
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("cannot be read: %v", err)
	}
	if dsn == "" {
		return "", fmt.Errorf("has no %s line", upstream.DSNEnv)
	}
	return dsn, nil
}

// childEnv — 화면이 띄우는 자식 프로세스(관측 감독 · Ansible)의 환경. PQCOTA_DSN은 넘기지 않는다.
func childEnv() []string {
	var out []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, upstream.DSNEnv+"=") {
			out = append(out, e)
		}
	}
	return out
}

// connectUpstream — 시작할 때 pqcota 이력을 열고 필수 조회(스키마 · 읽기 권한)를 통과시킨다. 오류에 연결 정보가 없다.
func (s *server) connectUpstream() error {
	org, err := upstream.OrgFor(s.upstreamOrg, s.orgName())
	if err != nil {
		return fmt.Errorf("pqcota history: %w — the screen does not start", err)
	}
	dsn, where, err := s.loadDSN()
	if err != nil {
		return fmt.Errorf("pqcota history: %w — the screen does not start without it", err)
	}
	l, name, err := openUpstream(dsn, org)
	if err != nil {
		return fmt.Errorf("pqcota history (connection from %s): %w — the screen does not start without it", where, err)
	}
	if err := upstream.Probe(l); err != nil {
		l.Close()
		return fmt.Errorf("pqcota history (connection from %s): %w — the screen does not start without it", where, err)
	}
	s.upstream, s.upstreamOrgName = l, name
	return nil
}

// reconcileUpstream — 결과 폴더를 policyPath로 대조한 노드별 지문이 pqcota 이력의 노드별 최신 스냅샷과 모두 같은가(전체 10초).
// 명령과 같은 함수다.
func (s *server) reconcileUpstream(policyPath string) (upstream.Result, error) {
	if s.upstream == nil {
		return upstream.Result{}, fmt.Errorf("%w: not connected", upstream.ErrUnreachable)
	}
	// 조회하기 전에 조직을 맞춘다: 연 이력의 조직 · 판정 조직 · 판정 세션의 조직이 모두 같아야 한다.
	sf, err := review.Load(s.path)
	if err != nil {
		return upstream.Result{}, err
	}
	if err := upstream.SameOrg(s.upstreamOrgName, s.orgName(), sf.Scope); err != nil {
		return upstream.Result{}, err
	}
	d, err := decl.Load(s.decl)
	if err != nil {
		return upstream.Result{}, err
	}
	return upstream.Reconcile(s.upstream, s.upstreamOrgName, d, s.results, policyPath, s.now())
}

// upstreamRefuse — 대조 실패를 거절 종류로. 처리했으면 true다. 표준오류에는 분류된 오류만 적는다(연결 정보가 없다).
func (s *server) upstreamRefuse(w http.ResponseWriter, r *http.Request, back string, err error) bool {
	kind, code := "", http.StatusConflict
	switch {
	case errors.Is(err, upstream.ErrOrg):
		kind = ui.RefusedUpstreamOrg
	case errors.Is(err, upstream.ErrRevert):
		kind = ui.RefusedUpstreamRevert
	case errors.Is(err, upstream.ErrMissing):
		kind = ui.RefusedUpstreamMissing
	case errors.Is(err, upstream.ErrNothingToVerify):
		kind = ui.RefusedUpstreamEmpty
	case errors.Is(err, upstream.ErrUnreachable):
		kind, code = ui.RefusedUpstreamUnreachable, http.StatusServiceUnavailable
	default:
		return false
	}
	fmt.Fprintln(os.Stderr, "scope round:", err)
	s.refuse(w, r, code, kind, back, nil)
	return true
}

// checkedAt — 확인 페이지에 적을 확인 시각(분까지, 서버의 시간대).
func checkedAt(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }

// recordOf — 처리 기록 · 전환 보고서에 실을 대조 기록.
func recordOf(res upstream.Result) *review.UpstreamCheck { return res.Record() }
