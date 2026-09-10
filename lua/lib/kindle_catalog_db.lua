-- Kindle content-catalog SQLite connection support.
--
-- KOReader must access /var/local/cc.db directly because No Framework mode
-- stops Amazon's content-catalog service.  This module keeps that one direct
-- SQLite path aligned with the firmware contract where writes need more than
-- stock SQLite provides:
--   * reconstruct Kindle's firmware-defined "icu" collation with generic system
--     ICU before changing p_lastAccess (EntriesLastAccessIndex depends on it), and
--   * register the current firmware's audit-trigger scalar functions so
--     trigger-bearing schemas can prepare the same UPDATE without Amazon ccat.

local ffi = require("ffi")
local json = require("json")
local lfs = require("libs/libkoreader-lfs")
local logger = require("logger")

local KindleCatalogDb = {}

local COLLATION_STATE_SQL = [[
SELECT
    (SELECT locale FROM Locale LIMIT 1),
    (SELECT collation FROM Collation LIMIT 1)
]]

local function jsonString(value)
    return json.encode(tostring(value))
end

local JSON_NULL = {}

local function orNull(value)
    if value == nil then
        return JSON_NULL
    end
    return value
end

local function jsonValue(value)
    if value == JSON_NULL then
        return "null"
    end
    local value_type = type(value)
    if value_type == "number" then
        return tostring(value)
    elseif value_type == "boolean" then
        return value and "true" or "false"
    end
    return jsonString(value)
end

local function encodeFlatObject(fields)
    local parts = {}
    for _, field in ipairs(fields) do
        if field.include then
            table.insert(parts, jsonString(field.name) .. ":" .. jsonValue(field.value))
        end
    end
    return "{" .. table.concat(parts, ",") .. "}"
end

