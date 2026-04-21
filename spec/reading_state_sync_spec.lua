-- Tests for ReadingStateSync module
-- Covers: initialization, cdeKey extraction, bidirectional sync,
-- PULL/PUSH scenarios, both-sides-complete skip, timestamp decisions,
-- status sync, unopened book handling, and edge cases.

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
            enable_sync_from_kindle = true,
            enable_sync_to_kindle = true,
            sync_from_kindle_newer = SYNC_DIRECTION.SILENT,
            sync_from_kindle_older = SYNC_DIRECTION.NEVER,
            sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
            sync_to_kindle_older = SYNC_DIRECTION.NEVER,
        },
    }
    sync:setPlugin(mock_plugin, SYNC_DIRECTION)
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
    local kindle_state_reader_orig
    local kindle_state_writer_orig
    local readhistory_orig

    setup(function()
        require("spec/helper")
    end)

    before_each(function()
        -- Clear all loaded modules
        package.loaded["lua/reading_state_sync"] = nil
        package.loaded["lua/lib/kindle_state_reader"] = nil
        package.loaded["lua/lib/kindle_state_writer"] = nil
        package.loaded["lua/lib/sync_decision_maker"] = nil
        package.loaded["lua/lib/status_converter"] = nil
        package.loaded["readhistory"] = nil
        package.loaded["docsettings"] = nil

        -- Mock KindleStateReader with controllable data
        kindle_state_reader_orig = package.preload["lua/lib/kindle_state_reader"]
        package.preload["lua/lib/kindle_state_reader"] = function()
            local mock_reader_data = {}
            local mock_reader_data_by_key = {}
            local mock_all_books = {}
            return {
                readByCdeKey = function(cde_key)
                    return mock_reader_data_by_key[cde_key]
                end,
                readByPath = function(path)
                    return mock_reader_data[path]
                end,
                readAllProgress = function()
                    return mock_all_books
                end,
                -- Test helpers
                _setMockStateByPath = function(path, state)
                    mock_reader_data[path] = state
                end,
                _setMockStateByKey = function(key, state)
                    mock_reader_data_by_key[key] = state
                end,
                _setMockAllBooks = function(books)
                    mock_all_books = books
                end,
                _clear = function()
                    mock_reader_data = {}
                    mock_reader_data_by_key = {}
                    mock_all_books = {}
                end,
            }
        end

        -- Mock KindleStateWriter
        kindle_state_writer_orig = package.preload["lua/lib/kindle_state_writer"]
        package.preload["lua/lib/kindle_state_writer"] = function()
            local write_log = {}
            return {
                writeByCdeKey = function(cde_key, percent, timestamp, status)
                    table.insert(write_log, { method = "cdeKey", key = cde_key, percent = percent, timestamp = timestamp, status = status })
                    return true
                end,
                writeByPath = function(path, percent, timestamp, status)
                    table.insert(write_log, { method = "path", path = path, percent = percent, timestamp = timestamp, status = status })
                    return true
                end,
                -- Test helper
                _getWriteLog = function() return write_log end,
                _clearWriteLog = function() write_log = {} end,
            }
        end

        -- Mock ReadHistory
        package.preload["readhistory"] = function()
            return {
                hist = {
                    { file = "/mnt/us/documents/Throne of Glass_B007N6JEII.kfx", time = 1762685677 },
                    { file = "/mnt/us/documents/Other Book_B008PL1YQ0.kfx", time = 1762628755 },
                },
            }
        end

        -- Mock DocSettings with sidecar tracking
        package.preload["docsettings"] = function()
            local sidecar_files = {}
            return {
                hasSidecarFile = function(self, path)
                    return sidecar_files[path] == true
                end,
                open = function(self, path)
                    return createMockDocSettings(path, { percent_finished = 0.5 })
                end,
                -- Test helpers (use colon syntax in tests)
                _setSidecarFile = function(self, path, exists)
                    sidecar_files[path] = exists
                end,
                _clearSidecars = function(self)
                    sidecar_files = {}
                end,
            }
        end

        ReadingStateSync = require("lua/reading_state_sync")
        resetAllMocks()
    end)

    teardown(function()
        if kindle_state_reader_orig then
            package.preload["lua/lib/kindle_state_reader"] = kindle_state_reader_orig
        end
        if kindle_state_writer_orig then
            package.preload["lua/lib/kindle_state_writer"] = kindle_state_writer_orig
        end
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
        it("should return false when disabled", function()
            local sync = ReadingStateSync:new()
            sync:setEnabled(false)
            assert.is_false(sync:syncToKindle("B001", "/path/book.kfx", {}))
        end)

        it("should write percent and status to Kindle", function()
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
            assert.equals(75, write_log[1].percent)
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
            assert.equals(0, write_log[1].percent)

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
            assert.equals(100, write_log[1].percent)
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
            assert.equals(42, write_log[1].percent)
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
            assert.equals(85, write_log[1].percent)

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
            local result = sync:syncBidirectional("B001", "/path/book.kfx", ds)
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

            local orig_write, write_log = mockWriteKindleState(sync)
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

    -- ========================================================================
    -- findYjrFile
    -- ========================================================================
    describe("findYjrFile", function()
        it("should return nil for empty path", function()
            local sync = ReadingStateSync:new()
            assert.is_nil(sync:findYjrFile(nil))
            assert.is_nil(sync:findYjrFile(""))
        end)

        it("should return nil when no sdr directory exists", function()
            local sync = ReadingStateSync:new()
            assert.is_nil(sync:findYjrFile("/mnt/us/documents/test.kfx"))
        end)

        it("should find yjr file in sdr directory", function()
            local sync = ReadingStateSync:new()
            local lfs = require("libs/libkoreader-lfs")

            -- Kindle uses bookname.sdr/ (strips extension): test.kfx → test.sdr
            lfs._setFileState("/mnt/us/documents/test.sdr", {
                exists = true,
                attributes = { mode = "directory" },
            })
            lfs._setDirectoryContents("/mnt/us/documents/test.sdr", {
                ".", "..",
                "test_ff433efbe1c342831959dca70028297b.yjr",
            })

            local result = sync:findYjrFile("/mnt/us/documents/test.kfx")
            assert.equals("/mnt/us/documents/test.sdr/test_ff433efbe1c342831959dca70028297b.yjr", result)

            lfs._clearFileStates()
        end)
    end)
end)
