package main

import (
	"os"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
