package analysis

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
)

func ConstructLivegrepIndex(a Archive) []byte {
	conffile := writeConfig(a)
	defer os.Remove(conffile)

	index, err := ioutil.TempFile("", "lgi")
	if err != nil {
		panic(err)
	}
	index.Close()
	defer os.Remove(index.Name())

	cmd := exec.Command(
		"codesearch",
		"-index_only",
		"-dump_index", index.Name(),
		conffile,
	)
	_, err = cmd.Output()
	if err != nil {
		panic(err)
	}

	data, err := ioutil.ReadFile(index.Name())
	if err != nil {
		panic(err)
	}
	return data
}

func writeConfig(a Archive) string {
	var config = make(map[string]interface{})
	config["name"] = a.Pkg.Name
	config["fs_paths"] = []map[string]interface{}{
		{
			"name": a.Pkg.Name,
			"path": a.Dir,
		},
	}

	f, err := ioutil.TempFile("", "lgc")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		panic(err)
	}

	_, err = f.Write(data)
	if err != nil {
		panic(err)
	}

	return f.Name()
}
