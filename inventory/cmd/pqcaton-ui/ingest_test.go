// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/scope"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	commonv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/common/v1"
	discoveryv1 "github.com/randyinthedev-hash/pqcota-common/gen/pqcota/discovery/v1"
	"github.com/randyinthedev-hash/pqcota-common/pkg/kernel/sign"
	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/ui"
	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

// 관측된 OpenSSL 자산 하나(libcrypto, sshd가 쓴다). 정책이 sshd를 빼면 이 자산이 빠져 스냅샷의 지문이 달라진다.
const ingestCBOM = `{"bomFormat":"CycloneDX","specVersion":"1.6","components":[{"type":"cryptographic-asset","name":"libcrypto.so.3","properties":[` +
	`{"name":"pqcota:crypto_runtime","value":"openssl"},{"name":"pqcota:detection_method","value":"runtime-introspection+symbol-analysis"},` +
	`{"name":"pqcota:openssl.lib","value":"libcrypto"},{"name":"pqcota:openssl.fork","value":"OpenSSL"},{"name":"pqcota:openssl.version","value":"3.0.13"},` +
	`{"name":"pqcota:openssl.binding_mode","value":"dynamic"},{"name":"pqcota:openssl.path","value":"/usr/lib/x86_64-linux-gnu/libcrypto.so.3"},` +
	`{"name":"pqcota:app_keys","value":"/usr/sbin/sshd"}]}]}`

const (
	policyExcludeSSHD = "action,runtime,lib,app_key,note\nexclude,*,*,/usr/sbin/sshd*,managed by OS patching\n"
	policyAll         = "action,runtime,lib,app_key,note\n"
	ingestSecret      = "postgres://pqcota_ingest:s3cret-ingest@db.internal:5432/pqcota"
)

// signedResult: 수집기의 서명 키로 서명한 OpenSSL 관측 결과 하나(JSON).
func signedResult(t *testing.T, s *server, node string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.collectorKeys, idOpenSSL+collectorKeyExt))
	if err != nil {
		t.Fatal(err)
	}
	r := &discoveryv1.CollectionResult{
		Envelope:      &commonv1.Envelope{CollectorId: idOpenSSL, CollectorVersion: "v0.20.0", TargetNodeId: node},
		CbomCyclonedx: []byte(ingestCBOM), CyclonedxSpecVersion: "1.6",
	}
	if err := sign.SignAll(privateKeyIn(b), []*discoveryv1.CollectionResult{r}); err != nil {
		t.Fatal(err)
	}
	out, err := protojson.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ingestServer: 서명된 결과 둘(web · db)과 활성 정책(sshd 제외), 조회용 · 적재용 연결 정보를 갖춘 서버. 이력은 메모리 저장소
