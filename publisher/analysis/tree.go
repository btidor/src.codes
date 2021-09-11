package analysis

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type INode interface {
	isAnINode()
}

type Directory struct {
	Contents map[string]INode
}

func (d Directory) isAnINode() {}

func (d Directory) Files() []File {
	var files []File
	for _, node := range d.Contents {
		switch node := node.(type) {
		case Directory:
			files = append(files, node.Files()...)
		case File:
			files = append(files, node)
		case SymbolicLink:
		default:
			err := fmt.Errorf("inode of unknown type: %#v", node)
			panic(err)
		}
	}
	return files
}

type File struct {
	Size   int64
	SHA256 [64]byte

	localPath string
}

func (f File) isAnINode() {}

func (f File) ShortHash() [8]byte {
	var res [8]byte
	copy(res[:], f.SHA256[:8])
	return res
}

func (f File) Open() *os.File {
	handle, err := os.Open(f.localPath)
	if err != nil {
		panic(err)
	}
	return handle
}

type SymbolicLink struct {
	SymlinkTo string
}

func (s SymbolicLink) isAnINode() {}

func constructTree(dir string) Directory {
	var root Directory

	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	var parents = make(map[string]*Directory)
	parents[dir] = &root

	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		var node INode
		parentdir, _ := filepath.Split(dir)
		if info.Mode().Type()&fs.ModeSymlink != 0 {
			dst, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// TODO: can we determine if the destination is a file/directory
			// based on Mode().Type()? If so, add to tree.
			node = SymbolicLink{
				SymlinkTo: strings.TrimPrefix(dst, dir),
			}
		} else if info.IsDir() {
			var obj = Directory{
				Contents: make(map[string]INode),
			}
			parents[path] = &obj
			node = obj
		} else if info.Mode().IsRegular() {
			var obj = File{
				localPath: strings.TrimPrefix(path, dir),
				Size:      info.Size(),
			}
			h := sha256.New()
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err = io.Copy(h, f); err != nil {
				f.Close()
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}
			copy(obj.SHA256[:], h.Sum(nil))
			node = obj
		} else if info.Mode()&os.ModeNamedPipe != 0 {
			fmt.Printf("Skipping named pipe %#v\n", path)
		} else {
			err := fmt.Errorf("unknown special file at %#v", path)
			panic(err)
		}
		parent, found := parents[parentdir]
		if !found {
			return fmt.Errorf("parent not found: %s", parentdir)
		}
		parent.Contents[info.Name()] = node
		return nil
	})
	if err != nil {
		panic(err)
	}
	return root
}
