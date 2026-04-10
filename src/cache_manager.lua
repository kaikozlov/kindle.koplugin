local json = require("json")
local util = require("util")

local CacheManager = {}
CacheManager.__index = CacheManager

CacheManager.CONVERTER_VERSION = "1"

function CacheManager:new(helper_client, virtual_library)
    local instance = {
        helper_client = helper_client,
        virtual_library = virtual_library,
        settings = {},
    }
    setmetatable(instance, self)
    return instance
end

function CacheManager:setSettings(settings)
    self.settings = settings or {}
end

local function fileExists(path)
    local handle = io.open(path, "rb")
    if handle then
        handle:close()
        return true
    end
    return false
end

local function sanitizeId(book_id)
    return (book_id or "unknown"):gsub("[^%w%.%-_]", "_")
end

function CacheManager:getCacheDir()
    return self.settings.cache_dir or "/tmp/kindle.koplugin.cache"
end

function CacheManager:getCachePaths(book)
    local safe_id = sanitizeId(book.id)
    local base = self:getCacheDir() .. "/" .. safe_id
    return base .. ".epub", base .. ".json"
end

function CacheManager:ensureCacheDir()
    local cache_dir = self:getCacheDir()
    local cmd = util.shell_escape({ "mkdir", "-p", cache_dir })
    return os.execute(cmd) == 0
end

function CacheManager:readMetadata(meta_path)
    local handle = io.open(meta_path, "rb")
    if not handle then
        return nil
    end

    local raw = handle:read("*a")
    handle:close()

    local ok, decoded = pcall(json.decode, raw)
    if not ok then
        return nil
    end

    return decoded
end

function CacheManager:writeMetadata(meta_path, book)
    local handle = io.open(meta_path, "wb")
    if not handle then
        return false, "failed to create cache metadata"
    end

    handle:write(json.encode({
        converter_version = self.CONVERTER_VERSION,
        source_mtime = book.source_mtime,
        source_size = book.source_size,
    }))
    handle:close()

    return true
end

function CacheManager:isFresh(book)
    local epub_path, meta_path = self:getCachePaths(book)
    if not fileExists(epub_path) or not fileExists(meta_path) then
        return false, epub_path, meta_path
    end

    local metadata = self:readMetadata(meta_path)
    if not metadata then
        return false, epub_path, meta_path
    end

    if metadata.converter_version ~= self.CONVERTER_VERSION then
        return false, epub_path, meta_path
    end

    if metadata.source_mtime ~= book.source_mtime or metadata.source_size ~= book.source_size then
        return false, epub_path, meta_path
    end

    return true, epub_path, meta_path
end

function CacheManager:ensureCachedEpub(book)
    local fresh, epub_path, meta_path = self:isFresh(book)
    if fresh then
        return epub_path
    end

    if not self:ensureCacheDir() then
        return nil, "failed to create cache directory"
    end

    local result, err = self.helper_client:convert(book.source_path, epub_path)
    if not result then
        return nil, err
    end

    if result.ok ~= true then
        return nil, result.code or result.message or "conversion_failed"
    end

    local ok, write_err = self:writeMetadata(meta_path, book)
    if not ok then
        return nil, write_err
    end

    return result.output_path or epub_path
end

function CacheManager:clearBookCache(book)
    local epub_path, meta_path = self:getCachePaths(book)
    os.remove(epub_path)
    os.remove(meta_path)
    return true
end

function CacheManager:clearAllCache()
    local cache_dir = self:getCacheDir()
    if not self:ensureCacheDir() then
        return false, "failed to create cache directory"
    end

    local handle = io.popen(
        "find "
            .. util.shell_escape({ cache_dir })
            .. " -maxdepth 1 -type f \\( -name '*.epub' -o -name '*.json' \\) -print"
    )
    if not handle then
        return false, "failed to enumerate cache files"
    end

    local output = handle:read("*a") or ""
    handle:close()

    for file_path in output:gmatch("[^\r\n]+") do
        os.remove(file_path)
    end
    return true
end

return CacheManager
