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

    describe("ljsqlite3 catalog trigger compatibility", function()
        local function fakeSQ3(changes, prepare_error)
            local calls = {}
            local callbacks = {}
            local stmt = {}

            function stmt:reset()
                table.insert(calls, "reset")
                return self
            end
            function stmt:bind(...)
                self.bound = { ... }
                table.insert(calls, "bind")
                return self
            end
            function stmt:step()
                table.insert(calls, "step")
                return self
            end
            function stmt:close()
                table.insert(calls, "statement_close")
            end

            local conn = {}
            function conn:set_busy_timeout(timeout)
                self.timeout = timeout
                table.insert(calls, "busy_timeout")
            end
            function conn:setscalar(name, callback)
                callbacks[name] = callback
                table.insert(calls, "setscalar:" .. name)
            end
            function conn:exec(sql)
                table.insert(calls, sql)
            end
            function conn:prepare(sql)
                table.insert(calls, "prepare:" .. sql)
                if prepare_error then
                    error("no such function")
                end
                return stmt
            end
            function conn:rowexec(sql)
                table.insert(calls, sql)
                return changes
            end
            function conn:close()
                table.insert(calls, "connection_close")
            end

            return {
                open = function() return conn end,
            }, calls, callbacks, stmt
        end

        it("registers firmware trigger shims and commits a matched update", function()
            local SQ3, calls, callbacks, stmt = fakeSQ3(1, false)

            local backend_ok, updated = KindleStateWriter._writeWithSQ3(
                SQ3,
                "p_cdeKey = ?",
                "B007N6JEII",
                48,
                1
            )

            assert.is_true(backend_ok)
            assert.is_true(updated)
            assert.equals(5000, SQ3.open().timeout)
            assert.is_function(callbacks.get_companion_relation_external_id)
            assert.is_function(callbacks.get_entry_external_id)
            assert.is_function(callbacks.get_entry_change_type)
            assert.is_function(callbacks.build_merge_changes)
            assert.is_function(callbacks.build_merge_changes_delta)
            assert.is_nil(callbacks.get_entry_external_id("ignored"))
            assert.same({ 48, 1, "B007N6JEII" }, stmt.bound)
            assert.is_true(table.concat(calls, "\n"):find("BEGIN IMMEDIATE", 1, true) ~= nil)
            assert.is_true(table.concat(calls, "\n"):find("COMMIT", 1, true) ~= nil)
        end)

        it("rolls back when SQLite still cannot prepare the update", function()
            local SQ3, calls = fakeSQ3(1, true)

            local backend_ok, updated = KindleStateWriter._writeWithSQ3(
                SQ3,
                "p_cdeKey = ?",
                "B007N6JEII",
                48,
                1
            )

            assert.is_false(backend_ok)
            assert.is_false(updated)
            assert.is_true(table.concat(calls, "\n"):find("ROLLBACK", 1, true) ~= nil)
        end)

        it("rolls back and reports no match when zero rows change", function()
            local SQ3, calls = fakeSQ3(0, false)

            local backend_ok, updated = KindleStateWriter._writeWithSQ3(
                SQ3,
                "p_cdeKey = ?",
                "missing",
                48,
                1
            )

            assert.is_true(backend_ok)
            assert.is_false(updated)
            assert.is_true(table.concat(calls, "\n"):find("ROLLBACK", 1, true) ~= nil)
            assert.is_nil(table.concat(calls, "\n"):find("COMMIT", 1, true))
        end)
    end)
end)
