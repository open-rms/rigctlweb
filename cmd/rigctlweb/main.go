// Command rigctlweb serves a web UI for controlling radios through Hamlib's rigctld.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/open-rms/rigctlweb/internal/config"
	"github.com/open-rms/rigctlweb/internal/integration"
	"github.com/open-rms/rigctlweb/internal/presets"
	"github.com/open-rms/rigctlweb/internal/rig"
	"github.com/open-rms/rigctlweb/internal/server"
	"github.com/open-rms/rigctlweb/web"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "audio" {
		os.Exit(audioCmd(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "openapi" {
		os.Exit(openapiCmd(os.Args[2:]))
	}
	var (
		cfgPath = flag.String("config", "", "config file (default: per-user config dir)")
		listen  = flag.String("listen", "", "listen address, overrides config (e.g. 0.0.0.0:8480)")
		rigAddr = flag.String("rig", "", "add a rigctld host:port for this run only (e.g. 127.0.0.1:4532)")
		debug   = flag.Bool("debug", false, "verbose logging")
		noAuth  = flag.Bool("dev-no-auth", false, "DEV ONLY: disable the access token (refused unless listening on loopback)")
		showTok = flag.Bool("show-token", false, "print the access token at startup (it is otherwise only printed when first generated)")
		showVer = flag.Bool("version", false, "print version and exit")
		basePth = flag.String("base-path", "", "serve under this URL prefix, e.g. /rigctlweb behind a reverse proxy (default: the root)")
		rigsURL = flag.String("presets-url", presets.DefaultBaseURL, "openRMS Rigs API for the presets library (fetched only when you press Update library)")
	)
	flag.Parse()
	if *showVer {
		fmt.Println("rigctlweb", version)
		return
	}

	lvl := slog.LevelInfo
	if *debug {
		lvl = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))

	// hamstack integration mode: hamstack starts this program with HAMSTACK_* variables, proxies it
	// behind its own login, and hands it the radios. Anything wrong with what it was given is fatal:
	// starting half set up could leave it open without a login, or let it transmit where the owner
	// said read only.
	henv, integrated := integration.FromEnv(os.Getenv)
	var secret string
	var hrigs []integration.Rig
	if integrated {
		var err error
		if secret, err = integration.ReadSecret(henv.SecretFile); err != nil {
			log.Error("integration mode: refusing to start", "err", err)
			os.Exit(1)
		}
		if hrigs, err = integration.ParseRigs(henv.Rigs); err != nil {
			log.Error("integration mode: refusing to start", "err", err)
			os.Exit(1)
		}
		if *basePth == "" {
			*basePth = henv.BasePath
		}
		if *cfgPath == "" && henv.ConfigDir != "" {
			*cfgPath = filepath.Join(henv.ConfigDir, "config.toml")
		}
		if *noAuth {
			log.Error("integration mode: --dev-no-auth is not allowed")
			os.Exit(1)
		}
	}
	base, err := integration.NormalizeBasePath(*basePth)
	if err != nil {
		log.Error("refusing to start", "err", err)
		os.Exit(1)
	}

	path := *cfgPath
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			log.Error("no config dir", "err", err)
			os.Exit(1)
		}
		path = p
	}
	store, err := config.Open(path)
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	cfg := store.Get()
	addr := cfg.Listen
	if *listen != "" {
		addr = *listen
	}

	if integrated && !server.IsLoopback(addr) {
		log.Error("integration mode only listens on localhost (hamstack proxies it); refusing to start", "listen", addr)
		os.Exit(1)
	}
	if *noAuth && !server.IsLoopback(addr) {
		log.Error("--dev-no-auth is only allowed when listening on localhost; refusing to start", "listen", addr)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mgr := rig.NewManager(ctx, log)
	stateDir := filepath.Join(filepath.Dir(path), "state")
	if integrated && henv.DataDir != "" {
		// hamstack's stop hook looks for keying markers here (state/keyed-<rig id>).
		stateDir = filepath.Join(henv.DataDir, "state")
	}
	mgr.SetStateDir(stateDir)
	switch {
	case integrated:
		// The radios come from hamstack only; config rigs and -rig are ignored.
		if len(cfg.Rigs) > 0 || *rigAddr != "" {
			log.Info("integration mode: using hamstack's radios; radios in the config file and -rig are ignored")
		}
		for _, hr := range hrigs {
			if err := mgr.Add(rig.Config{ID: hr.ID, Name: hr.ID, Addr: hr.Addr, ReadOnly: hr.ReadOnly}); err != nil {
				log.Error("rig", "id", hr.ID, "err", err)
			}
		}
	default:
		for _, rc := range cfg.Rigs {
			if err := mgr.Add(rc); err != nil {
				log.Error("rig", "id", rc.ID, "err", err)
			}
		}
	}
	if *rigAddr != "" && !integrated {
		if err := mgr.Add(rig.Config{ID: "cli", Name: "rigctld " + *rigAddr, Addr: *rigAddr}); err != nil {
			log.Error("rig", "err", err)
		}
	}

	app := server.New(store, mgr, web.UI(), version, log)
	app.SetBasePath(base)
	if integrated {
		app.SetIntegration(secret)
	}
	lib, err := presets.NewLibrary(stateDir, *rigsURL, "rigctlweb/"+version)
	if lib == nil {
		log.Error("presets library", "err", err)
		os.Exit(1)
	}
	if err != nil {
		log.Warn("presets library: the saved copy could not be read", "err", err)
	}
	app.SetPresets(presets.NewService(lib, stateDir, log))
	if *noAuth {
		app.DisableAuth()
		log.Warn("AUTHENTICATION DISABLED (--dev-no-auth); anyone on this machine can control the radio")
	}
	// Requests run on a context cancelled at shutdown, so long-lived streams (Server-Sent Events)
	// end instead of holding srv.Shutdown until its deadline.
	reqCtx, endRequests := context.WithCancel(context.Background())
	defer endRequests()
	srv := &http.Server{
		Addr:              addr,
		BaseContext:       func(net.Listener) context.Context { return reqCtx },
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	log.Info("rigctlweb starting", "version", version, "listen", addr, "base_path", base, "config", path, "mode", map[bool]string{true: "hamstack", false: "standalone"}[integrated])
	if !server.IsLoopback(addr) {
		log.Warn("listening beyond localhost: keep the access token secret and put a TLS reverse proxy " +
			"(Caddy/nginx) or a VPN (WireGuard/Tailscale) in front; never expose this directly to the internet")
	}
	switch {
	case integrated:
		// No token banner: sign-in is hamstack's, and stderr goes to its journal.
	case *noAuth:
		fmt.Fprintf(os.Stderr, "\n  Open http://%s%s/ (no sign-in: dev mode)\n\n", displayAddr(addr), base)
	case store.Created || *showTok:
		// Printed only on first run or on request so the token doesn't accumulate in service logs.
		fmt.Fprintf(os.Stderr, "\n  Open http://%s%s/ and sign in with this access token:\n\n    %s\n\n", displayAddr(addr), base, cfg.Token)
	default:
		fmt.Fprintf(os.Stderr, "\n  Open http://%s%s/. The access token is in %s (or run with --show-token).\n\n", displayAddr(addr), base, path)
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server", "err", err)
		}
	case <-ctx.Done():
	}
	// Drop the transmitter first, on a context the termination signal can't cancel, and exit
	// within 5 s in all (the supervisor contract). Only rigs this program keyed are touched
	// (see internal/rig/keying.go).
	log.Info("shutting down; unkeying rigs first")
	deadline := time.Now().Add(4500 * time.Millisecond)
	uctx, ucancel := context.WithDeadline(context.Background(), deadline.Add(-1500*time.Millisecond))
	mgr.UnkeyAll(uctx)
	ucancel()
	endRequests()
	sctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	_ = srv.Shutdown(sctx)
	mgr.Shutdown(sctx)
}

func displayAddr(a string) string {
	if len(a) > 0 && a[0] == ':' {
		return "localhost" + a
	}
	return a
}
