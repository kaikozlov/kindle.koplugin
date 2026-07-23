-- Tests for SessionFlags module using real files under /tmp.

require('busted.runner')()
local helper = require("spec/test_helper")

describe("SessionFlags", function()
    local original_time
    local test_dirs = {}

    setup(function()
        helper.setup_complete()
        original_time = os.time
    end)

    after_each(function()
        rawset(os, "time", original_time)
        for _, dir in ipairs(test_dirs) do
            os.execute("rm -rf " .. dir)
        end
        test_dirs = {}
    end)

    local function loadSessionFlags(timestamp)
        rawset(os, "time", function() return timestamp end)
        package.loaded["lua/lib/session_flags"] = nil
        local SessionFlags = require("lua/lib/session_flags")
        rawset(os, "time", original_time)

        local dir = "/tmp/kindle.koplugin/" .. tostring(timestamp)
        table.insert(test_dirs, dir)
        os.execute("mkdir -p " .. dir)
        return SessionFlags
    end

    describe("setFlag / isFlagSet", function()
        it("should set and detect a flag", function()
            local SessionFlags = loadSessionFlags(12345)

            SessionFlags:setFlag("test_flag")

            assert.is_true(SessionFlags:isFlagSet("test_flag"))
        end)

        it("should return false for unset flag", function()
            local SessionFlags = loadSessionFlags(12346)

            assert.is_false(SessionFlags:isFlagSet("nonexistent"))
        end)
    end)

    describe("clearFlag", function()
        it("should clear a set flag", function()
            local SessionFlags = loadSessionFlags(12347)
            SessionFlags:setFlag("to_clear")
            assert.is_true(SessionFlags:isFlagSet("to_clear"))

            SessionFlags:clearFlag("to_clear")

            assert.is_false(SessionFlags:isFlagSet("to_clear"))
        end)
    end)

    describe("clearAllFlags", function()
        it("should clear all flags", function()
            local SessionFlags = loadSessionFlags(12348)
            SessionFlags:setFlag("flag1")
            SessionFlags:setFlag("flag2")
            assert.is_true(SessionFlags:isFlagSet("flag1"))
            assert.is_true(SessionFlags:isFlagSet("flag2"))

            SessionFlags:clearAllFlags()

            assert.is_false(SessionFlags:isFlagSet("flag1"))
            assert.is_false(SessionFlags:isFlagSet("flag2"))
        end)
    end)
end)
