package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const appendLimit = 8 << 20

// Test seams for filesystem races and write failures.
var (
	appendBeforeOpen = func() {}
	appendAfterOpen  = func() {}
	appendWrite      = func(file *os.File, data []byte) (int, error) { return file.Write(data) }
)

// Append adds LF-delimited data to an existing regular file outside Azure.
// It writes in place through one no-follow O_APPEND handle so the file keeps
// its inode, owner and mode: CI runners may read it as a different user (for
// example root container steps writing a host runner's GITHUB_ENV). Unlike
// Write it never creates, replaces or chmods the file or its parent.
func Append(path, azureDir string, data []byte) (err error) {
	if err := validate(path, azureDir); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 == 0 {
		return errors.New("output destination must be an existing writable regular file, not a symlink")
	}
	// Windows loads Lstat file IDs lazily by path; pin them before opening.
	_ = os.SameFile(info, info)
	appendBeforeOpen()
	file, err := openAppend(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close output destination: %w", closeErr)
		}
	}()
	appendAfterOpen()
	size, err := verifyAppendTarget(file, path, azureDir, info)
	if err != nil {
		return err
	}
	payload, err := separatedPayload(file, size, data)
	if err != nil {
		return err
	}
	return writePayload(file, payload)
}

// separatedPayload prefixes LF when the existing file lacks a final newline,
// so separator and data land in the same write.
func separatedPayload(file *os.File, size int64, data []byte) ([]byte, error) {
	if size == 0 {
		return data, nil
	}
	last := make([]byte, 1)
	if _, err := file.ReadAt(last, size-1); err != nil {
		return nil, fmt.Errorf("read output destination: %w", err)
	}
	if last[0] == '\n' {
		return data, nil
	}
	return append([]byte{'\n'}, data...), nil
}

func writePayload(file *os.File, payload []byte) error {
	written, err := appendWrite(file, payload)
	if written == len(payload) && err == nil {
		return nil
	}
	if err == nil {
		err = io.ErrShortWrite
	}
	if written > 0 {
		return fmt.Errorf("partial append: wrote %d of %d bytes: %w", written, len(payload), err)
	}
	return fmt.Errorf("append output destination: %w", err)
}

// verifyAppendTarget checks the opened handle is still the validated path:
// the same single-link regular file, outside Azure, within the size limit.
func verifyAppendTarget(file *os.File, path, azureDir string, info os.FileInfo) (int64, error) {
	opened, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return 0, errors.New("output destination changed before append")
	}
	if err := requireSingleLink(file, opened); err != nil {
		return 0, err
	}
	if err := validate(path, azureDir); err != nil {
		return 0, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !os.SameFile(opened, current) {
		return 0, errors.New("output destination changed while preparing append")
	}
	if opened.Size() > appendLimit {
		return 0, fmt.Errorf("output destination exceeds %d bytes", appendLimit)
	}
	return opened.Size(), nil
}
