--- Scanner that reads the Kindle content catalog (cc.db) directly.
--- Uses KOReader's built-in lua-ljsqlite3 to query /var/local/cc.db.
--- Provides proper titles, authors, DRM status, thumbnails, and reading progress
--- that the Kindle's own scanner has already indexed.

local logger = require("logger")

local CcDbScanner = {}
CcDbScanner.__index = CcDbScanner

--- Path to the Kindle content catalog database.
CcDbScanner.CC_DB_PATH = "/var/local/cc.db"

--- MIME types we show in the virtual library, mapped to logical format.
CcDbScanner.BOOK_MIME_TYPES = {
    ["application/x-kfx-ebook"] = "kfx",
    ["application/x-mobipocket-ebook"] = "azw",
}

--- Query to fetch visible, non-archived book entries.
--- Filters out dictionaries and system entries by requiring p_type = 'Entry:Item'.
local QUERY = [[
SELECT
    p_uuid,
    p_location,
    p_titles_0_nominal,
    j_titles,
    j_credits,
    p_mimeType,
    p_cdeKey,
    p_cdeType,
    p_isDRMProtected,
    p_isArchived,
    p_percentFinished,
    p_thumbnail,
    p_diskUsage,
    p_contentSize,
    p_modificationTime
FROM Entries
WHERE p_type = 'Entry:Item'
    AND p_mimeType IN ('application/x-kfx-ebook', 'application/x-mobipocket-ebook')
    AND p_isVisibleInHome = 1
    AND COALESCE(p_isArchived, 0) = 0
ORDER BY p_titles_0_nominal
]]

function CcDbScanner:new()
    local instance = {}
    setmetatable(instance, self)
    return instance
end

--- Check if cc.db exists and is readable.
--- @return boolean
function CcDbScanner:isAvailable()
    local lfs = require("libs/libkoreader-lfs")
    return lfs.attributes(self.CC_DB_PATH, "mode") == "file"
end

--- Parse a JSON credits array to extract author names.
--- Input: [{"name":{"display":"Author Name"},"kind":"Author"}]
--- @param json_str string|nil
--- @return table: List of author display names.
local function parseAuthors(json_str)
    if not json_str or json_str == "" then
        return {}
    end

    local authors = {}
    -- Simple JSON parse: extract "display":"Name" from credit entries
    for display in json_str:gmatch('"display"%s*:%s*"([^"]*)"') do
        table.insert(authors, display)
    end
    return authors
end

--- Determine open_mode for a book based on MIME type and DRM status.
--- @param mime_type string
--- @param is_drm string|nil "1" or nil
--- @param location string|nil File path (empty if archived)
--- @return string: open_mode ("convert", "direct", "blocked")
--- @return string|nil: block_reason (set when open_mode is "blocked")
local function classifyBook(mime_type, is_drm, location)
    -- No local file means it's cloud-only
    if not location or location == "" then
        return "blocked", "missing_source"
    end

    if mime_type == "application/x-kfx-ebook" then
        -- All KFX files need conversion to EPUB, regardless of DRM.
        -- DRMION decryption is handled by the Python helper during conversion.
        return "convert", nil
    end

    if mime_type == "application/x-mobipocket-ebook" then
        -- AZW/MOBI can be opened directly by KOReader's crengine if DRM-free.
        if is_drm == "1" then
            return "blocked", "drm"
        end
        return "direct", nil
    end

    return "blocked", "unsupported_format"
end

--- Get the file extension for a given MIME type.
--- @param mime_type string
--- @return string
local function mimeToExt(mime_type)
    if mime_type == "application/x-kfx-ebook" then
        return "kfx"
    elseif mime_type == "application/x-mobipocket-ebook" then
        return "azw"
    end
    return "bin"
end

--- Query cc.db and return a list of book entries.
--- @return table|nil: List of book tables, or nil on error.
--- @return string|nil: Error message if nil.
function CcDbScanner:scan()
    local SQ3 = require("lua-ljsqlite3/init")
    local conn, err = SQ3.open(self.CC_DB_PATH, "ro")

    if not conn then
        return nil, "failed to open cc.db: " .. tostring(err)
    end

    local results, nrow
    local ok, db_err = pcall(function()
        results, nrow = conn:exec(QUERY)
    end)

    conn:close()

    if not ok then
        return nil, "cc.db query failed: " .. tostring(db_err)
    end

    if not results or not nrow or nrow == 0 then
        return {}
    end

    -- results is columnar: results[column_name][row_index] (1-based)
    local books = {}

    for i = 1, nrow do
        local uuid = results.p_uuid[i]
        local location = results.p_location[i]
        local title = results.p_titles_0_nominal[i] or "Untitled"
        local mime_type = results.p_mimeType[i] or ""
        local cde_key = results.p_cdeKey[i] or ""
        local cde_type = results.p_cdeType[i] or ""
        local is_drm = results.p_isDRMProtected[i]
        local is_archived = results.p_isArchived[i]
        local percent_finished = results.p_percentFinished[i]
        local thumbnail = results.p_thumbnail[i]
        local disk_usage = results.p_diskUsage[i]
        local content_size = results.p_contentSize[i]
        local modification_time = results.p_modificationTime[i]

        local authors = parseAuthors(results.j_credits[i])

        local open_mode, block_reason = classifyBook(mime_type, is_drm, location)

        local ext = mimeToExt(mime_type)
        local logical_ext = ext == "kfx" and "epub" or ext

        -- For scripts, use the script path directly. For books, use the file path.
        local source_path = (location and location ~= "") and location or nil

        -- Build a stable ID from the cc.db UUID
        local book_id = "cc:" .. uuid

        local book = {
            id = book_id,
            source_path = source_path,
            uuid = uuid,
            format = ext,
            logical_ext = logical_ext,
            title = title,
            authors = authors,
            display_name = title,
            cde_key = cde_key,
            cde_type = cde_type,
            open_mode = open_mode,
            source_mtime = tonumber(modification_time) or 0,
            source_size = tonumber(disk_usage) or tonumber(content_size) or 0,
            thumbnail_path = (thumbnail and thumbnail ~= "") and thumbnail or nil,
            percent_finished = tonumber(percent_finished) or 0,
        }

        if block_reason then
            book.block_reason = block_reason
        end

        table.insert(books, book)
    end

    logger.info("KindlePlugin: cc.db scan found", #books, "entries")
    return books
end

return CcDbScanner
