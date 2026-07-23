-- Tests for KindleStateWriter module

require('busted.runner')()
local helper = require("spec/test_helper")

describe("KindleStateWriter", function()
    local KindleStateWriter
    local original_execute

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        helper.install_sqlite_unavailable()
        package.loaded["lua/lib/kindle_state_writer"] = nil
        KindleStateWriter = require("lua/lib/kindle_state_writer")
        original_execute = os.execute
    end)

    after_each(function()
        rawset(os, "execute", original_execute)
    end)

    local function captureExecute(result)
        local executed_cmd
        rawset(os, "execute", function(cmd)
            executed_cmd = cmd
            return result
        end)
        return function() return executed_cmd end
    end

    describe("writeByPath", function()
        it("should return false for nil path", function()
            assert.is_false(KindleStateWriter.writeByPath(nil, 50, os.time(), "reading"))
        end)

        it("should return false for empty path", function()
            assert.is_false(KindleStateWriter.writeByPath("", 50, os.time(), "reading"))
        end)

        it("should execute sqlite3 UPDATE via CLI", function()
            local get_executed_cmd = captureExecute(0)

            local ok = KindleStateWriter.writeByPath(
                "/mnt/us/documents/test.kfx",
                56,
                1775769644,
                "reading"
            )

            local executed_cmd = get_executed_cmd()
            assert.is_true(ok)
            assert.is_not_nil(executed_cmd)
            assert.is_true(executed_cmd:match("UPDATE Entries") ~= nil)
            assert.is_true(executed_cmd:match("p_percentFinished") ~= nil)
            -- p_lastAccess is NOT updated (ICU collation index)
            assert.is_true(executed_cmd:match("p_readState") ~= nil)
        end)

        it("should return false when sqlite3 fails", function()
            captureExecute(1)

            local ok = KindleStateWriter.writeByPath(
                "/mnt/us/documents/test.kfx",
                56,
                1775769644,
                "reading"
            )

            assert.is_false(ok)
        end)
    end)

    describe("writeByCdeKey", function()
        it("should return false for nil key", function()
            assert.is_false(KindleStateWriter.writeByCdeKey(nil, 50, os.time(), "reading"))
        end)

        it("should write by ASIN with correct WHERE clause", function()
            local get_executed_cmd = captureExecute(0)

            local ok = KindleStateWriter.writeByCdeKey(
                "B007N6JEII",
                1,
                1776640914,
                "reading"
            )

            local executed_cmd = get_executed_cmd()
            assert.is_true(ok)
            assert.is_not_nil(executed_cmd)
            assert.is_true(executed_cmd:match("B007N6JEII") ~= nil)
            assert.is_true(executed_cmd:match("p_isLatestItem") ~= nil)
        end)
    end)

    describe("percent formatting", function()
        it("should floor the percent value", function()
            local get_executed_cmd = captureExecute(0)

            KindleStateWriter.writeByPath(
                "/mnt/us/documents/test.kfx",
                56.7,
                os.time(),
                "reading"
            )

            local executed_cmd = get_executed_cmd()
            assert.is_true(executed_cmd:match("p_percentFinished = 56") ~= nil)
        end)
    end)
end)
