package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/btidor/src.codes/internal"
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

var knownExtns = []string{".csi", ".fzf", ".json", ".symbols", ".tags", ".zst"}

var db *database.Database
var up *upload.Uploader

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
	log.Println("\u2713 Database")

	// Connect to Backblaze B2. Requires the env vars listed below to contain a
	// "keyId:applicationKey:bucketName" tuple.
	up, err = upload.NewUploader("B2_LS_KEY", "B2_CAT_KEY", "B2_META_KEY", downloadThreads)
	if err != nil {
		panic(err)
	}
	log.Println("\u2713 Backblaze")

	// Read config file from `../distributions.toml`
	var rawConfig map[string]internal.ConfigEntry
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
	log.Println("\u2713 Distro Config")

	// Start debug server
	// http://localhost:6060/debug/pprof/goroutine?debug=2
	go func() {
		http.ListenAndServe("localhost:6060", nil)
	}()
	log.Println("\u2713 Debug Server")
	log.Println()

	// Run!
	if len(os.Args) > 1 && os.Args[1] == "prune" {
		knownDistros := make(map[string]bool)
		for _, distro := range config {
			knownDistros[distro.Name] = true
		}
		knownSuffixesMap := make(map[string]bool)
		for _, suff := range knownExtns {
			knownSuffixesMap[suff] = true
		}
		up.PruneLs(knownSuffixesMap, knownDistros)
		// TODO: also prune `cat` and `meta`
	} else {
		var errored = false
		for _, distro := range config {
			if processDistro(distro) {
				errored = true
			}
		}
		if errored {
			os.Exit(1)
		}
	}
}

func processDistro(distro publisher.Distro) (errored bool) {
	defer func() {
		if err := recover(); err != nil {
			// If we fail when processing one distro, log the error and
			// continue.
			log.Println()
			log.Printf("***** PANIC in distro %s *****\n", distro.Name)
			log.Println(err)
			log.Println()
			log.Println(string(debug.Stack()))
			log.Println("*****************")
			errored = true
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
				if pv, suberrored := processPackage(pkg); suberrored {
					errored = true
				} else {
					results <- pv
				}
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
		} else {
			// Package version is new, must be processed
			jobs <- pkg
			log.Printf("[%s] Feed: % 5d / % 5d\n", distro.Name, count, len(packages))
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
		log.Printf("[%s] No new packages, skipping index creation\n", distro.Name)
		return
	}

	log.Printf("[%s] Updating table of contents in DB\n", distro.Name)
	db.UpdateDistroContents(distro.Name, pkgvers)

	log.Printf("[%s] Preparing package list\n", distro.Name)
	pkgvers = db.ListDistroContents(distro.Name)
	up.UploadPackageList(distro.Name, pkgvers)

	log.Printf("[%s] Compiling consolidated fzf index\n", distro.Name)
	up.ConsolidateFzfIndex(distro.Name, pkgvers)

	log.Printf("[%s] Compiling consolidated symbols index\n", distro.Name)
	up.ConsolidateSymbolsIndex(distro.Name, pkgvers)

	log.Printf("[%s] Done!\n", distro.Name)
	return
}

func processPackage(pkg apt.Package) (_ database.PackageVersion, errored bool) {
	defer func() {
		if err := recover(); err != nil {
			// If we fail when processing one package, log the error and
			// continue.
			log.Println()
			log.Printf("***** PANIC in package %s *****\n", pkg.Slug())
			log.Println(err)
			log.Println()
			log.Println(string(debug.Stack()))
			log.Println("*****************")
			errored = true
		}
	}()

	log.Printf("[%s] Begin download, extract + walk tree\n", pkg.Slug())
	var archive = analysis.DownloadExtractAndWalkTree(pkg)
	defer archive.CleanUp()

	log.Printf("[%s] Begin deduplication\n", pkg.Slug())
	var files []analysis.File
	if !reindexPkgs {
		files = db.DeduplicateFiles(archive.Tree.Files())
	}

	log.Printf("[%s] Begin upload of %d files\n", pkg.Slug(), len(files))
	var count atomic.Int64
	var wg sync.WaitGroup
	jobs := make(chan analysis.File)
	for w := 0; w < uploadThreads; w++ {
		wg.Add(1)
		go func(w int, jobs <-chan analysis.File, wg *sync.WaitGroup) {
			defer wg.Done()

			var hashes [][32]byte
			for file := range jobs {
				up.UploadFile(file)

				hashes = append(hashes, file.SHA256)
				if len(hashes) >= checkpointLimit {
					progress := count.Add(int64(len(hashes)))
					log.Printf("[%s] Progress: %d / %d", pkg.Slug(), progress, len(files))
					db.RecordHashes(hashes)
					hashes = nil
				}
			}
			// Record final files
			db.RecordHashes(hashes)
		}(w, jobs, &wg)
	}

	for _, file := range files {
		jobs <- file
	}
	close(jobs)
	wg.Wait()

	log.Printf("[%s] Uploaded %d files; uploading tree\n", pkg.Slug(), len(files))
	up.UploadTree(archive)

	log.Printf("[%s] Computing and uploading fzf index\n", pkg.Slug())
	fzf := analysis.ConstructFzfIndex(archive)
	up.UploadFzfPackageIndex(*archive.Pkg, fzf)

	log.Printf("[%s] Computing and uploading ctags index\n", pkg.Slug())
	ctags := analysis.ConstructCtagsIndex(archive)
	up.UploadCtagsPackageIndex(*archive.Pkg, ctags)

	log.Printf("[%s] Computing and uploading symbols index\n", pkg.Slug())
	symbols := analysis.ConstructSymbolsIndex(archive, ctags)
	up.UploadSymbolsPackageIndex(*archive.Pkg, symbols)

	log.Printf("[%s] Computing and uploading codesearch index\n", pkg.Slug())
	codesearch, sourcetar := analysis.ConstructCodesearchIndex(archive)
	up.UploadCodesearchPackageIndex(*archive.Pkg, codesearch, sourcetar)

	log.Printf("[%s] Recording package version in DB\n", pkg.Slug())
	var pv = db.RecordPackageVersion(archive)

	log.Printf("[%s] Done!\n", pkg.Slug())
	return pv, false
}
