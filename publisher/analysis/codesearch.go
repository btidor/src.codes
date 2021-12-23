package analysis

import (
	"archive/tar"
	"bytes"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/codesearch/index"
)

const (
	// To determine if a file is binary, check if the first <window> (1K) bytes
	// contain a null byte.
	binarySniffingWindow = 1024

	// Files over <size> (1M) bytes are assumed not to be code and are skipped.
	largeFileSize = 1024 * 1024
)

func ConstructCodesearchIndex(a Archive) ([]byte, []byte) {
	container, err := ioutil.TempDir("", "srccodes-cs-"+a.Pkg.Name)
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(container)

	cs := filepath.Join(container, "codesearch")
	ix := index.Create(cs)
	ix.AddPaths([]string{
		a.Pkg.Name,
	})

	an := filepath.Join(container, "tar")
	af, err := os.Create(an)
	if err != nil {
		panic(err)
	}
	ar := tar.NewWriter(af)

	win := make([]byte, binarySniffingWindow)

	dir := strings.TrimSuffix(a.Dir, "/")
	err = filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		} else if !info.Mode().IsRegular() {
			return nil
		}

		if info.Size() > largeFileSize {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		defer file.Close()

		// Skip binary files (see binarySniffingWindow, above)
		n, err := file.Read(win)
		if err != nil && err != io.EOF {
			panic(err)
		}
		if bytes.Contains(win[:n], []byte("\x00")) {
			return nil
		}

		// Reset cursor to start of file
		if _, err = file.Seek(0, 0); err != nil {
			panic(err)
		}

		// Add to tar archive
		name := filepath.Join(a.Pkg.Name, strings.TrimPrefix(path, dir))
		hdr := &tar.Header{
			Name: name,
			Mode: int64(info.Mode()),
			Size: info.Size(),
		}
		if err := ar.WriteHeader(hdr); err != nil {
			panic(err)
		}
		if _, err := io.Copy(ar, file); err != nil {
			panic(err)
		}

		// Reset cursor to start of file
		if _, err = file.Seek(0, 0); err != nil {
			panic(err)
		}

		// Add to codesearch index
		ix.Add(name, file)
		return nil
	})
	if err != nil {
		panic(err)
	}

	ix.Flush()

	if err := ar.Close(); err != nil {
		panic(err)
	}
	if err := af.Close(); err != nil {
		panic(err)
	}

	buf1, err := ioutil.ReadFile(cs)
	if err != nil {
		panic(err)
	}
	buf2, err := ioutil.ReadFile(an)
	if err != nil {
		panic(err)
	}
	return buf1, buf2
}
