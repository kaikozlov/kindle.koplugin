-- Tests for FileChooserExt module

describe("FileChooserExt", function()
    local FileChooserExt
    local VirtualLibrary

    setup(function()
        require("spec/helper")
        FileChooserExt = require("lua/filechooser_ext")
        VirtualLibrary = require("lua/virtual_library")
    end)

    before_each(function()
        package.loaded["lua/filechooser_ext"] = nil
        package.loaded["lua/virtual_library"] = nil
        FileChooserExt = require("lua/filechooser_ext")
        VirtualLibrary = require("lua/virtual_library")
        resetAllMocks()
    end)

    describe("initialization", function()
        it("should initialize with virtual_library and cache_manager", function()
            local ext = FileChooserExt
            local mock_vl = {}
            local mock_cm = {}

            ext:init(mock_vl, mock_cm)

            assert.equals(mock_vl, ext.virtual_library)
            assert.equals(mock_cm, ext.cache_manager)
        end)
    end)

    describe("apply", function()
        local mock_virtual_library
        local mock_filechooser

        before_each(function()
            mock_virtual_library = {
                isActive = function() return true end,
                isVirtualPath = function(self, path)
                    return type(path) == "string" and path:match("^KINDLE_VIRTUAL://") ~= nil
                end,
                VIRTUAL_LIBRARY_NAME = "Kindle Library",
                VIRTUAL_PATH_PREFIX = "KINDLE_VIRTUAL://",
                _file_chooser_bypass_active = false,
                getBookEntries = function()
                    return {
                        { text = "Test Book", path = "KINDLE_VIRTUAL://b1/Book.epub", is_file = true,
                          attr = { size = 100 }, kindle_open_mode = "convert" },
                    }
                end,
                getBook = function() return nil end,
                getBlockedReasonText = function() return "blocked" end,
                createVirtualFolderEntry = function(self, parent)
                    return {
                        text = "Kindle Library/",
                        path = parent .. "/Kindle Library",
                        is_kindle_virtual_folder = true,
                    }
                end,
                buildMappings = function() end,
                buildPathMappings = function() end,
            }

            mock_filechooser = {
                init = function() end,
                changeToPath = function() end,
                refreshPath = function() end,
                genItemTable = function() return {} end,
                onMenuSelect = function() return false end,
                onMenuHold = function() return false end,
                switchItemTable = function() end,
            }

            FileChooserExt:init(mock_virtual_library, { getCacheDir = function() return "/cache" end })
        end)

        it("should patch FileChooser methods", function()
            FileChooserExt:apply(mock_filechooser)

            assert.is_function(mock_filechooser.showKindleVirtualLibrary)
            assert.is_not_nil(FileChooserExt.original_methods.init)
            assert.is_not_nil(FileChooserExt.original_methods.changeToPath)
        end)

        describe("changeToPath", function()
            it("should redirect to virtual library when returning from a virtual book", function()
                FileChooserExt:apply(mock_filechooser)

                -- Simulate returning from a virtual library book
                mock_virtual_library._return_to_virtual_pending = true

                local redirected = false
                mock_filechooser.showKindleVirtualLibrary = function()
                    redirected = true
                end

                mock_filechooser:changeToPath("/cache")

                assert.is_true(redirected)
                assert.is_false(mock_virtual_library._return_to_virtual_pending)
            end)

            it("should NOT redirect when user explicitly navigates to cache dir", function()
                FileChooserExt:apply(mock_filechooser)

                -- No pending return — user is browsing explicitly
                mock_virtual_library._return_to_virtual_pending = false

                local redirected = false
                mock_filechooser.showKindleVirtualLibrary = function()
                    redirected = true
                end

                mock_filechooser:changeToPath("/cache")

                assert.is_false(redirected)
            end)

            it("should redirect to virtual library for KINDLE_VIRTUAL:// paths", function()
                FileChooserExt:apply(mock_filechooser)

                local redirected = false
                mock_filechooser.showKindleVirtualLibrary = function()
                    redirected = true
                end

                mock_filechooser:changeToPath("KINDLE_VIRTUAL://")

                assert.is_true(redirected)
            end)
        end)

        describe("genItemTable", function()
            it("should inject virtual folder entry at root", function()
                G_reader_settings:saveSetting("home_dir", "/mnt/us")

                FileChooserExt:apply(mock_filechooser)

                local item_table = mock_filechooser:genItemTable({}, {}, "/mnt/us")

                -- Should have the virtual folder entry
                local found = false
                for _, item in ipairs(item_table) do
                    if item.is_kindle_virtual_folder then
                        found = true
                    end
                end
                assert.is_true(found)
            end)

            it("should not inject virtual folder at non-home paths", function()
                G_reader_settings:saveSetting("home_dir", "/mnt/us")

                FileChooserExt:apply(mock_filechooser)

                local item_table = mock_filechooser:genItemTable({}, {}, "/some/other/path")

                local found = false
                for _, item in ipairs(item_table) do
                    if item.is_kindle_virtual_folder then
                        found = true
                    end
                end
                assert.is_false(found)
            end)

            it("should respect bypass flag", function()
                G_reader_settings:saveSetting("home_dir", "/mnt/us")
                mock_virtual_library._file_chooser_bypass_active = true

                FileChooserExt:apply(mock_filechooser)

                local item_table = mock_filechooser:genItemTable({}, {}, "/mnt/us")

                local found = false
                for _, item in ipairs(item_table) do
                    if item.is_kindle_virtual_folder then
                        found = true
                    end
                end
                assert.is_false(found)
            end)
        end)

        describe("onMenuSelect", function()
            it("should handle virtual folder click", function()
                FileChooserExt:apply(mock_filechooser)

                local item = { is_kindle_virtual_folder = true }

                local result = mock_filechooser:onMenuSelect(item)

                assert.is_true(result)
            end)

            it("should delegate non-virtual items to original", function()
                FileChooserExt:apply(mock_filechooser)

                local item = { path = "/regular/path.epub" }

                local result = mock_filechooser:onMenuSelect(item)

                assert.is_false(result)
            end)
        end)

        describe("onMenuHold", function()
            it("should handle virtual path items", function()
                FileChooserExt:apply(mock_filechooser)

                local item = { path = "KINDLE_VIRTUAL://b1/Book.epub" }

                local result = mock_filechooser:onMenuHold(item)

                assert.is_true(result)
            end)

            it("should delegate non-virtual items to original", function()
                FileChooserExt:apply(mock_filechooser)

                local item = { path = "/regular/path.epub" }

                local result = mock_filechooser:onMenuHold(item)

                assert.is_false(result)
            end)
        end)

        describe("showKindleVirtualLibrary", function()
            it("should set path and populate book entries", function()
                FileChooserExt:apply(mock_filechooser)
                mock_filechooser.path = ""

                mock_filechooser.last_book_entries = nil
                mock_filechooser.switchItemTable = function(self, arg1, entries, arg3, arg4, arg5)
                    self.last_book_entries = entries
                end

                mock_filechooser:showKindleVirtualLibrary()

                assert.equals("KINDLE_VIRTUAL://", mock_filechooser.path)
                assert.is_not_nil(mock_filechooser.last_book_entries)
                -- Should have back entry + 1 book
                assert.is_true(#mock_filechooser.last_book_entries >= 2)
            end)

            it("should not add back entry when locked at home pointing to virtual", function()
                G_reader_settings:saveSetting("home_dir", "KINDLE_VIRTUAL://")
                G_reader_settings:saveSetting("lock_home_folder", true)

                FileChooserExt:apply(mock_filechooser)
                mock_filechooser.last_book_entries = nil
                mock_filechooser.switchItemTable = function(self, arg1, entries)
                    self.last_book_entries = entries
                end

                mock_filechooser:showKindleVirtualLibrary()

                -- Should have only the book entry, no back entry
                local has_go_up = false
                for _, item in ipairs(mock_filechooser.last_book_entries or {}) do
                    if item.is_go_up then has_go_up = true end
                end
                assert.is_false(has_go_up)

                G_reader_settings._settings = {}
            end)
        end)
    end)

    describe("unapply", function()
        it("should restore original methods", function()
            local orig_genItemTable = function() return {"original"} end
            local mock_fc = { genItemTable = orig_genItemTable }

            local mock_vl = {
                _file_chooser_bypass_active = false,
                VIRTUAL_PATH_PREFIX = "KINDLE_VIRTUAL://",
            }

            FileChooserExt:init(mock_vl, { getCacheDir = function() return "/cache" end })
            FileChooserExt:apply(mock_fc)

            assert.is_not.equals(orig_genItemTable, mock_fc.genItemTable)

            FileChooserExt:unapply(mock_fc)

            assert.equals(orig_genItemTable, mock_fc.genItemTable)
            assert.is_nil(mock_fc.showKindleVirtualLibrary)
        end)
    end)
end)
