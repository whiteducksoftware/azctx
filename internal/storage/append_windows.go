package storage

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

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
