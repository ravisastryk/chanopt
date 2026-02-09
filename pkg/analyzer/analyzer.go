package analyzer

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

// Analyzer is the exported [analysis.Analyzer] for chanopt.
//
// Usage:
//
//	go vet -vettool=$(which chanopt) ./...
var Analyzer = &analysis.Analyzer{
	Name:     "chanopt",
	Doc:      "detect channel patterns replaceable with mutex/atomic (8-127x faster)",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, cp := range detect(pass, file) {
			pat, conf := classify(cp, pass)
			if pat == Unknown || conf < 0.5 {
				continue
			}
			spec := Registry[pat]
			pass.Reportf(cp.makePos,
				"chanopt: %s pattern — replace channel with %s (%s speedup, %.0f%% confidence)",
				pat, spec.Replacement, spec.Speedup, conf*100,
			)
		}
	}
	return nil, nil
}
