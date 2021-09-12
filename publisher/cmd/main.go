package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"sync"

	"github.com/btidor/src.codes/publisher"
	"github.com/btidor/src.codes/publisher/analysis"
	"github.com/btidor/src.codes/publisher/apt"
	"github.com/btidor/src.codes/publisher/database"
	"github.com/btidor/src.codes/publisher/upload"

	_ "net/http/pprof"

	"github.com/BurntSushi/toml"
)

const (
	configPath      string = "distributions.toml"
	pkgThreads      int    = 8
	uploadThreads   int    = 4
	downloadThreads int    = 16
	dbBatchSize     int    = 2048
	checkpointLimit int    = 32

	// When reindexPkgs mode is on, all packages are reprocessed and index files are
	// recomputed and reuploaded. To save on database reads, we do not run file
	// deduplication and no files from the archive are uploaded. (This has a
	// similar effect to bumping the epoch, but is intended for development.)
	reindexPkgs   bool = false
	reindexDistro bool = false
)

var db *database.Database
var up *upload.Uploader

type configEntry struct {
	Mirror     string
	Areas      []string
	Components []string
}

func main() {
	var err error

	// Get a database handle. Requires the DATABASE env var to contain a MySQL
	// connection string.
	var conn = os.Getenv("DATABASE")
	if conn == "" {
		err := fmt.Errorf("expected connection string in DATABASE")
		panic(err)
	}
	db, err = database.Connect(conn, dbBatchSize)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	fmt.Println("\u2713 Database")

	// Connect to Backblaze B2. Requires the env vars listed below to contain a
	// "keyId:applicationKey:bucketName" tuple.
	up, err = upload.NewUploader("B2_LS_KEY", "B2_CAT_KEY", "B2_META_KEY", downloadThreads)
	if err != nil {
		panic(err)
	}
	fmt.Println("\u2713 Backblaze")

	// Read config file from `./distributions.toml`
	var rawConfig map[string]configEntry
	_, err = toml.DecodeFile(configPath, &rawConfig)
	if err != nil {
		panic(err)
	}
	if len(rawConfig) == 0 {
		err = fmt.Errorf("config file is empty or failed to parse")
		panic(err)
	}
	var config []publisher.Distro
	for name, cfg := range rawConfig {
		u, err := url.Parse(cfg.Mirror)
		if err != nil {
			panic(err)
		}
		config = append(config, publisher.Distro{
			Name:       name,
			Mirror:     u,
			Areas:      cfg.Areas,
			Components: cfg.Components,
		})
	}
	fmt.Println("\u2713 Distro Config")

	// Start debug server
	// http://localhost:6060/debug/pprof/goroutine?debug=2
	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()
	fmt.Println("\u2713 Debug Server")
	fmt.Println()

	// Run!
	for _, distro := range config {
		processDistro(distro)
	}
}

