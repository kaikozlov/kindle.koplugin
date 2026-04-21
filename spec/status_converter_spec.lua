-- Tests for StatusConverter module

describe("StatusConverter", function()
    local StatusConverter

    setup(function()
        require("spec/helper")
    end)

    before_each(function()
        package.loaded["lua/lib/status_converter"] = nil
        StatusConverter = require("lua/lib/status_converter")
    end)

    describe("kindleToKoreader", function()
        it("should convert NULL/0 to empty string", function()
            assert.equals("", StatusConverter.kindleToKoreader(nil))
            assert.equals("", StatusConverter.kindleToKoreader(0))
        end)

        it("should convert 6 (reading) to 'reading'", function()
            assert.equals("reading", StatusConverter.kindleToKoreader(6))
        end)

        it("should convert unknown values to 'reading'", function()
            assert.equals("reading", StatusConverter.kindleToKoreader(99))
            assert.equals("reading", StatusConverter.kindleToKoreader(1))
        end)

        it("should handle string '0' as numeric 0", function()
            assert.equals("", StatusConverter.kindleToKoreader("0"))
        end)

        it("should handle string '6' as numeric 6", function()
            assert.equals("reading", StatusConverter.kindleToKoreader("6"))
        end)

        it("should handle non-numeric string gracefully", function()
            -- tonumber('abc') returns nil → or 0 → returns ""
            assert.equals("", StatusConverter.kindleToKoreader("abc"))
        end)
    end)

    describe("koreaderToKindle", function()
        it("should convert 'reading' to 6", function()
            assert.equals(6, StatusConverter.koreaderToKindle("reading"))
        end)

        it("should convert 'complete' to 6", function()
            assert.equals(6, StatusConverter.koreaderToKindle("complete"))
        end)

        it("should convert 'finished' to 6", function()
            assert.equals(6, StatusConverter.koreaderToKindle("finished"))
        end)

        it("should convert 'in-progress' to 6", function()
            assert.equals(6, StatusConverter.koreaderToKindle("in-progress"))
        end)

        it("should convert unknown status to 6", function()
            assert.equals(6, StatusConverter.koreaderToKindle("abandoned"))
            assert.equals(6, StatusConverter.koreaderToKindle("on-hold"))
        end)

        it("should convert nil to 6", function()
            assert.equals(6, StatusConverter.koreaderToKindle(nil))
        end)

        it("should convert empty string to 6", function()
            assert.equals(6, StatusConverter.koreaderToKindle(""))
        end)
    end)

    describe("round-trip consistency", function()
        it("should round-trip reading status consistently", function()
            -- KOReader 'reading' → Kindle 6 → KOReader 'reading'
            local kindle_val = StatusConverter.koreaderToKindle("reading")
            assert.equals("reading", StatusConverter.kindleToKoreader(kindle_val))
        end)

        it("should round-trip unopened status consistently", function()
            -- Kindle 0 → KOReader '' → Kindle 6 (unopened → something = opened)
            -- This is expected: KOReader opening a book makes it 'reading'
            local kr_status = StatusConverter.kindleToKoreader(0)
            assert.equals("", kr_status)
        end)
    end)
end)
