package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	_ "net/http/pprof"

	_ "github.com/go-sql-driver/mysql"

	"github.com/BurntSushi/toml"
	"github.com/kurin/blazer/b2"
)

type Config struct {
	Distributions map[string]DistributionConfig
}

type DistributionConfig struct {
	Mirror     string
	Keyfile    string
	Areas      []string
	Components []string
}

var db *sql.DB
var cat *b2.Bucket
var ls *b2.Bucket
var ctx context.Context = context.Background()
var tempDir string = os.TempDir()

func main() {
	// Start debug server
	// http://localhost:6060/debug/pprof/goroutine?debug=2
	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()

	// Get a database handle.
	var err error
	conn := os.Getenv("DBCONN")
	if conn == "" {
		log.Fatal("Error: the DBCONN environment variable should contain a connection string!")
	}
	db, err = sql.Open("mysql", conn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	pingErr := db.Ping()
	if pingErr != nil {
		log.Fatal(pingErr)
	}
	fmt.Println("Connected!")

	if t := os.Getenv("SRCCODES_TEMPDIR"); t != "" {
		fmt.Printf("Using temporary directory: %v\n", t)
		tempDir = t
	}

	b2cat := strings.Split(os.Getenv("B2_CAT_CREDENTIALS"), ":")
	if len(b2cat) != 3 {
		log.Fatal("Error: could not find/parse B2_CAT_CREDENTIALS")
	}
	b2c, err := b2.NewClient(ctx, b2cat[0], b2cat[1], b2.UserAgent("src.codes"))
	if err != nil {
		log.Fatal(err)
	}
	if cat, err = b2c.Bucket(ctx, b2cat[2]); err != nil {
		log.Fatal(err)
	}

	b2ls := strings.Split(os.Getenv("B2_LS_CREDENTIALS"), ":")
	if len(b2cat) != 3 {
		log.Fatal("Error: could not find/parse B2_CAT_CREDENTIALS")
	}
	b2d, err := b2.NewClient(ctx, b2ls[0], b2ls[1], b2.UserAgent("src.codes"))
	if err != nil {
		log.Fatal(err)
	}
	if ls, err = b2d.Bucket(ctx, b2ls[2]); err != nil {
		log.Fatal(err)
	}

	rawConfig, err := os.ReadFile("config.toml")
	if err != nil {
		panic(err)
	}

	var config Config
	if _, err := toml.Decode(string(rawConfig), &config); err != nil {
		panic(err)
	}
	if len(config.Distributions) == 0 {
		log.Panicf("Failed to parse configuration: no distributions defined")
	}

	for name, options := range config.Distributions {
		// TODO: recover from panics
		HandleDistribution(name, options)
	}
}

func HandleDistribution(name string, options DistributionConfig) {
	apt := APTDistribution{name, options}
	sources, err := apt.GetSources()
	if err != nil {
		panic(err)
	}

	for _, source := range *sources {
		jobs := make(chan *APTPackage)
		var wg sync.WaitGroup
		for w := 0; w < 8; w++ {
			wg.Add(1)
			go func(w int, jobs <-chan *APTPackage, wg *sync.WaitGroup) {
				// TODO: recover from panics
				defer wg.Done()
				for pkg := range jobs {
					HandlePackage(pkg, &options)
				}
			}(w, jobs, &wg)
		}

		packages, err := source.GetPackages()
		if err != nil {
			panic(err)
		}

		filtered, err := DeduplicatePackages(packages)
		if err != nil {
			panic(err)
		}

		for i, pkg := range filtered {
			// TODO: figure out what's going on with mutable loop vars
			pkg2 := pkg
			jobs <- pkg2
			fmt.Printf("Feed: %06d / %06d\n", i, len(filtered))
		}
		close(jobs)
		wg.Wait()

		Finalize(packages, name)
	}
}

func HandlePackage(pkg *APTPackage, options *DistributionConfig) {
	fmt.Printf("^ %v\n", pkg.Name)

	archives, err := pkg.GetFiles(pkg.SourceExerpt, options.Mirror)
	if err != nil {
		panic(err)
	}
	directory, err := UnpackArchives(archives)
	defer os.RemoveAll(directory)
	if err != nil {
		panic(err)
	}

	index, err := IndexDirectory(directory)
	if err != nil {
		panic(err)
	}

	var files []*INode
	for _, node := range index {
		if node.Type == File && node.SymlinkTo == "" {
			files = append(files, node)
		}
	}

	deduped, err := DeduplicateFiles(files)
	if err != nil {
		panic(err)
	}

	UploadAndRecordFiles(deduped, directory, pkg.ControlHash)

	err = RecordFiles(files, pkg.ControlHash)
	if err != nil {
		panic(err)
	}

	fmt.Printf("$ %s: %d %d\n", pkg.Name, len(files), len(deduped))

	nestedIndex, err := NestINodes(index)
	if err != nil {
		panic(err)
	}

	indexSlug, err := UploadIndex(nestedIndex, pkg)
	if err != nil {
		panic(err)
	}

	if err := RecordPackage(pkg, indexSlug); err != nil {
		panic(err)
	}
}

func Finalize(aptpkg []*APTPackage, distribution string) {
	dbpkg, err := ListPackages(distribution)
	if err != nil {
		panic(err)
	}

	aptpkgl := make(map[string]*APTPackage)
	for _, ap := range aptpkg {
		aptpkgl[ap.Name] = ap
	}

	var remaining, delete []DBPackage
	for _, dp := range *dbpkg {
		if _, found := aptpkgl[dp.Name]; found {
			// Package exists in current index
			remaining = append(remaining, dp)
		} else {
			// Package has been removed by upstream
			delete = append(delete, dp)
		}
	}

	if err := DeletePackagesFromLatest(&delete, distribution); err != nil {
		panic(err)
	}

	if err := UploadPackageList(&remaining, distribution); err != nil {
		panic(err)
	}
}
