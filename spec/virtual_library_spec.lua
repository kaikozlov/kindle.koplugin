require("busted.runner")()
local helper = require("spec/test_helper")

describe("VirtualLibrary real-path model", function()
    local VirtualLibrary

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/virtual_library"] = nil
        VirtualLibrary = require("lua/virtual_library")
    end)

    it("indexes source and deterministic cache paths without exposing virtual files", function()
        local books = {
            {
                id = "b1",
                display_name = "Book One",
                source_path = "/documents/book.kfx",
                open_mode = "convert",
                logical_ext = "epub",
            },
        }
        local vlib = VirtualLibrary:new({ getBooks = function() return books end })
        vlib:setCacheManager({
            getCachePaths = function() return "/cache/b1.epub", "/cache/b1.json" end,
        })

        vlib:buildMappings(false)

        assert.equals(books[1], vlib:getBook("b1"))
        assert.equals(books[1], vlib:getBook("/documents/book.kfx"))
        assert.equals(books[1], vlib:getBook("/cache/b1.epub"))
        assert.is_nil(books[1].virtual_path)
    end)

    it("lazily rebuilds mappings for a cold-start cached EPUB", function()
        local calls = 0
        local books = {
            { id = "b1", source_path = "/documents/book.kfx", open_mode = "convert" },
        }
        local vlib = VirtualLibrary:new({
            getBooks = function()
                calls = calls + 1
                return books
            end,
        })
        vlib:setSettings({ cache_dir = "/cache" })
        vlib:setCacheManager({ getCachePaths = function() return "/cache/b1.epub" end })

        assert.equals(books[1], vlib:getBook("/cache/b1.epub"))
        assert.equals(1, calls)
    end)

    it("does not scan the Kindle library for an unrelated KOReader path", function()
        local calls = 0
        local vlib = VirtualLibrary:new({
            getBooks = function()
                calls = calls + 1
                return {}
            end,
        })
        vlib:setSettings({
            cache_dir = "/cache",
            documents_root = "/mnt/us/documents",
        })

        assert.is_nil(vlib:getBook("/mnt/us/books/unrelated.epub"))
        assert.equals(0, calls)
    end)

    it("does not repeat a failed lazy mapping probe until an explicit refresh", function()
        local calls = 0
        local vlib = VirtualLibrary:new({
            getBooks = function()
                calls = calls + 1
                return nil, "scan failed"
            end,
        })
        vlib:setSettings({ cache_dir = "/cache" })
        vlib:setCacheManager({ getCachePaths = function() return "/cache/book.epub" end })

        assert.is_nil(vlib:getBook("/cache/book.epub"))
        assert.is_nil(vlib:getBook("/cache/book.epub"))
        assert.equals(1, calls)

        local books, err = vlib:refresh(true)
        assert.is_nil(books)
        assert.equals("scan failed", err)
        assert.equals(2, calls)
    end)

    it("keeps cloud-only catalog entries without using nil as a path key", function()
        local book = {
            id = "cc:cloud",
            source_path = nil,
            open_mode = "blocked",
            block_reason = "missing_source",
        }
        local vlib = VirtualLibrary:new({ getBooks = function() return { book } end })
        local result = vlib:buildMappings(false)
        assert.equals(1, #result)
        assert.equals(book, vlib:getBook("cc:cloud"))
        assert.is_nil(vlib:getRealPath("KINDLE_VIRTUAL://cc:cloud/Cloud.epub"))
    end)

    it("uses KINDLE_VIRTUAL only as a legacy migration identifier", function()
        local book = { id = "b1", display_name = "A/B", logical_ext = "epub" }
        local vlib = VirtualLibrary:new({})
        local legacy = vlib:generateVirtualPath(book)
        assert.equals("KINDLE_VIRTUAL://b1/A B.epub", legacy)
        assert.is_true(vlib:isVirtualPath(legacy))
        assert.equals("b1", vlib:getBookId(legacy))
    end)

    it("creates a synthetic folder entry whose path remains a real directory", function()
        local vlib = VirtualLibrary:new({})
        vlib:setSettings({})
        local entry = vlib:createVirtualFolderEntry("/mnt/us")
        assert.is_true(entry.is_kindle_library_folder)
        assert.equals("/mnt/us", entry.path)
        assert.equals("directory", entry.attr.mode)
    end)

    it("returns direct paths unchanged and converts only on explicit open", function()
        local direct = { id = "pdf", source_path = "/documents/book.pdf", open_mode = "direct" }
        local convert = { id = "kfx", source_path = "/documents/book.kfx", open_mode = "convert" }
        local conversions = 0
        local vlib = VirtualLibrary:new({ getBooks = function() return { direct, convert } end })
        vlib:setCacheManager({
            isFresh = function(_, book)
                if book.id == "kfx" then return false, "/cache/kfx.epub" end
                return false
            end,
            getCachePaths = function(_, book) return "/cache/" .. book.id .. ".epub" end,
            ensureCachedEpub = function()
                conversions = conversions + 1
                return "/cache/kfx.epub"
            end,
        })
        vlib:buildMappings(false)

        assert.equals("/documents/book.pdf", vlib:resolveBookPath(direct))
        assert.equals(0, conversions)
        assert.equals("/cache/kfx.epub", vlib:resolveBookPath(convert))
        assert.equals(1, conversions)
    end)
end)
