package kfx

// PDF content-stream scanning used by getPDFPageImage
// (yj_to_epub_resources.go).
//
// Python reference: resources.py:394-395 (get_pdf_page_image):
//
//	text = page.extract_text()
//	if text:
//	    return default_image
//
// Python uses pypdf's page.extract_text(). The exact bundled pypdf
// (calibre-plugin-modules/pypdf/_page.py, version 20260822) is token-based:
// it processes content-stream operands and operators, and text is produced
// only when a text-showing operator (Tj, TJ, ', ") receives operand data
// that decodes to glyphs. Empty strings (``() Tj``), kerning-only arrays
// (``[] TJ``, ``[-120] TJ``) etc. extract no text. Form XObjects contribute
// text only when invoked by a ``Do`` operator whose operand names them
// (_extract_text__xform, _page.py:1923/1971-2023), with cyclic references
// skipped via a known-ids set that is discarded after each recursion
// (siblings may legitimately reuse a form) and a global budget of
// MAX_XFORM_INVOCATIONS_PER_EXTRACTION = 5000 (_page.py:101).
//
// pdfcpu v0.12.0 has no text extraction API, so this scanner approximates
// extract_text() truthiness with a small operand-stack interpreter: it
// tracks string/array/name operands and reports text only for non-empty
// text operands, recursing exactly into the forms a ``Do`` invokes.
//
// Remaining conservative deviations from pypdf (documented, deliberately
// under-claiming parity):
//   - Glyph decodability is not modeled: a Tj with a non-empty string whose
//     glyphs are all whitespace/zero-width still counts as text for pypdf's
//     truthiness only if it produces characters; whitespace-only strings do
//     extract as spaces in pypdf and would count as text there, and they do
//     here too — this matches. Strings referencing entirely missing glyph
//     data can differ in principle.
//   - Type 3 font glyph procedures embedded in font charprocs are not
//     scanned; pypdf's default extraction does not descend into them either.
//
// The scanner also reports inline images (BI ... ID ... EI): pypdf's
// PageObject.images includes inline images from executed content
// (_parse_images_from_content_stream, _page.py:823+), which affects the
// "exactly one image" check and its inline-image extraction path.

// pdfOperand is a simplified content-stream operand: strings, names, arrays
// (string items tracked) and everything else.
type pdfOperand struct {
	kind  byte // 's' string, 'n' name, 'a' array, '#' other
	str   string
	items []pdfOperand // array string items, for 'a'
}

