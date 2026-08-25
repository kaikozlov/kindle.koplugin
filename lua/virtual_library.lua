local BD = require("ui/bidi")
local Device = require("device")
local logger = require("logger")
local util = require("util")
local _ = require("gettext")

-- Kindle library model.
--
-- KOReader only ever sees real document paths: Kindle source files or the
-- plugin's cached converted EPUBs.
local VirtualLibrary = {}
VirtualLibrary.__index = VirtualLibrary

VirtualLibrary.VIRTUAL_LIBRARY_NAME = "Kindle Library"
VirtualLibrary.KINDLE_DOCUMENTS_ROOT = "/mnt/us/documents"

function VirtualLibrary:new(library_index)
    local instance = {
        library_index = library_index,
        settings = {},
        cache_manager = nil,
        books_by_id = {},
        books_by_real_path = {},
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

local function isKindleSourcePath(path)
    if type(path) ~= "string" then
        return false
    end
    local extension = path:lower():match("%.([%w]+)$")
    return extension == "kfx" or extension == "azw" or extension == "azw3" or extension == "mobi"
end

local function sanitizeDisplayName(name)
    local cleaned = (name or "Untitled"):gsub("[/\\]+", " "):gsub("%s+", " ")
    cleaned = cleaned:gsub("^%s+", ""):gsub("%s+$", "")
    return cleaned ~= "" and cleaned or "Untitled"
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

    logger.info("KindlePlugin: built real-path mappings for", #books, "books")
    return books
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

    -- Reader startup, History, and Collections can reach us before the Kindle
    -- list has ever been opened in this process. Rebuild lazily only for paths
    -- that can plausibly belong to this plugin; the global open resolver must
    -- not turn an unrelated KOReader document open into a Kindle library scan.
    local cache_dir = self.settings.cache_dir
    if not cache_dir and self.cache_manager and self.cache_manager.getCacheDir then
        cache_dir = self.cache_manager:getCacheDir()
    end
    local should_build = type(path_or_id) == "string"
        and (
            path_or_id:match("^cc:")
            or path_or_id:match("^sha1:")
            or isPathWithin(path_or_id, cache_dir)
            or isPathWithin(path_or_id, self.KINDLE_DOCUMENTS_ROOT)
            or isKindleSourcePath(path_or_id)
        )
    if not self.mapping_attempted and should_build then
        local books = self:buildMappings(false)
        if books then
            return self.books_by_id[path_or_id] or self.books_by_real_path[path_or_id]
        end
    end

    return nil
end

function VirtualLibrary:getBlockedReasonText(book)
    local reason = book and book.block_reason or "conversion_failed"
    local text = {
        drm = _("This DRM-protected Kindle format is not supported."),
        missing_source = _("The source file is missing."),
        unsupported_format = _("This Kindle file format is not supported."),
        conversion_failed = _("Failed to prepare this book for reading."),
        drm_extractor_unavailable = _(
            "This Kindle firmware cannot extract this book's access key by itself. "
                .. "Install a compatible kfxdedrm native extractor, then reopen the book."
        ),
        drm_key_extraction_failed = _("Could not extract this book's access key. Check the KOReader debug log for details."),
        drm_after_key_extraction = _(
            "A book access key was extracted, but the book still could not be decrypted. "
                .. "Try re-downloading the book in the Kindle reader and opening it again."
        ),
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

function VirtualLibrary:resolveBookPath(book)
    if not book then
        return nil, "missing book"
    end
    if book.open_mode == "blocked" then
        return nil, book.block_reason or "conversion_failed"
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
        bidi_wrap_func = BD.directory,
    }
    if self.settings.virtual_library_cover_path and self.settings.virtual_library_cover_path ~= "" then
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
        -- A fresh cached EPUB gives CoverBrowser a real provider-backed path
        -- for cover/metadata extraction; unprepared books keep the Kindle
        -- source path and render the placeholder cover.
        local entry_file = book.source_path or ""
        if book.open_mode ~= "blocked" and book.open_mode ~= "direct" and self.cache_manager and self:isBookPrepared(book) then
            entry_file = self.cache_manager:getCachePaths(book) or entry_file
        end
        table.insert(entries, {
            text = title,
            file = entry_file,
            path = book.source_path or "",
            attr = { mode = "file", size = book.source_size or 0 },
            mandatory = mandatory,
            kindle_book_id = book.id,
        })
    end
    return entries
end

return VirtualLibrary
