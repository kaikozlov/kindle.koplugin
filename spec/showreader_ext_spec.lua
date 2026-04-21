-- Tests for ShowReaderExt module
-- Covers: initialization, showReader interception, blocked books,
-- PULL sync before open, alias registration, direct mode bypass.

describe("ShowReaderExt", function()
    local ShowReaderExt
    local readerui_module
    local original_showReader_calls

    setup(function()
        require("spec/helper")

        -- Mock ReaderUI module with a trackable showReader
        original_showReader_calls = {}
        package.preload["apps/reader/readerui"] = function()
            return {
                showReader = function(self, file, provider, seamless, is_provider_forced, after_open_callback)
                    table.insert(original_showReader_calls, {
                        file = file,
                        provider = provider,
                    })
                    return true
                end,
            }
        end

        -- Mock credocument provider
        package.preload["document/credocument"] = function()
            return { is_cre = true }
        end
    end)

    before_each(function()
        package.loaded["lua/showreader_ext"] = nil
        package.loaded["apps/reader/readerui"] = nil
        original_showReader_calls = {}
        ShowReaderExt = require("lua/showreader_ext")
        readerui_module = require("apps/reader/readerui")
    end)

    describe("initialization", function()
        it("should create instance with init", function()
            ShowReaderExt:init({}, {})
            assert.is_not_nil(ShowReaderExt.virtual_library)
            assert.is_not_nil(ShowReaderExt.reading_state_sync)
        end)
    end)

    describe("showReader interception", function()
        local function createMockVirtualLibrary()
            return {
                isVirtualPath = function(self, path)
                    return path and path:match("^KINDLE_VIRTUAL://") ~= nil
                end,
                getBook = function(self, virtual_path)
                    if virtual_path == "KINDLE_VIRTUAL://B001/book.kfx" then
                        return {
                            open_mode = "convert",
                            source_path = "/mnt/us/documents/book_B001.kfx",
                        }
                    end
                    return nil
                end,
                resolveBookPath = function(self, book)
                    if book.open_mode == "blocked" then return nil, "unsupported_layout" end
                    return "/cache/book.epub"
                end,
                getBlockedReasonText = function(self, book)
                    return "Book blocked: " .. (book.block_reason or "unknown")
                end,
                registerOpenAlias = function(self, real_file, virtual_path) end,
            }
        end

        it("should pass through non-virtual paths to original showReader", function()
            ShowReaderExt:init(createMockVirtualLibrary(), nil)
            ShowReaderExt:apply()

            readerui_module:showReader("/mnt/us/documents/real.epub")
            assert.equals(1, #original_showReader_calls)
            assert.equals("/mnt/us/documents/real.epub", original_showReader_calls[1].file)

            ShowReaderExt:unapply()
        end)

        it("should show error for book not found", function()
            ShowReaderExt:init(createMockVirtualLibrary(), nil)
            ShowReaderExt:apply()

            readerui_module:showReader("KINDLE_VIRTUAL://NONEXISTENT/book.epub")

            local UIManager = require("ui/uimanager")
            assert.is_true(#UIManager._show_calls > 0)

            ShowReaderExt:unapply()
        end)

        it("should show error for blocked books", function()
            local mock_vlib = createMockVirtualLibrary()
            mock_vlib.getBook = function(self, path)
                return { open_mode = "blocked", block_reason = "unsupported_kfx_layout" }
            end

            ShowReaderExt:init(mock_vlib, nil)
            ShowReaderExt:apply()

            readerui_module:showReader("KINDLE_VIRTUAL://B001/book.kfx")

            local UIManager = require("ui/uimanager")
            assert.is_true(#UIManager._show_calls > 0)

            ShowReaderExt:unapply()
        end)

        it("should resolve virtual path and delegate to original showReader", function()
            ShowReaderExt:init(createMockVirtualLibrary(), nil)
            ShowReaderExt:apply()

            readerui_module:showReader("KINDLE_VIRTUAL://B001/book.kfx")
            assert.equals(1, #original_showReader_calls)
            assert.equals("/cache/book.epub", original_showReader_calls[1].file)

            ShowReaderExt:unapply()
        end)

        it("should trigger PULL sync when sync is enabled", function()
            local sync_tracker = { called = false }
            local mock_sync = {
                isEnabled = function() return true end,
                extractCdeKey = function(self, path)
                    return path:match("^KINDLE_VIRTUAL://([A-Z0-9]+)/")
                end,
                syncFromKindle = function(self, cde_key, source_path, doc_settings)
                    sync_tracker.called = true
                    sync_tracker.cde_key = cde_key
                    sync_tracker.source_path = source_path
                    return true
                end,
            }

            -- Mock DocSettings before requiring
            package.loaded["docsettings"] = nil
            package.preload["docsettings"] = function()
                return {
                    open = function(self, path)
                        return {
                            readSetting = function() return nil end,
                            saveSetting = function() end,
                            flush = function() end,
                        }
                    end,
                }
            end

            ShowReaderExt:init(createMockVirtualLibrary(), mock_sync)
            ShowReaderExt:apply()

            readerui_module:showReader("KINDLE_VIRTUAL://B001/book.kfx")

            assert.is_true(sync_tracker.called)
            assert.equals("B001", sync_tracker.cde_key)
            assert.equals("/mnt/us/documents/book_B001.kfx", sync_tracker.source_path)

            ShowReaderExt:unapply()
        end)

        it("should not trigger sync when disabled", function()
            local sync_tracker = { called = false }
            local mock_sync = {
                isEnabled = function() return false end,
                syncFromKindle = function()
                    sync_tracker.called = true
                    return false
                end,
            }

            ShowReaderExt:init(createMockVirtualLibrary(), mock_sync)
            ShowReaderExt:apply()

            readerui_module:showReader("KINDLE_VIRTUAL://B001/book.kfx")

            assert.is_false(sync_tracker.called)

            ShowReaderExt:unapply()
        end)

        it("should register open alias for resolved book", function()
            local alias_registered = false
            local alias_real = nil
            local alias_virtual = nil
            local mock_vlib = createMockVirtualLibrary()
            mock_vlib.registerOpenAlias = function(self, real, virtual)
                alias_registered = true
                alias_real = real
                alias_virtual = virtual
            end

            ShowReaderExt:init(mock_vlib, nil)
            ShowReaderExt:apply()

            readerui_module:showReader("KINDLE_VIRTUAL://B001/book.kfx")

            assert.is_true(alias_registered)
            assert.equals("/cache/book.epub", alias_real)
            assert.equals("KINDLE_VIRTUAL://B001/book.kfx", alias_virtual)

            ShowReaderExt:unapply()
        end)

        it("should unapply and restore original showReader", function()
            local original = readerui_module.showReader
            ShowReaderExt:init(createMockVirtualLibrary(), nil)
            ShowReaderExt:apply()

            assert.is_not.equals(original, readerui_module.showReader)

            ShowReaderExt:unapply()
            assert.equals(original, readerui_module.showReader)
        end)
    end)
end)
