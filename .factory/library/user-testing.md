# User Testing: EPUB Comparison Validation

## Validation Surface

**Surface:** EPUB file comparison (Go output vs Calibre Python reference)

**Tools:** bash scripts that build Go binary, convert KFX→EPUB, unzip, and diff text files.

**How it works:**
1. Build Go binary: `go build ./cmd/kindle-helper/`
2. Convert KFX to EPUB: `./kindle-helper convert --input INPUT --output OUTPUT`
3. Unzip both Go and Python EPUBs
4. Strip `dcterms:modified` timestamps
5. Diff all non-binary files (exclude jpg, png, gif, svg, fonts)
6. Count matches vs diffs

## Validation Concurrency

**Max concurrent validators:** 1 (sequential comparison)
**Rationale:** EPUB comparison is CPU-bound (Go build + conversion) and I/O-bound (unzip + diff). Running multiple comparisons provides no benefit since each test book is independent but shares the Go binary. The comparison scripts run all books sequentially.

## Resource Cost

- Go build: ~5s, ~200MB RAM
- Per-book conversion: ~2-10s, ~100MB RAM (varies by book size)
- Full comparison (6 books): ~60s total
- Full comparison (4 new books): ~40s total

## Test Commands

```bash
# Original 6 books (must be 394/394)
bash /tmp/compare_all.sh

# New 4 books (target: 0 diffs)
bash /tmp/compare_new.sh

# Single book detailed diff
scripts/diff_kfx_parity.sh "BOOK_NAME"

# Unit tests
go test ./internal/kfx/ -count=1 -timeout 120s
```

## Pass Criteria

- All assertions in validation-contract.md must be "passed"
- Original 6 books: 394/394 matching, 0 diffs
- New 4 books: 0 text diffs (image encoding differences are acceptable if visual quality matches)
- go test ./internal/kfx/ passes with 0 failures
