-- Tests for LibraryIndex module

require("busted.runner")()
local helper = require("spec/test_helper")

describe("LibraryIndex", function()
    local LibraryIndex
    local CatalogDb
    local lfs
    local SQ3

    setup(function()
        helper.setup_complete()
        LibraryIndex = require("lua/library_index")
        CatalogDb = require("lua/lib/kindle_catalog_db")
        lfs = require("libs/libkoreader-lfs")
    end)

    before_each(function()
        package.loaded["lua/library_index"] = nil
        LibraryIndex = require("lua/library_index")
        helper.before_each()
        SQ3 = helper.install_sqlite_mock()
        CatalogDb._test_icu_installer = function()
            return true, function() end
        end
        -- cc.db present by default; individual tests opt out.
        lfs._setFileState("/var/local/cc.db", { exists = true, mode = "file" })
    end)

    after_each(function()
        CatalogDb._test_icu_installer = nil
        lfs._clearFileStates()
        SQ3._reset()
    end)

    -- Minimal columnar result for one catalog row.
    local function mockCatalogRows(names)
        local columns = {
            p_uuid = {},
            p_location = {},
            p_titles_0_nominal = {},
            j_credits = {},
            p_mimeType = {},
            p_cdeKey = {},
            p_isDRMProtected = {},
            p_diskUsage = {},
            p_contentSize = {},
        }
        for i, name in ipairs(names) do
            for column, values in pairs(columns) do
                values[i] = column == "p_titles_0_nominal" and name or ""
            end
        end
        return columns, #names
    end

    describe("refresh", function()
        it("scans visible catalog entries through cc.db", function()
            SQ3._setMockResults(mockCatalogRows({ "Alpha Book", "Beta Book" }))

            local idx = LibraryIndex:new()
            idx:setSettings({ index_ttl_seconds = 0 })

            local books = idx:refresh(true)

            assert.is_not_nil(books)
            assert.equals(2, #books)
            assert.equals("Alpha Book", books[1].display_name)
            assert.equals("Beta Book", books[2].display_name)
        end)

        it("preserves Kindle catalog order instead of re-sorting display titles", function()
            local idx = LibraryIndex:new({
                isAvailable = function()
                    return true
                end,
                scan = function()
                    return {
                        { display_name = "The Familiars", sort_key = "Familiars The" },
                        { display_name = "Elvis and the Underdogs", sort_key = "Elvis and the Underdogs" },
                    }
                end,
            })

            local books = assert(idx:refresh(true))

            assert.equals("The Familiars", books[1].display_name)
            assert.equals("Elvis and the Underdogs", books[2].display_name)
        end)

        it("should return cached books when within TTL", function()
            local scans = 0
            local idx = LibraryIndex:new({
                isAvailable = function()
                    return true
                end,
                scan = function()
                    scans = scans + 1
                    return { { id = "b1" } }
                end,
            })
            idx:setSettings({ index_ttl_seconds = 300 })

            idx:refresh(true)
            assert.equals(1, scans)

            local books = idx:refresh(false)

            assert.equals(1, scans)
            assert.is_not_nil(books)
        end)

        it("fails loudly when the catalog is missing", function()
            lfs._setFileState("/var/local/cc.db", { exists = false })
            local idx = LibraryIndex:new()
            idx:setSettings({ index_ttl_seconds = 0 })

            local books, err = idx:refresh(true)

            assert.is_nil(books)
            assert.is_truthy(err:match("cc%.db"))
        end)

        it("propagates catalog scan errors", function()
            local idx = LibraryIndex:new({
                isAvailable = function()
                    return true
                end,
                scan = function()
                    return nil, "catalog locked"
                end,
            })
            idx:setSettings({ index_ttl_seconds = 0 })

            local books, err = idx:refresh(true)

            assert.is_nil(books)
            assert.equals("catalog locked", err)
        end)
    end)

    describe("getBooks", function()
        it("should delegate to refresh", function()
            SQ3._setMockResults(mockCatalogRows({ "Solo" }))
            local idx = LibraryIndex:new()
            idx:setSettings({ index_ttl_seconds = 0 })

            local books = idx:getBooks(true)

            assert.equals(1, #books)
        end)
    end)
end)