// 하나이고, 적재 프로세스 대신 같은 프로세스에서 같은 함수(ingestRun)를 부른다.
func ingestServer(t *testing.T) (*server, *history.MemStore) {
	t.Helper()
	s := everyScreen(t)
	s.collectorKeys, s.collectorVerify = collectorKeyDir(t, idOpenSSL, idNetwork, idJVM)
	s.scopeAssets = filepath.Join(filepath.Dir(s.planOut), wsScopeAssets)
	for name, body := range map[string][]byte{s.scopeAssets: []byte(policyExcludeSSHD),
		filepath.Join(s.results, "web-openssl.json"): signedResult(t, s, "web"), filepath.Join(s.results, "db-openssl.json"): signedResult(t, s, "db")} {
		if err := os.WriteFile(name, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mem := history.NewMemStore()
	useUpstream(s, mem)
	t.Setenv(readonlyDSNEnv, "memory://read")
	t.Setenv(upstream.DSNEnv, ingestSecret)
	oldID, oldOpen, oldSpawn := dbIdentity, openIngestStore, ingestSpawn
	dbIdentity = func(string) (string, error) { return "db-1", nil }
	openIngestStore = func(string, string) (history.Store, func(), error) { return mem, func() {}, nil }
	ingestSpawn = func(dir, dsn string) { ingestRun(dir, dsn) }
	t.Cleanup(func() { dbIdentity, openIngestStore, ingestSpawn = oldID, oldOpen, oldSpawn })
	return s, mem
}

func waitIngest(t *testing.T, s *server) {
	t.Helper()
	s.ingest.mu.Lock()
	done := s.ingest.done
	s.ingest.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the ingest did not finish")
	}
}

// ingestForm: 적재 확인 페이지를 열어 그 양식을 돌려준다.
func ingestForm(t *testing.T, s *server) (url.Values, string) {
	t.Helper()
	w := postFormLang(t, s, ui.IngestConfirmPath, "ko", withRev(url.Values{}, s.revFor(ui.ScreenObserve)))
	if w.Code != http.StatusOK {
		t.Fatalf("ingest confirm = %d\n%s", w.Code, w.Body.String())
	}
	return hiddenFields(w.Body.String()), w.Body.String()
}

var ingestNodeAttr = regexp.MustCompile(`data-ingest-node="([^"]+)" data-ingest-state="([^"]+)"`)

// ingestStates: 그 구역(id가 비면 화면 전체에서 첫 표)의 「노드=상태」.
func ingestStates(page, id string) string {
	if id != "" {
		i := strings.Index(page, `id="`+id+`"`)
		if i < 0 {
			return "(no #" + id + ")"
		}
		page = page[i:]
	} else if i := strings.Index(page, `id="ingest-last"`); i >= 0 {
		page = page[:i]
	}
	var out []string
	for _, m := range ingestNodeAttr.FindAllStringSubmatch(page, -1) {
		out = append(out, m[1]+"="+m[2])
	}
	return strings.Join(out, " ")
}

func observePage(t *testing.T, s *server) string {
	t.Helper()
	return get(t, s, ui.ScreenObserve+"?lang=ko").Body.String()
}

func ingestRunCount(s *server) int { return len(s.ingestRuns()) }

func latestDigest(t *testing.T, mem *history.MemStore, node string) string {
	t.Helper()
	snap, err := mem.Latest(node)
	if err != nil {
		t.Fatal(err)
	}
	if snap == nil {
		return ""
	}
	return history.ContentHashV1(snap)
}

// IC-U246 — **01에서 서명된 결과를 활성 정책으로 이력에 적재한다. 운영자가 확인을 거쳐 누를 때만.** 적재 전 화면은 노드마다
// 「기록되지 않음」과 적재 확인으로 가는 버튼을 보인다. 확인 페이지는 조직 · 활성 정책의 지문 · 서명을 확인한 결과 파일 수 ·
// 정책으로 제외되는 자산 수 · 노드별 기대 참조를 보이고 **아무것도 적재하지 않는다.** 적재 뒤 이력의 노드별 최신 참조가 화면이
// 기대한 참조와 같고, 화면은 적재 전의 최신 참조(없음)와 지금의 것을 함께 보인다. 그 뒤에는 「이미 적재됨」이고 버튼이 없으며
// 직접 보낸 요청은 409다. 적재용 연결 정보는 화면 · 확인 페이지 · 설정 화면 · 실행 자리의 어느 파일에도 없다.
func TestResultsAreIngestedOnlyWhenTheOperatorConfirms(t *testing.T) {
	s, mem := ingestServer(t)
	page := observePage(t, s)
	if got := ingestStates(page, ""); got != "db=absent web=absent" || !strings.Contains(page, "data-ingest-go") {
		t.Fatalf("before the ingest: %q\n%s", got, section(page, "observe-ingest"))
	}
	// 요약 줄의 정책 지문은 형식 이름만 남게 잘리지 않는다: 형식 이름 뒤에 해시의 앞부분이 온다.
	if !regexp.MustCompile(`활성 정책 scope-policy-v1:[0-9a-f]{12} `).MatchString(page) {
		t.Errorf("the summary does not show a recognizable policy fingerprint\n%s", section(page, "observe-ingest"))
	}
	form, confirm := ingestForm(t, s)
	for _, want := range []string{"조직: " + s.orgName(), "활성 정책의 지문: ", "서명을 확인한 결과 파일: 2개", "정책으로 제외되는 자산: ", "web: 기대하는 참조", "되돌릴 수 없습니다", ">이력에 적재<"} {
		if !strings.Contains(confirm, want) {
			t.Errorf("the confirmation page lacks %q", want)
		}
	}
	if latestDigest(t, mem, "web") != "" || ingestRunCount(s) != 0 {
		t.Fatal("opening the confirmation ingested something")
	}
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusSeeOther {
		t.Fatalf("ingest = %d\n%s", w.Code, w.Body.String())
	}
	waitIngest(t, s)
	page = observePage(t, s)
	if got := ingestStates(page, "ingest-last"); got != "db=current web=current" || !strings.Contains(page, `data-ingest-outcome="recorded"`) {
		t.Errorf("after the ingest: %q\n%s", got, section(page, "ingest-last"))
	}
	runs := s.ingestRuns()
	m, _ := readIngestManifest(runs[len(runs)-1])
	for _, ref := range m.Refs {
		if got := latestDigest(t, mem, ref.Node); got != ref.Digest {
			t.Errorf("node %s: latest in the history %s, expected %s", ref.Node, got, ref.Digest)
		}
	}
	for _, b := range m.Before {
		if b.Latest != "" || b.Body {
			t.Errorf("the record of the state before: %+v, want nothing", b)
		}
	}
	if !strings.Contains(page, `data-ingest="already"`) || strings.Contains(page, "data-ingest-go") {
		t.Error("after the ingest the screen should say it is already ingested and offer no button")
	}
	if w := postForm(t, s, ui.IngestConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenObserve))); w.Code != http.StatusConflict {
		t.Errorf("a confirmation when everything is current = %d, want 409", w.Code)
	}
	settings := get(t, s, ui.ScreenSettings+"?lang=ko").Body.String()
	if !strings.Contains(settings, "PQCOTA_READONLY_DSN") || !strings.Contains(settings, `id="setting-ingest-dsn"`) {
		t.Error("the settings screen should name the read variable and show the ingest row")
	}
	for name, text := range map[string]string{"01": page, "confirmation": confirm, "settings": settings} {
		if strings.Contains(text, "s3cret-ingest") || strings.Contains(text, "db.internal") {
			t.Errorf("%s shows the ingest connection", name)
		}
	}
	_ = filepath.WalkDir(s.ingestDir(), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(readOrEmpty(p), "s3cret-ingest") {
			t.Errorf("%s holds the ingest connection", p)
		}
		return nil
	})
}

// dropSignature: 결과에서 서명 필드를 뺀다. protojson의 출력은 빌드마다 공백이 달라서 글자로 지우면 JSON이 깨질 수 있다.
// 그래서 해독해서 뺀다.
func dropSignature(b string) string {
	var v any
	if err := json.Unmarshal([]byte(b), &v); err != nil {
		return b
	}
	var walk func(any)
	walk = func(x any) {
		if m, ok := x.(map[string]any); ok {
			delete(m, "signature")
			for _, c := range m {
				walk(c)
			}
		}
	}
	walk(v)
	out, err := json.Marshal(v)
	if err != nil {
		return b
	}
	return string(out)
}

func readOrEmpty(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

// IC-U247 — **결과 하나라도 서명이 없거나 맞지 않으면 묶음 전체를 적재하지 않는다.** 서명 뒤에 바뀐 결과가 하나 섞이면 01은
// 파일 · 수집기 ID · 사유를 보이고 버튼을 내지 않는다. 확인 요청은 409이고 이력에는 아무것도 쓰이지 않는다(맞는 서명의 결과도).
// 서명이 없는 결과도 같다. 해독되지 않는 파일도 같다.
func TestOneBadSignatureRefusesTheWholeSet(t *testing.T) {
	for name, c := range map[string]struct {
		edit func(string) string
		why  string
	}{
		"changed after signing": {func(b string) string { return strings.Replace(b, "v0.20.0", "v9.9.9", 1) }, ui.IngestWhyMismatch},
		"not signed":            {dropSignature, ui.IngestWhyUnsigned},
		"not a result":          {func(string) string { return `{"envelope": 1` }, ui.IngestWhyNotResult},
	} {
		t.Run(name, func(t *testing.T) {
			s, mem := ingestServer(t)
			p := filepath.Join(s.results, "db-openssl.json")
			edited := c.edit(readOrEmpty(p))
			if edited == readOrEmpty(p) {
				t.Fatal("the edit changed nothing")
			}
			if err := os.WriteFile(p, []byte(edited), 0o600); err != nil {
				t.Fatal(err)
			}
			page := observePage(t, s)
			if !strings.Contains(page, `data-ingest="refused"`) || !strings.Contains(page, `data-ingest-why="`+c.why+`"`) || !strings.Contains(page, "<code>db-openssl.json</code>") ||
				strings.Contains(page, "data-ingest-go") {
				t.Errorf("want the file named with reason %s and no button:\n%s", c.why, section(page, "observe-ingest"))
			}
			if c.why != ui.IngestWhyNotResult && !strings.Contains(page, "<code>"+idOpenSSL+"</code>") {
				t.Error("the collector ID is not shown")
			}
			if w := postForm(t, s, ui.IngestConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenObserve))); w.Code != http.StatusConflict {
				t.Errorf("confirm = %d, want 409", w.Code)
			}
			if latestDigest(t, mem, "web") != "" || latestDigest(t, mem, "db") != "" || ingestRunCount(s) != 0 {
				t.Error("something was written although one result did not verify")
			}
		})
	}
}

