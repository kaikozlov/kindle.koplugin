local DataStorage = require("datastorage")
local json = require("json")
local logger = require("logger")
local util = require("util")

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

function HelperClient:scan(root)
    logger.info("KindlePlugin: scanning root:", root)
    local result, err = self:_run({
        self:getBinaryPath(),
        "scan",
        "--root",
        root,
    })
    if result then
        local book_count = result.books and #result.books or 0
        logger.info("KindlePlugin: scan found", book_count, "books")
    else
        logger.warn("KindlePlugin: scan failed:", err)
    end
    return result, err
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

function HelperClient:drmInit()
    local root = self.settings.documents_root or "/mnt/us/documents"
    local cache_dir = self.settings.cache_dir or ""
    logger.info("KindlePlugin: running drm-init on root:", root, "cache:", cache_dir)
    local result, err = self:_run({
        self:getBinaryPath(),
        "drm-init",
        "--root",
        root,
        "--cache-dir",
        cache_dir,
    })
    if result then
        if result.ok then
            logger.info("KindlePlugin: drm-init succeeded, books:", result.books_found, "keys:", result.keys_found)
        else
            logger.warn("KindlePlugin: drm-init failed:", result.message)
        end
    else
        logger.warn("KindlePlugin: drm-init failed:", err)
    end
    return result, err
end

return HelperClient
