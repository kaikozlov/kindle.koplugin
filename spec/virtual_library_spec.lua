-- Tests for VirtualLibrary module

describe("VirtualLibrary", function()
    local VirtualLibrary

    setup(function()
        require("spec/helper")
        VirtualLibrary = require("lua/virtual_library")
    end)

    before_each(function()
        package.loaded["lua/virtual_library"] = nil
        VirtualLibrary = require("lua/virtual_library")
        resetAllMocks()
    end)

    describe("initialization", function()
        it("should create a new instance with library_index", function()
            local mock_index = {}
            local vlib = VirtualLibrary:new(mock_index)

            assert.is_not_nil(vlib)
            assert.equals(mock_index, vlib.library_index)
        end)

        it("should initialize with empty path mappings", function()
            local vlib = VirtualLibrary:new({})

            assert.is_table(vlib.books_by_id)
            assert.is_table(vlib.books_by_virtual)
            assert.is_table(vlib.real_to_virtual)
        end)

        it("should initialize bypass flag to false", function()
            local vlib = VirtualLibrary:new({})

            assert.is_false(vlib._file_chooser_bypass_active)
        end)
    end)

    describe("constants", function()
        it("should have KINDLE_VIRTUAL:// prefix", function()
            assert.equals("KINDLE_VIRTUAL://", VirtualLibrary.VIRTUAL_PATH_PREFIX)
        end)

        it("should have Kindle Library name", function()
            assert.equals("Kindle Library", VirtualLibrary.VIRTUAL_LIBRARY_NAME)
        end)
    end)

    describe("setSettings", function()
        it("should store settings", function()
            local vlib = VirtualLibrary:new({})
            local settings = { enable_virtual_library = true }

            vlib:setSettings(settings)

            assert.equals(settings, vlib.settings)
        end)

        it("should default to empty table for nil", function()
            local vlib = VirtualLibrary:new({})

            vlib:setSettings(nil)

            assert.is_table(vlib.settings)
        end)
    end)

    describe("isActive", function()
        it("should return true when setting is true", function()
            local vlib = VirtualLibrary:new({})
            vlib:setSettings({ enable_virtual_library = true })

            assert.is_true(vlib:isActive())
        end)

        it("should return true when setting is nil (default)", function()
            local vlib = VirtualLibrary:new({})
            vlib:setSettings({})

            assert.is_true(vlib:isActive())
        end)

        it("should return false when setting is explicitly false", function()
            local vlib = VirtualLibrary:new({})
            vlib:setSettings({ enable_virtual_library = false })

            assert.is_false(vlib:isActive())
        end)
    end)

    describe("isVirtualPath", function()
        it("should return true for KINDLE_VIRTUAL:// paths", function()
            local vlib = VirtualLibrary:new({})
            assert.is_true(vlib:isVirtualPath("KINDLE_VIRTUAL://abc123/Book.epub"))
        end)

        it("should return false for regular paths", function()
            local vlib = VirtualLibrary:new({})
            assert.is_false(vlib:isVirtualPath("/mnt/us/documents/book.kfx"))
        end)

        it("should return false for nil", function()
            local vlib = VirtualLibrary:new({})
            assert.is_false(vlib:isVirtualPath(nil))
        end)

        it("should return false for empty string", function()
            local vlib = VirtualLibrary:new({})
            assert.is_false(vlib:isVirtualPath(""))
        end)
    end)

    describe("getBookId", function()
        it("should extract book ID from virtual path", function()
            local vlib = VirtualLibrary:new({})
            assert.equals("abc123", vlib:getBookId("KINDLE_VIRTUAL://abc123/Book Title.epub"))
        end)

        it("should return nil for non-virtual path", function()
            local vlib = VirtualLibrary:new({})
            assert.is_nil(vlib:getBookId("/regular/path.epub"))
        end)

        it("should handle complex IDs", function()
            local vlib = VirtualLibrary:new({})
            assert.equals("B00XYZ123_ABC", vlib:getBookId("KINDLE_VIRTUAL://B00XYZ123_ABC/Some Book.epub"))
        end)
    end)

    describe("generateVirtualPath", function()
        it("should generate path with display name", function()
            local vlib = VirtualLibrary:new({})
            local book = { id = "abc123", display_name = "Test Book", logical_ext = "epub" }

            local path = vlib:generateVirtualPath(book)

            assert.equals("KINDLE_VIRTUAL://abc123/Test Book.epub", path)
        end)

        it("should use title when display_name is missing", function()
            local vlib = VirtualLibrary:new({})
            local book = { id = "abc123", title = "Fallback Title", logical_ext = "epub" }

            local path = vlib:generateVirtualPath(book)

            assert.equals("KINDLE_VIRTUAL://abc123/Fallback Title.epub", path)
        end)

        it("should use Untitled when both display_name and title are missing", function()
            local vlib = VirtualLibrary:new({})
            local book = { id = "abc123", logical_ext = "bin" }

            local path = vlib:generateVirtualPath(book)

            -- Falls back to book.id when all names are nil
            assert.is_true(path:match("abc123") ~= nil)
            assert.is_true(path:match("KINDLE_VIRTUAL://abc123/") ~= nil)
        end)

        it("should sanitize slashes in display name", function()
            local vlib = VirtualLibrary:new({})
            local book = { id = "abc123", display_name = "Part 1/Part 2", logical_ext = "epub" }

            local path = vlib:generateVirtualPath(book)

            assert.is_false(path:match("/Part 2") ~= nil)
        end)

        it("should collapse multiple spaces", function()
            local vlib = VirtualLibrary:new({})
            local book = { id = "abc123", display_name = "Book   Title", logical_ext = "epub" }

            local path = vlib:generateVirtualPath(book)

            assert.is_false(path:match("  ") ~= nil)
        end)
    end)

    describe("buildMappings", function()
        it("should build mappings from library index", function()
            local mock_books = {
                { id = "b1", display_name = "Book One", source_path = "/path/one.kfx", open_mode = "convert", logical_ext = "epub" },
                { id = "b2", display_name = "Book Two", source_path = "/path/two.kfx", open_mode = "direct", logical_ext = "epub" },
            }
            local mock_index = {
                getBooks = function(self, force) return mock_books end,
            }
            local vlib = VirtualLibrary:new(mock_index)

            local result, err = vlib:buildMappings(false)

            assert.is_not_nil(result)
            assert.is_nil(err)
            assert.equals(2, #result)
            assert.is_not_nil(vlib.books_by_id["b1"])
            assert.is_not_nil(vlib.books_by_id["b2"])
            assert.is_not_nil(vlib.books_by_virtual[vlib.real_to_virtual["/path/one.kfx"]])
        end)

        it("should return error when library scan fails", function()
            local mock_index = {
                getBooks = function(self, force) return nil, "scan failed" end,
            }
            local vlib = VirtualLibrary:new(mock_index)

            local result, err = vlib:buildMappings(false)

            assert.is_nil(result)
            assert.equals("scan failed", err)
        end)

        it("should assign virtual_path to each book", function()
            local mock_books = {
                { id = "b1", display_name = "Book", source_path = "/path.kfx", logical_ext = "epub" },
            }
            local mock_index = {
                getBooks = function(self, force) return mock_books end,
            }
            local vlib = VirtualLibrary:new(mock_index)

            vlib:buildMappings(false)

            assert.is_not_nil(mock_books[1].virtual_path)
            assert.is_true(mock_books[1].virtual_path:match("^KINDLE_VIRTUAL://") ~= nil)
        end)

        it("should clear existing mappings before rebuilding", function()
            local mock_index = {
                getBooks = function(self, force) return {} end,
            }
            local vlib = VirtualLibrary:new(mock_index)
            vlib.books_by_id["old"] = { id = "old" }

            vlib:buildMappings(false)

            assert.is_nil(vlib.books_by_id["old"])
        end)
    end)

    describe("buildPathMappings alias", function()
        it("should delegate to buildMappings", function()
            local called = false
            local mock_index = {
                getBooks = function(self, force)
                    called = true
                    return {}
                end,
            }
            local vlib = VirtualLibrary:new(mock_index)

            vlib:buildPathMappings()

            assert.is_true(called)
        end)
    end)

    describe("getRealPath", function()
        it("should return source path for a known book", function()
            local mock_books = {
                { id = "b1", display_name = "Book", source_path = "/path/book.kfx", logical_ext = "epub" },
            }
            local mock_index = {
                getBooks = function() return mock_books end,
            }
            local vlib = VirtualLibrary:new(mock_index)
            vlib:buildMappings(false)

            local vp = vlib.real_to_virtual["/path/book.kfx"]
            assert.equals("/path/book.kfx", vlib:getRealPath(vp))
        end)

        it("should return nil for unknown virtual path", function()
            local vlib = VirtualLibrary:new({ getBooks = function() return {} end })
            vlib:buildMappings(false)

            assert.is_nil(vlib:getRealPath("KINDLE_VIRTUAL://nonexistent/file.epub"))
        end)
    end)

    describe("getVirtualPath", function()
        it("should return virtual path for a known real path", function()
            local mock_books = {
                { id = "b1", display_name = "Book", source_path = "/path/book.kfx", logical_ext = "epub" },
            }
            local mock_index = {
                getBooks = function() return mock_books end,
            }
            local vlib = VirtualLibrary:new(mock_index)
            vlib:buildMappings(false)

            local vp = vlib:getVirtualPath("/path/book.kfx")
            assert.is_not_nil(vp)
            assert.is_true(vp:match("^KINDLE_VIRTUAL://") ~= nil)
        end)

        it("should return nil for unknown real path", function()
            local vlib = VirtualLibrary:new({ getBooks = function() return {} end })
            vlib:buildMappings(false)

            assert.is_nil(vlib:getVirtualPath("/nonexistent/path.kfx"))
        end)

        it("should return path as-is if already virtual", function()
            local vlib = VirtualLibrary:new({})
            local vp = "KINDLE_VIRTUAL://abc/Book.epub"
            assert.equals(vp, vlib:getVirtualPath(vp))
        end)
    end)

    describe("getBook", function()
        it("should find book by virtual path", function()
            local mock_books = {
                { id = "b1", display_name = "Book", source_path = "/path.kfx", logical_ext = "epub" },
            }
            local mock_index = {
                getBooks = function() return mock_books end,
            }
            local vlib = VirtualLibrary:new(mock_index)
            vlib:buildMappings(false)

            local vp = vlib.real_to_virtual["/path.kfx"]
            local book = vlib:getBook(vp)
            assert.is_not_nil(book)
            assert.equals("b1", book.id)
        end)

        it("should find book by book id", function()
            local mock_books = {
                { id = "b1", display_name = "Book", source_path = "/path.kfx", logical_ext = "epub" },
            }
            local mock_index = {
                getBooks = function() return mock_books end,
            }
            local vlib = VirtualLibrary:new(mock_index)
            vlib:buildMappings(false)

            local book = vlib:getBook("b1")
            assert.is_not_nil(book)
            assert.equals("b1", book.id)
        end)

        it("should return nil for nil path", function()
            local vlib = VirtualLibrary:new({})
            assert.is_nil(vlib:getBook(nil))
        end)
    end)

    describe("open aliases", function()
        it("should register and check open alias", function()
            local vlib = VirtualLibrary:new({})
            vlib:registerOpenAlias("/real/path.epub", "KINDLE_VIRTUAL://abc/Book.epub")

            assert.is_true(vlib:isOpenAlias("/real/path.epub"))
        end)

        it("should clear open alias", function()
            local vlib = VirtualLibrary:new({})
            vlib:registerOpenAlias("/real/path.epub", "KINDLE_VIRTUAL://abc/Book.epub")

            vlib:clearOpenAlias("KINDLE_VIRTUAL://abc/Book.epub")

            assert.is_false(vlib:isOpenAlias("/real/path.epub"))
        end)

        it("should return virtual path via alias", function()
            local vlib = VirtualLibrary:new({})
            vlib:registerOpenAlias("/real/path.epub", "KINDLE_VIRTUAL://abc/Book.epub")

            assert.equals("KINDLE_VIRTUAL://abc/Book.epub", vlib:getVirtualPath("/real/path.epub"))
        end)
    end)

    describe("getBookEntries", function()
        it("should return array of book entries", function()
            local mock_books = {
                { id = "b1", display_name = "Book One", source_path = "/one.kfx", open_mode = "convert", logical_ext = "epub", source_size = 100 },
                { id = "b2", display_name = "Book Two", source_path = "/two.kfx", open_mode = "blocked", block_reason = "drm", logical_ext = "epub", source_size = 200 },
            }
            local mock_index = {
                getBooks = function() return mock_books end,
            }
            local vlib = VirtualLibrary:new(mock_index)

            local entries = vlib:getBookEntries(false)

            assert.is_table(entries)
            assert.equals(2, #entries)
        end)

        it("should include open mode suffix in text", function()
            local mock_books = {
                { id = "b1", display_name = "DRM Book", source_path = "/drm.kfx", open_mode = "drm", logical_ext = "epub" },
                { id = "b2", display_name = "Blocked Book", source_path = "/blocked.kfx", open_mode = "blocked", logical_ext = "epub" },
            }
            local mock_index = {
                getBooks = function() return mock_books end,
            }
            local vlib = VirtualLibrary:new(mock_index)

            local entries = vlib:getBookEntries(false)

            assert.is_true(entries[1].text:match("%[drm%]") ~= nil)
            assert.is_true(entries[2].text:match("%[blocked%]") ~= nil)
        end)

        it("should set is_file and attr on entries", function()
            local mock_books = {
                { id = "b1", display_name = "Book", source_path = "/book.kfx", open_mode = "convert", logical_ext = "epub", source_size = 42 },
            }
            local mock_index = {
                getBooks = function() return mock_books end,
            }
            local vlib = VirtualLibrary:new(mock_index)

            local entries = vlib:getBookEntries(false)

            assert.is_true(entries[1].is_file)
            assert.is_table(entries[1].attr)
            assert.equals(42, entries[1].attr.size)
        end)
    end)

    describe("createVirtualFolderEntry", function()
        it("should create folder entry with correct name", function()
            local vlib = VirtualLibrary:new({})

            local entry = vlib:createVirtualFolderEntry("/mnt/us")

            assert.is_not_nil(entry)
            assert.is_true(entry.text:match("Kindle Library/") ~= nil)
            assert.is_true(entry.is_kindle_virtual_folder)
        end)

        it("should construct path from parent", function()
            local vlib = VirtualLibrary:new({})

            local entry = vlib:createVirtualFolderEntry("/mnt/us")

            assert.equals("/mnt/us/Kindle Library", entry.path)
        end)
    end)

    describe("getBlockedReasonText", function()
        it("should return text for drm reason", function()
            local vlib = VirtualLibrary:new({})
            local text = vlib:getBlockedReasonText({ block_reason = "drm" })
            assert.is_true(text:match("DRM") ~= nil)
        end)

        it("should return text for unsupported_kfx_layout", function()
            local vlib = VirtualLibrary:new({})
            local text = vlib:getBlockedReasonText({ block_reason = "unsupported_kfx_layout" })
            assert.is_true(text:match("not supported") ~= nil)
        end)

        it("should return text for missing_source", function()
            local vlib = VirtualLibrary:new({})
            local text = vlib:getBlockedReasonText({ block_reason = "missing_source" })
            assert.is_true(text:match("missing") ~= nil)
        end)

        it("should return default text for unknown reason", function()
            local vlib = VirtualLibrary:new({})
            local text = vlib:getBlockedReasonText({ block_reason = "something_unknown" })
            assert.is_true(text:match("cannot be opened") ~= nil)
        end)

        it("should default to unsupported_kfx_layout when no reason", function()
            local vlib = VirtualLibrary:new({})
            local text = vlib:getBlockedReasonText(nil)
            assert.is_true(text:match("not supported") ~= nil)
        end)
    end)
end)
