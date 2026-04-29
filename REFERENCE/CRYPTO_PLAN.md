# CRYPTO_PLAN

## Goal

Understand why older Kindle firmware (especially Paperwhite 2 on 5.12.2.2) is now receiving books with the newer DRM voucher format seen in DeDRM issue #993, determine whether the change is server-side or device-side, and define the shortest path to recover the real `CLIENT_ID` / voucher decryption inputs if we ever want an offline path.

---

## Executive Summary

Current best explanation:

- Amazon did **not** need to "infect" old firmware with a new KFX payload or a hidden DRM update.
- The most likely change is **server-side voucher issuance**: Amazon started delivering vouchers using newer envelope versions and `CLIENT_ID`-based locking.
- Older device firmware appears to already contain enough DRM logic to handle this on-device.
- DeDRM fails because it is trying to reproduce the decryption path offline with incomplete assumptions:
  - it treats `CLIENT_ID` like the device serial / DSN,
  - and it does not appear to have a known `V10014`-specific obfuscation/KDF path.
- Our KOReader + `LD_PRELOAD` approach is still the right direction because it intercepts the **actual AES keys in use** and does not need to guess the derivation.

---

## Findings So Far

## 1. GitHub issue #993 matches our overall approach

Issue: `https://github.com/noDRM/DeDRM_tools/issues/993`

Observed in the issue:

- Older devices on **5.12.2.2** started receiving books that now fail in DeDRM.
- Newer DeDRM logs show:
  - voucher type parses as `com.amazon.drm.VoucherEnvelope@10014.0`
  - lock parameters used are `['CLIENT_ID']`
  - decryption then fails with wrong-key / padding errors
- Satsuoni suggested that the only promising Linux/on-device path would likely be an `LD_PRELOAD` approach.

That is essentially what this project already does:

- inject `lib/crypto_hook.so` via `LD_PRELOAD`
- run `/usr/java/bin/cvm`
- trigger the on-device DRM SDK with `lib/KFXVoucherExtractor.jar`
- capture the AES material from the real device path

Conclusion:

- The DeDRM issue reinforces that our direction is structurally sound.
- The remaining question is not whether `LD_PRELOAD` is valid, but whether we can also understand enough of the new path to explain it or reproduce parts of it offline.

---

## 2. PW2 firmware is structurally compatible with our runtime approach

Analyzed firmware:

- `REFERENCE/update_kindle_paperwhite_v2_5.12.2.2.bin`
- extracted to `REFERENCE/firmware_pw2_5.12.2.2/`

Confirmed on PW2 rootfs:

- `/usr/java/bin/cvm` exists
- `/usr/java/lib/arm/minimal/libjvm.so` exists
- `/proc/usid` is a standard Kindle proc entry (confirmed via `idme` strings)
- `/var/local/java/prefs/` is the normal runtime Java prefs area
- `/var/local` is mounted from the persistent local partition at boot
- `glibc` is **2.20**
- `libcrypto.so.1.0.0` exists and exports the exact symbols our hook uses:
  - `EVP_DecryptInit_ex`
  - `EVP_EncryptInit_ex`
  - `AES_set_decrypt_key`
  - `AES_set_encrypt_key`
  - `EVP_aes_128_cbc`
  - `EVP_aes_256_cbc`
- `libYJSDK-shared.so` exists and references the EVP AES decrypt path
- `YJReader-impl.jar` contains the same Java API we use today:
  - `com.amazon.yjreadersdk.BookSecurity`
  - `com.amazon.yjreadersdk.interfaces.IBookSecurity`

`IBookSecurity` on PW2 exposes the same methods we rely on:

- `setAccountSecrets(String)`
- `attachVouchers(List)`
- `setLockParameters(Map)`
- `getSupportedVoucherVersions()`
- `getVersion()`

Important runtime detail:

- PW2 `framework` already exports:
  - `LD_PRELOAD="$JLIB/arm/libdlopen_global.so"`
- So our hook would need to **append** to existing `LD_PRELOAD`, not replace it.

### Only blocker discovered on PW2

The blocker is **build ABI**, not logic.

- PW2 `cvm` / system libs are **armel / soft-float**
  - interpreter: `/lib/ld-linux.so.3`
  - ELF flags indicate soft-float ABI
- Newer PW12 firmware is **armhf / hard-float**
  - interpreter: `/lib/ld-linux-armhf.so.3`
- Our current hook / helper build is **armhf**.

Implication:

- No code changes are currently required for PW2 logic.
- But to run on PW2 we need an **armel build** of:
  - `lib/crypto_hook.so`
  - the Python helper bundle / wrapper
- This is a build-target problem, not an architectural mismatch.

---

## 3. There is no evidence that KFX itself is carrying a DRM updater payload

What we checked:

- firmware-side content pack services
- KFX/container code paths in our local `python/kfxlib/`
- content pack strings in PW2 firmware

Findings:

- PW2 `contentpackd` appears focused on **fonts / locale / mounted content packs**, not DRM SDK replacement.
- `libcontentpackd.so` strings reference:
  - `/mnt/us/system/fonts`
  - `/usr/share/locale`
  - font update/remount flows
- We found no convincing evidence that KFX book containers include a generic mechanism to ship or install DRM runtime updates.
- KFX is still best understood as a **content container**, not an update channel for core DRM libraries.

Conclusion:

- The current evidence argues **against** a book-embedded DRM updater mechanism.
- If old devices are seeing new voucher behavior, the simplest explanation is still **server-side voucher changes**, not a sideloaded SDK update hidden in the book.

---

## 4. DeDRM already knows about `VoucherEnvelope@10014.0` as a type name, but not necessarily how to decrypt it

From `REFERENCE/DeDRM_tools/DeDRM_plugin/ion.py`:

- the protected symbol table already includes voucher envelope ranges up through `10001..11110`
- that explains why the newer fork can parse `VoucherEnvelope@10014.0`

But parsing is not the same as decrypting.

Key observation from `decryptvoucher()`:

- DeDRM constructs a `shared` blob from:
  - `PIDv3`
  - enc algorithm / transformation / hash names
  - lock parameters and corresponding values
- for `ACCOUNT_SECRET`, it appends the account secret
- for `CLIENT_ID`, it currently appends `self.dsn`

Relevant code path:

- if `param == "CLIENT_ID"` then `shared += param.encode('ASCII') + self.dsn`

That is a strong clue that PC-side DeDRM is assuming:

- `CLIENT_ID == device serial / DSN`

That assumption now looks doubtful.

### Working hypothesis

The real `CLIENT_ID` in the newer vouchers is likely:

- a registration token,
- or some server-issued / account-bound device identifier,
- not just the hardware serial.

That would explain why:

- the device can still use it,
- but offline DeDRM fails even when it knows the envelope type.

---

## 5. `V10014` likely adds another missing piece beyond `CLIENT_ID`

This is not yet proven, but it is a strong hypothesis.

What we found in `ion.py`:

- `OBFUSCATION_TABLE` has mappings for:
  - V1..V28
  - several hardcoded 4-digit versions like `9708`, `1031`, `2069`, etc.
- there is **no explicit `V10014` entry**
- there is also no `process_V10014()` helper

So even though DeDRM can parse the envelope annotation, it does **not obviously have a known V10014-specific KDF/obfuscation path**.

Current best interpretation:

- offline failure may be caused by the wrong `CLIENT_ID`
- or by a missing `V10014` scramble/KDF
- or by both

This means:

- finding the real `CLIENT_ID` alone may or may not be sufficient
- but it is still one of the most useful immediate things to learn

---

## 6. PW2 and PW12 `libYJSDK` are not identical

Observed differences:

- PW2 `libYJSDK-shared.so` is ~2.6 MB
- PW12 `libYJSDK-shared.so` is ~3.0 MB
- PW12 contains many explicit DRMSDK-related symbols / strings, including:
  - `DRMSDK::VoucherDecryption`
  - `VoucherVersionIDKey`
  - `VoucherParseErrorCodeKey`
  - `NoahLib`
  - `ZSProtectionAlerts`
- PW2 lacks these explicit newer DRMSDK strings

Interpretation:

- newer devices likely have a more instrumented / expanded DRM implementation
- older devices still appear to expose the same **Java `BookSecurity` API**, which is enough for our hook-based runtime path
- the lack of explicit newer strings on PW2 does **not** prove inability to process the new vouchers, only that the internal implementation differs

This is a useful caution:

- we should not assume the exact same internal derivation code across generations
- but we also do not currently need that assumption for the runtime hook path

---

## 7. Best current explanation for the "infection"

Most likely sequence:

1. Amazon changes voucher issuance on the server.
2. New downloads arrive with `VoucherEnvelope@10014.0`.
3. The lock parameters now emphasize `CLIENT_ID` instead of the old path DeDRM could reproduce.
4. Older devices still have enough built-in DRM capability to consume the vouchers on-device.
5. PC-side DeDRM cannot reproduce the derivation because it lacks:
   - the real `CLIENT_ID`,
   - and possibly the right V10014 KDF/obfuscation behavior.

So the old firmware was probably not "updated by the book".

It was more likely:

- already compatible enough for Amazon's new server-issued voucher style,
- while external tools were left behind.

---

## Confidence Levels

### Confirmed

- PW2 runtime paths and Java API are compatible with our current logic.
- PW2 requires an armel build, not logic changes.
- `CLIENT_ID` is now appearing in live vouchers from the DeDRM issue logs.
- DeDRM maps `CLIENT_ID` to `self.dsn`.
- DeDRM has no obvious explicit `V10014` handler.
- No evidence found for KFX as a DRM runtime updater.
- No evidence found for `contentpackd` replacing DRM libraries.

### Likely

- old devices are receiving newer vouchers because of a server-side change.
- the real `CLIENT_ID` is not just the hardware serial.
- V10014 likely needs either a new KDF/scramble or a different interpretation of lock inputs.

### Unknown

- whether the real `CLIENT_ID` is stored plainly or derived on-device
- whether correct `CLIENT_ID` alone is enough to make offline decryption work
- whether PW2 and PW12 share the same V10014 derivation internally

---

## Investigation Plan

## Phase 0 - Prerequisite: make PW2 runnable

Goal: get our existing runtime approach onto PW2 for direct observation.

Tasks:

1. Produce an **armel** build instead of the current armhf build.
2. Update the build to use the soft-float runtime / linker.
3. Keep code unchanged unless runtime testing proves otherwise.

Expected outcome:

- same logic,
- different target ABI,
- direct PW2 testing becomes possible.

---

## Phase 1 - Observe what the device really knows

This is the highest-value next step.

### 1.1 Ask the SDK what voucher versions it supports

We already know `IBookSecurity` exposes:

- `getSupportedVoucherVersions()`
- `getVersion()`

Immediate experiment:

- add a tiny Java helper (or temporarily extend `KFXVoucherExtractor.java`) to print:
  - `sec.getVersion()`
  - `Arrays.toString(sec.getSupportedVoucherVersions())`

Why this matters:

- it directly tells us whether the older device SDK openly reports support for newer voucher versions
- it helps separate "server-side change only" from "some hidden device update happened"

### 1.2 Search the device for candidate `CLIENT_ID` material

Likely locations to inspect on a jailbroken device:

- `/var/local/java/prefs/`
- `/var/local/`
- `/keys/`
- sqlite databases under `/var/local/`
- registration / auth cookie stores
- any Java prefs keys referencing registration, client, account, auth, or device IDs

Goal:

- identify a value that is stable enough to be the real `CLIENT_ID`

