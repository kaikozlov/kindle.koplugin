-- Kindle cc.db state reader.
-- Reads reading progress from Kindle's content catalog SQLite database.
-- DB location: /var/local/cc.db
-- Key table: Entries
-- Key columns: p_percentFinished, p_lastAccess, p_readState, p_cdeKey, p_location

local StatusConverter = require("lua/lib/status_converter")
local logger = require("logger")

local KindleStateReader = {}

--- Path to the Kindle content catalog database.
local CC_DB_PATH = "/var/local/cc.db"

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

---
--- Internal: reads reading state from cc.db.
--- @param where_clause string: WHERE clause with placeholder.
--- @param where_value string: Value to bind.
--- @return table|nil: State table or nil.
function KindleStateReader._read(where_clause, where_value)
    -- Use ljsqlite3 if available (on-device), otherwise fall back to os.execute
    local SQ3 = package.loaded["lua-ljsqlite3/init"]
    if not SQ3 then
        -- Try alternate require path
        local ok
        ok, SQ3 = pcall(require, "lua-ljsqlite3/init")
        if not ok then
            SQ3 = nil
        end
    end

    if SQ3 then
        local ok, result = KindleStateReader._readWithSQ3(SQ3, where_clause, where_value)
        if ok then
            return result
        end
        logger.info("KindlePlugin: ljsqlite3 read failed, falling back to CLI")
    end

    -- Fallback: use sqlite3 CLI via os.execute
    return KindleStateReader._readWithCLI(where_clause, where_value)
end

---
--- Reads state using ljsqlite3 (on-device, efficient).
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
--- Reads state using sqlite3 CLI (fallback, works in dev/test).
function KindleStateReader._readWithCLI(where_clause, where_value)
    if not where_value then
        return nil
    end
    -- Escape single quotes for SQL
    local escaped = where_value:gsub("'", "''")

    local query = string.format(
        "SELECT p_percentFinished, p_lastAccess, p_readState, p_titles_0_nominal, p_cdeKey FROM Entries WHERE %s LIMIT 1;",
        where_clause:gsub("?", "'" .. escaped .. "'")
    )

    local cmd = string.format("sqlite3 -separator '|' '%s' \"%s\" 2>/dev/null", CC_DB_PATH, query)
    local handle = io.popen(cmd, "r")
    if not handle then
        logger.warn("KindlePlugin: Failed to execute sqlite3 CLI")
        return nil
    end

    local line = handle:read("*l")
    handle:close()

    if not line or line == "" then
        return nil
    end

    -- Parse: percent_finished|last_access|read_state|title|cde_key
    local percent_str, access_str, state_str, title, cde_key = line:match("^(.-)|(.-)|(.-)|(.-)|(.+)$")
    if not percent_str then
        return nil
    end

    local percent_finished = tonumber(percent_str)
    local last_access = tonumber(access_str) or 0
    local read_state = tonumber(state_str) or 0

    if percent_finished == nil then
        percent_finished = 0
    end

    return {
        percent_read = percent_finished,
        timestamp = last_access,
        status = StatusConverter.kindleToKoreader(read_state),
        kindle_status = read_state,
        title = title or "",
        cde_key = cde_key or "",
    }
end

---
--- Reads all books with reading progress from cc.db.
--- @return table|nil: Array of {cde_key, title, percent_read, last_access, location}, or nil on error.
function KindleStateReader.readAllProgress()
    local SQ3 = package.loaded["lua-ljsqlite3/init"]
    if not SQ3 then
        local ok
        ok, SQ3 = pcall(require, "lua-ljsqlite3/init")
        if not ok then
            SQ3 = nil
        end
    end

    if SQ3 then
        local ok, result = KindleStateReader._readAllWithSQ3(SQ3)
        if ok then
            return result
        end
        logger.info("KindlePlugin: ljsqlite3 readAll failed, falling back to CLI")
    end

    return KindleStateReader._readAllWithCLI()
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

function KindleStateReader._readAllWithCLI()
    local query = "SELECT p_cdeKey, p_cdeType, p_titles_0_nominal, p_percentFinished, p_lastAccess, p_location "
        .. "FROM Entries WHERE p_cdeType IN ('EBOK','PDOC') AND p_isLatestItem = 1 "
        .. "AND p_location IS NOT NULL AND p_type NOT LIKE '%Dictionary%';"

    local cmd = string.format("sqlite3 -separator '|' '%s' \"%s\" 2>/dev/null", CC_DB_PATH, query)
    local handle = io.popen(cmd, "r")
    if not handle then
        return nil
    end

    local books = {}
    for line in handle:lines() do
        local cde_key, cde_type, title, percent_str, access_str, location = line:match(
            "^(.-)|(.-)|(.-)|(.-)|(.-)|(.+)$"
        )
        if cde_key then
            table.insert(books, {
                cde_key = cde_key,
                cde_type = cde_type,
                title = title,
                percent_read = tonumber(percent_str) or 0,
                last_access = tonumber(access_str) or 0,
                location = location,
            })
        end
    end
    handle:close()

    return books
end

return KindleStateReader
