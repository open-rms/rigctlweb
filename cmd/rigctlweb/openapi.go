package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/open-rms/rigctlweb/internal/server"
)

// openapiCmd prints the OpenAPI document (`rigctlweb openapi > docs/openapi.json`).
func openapiCmd(args []string) int {
	fs := flag.NewFlagSet("openapi", flag.ExitOnError)
	url := fs.String("server", server.DefaultServerURL, "server URL to name in the document")
	_ = fs.Parse(args)
	fmt.Print(string(server.OpenAPIJSON("", *url)))
	_ = os.Stdout.Sync()
	return 0
}
