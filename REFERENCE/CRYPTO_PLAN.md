# CRYPTO_PLAN — Kindle DRM Derivation Pipeline

## Goal

Fully reverse-engineer the Kindle DRM voucher derivation pipeline inside `libYJSDK-shared.so` to enable offline decryption without the device SDK. The target is mechanistic understanding sufficient to reimplement the derivation.

---

## Current Status

We have mapped the **entire derivation pipeline from Java entry point to AES key output**. The remaining black box is the 244KB obfuscated state machine at `0x17c4fc` which implements:
1. A deterministic decision-tree encoding of the CLIENT_ID serial into a decimal numeric string (no secret involved)
2. A one-way function combining the serial + ACSR into the HMAC key blob (secret involved)

**Next concrete steps** to crack the state machine:
1. Extract the lookup tables from the binary — find the CMP/movw/movt constants that implement the 3-class byte encoding
2. Decompile the two obfuscated crypto helpers (`0x264ac0`, `0x26398c`) — these are smaller functions called from the state machine that may contain the core PRF
3. Resolve the 43 `BLX R3` indirect calls at runtime via trampoline patching

---

## Derivation Pipeline (Confirmed)

```
INPUT STAGE:
  Java: setLockParameters({ACCOUNT_SECRET: acsr, CLIENT_ID: serial})
    → JNI bridge 0xdc9c → std::map<string,string> in RB-tree
    → CLIENT_ID value stored as std::string at f4-0x50 in RB-tree node

  Java: attachVoucher(voucher_file)
    → JNI bridge 0xd88c → vtable[5] = 0x151200 (voucher parsing)
    → Constructs strategy object (vtable 0x32f850)
    → Populates this[1] config object (vtable 0x32f248):
       +0xC0: version gate (0x76)
       +0xC4: raw voucher blob pointer (ION, magic \xe0\x01\x00\xea)
       +0xC8: atv:kin:2:<base64> device token pointer
       +0xF4: lock params std::map pointer

DERIVATION STAGE:
  fcn.0015134c → loads vtable[8] from strategy → calls 0x150c40
  0x150c40: version gate check (*(this[1]+0xC0) == 0x76) → calls 0x17c4fc

  0x17c4fc (244KB obfuscated state machine):
    Reads:
      - CLIENT_ID from RB-tree at f4-0x50 (std::string, 16 chars)
      - ACCOUNT_SECRET from RB-tree
      - Voucher blob from this[1]+0xC4
      - Device token from this[1]+0xC8
      - ACSR from /var/local/java/prefs/acsr (stage-1 key/IV)
    Produces:
      (a) Numeric string: 827-860 decimal digits (CLIENT_ID encoded via decision tree)
      (b) HMAC key blob: 9034-10330 bytes (one-way f(serial, ACSR))

CRYPTO STAGE:
  HMAC-SHA256(hmac_key_blob, numeric_string) → voucher_key_256
  AES-256-CBC(voucher_key_256, inner_IV) → decrypted voucher content
  HMAC-SHA256 integrity check (vtable[8] fires second time)
```

### Stage-1 Decrypt (Pre-state-machine)

```
ACSR from disk → double-base64-decode → 64 bytes → first 16 = IV
AES-256-CBC(
  key = "e35f5062f97cc8b1244f6f1a2414e31c" (hardcoded in libYJSDK),
  IV  = ACSR_first_16_bytes,
  ciphertext = 48 bytes from voucher
)
→ 40-char hex string "54e869c4b43348062477a52df5467be8c4e08420"
→ IDENTICAL regardless of CLIENT_ID
```

This 40-byte hex string is consumed internally by the state machine but its exact role is unclear.

---

## Numeric String Encoding (Decision Tree)

The state machine encodes each byte of the 16-char CLIENT_ID serial into a variable-length decimal token sequence using a **multi-level deterministic decision tree** (pure lookup table, no secret involved).

### Properties

- **Sequential**: serial processed left-to-right; each char contributes ~15 digits
- **Variable-length**: different chars produce different token lengths
- **3 first-level classes**: determined by byte value via hardcoded table

### First-Level Classification

| Class | Starting Pattern | Tested ASCII Values |
|-------|-----------------|-------------------|
| A | `1456864488883...` | 1(49), 4(52), 8(56), A(65), H(72), L(76), R(82), V(86) |
| B | `4488861456883...` | 2,3,5,6,7,C,E,F,G,I,J,K,O,P,Q,S,T,U,Y,Z,c (20+ values) |
| C | `4488883223760...` | 0(48), 9(57), B(66), D(68), M(77), N(78), W(87), X(88), a(97), b(98) |

**No simple arithmetic function** (mod, XOR, multiply) maps byte→class. It's a hardcoded lookup table in the obfuscated code.

### Kindle Serial Character Map

```
Char: 0 1 2 3 4 5 6 7 8 9
Map:  C A B B A B B B A C

Char: A B C D E F G H I J K L M N O P Q R S T U V W X Y Z
Map:  A C B C B B B A B B B A C C B B B A B B B A C C B B

Char: a b c
Map:  C C B
```

