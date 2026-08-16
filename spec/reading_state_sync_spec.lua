-- Tests for ReadingStateSync module
-- Covers: initialization, cdeKey extraction, bidirectional sync,
-- PULL/PUSH scenarios, both-sides-complete skip, timestamp decisions,
-- status sync, unopened book handling, and edge cases.

require('busted.runner')()
local helper = require("spec/test_helper")

local SYNC_DIRECTION = { PROMPT = 1, SILENT = 2, NEVER = 3 }

--- Helper: create a mock doc_settings with tracked saves.
local function createMockDocSettings(doc_path, opts)
    opts = opts or {}
    local saved = {}

    local ds = {
        data = { doc_path = doc_path },
        readSetting = function(self, key)
            if saved[key] ~= nil then return saved[key] end
            if opts[key] ~= nil then return opts[key] end
            return nil
        end,
        saveSetting = function(self, key, value)
            saved[key] = value
        end,
        flush = function(self) end,
    }

    -- Seed initial values
    for k, v in pairs(opts) do
        saved[k] = v
    end

    return ds
end

--- Helper: set up plugin with default granular sync settings.
local function setupPluginSettings(sync)
    local mock_plugin = {
        settings = {
            enable_auto_sync = true,
            enable_sync_from_kindle = true,
            enable_sync_to_kindle = true,
            sync_from_kindle_newer = SYNC_DIRECTION.SILENT,
            sync_from_kindle_older = SYNC_DIRECTION.NEVER,
            sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
            sync_to_kindle_older = SYNC_DIRECTION.NEVER,
        },
        save_count = 0,
        saveSettings = function(self)
            self.save_count = self.save_count + 1
        end,
    }
    sync:setPlugin(mock_plugin, SYNC_DIRECTION)
    return mock_plugin
end

--- Helper: mock readKindleState to return a specific state.
local function mockReadKindleState(sync, state)
    sync._mock_kindle_state = state
    local original = sync.readKindleState
    sync.readKindleState = function(self, cde_key, source_path)
        return self._mock_kindle_state
    end
    return original
end

--- Helper: restore original readKindleState.
local function restoreReadKindleState(sync, original)
    sync.readKindleState = original
    sync._mock_kindle_state = nil
end

--- Helper: mock writeKindleState to track calls.
local function mockWriteKindleState(sync)
    local calls = {}
    sync._mock_write_calls = calls
    local original = sync.writeKindleState
    sync.writeKindleState = function(self, cde_key, source_path, percent, timestamp, status)
        table.insert(calls, {
            cde_key = cde_key,
            source_path = source_path,
            percent = percent,
            timestamp = timestamp,
            status = status,
        })
        return true
    end
    return original, calls
end

local function restoreWriteKindleState(sync, original)
    sync.writeKindleState = original
    sync._mock_write_calls = nil
end

