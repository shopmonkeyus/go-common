package sys

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// CopyFile will copy src to dst
func CopyFile(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

// CopyDir will copy all files recursively from src to dst
func CopyDir(src string, dst string) error {
	var err error
	var fds []os.DirEntry
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return fmt.Errorf("error reading %s: %w", src, err)
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return fmt.Errorf("error mkdir %s: %w", dst, err)
	}

	if fds, err = os.ReadDir(src); err != nil {
		return fmt.Errorf("error readdir %s: %w", src, err)
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = CopyDir(srcfp, dstfp); err != nil {
				return fmt.Errorf("error copying directory from %s to %s: %w", srcfp, dstfp, err)
			}
		} else {
			if _, err = CopyFile(srcfp, dstfp); err != nil {
				return fmt.Errorf("error copying file from %s to %s: %w", srcfp, dstfp, err)
			}
		}
	}
	return nil
}

// Exists returns true if the filename or directory specified by fn exists.
func Exists(fn string) bool {
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return false
	}
	return true
}

// ListDir will return an array of files recursively walking into sub directories
func ListDir(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0)
	for _, file := range files {
		if file.IsDir() {
			newres, err := ListDir(filepath.Join(dir, file.Name()))
			if err != nil {
				return nil, err
			}
			res = append(res, newres...)
		} else {
			if file.Name() == ".DS_Store" {
				continue
			}
			res = append(res, filepath.Join(dir, file.Name()))
		}
	}
	return res, nil
}

// GzipFile compresses a file using gzip.
func GzipFile(filepath string) error {
	infile, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer infile.Close()

	outfile, err := os.Create(filepath + ".gz")
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer outfile.Close()

	zr := gzip.NewWriter(outfile)
	defer zr.Close()
	_, err = io.Copy(zr, infile)
	if err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	return nil
}
