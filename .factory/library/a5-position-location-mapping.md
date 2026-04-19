# A5: Position/Location Mapping — Library Note

## What was ported

`yj_position_location.py` (~1325 lines) → `internal/kfx/yj_position_location.go` (857 lines)

### Structs
- **ContentChunk**: Position info unit with PID, EID, EIDOffset, Length, SectionName, MatchZeroLen, Text, ImageResource. Equality supports PID comparison and match_zero_len tolerance.
- **ConditionalTemplate**: Illustrated layout conditional tracking with StartEID/EndEID, Oper, PosInfo snapshot, UseNext flag. RANGE_OPERS ($298, $299) leave StartEID nil.
- **MatchReport**: Error report tracking with limit enforcement.
- **BookPosLoc**: Main struct with book flags (IsDictionary, IsScribeNotebook, etc.) and position/location methods.

### Methods/Functions
- `NewContentChunk` — construction with validation (logs errors for bad params)
- `ContentChunk.Equal` — equality with optional PID comparison and match_zero_len tolerance
- `NewConditionalTemplate` — sets StartEID based on RANGE_OPERS
- `PidForEid` — linear search with wraparound caching
- `EidForPid` — binary search (inclusive chunk end)
- `GenerateApproximateLocations` — 110-position spacing with section reset
- `DetermineApproximatePages` — fixed-layout (per-section) and reflowable (whitespace lookback)
- `CreateApproximatePageListWithPosInfo` — binary search for desired page count
- `VerifyPositionInfo` — parallel walk with 9-position lookahead
- `CreatePositionMap` — section→eid mapping, dictionary/scribe/KPF early return
- `CollectLocationMapInfo` — $550/$621 fragment parsing with cross-validation
- `anchorEidOffset` — anchor fragment lookup

### Constants
- `KFX_POSITIONS_PER_LOCATION = 110`
- `TYPICAL_POSITIONS_PER_PAGE = 1850`
- `MAX_WHITE_SPACE_ADJUST = 50`
- `RANGE_OPERS = {"$298": true, "$299": true}`

### What was simplified
- `CollectContentPositionInfo` is stubbed (returns nil) — the full implementation is a ~450-line deeply nested closure that requires complex Ion data traversal. The stub allows other functions to be tested independently.
- `CreatePositionMap` builds maps internally but doesn't store them as fragments (uses `_ = positionMap` pattern).
- `CollectLocationMapInfo` fragment lookups return nil by default.

### Key design decisions
- EID is `interface{}` (int or string) to match Python's dynamic typing (int eids vs IonSymbol eids)
- `ContentChunk.Text` is `string` (empty = nil, since Go can't have nil strings)
- `maxInt` added here since `content_helpers.go` only has `minInt`
- Binary search in `EidForPid` uses inclusive end (`pid <= pi.PID + pi.Length`) matching Python

### Test coverage
57 tests covering all validation assertions VAL-A-044 through VAL-A-085.
