local DataStorage = require("datastorage")

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

function DocSettingsExt:init(virtual_library)
    self.virtual_library = virtual_library
    self.original_methods = {}
end

function DocSettingsExt:apply(DocSettings)
    self.original_methods.getSidecarDir = DocSettings.getSidecarDir
    self.original_methods.getSidecarFilename = DocSettings.getSidecarFilename
    self.original_methods.getHistoryPath = DocSettings.getHistoryPath

    DocSettings.getSidecarDir = function(ds_self, doc_path, force_location)
        local book = resolveBook(self.virtual_library, doc_path)
        if not book then
            return self.original_methods.getSidecarDir(ds_self, doc_path, force_location)
        end

        return DOCSETTINGS_DIR .. "/kindle_virtual/" .. sanitizeId(book.id) .. ".sdr"
    end

    DocSettings.getSidecarFilename = function(doc_path)
        local book, canonical = resolveBook(self.virtual_library, doc_path)
        if not book then
            return self.original_methods.getSidecarFilename(doc_path)
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
