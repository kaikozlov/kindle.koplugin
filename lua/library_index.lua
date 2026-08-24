local logger = require("logger")

local LibraryIndex = {}
LibraryIndex.__index = LibraryIndex

function LibraryIndex:new(ccdb_scanner)
    local instance = {
        ccdb_scanner = ccdb_scanner,
        settings = {},
        books = {},
        loaded_at = 0,
    }
    setmetatable(instance, self)
    return instance
end

function LibraryIndex:setSettings(settings)
    self.settings = settings or {}
end

local function sortBooks(books)
    table.sort(books, function(left, right)
        local left_name = (left.display_name or left.title or left.source_path or ""):lower()
        local right_name = (right.display_name or right.title or right.source_path or ""):lower()
        if left_name == right_name then
            return (left.source_path or "") < (right.source_path or "")
        end
        return left_name < right_name
    end)
end

--- Scan the Kindle content catalog.
---
--- cc.db is the single library authority: it owns the stable p_uuid identity
--- that reading-position receipts, cc.* book ids, and catalog progress writes
--- all depend on. A filesystem scan could only produce hash identities, which
--- would silently break that continuity, so catalog unavailability is a hard
--- error rather than a degraded fallback.
--- @return table|nil: List of book entries.
--- @return string|nil: Error message on failure.
function LibraryIndex:scan()
    if not self.ccdb_scanner then
        local ok, CcDbScanner = pcall(require, "lua/ccdb_scanner")
        if ok then
            self.ccdb_scanner = CcDbScanner:new()
        end
    end

    if not self.ccdb_scanner then
        return nil, "cc.db scanner unavailable"
    end
    if not self.ccdb_scanner:isAvailable() then
        return nil, "Kindle content catalog (cc.db) is unavailable"
    end

    logger.info("KindlePlugin: scanning library via cc.db")
    local books, err = self.ccdb_scanner:scan()
    if not books then
        logger.warn("KindlePlugin: cc.db scan failed:", err)
        return nil, err
    end
    sortBooks(books)
    return books
end

function LibraryIndex:refresh(force)
    local ttl = tonumber(self.settings.index_ttl_seconds) or 300
    if not force and (os.time() - self.loaded_at) < ttl and #self.books > 0 then
        return self.books
    end

    local books, err = self:scan()
    if not books then
        return nil, err
    end

    self.books = books
    self.loaded_at = os.time()
    return books
end

function LibraryIndex:getBooks(force)
    return self:refresh(force)
end

return LibraryIndex
