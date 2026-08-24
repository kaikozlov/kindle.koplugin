-- Kindle cc.db state reader.
-- Reads reading progress from Kindle's content catalog SQLite database
-- through KOReader's bundled lua-ljsqlite3.
-- DB location: /var/local/cc.db
-- Key table: Entries
-- Key columns: p_percentFinished, p_lastAccess, p_readState, p_cdeKey, p_location

local StatusConverter = require("lua/lib/status_converter")
local logger = require("logger")

local KindleStateReader = {}

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

---
--- Reads reading state from Kindle cc.db for a book identified by file path.
--- @param book_path string: File path on device (matched against p_location).
--- @return table|nil: State table with percent_read, timestamp, status, kindle_status, title; or nil on error.
function KindleStateReader.readByPath(book_path)
    if not book_path or book_path == "" then
        return nil
    end
    return KindleStateReader._read("p_location = ?", book_path)
end

---
--- Reads reading state from Kindle cc.db for a book identified by ASIN/cdeKey.
--- @param cde_key string: Kindle ASIN (e.g., "B007N6JEII") or PDOC hash.
--- @return table|nil: State table with percent_read, timestamp, status, kindle_status, title; or nil on error.
function KindleStateReader.readByCdeKey(cde_key)
    if not cde_key or cde_key == "" then
        return nil
    end
    return KindleStateReader._read("p_cdeKey = ? AND p_isLatestItem = 1", cde_key)
end

--- Reads reading state for a catalog entry identified by p_uuid.
--- Virtual-library IDs use the form cc:<uuid>; p_cdeKey contains the ASIN on
--- current firmware, so treating that virtual ID as a cdeKey cannot match.
function KindleStateReader.readByUuid(uuid)
    if not uuid or uuid == "" then
        return nil
    end
    return KindleStateReader._read(
        "p_uuid = (SELECT p_sourceUuid FROM Entries WHERE p_uuid = ?)", uuid
    )
end

---
--- Internal: reads reading state from cc.db via lua-ljsqlite3.
--- @param where_clause string: WHERE clause with placeholder.
--- @param where_value string: Value to bind.
--- @return table|nil: State table or nil.
function KindleStateReader._read(where_clause, where_value)
    local SQ3 = openSqlite()
    if not SQ3 then
        logger.warn("KindlePlugin: lua-ljsqlite3 unavailable for cc.db read")
        return nil
    end
    local ok, result = KindleStateReader._readWithSQ3(SQ3, where_clause, where_value)
    if not ok then
        return nil
    end
    return result
end

---
--- Reads state using ljsqlite3.
function KindleStateReader._readWithSQ3(SQ3, where_clause, where_value)
    local conn, err = SQ3.open(CC_DB_PATH)
    if not conn then
        logger.warn("KindlePlugin: Failed to open cc.db:", err)
        return false, nil
    end

    local ok, result = pcall(function()
        local stmt = conn:prepare(
            string.format(
                "SELECT p_percentFinished, p_lastAccess, p_readState, p_titles_0_nominal, p_cdeKey FROM Entries WHERE %s",
                where_clause
            )
        )
        if not stmt then
            return nil
        end

        local res = stmt:reset():bind(where_value):resultset()

        if not res or not res[1] or #res[1] == 0 then
            return nil
        end

        local percent_finished = tonumber(res[1][1])
        local last_access = tonumber(res[2][1]) or 0
        local read_state = tonumber(res[3][1]) or 0
        local title = res[4][1] or ""
        local cde_key = res[5][1] or ""

        -- NULL percent_finished means never opened
        if percent_finished == nil then
            percent_finished = 0
        end

        return {
            percent_read = percent_finished,
            timestamp = last_access,
            status = StatusConverter.kindleToKoreader(read_state),
            kindle_status = read_state,
            title = title,
            cde_key = cde_key,
        }
    end)

    pcall(function() conn:close() end)

    if not ok then
        logger.warn("KindlePlugin: Error reading cc.db:", result)
        return false, nil
    end

    return true, result
end

---
--- Reads all books with reading progress from cc.db.
--- @return table|nil: Array of {cde_key, title, percent_read, last_access, location}, or nil on error.
function KindleStateReader.readAllProgress()
    local SQ3 = openSqlite()
    if not SQ3 then
        logger.warn("KindlePlugin: lua-ljsqlite3 unavailable for cc.db readAll")
        return nil
    end
    local ok, result = KindleStateReader._readAllWithSQ3(SQ3)
    if not ok then
        return nil
    end
    return result
end

function KindleStateReader._readAllWithSQ3(SQ3)
    local conn = SQ3.open(CC_DB_PATH)
    if not conn then
        return false, nil
    end

    local ok, result = pcall(function()
        local stmt = conn:prepare(
            "SELECT p_cdeKey, p_cdeType, p_titles_0_nominal, p_percentFinished, p_lastAccess, p_location "
            .. "FROM Entries WHERE p_cdeType IN ('EBOK','PDOC') AND p_isLatestItem = 1 "
            .. "AND p_location IS NOT NULL AND p_type NOT LIKE '%Dictionary%'"
        )
        if not stmt then
            return nil
        end

        local res = stmt:reset():resultset()
        if not res or not res[1] then
            return {}
        end

        local books = {}
        for i = 1, #res[1] do
            table.insert(books, {
                cde_key = res[1][i] or "",
                cde_type = res[2][i] or "",
                title = res[3][i] or "",
                percent_read = tonumber(res[4][i]) or 0,
                last_access = tonumber(res[5][i]) or 0,
                location = res[6][i] or "",
            })
        end

        return books
    end)

    pcall(function() conn:close() end)

    if not ok then
        logger.warn("KindlePlugin: Error reading all from cc.db:", result)
        return false, nil
    end

    return true, result
end

return KindleStateReader
