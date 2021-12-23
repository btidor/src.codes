package upload

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/btidor/src.codes/internal"
	"github.com/btidor/src.codes/publisher"
	"github.com/btidor/src.codes/publisher/analysis"
	"github.com/btidor/src.codes/publisher/apt"
	"github.com/btidor/src.codes/publisher/database"
	"github.com/kurin/blazer/b2"
	"github.com/vmihailenco/msgpack/v5"
)

const emptySHA string = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

var lsBase = internal.URLMustParse("https://ls.src.codes")

type Uploader struct {
	ctx context.Context

	ls   *b2.Bucket
	cat  *b2.Bucket
	meta *b2.Bucket

	downloadThreads int
}

func NewUploader(lsvar, catvar, metavar string, downloadThreads int) (*Uploader, error) {
	var ctx = context.Background()

	ls, err := connect(ctx, lsvar)
	if err != nil {
		return nil, err
	}
	cat, err := connect(ctx, catvar)
	if err != nil {
		return nil, err
	}
	meta, err := connect(ctx, metavar)
	if err != nil {
		return nil, err
	}

	return &Uploader{
		ctx, ls, cat, meta, downloadThreads,
	}, nil
}

func connect(ctx context.Context, envvar string) (*b2.Bucket, error) {
	config := strings.Split(os.Getenv(envvar), ":")
	if len(config) != 3 {
		return nil, fmt.Errorf("could not find/parse %s", envvar)
	}

	client, err := b2.NewClient(ctx, config[0], config[1], b2.UserAgent("src.codes"))
	if err != nil {
		return nil, err
	}

	return client.Bucket(ctx, config[2])
}

