package upload

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"sync"

	"github.com/btidor/src.codes/publisher"
	"github.com/btidor/src.codes/publisher/analysis"
	"github.com/btidor/src.codes/publisher/apt"
	"github.com/btidor/src.codes/publisher/database"
	"github.com/vmihailenco/msgpack/v5"
)

const emptySHA string = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type Uploader struct {
	ls   *Bucket
	cat  *Bucket
	meta *Bucket

	downloadThreads int
}

func NewUploader(lsvar, catvar, metavar string, downloadThreads int) (*Uploader, error) {
	ls, err := OpenBucket(lsvar)
	if err != nil {
		return nil, err
	}
	cat, err := OpenBucket(catvar)
	if err != nil {
		return nil, err
	}
	meta, err := OpenBucket(metavar)
	if err != nil {
		return nil, err
	}

	return &Uploader{
		ls, cat, meta, downloadThreads,
	}, nil
}

func (up *Uploader) UploadFile(f analysis.File) {
	hex := hex.EncodeToString(f.SHA256[:])
	if f.Size == 0 && hex != emptySHA {
		err := fmt.Errorf("unexpected empty file %#v", f)
		panic(err)
	}
	file, err := os.Open(f.LocalPath)
	if err != nil {
		panic(err)
	}
	remote := path.Join(hex[0:2], hex[0:4], hex)
	if err := up.cat.Put(remote, file, ""); err != nil {
		panic(err)
	}
}

func (up *Uploader) UploadTree(a analysis.Archive) {
	data, err := json.MarshalIndent(a.Tree, "", "  ")
	if err != nil {
		panic(err)
	}

	var in bytes.Buffer
	zw := gzip.NewWriter(&in)
	_, err = zw.Write(data)
	if err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}

	filename := fmt.Sprintf(
		"%s_%s:%d.json", a.Pkg.Name, a.Pkg.Version, publisher.Epoch,
	)
	remote := path.Join(a.Pkg.Source.Distro, a.Pkg.Name, filename)
	if err := up.ls.Put(remote, &in, "application/json"); err != nil {
		panic(err)
	}
}

func (up *Uploader) UploadFzfPackageIndex(pkg apt.Package, fzf analysis.Node) {
	data, err := msgpack.Marshal(fzf)
	if err != nil {
		panic(err)
	}

	filename := fmt.Sprintf(
		"%s_%s:%d.fzf", pkg.Name, pkg.Version, publisher.Epoch,
	)
	remote := path.Join(pkg.Source.Distro, pkg.Name, filename)
	if err := up.ls.Put(remote, bytes.NewBuffer(data), ""); err != nil {
		panic(err)
	}
}

func (up *Uploader) ConsolidateFzfIndex(distro string, pkgvers []database.PackageVersion) {
	// Download and concatenate indexes for each package
	var consolidated = new(bytes.Buffer)
	enc := msgpack.NewEncoder(consolidated)
	if err := enc.EncodeArrayLen(len(pkgvers)); err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	jobs := make(chan string)
	results := make(chan *bytes.Buffer, 16)
	for w := 0; w < up.downloadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan string, wg *sync.WaitGroup) {
			defer wg.Done()
			for path := range jobs {
				log.Printf("Downloading %s\n", path)
				data, err := up.ls.Get(path)
				if err != nil {
					panic(err)
				}
				results <- data
				log.Printf("  done %s\n", path)
			}
		}(w, jobs, &wg)
	}

	var wg2 sync.WaitGroup
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		for buf := range results {
			err := enc.EncodeBytes(buf.Bytes())
			if err != nil {
				panic(err)
			}
		}
	}()

	for _, pv := range pkgvers {
		jobs <- path.Join(distro, pv.Name, fmt.Sprintf(
			"%s_%s:%d.fzf", pv.Name, pv.Version, pv.Epoch,
		))
	}

	close(jobs)
	wg.Wait()
	close(results)
	wg2.Wait()

	remote := path.Join(distro, "paths.fzf")
	if err := up.meta.Put(remote, consolidated, ""); err != nil {
		panic(err)
	}
}

