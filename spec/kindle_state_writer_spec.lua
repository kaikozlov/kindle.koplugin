-- Tests for KindleStateWriter module
-- cc.db access is virtualized through the shared lua-ljsqlite3 mock.

require('busted.runner')()
local helper = require("spec/test_helper")

describe("KindleStateWriter", function()
    local KindleStateWriter
    local SQ3

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        SQ3 = helper.install_sqlite_mock()
        package.loaded["lua/lib/kindle_state_writer"] = nil
        KindleStateWriter = require("lua/lib/kindle_state_writer")
    end)

    after_each(function()
        helper.reset_state()
    end)

    describe("writeByPath", function()
        it("should return false for nil path", function()
            assert.is_false(KindleStateWriter.writeByPath(nil, 50, os.time(), "reading"))
        end)

        it("should return false for empty path", function()
            assert.is_false(KindleStateWriter.writeByPath("", 50, os.time(), "reading"))
        end)

        it("should update only progress and read state", function()
            SQ3._getMock().rowexec_results["SELECT changes()"] = "1"

            local ok = KindleStateWriter.writeByPath(
                "/mnt/us/documents/test.kfx",
                56,
                1775769644,
                "reading"
            )

            local mock = SQ3._getMock()
            assert.is_true(ok)
            assert.is_not_nil(mock.prepared_sql[1]:match("UPDATE Entries"))
            assert.is_not_nil(mock.prepared_sql[1]:match("p_percentFinished"))
            assert.is_not_nil(mock.prepared_sql[1]:match("p_readState"))
            -- p_lastAccess is NOT updated (ICU collation index)
            assert.is_nil(mock.prepared_sql[1]:match("p_lastAccess"))
            assert.same({ 56, 6, "/mnt/us/documents/test.kfx" }, mock.bound_values)
            assert.is_not_nil(table.concat(mock.executed, "\n"):find("COMMIT", 1, true))
        end)

        it("should return false when no catalog row matches", function()
            SQ3._getMock().rowexec_results["SELECT changes()"] = "0"

            local ok = KindleStateWriter.writeByPath(
                "/mnt/us/documents/test.kfx",
                56,
                1775769644,
                "reading"
            )

            assert.is_false(ok)
            assert.is_nil(table.concat(SQ3._getMock().executed, "\n"):find("COMMIT", 1, true))
        end)

        it("should return false without ljsqlite3", function()
            helper.install_sqlite_unavailable()
            package.loaded["lua/lib/kindle_state_writer"] = nil
            local Writer = require("lua/lib/kindle_state_writer")

            assert.is_false(Writer.writeByPath("/mnt/us/documents/test.kfx", 56, 0, "reading"))
        end)
    end)

    describe("writeByCdeKey", function()
        it("should return false for nil key", function()
            assert.is_false(KindleStateWriter.writeByCdeKey(nil, 50, os.time(), "reading"))
        end)

        it("should write by ASIN with the latest-item guard", function()
            SQ3._getMock().rowexec_results["SELECT changes()"] = "1"

            local ok = KindleStateWriter.writeByCdeKey(
                "B007N6JEII",
                1,
                1776640914,
                "reading"
            )

            assert.is_true(ok)
            assert.is_not_nil(
                SQ3._getMock().prepared_sql[1]:match("p_cdeKey = %? AND p_isLatestItem = 1"))
            assert.equals("B007N6JEII", SQ3._getMock().bound_values[3])
        end)
    end)

    describe("writeByUuid", function()
        it("should write a virtual-library catalog row by p_uuid", function()
            SQ3._getMock().rowexec_results["SELECT changes()"] = "1"

            local ok = KindleStateWriter.writeByUuid(
                "f82913d4-094a-43c6-8166-e330d40c1d7c",
                48,
                1776640914,
                "reading"
            )

            assert.is_true(ok)
            assert.is_not_nil(
                SQ3._getMock().prepared_sql[1]:match("p_uuid = %(SELECT p_sourceUuid"))
            assert.equals("f82913d4-094a-43c6-8166-e330d40c1d7c", SQ3._getMock().bound_values[3])
        end)
    end)
    describe("percent handling", function()
        it("should bind the caller-supplied percent value unchanged", function()
            SQ3._getMock().rowexec_results["SELECT changes()"] = "1"

            KindleStateWriter.writeByPath(
                "/mnt/us/documents/test.kfx",
                56.7,
                os.time(),
                "reading"
            )

            -- Callers floor whole-number percents; exact pushes keep Kindle's
            -- own fractional renderer percentage. The writer binds verbatim.
            assert.equals(56.7, SQ3._getMock().bound_values[1])
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
