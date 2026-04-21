-- Kindle cc.db state writer.
-- Writes reading progress to Kindle's content catalog SQLite database.
-- DB location: /var/local/cc.db
-- Key table: Entries
-- Key columns: p_percentFinished, p_readState
--
-- NOTE: p_lastAccess CANNOT be updated because its index
-- (EntriesLastAccessIndex) includes p_titles_0_collation which uses
-- ICU collation, and neither ljsqlite3 nor the on-device sqlite3 CLI
-- have ICU support. We update p_percentFinished and p_readState only.

local StatusConverter = require("lua/lib/status_converter")
local logger = require("logger")

local KindleStateWriter = {}

--- Path to the Kindle content catalog database.
local CC_DB_PATH = "/var/local/cc.db"

---
--- Writes reading state to Kindle cc.db for a book identified by file path.
--- @param book_path string: File path on device (matched against p_location).
--- @param percent_read number: Progress percentage (0-100).
--- @param timestamp number: Unix timestamp of last read (unused — ICU index).
--- @param status string: KOReader status string.
--- @return boolean: True if write succeeded.
function KindleStateWriter.writeByPath(book_path, percent_read, timestamp, status)
    if not book_path or book_path == "" then
        return false
    end

    return KindleStateWriter._write("p_location = ?", book_path, percent_read, timestamp, status)
end

---
--- Writes reading state to Kindle cc.db for a book identified by ASIN/cdeKey.
--- @param cde_key string: Kindle ASIN or PDOC hash.
--- @param percent_read number: Progress percentage (0-100).
--- @param timestamp number: Unix timestamp of last read (unused — ICU index).
--- @param status string: KOReader status string.
--- @return boolean: True if write succeeded.
function KindleStateWriter.writeByCdeKey(cde_key, percent_read, timestamp, status)
    if not cde_key or cde_key == "" then
        return false
    end

    return KindleStateWriter._write(
        "p_cdeKey = ? AND p_isLatestItem = 1 AND p_location IS NOT NULL",
        cde_key,
        percent_read,
        timestamp,
        status
    )
end

---
--- Internal: writes reading state to cc.db.
--- Updates p_percentFinished and p_readState only (p_lastAccess skipped due to ICU index).
--- @param where_clause string: WHERE clause with placeholder.
--- @param where_value string|nil: Value to bind.
--- @param percent_read number: Progress percentage (0-100).
--- @param timestamp number: Unix timestamp (unused).
--- @param status string: KOReader status string.
--- @return boolean: True if write succeeded.
function KindleStateWriter._write(where_clause, where_value, percent_read, timestamp, status)
    if not where_value then
        return false
    end
    percent_read = tonumber(percent_read) or 0
    local read_state = status and StatusConverter.koreaderToKindle(status) or 6

    -- Use ljsqlite3 if available (on-device), otherwise fall back to CLI
    local SQ3 = package.loaded["lua-ljsqlite3/init"]
    if not SQ3 then
        local ok
        ok, SQ3 = pcall(require, "lua-ljsqlite3/init")
        if not ok then
            SQ3 = nil
        end
    end

    if SQ3 then
        local ok, result = KindleStateWriter._writeWithSQ3(SQ3, where_clause, where_value, percent_read, read_state)
        if ok then
            return result
        end
        logger.info("KindlePlugin: ljsqlite3 write failed, falling back to CLI")
    end

    return KindleStateWriter._writeWithCLI(where_clause, where_value, percent_read, read_state)
end

---
--- Writes state using ljsqlite3 (on-device, efficient).
--- Only updates p_percentFinished and p_readState (p_lastAccess skipped — ICU index).
function KindleStateWriter._writeWithSQ3(SQ3, where_clause, where_value, percent_read, read_state)
    local conn = SQ3.open(CC_DB_PATH)
    if not conn then
        logger.warn("KindlePlugin: Failed to open cc.db for writing")
        return false, false
    end

    local ok, result = pcall(function()
        local sql = string.format(
            "UPDATE Entries SET p_percentFinished = ?, p_readState = ? WHERE %s",
            where_clause
        )

        local stmt = conn:prepare(sql)
        if not stmt then
            logger.warn("KindlePlugin: Failed to prepare UPDATE statement")
            return false
        end

        stmt:reset():bind(
            percent_read,
            read_state,
            where_value
        ):step()

        return true
    end)

    pcall(function() conn:close() end)

    if not ok then
        logger.warn("KindlePlugin: Error writing to cc.db:", result)
        return false, false
    end

    logger.info(
        "KindlePlugin: Wrote Kindle reading progress:",
        "percent:", percent_read,
        "read_state:", read_state
    )

    return true, result
end

---
--- Writes state using sqlite3 CLI (fallback).
--- Only updates p_percentFinished and p_readState (p_lastAccess skipped — ICU index).
function KindleStateWriter._writeWithCLI(where_clause, where_value, percent_read, read_state)
    local escaped = where_value:gsub("'", "''")

    local sql = string.format(
        "UPDATE Entries SET p_percentFinished = %s, p_readState = %d WHERE %s;",
        tostring(percent_read),
        read_state,
        where_clause:gsub("?", "'" .. escaped .. "'")
    )

    local cmd = string.format("sqlite3 '%s' \"%s\" 2>/dev/null", CC_DB_PATH, sql)
    local result = os.execute(cmd)

    if type(result) == "number" then
        result = result == 0
    elseif type(result) == "boolean" then
        -- already boolean
    else
        result = false
    end

    if result then
        logger.info(
            "KindlePlugin: Wrote Kindle reading progress via CLI:",
            "percent:", percent_read,
            "read_state:", read_state
        )
    else
        logger.warn("KindlePlugin: Failed to write to cc.db via CLI")
    end

    return result
end

return KindleStateWriter
