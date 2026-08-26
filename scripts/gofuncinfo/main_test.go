package main

import (
	"encoding/json"
	"testing"
)

func runScan(t *testing.T, src string) map[string]FuncInfo {
	t.Helper()
	funcs, _, err := scanSources(map[string][]byte{"fixture.go": []byte(src)})
	if err != nil {
		t.Fatalf("scanSources: %v", err)
	}
	out := map[string]FuncInfo{}
	for _, f := range funcs {
		out[f.Name] = f
	}
	return out
}

const fixtureSrc = `package fixture

import "fmt"

func realWork(x int) int {
	total := 0
	for i := 0; i < x; i++ {
		if i%2 == 0 {
			total += i
		}
	}
	return total
}

func emptyStub() {}

func nilReturn() error { return nil }

func blankReturn() string { return "" }

func zeroReturn() int { return 0 }

func constReturn() int { return 1 }

func identityReturn(data []byte) []byte { return data }

func trueReturn() bool { return true }

func falseReturn() bool { return false }

func notImplemented(input string) error {
	return fmt.Errorf("not implemented")
}

func notSupported(input string) error {
	return errors.New("unsupported output mode")
}

func redirectStub() (*Book, error) {
	return nil, fmt.Errorf("use decodeBookFromData")
}

func mixedNilAndError() (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func assignThenReturn(x int) int {
	y := x * 2
	return y
}

func panics() {
	panic("not implemented yet")
}

func selfRecursive(n int) int {
	if n <= 0 {
		return 0
	}
	return selfRecursive(n-1)
}

func caller() int {
	return realWork(3) + zeroReturn()
}
`

func TestSubstanceClassification(t *testing.T) {
	fns := runScan(t, fixtureSrc)

	cases := []struct {
		name              string
		wantEmpty         bool
		wantConstOnly     bool
		wantErrorOnly     bool
		wantNotImpl       bool
		wantSubstantiveNS int // expected lower bound on nstmt
	}{
		{"realWork", false, false, false, false, 6},
		{"emptyStub", true, false, false, false, 0},
		{"nilReturn", false, true, false, false, 0},
		{"blankReturn", false, true, false, false, 0},
		{"zeroReturn", false, true, false, false, 0},
		{"constReturn", false, true, false, false, 0},
		{"identityReturn", false, true, false, false, 0},
		{"notImplemented", false, false, true, true, 0},
		{"notSupported", false, false, true, true, 0},
		{"redirectStub", false, false, true, true, 0},
		{"mixedNilAndError", false, false, true, true, 0},
		{"assignThenReturn", false, false, false, false, 1},
		{"panics", false, false, false, true, 0},
	}
	for _, tc := range cases {
		f, ok := fns[tc.name]
		if !ok {
			t.Fatalf("function %s not found", tc.name)
		}
		if f.Empty != tc.wantEmpty {
			t.Errorf("%s: Empty=%v want %v", tc.name, f.Empty, tc.wantEmpty)
		}
		if f.ConstOnly != tc.wantConstOnly {
			t.Errorf("%s: ConstOnly=%v want %v", tc.name, f.ConstOnly, tc.wantConstOnly)
		}
		if f.ErrorOnly != tc.wantErrorOnly {
			t.Errorf("%s: ErrorOnly=%v want %v", tc.name, f.ErrorOnly, tc.wantErrorOnly)
		}
		if f.NotImpl != tc.wantNotImpl {
			t.Errorf("%s: NotImpl=%v want %v", tc.name, f.NotImpl, tc.wantNotImpl)
		}
		if f.NStmt < tc.wantSubstantiveNS {
			t.Errorf("%s: NStmt=%d want >= %d", tc.name, f.NStmt, tc.wantSubstantiveNS)
		}
	}
}

func TestCallGraph(t *testing.T) {
	_, counts, err := scanSources(map[string][]byte{"fixture.go": []byte(fixtureSrc)})
	if err != nil {
		t.Fatalf("scanSources: %v", err)
	}
	if counts["realWork"] != 1 {
		t.Errorf("realWork call count = %d, want 1", counts["realWork"])
	}
	if counts["zeroReturn"] != 1 {
		t.Errorf("zeroReturn call count = %d, want 1", counts["zeroReturn"])
	}
	if counts["selfRecursive"] != 1 {
		t.Errorf("selfRecursive call count = %d, want 1 (self only)", counts["selfRecursive"])
	}
	if counts["notImplemented"] != 0 {
		t.Errorf("notImplemented call count = %d, want 0", counts["notImplemented"])
	}

	fns := runScan(t, fixtureSrc)
	if got := fns["selfRecursive"].SelfCalls; got != 1 {
		t.Errorf("selfRecursive SelfCalls = %d, want 1", got)
	}
	if got := fns["selfRecursive"].CalledBy; got != 0 {
		t.Errorf("selfRecursive CalledBy = %d, want 0", got)
	}
	if got := fns["realWork"].CalledBy; got != 1 {
		t.Errorf("realWork CalledBy = %d, want 1", got)
	}
	if got := fns["notImplemented"].CalledBy; got != 0 {
		t.Errorf("notImplemented CalledBy = %d, want 0", got)
	}
}