// IC-U248 — **확인한 묶음과 적재하는 묶음이 같아야 하고, 확인 하나로 적재는 한 번만 시작된다.** 확인 페이지를 연 뒤 활성 정책이
// 바뀌거나 결과 파일이 바뀌거나 검증 키가 바뀌면 그 양식으로 보낸 적재는 409이고 아무것도 쓰이지 않는다. 거절은 「적재를
// 시작하지 않았습니다」와 함께 **무엇이 바뀌었는지**를 짚는다(활성 정책 · 결과 파일 · 검증 키). 이미 쓴 확인은 그사이 조건이
// 달라졌어도 「이미 사용한 적재 확인」으로 거절되고, 돌아가는 주소가 그 확인이 시작한 실행이다. 같은 양식을 동시에 둘 보내면 하나만
// 시작된다. 쓰기 전에 멈춘 적재(조회와 적재의 데이터베이스가 다름) 뒤에 같은 양식을 다시 보내도 409이고 두 번째 실행 자리가
// 생기지 않는다.
func TestTheConfirmedSetIsWhatIsIngestedAndOnlyOnce(t *testing.T) {
	s, mem := ingestServer(t)
	form, _ := ingestForm(t, s)
	if err := os.WriteFile(s.scopeAssets, []byte(policyAll), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := postFormLang(t, s, ui.IngestPath, "ko", form); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedIngestChangedPolicy) ||
		!strings.Contains(w.Body.String(), "적재를 시작하지 않았습니다") || !strings.Contains(w.Body.String(), "확인한 뒤 활성 정책이 바뀌었습니다") {
		t.Errorf("ingest after the active policy changed = %d, want 409 %s\n%s", w.Code, ui.RefusedIngestChangedPolicy, w.Body.String())
	}
	if err := os.WriteFile(s.scopeAssets, []byte(policyExcludeSSHD), 0o600); err != nil {
		t.Fatal(err)
	}
	form, _ = ingestForm(t, s)
	extra := filepath.Join(s.results, "app-openssl.json")
	if err := os.WriteFile(extra, signedResult(t, s, "app"), 0o600); err != nil { // 확인한 뒤에 결과가 하나 더 들어왔다
		t.Fatal(err)
	}
	if w := postFormLang(t, s, ui.IngestPath, "ko", form); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedIngestChangedResults) ||
		!strings.Contains(w.Body.String(), "확인한 뒤 결과 파일이 바뀌었습니다") {
		t.Errorf("ingest after the result files changed = %d, want 409 %s", w.Code, ui.RefusedIngestChangedResults)
	}
	// 결과와 정책이 함께 바뀌면 하나를 짚지 않는다.
	if err := os.WriteFile(s.scopeAssets, []byte(policyAll), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := postFormLang(t, s, ui.IngestPath, "ko", form); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedIngestChanged+"<") {
		t.Errorf("ingest after two inputs changed = %d, want 409 %s", w.Code, ui.RefusedIngestChanged)
	}
	if err := os.WriteFile(s.scopeAssets, []byte(policyExcludeSSHD), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	// 검증 키: 쓰지 않는 수집기 ID의 공개 키 하나를 뺀다(결과의 서명은 그대로 맞는다).
	form, _ = ingestForm(t, s)
	keys := readOrEmpty(s.collectorVerify)
	if err := os.WriteFile(s.collectorVerify, []byte(keys[:strings.LastIndex(keys, ",")]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if w := postFormLang(t, s, ui.IngestPath, "ko", form); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedIngestChangedKeys) ||
		!strings.Contains(w.Body.String(), "확인한 뒤 검증 키") {
		t.Errorf("ingest after the verification keys changed = %d, want 409 %s\n%s", w.Code, ui.RefusedIngestChangedKeys, w.Body.String())
	}
	if err := os.WriteFile(s.collectorVerify, []byte(keys), 0o644); err != nil {
		t.Fatal(err)
	}
	if latestDigest(t, mem, "web") != "" || ingestRunCount(s) != 0 {
		t.Fatal("something was ingested from a stale confirmation")
	}

	// 조회와 적재의 데이터베이스가 다르면 쓰기 전에 멈춘다. 그 확인을 다시 써도 시작되지 않는다.
	dbIdentity = func(dsn string) (string, error) {
		if dsn == ingestSecret {
			return "db-2", nil
		}
		return "db-1", nil
	}
	form, _ = ingestForm(t, s)
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusSeeOther {
		t.Fatalf("ingest = %d", w.Code)
	}
	waitIngest(t, s)
	page := observePage(t, s)
	if !strings.Contains(page, `data-ingest-reason="other-db"`) || !strings.Contains(page, "화면이 조회하는 데이터베이스가 아니어서 쓰기 전에 멈췄습니다") ||
		ingestStates(page, "ingest-last") != "db=absent web=absent" || latestDigest(t, mem, "web") != "" {
		t.Errorf("another database: %s\n%s", ingestStates(page, "ingest-last"), section(page, "ingest-last"))
	}
	// 쓰기 전에 멈춘 적재는 「적재에 실패했습니다」와 「이력 변화가 없습니다」(본체 · 최신 참조 · 관측 기록 수가 그대로라고 확인됨)다.
	if !strings.Contains(page, "적재에 실패했습니다") || !strings.Contains(page, `data-ingest-outcome="unchanged"`) || !strings.Contains(page, "이력 변화가 없습니다") {
		t.Errorf("an ingest stopped before writing should say the history did not change:\n%s", section(page, "ingest-last"))
	}
	runs := s.ingestRuns()
	first, _ := readIngestManifest(runs[len(runs)-1])
	// 이미 쓴 확인: 그사이 입력이 바뀌었어도 까닭은 「이미 사용한 적재 확인」이고, 돌아가는 주소가 그 실행이다.
	if err := os.WriteFile(s.scopeAssets, []byte(policyAll), 0o600); err != nil {
		t.Fatal(err)
	}
	w := postFormLang(t, s, ui.IngestPath, "ko", form)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedIngestReplay) || ingestRunCount(s) != 1 ||
		!strings.Contains(w.Body.String(), "이미 사용한 적재 확인입니다. 새 적재를 시작하지 않았습니다") ||
		!strings.Contains(w.Body.String(), ui.IngestRunParam+"="+first.ID+"#ingest-last") {
		t.Errorf("the same confirmation again = %d (runs %d), want 409 %s, one run and a link to run %s\n%s", w.Code, ingestRunCount(s), ui.RefusedIngestReplay, first.ID, w.Body.String())
	}
	if err := os.WriteFile(s.scopeAssets, []byte(policyExcludeSSHD), 0o600); err != nil {
		t.Fatal(err)
	}

	// 같은 양식을 동시에 둘: 하나만 시작된다.
	dbIdentity = func(string) (string, error) { return "db-1", nil }
	form, _ = ingestForm(t, s)
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = postForm(t, s, ui.IngestPath, form).Code
		}(i)
	}
	wg.Wait()
	waitIngest(t, s)
	if !(codes[0] == http.StatusSeeOther && codes[1] == http.StatusConflict) && !(codes[1] == http.StatusSeeOther && codes[0] == http.StatusConflict) {
		t.Errorf("two at once = %v, want one 303 and one 409", codes)
	}
	if ingestRunCount(s) != 2 || latestDigest(t, mem, "web") == "" {
		t.Errorf("runs = %d, want 2 (the refused one and this one)", ingestRunCount(s))
	}
}

