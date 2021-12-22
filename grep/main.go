package main

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/btidor/src.codes/internal"
)

var (
	lsBase              = internal.URLMustParse("https://ls.src.codes")
	metaBase            = internal.URLMustParse("https://meta.src.codes")
	configPath          = "../distributions.toml"
	localCache          = "/data/codesearch"
	downloadThreads int = 16
)

func main() {
	// Read config file from `../distributions.toml`
	var rawConfig map[string]internal.ConfigEntry
	_, err := toml.DecodeFile(configPath, &rawConfig)
	if err != nil {
		panic(err)
	}
	if len(rawConfig) == 0 {
		err = fmt.Errorf("config file is empty or failed to parse")
		panic(err)
	}
	var distros []string
	for name := range rawConfig {
		distros = append(distros, name)
	}

	for _, distro := range distros {
		updateDistro(distro)
	}
}
