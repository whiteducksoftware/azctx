package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// Append adds LF-delimited data to an existing regular file outside Azure.
// Replacement avoids in-place writes through hard links. Unlike Write it does
// not create or chmod the parent (pipeline directories are not azctx-owned).
func Append(path, azureDir string, data []byte) error {
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
	existing, err := readAppendFile(path, info)
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) {
		return errors.New("output destination changed while preparing append")
	}
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		existing = append(existing, '\n')
	}
	return replace(path, azureDir, append(existing, data...))
}

func readAppendFile(path string, info os.FileInfo) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, errors.New("output destination changed before append")
	}
	if err := requireSingleLink(file, opened); err != nil {
		return nil, err
	}
	const limit = 8 << 20
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("output destination exceeds %d bytes", limit)
	}
	return data, nil
}
