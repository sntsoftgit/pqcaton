package intake_test

import (
	"errors"
	"testing"
	"time"

	commonv1 "github.com/pqcota/pqcota/gen/pqcota/common/v1"
	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"
	"github.com/pqcota/pqcota/pkg/discovery/history"
	"github.com/pqcota/pqcota/pkg/kernel/sign"
	"github.com/pqcota/pqcota/pkg/org"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/sntsoftgit/pqcaton/saas/internal/access"
	"github.com/sntsoftgit/pqcaton/saas/internal/intake"
)

const (
	acme      = org.ID("acme")
	collector = "openssl-collector"
)

var t0 = time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)

// result — 서명하지 않은 결과 하나. raw로 내용을 갈라 둔다.
func result(node, raw string, at time.Time) *discoveryv1.CollectionResult {
	return &discoveryv1.CollectionResult{
		Envelope: &commonv1.Envelope{
			CollectorId:      collector,
			CollectorVersion: "0.1.0",
			DetectionMethod:  commonv1.DetectionMethod_DETECTION_METHOD_RUNTIME_INTROSPECTION,
			CollectedAt:      timestamppb.New(at),
			TargetNodeId:     node,
		},
		RawCapture:           []byte(raw),
		RawFormat:            "openssl-collector/native-v1",
		CyclonedxSpecVersion: "1.6",
	}
}

func signed(t *testing.T, priv string, res *discoveryv1.CollectionResult) *discoveryv1.CollectionResult {
	t.Helper()
	sig, err := sign.Sign(priv, res)
	if err != nil {
		t.Fatalf("서명: %v", err)
	}
	res.Envelope.Signature = sig
	return res
}

// fixture — 조직 하나에 collector 키 하나가 등록된 상태.
type fixture struct {
	keys  *access.MemStore
	hist  *history.MemStore
	seen  *intake.MemSeen
	pub   string
	priv  string
	opts  intake.Options
	store *history.MemStore
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pub, priv, err := sign.Generate()
	if err != nil {
		t.Fatalf("키 생성: %v", err)
	}
	keys := access.NewMemStore()
	if err := keys.RegisterKey(access.CollectorKey{
		Org: acme, CollectorID: collector, PublicKey: pub, Registered: t0,
	}); err != nil {
		t.Fatalf("키 등록: %v", err)
	}
	h := history.NewMemStore()
	f := &fixture{keys: keys, hist: h, seen: intake.NewMemSeen(), pub: pub, priv: priv, store: h}
	f.opts = intake.Options{
		Org: acme, Keys: keys, History: h, Rejections: h, Seen: f.seen,
		SnapshotPrefix: "snap", RulesetVersion: "r", RunnerVersion: "0.1.0",
	}
	return f
}

// CP-INTAKE-1 — 등록된 키로 서명된 결과가 통과해 쌓인다.
// 거절만 시험하면 게이트가 정상 반입까지 막는 것을 못 잡는다.
func TestAcceptsResultSignedByRegisteredKey(t *testing.T) {
	f := newFixture(t)
	rep, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{
		signed(t, f.priv, result("web-01", "a", t0)),
	})
	if err != nil {
		t.Fatalf("수신: %v", err)
	}
	if rep.Accepted != 1 || rep.Rejected != 0 || rep.Duplicate != 0 {
		t.Fatalf("정상 결과가 통과하지 못했다: %+v", rep)
	}
	if snap, _ := f.store.Latest("web-01"); snap == nil {
		t.Fatal("스냅샷이 쌓이지 않았다")
	}
}

// CP-INTAKE-2 — 서명 없는 결과는 받지 않는다.
func TestRejectsUnsignedResult(t *testing.T) {
	f := newFixture(t)
	rep, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{result("web-01", "a", t0)})
	if err != nil {
		t.Fatalf("수신: %v", err)
	}
	if rep.Accepted != 0 || rep.Rejected != 1 {
		t.Fatalf("서명 없는 결과가 통과했다: %+v", rep)
	}
}

// CP-INTAKE-3 — **다른 조직의 키로 서명된 결과는 통과하지 못한다.**
//
// 등록소 조회에 조직 조건이 붙는다는 것이 여기서 값을 한다. 조회가 조직을 안 걸면
// 남의 collector가 서명한 결과가 우리 조직으로 들어온다.
func TestRejectsResultSignedByAnotherOrgsKey(t *testing.T) {
	f := newFixture(t)
	otherPub, otherPriv, _ := sign.Generate()
	_ = f.keys.RegisterKey(access.CollectorKey{
		Org: "beta", CollectorID: collector, PublicKey: otherPub, Registered: t0,
	})

	rep, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{
		signed(t, otherPriv, result("web-01", "a", t0)),
	})
	if err != nil {
		t.Fatalf("수신: %v", err)
	}
	if rep.Accepted != 0 || rep.Rejected != 1 {
		t.Fatalf("다른 조직 키가 통과했다: %+v", rep)
	}
}

