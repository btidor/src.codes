package main

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/btidor/src.codes/internal"
	"github.com/klauspost/compress/zstd"
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

	var mu sync.Mutex
	var errored bool // protected by `mu`

	for w := 0; w < downloadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan Package, wg *sync.WaitGroup) {
			defer wg.Done()
			for pkg := range jobs {
				if pkg.Update() {
					mu.Lock()
					errored = true
					mu.Unlock()
				}
			}
		}(w, jobs, &wg)
	}

	for _, p := range listPackages(distro) {
		jobs <- p
	}
	close(jobs)
	wg.Wait()

	// TODO: clean up excess files
	if errored {
		os.Exit(1)
	}
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

func (p Package) LocalDir(base string) string {
	prefix := string(p.Name[0])
	if strings.HasPrefix(p.Name, "lib") && len(p.Name) > 3 {
		prefix = p.Name[0:4]
	}
	return filepath.Join(base, "grep", p.Distro, prefix, p.Name)
}

func (p Package) Update() (errored bool) {
	defer func() {
		if err := recover(); err != nil {
			// If we fail when processing one distro, log the error and
			// continue.
			fmt.Printf("\n***** PANIC in package %s/%s *****\n", p.Distro, p.Name)
			fmt.Println(err)
			fmt.Println()
			fmt.Println(string(debug.Stack()))
			fmt.Println("*****************")
		}
		errored = true
	}()

	fmt.Printf("[%s] Downloading codesearch index\n", p.Slug())
	p.Download(fastDir, ".csi")
	p.Download(bulkDir, ".tar.zst")
	p.Decompress()
	return
}

func (p Package) Download(base, ext string) {
	if err := os.MkdirAll(p.LocalDir(base), 0755); err != nil {
		panic(err)
	}

	local := filepath.Join(p.LocalDir(base), p.Filename(ext))
	if _, err := os.Stat(local); err != nil {
		// File not yet downloaded
		remote := internal.URLWithPathForBackblaze(
			lsBase, p.Distro, p.Name, p.Filename(ext),
		)
		internal.SaveFile(local, remote)
	}
}

func (p Package) Decompress() {
	archive := filepath.Join(p.LocalDir(bulkDir), p.Filename(".tar.zst"))

	f, err := os.Open(archive)
	if err != nil {
		panic("couldn't open archive: " + archive)
	}
	defer f.Close()
	zr, err := zstd.NewReader(f)
	if err != nil {
		panic(err)
	}
	defer zr.Close()
	ar := tar.NewReader(zr)
	for {
		hdr, err := ar.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}
		filename := filepath.Join(p.LocalDir(bulkDir), strings.TrimPrefix(hdr.Name, p.Name))
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			panic(err)
		}
		out, err := os.Create(filename)
		if err != nil {
			panic(err)
		}
		if _, err := io.Copy(out, ar); err != nil {
			panic(err)
		}
		if err := out.Close(); err != nil {
			panic(err)
		}
	}
}
