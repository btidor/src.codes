package apt

import (
	"bytes"
	"io"
	"strings"

	"github.com/btidor/src.codes/internal"
	"github.com/btidor/src.codes/publisher/control"

	"github.com/ulikunitz/xz"
)

type Package struct {
	Source    Source
	Name      string
	Version   string
	Files     []control.File
	Directory string
}

func (p Package) Slug() string {
	return p.Source.Slug() + "/" + p.Name
}

func FetchPackages(s Source) []Package {
	// Download
	buf1 := internal.DownloadFile(s.SourceIndex)
	r, err := xz.NewReader(buf1)
	if err != nil {
		panic(err)
	}

	// Unzip
	var buf2 bytes.Buffer
	if _, err := io.Copy(&buf2, r); err != nil {
		panic(err)
	}

	// Iterate
	var packages []Package
	for _, raw := range strings.Split(buf2.String(), "\n\n") {
		if len(raw) == 0 {
			// Extra whitespace, e.g at end of file
			continue
		}
		m, err := control.Parse(raw)
		if err != nil {
			panic(err)
		}

		packages = append(packages, Package{
			Source:    s,
			Name:      m.GetString("Package"),
			Version:   m.GetString("Version"),
			Files:     m.GetFiles("Files"),
			Directory: m.GetString("Directory"),
		})
	}
	return packages
}
