package upload

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/btidor/src.codes/publisher"
	"github.com/btidor/src.codes/publisher/analysis"
	"github.com/btidor/src.codes/publisher/database"
	"github.com/kurin/blazer/b2"
)

const emptySHA string = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

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
	// TODO: proper serialization for INode
	data, err := json.MarshalIndent(a.Tree, "", "  ")
	if err != nil {
		panic(err)
	}

	filename := fmt.Sprintf(
		"%s_%s:%d.json", a.Pkg.Name, a.Pkg.Version, publisher.Epoch,
	)
	remote := path.Join(a.Pkg.Source.Distro, filename)

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

func (up *Uploader) UploadPackageList(distro string, pkgvers []database.PackageVersion) {
	// TODO: check JSON serialization
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
