// SPDX-License-Identifier: MIT

// Command ieds serves the demo substation's IEDs live on 127.0.0.1, one
// port each, from their CIDs: changing measurements, reports, controls
// (SBO with enhanced security, an interlocked disconnector), setting
// groups, a file store and an event journal. Ctrl-C stops it.
//
//	go run ./ieds
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/otfabric/iec61850-explorer/demo/internal/ied"
	"github.com/otfabric/iec61850-explorer/demo/internal/site"
)

func main() {
	host := flag.String("host", "127.0.0.1", "address to listen on")
	base := flag.Int("base-port", 10201, "port of the first IED; the others follow (feeder 1, feeder 2, transformer, busbar, merging unit)")
	exact := flag.Bool("exact", false, "serve feeder 1 exactly as its CID (by default it differs: firmware 2.4.1, I> 650 A in group 1)")
	verbose := flag.Bool("v", false, "log the servers' activity")
	flag.Parse()
	logger := slog.New(slog.DiscardHandler)
	if *verbose {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	order := []string{site.F1Name, site.F2Name, site.T1Name, site.BBName, site.MUName}
	var running []*ied.IED
	defer func() {
		for _, r := range running {
			r.Close()
		}
	}()
	fmt.Printf("AA1 Northfield 110/20 kV demo IEDs (Ctrl-C stops)\n\n")
	for k, name := range order {
		d := site.ByName(name)
		addr := fmt.Sprintf("%s:%d", *host, *base+k)
		r, err := ied.Start(d, addr, ied.Options{Live: true, Deviate: !*exact, Logger: logger})
		if err != nil {
			fmt.Fprintln(os.Stderr, "ieds:", err)
			os.Exit(1)
		}
		running = append(running, r)
		note := ""
		if name == site.F1Name && !*exact {
			note = "  (differs from its CID: see README)"
		}
		fmt.Printf("  %-12s %-21s %s%s\n", name, r.Addr, d.Desc, note)
	}
	fmt.Printf("\nCIDs: demo/scl/<name>.cid. In the Explorer, connect to each address (or open the SCD and use its addresses with these ports).\n")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	fmt.Println("stopping")
}
