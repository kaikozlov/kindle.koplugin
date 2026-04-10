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
        return nil, "kindle-helper binary not found at " .. self:getBinaryPath()
    end

    local command = util.shell_escape(args) .. " 2>&1"
    local handle = io.popen(command)
    if not handle then
        return nil, "failed to start helper process"
    end

    local output = handle:read("*a") or ""
    handle:close()

    local ok, decoded = pcall(json.decode, output)
    if not ok then
        logger.warn("KindlePlugin: failed to decode helper JSON:", output)
        return nil, "invalid helper JSON"
    end

    return decoded
end

function HelperClient:scan(root)
    return self:_run({
        self:getBinaryPath(),
        "scan",
        "--root",
        root,
    })
end

function HelperClient:convert(input_path, output_path)
    return self:_run({
        self:getBinaryPath(),
        "convert",
        "--input",
        input_path,
        "--output",
        output_path,
    })
end

return HelperClient
