// SPDX-License-Identifier: MIT

// Command iec61850-explorer is the OT Fabric IEC 61850 Explorer desktop
// application: a tool to discover an IED, browse its IEC 61850 model and read
// its values. In M0 it works in Observe mode only: no MMS service that
// changes the IED's state can be sent.
//
// The engine runs in this process (Local mode); there is no other mode. To
// reach an IED behind a gateway, run iec61850-explorerd there and open its
// web UI in a browser through an SSH port forward.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/otfabric/iec61850-explorer/frontend"
	"github.com/otfabric/iec61850-explorer/internal/buildinfo"
	"github.com/otfabric/iec61850-explorer/internal/daemon"
	"github.com/otfabric/iec61850-explorer/internal/engine"
	"github.com/otfabric/iec61850-explorer/internal/gui"
)

const appTitle = "IEC 61850 Explorer"

// appIcon is the IEC 61850 icon of the OT Fabric website (docs/BRANDING.md,
// section 4): each OT Fabric GUI carries the icon of its protocol. It is the
// window and dock icon; the macOS bundle derives its .icns from the same
// file.
//
//go:embed appicon.png
var appIcon []byte

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "iec61850-explorer:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("iec61850-explorer", flag.ContinueOnError)
	version := flags.Bool("version", false, "print the version and exit")
	debug := flags.Bool("debug", false, "log at debug level")
	dataDir := flags.String("data-dir", defaultDataDir(), "directory for the connection profiles")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *version {
		fmt.Printf("iec61850-explorer %s (go-iec61850 %s, go-mms %s)\n", buildinfo.Version,
			buildinfo.ModuleVersion(buildinfo.GoIEC61850Path), buildinfo.ModuleVersion(buildinfo.GoMMSPath))
		return nil
	}

	assets := frontend.Wails()
	if !frontend.Built(assets) {
		return fmt.Errorf("this binary was built without the user interface: run \"make frontend\" and build again")
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// The desktop application connects wherever its user can: it has no
	// target allow-list (that protects a shared gateway, see the daemon).
	// It offers Commission mode (ADR 0004); a grant needs the data
	// directory for the audit trail.
	opts := engine.Options{Logger: log, Commission: true}
	// Without a usable data directory the application still explores; it
	// only cannot keep profiles.
	if err := ensureDir(*dataDir); err != nil {
		log.Warn("connection profiles are not available", "error", err)
	} else {
		opts.DataDir = *dataDir
	}
	eng := engine.New(opts)
	defer eng.Close()

	app := application.New(application.Options{
		Name:        appTitle,
		Description: "Explorer for IEC 61850 devices over MMS",
		Icon:        appIcon,
		Logger:      log,
		Services: []application.Service{
			application.NewServiceWithOptions(gui.New(eng),
				application.ServiceOptions{MarshalError: gui.MarshalError}),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     appTitle,
		Width:     1360,
		Height:    860,
		MinWidth:  960,
		MinHeight: 600,
		URL:       "/",
		// --ot-bg of the dark theme, so the window does not flash white.
		BackgroundColour: application.NewRGB(0x11, 0x18, 0x1f),
	})
	return app.Run()
}

// defaultDataDir is the user's configuration directory: it survives reboots,
// unlike the daemon's temporary default.
func defaultDataDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "iec61850-explorer")
	}
	return daemon.DefaultDataDir()
}

// ensureDir creates the parent of dir as needed and dir itself with the
// daemon's checks: mode 0700, owned by this user, not a symbolic link.
func ensureDir(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	return daemon.EnsureDataDir(dir)
}