### 1.3 Instrument the Java invocation path

Add temporary logging around:

- `BookSecurity.getNativeInstance()`
- `setAccountSecrets(...)`
- `setLockParameters(...)`
- `attachVouchers(...)`

Goal:

- confirm exactly which inputs we hand to the SDK,
- and whether the SDK mutates / supplements them internally.

### 1.4 Expand the native hook if needed

Current hook captures AES setup. That is already valuable.

If necessary, extend tracing to include:

- HMAC functions (`HMAC_Init_ex`, `HMAC_Update`, `HMAC_Final`)
- SHA256 paths
- any KDF-related EVP entry points

Goal:

- catch the material immediately before voucher unwrap,
- determine whether the unknown is the `CLIENT_ID`, the KDF, or both.

Note:

- this will be noisy, so keep instrumentation narrow and easy to disable.

---

## Phase 2 - Static reverse engineering

If runtime logs are not enough, move to RE.

### 2.1 Compare PW2 vs PW12 around `BookSecurity`

Targets:

- `YJReader-impl.jar`
- `libYJSDK-shared.so`

Questions:

- where does `CLIENT_ID` come from internally?
- is there a code path that resolves it from registration state?
- do PW2 and PW12 diverge in voucher decrypt internals?

### 2.2 Locate V10014-specific logic

Search strategy:

- identify voucher-version branches in native code
- look for code adjacent to AES/HMAC setup
- correlate with `getSupportedVoucherVersions()` results

Questions:

- is `V10014` a new scramble path?
- is it just a new version label over an older derivation?
- is the actual change entirely in lock parameter meaning?

### 2.3 Diff for registration-token accessors

Focus on strings / symbols related to:

- registration
- client
- account auth
- cookies / prefs
- device GUID / account-bound identifiers

Goal:

- find the on-device source of the true `CLIENT_ID`

---

## Phase 3 - Test offline reproduction

Only do this after we have runtime evidence.

### 3.1 Patch a local test harness

Create a local experiment path that can:

- override `CLIENT_ID`
- try candidate values from device state
- optionally plug in a V10014-specific KDF once understood

### 3.2 Replay captured vouchers

Use:

- a real voucher from a device test case
- the captured AES result from our hook as ground truth

Success condition:

- offline reproduction yields the same key material / successful voucher unwrap

### 3.3 Decide whether it is worth upstreaming

Possible outcomes:

- runtime-only path remains the pragmatic answer
- or enough is learned to help noDRM/DeDRM recover a PC-side path

---

## Concrete Next Steps

If we only do a few things next, they should be these:

1. **Build armel artifacts for PW2** so we can run our current path there.
2. **Print `getSupportedVoucherVersions()` and `getVersion()`** from `IBookSecurity` on-device.
3. **Search `/var/local/java/prefs/` and related stores** for a candidate real `CLIENT_ID`.
4. **Add narrow HMAC tracing** if AES capture alone does not answer the question.
5. **Only then** decide whether static RE of V10014 is necessary.

---

## Relevant Files / Paths

Project files:

- `python/dedrm/drm_init.py`
- `lib/KFXVoucherExtractor.java`
- `lib/crypto_hook.c`
- `.github/Dockerfile.arm`

Firmware / reference material:

- `REFERENCE/update_kindle_paperwhite_v2_5.12.2.2.bin`
- `REFERENCE/firmware_pw2_5.12.2.2/rootfs.img`
- `REFERENCE/firmware/pw12_5.17.1.0.4/rootfs/`
- `REFERENCE/DeDRM_tools/DeDRM_plugin/ion.py`

Key runtime paths on device:

- `/usr/java/bin/cvm`
- `/usr/java/lib/arm/minimal/libjvm.so`
- `/var/local/java/prefs/acsr`
- `/proc/usid`
- `/mnt/us/crypto_keys.log`

---

## Bottom Line

Current best model:

- **not** a hidden firmware infection through KFX
- **not** evidence of `contentpackd` updating DRM runtime code
- **yes** to a likely server-side voucher shift
- **yes** to old firmware already having enough on-device support
- **yes** to our `LD_PRELOAD` interception path still being the strongest practical method
- **maybe** to recovering the true `CLIENT_ID`
- **harder/more uncertain** to fully reproducing V10014 offline

---

## On-Device Investigation Results (2026-04-28)

Device tested: **Kindle Paperwhite 6 (PW6, sangria/bellatrix4)**

- Firmware: `001-juno_18050001_sangria_bellatrix4-455680` (5.18.5)
- Kernel: `5.15.41-lab126` armv7l
- Serial: `GR733X1151821324`
- CPU: ARMv7 rev 4, hard-float (armhf)
- ACSR path: `/var/local/java/prefs/acsr` (confirmed present, 121 bytes)
- libcrypto: **OpenSSL 3.x** (`libcrypto.so.3`, 3MB) — not 1.0.0 like older firmware
- libYJSDK: **3.3MB** (much larger than PW2's 2.6MB and PW12 firmware's 3.0MB)
- 10 test books with vouchers available on device

### All vouchers on this device are `@10005.0`

Checked all 4 non-tmp vouchers plus 6 books from reference collection:

```
1984:             com.amazon.drm.VoucherEnvelope@10005.0
Elvis:            com.amazon.drm.VoucherEnvelope@10005.0
Familiars:        com.amazon.drm.VoucherEnvelope@10005.0
Heated Rivalry:   com.amazon.drm.VoucherEnvelope@10005.0
Hunger Games:     com.amazon.drm.VoucherEnvelope@10005.0
Secrets Crown:   com.amazon.drm.VoucherEnvelope@10005.0
Sunrise Reaping:  com.amazon.drm.VoucherEnvelope@10005.0
Three Below:      com.amazon.drm.VoucherEnvelope@10005.0
Throne of Glass:  com.amazon.drm.VoucherEnvelope@10005.0
```

These were all downloaded within the last ~20 days (April 9–24). So `@10005.0` is the current production voucher version for this device/account. The `@10014.0` seen in DeDRM issue #993 may be specific to older firmware devices or a different rollout tier.

All vouchers use lock parameters: **`ACCOUNT_SECRET` + `CLIENT_ID`** (not just `CLIENT_ID` alone).

### SDK `getSupportedVoucherVersions()` results

Deployed `SDKProbe.jar` to device, ran against device `cvm`:

```
Supported voucher versions: 39
  V1  V2  V3  ... V28
  V9708  V1031  V2069  V9041  V3646  V6052
  V9479  V9888  V4648  V5683  V3972

SDK version: 0
```

**No 5-digit versions reported.** The list is identical before and after `setAccountSecrets()`. This matches exactly what DeDRM's `OBFUSCATION_TABLE` covers (V1–V28 plus the 4-digit specials), with one exception: **V3972 appears in the SDK list but NOT in DeDRM's obfuscation table**.

This is a critical finding: **the SDK claims to support only V1–V28 and 4-digit versions, yet it successfully decrypts V10005 vouchers.** This means 5-digit versions use a completely different code path that is NOT reflected in `getSupportedVoucherVersions()`.

### Hook capture on live device

Ran `crypto_hook.so` + `KFXVoucherExtractor.jar` on the live PW6:

```
$ LD_PRELOAD=/mnt/us/crypto_hook.so cvm ... KFXVoucherExtractor $SERIAL $VOUCHER

Security initialized
Voucher: .../Three Below.../voucher
All vouchers attached
Done

=== Captured keys ===
EVP_256_KEY:65333566...e3163 IV:398161d58ee053e63a7b45ced0c26ebb
EVP_256_KEY:eb035e31...5b49 IV:039688bced56a21cacc522af2e8be3aa
```

The second key (`eb035e31...5b49`) matches `voucher_key_256` for "Three Below" in `drm_keys.json` — **confirming our hook works end-to-end on live hardware**.

Two AES-256 keys are captured per voucher attachment:
1. First key: `e35f5062f97cc8b1244f6f1a2414e31c` (16 bytes decoded from hex-encoded ASCII) — possibly an intermediate/shared secret
2. Second key: `eb035e31...` — the actual voucher decryption key

### Registration state found on device

`/var/local/java/prefs/reginfo`:
```
givenName=Kai
userId=amzn1.account.AFDOR6TO5UGPAQKZBA4FBQYNY7KQ
deviceName=Kai's Kindle Paperwhite
userName=Kai
deviceEmailAddress=kaikozlov_wdLKjM@kindle.com
```

`/var/local/java/prefs/household.json` contains account profile with `directedId: amzn1.account.AFDOR6TO5UGPAQKZBA4FBQYNY7KQ`.

Candidate `CLIENT_ID` values to investigate:
- Device serial: `GR733X1151821324`
- Account ID: `amzn1.account.AFDOR6TO5UGPAQKZBA4FBQYNY7KQ`
- ACSR token (121 bytes, base64-encoded)
- Some combination or hash of the above

### Updated confidence levels

**Now confirmed:**
- `@10005.0` is the current production voucher version (not just theoretical)
- Lock params are `ACCOUNT_SECRET + CLIENT_ID` together, not just `CLIENT_ID`
- SDK reports 39 supported versions, NONE of which are 5-digit
- 5-digit voucher versions use a separate, unreported decryption path
- Our hook captures working keys on live hardware
- Device uses OpenSSL 3.x (not 1.0.0)
- `V3972` is supported by SDK but missing from DeDRM

**Revised hypothesis:**
- 5-digit voucher versions likely bypass the `obfuscate()/scramble()` path entirely
- They probably use a different KDF that does not involve the version-number-based obfuscation table
- The SDK's `getSupportedVoucherVersions()` only lists versions handled by the OLD path
- The NEW path (5-digit versions) may use a standard key agreement or HMAC-based derivation without version-specific scrambling
- This would explain why DeDRM fails even with the right `CLIENT_ID` — it is trying all the old scramble functions that simply are not used for 5-digit versions

### What this means for derivation investigation

The problem is NOT just "find the right CLIENT_ID." It is also "find the KDF for 5-digit versions."

Most productive next steps:
1. **Run a controlled lock-parameter perturbation matrix** with a tiny configurable Java probe:
   - baseline: correct `ACCOUNT_SECRET`, correct `CLIENT_ID`
   - wrong `CLIENT_ID`, correct `ACCOUNT_SECRET`
   - omit `CLIENT_ID`
   - wrong `ACCOUNT_SECRET`, correct `CLIENT_ID`
   - omit `ACCOUNT_SECRET`
   - no `setLockParameters()`, only `setAccountSecrets()`
2. For each run, capture with the existing hook:
   - whether `attachVouchers()` succeeds
   - stage-1 AES key / IV
   - HMAC key length
   - final `voucher_key_256`
3. Use that matrix to answer the highest-value binary question: **does the SDK actually use the supplied `CLIENT_ID`, or does it resolve/override it internally?**
4. **Check whether the numeric-string HMAC input exists in the raw voucher bytes**. If yes, parse it offline; if not, it is produced after the stage-1 unwrap path.
5. **Add narrow `EVP_DecryptUpdate` / `EVP_DecryptFinal_ex` tracing** (and `inflate` if needed) to determine whether the large per-voucher HMAC key blob is:
   - direct AES plaintext,
   - AES plaintext followed by decompression,
   - or an assembled intermediate structure.
6. Only after those discriminating experiments should we choose between:
   - internal `CLIENT_ID` hunting,
   - offline stage-1 unwrap reconstruction,
   - or static RE of `libYJSDK-shared.so`.

### Recommended decision tree

