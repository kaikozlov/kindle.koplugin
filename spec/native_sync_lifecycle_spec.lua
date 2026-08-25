require("busted.runner")()
local helper = require("spec/test_helper")

describe("native KOReader sync lifecycle", function()
    local LibraryIndex
    local ReadingStateSync
    local UIManager
    local original_get_books
    local original_pull
    local original_push
    local original_verify
    local original_show
    local original_next_tick
    local instance

    local SYNC_DIRECTION = {
        PROMPT = 1,
        SILENT = 2,
        NEVER = 3,
    }

    setup(function()
        helper.setup_complete()
        LibraryIndex = require("lua/library_index")
        ReadingStateSync = require("lua/reading_state_sync")
        UIManager = require("ui/uimanager")
        original_get_books = LibraryIndex.getBooks
        original_pull = ReadingStateSync.syncFromKindleAutomatic
        original_push = ReadingStateSync.syncToKindleAutomatic
        original_verify = ReadingStateSync.verifyOpenedKOReaderPosition
        original_show = UIManager.show
        original_next_tick = UIManager.nextTick
    end)

    before_each(function()
        helper.before_each()
        package.loaded["main"] = nil
        instance = nil
    end)

    after_each(function()
        if instance then
            pcall(function()
                instance:stopPlugin()
            end)
        end
        LibraryIndex.getBooks = original_get_books
        ReadingStateSync.syncFromKindleAutomatic = original_pull
        ReadingStateSync.syncToKindleAutomatic = original_push
        ReadingStateSync.verifyOpenedKOReaderPosition = original_verify
        UIManager.show = original_show
        UIManager.nextTick = original_next_tick
    end)

    local function fakeSettings(initial)
        local data = initial or {}
        local flushes = 0
        return {
            data = { doc_path = "/cache/book.epub" },
            readSetting = function(_, key)
                return data[key]
            end,
            saveSetting = function(_, key, value)
                data[key] = value
            end,
            flush = function()
                flushes = flushes + 1
            end,
            _data = data,
            _flushes = function()
                return flushes
            end,
        }
    end

    local function buildReaderPlugin(book, document_path, doc_settings, overrides)
        LibraryIndex.getBooks = function()
            return { book }
        end
        local settings = {
            enable_virtual_library = true,
            sync_reading_state = true,
            enable_auto_sync = true,
            cache_dir = "/cache",
        }
        for key, value in pairs(overrides or {}) do
            settings[key] = value
        end
        G_reader_settings:saveSetting("kindle_plugin", settings)
        local KindlePlugin = require("main")
        local ui = {
            document = { file = document_path },
            doc_settings = doc_settings,
            menu = { registerToMainMenu = function() end },
        }
        -- Normal ReaderUI teardown has dialog == self. That is the path where
        -- final SaveSettings occurs after CloseDocument.
        ui.dialog = ui
        instance = KindlePlugin:new({ ui = ui })
        return instance
    end

    it("pulls silently in DocSettingsLoad without prematurely flushing a sidecar", function()
        local book = {
            id = "book",
            cde_key = "B000000001",
            source_path = "/documents/book.kfx",
            open_mode = "convert",
        }
        local settings = fakeSettings({ last_xpointer = "old" })
        local pull_args
        ReadingStateSync.syncFromKindleAutomatic = function(_, cde_key, source_path, ds, epub_path, approval_handler, conflict_handler)
            pull_args = {
                cde_key,
                source_path,
                ds,
                epub_path,
                approval_handler,
                conflict_handler,
            }
            ds:saveSetting("last_xpointer", "synced")
            return true
        end

        buildReaderPlugin(book, "/cache/book.epub", settings)
        instance:onDocSettingsLoad(settings, instance.ui.document)

        assert.equals("B000000001", pull_args[1])
        assert.equals("/documents/book.kfx", pull_args[2])
        assert.equals(settings, pull_args[3])
        assert.equals("/cache/book.epub", pull_args[4])
        assert.is_function(pull_args[5])
        assert.is_function(pull_args[6])
        assert.equals("synced", settings:readSetting("last_xpointer"))
        assert.equals(0, settings._flushes())
    end)

    it("acknowledges a staged exact pull only at ReaderReady", function()
        local book = {
            id = "book",
            cde_key = "B000000001",
            source_path = "/documents/book.kfx",
            open_mode = "convert",
        }
        local settings = fakeSettings({ last_xpointer = "native-xpointer" })
        local verified
        ReadingStateSync.verifyOpenedKOReaderPosition = function(_, reader, epub_path)
            verified = { reader = reader, epub_path = epub_path }
            return true
        end

        buildReaderPlugin(book, "/cache/book.epub", settings)
        instance:onReaderReady()

        assert.equals(instance.ui, verified.reader)
        assert.equals("/cache/book.epub", verified.epub_path)
    end)

    it("stages last_page for a silent pull into a paging document", function()
        local book = {
            id = "pdf",
            cde_key = "B000000002",
            source_path = "/mnt/us/documents/book.pdf",
            open_mode = "direct",
        }
        local settings = fakeSettings({ percent_finished = 0.2, last_page = 20 })
        ReadingStateSync.syncFromKindleAutomatic = function(_, _, _, ds)
            ds:saveSetting("percent_finished", 0.6)
            return true
        end

        buildReaderPlugin(book, book.source_path, settings)
        instance.ui.paging = { number_of_pages = 200 }
        instance:onDocSettingsLoad(settings, instance.ui.document)

        assert.equals(0.6, settings:readSetting("percent_finished"))
        assert.equals(120, settings:readSetting("last_page"))
        assert.equals(0, settings._flushes())
    end)

    it("defers a PROMPT pull until the asynchronous answer, then moves the live reader", function()
        local book = {
            id = "book",
            cde_key = "B000000001",
            source_path = "/documents/book.kfx",
            open_mode = "convert",
        }
        local settings = fakeSettings({ last_xpointer = "old", percent_finished = 0.2 })
        local confirm
        local next_tick
        UIManager.show = function(_, widget)
            confirm = widget
        end
        UIManager.nextTick = function(_, callback)
            next_tick = callback
        end
        local live_xpointer

        ReadingStateSync.syncFromKindleAutomatic = function(_, _, _, ds, _, approval_handler)
            return approval_handler(instance, SYNC_DIRECTION, true, true, function()
                ds:saveSetting("last_xpointer", "native-xpointer")
                ds:saveSetting("percent_finished", 0.6)
            end, { book_title = "Book", source_percent = 60, dest_percent = 20 })
        end

        buildReaderPlugin(book, "/cache/book.epub", settings, {
            enable_sync_from_kindle = true,
            sync_from_kindle_newer = SYNC_DIRECTION.PROMPT,
        })
        instance.ui.rolling = {
            onGotoXPointer = function(_, xpointer)
                live_xpointer = xpointer
            end,
            onGotoPercent = function()
                error("exact pull should use xpointer")
            end,
        }

        instance:onDocSettingsLoad(settings, instance.ui.document)
        assert.is_nil(confirm)
        assert.is_function(next_tick)
        assert.equals("old", settings:readSetting("last_xpointer"))
        assert.is_nil(live_xpointer)

        next_tick()
        assert.is_truthy(confirm)
        confirm.ok_callback()
        assert.equals("native-xpointer", settings:readSetting("last_xpointer"))
        assert.equals("native-xpointer", live_xpointer)
    end)

    it("always defers an exact conflict prompt and applies the explicit Kindle choice live", function()
        local book = {
            id = "book",
            cde_key = "B000000001",
            source_path = "/documents/book.kfx",
            open_mode = "convert",
        }
        local settings = fakeSettings({ last_xpointer = "koreader-xpointer", percent_finished = 0.52 })
        local conflict_dialog
        local next_tick
        local live_xpointer
        local verified = 0
        UIManager.show = function(_, widget)
            conflict_dialog = widget
        end
        UIManager.nextTick = function(_, callback)
            next_tick = callback
        end
        ReadingStateSync.verifyOpenedKOReaderPosition = function()
            verified = verified + 1
            return true
        end
        ReadingStateSync.syncFromKindleAutomatic = function(_, _, _, ds, _, _, conflict_handler)
            return conflict_handler({
                book_title = "Book",
                kindle_percent = 38,
                koreader_percent = 52,
            }, function()
                ds:saveSetting("last_xpointer", "kindle-xpointer")
                return true
            end, function()
                error("KOReader choice should not run")
            end)
        end

        buildReaderPlugin(book, "/cache/book.epub", settings)
        instance.ui.rolling = {
            onGotoXPointer = function(_, xpointer)
                live_xpointer = xpointer
            end,
        }

        instance:onDocSettingsLoad(settings, instance.ui.document)
        assert.is_nil(conflict_dialog)
        assert.is_function(next_tick)
        assert.equals("koreader-xpointer", settings:readSetting("last_xpointer"))

        next_tick()
        assert.is_truthy(conflict_dialog)
        assert.is_truthy(conflict_dialog.text:find("Kindle: 38.0%%"))
        assert.is_truthy(conflict_dialog.text:find("KOReader: 52.0%%"))
        conflict_dialog.choice1_callback()

        assert.equals("kindle-xpointer", live_xpointer)
        assert.equals(1, verified)
    end)

    it("moves a live paging document after an approved PROMPT pull", function()
        local book = {
            id = "pdf",
            cde_key = "B000000002",
            source_path = "/mnt/us/documents/book.pdf",
            open_mode = "direct",
        }
        local settings = fakeSettings({ percent_finished = 0.2, last_page = 20 })
        local confirm
        local next_tick
        local goto_percent
        UIManager.show = function(_, widget)
            confirm = widget
        end
        UIManager.nextTick = function(_, callback)
            next_tick = callback
        end

        ReadingStateSync.syncFromKindleAutomatic = function(_, _, _, ds, _, approval_handler)
            return approval_handler(instance, SYNC_DIRECTION, true, true, function()
                ds:saveSetting("percent_finished", 0.6)
            end, { book_title = "PDF", source_percent = 60, dest_percent = 20 })
        end

        buildReaderPlugin(book, book.source_path, settings, {
            enable_sync_from_kindle = true,
            sync_from_kindle_newer = SYNC_DIRECTION.PROMPT,
        })
        instance.ui.paging = {
            number_of_pages = 200,
            onGotoPercent = function(_, percent)
                goto_percent = percent
            end,
        }

        instance:onDocSettingsLoad(settings, instance.ui.document)
        assert.is_function(next_tick)
        assert.is_nil(goto_percent)
        next_tick()
        assert.is_truthy(confirm)
        confirm.ok_callback()

        assert.equals(60, goto_percent)
        assert.equals(0.6, settings:readSetting("percent_finished"))
        assert.equals(0, settings._flushes())
    end)

    local function captureDeferredSync()
        local scheduled
        local original_schedule = UIManager.scheduleIn
        UIManager.scheduleIn = function(_, _delay, callback)
            scheduled = callback
        end
        return function()
            assert.is_function(scheduled, "close sync must be deferred")
            scheduled()
            UIManager.scheduleIn = original_schedule
        end
    end

    it("pushes only after ReaderRolling's final SaveSettings in normal teardown", function()
        local book = {
            id = "book",
            cde_key = "B000000001",
            source_path = "/documents/book.kfx",
            open_mode = "convert",
        }
        local settings = fakeSettings({ last_xpointer = "stale", percent_finished = 0.2 })
        local seen
        ReadingStateSync.syncToKindleAutomatic = function(_, cde_key, source_path, ds, epub_path)
            seen = {
                cde_key = cde_key,
                source_path = source_path,
                xpointer = ds:readSetting("last_xpointer"),
                percent = ds:readSetting("percent_finished"),
                epub_path = epub_path,
            }
            return true
        end

        local run_deferred = captureDeferredSync()

        buildReaderPlugin(book, "/cache/book.epub", settings)
        instance:onCloseDocument()
        assert.is_nil(seen)

        -- ReaderRolling:onSaveSettings runs before plugin onSaveSettings.
        settings:saveSetting("last_xpointer", "final-xpointer")
        settings:saveSetting("percent_finished", 0.73)
        instance:onSaveSettings()
        run_deferred()
        assert.equals("B000000001", seen.cde_key)
        assert.equals("/documents/book.kfx", seen.source_path)
        assert.equals("final-xpointer", seen.xpointer)
        assert.equals(0.73, seen.percent)
        assert.equals("/cache/book.epub", seen.epub_path)
    end)

    it("defers a mandatory close-time conflict prompt until after final settings are captured", function()
        local book = {
            id = "book",
            cde_key = "B000000001",
            source_path = "/documents/book.kfx",
            open_mode = "convert",
        }
        local settings = fakeSettings({ last_xpointer = "stale", percent_finished = 0.2 })
        local conflict_dialog
        local next_tick
        local selected
        UIManager.show = function(_, widget)
            conflict_dialog = widget
        end
        UIManager.nextTick = function(_, callback)
            next_tick = callback
        end

        ReadingStateSync.syncToKindleAutomatic = function(_, _, _, ds, _, _, conflict_handler)
            return conflict_handler({
                book_title = "Book",
                kindle_percent = 38,
                koreader_percent = 73,
            }, function()
                selected = "kindle"
            end, function()
                selected = ds:readSetting("last_xpointer")
                return true
            end)
        end

        buildReaderPlugin(book, "/cache/book.epub", settings)
        instance:onCloseDocument()
        settings:saveSetting("last_xpointer", "final-xpointer")
        settings:saveSetting("percent_finished", 0.73)
        local run_deferred = captureDeferredSync()
        instance:onSaveSettings()
        run_deferred()

        assert.is_nil(conflict_dialog)
        assert.is_function(next_tick)
        assert.is_nil(selected)
        next_tick()
        assert.is_truthy(conflict_dialog)
        assert.is_truthy(conflict_dialog.text:find("Kindle: 38.0%%"))
        assert.is_truthy(conflict_dialog.text:find("KOReader: 73.0%%"))
        conflict_dialog.choice2_callback()
        assert.equals("final-xpointer", selected)
    end)

    it("forces close-time PROMPT approval asynchronous after final settings are captured", function()
        local book = {
            id = "book",
            cde_key = "B000000001",
            source_path = "/documents/book.kfx",
            open_mode = "convert",
        }
        local settings = fakeSettings({ last_xpointer = "stale", percent_finished = 0.2 })
        local confirm
        local next_tick
        local pushed_xpointer
        UIManager.show = function(_, widget)
            confirm = widget
        end
        UIManager.nextTick = function(_, callback)
            next_tick = callback
        end

        ReadingStateSync.syncToKindleAutomatic = function(_, _, _, ds, _, approval_handler)
            return approval_handler(instance, SYNC_DIRECTION, false, true, function()
                pushed_xpointer = ds:readSetting("last_xpointer")
            end, { book_title = "Book", source_percent = 70, dest_percent = 20 })
        end

        buildReaderPlugin(book, "/cache/book.epub", settings, {
            enable_sync_to_kindle = true,
            sync_to_kindle_newer = SYNC_DIRECTION.PROMPT,
        })
        instance:onCloseDocument()
        settings:saveSetting("last_xpointer", "final-xpointer")
        local run_deferred = captureDeferredSync()
        instance:onSaveSettings()
        run_deferred()

        assert.is_nil(confirm)
        assert.is_function(next_tick)
        assert.is_nil(pushed_xpointer)
        next_tick()
        assert.is_truthy(confirm)
        confirm.ok_callback()
        assert.equals("final-xpointer", pushed_xpointer)
    end)
end)
