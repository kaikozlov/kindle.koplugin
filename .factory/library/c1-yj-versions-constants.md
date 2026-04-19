# yj_versions.go — Version Constants & Validation

## What's Ported

`internal/kfx/yj_versions.go` — Full port of `yj_versions.py` (1124 lines).

### Key Design Decisions

1. **ANY sentinel**: Python's `ANY = True` is a bool, but `True == 1` in Python. The `IsKnownFeature` function replicates this by checking both `AnyVersionKey()` and `IntVersionKey(1)` in the version map. This matches Python's `ANY in vals` behavior where `True in {1: ...}` evaluates to `True`.

2. **VersionKey type**: Python uses mixed dict keys (int, tuple, bool). Go uses a `VersionKey` struct with `IntVal`, `Tuple [2]int`, `IsTuple`, and `IsAny` fields. Use `IntVersionKey(n)`, `TupleVersionKey(a,b)`, or `AnyVersionKey()` to construct keys.

3. **metadataValueSet**: Python uses `ANY` sentinel or specific sets (set of strings, ints, floats, bools). Go uses a struct with typed maps and an `IsAny` flag.

4. **KINDLE_VERSION_CAPABILITIES**: The `5.14.3.2` entry maps to a single string (not a slice), matching Python where it's a single value, not a list. Use type assertion when iterating.

5. **KNOWN_KCB_DATA metadata references**: `edited_tool_versions` and `tool_version` in the `metadata` category share the `creator_version` string set from `KNOWN_METADATA["kindle_audit_metadata"]["creator_version"]`. This is populated in `init()`.

### Actual Counts (from Python)

| Data Structure | Count |
|---|---|
| Feature name string constants | 73 (plus NMDL tuple constants ported as strings → 92 total in Go) |
| KNOWN_KFX_GENERATORS | 50 entries |
| GENERIC_CREATOR_VERSIONS | 3 entries |
| KNOWN_SUPPORTED_FEATURES | 5 entries |
| KNOWN_FEATURES categories | 4 (format_capabilities, SDK.Marker, com.amazon.kindle.nmdl, com.amazon.yjconversion) |
| KNOWN_FEATURES keys in yjconversion | 44 |
| KNOWN_METADATA categories | 8 |
| KNOWN_AUXILIARY_METADATA keys | 36 |
| KINDLE_VERSION_CAPABILITIES entries | 23 |
| KINDLE_CAPABILITY_VERSIONS entries | 64 |
