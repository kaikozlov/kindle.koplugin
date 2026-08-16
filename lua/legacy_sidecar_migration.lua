local DataStorage = require("datastorage")
local DocSettings = require("docsettings")
local ffiUtil = require("ffi/util")
local logger = require("logger")
local util = require("util")

local LegacySidecarMigration = {}
LegacySidecarMigration.__index = LegacySidecarMigration

function LegacySidecarMigration:new(virtual_library)
    return setmetatable({ virtual_library = virtual_library }, self)
end

local function sanitizeId(book_id)
    return (book_id or "unknown"):gsub("[^%w%.%-_]", "_")
end

local function isFile(path)
    local f = io.open(path, "rb")
    if not f then
        return false
    end
    f:close()
    return true
end

local function copyFile(source, destination)
    if not isFile(source) then
        return false
    end
    local ok, result = pcall(ffiUtil.copyFile, source, destination)
    return ok and result ~= false
end

--- Migrate the sidecar layout used by pre-real-path kindle.koplugin releases.
--- Native KOReader sidecars always win; this helper never overwrites an
--- existing doc/dir/hash/history candidate.
function LegacySidecarMigration.migrate(_, book, real_path)
    if not book or not real_path or real_path == "" then
        return false
    end

    local native_settings = DocSettings:open(real_path)
    if native_settings.source_candidate then
        return false
    end

    local legacy_dir = DataStorage:getDocSettingsDir()
        .. "/kindle_virtual/" .. sanitizeId(book.id) .. ".sdr"
    local logical_ext = book.logical_ext or book.format or "epub"
    local legacy_filename = DocSettings.getSidecarFilename("book." .. logical_ext)
    local legacy_primary = legacy_dir .. "/" .. legacy_filename
    local legacy_backup = legacy_primary .. ".old"
    if not isFile(legacy_primary) and not isFile(legacy_backup) then
        return false
    end

    local preferred_location = G_reader_settings:readSetting("document_metadata_folder", "doc")
    local target_dir = DocSettings:getSidecarDir(real_path, preferred_location)
    local target_filename = DocSettings.getSidecarFilename(real_path)
    local target_primary = target_dir .. "/" .. target_filename
    local target_backup = target_primary .. ".old"

    if isFile(target_primary) or isFile(target_backup) then
        return false
    end
    if not util.makePath(target_dir) then
        logger.warn("KindlePlugin: cannot create sidecar directory for legacy migration:", target_dir)
        return false
    end
    -- Hash-location availability is cached inside DocSettings. If this is the
    -- first hash sidecar created in the process, invalidate that cache so the
    -- sidecar we are about to write is immediately discoverable this session.
    if preferred_location == "hash" then
        DocSettings.setIsHashLocationEnabled(nil)
    end

    local copied_primary = not isFile(legacy_primary) or copyFile(legacy_primary, target_primary)
    local copied_backup = not isFile(legacy_backup) or copyFile(legacy_backup, target_backup)
    if not copied_primary or not copied_backup then
        logger.warn("KindlePlugin: legacy sidecar migration failed for", book.id)
        return false
    end

    if isFile(legacy_primary) then os.remove(legacy_primary) end
    if isFile(legacy_backup) then os.remove(legacy_backup) end
    os.remove(legacy_dir)
    logger.info("KindlePlugin: migrated legacy virtual sidecar to native KOReader path for", book.id)
    return true
end

return LegacySidecarMigration
