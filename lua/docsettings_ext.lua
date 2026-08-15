local DataStorage = require("datastorage")
local logger = require("logger")
local util = require("util")

local DocSettingsExt = {}

local DOCSETTINGS_DIR = DataStorage:getDocSettingsDir()
local HISTORY_DIR = DataStorage:getHistoryDir()

local function sanitizeId(book_id)
    return (book_id or "unknown"):gsub("[^%w%.%-_]", "_")
end

local function resolveBook(virtual_library, doc_path)
    local canonical = virtual_library:getCanonicalPath(doc_path)
    return virtual_library:getBook(canonical), canonical
end

local function virtualFilename(virtual_path)
    return virtual_path and virtual_path:match("KINDLE_VIRTUAL://[^/]+/(.+)$") or nil
end

local function buildHistoryPath(virtual_path)
    return HISTORY_DIR .. "/[" .. virtual_path:gsub("(.*/)([^/]+)", "%1] %2"):gsub("/", "#") .. ".lua"
end

local function preferredDocumentPath(virtual_library, book)
    if book.open_mode == "direct" and book.source_path then
        return book.source_path
    end
    if virtual_library.cache_manager and virtual_library.cache_manager.getCachePaths then
        return virtual_library.cache_manager:getCachePaths(book)
    end
    return nil
end

-- On Kindle's /mnt/us FUSE mount, KOReader's util.fileExists may return false
-- for an existing file. Opening the file is the authoritative check and also
-- matches the operation migration performs immediately afterwards.
local function readableFileExists(path)
    local handle = io.open(path, "rb")
    if not handle then
        return false
    end
    handle:close()
    return true
end

local function copyFileIfMissing(source_path, destination_path)
    if readableFileExists(destination_path) or not readableFileExists(source_path) then
        return false
    end
    local source = io.open(source_path, "rb")
    if not source then
        return false
    end
    local contents = source:read("*a")
    source:close()
    local destination = io.open(destination_path, "wb")
    if not destination then
        return false
    end
    destination:write(contents)
    destination:close()
    return true
end

function DocSettingsExt:migrateLegacySidecar(book, canonical, preferred_doc_path, preferred_dir)
    local legacy_dir = DOCSETTINGS_DIR .. "/kindle_virtual/" .. sanitizeId(book.id) .. ".sdr"
    if legacy_dir == preferred_dir then
        return
    end

    local legacy_filename = self.original_methods.getSidecarFilename(
        virtualFilename(canonical) or "book.epub"
    )
    local preferred_filename = self.original_methods.getSidecarFilename(preferred_doc_path)
    local legacy_path = legacy_dir .. "/" .. legacy_filename
    local preferred_path = preferred_dir .. "/" .. preferred_filename
    -- KOReader rotates metadata by moving the current file to .old before it
    -- writes the replacement. During that window the preferred path is absent,
    -- but the canonical sidecar already exists and must not be replaced by a
    -- stale legacy copy (which may contain fewer annotations or older progress).
    if readableFileExists(preferred_path)
        or readableFileExists(preferred_path .. ".old")
        or not readableFileExists(legacy_path)
    then
        return
    end

    if not util.makePath(preferred_dir) then
        logger.warn("KindlePlugin: failed to create canonical sidecar directory for", book.id)
        return
    end
    if copyFileIfMissing(legacy_path, preferred_path) then
        copyFileIfMissing(legacy_path .. ".old", preferred_path .. ".old")
        logger.info("KindlePlugin: migrated legacy virtual sidecar for", book.id)
    end
end

function DocSettingsExt:init(virtual_library)
    self.virtual_library = virtual_library
    self.original_methods = {}
end

function DocSettingsExt:apply(DocSettings)
    logger.info("KindlePlugin: applying DocSettings patches")
    self.original_methods.getSidecarDir = DocSettings.getSidecarDir
    self.original_methods.getSidecarFilename = DocSettings.getSidecarFilename
    self.original_methods.getHistoryPath = DocSettings.getHistoryPath

    DocSettings.getSidecarDir = function(ds_self, doc_path, force_location)
        local book, canonical = resolveBook(self.virtual_library, doc_path)
        if not book then
            return self.original_methods.getSidecarDir(ds_self, doc_path, force_location)
        end

        local preferred_doc_path = preferredDocumentPath(self.virtual_library, book)
        if not preferred_doc_path then
            return DOCSETTINGS_DIR .. "/kindle_virtual/" .. sanitizeId(book.id) .. ".sdr"
        end
        -- A virtual book has exactly one canonical sidecar. KOReader probes
        -- alternate locations by passing force_location while searching; if we
        -- honor those probes here, a cached EPUB can acquire duplicate
        -- sidecars under docsettings/ or hashdocsettings/.
        local preferred_dir = self.original_methods.getSidecarDir(ds_self, preferred_doc_path)
        self:migrateLegacySidecar(book, canonical, preferred_doc_path, preferred_dir)
        return preferred_dir
    end

    DocSettings.getSidecarFilename = function(doc_path)
        local book, canonical = resolveBook(self.virtual_library, doc_path)
        if not book then
            return self.original_methods.getSidecarFilename(doc_path)
        end

        local preferred_doc_path = preferredDocumentPath(self.virtual_library, book)
        if preferred_doc_path then
            return self.original_methods.getSidecarFilename(preferred_doc_path)
        end
        local filename = virtualFilename(canonical) or "book.epub"
        return self.original_methods.getSidecarFilename(filename)
    end

    DocSettings.getHistoryPath = function(ds_self, doc_path)
        local book, canonical = resolveBook(self.virtual_library, doc_path)
        if not book then
            return self.original_methods.getHistoryPath(ds_self, doc_path)
        end

        return buildHistoryPath(canonical)
    end
end

function DocSettingsExt:unapply(DocSettings)
    for method_name, original_method in pairs(self.original_methods) do
        DocSettings[method_name] = original_method
    end
    self.original_methods = {}
end

return DocSettingsExt