func (up *Uploader) UploadFile(f analysis.File) {
	in := f.Open()
	defer in.Close()

	hex := hex.EncodeToString(f.SHA256[:])
	if f.Size == 0 && hex != emptySHA {
		err := fmt.Errorf("unexpected empty file %#v", f)
		panic(err)
	}
	remote := path.Join(hex[0:2], hex[0:4], hex)

	obj := up.cat.Object(remote)
	out := obj.NewWriter(up.ctx)
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		panic(err)
	}
	if err := out.Close(); err != nil {
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

	obj := up.ls.Object(remote)
	opts := b2.WithAttrsOption(&b2.Attrs{
		ContentType: "application/json",
		Info:        map[string]string{"b2-content-encoding": "gzip"},
	})
	out := obj.NewWriter(up.ctx, opts)
	if _, err := io.Copy(out, &in); err != nil {
		out.Close()
		panic(err)
	}
	if err = out.Close(); err != nil {
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

	obj := up.ls.Object(remote)
	out := obj.NewWriter(up.ctx)
	if _, err := out.Write(data); err != nil {
		out.Close()
		panic(err)
	}
	if err = out.Close(); err != nil {
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

	obj := up.meta.Object(remote)
	out := obj.NewWriter(up.ctx)
	if _, err := io.Copy(out, consolidated); err != nil {
		out.Close()
		panic(err)
	}
	if err := out.Close(); err != nil {
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

	obj := up.ls.Object(remote)
	opts := b2.WithAttrsOption(&b2.Attrs{
		ContentType: "text/plain",
		Info:        map[string]string{"b2-content-encoding": "gzip"},
	})
	out := obj.NewWriter(up.ctx, opts)
	if _, err := io.Copy(out, &in); err != nil {
		out.Close()
		panic(err)
	}
	if err = out.Close(); err != nil {
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

	obj := up.ls.Object(remote)
	opts := b2.WithAttrsOption(&b2.Attrs{
		ContentType: "text/plain",
		Info:        map[string]string{"b2-content-encoding": "gzip"},
	})
	out := obj.NewWriter(up.ctx, opts)
	if _, err := io.Copy(out, &in); err != nil {
		out.Close()
		panic(err)
	}
	if err = out.Close(); err != nil {
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
	obj := up.meta.Object(remote)
	opts := b2.WithAttrsOption(&b2.Attrs{
		ContentType: "text/plain",
		Info:        map[string]string{"b2-content-encoding": "gzip"},
	})
	out := obj.NewWriter(up.ctx, opts)
	if _, err := io.Copy(out, &in); err != nil {
		out.Close()
		panic(err)
	}
	if err := out.Close(); err != nil {
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

	obj := up.ls.Object(remote)
	opts := b2.WithAttrsOption(&b2.Attrs{
		Info: map[string]string{"b2-content-encoding": "gzip"},
	})
	out := obj.NewWriter(up.ctx, opts)
	if _, err := io.Copy(out, &in); err != nil {
		out.Close()
		panic(err)
	}
	if err := out.Close(); err != nil {
		panic(err)
	}

	filename = fmt.Sprintf(
		"%s_%s:%d.tar", pkg.Name, pkg.Version, publisher.Epoch,
	)
	obj = up.ls.Object(
		path.Join(pkg.Source.Distro, pkg.Name, filename),
	)
	out = obj.NewWriter(up.ctx)
	if _, err := out.Write(sourcetar); err != nil {
		out.Close()
		panic(err)
	}
	if err := out.Close(); err != nil {
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

	obj := up.meta.Object(remote)
	out := obj.NewWriter(up.ctx)
	if _, err := out.Write(data); err != nil {
		out.Close()
		panic(err)
	}
	if err := out.Close(); err != nil {
		panic(err)
	}
}

type pruningCategories struct {
	Corrupted        []*b2.Object
	UnknownDistro    []*b2.Object
	UnknownExtension []*b2.Object
	WrongEpoch       []*b2.Object
	Current          int
}

func (up *Uploader) PruneLs(knownSuffixes map[string]bool, knownDistros map[string]bool) {
	var p pruningCategories

	iter := up.ls.List(up.ctx)
	for iter.Next() {
		filename := iter.Object().Name()
		parts := strings.Split(filename, "/")
		if filename == "robots.txt" {
			// skip
		} else if len(parts) != 3 {
			p.Corrupted = append(p.Corrupted, iter.Object())
		} else if !knownDistros[parts[0]] {
			p.UnknownDistro = append(p.UnknownDistro, iter.Object())
		} else if !knownSuffixes[filepath.Ext(filename)] {
			p.UnknownExtension = append(p.UnknownExtension, iter.Object())
		} else if epochFromFilename(filename) != publisher.Epoch {
			p.WrongEpoch = append(p.WrongEpoch, iter.Object())
		} else {
			p.Current += 1
		}
	}
	if err := iter.Err(); err != nil {
		panic(err)
	}

	if ct := len(p.Corrupted); ct > 0 {
		fmt.Printf("Corrupted:\t% 6d\t%s...\n", ct, p.Corrupted[0].Name())
	}
	if len(p.UnknownDistro) > 0 {
		unknown := make(map[string]int)
		for _, file := range p.UnknownDistro {
			distro := strings.Split(file.Name(), "/")[0]
			unknown[distro] += 1
		}
		for distro, ct := range unknown {
			fmt.Printf("Unknown Distro:\t% 6d\t%s\n", ct, distro)
		}
	}
	if len(p.UnknownExtension) > 0 {
		unknown := make(map[string]int)
		for _, file := range p.UnknownExtension {
			ext := filepath.Ext(file.Name())
			unknown[ext] += 1
		}
		for ext, ct := range unknown {
			fmt.Printf("Unknown Extn:\t% 6d\t%s\n", ct, ext)
		}
	}
	if len(p.WrongEpoch) > 0 {
		wrong := make(map[int]int)
		for _, file := range p.WrongEpoch {
			epoch := epochFromFilename(file.Name())
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
	count := up.deleteFiles(p.Corrupted, 0, total)
	count = up.deleteFiles(p.UnknownDistro, count, total)
	count = up.deleteFiles(p.UnknownExtension, count, total)
	up.deleteFiles(p.WrongEpoch, count, total)
	fmt.Printf("Done!\n")
}

func epochFromFilename(filename string) int {
	parts := strings.Split(
		strings.TrimSuffix(filename, filepath.Ext(filename)),
		":",
	)
	epoch, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		panic(err)
	}
	return epoch
}

func (up *Uploader) deleteFiles(files []*b2.Object, start, total int) int {
	var jobs = make(chan *b2.Object)
	var mutex sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < up.downloadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan *b2.Object, wg *sync.WaitGroup) {
			defer wg.Done()
			for file := range jobs {
				if err := file.Delete(up.ctx); err != nil {
					panic(err)
				}
				mutex.Lock()
				start += 1
				if start%1000 == 0 {
					fmt.Printf("% 6d / % 6d\n", start, total)
				}
				mutex.Unlock()
			}
		}(w, jobs, &wg)
	}
	for _, file := range files {
		jobs <- file
	}
	close(jobs)
	wg.Wait()

	return start
}
