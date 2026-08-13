// Package intake — 러너가 올린 관측 결과를 받는 경계.
//
// [러너 설계 §5.1](../../design.md)이 말한 경계의 일 — **서명 확인 · 조직 부여 ·
// 스키마 흡수** — 이 여기서 일어난다. HTTP는 이 위에 얹힌다: 이 패키지는 네트워크를
// 모르고, 조직은 이미 정해진 것을 받는다.
package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	discoveryv1 "github.com/pqcota/pqcota/gen/pqcota/discovery/v1"
	"github.com/pqcota/pqcota/pkg/discovery/history"
	"github.com/pqcota/pqcota/pkg/inventory/ingest"
	"github.com/pqcota/pqcota/pkg/kernel/scope"
	"github.com/pqcota/pqcota/pkg/kernel/sign"
	"github.com/pqcota/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/saas/internal/access"
)

// ErrNoOrg — 조직 없이 부를 수 없다. 여기까지 왔는데 비었으면 인증 경로가 깨진 것이다.
var ErrNoOrg = errors.New("조직 없이 적재할 수 없다")

// Options — 수신 한 번의 설정.
type Options struct {
	// Org — 인증에서 유도된 조직. **본문의 주장이 아니다**(§6.4).
	Org org.ID
	// Keys — collector 공개키 등록소. 조직·collector로 좁혀 조회한다(§6.4.2).
	Keys access.KeyStore
	// History — 그 조직에 묶인 히스토리 저장소(history.NewPgStoreIn).
	History history.Store
	// Rejections — 받지 않은 사실을 남길 곳. history.PgStore·MemStore가 만족한다.
	Rejections ingest.RejectionStore
	// Seen — 이미 받은 결과인지 아는 곳. 재전송을 접는다.
	Seen SeenStore
	// RunnerVersion — 이 결과를 올린 러너의 버전. 거절을 기록할 때 함께 남긴다 —
	// 서명 실패가 갱신 문제인지 위조인지는 사유만으로 못 가리므로, "이 러너의 엣지
	// 결과만 전부 실패"라는 모양이 보이게 해 둔다(§6.6).
	RunnerVersion string

	// Master — 스코프 마스터. nil이면 게이트를 생략한다(등재 전 단계·데모).
	Master *scope.Master
	// SnapshotPrefix·RulesetVersion — 상류 적재가 요구하는 값.
	SnapshotPrefix string
	RulesetVersion string
}

// Report — 수신 결과. 상류의 IngestReport에 **중복**을 더한 것이다.
type Report struct {
	ingest.IngestReport
	// Duplicate — 이미 받은 것이라 다시 쌓지 않은 수. 거절이 아니다 — 받았다고 답한다.
	Duplicate int
}

