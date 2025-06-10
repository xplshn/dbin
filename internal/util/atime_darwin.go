//go:build darwin

package util

import (
	"syscall"
	"time"
)

// ATime returns the access time stored in Stat_t on macOS.
func ATime(st *syscall.Stat_t) time.Time {
	return time.Unix(int64(st.Atimespec.Sec), int64(st.Atimespec.Nsec))
}
