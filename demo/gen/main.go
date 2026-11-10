// SPDX-License-Identifier: MIT

// Command gen writes the demo set: the SCD and one CID per IED (scl/),
// the vendor namespace (nsd/) and the captures (captures/). Its output
// is the same on every run.
//
//	go run ./gen -out .
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/otfabric/iec61850-explorer/demo/internal/scenario"
	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

func main() {
	out := flag.String("out", ".", "directory to write scl/, nsd/ and captures/ into")
	only := flag.String("only", "", "write only this capture (file name); empty: everything")
	flag.Parse()
	if err := run(*out, *only); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func write(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("%8d  %s\n", len(b), path)
	return nil
}

func run(out, only string) error {
	if only == "" {
		if err := write(filepath.Join(out, "scl", "AA1_Northfield.scd"), site.SCD()); err != nil {
			return err
		}
		for _, d := range site.ServedIEDs() {
			if err := write(filepath.Join(out, "scl", d.Name+".cid"), site.CID(d)); err != nil {
				return err
			}
		}
		if err := write(filepath.Join(out, "nsd", "corvane-7-4x-2026a.nsd"), site.VendorNSD()); err != nil {
			return err
		}
	}
	var names []string
	for _, d := range site.ServedIEDs() {
		names = append(names, d.Name)
	}
	for _, s := range scenario.All() {
		if only != "" && s.File != only {
			continue
		}
		b, err := scenario.Record(s, names...)
		if err != nil {
			return err
		}
		if err := write(filepath.Join(out, "captures", s.File), b); err != nil {
			return err
		}
	}
	return nil
}