### Sub-Grouping

Within each class, deeper sub-classification occurs:
- Pattern A: `{4,8,H,L}` share 348-char prefix; `V` shares only 15 chars (different deep sub-class)
- Pattern C: `{B,b}` share 851-char prefix; `{M,W,a}` share ~94 chars

### 99.5% Variability

Only 4 isolated single-char constant positions exist in the 827-860 char numeric string. "Islands of stability" (36-char common substrings) are statistical artifacts, not structural boundaries.

---

## HMAC Key Blob (Secret-Bearing Component)

The HMAC key blob (variable size, 10330 bytes for baseline) is derived from **both** CLIENT_ID serial and ACSR account secret via a one-way function inside the state machine.

### Differential Analysis

**Baseline vs off-by-1** (differ only in last serial digit, 15/16 chars shared):
| Region | Bytes | Match? |
|--------|-------|--------|
| 0–383 | 384 | ✅ IDENTICAL |
| 384–7835 | 7450 | ❌ 99%+ different |
| 7836–9043 | 1207 | ✅ IDENTICAL |
| 9044–9214 | 170 | ❌ Different |
| 9215–10330 | 1115 | ✅ IDENTICAL |

Total: 31.6% match (3265/10330 bytes). Changing 1 serial character cascades to 68% of the blob.

**Completely different serials**: 99.4%+ of bytes differ (essentially random).

---

## Obfuscated State Machine (0x17c4fc)

### Characteristics

- **Size**: 243,762 bytes (244KB) in a single function
- **Obfuscation**: Control-flow flattening (136+ switch cases), anti-debug (`software_udf`), opaque predicates
- **Dispatch**: 43 `BLX R3` indirect calls (obfuscated dispatch), 25 unique direct `BL` targets
- **Internal helpers**: `0x17ca40` (switch handler), `0x17e33e` (helper), `0x1ae690` (trap)
- **External crypto helpers**: `0x264ac0` (4 calls, constants 0x89259a5d, 0xe4300578, 0xf15c8d93, 0xebe2a8b7), `0x26398c` (2 calls, constants 0x4528a1e4, 0x023d2a8d, 0xe1029034, 0x0213bff3)
- **Standard crypto constants NOT matched**: SHA-256, SHA-1, MD5, MT19937 — these are custom DRM-specific values

### Call Graph

Top direct BL targets from 0x17c4fc:
| Address | Calls | Function |
|---------|-------|----------|
| 0x1818e0 | 8 | std::vector::insert (realloc) |
| 0x181b50 | 5 | std::vector (nested indexing) |
| 0x264ac0 | 4 | **Obfuscated crypto helper** |
| 0x26398c | 2 | **Obfuscated crypto helper** |
| 0x181a78 | 2 | std::vector allocation |
| 0x15fc58 | 2 | std::vector::push_back |

---

## Key Functions

| Address | Role |
|---------|------|
| `0xdc9c` | JNI: `setLockParameters` (0x280 bytes, parses key/value pairs) |
| `0xd88c` | JNI: `attachVoucher` (0x64 bytes, calls vtable[5]) |
| `0xd8f0` | JNI: `getInstance` (0x7c bytes) |
| `0xd81c` | JNI: `setAccountSecrets` (0x70 bytes, **no-op for crypto**) |
| `0xd9e4` | JNI: `getSupportedVoucherVersions` (0x118 bytes) |
| `0x151200` | Voucher parsing loop → constructs strategy object → calls 0x15134c |
| `0x15134c` | Bridge: `ldr r3,[r0]; ldr r3,[r3,#0x20]; blx r3` (calls vtable[8]) |
| `0x150c40` | vtable[8] dispatcher: checks `this[1]+0xC0==0x76` → 0x17c4fc |
| `0x17c4fc` | **244KB obfuscated state machine** (the core derivation) |
| `0x137cb7` | Stage-1 AES init (hardcoded key + ACSR IV) |
| `0x151518` | HMAC#1 call (key derivation) |
| `0x151481` | HMAC#2 call (integrity check) |
| `0x1b27d4` | Shared EVP decrypt helper |

### Vtable at 0x32f850 (DRMSdk::VoucherDecryption)

| Slot | Offset | Address | Role |
|------|--------|---------|------|
| 0 | 0x00 | 0x15fb30 | destructor? |
| 5 | 0x14 | 0x151200 | attachVoucher |
| 8 | 0x20 | 0x150c40 | **crypto dispatcher** |
| 9 | 0x24 | 0x158864 | stub (24 bytes) |

---

## Perturbation Matrix (7-Case Experiment)

| Case | setAcctSecrets | Lock Params | Attach | Stage-1 AES | HMAC key len | Error |
|------|---------------|-------------|--------|-------------|-------------|-------|
| baseline | correct ACSR | ACCT_SEC+CID=serial | ✅ | same | 10330 | — |
| wrong_cid | correct ACSR | ACCT_SEC+CID=XXXXXX | ❌ | same | 9690 | Err48 |
| no_cid | correct ACSR | ACCT_SEC only | ❌ | same | 0 | Err43 |
| wrong_as | WRONG ACSR | ACCT_SEC+CID=serial | ✅ | same | 10330 | — |
| no_as | correct ACSR | CID only | ❌ | none | 0 | Err43 |
| no_lock | correct ACSR | (skipped) | ❌ | none | 0 | Err43 |
| no_secrets | (skipped) | ACCT_SEC+CID=serial | ✅ | same | 10330 | — |