-- These callbacks mirror the functions installed by 5.19.6
-- /usr/lib/ccat/sql_functions.lua. Registering them is cheaper than scanning
-- sqlite_master for trigger-bearing schema variants, and unused functions are
-- inert on schemas without those triggers.
local firmwareSqlFunctions = {
    get_entry_external_id = function(p_type, p_uuid, p_cde_key, p_cde_type, p_cde_group)
        if not p_type then
            return nil
        elseif p_type == "Collection" then
            return p_uuid
        elseif p_type == "Entry:Item:Series" then
            if p_cde_key and p_cde_key ~= "" then
                return p_cde_key
            elseif p_cde_group and p_cde_group ~= "" then
                return string.sub(p_cde_group, -10)
            end
            return nil
        elseif p_type == "Entry:Item:PVC" then
            return nil
        elseif p_cde_key and p_cde_type then
            return p_cde_key .. "!!" .. p_cde_type
        end
        return nil
    end,

    get_entry_change_type = function(p_type)
        if not p_type then
            return nil
        elseif p_type == "Collection" then
            return "COLLECTION"
        elseif p_type == "Entry:Item:Series" then
            return "SERIES"
        elseif p_type == "Entry:Item:PVC" then
            return "PERIODICAL_COLLECTION"
        end
        return "LIBRARY_ITEM"
    end,

    get_companion_relation_external_id = function(p_cde_key, p_cde_type)
        if not p_cde_key or not p_cde_type then
            return nil
        end
        return p_cde_key .. "!!" .. p_cde_type .. "!!HAS_COMPANION"
    end,

    build_merge_changes = function(
        p_type,
        p_conversion_status,
        p_last_access,
        p_read_state,
        p_titles_0_nominal,
        p_titles_0_pronunciation,
        p_titles_0_collation,
        p_is_visible_in_home
    )
        if p_type == "Collection" then
            return encodeFlatObject({
                { name = "p_titles_0_nominal", value = p_titles_0_nominal, include = p_titles_0_nominal ~= nil },
                { name = "p_titles_0_pronunciation", value = p_titles_0_pronunciation, include = p_titles_0_pronunciation ~= nil },
                { name = "p_titles_0_collation", value = p_titles_0_collation, include = p_titles_0_collation ~= nil },
                { name = "p_lastAccess", value = p_last_access, include = p_last_access ~= nil },
                { name = "p_isVisibleInHome", value = p_is_visible_in_home, include = p_is_visible_in_home ~= nil },
            })
        end

        local any = p_conversion_status ~= nil or p_last_access ~= nil or p_read_state ~= nil
        return encodeFlatObject({
            { name = "p_conversionStatus", value = orNull(p_conversion_status), include = any },
            { name = "p_lastAccess", value = orNull(p_last_access), include = any },
            { name = "p_readState", value = orNull(p_read_state), include = any },
        })
    end,

    build_merge_changes_delta = function(
        p_type,
        new_conversion_status,
        old_conversion_status,
        new_last_access,
        old_last_access,
        new_read_state,
        old_read_state,
        new_titles_0_nominal,
        old_titles_0_nominal,
        new_titles_0_pronunciation,
        old_titles_0_pronunciation,
        new_titles_0_collation,
        old_titles_0_collation,
        new_is_visible_in_home,
        old_is_visible_in_home
    )
        if p_type == "Collection" then
            return encodeFlatObject({
                {
                    name = "p_titles_0_nominal",
                    value = orNull(new_titles_0_nominal),
                    include = new_titles_0_nominal ~= old_titles_0_nominal,
                },
                {
                    name = "p_titles_0_pronunciation",
                    value = orNull(new_titles_0_pronunciation),
                    include = new_titles_0_pronunciation ~= old_titles_0_pronunciation,
                },
                {
                    name = "p_titles_0_collation",
                    value = orNull(new_titles_0_collation),
                    include = new_titles_0_collation ~= old_titles_0_collation,
                },
                { name = "p_lastAccess", value = orNull(new_last_access), include = new_last_access ~= old_last_access },
                {
                    name = "p_isVisibleInHome",
                    value = orNull(new_is_visible_in_home),
                    include = new_is_visible_in_home ~= old_is_visible_in_home,
                },
            })
        end

        return encodeFlatObject({
            {
                name = "p_conversionStatus",
                value = orNull(new_conversion_status),
                include = new_conversion_status ~= old_conversion_status,
            },
            { name = "p_lastAccess", value = orNull(new_last_access), include = new_last_access ~= old_last_access },
            { name = "p_readState", value = orNull(new_read_state), include = new_read_state ~= old_read_state },
        })
    end,
}

local function registerEntryTriggerFunctions(conn)
    if type(conn.setscalar) ~= "function" then
        return
    end
    for name, callback in pairs(firmwareSqlFunctions) do
        conn:setscalar(name, callback)
    end
end

local icu_cdefs = {}
local loaded_icu_runtime

-- Reverse engineered from Kindle 5.19.6 libccat::localeMappings.  The first
-- matching locale prefix is opened from ICU's short-string syntax; any suffix
-- after ':' is translated into the same script reorder list used by ccat.
local KINDLE_LOCALE_MAPPINGS = {
    { prefix = "en_US_POSIX", short = "Lroot" },
    { prefix = "zh", short = "Lzh:Hani" },
    { prefix = "ja", short = "Lja_S4_HO:Hira,Kana,Hani" },
    { prefix = "ru", short = "Lru:y.Cyrl" },
}

local U_ZERO_ERROR = 0
local UCOL_NUMERIC_COLLATION = 7
local UCOL_ON = 17
local UCOL_REORDER_CODE_OTHERS = 0x1004
local UCOL_DEFAULT = -1
local SQLITE_UTF8 = 1

local base_ffi_declared = false
local function declareBaseFfi()
    if base_ffi_declared then
        return true
    end
    local ok, err = pcall(
        ffi.cdef,
        [[
            typedef int (*kindle_sqlite_compare_cb)(void *, int, const void *, int, const void *);
            int sqlite3_create_collation(
                void *db,
                const char *name,
                int text_rep,
                void *context,
                kindle_sqlite_compare_cb compare
            );
        ]]
    )
    if not ok and not tostring(err):find("redef", 1, true) then
        return false, err
    end
    base_ffi_declared = true
    return true
