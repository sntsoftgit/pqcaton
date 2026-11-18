// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package upstream_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/pgschema"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// IC-S27 — **시작 점검은 스키마가 없는 것과 읽을 권한이 없는 것을 가른다.** 상류가 「없다」고 하면(표 · 열) `-init-schema`를
// 권하고, 「권한이 없다」고 하면(SELECT · USAGE, 없는 표가 함께 있어도) 초기화를 권하지 않는다. 열고 난 뒤 조회에서 받은
// SQLSTATE 42501도 권한 없음이다. 어느 분류에도 상류 문구의 표 · 스키마 이름을 싣지 않는다.
func TestClassifyTellsMissingSchemaFromMissingPrivilege(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"missing table", &pgschema.Problem{Missing: []string{"pqcota_snapshots"}, Hint: history.InitHint}, upstream.ClassSchemaMissing},
		{"missing column", fmt.Errorf("open: %w", &pgschema.Problem{Missing: []string{"pqcota_snapshots.ruleset_version"}}), upstream.ClassSchemaMissing},
		{"no SELECT", &pgschema.Problem{Denied: []string{"SELECT on pqcota_snapshots"}}, upstream.ClassNoPrivilege},
		{"no USAGE and a missing table", &pgschema.Problem{Denied: []string{"USAGE on schema secretschema"}, Missing: []string{"pqcota_x"}}, upstream.ClassNoPrivilege},
		{"42501 after opening", &pgconn.PgError{Code: "42501", Message: "permission denied for table pqcota_snapshots"}, upstream.ClassNoPrivilege + " (SQLSTATE 42501)"},
		{"other server error", &pgconn.PgError{Code: "57P01"}, "server error SQLSTATE 57P01"},
	} {
		got := upstream.Classify(c.err)
		if got != c.want {
			t.Errorf("%s: Classify = %q, want %q", c.name, got, c.want)
		}
		for _, leak := range []string{"pqcota_snapshots", "pqcota_x", "secretschema", "ruleset_version"} {
			if strings.Contains(got, leak) {
				t.Errorf("%s: the classification carries %q from the upstream message: %q", c.name, leak, got)
			}
		}
	}
	if strings.Contains(upstream.ClassNoPrivilege, "-init-schema") {
		t.Errorf("the privilege classification must not suggest initializing the schema: %q", upstream.ClassNoPrivilege)
	}
}

// IC-S28 — **실제 Postgres에서 화면 계정의 시작 점검(pqcota v0.16.0).** 빈 스키마는 「스키마 없음」, 표가 있어도 SELECT가 없으면
// 「권한 없음」, 스키마 USAGE가 없어도 「권한 없음」이고, 셋 다 ErrUnreachable이며 오류에 연결 정보(사용자 · 비밀번호 · 호스트)가
// 없다. 이력 표 전부에 SELECT와 스키마 USAGE만 준 계정은 연결과 Probe를 통과한다(조회 계정에 DDL 권한이 필요 없다).
// 시험마다 새 스키마와 역할을 만들고 끝나면 지운다. PQCOTA_TEST_DSN(역할을 만들 수 있는 계정의 URL 형식)이 있을 때만 돈다.
func TestStartupCheckOnPostgresRoles(t *testing.T) {
	dsn := os.Getenv("PQCOTA_TEST_DSN")
	if dsn == "" {
		t.Skip("PQCOTA_TEST_DSN is not set: the Postgres startup check test is skipped")
	}
	base, err := url.Parse(dsn)
	if err != nil || base.Scheme == "" || base.User == nil {
		t.Fatal("PQCOTA_TEST_DSN must be a postgres:// URL with a user for this test")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("cannot connect with PQCOTA_TEST_DSN")
	}
	defer admin.Close(ctx)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	schema, role, pass := "pqcaton_s28_"+suffix, "pqcaton_s28_ui_"+suffix, "s28-"+suffix+"-pw"
	exec := func(sql string) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec("CREATE SCHEMA " + schema)
	exec("CREATE ROLE " + role + " LOGIN PASSWORD '" + pass + "'")
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		_, _ = admin.Exec(ctx, "DROP OWNED BY "+role)
		_, _ = admin.Exec(ctx, "DROP ROLE IF EXISTS "+role)
	})

	// with — 같은 서버 · 데이터베이스에 다른 사용자와 search_path로 붙는 DSN.
	with := func(u *url.Userinfo) string {
		d := *base
		d.User = u
		q := d.Query()
		q.Set("search_path", schema)
		d.RawQuery = q.Encode()
		return d.String()
	}
	adminDSN, uiDSN := with(base.User), with(url.UserPassword(role, pass))
	org := "pqcaton-s28-" + suffix

	open := func(when, want string) {
		t.Helper()
		st, _, err := upstream.OpenPg(uiDSN, org)
		if err == nil {
			st.Close()
			t.Fatalf("%s: OpenPg passed, want %q", when, want)
		}
		if !errors.Is(err, upstream.ErrUnreachable) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: OpenPg = %v, want ErrUnreachable with %q", when, err, want)
		}
		for _, leak := range []string{role, pass, base.Host} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: the error carries connection details: %v", when, err)
			}
		}
	}

	exec("GRANT USAGE ON SCHEMA " + schema + " TO " + role)
	open("empty schema", upstream.ClassSchemaMissing)

	// 스키마를 만든다(관리 계정, 자동 생성). 화면 계정에는 아직 SELECT가 없다.
	st, err := history.NewPgStoreIn(ctx, adminDSN, org)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	open("tables without SELECT", upstream.ClassNoPrivilege)

	exec("GRANT SELECT ON ALL TABLES IN SCHEMA " + schema + " TO " + role)
	exec("REVOKE USAGE ON SCHEMA " + schema + " FROM " + role)
	open("SELECT without USAGE on the schema", upstream.ClassNoPrivilege)

	exec("GRANT USAGE ON SCHEMA " + schema + " TO " + role)
	ro, _, err := upstream.OpenPg(uiDSN, org)
	if err != nil {
		t.Fatalf("an account with SELECT and USAGE only: OpenPg = %v, want it to open", err)
	}
	defer ro.Close()
	if err := upstream.Probe(ro); err != nil {
		t.Errorf("an account with SELECT and USAGE only: Probe = %v, want it to pass", err)
	}
}
