//go:build !windows

package journal

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// intentarCerrojo pide el cerrojo exclusivo sin bloquear. El segundo valor
// distingue "lo tiene otro" (ok=false, sin error) de "algo salió mal".
func intentarCerrojo(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, unix.EWOULDBLOCK):
		return false, nil
	default:
		return false, err
	}
}

func soltarCerrojo(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