end

local function declareIcu(major)
    if icu_cdefs[major] then
        return true
    end
    local declaration = string.format(
        [[
            void *ucol_open_%d(const char *locale, int32_t *status);
            void *ucol_openFromShortString_%d(const char *definition, int force_defaults, void *parse_error, int32_t *status);
            void *ucol_openRules_%d(
                const uint16_t *rules,
                int32_t rules_length,
                int normalization_mode,
                int strength,
                void *parse_error,
                int32_t *status
            );
            void ucol_close_%d(void *collator);
            int ucol_strcollUTF8_%d(
                const void *collator,
                const char *left,
                int32_t left_length,
                const char *right,
                int32_t right_length,
                int32_t *status
            );
            void ucol_setAttribute_%d(void *collator, int attribute, int value, int32_t *status);
            int ucol_getAttribute_%d(const void *collator, int attribute, int32_t *status);
            int ucol_getStrength_%d(const void *collator);
            const uint16_t *ucol_getRules_%d(const void *collator, int32_t *length);
            int32_t ucol_getReorderCodes_%d(const void *collator, int32_t *dest, int32_t dest_capacity, int32_t *status);
            void ucol_setReorderCodes_%d(void *collator, const int32_t *reorder_codes, int32_t reorder_codes_length, int32_t *status);
            int32_t uloc_getLanguage_%d(const char *locale_id, char *language, int32_t language_capacity, int32_t *status);
            int32_t uscript_getCode_%d(const char *name_or_abbr_or_locale, int32_t *fill_in, int32_t capacity, int32_t *status);
            int32_t u_strlen_%d(const uint16_t *s);
            uint16_t *u_strcpy_%d(uint16_t *dst, const uint16_t *src);
            uint16_t *u_strcat_%d(uint16_t *dst, const uint16_t *src);
            uint16_t *u_strstr_%d(const uint16_t *s, const uint16_t *substring);
            int32_t u_strFromUTF8_%d(
                uint16_t *dest,
                int32_t dest_capacity,
                int32_t *dest_length,
                const char *src,
                int32_t src_length,
                int32_t *status
            );
        ]],
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major,
        major
    )
    local ok, err = pcall(ffi.cdef, declaration)
    if not ok and not tostring(err):find("redef", 1, true) then
        return false, err
    end
    icu_cdefs[major] = true
    return true
end

local function buildIcuCandidates(names)
    local majors = {}

    local function record(kind, name)
        local stem = kind == "i18n" and "libicui18n" or "libicuuc"
        local major = tonumber(name:match("^" .. stem .. "%.so%.(%d+)$") or name:match("^" .. stem .. "%.so%.(%d+)%..+$"))
        if not major then
            return
        end

        local entry = majors[major] or { major = major }
        local exact_major_name = stem .. ".so." .. major
        if not entry[kind] or name == exact_major_name then
            entry[kind] = "/usr/lib/" .. name
        end
        majors[major] = entry
    end

    for _, name in ipairs(names) do
        record("i18n", name)
        record("uc", name)
    end

    local candidates = {}
    for _, entry in pairs(majors) do
        if entry.i18n and entry.uc then
            table.insert(candidates, entry)
        end
    end
    table.sort(candidates, function(a, b)
        return a.major > b.major
    end)
    return candidates
end

local function findIcuCandidates()
    local names = {}
    local ok = pcall(function()
        for name in lfs.dir("/usr/lib") do
            table.insert(names, name)
        end
    end)
    if not ok then
        return {}
    end
    return buildIcuCandidates(names)
end

