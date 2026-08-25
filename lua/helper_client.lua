local json = require("json")
local logger = require("logger")
local util = require("util")

local KindleSidecar = require("lua/lib/kindle_sidecar")
local PositionMap = require("lua/lib/position_map")

local DataStorage = require("datastorage")

local HelperClient = {}
HelperClient.__index = HelperClient

function HelperClient:new(opts)
    local instance = opts or {}
    setmetatable(instance, self)
    return instance
end

function HelperClient:setSettings(settings)
    self.settings = settings or {}
end

function HelperClient:getPluginPath()
    return DataStorage:getFullDataDir() .. "/plugins/kindle.koplugin"
end

function HelperClient:getBinaryPath()
    if self.binary_path then
        return self.binary_path
    end

    return self:getPluginPath() .. "/kindle-helper"
end

function HelperClient:binaryExists()
    local handle = io.open(self:getBinaryPath(), "rb")
    if handle then
        handle:close()
        return true
    end

    return false
end

function HelperClient:_run(args)
    if self.runner then
        return self.runner(args)
    end

    if not self:binaryExists() then
        logger.warn("KindlePlugin: kindle-helper binary not found at", self:getBinaryPath())
        return nil, "kindle-helper binary not found at " .. self:getBinaryPath()
    end

    -- Capture stdout (JSON) cleanly; redirect stderr to temp file for debug
    local tmp_stderr = os.tmpname()
    local command = util.shell_escape(args) .. " 2>" .. util.shell_escape({tmp_stderr})
    logger.dbg("KindlePlugin: running helper:", util.shell_escape(args))
    local handle = io.popen(command)
    if not handle then
        os.remove(tmp_stderr)
        logger.warn("KindlePlugin: failed to start helper process")
        return nil, "failed to start helper process"
    end

    local output = handle:read("*a") or ""
    handle:close()

    -- Log stderr for debugging
    local stderr_handle = io.open(tmp_stderr, "rb")
    if stderr_handle then
        local stderr_output = stderr_handle:read("*a") or ""
        stderr_handle:close()
        if stderr_output ~= "" then
            logger.dbg("KindlePlugin: helper stderr:", stderr_output:sub(1, 500))
        end
    end
    os.remove(tmp_stderr)

    logger.dbg("KindlePlugin: helper stdout length:", #output)

    local ok, decoded = pcall(json.decode, output)
    if not ok then
        logger.warn("KindlePlugin: failed to decode helper JSON, raw output:", output:sub(1, 200))
        return nil, "invalid helper JSON"
    end

    return decoded
end

function HelperClient:convert(input_path, output_path)
    logger.info("KindlePlugin: converting", input_path, "->", output_path)
    local result, err = self:_run({
        self:getBinaryPath(),
        "convert",
        "--input",
        input_path,
        "--output",
        output_path,
        "--cache-dir",
        self.settings.cache_dir or "",
    })
    if result then
        if result.ok then
            logger.info("KindlePlugin: conversion succeeded:", result.output_path)
        else
            logger.warn("KindlePlugin: conversion failed:", result.code, result.message)
        end
    else
        logger.warn("KindlePlugin: convert failed:", err)
    end
    return result, err
end

--- Load the conversion-time position map for a cached EPUB.
function HelperClient:_positionMap(epub_path)
    if self._position_map_path == epub_path and self._position_map then
        return self._position_map
    end
    local map, err = PositionMap.load(epub_path)
    if not map then
        return nil, err
    end
    self._position_map_path = epub_path
    self._position_map = map
    return map
end

--- Translate a KOReader XPointer into Kindle's exact long and short position.
function HelperClient:translatePosition(epub_path, xpointer)
    if self.translate_position then
        return self.translate_position(epub_path, xpointer)
    end
    local map, map_error = self:_positionMap(epub_path)
    if not map then
        return nil, map_error
    end
    local result, err = PositionMap.translate_xpointer(map, xpointer)
    if not result then
        return nil, err
    end
    self._last_native_percent = result.percent
    return result
end

function HelperClient:translateNativePosition(epub_path, long_position)
    if self.translate_native then
        return self.translate_native(epub_path, long_position)
    end
    local map, map_error = self:_positionMap(epub_path)
    if not map then
        return nil, map_error
    end
    return PositionMap.translate_native(map, long_position)
end

local function readableFile(path)
    local handle = io.open(path, "rb")
    if not handle then
        return false
    end
    handle:close()
    return true
end

--- Find every KRDS reading-position sidecar next to a Kindle book.
local function positionSidecars(native_path)
    local stem = native_path:gsub("%.%w+$", "")
    local sidecar_dir = stem .. ".sdr"
    local candidates = {}
    local opened = io.open(sidecar_dir, "rb")
    if not opened then
        -- lfs.dir requires the dir to exist; a plain open won't list it, so
        -- fall back to lfs below.
        opened = nil
    else
        opened:close()
    end
    local lfs = require("libs/libkoreader-lfs")
    if lfs.attributes(sidecar_dir, "mode") ~= "directory" then
        return candidates
    end
    for name in lfs.dir(sidecar_dir) do
        local extension = name:match("%.(%w+)$")
        if extension == "yjf" or extension == "yjr"
            or extension == "azw3f" or extension == "azw3r"
        then
            local path = sidecar_dir .. "/" .. name
            if lfs.attributes(path, "mode") == "file" then
                table.insert(candidates, path)
            end
        end
    end
    -- Newest first; the freshly written sidecar is the authority.
    table.sort(candidates, function(a, b)
        return (lfs.attributes(a, "modification") or 0)
            > (lfs.attributes(b, "modification") or 0)
    end)
    return candidates
end

local function readSidecarBytes(data)
    local store, err = KindleSidecar.parse(data)
    if not store then
        return nil, err
    end
    -- Match the helper's policy: lpr wins regardless of timestamp, then the
    local best_position, best_timestamp, best_score
    for _, name in ipairs({ "lpr", "updated_lpr", "erl" }) do
        for _, obj in ipairs(KindleSidecar.objects(store, name)) do
            local position, timestamp = KindleSidecar.position_from_object(obj)
            if position then
                local long, pid = position:match("^([^:]+):(%d+)$")
                if long and pid then
                    local score = (name == "lpr" and 1e15 or 0)
                        + (timestamp or -1)
                    if not best_score or score > best_score then
                        best_score = score
                        best_timestamp = timestamp
                        best_position = position
                    end
                end
            end
        end
    end
    if not best_position then
        return nil, "sidecar has no readable last-page position"
    end
    local long, pid = best_position:match("^([^:]+):(%d+)$")
    return {
        store = store,
        long = long,
        pid = tonumber(pid),
        timestamp_ms = best_timestamp,
    }
end

local function readSidecarPosition(path)
    local handle = io.open(path, "rb")
    if not handle then
        return nil, "cannot open sidecar"
    end
    local data = handle:read("*a")
    handle:close()
    return readSidecarBytes(data)
end

local function writeSidecarPosition(path, long_position, pid, timestamp_ms)
    local handle = io.open(path, "rb")
    if not handle then
        return nil, "cannot open sidecar"
    end
    local data = handle:read("*a")
    handle:close()
    local store, err = KindleSidecar.parse(data)
    if not store then
        return nil, err
    end
    local roundtrip = KindleSidecar.encode(store)
    if roundtrip ~= data then
        return nil, "KRDS round-trip changed unmodified data"
    end
    local position = long_position .. ":" .. pid
    local timestamp = timestamp_ms or math.floor(os.time() * 1000)
    for _, name in ipairs({ "lpr", "updated_lpr", "erl" }) do
        for _, obj in ipairs(KindleSidecar.objects(store, name)) do
            KindleSidecar.set_object_position(obj, position, timestamp)
        end
    end
    for _, obj in ipairs(KindleSidecar.objects(store, "fpr")) do
        local current = KindleSidecar.position_from_object(obj)
        local current_pid = current and tonumber((current:match(":(%d+)$"))) or -1
        if current_pid < pid then
            KindleSidecar.set_object_position(obj, position, timestamp)
        end
    end
    for _, obj in ipairs(KindleSidecar.objects(store, "sync_lpr")) do
        local first = obj.values and obj.values[1]
        if first and first.tag == 0 then
            first.value = true
        end
    end

    local encoded = KindleSidecar.encode(store)
    if not encoded then
        return nil, "KRDS re-encode failed"
    end
    -- Verify the rewrite on the encoded bytes before replacing the file.
    local verified = readSidecarBytes(encoded)
    if not verified or verified.long ~= long_position or verified.pid ~= pid then
        return nil, "KRDS position readback mismatch"
    end
    local temp_path = path .. ".kindle-tmp." .. tostring(os.time())
    local temp = io.open(temp_path, "wb")
    if not temp then
        os.remove(temp_path)
        return nil, "cannot create temporary sidecar"
    end
    temp:write(encoded)
    temp:flush()
    temp:close()
    os.rename(temp_path, path)
    return true
end

--- Whether an exact Kindle coordinate backend is usable for this native book.
--- Position translation and Reader Data Store sidecars run in-process; the
--- Java ReaderSDK agent remains a compatibility fallback when available.
function HelperClient:nativeProgressAvailable(asin, native_path)
    if type(asin) ~= "string" or not asin:match("^B[A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9]$")
        or type(native_path) ~= "string"
        or not native_path:match("^/mnt/us/documents/.+%.kfx$")
    then
        return false
    end

    if self.native_progress_available ~= nil then
        if type(self.native_progress_available) == "function" then
            return self.native_progress_available(asin, native_path) == true
        end
        return self.native_progress_available == true
    end
    if self._native_progress_failed then
        return false
    end
    if self.native_progress_runner or self.native_progress_reader then
        return true
    end

    local plugin = self:getPluginPath()
    return readableFile(self:getBinaryPath())
        or (readableFile("/usr/java/bin/java")
            and readableFile(plugin .. "/bin/sync-native-progress")
            and readableFile(plugin .. "/bin/native-reading-progress-agent-v6.jar")
            and readableFile(plugin .. "/bin/classes/AttachLauncher.class"))
end

--- Save an exact position through the Kindle Reader Data Store sidecars.
function HelperClient:saveNativeProgress(asin, native_path, position)
    if type(asin) ~= "string" or not asin:match("^B[A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9][A-Z0-9]$") then
        return false, "invalid ASIN"
    end
    local pattern = self.native_path_pattern or "^/mnt/us/documents/.+%.kfx$"
    if type(native_path) ~= "string"
        or not native_path:match(pattern)
    then
        return false, "invalid native path"
    end
    if type(position) ~= "table" or type(position.long) ~= "string"
        or type(position.pid) ~= "number" then
        return false, "invalid native position"
    end
    if self.native_progress_runner then
        return self.native_progress_runner(asin, native_path, position)
    end

    if type(position.percent) == "number"
        and position.percent >= 0 and position.percent <= 100
    then
        local written_any = false
        for _, sidecar in ipairs(positionSidecars(native_path)) do
            local ok = writeSidecarPosition(sidecar, position.long, position.pid)
            if ok then
                written_any = true
            else
                logger.warn("KindlePlugin: sidecar position write failed:", sidecar)
            end
        end
        if written_any then
            logger.info("KindlePlugin: exact sidecar position saved:", asin, position.pid)
            return true, nil, position.percent, {
                long = position.long,
                pid = position.pid,
                percent = position.percent,
            }
        end
    end

    return false, "no Kindle position sidecar is writable"
end

--- Read Kindle's authoritative local last-page-read position.
function HelperClient:readNativeProgress(asin, native_path)
    if type(asin) ~= "string" or #asin ~= 10 or not asin:match("^B[A-Z0-9]+$") then
        return nil, "invalid ASIN"
    end
    local pattern = self.native_path_pattern or "^/mnt/us/documents/.+%.kfx$"
    if type(native_path) ~= "string"
        or not native_path:match(pattern)
    then
        return nil, "invalid native path"
    end
    if self.native_progress_reader then
        return self.native_progress_reader(asin, native_path)
    end

    for _, sidecar in ipairs(positionSidecars(native_path)) do
        local result = readSidecarPosition(sidecar)
        if result then
            return {
                long = result.long,
                pid = result.pid,
                timestamp_ms = result.timestamp_ms,
            }
        end
    end
    return nil, "Kindle position sidecar is unavailable"
end

--- One in-process read of everything an exact open/close sync needs: the
--- Kindle position sidecar, its reverse translation, and the forward
--- translation of KOReader's XPointer.
function HelperClient:readCloseState(native_path, epub_path, xpointer)
    if self.read_close_state then
        return self.read_close_state(native_path, epub_path, xpointer)
    end

    local result = { ok = true }
    local native = self:readNativeProgress("B000000000", native_path)
    if native then
        result.native = native
        local translated = self:translateNativePosition(epub_path, native.long)
        if translated then
            result.native_xpointer = translated.xpointer
            result.native_pid = translated.pid
            result.native_percent = translated.percent
        else
            result.native_translate_error = "native reverse translation failed"
        end
    else
        result.native_error = "Kindle position sidecar is unavailable"
    end

    local koreader = self:translatePosition(epub_path, xpointer)
    if koreader then
        result.koreader = koreader
    else
        result.ok = false
        result.koreader_error = "forward translation failed"
    end
    return result
end

--- Extracts the decryption key for a single book (JIT key extraction).
--- @param kfx_path string: Path to the KFX file.
--- @return table|nil: Result table, or nil on error.
--- @return string|nil: Error message if result is nil.
function HelperClient:extractBookKey(kfx_path)
    local cache_dir = self.settings.cache_dir or ""
    logger.info("KindlePlugin: extracting key for", kfx_path)
    local result, err = self:_run({
        self:getBinaryPath(),
        "extract-key",
        "--input",
        kfx_path,
        "--cache-dir",
        cache_dir,
    })
    if result then
        if result.ok then
            logger.info("KindlePlugin: key extracted for", result.book_id)
        else
            logger.warn("KindlePlugin: key extraction failed:", result.message)
        end
    else
        logger.warn("KindlePlugin: key extraction failed:", err)
    end
    return result, err
end

return HelperClient
