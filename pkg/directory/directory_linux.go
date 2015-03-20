// +build linux

package directory

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"syscall"
)

// Size walks a directory tree and returns its total size in bytes.
func Size(dir string) (size int64, err error) {
	data := make(map[uint64]struct{})
	err = filepath.Walk(dir, func(d string, fileInfo os.FileInfo, e error) error {
		// Ignore directory sizes
		if fileInfo == nil {
			return nil
		}

		s := fileInfo.Size()
		if fileInfo.IsDir() || s == 0 {
			return nil
		}

		// Check inode to handle hard links correctly
		inode := fileInfo.Sys().(*syscall.Stat_t).Ino
		// inode is not a uint64 on all platforms. Cast it to avoid issues.
		if _, exists := data[uint64(inode)]; exists {
			return nil
		}
		// inode is not a uint64 on all platforms. Cast it to avoid issues.
		data[uint64(inode)] = struct{}{}

		size += s

		return nil
	})
	return
}

// Move all contents of a directory to a subdirectory underneath the original path
func MoveDirToSubdir(oldpath, subdir string) error {

	infos, err := ioutil.ReadDir(oldpath)
	if err != nil {
		return err
	}
	for _, info := range infos {
		if info.Name() != subdir {
			oldName := path.Join(oldpath, info.Name())
			newName := path.Join(oldpath, subdir, info.Name())
			if err := os.Rename(oldName, newName); err != nil {
				return err
			}
		}
	}
	return nil
}