local REQUIRED_I18N_SYMBOLS = {
    "ucol_open",
    "ucol_openFromShortString",
    "ucol_openRules",
    "ucol_close",
    "ucol_strcollUTF8",
    "ucol_setAttribute",
    "ucol_getAttribute",
    "ucol_getStrength",
    "ucol_getRules",
    "ucol_getReorderCodes",
    "ucol_setReorderCodes",
}

local REQUIRED_UC_SYMBOLS = {
    "uloc_getLanguage",
    "uscript_getCode",
    "u_strlen",
    "u_strcpy",
    "u_strcat",
    "u_strstr",
    "u_strFromUTF8",
}

local function hasVersionedSymbols(handle, names, major)
    for _, name in ipairs(names) do
        local ok = pcall(function()
            return handle[name .. "_" .. major]
        end)
        if not ok then
            return false, name .. "_" .. major
        end
    end
    return true
end

local function loadSystemIcu()
    if loaded_icu_runtime then
        return loaded_icu_runtime.sqlite, loaded_icu_runtime.i18n, loaded_icu_runtime.uc, loaded_icu_runtime.major
    end

    local base_ok, base_error = declareBaseFfi()
    if not base_ok then
        return nil, nil, nil, "cannot declare Kindle catalog ABI: " .. tostring(base_error)
    end

    if type(ffi.loadlib) ~= "function" then
        pcall(require, "ffi/loadlib")
    end
    if type(ffi.loadlib) ~= "function" then
        return nil, nil, nil, "KOReader SQLite loader is unavailable"
    end

    local sqlite_ok, sqlite = pcall(ffi.loadlib, "sqlite3", "0")
    if not sqlite_ok then
        return nil, nil, nil, "cannot load KOReader SQLite: " .. tostring(sqlite)
    end

    local failures = {}
    for _, candidate in ipairs(findIcuCandidates()) do
        local major = candidate.major
        local declared, declaration_error = declareIcu(major)
        if declared then
            local i18n_ok, icui18n = pcall(ffi.load, candidate.i18n)
            local uc_ok, icuuc = pcall(ffi.load, candidate.uc)
            if i18n_ok and uc_ok then
                local i18n_symbols_ok, missing_i18n = hasVersionedSymbols(icui18n, REQUIRED_I18N_SYMBOLS, major)
                local uc_symbols_ok, missing_uc = hasVersionedSymbols(icuuc, REQUIRED_UC_SYMBOLS, major)
                if i18n_symbols_ok and uc_symbols_ok then
                    loaded_icu_runtime = {
                        sqlite = sqlite,
                        i18n = icui18n,
                        uc = icuuc,
                        major = major,
                    }
                    return sqlite, icui18n, icuuc, major
                end
                table.insert(failures, "ICU " .. major .. " missing " .. tostring(missing_i18n or missing_uc))
            else
                table.insert(failures, "ICU " .. major .. " load failed")
            end
        else
            table.insert(failures, "ICU " .. major .. " ABI declaration failed: " .. tostring(declaration_error))
        end
    end

    if #failures == 0 then
        return nil, nil, nil, "cannot find a matched Kindle ICU i18n/core library pair"
    end
    return nil, nil, nil, table.concat(failures, "; ")
end

