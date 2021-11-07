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
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

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
		filename := fmt.Sprintf(
			"%s_%s:%d.fzf", pv.Name, pv.Version, pv.Epoch,
		)
		u := internal.URLWithPath(lsBase, distro, pv.Name, filename)
		// Backblaze incorrectly treats a "+" in the path component as a space,
		// so we need to escape this. Debugging FYI: Url.String() will ignore
		// RawPath unless it's a valid encoding of Path. See:
		// https://github.com/golang/go/issues/17340
		u.RawPath = strings.ReplaceAll(u.Path, "+", "%2B")
		jobs <- u
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

func (up *Uploader) ConsolidateCtagsIndex(distro string, pkgvers []database.PackageVersion) {
	var m = make(map[string]map[int]bool)

	var pvLookup = make(map[string]int)
	for i, pv := range pkgvers {
		pvLookup[pv.Name] = i
	}

	var wg sync.WaitGroup
	jobs := make(chan database.PackageVersion)
	results := make(chan string, 16)
	for w := 0; w < up.downloadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan database.PackageVersion, wg *sync.WaitGroup) {
			defer wg.Done()
			for pv := range jobs {
				filename := fmt.Sprintf(
					"%s_%s:%d.tags", pv.Name, pv.Version, pv.Epoch,
				)
				u := internal.URLWithPath(lsBase, distro, pv.Name, filename)
				// See comment in ConsolidateFzfIndex, above
				u.RawPath = strings.ReplaceAll(u.Path, "+", "%2B")

				fmt.Printf("Downloading %s\n", u.String())
				raw := internal.DownloadFile(u)
				scanner := bufio.NewScanner(raw)
				for scanner.Scan() {
					line := scanner.Text()
					// insert package name into path
					parts := strings.Split(line, "\t")
					results <- pv.Name + "," + parts[0]
				}
				fmt.Printf("  done %s\n", u.String())
			}
		}(w, jobs, &wg)
	}

	var wg2 sync.WaitGroup
	wg2.Add(1)
	go func() {
		defer wg2.Done()
		for line := range results {
			parts := strings.SplitN(line, ",", 2)
			pkg := parts[0]
			tag := parts[1]
			if _, ok := m[tag]; !ok {
				m[tag] = make(map[int]bool)
			}
			if pi, ok := pvLookup[pkg]; !ok {
				panic("Unknown package: " + pkg)
			} else {
				m[tag][pi] = true
			}
		}
	}()

	for _, pv := range pkgvers {
		jobs <- pv
	}

	close(jobs)
	wg.Wait()
	close(results)
	wg2.Wait()

	var n []string
	for tag := range m {
		if utf8.ValidString(tag) {
			n = append(n, tag)
		}
	}
	sort.Strings(n)

	var consolidated = new(bytes.Buffer)
	enc := msgpack.NewEncoder(consolidated)
	if err := enc.EncodeArrayLen(len(pvLookup)); err != nil {
		panic(err)
	}
	for pn, pi := range pvLookup {
		if err := enc.EncodeString(pn); err != nil {
			panic(err)
		}
		if err := enc.EncodeUint16(uint16(pi)); err != nil {
			panic(err)
		}
	}
	if err := enc.EncodeArrayLen(len(n)); err != nil {
		panic(err)
	}
	for _, tag := range n {
		if err := enc.EncodeString(tag); err != nil {
			panic(err)
		}
		var p []int
		for pi := range m[tag] {
			p = append(p, int(pi))
		}
		sort.Ints(p)
		if err := enc.EncodeArrayLen(len(p)); err != nil {
			panic(err)
		}
		for _, pi := range p {
			if err := enc.EncodeUint16(uint16(pi)); err != nil {
				panic(err)
			}
		}
	}

	remote := path.Join(distro, "tags.fzf")

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
