//go:build !windows

package core

import "os/user"

// currentUsername returns the current Unix login name, or "" if it can't
// be determined. Split into its own file so that the Windows stub can
// provide a no-op without pulling in os/user on platforms where it is
// unnecessary here.
func currentUsername() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}
