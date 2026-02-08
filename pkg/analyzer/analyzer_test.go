package analyzer_test

import (
	"testing"

	"github.com/ravisastryk/chanopt/pkg/analyzer"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPositivePatterns(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), analyzer.Analyzer, "positive")
}

func TestNegativePatterns(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), analyzer.Analyzer, "negative")
}
