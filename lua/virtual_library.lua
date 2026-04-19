local BD = require("ui/bidi")
local logger = require("logger")

local VirtualLibrary = {}
VirtualLibrary.__index = VirtualLibrary

VirtualLibrary.VIRTUAL_PATH_PREFIX = "KINDLE_VIRTUAL://"
VirtualLibrary.VIRTUAL_LIBRARY_NAME = "Kindle Library"

function VirtualLibrary:new(library_index)
    local instance = {
        library_index = library_index,
        settings = {},
        cache_manager = nil,
        books_by_id = {},
        books_by_virtual = {},
        real_to_virtual = {},
        open_alias_to_virtual = {},
        virtual_to_open_alias = {},
    }
    setmetatable(instance, self)
    return instance
end

function VirtualLibrary:setSettings(settings)
    self.settings = settings or {}
end

function VirtualLibrary:setCacheManager(cache_manager)
    self.cache_manager = cache_manager
end

local function sanitizeDisplayName(name)
    local cleaned = (name or "Untitled"):gsub("[/\\]+", " "):gsub("%s+", " ")
    cleaned = cleaned:gsub("^%s+", ""):gsub("%s+$", "")
    if cleaned == "" then
        cleaned = "Untitled"
    end
    return cleaned
end

function VirtualLibrary:generateVirtualPath(book)
    local filename = sanitizeDisplayName(book.display_name or book.title or book.id)
    local logical_ext = book.logical_ext or book.format or "bin"
    return self.VIRTUAL_PATH_PREFIX .. book.id .. "/" .. filename .. "." .. logical_ext
end

function VirtualLibrary:buildMappings(force)
    local books, err = self.library_index:getBooks(force)
    if not books then
        return nil, err
    end

    self.books_by_id = {}
    self.books_by_virtual = {}
    self.real_to_virtual = {}

    for _, book in ipairs(books) do
        book.virtual_path = self:generateVirtualPath(book)
        self.books_by_id[book.id] = book
        self.books_by_virtual[book.virtual_path] = book
        self.real_to_virtual[book.source_path] = book.virtual_path
    end

    logger.info("KindlePlugin: built mappings for", #books, "books")
    return books
end

function VirtualLibrary:refresh(force)
    return self:buildMappings(force)
end

function VirtualLibrary:isVirtualPath(path)
    return type(path) == "string" and path:match("^KINDLE_VIRTUAL://") ~= nil
end

function VirtualLibrary:getBookId(virtual_path)
    if not self:isVirtualPath(virtual_path) then
        return nil
    end

    return virtual_path:match("^KINDLE_VIRTUAL://([^/]+)/")
end

function VirtualLibrary:getBook(path)
    if not path then
        return nil
    end

    if self.books_by_virtual[path] then
        return self.books_by_virtual[path]
    end

    if self.books_by_id[path] then
        return self.books_by_id[path]
    end

    local canonical = self:getCanonicalPath(path)
    if self.books_by_virtual[canonical] then
        return self.books_by_virtual[canonical]
    end

    local virtual_path = self.real_to_virtual[path]
    if virtual_path then
        return self.books_by_virtual[virtual_path]
    end

    return nil
end

function VirtualLibrary:getVirtualPath(path)
    if self:isVirtualPath(path) then
        return path
    end

    if self.open_alias_to_virtual[path] then
        return self.open_alias_to_virtual[path]
    end

    return self.real_to_virtual[path]
end

function VirtualLibrary:getRealPath(path)
    local book = self:getBook(path)
    return book and book.source_path or nil
end

function VirtualLibrary:registerOpenAlias(real_path, virtual_path)
    if not real_path or not virtual_path then
        return
    end

    self.open_alias_to_virtual[real_path] = virtual_path
    self.virtual_to_open_alias[virtual_path] = real_path
end

function VirtualLibrary:isOpenAlias(real_path)
    return self.open_alias_to_virtual[real_path] ~= nil
end

function VirtualLibrary:clearOpenAlias(path)
    local virtual_path = self:getVirtualPath(path)
    if not virtual_path then
        return
    end

    local real_path = self.virtual_to_open_alias[virtual_path]
    if real_path then
        self.open_alias_to_virtual[real_path] = nil
    end
    self.virtual_to_open_alias[virtual_path] = nil
end

function VirtualLibrary:getCanonicalPath(path)
    return self:getVirtualPath(path) or path
end

function VirtualLibrary:getBlockedReasonText(book)
    local reason = book and book.block_reason or "unsupported_kfx_layout"
    local text = {
        drm = "This Kindle book is DRM-protected. Run DRM setup to decrypt.",
        unsupported_kfx_layout = "This KFX layout is not supported by the proof-of-concept converter yet.",
        missing_source = "The source file is missing.",
        conversion_failed = "Conversion failed.",
        drm_not_initialized = "DRM decryption has not been set up. Run DRM setup in the Kindle Library menu.",
    }
    return text[reason] or "This book cannot be opened yet."
end

local function createBookEntry(book)
    local suffix = ""
    if book.open_mode == "blocked" then
        suffix = " [blocked]"
    elseif book.open_mode == "drm" then
        suffix = " [drm]"
    elseif book.open_mode == "convert" then
        suffix = " [convert]"
    end

    return {
        text = sanitizeDisplayName(book.display_name or book.title) .. suffix,
        path = book.virtual_path,
        is_file = true,
        attr = {
            mode = "file",
            size = book.source_size or 0,
        },
        bidi_wrap_func = BD.filename,
        kindle_book_id = book.id,
        kindle_open_mode = book.open_mode,
        kindle_block_reason = book.block_reason,
        kindle_source_path = book.source_path,
    }
end

function VirtualLibrary:getBookEntries(force)
    local books, err = self:buildMappings(force)
    if not books then
        return nil, err
    end

    local entries = {}
    for _, book in ipairs(books) do
        table.insert(entries, createBookEntry(book))
    end

    return entries
end

function VirtualLibrary:createVirtualFolderEntry(parent_path)
    return {
        text = self.VIRTUAL_LIBRARY_NAME .. "/",
        path = (parent_path or "") .. "/" .. self.VIRTUAL_LIBRARY_NAME,
        is_kindle_virtual_folder = true,
        bidi_wrap_func = BD.directory,
    }
end

function VirtualLibrary:resolveBookPath(book)
    if not book then
        return nil, "missing book"
    end

    logger.info("KindlePlugin: resolveBookPath for", book.id,
        "mode:", book.open_mode, "source:", book.source_path)

    if book.open_mode == "blocked" then
        logger.warn("KindlePlugin: book is blocked:", book.block_reason)
        return nil, book.block_reason or "unsupported_kfx_layout"
    end

    if book.open_mode == "direct" then
        logger.info("KindlePlugin: direct open:", book.source_path)
        self:registerOpenAlias(book.source_path, book.virtual_path)
        return book.source_path
    end

    -- open_mode "convert" or "drm" — both go through cache/conversion pipeline
    -- For "drm", the Go binary handles DRMION decryption internally
    if not self.cache_manager then
        logger.warn("KindlePlugin: cache manager is not configured")
        return nil, "conversion_failed"
    end

    local cached_path, err = self.cache_manager:ensureCachedEpub(book)
    if cached_path then
        logger.info("KindlePlugin: resolved to cached EPUB:", cached_path)
        self:registerOpenAlias(cached_path, book.virtual_path)
        return cached_path
    end

    logger.warn("KindlePlugin: resolveBookPath failed:", err or "unknown")
    return nil, err or "conversion_failed"
end

return VirtualLibrary