func processDistro(distro publisher.Distro) {
	defer func() {
		if err := recover(); err != nil {
			// If we fail when processing one distro, log the error and
			// continue.
			fmt.Printf("\n***** PANIC in distro %s *****\n", distro.Name)
			fmt.Println(err)
			fmt.Println()
			fmt.Println(string(debug.Stack()))
			fmt.Println("*****************")
		}
	}()

	// Gather the list of packages to process
	var packages = make(map[string]apt.Package)
	for _, source := range apt.FetchSources(distro) {
		for _, pkg := range apt.FetchPackages(source) {
			// Sources are processed in order of priority, highest to lowest. So
			// if a package already in our map appears again, skip it.
			if _, found := packages[pkg.Name]; !found {
				packages[pkg.Name] = pkg
			}
		}
	}

	var existing = db.ListExistingPackages(distro.Name, packages)

	// Process packages in parallel
	var jobs = make(chan apt.Package)
	var results = make(chan database.PackageVersion, len(packages))
	var wg sync.WaitGroup
	for w := 0; w < pkgThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan apt.Package, wg *sync.WaitGroup) {
			defer wg.Done()
			for pkg := range jobs {
				results <- processPackage(pkg)
			}
		}(w, jobs, &wg)
	}

	var pkgvers []database.PackageVersion
	var count int = 0
	for _, pkg := range packages {
		count += 1
		ex, found := existing[pkg.Name]
		if found && ex.Version == pkg.Version && ex.Epoch >= publisher.Epoch && !reindexPkgs {
			// Package version has been processed on a previous run
			pkgvers = append(pkgvers, ex)
			fmt.Printf("[%s] Skip: % 5d / % 5d\n", distro.Name, count, len(packages))
		} else {
			// Package version is new, must be processed
			jobs <- pkg
			fmt.Printf("[%s] Feed: % 5d / % 5d\n", distro.Name, count, len(packages))
		}
	}
	close(jobs)
	wg.Wait()
	close(results)

	var processed = false
	for pv := range results {
		pkgvers = append(pkgvers, pv)
		processed = true
	}
	if !processed && !reindexDistro {
		// We didn't update any packages, so skip recomputing the indexes.
		fmt.Printf("[%s] No new packages, skipping index creation\n", distro.Name)
		return
	}

	fmt.Printf("[%s] Updating table of contents in DB\n", distro.Name)
	db.UpdateDistroContents(distro.Name, pkgvers)

	fmt.Printf("[%s] Preparing package list\n", distro.Name)
	pkgvers = db.ListDistroContents(distro.Name)
	up.UploadPackageList(distro.Name, pkgvers)

	// TODO: make this faster!
	fmt.Printf("[%s] Compiling consolidated fzf index\n", distro.Name)
	up.ConsolidateFzfIndex(distro.Name, pkgvers)

	fmt.Printf("[%s] Done!\n", distro.Name)
}

func processPackage(pkg apt.Package) database.PackageVersion {
	defer func() {
		if err := recover(); err != nil {
			// If we fail when processing one package, log the error and
			// continue.
			fmt.Printf("\n***** PANIC in package %s *****\n", pkg.Slug())
			fmt.Println(err)
			fmt.Println()
			fmt.Println(string(debug.Stack()))
			fmt.Println("*****************")
		}
	}()

	// TODO: switch to log for timestamps
	fmt.Printf("[%s] Begin download, extract + walk tree\n", pkg.Slug())

	var archive = analysis.DownloadExtractAndWalkTree(pkg)
	defer archive.CleanUp()

	fmt.Printf("[%s] Begin deduplicate + upload\n", pkg.Slug())

	var files []analysis.File
	if !reindexPkgs {
		files = db.DeduplicateFiles(archive.Tree.Files())
	}

	var wg sync.WaitGroup
	jobs := make(chan analysis.File)
	for w := 0; w < uploadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan analysis.File, wg *sync.WaitGroup) {
			defer wg.Done()

			var processed []analysis.File
			for file := range jobs {
				// TODO: dedupe against global map
				up.UploadFile(file)

				processed = append(processed, file)
				if len(processed) > checkpointLimit {
					db.RecordFiles(processed)
				}
				processed = []analysis.File{}
				// TODO: write into global map
			}
			// Record final files
			db.RecordFiles(processed)
		}(w, jobs, &wg)
	}

	for _, file := range files {
		jobs <- file
	}
	close(jobs)
	wg.Wait()

	fmt.Printf("[%s] Uploaded %d files; uploading tree\n", pkg.Slug(), len(files))
	up.UploadTree(archive)

	fmt.Printf("[%s] Computing and uploading fzf index\n", pkg.Slug())
	fzf := analysis.ConstructFzfIndex(archive)
	up.UploadFzfPackageIndex(*archive.Pkg, fzf)

	fmt.Printf("[%s] Recording package version in DB\n", pkg.Slug())
	var pv = db.RecordPackageVersion(archive)

	fmt.Printf("[%s] Done!\n", pkg.Slug())
	return pv
}