// IC-U249 — **「이미 적재됨」은 노드의 최신 참조로만 말한다(A → B → A).** 정책 A로 적재한 뒤 활성 정책을 B로 바꾸면 기대하는
// 참조가 달라져 다시 적재할 수 있다. B로 적재한 뒤 A로 되돌리면, A의 스냅샷 본체는 이력에 있지만 최신 참조가 아니다: 화면은
// 「본체는 있으나 최신 참조가 아님」과 버튼을 보이고, 적재하면 최신 참조가 다시 A의 것이 된다. 일부 노드만 최신이면 어느 노드가
// 그런지 보인다.
func TestAlreadyIngestedMeansTheLatestReference(t *testing.T) {
	s, mem := ingestServer(t)
	run := func() {
		t.Helper()
		form, _ := ingestForm(t, s)
		if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusSeeOther {
			t.Fatalf("ingest = %d\n%s", w.Code, w.Body.String())
		}
		waitIngest(t, s)
	}
	run()
	a := latestDigest(t, mem, "web")
	if err := os.WriteFile(s.scopeAssets, []byte(policyAll), 0o600); err != nil {
		t.Fatal(err)
	}
	if page := observePage(t, s); ingestStates(page, "") != "db=absent web=absent" || !strings.Contains(page, "data-ingest-go") {
		t.Fatalf("under another policy the expected references should differ: %s", ingestStates(page, ""))
	}
	run()
	b := latestDigest(t, mem, "web")
	if a == b || a == "" {
		t.Fatalf("the two policies should give different snapshots: %s %s", a, b)
	}
	if err := os.WriteFile(s.scopeAssets, []byte(policyExcludeSSHD), 0o600); err != nil {
		t.Fatal(err)
	}
	page := observePage(t, s)
	if got := ingestStates(page, ""); got != "db=body-only web=body-only" || !strings.Contains(page, "본체는 있으나 최신 참조가 아님") || !strings.Contains(page, "data-ingest-go") ||
		strings.Contains(page, `data-ingest="already"`) {
		t.Fatalf("back under the first policy: %q\n%s", got, section(page, "observe-ingest"))
	}
	run()
	if got := latestDigest(t, mem, "web"); got != a {
		t.Errorf("after re-ingesting under the first policy the latest is %s, want %s", got, a)
	}
	if !strings.Contains(observePage(t, s), `data-ingest="already"`) {
		t.Error("now it should read as already ingested")
	}
}

// failAfter: n번째 Append부터 실패하는 이력. 적재가 도중에 끊긴 것을 흉내 낸다.
type failAfter struct {
	*history.MemStore
	left int
}

func (f *failAfter) Append(s *history.Snapshot) error {
	if f.left <= 0 {
		return errors.New("write failed")
	}
	f.left--
	return f.MemStore.Append(s)
}

// IC-U250 — **적재가 도중에 끝나면 이력을 다시 읽어 노드마다 보인다. 종료 코드로 말하지 않는다.** 첫 노드만 기록되고 적재가
// 실패하면 화면은 「일부 노드만 기록됐습니다」와 함께 그 노드 「최신 참조가 기대와 같음」, 나머지 「기록되지 않음」을 보인다.
// 적재 프로세스가 결과를 남기지 못하고 끝나도(강제 종료) 같은 표가 나오고 그 사실을 적는다. 이력을 읽지 못하면 모든 노드가
// 「확인할 수 없음」이고 「기록 여부를 확인하지 못했습니다」이며 적재를 시작하지 않는다. 그 뒤 다시 확인해 적재하면 모든 노드가
// 기록된다. **「이력 변화가 없습니다」는 확인했을 때만 적는다**: 아무것도 쓰지 못한 채 실패한 적재(권한 없음)는 그 문장과 함께
// 적재 계정의 쓰기 권한을 확인하라고 안내하고, 일부가 기록된 적재에는 그 문장이 없다.
func TestAnInterruptedIngestShowsWhatTheHistoryHolds(t *testing.T) {
	s, mem := ingestServer(t)
	openIngestStore = func(string, string) (history.Store, func(), error) { return &failAfter{mem, 1}, func() {}, nil }
	form, _ := ingestForm(t, s)
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusSeeOther {
		t.Fatalf("ingest = %d", w.Code)
	}
	waitIngest(t, s)
	page := observePage(t, s)
	got := ingestStates(page, "ingest-last")
	if (got != "db=current web=absent" && got != "db=absent web=current") || !strings.Contains(page, `data-ingest-outcome="partial"`) ||
		!strings.Contains(page, `data-ingest-reason="ingest-failed"`) || !strings.Contains(page, "적재에 실패했습니다") ||
		!strings.Contains(page, "이력을 다시 읽은 결과: 일부 노드만 기록됐습니다") || strings.Contains(page, "이력 변화가 없습니다") {
		t.Errorf("after a failure partway: %q\n%s", got, section(page, "ingest-last"))
	}
	// 오류의 문장은 화면에도 실행 자리에도 없다(연결 정보가 섞일 수 있다).
	if strings.Contains(page, "write failed") || strings.Contains(readOrEmpty(filepath.Join(s.ingestRuns()[0], ingestResultFile)), "write failed") {
		t.Error("the text of the error is shown or stored")
	}
	// 결과를 남기지 못하고 끝난 프로세스: 결과 파일을 지운다(강제 종료와 같은 자리다).
	runs := s.ingestRuns()
	if err := os.Remove(filepath.Join(runs[len(runs)-1], ingestResultFile)); err != nil {
		t.Fatal(err)
	}
	page = observePage(t, s)
	if ingestStates(page, "ingest-last") != got || !strings.Contains(page, "결과를 남기지 못하고 끝났습니다") || !strings.Contains(page, `data-ingest-outcome="partial"`) {
		t.Errorf("with no result left the screen should still read the history:\n%s", section(page, "ingest-last"))
	}
	// 이력을 읽지 못하면 확인할 수 없다.
	useUpstream(s, downLookup{})
	page = observePage(t, s)
	if ingestStates(page, "ingest-last") != "db=unknown web=unknown" || !strings.Contains(page, `data-ingest-outcome="unknown"`) ||
		!strings.Contains(page, "기록 여부를 확인하지 못했습니다") || strings.Contains(page, "이력 변화가 없습니다") ||
		!strings.Contains(page, `data-ingest="state-unknown"`) || strings.Contains(page, "data-ingest-go") {
		t.Errorf("with the history unreachable:\n%s", section(page, "observe-ingest"))
	}
	if w := postForm(t, s, ui.IngestConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenObserve))); w.Code != http.StatusConflict {
		t.Errorf("confirm with the history unreachable = %d, want 409", w.Code)
	}
	useUpstream(s, mem)
	openIngestStore = func(string, string) (history.Store, func(), error) { return mem, func() {}, nil }
	form, _ = ingestForm(t, s)
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusSeeOther {
		t.Fatalf("the retry = %d", w.Code)
	}
	waitIngest(t, s)
	if page = observePage(t, s); ingestStates(page, "ingest-last") != "db=current web=current" {
		t.Errorf("after the retry: %s", ingestStates(page, "ingest-last"))
	}
}

