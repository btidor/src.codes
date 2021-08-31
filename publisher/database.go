package main

import (
	"fmt"
	"strings"
)

const batchSize = 250

func DeduplicateFiles(files []*INode) ([]*INode, error) {
	var deduped []*INode
	for i := 0; i < len(files); i += batchSize {
		var hashes []interface{}
		for j := i; j < i+batchSize && j < len(files); j++ {
			hashes = append(hashes, files[j].SHA256)
		}

		rows, err := db.Query(
			"SELECT DISTINCT file_hash FROM files WHERE file_hash IN (?"+
				strings.Repeat(", ?", len(hashes)-1)+")",
			hashes...,
		)
		if err != nil {
			return nil, err
		}

		var hash string
		existing := make(map[string]bool, batchSize)
		for rows.Next() {
			if err := rows.Scan(&hash); err != nil {
				return nil, err
			}
			existing[hash] = true
		}

		for j := i; j < i+batchSize && j < len(files); j++ {
			h := files[j].SHA256
			_, found := existing[h]
			if !found {
				deduped = append(deduped, files[j])
			}
		}
	}
	return deduped, nil
}

func RecordFiles(files []*INode, controlHash string) error {
	// Remember to call with *all* files, not just deduped ones
	for i := 0; i < len(files); i += batchSize {
		var values []interface{}
		for j := i; j < i+batchSize && j < len(files); j++ {
			values = append(values, controlHash, files[j].Path, files[j].SHA256, files[j].Size)
		}
		ct := len(values) / 4

		_, err := db.Exec(
			"INSERT INTO files(control_hash, file_path, file_hash, file_size) VALUES (?, ?, ?, ?)"+
				strings.Repeat(", (?, ?, ?, ?)", ct-1)+
				" ON DUPLICATE KEY UPDATE control_hash = VALUES(control_hash)",
			values...,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func DeduplicatePackages(pkgs []*APTPackage) ([]*APTPackage, error) {
	var deduped []*APTPackage
	for i := 0; i < len(pkgs); i += batchSize {
		var hashes []interface{}
		for j := i; j < i+batchSize && j < len(pkgs); j++ {
			hashes = append(hashes, pkgs[j].ControlHash)
		}

		rows, err := db.Query(
			"SELECT DISTINCT control_hash FROM packages WHERE control_hash IN (?"+
				strings.Repeat(", ?", len(hashes)-1)+")",
			hashes...,
		)
		if err != nil {
			return nil, err
		}

		var hash string
		existing := make(map[string]bool, batchSize)
		for rows.Next() {
			if err := rows.Scan(&hash); err != nil {
				return nil, err
			}
			existing[hash] = true
		}

		for j := i; j < i+batchSize && j < len(pkgs); j++ {
			h := pkgs[j].ControlHash
			_, found := existing[h]
			if !found {
				deduped = append(deduped, pkgs[j])
			} else {
				fmt.Printf("Skipping package %s, already processed\n", pkgs[j].Name)
			}
		}
	}
	return deduped, nil

}

func RecordPackage(pkg *APTPackage, indexSlug string) error {
	_, err := db.Exec(
		"INSERT INTO packages(distribution, area, component, package_name, package_version, control_hash, index_slug)"+
			" VALUES (?, ?, ?, ?, ?, ?, ?)"+
			" ON DUPLICATE KEY UPDATE distribution = VALUES(distribution)",
		pkg.Distribution, pkg.Area, pkg.Component, pkg.Name, pkg.Version, pkg.ControlHash, indexSlug,
	)
	if err != nil {
		return err
	}

	_, err = db.Exec(
		"REPLACE INTO latest_packages(distribution, package_name, latest_version)"+
			" VALUES(?, ?, ?)",
		pkg.Distribution, pkg.Name, pkg.Version,
	)
	return err
}

type DBPackage struct {
	Name      string
	Version   string
	IndexSlug string
}

func ListPackages(distribution string) (*[]DBPackage, error) {
	rows, err := db.Query(
		"SELECT l.package_name, l.latest_version, p.index_slug"+
			" FROM latest_packages l"+
			" JOIN packages p"+
			" ON l.distribution = p.distribution"+
			" AND l.package_name = p.package_name"+
			" AND l.latest_version = p.package_version"+
			" WHERE l.distribution = ?",
		distribution,
	)
	if err != nil {
		return nil, err
	}

	var pkgs []DBPackage
	for rows.Next() {
		pkg := DBPackage{}
		if err := rows.Scan(&pkg.Name, &pkg.Version, &pkg.IndexSlug); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, pkg)
	}
	return &pkgs, nil
}

func DeletePackagesFromLatest(pkgs *[]DBPackage, distribution string) error {
	for i := 0; i < len(*pkgs); i += batchSize {
		var values []interface{}
		values = append(values, distribution)
		for j := i; j < i+batchSize && j < len(*pkgs); j++ {
			values = append(values, (*pkgs)[j].Name)
		}
		ct := len(values)

		_, err := db.Exec(
			"DELETE FROM latest_packages WHERE distribution = ? AND package_name IN (?"+
				strings.Repeat(", ?", ct-2)+")",
			values...,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
