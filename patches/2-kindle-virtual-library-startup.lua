-- User patch to support Kindle virtual library paths at startup
-- Priority: 2
-- Adapted from kobo.koplugin/patches/2-kobo-virtual-library-startup.lua

-- Get the absolute path of this patch file to determine plugin location
local patch_info = debug.getinfo(1, "S")
local patch_path = patch_info.source:match("^@(.+)$")
local koreader_root = patch_path:match("^(.+)/patches/[^/]+$")

local PatternUtils = dofile(koreader_root .. "/plugins/kindle.koplugin/lua/lib/pattern_utils.lua")
local logger = require("logger")

local plugin_settings = G_reader_settings:readSetting("kindle_plugin") or { enable_virtual_library = true }

logger.dbg("KindlePlugin Startup Patch: plugin_settings", plugin_settings)

if not plugin_settings.enable_virtual_library then
    logger.info("KindlePlugin Startup Patch: skipping virtual library patch due to disabled setting")
    return
end

local virtual_prefix = "KINDLE_VIRTUAL://"
local escaped_prefix = PatternUtils.escape(virtual_prefix)

--- Get the cache directory for converted EPUBs.
local function getCacheDir()
    if plugin_settings.cache_dir and plugin_settings.cache_dir ~= "" then
        return plugin_settings.cache_dir
    end
    return "/tmp/kindle.koplugin.cache"
end

-- Patch ffi/util.realpath to handle KINDLE_VIRTUAL:// paths.
-- This is critical: many parts of KOReader call realpath() to validate paths
-- before using them. Without this patch, virtual library paths would be rejected.
if pcall(require, "ffi/util") then
    local ffiUtil = require("ffi/util")

    if not ffiUtil._kindle_patched_realpath then
        local original_realpath = ffiUtil.realpath

        ffiUtil.realpath = function(path)
            if type(path) ~= "string" then
                return original_realpath(path)
            end

            -- Virtual library root: translate to cache directory
            if path == virtual_prefix or path == virtual_prefix .. "/" then
                local cache_dir = getCacheDir()
                logger.dbg("KindlePlugin Startup Patch: realpath intercepted virtual root:", path, "->", cache_dir)
                return cache_dir
            end

            -- Any KINDLE_VIRTUAL:// path: translate to cache directory
            if path:sub(1, #virtual_prefix) == virtual_prefix then
                local cache_dir = getCacheDir()
                logger.dbg("KindlePlugin Startup Patch: realpath intercepted virtual path:", path, "->", cache_dir)
                return cache_dir
            end

            -- Check if path contains "Kindle Library/" in the middle
            -- (e.g. "/mnt/us/Kindle Library/book.epub")
            local virtual_name = "Kindle Library"
            local escaped_name = PatternUtils.escape(virtual_name)
            local suffix = path:match("^" .. escaped_name .. "/(.*)$")

            if suffix then
                local cache_dir = getCacheDir()
                logger.dbg(
                    "KindlePlugin Startup Patch: realpath intercepted virtual library path:",
                    path,
                    "->",
                    cache_dir
                )
                return cache_dir
            end

            -- Check /name/ in the middle of path
            local prefix, path_suffix = path:match("^(.+)/" .. escaped_name .. "/(.+)$")
            if prefix and path_suffix then
                local cache_dir = getCacheDir()
                logger.dbg(
                    "KindlePlugin Startup Patch: realpath intercepted virtual library path:",
                    path,
                    "->",
                    cache_dir
                )
                return cache_dir
            end

            -- Path ends with /Kindle Library (folder itself)
            if path:match("/" .. escaped_name .. "/?$") then
                local cache_dir = getCacheDir()
                logger.dbg(
                    "KindlePlugin Startup Patch: realpath intercepted virtual library folder:",
                    path,
                    "->",
                    cache_dir
                )
                return cache_dir
            end

            return original_realpath(path)
        end

        ffiUtil._kindle_patched_realpath = true
        logger.info("KindlePlugin Startup Patch: Applied ffi/util.realpath patch for virtual library support")
    end
end
