package analysis

import (
	"os/exec"
)

func ConstructCtagsIndex(a Archive) []byte {
	out, err := exec.Command("ctags", "-f", "-", "--recurse", "--links=no", "--excmd=number", a.Dir).Output()
	if err != nil {
		panic(err)
	}
	return out
}
