package database

import (
	"fmt"

	"github.com/btidor/src.codes/publisher"
	"github.com/btidor/src.codes/publisher/analysis"
	"github.com/btidor/src.codes/publisher/apt"
)

type PackageVersion struct {
	ID      int64
	Name    string
	Version string
	Epoch   int
}

func (db *Database) RecordPackageVersion(a analysis.Archive) PackageVersion {
	_, err := db.Exec(
		"INSERT INTO package_versions (distro, pkg_name, pkg_version, sc_epoch)"+
			" VALUES ($1, $2, $3, $4)"+
			" ON CONFLICT (distro, pkg_name, pkg_version)"+
			" DO UPDATE SET sc_epoch = EXCLUDED.sc_epoch",
		a.Pkg.Source.Distro, a.Pkg.Name, a.Pkg.Version, publisher.Epoch,
	)
	if err != nil {
		panic(err)
	}

	// Normally we'd use LastInsertId(), but it's always returning zero.
	var id int64
	row := db.QueryRow(
		"SELECT id FROM package_versions WHERE"+
			" distro = $1 AND pkg_name = $2 AND pkg_version = $3",
		a.Pkg.Source.Distro, a.Pkg.Name, a.Pkg.Version,
	)
	if err := row.Scan(&id); err != nil {
		panic(err)
	}

	return PackageVersion{
		ID:      id,
		Name:    a.Pkg.Name,
		Version: a.Pkg.Version,
		Epoch:   publisher.Epoch,
	}
}

func (db *Database) ListExistingPackages(distro string, pkgs map[string]apt.Package) map[string]PackageVersion {
	var plist []apt.Package
	for _, pkg := range pkgs {
		plist = append(plist, pkg)
	}

	var existing = make(map[string]PackageVersion)
	for i := 0; i < len(plist); i += db.batchSize {
		var values []interface{}
		var query string = "SELECT id, pkg_name, pkg_version, sc_epoch" +
			" FROM package_versions" +
			" WHERE distro = $1 AND (pkg_name, pkg_version) IN ("
		var n int = 2
		values = append(values, distro)
		for j := i; j < i+db.batchSize && j < len(plist); j++ {
			values = append(values, plist[j].Name)
			values = append(values, plist[j].Version)
			query += fmt.Sprintf("($%d, $%d), ", n, n+1)
			n += 2
		}
		query = query[:len(query)-2] + ")"
		rows, err := db.Query(query, values...)
		if err != nil {
			panic(err)
		}

		for rows.Next() {
			pv := PackageVersion{}
			if err := rows.Scan(&pv.ID, &pv.Name, &pv.Version, &pv.Epoch); err != nil {
				panic(err)
			}
			existing[pv.Name] = pv
		}
	}
	return existing
}

func (db *Database) ListDistroContents(distro string) []PackageVersion {
	rows, err := db.Query(
		"SELECT pv.id, pv.pkg_name, pv.pkg_version, pv.sc_epoch"+
			" FROM distribution_contents dc"+
			" JOIN package_versions pv ON dc.current = pv.id"+
			" WHERE dc.distro = $1",
		distro,
	)
	if err != nil {
		panic(err)
	}

	var pvs []PackageVersion
	for rows.Next() {
		pv := PackageVersion{}
		if err := rows.Scan(&pv.ID, &pv.Name, &pv.Version, &pv.Epoch); err != nil {
			panic(err)
		}
		pvs = append(pvs, pv)
	}
	return pvs
}

func (db *Database) UpdateDistroContents(distro string, pvs []PackageVersion) {
	// Ensure all packages are present in database
	for i := 0; i < len(pvs); i += db.batchSize {
		var values []interface{}
		var query string = "REPLACE INTO distribution_contents" +
			" (distro, pkg_name, current) VALUES "
		var n int = 1
		for j := i; j < i+db.batchSize && j < len(pvs); j++ {
			values = append(values, distro)
			values = append(values, pvs[j].Name)
			values = append(values, pvs[j].ID)
			query += fmt.Sprintf("($%d, $%d, $%d), ", n, n+1, n+2)
			n += 3
		}
		query = query[:len(query)-2]
		_, err := db.Exec(query, values...)
		if err != nil {
			panic(err)
		}
	}

	// Find any packages not seen in this run and remove them from
	// distribution_contents.
	var seen = make(map[string]bool)
	for _, p := range pvs {
		seen[p.Name] = true
	}

	var toDelete []interface{}
	for _, q := range db.ListDistroContents(distro) {
		if _, found := seen[q.Name]; !found {
			toDelete = append(toDelete, q.ID)
		}
	}

	if len(toDelete) > 0 {
		var query string = "DELETE FROM distribution_contents WHERE ID IN ("
		for i := range toDelete {
			query += fmt.Sprintf("$%d, ", i+1)
		}
		query = query[:len(query)-2] + ")"
		_, err := db.Exec(query, toDelete...)
		if err != nil {
			panic(err)
		}
	}
}
