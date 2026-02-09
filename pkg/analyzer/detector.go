package analyzer

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// channelProducer is a detected goroutine that sends values into a locally
// created channel which is then returned.
type channelProducer struct {
	sends     []*ast.SendStmt
	funcLit   *ast.FuncLit
	chanIdent *ast.Ident
	chanType  *types.Chan
	makePos   token.Pos
	bufSize   int
}

// detect scans a file for the generator idiom:
//
//	func F() <-chan T {
//	    ch := make(chan T [, N])
//	    go func() { ... ch <- v ... }()
//	    return ch
//	}
func detect(pass *analysis.Pass, file *ast.File) []channelProducer {
	var results []channelProducer

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Type.Results == nil {
			continue
		}
		if !returnsChan(fn.Type.Results) {
			continue
		}

		var chanVar *ast.Ident
		var makePos token.Pos
		var bufSize int
		var goStmts []*ast.GoStmt

		for _, stmt := range fn.Body.List {
			switch s := stmt.(type) {
			case *ast.AssignStmt:
				if id, pos, buf, found := extractMakeChan(s); found {
					chanVar = id
					makePos = pos
					bufSize = buf
				}
			case *ast.GoStmt:
				goStmts = append(goStmts, s)
			}
		}

		// Must have exactly one channel and one goroutine.
		if chanVar == nil || len(goStmts) != 1 {
			continue
		}

		funcLit, ok := goStmts[0].Call.Fun.(*ast.FuncLit)
		if !ok {
			continue
		}

		sends := collectSends(funcLit, chanVar.Name)
		if len(sends) == 0 {
			continue
		}

		var ct *types.Chan
		if obj := pass.TypesInfo.ObjectOf(chanVar); obj != nil {
			ct, _ = obj.Type().(*types.Chan)
		}

		results = append(results, channelProducer{
			funcLit:   funcLit,
			chanIdent: chanVar,
			chanType:  ct,
			makePos:   makePos,
			sends:     sends,
			bufSize:   bufSize,
		})
	}

	return results
}

// returnsChan checks if any return value is a channel type.
func returnsChan(results *ast.FieldList) bool {
	for _, f := range results.List {
		if _, ok := f.Type.(*ast.ChanType); ok {
			return true
		}
	}
	return false
}

// extractMakeChan finds `ch := make(chan T [, N])` assignments.
func extractMakeChan(s *ast.AssignStmt) (*ast.Ident, token.Pos, int, bool) {
	if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
		return nil, 0, 0, false
	}
	id, ok := s.Lhs[0].(*ast.Ident)
	if !ok {
		return nil, 0, 0, false
	}
	call, ok := s.Rhs[0].(*ast.CallExpr)
	if !ok {
		return nil, 0, 0, false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "make" {
		return nil, 0, 0, false
	}
	if len(call.Args) < 1 {
		return nil, 0, 0, false
	}
	if _, ok := call.Args[0].(*ast.ChanType); !ok {
		return nil, 0, 0, false
	}
	buf := 0
	if len(call.Args) >= 2 {
		if lit, ok := call.Args[1].(*ast.BasicLit); ok && lit.Kind == token.INT {
			for _, c := range lit.Value {
				buf = buf*10 + int(c-'0')
			}
		}
	}
	return id, s.Pos(), buf, true
}

// collectSends finds all `ch <- expr` statements inside a function literal.
func collectSends(fl *ast.FuncLit, chanName string) []*ast.SendStmt {
	var sends []*ast.SendStmt
	ast.Inspect(fl, func(n ast.Node) bool {
		s, ok := n.(*ast.SendStmt)
		if !ok {
			return true
		}
		if ident, ok := s.Chan.(*ast.Ident); ok && ident.Name == chanName {
			sends = append(sends, s)
		}
		return true
	})
	return sends
}
