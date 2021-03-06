package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/btidor/src.codes/internal"
)

var (
	lsBase              = internal.URLMustParse("https://ls.src.codes")
	metaBase            = internal.URLMustParse("https://meta.src.codes")
	downloadThreads int = 16
)

var commit string = "dev"

var configPath, bulkDir, fastDir, socket string
var distros = make(map[string]bool)

func main() {
	// Command-line interface
	var updateCmd = flag.NewFlagSet("update", flag.ExitOnError)
	var serveCmd = flag.NewFlagSet("serve", flag.ExitOnError)
	for _, fs := range []*flag.FlagSet{updateCmd, serveCmd} {
		// Common flags
		fs.StringVar(
			&configPath, "config", "distributions.toml",
			"Path to configuration file",
		)
		fs.StringVar(
			&bulkDir, "bulkData", "/data",
			"Path to directory for bulk data (HDD)",
		)
		fs.StringVar(
			&fastDir, "fastData", "/data",
			"Path to directory for fast data (SSD)",
		)
	}
	serveCmd.StringVar(
		&socket, "socket", "", "Unix domain socket to listen on",
	)

	var subcommand = ""
	if len(os.Args) > 1 {
		subcommand = os.Args[1]
	}
	switch subcommand {
	case "update":
		updateCmd.Parse(os.Args[2:])
	case "serve":
		serveCmd.Parse(os.Args[2:])
	default:
		fmt.Printf("usage: %s <command> [options]\n", os.Args[0])
		fmt.Printf("\nCommands: update, serve\n")
		fmt.Printf("\nOptions for update:\n")
		updateCmd.PrintDefaults()
		fmt.Printf("\nOptions for serve:\n")
		serveCmd.PrintDefaults()
		os.Exit(2)
	}

	// Read config file
	var rawConfig map[string]internal.ConfigEntry
	_, err := toml.DecodeFile(configPath, &rawConfig)
	if err != nil {
		panic(err)
	}
	if len(rawConfig) == 0 {
		err = fmt.Errorf("config file is empty or failed to parse")
		panic(err)
	}
	for name := range rawConfig {
		distros[name] = true
	}

	// Run command
	switch subcommand {
	case "update":
		update()
	case "serve":
		serve()
	default:
		panic("unknown subcommand")
	}
}

func init() {
	if len(commit) > 7 {
		commit = commit[:7]
	}
}
