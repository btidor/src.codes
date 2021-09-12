package upload

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/btidor/src.codes/fzf"
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
}

func NewUploader(lsvar, catvar, metavar string) (*Uploader, error) {
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
		ctx, ls, cat, meta,
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

	filename := fmt.Sprintf(
		"%s_%s:%d.json", a.Pkg.Name, a.Pkg.Version, publisher.Epoch,
	)
	remote := path.Join(a.Pkg.Source.Distro, a.Pkg.Name, filename)

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

func (up *Uploader) UploadFzfPackageIndex(pkg apt.Package, fzf fzf.Node) {
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
		buf := internal.DownloadFile(u)
		if err := enc.EncodeBytes(buf.Bytes()); err != nil {
			panic(err)
		}
	}

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
