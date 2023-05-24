package upload

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/btidor/src.codes/internal"
	"github.com/btidor/src.codes/publisher"
	"github.com/btidor/src.codes/publisher/analysis"
	"github.com/btidor/src.codes/publisher/apt"
	"github.com/btidor/src.codes/publisher/database"
	"github.com/minio/minio-go/v7"
	"github.com/vmihailenco/msgpack/v5"
)

const emptySHA string = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

var lsBase = internal.URLMustParse("https://ls.src.codes")

var epochExtractor = regexp.MustCompile(`:([0-9]+)(\.[a-z0-9]+)+$`)

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
	remote := path.Join(hex[0:2], hex[0:4], hex)

	if err := up.cat.PutFile(remote, f.LocalPath, "", ""); err != nil {
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

	if err := up.ls.PutBuffer(remote, &in, "application/json", "gzip"); err != nil {
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

	if err := up.ls.PutBytes(remote, data, "", ""); err != nil {
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
	jobs := make(chan *url.URL)
	results := make(chan *bytes.Buffer, 16)
	for w := 0; w < up.downloadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan *url.URL, wg *sync.WaitGroup) {
			defer wg.Done()
			for u := range jobs {
				fmt.Printf("Downloading %s\n", u.String())
				results <- internal.DownloadFile(u)
				fmt.Printf("  done %s\n", u.String())
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
		jobs <- internal.URLWithPathForBackblaze(
			lsBase, distro, pv.Name,
			fmt.Sprintf(
				"%s_%s:%d.fzf", pv.Name, pv.Version, pv.Epoch,
			),
		)
	}

	close(jobs)
	wg.Wait()
	close(results)
	wg2.Wait()

	remote := path.Join(distro, "paths.fzf")

	if err := up.meta.PutBuffer(remote, consolidated, "", ""); err != nil {
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

	if err := up.ls.PutBuffer(remote, &in, "text/plain", "gzip"); err != nil {
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

	if err := up.ls.PutBuffer(remote, &in, "text/plain", "gzip"); err != nil {
		panic(err)
	}
}

func (up *Uploader) ConsolidateSymbolsIndex(distro string, pkgvers []database.PackageVersion) {
	var pvLookup = make(map[string]int)
	for i, pv := range pkgvers {
		pvLookup[pv.Name] = i
	}

	var wg sync.WaitGroup
	jobs := make(chan *url.URL)
	results := make(chan *bytes.Buffer, 16)
	for w := 0; w < up.downloadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan *url.URL, wg *sync.WaitGroup) {
			defer wg.Done()
			for u := range jobs {
				fmt.Printf("Downloading %s\n", u.String())
				raw := internal.DownloadFile(u)
				results <- raw
				fmt.Printf("  done %s\n", u.String())
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
			_, err := io.Copy(zw, symbols)
			if err != nil {
				panic(err)
			}
		}
	}()

	for _, pv := range pkgvers {
		jobs <- internal.URLWithPathForBackblaze(
			lsBase, distro, pv.Name,
			fmt.Sprintf(
				"%s_%s:%d.symbols", pv.Name, pv.Version, pv.Epoch,
			),
		)
	}

	close(jobs)
	wg.Wait()
	close(results)
	wg2.Wait()

	if err := zw.Close(); err != nil {
		panic(err)
	}

	remote := path.Join(distro, "symbols.txt")
	if err := up.meta.PutBuffer(remote, &in, "text/plain", "gzip"); err != nil {
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

	if err := up.ls.PutBuffer(remote, &in, "", "gzip"); err != nil {
		panic(err)
	}

	filename = fmt.Sprintf(
		"%s_%s:%d.tar.zst", pkg.Name, pkg.Version, publisher.Epoch,
	)
	remote = path.Join(pkg.Source.Distro, pkg.Name, filename)
	if err := up.ls.PutBytes(remote, sourcetar, "", ""); err != nil {
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

	if err := up.meta.PutBytes(remote, data, "", ""); err != nil {
		panic(err)
	}
}

type pruningCategories struct {
	Corrupted        []*minio.ObjectInfo
	UnknownDistro    []*minio.ObjectInfo
	UnknownExtension []*minio.ObjectInfo
	WrongEpoch       []*minio.ObjectInfo
	Current          int
}

func (up *Uploader) PruneLs(knownSuffixes map[string]bool, knownDistros map[string]bool) {
	var p pruningCategories

	for obj := range up.ls.ListObjects() {
		if obj.Err != nil {
			panic(obj.Err)
		}

		parts := strings.Split(obj.Key, "/")
		if obj.Key == "robots.txt" {
			// skip
		} else if len(parts) != 3 {
			p.Corrupted = append(p.Corrupted, &obj)
		} else if !knownDistros[parts[0]] {
			p.UnknownDistro = append(p.UnknownDistro, &obj)
		} else if !knownSuffixes[filepath.Ext(obj.Key)] {
			p.UnknownExtension = append(p.UnknownExtension, &obj)
		} else if epochFromFilename(obj.Key) != publisher.Epoch {
			p.WrongEpoch = append(p.WrongEpoch, &obj)
		} else {
			p.Current += 1
		}
	}

	if ct := len(p.Corrupted); ct > 0 {
		fmt.Printf("Corrupted:\t% 6d\t%s...\n", ct, p.Corrupted[0].Key)
	}
	if len(p.UnknownDistro) > 0 {
		unknown := make(map[string]int)
		for _, obj := range p.UnknownDistro {
			distro := strings.Split(obj.Key, "/")[0]
			unknown[distro] += 1
		}
		for distro, ct := range unknown {
			fmt.Printf("Unknown Distro:\t% 6d\t%s\n", ct, distro)
		}
	}
	if len(p.UnknownExtension) > 0 {
		unknown := make(map[string]int)
		for _, obj := range p.UnknownExtension {
			ext := filepath.Ext(obj.Key)
			unknown[ext] += 1
		}
		for ext, ct := range unknown {
			fmt.Printf("Unknown Extn:\t% 6d\t%s\n", ct, ext)
		}
	}
	if len(p.WrongEpoch) > 0 {
		wrong := make(map[int]int)
		for _, obj := range p.WrongEpoch {
			epoch := epochFromFilename(obj.Key)
			wrong[epoch] += 1
		}
		for epoch, ct := range wrong {
			fmt.Printf("Wrong Epoch:\t% 6d\t%d\n", ct, epoch)
		}
	}
	fmt.Printf("\nTo Keep:\t% 6d\n\n", p.Current)

	total := len(p.Corrupted) + len(p.UnknownDistro) + len(p.UnknownExtension) + len(p.WrongEpoch)
	if total == 0 {
		fmt.Printf("Nothing to do!\n")
		return
	}
	fmt.Printf("Press ENTER to delete %d files!\n", total)
	bufio.NewReader(os.Stdin).ReadString('\n')

	fmt.Printf("Deleting...\n")
	count := up.ls.DeleteFiles(p.Corrupted, 0, total)
	count = up.ls.DeleteFiles(p.UnknownDistro, count, total)
	count = up.ls.DeleteFiles(p.UnknownExtension, count, total)
	up.ls.DeleteFiles(p.WrongEpoch, count, total)
	fmt.Printf("Done!\n")
}

func epochFromFilename(filename string) int {
	parts := epochExtractor.FindStringSubmatch(filename)
	if parts == nil || len(parts) < 2 {
		panic("could not match filename: " + filename)
	}
	epoch, err := strconv.Atoi(parts[1])
	if err != nil {
		panic(err)
	}
	return epoch
}