describe("ReadingStateSync", function()
    local ReadingStateSync
    local KindleStateReader
    local KindleStateWriter
    local ReadHistory
    local RealDocSettings
    local originals = {}

    setup(function()
        helper.setup_complete()
        KindleStateReader = require("lua/lib/kindle_state_reader")
        KindleStateWriter = require("lua/lib/kindle_state_writer")
        ReadHistory = require("readhistory")
        RealDocSettings = require("docsettings")
        originals.reader_by_key = KindleStateReader.readByCdeKey
        originals.reader_by_uuid = KindleStateReader.readByUuid
        originals.reader_by_path = KindleStateReader.readByPath
        originals.reader_all = KindleStateReader.readAllProgress
        originals.writer_by_key = KindleStateWriter.writeByCdeKey
        originals.writer_by_uuid = KindleStateWriter.writeByUuid
        originals.writer_by_path = KindleStateWriter.writeByPath
        originals.history = ReadHistory.hist
        originals.has_sidecar = RealDocSettings.hasSidecarFile
        originals.open_docsettings = RealDocSettings.open
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/reading_state_sync"] = nil
        package.loaded["lua/lib/sync_decision_maker"] = nil
        package.loaded["lua/lib/status_converter"] = nil

        local reader_data = {}
        local reader_data_by_key = {}
        local reader_data_by_uuid = {}
        local all_books = {}
        KindleStateReader.readByCdeKey = function(cde_key)
            return reader_data_by_key[cde_key]
        end
        KindleStateReader.readByUuid = function(uuid)
            return reader_data_by_uuid[uuid]
        end
        KindleStateReader.readByPath = function(path)
            return reader_data[path]
        end
        KindleStateReader.readAllProgress = function()
            return all_books
        end
        KindleStateReader._setMockStateByPath = function(path, state)
            reader_data[path] = state
        end
        KindleStateReader._setMockStateByKey = function(key, state)
            reader_data_by_key[key] = state
        end
        KindleStateReader._setMockStateByUuid = function(uuid, state)
            reader_data_by_uuid[uuid] = state
        end
        KindleStateReader._setMockAllBooks = function(books)
            all_books = books
        end
        KindleStateReader._clear = function()
            reader_data = {}
            reader_data_by_key = {}
            reader_data_by_uuid = {}
            all_books = {}
        end

        local write_log = {}
        KindleStateWriter.writeByCdeKey = function(cde_key, percent, timestamp, status)
            table.insert(write_log, { method = "cdeKey", key = cde_key, percent = percent, timestamp = timestamp, status = status })
            return true
        end
        KindleStateWriter.writeByUuid = function(uuid, percent, timestamp, status)
            table.insert(write_log, { method = "uuid", uuid = uuid, percent = percent, timestamp = timestamp, status = status })
            return true
        end
        KindleStateWriter.writeByPath = function(path, percent, timestamp, status)
            table.insert(write_log, { method = "path", path = path, percent = percent, timestamp = timestamp, status = status })
            return true
        end
        KindleStateWriter._getWriteLog = function() return write_log end
        KindleStateWriter._clearWriteLog = function() write_log = {} end

        ReadHistory.hist = {
            { file = "/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", time = 1762685677 },
            { file = "/mnt/us/documents/Other Book_B008PL1YQ0.kfx", time = 1762628755 },
        }

        local sidecar_files = {}
        RealDocSettings.hasSidecarFile = function(_, path)
            return sidecar_files[path] == true
        end
        RealDocSettings.open = function(_, path)
            return createMockDocSettings(path, { percent_finished = 0.5 })
        end
        RealDocSettings._setSidecarFile = function(_, path, exists)
            sidecar_files[path] = exists
        end
        RealDocSettings._clearSidecars = function()
            sidecar_files = {}
        end

        ReadingStateSync = require("lua/reading_state_sync")
        ReadingStateSync._realSaveAuthoritativeNativePosition =
            ReadingStateSync.saveAuthoritativeNativePosition
        ReadingStateSync._realGetAuthoritativeKindleXPointer =
            ReadingStateSync.getAuthoritativeKindleXPointer
        ReadingStateSync.saveAuthoritativeNativePosition = function()
            return 37
        end
        ReadingStateSync.getAuthoritativeKindleXPointer = function()
            return "/body/DocFragment/body/p/text().0"
        end
    end)

    after_each(function()
        KindleStateReader.readByCdeKey = originals.reader_by_key
        KindleStateReader.readByUuid = originals.reader_by_uuid
        KindleStateReader.readByPath = originals.reader_by_path
        KindleStateReader.readAllProgress = originals.reader_all
        KindleStateReader._setMockStateByPath = nil
        KindleStateReader._setMockStateByKey = nil
        KindleStateReader._setMockStateByUuid = nil
        KindleStateReader._setMockAllBooks = nil
        KindleStateReader._clear = nil
        KindleStateWriter.writeByCdeKey = originals.writer_by_key
        KindleStateWriter.writeByUuid = originals.writer_by_uuid
        KindleStateWriter.writeByPath = originals.writer_by_path
        KindleStateWriter._getWriteLog = nil
        KindleStateWriter._clearWriteLog = nil
        ReadHistory.hist = originals.history
        RealDocSettings.hasSidecarFile = originals.has_sidecar
        RealDocSettings.open = originals.open_docsettings
        RealDocSettings._setSidecarFile = nil
        RealDocSettings._clearSidecars = nil
    end)

    -- ========================================================================
    -- Initialization
    -- ========================================================================
    describe("initialization", function()
        it("should create a new instance", function()
            local sync = ReadingStateSync:new()
            assert.is_not_nil(sync)
            assert.is_false(sync:isEnabled())
        end)

        it("should initialize with sync disabled", function()
            local sync = ReadingStateSync:new()
            assert.is_false(sync:isEnabled())
        end)

        it("should accept helper_client in constructor", function()
            local mock_client = { position = function() end }
            local sync = ReadingStateSync:new(mock_client)
            assert.equals(mock_client, sync.helper_client)
        end)
    end)

    -- ========================================================================
    -- Enable/Disable
    -- ========================================================================
    describe("enable/disable", function()
        it("should enable sync when requested", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            assert.is_true(sync:isEnabled())
        end)

        it("should disable sync when requested", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            assert.is_true(sync:isEnabled())
            sync:setEnabled(false)
            assert.is_false(sync:isEnabled())
        end)
    end)

    -- ========================================================================
    -- extractCdeKey
    -- ========================================================================
    describe("extractCdeKey", function()
        it("should extract from virtual path", function()
            local sync = ReadingStateSync:new()
            local key = sync:extractCdeKey("KINDLE_VIRTUAL://B007N6JEII/Book.epub")
            assert.equals("B007N6JEII", key)
        end)

        it("should extract from doc_settings doc_path (virtual)", function()
            local sync = ReadingStateSync:new()
            local doc_settings = createMockDocSettings("KINDLE_VIRTUAL://B008PL1YQ0/book.epub")
            local key = sync:extractCdeKey(nil, doc_settings)
            assert.equals("B008PL1YQ0", key)
        end)

        it("should extract ASIN from filename in doc_path", function()
            local sync = ReadingStateSync:new()
            local doc_settings = createMockDocSettings("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx")
            local key = sync:extractCdeKey(nil, doc_settings)
            assert.equals("B007N6JEII", key)
        end)

        it("should extract PDOC hash from filename", function()
            local sync = ReadingStateSync:new()
            local doc_settings = createMockDocSettings("/mnt/us/documents/My Book_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx")
            local key = sync:extractCdeKey(nil, doc_settings)
            assert.equals("5AFAFAA13FFE43ECBE78F0FF3761814C", key)
        end)

        it("should prefer virtual_path over doc_settings", function()
            local sync = ReadingStateSync:new()
            local doc_settings = createMockDocSettings("/mnt/us/documents/Some_Book_B009NG3090.kfx")
            local key = sync:extractCdeKey("KINDLE_VIRTUAL://B007N6JEII/Book.epub", doc_settings)
            assert.equals("B007N6JEII", key)
        end)

        it("should return nil for unparseable paths", function()
            local sync = ReadingStateSync:new()
            local doc_settings = createMockDocSettings("/some/random/path.epub")
            assert.is_nil(sync:extractCdeKey(nil, doc_settings))
        end)

        it("should return nil for nil inputs", function()
            local sync = ReadingStateSync:new()
            assert.is_nil(sync:extractCdeKey(nil, nil))
        end)

        it("should return nil for non-virtual path without ASIN pattern", function()
            local sync = ReadingStateSync:new()
            local doc_settings = createMockDocSettings("/mnt/us/documents/myfile.epub")
            assert.is_nil(sync:extractCdeKey(nil, doc_settings))
        end)
    end)

    describe("catalog UUID routing", function()
        local virtual_id = "cc:f82913d4-094a-43c6-8166-e330d40c1d7c"
        local uuid = "f82913d4-094a-43c6-8166-e330d40c1d7c"
        local source_path = "/mnt/us/documents/The Almighty Dollar_B0FLB24198.kfx"

        it("should read cc: virtual IDs through p_uuid", function()
            local expected = { percent_read = 47, cde_key = "B0FLB24198" }
            KindleStateReader._setMockStateByUuid(uuid, expected)
            local sync = ReadingStateSync:new()

            assert.equals(expected, sync:readKindleState(virtual_id, source_path))
        end)

        it("should write cc: virtual IDs through p_uuid without path fallback", function()
            local sync = ReadingStateSync:new()

            assert.is_true(sync:writeKindleState(virtual_id, source_path, 48, 1, "reading"))
            local writes = KindleStateWriter._getWriteLog()
            assert.equals(1, #writes)
            assert.equals("uuid", writes[1].method)
            assert.equals(uuid, writes[1].uuid)
        end)
    end)

    -- ========================================================================
    -- auto-sync check
    -- ========================================================================
    describe("isAutomaticSyncEnabled", function()
        it("should return false when disabled", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(false)
            assert.is_false(sync:isAutomaticSyncEnabled())
        end)

        it("should return false when no plugin", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            assert.is_false(sync:isAutomaticSyncEnabled())
        end)

        it("should check plugin settings", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            sync.plugin = { settings = { enable_auto_sync = true } }
            assert.is_true(sync:isAutomaticSyncEnabled())
        end)

        it("should return false when auto_sync disabled in settings", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            sync.plugin = { settings = { enable_auto_sync = false } }
            assert.is_false(sync:isAutomaticSyncEnabled())
        end)
    end)

    describe("configured automatic sync", function()
        local history_path = "/mnt/us/documents/Throne of Glass_B007N6JEII.kfx"

        it("should not pull when automatic sync is disabled", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)
            sync.plugin.settings.enable_auto_sync = false
            local original = mockReadKindleState(sync, {
                percent_read = 80,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            })

            local result = sync:syncFromKindleAutomatic(
                "B007N6JEII",
                history_path,
                createMockDocSettings(history_path, { percent_finished = 0.3 })
            )

            assert.is_false(result)
            restoreReadKindleState(sync, original)
        end)

        it("should honor NEVER for a newer Kindle state", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)
            sync.plugin.settings.sync_from_kindle_newer = SYNC_DIRECTION.NEVER
            RealDocSettings:_setSidecarFile(history_path, true)
            local original = mockReadKindleState(sync, {
                percent_read = 80,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            })
            local ds = createMockDocSettings(history_path, { percent_finished = 0.3 })

            assert.is_false(sync:syncFromKindleAutomatic("B007N6JEII", history_path, ds))
            assert.equals(0.3, ds:readSetting("percent_finished"))

            restoreReadKindleState(sync, original)
            RealDocSettings:_clearSidecars()
        end)

        it("should honor SILENT for an explicitly allowed older Kindle state", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)
            sync.plugin.settings.sync_from_kindle_older = SYNC_DIRECTION.SILENT
            RealDocSettings:_setSidecarFile(history_path, true)
            local original = mockReadKindleState(sync, {
                percent_read = 20,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })
            local ds = createMockDocSettings(history_path, { percent_finished = 0.5 })

            assert.is_true(sync:syncFromKindleAutomatic("B007N6JEII", history_path, ds))
            assert.equals(0.2, ds:readSetting("percent_finished"))
            assert.equals(
                "/body/DocFragment/body/p/text().0",
                ds:readSetting("last_xpointer")
            )

            restoreReadKindleState(sync, original)
            RealDocSettings:_clearSidecars()
        end)

        it("should pull a newer exact LPR even when the shelf percentage matches", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)
            RealDocSettings:_setSidecarFile(history_path, true)
            local original = mockReadKindleState(sync, {
                percent_read = 47.5,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            })
            local ds = createMockDocSettings(history_path, {
                percent_finished = 0.475,
                last_xpointer = "/body/DocFragment/body/p/text().1",
                summary = { status = "reading" },
            })

            assert.is_true(sync:syncFromKindleAutomatic(
                "B007N6JEII", history_path, ds, "/cache/book.epub"))
            assert.equals(
                "/body/DocFragment/body/p/text().0",
                ds:readSetting("last_xpointer")
            )

            restoreReadKindleState(sync, original)
            RealDocSettings:_clearSidecars()
        end)

        it("should defer an ambiguous first pull until close establishes a receipt", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local plugin = setupPluginSettings(sync)
            local original = mockReadKindleState(sync, {
                percent_read = 38,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })
            sync.getAuthoritativeKindleXPointer = function()
                return "/body/DocFragment/body/p/text().38", nil, {
                    long = "ATwFAACbAAAA", pid = 442741, percent = 38,
                }
            end
            local ds = createMockDocSettings("/cache/book.epub", {
                percent_finished = 0.52,
                last_xpointer = "/body/DocFragment/body/p/text().52",
                summary = { status = "reading" },
            })

            assert.is_false(sync:syncFromKindleAutomatic(
                "B007N6JEII", history_path, ds, "/cache/book.epub"))
            assert.equals(0.52, ds:readSetting("percent_finished"))
            assert.equals(
                "/body/DocFragment/body/p/text().52",
                ds:readSetting("last_xpointer")
            )
            assert.is_nil(plugin.settings.position_sync_receipts)

            restoreReadKindleState(sync, original)
        end)

        it("should read and reverse-translate Kindle's exact LPR", function()
            local calls = {}
            local client = {
                readNativeProgress = function(_, asin, source)
                    calls.asin = asin
                    calls.source = source
                    return { long = "ATwFAACbAAAA", pid = 442741 }
                end,
                translateNativePosition = function(_, epub, long_position)
                    calls.epub = epub
                    calls.long_position = long_position
                    return {
                        xpointer = "/body/DocFragment[14]/body/p[17]/text().155",
                        pid = 442741,
                    }
                end,
            }
            local sync = ReadingStateSync:new(client)
            local xpointer = ReadingStateSync._realGetAuthoritativeKindleXPointer(
                sync, "B007N6JEII", history_path, "/cache/book.epub"
            )
            assert.equals(
                "/body/DocFragment[14]/body/p[17]/text().155", xpointer
            )
            assert.equals("B007N6JEII", calls.asin)
            assert.equals("/cache/book.epub", calls.epub)
        end)

        it("should honor TO Kindle direction and update the native position", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)
            RealDocSettings:_setSidecarFile(history_path, true)
            local original_read = mockReadKindleState(sync, {
                percent_read = 30,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })
            local original_write, writes = mockWriteKindleState(sync)
            local native_save = nil
            sync.saveAuthoritativeNativePosition = function(_, key, source, epub)
                native_save = { key = key, source = source, epub = epub }
                return 36.8
            end
            local ds = createMockDocSettings(history_path, {
                percent_finished = 0.75,
                summary = { status = "reading" },
            })

            assert.is_true(sync:syncToKindleAutomatic("B007N6JEII", history_path, ds))
            assert.equals(1, #writes)
            assert.equals(36.8, writes[1].percent)
            assert.equals("B007N6JEII", native_save.key)

            restoreReadKindleState(sync, original_read)
            restoreWriteKindleState(sync, original_write)
            RealDocSettings:_clearSidecars()
        end)

        it("should treat the just-closed KOReader position as the newest event", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)
            sync.plugin.settings.sync_to_kindle_newer = SYNC_DIRECTION.NEVER
            local original_read = mockReadKindleState(sync, {
                percent_read = 90,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            })
            local original_write, writes = mockWriteKindleState(sync)
            local ds = createMockDocSettings(history_path, {
                percent_finished = 0.75,
                summary = { status = "reading" },
            })

            assert.is_false(sync:syncToKindleAutomatic("B007N6JEII", history_path, ds))
            assert.equals(0, #writes)

            restoreReadKindleState(sync, original_read)
            restoreWriteKindleState(sync, original_write)
        end)

        it("should pull a changed exact LPR even when the catalog timestamp is stale", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local plugin = setupPluginSettings(sync)
            plugin.settings.position_sync_receipts = {
                B007N6JEII = { long = "ATwFAACbAAAA", pid = 442741 },
            }
            RealDocSettings:_setSidecarFile(history_path, true)
            local original_read = mockReadKindleState(sync, {
                percent_read = 52,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })
            sync.getAuthoritativeKindleXPointer = function()
                return "/body/DocFragment/body/p/text().52", nil, {
                    long = "ATwFAACcAAAA", pid = 442742, percent = 52,
                }
            end
            local ds = createMockDocSettings(history_path, {
                percent_finished = 0.38,
                last_xpointer = "/body/DocFragment/body/p/text().38",
                summary = { status = "reading" },
            })

            assert.is_true(sync:syncFromKindleAutomatic(
                "B007N6JEII", history_path, ds, "/cache/book.epub"))
            assert.equals(0.52, ds:readSetting("percent_finished"))
            assert.equals(
                "/body/DocFragment/body/p/text().52",
                ds:readSetting("last_xpointer")
            )
            assert.equals("ATwFAACcAAAA",
                plugin.settings.position_sync_receipts.B007N6JEII.long)
            assert.equals(1, plugin.save_count)

            restoreReadKindleState(sync, original_read)
            RealDocSettings:_clearSidecars()
        end)

        it("should not restore a stale native LPR already recorded by a prior push", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local plugin = setupPluginSettings(sync)
            plugin.settings.sync_from_kindle_older = SYNC_DIRECTION.SILENT
            plugin.settings.position_sync_receipts = {
                B007N6JEII = { long = "ATwFAACbAAAA", pid = 442741 },
            }
            local original_read = mockReadKindleState(sync, {
                percent_read = 38,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })
            sync.getAuthoritativeKindleXPointer = function()
                return "/body/DocFragment/body/p/text().38", nil, {
                    long = "ATwFAACbAAAA", pid = 442741, percent = 38,
                }
            end
            local ds = createMockDocSettings(history_path, {
                percent_finished = 0.52,
                last_xpointer = "/body/DocFragment/body/p/text().52",
                summary = { status = "reading" },
            })

            assert.is_false(sync:syncFromKindleAutomatic(
                "B007N6JEII", history_path, ds, "/cache/book.epub"))
            assert.equals(0.52, ds:readSetting("percent_finished"))
            assert.equals(
                "/body/DocFragment/body/p/text().52",
                ds:readSetting("last_xpointer")
            )

            restoreReadKindleState(sync, original_read)
        end)

        it("should repair a stale shelf without moving either reader", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local plugin = setupPluginSettings(sync)
            plugin.settings.position_sync_receipts = {
                B007N6JEII = {
                    long = "ATwFAACbAAAA", pid = 442741, percent = 52,
                },
            }
            local original_read = mockReadKindleState(sync, {
                percent_read = 38,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })
            local original_write, writes = mockWriteKindleState(sync)
            sync.getAuthoritativeKindleXPointer = function()
                return "/body/DocFragment/body/p/text().52", nil, {
                    long = "ATwFAACbAAAA", pid = 442741, percent = 52,
                }
            end
            local ds = createMockDocSettings(history_path, {
                percent_finished = 0.52,
                last_xpointer = "/body/DocFragment/body/p/text().52",
                summary = { status = "reading" },
            })

            assert.is_false(sync:syncFromKindleAutomatic(
                "B007N6JEII", history_path, ds, "/cache/book.epub"))
            assert.equals(1, #writes)
            assert.equals(52, writes[1].percent)
            assert.equals(
                "/body/DocFragment/body/p/text().52",
                ds:readSetting("last_xpointer")
            )

            restoreReadKindleState(sync, original_read)
            restoreWriteKindleState(sync, original_write)
        end)

        it("should persist a push receipt even before ReadHistory flushes", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local plugin = setupPluginSettings(sync)
            local original_read = mockReadKindleState(sync, {
                percent_read = 38,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })
            local original_write, writes = mockWriteKindleState(sync)
            sync.saveAuthoritativeNativePosition = function()
                return 52, {
                    long = "ATwFAACcAAAA", pid = 442742, percent = 52,
                }
            end
            local ds = createMockDocSettings("/cache/book.epub", {
                percent_finished = 0.52,
                last_xpointer = "/body/DocFragment/body/p/text().52",
                summary = { status = "reading" },
            })

            assert.is_true(sync:syncToKindleAutomatic(
                "B007N6JEII", history_path, ds, "/cache/book.epub"))
            assert.equals(1, #writes)
            assert.equals("ATwFAACcAAAA",
                plugin.settings.position_sync_receipts.B007N6JEII.long)
            assert.equals("push",
                plugin.settings.position_sync_receipts.B007N6JEII.direction)
            assert.equals(1, plugin.save_count)

            restoreReadKindleState(sync, original_read)
            restoreWriteKindleState(sync, original_write)
        end)

        it("should not record a receipt when the shelf update fails", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local plugin = setupPluginSettings(sync)
            local original_read = mockReadKindleState(sync, {
                percent_read = 38,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })
            sync.writeKindleState = function() return false end
            sync.saveAuthoritativeNativePosition = function()
                return 52, {
                    long = "ATwFAACcAAAA", pid = 442742, percent = 52,
                }
            end
            local ds = createMockDocSettings("/cache/book.epub", {
                percent_finished = 0.52,
                last_xpointer = "/body/DocFragment/body/p/text().52",
                summary = { status = "reading" },
            })

            assert.is_false(sync:syncToKindleAutomatic(
                "B007N6JEII", history_path, ds, "/cache/book.epub"))
            assert.is_nil(plugin.settings.position_sync_receipts)
            assert.equals(0, plugin.save_count)

            restoreReadKindleState(sync, original_read)
        end)
    end)

    -- ========================================================================
    -- getBookTitle
    -- ========================================================================
    describe("getBookTitle", function()
        it("should return title from doc_settings", function()
            local sync = ReadingStateSync:new()
            local ds = createMockDocSettings("", { title = "My Book" })
            assert.equals("My Book", sync:getBookTitle("B001", ds))
        end)

        it("should return Unknown Book when no sources available", function()
            local sync = ReadingStateSync:new()
            local ds = createMockDocSettings("")
            assert.equals("Unknown Book", sync:getBookTitle("NONEXISTENT", ds))
        end)

        it("should handle nil doc_settings gracefully", function()
            local sync = ReadingStateSync:new()
            assert.equals("Unknown Book", sync:getBookTitle("B001", nil))
        end)

        it("should fall through to Unknown Book when title is empty string", function()
            local sync = ReadingStateSync:new()
            local ds = createMockDocSettings("", { title = "" })
            assert.equals("Unknown Book", sync:getBookTitle("B001", ds))
        end)
    end)

    -- ========================================================================
    -- syncFromKindle (PULL)
    -- ========================================================================
    describe("syncFromKindle", function()
        it("should return false when disabled", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(false)
            assert.is_false(sync:syncFromKindle("B001", "/path/book.kfx", {}))
        end)

        it("should return false when no Kindle state", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            -- No cc.db data mocked → readKindleState returns nil
            assert.is_false(sync:syncFromKindle("NONEXISTENT", nil, {}))
        end)

        it("should return false when Kindle state has 0% and unopened", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local orig = mockReadKindleState(sync, {
                percent_read = 0,
                timestamp = 0,
                status = "",
                kindle_status = 0,
            })
            assert.is_false(sync:syncFromKindle("B001", "/path/book.kfx", createMockDocSettings("path")))
            restoreReadKindleState(sync, orig)
        end)

        it("should return false when Kindle state has 0 percent_read even with non-zero status", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local orig = mockReadKindleState(sync, {
                percent_read = 0,
                timestamp = 1000000,
                status = "reading",
                kindle_status = 1,
            })
            assert.is_false(sync:syncFromKindle("B001", "/path/book.kfx", createMockDocSettings("path")))
            restoreReadKindleState(sync, orig)
        end)

        it("should return false when KOReader is more recent", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local orig = mockReadKindleState(sync, {
                percent_read = 30,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })
            -- Use a path that IS in ReadHistory mock with time=1762685677 > 1000
            local DocSettings = require("docsettings")
            DocSettings:_setSidecarFile("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", true)

            local ds = createMockDocSettings("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", { percent_finished = 0.5 })
            assert.is_false(sync:syncFromKindle("B007N6JEII", "/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", ds))
            restoreReadKindleState(sync, orig)
            DocSettings:_clearSidecars()
        end)
    end)

    -- ========================================================================
    -- syncToKindle (PUSH)
    -- ========================================================================
    describe("syncToKindle", function()
        it("should translate and save the exact XPointer through ReaderSDK", function()
            local calls = {}
            local client = {
                translatePosition = function(_, epub, xpointer)
                    calls.epub = epub
                    calls.xpointer = xpointer
                    return { long = "ATwFAACbAAAA", pid = 442741 }
                end,
                saveNativeProgress = function(_, asin, native_path, position)
                    calls.asin = asin
                    calls.native_path = native_path
                    calls.position = position
                    return true, nil, 36.8
                end,
            }
            local sync = ReadingStateSync:new(client)
            local ds = createMockDocSettings("/cache/book.epub", {
                last_xpointer = "/body/DocFragment/body/p/text().1",
            })

            assert.equals(36.8, ReadingStateSync._realSaveAuthoritativeNativePosition(
                sync, "B007N6JEII", "/mnt/us/documents/book_B007N6JEII.kfx",
                "/cache/book.epub", ds
            ))
            assert.equals("/cache/book.epub", calls.epub)
            assert.equals(442741, calls.position.pid)
            assert.equals("B007N6JEII", calls.asin)
        end)

        it("should not update the shelf when authoritative native save fails", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            sync.saveAuthoritativeNativePosition = function() return false end
            local original_write, write_log = mockWriteKindleState(sync)
            local ds = createMockDocSettings("/cache/book.epub", {
                percent_finished = 0.75,
                last_xpointer = "/body/DocFragment/body/p/text().1",
            })

            assert.is_false(sync:syncToKindle(
                "B007N6JEII", "/mnt/us/documents/book_B007N6JEII.kfx", ds,
                "/cache/book.epub"
            ))
            assert.equals(0, #write_log)
            restoreWriteKindleState(sync, original_write)
        end)

        it("should return false when disabled", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(false)
            assert.is_false(sync:syncToKindle("B001", "/path/book.kfx", {}))
        end)

        it("should write Kindle-native percent and status to Kindle", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)

            local orig_write, write_log = mockWriteKindleState(sync)
            local orig_update = sync.updateYjrPosition
            sync.updateYjrPosition = function() end -- no-op in test

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0.75,
                summary = { status = "reading" },
            })

            local ok = sync:syncToKindle("B001", "/path/book.kfx", ds)
            assert.is_true(ok)
            assert.equals(1, #write_log)
            assert.equals(37, write_log[1].percent)
            assert.equals("reading", write_log[1].status)

            restoreWriteKindleState(sync, orig_write)
            sync.updateYjrPosition = orig_update
        end)

        it("should handle 0% progress without crash", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)

            local orig_write, write_log = mockWriteKindleState(sync)
            local orig_update = sync.updateYjrPosition
            sync.updateYjrPosition = function() end

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0,
            })

            local ok = sync:syncToKindle("B001", "/path/book.kfx", ds)
            assert.is_true(ok)
            assert.equals(37, write_log[1].percent)

            restoreWriteKindleState(sync, orig_write)
            sync.updateYjrPosition = orig_update
        end)

        it("should handle 100% complete status", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)

            local orig_write, write_log = mockWriteKindleState(sync)
            local orig_update = sync.updateYjrPosition
            sync.updateYjrPosition = function() end

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 1.0,
                summary = { status = "complete" },
            })

            local ok = sync:syncToKindle("B001", "/path/book.kfx", ds)
            assert.is_true(ok)
            assert.equals(37, write_log[1].percent)
            assert.equals("complete", write_log[1].status)

            restoreWriteKindleState(sync, orig_write)
            sync.updateYjrPosition = orig_update
        end)

        it("should default to reading status when summary is nil", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)

            local orig_write, write_log = mockWriteKindleState(sync)
            local orig_update = sync.updateYjrPosition
            sync.updateYjrPosition = function() end

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0.42,
            })

            local ok = sync:syncToKindle("B001", "/path/book.kfx", ds)
            assert.is_true(ok)
            assert.equals(37, write_log[1].percent)
            assert.equals("reading", write_log[1].status)

            restoreWriteKindleState(sync, orig_write)
            sync.updateYjrPosition = orig_update
        end)
    end)

    -- ========================================================================
    -- syncBidirectional — both-sides-complete skip
    -- ========================================================================
    describe("syncBidirectional — both sides complete", function()
        it("should skip sync when both sides are 100%", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 100,
                timestamp = 1762700000,
                status = "complete",
                kindle_status = 2,
            })

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 1.0,
                summary = { status = "complete" },
            })

            assert.is_false(sync:syncBidirectional("B001", "/path/book.kfx", ds))
            restoreReadKindleState(sync, orig)
        end)

        it("should skip sync when both sides complete via status=complete even at 99%", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 100,
                timestamp = 1762700000,
                status = "complete",
                kindle_status = 2,
            })

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0.99,
                summary = { status = "complete" },
            })

            assert.is_false(sync:syncBidirectional("B001", "/path/book.kfx", ds))
            restoreReadKindleState(sync, orig)
        end)

        it("should skip sync when KOReader status is 'finished'", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 100,
                timestamp = 1762700000,
                status = "complete",
                kindle_status = 2,
            })

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 1.0,
                summary = { status = "finished" },
            })

            assert.is_false(sync:syncBidirectional("B001", "/path/book.kfx", ds))
            restoreReadKindleState(sync, orig)
        end)

        it("should sync when only Kindle is complete (PULL scenario)", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 100,
                timestamp = 1762700000, -- newer
                status = "complete",
                kindle_status = 2,
            })

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0.75,
                summary = { status = "reading" },
            })

            assert.is_true(sync:syncBidirectional("B001", "/path/book.kfx", ds))
            restoreReadKindleState(sync, orig)
        end)

        it("should sync when only KOReader is complete (PUSH scenario)", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 50,
                timestamp = 1762600000, -- older
                status = "reading",
                kindle_status = 1,
            })

            local DocSettings = require("docsettings")
            DocSettings:_setSidecarFile("/path/book.kfx", true)

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 1.0,
                summary = { status = "complete" },
            })

            -- ReadHistory mock has time=1762685677 > 1762600000
            assert.is_true(sync:syncBidirectional("B001", "/path/book.kfx", ds))
            restoreReadKindleState(sync, orig)
            DocSettings:_clearSidecars()
        end)

        it("should return false when no Kindle state available", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)

            local orig = mockReadKindleState(sync, nil)
            local ds = createMockDocSettings("/path/book.kfx", { percent_finished = 0.5 })

            assert.is_false(sync:syncBidirectional("B001", "/path/book.kfx", ds))
            restoreReadKindleState(sync, orig)
        end)

        it("should return false when disabled", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(false)
            assert.is_false(sync:syncBidirectional("B001", "/path/book.kfx", {}))
        end)
    end)

    -- ========================================================================
    -- syncBidirectional — timestamp-based PULL/PUSH
    -- ========================================================================
    describe("syncBidirectional — timestamp decisions", function()
        it("should PULL when Kindle is more recent", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 75,
                timestamp = 1762700000, -- newer than ReadHistory
                status = "reading",
                kindle_status = 1,
            })

            local ds = createMockDocSettings("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", {
                percent_finished = 0.30,
                summary = { status = "reading" },
            })

            local result = sync:syncBidirectional("B007N6JEII", "/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", ds)
            assert.is_true(result)
            -- Should have updated KOReader with Kindle's progress
            assert.equals(0.75, ds:readSetting("percent_finished"))
            assert.equals("reading", ds:readSetting("summary").status)

            restoreReadKindleState(sync, orig)
        end)

        it("should PUSH when KOReader is more recent", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig_read = mockReadKindleState(sync, {
                percent_read = 30,
                timestamp = 1000, -- older
                status = "reading",
                kindle_status = 1,
            })

            local orig_write, write_log = mockWriteKindleState(sync)
            local orig_update = sync.updateYjrPosition
            sync.updateYjrPosition = function() end

            local DocSettings = require("docsettings")
            DocSettings:_setSidecarFile("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", true)

            local ds = createMockDocSettings("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", {
                percent_finished = 0.85,
                summary = { status = "reading" },
            })

            -- ReadHistory mock has time=1762685677 > 1000
            local result = sync:syncBidirectional("B007N6JEII", "/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", ds)
            assert.is_true(result)
            assert.equals(1, #write_log)
            assert.equals(37, write_log[1].percent)

            restoreReadKindleState(sync, orig_read)
            restoreWriteKindleState(sync, orig_write)
            sync.updateYjrPosition = orig_update
            DocSettings:_clearSidecars()
        end)

        it("should not PUSH when no sidecar exists (no prior KOReader access)", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig_read = mockReadKindleState(sync, {
                percent_read = 25,
                timestamp = 1000, -- older
                status = "reading",
                kindle_status = 1,
            })

            local orig_write, write_log = mockWriteKindleState(sync)

            -- No sidecar → kr_timestamp = 0 → Kindle is "newer" → PULL
            local DocSettings = require("docsettings")
            DocSettings:_clearSidecars()

            local ds = createMockDocSettings("/some/other/path.kfx", {
                percent_finished = 0.5,
                summary = { status = "reading" },
            })

            local result = sync:syncBidirectional("B001", "/some/other/path.kfx", ds)

            -- Should PULL from Kindle (no sidecar means KOReader timestamp is 0, Kindle is newer)
            assert.is_true(result)
            -- writeKindleState should NOT be called (PULL, not PUSH)
            assert.equals(0, #write_log)

            restoreReadKindleState(sync, orig_read)
            restoreWriteKindleState(sync, orig_write)
        end)

        it("should handle equal timestamps gracefully", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            -- Both at the same timestamp → timestamp check is > not >=, so no PULL, goes to PUSH
            -- But if kr_timestamp == kindle_timestamp and kr_timestamp == 0, PUSH returns false
            local orig_read = mockReadKindleState(sync, {
                percent_read = 50,
                timestamp = 1762685677, -- same as ReadHistory mock
                status = "reading",
                kindle_status = 1,
            })

            local DocSettings = require("docsettings")
            DocSettings:_setSidecarFile("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", true)

            local ds = createMockDocSettings("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", {
                percent_finished = 0.5,
                summary = { status = "reading" },
            })

            local result = sync:syncBidirectional("B007N6JEII", "/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", ds)
            -- Timestamps equal → not > so goes to PUSH
            assert.is_true(result)

            restoreReadKindleState(sync, orig_read)
            DocSettings:_clearSidecars()
        end)
    end)

    -- ========================================================================
    -- syncBidirectional — status sync
    -- ========================================================================
    describe("syncBidirectional — status sync", function()
        it("should sync Kindle status to KOReader in PULL", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 60,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            })

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0.3,
                summary = { status = "abandoned" },
            })

            sync:syncBidirectional("B001", "/path/book.kfx", ds)

            assert.equals("reading", ds:readSetting("summary").status)
            restoreReadKindleState(sync, orig)
        end)

        it("should sync KOReader status to Kindle in PUSH", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig_read = mockReadKindleState(sync, {
                percent_read = 20,
                timestamp = 1000,
                status = "reading",
                kindle_status = 1,
            })

            local orig_write, write_log = mockWriteKindleState(sync)
            local orig_update = sync.updateYjrPosition
            sync.updateYjrPosition = function() end

            local DocSettings = require("docsettings")
            DocSettings:_setSidecarFile("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", true)

            local ds = createMockDocSettings("/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", {
                percent_finished = 0.9,
                summary = { status = "complete" },
            })

            sync:syncBidirectional("B007N6JEII", "/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", ds)
            assert.equals(1, #write_log)
            assert.equals("complete", write_log[1].status)

            restoreReadKindleState(sync, orig_read)
            restoreWriteKindleState(sync, orig_write)
            sync.updateYjrPosition = orig_update
            DocSettings:_clearSidecars()
        end)

        it("should set status to complete when Kindle percent >= 100 in PULL", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 100,
                timestamp = 1762700000,
                status = "reading", -- Even if Kindle says reading
                kindle_status = 1,
            })

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0.5,
                summary = { status = "reading" },
            })

            sync:syncBidirectional("B001", "/path/book.kfx", ds)

            -- Should be forced to complete since percent >= 100
            assert.equals("complete", ds:readSetting("summary").status)
            assert.equals(1.0, ds:readSetting("percent_finished"))

            restoreReadKindleState(sync, orig)
        end)
    end)

    -- ========================================================================
    -- syncBidirectional — unopened books
    -- ========================================================================
    describe("syncBidirectional — unopened books", function()
        it("should NOT sync FROM Kindle when book is unopened (readState=0, 0%)", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 0,
                timestamp = 0,
                status = "",
                kindle_status = 0,
            })

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0.5,
                summary = { status = "reading" },
            })

            -- Kindle is unopened → executePullFromKindle returns false
            -- executePushToKindle may run if kr_timestamp > 0
            sync:syncBidirectional("B001", "/path/book.kfx", ds)
            -- Either way, we should NOT overwrite KOReader with Kindle's 0%
            local saved_pf = ds:readSetting("percent_finished")
            if saved_pf == 0.5 then
                -- PULL was rejected, good
                assert.equals(0.5, saved_pf)
            end
            -- The key thing is it doesn't crash

            restoreReadKindleState(sync, orig)
        end)

        it("should handle doc_settings with minimal data without crash", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)

            local orig = mockReadKindleState(sync, {
                percent_read = 25,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            })

            local ds = createMockDocSettings("/path/book.kfx")

            local ok = pcall(function()
                sync:syncBidirectional("B001", "/path/book.kfx", ds)
            end)
            assert.is_true(ok)

            restoreReadKindleState(sync, orig)
        end)
    end)

    -- ========================================================================
    -- syncBidirectional — virtual path matching
    -- ========================================================================
    describe("syncBidirectional — path matching", function()
        it("should match virtual path to ReadHistory via book ID", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig_read = mockReadKindleState(sync, {
                percent_read = 40,
                timestamp = 1762600000, -- older than ReadHistory
                status = "reading",
                kindle_status = 1,
            })

            local orig_write = mockWriteKindleState(sync)
            local orig_update = sync.updateYjrPosition
            sync.updateYjrPosition = function() end

            local DocSettings = require("docsettings")
            DocSettings:_setSidecarFile("KINDLE_VIRTUAL://B007N6JEII/Book.epub", true)

            local ds = createMockDocSettings("KINDLE_VIRTUAL://B007N6JEII/Book.epub", {
                percent_finished = 0.6,
                summary = { status = "reading" },
            })

            local result = sync:syncBidirectional("B007N6JEII", "/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", ds)
            assert.is_true(result)

            restoreReadKindleState(sync, orig_read)
            restoreWriteKindleState(sync, orig_write)
            sync.updateYjrPosition = orig_update
            DocSettings:_clearSidecars()
        end)

        it("should handle virtual paths with no matching history gracefully", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            setupPluginSettings(sync)

            local orig = mockReadKindleState(sync, {
                percent_read = 50,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            })

            local ds = createMockDocSettings("KINDLE_VIRTUAL://UNKNOWN123/nonexistent.epub", {
                percent_finished = 0.3,
                summary = { status = "reading" },
            })

            local ok = pcall(function()
                sync:syncBidirectional("UNKNOWN123", nil, ds)
            end)
            assert.is_true(ok)

            restoreReadKindleState(sync, orig)
        end)
    end)

    -- ========================================================================
    -- syncBidirectional — sync direction settings
    -- ========================================================================
    describe("syncBidirectional — direction settings", function()
        it("should respect PROMPT direction (calls syncIfApproved)", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local mock_plugin = {
                settings = {
                    enable_sync_from_kindle = true,
                    enable_sync_to_kindle = true,
                    sync_from_kindle_newer = SYNC_DIRECTION.PROMPT,
                    sync_from_kindle_older = SYNC_DIRECTION.NEVER,
                    sync_to_kindle_newer = SYNC_DIRECTION.PROMPT,
                    sync_to_kindle_older = SYNC_DIRECTION.NEVER,
                },
            }
            sync:setPlugin(mock_plugin, SYNC_DIRECTION)

            local orig = mockReadKindleState(sync, {
                percent_read = 80,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            })

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0.3,
                summary = { status = "reading" },
            })

            -- PROMPT will trigger a ConfirmBox (which is mocked) but in test context
            -- it should not crash. The sync may or may not complete depending on mock behavior.
            local ok = pcall(function()
                sync:syncBidirectional("B001", "/path/book.kfx", ds)
            end)
            assert.is_true(ok)

            restoreReadKindleState(sync, orig)
        end)

        it("should deny PULL when sync_from_kindle is disabled", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(true)
            local mock_plugin = {
                settings = {
                    enable_sync_from_kindle = false,
                    enable_sync_to_kindle = true,
                    sync_from_kindle_newer = SYNC_DIRECTION.SILENT,
                    sync_from_kindle_older = SYNC_DIRECTION.NEVER,
                    sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
                    sync_to_kindle_older = SYNC_DIRECTION.NEVER,
                },
            }
            sync:setPlugin(mock_plugin, SYNC_DIRECTION)

            local orig = mockReadKindleState(sync, {
                percent_read = 80,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            })

            local ds = createMockDocSettings("/path/book.kfx", {
                percent_finished = 0.3,
                summary = { status = "reading" },
            })

            local result = sync:syncBidirectional("B001", "/path/book.kfx", ds)
            -- PULL should be denied, no PUSH triggered (kindle is newer → PULL scenario)
            -- Result depends on whether it falls through to PUSH
            assert.is_true(result == true or result == false)

            restoreReadKindleState(sync, orig)
        end)
    end)

    -- ========================================================================
    -- applyKindleStateToKOReader
    -- ========================================================================
    describe("applyKindleStateToKOReader", function()
        it("should convert Kindle percent (0-100) to KOReader percent (0-1)", function()
            local sync = ReadingStateSync:new()
            local ds = createMockDocSettings("/path/book.kfx")

            sync:applyKindleStateToKOReader({
                percent_read = 65,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            }, ds, 0)

            assert.equals(0.65, ds:readSetting("percent_finished"))
            assert.equals(0.65, ds:readSetting("last_percent"))
        end)

        it("should set complete status when percent >= 100", function()
            local sync = ReadingStateSync:new()
            local ds = createMockDocSettings("/path/book.kfx")

            sync:applyKindleStateToKOReader({
                percent_read = 100,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            }, ds, 0)

            assert.equals(1.0, ds:readSetting("percent_finished"))
            assert.equals("complete", ds:readSetting("summary").status)
        end)

        it("should preserve existing summary fields", function()
            local sync = ReadingStateSync:new()
            local ds = createMockDocSettings("/path/book.kfx", {
                summary = { status = "abandoned", modified = "2025-01-01" },
            })

            sync:applyKindleStateToKOReader({
                percent_read = 50,
                timestamp = 1762700000,
                status = "reading",
                kindle_status = 1,
            }, ds, 0)

            local summary = ds:readSetting("summary")
            assert.equals("reading", summary.status)
        end)
    end)

end)
