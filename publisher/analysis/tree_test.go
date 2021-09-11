package analysis

import (
	"encoding/json"
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
