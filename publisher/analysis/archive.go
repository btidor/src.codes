// Package analysis provides tools for extracting, indexing and analyzing source
// code archives.
package analysis

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/btidor/src.codes/internal"
	"github.com/btidor/src.codes/publisher/apt"
)

// An Archive represents a package that's been extracted to the local
// filesystem.
type Archive struct {
	Pkg  *apt.Package // source package
	Dir  string       // local directory
	Tree Directory    // index of package contents

	// Because of how dpkg-extract works, we create a temporary directory and
	// the archive is extracted to a subdirectory (`Dir`). To make sure we clean
	// everything up, we need to remember the parent directory and remove it
	// when we're done.
	parent string
}

// CleanUp deletes the extracted archive from disk.
//
//   var a Archive = DownloadExtractAndWalkTree(...)
//   defer a.CleanUp()
//
func (a Archive) CleanUp() error {
	return os.RemoveAll(a.parent)
}

// DownloadExtractAndWalkTree creates an Archive from an apt.Package. It
// downloads the files listed in the package's control file, extracts and
// combines them using dpkg-source, and walks the resulting directory to create
// the index.
func DownloadExtractAndWalkTree(pkg apt.Package) Archive {
	// Create temporary directory
	tempdir, err := ioutil.TempDir("", "srccodes-"+pkg.Name)
	if err != nil {
		panic(err)
	}

	// Download the source package contents and identify the *.dsc
	var dsc string
	for _, file := range pkg.Files {
		var localName = filepath.Join(tempdir, filepath.Base(file.Name))
		var data = internal.DownloadFile(pkg.Source.DownloadBase, pkg.Directory, file.Name)
		err = ioutil.WriteFile(localName, data.Bytes(), 0644)
		if err != nil {
			panic(err)
		}

		if strings.HasSuffix(localName, ".dsc") {
			if dsc == "" {
				dsc = localName
			} else {
				err = fmt.Errorf("duplicate *.dsc files: %#v, %#v", dsc, localName)
				panic(err)
			}
		}
	}

	if dsc == "" {
		err = fmt.Errorf("source package is missing *.dsc")
		panic(err)
	}

	// Extract
	var extracted = path.Join(tempdir, "source")
	out, err := exec.Command("dpkg-source", "--extract", dsc, extracted).CombinedOutput()
	if err != nil {
		err := fmt.Errorf("dpkg-source failed: %#v\noutput: %s", err, string(out))
		panic(err)
	}

	// Walk, hash and construct tree
	var tree = constructTree(extracted)

	return Archive{
		Pkg:    &pkg,
		Dir:    extracted,
		Tree:   tree,
		parent: tempdir,
	}
}
