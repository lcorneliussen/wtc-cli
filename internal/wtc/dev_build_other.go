//go:build !unix

package wtc

import "os"

func ownedByCurrentUser(os.FileInfo) bool { return false }
