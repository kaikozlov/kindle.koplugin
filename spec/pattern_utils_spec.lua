-- Tests for PatternUtils module

describe("PatternUtils", function()
    local PatternUtils

    setup(function()
        require("spec/helper")
        PatternUtils = require("lua/lib/pattern_utils")
    end)

    describe("escape", function()
        it("should escape dots", function()
            assert.equals("test%.lua", PatternUtils.escape("test.lua"))
        end)

        it("should escape dashes", function()
            assert.equals("a%-b", PatternUtils.escape("a-b"))
        end)

        it("should escape plus signs", function()
            assert.equals("a%+b", PatternUtils.escape("a+b"))
        end)

        it("should escape asterisks", function()
            assert.equals("a%*b", PatternUtils.escape("a*b"))
        end)

        it("should escape question marks", function()
            assert.equals("a%?b", PatternUtils.escape("a?b"))
        end)

        it("should escape carets", function()
            assert.equals("a%^b", PatternUtils.escape("a^b"))
        end)

        it("should escape dollar signs", function()
            assert.equals("a%$b", PatternUtils.escape("a$b"))
        end)

        it("should escape parentheses", function()
            assert.equals("%(%)", PatternUtils.escape("()"))
        end)

        it("should escape square brackets", function()
            assert.equals("%[%]", PatternUtils.escape("[]"))
        end)

        it("should escape percent signs", function()
            assert.equals("%%", PatternUtils.escape("%"))
        end)

        it("should return nil for nil input", function()
            assert.is_nil(PatternUtils.escape(nil))
        end)

        it("should return non-string input unchanged", function()
            assert.equals(42, PatternUtils.escape(42))
        end)

        it("should handle empty string", function()
            assert.equals("", PatternUtils.escape(""))
        end)

        it("should escape a full file path", function()
            local escaped = PatternUtils.escape("/mnt/us/documents/test-book.epub")

            -- Verify dashes and dots are escaped
            assert.is_true(escaped:match("%%-") ~= nil)
            assert.is_true(escaped:match("%%.") ~= nil)
            -- Verify the result can be used as a pattern to match the original
            assert.is_true(("/mnt/us/documents/test-book.epub"):match(escaped) ~= nil)
        end)
    end)
end)
