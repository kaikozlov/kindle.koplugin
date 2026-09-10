-- Kindle content-catalog SQLite connection support.
--
-- KOReader must access /var/local/cc.db directly because No Framework mode
-- stops Amazon's content-catalog service.  This module keeps that one direct
-- SQLite path aligned with the firmware contract where writes need more than
-- stock SQLite provides:
--   * install Amazon's real "icu" comparator on KOReader's sqlite3* before
--     changing p_lastAccess (the EntriesLastAccessIndex depends on it), and
--   * register the current firmware's audit-trigger scalar functions only when
--     the instantiated cc.db actually has Entries triggers.

local ffi = require("ffi")
local json = require("json")
local lfs = require("libs/libkoreader-lfs")
local logger = require("logger")

local KindleCatalogDb = {}

local LAST_ACCESS_INDEX_SQL = "SELECT sql FROM sqlite_master WHERE type='index' AND name='EntriesLastAccessIndex'"
local ENTRY_TRIGGER_COUNT_SQL = "SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND tbl_name='Entries'"
local ENTRIES_TABLE_SQL = "SELECT sql FROM sqlite_master WHERE type='table' AND name='Entries'"
local COLLATION_SQL = "SELECT collation FROM Collation LIMIT 1"
local LOCALE_SQL = "SELECT locale FROM Locale LIMIT 1"

local function queryScalar(conn, sql)
    local ok, value = pcall(conn.rowexec, conn, sql)
    if not ok then
        return false, value
    end
    return true, value
end

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
-- /usr/lib/ccat/sql_functions.lua.  They are registered only when Entries
-- triggers exist, so the ordinary schema pays no compatibility cost.
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
    local ok, trigger_count = queryScalar(conn, ENTRY_TRIGGER_COUNT_SQL)
    if not ok then
        return false, "cannot inspect Kindle catalog triggers: " .. tostring(trigger_count)
    end
    if (tonumber(trigger_count) or 0) == 0 then
        return true
    end
    if type(conn.setscalar) ~= "function" then
        return false, "Kindle catalog has Entries triggers but SQLite scalar registration is unavailable"
    end

    for name, callback in pairs(firmwareSqlFunctions) do
        conn:setscalar(name, callback)
    end
    return true
end

local icu_cdefs = {}

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
            char *setlocale(int category, const char *locale);
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
            const char *uloc_getDefault_%d(void);
            void uloc_setDefault_%d(const char *locale_id, int32_t *status);
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

local function findIcuMajor()
    local major
    local ok = pcall(function()
        for name in lfs.dir("/usr/lib") do
            local candidate = tonumber(name:match("^libicui18n%.so%.(%d+)$") or name:match("^libicui18n%.so%.(%d+)%..*$"))
            if candidate and (not major or candidate > major) then
                major = candidate
            end
        end
    end)
    return ok and major or nil
end

local function loadSystemIcu()
    local base_ok, base_error = declareBaseFfi()
    if not base_ok then
        return nil, nil, nil, "cannot declare Kindle catalog ABI: " .. tostring(base_error)
    end

    local major = findIcuMajor()
    if not major then
        return nil, nil, nil, "cannot determine Kindle ICU version"
    end
    local declared, declaration_error = declareIcu(major)
    if not declared then
        return nil, nil, nil, "cannot declare Kindle ICU ABI: " .. tostring(declaration_error)
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
    local icui18n_ok, icui18n = pcall(ffi.load, "/usr/lib/libicui18n.so", true)
    if not icui18n_ok then
        return nil, nil, nil, "cannot load Kindle ICU i18n: " .. tostring(icui18n)
    end
    local icuuc_ok, icuuc = pcall(ffi.load, "/usr/lib/libicuuc.so", true)
    if not icuuc_ok then
        return nil, nil, nil, "cannot load Kindle ICU core: " .. tostring(icuuc)
    end
    return sqlite, icui18n, icuuc, major
end

