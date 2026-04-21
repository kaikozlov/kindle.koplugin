-- Tests for LibraryIndex module

describe("LibraryIndex", function()
    local LibraryIndex

    setup(function()
        require("spec/helper")
        LibraryIndex = require("lua/library_index")
    end)

    before_each(function()
        package.loaded["lua/library_index"] = nil
        LibraryIndex = require("lua/library_index")
        resetAllMocks()
    end)

    describe("initialization", function()
        it("should create instance with helper_client", function()
            local mock_client = {}
            local idx = LibraryIndex:new(mock_client)

            assert.is_not_nil(idx)
            assert.equals(mock_client, idx.helper_client)
        end)

        it("should start with empty books and zero loaded_at", function()
            local idx = LibraryIndex:new({})

            assert.is_nil(idx.books)
            assert.equals(0, idx.loaded_at)
        end)
    end)

    describe("setSettings", function()
        it("should store settings", function()
            local idx = LibraryIndex:new({})
            local settings = { documents_root = "/test/docs" }

            idx:setSettings(settings)

            assert.equals(settings, idx.settings)
        end)
    end)

    describe("refresh", function()
        it("should call helper_client:scan with documents root", function()
            local scanned_root = nil
            local mock_client = {
                scan = function(self, root)
                    scanned_root = root
                    return { books = {
                        { id = "b1", display_name = "Book" },
                    } }
                end,
            }
            local idx = LibraryIndex:new(mock_client)
            idx:setSettings({ documents_root = "/test/docs", index_ttl_seconds = 0 })

            local books = idx:refresh(true)

            assert.is_not_nil(books)
            assert.equals(1, #books)
            assert.equals("/test/docs", scanned_root)
        end)

        it("should return cached books when within TTL", function()
            local scan_called = false
            local mock_client = {
                scan = function(self, root)
                    scan_called = true
                    return { books = { { id = "b1" } } }
                end,
            }
            local idx = LibraryIndex:new(mock_client)
            idx:setSettings({ documents_root = "/test/docs", index_ttl_seconds = 300 })

            -- First call populates cache
            idx:refresh(true)
            scan_called = false

            -- Second call should use cache (not forced)
            local books = idx:refresh(false)

            assert.is_false(scan_called)
            assert.is_not_nil(books)
        end)

        it("should return error when scan fails", function()
            local mock_client = {
                scan = function(self, root)
                    return nil, "scan error"
                end,
            }
            local idx = LibraryIndex:new(mock_client)
            idx:setSettings({ documents_root = "/test/docs", index_ttl_seconds = 0 })

            local books, err = idx:refresh(true)

            assert.is_nil(books)
            assert.equals("scan error", err)
        end)

        it("should sort books alphabetically by display name", function()
            local mock_client = {
                scan = function(self, root)
                    return { books = {
                        { id = "b2", display_name = "Zebra Book" },
                        { id = "b1", display_name = "Alpha Book" },
                        { id = "b3", display_name = "Middle Book" },
                    } }
                end,
            }
            local idx = LibraryIndex:new(mock_client)
            idx:setSettings({ documents_root = "/test/docs", index_ttl_seconds = 0 })

            local books = idx:refresh(true)

            assert.equals("Alpha Book", books[1].display_name)
            assert.equals("Middle Book", books[2].display_name)
            assert.equals("Zebra Book", books[3].display_name)
        end)

        it("should reject invalid books payload", function()
            local mock_client = {
                scan = function(self, root)
                    return { not_books = {} }
                end,
            }
            local idx = LibraryIndex:new(mock_client)
            idx:setSettings({ documents_root = "/test/docs", index_ttl_seconds = 0 })

            local books, err = idx:refresh(true)

            assert.is_nil(books)
            assert.is_string(err)
        end)
    end)

    describe("getBooks", function()
        it("should delegate to refresh", function()
            local mock_client = {
                scan = function(self, root)
                    return { books = { { id = "b1" } } }
                end,
            }
            local idx = LibraryIndex:new(mock_client)
            idx:setSettings({ documents_root = "/test/docs", index_ttl_seconds = 0 })

            local books = idx:getBooks(true)

            assert.is_not_nil(books)
            assert.equals(1, #books)
        end)
    end)
end)
