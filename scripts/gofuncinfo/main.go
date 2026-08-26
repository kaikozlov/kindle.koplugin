// gofuncinfo — Go function metadata dumper for the parity auditor.
//
// Parses Go source files with go/ast and emits JSON describing every
// top-level function: size, call graph, and "trivial body" flags used by
// scripts/audit_parity.py to detect name-only stubs.
//
// Flags:
//   empty        body has zero statements
//   const_only   body is only return statements of constants/nil/identity
//   error_only   body only returns errors.New/fmt.Errorf(...) results
//   notimpl      admits "not implemented"/"not supported" (or a
//                "use <other-func>" redirect) in a returned error/panic
//
// A function with empty/const_only/error_only body whose Python reference
// is substantive is a stub, not a port.
//
// Usage:
//
//	go run ./scripts/gofuncinfo internal cmd
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// FuncInfo is the audit record for one top-level Go function.
type FuncInfo struct {
	File      string   `json:"file"`
	Name      string   `json:"name"`
	Recv      string   `json:"recv,omitempty"`
	Line      int      `json:"line"`
	EndLine   int      `json:"end_line"`
	NStmt     int      `json:"nstmt"`
	NLit      int      `json:"nlit"` // composite literal element count (struct/map/slice literals)
	NChars    int      `json:"nchars"`
	Empty     bool     `json:"empty"`
	ConstOnly bool     `json:"const_only"`
	ErrorOnly bool     `json:"error_only"`
	NotImpl   bool     `json:"notimpl"`
	Calls     []string `json:"calls"`
	SelfCalls int      `json:"self_calls"`
	// CalledBy counts CallExpr sites with this function's name anywhere in
	// the scanned corpus (methods matched by bare name), excluding its own
	// body. Zero means "no production call site found".
	CalledBy int `json:"called_by"`
}

var notImplRe = regexp.MustCompile(`(?i)not (?:yet )?implemented|unimplemented|not supported|unsupported|^use [A-Za-z_]\w*$`)

func main() {
	dirs := os.Args[1:]
	if len(dirs) == 0 {
		dirs = []string{"internal", "cmd"}
	}

	sources, err := readSources(dirs)
	if err != nil {
		fatal(err)
	}
	funcs, callCounts, err := scanSources(sources)
	if err != nil {
		fatal(err)
	}

	out := struct {
		Functions  []FuncInfo     `json:"functions"`
		CallCounts map[string]int `json:"call_counts"`
	}{funcs, callCounts}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	if err := enc.Encode(out); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gofuncinfo:", err)
	os.Exit(1)
}

func readSources(dirs []string) (map[string][]byte, error) {
	var files []string
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			name := info.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	sources := map[string][]byte{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		sources[f] = data
	}
	return sources, nil
}

// scanSources parses Go sources and produces FuncInfo records plus a
// global name -> CallExpr-site count map.
func scanSources(sources map[string][]byte) ([]FuncInfo, map[string]int, error) {
	fset := token.NewFileSet()
	var funcs []FuncInfo
	callCounts := map[string]int{}

	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		f, err := parser.ParseFile(fset, name, sources[name], 0)
		if err != nil {
			return nil, nil, err
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			funcs = append(funcs, analyzeFunc(name, fd, fset))
		}
		countCalls(f, callCounts)
	}
	for i := range funcs {
		f := &funcs[i]
		f.CalledBy = callCounts[f.Name] - f.SelfCalls
	}
	return funcs, callCounts, nil
}