// denied: 쓰기가 권한 오류로 실패하는 이력.
type denied struct{ *history.MemStore }

func (denied) Append(*history.Snapshot) error {
	return &pgconn.PgError{Code: "42501", Message: "permission denied for table pqcota_snapshots (host=db.internal user=svc)"}
}

// IC-U252 — **아무것도 쓰지 못한 채 실패한 적재는 「이력 변화가 없습니다」를 근거와 함께 적고, 권한 오류면 적재 계정의 쓰기
// 권한을 확인하라고 안내한다.** 근거는 노드마다 적재 전에 적어 둔 본체 · 최신 참조 · 관측 기록 수가 지금과 같다는 것이다.
// 오류의 문장(호스트 · 사용자가 실릴 수 있다)은 화면에도 실행 자리에도 없다. 같은 내용을 다시 적재해 관측 기록만 늘어난 경우는
// 「변화 없음」이 아니다: 그 뒤 실패한 적재의 기록에서 관측 기록 수가 달라지면 그 문장을 적지 않는다.
func TestAFailedIngestSaysUnchangedOnlyWhenItWasChecked(t *testing.T) {
	s, mem := ingestServer(t)
	openIngestStore = func(string, string) (history.Store, func(), error) { return denied{mem}, func() {}, nil }
	form, _ := ingestForm(t, s)
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusSeeOther {
		t.Fatalf("ingest = %d", w.Code)
	}
	waitIngest(t, s)
	page := observePage(t, s)
	for _, want := range []string{`data-ingest-reason="ingest-failed"`, `data-ingest-outcome="unchanged"`, "적재에 실패했습니다.", "이력 변화가 없습니다",
		"PQCOTA_DSN의 계정이 이력 표에 쓸 수 있는지 확인하십시오"} {
		if !strings.Contains(page, want) {
			t.Errorf("the screen lacks %q\n%s", want, section(page, "ingest-last"))
		}
	}
	dir := s.ingestRuns()[0]
	for _, leak := range []string{"permission denied", "db.internal", "user=svc", "lacks privileges"} {
		if strings.Contains(page, leak) || strings.Contains(readOrEmpty(filepath.Join(dir, ingestResultFile)), leak) {
			t.Errorf("%q from the error is shown or stored", leak)
		}
	}
	// 그 뒤 다른 경로로 이력이 달라지면(터미널에서 적재) 그 실패한 실행의 기록은 더는 「변화 없음」이 아니다.
	ingestedUnder(t, s, s.scopeAssets, mem)
	if page = observePage(t, s); strings.Contains(section(page, "ingest-last"), "이력 변화가 없습니다") {
		t.Errorf("the history changed since, so the record must not say it did not:\n%s", section(page, "ingest-last"))
	}
}

// IC-U251 — **업그레이드만으로 PQCOTA_DSN이 적재용으로 읽히지 않는다.** PQCOTA_DSN만 있으면 화면은 그것으로 조회하고(v0.33.0
// 까지와 같다) 적재하지 않는다. PQCOTA_READONLY_DSN만 있어도 적재하지 않는다. 둘 다 있을 때만 PQCOTA_READONLY_DSN으로 조회하고
// PQCOTA_DSN을 적재에 쓴다. 적재하지 않는 화면은 터미널 절차를 안내하고 직접 보낸 요청은 409다. 띄운 자식 프로세스의 환경에는
// 두 값이 없다.
func TestIngestNeedsBothConnections(t *testing.T) {
	s, _ := ingestServer(t)
	for name, c := range map[string]struct{ ro, dsn, readVar string }{
		"only PQCOTA_DSN":          {"", "postgres://read", upstream.DSNEnv},
		"only PQCOTA_READONLY_DSN": {"postgres://read", "", readonlyDSNEnv},
	} {
		t.Setenv(readonlyDSNEnv, c.ro)
		t.Setenv(upstream.DSNEnv, c.dsn)
		cfg, err := s.loadDSNs()
		if err != nil || cfg.read != "postgres://read" || cfg.readVar != c.readVar || cfg.ingest != "" {
			t.Errorf("%s: %+v %v", name, cfg, err)
		}
		page := observePage(t, s)
		if !strings.Contains(page, `data-ingest="not-configured"`) || strings.Contains(page, "data-ingest-go") || !strings.Contains(page, "pqcota-ingest -verify-keys") {
			t.Errorf("%s: the screen should explain and offer no button:\n%s", name, section(page, "observe-ingest"))
		}
		if w := postForm(t, s, ui.IngestConfirmPath, withRev(url.Values{}, s.revFor(ui.ScreenObserve))); w.Code != http.StatusConflict {
			t.Errorf("%s: confirm = %d, want 409", name, w.Code)
		}
	}
	t.Setenv(readonlyDSNEnv, "postgres://read")
	t.Setenv(upstream.DSNEnv, ingestSecret)
	cfg, err := s.loadDSNs()
	if err != nil || cfg.read != "postgres://read" || cfg.readVar != readonlyDSNEnv || cfg.ingest != ingestSecret {
		t.Errorf("both: %+v %v", cfg, err)
	}
	for _, e := range childEnv() {
		if strings.HasPrefix(e, readonlyDSNEnv+"=") || strings.HasPrefix(e, upstream.DSNEnv+"=") {
			t.Errorf("a child would inherit %s", strings.SplitN(e, "=", 2)[0])
		}
	}
	if vals, err := parseEnvFile([]byte("# both\nPQCOTA_READONLY_DSN=postgres://ro\nPQCOTA_DSN=postgres://rw\n")); err != nil || vals[readonlyDSNEnv] != "postgres://ro" || vals[upstream.DSNEnv] != "postgres://rw" {
		t.Errorf("pqcota.env with both lines: %v %v", vals, err)
	}
	if _, err := parseEnvFile([]byte("PQCOTA_INGEST_DSN=x\n")); err == nil {
		t.Error("another variable in pqcota.env should be refused")
	}
}

