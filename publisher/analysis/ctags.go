package analysis

import (
	"bufio"
	"bytes"
	"os/exec"
	"strings"
)

func ConstructCtagsIndex(a Archive) []byte {
	cmd := exec.Command(
		"ctags", "-f", "-", "--recurse", "--links=no", "--excmd=number",
		"--exclude=*.json",  // due to segfault on libcpanel-json-xs-perl test cases
		"--exclude=*.patch", // tags are unnecessary and garbled
		"--exclude=*.md",    // verbose + not useful (?)
	)
	cmd.Dir = a.Dir // paths are relative to this directory
	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	return out
}

func parseCtags(ctags []byte) map[string][]string {
	var result = make(map[string][]string)

	rd := bytes.NewReader(ctags)
	sc := bufio.NewScanner(rd)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 2)
		tag := parts[0]
		result[tag] = append(result[tag], parts[1])
	}
	return result
}
