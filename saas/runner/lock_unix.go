//go:build unix

package runner

import (
	"os"
	"syscall"
)

// flock — 배타 잠금을 **기다리지 않고** 시도한다. 표준 라이브러리라 새 의존이 붙지 않는다.
func flock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == syscall.EWOULDBLOCK {
		return ErrAlreadyRunning
	}
	return err
}
