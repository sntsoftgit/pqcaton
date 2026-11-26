// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"errors"
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

// IC-U246 — **03에서 서명된 결과를 활성 정책으로 이력에 적재한다. 운영자가 확인을 거쳐 누를 때만.** 적재 전 화면은 노드마다
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
	for name, text := range map[string]string{"03": page, "confirmation": confirm, "settings": settings} {
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

func readOrEmpty(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

// IC-U247 — **결과 하나라도 서명이 없거나 맞지 않으면 묶음 전체를 적재하지 않는다.** 서명 뒤에 바뀐 결과가 하나 섞이면 03은
// 파일 · 수집기 ID · 사유를 보이고 버튼을 내지 않는다. 확인 요청은 409이고 이력에는 아무것도 쓰이지 않는다(맞는 서명의 결과도).
// 서명이 없는 결과도 같다. 해독되지 않는 파일도 같다.
func TestOneBadSignatureRefusesTheWholeSet(t *testing.T) {
	for name, c := range map[string]struct {
		edit func(string) string
		why  string
	}{
		"changed after signing": {func(b string) string { return strings.Replace(b, "v0.20.0", "v9.9.9", 1) }, ui.IngestWhyMismatch},
		"not signed": {func(b string) string {
			return regexp.MustCompile(`,?"signature":"[^"]*"`).ReplaceAllString(b, "")
		}, ui.IngestWhyUnsigned},
		"not a result": {func(string) string { return `{"envelope": 1` }, ui.IngestWhyNotResult},
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
// 바뀌거나 결과 파일이 바뀌면 그 양식으로 보낸 적재는 409이고 아무것도 쓰이지 않는다. 같은 양식을 동시에 둘 보내면 하나만
// 시작된다. 쓰기 전에 멈춘 적재(조회와 적재의 데이터베이스가 다름) 뒤에 같은 양식을 다시 보내도 409이고 두 번째 실행 자리가
// 생기지 않는다.
func TestTheConfirmedSetIsWhatIsIngestedAndOnlyOnce(t *testing.T) {
	s, mem := ingestServer(t)
	form, _ := ingestForm(t, s)
	if err := os.WriteFile(s.scopeAssets, []byte(policyAll), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusConflict {
		t.Errorf("ingest after the active policy changed = %d, want 409", w.Code)
	}
	if err := os.WriteFile(s.scopeAssets, []byte(policyExcludeSSHD), 0o600); err != nil {
		t.Fatal(err)
	}
	form, _ = ingestForm(t, s)
	extra := filepath.Join(s.results, "app-openssl.json")
	if err := os.WriteFile(extra, signedResult(t, s, "app"), 0o600); err != nil { // 확인한 뒤에 결과가 하나 더 들어왔다
		t.Fatal(err)
	}
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusConflict {
		t.Errorf("ingest after the result files changed = %d, want 409", w.Code)
	}
	if err := os.Remove(extra); err != nil {
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
	if w := postForm(t, s, ui.IngestPath, form); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), ui.RefusedIngestReplay) || ingestRunCount(s) != 1 {
		t.Errorf("the same confirmation again = %d (runs %d), want 409 %s and one run", w.Code, ingestRunCount(s), ui.RefusedIngestReplay)
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
// 「확인할 수 없음」이고 적재를 시작하지 않는다. 그 뒤 다시 확인해 적재하면 모든 노드가 기록된다.
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
		!strings.Contains(page, `data-ingest-reason="ingest-failed"`) || !strings.Contains(page, "일부가 기록됐을 수 있습니다") {
		t.Errorf("after a failure partway: %q\n%s", got, section(page, "ingest-last"))
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