local function matchingLocaleMapping(locale)
    for _, mapping in ipairs(KINDLE_LOCALE_MAPPINGS) do
        if locale:sub(1, #mapping.prefix) == mapping.prefix then
            return mapping.short
        end
    end
end

local function applyShortStringReorder(collator, suffix, icui18n, icuuc, major)
    if not suffix or suffix == "" then
        return true
    end

    local mode, scripts = suffix:match("^(%a)%.(.+)$")
    local others_first = mode == "y"
    if not scripts then
        scripts = suffix
    end

    local codes = ffi.new("int32_t[11]")
    local count = 0
    if others_first then
        codes[count] = UCOL_REORDER_CODE_OTHERS
        count = count + 1
    end

    for script in scripts:gmatch("[^,]+") do
        local status = ffi.new("int32_t[1]", U_ZERO_ERROR)
        local added = icuuc["uscript_getCode_" .. major](script, codes + count, 10 - count, status)
        if tonumber(status[0]) > U_ZERO_ERROR or added < 1 then
            return false, "cannot resolve Kindle ICU reorder script " .. script
        end
        count = count + tonumber(added)
        if count >= 10 then
            break
        end
    end

    if not others_first then
        codes[count] = UCOL_REORDER_CODE_OTHERS
        count = count + 1
    end

    local status = ffi.new("int32_t[1]", U_ZERO_ERROR)
    icui18n["ucol_setReorderCodes_" .. major](collator, codes, count, status)
    if tonumber(status[0]) > U_ZERO_ERROR then
        -- Kindle 5.19.6 treats a reorder failure as non-fatal and keeps the
        -- collator produced by ucol_openFromShortString(). This is observable
        -- for Lja_S4_HO:Hira,Kana,Hani: ICU rejects the explicit vector, while
        -- the short-string collator already contains the intended Japanese
        -- ordering. Match that behavior instead of disabling p_lastAccess.
        return true
    end
    return true
end

local function openBaseCollator(locale, icui18n, icuuc, major)
    local mapped = matchingLocaleMapping(locale)
    local status = ffi.new("int32_t[1]", U_ZERO_ERROR)
    local collator
    if mapped then
        local definition, suffix = mapped:match("^([^:]+):?(.*)$")
        collator = icui18n["ucol_openFromShortString_" .. major](definition, 0, nil, status)
        if collator ~= nil and tonumber(status[0]) <= U_ZERO_ERROR then
            local ok, err = applyShortStringReorder(collator, suffix, icui18n, icuuc, major)
            if not ok then
                icui18n["ucol_close_" .. major](collator)
                return nil, err
            end
        end
    else
        collator = icui18n["ucol_open_" .. major](locale, status)
    end
    if collator == nil or tonumber(status[0]) > U_ZERO_ERROR then
        if collator ~= nil then
            icui18n["ucol_close_" .. major](collator)
        end
        return nil, "cannot open Kindle ICU collator for " .. locale
    end

    status[0] = U_ZERO_ERROR
    icui18n["ucol_setAttribute_" .. major](collator, UCOL_NUMERIC_COLLATION, UCOL_ON, status)
    if tonumber(status[0]) > U_ZERO_ERROR then
        icui18n["ucol_close_" .. major](collator)
        return nil, "cannot enable Kindle ICU numeric collation"
    end
    return collator
end

-- Kindle's append_collation_preference() appends the selected preference
-- locale's ICU rules to the base collator while preserving base strength,
-- reorder codes and numeric mode.  cc.db's Collation table records the value
-- used when the instantiated index was last rebuilt, so it is the authoritative
-- input for reproducing that index without querying Amazon's preference stack.
local function appendStoredPreference(collator, preference, icui18n, icuuc, major)
    if type(preference) ~= "string" or preference == "" then
        return collator
    end

    local status = ffi.new("int32_t[1]", U_ZERO_ERROR)
    local reorder_codes = ffi.new("int32_t[10]")
    local reorder_count = icui18n["ucol_getReorderCodes_" .. major](collator, reorder_codes, 10, status)
    if tonumber(status[0]) > U_ZERO_ERROR then
        return nil, "cannot read Kindle ICU reorder codes"
    end
    if reorder_count > 10 then
        return nil, "Kindle ICU reorder list exceeds supported firmware limit"
    end

    local strength = icui18n["ucol_getStrength_" .. major](collator)
    status[0] = U_ZERO_ERROR
    local numeric = icui18n["ucol_getAttribute_" .. major](collator, UCOL_NUMERIC_COLLATION, status)
    if tonumber(status[0]) > U_ZERO_ERROR then
        return nil, "cannot read Kindle ICU numeric-collation state"
    end

    status[0] = U_ZERO_ERROR
    local preference_collator = icui18n["ucol_open_" .. major](preference, status)
    if preference_collator == nil or tonumber(status[0]) > U_ZERO_ERROR then
        if preference_collator ~= nil then
            icui18n["ucol_close_" .. major](preference_collator)
        end
        return nil, "cannot open Kindle preference collation " .. preference
    end

    local preference_length = ffi.new("int32_t[1]")
    local preference_rules = icui18n["ucol_getRules_" .. major](preference_collator, preference_length)

    -- libccat contains one Chinese compatibility adjustment before appending
    -- preference rules.  The marker is firmware policy, not an ICU primitive;
    -- preserve it here verbatim from the 5.19.6 implementation.
    local language = ffi.new("char[12]")
    status[0] = U_ZERO_ERROR
    icuuc["uloc_getLanguage_" .. major](preference, language, 12, status)
    if tonumber(status[0]) <= U_ZERO_ERROR and ffi.string(language) == "zh" then
        local marker_utf8 = "[import zh-u-co-private-pinyin]"
        local marker = ffi.new("uint16_t[64]")
        local marker_length = ffi.new("int32_t[1]")
        status[0] = U_ZERO_ERROR
        icuuc["u_strFromUTF8_" .. major](marker, 64, marker_length, marker_utf8, #marker_utf8, status)
        if tonumber(status[0]) <= U_ZERO_ERROR then
            local found = icuuc["u_strstr_" .. major](preference_rules, marker)
            if found ~= nil then
                preference_rules = found + marker_length[0]
                preference_length[0] = icuuc["u_strlen_" .. major](preference_rules)
            end
        end
    end

    local base_length = ffi.new("int32_t[1]")
    local base_rules = icui18n["ucol_getRules_" .. major](collator, base_length)
    local total_length = tonumber(base_length[0] + preference_length[0])
    local combined = ffi.new("uint16_t[?]", total_length + 1)
    icuuc["u_strcpy_" .. major](combined, base_rules)
    icuuc["u_strcat_" .. major](combined, preference_rules)

    status[0] = U_ZERO_ERROR
    local replacement = icui18n["ucol_openRules_" .. major](combined, -1, UCOL_DEFAULT, strength, nil, status)
    if replacement == nil or tonumber(status[0]) > U_ZERO_ERROR then
        if replacement ~= nil then
            icui18n["ucol_close_" .. major](replacement)
        end
        icui18n["ucol_close_" .. major](preference_collator)
        return nil, "cannot append Kindle preference collation rules"
    end

    status[0] = U_ZERO_ERROR
    icui18n["ucol_setReorderCodes_" .. major](replacement, reorder_codes, reorder_count, status)
    if tonumber(status[0]) <= U_ZERO_ERROR then
        icui18n["ucol_setAttribute_" .. major](replacement, UCOL_NUMERIC_COLLATION, numeric, status)
    end
    icui18n["ucol_close_" .. major](preference_collator)
    if tonumber(status[0]) > U_ZERO_ERROR then
        icui18n["ucol_close_" .. major](replacement)
        return nil, "cannot restore Kindle ICU collation attributes"
    end

    icui18n["ucol_close_" .. major](collator)
    return replacement
end

local function createKindleCollator(conn, icui18n, icuuc, major)
    local state_ok, locale, stored_collation = pcall(conn.rowexec, conn, COLLATION_STATE_SQL)
    if not state_ok or type(locale) ~= "string" or locale == "" then
        return nil, "Kindle catalog locale/collation state is unavailable"
    end

    -- Every constructor input is explicit: the catalog's stored locale, the
    -- firmware-derived locale mapping, and its stored preference collation.
    -- Do not mutate libc's process locale or ICU's process-global default;
    -- doing so is unnecessary and could race unrelated KOReader/native work.
    local collator, open_error = openBaseCollator(locale, icui18n, icuuc, major)
    if collator then
        local base_collator = collator
        collator, open_error = appendStoredPreference(base_collator, stored_collation, icui18n, icuuc, major)
        if not collator then
            icui18n["ucol_close_" .. major](base_collator)
        end
    end
    return collator, open_error
end

--- Install a self-contained collation with the firmware's "icu" semantics on
--- KOReader's sqlite3 connection.  Kindle's SQLite registers this as UTF-16,
--- but KOReader's bundled SQLite uses SQLITE_OMIT_UTF16, so the same UCollator
--- is exposed through ICU's UTF-8 comparison entry point instead.
function KindleCatalogDb.installIcuCollation(conn)
    if KindleCatalogDb._test_icu_installer then
        return KindleCatalogDb._test_icu_installer(conn)
    end
    if conn._ptr == nil then
        return false, "SQLite connection does not expose sqlite3*"
    end

    local sqlite, icui18n, icuuc, major_or_error = loadSystemIcu()
    if not sqlite then
        return false, major_or_error
    end
    local major = major_or_error

    local collator, collator_error = createKindleCollator(conn, icui18n, icuuc, major)
    if not collator then
        return false, collator_error
    end

    local callback
    callback = ffi.cast("kindle_sqlite_compare_cb", function(_, left_length, left, right_length, right)
        -- Match Kindle's icuCompare() empty-string ordering exactly.
        if left_length == 0 or right_length == 0 then
            if left_length == 0 and right_length == 0 then
                return 0
            elseif left_length == 0 then
                return 1
            end
            return -1
        end

        local status = ffi.new("int32_t[1]", U_ZERO_ERROR)
        local result = icui18n["ucol_strcollUTF8_" .. major](
            collator,
            ffi.cast("const char *", left),
            left_length,
            ffi.cast("const char *", right),
            right_length,
            status
        )
        if tonumber(status[0]) > U_ZERO_ERROR then
            return 0
        end
        return result
    end)

    local rc = sqlite.sqlite3_create_collation(conn._ptr, "icu", SQLITE_UTF8, nil, callback)
    if tonumber(rc) ~= 0 then
        callback:free()
        icui18n["ucol_close_" .. major](collator)
        return false, "KOReader ICU collation registration failed with SQLite code " .. tostring(tonumber(rc))
    end

    return true, function()
        callback:free()
        icui18n["ucol_close_" .. major](collator)
    end
end

KindleCatalogDb._locale_mappings = KINDLE_LOCALE_MAPPINGS
KindleCatalogDb._matching_locale_mapping = matchingLocaleMapping
KindleCatalogDb._build_icu_candidates = buildIcuCandidates
KindleCatalogDb._apply_short_string_reorder = applyShortStringReorder

--- Prepare an already write-locked cc.db connection for an Entries update.
--- The caller must BEGIN IMMEDIATE first so Locale/Collation cannot change
--- between comparator construction and the update.
---
--- Returns a context with write_last_access and close(), or nil + error when
--- trigger semantics cannot be preserved.
function KindleCatalogDb.prepareWriteConnection(conn)
    -- Register directly; unused functions are inert and this avoids a schema scan.
    -- On schemas without these triggers the functions are simply unused; on a
    -- schema that needs them, SQLite has the exact firmware callbacks available.
    registerEntryTriggerFunctions(conn)

    local cleanup
    local write_last_access = false
    local call_ok, installed, result = pcall(KindleCatalogDb.installIcuCollation, conn)
    if call_ok and installed then
        cleanup = result
        write_last_access = true
    else
        local reason = call_ok and result or installed
        logger.warn("KindlePlugin: Kindle ICU collation unavailable; leaving p_lastAccess unchanged:", reason)
    end

    return {
        write_last_access = write_last_access,
        close = function()
            if cleanup then
                cleanup()
                cleanup = nil
            end
        end,
    }
end

KindleCatalogDb._firmware_sql_functions = firmwareSqlFunctions

return KindleCatalogDb