// candidateServer: ingestServer에 판정 세션과 자산 스코프 화면에서 확정한 새 정책(제외 없음)을 더한다. 활성 정책은 sshd를 제외한다.
func candidateServer(t *testing.T) (*server, *history.MemStore) {
	t.Helper()
	s, mem := ingestServer(t)
	savedSession(t, s)
	finalizeCandidate(t, s, policyAll)
	return s, mem
}

// policySection: 01의 「활성화할 확정 정책」 구역(다음 구역 앞까지).
func policySection(page string) string {
	i := strings.Index(page, `id="scope-activate"`)
	if i < 0 {
		return ""
	}
	if j := strings.Index(page[i:], `id="observe-ingest"`); j >= 0 {
		return page[i : i+j]
	}
	return page[i:]
}

// candidateForm: 활성화할 확정 정책으로 재적재하는 확인 페이지의 양식.
func candidateForm(t *testing.T, s *server) (url.Values, string) {
	t.Helper()
	w := postFormLang(t, s, ui.IngestConfirmPath, "ko", withRev(url.Values{ui.IngestPolicyField: {ui.IngestPolicyCandidate}}, s.revFor(ui.ScreenObserve)))
	if w.Code != http.StatusOK {
		t.Fatalf("candidate ingest confirm = %d\n%s", w.Code, w.Body.String())
	}
	return hiddenFields(w.Body.String()), w.Body.String()
}

