require("busted.runner")()
local helper = require("spec/test_helper")

describe("OpenFileExt real-path cache refresh", function()
    local filemanagerutil
    local Trapper
    local original_open_file
    local original_wrap
    local original_info
    local original_clear
    local UIManager
    local original_show
    local OpenFileExt

    setup(function()
        helper.setup_complete()
        filemanagerutil = require("apps/filemanager/filemanagerutil")
        Trapper = require("ui/trapper")
        original_open_file = filemanagerutil.openFile
        original_wrap = Trapper.wrap
        original_info = Trapper.info
        original_clear = Trapper.clear
        UIManager = require("ui/uimanager")
        original_show = UIManager.show
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/open_file_ext"] = nil
        OpenFileExt = require("lua/open_file_ext")
        Trapper.wrap = function(_, fn) return fn() end
        Trapper.info = function() return true end
        Trapper.clear = function() end
    end)

    after_each(function()
        pcall(function() OpenFileExt:unapply() end)
        filemanagerutil.openFile = original_open_file
        Trapper.wrap = original_wrap
        Trapper.info = original_info
        Trapper.clear = original_clear
        UIManager.show = original_show
    end)

    it("recreates a missing persisted cached EPUB before KOReader opens it", function()
        local book = {
            id = "book",
            source_path = "/documents/book.kfx",
            open_mode = "convert",
            display_name = "Book",
        }
        local refreshes = 0
        local resolves = 0
        local migrations = 0
        local virtual_library = {
            getBook = function(_, path)
                if path == "/cache/book.epub" or path == book.source_path then
                    return book
                end
            end,
            refresh = function()
                refreshes = refreshes + 1
                return { book }
            end,
            resolveBookPath = function()
                resolves = resolves + 1
                return "/cache/book.epub"
            end,
            getBlockedReasonText = function() return "blocked" end,
        }
        local cache_manager = {
            getCachePaths = function() return "/cache/book.epub", "/cache/book.json" end,
            isFresh = function() return false, "/cache/book.epub", "/cache/book.json" end,
        }
        local migration = {
            migrate = function(_, migrated_book, path)
                assert.equals(book, migrated_book)
                assert.equals("/cache/book.epub", path)
                migrations = migrations + 1
            end,
        }
        local delegated
        local callback = function() end
        filemanagerutil.openFile = function(ui, path, pre_callback, no_dialog)
            delegated = { ui, path, pre_callback, no_dialog }
            return "delegated"
        end

        OpenFileExt:init(virtual_library, cache_manager, migration)
        OpenFileExt:apply()
        local ui = { marker = true }
        local result = filemanagerutil.openFile(ui, "/cache/book.epub", callback, true)

        assert.equals("delegated", result)
        assert.same({ ui, "/cache/book.epub", callback, true }, delegated)
        assert.equals(1, refreshes)
        assert.equals(1, resolves)
        assert.equals(1, migrations)
    end)

    it("trusts CacheManager source stat checks without rescanning an existing source", function()
        local source_path = "/tmp/kindle-open-file-ext-source.kfx"
        local source = assert(io.open(source_path, "wb"))
        source:write("kfx")
        source:close()

        local book = {
            id = "book",
            source_path = source_path,
            open_mode = "convert",
        }
        local refreshes = 0
        local virtual_library = {
            getBook = function() return book end,
            refresh = function()
                refreshes = refreshes + 1
                return { book }
            end,
            resolveBookPath = function(_, resolved_book)
                assert.equals(book, resolved_book)
                return "/cache/book.epub"
            end,
            getBlockedReasonText = function() return "blocked" end,
        }
        local checked_book
        local cache_manager = {
            getCachePaths = function() return "/cache/book.epub" end,
            isFresh = function(_, candidate)
                checked_book = candidate
                return false, "/cache/book.epub"
            end,
        }
        filemanagerutil.openFile = function(_, path) return path end

        OpenFileExt:init(virtual_library, cache_manager, nil)
        OpenFileExt:apply()
        assert.equals("/cache/book.epub", filemanagerutil.openFile({}, "/cache/book.epub"))
        assert.equals(book, checked_book)
        assert.equals(0, refreshes)
        os.remove(source_path)
    end)

    it("leaves direct PDFs and native provider selection untouched", function()
        local book = {
            id = "pdf",
            source_path = "/documents/book.pdf",
            open_mode = "direct",
        }
        local virtual_library = {
            getBook = function(_, path) return path == book.source_path and book or nil end,
        }
        local cache_calls = 0
        local cache_manager = {
            getCachePaths = function() cache_calls = cache_calls + 1 end,
            isFresh = function() cache_calls = cache_calls + 1 end,
        }
        local delegated
        filemanagerutil.openFile = function(_, path)
            delegated = path
            return path
        end

        OpenFileExt:init(virtual_library, cache_manager, nil)
        OpenFileExt:apply()
        assert.equals(book.source_path, filemanagerutil.openFile({}, book.source_path))
        assert.equals(book.source_path, delegated)
        assert.equals(0, cache_calls)

        local provider = require("document/documentregistry"):getProvider(delegated)
        assert.equals("mupdf", provider.provider)
    end)

    it("waits for KOReader's open confirmation before preparing", function()
        local book = {
            id = "book",
            source_path = "/documents/book.kfx",
            open_mode = "convert",
            display_name = "Book",
        }
        local resolves = 0
        local virtual_library = {
            getBook = function(_, path)
                if path == book.source_path or path == "/cache/book.epub" then
                    return book
                end
            end,
            isActive = function() return true end,
            refresh = function() return { book } end,
            resolveBookPath = function()
                resolves = resolves + 1
                return "/cache/book.epub"
            end,
            getBlockedReasonText = function() return "blocked" end,
        }
        local cache_manager = {
            getCachePaths = function() return "/cache/book.epub" end,
            isFresh = function() return false, "/cache/book.epub" end,
        }
        local delegated
        filemanagerutil.openFile = function(_, path)
            delegated = path
        end
        local confirm
        UIManager.show = function(_, widget)
            confirm = widget
        end
        G_reader_settings:saveSetting("file_ask_to_open", true)

        OpenFileExt:init(virtual_library, cache_manager, nil)
        OpenFileExt:apply()
        filemanagerutil.openFile({}, book.source_path)

        assert.is_truthy(confirm)
        assert.equals(0, resolves)
        assert.is_nil(delegated)
        confirm.ok_callback()
        assert.equals(1, resolves)
        assert.equals("/cache/book.epub", delegated)
    end)

    it("does not convert a source KFX when the virtual library is disabled", function()
        local source_path = "/tmp/kindle-open-file-disabled.kfx"
        local source = assert(io.open(source_path, "wb"))
        source:write("kfx")
        source:close()
        local book = {
            id = "book",
            source_path = source_path,
            open_mode = "convert",
        }
        local resolves = 0
        local virtual_library = {
            getBook = function(_, path) return path == source_path and book or nil end,
            isActive = function() return false end,
            getBlockedReasonText = function() return "blocked" end,
        }
        local cache_manager = {
            getCachePaths = function() return "/cache/book.epub" end,
            isFresh = function() error("disabled source open must not inspect conversion cache") end,
        }
        virtual_library.resolveBookPath = function()
            resolves = resolves + 1
            return "/cache/book.epub"
        end
        local delegated
        filemanagerutil.openFile = function(_, path)
            delegated = path
            return path
        end

        OpenFileExt:init(virtual_library, cache_manager, nil)
        OpenFileExt:apply()
        assert.equals(source_path, filemanagerutil.openFile({}, source_path, nil, true))
        assert.equals(source_path, delegated)
        assert.equals(0, resolves)
        os.remove(source_path)
    end)

    it("restores the original KOReader open function on stop", function()
        local sentinel = function() return "original" end
        filemanagerutil.openFile = sentinel
        OpenFileExt:init({ getBook = function() end }, {}, nil)
        OpenFileExt:apply()
        assert.not_equals(sentinel, filemanagerutil.openFile)
        OpenFileExt:unapply()
        assert.equals(sentinel, filemanagerutil.openFile)
    end)
end)
