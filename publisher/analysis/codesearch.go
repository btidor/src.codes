package analysis

import (
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/codesearch/index"
)

func ConstructCodesearchIndex(a Archive) []byte {
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

	dir := strings.TrimSuffix(a.Dir, "/")
	err = filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		} else if !info.Mode().IsRegular() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		ix.Add(
			filepath.Join(a.Pkg.Name, strings.TrimPrefix(path, dir)),
			file,
		)
		return nil
	})
	if err != nil {
		panic(err)
	}
	ix.Flush()

	buf, err := ioutil.ReadFile(cs)
	if err != nil {
		panic(err)
	}
	return buf
}
