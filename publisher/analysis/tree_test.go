package analysis

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var file = File{
	Size: 123,
	SHA256: [32]byte{
		0x52, 0xfd, 0xfc, 0x07, 0x21, 0x82, 0x65, 0x4f, 0x16, 0x3f, 0x5f, 0x0f, 0x48, 0x7f, 0x69, 0x99,
		0x9a, 0x62, 0x1d, 0x72, 0x95, 0x66, 0xc7, 0x4d, 0x10, 0x03, 0x7c, 0x4d, 0x7b, 0xbb, 0x04, 0x07,
	},
}

func TestSerializeEmpty(t *testing.T) {
	var root = Directory{
		Contents: map[string]INode{},
	}
	actual, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	expected := `{
  "type": "directory",
  "contents": {}
}`
	if string(actual) != expected {
		t.Errorf("Incorrect JSON serialization\nGot %#v\nExp %#v", string(actual), expected)
	}
}

func TestSerializeFile(t *testing.T) {
	var root = Directory{
		Contents: map[string]INode{
			"file.txt": file,
		},
	}
	actual, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	expected := `{
  "type": "directory",
  "contents": {
    "file.txt": {
      "type": "file",
      "size": 123,
      "sha256": "52fdfc072182654f163f5f0f487f69999a621d729566c74d10037c4d7bbb0407"
    }
  }
}`
	if string(actual) != expected {
		t.Errorf("Incorrect JSON serialization\nGot %#v\nExp %#v", string(actual), expected)
	}
}

func TestSerializeSymbolicLink(t *testing.T) {
	var root = Directory{
		Contents: map[string]INode{
			"file.txt": file,
			"alias": SymbolicLink{
				SymlinkTo: "file.txt",
			},
		},
	}
	actual, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	expected := `{
  "type": "directory",
  "contents": {
    "alias": {
      "type": "symlink",
      "symlink_to": "file.txt"
    },
    "file.txt": {
      "type": "file",
      "size": 123,
      "sha256": "52fdfc072182654f163f5f0f487f69999a621d729566c74d10037c4d7bbb0407"
    }
  }
}`
	if string(actual) != expected {
		t.Errorf("Incorrect JSON serialization\nGot %#v\nExp %#v", string(actual), expected)
	}
}

func TestSerializeNested(t *testing.T) {
	var root = Directory{
		Contents: map[string]INode{
			"subdir1": Directory{
				Contents: map[string]INode{
					"alias": SymbolicLink{
						SymlinkTo: "../subdir2/file.txt",
					},
				},
			},
			"subdir2": Directory{
				Contents: map[string]INode{
					"file.txt": file,
				},
			},
		},
	}
	actual, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	expected := `{
  "type": "directory",
  "contents": {
    "subdir1": {
      "type": "directory",
      "contents": {
        "alias": {
          "type": "symlink",
          "symlink_to": "../subdir2/file.txt"
        }
      }
    },
    "subdir2": {
      "type": "directory",
      "contents": {
        "file.txt": {
          "type": "file",
          "size": 123,
          "sha256": "52fdfc072182654f163f5f0f487f69999a621d729566c74d10037c4d7bbb0407"
        }
      }
    }
  }
}`
	if string(actual) != expected {
		t.Errorf("Incorrect JSON serialization\nGot %#v\nExp %#v", string(actual), expected)
	}
}

func setUpDummyTree() (string, error) {
	tempdir, err := ioutil.TempDir("", "sctest")
	if err != nil {
		return "", err
	}
	fmt.Println(tempdir)
	//defer os.RemoveAll(tempdir)

	err = ioutil.WriteFile(
		filepath.Join(tempdir, "hello.txt"),
		[]byte("Hello, World!\n"), // c98c24b677eff44860afea6f493bbaec5bb1c4cbb209c6fc2bbb47f66ff2ad31
		0644,
	)
	if err != nil {
		return "", err
	}

	err = os.Mkdir(filepath.Join(tempdir, "somedir"), 0755)
	if err != nil {
		return "", err
	}

	err = ioutil.WriteFile(
		filepath.Join(tempdir, "somedir", "foo.bar"),
		[]byte("Buzz\n"), // 49753fbc6dd206f47e0db4841da0a7c9b5150e75334121b3085fb994f1d3e192
		0644,
	)
	if err != nil {
		return "", err
	}

	err = os.Symlink(
		"../hello.txt",
		filepath.Join(tempdir, "somedir", "somelink"),
	)
	if err != nil {
		return "", err
	}
	return tempdir, nil
}

func TestFiles(t *testing.T) {
	tempdir, err := setUpDummyTree()
	if err != nil {
		t.Fatal(err)
	}

	var files = constructTree(tempdir).Files()
	if len(files) != 2 {
		t.Errorf("Wrong number of files: %#v", files)
	}

	if !strings.HasSuffix(files[0].localPath, "/hello.txt") {
		t.Errorf("Incorrect first file: %#v", files[0])
	}
	if !strings.HasSuffix(files[1].localPath, "/somedir/foo.bar") {
		t.Errorf("Incorrect second file: %#v", files[1])
	}
}

func TestOpen(t *testing.T) {
	tempdir, err := setUpDummyTree()
	if err != nil {
		t.Fatal(err)
	}

	var file = constructTree(tempdir).Contents["hello.txt"].(File)
	var handle = file.Open()
	defer handle.Close()

	var buf strings.Builder
	_, err = io.Copy(&buf, handle)
	if err != nil {
		t.Error(err)
	}

	if buf.String() != "Hello, World!\n" {
		t.Errorf("Unexpected file contents: %#v", buf.String())
	}
}

func TestConstructTree(t *testing.T) {
	tempdir, err := setUpDummyTree()
	if err != nil {
		t.Fatal(err)
	}

	var tree = constructTree(tempdir)
	if len(tree.Contents) != 2 {
		t.Errorf("Wrong number of elements in root: %#v", tree)
	}

	f1 := tree.Contents["hello.txt"].(File)
	if f1.Size != 14 {
		t.Errorf("File has incorrect size: %#v", f1)
	}
	if hex.EncodeToString(f1.SHA256[:]) != "c98c24b677eff44860afea6f493bbaec5bb1c4cbb209c6fc2bbb47f66ff2ad31" {
		t.Errorf("File has incorrect hash: %#v", f1)
	}

	dir := tree.Contents["somedir"].(Directory)
	if len(dir.Contents) != 2 {
		t.Errorf("Wrong number of elements in somedir: %#v", dir)
	}

	f2 := dir.Contents["foo.bar"].(File)
	if f2.Size != 5 {
		t.Errorf("File has incorrect size: %#v", f2)
	}
	if hex.EncodeToString(f2.SHA256[:]) != "49753fbc6dd206f47e0db4841da0a7c9b5150e75334121b3085fb994f1d3e192" {
		t.Errorf("File has incorrect hash: %#v", f2)
	}

	sym := dir.Contents["somelink"].(SymbolicLink)
	if sym.SymlinkTo != "../hello.txt" {
		t.Errorf("Symlink has wrong destination: %#v", sym)
	}
}