// IC-U253 — **01이 현재 활성 정책과 활성화할 확정 정책을 나란히 보이고, 재적재와 활성화는 따로다.** 자산 스코프 화면에서 확정한 정책이 활성 정책과
// 다르면 01에 두 지문이 나란히 있고, 활성화할 확정 정책으로 재적재하는 단추가 있으며, 활성화 단추는 없다. 확인 페이지는 적재에 쓸
// 정책과 현재 활성 정책을 함께 적고 「적재가 끝나도 활성 정책은 바뀌지 않습니다」를 밝힌다.
//
//   - **활성화할 확정 정책으로 재적재가 도중에 실패하면 활성 정책은 그대로다.** 활성 정책 파일의 바이트가 같고, 활성화 단추가 없으며,
//     활성화를 직접 요청해도 409이고 파일이 그대로다.
//   - **재적재가 끝나도 운영자가 활성화를 확인하기 전까지 활성 정책은 그대로다.** 이력의 최신 참조는 활성화할 확정 정책의 것이고
//     활성 정책 파일은 같다. 그때 비로소 활성화 단추가 나온다. 적재 기록은 「이 실행은 이력에만 적재했습니다」라고 적는다(뒤에 활성화해도 맞는 말이다).
//     활성화를 기다리는 동안 주 동작은 「정책 활성화 확인」이고, 활성 정책으로 적재하는 구역은 「현재 활성 정책으로 다시 적재」
//     아래에 접혀 있으며 펼치면 최신 참조가 돌아간다는 결과를 알린다.
//   - **활성화 직전에 최신 참조가 바뀌면 거절된다.** 활성화 확인 페이지를 연 뒤 활성 정책으로 다시 적재되면(최신 참조가 옛 정책의
//     것으로 돌아갔다) 활성화는 409 `activate-history-changed`이고 활성 정책 파일이 그대로다. 거절은 「정책을 활성화하지
//     않았습니다」로 서고, 확인 후 최신 참조가 달라졌으며 현재 활성 정책은 그대로라고 적고, 01의 활성화 구역으로 잇는다. 01에는
//     다시 재적재 단추가 나오고 활성화 단추가 없으며 접힌 구역도 없다.
//   - 다시 재적재해 확인을 거쳐 활성화하면 활성 정책이 새 정책이 되고 01로 돌아오며 「활성화할 확정 정책」 구역이 없어진다.
func TestReingestUnderTheNewPolicyAndActivationAreSeparate(t *testing.T) {
	s, mem := candidateServer(t)
	active := readFile(t, s.scopeAssets)
	from, _ := scope.PolicyFileDigest(s.scopeAssets)
	to, _ := scope.PolicyFileDigest(s.scopeOut)
	same := func(when string) {
		t.Helper()
		if readFile(t, s.scopeAssets) != active {
			t.Fatalf("%s: the active policy file changed", when)
		}
	}
	sec := policySection(observePage(t, s))
	if a, c := strings.Index(sec, from), strings.Index(sec, to); a < 0 || c < a || !strings.Contains(sec, "현재 활성 정책") || !strings.Contains(sec, "활성화할 확정 정책") {
		t.Fatalf("01 does not show the active and the newly finalized policy side by side:\n%s", sec)
	}
	if !strings.Contains(sec, "data-policy-go") || strings.Contains(sec, "data-policy-activate>") || !strings.Contains(sec, "data-policy-activate-wait") {
		t.Fatalf("before the re-ingest: want a re-ingest button and no activation button:\n%s", sec)
	}

	// 1. 재적재가 도중에 실패한다.
	openIngestStore = func(string, string) (history.Store, func(), error) { return &failAfter{mem, 1}, func() {}, nil }
	form, confirm := candidateForm(t, s)
	for _, want := range []string{"적재에 쓸 정책: 활성화할 확정 정책 " + to, "현재 활성 정책: " + from, "적재가 끝나도 활성 정책은 바뀌지 않습니다"} {
		if !strings.Contains(confirm, want) {
			t.Errorf("the confirmation page lacks %q", want)
		}
	}
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusSeeOther {
		t.Fatalf("candidate ingest = %d\n%s", w.Code, w.Body.String())
	}
	waitIngest(t, s)
	same("after a re-ingest that failed partway")
	page := observePage(t, s)
	if sec = policySection(page); strings.Contains(sec, "data-policy-activate>") || !strings.Contains(sec, "data-policy-go") {
		t.Errorf("after a failed re-ingest: want no activation button and the re-ingest button again:\n%s", sec)
	}
	if !strings.Contains(page, `data-ingest-policy="candidate"`) || !strings.Contains(page, "적재에 실패했습니다") {
		t.Errorf("the record should say which policy was used and that it failed:\n%s", section(page, "ingest-last"))
	}
	if w := postFormLang(t, s, ui.ReviewScopeActivateConfirmPath, "ko", url.Values{ui.ActivateFromField: {ui.ActivateFromObserve}}); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), `href="`+ui.ScreenObserve+`"`) || !strings.Contains(w.Body.String(), "정책을 활성화하지 않았습니다") ||
		strings.Contains(w.Body.String(), "저장하지 않았습니다") {
		t.Errorf("activation confirm after a failed re-ingest = %d, want 409 headed as an activation refusal and leading back to 01\n%s", w.Code, w.Body.String())
	}
	same("after a refused activation")

	// 2. 재적재가 끝난다. 활성화를 확인하기 전까지 활성 정책은 그대로다.
	openIngestStore = func(string, string) (history.Store, func(), error) { return mem, func() {}, nil }
	form, _ = candidateForm(t, s)
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusSeeOther {
		t.Fatalf("candidate ingest = %d\n%s", w.Code, w.Body.String())
	}
	waitIngest(t, s)
	same("after the re-ingest finished")
	page = observePage(t, s)
	sec = policySection(page)
	if !strings.Contains(sec, `data-policy-state="reingested"`) || !strings.Contains(sec, "data-policy-activate>") || strings.Contains(sec, "data-policy-go") ||
		!strings.Contains(sec, "현재 활성 정책은 아직 그대로입니다") {
		t.Fatalf("after the re-ingest: want the activation button and no re-ingest button:\n%s", sec)
	}
	if !strings.Contains(page, `data-ingest-outcome="recorded"`) || !strings.Contains(page, "이 실행은 이력에만 적재했습니다") {
		t.Errorf("the record of the re-ingest:\n%s", section(page, "ingest-last"))
	}
	// 활성화를 기다리는 동안: 주 동작은 활성화 확인이고, 활성 정책으로 적재하는 구역은 접혀 있다.
	if !strings.Contains(sec, `<button class="primary" data-policy-activate>정책 활성화 확인</button>`) {
		t.Errorf("the activation confirmation is not the primary action:\n%s", sec)
	}
	fold := page[max(strings.Index(page, `id="observe-ingest"`), 0):]
	if !strings.Contains(fold, "data-ingest-fold") || !strings.Contains(fold, "현재 활성 정책으로 다시 적재") ||
		!strings.Contains(fold, "최신 참조가 현재 활성 정책의 것으로 돌아갑니다") || !strings.Contains(fold, "<button data-ingest-go>") ||
		strings.Contains(fold, `class="primary" data-ingest-go`) {
		t.Errorf("while waiting for the activation the active-policy ingest should be folded, warn, and not be primary:\n%s", fold)
	}
	activate := func() (url.Values, string) {
		t.Helper()
		w := postFormLang(t, s, ui.ReviewScopeActivateConfirmPath, "ko", url.Values{ui.ActivateFromField: {ui.ActivateFromObserve}})
		if w.Code != http.StatusOK {
			t.Fatalf("activation confirm = %d\n%s", w.Code, w.Body.String())
		}
		return hiddenFields(w.Body.String()), w.Body.String()
	}
	aform, abody := activate()
	if !strings.Contains(abody, from) || !strings.Contains(abody, to) {
		t.Error("the activation confirmation does not show the two fingerprints")
	}
	same("after opening the activation confirmation")

	// 3. 활성화 직전에 최신 참조가 바뀐다: 활성 정책으로 다시 적재됐다.
	ingestedUnder(t, s, s.scopeAssets, mem)
	w3 := postFormLang(t, s, ui.ReviewScopeActivatePath, "ko", aform)
	if w3.Code != http.StatusConflict {
		t.Fatalf("activation after the latest references changed = %d, want 409\n%s", w3.Code, w3.Body.String())
	}
	for _, want := range []string{ui.RefusedActivateHistoryChanged, "정책을 활성화하지 않았습니다", "확인 후 이력의 최신 참조가 달라졌습니다. 현재 활성 정책은 그대로입니다.",
		`href="` + ui.ScreenObserve + `?lang=ko#scope-activate"`} {
		if !strings.Contains(w3.Body.String(), want) {
			t.Errorf("the refusal after the latest references changed lacks %q\n%s", want, w3.Body.String())
		}
	}
	same("after an activation refused because the latest references changed")
	page = observePage(t, s)
	if sec = policySection(page); strings.Contains(sec, "data-policy-activate>") || !strings.Contains(sec, "data-policy-go") {
		t.Errorf("after the latest references changed: want the re-ingest button and no activation button:\n%s", sec)
	}
	if strings.Contains(page, "data-ingest-fold") {
		t.Error("the active-policy ingest is still folded although nothing waits for activation")
	}

	// 다시 재적재하고, 확인을 거쳐 활성화한다.
	form, _ = candidateForm(t, s)
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusSeeOther {
		t.Fatalf("candidate ingest = %d", w.Code)
	}
	waitIngest(t, s)
	same("after the second re-ingest")
	aform, _ = activate()
	w := postForm(t, s, ui.ReviewScopeActivatePath, aform)
	if w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), ui.ScreenObserve) {
		t.Fatalf("activate = %d to %q, want 303 back to 01\n%s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if readFile(t, s.scopeAssets) != policyAll {
		t.Error("the active policy was not replaced by the newly finalized one")
	}
	// 활성화한 뒤: 위쪽에 현재 활성 정책의 지문이 있고, 지난 재적재의 기록은 여전히 맞는 말이다.
	if page = observePage(t, s); !strings.Contains(page, "<p data-active-policy>현재 활성 정책: <code>"+to+"</code>") ||
		!strings.Contains(page, "이 실행은 이력에만 적재했습니다") || strings.Contains(page, "data-ingest-fold") {
		t.Errorf("after the activation 01 should show the active policy's fingerprint above the record:\n%s", section(page, "observe-ingest"))
	}
	if policySection(page) != "" || !strings.Contains(page, `data-ingest="already"`) {
		t.Errorf("after the activation 01 should have no pending policy and read already ingested under the active one:\n%s", section(page, "observe-ingest"))
	}
}

