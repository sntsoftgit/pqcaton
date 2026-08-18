// Package probe — 계약이 실제로 소비되는지 확인하는 최소 코드.
//
// 이 리포의 첫 관심사는 "pqcota의 계약 타입을 import해서 쓸 수 있는가"다. 그것이
// 되지 않으면 나머지 설계가 의미가 없다. 아래 한 함수가 그 연결을 붙들어 둔다 —
// 지워도 빌드는 되지만, 그러면 go.mod의 require가 조용히 미사용이 된다.
package probe

import (
	commonv1 "github.com/randyinthedev-hash/pqcota/gen/pqcota/common/v1"
)

// RuntimeName — 계약의 CryptoRuntime enum을 사람이 읽는 이름으로.
// 계약 어휘를 이 리포가 그대로 쓴다는 것을 보이는 최소 예시다.
func RuntimeName(r commonv1.CryptoRuntime) string {
	return r.String()
}
