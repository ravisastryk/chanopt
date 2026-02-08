// Command chanopt runs the channel pattern analyzer as a standalone tool.
//
// Install:
//
//	go install github.com/ravisastryk/chanopt/cmd/chanopt@latest
//
// Usage:
//
//	go vet -vettool=$(which chanopt) ./...
package main

import (
	"github.com/ravisastryk/chanopt/pkg/analyzer"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(analyzer.Analyzer)
}
