package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/btidor/src.codes/internal"
)

func update() {
	for distro := range distros {
		updateDistro(distro)
	}
	fmt.Printf("Done!\n")
}

func updateDistro(distro string) {
	// Download package files in parallel
	var wg sync.WaitGroup
	jobs := make(chan Package)

	for w := 0; w < downloadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan Package, wg *sync.WaitGroup) {
			defer wg.Done()
			for pkg := range jobs {
				fmt.Printf("[%s] Downloading codesearch index\n", pkg.Slug())
				pkg.Download(".csi")
				pkg.Download(".tar.zst")
			}
		}(w, jobs, &wg)
	}

	for _, p := range listPackages(distro) {
		jobs <- p
	}
	close(jobs)
	wg.Wait()

	// TODO: clean up excess files
}

func listPackages(distro string) []Package {
	u := internal.URLWithPathForBackblaze(
		metaBase, distro, "packages.json",
	)
	raw := internal.DownloadFile(u)
	data := make(map[string]struct {
		Version string `json:"version"`
		Epoch   int    `json:"epoch"`
	})
	err := json.Unmarshal(raw.Bytes(), &data)
	if err != nil {
		panic(err)
	}

	var packages []Package
	for name, info := range data {
		packages = append(packages, Package{
			Distro:  distro,
			Name:    name,
			Version: info.Version,
			Epoch:   info.Epoch,
		})
	}
	return packages
}

type Package struct {
	Distro  string
	Name    string
	Version string
	Epoch   int
}

func (p Package) Slug() string {
	return p.Distro + "/" + p.Name
}

func (p Package) Filename(ext string) string {
	return fmt.Sprintf("%s_%s:%d%s", p.Name, p.Version, p.Epoch, ext)
}

func (p Package) LocalDir() string {
	return filepath.Join(dataDir, "grep", p.Distro, p.Name)
}

func (p Package) Download(ext string) {
	if err := os.MkdirAll(p.LocalDir(), 0755); err != nil {
		panic(err)
	}

	local := filepath.Join(p.LocalDir(), p.Filename(ext))
	if _, err := os.Stat(local); err != nil {
		// File not yet downloaded
		remote := internal.URLWithPathForBackblaze(
			lsBase, p.Distro, p.Name, p.Filename(ext),
		)
		internal.SaveFile(local, remote)
	}
}
