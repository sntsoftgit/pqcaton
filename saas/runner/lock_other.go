//go:build !unix

package runner

import (
	"errors"
	"os"
)

// flock — 유닉스가 아닌 곳에서는 **조용히 통과시키지 않는다.**
//
// 잠금 없이 도는 것은 겹쳐 도는 것을 허용하는 것이고, 그러면 같은 노드에 두 플레이북이
// 붙는다. 러너는 고객 리눅스 서버에서 도는 것이 전제다.
func flock(*os.File) error {
	return errors.New("cannot take a run lock on this OS — the runner assumes a unix-like system")
}
