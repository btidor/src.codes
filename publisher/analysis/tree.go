package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// An INode represents anything that can be contained in a directory: a file,
// another directory or a symbolic link.
type INode interface{ isAnINode() }

type Directory struct {
	Contents map[string]INode
}

func (d Directory) isAnINode() {}

// Files recursively enumerates the directory and returns a flattened list of
// its contents. Nodes are added in sorted order by name.
func (d Directory) Files() []File {
	var files []File

	// Enumerate files in sorted order
	var keys []string
	for name := range d.Contents {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	for _, k := range keys {
		switch node := d.Contents[k].(type) {
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

func (d Directory) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Type     string           `json:"type"`
		Contents map[string]INode `json:"contents"`
	}{
		Type:     "directory",
		Contents: d.Contents,
	})
}

type File struct {
	Size      int64
	SHA256    [32]byte
	LocalPath string
}

func (f File) isAnINode() {}

func (f File) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Type   string `json:"type"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	}{
		Type:   "file",
		Size:   f.Size,
		SHA256: hex.EncodeToString(f.SHA256[:]),
	})
}

type SymbolicLink struct {
	SymlinkTo string
	IsDir     bool
}

func (s SymbolicLink) isAnINode() {}

func (s SymbolicLink) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Type      string `json:"type"`
		SymlinkTo string `json:"symlink_to"`
		IsDir     bool   `json:"is_directory"`
	}{
		Type:      "symlink",
		SymlinkTo: s.SymlinkTo,
		IsDir:     s.IsDir,
	})
}

// constructTree walks a directory and produces a Directory object (with linked
// Files, Directories and SymbolicLinks) that represents the state of the
// filesystem.
//
// Note: this function computes the hash of every file it encounters which may
// cause churn if the filesystem is on a hard disk.
func constructTree(dir string) Directory {
	var root = Directory{
		Contents: make(map[string]INode),
	}

	dir = strings.TrimSuffix(dir, "/")

	var parents = make(map[string]*Directory)
	parents[dir] = &root

	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if dir == path {
			// skip root dir, we've already processed it
			return nil
		}

		var node INode
		var parentdir = strings.TrimSuffix(filepath.Dir(path), "/")
		if info.Mode().Type()&fs.ModeSymlink != 0 {
			dst, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// We ignore errors here, since some packages contain invalid links.
			// We'll treat these as file symlinks, not that it matters much.
			var isDir bool
			if dstInfo, err := os.Stat(path); err == nil {
				isDir = dstInfo.IsDir()
			}
			node = SymbolicLink{
				SymlinkTo: strings.TrimPrefix(dst, dir),
				IsDir:     isDir,
			}
		} else if info.IsDir() {
			var obj = Directory{
				Contents: make(map[string]INode),
			}
			parents[path] = &obj
			node = obj
		} else if info.Mode().IsRegular() {
			var obj = File{
				LocalPath: path,
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
		} else {
			if info.Mode()&fs.ModeNamedPipe != 0 {
				fmt.Printf("Skipping named pipe %#v\n", path)
			} else {
				err := fmt.Errorf("unknown special file at %#v", path)
				panic(err)
			}
			// To skip, don't add node to parent.Contents
			return nil
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
