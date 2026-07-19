//go:build windows

package driftline

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func readRegularFileNoFollow(path string) ([]byte, os.FileMode, error) {
	pathPtr, err := syscall.UTF16PtrFromString(windowsExtendedPath(path))
	if err != nil {
		return nil, 0, &os.PathError{Op: "open", Path: path, Err: err}
	}
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, 0, &os.PathError{Op: "open", Path: path, Err: err}
	}

	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		syscall.CloseHandle(handle)
		return nil, 0, &os.PathError{Op: "open", Path: path, Err: syscall.EINVAL}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || attributes.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 || !info.Mode().IsRegular() {
		return nil, 0, errOpenedTargetNotRegular
	}
	data, err := io.ReadAll(file)
	return data, info.Mode().Perm(), err
}

func windowsExtendedPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) ||
		strings.HasPrefix(path, `\\.\`) ||
		strings.HasPrefix(path, `\??\`) ||
		!filepath.IsAbs(path) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}
