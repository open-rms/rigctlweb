package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/open-rms/rigctlweb/internal/audio"
)

// audioCmd handles `rigctlweb audio ...`. Only the read-only detect step exists so far.
func audioCmd(args []string) int {
	if len(args) == 0 || args[0] != "detect" {
		fmt.Fprintln(os.Stderr, "usage: rigctlweb audio detect [-json]")
		return 2
	}
	fs := flag.NewFlagSet("audio detect", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	_ = fs.Parse(args[1:])

	r := audio.Detect(context.Background())
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
		return 0
	}

	fmt.Printf("System:  %s/%s\n", r.OS, r.Arch)
	fmt.Printf("Method:  %s\n", r.Method)
	if len(r.Server) > 0 {
		var s []string
		for k, v := range r.Server {
			s = append(s, k+" ("+v+")")
		}
		fmt.Printf("Servers: %s\n", strings.Join(s, ", "))
	}
	fmt.Println("Devices:")
	if len(r.Devices) == 0 {
		fmt.Println("  (none)")
	}
	for _, d := range r.Devices {
		fmt.Printf("  %-60s %s\n", d.ID, d.Name)
	}
	if len(r.Command) > 0 {
		fmt.Printf("Capture command (first device):\n  %s\n", strings.Join(r.Command, " "))
	}
	for _, n := range r.Notes {
		fmt.Printf("Note: %s\n", n)
	}
	fmt.Println("\nThis only looked; nothing was changed.")
	return 0
}