func (up *Uploader) UploadCtagsPackageIndex(pkg apt.Package, ctags []byte) {
	var in bytes.Buffer
	zw := gzip.NewWriter(&in)
	_, err := zw.Write(ctags)
	if err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}

	filename := fmt.Sprintf(
		"%s_%s:%d.tags", pkg.Name, pkg.Version, publisher.Epoch,
	)
	remote := path.Join(pkg.Source.Distro, pkg.Name, filename)
	if err := up.ls.Put(remote, &in, "text/plain"); err != nil {
		panic(err)
	}
}

func (up *Uploader) UploadSymbolsPackageIndex(pkg apt.Package, symbols []byte) {
	var in bytes.Buffer
	zw := gzip.NewWriter(&in)
	_, err := zw.Write(symbols)
	if err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}

	filename := fmt.Sprintf(
		"%s_%s:%d.symbols", pkg.Name, pkg.Version, publisher.Epoch,
	)
	remote := path.Join(pkg.Source.Distro, pkg.Name, filename)
	if err := up.ls.Put(remote, &in, "text/plain"); err != nil {
		panic(err)
	}
}

func (up *Uploader) ConsolidateSymbolsIndex(distro string, pkgvers []database.PackageVersion) {
	var pvLookup = make(map[string]int)
	for i, pv := range pkgvers {
		pvLookup[pv.Name] = i
	}

	var wg sync.WaitGroup
	jobs := make(chan string)
	results := make(chan bytes.Buffer, 16)
	for w := 0; w < up.downloadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan string, wg *sync.WaitGroup) {
			defer wg.Done()
			for path := range jobs {
				log.Printf("Downloading %s\n", path)
				data, err := up.ls.Get(path)
				if err != nil {
					panic(err)
				}
				rd, err := gzip.NewReader(data)
				if err != nil {
					panic(err)
				}
				var out bytes.Buffer
				io.Copy(&out, rd)
				results <- out
				log.Printf("  done %s\n", path)
			}
		}(w, jobs, &wg)
	}

	var in bytes.Buffer
	zw := gzip.NewWriter(&in)

	var wg2 sync.WaitGroup
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		for symbols := range results {
			_, err := io.Copy(zw, &symbols)
			if err != nil {
				panic(err)
			}
		}
	}()

	for _, pv := range pkgvers {
		jobs <- path.Join(distro, pv.Name, fmt.Sprintf(
			"%s_%s:%d.symbols", pv.Name, pv.Version, pv.Epoch,
		))
	}

	close(jobs)
	wg.Wait()
	close(results)
	wg2.Wait()

	if err := zw.Close(); err != nil {
		panic(err)
	}

	remote := path.Join(distro, "symbols.txt")
	if err := up.meta.Put(remote, &in, "text/plain"); err != nil {
		panic(err)
	}
}

func (up *Uploader) UploadCodesearchPackageIndex(pkg apt.Package, codesearch, sourcetar []byte) {
	var in bytes.Buffer
	zw := gzip.NewWriter(&in)
	_, err := zw.Write(codesearch)
	if err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}

	filename := fmt.Sprintf(
		"%s_%s:%d.csi", pkg.Name, pkg.Version, publisher.Epoch,
	)
	remote := path.Join(pkg.Source.Distro, pkg.Name, filename)
	if err := up.ls.Put(remote, &in, ""); err != nil {
		panic(err)
	}

	filename = fmt.Sprintf(
		"%s_%s:%d.tar.zst", pkg.Name, pkg.Version, publisher.Epoch,
	)
	remote = path.Join(pkg.Source.Distro, pkg.Name, filename)
	if err := up.ls.Put(remote, bytes.NewBuffer(sourcetar), ""); err != nil {
		panic(err)
	}
}

func (up *Uploader) UploadPackageList(distro string, pkgvers []database.PackageVersion) {
	var list = make(map[string]interface{})
	for _, pv := range pkgvers {
		list[pv.Name] = struct {
			Version string `json:"version"`
			Epoch   int    `json:"epoch"`
		}{
			Version: pv.Version,
			Epoch:   pv.Epoch,
		}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		panic(err)
	}
	remote := path.Join(distro, "packages.json")
	if err := up.meta.Put(remote, bytes.NewBuffer(data), ""); err != nil {
		panic(err)
	}
}
