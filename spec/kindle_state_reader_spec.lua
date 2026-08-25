-- Tests for KindleStateReader module
-- cc.db access is virtualized through the shared lua-ljsqlite3 mock.

require("busted.runner")()
local helper = require("spec/test_helper")

describe("KindleStateReader", function()
    local KindleStateReader
    local SQ3

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        SQ3 = helper.install_sqlite_mock()
        package.loaded["lua/lib/kindle_state_reader"] = nil
        KindleStateReader = require("lua/lib/kindle_state_reader")
    end)

    after_each(function()
        helper.reset_state()
    end)

    describe("readByPath", function()
        it("should return nil for nil path", function()
            assert.is_nil(KindleStateReader.readByPath(nil))
        end)

        it("should return nil for empty path", function()
            assert.is_nil(KindleStateReader.readByPath(""))
        end)

        it("should read reading progress through ljsqlite3", function()
            SQ3._setMockResults({
                { "56.477375" },
                { "1775769644" },
                { "" },
                { "Floors #2: 3 Below" },
                { "B008PL1YQ0" },
            }, 1)

            local state = KindleStateReader.readByPath("/mnt/us/documents/test.kfx")

            assert.is_not_nil(state)
            assert.equals(56.477375, state.percent_read)
            assert.equals(1775769644, state.timestamp)
            assert.equals("Floors #2: 3 Below", state.title)
            assert.equals("B008PL1YQ0", state.cde_key)
            assert.is_true(SQ3._getMock().prepared_sql[1]:match("p_location = %?") ~= nil)
            assert.equals("/mnt/us/documents/test.kfx", SQ3._getMock().bound_values[1])
        end)

        it("should handle NULL percent_finished as 0", function()
            SQ3._setMockResults({
                { "" },
                { "" },
                { "" },
                { "Some Book" },
                { "B001" },
            }, 1)

            local state = KindleStateReader.readByPath("/mnt/us/documents/test.kfx")

            assert.is_not_nil(state)
            assert.equals(0, state.percent_read)
        end)

        it("should return nil when no row matches", function()
            SQ3._setMockResults(nil, 0)

            local state = KindleStateReader.readByPath("/mnt/us/documents/nonexistent.kfx")

            assert.is_nil(state)
        end)

        it("should return nil without ljsqlite3", function()
            helper.install_sqlite_unavailable()
            package.loaded["lua/lib/kindle_state_reader"] = nil
            local Reader = require("lua/lib/kindle_state_reader")

            assert.is_nil(Reader.readByPath("/mnt/us/documents/test.kfx"))
        end)
    end)

    describe("readByCdeKey", function()
        it("should return nil for nil key", function()
            assert.is_nil(KindleStateReader.readByCdeKey(nil))
        end)

        it("should read progress by ASIN", function()
            SQ3._setMockResults({
                { "67.035034" },
                { "1775770105" },
                { "" },
                { "The Hunger Games Trilogy" },
                { "B004XJRQUQ" },
            }, 1)

            local state = KindleStateReader.readByCdeKey("B004XJRQUQ")

            assert.is_not_nil(state)
            assert.equals(67.035034, state.percent_read)
            assert.equals("B004XJRQUQ", state.cde_key)
            assert.is_true(SQ3._getMock().prepared_sql[1]:match("p_cdeKey = %?") ~= nil)
        end)
    end)

    describe("readByUuid", function()
        it("should return nil for nil UUID", function()
            assert.is_nil(KindleStateReader.readByUuid(nil))
        end)

        it("should read a virtual-library catalog row by p_uuid", function()
            SQ3._setMockResults({
                { "47" },
                { "1775770105" },
                { "6" },
                { "The Almighty Dollar" },
                { "B0FLB24198" },
            }, 1)

            local state = KindleStateReader.readByUuid("f82913d4-094a-43c6-8166-e330d40c1d7c")

            assert.equals(47, state.percent_read)
            assert.equals("B0FLB24198", state.cde_key)
            assert.is_true(SQ3._getMock().prepared_sql[1]:match("p_uuid = ") ~= nil)
        end)
    end)

    describe("readAllProgress", function()
        it("should read multiple books from cc.db", function()
            SQ3._setMockResults({
                { "B007N6JEII", "B008PL1YQ0" },
                { "EBOK", "EBOK" },
                { "Throne of Glass", "Three Below" },
                { "1.162167", "56.477375" },
                { "1776640914", "1775769644" },
                { "/mnt/us/documents/test.kfx", "/mnt/us/documents/test2.kfx" },
            }, 2)

            local books = KindleStateReader.readAllProgress()

            assert.is_not_nil(books)
            assert.equals(2, #books)
            assert.equals("B007N6JEII", books[1].cde_key)
            assert.equals(1.162167, books[1].percent_read)
            assert.equals("B008PL1YQ0", books[2].cde_key)
            assert.equals(56.477375, books[2].percent_read)
        end)

        it("should return an empty table when no rows match", function()
            SQ3._setMockResults(nil, 0)

            local books = KindleStateReader.readAllProgress()

            assert.is_not_nil(books)
            assert.equals(0, #books)
        end)
    end)
end)
