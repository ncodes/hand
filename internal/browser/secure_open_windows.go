//go:build windows

package browser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func openSecureUpload(path string, maxBytes int64) (*os.File, int64, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return nil, 0, errors.New("browser upload source must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.EqualFold(filepath.Clean(resolved), path) {
		return nil, 0, errors.New("browser upload source must not traverse a symbolic link or junction")
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, 0, err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN,
		0,
	)
	if err != nil {
		return nil, 0, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, 0, err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		_ = windows.CloseHandle(handle)
		return nil, 0, errors.New("browser upload source must be a regular file without reparse points")
	}
	if info.NumberOfLinks != 1 {
		_ = windows.CloseHandle(handle)
		return nil, 0, errors.New("browser upload source must not be hard linked")
	}
	size := int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow)
	if size > maxBytes {
		_ = windows.CloseHandle(handle)
		return nil, 0, errors.New("browser upload exceeds the size limit")
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, 0, errors.New("browser upload source could not be opened")
	}

	return file, size, nil
}