**Key findings:**
- `setAccountSecrets()` is a **no-op for crypto** — SDK reads ACSR directly from `/var/local/java/prefs/acsr`
- ACCOUNT_SECRET in lock params must be PRESENT but VALUE is ignored
- CLIENT_ID value directly affects derivation — wrong value → ErrorCode 48
- Stage-1 AES key/IV constant across ALL cases

---

## Device Context

| What | Value |
|------|-------|
| Device | Kindle PW6 (sangria/bellatrix4) |
| Firmware | 5.18.5 |
| Serial | GR733X1151821324 |
| SSH | root@10.0.0.103:5132 |
| ACSR | `/var/local/java/prefs/acsr` (121 bytes) |
| Vouchers | `/mnt/us/documents/Downloads/Items01/*/sdr/assets/voucher` |
| All vouchers | `@10005.0`, lock params `ACCOUNT_SECRET + CLIENT_ID` |
| SDK versions | 39 reported, NONE are 5-digit (V10005 uses separate path) |
| Voucher moved | Was `/mnt/us/Items01/`, now `/mnt/us/documents/Downloads/Items01/` |

---

## Firmware Context

| Firmware | Library | Size | DRMSdk strings | ABI |
|----------|---------|------|----------------|-----|
| PW2 5.12.2.2 | libYJSDK | 2.6MB | Lacks explicit newer strings | armel (soft-float) |
| PW12 5.17.1.0.4 | libYJSDK | 3.0MB | DRMSdk::VoucherDecryption, metrics keys | armhf |
| PW6 5.18.5 (live) | libYJSDK | 3.3MB | Full DRMSdk instrumentation | armhf |

PW2 requires **armel build** (soft-float) — no logic changes needed, only build target.

---

## DeDRM Context (Issue #993)

- Older devices receiving `VoucherEnvelope@10014.0` — fails in DeDRM
- DeDRM maps `CLIENT_ID → self.dsn` (device serial) — **likely wrong for V10014**
- DeDRM has no `process_V10014()` or V10014 entry in `OBFUSCATION_TABLE`
- 5-digit voucher versions use a **completely different code path** not reflected in `getSupportedVoucherVersions()`
- V3972 appears in SDK list but NOT in DeDRM's table

The server-side change hypothesis: Amazon switched voucher issuance globally; old firmware already has the SDK to handle it; DeDRM's offline approach can't reproduce the derivation.

---

## Instrumentation

### Hooks deployed on device

All in `/mnt/us/` — built with Docker armhf cross-compilation:
```
docker run --rm -v /tmp:/tmp arm32v7/python:3.11-slim-bookworm sh -c \
  'apt-get update -qq && apt-get install -y -qq gcc libc6-dev && \
   gcc -shared -fPIC -O2 -mthumb -o /tmp/<name>.so /tmp/<name>.c -ldl -pthread'
```

### Key hook: `phase_chosen.so`
- Patches vtable slot 8 (data pointer replacement, mprotect .data.rel.ro writable)
- Captures HMAC key blob + numeric string for first HMAC call
- Saves to `/mnt/us/chosen_<label>_hmac_key.bin` and `chosen_<label>_hmac_data.bin`

### Captured data

- `/tmp/chosen_input/` — 7-case baseline/wrong-id experiment
- `/tmp/fc2_data/` — 33-case first-character sweep (B-Z, a-c, 2-9)
- `/tmp/phase_c_baseline/` and `/tmp/phase_c_wrong_cid/` — binary dumps of `this`, `this[1]`, HMAC data

---

## Open Questions

1. **What are the obfuscated crypto helpers doing?** `0x264ac0` (4 calls) and `0x26398c` (2 calls) use movw/movt constant construction with custom DRM constants. These are the most likely candidates for the core PRF/hash that combines serial + ACSR into the HMAC key blob.

2. **What are the 43 BLX R3 dispatch targets?** Resolving these at runtime would map the state machine's internal control flow. Blocked by 2-byte Thumb instruction size (BLX R3 = 2 bytes, BL trampoline = 4 bytes). Options: find code caves within ±2KB, or use a different patching strategy.

3. **Where exactly does ACSR enter the state machine?** We know it affects the HMAC key blob but not the numeric string. The entry point is somewhere inside `0x17c4fc`.

4. **What role does the stage-1 output play?** The 40-byte hex string `54e869c4...8420` is produced before the state machine runs and is consumed internally. It may be a key component of the HMAC key blob derivation.

---

## Archived Material

The full chronological experiment log (2325 lines of raw notes) is preserved in `REFERENCE/CRYPTO_PLAN_raw_log.md`.
