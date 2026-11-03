package storage

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

const openOp = "open"

// openAppend opens path for in-place appends. FILE_APPEND_DATA without
// FILE_WRITE_DATA makes every write land at end of file, and
// FILE_FLAG_OPEN_REPARSE_POINT opens a symlink/junction itself instead of its
// target. Omitting FILE_SHARE_DELETE pins the name to this file while open.
func openAppend(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: openOp, Path: path, Err: err}
	}
	handle, err := windows.CreateFile(name,
		windows.FILE_READ_DATA|windows.FILE_APPEND_DATA|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: openOp, Path: path, Err: err}
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, &os.PathError{Op: openOp, Path: path, Err: err}
	}
	fileType, err := windows.GetFileType(handle)
	if err != nil || fileType != windows.FILE_TYPE_DISK || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("output destination must be an existing writable regular file, not a symlink")
	}
	return os.NewFile(uintptr(handle), path), nil
}

func requireSingleLink(file *os.File, _ os.FileInfo) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks != 1 {
		return errors.New("output destination must not have hard links")
	}
	return nil
}