// IC-U254 — **활성화할 확정 정책으로 적재하는 확인은 그 정책에 묶인다.** 확인 페이지를 연 뒤 자산 스코프 화면에서 확정한 정책 파일이 바뀌면(확정이
// 쓴 그대로가 아니다) 그 양식으로 보낸 적재는 409이고 아무것도 쓰이지 않는다. 활성 정책으로 적재하는 확인의 양식에 정책 칸만
// 바꿔 보내도 거절된다(확인한 묶음이 아니다). 알 수 없는 정책 값도 거절된다. 자산 스코프 화면에서 확정한 정책이 활성 정책과 같으면 01에 그
// 구역이 없고 활성화할 확정 정책으로 적재하는 확인 요청은 409다.
func TestTheCandidateIngestIsBoundToItsPolicy(t *testing.T) {
	s, mem := candidateServer(t)
	form, _ := candidateForm(t, s)
	finalized := readFile(t, s.scopeOut)
	if err := os.WriteFile(s.scopeOut, []byte(finalized+"exclude,*,*,/opt/*,hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusConflict {
		t.Errorf("candidate ingest after the candidate file changed = %d, want 409\n%s", w.Code, w.Body.String())
	}
	if err := os.WriteFile(s.scopeOut, []byte(finalized), 0o644); err != nil {
		t.Fatal(err)
	}
	form, _ = ingestForm(t, s)
	form.Set(ui.IngestPolicyField, ui.IngestPolicyCandidate)
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedIngestChangedPolicy) {
		t.Errorf("an active-policy confirmation sent as a candidate ingest = %d, want 409 %s", w.Code, ui.RefusedIngestChangedPolicy)
	}
	form.Set(ui.IngestPolicyField, "something-else")
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusConflict {
		t.Errorf("an unknown policy value = %d, want 409", w.Code)
	}
	if latestDigest(t, mem, "web") != "" || ingestRunCount(s) != 0 {
		t.Fatal("something was ingested")
	}

	same, _ := ingestServer(t)
	savedSession(t, same)
	finalizeCandidate(t, same, readFile(t, same.scopeAssets))
	if policySection(observePage(t, same)) != "" {
		t.Error("01 shows a pending policy although the finalized one equals the active one")
	}
	if w := postForm(t, same, ui.IngestConfirmPath, withRev(url.Values{ui.IngestPolicyField: {ui.IngestPolicyCandidate}}, same.revFor(ui.ScreenObserve))); w.Code != http.StatusConflict {
		t.Errorf("candidate ingest confirm with nothing pending = %d, want 409", w.Code)
	}
}

// IC-U255 — **화면이 서명을 확인하지 못하는 결과를 터미널에서 다시 적재했어도, 이력이 맞으면 활성화할 수 있다.** 활성화의 조건은
// 이력 대조뿐이고, 화면이 적재할 수 있는가(서명 · 검증 키)와는 따로다. 서명이 없는 결과가 섞였거나 검증 키가 없으면 01의
// 「이 정책으로 재적재」 단추는 나오지 않고 까닭이 보인다. 그 상태에서 이력이 아직 활성 정책의 것이면 활성화 단추도 없다.
// 같은 결과를 확정 정책으로 (화면 밖에서) 적재하면 01이 노드별 상태로 그것을 확인하고 「정책 활성화 확인」을 내며, 확인을 거쳐
// 활성화된다. 막힌 까닭은 재적재가 더 필요 없으므로 사라진다.
func TestActivationFollowsTheHistoryNotTheScreenIngest(t *testing.T) {
	for name, blind := range map[string]func(t *testing.T, s *server) string{
		"an unsigned result": func(t *testing.T, s *server) string {
			path := filepath.Join(s.results, "web-openssl.json")
			if err := os.WriteFile(path, []byte(dropSignature(readFile(t, path))), 0o600); err != nil {
				t.Fatal(err)
			}
			return `data-policy-block="problems"`
		},
		"no verification key": func(t *testing.T, s *server) string {
			s.collectorVerify = filepath.Join(t.TempDir(), "missing.pub")
			return `data-policy-block="` + ui.IngestNoVerifyKey + `"`
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, mem := candidateServer(t)
			block := blind(t, s)
			ingestedUnder(t, s, s.scopeAssets, mem)
			sec := policySection(observePage(t, s))
			if !strings.Contains(sec, block) || strings.Contains(sec, "data-policy-go") || strings.Contains(sec, "data-policy-activate>") {
				t.Fatalf("before the re-ingest: want the reason, no re-ingest button and no activation button:\n%s", sec)
			}
			if !strings.Contains(sec, `data-ingest-state="body-only"`) && !strings.Contains(sec, `data-ingest-state="absent"`) {
				t.Errorf("the per-node state of the history is not shown although the screen cannot ingest:\n%s", sec)
			}
			ingestedUnder(t, s, s.scopeOut, mem) // 화면 밖에서(터미널) 확정 정책으로 다시 적재했다.
			sec = policySection(observePage(t, s))
			if !strings.Contains(sec, `data-policy-state="reingested"`) || !strings.Contains(sec, "data-policy-activate>") || strings.Contains(sec, block) {
				t.Fatalf("after a re-ingest outside the screen: want the activation button and no reason left:\n%s", sec)
			}
			w := postFormLang(t, s, ui.ReviewScopeActivateConfirmPath, "ko", url.Values{ui.ActivateFromField: {ui.ActivateFromObserve}})
			if w.Code != http.StatusOK {
				t.Fatalf("activation confirm = %d\n%s", w.Code, w.Body.String())
			}
			if w = postForm(t, s, ui.ReviewScopeActivatePath, hiddenFields(w.Body.String())); w.Code != http.StatusSeeOther {
				t.Fatalf("activate = %d\n%s", w.Code, w.Body.String())
			}
			if readFile(t, s.scopeAssets) != policyAll {
				t.Error("the active policy was not replaced")
			}
		})
	}
}
