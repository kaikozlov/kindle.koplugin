-- Tests for HelperClient module

describe("HelperClient", function()
    local HelperClient

    setup(function()
        require("spec/helper")
        HelperClient = require("lua/helper_client")
    end)

    before_each(function()
        package.loaded["lua/helper_client"] = nil
        HelperClient = require("lua/helper_client")
        resetAllMocks()
    end)

    describe("initialization", function()
        it("should create new instance", function()
            local client = HelperClient:new()

            assert.is_not_nil(client)
        end)

        it("should accept opts table", function()
            local client = HelperClient:new({ binary_path = "/custom/binary" })

            assert.equals("/custom/binary", client.binary_path)
        end)
    end)

    describe("setSettings", function()
        it("should store settings", function()
            local client = HelperClient:new()
            local settings = { cache_dir = "/cache" }

            client:setSettings(settings)

            assert.equals(settings, client.settings)
        end)
    end)

    describe("getPluginPath", function()
        it("should return plugin path under DataStorage", function()
            local client = HelperClient:new()

            local path = client:getPluginPath()

            assert.is_string(path)
            assert.is_true(path:match("kindle%.koplugin") ~= nil)
        end)
    end)

    describe("getBinaryPath", function()
        it("should use custom binary_path when set", function()
            local client = HelperClient:new({ binary_path = "/custom/kindle-helper" })

            assert.equals("/custom/kindle-helper", client:getBinaryPath())
        end)

        it("should default to plugin path + kindle-helper", function()
            local client = HelperClient:new()

            local path = client:getBinaryPath()

            assert.is_true(path:match("kindle%-helper$") ~= nil)
        end)
    end)

    describe("_run", function()
        it("should use runner function when available", function()
            local args_passed = nil
            local client = HelperClient:new({
                runner = function(args)
                    args_passed = args
                    return { ok = true }
                end,
            })

            local result = client:_run({ "test-binary", "scan" })

            assert.is_not_nil(result)
            assert.is_table(args_passed)
        end)

        it("should return error when binary not found", function()
            local client = HelperClient:new({
                binary_path = "/nonexistent/binary",
            })

            local result, err = client:_run({ "/nonexistent/binary", "scan" })

            assert.is_nil(result)
            assert.is_string(err)
        end)
    end)

    describe("scan", function()
        it("should call _run with scan command", function()
            local client = HelperClient:new({
                runner = function(args)
                    return { books = { { id = "b1" } } }
                end,
            })
            client:setSettings({})

            local result = client:scan("/test/root")

            assert.is_not_nil(result)
            assert.is_table(result.books)
        end)
    end)

    describe("convert", function()
        it("should call _run with convert command", function()
            local client = HelperClient:new({
                runner = function(args)
                    return { ok = true, output_path = "/output.epub" }
                end,
            })
            client:setSettings({})

            local result = client:convert("/input.kfx", "/output.epub")

            assert.is_not_nil(result)
            assert.is_true(result.ok)
        end)
    end)

    describe("drmInit", function()
        it("should call _run with drm-init command", function()
            local client = HelperClient:new({
                runner = function(args)
                    return { ok = true, books_found = 5, keys_found = 3 }
                end,
            })
            client:setSettings({})

            local result = client:drmInit()

            assert.is_not_nil(result)
            assert.is_true(result.ok)
            assert.equals(5, result.books_found)
            assert.equals(3, result.keys_found)
        end)
    end)
end)