local function restoreLocaleState(old_collate, old_icu, icuuc, major)
    if old_collate then
        ffi.C.setlocale(3, old_collate) -- LC_COLLATE
    end
    if old_icu then
        local status = ffi.new("int32_t[1]", U_ZERO_ERROR)
        icuuc["uloc_setDefault_" .. major](old_icu, status)
    end
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
        return false, "cannot apply Kindle ICU reorder codes"
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
    local locale_ok, locale = queryScalar(conn, LOCALE_SQL)
    if not locale_ok or type(locale) ~= "string" or locale == "" then
        return nil, "Kindle catalog locale is unavailable"
    end
    local collation_ok, stored_collation = queryScalar(conn, COLLATION_SQL)
    if not collation_ok then
        return nil, "Kindle catalog preference collation is unavailable"
    end

    local old_collate_ptr = ffi.C.setlocale(3, nil) -- LC_COLLATE
    local old_collate = old_collate_ptr ~= nil and ffi.string(old_collate_ptr) or nil
    local old_icu_ptr = icuuc["uloc_getDefault_" .. major]()
    local old_icu = old_icu_ptr ~= nil and ffi.string(old_icu_ptr) or nil

    if ffi.C.setlocale(3, locale) == nil then
        return nil, "cannot activate Kindle catalog locale " .. locale
    end
    local status = ffi.new("int32_t[1]", U_ZERO_ERROR)
    icuuc["uloc_setDefault_" .. major](locale, status)
    if tonumber(status[0]) > U_ZERO_ERROR then
        restoreLocaleState(old_collate, old_icu, icuuc, major)
        return nil, "cannot set Kindle ICU default locale " .. locale
    end

    local collator, open_error = openBaseCollator(locale, icui18n, icuuc, major)
    if collator then
        local base_collator = collator
        collator, open_error = appendStoredPreference(base_collator, stored_collation, icui18n, icuuc, major)
        if not collator then
            icui18n["ucol_close_" .. major](base_collator)
        end
    end
    restoreLocaleState(old_collate, old_icu, icuuc, major)
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

--- Prepare an already write-locked cc.db connection for an Entries update.
--- The caller must BEGIN IMMEDIATE first so Locale/Collation cannot be
--- reindexed between inspection and the update.
---
--- Returns a context with write_last_access and close(), or nil + error when
--- trigger semantics cannot be preserved.
function KindleCatalogDb.prepareWriteConnection(conn)
    local triggers_ok, trigger_error = registerEntryTriggerFunctions(conn)
    if not triggers_ok then
        return nil, trigger_error
    end

    local index_ok, index_sql = queryScalar(conn, LAST_ACCESS_INDEX_SQL)
    if not index_ok then
        return nil, "cannot inspect EntriesLastAccessIndex: " .. tostring(index_sql)
    end

    local cleanup
    local write_last_access = true
    if type(index_sql) == "string" and index_sql ~= "" then
        local table_ok, table_sql = queryScalar(conn, ENTRIES_TABLE_SQL)
        if not table_ok then
            return nil, "cannot inspect Entries schema: " .. tostring(table_sql)
        end
        local index_lower = index_sql:lower()
        local table_lower = type(table_sql) == "string" and table_sql:lower() or ""
        local inherited_icu = index_lower:find("p_titles_0_collation", 1, true) and table_lower:match("p_titles_0_collation%s+collate%s+icu")
        local explicit_icu = index_lower:find("collate icu", 1, true)
        if inherited_icu or explicit_icu then
            local call_ok, installed, result = pcall(KindleCatalogDb.installIcuCollation, conn)
            if call_ok and installed then
                cleanup = result
            else
                write_last_access = false
                local reason = call_ok and result or installed
                logger.warn("KindlePlugin: Kindle ICU collation unavailable; leaving p_lastAccess unchanged:", reason)
            end
        end
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
KindleCatalogDb._queries = {
    entry_trigger_count = ENTRY_TRIGGER_COUNT_SQL,
    last_access_index = LAST_ACCESS_INDEX_SQL,
    entries_table = ENTRIES_TABLE_SQL,
    collation = COLLATION_SQL,
    locale = LOCALE_SQL,
}

return KindleCatalogDb
