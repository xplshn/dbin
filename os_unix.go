//go:build !darwin

package main

import (
	"syscall"
	"time"
)

// ATime returns the access time stored in Stat_t on Linux/BSD.
func ATime(st *syscall.Stat_t) time.Time {
	return time.Unix(int64(st.Atim.Sec), int64(st.Atim.Nsec))
}
