-- Tests for KindleStateWriter module
-- cc.db access is virtualized through the shared lua-ljsqlite3 mock.

require("busted.runner")()
local helper = require("spec/test_helper")

describe("KindleStateWriter", function()
    local KindleStateWriter
    local CatalogDb
    local SQ3

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        SQ3 = helper.install_sqlite_mock()
        package.loaded["lua/lib/kindle_catalog_db"] = nil
        CatalogDb = require("lua/lib/kindle_catalog_db")
        CatalogDb._test_icu_installer = function()
            return true, function() end
        end
        package.loaded["lua/lib/kindle_state_writer"] = nil
        KindleStateWriter = require("lua/lib/kindle_state_writer")
    end)

    after_each(function()
        if CatalogDb then
            CatalogDb._test_icu_installer = nil
        end
        helper.reset_state()
    end)

    describe("writeByPath", function()
        it("should return false for nil path", function()
            assert.is_false(KindleStateWriter.writeByPath(nil, 50, os.time(), "reading"))
        end)

        it("should return false for empty path", function()
            assert.is_false(KindleStateWriter.writeByPath("", 50, os.time(), "reading"))
        end)

        it("should update progress, read state, and last access when the schema permits it", function()
            SQ3._getMock().rowexec_results["SELECT changes()"] = "1"

            local ok = KindleStateWriter.writeByPath("/mnt/us/documents/test.kfx", 56, 1775769644, "reading")

            local mock = SQ3._getMock()
            assert.is_true(ok)
            assert.is_not_nil(mock.prepared_sql[1]:match("UPDATE Entries"))
            assert.is_not_nil(mock.prepared_sql[1]:match("p_percentFinished"))
            assert.is_not_nil(mock.prepared_sql[1]:match("p_readState"))
            assert.is_not_nil(mock.prepared_sql[1]:match("p_lastAccess"))
            assert.equals(56, mock.bound_values[1])
            assert.equals(6, mock.bound_values[2])
            assert.equals(1775769644, tonumber(mock.bound_values[3]))
            assert.is_truthy(tostring(require("ffi").typeof(mock.bound_values[3])):find("int64_t", 1, true))
            assert.equals("/mnt/us/documents/test.kfx", mock.bound_values[4])
            assert.is_true(mock.prepared_sql[1]:find("COALESCE(p_isArchived, 0) = 0", 1, true) ~= nil)
            assert.is_not_nil(table.concat(mock.executed, "\n"):find("COMMIT", 1, true))
        end)

        it("should return false when no catalog row matches", function()
            SQ3._getMock().rowexec_results["SELECT changes()"] = "0"

            local ok = KindleStateWriter.writeByPath("/mnt/us/documents/test.kfx", 56, 1775769644, "reading")

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

            local ok = KindleStateWriter.writeByCdeKey("B007N6JEII", 1, 1776640914, "reading")

            assert.is_true(ok)
            local sql = SQ3._getMock().prepared_sql[1]
            assert.is_not_nil(sql:match("p_cdeKey = %? AND p_isLatestItem = 1"))
            assert.is_true(sql:find("p_location IS NOT NULL", 1, true) ~= nil)
            assert.is_true(sql:find("p_location <> ''", 1, true) ~= nil)
            assert.is_true(sql:find("COALESCE(p_isArchived, 0) = 0", 1, true) ~= nil)
            assert.equals("B007N6JEII", SQ3._getMock().bound_values[4])
        end)
    end)

    describe("writeByUuid", function()
        it("should write a virtual-library catalog row by p_uuid", function()
            SQ3._getMock().rowexec_results["SELECT changes()"] = "1"

            local ok = KindleStateWriter.writeByUuid("f82913d4-094a-43c6-8166-e330d40c1d7c", 48, 1776640914, "reading")

            assert.is_true(ok)
            local sql = SQ3._getMock().prepared_sql[1]
            assert.is_not_nil(sql:match("p_uuid = %?"))
            assert.is_nil(sql:find("p_sourceUuid", 1, true))
            assert.is_true(sql:find("COALESCE(p_isArchived, 0) = 0", 1, true) ~= nil)
            assert.equals("f82913d4-094a-43c6-8166-e330d40c1d7c", SQ3._getMock().bound_values[4])
        end)
    end)
    describe("percent handling", function()
        it("should bind the caller-supplied percent value unchanged", function()
            SQ3._getMock().rowexec_results["SELECT changes()"] = "1"

            KindleStateWriter.writeByPath("/mnt/us/documents/test.kfx", 56.7, os.time(), "reading")

            -- Callers floor whole-number percents; exact pushes keep Kindle's
            -- own fractional renderer percentage. The writer binds verbatim.
            assert.equals(56.7, SQ3._getMock().bound_values[1])
        end)
    end)

    describe("firmware-aware connection setup", function()
        it("falls back to progress-only fields when Amazon ICU cannot be attached", function()
            local original = CatalogDb.prepareWriteConnection
            CatalogDb.prepareWriteConnection = function()
                return { write_last_access = false, close = function() end }
            end
            SQ3._getMock().rowexec_results["SELECT changes()"] = "1"

            local ok = KindleStateWriter.writeByPath("/mnt/us/documents/test.kfx", 48, 1776640914, "reading")
            CatalogDb.prepareWriteConnection = original

            assert.is_true(ok)
            local mock = SQ3._getMock()
            assert.is_nil(mock.prepared_sql[1]:match("p_lastAccess"))
            assert.same({ 48, 6, "/mnt/us/documents/test.kfx" }, mock.bound_values)
        end)

        it("rolls back when catalog connection semantics cannot be preserved", function()
            local original = CatalogDb.prepareWriteConnection
            CatalogDb.prepareWriteConnection = function()
                return nil, "unsupported trigger contract"
            end
            SQ3._getMock().rowexec_results["SELECT changes()"] = "1"

            local ok = KindleStateWriter.writeByPath("/mnt/us/documents/test.kfx", 48, 1776640914, "reading")
            CatalogDb.prepareWriteConnection = original

            assert.is_false(ok)
            assert.is_true(table.concat(SQ3._getMock().executed, "\n"):find("ROLLBACK", 1, true) ~= nil)
        end)
    end)
end)
