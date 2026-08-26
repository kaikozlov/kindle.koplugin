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
