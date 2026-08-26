package kfx

// PDF content-stream text detection used by getPDFPageImage
// (yj_to_epub_resources.go).
//
// Python reference: resources.py:394-395 (get_pdf_page_image):
//
//	text = page.extract_text()
//	if text:
//	    return default_image
//
// Python uses pypdf's full text extraction. pdfcpu v0.12.0 has no text
// extraction API, so this scanner approximates the check by looking for the
// four text-showing operators (Tj, TJ, ', ") in the page's decoded content
// streams (and in any Form XObjects they invoke). A page that paints glyphs
// must use one of these operators (possibly via a Type 3 glyph procedure),
// so this is a faithful proxy for "page.extract_text() is non-empty" for the
// purpose of the check: identifying pages that are *not* a bare single image.
//
// The scanner is token-aware: PDF comments, literal strings (with escapes and
// nesting), hex strings, names (/Tj) and inline image binary data (BI ... ID
// ... EI) are skipped so that operator-like bytes inside operands or binary
// payloads cannot produce false positives.

// pdfContentStreamHasText reports whether a decoded PDF content stream shows
// any text (Tj, TJ, ' or " operator present).
func pdfContentStreamHasText(data []byte) bool {
	i, n := 0, len(data)
	for i < n {
		c := data[i]
		switch {
		case c == '%': // comment runs to end of line
			for i < n && data[i] != '\n' && data[i] != '\r' {
				i++
			}

		case c == '(': // literal string: escapes + paren nesting
			i++
			depth := 1
			for i < n && depth > 0 {
				if data[i] == '\\' {
					i += 2
					continue
				}
				if data[i] == '(' {
					depth++
				} else if data[i] == ')' {
					depth--
				}
				i++
			}

		case c == '<' && i+1 < n && data[i+1] == '<': // dictionary open
			i += 2

		case c == '<': // hex string
			i++
			for i < n && data[i] != '>' {
				i++
			}
			i++

		case c == '/': // name (including operator-like names such as /Tj)
			i++
			for i < n && isPDFRegularChar(data[i]) {
				i++
			}

		case isPDFRegularChar(c):
			j := i
			for j < n && isPDFRegularChar(data[j]) {
				j++
			}
			tok := data[i:j]
			i = j
			switch string(tok) {
			case "Tj", "TJ", "'", "\"":
				return true
			case "BI":
				// Inline image: skip the dict and the binary payload up to EI
				// so raw image bytes cannot be mistaken for operators.
				next, ok := skipPDFInlineImage(data, i)
				if !ok {
					return false
				}
				i = next
			}

		default: // whitespace and other delimiters ( ) < > [ ] { }
			i++
		}
	}
	return false
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
			i++
			depth := 1
			for i < n && depth > 0 {
				if data[i] == '\\' {
					i += 2
					continue
				}
				if data[i] == '(' {
					depth++
				} else if data[i] == ')' {
					depth--
				}
				i++
			}
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