// CP-INTAKE-4 — 같은 결과를 다시 올리면 접힌다. 거절이 아니라 **받았다고 답한다.**
func TestResendIsFoldedNotRejected(t *testing.T) {
	f := newFixture(t)
	res := signed(t, f.priv, result("web-01", "a", t0))

	if _, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{res}); err != nil {
		t.Fatalf("첫 수신: %v", err)
	}
	rep, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{res})
	if err != nil {
		t.Fatalf("재전송: %v", err)
	}
	if rep.Duplicate != 1 || rep.Rejected != 0 || rep.Accepted != 0 {
		t.Fatalf("재전송이 접히지 않았다: %+v", rep)
	}
}

// CP-INTAKE-5 — **노드당 결과가 여럿이어도 서로 접히지 않는다.**
//
// 이 케이스가 멱등 키 선택의 이유다. 엔벨로프의 collector_id+node_id+시각을 키로 삼으면
// 아래 둘이 같은 키가 되어, 정상 결과 하나가 조용히 사라진다(JVM마다 결과 하나가 나오는
// 경우가 그렇다).
func TestResultsFromSameNodeAndInstantAreDistinct(t *testing.T) {
	f := newFixture(t)
	a := signed(t, f.priv, result("web-01", "jvm-1", t0))
	b := signed(t, f.priv, result("web-01", "jvm-2", t0)) // 같은 collector·노드·시각

	rep, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{a, b})
	if err != nil {
		t.Fatalf("수신: %v", err)
	}
	if rep.Duplicate != 0 {
		t.Fatalf("서로 다른 결과가 중복으로 접혔다: %+v", rep)
	}
	if rep.Accepted != 2 {
		t.Fatalf("둘 다 통과하지 않았다: %+v", rep)
	}
	if intake.Fingerprint(a) == intake.Fingerprint(b) {
		t.Fatal("지문이 같다 — 내용이 다른데 같은 키가 나온다")
	}
}

// CP-INTAKE-6 — **거절된 결과는 「본 것」으로 남지 않는다.**
//
// 남으면 그 결과는 영영 못 들어온다 — 키를 뒤늦게 등록하고 다시 올려도 중복으로 접힌다.
// 멱등은 재전송을 접는 장치이지 실패를 굳히는 장치가 아니다.
func TestRejectedResultCanBeRetriedAfterKeyIsRegistered(t *testing.T) {
	f := newFixture(t)
	latePub, latePriv, _ := sign.Generate()
	res := signed(t, latePriv, result("web-01", "a", t0))

	rep, _ := intake.Receive(f.opts, []*discoveryv1.CollectionResult{res})
	if rep.Rejected != 1 {
		t.Fatalf("등록 안 된 키가 통과했다: %+v", rep)
	}

	// 뒤늦게 그 키를 등록하고 같은 결과를 다시 올린다.
	_ = f.keys.RegisterKey(access.CollectorKey{
		Org: acme, CollectorID: collector, PublicKey: latePub, Registered: t0,
	})
	rep, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{res})
	if err != nil {
		t.Fatalf("재시도: %v", err)
	}
	if rep.Accepted != 1 || rep.Duplicate != 0 {
		t.Fatalf("거절이 굳어 다시 들어오지 못했다: %+v", rep)
	}
}

// CP-INTAKE-7 — 키 교체 구간에서는 옛 키와 새 키가 **둘 다** 통과한다.
// 이 구간이 없으면 키를 바꾸는 순간 그 조직의 관측이 전부 거절된다.
func TestBothKeysWorkDuringRotation(t *testing.T) {
	f := newFixture(t)
	newPub, newPriv, _ := sign.Generate()
	_ = f.keys.RegisterKey(access.CollectorKey{
		Org: acme, CollectorID: collector, PublicKey: newPub, Registered: t0,
	})

	rep, err := intake.Receive(f.opts, []*discoveryv1.CollectionResult{
		signed(t, f.priv, result("web-01", "old", t0)),
		signed(t, newPriv, result("web-02", "new", t0)),
	})
	if err != nil {
		t.Fatalf("수신: %v", err)
	}
	if rep.Accepted != 2 || rep.Rejected != 0 {
		t.Fatalf("교체 구간에서 한쪽이 막혔다: %+v", rep)
	}
}

// CP-INTAKE-8 — 조직 없이 부를 수 없다.
func TestReceiveRequiresOrg(t *testing.T) {
	f := newFixture(t)
	o := f.opts
	o.Org = ""
	if _, err := intake.Receive(o, nil); !errors.Is(err, intake.ErrNoOrg) {
		t.Fatalf("조직 없이 적재됐다: %v", err)
	}
}

// CP-INTAKE-9 — 멱등 저장소도 조직으로 갈린다.
// 여기서 섞이면 한 조직의 결과가 다른 조직에서 "이미 받았다"로 사라진다.
func TestSeenStoreIsolatesOrg(t *testing.T) {
	s := intake.NewMemSeen()
	if ok, err := s.Claim(acme, "fp"); err != nil || !ok {
		t.Fatalf("확보: %v %v", ok, err)
	}
	fresh, err := s.Claim("beta", "fp")
	if err != nil {
		t.Fatalf("확보: %v", err)
	}
	if !fresh {
		t.Fatal("다른 조직의 지문이 보인다")
	}
	if _, err := s.Claim("", "fp"); !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("조직 없이 확보됐다: %v", err)
	}
}
