# Python Porting Guide

Methodology for porting Python functions from the Calibre KFX Input reference to Go.

## What belongs here

Rules, patterns, and conventions for the 1:1 Python→Go porting process.

---

## Three-Fold Parity Rule

1. **Structural**: Python file `yj_structure.py` → Go file `yj_structure.go` (or split into matching files)
2. **Function-level**: Python `classify_symbol()` → Go `classifySymbol()` with same name and purpose
3. **Logic-level**: Same conditionals, same order, same return values

## Naming Conventions

| Python | Go |
|--------|-----|
| `snake_case` functions | `camelCase` methods/functions |
| `UPPER_CASE` constants | `UpperCase` exported constants |
| `self.field` | `s.field` or `receiver.field` |
| `ClassName` | `structName` |
| `True/False` | `true/false` |
| `None` | `nil` |

## Data Structure Mapping

| Python | Go |
|--------|-----|
| `dict[str, T]` | `map[string]T` |
| `set[str]` | `map[string]bool` or `map[string]struct{}` |
| `list[T]` | `[]T` |
| `tuple` | struct with named fields |
| IonSymbol | string (symbols are resolved to text) |
| IonStruct | map with string keys |
| IonList | `[]interface{}` or typed slice |
| Class with `__init__` | struct with fields |

## Error Handling

- Python's `log.error(...)` → Go's `log.Printf(...)` or structured logging
- Python's `log.warning(...)` → Go's `log.Printf(...)` with warning prefix
- Never panic on expected errors; log and continue like Python does
- Go error returns where Python returns None/empty on error

## Ion Data Handling

The Go code uses `github.com/amazon-ion/ion-go` for Ion parsing. The Python code uses custom Ion classes.

Key differences:
- Python `IonSymbol("name")` → Go uses symbol table resolution, text is a `string`
- Python `IonStruct({key: val})` → Go uses typed structs or `map[string]interface{}`
- Python `isinstance(x, IonSymbol)` → Go type assertions or type switches

## Testing Rules

- Every ported function must have synthetic Go unit tests
- Tests must NOT depend on KFX fixture files
- Construct test data using Go literal syntax
- Test each conditional branch in the Python source
- Verify behavior matches, not implementation details

## Commit Discipline

- Commit after every step (success or failure)
- Never accumulate more than one step of uncommitted changes
- If a change introduces unexpected test failures, REVERT immediately
- Python code is READ ONLY — never modify it

## IonSExp vs IonList Distinction

Python's Ion types use a class hierarchy (IonSExp and IonList both extend list) to differentiate between S-expressions and lists. Go's type system cannot distinguish `[]interface{}` as List vs SExp at runtime. When processing real KFX Ion data, a tagging mechanism (e.g., wrapper structs `IonSExp{items []interface{}}` and `IonList{items []interface{}}`) will be needed. The SExp dispatch semantics differ from List: IonSExp uses `data[0]` as an operator and iterates `data[1:]`, while IonList iterates all elements.

Reference: `yj_structure.py` `walk_fragment()` dispatch at line ~880.

## Detailed Reference Documents

Workers should read the appropriate stream's reference document before starting:
- `.factory/research/stream-a-reference-details.md` — Stream A (Core Conversion Fidelity)
- `.factory/research/stream-b-reference-details.md` — Stream B (EPUB Output Quality)
- `.factory/research/stream-cd-reference-details.md` — Stream C+D (Version Registry, Niche Features)