If **wrong `CLIENT_ID` has no effect**:
- treat `CLIENT_ID` as likely internally resolved / overridden
- focus next on internal device/account identifier lookup rather than brute-forcing candidates

If **wrong `CLIENT_ID` changes output**:
- `CLIENT_ID` really participates in derivation
- compare near-identical fake IDs and trace where it first affects the pipeline

If **the numeric HMAC input exists in the raw voucher**:
- parse that field offline and build a local reproducer around it

If **the numeric HMAC input only appears after stage 1**:
- focus on stage-1 decrypt/unpack
- deprioritize DeDRM's legacy `obfuscate()/scramble()` model for 5-digit versions

### Strategy note

We should explicitly avoid looping by:
- not collecting more random crypto traces without a hypothesis,
- not brute-forcing `CLIENT_ID` candidates prematurely,
- and not jumping into deep static RE before the perturbation matrix and decrypt/update tracing tell us which branch is worth pursuing.

---

## Perturbation Matrix Results (2026-04-28)

Ran a 7-case controlled experiment varying `setAccountSecrets()` and `setLockParameters()` inputs to the DRM SDK, capturing all AES/HMAC operations with `hook_fulldump.so`.

### Full derivation chain (confirmed)

```
Stage 1: AES-256-CBC decrypt
  key = 65333566...33163 (hex-encoded "e35f5062f97cc8b1244f6f1a2414e31c")
  IV  = 398161d58ee053e63a7b45ced0c26ebb (= first 16 bytes of double-decoded ACSR)
  → produces per-voucher HMAC key blob (variable size: 2670–10330 bytes)

Stage 2: HMAC-SHA256(per-voucher key blob, numeric-string)
  → produces voucher_key_256 (32 bytes)

Stage 3: AES-256-CBC decrypt
  key = voucher_key_256 from stage 2
  IV  = from voucher inner structure
  → decrypts book content
```

Both HMAC outputs verified locally with `hmac.new(key, data, hashlib.sha256)`.

### Matrix results

| Case | setAcctSecrets | Lock params | Attach | Stage-1 AES | HMAC key len | voucher_key_256 | Error |
|------|---------------|-------------|--------|-------------|-------------|-----------------|-------|
| baseline | correct ACSR | ACCT_SEC+CID=serial | OK | same | 10330 | eb035...5b49 | - |
| wrong_cid | correct ACSR | ACCT_SEC+CID=XXXXXX | FAIL | same | 9186 | ab09f...308a | Err48 |
| no_cid | correct ACSR | ACCT_SEC only | FAIL | same | 0 | (none) | Err43 |
| wrong_as | WRONG ACSR | ACCT_SEC+CID=serial | OK | same | 10330 | eb035...5b49 | - |
| no_as | correct ACSR | CID only | FAIL | none | 0 | (none) | Err43 |
| no_lock | correct ACSR | (skipped) | FAIL | none | 0 | (none) | Err43 |
| no_secrets | (skipped) | ACCT_SEC+CID=serial | OK | same | 10330 | eb035...5b49 | - |

### Critical findings

1. **`setAccountSecrets()` is completely irrelevant to crypto.** Wrong value or skipped entirely → identical AES keys, identical HMAC output. The SDK reads ACSR directly from `/var/local/java/prefs/acsr` internally, bypassing the Java API.

2. **ACCOUNT_SECRET in lock params must be PRESENT but its VALUE is ignored.** Omitting it → ErrorCode 43. But a garbage value → identical output to baseline.

3. **CLIENT_ID value directly affects derivation.** Wrong CLIENT_ID → different HMAC key (9186 vs 10330 bytes), different voucher_key_256, ErrorCode 48. Omitting it → ErrorCode 43 with no crypto at all.

4. **Stage-1 AES key and IV are constant across ALL cases** (even wrong ACSR, wrong CLIENT_ID, skipped setAccountSecrets). This means stage 1 uses a hardcoded or vendor-provided key that does not depend on external inputs. The IV comes from the ACSR file on disk (first 16 bytes of double-base64-decoded ACSR).

5. **The HMAC numeric-string input does NOT exist in the raw voucher bytes.** It is produced by the stage-1 decryption — the SDK decrypts something from the voucher, and the plaintext includes a decimal-encoded number that feeds into the HMAC.

6. **The per-voucher HMAC key blob is variable-length** (2670 bytes for Elvis, 10330 bytes for Three Below). It comes from stage-1 decryption of voucher content.

### What this means

The derivation is:
```
ACSR (from disk) → double-base64-decode → first 16 bytes = stage-1 IV
stage-1 key = hardcoded in libYJSDK ("e35f5062f97cc8b1244f6f1a2414e31c")
stage-1 decrypt(voucher cipher blob) → plaintext → extract:
  - per-voucher HMAC key blob (thousands of bytes)
  - numeric string (hundreds of digits)
CLIENT_ID (from lock params) → used somewhere in stage-1 or HMAC construction
HMAC-SHA256(key blob, numeric string) → voucher_key_256
AES-256-CBC(voucher_key_256, inner IV) → book decryption key
```

The CLIENT_ID must be the correct device serial number. The SDK does NOT override or resolve it internally — it uses exactly what we pass via `setLockParameters()`.

### Next steps (prioritized)

1. **Trace stage-1 decrypt output**: Hook `EVP_DecryptUpdate`/`EVP_DecryptFinal` to capture the plaintext from stage 1. This will reveal the structure of the HMAC key blob and numeric string.

2. **Determine where CLIENT_ID enters**: In the `wrong_cid` case, the HMAC key is 9186 bytes (vs 10330 baseline). CLIENT_ID likely affects stage-1 decryption or post-processing of the decrypted blob. Understanding this is the key to offline reproduction.

3. **If we can reproduce stage 1 offline**: We would need only the ACSR (from disk), the hardcoded stage-1 key, and the correct CLIENT_ID (device serial) to derive everything without the SDK.

4. **Static RE of stage-1 key**: Confirm whether `e35f5062f97cc8b1244f6f1a2414e31c` is truly hardcoded or derived from something predictable. Search `libYJSDK-shared.so` for this byte pattern.

---

## Stage-1 Decrypt Tracing Results (2026-04-28)

Added `EVP_DecryptUpdate`/`EVP_DecryptFinal_ex` hooks to capture stage-1 plaintext.

### Stage-1 decrypt is tiny and constant

```
EVP_DecryptInit_ex(key=65333566...33163, iv=398161d5...ebb)
EVP_DecryptUpdate(in=48, out=32)
EVP_DecryptFinal_ex(out=8)
Total plaintext: 40 bytes = "54e869c4b43348062477a52df5467be8c4e08420"
```

This 40-byte hex string (20 raw bytes) is **identical in both baseline and wrong_cid cases**. It is NOT the HMAC key. The HMAC key (10330 bytes) comes from somewhere else entirely.

### Stage-2 decrypt

```
EVP_DecryptInit_ex(key=voucher_key_256_from_HMAC, iv=from_voucher)
EVP_DecryptUpdate(in=512, out=496)
EVP_DecryptFinal_ex(out=13)
Total plaintext: 509 bytes (starts with e00100ea = ProtectedData)
```

### The missing link: custom crypto in libYJSDK

The 10330-byte HMAC key blob does NOT come from any visible AES/HMAC/SHA operation. libYJSDK only imports:
- `EVP_Decrypt*` (AES-CBC decrypt)
- `HMAC`
- `EVP_sha256`
- `EVP_PKEY_verify` (signature check only)
- `EVP_Digest*` (hashing)

No `EVP_PKEY_decrypt`, no `EVP_PKEY_derive`, no `BN_*`, no `RSA_*`, no `DH_*` symbols. The library does custom bignum/crypto internally without calling libcrypto for key agreement.

### Updated derivation chain

```
1. SDK reads ACSR from /var/local/java/prefs/acsr (ignoring Java API)
2. double-base64-decode(ACSR) → 64 bytes → first 16 = stage-1 IV
3. AES-256-CBC(key="e35f5062f97cc8b1244f6f1a2414e31c" (hardcoded),
              IV=ACSR_first_16_bytes,
              ciphertext=48_bytes_from_voucher)
   → 40-char hex string "54e869c4...8420" (20 bytes)
   → This is IDENTICAL regardless of CLIENT_ID

4. CUSTOM CRYPTO INSIDE libYJSDK:
   Input: voucher cipher blob + CLIENT_ID (from setLockParameters)
   Output: HMAC key blob (variable size: 10330/9186/...) + numeric string
   Method: UNKNOWN — no libcrypto calls, custom bignum arithmetic
   CLIENT_ID directly affects this step (different CLIENT_ID → different sizes)

5. HMAC-SHA256(HMAC_key_blob, numeric_string) → voucher_key_256
   (verified offline — exact match)

6. AES-256-CBC(voucher_key_256, inner_IV_from_voucher) → decrypted book voucher
```

### Symbol analysis of libYJSDK-shared.so

The library has 1151 exported symbols, but DRM-relevant ones are minimal:
- `yjsdk::IBookSecurity::getInstance()`
- `yjsdk::BookFactory::canOpenBook()` / `getBook()`
- `yjfp::Key` class (getData/getType)
- All voucher/decrypt/crypto logic is in hidden/internal symbols

JNI interface (`libYJSDKJNI-shared.so`) exposes 7 BookSecurity methods:
- `getInstance`, `deleteBookSecurity`, `setAccountSecrets(String)`
- `setLockParameters(String[][])`, `attachVoucher(String)`
- `getSupportedVoucherVersions()`, `getVersion()`

### What this means for offline derivation

**The blocker is step 4: custom crypto inside libYJSDK.**

We cannot reproduce the HMAC key offline because:
- The operation is not any standard crypto (no DH/RSA/ECDH through libcrypto)
- It's custom code in a stripped 3.3MB native library
- It takes CLIENT_ID and voucher blob as input and produces variable-size output

**Options to proceed:**

1. **Static RE of libYJSDK**: Disassemble around `attachVoucher` JNI entry point. Look for custom bignum/modpow implementation. This is the most certain path but significant effort.

2. **Our LD_PRELOAD approach remains the pragmatic answer**: We don't need to understand step 4. We just hook HMAC and AES after the SDK has done its work. This is what our plugin already does.

3. **Help DeDRM**: The key insight for DeDRM is that the 5-digit voucher versions use a COMPLETELY DIFFERENT path than the `obfuscate()/scramble()` model. Even with the correct CLIENT_ID, DeDRM's existing code cannot derive the key because the custom crypto step doesn't match any OBFUSCATION_TABLE entry.

### Concrete recommendations

1. **For our plugin**: Continue with LD_PRELOAD. It works. The hook captures keys correctly. No need to reverse the custom crypto.

2. **For DeDRM upstream**: Document that 5-digit voucher versions use custom native crypto that cannot be reproduced without the device SDK. The only viable approach for PC-side decryption would be static RE of libYJSDK.

3. **If we ever want offline derivation**: The entry point for RE is `Java_com_amazon_yjreadersdk_BookSecurity_attachVoucher` in `libYJSDKJNI.so` at offset `0xd88c`. This calls into `libYJSDK.so` where the custom crypto lives. The voucher cipher blob and CLIENT_ID are the inputs; the HMAC key blob and numeric string are the outputs.

---

## Recommended Reverse-Engineering Strategy

At this point, the right goal is **not** “immediately reproduce offline derivation.”
The right next goal is to **localize the exact native routine** inside `libYJSDK-shared.so` that turns:

- voucher blob,
- `CLIENT_ID`,
- ACSR-derived state,