func TestIdentCallsExcludeSelectors(t *testing.T) {
	// Selector calls must never be delegation evidence: strings.TrimSpace
	// must not resolve to a project function named TrimSpace, and obj.Get
	// must not resolve to an unrelated method named Get. Only unqualified
	// Ident calls (helper()) may be followed transitively.
	fns := runScan(t, `package p

import "strings"

func TrimSpace(s string) string { return s }

func (t thing) Get() int { return 1 }

type thing struct{ x int }

func wrapper(s string, obj thing) string {
	return strings.TrimSpace(s) + "\u0000" + string(obj.Get())
}

func honestWrapper(s string) string {
	return helper(s)
}

func helper(s string) string {
	total := ""
	for i := 0; i < 10; i++ {
		total += s
	}
	return total
}
`)
	w := fns["wrapper"]
	if contains(w.IdentCalls, "TrimSpace") {
		t.Errorf("wrapper IdentCalls must not include selector TrimSpace: %v", w.IdentCalls)
	}
	if contains(w.IdentCalls, "Get") {
		t.Errorf("wrapper IdentCalls must not include selector Get: %v", w.IdentCalls)
	}
	if !contains(w.Calls, "TrimSpace") || !contains(w.Calls, "Get") {
		t.Errorf("wrapper Calls should still report all calls for diagnostics: %v", w.Calls)
	}
	hw := fns["honestWrapper"]
	if !contains(hw.IdentCalls, "helper") {
		t.Errorf("honestWrapper IdentCalls must include helper: %v", hw.IdentCalls)
	}
	if !contains(hw.Calls, "helper") {
		t.Errorf("honestWrapper Calls must include helper: %v", hw.Calls)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestFuncLitNotCountedAsOuterSubstance(t *testing.T) {
	// A thin wrapper that defines a large UNUSED closure must stay trivial:
	// closure statements must not inflate the outer function's nstmt/nlit.
	fns := runScan(t, `package p

func wrapperWithBigUnusedClosure() error {
	_ = func() {
		total := 0
		for i := 0; i < 100; i++ {
			total += i
		}
		_ = map[string]int{"one": 1, "two": 2, "three": 3}
	}
	return nil
}

func realClosureWork(fn func(int) int, x int) int {
	return fn(x) + 1
}
`)
	w := fns["wrapperWithBigUnusedClosure"]
	if w.NStmt > 2 {
		t.Errorf("outer nstmt = %d, want <= 2 (closure must not count)", w.NStmt)
	}
	if w.NLit != 0 {
		t.Errorf("outer nlit = %d, want 0 (closure literals must not count)", w.NLit)
	}
	if !w.ConstOnly {
		t.Error("wrapper returning only nil must stay const_only")
	}
	if w.TrivialShape != "const:nil" {
		t.Errorf("TrivialShape = %q, want const:nil", w.TrivialShape)
	}
}

func TestTrivialShapes(t *testing.T) {
	fns := runScan(t, fixtureSrc)
	want := map[string]string{
		"emptyStub":        "void",
		"nilReturn":        "const:nil",
		"blankReturn":      "const:empty-string",
		"zeroReturn":       "const:int:0",
		"constReturn":      "const:int:1",
		"identityReturn":   "arg:data",
		"trueReturn":       "const:true",
		"falseReturn":      "const:false",
	}
	for name, shape := range want {
		if got := fns[name].TrivialShape; got != shape {
			t.Errorf("%s TrivialShape = %q, want %q", name, got, shape)
		}
	}
	// Call-shaped and computed returns are never semantically trivial.
	for _, name := range []string{"notImplemented", "notSupported", "redirectStub", "assignThenReturn"} {
		if fns[name].TrivialShape != "" {
			t.Errorf("%s TrivialShape = %q, want empty (not trivial)", name, fns[name].TrivialShape)
		}
	}
}

func TestJSONRoundTrip(t *testing.T) {
	funcs, counts, err := scanSources(map[string][]byte{"fixture.go": []byte(fixtureSrc)})
	if err != nil {
		t.Fatalf("scanSources: %v", err)
	}
	data, err := json.Marshal(struct {
		Functions  []FuncInfo     `json:"functions"`
		CallCounts map[string]int `json:"call_counts"`
	}{funcs, counts})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back struct {
		Functions  []FuncInfo     `json:"functions"`
		CallCounts map[string]int `json:"call_counts"`
	}
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Functions) != len(funcs) {
		t.Errorf("round trip lost functions: %d != %d", len(back.Functions), len(funcs))
	}
}

func TestNLitCountsCompositeLiterals(t *testing.T) {
	fns := runScan(t, `package p

type cfg struct{ A, B, C int }

func constructor() cfg {
	return cfg{A: 1, B: 2, C: 3}
}

func table() map[string]int {
	return map[string]int{"one": 1, "two": 2, "three": 3, "four": 4}
}

func nothing() cfg { return cfg{} }
`)
	if got := fns["constructor"].NLit; got != 3 {
		t.Errorf("constructor NLit = %d, want 3", got)
	}
	if got := fns["table"].NLit; got != 4 {
		t.Errorf("table NLit = %d, want 4", got)
	}
	if got := fns["nothing"].NLit; got != 0 {
		t.Errorf("nothing NLit = %d, want 0", got)
	}
}

func TestTrivialReturnIsZeroValueOnly(t *testing.T) {
	// A function returning a non-constant computed value must NOT be
	// const_only even if its body is a single return.
	fns := runScan(t, `package p
func computed(x int) int { return x + 1 }
`)
	if fns["computed"].ConstOnly {
		t.Error("computed(x) { return x + 1 } flagged ConstOnly; parameter arithmetic is logic")
	}
}