// pdfScanContent scans a decoded content stream and reports whether any text
// is shown, and whether any inline image is painted. invokeForm (optional)
// resolves a “Do“ operand name to "does this invoked Form XObject show
// text" — mirroring pypdf's _extract_text__xform recursion. Uninvoked forms
// are never consulted, matching pypdf.
func pdfScanContent(data []byte, invokeForm func(name string) bool) (hasText, hasInlineImage bool) {
	var stack []pdfOperand
	i, n := 0, len(data)

	pop := func() (pdfOperand, bool) {
		if len(stack) == 0 {
			return pdfOperand{kind: '#'}, false
		}
		op := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return op, true
	}

	for i < n {
		c := data[i]
		switch {
		case c == '%': // comment runs to end of line
			for i < n && data[i] != '\n' && data[i] != '\r' {
				i++
			}

		case c == '(':
			s, next := scanPDFLiteralString(data, i)
			i = next
			stack = append(stack, pdfOperand{kind: 's', str: s})

		case c == '<' && i+1 < n && data[i+1] == '<':
			// Dictionary: skip to matching >> so its values stay off the
			// operand stack.
			i = skipPDFDict(data, i)

		case c == '<':
			s, next := scanPDFHexString(data, i)
			i = next
			stack = append(stack, pdfOperand{kind: 's', str: s})

		case c == '[':
			items, next := scanPDFArray(data, i)
			i = next
			stack = append(stack, pdfOperand{kind: 'a', items: items})

		case c == '/':
			j := i + 1
			for j < n && isPDFRegularChar(data[j]) {
				j++
			}
			stack = append(stack, pdfOperand{kind: 'n', str: string(data[i+1 : j])})
			i = j

		case isPDFRegularChar(c):
			j := i
			for j < n && isPDFRegularChar(data[j]) {
				j++
			}
			tok := string(data[i:j])
			i = j
			switch tok {
			case "Tj", "'":
				if op, ok := pop(); ok && op.kind == 's' && len(op.str) > 0 {
					return true, hasInlineImage
				}
				stack = stack[:0]
			case "\"":
				// aw ac (string) — the string is the top operand.
				if op, ok := pop(); ok && op.kind == 's' && len(op.str) > 0 {
					return true, hasInlineImage
				}
				stack = stack[:0]
			case "TJ":
				if op, ok := pop(); ok && op.kind == 'a' {
					for _, it := range op.items {
						if it.kind == 's' && len(it.str) > 0 {
							return true, hasInlineImage
						}
					}
				}
				stack = stack[:0]
			case "Do":
				if op, ok := pop(); ok && op.kind == 'n' && invokeForm != nil {
					if invokeForm(op.str) {
						return true, hasInlineImage
					}
				}
				stack = stack[:0]
			case "BI":
				next, ok := skipPDFInlineImage(data, i)
				if !ok {
					return hasText, true // unterminated: rest is binary payload
				}
				i = next
				hasInlineImage = true
				stack = stack[:0]
			default:
				stack = stack[:0]
			}

		default: // whitespace and delimiters ) > ] } >
			if c == ')' || c == '>' || c == ']' || c == '}' {
				// Stray closing delimiter: drop the top operand defensively.
				_, _ = pop()
			}
			i++
		}
	}
	return hasText, hasInlineImage
}

// scanPDFLiteralString returns the unescaped content of a ( … ) string and
// the scan position after it.
func scanPDFLiteralString(data []byte, pos int) (string, int) {
	i := pos + 1
	n := len(data)
	depth := 1
	var b []byte
	for i < n && depth > 0 {
		c := data[i]
		switch {
		case c == '\\' && i+1 < n:
			e := data[i+1]
			switch e {
			case 'n':
				b = append(b, '\n')
			case 'r':
				b = append(b, '\r')
			case 't':
				b = append(b, '\t')
			case 'b':
				b = append(b, '\b')
			case 'f':
				b = append(b, '\f')
			case '(', ')', '\\':
				b = append(b, e)
			default:
				if e >= '0' && e <= '7' {
					v, k, cnt := 0, i+1, 0
					for k < n && cnt < 3 && data[k] >= '0' && data[k] <= '7' {
						v = v*8 + int(data[k]-'0')
						k++
						cnt++
					}
					b = append(b, byte(v))
					i = k - 2
				} else if e != '\n' {
					// '\n' after backslash is a line continuation: no character.
					b = append(b, e)
				}
			}
			i += 2
			continue
		case c == '(':
			depth++
			b = append(b, c)
		case c == ')':
			depth--
			if depth > 0 {
				b = append(b, c)
			}
		default:
			b = append(b, c)
		}
		i++
	}
	return string(b), i
}

// scanPDFHexString returns the decoded content of a < … > string and the scan
// position after it.
func scanPDFHexString(data []byte, pos int) (string, int) {
	i := pos + 1
	n := len(data)
	var nibbles []byte
	for i < n && data[i] != '>' {
		c := data[i]
		if isPDFHexDigit(c) {
			nibbles = append(nibbles, c)
		}
		i++
	}
	if i < n {
		i++ // consume '>'
	}
	if len(nibbles)%2 == 1 {
		nibbles = append(nibbles, '0')
	}
	b := make([]byte, len(nibbles)/2)
	for k := range b {
		b[k] = hexVal(nibbles[2*k])<<4 | hexVal(nibbles[2*k+1])
	}
	return string(b), i
}

func isPDFHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

// scanPDFArray collects string operands of a [ … ] array and returns the scan
// position after the closing bracket.
func scanPDFArray(data []byte, pos int) ([]pdfOperand, int) {
	i := pos + 1
	n := len(data)
	var items []pdfOperand
	for i < n {
		c := data[i]
		switch {
		case c == ']':
			return items, i + 1
		case c == '(':
			s, next := scanPDFLiteralString(data, i)
			i = next
			items = append(items, pdfOperand{kind: 's', str: s})
		case c == '<' && i+1 < n && data[i+1] == '<':
			i = skipPDFDict(data, i)
		case c == '<':
			s, next := scanPDFHexString(data, i)
			i = next
			items = append(items, pdfOperand{kind: 's', str: s})
		case c == '[': // nested array: keep its string items flat
			sub, next := scanPDFArray(data, i)
			i = next
			items = append(items, sub...)
		default:
			i++
		}
	}
	return items, i
}

// skipPDFDict skips a << … >> dictionary, honoring nesting and strings.
func skipPDFDict(data []byte, pos int) int {
	i, n := pos, len(data)
	depth := 0
	for i < n {
		c := data[i]
		switch {
		case c == '<' && i+1 < n && data[i+1] == '<':
			depth++
			i += 2
		case c == '>' && i+1 < n && data[i+1] == '>':
			depth--
			i += 2
			if depth <= 0 {
				return i
			}
		case c == '(':
			_, next := scanPDFLiteralString(data, i)
			i = next
		default:
			i++
		}
	}
	return i
}

// skipPDFInlineImage skips an inline image starting just after the BI token.
// Returns (next scan position, true), or (0, false) when no EI terminator is
// found (treat the remainder of the stream as consumed).
func skipPDFInlineImage(data []byte, pos int) (int, bool) {
	idEnd, ok := findInlineImageIDEnd(data, pos)
	if !ok {
		return 0, false
	}

	// One whitespace byte separates ID from the image data.
	i := idEnd
	n := len(data)
	if i < n && (data[i] == '\n' || data[i] == '\r' || data[i] == ' ' || data[i] == '\t') {
		i++
	}

	// The payload runs until an EI token delimited by whitespace (PDF spec:
	// at least one whitespace precedes EI; EI followed by a delimiter ends it).
	for k := i; k+2 < n; k++ {
		if data[k] == 'E' && data[k+1] == 'I' {
			before := data[k-1]
			after := byte(0)
			if k+2 < n {
				after = data[k+2]
			}
			if (before == ' ' || before == '\n' || before == '\r' || before == '\t') &&
				(!isPDFRegularChar(after)) {
				return k + 2, true
			}
		}
	}
	return 0, false
}

// findInlineImageIDEnd scans the inline image dict for the ID keyword and
// returns the scan position just after it.
func findInlineImageIDEnd(data []byte, pos int) (int, bool) {
	i, n := pos, len(data)
	for i < n {
		c := data[i]
		switch {
		case c == '%':
			for i < n && data[i] != '\n' && data[i] != '\r' {
				i++
			}
		case c == '(':
			_, next := scanPDFLiteralString(data, i)
			i = next
		case c == '<' && i+1 < n && data[i+1] == '<':
			i = skipPDFDict(data, i)
		case isPDFRegularChar(c):
			j := i
			for j < n && isPDFRegularChar(data[j]) {
				j++
			}
			if string(data[i:j]) == "ID" {
				return j, true
			}
			i = j
		default:
			i++
		}
	}
	return 0, false
}

// isPDFRegularChar reports whether c is a regular (non-delimiter,
// non-whitespace) character per PDF lexical rules.
func isPDFRegularChar(c byte) bool {
	switch c {
	case 0, '\t', '\n', '\f', '\r', ' ',
		'(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return false
	}
	return true
}