func analyzeFunc(file string, fd *ast.FuncDecl, fset *token.FileSet) FuncInfo {
	info := FuncInfo{
		File:    filepath.Base(file),
		Name:    fd.Name.Name,
		Line:    fset.Position(fd.Pos()).Line,
		EndLine: fset.Position(fd.End()).Line,
	}
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		info.Recv = recvTypeName(fd.Recv.List[0].Type)
	}

	params := map[string]bool{}
	if fd.Type.Params != nil {
		for _, field := range fd.Type.Params.List {
			for _, n := range field.Names {
				params[n.Name] = true
			}
		}
	}

	if fd.Body != nil {
		info.NChars = fset.Position(fd.Body.End()).Offset - fset.Position(fd.Body.Pos()).Offset
	}

	stmts := 0
	nlit := 0
	calls := map[string]bool{}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if _, ok := n.(ast.Stmt); ok {
			if _, isBlock := n.(*ast.BlockStmt); !isBlock {
				stmts++
			}
		}
		if cl, ok := n.(*ast.CompositeLit); ok {
			nlit += len(cl.Elts)
		}
		if ce, ok := n.(*ast.CallExpr); ok {
			if name := callName(ce.Fun); name != "" {
				calls[name] = true
				if name == info.Name {
					info.SelfCalls++
				}
			}
		}
		return true
	})
	info.NStmt = stmts
	info.NLit = nlit
	info.Empty = len(fd.Body.List) == 0

	callNames := make([]string, 0, len(calls))
	for c := range calls {
		callNames = append(callNames, c)
	}
	sort.Strings(callNames)
	info.Calls = callNames

	// Trivial-body analysis over top-level statements.
	hasNonTrivial := false // a returned value that is neither constant nor error ctor
	hasErrorCtor := false  // a returned errors.New/fmt.Errorf(...) value
	allReturns := true     // every top-level statement is a return
	for _, stmt := range fd.Body.List {
		rs, ok := stmt.(*ast.ReturnStmt)
		if !ok {
			allReturns = false
			hasNonTrivial = true // real logic: something besides returning
			break
		}
		if len(rs.Results) == 0 {
			continue // bare return: still trivial
		}
		for _, res := range rs.Results {
			if isErrorCtor(res) {
				hasErrorCtor = true
				if msg := ctorMessage(res); msg != "" && notImplRe.MatchString(msg) {
					info.NotImpl = true
				}
				continue
			}
			if !isTrivialExpr(res, params) {
				hasNonTrivial = true
			}
		}
	}
	if len(fd.Body.List) > 0 && allReturns {
		info.ConstOnly = !hasNonTrivial && !hasErrorCtor
		info.ErrorOnly = !hasNonTrivial && hasErrorCtor
	}

	// panic("not implemented")
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "panic" && len(ce.Args) == 1 {
				if lit, ok := ce.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if notImplRe.MatchString(unquote(lit.Value)) {
						info.NotImpl = true
					}
				}
			}
		}
		return true
	})

	return info
}

func recvTypeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return recvTypeName(v.X)
	case *ast.IndexExpr:
		return recvTypeName(v.X)
	}
	return ""
}

func callName(fun ast.Expr) string {
	switch v := fun.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	}
	return ""
}

func isErrorCtor(e ast.Expr) bool {
	ce, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch callName(ce.Fun) {
	case "New", "Errorf", "Error":
		return true
	}
	return false
}

func ctorMessage(e ast.Expr) string {
	ce, ok := e.(*ast.CallExpr)
	if !ok || len(ce.Args) == 0 {
		return ""
	}
	if format, ok := ce.Args[0].(*ast.BasicLit); ok && format.Kind == token.STRING {
		return unquote(format.Value)
	}
	return ""
}

func isTrivialExpr(e ast.Expr, params map[string]bool) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name == "nil" || v.Name == "true" || v.Name == "false" || params[v.Name]
	case *ast.BasicLit:
		return true
	case *ast.CompositeLit:
		return len(v.Elts) == 0
	case *ast.ParenExpr:
		return isTrivialExpr(v.X, params)
	case *ast.UnaryExpr:
		return v.Op == token.SUB
	}
	return false
}

func unquote(s string) string {
	if len(s) >= 2 {
		return s[1 : len(s)-1]
	}
	return s
}

func countCalls(f *ast.File, counts map[string]int) {
	ast.Inspect(f, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if name := callName(ce.Fun); name != "" {
				counts[name]++
			}
		}
		return true
	})
}
