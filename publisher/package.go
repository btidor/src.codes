package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/ulikunitz/xz"
)

type APTDistribution struct {
	Name string
	DistributionConfig
}

type APTSource struct {
	Distribution string
	Area         string
	Component    string
	URL          *url.URL
	SHA256       string
}

type APTPackage struct {
	Distribution string
	Area         string
	Component    string
	Name         string
	Version      string
	SourceExerpt *ControlFile
	ControlHash  string
}

type APTArchives struct {
	Package     string
	Mirror      string
	Directory   string
	DSCContents string
	FileList    *ControlFileList
}

var dscRegexp = regexp.MustCompile(`\.dsc$`)

func (d *APTDistribution) GetSources() (*[]APTSource, error) {
	var result []APTSource
	for _, area := range d.Areas {
		slug := d.Name
		if area != "" {
			slug += "-" + area
		}

		url := ConstructURL(d.Mirror, "dists", slug, "InRelease")
		inrelease, err := DownloadAndVerifyArmored(url, d.Keyfile)
		if err != nil {
			return nil, err
		}

		control, err := ParseControlFile(inrelease)
		if err != nil {
			return nil, err
		}

		section := control.MustGetListEntry("SHA256")

		for _, component := range d.Components {
			file := section.MustGetFile(component, "source", "Sources.xz")
			if file.Size == 0 {
				log.Printf("%s/%s: Sources index has length zero, skipping", slug, component)
				break
			}
			url := ConstructURL(d.Mirror, "dists", slug, component, "source", "by-hash", "SHA256", file.Hash)
			result = append(result, APTSource{
				Distribution: d.Name,
				Area:         area,
				Component:    component,
				URL:          url,
				SHA256:       file.Hash,
			})
		}
	}
	return &result, nil
}

func (s *APTSource) GetPackages() ([]*APTPackage, error) {
	buf1, err := DownloadAndVerifySHA256(s.URL, s.SHA256)
	if err != nil {
		return nil, err
	}

	r, err := xz.NewReader(buf1)
	if err != nil {
		return nil, err
	}
	buf2 := bytes.Buffer{}
	if _, err := io.Copy(&buf2, r); err != nil {
		return nil, err
	}

	var result []*APTPackage
	for _, pkg := range strings.Split(buf2.String(), "\n\n") {
		if len(pkg) == 0 {
			// Extra whitespace, e.g at end of file
			continue
		}
		src, err := ParseControlFile(pkg)
		if err != nil {
			return nil, err
		}
		pkgName := src.MustGetStringEntry("Package")
		pkgVersion := src.MustGetStringEntry("Version")
		dsc, err := src.MustGetListEntry("Checksums-Sha256").FindFile(
			regexp.MustCompile(`\.dsc$`),
		)
		if err != nil {
			return nil, err
		}
		result = append(result, &APTPackage{
			Distribution: s.Distribution,
			Area:         s.Area,
			Component:    s.Component,
			Name:         pkgName,
			Version:      pkgVersion,
			SourceExerpt: src,
			ControlHash:  dsc.Hash,
		})
	}
	return result, nil
}

func (p *APTPackage) GetFiles(control *ControlFile, mirror string) (*APTArchives, error) {
	package1 := control.MustGetStringEntry("Package")
	directory := control.MustGetStringEntry("Directory")
	files1 := control.MustGetListEntry("Checksums-Sha256")
	dscInfo, err := files1.FindFile(dscRegexp)
	if err != nil {
		return nil, err
	}

	url := ConstructURL(mirror, directory, dscInfo.Name)
	buf, err := DownloadAndVerifySHA256(url, dscInfo.Hash)
	if err != nil {
		return nil, err
	}

	message, err := crypto.NewClearTextMessageFromArmored(buf.String())
	if err != nil {
		return nil, err
	}

	dsc, err := ParseControlFile(message.GetString())
	if err != nil {
		return nil, err
	}

	package2 := dsc.MustGetStringEntry("Source")
	if package1 != package2 {
		return nil, fmt.Errorf("package name mismatch: %#v / %#v", package1, package2)
	}

	// Some *.dsc's, like atmel-firmware, haven't been updated in a while and
	// don't contain a Checksums-Sha256 section. So we have to use the older
	// Files section, which contains MD5 hashes.
	return &APTArchives{
		Package:     package2,
		Mirror:      mirror,
		Directory:   directory,
		DSCContents: message.GetString(),
		FileList:    dsc.MustGetListEntry("Files"),
	}, nil
}

func UnpackArchives(archives *APTArchives) (string, error) {
	tempdir, err := ioutil.TempDir(tempDir, "srccodes-"+archives.Package)
	if err != nil {
		return "", err
	}
	archivedir := path.Join(tempdir, "archive")
	if err := os.Mkdir(archivedir, 0755); err != nil {
		return "", err
	}
	defer os.RemoveAll(archivedir)

	// Write out the *.dsc
	dscPath := path.Join(archivedir, archives.Package+".dsc")
	if err := ioutil.WriteFile(dscPath, []byte(archives.DSCContents), 0644); err != nil {
		return "", err
	}

	// Download all files listed in the *.dsc
	for _, file := range archives.FileList.Files {
		loc := path.Join(archivedir, file.Name)
		url := ConstructURL(archives.Mirror, archives.Directory, file.Name)
		if err := DownloadToFileAndMD5(url, loc, file.Hash); err != nil {
			return "", err
		}
	}

	// Extract!
	extractdir := path.Join(tempdir, "source")
	out, err := exec.Command("dpkg-source", "--extract", dscPath, extractdir).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("dpkg-source failed: %#v\noutput: %s", err, string(out))
	}
	return extractdir, nil
}
