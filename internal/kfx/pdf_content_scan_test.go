package kfx

import "testing"

// Scanner tests for pdfScanContent, which backs the "page.extract_text()"
// truthiness check of Python resources.py:394-395 against the exact bundled
// pypdf semantics (_page.py: text only from Tj/TJ/'/" operators with
// non-empty operands; Forms contribute only when Do-invoked).

func scanText(t *testing.T, content string) bool {
	t.Helper()
	hasText, _ := pdfScanContent([]byte(content), nil)
	return hasText
}

func TestPDFScanContent_PlainImageDraw(t *testing.T) {
	if scanText(t, "q 144 0 0 288 0 0 cm /Im0 Do Q") {
		t.Fatal("plain image draw must not be reported as text")
	}
}

func TestPDFScanContent_TextOperators(t *testing.T) {
	for _, content := range []string{
		"BT /F1 12 Tf 72 720 Td (Hello) Tj ET",
		"BT /F1 12 Tf [(A) -250 (B)] TJ ET",
		"(continued) '",
		"100 (kern) \"",
	} {
		if !scanText(t, content) {
			t.Fatalf("%q must be reported as text", content)
		}
	}
}

func TestPDFScanContent_EmptyTextOperands_NoText(t *testing.T) {
	// pypdf extracts no text from empty strings and kerning-only arrays;
	// operator presence alone is not text.
	for _, content := range []string{
		"BT () Tj ET",
		"BT [] TJ ET",
		"BT [-250] TJ ET",
		"BT [() -120 ()] TJ ET",
		"BT <> Tj ET", // empty hex string
	} {
		if scanText(t, content) {
			t.Fatalf("%q must not be reported as text (empty operands)", content)
		}
	}
}

func TestPDFScanContent_NonEmptyTextInMixedArray(t *testing.T) {
	if !scanText(t, "BT [() (A) -120 ()] TJ ET") {
		t.Fatal("array containing a non-empty string must be reported as text")
	}
}

func TestPDFScanContent_OperatorBytesInString(t *testing.T) {
	if scanText(t, "(an embedded Tj inside a string) 0 0 m") {
		t.Fatal("operator-like bytes inside a literal string must not count as text")
	}
}

func TestPDFScanContent_EscapedAndNestedParens(t *testing.T) {
	if scanText(t, `(escaped \) paren and (nested (Tj) parens) here) 1 0 0 1 0 0 cm /Im0 Do`) {
		t.Fatal("escaped/nested parens containing Tj bytes must not count as text")
	}
	// Octal escape (\\101 == 'A') is a real character and produces text.
	if !scanText(t, `BT (\\101) Tj ET`) {
		t.Fatal("octal-escaped character must count as text")
	}
	// Line continuation after backslash contributes no character: still empty.
	if scanText(t, "BT (\\\n) Tj ET") {
		t.Fatal("line-continued empty string must not count as text")
	}
}

func TestPDFScanContent_OperatorBytesInName(t *testing.T) {
	if scanText(t, "/Tj 1 0 0 1 0 0 cm /Im0 Do") {
		t.Fatal("a PDF name /Tj must not count as a text operator")
	}
}

func TestPDFScanContent_OperatorBytesInHexAndComment(t *testing.T) {
	if scanText(t, "<546a20686578> 0 0 m") {
		t.Fatal("operator-like bytes in a hex string must not count as text")
	}
	if scanText(t, "% Tj in a comment\n0 0 m") {
		t.Fatal("operator-like bytes in a comment must not count as text")
	}
}

func TestPDFScanContent_OperandsDoNotLeakAcrossOperators(t *testing.T) {
	// (hello) is an operand of m, not of any text operator.
	if scanText(t, "(hello) 0 0 m 1 0 0 1 0 0 cm") {
		t.Fatal("operands consumed by non-text operators must not leak into text operators")
	}
	if scanText(t, "(hello) T*") {
		t.Fatal("T* is not a text-showing operator")
	}
}

func TestPDFScanContent_DictionaryOperandSkipped(t *testing.T) {
	// Marked-content property dicts contain strings that are not operands.
	if scanText(t, "/Span <</ActualText (Tj here)>> BDC q 72 0 0 108 0 0 cm /Im0 Do Q EMC") {
		t.Fatal("strings inside dictionary operands must not count as text")
	}
}

func TestPDFScanContent_InlineImageBinary(t *testing.T) {
	payload := []byte{0x00, 0xff, 0x10, 'T', 'j', 0x42, 'T', 'J', 0x99}
	content := append([]byte("q BI /W 4 /H 4 /CS /G /BPC 8 ID "), payload...)
	content = append(content, []byte("\nEI Q\nBT 1 Tf (x) Tj ET")...)
	hasText, hasInline := pdfScanContent(content, nil)
	if !hasText {
		t.Fatal("text operator after an inline image must be found")
	}
	if !hasInline {
		t.Fatal("inline image must be detected")
	}

	content2 := append([]byte("q BI /W 4 /H 4 /CS /G /BPC 8 ID "), payload...)
	content2 = append(content2, []byte("\nEI Q")...)
	hasText2, hasInline2 := pdfScanContent(content2, nil)
	if hasText2 {
		t.Fatal("operator-like bytes inside inline image binary must not count as text")
	}
	if !hasInline2 {
		t.Fatal("inline image must be detected even without trailing text")
	}
}

func TestPDFScanContent_DoInvocationSemantics(t *testing.T) {
	invoked := map[string]bool{}
	pdfScanContent([]byte("q 72 0 0 108 0 0 cm /Im0 Do Q q /Fm0 Do Q"), func(name string) bool {
		invoked[name] = true
		return false
	})
	if !invoked["Im0"] || !invoked["Fm0"] {
		t.Fatalf("both Do operands must be resolved, got %v", invoked)
	}
	if len(invoked) != 2 {
		t.Fatalf("no other names must be treated as invoked, got %v", invoked)
	}

	// An uninvoked resource name must never reach the callback.
	invoked2 := map[string]bool{}
	pdfScanContent([]byte("/Fm1 0 0 m q 1 0 0 1 0 0 cm /Im0 Do Q"), func(name string) bool {
		invoked2[name] = true
		return false
	})
	if invoked2["Fm1"] {
		t.Fatal("a name operand of a non-Do operator must not count as invoked")
	}

	// Invoked form returning true reports text.
	if hasText, _ := pdfScanContent([]byte("q /Fm0 Do Q"), func(name string) bool {
		return name == "Fm0"
	}); !hasText {
		t.Fatal("Do-invoked form reporting text must propagate")
	}
}
