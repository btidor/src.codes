package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kurin/blazer/b2"
)

type NodeType string

const (
	Directory NodeType = "directory"
	File      NodeType = "file"
)

type INode struct {
	Path      string
	Type      NodeType
	Size      uint64
	SHA256    string
	SymlinkTo string
}

type NNode struct {
	Name      string            `json:"name,omitempty"`
	Type      NodeType          `json:"type",omitempty`
	Contents  map[string]*NNode `json:"contents,omitempty"`
	Size      uint64            `json:"size,omitempty"`
	SHA256    string            `json:"sha256,omitempty"`
	SymlinkTo string            `json:"symlink_to,omitempty"`
}

func IndexDirectory(directory string) ([]*INode, error) {
	var index []*INode

	if !strings.HasSuffix(directory, "/") {
		directory += "/"
	}

	err := filepath.Walk(directory, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		node := INode{
			Path: strings.TrimPrefix(path, directory),
		}
		if info.Mode().Type()&fs.ModeSymlink != 0 {
			// Symbolic link
			dest, err := os.Readlink(path)
			if err != nil {
				return err
			}
			node.SymlinkTo = strings.TrimPrefix(dest, directory)
		} else {
			// Regular file or directory
			if info.IsDir() {
				node.Type = Directory
			} else {
				node.Type = File
				node.Size = uint64(info.Size())

				h := sha256.New()
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				if _, err := io.Copy(h, f); err != nil {
					f.Close()
					return err
				}
				f.Close()
				node.SHA256 = fmt.Sprintf("%x", h.Sum(nil))
			}
		}
		index = append(index, &node)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return index, nil
}

func UploadAndRecordFiles(deduped []*INode, directory string, controlHash string) {
	// TODO: set up prod buckets
	// TODO: handle concurrent threads uploading similar packages
	var wg sync.WaitGroup
	jobs := make(chan *INode)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan *INode, wg *sync.WaitGroup) {
			defer wg.Done()

			var processed []*INode
			for node := range jobs {
				f, err := os.Open(path.Join(directory, node.Path))
				if err != nil {
					panic(err)
				}

				casPath := path.Join(node.SHA256[0:2], node.SHA256[0:4], node.SHA256)
				obj := cat.Object(casPath)
				w := obj.NewWriter(ctx)
				if _, err := io.Copy(w, f); err != nil {
					w.Close()
					panic(err)
				}
				if err := w.Close(); err != nil {
					panic(err)
				}

				processed = append(processed, node)
				if len(processed) > batchSize {
					if err := RecordFiles(processed, controlHash); err != nil {
						panic(err)
					}
					fmt.Print("*")
					processed = make([]*INode, 0)
				}
			}

			// Final record
			if len(processed) > 0 {
				if err := RecordFiles(processed, controlHash); err != nil {
					panic(err)
				}
				fmt.Print("%")
			}
		}(w, jobs, &wg)
	}

	for _, node := range deduped {
		if node.Type != File || node.SymlinkTo != "" || node.SHA256 == "" {
			err := fmt.Errorf("UploadFiles received an invalid node: %#v", node)
			panic(err)
		}
		if node.Size == 0 && node.SHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
			err := fmt.Errorf("UploadFiles received an invalidly empty node: %#v", node)
			panic(err)
		}
		jobs <- node
	}

	close(jobs)
	wg.Wait()
}

func NestINodes(input []*INode) (*NNode, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("NestINodes requires at least 1 node")
	}
	rootI := input[0]
	if rootI.Path != "" || rootI.Type != Directory {
		return nil, fmt.Errorf("root node must come first\nroot: %#v", rootI)
	}

	root := NNode{
		Name:     "",
		Type:     Directory,
		Contents: make(map[string]*NNode),
	}
	dirLookup := make(map[string]*NNode)
	dirLookup[""] = &root

	for _, inode := range input[1:] {
		path := strings.TrimSuffix(inode.Path, "/")
		dir, file := filepath.Split(path)
		node := NNode{
			Name:      file,
			Type:      inode.Type,
			Size:      inode.Size,
			SHA256:    inode.SHA256,
			SymlinkTo: inode.SymlinkTo,
		}
		parent, found := dirLookup[dir]
		if !found {
			return nil, fmt.Errorf("parent directory not found for path %#v", inode.Path)
		}
		parent.Contents[file] = &node
		if inode.Type == Directory {
			node.Contents = make(map[string]*NNode)
			dirLookup[path+"/"] = &node
		}
	}
	return &root, nil
}

func UploadIndex(tree *NNode, pkg *APTPackage) (string, error) {
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return "", err
	}

	h := sha256.New()
	h.Write(data)
	slug := fmt.Sprintf("%x", h.Sum(nil))[:8]

	filename := fmt.Sprintf("%s_%s_%s.json", pkg.Name, pkg.Version, slug)
	path := path.Join(pkg.Distribution, filename)

	obj := ls.Object(path)
	w := obj.NewWriter(ctx)
	if _, err := w.Write(data); err != nil {
		w.Close()
		return "", err
	}
	return slug, w.Close()
}

type JSONPackage struct {
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

func UploadPackageList(pkgs *[]DBPackage, distribution string) error {
	list := make(map[string]JSONPackage)
	for _, p := range *pkgs {
		list[p.Name] = JSONPackage{
			Version: p.Version,
			Hash:    p.IndexSlug,
		}
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	path := path.Join(distribution, "packages.json")

	obj := ls.Object(path)
	opt := b2.WithAttrsOption(&b2.Attrs{
		Info: map[string]string{"b2-cache-control": "public, max-age=300"},
	})
	w := obj.NewWriter(ctx, opt)
	if _, err := w.Write(data); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
