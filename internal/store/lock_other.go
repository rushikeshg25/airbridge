//go:build !linux && !darwin

package store

import (
	"errors"
	"os"
)

func lockDirectory(dir string) (*os.File, error) {
	return nil, errors.New("hosting currently supports macOS and Linux; other devices can join in a browser")
}
