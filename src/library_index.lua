local logger = require("logger")

local LibraryIndex = {}
LibraryIndex.__index = LibraryIndex

function LibraryIndex:new(helper_client)
    local instance = {
        helper_client = helper_client,
        books = nil,
        loaded_at = 0,
        settings = {},
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

function LibraryIndex:refresh(force)
    local ttl = tonumber(self.settings.index_ttl_seconds) or 300
    local now = os.time()

    if not force and self.books and (now - self.loaded_at) < ttl then
        return self.books
    end

    local root = self.settings.documents_root or "/mnt/us/documents"
    local result, err = self.helper_client:scan(root)
    if not result then
        return nil, err
    end

    if type(result.books) ~= "table" then
        logger.warn("KindlePlugin: helper scan returned invalid payload")
        return nil, "helper scan payload missing books"
    end

    self.books = result.books
    sortBooks(self.books)
    self.loaded_at = now
    self.settings.last_scan_at = now

    return self.books
end

function LibraryIndex:getBooks(force)
    return self:refresh(force)
end

return LibraryIndex
