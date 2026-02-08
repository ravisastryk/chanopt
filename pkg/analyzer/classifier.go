package analyzer

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// classify determines which of the 10 patterns a channelProducer matches.
// Returns (Unknown, 0) if no pattern matches or safety gates reject it.
func classify(cp channelProducer, pass *analysis.Pass) (Pattern, float64) {
	body := cp.funcLit.Body
	if body == nil {
		return Unknown, 0
	}

	// ── Safety gates (must ALL pass) ──
	if containsMultiCaseSelect(body) {
		return Unknown, 0 // genuine coordination
	}
	if containsIO(body, pass) {
		return Unknown, 0 // I/O side effects
	}
	if rangesOverChannel(body, pass) {
		return Unknown, 0 // legitimate pipeline stage
	}

	ind := extractIndicators(body, cp.chanIdent.Name, pass)

	// ── Pattern matching (ordered by specificity) ──
	switch {
	// Bounded iterator: range over collection + close(ch)
	case ind.hasRange && ind.hasClose:
		return BoundedIterator, 0.92

	// Round-robin: modulo arithmetic + slice indexing in loop
	case ind.hasModulo && ind.hasIndexExpr && ind.infiniteLoop:
		return RoundRobin, 0.90

	// ID generator: counter increment in infinite loop
	case ind.hasIncrement && ind.infiniteLoop && !ind.hasTimeSleep:
		return IDGenerator, 0.95

	// Rate limiter: time.Ticker feeding a channel
	case ind.hasTimeTicker:
		return RateLimiter, 0.78

	// Ticker/Heartbeat: time.Sleep in infinite loop sending signals
	case ind.hasTimeSleep && ind.infiniteLoop:
		return ChanTicker, 0.80

	// Singleton: sends exactly once (single send, no loop around it)
	case len(cp.sends) == 1 && !ind.infiniteLoop && !ind.hasRange:
		return Singleton, 0.70

	default:
		return Unknown, 0
	}
}

// indicators are structural AST signals extracted in a single walk.
type indicators struct {
	hasIncrement  bool // i++ or i += 1
	hasModulo     bool // expr % expr
	hasIndexExpr  bool // slice[i]
	hasRange      bool // for _, v := range ...
	hasClose      bool // close(ch)
	hasTimeSleep  bool // time.Sleep(...)
	hasTimeTicker bool // time.NewTicker / time.Tick
	infiniteLoop  bool // for { ... } with no condition
}

func extractIndicators(body *ast.BlockStmt, chanName string, pass *analysis.Pass) indicators {
	var ind indicators
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IncDecStmt:
			if node.Tok == token.INC {
				ind.hasIncrement = true
			}
		case *ast.AssignStmt:
			for _, rhs := range node.Rhs {
				if bin, ok := rhs.(*ast.BinaryExpr); ok && bin.Op == token.REM {
					ind.hasModulo = true
				}
			}
		case *ast.IndexExpr:
			ind.hasIndexExpr = true
		case *ast.RangeStmt:
			// Only flag hasRange if ranging over a collection (slice/array/map),
			// not an input channel (which is a legitimate pipeline stage)
			if tv, ok := pass.TypesInfo.Types[node.X]; ok {
				// Skip if ranging over a channel type
				if _, isChanType := tv.Type.Underlying().(*types.Chan); !isChanType {
					ind.hasRange = true
				}
			} else {
				// No type info available, conservatively flag it
				ind.hasRange = true
			}
		case *ast.ForStmt:
			// Infinite loop: no condition (for { } or for i := 0; ; i++ { })
			if node.Cond == nil {
				ind.infiniteLoop = true
			}
		case *ast.CallExpr:
			// close(ch)
			if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "close" {
				if len(node.Args) == 1 {
					if arg, ok := node.Args[0].(*ast.Ident); ok && arg.Name == chanName {
						ind.hasClose = true
					}
				}
			}
			// time.Sleep, time.NewTicker, time.Tick
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "time" {
					switch sel.Sel.Name {
					case "Sleep":
						ind.hasTimeSleep = true
					case "NewTicker", "Tick":
						ind.hasTimeTicker = true
					}
				}
			}
		}
		return true
	})
	return ind
}

// containsMultiCaseSelect returns true if body has a select with 2+ cases.
// This indicates genuine coordination (e.g., with context cancellation).
func containsMultiCaseSelect(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		if sel, ok := n.(*ast.SelectStmt); ok && sel.Body != nil {
			if len(sel.Body.List) >= 2 {
				found = true
			}
		}
		return !found
	})
	return found
}

// containsIO returns true if the goroutine body calls net/os/io/database.
func containsIO(body *ast.BlockStmt, pass *analysis.Pass) bool {
	ioPkgs := map[string]bool{
		"net": true, "net/http": true, "os": true,
		"io": true, "database/sql": true,
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if obj := pass.TypesInfo.ObjectOf(ident); obj != nil {
			if pkg, ok := obj.(*types.PkgName); ok {
				if ioPkgs[pkg.Imported().Path()] {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// rangesOverChannel returns true if the goroutine ranges over an input channel parameter.
// This indicates a pipeline stage (channel-to-channel transformation), not a generator.
// Ranging over ticker.C or other internal channels is fine (not a pipeline stage).
func rangesOverChannel(body *ast.BlockStmt, pass *analysis.Pass) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		rangeStmt, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}

		// Only filter out if ranging over a simple identifier (likely a parameter)
		// Selectors like ticker.C are internal and don't indicate pipeline stages
		ident, isIdent := rangeStmt.X.(*ast.Ident)
		if !isIdent {
			return true // not an identifier, continue searching
		}

		// Check if this identifier is a channel type
		if tv, ok := pass.TypesInfo.Types[rangeStmt.X]; ok {
			if _, isChanType := tv.Type.Underlying().(*types.Chan); isChanType {
				// Check if it's a function parameter (not a local variable)
				if obj := pass.TypesInfo.ObjectOf(ident); obj != nil {
					// Parameters have parent scope of the function, locals are in inner scope
					// For now, conservatively filter out any channel identifier
					// This catches input channels from function parameters
					found = true
				}
			}
		}
		return !found
	})
	return found
}
