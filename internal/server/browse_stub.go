//go:build !windows

package server

import "fmt"

func browseFolderNative() (string, error) {
	return "", fmt.Errorf("folder browser only available on Windows")
}