into:

- HMAC key blob,
- numeric-string HMAC input.

That is the shortest path to determining whether offline derivation is realistic.

### Success hierarchy

1. **Practical success**
   - extract working keys on-device
   - already achieved via `LD_PRELOAD`

2. **Analytical success**
   - identify the exact native code path implementing the custom 5-digit voucher crypto
   - this is the next realistic target

3. **Offline derivation success**
   - reimplement or reproduce that native path outside the SDK
   - only worth attempting after (2)

### Core strategy: dynamic → static loop

Use a tight hybrid loop:

1. **Dynamic instrumentation** to identify *where* inside `libYJSDK` the interesting calls originate
2. **Static RE** only after we have concrete offsets / callsites
3. Keep using **baseline vs wrong_cid** on the **same voucher** as the canonical discriminating pair

This avoids trying to reverse-engineer a 3.3 MB stripped native library blindly.

### Phase 1 — Add caller-offset logging

Extend the existing hooks for:

- `HMAC`
- `EVP_DecryptInit_ex`
- `EVP_DecryptUpdate`
- `EVP_DecryptFinal_ex`

to also log:

- return address / caller address
- module name via `dladdr`
- offset within `libYJSDK-shared.so`

#### Why this is high value

We already know **what data** appears.
We do not yet know **which function** in `libYJSDK` creates it.

Caller offsets will give us a concrete static RE target, e.g.:

- HMAC called from `libYJSDK+0xABC123`
- stage-1 AES called from `libYJSDK+0xDEF456`
- stage-2 AES called from `libYJSDK+0x123789`

That is far more valuable than collecting more raw buffers.

### Phase 2 — Use the discriminating pair

Always compare:

- **baseline**
- **wrong_cid**

using the **same voucher**.

#### Questions to answer

- do both cases hit the same HMAC callsite?
- same AES callsites?
- same number of calls?
- at what callsite / branch / buffer length do they first diverge?

#### Success criterion

Find the **first point** where baseline and wrong_cid differ:

- function offset
- branch path
- call count
- or produced buffer length

That will identify the true CLIENT_ID-dependent code region.

### Phase 3 — Static RE only around identified offsets

Once we have offsets, inspect only those regions in Ghidra:

1. the function containing the `HMAC` caller
2. its caller
3. the function that consumes `CLIENT_ID`
4. the function that emits the numeric string / HMAC key blob

#### First labels to establish

- voucher input buffer
- `CLIENT_ID` string input
- HMAC key blob output buffer
- numeric-string output buffer
- any helper that consumes the 20-byte stage-1 output

#### Why this order

The most efficient path is:

`HMAC caller` → `producer of HMAC args` → `CLIENT_ID dependency`

not:

`attachVoucher entry` → `entire DRM subsystem`

### Phase 4 — Only one more kind of dynamic instrumentation if needed

If caller offsets are not enough, add **shallow backtrace logging** for `HMAC`:

- immediate caller
- caller’s caller
- optionally one more frame

Even 2–3 frames should be enough to reveal the real voucher-specific function if `HMAC` is wrapped in a helper.

#### Avoid for now

Do **not** add:

- `malloc/free` tracing
- `memcpy/memmove` tracing
- syscall tracing
- huge buffer dumps everywhere

Those will create noise without shrinking the search space.

### Phase 5 — Decide whether offline derivation is realistic

Continue toward offline derivation only if static RE shows that:

- the CLIENT_ID-dependent logic is localized,
- the transformation is understandable,
- outputs are deterministic from captured inputs,
- and the custom math is not spread across a large proprietary big-int engine.

Stop and document if the core turns out to be:

- a large custom arithmetic engine,
- table-heavy proprietary crypto,
- or otherwise too costly to reconstruct relative to the value.

That would still be a successful analytical result because it would explain precisely why DeDRM fails and why the SDK/native dependency is essential.

### Concrete next actions

#### Session 1

Implement caller-offset logging and rerun:

- baseline
- wrong_cid

Expected output:

- exact `libYJSDK` offsets for HMAC and AES callsites
- identification of whether the two cases diverge at the same callsite or earlier/later in the path

#### Session 2

Open `libYJSDK-shared.so` in Ghidra and inspect those offsets.

Expected output:

- labeled HMAC caller function
- rough call chain
- probable CLIENT_ID consumer

#### Session 3

If needed, add shallow backtrace logging and rerun.

Expected output:

- parent-chain of the custom crypto path
- better anchors for static RE

#### Session 4

Make a decision:

- pursue offline derivation,
- or conclude that the SDK-native black box is too custom and document limits.

### Guardrails to avoid looping

- use **one voucher** as the main specimen until the path is understood
- only add a second voucher after understanding the first
- keep **baseline vs wrong_cid** as the canonical discriminating experiment
- every new hook must answer a specific question
- if a probe does not shrink the search space, remove it

---

## Caller-Offset Tracing Results (2026-04-28)

Added `__builtin_return_address(0)` + `dladdr()` to capture the caller of each hooked function.
Captured at hook entry (before calling real), so the return address points into the actual calling code in `libYJSDK-shared.so`.

### Callsite map

All offsets are into `libYJSDK-shared.so` (3,375,040 bytes).

| Hook call | Baseline offset | Wrong-CID offset | Same? |
|-----------|----------------|------------------|-------|
| DEC_INIT #1 (stage-1 AES) | `+0x137cb7` | `+0x137cb7` | YES |
| DEC_UPDATE #1 | `+0x1b2803` | `+0x1b2803` | YES |
| DEC_FINAL #1 | `+0x1b280f` | `+0x1b280f` | YES |
| HMAC #1 (→ voucher_key) | `+0x15151d` | `+0x15151d` | YES |
| DEC_INIT #2 (stage-2 AES) | `+0x15122d` | `+0x15122d` | YES |
| DEC_UPDATE #2 | `+0x1b2803` | `+0x1b2803` | YES |
| DEC_FINAL #2 | `+0x1b280f` | `+0x1b280f` | YES |
| HMAC #2 (integrity) | `+0x151481` | (not reached) | - |

### Key observations

1. **ALL callsites are identical between baseline and wrong_cid.** The code path does not branch based on CLIENT_ID. The same functions run; they just produce different outputs because CLIENT_ID is fed as input to the custom crypto.

2. **Three distinct call regions in libYJSDK:**
   - `0x137cb7`: stage-1 AES init — one function that sets up the hardcoded key + ACSR-derived IV
   - `0x15122d` and `0x15151d` / `0x151481`: stage-2 AES init + HMAC calls — all within ~734 bytes of each other, likely the same voucher processing function
   - `0x1b2803` / `0x1b280f`: shared `EVP_DecryptUpdate`/`Final` helper — a generic decryption utility called by both stages

3. **The custom crypto black box** (which transforms voucher blob + CLIENT_ID into the HMAC key blob and numeric string) **executes between `DEC_FINAL #1` (offset `0x1b280f`) and `HMAC #1` (offset `0x15151d`).** It does NOT call any libcrypto functions. The distance between these two points in the execution trace is where the proprietary computation lives.

4. **In static RE terms:** the function at `0x15151d` (HMAC caller) is the anchor point. Working backwards from there, we should find:
   - the code that constructs the HMAC key buffer (10330/9186 bytes)
   - the code that constructs the numeric-string HMAC data
   - the code that takes CLIENT_ID as input
   - and the custom bignum/crypto engine that connects them

### Next step: Ghidra

With these offsets, we can now:

1. Open `libYJSDK-shared.so` in Ghidra
2. Navigate to `0x137cb7` → identify the stage-1 decrypt function and its caller
3. Navigate to `0x15151d` → identify the HMAC caller function
4. Trace backwards from `0x15151d` to find the custom crypto that feeds the HMAC
5. Compare the function at `0x137cb7` and `0x15122d` — they are ~103 KB apart, likely different functions in the voucher processing pipeline

---

## Disassembly Analysis Results (2026-04-28)

Disassembled `libYJSDK-shared.so` (Thumb2, ARM) at the key caller offsets.

### Function map from caller offsets

| Offset | What | Details |
|--------|------|----------|
| `0x151200` | **Voucher processing function** (push `{r4-r9,lr}`, ~900 bytes) | Contains HMAC#1 (`0x151518`), DEC_INIT#2 (`0x151228`), and HMAC#2 (`0x151481`). This is the core voucher unwrap function. |
| `0x137cb2` | **Stage-1 AES init caller** | `blx EVP_DecryptInit_ex@plt` in a different function. Sets up hardcoded key + ACSR-derived IV. |
| `0x1b27d4` | **Shared decrypt helper** (`~42 bytes`) | Wrapper that calls `EVP_DecryptUpdate` + `EVP_DecryptFinal_ex`. Called by both stage-1 and stage-2 decrypt. |

### Voucher processing function (`0x151200`) — detailed walkthrough

```
0x151200: push {r4-r9, lr}; sub sp, #92
0x151210: EVP_CIPHER_CTX_new()
0x151216: EVP_aes_256_cbc()                    // get cipher type
0x151228: EVP_DecryptInit_ex(ctx, aes256cbc, NULL, key_from_r9[0x10], iv_from_r6)
0x151242: bl decrypt_helper(1b27d4)            // EVP_DecryptUpdate + Final → stage-2 plaintext
  // After decrypt, sets up HMAC:
0x1514fa: malloc(64)                            // output buffer for HMAC result
0x151502: EVP_sha256()                         // get hash algorithm
0x151514: r2 = r8[1] - r8[0]                   // HMAC data length (std::vector)
0x151518: HMAC(sha256, key_ptr, key_len, data_ptr, data_len, out_buf, &out_len)
```

The function receives arguments:
- `r1` (→ `r5`) = voucher key pointer
- `r2` (→ `r8`) = pointer to std::vector containing HMAC key material
- `r3` (→ `r9`) = pointer to struct containing AES key (at offset `0x10`)
- `sp+0x7c` (→ `r6`) = AES IV
- `sp+0x84` (→ `r4`) = output/result structure

### Key insight: where is the custom crypto?

The HMAC key material arrives in `r8` (a `std::vector<uint8_t>` passed as argument to this function). **This function does NOT produce the HMAC key** — it receives it as input from its caller.

The custom crypto that transforms voucher blob + CLIENT_ID into the HMAC key happens in the **caller of `0x151200`** — or somewhere further up the call chain before this function is invoked.

This is a critical narrowing: the black box is NOT inside the voucher processing function at `0x151200`. It's in whatever calls this function.

### Stage-1 decrypt function

The stage-1 AES init at `0x137cb2` is in a different function. Looking at the code around it:
- It calls `EVP_DecryptInit_ex` with the hardcoded key and ACSR-derived IV
- Then calls the decrypt helper at `0x1b27d4` to get the 40-byte hex string
- This function is ~103 KB before the voucher processing function

### Next RE target

The highest-value next target is: **find what calls `0x151200` and trace how `r8` (the HMAC key vector) is populated.**

Approaches:
1. Search for calls to `0x151200` (i.e., `bl`/`blx` targeting that address) in the binary
2. Or use Ghidra's cross-reference feature once a project is set up
3. The caller will contain the custom crypto logic that depends on CLIENT_ID

### JNI attachVoucher analysis

Disassembled `Java_com_amazon_yjreadersdk_BookSecurity_attachVoucher` at `0xd88c` in `libYJSDKJNI.so`:

```
d88c: push {r3-r7, lr}
d898: GetStringUTFChars(env, voucherPath)  // get voucher path string
d8a2: getHandle(env, this)               // get native IBookSecurity handle
d8c2: ldr r0, [r0]                       // dereference handle → object pointer
d8c4: mov r1, r6                         // arg1 = voucher path
d8c6: ldr r3, [r3, #0]                   // load vtable pointer
d8c8: ldr r3, [r3, #20]                  // load vtable[5] = 0x151200 (Thumb)
d8ca: blx r3                             // call voucher processing function
d8cc: mov r2, r0                         // result → r2
d8ce: cbz r0, d8da                       // if NULL, success
d8d0-d8d6: throw exception if failed
d8da-d8e4: ReleaseStringUTFChars, return
```

The JNI code calls vtable slot 5 directly on the IBookSecurity native object. This IS the function at `0x151200` in `libYJSDK.so`. The voucher path is passed as `r1`.

### Critical realization about the function boundary

The function at `0x151200` has multiple exit points:
- `0x15131e`: `pop {r4-r9, pc}` (early exit / success)
- `0x151386`: `pop {r4-r9, sl, pc}` (another function or sub-function)
- `0x1514f6`: `pop {r4-r9, sl, fp, pc}` (after HMAC#1)

The HMAC#1 at `0x151518` is AFTER the `0x1514f6` exit point, which means HMAC#1 is in a **different sub-function** called from within the voucher processing flow. This is important — the custom crypto that produces the HMAC key may be in the function between `0x151386` and `0x1514f6`.

### Recommended next step for Ghidra

With a Ghidra project set up, the following would be straightforward:
1. Import `libYJSDK-shared.so` (ARM Cortex-A9, Thumb2)
2. Navigate to `0x151200` → decompile the voucher processing function
3. Use cross-references on the function to find its callers
4. Decompile the function containing the custom crypto (likely `0x151340`-`0x1514f6`)
5. Look for string references to CLIENT_ID and the custom math

Without Ghidra, the next useful dynamic probe would be to log the caller address when the function at `0x151200` is entered — this would confirm whether it's called directly from JNI or through an intermediate dispatcher.

---

## r2ghidra Decompilation Results (2026-04-28)

Installed `radare2` + `r2ghidra` plugin and decompiled the voucher processing function at `0x151200`.

### Decompiled HMAC#1 call (key derivation)

```c
// aiStack_e0 is the HMAC key buffer (std::vector<uint8_t>)
aiStack_e0[0] = 0;  // key pointer = NULL
aiStack_e0[1] = 0;  // key end = NULL  
aiStack_e0[2] = 0;  // key capacity = NULL

// ... set up decryption context ...
piVar4 = pcVar18 >> 0x12;  // derive object pointer from voucher data

// THIS IS THE CUSTOM CRYPTO CALL:
pcVar11 = (**(*piVar4 + 0x20))(piVar4, &stack0xffffff20, extraout_r1);
//          ^^^^^^^^^^^^^^^^^^
//          calls piVar4->vtable[8] (offset 0x20)
//          This is the function that populates aiStack_e0 (HMAC key)

if (pcVar11 != NULL) goto error_handler;

// ... more virtual dispatches for cleanup/swap ...

// HMAC#1: derives voucher_key_256
uVar2 = EVP_sha256();
HMAC(uVar2, aiStack_e0[0], aiStack_e0[1] - aiStack_e0[0], *(piVar4[1] + 0xc4));
```

### Critical finding

The HMAC key buffer (`aiStack_e0`) is populated by a **virtual method call** at vtable offset `0x20` (slot 8) on object `piVar4`.

This is the custom crypto entry point:
```
pcVar11 = (**(*piVar4 + 0x20))(piVar4, &stack0xffffff20, extraout_r1);
```

The object `piVar4` is derived from the voucher data (`pcVar18 >> 0x12`). Its vtable slot 8 contains the function that:
1. Takes the voucher cipher blob and CLIENT_ID as input
2. Performs the custom bignum/crypto
3. Produces the HMAC key blob and numeric string

### What piVar4 is

`piVar4` is derived from `pcVar18 >> 0x12` where `pcVar18` comes from the voucher parsing. This is likely a **voucher strategy object** — the ION voucher contains a `strategy` field (we saw `com.amazon.drm.PIDv3@1.0` in the raw voucher). The strategy determines which vtable is used, and thus which crypto algorithm runs.

For `PIDv3@1.0` vouchers, vtable slot 8 points to the function that implements the custom CLIENT_ID-dependent derivation.

### Next step for RE

With r2ghidra, the next step is:
1. Decompile the function at the vtable slot 8 address
2. Find what class implements this vtable
3. The function body will contain the custom crypto that takes CLIENT_ID and produces the HMAC key

This narrows the search from 3.3 MB to a single virtual method.

---

## Vtable Slot 8 Deep Dive (2026-04-28)

### Vtable map for VoucherDecryption class

Vtable at `0x32f850` (16+ slots):

| Slot | Offset | Address | Notes |
|------|--------|---------|-------|
| 0 | 0x00 | 0x15fb30 | destructor? |
| 1 | 0x04 | 0x15fb3c | destructor? |
| 2 | 0x08 | 0x150c74 | |
| 3 | 0x0c | 0x152754 | |
| 4 | 0x10 | 0x154088 | |
| 5 | 0x14 | 0x151200 | **voucher processing (attachVoucher)** |
| 6 | 0x18 | 0x1510b8 | |
| 7 | 0x1c | 0x1514bc | |
| 8 | 0x20 | 0x150c40 | **custom crypto dispatcher** |
| 9 | 0x24 | 0x158864 | stub (24 bytes) |
| 10 | 0x28 | 0x156ccc | |
| 11 | 0x2c | 0x15134c | |
| 12 | 0x30 | 0x150c3c | |
| 13 | 0x34 | 0x150c38 | |

### Slot 8 dispatcher (`0x150c40`)

```c
void fcn_0x150c40(int *this, ...) {
    if (*(this[1] + 0xc0) != 0x76) {       // 0x76 = 118
        (**(*this + 0x24))();              // call vtable[9]
        (**(*this + 0x28))(this, ...);     // call vtable[10]
        return;
    }
    fcn_0x17c4fc(...);  // THE custom crypto engine
}
```

The function checks `*(this[1] + 0xc0) == 0x76` (118). This is likely an internal protocol version check — only vouchers matching protocol version 118 take the custom crypto path. Other versions go through vtable[9]/[10] which may be a simpler/legacy path.

### THE CUSTOM CRYPTO: `fcn_0x17c4fc` — 244 KB obfuscated state machine

**Size: 243,762 bytes** (~244 KB in a single function).

The function uses **control-flow flattening obfuscation** — the entire body is a giant `switch` statement with 136+ cases, where the next case is computed via complex arithmetic:

```c
uint fcn_0x17c4fc(...) {
    uStack_204 = 0xf941e883;          // initial state seed
    puVar35 = 0x94a3e212;             // state variable
    puStack_230 = 0x7ed06214;         // state variable
    puStack_234 = 0x34506731;         // state variable
    puStack_22c = 0x9d9534b5;         // state variable
    uVar23 = *(*0x17d268 + 0x17c574) ^ 7;  // computed initial switch key

    do {
        if (uVar23 == 0x87) {
            software_udf(0xff, 0x17ee9a);   // UNDEFINED INSTRUCTION — anti-debug
            (*pcVar1)();
        }
        iVar17 = *(*0x17d270 + 0x17c5e0) + uVar23;  // switch index computation
        
        switch(iVar17) {
        case 0: goto code_r0x0017e78e;        // state transitions
        case 1: uVar23 = 0x26 - (r9 * -0x4d3316c9 >> 0x11) & 0xff; break;
        case 2: r9 = 0xa9dbdd2; uVar23 = 0xe; break;
        case 3: /* complex arithmetic */ break;
        // ... 136+ cases ...
        case 6:
            // BigInt modular operation: ((a^b) * (a|b) * (a&b)) % 500
            iStack_208 = ((uVar23 ^ puStack_214) * (uVar23 | puStack_214) * (uVar23 & puStack_214)) % 500;
            break;
        }
    } while (...);
}
```

### Obfuscation characteristics

1. **Control-flow flattening**: All basic blocks are in a single switch; next-block index computed via multiplicative PRNG-like expressions
2. **Anti-debugging**: `software_udf(0xff)` — triggers undefined instruction trap when state variable == 0x87
3. **Opaque predicates**: Branches on `in_NG`, `in_CY`, `in_OV` (CPU flag registers) that are deterministic but hard to analyze statically
4. **State-dependent arithmetic**: Variables like `puVar35`, `puStack_230`, `puStack_234` carry obfuscated state across switch iterations
5. **Bignum operations**: Case 6 shows `(a^b) * (a|b) * (a&b) % 500` — custom modular arithmetic
6. **Bogus control flow**: `halt_baddata()`, `software_interrupt(0x303)` — dead code traps

### RTTI/metrics insight

The binary contains RTTI strings for `DRMSDK::VoucherDecryption` with metric keys:
- `VoucherVersionIDKey` — voucher version (e.g., 10005)
- `VoucherParseErrorCodeKey` — parse error code
- `IsAccountSecretUsedKey` — whether ACSR was used
- `IsDSNUsedKey` — whether device serial was used
- `IsDSNRequiredKey` — whether DSN is required
- `IsACSRRequiredKey` — whether ACSR is required
- `CountOfDSNUsedKey` — how many times DSN was referenced
- `CountOfACSRUsedKey` — how many times ACSR was referenced
- `VoucherVersionSymbolIDKey` — version symbol
- `ErrorCodeKey` — error code (we've seen ErrorCode 48)
- `InternalErrorKey` — internal error string
- `ScopeKey` — scope identifier

These are telemetry/metrics keys, confirming the SDK internally tracks exactly which secrets are used during voucher decryption.

---

## Final RE Assessment

### Static-only RE is impractical; dynamic on-device RE is still very viable

The function at `0x17c4fc` is a **244 KB control-flow-flattened state machine** with 136+ cases, anti-debugging traps, and opaque predicates. That makes a pure static recovery path unattractive.

However, we are **not limited to static RE**:
- we can run the real SDK on-device,
- we already control `LD_PRELOAD`,
- we can perturb inputs (`CLIENT_ID`, voucher, ACSR context),
- and we can observe concrete intermediate buffers at the crypto boundary.

So the right conclusion is **not** "stop RE". The right conclusion is:
- **do not try to fully deobfuscate `0x17c4fc` statically first**,
- **do use the live device as an oracle and instrument the function boundary aggressively**.

### Confirmed derivation pipeline so far

```
Voucher ION blob
    │
    ▼
VoucherDecryption::attachVoucher() [0x151200]
    │
    ├─► Stage-1 AES-256-CBC decrypt (hardcoded key + ACSR IV)
    │      → 40-char hex string (20 bytes)
    │
    ├─► Protocol/version gate via vtable[8] dispatcher [0x150c40]
    │      │
    │      └─► fcn_0x17c4fc (244KB obfuscated state machine)
    │            │  Consumes voucher-derived state + CLIENT_ID-sensitive state
    │            │  Reads ACSR from /var/local/java/prefs/acsr
    │            │  Produces HMAC key blob (variable size) + numeric string
    │            ▼
    │         HMAC-SHA256(hmac_key, numeric_string) → voucher_key_256
    │
    ├─► Stage-2 AES-256-CBC decrypt (voucher_key_256)
    │      → Decrypted book voucher
    │
    └─► Integrity HMAC-SHA256 check
```

### Why further RE is justified

The existing `LD_PRELOAD` hooks are enough for production decryption, but they do **not** answer the deeper questions we still care about:
- where exactly the real `CLIENT_ID` is consumed,
- what object layout / arguments feed `0x17c4fc`,
- what the pre-HMAC transformation boundary looks like,
- whether the custom path can be reduced to a much smaller black-box core,
- and whether older firmware follows the same dynamic contract.

Those are all still tractable with **dynamic, targeted, on-device instrumentation**.

---

## Revised RE Plan: Live On-Device First

### Phase A — Lock down the real runtime ABI

Goal: stop relying on imperfect decompiler guesses and capture the actual call contract.

1. Instrument entry to `0x150c40` and `0x17c4fc`
   - log `this`
   - log `this[0]` (vtable)
   - log `this[1]`
   - dump `*(this[1] + 0xc0)` and nearby fields
   - dump the relevant stack argument window on entry
   - record return value and output vector state before/after
2. Confirm which vtable/object instance is active for `PIDv3@1.0`
   - dump vtable slots 0..13 for the live object
   - correlate with static vtable `0x32f850`
3. Recover exact output container layout
   - identify which local / argument becomes the HMAC key vector
   - log pointer/start/end/capacity before and after `vtable[8]`

Success criterion:
- a concrete runtime struct/argument map for `0x150c40` and `0x17c4fc`

### Phase B — Isolate the CLIENT_ID ingestion point

Goal: find the narrowest point where the real device identifier influences the obfuscated path.

1. Correlate live inputs under controlled perturbation
   - baseline
   - wrong `CLIENT_ID`
   - missing/altered lock parameters
   - different voucher from same device
2. Add probes around the object fields consumed by `0x150c40`
   - especially `this[1] + 0xc0` and adjacent offsets
   - any pointer fields later used to build the HMAC key / numeric string
3. Intercept file reads / string construction for likely sources
   - `/proc/usid`
   - cached registration/device identifiers in Java prefs
   - any JNI bridge strings passed into DRMSDK
4. Patch the last in-memory candidate value just before `0x17c4fc`
   - confirm whether changing that one value is sufficient to perturb the HMAC key

Success criterion:
- identify the last responsible memory location for `CLIENT_ID`-sensitive input

### Phase C — Trace the obfuscated function as a black box

Goal: learn the internal contract without fully decompiling 244 KB of flattened code.

1. Log dispatch-state evolution at the switch hub
   - capture the case index / state byte at the top of the dispatch loop
   - compare traces for baseline vs wrong `CLIENT_ID`
   - find earliest divergence point
2. Attribute allocator activity to the call
   - hook allocator/grow/free paths used by the vector/string containers
   - dump new buffers associated with the `0x17c4fc` call frame
3. Snapshot selected intermediate buffers
   - buffers that later become HMAC key input
   - buffers that later become numeric-string HMAC data
   - any big integer / limb arrays with stable size patterns
4. Use differential tracing rather than full traces when possible
   - stop at first divergence
   - hash large buffers instead of dumping everything every run
   - only fully dump when a candidate boundary changes

Success criterion:
- a minimal set of intermediate states showing where baseline and wrong `CLIENT_ID` diverge

### Phase D — Collapse the black box into smaller subproblems

Goal: reduce `0x17c4fc` from one huge blob into a handful of meaningful subroutines or data transforms.

1. Enumerate all direct calls out of `0x17c4fc`
   - classify helpers as container management, math, parsing, or traps
2. Correlate helper calls with captured state transitions
   - especially helpers immediately before HMAC-key-buffer mutation
3. Identify invariant vs varying regions of the HMAC key blob
   - compare across books on same device
   - compare same book with wrong `CLIENT_ID`
   - compare books with different voucher sizes
4. Try to isolate a smaller reusable core
   - e.g. parser → canonicalized numeric material → obfuscated math core → output vector

Success criterion:
- a reduced graph of “inputs → helper cluster → output buffers” instead of one opaque monolith

### Phase E — Cross-firmware validation

Goal: determine whether PW2 and PW6 share the same dynamic contract even if the binaries differ.

1. Build armel instrumentation artifacts for PW2
2. Repeat Phase A/B on PW2 if a test device is available
3. Compare:
   - object layout
   - presence/absence of the `0x76` gate analogue
   - HMAC-key blob size patterns
   - divergence behavior under wrong `CLIENT_ID`

Success criterion:
- know whether the live-observed contract generalizes across old/new firmware

---

## Concrete next experiments

1. **Entry/exit hook for `0x150c40` / `0x17c4fc`**
   - add a tiny trampoline or inline patch on device
   - dump object pointers, key vector triple, and nearby fields
2. **Vector-growth attribution**
   - hook allocator/realloc used by the output vector during `vtable[8]`
   - map which allocations become the final HMAC key blob
3. **First-divergence tracing**
   - record dispatch-state sequence for baseline vs wrong `CLIENT_ID`
   - stop logging once the first differing state is found
4. **Last-writer identification**
   - hook writes into the final HMAC key buffer range before `HMAC()`
   - identify which helper/function last touched each region
5. **CLIENT_ID source patch test**
   - replace the last candidate identifier in memory immediately before `0x17c4fc`
   - confirm whether that alone reproduces the wrong-key behavior

---

## Working conclusion

- **Full static deobfuscation is the wrong first move.**
- **Dynamic RE on real hardware is the right move and should continue.**
- The immediate objective is to turn `0x17c4fc` into a **measured black box with known inputs, outputs, and divergence points**, not to fully lift 244 KB of flattened code into readable C.
- Once those boundaries are nailed down, we can decide whether any smaller subcomponent is worth extracting or reproducing offline.

---

## Phase A Results: Vtable Hook Works (2026-04-28)

### Instrumentation

- Deployed `phase_a_v2.so` via `LD_PRELOAD` on PW6
- Patched vtable slot 8 at runtime: replaced function pointer `0x150c41` with `hook_slot8`
- Hook fires on every `vtable[8]` call, dumps `this`/`this[1]`/args before and after
- No prologue patching needed — vtable in `.data.rel.ro` can be mprotected writable

### Key findings

1. **vtable slot 8 fires TWICE per voucher**
   - Call #1: HMAC key derivation (produces the 10330-byte key blob)
   - Call #2: HMAC integrity check (same key blob, different data)

2. **`this` object contains identifiable strings**
   - `this[4:9]` = `"client_restrictions"` (ASCII, little-endian)
   - Same string duplicated at `this[10:15]`

3. **`this[1]` is a rich configuration object**
   - `this[1]+0x44` = `"HmacSHA256"` (the HMAC algorithm)
   - `this[1]+0x54` = `"Purchase"`
   - `this[1]+0x80` = `"ARM.exidx"`
   - `this[1]+0xC0` = `0x76` (the version gate)
   - `this[1]+0x44` = `"HmacSHA256"` — confirmed SHA256 is the HMAC algorithm
   - Multiple ELF-like section names (`.gnu.build`, `.dynstr`, `data.re`) — appears to be a serialized map/properties object

4. **arg2 is the output vector**
   - Before call: `[NULL, NULL, NULL]`
   - After call: `[begin, end, capacity]` → std::vector layout
   - Contains the HMAC key blob (10330 bytes for "Three Below")

5. **Full vtable dump matches static analysis**
   - vtable[5] = 0x...8201 (slot 5 = attachVoucher → 0x151200 + base + 1)
   - vtable[8] = our hook pointer
   - vtable[9..13] match the static vtable at 0x32f850

---

## Phase B Results: CLIENT_ID Differential (2026-04-28)

### Method

- Baseline: `GR733X1151821324` (correct device serial)
- Wrong CID: `XXXXXXXXXXXXXXXX` (16 X's)
- Same voucher file, same ACSR

### Critical differences

| Field | Baseline | Wrong CID |
|-------|----------|------------|
| HMAC key len | **10330** | **9690** |
| HMAC data len | 856 | 860 |
| HMAC result | `eb035e31...` | `1db9f17b...` |
| `this[3]` | 0x1d (29) | **0xc0d (3085)** |
| `this[4:9]` | `"client_restrictions"` | **0x400, 0x80, 0x6, 0x5, 0x300** |
| `this[1]` strings | `"HmacSHA256"`, `"Purchase"` | **numeric codes** |
| HMAC key prefix | `058b1d8b...` | **`1d7b7b6b...`** |
| Exit code | 0 | 1 (error) |

### Interpretation

1. **`this` object layout is COMPLETELY different between runs**
   - The voucher parsing produces a different internal structure when CLIENT_ID is wrong
   - This suggests CLIENT_ID is consumed BEFORE `vtable[8]` is called — during voucher parsing, not inside the obfuscated crypto
   - The object fields change because the parsed voucher data differs (wrong decryption of the voucher metadata)

2. **The HMAC key blob is entirely different** (different prefix, different length)
   - Wrong CID produces 9690-byte key vs 10330-byte correct key
   - This is consistent with earlier perturbation matrix findings

3. **The version gate `0x76` is present in BOTH cases** — both take the obfuscated path

4. **CLIENT_ID influences the voucher parsing, not just the crypto engine**
   - The different `this[3]` values (29 vs 3085) suggest different parsed structures
   - The wrong CID case may be producing garbage from the voucher parsing stage

### Key insight: CLIENT_ID enters BEFORE vtable[8]

The `this` object is already different when `vtable[8]` is called. This means CLIENT_ID is consumed in the caller (`attachVoucher` at 0x151200) or in voucher parsing (before vtable[8] is dispatched). The obfuscated crypto at `0x17c4fc` receives already-different inputs.

---

## Phase D Results: Helper Call Enumeration from 0x17c4fc (2026-04-28)

### Summary

The 244 KB obfuscated function at `0x17c4fc` makes:
- **164 indirect calls via `BLX R3`** — the dominant call pattern (virtual dispatch / function pointers)
- **11 indirect calls via `BLX R2`**
- **~200+ direct `BL` calls** to unique targets
- **4 `BLX R4`**, **3 `BLX LR`**
- Total: ~400+ function calls

### Call classification

The BLX_R (indirect register calls) dominate:
- `BLX R3`: 164 calls — this is the primary dispatch mechanism for the obfuscated state machine
- `BLX R2`: 11 calls — secondary dispatch

Most unique `BL` targets are called only once each. Many addresses are outside the function range (> 0x1b8086), meaning they call into other library functions.

### Notable local calls (within the function)

Several `BL` targets are within the function body itself (subroutines):
- `0x17c2e2`, `0x17c662`, `0x17ca40`, `0x17ca8e`, `0x17ca2e` — internal helpers near the function start
- `0x17be2a`, `0x17e33e`, `0x17fadc` — internal helpers
- `0x1a9852`, `0x1ae380`, `0x1ae690`, `0x1ae89e`, `0x1ae9e8`, `0x1aeb2c` — larger internal helpers
- `0x1b016e`, `0x1b060c`, `0x1b11a4`, `0x1b2c98` — helpers near the end

### Notable external calls

- `0x160c1c`, `0x160ab6`, `0x164a42`, `0x1646ba`, `0x165574`, `0x1653b0` — functions in nearby library code
- `0x1b27d4` — the shared EVP decrypt helper (not in the BL list but called via BLX)
- `0x1847e` — very low address, possibly PLT entry or utility
- `0x5xxxx` addresses — standard library functions (malloc, free, memcpy, etc.)

### Conclusion from Phase D

The function is heavily decomposed into many small sub-calls. The 164 BLX R3 calls suggest a function-pointer dispatch table (part of the control-flow flattening). The ~200 unique BL targets are a mix of:
- Internal subroutines (math operations)
- Container management (vector grow, string operations)
- Library calls (crypto, memory)
- Obfuscation noise (dead code, traps)

A complete static classification would require resolving each BLX R3 target dynamically.

---

## Phase C Results: Differential Binary Analysis (2026-04-28)

### Instrumentation

Deployed `phase_c_hook.so` with enhanced binary dumps:
- Full `this` (256 bytes) and `this[1]` (1024 bytes) to binary files
- Stage-1 decrypt plaintext capture via `EVP_DecryptUpdate`
- HMAC data (numeric string) capture
- HMAC key blob capture

### Key finding: stage-1 decrypt is IDENTICAL

The stage-1 AES-256-CBC decrypt (hardcoded key + ACSR IV) produces **exactly the same output** regardless of CLIENT_ID:
```
plaintext = "54e869c4b43348062477a52df5467be8c4e08420" (hex string)
```

This confirms: **CLIENT_ID does NOT influence stage-1**.

### HMAC data (numeric string) diverges at byte 5

```
baseline:   44888|614562856883288884488237601316715176972376...
wrong_cid:  44888|8322376016192868888448814561316715176093632...
                  ^ divergence at byte 5
```

The common prefix is only 5 characters (`44888`). This means the custom crypto's first significant computation already differs between baseline and wrong CID.

### HMAC key blob diverges at byte 0

```
baseline:   05 8b 1d 8b 7a 8b 7a 8b 7a 9b 6a 9b ...
wrong_cid:  1d 7b 7b 6b 6b 6b 6b 6b 6b 6b 6b 6b ...
              ^^^ completely different from byte 0
```

The HMAC key blobs share no common prefix at all.

### Numeric string analysis

The HMAC data strings contain repeated numeric tokens:
- `44888` appears as a common prefix
- `15176` appears frequently (voucher version ID?)
- `1456` appears frequently
- `16192` appears only in wrong CID

These look like parsed ION integer values or computed numeric identifiers.

### Summary of divergence chain

```
Stage-1 AES (hardcoded key)  → IDENTICAL (not CLIENT_ID-dependent)
                                  ↓
Custom crypto (0x17c4fc)      → DIVERGES at byte 5 of output numeric string
                                  ↓
HMAC key blob                 → COMPLETELY DIFFERENT from byte 0
                                  ↓
HMAC result                   → DIFFERENT
                                  ↓
Stage-2 AES                   → Uses wrong key → DECRYPTION FAILS
```

### Conclusion: CLIENT_ID enters the custom crypto at the very start

The divergence at byte 5 of the numeric string (which is produced by `0x17c4fc`) means CLIENT_ID is consumed within the first few operations of the obfuscated state machine. This is consistent with the `this` object being different — the object fields carry CLIENT_ID-derived data into the function.

---

## Ingestion Site Analysis (2026-04-28)

### Call order discovery

The actual call order is **NOT** vtable[5] then vtable[8]:

```
1. DEC_256 (stage-1 AES, hardcoded key)
2. SLOT8 #1 (vtable[8] — custom crypto + HMAC derivation)
3. HMAC (derives voucher_key_256)
4. SLOT5 #1 (vtable[5] — attachVoucher, stage-2 decrypt)
5. DEC_256 (stage-2 AES, uses voucher_key_256)
```

**vtable[8] fires BEFORE vtable[5]** — the crypto dispatch happens during voucher parsing, not during attachVoucher.

### The strategy object

`fcn.0015134c` (inner function at offset `0x15134c`) is the bridge:

```asm
0x15134c: push.w {r4-r10, lr}   ; function prologue
0x15135c: ldr r3, [r0]          ; load vtable from strategy object (r0 = arg1)
0x151368: ldr r3, [r3, #0x20]   ; load vtable[8]
0x151370: blx r3                ; call crypto dispatch
```

The strategy object is passed as `r0` (first argument). Its vtable is at `0x32f850`.

### Strategy object is IDENTICAL between baseline and wrong CID

```
baseline:  506809a4 4893d0b5 c88fdb5 0d0c0000 00040000 80000000 06000000 05000000
wrong_cid: 506809a4 001cd9b5 9890dfb5 0d0c0000 00040000 80000000 06000000 05000000
                       ^^^^^^^^^^^^^^^^^ only pointer fields differ (ASLR)
```

Bytes 12+ are **byte-identical**. Only `this[1]` and `this[2]` (pointer fields) differ due to ASLR.

### this[1] is the same class but different instance data

- Both have vtable at file offset `0x32f248` (same class)
- Baseline instance: small numeric tag values (`0x00080700`, `0x00740500`)
- Wrong CID instance: different values at the same offsets
- The `this[1]+0xC0` = `0x76` in both cases (version gate passes)

### this[2] sub-object

- `this[2]` contains a `"false"` string at offset `0x30` in both cases
- Differs at the end: baseline has zeros, wrong CID has `0x05000000`
- Likely contains voucher parsing results (booleans, flags)

### Allocation tracing

- HMAC key output vector matches the last `std::vector` realloc (16384 bytes, doubling from 128)
- All 20 allocations come from `libstdc++.so.6` vector growth
- The `__builtin_return_address(1)` parent is unresolved — the obfuscated code uses `BLX R3` indirect calls without proper frame chains
- Same allocation pattern in both baseline (10330-byte output) and wrong CID (9690-byte output)

### Key conclusion

CLIENT_ID is consumed **before** `fcn.0015134c` is called — during the voucher parsing loop in `fcn.00151200`. The strategy object's static fields are identical, but `this[1]` (the instance configuration object) contains different values that encode the CLIENT_ID-dependent state. The obfuscated function at `0x17c4fc` then processes this `this[1]` data, producing the different HMAC key blob and numeric string.

### Still open

1. **How `this[1]` is populated**: which voucher parsing helper writes CLIENT_ID-dependent data into `this[1]`
2. **CLIENT_ID value storage**: the key name is at `this1[0xf4]+0x58` but the value is stored as a JNI Java String object (not a C string), making it harder to extract from C hooks

---

## Object Graph Analysis (2026-04-28)

### Lock parameter storage structure

The `this[1]` object (vtable `0x32f248`) contains parsed voucher data with the following layout:

| this1 offset | Content | Details |
|-------------|---------|--------|
| `0xc0` | version field | `0x76` (protocol version gate) |
| `0xc4` | voucher blob ptr | Raw voucher bytes starting with `\xe0\x01\x00\xea` (voucher magic) |
| `0xc8` | voucher ref ptr | `atv:kin:2:<base64_token>` — Amazon device token |
| `0xf4` | lock params ptr | Structure containing ACCOUNT_SECRET and CLIENT_ID |
| `0xf4+0x40` | ACCOUNT_SECRET key | String "ACCOUNT_SECRET" |
| `0xf4+0x58` | CLIENT_ID key | String "CLIENT_ID" |
| `0xf4+0x70` | ACSR value | Base64 string (ACCOUNT_SECRET value) |

### Lock params structure at this1[0xf4]

```
0x30: 00 04 00 00 3d 00 00 00 <ptr> 0e 00 00 00
0x40: ACCOUNT_SECRET \x00\x00
0x50: <ptr> 09 00 00 00
0x58: CLIENT_ID \x00
0x60: <ptr> <ptr> <ptr> <ptr> 85 00 00 00
0x70: T1lGaDFZN2dVK1k2ZTBYTzBNSnV1L2...  (base64 ACSR value)
```

### Where CLIENT_ID VALUE is stored

The CLIENT_ID **key name** is at `this1[0xf4]+0x58`, but the actual value (device serial) is NOT stored as a plain C string anywhere in the reachable object graph. The serial was not found in any of:
- `this[1]` raw bytes (1024 bytes)
- `this1[0xf4]` target (8192 bytes)
- All inner pointer targets (4096 bytes each)
- `this1[0xf0]` target (4096 bytes)
- All pointer targets in `this1[0xf0..0x120]` range

This strongly suggests the CLIENT_ID value is stored as a **JNI Java String object** in the JVM heap, not as a C string. The native code accesses it via JNI call or through the Java string's internal byte array.

### Voucher reference string

Found at `this1[0xc8]`:
```
atv:kin:2:GEyx9Sg/VHuLK0zs2pVe33rWhA2v6nZxG8SO5dZn+hIefVrQMB...
```

This is the Amazon device token (DRM voucher reference) containing a base64-encoded identity credential.

### Allocation tracing for HMAC key blob

- HMAC key output vector = last `std::vector` realloc (16384 bytes)
- Growth pattern: 128→256→512→1024→2048→4096→8192→16384 (classic doubling)
- 20 allocations total during SLOT8, all from `libstdc++.so.6` vector internals
- Baseline: output 10330 bytes; wrong CID: output 9690 bytes (same allocation count)
- `__builtin_return_address(1)` = unresolved (obfuscated BLX R3 calls break frame chain)

### Internal helper classification

| Address | Type | Notes |
|---------|------|-------|
| `0x17ca40` | Switch case handler | Named "case.0x17c5ec.123" — part of the state machine dispatch |
| `0x17e33e` | Helper function | Similar structure to case handlers |
| `0x1ae690` | Trap | `halt_baddata()` — dead code / anti-debug |
| `0x1ae89e` | Unknown | r2 couldn't analyze |
| `0x17be2a` | Unknown | r2 couldn't analyze |
| `0x1825d6` | Unknown | r2 couldn't analyze |

### Summary of mechanistic understanding

```
Voucher ION blob + atv:kin device token
    │
    ▼
fcn.00151200 (voucher parsing loop)
    │  Reads CLIENT_ID and ACCOUNT_SECRET from lock params
    │  Builds this[1] config object (vtable 0x32f248)
    │  Stores: version, voucher blob, device token, lock params
    │
    ▼
fcn.0015134c (bridge function)
    │  arg1 = strategy object (from parsed voucher)
    │  strategy->vtable[8]() → dispatches to crypto
    │
    ▼
0x150c40 (version gate)
    │  if *(this[1]+0xc0) == 0x76 → 0x17c4fc (obfuscated engine)
    │  else → vtable[9]/[10] (legacy path)
    │
    ▼
0x17c4fc (244KB obfuscated state machine)
    │  Reads this[1] config (contains CLIENT_ID via JNI)
    │  Reads this[1]+0xc4 voucher blob
    │  Reads this[1]+0xc8 device token
    │  Internal state machine (136+ cases, BLX R3 dispatch)
    │  Writes to output vector via std::vector (20 reallocs)
    │  Produces: HMAC key blob + numeric string
    │
    ▼
HMAC-SHA256(hmac_key, numeric_string) → voucher_key_256
    │
    ▼
Stage-2 AES-256-CBC(voucher_key_256) → decrypted voucher
    │
    ▼
HMAC-SHA256 integrity check
```

### What we now fully understand

1. ✅ **Where lock parameters are stored**: `this1[0xf4]` structure with ACCOUNT_SECRET and CLIENT_ID keys
2. ✅ **The voucher reference format**: `atv:kin:2:<base64>` at `this1[0xc8]`
3. ✅ **The version gate**: `this1+0xc0 == 0x76` determines which crypto path
4. ✅ **The allocation pattern**: std::vector doubling, 20 reallocs, last one = output
5. ✅ **The full call chain**: parsing → bridge → gate → state machine → HMAC → AES

### What remains unclear

1. **How CLIENT_ID value is accessed inside 0x17c4fc**: stored as JNI Java String, not directly readable from C
2. **Which specific fields in this[1] are read by the state machine**: all 60 fields differ, unclear which matter
3. **The internal structure of the state machine**: 136 cases, BLX R3 dispatch, needs runtime tracing
4. **What the numeric string represents**: diverges at byte 5, contains repeated tokens like 44888, 15176, 1456

## CLIENT_ID Value Location (2026-04-28, phase 2)

### Heap memory scan results

Using direct `memcpy`-based scanning (since `/proc/self/mem` is permission-denied on Kindle):

| Pattern | Occurrences | Key locations |
|---------|-------------|---------------|
| `GR733X1151821324` | 3 real + log artifacts | f4-0x50 (RB-tree node), region+0x2cad8 (separate node), region+0x3e7a0 (with stage-1 output) |
| `CLIENT_ID` | 26 | Multiple RB-tree nodes in lock params map |
| `ACCOUNT_SECRET` | 26 | Same map |

### CLIENT_ID value stored in RB-tree node

The serial `GR733X1151821324` is stored as a **heap-allocated std::string** (length=16, capacity=16) in an RB-tree node located at `f4 - 0x50`:

```
f4-0x50: node allocation start
f4-0x30: actual string chars "GR733X1151821324"  
f4-0x18: std::string._M_p → points to f4-0x30
f4-0x14: std::string._M_length = 16
f4-0x10: std::string._M_capacity = 16
```

### setLockParameters JNI bridge (0xdc9c in libYJSDKJNI.so)

The function:
1. Receives `jobjectArray` containing `[key, value]` string pairs
2. Iterates, calling `GetStringUTFChars` for each key and value
3. Stores as `std::string` pairs in `std::map<std::string, std::string>` (RB-tree)
4. Sorts into tree using `_Rb_tree_insert_and_rebalance`

### Memory co-location with stage-1 output

The third occurrence of the serial (at `f4 + 0x3e110`) is co-located with the stage-1 decrypted output `54e869c4b43348062477a52df5467be8c4e08420` in a separate RB-tree node. This confirms the SDK maintains internal bookkeeping that associates the serial with derived keys.

### Key JNI functions in libYJSDKJNI.so

| Address | Size | Function |
|---------|------|----------|
| `0xdc9c` | 0x280 | `setLockParameters` — parses key/value pairs from Java |
| `0xd88c` | 0x64 | `attachVoucher` — attaches voucher file to security object |
| `0xd8f0` | 0x7c | `getInstance` — creates IBookSecurity native instance |
| `0xd81c` | 0x70 | `setAccountSecrets` — sets account secret (ACSR) |
| `0xd9e4` | 0x118 | `getSupportedVoucherVersions` — returns supported versions |

### setLockParameters call sequence

```
Java: sec.setLockParameters(Map.of("ACCOUNT_SECRET", acsr, "CLIENT_ID", serial))
  ↓ JNI
0xdc9c: push {r4-r10,fp,lr}
  ↓ GetArrayLength (vtable[0x2ac])
  ↓ loop over array:
  │  GetObjectArrayElement[i*2]   → key jstring
  │  GetStringUTFChars(key)       → C string ("ACCOUNT_SECRET" or "CLIENT_ID")
  │  GetObjectArrayElement[i*2+1] → value jstring
  │  GetStringUTFChars(value)     → C string (ACSR or serial)
  │  std::string constructor (0xd63c)
  │  vector::push_back → std::map via RB-tree insertion
  ↓ ReleaseStringUTFChars, DeleteLocalRef
0xdecc: stack check + return
```

## HMAC Numeric Data Analysis (2026-04-28)

### The HMAC input is a DECIMAL DIGIT STRING

The 856/860-byte "numeric string" passed to HMAC-SHA256 is purely decimal digits.

**Token frequency analysis:**
| Token | Baseline count | Wrong CID count | Interpretation |
|-------|---------------|-----------------|----------------|
| `44888` | prefix | prefix | Common ION/voucher structure prefix |
| `1456` | 10 | 9 | **Symbol/value from correct CLIENT_ID** |
| `16192` | 1 | 16 | **Symbol/value from wrong CLIENT_ID** |
| `15176` | appears | appears | Common voucher field |
| `3760` | appears | appears | Common voucher field |

### The state machine produces different numeric strings depending on CLIENT_ID

The divergence happens at byte 5:
- Baseline: `44888|6|14562856883288884488...`
- Wrong CID: `44888|8|3223760161928688884...`

This suggests the state machine converts the voucher ION structure into a decimal digit sequence, and the CLIENT_ID value influences how certain ION symbols are encoded.

### BLX R3 dispatch analysis

43 `BLX R3` instructions identified in the state machine (0x17c4fc-0x1a1000):

| Address | Address | Address | Address |
|---------|---------|---------|---------|
| 0x17c74e | 0x17c77e | 0x17e168 | 0x17ee04 |
| 0x18029a | 0x180354 | 0x18137e | 0x182b94 |
| 0x184030 | 0x1840e8 | 0x184b78 | 0x184b8e |
| 0x185fa8 | 0x186168 | 0x186a28 | 0x186a3e |
| 0x186d7a | 0x1885f8 | 0x188762 | 0x188d60 |
| 0x188d76 | 0x18a7da | 0x18a940 | 0x18ada8 |
| 0x18adbe | 0x18b0e6 | 0x18b3f8 | 0x18e604 |
| 0x18e864 | 0x18e87a | ... | (43 total) |

Runtime resolution requires trampoline patching — deferred to future work.

### Complete mechanistic model (as understood)

```
INPUT STAGE:
  1. Java: setLockParameters({ACCOUNT_SECRET: acsr, CLIENT_ID: serial})
     → JNI bridge at 0xdc9c calls GetStringUTFChars
     → Stores as std::map<string,string> in RB-tree
     
  2. Java: attachVoucher(voucher_file)
     → JNI bridge at 0xd88c calls vtable[5] = 0x151200
     → Voucher parsing loop constructs strategy object
     → this[1] populated with:
       +0xc0: version (0x76)
       +0xc4: raw voucher blob pointer
       +0xc8: atv:kin device token pointer
       +0xf4: lock params map pointer

DERIVATION STAGE:
  3. fcn.0015134c bridges to vtable[8] = 0x150c40
  4. 0x150c40 checks version gate → calls 0x17c4fc (244KB state machine)
  5. 0x17c4fc reads this[1] config:
     - Reads CLIENT_ID from RB-tree at f4-0x50 (std::string, 16 chars)
     - Reads ACCOUNT_SECRET from RB-tree
     - Reads voucher blob from this[1]+0xc4
     - Reads device token from this[1]+0xc8
  6. State machine produces:
     (a) HMAC key blob: 10330 bytes (baseline) / 9690 bytes (wrong CID)
         → written to output std::vector (arg2)
     (b) Numeric string: 856/860 decimal digits
         → token "1456" vs "16192" is the CLIENT_ID fingerprint

CRYPTO STAGE:
  7. HMAC-SHA256(hmac_key_blob, numeric_string) → voucher_key_256
  8. AES-256-CBC(voucher_key_256, inner_IV) → decrypted voucher
  9. HMAC-SHA256 integrity check (SLOT8 fires second time)

KEY OBSERVATIONS:
  - CLIENT_ID value enters at step 5 (state machine reads it from RB-tree)
  - The state machine encodes CLIENT_ID into the numeric string as token 1456/16192
  - Different CLIENT_ID → different numeric string → different HMAC → different voucher key → ErrorCode 48
  - The stage-1 decrypt (hardcoded key + ACSR IV) is IDENTICAL regardless of CLIENT_ID
  - The entire derivation is inside the 244KB obfuscated state machine
```

## Numeric String Differential Analysis (2026-04-28)

### Common prefix: 604 chars

The first 604 characters of the HMAC numeric string are **IDENTICAL** between baseline and wrong CID. This means ~70% of the derivation is CLIENT_ID-independent.

### Tail divergence (positions 604+)

After the common prefix, differences are scattered in small clusters (1-27 chars):
- Baseline tail: `448815176538844592440404187362...`
- Wrong CID tail: `53001517615176180323080...`

### Token frequency

| Token | Baseline | Wrong CID | Delta |
|-------|----------|-----------|-------|
| `1456` | 10 | 9 | -1 |
| `16192` | 1 | **16** | **+15** |

### Digit frequency shift

| Digit | Baseline | Wrong CID | Delta |
|-------|----------|-----------|-------|
| `1` | 118 | 144 | +26 |
| `2` | 75 | 94 | +19 |
| `9` | 50 | 66 | +16 |
| `0` | 120 | 106 | -14 |
| `4` | 95 | 82 | -13 |

The shift from `1456` to `16192` increases digits 1, 2, 9 while decreasing 0, 4.

### Voucher ION structure (decoded fields)

The voucher contains these ION-encoded fields:

| Offset | Field | Value |
|--------|-------|-------|
| 0x038 | ACCOUNT_SECRET | (lock parameter key) |
| 0x047 | CLIENT_ID | (lock parameter key) |
| 0x052 | AES | (encryption algorithm) |
| 0x052 | AES/CBC/PKCS5Padding | (cipher spec) |
| 0x06e | HmacSHA256 | (MAC algorithm) |
| 0x329 | Purchase | (license type) |
| 0x335 | atv:kin:2:1YMxUy/... | (device identity token) |
| 0x400 | client_restrictions | (container struct) |
| 0x41a | ClippingLimit | 30 |
| 0x430 | TextToSpeechDisabled | false |

### Interpretation

The "1456"/"16192" tokens likely represent a **numeric encoding of the CLIENT_ID value** — some function of the device serial that produces a short integer. With the correct serial, this function yields ~1456 (or "14" "56" as separate values), while with a wrong serial it yields ~16192.

The state machine:
1. Parses the voucher ION structure
2. Extracts lock parameters (ACCOUNT_SECRET, CLIENT_ID)
3. Computes a numeric value from CLIENT_ID (the "1456"/"16192" token)
4. Constructs the full numeric string by encoding all voucher fields as decimal tokens
5. If the token doesn't match the expected value → error (wrong CID → ErrorCode 48)

### State machine timing

| Case | Duration | Exit Code |
|------|----------|-----------|
| Baseline | 1376 μs | 0 (success) |
| Wrong CID | 1116 μs | 1 (error) |

Wrong CID is **faster** — the error is detected early in the derivation, before completing the full computation.

### Static call graph from 0x17c4fc

25 unique direct BL targets, 43 BLX R3 indirect targets:

**Hottest direct targets:**
| Address | Calls | Function |
|---------|-------|----------|
| 0x1818e0 | 8 | std::vector::insert (realloc) |
| 0x181b50 | 5 | std::vector (nested indexing) |
| 0x264ac0 | 4 | **Obfuscated helper** (movw/movt constants) |
| 0x26398c | 2 | **Obfuscated helper** (movw/movt constants) |
| 0x181a78 | 2 | std::vector allocation (operator new) |
| 0x1819bc | 2 | std::vector element cleanup |
| 0x15fc58 | 2 | std::vector::push_back (byte vector) |

**Obfuscated constants (from 0x264ac0):**
- 0x89259a5d, 0xe4300578, 0xf15c8d93, 0xebe2a8b7
- Not matching SHA-256, SHA-1, MD5, MT19937, or any standard crypto constants
- Custom DRM-specific values

**Obfuscated constants (from 0x26398c):**
- 0x4528a1e4, 0x023d2a8d, 0xe1029034, 0x0213bff3
- Also custom DRM values
