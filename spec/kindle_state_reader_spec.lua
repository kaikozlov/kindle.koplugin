-- Tests for KindleStateReader module
-- These tests mock the sqlite3 CLI since we don't have ljsqlite3 in the test env

describe("KindleStateReader", function()
    local KindleStateReader
    local original_popen

    setup(function()
        require("spec/helper")
    end)

    before_each(function()
        package.loaded["lua/lib/kindle_state_reader"] = nil
        KindleStateReader = require("lua/lib/kindle_state_reader")
        original_popen = io.popen
    end)

    after_each(function()
        io.popen = original_popen
    end)

    local function mockPopen(output, should_fail)
        io.popen = function(cmd, mode)
            if should_fail then
                return nil
            end
            local mock_handle = {
                _output = output,
                _lines = {},
                read = function(self, fmt)
                    if fmt == "*l" then
                        local line = self._lines[1]
                        if line then
                            table.remove(self._lines, 1)
                            return line
                        end
                        return nil
                    end
                    return self._output
                end,
                lines = function(self)
                    local i = 0
                    return function()
                        i = i + 1
                        return self._lines[i]
                    end
                end,
                close = function(self) return 0 end,
            }
            -- Parse output into lines
            for line in (output or ""):gmatch("[^\n]+") do
                table.insert(mock_handle._lines, line)
            end
            return mock_handle
        end
    end

    describe("readByPath", function()
        it("should return nil for nil path", function()
            assert.is_nil(KindleStateReader.readByPath(nil))
        end)

        it("should return nil for empty path", function()
            assert.is_nil(KindleStateReader.readByPath(""))
        end)

        it("should parse reading progress from cc.db CLI output", function()
            mockPopen("56.477375|1775769644||Floors #2: 3 Below|B008PL1YQ0")

            local state = KindleStateReader.readByPath("/mnt/us/documents/test.kfx")

            assert.is_not_nil(state)
            assert.equals(56.477375, state.percent_read)
            assert.equals(1775769644, state.timestamp)
            assert.equals("Floors #2: 3 Below", state.title)
            assert.equals("B008PL1YQ0", state.cde_key)
        end)

        it("should handle NULL percent_finished as 0", function()
            mockPopen("|1775769644||Some Book|B001")

            local state = KindleStateReader.readByPath("/mnt/us/documents/test.kfx")

            assert.is_not_nil(state)
            assert.equals(0, state.percent_read)
        end)

        it("should return nil when no results found", function()
            mockPopen("")

            local state = KindleStateReader.readByPath("/mnt/us/documents/nonexistent.kfx")

            assert.is_nil(state)
        end)

        it("should return nil when popen fails", function()
            mockPopen("", true)

            local state = KindleStateReader.readByPath("/mnt/us/documents/test.kfx")

            assert.is_nil(state)
        end)
    end)

    describe("readByCdeKey", function()
        it("should return nil for nil key", function()
            assert.is_nil(KindleStateReader.readByCdeKey(nil))
        end)

        it("should read progress by ASIN", function()
            mockPopen("67.035034|1775770105||The Hunger Games Trilogy|B004XJRQUQ")

            local state = KindleStateReader.readByCdeKey("B004XJRQUQ")

            assert.is_not_nil(state)
            assert.equals(67.035034, state.percent_read)
            assert.equals("B004XJRQUQ", state.cde_key)
        end)
    end)

    describe("readAllProgress", function()
        it("should parse multiple books from CLI output", function()
            mockPopen(
                "B007N6JEII|EBOK|Throne of Glass|1.162167|1776640914|/mnt/us/documents/test.kfx\n"
                .. "B008PL1YQ0|EBOK|Three Below|56.477375|1775769644|/mnt/us/documents/test2.kfx"
            )

            local books = KindleStateReader.readAllProgress()

            assert.is_not_nil(books)
            assert.equals(2, #books)
            assert.equals("B007N6JEII", books[1].cde_key)
            assert.equals(1.162167, books[1].percent_read)
            assert.equals("B008PL1YQ0", books[2].cde_key)
            assert.equals(56.477375, books[2].percent_read)
        end)

        it("should return empty table for empty result", function()
            mockPopen("")

            local books = KindleStateReader.readAllProgress()

            -- CLI fallback returns {} when no lines parsed
            assert.is_not_nil(books)
        end)
    end)
end)
