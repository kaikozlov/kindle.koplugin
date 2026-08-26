package kfx

import "testing"

// Scanner tests for pdfContentStreamHasText, which backs the
// "page.extract_text()" check of Python resources.py:394-395.

func TestPDFContentStreamHasText_PlainImageDraw(t *testing.T) {
	content := "q 144 0 0 288 0 0 cm /Im0 Do Q"
	if pdfContentStreamHasText([]byte(content)) {
		t.Fatal("plain image draw must not be reported as text")
	}
}

func TestPDFContentStreamHasText_TjOperator(t *testing.T) {
	content := "BT /F1 12 Tf 72 720 Td (Hello) Tj ET"
	if !pdfContentStreamHasText([]byte(content)) {
		t.Fatal("Tj operator must be reported as text")
	}
}

func TestPDFContentStreamHasText_TJArrayOperator(t *testing.T) {
	content := "BT /F1 12 Tf [(A) -250 (B)] TJ ET"
	if !pdfContentStreamHasText([]byte(content)) {
		t.Fatal("TJ operator must be reported as text")
	}
}

func TestPDFContentStreamHasText_MoveAndDoubleQuoteOperators(t *testing.T) {
	if !pdfContentStreamHasText([]byte("(continued) '")) {
		t.Fatal("' operator must be reported as text")
	}
	if !pdfContentStreamHasText([]byte("100 (kern) \"")) {
		t.Fatal("\" operator must be reported as text")
	}
}

func TestPDFContentStreamHasText_OperatorBytesInString(t *testing.T) {
	// The literal string operand contains "Tj" bytes; no actual text operator.
	content := "(an embedded Tj inside a string) 0 0 m"
	if pdfContentStreamHasText([]byte(content)) {
		t.Fatal("operator-like bytes inside a literal string must not count as text")
	}
}

func TestPDFContentStreamHasText_EscapedAndNestedParens(t *testing.T) {
	content := `(escaped \) paren and (nested (Tj) parens) here) 1 0 0 1 0 0 cm /Im0 Do`
	if pdfContentStreamHasText([]byte(content)) {
		t.Fatal("escaped/nested parens containing Tj bytes must not count as text")
	}
}

func TestPDFContentStreamHasText_OperatorBytesInName(t *testing.T) {
	content := "/Tj 1 0 0 1 0 0 cm /Im0 Do"
	if pdfContentStreamHasText([]byte(content)) {
		t.Fatal("a PDF name /Tj must not count as a text operator")
	}
}

func TestPDFContentStreamHasText_OperatorBytesInHexAndComment(t *testing.T) {
	if pdfContentStreamHasText([]byte("<546a20686578> 0 0 m")) {
		t.Fatal("operator-like bytes in a hex string must not count as text")
	}
	if pdfContentStreamHasText([]byte("% Tj in a comment\n0 0 m")) {
		t.Fatal("operator-like bytes in a comment must not count as text")
	}
}

func TestPDFContentStreamHasText_InlineImageBinary(t *testing.T) {
	// Inline image whose binary payload contains operator-like bytes; only a
	// later genuine text operator ends the stream.
	payload := []byte{0x00, 0xff, 0x10, 'T', 'j', 0x42, 'T', 'J', 0x99}
	content := append([]byte("q BI /W 4 /H 4 /CS /G /BPC 8 ID "), payload...)
	content = append(content, []byte("\nEI Q\nBT 1 Tf (x) Tj ET")...)
	if !pdfContentStreamHasText(content) {
		t.Fatal("text operator after an inline image must be found")
	}

	content2 := append([]byte("q BI /W 4 /H 4 /CS /G /BPC 8 ID "), payload...)
	content2 = append(content2, []byte("\nEI Q")...)
	if pdfContentStreamHasText(content2) {
		t.Fatal("operator-like bytes inside inline image binary must not count as text")
	}
}
