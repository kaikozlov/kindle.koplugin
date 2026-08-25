local json = require("json")
local lfs = require("libs/libkoreader-lfs")
local logger = require("logger")
local util = require("util")

local CacheManager = {}
CacheManager.__index = CacheManager

CacheManager.CONVERTER_VERSION = "5"

function CacheManager:new(helper_client)
    local instance = {
        helper_client = helper_client,
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

local function getSourceSignature(book)
    if book.source_path then
        local attr = lfs.attributes(book.source_path)
        if attr and attr.mode == "file" then
            return attr.modification, attr.size
        end
        -- A catalog entry pointing at a missing source must never bless an old
        -- derived EPUB as fresh.
        return nil, nil
    end
    return book.source_mtime, book.source_size
end

function CacheManager:writeMetadata(meta_path, book)
    local handle = io.open(meta_path, "wb")
    if not handle then
        return false, "failed to create cache metadata"
    end

    local source_mtime, source_size = getSourceSignature(book)
    handle:write(json.encode({
        converter_version = self.CONVERTER_VERSION,
        source_mtime = source_mtime,
        source_size = source_size,
    }))
    handle:close()

    return true
end

function CacheManager:isFresh(book)
    local epub_path, meta_path = self:getCachePaths(book)
    if not fileExists(epub_path) or not fileExists(meta_path) then
        logger.dbg("KindlePlugin: cache miss for", book.id, "(epub or meta missing)")
        return false, epub_path, meta_path
    end
    -- Sync translates positions in-process from the conversion-time map; a
    -- cached EPUB without one cannot support exact sync.
    if not fileExists(epub_path:gsub("%.epub$", ".positions.json")) then
        logger.dbg("KindlePlugin: cache miss for", book.id, "(position map missing)")
        return false, epub_path, meta_path
    end

    local metadata = self:readMetadata(meta_path)
    if not metadata then
        logger.dbg("KindlePlugin: cache miss for", book.id, "(metadata unreadable)")
        return false, epub_path, meta_path
    end

    if metadata.converter_version ~= self.CONVERTER_VERSION then
        logger.dbg("KindlePlugin: cache stale for", book.id, "(converter version changed)")
        return false, epub_path, meta_path
    end

    local source_mtime, source_size = getSourceSignature(book)
    if source_mtime == nil or source_size == nil or metadata.source_mtime ~= source_mtime or metadata.source_size ~= source_size then
        logger.dbg("KindlePlugin: cache stale for", book.id, "(source file changed or missing)")
        return false, epub_path, meta_path
    end

    logger.dbg("KindlePlugin: cache hit for", book.id)
    return true, epub_path, meta_path
end

function CacheManager:ensureCachedEpub(book)
    logger.info("KindlePlugin: ensuring cached EPUB for", book.id, "mode:", book.open_mode, "source:", book.source_path)

    local fresh, epub_path, meta_path = self:isFresh(book)
    if fresh then
        logger.info("KindlePlugin: using cached EPUB:", epub_path)
        return epub_path
    end

    if not self:ensureCacheDir() then
        return nil, "failed to create cache directory"
    end

    logger.info("KindlePlugin: preparing", book.source_path, "->", epub_path)
    local result, err = self.helper_client:convert(book.source_path, epub_path)
    if not result then
        logger.warn("KindlePlugin: preparation failed:", err)
        return nil, err
    end

    if result.ok ~= true then
        -- JIT key extraction: if DRM-protected and no key, try extracting it
        if result.code == "drm" and book.source_path then
            logger.info("KindlePlugin: DRM key missing, attempting JIT extraction for", book.id)
            local key_result, key_err = self.helper_client:extractBookKey(book.source_path)
            if key_result and key_result.ok then
                logger.info("KindlePlugin: JIT key extracted, retrying preparation")
                -- Invalidate any stale cache
                os.remove(epub_path)
                os.remove(meta_path)
                result, err = self.helper_client:convert(book.source_path, epub_path)
                if result and result.ok then
                    local ok, write_err = self:writeMetadata(meta_path, book)
                    if not ok then
                        return nil, write_err
                    end
                    logger.info("KindlePlugin: preparation succeeded after key extraction:", result.output_path or epub_path)
                    return result.output_path or epub_path
                end
                logger.warn("KindlePlugin: preparation failed after key extraction:", result and result.code, result and result.message or err)
                return nil, "drm_after_key_extraction"
            end

            logger.warn("KindlePlugin: JIT key extraction failed:", key_result and key_result.message or key_err or "unknown")
            return nil, key_result and key_result.code or "drm_key_extraction_failed"
        end

        logger.warn("KindlePlugin: preparation error:", result.code, result.message)
        return nil, result.code or "conversion_failed"
    end

    local ok, write_err = self:writeMetadata(meta_path, book)
    if not ok then
        return nil, write_err
    end

    logger.info("KindlePlugin: preparation succeeded:", result.output_path or epub_path)
    return result.output_path or epub_path
end

function CacheManager:getDrmKeysPath()
    return self:getCacheDir() .. "/drm_keys.json"
end

function CacheManager:clearBookCache(book)
    local epub_path, meta_path = self:getCachePaths(book)
    local position_path = epub_path:gsub("%.epub$", ".positions.json")
    logger.info("KindlePlugin: clearing cache for", book.id)

    for _, path in ipairs({ epub_path, meta_path, position_path }) do
        if fileExists(path) then
            local ok, err = os.remove(path)
            if not ok then
                return false, err or "failed to remove cache file"
            end
        end
    end
    return true
end

function CacheManager:clearAllCache()
    local cache_dir = self:getCacheDir()
    if not self:ensureCacheDir() then
        return false, "failed to create cache directory"
    end

    local handle = io.popen("find " .. util.shell_escape({ cache_dir }) .. " -maxdepth 1 -type f \\( -name '*.epub' -o -name '*.json' \\) -print")
    if not handle then
        return false, "failed to enumerate cache files"
    end

    local output = handle:read("*a") or ""
    handle:close()

    local count = 0
    local drm_keys_path = self:getDrmKeysPath()
    for file_path in output:gmatch("[^\r\n]+") do
        -- Book-access keys have their own explicit clear action and may be
        -- expensive or impossible to re-extract on older firmware.
        if file_path ~= drm_keys_path then
            local ok, err = os.remove(file_path)
            if not ok then
                return false, err or "failed to remove cache file"
            end
            count = count + 1
        end
    end
    logger.info("KindlePlugin: cleared", count, "cache files")
    return true
end

--- Gets cache statistics (number of cached EPUBs and total size).
--- @return table: { count = number, total_size = number (bytes) }
function CacheManager:getCacheStats()
    local cache_dir = self:getCacheDir()
    local stats = { count = 0, total_size = 0 }

    local handle = io.popen("find " .. util.shell_escape({ cache_dir }) .. " -maxdepth 1 -type f -name '*.epub' -exec ls -l {} \\; 2>/dev/null")
    if not handle then
        return stats
    end

    for line in handle:lines() do
        local size = line:match("%s(%d+)%s")
        if size then
            stats.count = stats.count + 1
            stats.total_size = stats.total_size + tonumber(size)
        end
    end
    handle:close()

    return stats
end

return CacheManager
