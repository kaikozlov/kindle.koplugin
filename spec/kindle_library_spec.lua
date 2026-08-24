require("busted.runner")()
local helper = require("spec/test_helper")

describe("KindleLibrary native BookList opens", function()
    local filemanagerutil
    local original_open_file

    setup(function()
        helper.setup_complete()
        filemanagerutil = require("apps/filemanager/filemanagerutil")
        original_open_file = filemanagerutil.openFile
    end)

    before_each(function()
        helper.before_each()
    end)

    after_each(function()
        filemanagerutil.openFile = original_open_file
    end)

    local function makeManager(book)
        local vlib = {
            getBook = function(_, id) return id == book.id and book or nil end,
            getBlockedReasonText = function(_, blocked)
                return blocked and blocked.block_reason or "blocked"
            end,
        }
        local KindleLibrary = require("lua/kindle_library")
        local manager = KindleLibrary:new(vlib, {})
        manager:setUI({ marker = "filemanager" })
        return manager
    end

    it("requests a direct PDF by its real source path", function()
        local book = {
            id = "pdf",
            source_path = "/mnt/us/documents/book.pdf",
            open_mode = "direct",
        }
        local manager = makeManager(book)
        local opened_path
        filemanagerutil.openFile = function(_, path)
            opened_path = path
        end

        assert.is_true(manager:openItem({ kindle_book_id = "pdf" }))
        assert.equals(book.source_path, opened_path)

        local provider = require("document/documentregistry"):getProvider(opened_path)
        assert.is_truthy(provider)
        assert.equals("mupdf", provider.provider)
    end)

    it("does not convert a KFX while the BookList is deciding what to open", function()
        local book = {
            id = "kfx",
            source_path = "/mnt/us/documents/book.kfx",
            open_mode = "convert",
        }
        local manager = makeManager(book)
        local opened_path
        filemanagerutil.openFile = function(_, path)
            opened_path = path
        end

        manager:openItem({ kindle_book_id = "kfx" })
        assert.equals(book.source_path, opened_path)
        -- open_file_ext owns preparation after KOReader's optional confirmation.
    end)

    it("requests a library return only when native open begins", function()
        local book = {
            id = "kfx",
            source_path = "/mnt/us/documents/book.kfx",
            open_mode = "convert",
        }
        local manager = makeManager(book)
        manager.ui.file_chooser = { path = "/mnt/us" }
        local before_open
        local closed = 0
        manager.booklist_menu = {
            close_callback = function()
                closed = closed + 1
            end,
        }
        filemanagerutil.openFile = function(_, _, callback)
            before_open = callback
        end

        manager:openItem({ kindle_book_id = "kfx" })

        assert.is_function(before_open)
        assert.is_nil(manager:takeReturnToLibraryRequest())
        before_open()
        assert.equals(1, closed)
        assert.same({
            origin_path = "/mnt/us",
        }, manager:takeReturnToLibraryRequest())
        assert.is_nil(manager:takeReturnToLibraryRequest())
    end)

    it("never forwards a blocked/cloud-only entry to KOReader", function()
        local book = {
            id = "cloud",
            source_path = nil,
            open_mode = "blocked",
            block_reason = "missing_source",
        }
        local manager = makeManager(book)
        local opened = false
        filemanagerutil.openFile = function()
            opened = true
        end

        assert.is_true(manager:openItem({ kindle_book_id = "cloud" }))
        assert.is_false(opened)
    end)
end)