// Receive — 결과를 받는다.
//
// 순서가 중요하다: **멱등 → 서명 → 적재.** 이미 받은 것을 다시 검증하지 않는다 —
// 재전송은 정상 동작이라 검증 비용을 두 번 치를 이유가 없고, 거절 기록에 같은 사실이
// 두 번 남지도 않는다.
func Receive(o Options, results []*discoveryv1.CollectionResult) (Report, error) {
	if o.Org == "" {
		return Report{}, ErrNoOrg
	}
	if o.Seen == nil || o.Keys == nil || o.History == nil {
		return Report{}, errors.New("수신에 필요한 저장소가 비었다")
	}

	var rep Report
	fresh := make([]*discoveryv1.CollectionResult, 0, len(results))
	claimed := make([]string, 0, len(results))
	for _, res := range results {
		h := Fingerprint(res)
		ok, err := o.Seen.Claim(o.Org, h)
		if err != nil {
			return rep, fmt.Errorf("멱등 확보: %w", err)
		}
		if !ok {
			rep.Duplicate++
			continue
		}
		fresh = append(fresh, res)
		claimed = append(claimed, h)
	}
	if len(fresh) == 0 {
		return rep, nil
	}

	verify := o.verifier()
	out, err := ingest.IngestWith(fresh, ingest.IngestOptions{
		Master:         o.Master,
		VerifySig:      verify,
		SnapshotPrefix: o.SnapshotPrefix,
		RulesetVersion: o.RulesetVersion,
		Store:          o.History,
		Rejections:     o.Rejections,
		// 여러 조직의 결과가 한 저장소로 모이는 곳이다. 조용히 통과하는 경로를 열어 두지
		// 않는다 — 검증자가 없으면 적재 자체가 거절된다.
		RequireSignature: true,
	})
	if err != nil {
		// 적재가 통째로 실패하면 확보한 것을 전부 놓는다 — 안 그러면 그 결과들이
		// 영영 못 들어온다.
		o.releaseAll(claimed)
		return rep, err
	}
	rep.IngestReport = *out

	// **실제로 쌓이지 못한 것은 확보를 놓는다.**
	//
	// 쥔 채로 두면 그 결과는 영영 못 들어온다 — 키를 고쳐 등록하거나 노드를 등재한 뒤
	// 같은 결과를 다시 올려도 "이미 봤다"로 접힌다. 멱등은 재전송을 접는 장치이지
	// 실패를 굳히는 장치가 아니다.
	stored := make(map[string]bool, len(out.Nodes))
	for _, n := range out.Nodes {
		stored[n] = true
	}
	for i, res := range fresh {
		if stored[res.GetEnvelope().GetTargetNodeId()] && verify(res) {
			continue
		}
		if err := o.Seen.Release(o.Org, claimed[i]); err != nil {
			return rep, fmt.Errorf("멱등 반환: %w", err)
		}
	}
	return rep, nil
}

// verifier — 이 조직의 그 collector에 등록된 키로만 확인하는 검증자.
//
// sign.VerifyFrom을 쓰지 않는다. 그 함수는 collector당 키 하나만 받아 **교체 구간**을
// 표현하지 못한다(§6.4.2). 좁히는 일을 등록소 조회가 하므로 안전성은 같고, 조직 조건이
// 같은 질의에 붙어 조직 경계도 한 자리에서 걸린다.
func (o Options) verifier() func(*discoveryv1.CollectionResult) bool {
	return func(res *discoveryv1.CollectionResult) bool {
		cid := res.GetEnvelope().GetCollectorId()
		keys, err := o.Keys.ActiveKeys(o.Org, cid)
		if err != nil || len(keys) == 0 {
			return false // 모르는 collector의 주장은 받지 않는다
		}
		return sign.Verify(keys, res)
	}
}

// Fingerprint — 결과의 멱등 키. 서명 대상 바이트의 SHA-256이다.
//
// 엔벨로프의 collector_id+node_id+시각을 키로 삼지 않는 이유: CollectionResult는 **노드당
// 여러 개**가 나올 수 있어(JVM마다 하나) 그 셋이 같은 결과가 정상적으로 생긴다. 그러면
// 서로를 중복으로 접어 조용히 버리게 된다.
//
// canonical 바이트는 raw_capture와 CBOM 본문까지 덮으므로 다른 관측은 다른 값이 되고,
// 재전송만 같은 값이 된다. 새로 계산할 것도 없다 — 어차피 서명을 확인하려고 만드는 바이트다.
func Fingerprint(res *discoveryv1.CollectionResult) string {
	sum := sha256.Sum256(sign.Canonical(res))
	return hex.EncodeToString(sum[:])
}

// releaseAll — 확보한 지문을 전부 놓는다. 반환 실패는 삼킨다 —
// 이미 다른 오류를 올리는 중이고, 여기서 덮어쓰면 원인이 가려진다.
func (o Options) releaseAll(fps []string) {
	for _, fp := range fps {
		_ = o.Seen.Release(o.Org, fp)
	}
}
