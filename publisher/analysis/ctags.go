package analysis

import (
	"os/exec"
)

func ConstructCtagsIndex(a Archive) []byte {
	cmd := exec.Command("ctags", "-f", "-", "--recurse", "--links=no", "--excmd=number")
	cmd.Dir = a.Dir // paths are relative to this directory
	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	return out
}