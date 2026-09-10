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

local base_ffi_declared = false
local function declareBaseFfi()
    if base_ffi_declared then
        return true
    end
    local ok, err = pcall(
        ffi.cdef,
        [[
            typedef int (*kindle_sqlite_compare_cb)(void *, int, const void *, int, const void *);
            int pthread_rwlock_init(void *rwlock, const void *attr);
            int pthread_rwlock_destroy(void *rwlock);
            char *setlocale(int category, const char *locale);
            int sqlite3_create_collation(
                void *db,
                const char *name,
                int text_rep,
                void *context,
                kindle_sqlite_compare_cb compare
            );
            void update_icu_collator(void *state, void *db);
            const char *get_preference_collation(void);
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
            void ucol_close_%d(void *collator);
            int ucol_strcollUTF8_%d(
                const void *collator,
                const char *left,
                int32_t left_length,
                const char *right,
                int32_t right_length,
                int32_t *status
            );
            const char *uloc_getDefault_%d(void);
            void uloc_setDefault_%d(const char *locale_id, int32_t *status);
        ]],
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

local function loadFirmwareIcu()
    local base_ok, base_error = declareBaseFfi()
    if not base_ok then
        return nil, nil, nil, nil, "cannot declare Kindle catalog ABI: " .. tostring(base_error)
    end

    local major = findIcuMajor()
    if not major then
        return nil, nil, nil, nil, "cannot determine Kindle ICU version"
    end
    local declared, declaration_error = declareIcu(major)
    if not declared then
        return nil, nil, nil, nil, "cannot declare Kindle ICU ABI: " .. tostring(declaration_error)
    end

    if type(ffi.loadlib) ~= "function" then
        pcall(require, "ffi/loadlib")
    end
    if type(ffi.loadlib) ~= "function" then
        return nil, nil, nil, nil, "KOReader SQLite loader is unavailable"
    end

    local sqlite_ok, sqlite = pcall(ffi.loadlib, "sqlite3", "0")
    if not sqlite_ok then
        return nil, nil, nil, nil, "cannot load KOReader SQLite: " .. tostring(sqlite)
    end
    local icui18n_ok, icui18n = pcall(ffi.load, "/usr/lib/libicui18n.so", true)
    if not icui18n_ok then
        return nil, nil, nil, nil, "cannot load Kindle ICU i18n: " .. tostring(icui18n)
    end
    local icuuc_ok, icuuc = pcall(ffi.load, "/usr/lib/libicuuc.so", true)
    if not icuuc_ok then
        return nil, nil, nil, nil, "cannot load Kindle ICU core: " .. tostring(icuuc)
    end
    local ccat_ok, ccat = pcall(ffi.load, "/usr/lib/libccat.so.1", true)
    if not ccat_ok then
        ccat_ok, ccat = pcall(ffi.load, "/usr/lib/libccat.so", true)
    end
    if not ccat_ok then
        return nil, nil, nil, nil, "cannot load Kindle libccat: " .. tostring(ccat)
    end
    return sqlite, icui18n, icuuc, ccat, major
end

local function restoreLocaleState(old_collate, old_icu, icuuc, major)
    if old_collate then
        ffi.C.setlocale(3, old_collate) -- LC_COLLATE
    end
    if old_icu then
        local status = ffi.new("int32_t[1]", 0)
        icuuc["uloc_setDefault_" .. major](old_icu, status)
    end
end

local function createAmazonCollatorState(conn, icuuc, ccat, major)
    local locale_ok, locale = queryScalar(conn, LOCALE_SQL)
    if not locale_ok or type(locale) ~= "string" or locale == "" then
        return nil, "Kindle catalog locale is unavailable"
    end

    -- update_icu_collator() may reindex cc.db when its process locale or the
    -- user's preference collation differs from the values stored in cc.db.
    -- We only need it as an exact collator constructor, so refuse that case
    -- rather than allowing a helper call to mutate the catalog behind the
    -- ljsqlite3 transaction that already owns the write lock.
    local collation_ok, stored_collation = queryScalar(conn, COLLATION_SQL)
    if not collation_ok then
        stored_collation = nil
    end
    local preference_ptr = ccat.get_preference_collation()
    local preference = preference_ptr ~= nil and ffi.string(preference_ptr) or nil
    if preference and type(stored_collation) == "string" and preference ~= stored_collation then
        return nil, "Kindle preference collation differs from the instantiated catalog"
    end

    local old_collate_ptr = ffi.C.setlocale(3, nil) -- LC_COLLATE
    local old_collate = old_collate_ptr ~= nil and ffi.string(old_collate_ptr) or nil
    local old_icu_ptr = icuuc["uloc_getDefault_" .. major]()
    local old_icu = old_icu_ptr ~= nil and ffi.string(old_icu_ptr) or nil

    if ffi.C.setlocale(3, locale) == nil then
        return nil, "cannot activate Kindle catalog locale " .. locale
    end

    -- libccat's own run_ccat() allocates 0x10c bytes, zeros the collator slot
    -- at +4, initializes a pthread rwlock at +0xa8, then calls
    -- update_icu_collator(state, NULL). Reproduce that exact constructor path.
    local state = ffi.new("uint8_t[?]", 0x10c)
    local lock = state + 0xa8
    if ffi.C.pthread_rwlock_init(lock, nil) ~= 0 then
        restoreLocaleState(old_collate, old_icu, icuuc, major)
        return nil, "cannot initialize Kindle ICU collation lock"
    end

    local ok, construction_error = pcall(ccat.update_icu_collator, state, nil)
    restoreLocaleState(old_collate, old_icu, icuuc, major)
    if not ok then
        ffi.C.pthread_rwlock_destroy(lock)
        return nil, "Amazon ICU collator construction failed: " .. tostring(construction_error)
    end

    local collator = ffi.cast("void **", state + 4)[0]
    if collator == nil then
        ffi.C.pthread_rwlock_destroy(lock)
        return nil, "Amazon ICU collator construction returned no collator"
    end
    return {
        state = state,
        lock = lock,
        collator = collator,
    }
end

--- Install a collation with the exact semantics of Amazon's "icu" comparator
--- on KOReader's sqlite3 connection.
---
--- Amazon registers its comparator as SQLITE_UTF16 (4), but KOReader's bundled
--- SQLite is intentionally compiled with SQLITE_OMIT_UTF16. Registering the
--- firmware callback directly would therefore feed UTF-8 bytes to a callback
--- that interprets them as UTF-16. We instead keep Amazon's exact UCollator and
--- expose the same ordering through ICU's UTF-8 entry point.
function KindleCatalogDb.installIcuCollation(conn)
    if KindleCatalogDb._test_icu_installer then
        return KindleCatalogDb._test_icu_installer(conn)
    end
    if conn._ptr == nil then
        return false, "SQLite connection does not expose sqlite3*"
    end

    local sqlite, icui18n, icuuc, ccat, major_or_error = loadFirmwareIcu()
    if not sqlite then
        return false, major_or_error
    end
    local major = major_or_error

    local amazon_state, state_error = createAmazonCollatorState(conn, icuuc, ccat, major)
    if not amazon_state then
        return false, state_error
    end

    local callback
    callback = ffi.cast("kindle_sqlite_compare_cb", function(_, left_length, left, right_length, right)
        -- Match libccat icuCompare() exactly for empty strings.
        if left_length == 0 or right_length == 0 then
            if left_length == 0 and right_length == 0 then
                return 0
            elseif left_length == 0 then
                return 1
            end
            return -1
        end

        local status = ffi.new("int32_t[1]", 0)
        local result = icui18n["ucol_strcollUTF8_" .. major](
            amazon_state.collator,
            ffi.cast("const char *", left),
            left_length,
            ffi.cast("const char *", right),
            right_length,
            status
        )
        if tonumber(status[0]) > 0 then
            -- A comparator cannot report an SQLite error. Treat an ICU failure
            -- as equality; this is strictly a defensive path because valid
            -- catalog UTF-8 must not reach it.
            return 0
        end
        return result
    end)

    local rc = sqlite.sqlite3_create_collation(conn._ptr, "icu", 1, amazon_state.state, callback) -- SQLITE_UTF8
    if tonumber(rc) ~= 0 then
        callback:free()
        icui18n["ucol_close_" .. major](amazon_state.collator)
        ffi.C.pthread_rwlock_destroy(amazon_state.lock)
        return false, "KOReader ICU collation registration failed with SQLite code " .. tostring(tonumber(rc))
    end

    return true,
        function()
            callback:free()
            icui18n["ucol_close_" .. major](amazon_state.collator)
            ffi.C.pthread_rwlock_destroy(amazon_state.lock)
        end
end

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
