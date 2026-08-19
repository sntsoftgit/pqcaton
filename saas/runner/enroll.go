package runner

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// enrollDir — 등재 결과가 모이는 곳(`<ResultsDir>/enroll/`).
//
// 관측 결과와 **섞지 않는다.** 모양이 다르고(이쪽은 우리가 정한 형식, 저쪽은 pqcota 계약),
// 올라가는 자리도 다르다. 같은 디렉터리에 두면 `read`가 하나를 다른 하나로 읽는다.
const enrollDir = "enroll"

// enrollFile — 연결확인 플레이북이 **대상마다 하나씩** 쓰는 파일.
//
// 붙었으면 지문이, 못 붙었으면 사유가 있다. 둘 다 없는 파일은 말이 되지 않는다 —
// 그때는 사유를 우리가 붙여 올린다(아래 [toEnrollment]).
type enrollFile struct {
	NodeID      string `json:"node_id"`
	Fingerprint string `json:"fingerprint"`
	DisplayName string `json:"display_name"`
	// Addr — 접속 주소(`10.20.3.14:22`). **이 값은 밖으로 나가지 않는다** — 러너가
	// 토큰으로 바꾸고 버린다(§6.3.1). 파일에 있는 것은 토큰을 만들기 위해서다.
	Addr string `json:"addr"`
	// Err — 못 붙은 사유. 러너가 붙인 로컬 별칭과 함께 올라가, 주소 없이도 운영자가
	// 무엇이 안 붙었는지 안다(§6.3).
	Err string `json:"error"`
}

// addrTokenPrefix — 토큰임을 눈으로 알아보게 하는 접두어. 주소로 오인하면 그것으로
// 접속을 시도하는 사람이 생긴다.
const addrTokenPrefix = "addr-"

// addrToken — 주소를 조직 단위 키로 만든 토큰(§6.3.1).
//
// **주소 자체는 어디에도 올라가지 않는다.** 키는 러너에만 있어 우리 쪽에서는 되돌릴 수
// 없고, 같은 주소는 늘 같은 토큰이 되므로 **영역 간에 같은 상대를 이어 붙일 수 있다.**
//
// 키가 없으면 빈 값이다. 그러면 나중에 영역 간 엣지를 이어 붙일 표가 안 생긴다 —
// [RunOnce]가 그 사실을 로그로 남긴다.
func addrToken(key, addr string) string {
	if key == "" || addr == "" {
		return ""
	}
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(addr))
	return addrTokenPrefix + hex.EncodeToString(m.Sum(nil)[:8])
}

// toEnrollment — 파일 하나를 올릴 형태로 바꾼다. 주소는 여기서 토큰이 되고 버려진다.
//
// **붙었다는데 지문이 없으면 실패로 바꾼다.** 그대로 올리면 지문 없는 노드가 등재되어
// 클론 검출을 통째로 빠져나간다 — 그렇다고 조용히 버리면 운영자는 그 대상이 등재된 줄
// 안다. 사유를 붙여 올려야 **화면에서 보인다.**
func (f enrollFile) toEnrollment(key string) Enrollment {
	e := Enrollment{
		NodeID:      f.NodeID,
		Fingerprint: f.Fingerprint,
		DisplayName: f.DisplayName,
		AddrToken:   addrToken(key, f.Addr),
		Err:         f.Err,
	}
	if e.Err == "" && e.Fingerprint == "" {
		e.Err = "reported as connected but has no fingerprint — the connectivity check only half finished"
	}
	if e.DisplayName == "" {
		e.DisplayName = f.NodeID
	}
	return e
}

// enrollBatch — 등재 디렉터리를 한 번 읽은 것.
type enrollBatch struct {
	Items []Enrollment
	Good  []string // 보낸 뒤 옮길 파일
	Bad   []string // 읽을 수 없어 치울 파일
	// SawAddr — 주소가 적힌 파일이 있었나. 키가 없으면 토큰이 안 만들어지므로,
	// 그 사실을 [RunOnce]가 알린다.
	SawAddr bool
}

// readEnrollments — 등재 디렉터리를 읽는다. 보낼 것과 옮길 것, 치울 것으로 가른다.
//
// 관측 결과와 달리 **여기서는 내용을 본다.** 형식을 정한 쪽이 우리라서다 — `node_id`가
// 없는 파일은 컨트롤 플레인이 쓸 수 없으므로 보내지 않고 증거로 남긴다.
func readEnrollments(dir, key string) (enrollBatch, error) {
	var b enrollBatch
	files, err := jsonFiles(filepath.Join(dir, enrollDir))
	if err != nil {
		return b, err
	}
	for _, f := range files {
		raw, rerr := os.ReadFile(f)
		var ef enrollFile
		if rerr != nil || json.Unmarshal(raw, &ef) != nil || strings.TrimSpace(ef.NodeID) == "" {
			b.Bad = append(b.Bad, f)
			continue
		}
		if ef.Addr != "" {
			b.SawAddr = true
		}
		b.Items = append(b.Items, ef.toEnrollment(key))
		b.Good = append(b.Good, f)
	}
	return b, nil
}
