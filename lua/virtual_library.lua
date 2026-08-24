local BD = require("ui/bidi")
local Device = require("device")
local logger = require("logger")
local util = require("util")
local _ = require("gettext")

-- Kindle library model.
--
-- Historical releases exposed KINDLE_VIRTUAL:// paths to KOReader and then
-- patched filesystem/document APIs to make those paths look real.  Keep the
-- legacy path helpers only for migration/compatibility; KOReader-facing code
-- now uses real source paths or prepared cached EPUB paths exclusively.
local VirtualLibrary = {}
VirtualLibrary.__index = VirtualLibrary

VirtualLibrary.LEGACY_VIRTUAL_PATH_PREFIX = "KINDLE_VIRTUAL://"
VirtualLibrary.VIRTUAL_PATH_PREFIX = VirtualLibrary.LEGACY_VIRTUAL_PATH_PREFIX
VirtualLibrary.VIRTUAL_LIBRARY_NAME = "Kindle Library"

function VirtualLibrary:new(library_index)
    local instance = {
        library_index = library_index,
        settings = {},
        cache_manager = nil,
        books_by_id = {},
        books_by_real_path = {},
        mappings_built = false,
        mapping_attempted = false,
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

local function isPathWithin(path, root)
    if type(path) ~= "string" or type(root) ~= "string" or root == "" then
        return false
    end
    return path == root or path:sub(1, #root + 1) == root .. "/"
end

local function sanitizeDisplayName(name)
    local cleaned = (name or "Untitled"):gsub("[/\\]+", " "):gsub("%s+", " ")
    cleaned = cleaned:gsub("^%s+", ""):gsub("%s+$", "")
    return cleaned ~= "" and cleaned or "Untitled"
end

function VirtualLibrary:generateVirtualPath(book)
    local filename = sanitizeDisplayName(book.display_name or book.title or book.id)
    local logical_ext = book.logical_ext or book.format or "bin"
    return self.LEGACY_VIRTUAL_PATH_PREFIX .. book.id .. "/" .. filename .. "." .. logical_ext
end

function VirtualLibrary:isVirtualPath(path)
    return type(path) == "string"
        and path:sub(1, #self.LEGACY_VIRTUAL_PATH_PREFIX) == self.LEGACY_VIRTUAL_PATH_PREFIX
end

function VirtualLibrary:getBookId(path)
    if not self:isVirtualPath(path) then
        return nil
    end
    return path:match("^KINDLE_VIRTUAL://([^/]+)/")
end

function VirtualLibrary:buildMappings(force)
    self.mapping_attempted = true
    local books, err = self.library_index:getBooks(force)
    if not books then
        return nil, err
    end

    self.books_by_id = {}
    self.books_by_real_path = {}

    for _, book in ipairs(books) do
        self.books_by_id[book.id] = book
        if book.source_path then
            self.books_by_real_path[book.source_path] = book
        end
        -- Cache paths are deterministic even before the EPUB exists. Indexing
        -- them lets History/Collections and cold-start ReaderUI map a persisted
        -- cached EPUB back to its Kindle book after a restart.
        if self.cache_manager then
            local cached_path = self.cache_manager:getCachePaths(book)
            if cached_path then
                self.books_by_real_path[cached_path] = book
            end
        end
    end

    self.mappings_built = true
    logger.info("KindlePlugin: built real-path mappings for", #books, "books")
    return books
end

function VirtualLibrary:buildPathMappings()
    return self:buildMappings(false)
end

function VirtualLibrary:refresh(force)
    return self:buildMappings(force)
end

function VirtualLibrary:isActive()
    return self.settings.enable_virtual_library ~= false
end

function VirtualLibrary:getBook(path_or_id)
    if not path_or_id then
        return nil
    end

    local book = self.books_by_id[path_or_id] or self.books_by_real_path[path_or_id]
    if book then
        return book
    end

    local legacy_id = self:getBookId(path_or_id)
    if legacy_id and self.books_by_id[legacy_id] then
        return self.books_by_id[legacy_id]
    end

    -- Reader startup, History, and Collections can reach us before the Kindle
    -- list has ever been opened in this process. Rebuild lazily only for paths
    -- that can plausibly belong to this plugin; the global open resolver must
    -- not turn an unrelated KOReader document open into a Kindle library scan.
    local cache_dir = self.settings.cache_dir
    if not cache_dir and self.cache_manager and self.cache_manager.getCacheDir then
        cache_dir = self.cache_manager:getCacheDir()
    end
    local documents_root = self.settings.documents_root or "/mnt/us/documents"
    local should_build = legacy_id ~= nil
        or (type(path_or_id) == "string"
            and (path_or_id:match("^cc:")
                or path_or_id:match("^sha1:")
                or isPathWithin(path_or_id, cache_dir)
                or isPathWithin(path_or_id, documents_root)))
    if not self.mapping_attempted and should_build then
        local books = self:buildMappings(false)
        if books then
            return self.books_by_id[path_or_id]
                or self.books_by_real_path[path_or_id]
                or (legacy_id and self.books_by_id[legacy_id])
        end
    end

    return nil
end

function VirtualLibrary:getVirtualPath(real_path)
    local book = self:getBook(real_path)
    return book and self:generateVirtualPath(book) or nil
end

function VirtualLibrary:getCanonicalPath(path)
    -- Compatibility helper for legacy callers/tests. Real paths are canonical.
    local book = self:getBook(path)
    if not book then
        return path
    end
    if book.open_mode == "direct" then
        return book.source_path or path
    end
    if self.cache_manager then
        return self.cache_manager:getCachePaths(book) or path
    end
    return path
end

function VirtualLibrary:getRealPath(path)
    local book = self:getBook(path)
    return book and book.source_path or nil
end

function VirtualLibrary.getBlockedReasonText(_, book)
    local reason = book and book.block_reason or "unsupported_kfx_layout"
    local text = {
        drm = _("This DRM-protected Kindle format is not supported."),
        unsupported_kfx_layout = _("This KFX layout is not supported yet."),
        missing_source = _("The source file is missing."),
        conversion_failed = _("Failed to prepare this book for reading."),
        cannot_read = _("The source file could not be read."),
        unknown_format = _("This Kindle file format is not supported yet."),
    }
    return text[reason] or _("This book cannot be opened yet.")
end

function VirtualLibrary:isBookPrepared(book)
    if not book then
        return false
    end
    if book.open_mode == "direct" then
        return book.source_path ~= nil
    end
    if book.open_mode == "blocked" or not self.cache_manager then
        return false
    end
    local fresh = self.cache_manager:isFresh(book)
    return fresh == true
end

function VirtualLibrary:getPreparedPath(book)
    if not book then
        return nil
    end
    if book.open_mode == "direct" then
        return book.source_path
    end
    if book.open_mode == "blocked" or not self.cache_manager then
        return nil
    end
    local fresh, cached_path = self.cache_manager:isFresh(book)
    return fresh and cached_path or nil
end

function VirtualLibrary:resolveBookPath(book)
    if not book then
        return nil, "missing book"
    end
    if book.open_mode == "blocked" then
        return nil, book.block_reason or "unsupported_kfx_layout"
    end
    if book.open_mode == "direct" then
        return book.source_path
    end
    if not self.cache_manager then
        return nil, "conversion_failed"
    end

    local cached_path, err = self.cache_manager:ensureCachedEpub(book)
    if cached_path then
        self.books_by_real_path[cached_path] = book
        return cached_path
    end
    return nil, err or "conversion_failed"
end

function VirtualLibrary:createVirtualFolderEntry(parent_path)
    local entry = {
        text = self.VIRTUAL_LIBRARY_NAME .. "/",
        -- Keep a real filesystem path in the item. Selection is intercepted by
        -- FileChooserExt before normal directory navigation.
        path = parent_path or Device.home_dir or "/",
        attr = { mode = "directory" },
        is_kindle_library_folder = true,
        is_kindle_virtual_folder = true, -- compatibility with older specs
        bidi_wrap_func = BD.directory,
    }
    if self.settings.virtual_library_cover_path
        and self.settings.virtual_library_cover_path ~= "" then
        entry.pt_cover_path = self.settings.virtual_library_cover_path
    end
    return entry
end

function VirtualLibrary:getBookEntries(force)
    local books, err = self:buildMappings(force)
    if not books then
        return nil, err
    end

    local entries = {}
    for _, book in ipairs(books) do
        local title = sanitizeDisplayName(book.display_name or book.title or book.id)
        local authors = book.authors and table.concat(book.authors, ", ") or ""
        local mandatory = authors ~= "" and authors or util.getFriendlySize(book.source_size or 0)
        table.insert(entries, {
            text = title,
            file = book.source_path or "",
            path = book.source_path or "",
            attr = { mode = "file", size = book.source_size or 0 },
            mandatory = mandatory,
            kindle_book_id = book.id,
            kindle_open_mode = book.open_mode,
            kindle_block_reason = book.block_reason,
        })
    end
    return entries
end

return VirtualLibrary
