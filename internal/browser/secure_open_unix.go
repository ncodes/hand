//go:build !windows

package browser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func openSecureUpload(path string, maxBytes int64) (*os.File, int64, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return nil, 0, errors.New("browser upload source must be absolute")
	}
	root := string(filepath.Separator)
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, 0, err
	}
	parts := strings.Split(strings.TrimPrefix(path, root), string(filepath.Separator))
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			_ = unix.Close(fd)
			return nil, 0, errors.New("browser upload source path is invalid")
		}
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if index < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(fd, part, flags, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return nil, 0, openErr
		}
		fd = next
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, 0, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = unix.Close(fd)
		return nil, 0, errors.New("browser upload source must be a regular file")
	}
	if stat.Nlink != 1 {
		_ = unix.Close(fd)
		return nil, 0, errors.New("browser upload source must not be hard linked")
	}
	if stat.Size > maxBytes {
		_ = unix.Close(fd)
		return nil, 0, errors.New("browser upload exceeds the size limit")
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, 0, errors.New("browser upload source could not be opened")
	}

	return file, stat.Size, nil
}
