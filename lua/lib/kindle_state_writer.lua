-- Kindle cc.db progress writer.
-- Writes reading progress directly to Kindle's content catalog SQLite database
-- through KOReader's bundled lua-ljsqlite3. Direct SQLite is required in No
-- Framework mode, where Amazon's localhost catalog service is stopped.
--
-- DB location: /var/local/cc.db
-- Key table: Entries
-- Key columns: p_percentFinished, p_lastAccess
--
-- p_readState is deliberately not touched here. Kindle models read/unread as a
-- separate state machine with provenance (manual/automatic/backfill/borrowed),
-- not as part of ordinary reading-position persistence.

local ffi = require("ffi")
local KindleCatalogDb = require("lua/lib/kindle_catalog_db")
local logger = require("logger")

local KindleStateWriter = {}

--- Path to the Kindle content catalog database.
local CC_DB_PATH = "/var/local/cc.db"

local function openSqlite()
    local SQ3 = package.loaded["lua-ljsqlite3/init"]
    if SQ3 then
        return SQ3
    end
    local ok
    ok, SQ3 = pcall(require, "lua-ljsqlite3/init")
    if ok then
        return SQ3
    end
    return nil
end

--- Writes progress to Kindle cc.db for a downloaded book identified by file path.
function KindleStateWriter.writeByPath(book_path, percent_read, timestamp)
    if not book_path or book_path == "" then
        return false
    end
    return KindleStateWriter._write("p_location = ? AND COALESCE(p_isArchived, 0) = 0", book_path, percent_read, timestamp)
end

--- Writes progress to Kindle cc.db for a downloaded book identified by
--- ASIN/cdeKey. Hidden cloud/source rows have p_isArchived=1 on current
--- firmware and are not a device-local reading-state authority.
function KindleStateWriter.writeByCdeKey(cde_key, percent_read, timestamp)
    if not cde_key or cde_key == "" then
        return false
    end
    return KindleStateWriter._write(
        "p_cdeKey = ? AND p_isLatestItem = 1 AND COALESCE(p_isArchived, 0) = 0 AND p_location IS NOT NULL AND p_location <> ''",
        cde_key,
        percent_read,
        timestamp
    )
end

--- Writes progress for a downloaded catalog entry identified by p_uuid.
function KindleStateWriter.writeByUuid(uuid, percent_read, timestamp)
    if not uuid or uuid == "" then
        return false
    end
    return KindleStateWriter._write(
        "p_uuid = ? AND COALESCE(p_isArchived, 0) = 0 AND p_location IS NOT NULL AND p_location <> ''",
        uuid,
        percent_read,
        timestamp
    )
end

function KindleStateWriter._write(where_clause, where_value, percent_read, timestamp)
    if not where_value then
        return false
    end
    percent_read = tonumber(percent_read) or 0
    timestamp = tonumber(timestamp) or os.time()

    local SQ3 = openSqlite()
    if not SQ3 then
        logger.warn("KindlePlugin: lua-ljsqlite3 unavailable for cc.db write")
        return false
    end
    local ok, result = KindleStateWriter._writeWithSQ3(SQ3, where_clause, where_value, percent_read, timestamp)
    if not ok then
        return false
    end
    return result
end

--- Write progress using ljsqlite3 while preserving the instantiated firmware
--- schema's collation and trigger requirements. p_lastAccess is included when
--- the firmware-equivalent ICU comparator can be registered; otherwise only
--- p_percentFinished is changed. p_readState is never part of progress sync.
function KindleStateWriter._writeWithSQ3(SQ3, where_clause, where_value, percent_read, timestamp)
    local conn = SQ3.open(CC_DB_PATH)
    if not conn then
        logger.warn("KindlePlugin: Failed to open cc.db for writing")
        return false, false
    end

    local transaction_open = false
    local write_context
    local ok, result = pcall(function()
        if type(conn.set_busy_timeout) == "function" then
            conn:set_busy_timeout(5000)
        end

        -- Hold the writer lock before inspecting Locale/Collation so Amazon
        -- cannot reindex the catalog between comparator setup and UPDATE.
        conn:exec("BEGIN IMMEDIATE")
        transaction_open = true

        local context, context_error = KindleCatalogDb.prepareWriteConnection(conn)
        if not context then
            error(context_error)
        end
        write_context = context

        local sql
        local stmt
        if context.write_last_access then
            sql = string.format("UPDATE Entries SET p_percentFinished = ?, p_lastAccess = ? WHERE %s", where_clause)
            stmt = conn:prepare(sql)
            if not stmt then
                error("failed to prepare Kindle catalog UPDATE")
            end
            stmt:reset():bind(percent_read, ffi.new("int64_t", timestamp), where_value):step()
        else
            sql = string.format("UPDATE Entries SET p_percentFinished = ? WHERE %s", where_clause)
            stmt = conn:prepare(sql)
            if not stmt then
                error("failed to prepare Kindle catalog UPDATE")
            end
            stmt:reset():bind(percent_read, where_value):step()
        end
        stmt:close()

        local changed = tonumber(conn:rowexec("SELECT changes()")) or 0
        if changed < 1 then
            conn:exec("ROLLBACK")
            transaction_open = false
            return false
        end

        conn:exec("COMMIT")
        transaction_open = false
        return true
    end)

    if transaction_open then
        pcall(function()
            conn:exec("ROLLBACK")
        end)
    end

    pcall(function()
        conn:close()
    end)
    if write_context then
        pcall(write_context.close)
    end

    if not ok then
        logger.warn("KindlePlugin: Error writing to cc.db:", result)
        return false, false
    end

    if result then
        logger.info(
            "KindlePlugin: Wrote Kindle reading progress:",
            "percent:",
            percent_read,
            "last_access:",
            write_context and write_context.write_last_access and timestamp or "unchanged"
        )
    else
        logger.warn("KindlePlugin: No matching Kindle catalog entry to update")
    end

    return true, result
end

return KindleStateWriter
