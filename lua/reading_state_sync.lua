--- Reading State Synchronization for Kindle virtual library.
--- Syncs reading progress between KOReader and Kindle's authoritative ReaderSDK
--- state, then mirrors the accepted value to cc.db for shelf display.
--- Adapted from kobo.koplugin/src/reading_state_sync.lua.
---
--- Kindle stores progress in /var/local/cc.db, Entries table:
---   p_percentFinished (float 0-100), p_lastAccess (Unix timestamp), p_readState (int)
---
--- KOReader stores progress in doc_settings:
---   percent_finished (float 0-1), summary.status (string)

local DocSettings = require("docsettings")
local ReadHistory = require("readhistory")
local UIManager = require("ui/uimanager")
local _ = require("gettext")
local logger = require("logger")
local T = require("ffi/util").template
local BookList = require("ui/widget/booklist")
local Trapper = require("ui/trapper")
local ffiUtil = require("ffi/util")
local KindleStateReader = require("lua/lib/kindle_state_reader")
local KindleStateWriter = require("lua/lib/kindle_state_writer")
local SyncDecisionMaker = require("lua/lib/sync_decision_maker")

local ReadingStateSync = {}

--- Path to the Kindle content catalog database.
local CC_DB_PATH = "/var/local/cc.db"

---
--- Extracts book cdeKey (ASIN or PDOC hash) from virtual path.
--- @param virtual_path string: Virtual path in format KINDLE_VIRTUAL://BOOKID/filename.
--- @return string|nil: Book cdeKey if extracted.
local function extractCdeKeyFromVirtualPath(virtual_path)
    if not virtual_path or not virtual_path:match("^KINDLE_VIRTUAL://") then
        return nil
    end

    -- Extract book ID from virtual path
    local book_id = virtual_path:match("^KINDLE_VIRTUAL://([^/]+)/")
    if book_id then
        logger.dbg("KindlePlugin: Extracted book ID from virtual path:", book_id)
        return book_id
    end

    return nil
end

---
--- Extracts ASIN from a file path like .../Throne of Glass_B007N6JEII.kfx
--- @param file_path string: Real file path on device.
--- @return string|nil: ASIN if found.
local function extractCdeKeyFromPath(file_path)
    if not file_path then
        return nil
    end
    -- Match _B007N6JEII.kfx (ASIN) or _5AFAFAA13FFE43ECBE78F0FF3761814C.kfx (PDOC hash)
    -- The cdeKey is always the last segment before the extension after underscore
    local key = file_path:match("_([A-Z0-9]+)%.%w+$")
    if key and #key >= 10 then
        logger.dbg("KindlePlugin: Extracted cdeKey from path:", key)
        return key
    end
    return nil
end

---
--- Extracts book cdeKey from doc_path in doc_settings.
--- @param doc_settings table: Document settings instance.
--- @return string|nil: Book cdeKey if extracted.
local function extractCdeKeyFromDocPath(doc_settings)
    if not doc_settings or not doc_settings.data or not doc_settings.data.doc_path then
        return nil
    end

    local doc_path = doc_settings.data.doc_path
    -- Try extracting from virtual path
    local book_id = extractCdeKeyFromVirtualPath(doc_path)
    if book_id then
        return book_id
    end

    -- Try extracting ASIN from filename pattern like _B007N6JEII.kfx
    local asin = extractCdeKeyFromPath(doc_path)
    if asin then
        logger.dbg("KindlePlugin: Extracted ASIN from doc_path:", asin)
        return asin
    end

    return nil
end

---
--- Creates a new ReadingStateSync instance.
--- @return table: A new ReadingStateSync instance.
function ReadingStateSync:new(helper_client)
    local o = {
        enabled = false,
        plugin = nil,
        sync_direction = nil,
        helper_client = helper_client,
    }
    setmetatable(o, self)
    self.__index = self
    return o
end

---
--- Sets plugin instance and sync direction constants.
--- @param plugin table: Main plugin instance with settings.
--- @param sync_direction table: SYNC_DIRECTION constants.
function ReadingStateSync:setPlugin(plugin, sync_direction)
    self.plugin = plugin
    self.sync_direction = sync_direction
end

--- Provides access to the virtual-library mapping and its cached EPUBs.
function ReadingStateSync:setVirtualLibrary(virtual_library)
    self.virtual_library = virtual_library
end

---
--- Checks if reading state sync is enabled.
--- @return boolean: True if sync is enabled.
function ReadingStateSync:isEnabled()
    return self.enabled
end

---
--- Sets whether reading state sync is enabled.
--- @param enabled boolean: True to enable sync.
function ReadingStateSync:setEnabled(enabled)
    self.enabled = enabled
    logger.info("KindlePlugin: Reading state sync", enabled and "enabled" or "disabled")
end

---
--- Checks if automatic sync is enabled.
--- @return boolean: True if auto-sync is enabled.
function ReadingStateSync:isAutomaticSyncEnabled()
    if not self.enabled or not self.plugin then
        return false
    end

    return self.plugin.settings.enable_auto_sync == true
end

---
--- Extracts book cdeKey from various path formats.
--- @param virtual_path string|nil: Virtual path to check.
--- @param doc_settings table|nil: Document settings instance.
--- @return string|nil: Book cdeKey if extraction succeeds.
function ReadingStateSync:extractCdeKey(virtual_path, doc_settings)
    if not virtual_path and not doc_settings then
        return nil
    end

    local cde_key = extractCdeKeyFromVirtualPath(virtual_path)
    if cde_key then
        return cde_key
    end

    cde_key = extractCdeKeyFromDocPath(doc_settings)
    if cde_key then
        return cde_key
    end

    logger.dbg("KindlePlugin: Could not extract book cdeKey from paths")
    return nil
end

---
--- Gets book title from various sources.
--- @param cde_key string: Book cdeKey.
--- @param doc_settings table: Document settings instance.
--- @return string: Book title, or "Unknown Book" if not found.
function ReadingStateSync:getBookTitle(cde_key, doc_settings)
    -- First try doc_settings
    local title = doc_settings and doc_settings:readSetting("title")
    if title and title ~= "" then
        return title
    end

    -- Try reading from cc.db
    local state = self:readKindleState(cde_key, nil)
    if state and state.title and state.title ~= "" then
        return state.title
    end

    return "Unknown Book"
end

--- Persist the exact KOReader XPointer through Kindle's native ReaderSDK.
--- The visible catalog must only be advanced after this succeeds; otherwise
--- the native reader would reopen its older LPR and overwrite the shelf value.
function ReadingStateSync:saveAuthoritativeNativePosition(cde_key, source_path, epub_path, doc_settings)
    local asin = cde_key
    if not asin or not asin:match("^B[A-Z0-9]+$") then
        asin = extractCdeKeyFromPath(source_path)
    end
    if not asin or not asin:match("^B[A-Z0-9]+$") or #asin ~= 10 then
        logger.warn("KindlePlugin: exact native progress requires a Kindle ASIN")
        return false
    end
    if not epub_path or not epub_path:match("%.epub$") then
        local doc_path = doc_settings and doc_settings.data and doc_settings.data.doc_path
        if doc_path and doc_path:match("%.epub$") then
            epub_path = doc_path
        end
    end
    local xpointer = doc_settings and doc_settings:readSetting("last_xpointer")
    if not epub_path or not xpointer then
        logger.warn("KindlePlugin: exact native progress is missing EPUB path or XPointer")
        return false
    end

    local position, translate_error = self.helper_client:translatePosition(epub_path, xpointer)
    if not position then
        logger.warn("KindlePlugin: exact native position translation failed:", translate_error)
        return false
    end
    local saved, save_error, native_percent = self.helper_client:saveNativeProgress(
        asin, source_path, position
    )
    if not saved then
        logger.warn("KindlePlugin: exact native progress save failed:", save_error)
        return false
    end
    if type(native_percent) ~= "number" or native_percent < 0 or native_percent > 100 then
        logger.warn("KindlePlugin: native progress save omitted Kindle percentage")
        return false
    end
    return native_percent
end

--- Resolve Kindle's exact local LPR back to a KOReader XPointer.
function ReadingStateSync:getAuthoritativeKindleXPointer(cde_key, source_path, epub_path)
    local asin = cde_key
    if not asin or not asin:match("^B[A-Z0-9]+$") then
        asin = extractCdeKeyFromPath(source_path)
    end
    if not asin or #asin ~= 10 or not epub_path or not epub_path:match("%.epub$") then
        return nil, "exact native position is unavailable for this book"
    end
    local native, read_error = self.helper_client:readNativeProgress(asin, source_path)
    if not native then
        return nil, read_error
    end
    local translated, translate_error = self.helper_client:translateNativePosition(
        epub_path, native.long
    )
    if not translated then
        return nil, translate_error
    end
    if native.pid and translated.pid and native.pid ~= translated.pid then
        return nil, "native reverse position mismatch"
    end
    return translated.xpointer
end

--- Reads Kindle reading state from cc.db.
--- Tries cdeKey first, then source_path if provided.
--- @param cde_key string|nil: Book cdeKey (ASIN or hash).
--- @param source_path string|nil: Real file path on device.
--- @return table|nil: State table with percent_read, timestamp, status, kindle_status.
function ReadingStateSync:readKindleState(cde_key, source_path)
    local catalog_uuid = cde_key and cde_key:match("^cc:([%x%-]+)$")
    if catalog_uuid then
        local state = KindleStateReader.readByUuid(catalog_uuid)
        if state then
            return state
        end
    end

    -- Try by cdeKey first (avoids ICU collation issue with p_location index)
    -- Extract ASIN from source_path for virtual UUIDs and sha1 IDs.
    local actual_cde_key = cde_key
    if not actual_cde_key or catalog_uuid or actual_cde_key:match("^sha1:") then
        actual_cde_key = extractCdeKeyFromPath(source_path)
    end
    if actual_cde_key and actual_cde_key ~= "" then
        local state = KindleStateReader.readByCdeKey(actual_cde_key)
        if state then
            return state
        end
    end

    -- Fall back to source_path (may fail with ICU collation)
    if source_path and source_path ~= "" then
        return KindleStateReader.readByPath(source_path)
    end

    return nil
end

---
--- Writes KOReader reading state to Kindle cc.db.
--- Tries cdeKey first, then source_path if needed.
--- @param cde_key string|nil: Book cdeKey (ASIN or hash).
--- @param source_path string|nil: Real file path on device.
--- @param percent_read number: Progress percentage (0-100).
--- @param timestamp number: Unix timestamp of last read.
--- @param status string: KOReader status string.
--- @return boolean: True if write succeeded.
function ReadingStateSync:writeKindleState(cde_key, source_path, percent_read, timestamp, status)
    local catalog_uuid = cde_key and cde_key:match("^cc:([%x%-]+)$")
    if catalog_uuid then
        local ok = KindleStateWriter.writeByUuid(
            catalog_uuid,
            percent_read,
            timestamp,
            status
        )
        if ok then
            return true
        end
    end

    -- Try by cdeKey first (avoids ICU collation issue with p_location index)
    -- Extract ASIN from source_path for virtual UUIDs and sha1 IDs.
    local actual_cde_key = cde_key
    if not actual_cde_key or catalog_uuid or actual_cde_key:match("^sha1:") then
        actual_cde_key = extractCdeKeyFromPath(source_path)
    end
    if actual_cde_key and actual_cde_key ~= "" then
        local ok = KindleStateWriter.writeByCdeKey(actual_cde_key, percent_read, timestamp, status)
        if ok then
            return true
        end
    end

    -- Fall back to source_path (may fail with ICU collation)
    if source_path and source_path ~= "" then
        return KindleStateWriter.writeByPath(source_path, percent_read, timestamp, status)
    end

    return false
end

---
--- Evaluates whether a sync should proceed based on user settings.
--- @param is_pull_from_kindle boolean: True if pulling FROM Kindle.
--- @param is_newer boolean: True if source is newer.
--- @param sync_fn function: Callback to execute if approved.
--- @param sync_details table: Optional details for user prompt.
--- @return boolean: True if sync was executed.
function ReadingStateSync:syncIfApproved(is_pull_from_kindle, is_newer, sync_fn, sync_details)
    if not self.plugin or not self.sync_direction then
        logger.warn("KindlePlugin: Sync settings not configured, denying sync")
        return false
    end

    return SyncDecisionMaker.syncIfApproved(
        self.plugin,
        self.sync_direction,
        is_pull_from_kindle,
        is_newer,
        sync_fn,
        sync_details
    )
end

---
--- Gets KOReader timestamp from ReadHistory for a document.
--- @param doc_path string: Document path.
--- @return number: Timestamp from ReadHistory, or 0 if not found.
local function getKOReaderTimestampFromHistory(doc_path)
    if not doc_path then
        return 0
    end

    local book_id_from_virtual = nil
    if doc_path:match("^KINDLE_VIRTUAL://") then
        book_id_from_virtual = doc_path:match("^KINDLE_VIRTUAL://([^/]+)/")
    end

    for _, entry in ipairs(ReadHistory.hist) do
        if not entry.file then
            goto continue
        end

        if entry.file == doc_path then
            return entry.time or 0
        end

        if book_id_from_virtual and entry.file:match(book_id_from_virtual) then
            return entry.time or 0
        end

        ::continue::
    end

    return 0
end

---
--- Gets validated KOReader timestamp, checking for sidecar file existence.
--- @param doc_path string: Document path.
--- @return number: Valid timestamp, or 0 if no sidecar.
local function getValidatedKOReaderTimestamp(doc_path)
    local kr_timestamp = getKOReaderTimestampFromHistory(doc_path)
    if kr_timestamp == 0 then
        return 0
    end

    if not DocSettings:hasSidecarFile(doc_path) then
        logger.dbg("KindlePlugin: ReadHistory exists but no sidecar - ignoring timestamp")
        return 0
    end

    return kr_timestamp
end

---
--- Sync reading state from Kindle to KOReader (PULL).
--- @param cde_key string: Book cdeKey.
--- @param doc_settings table: Document settings instance.
--- @return boolean: True if sync was performed.
function ReadingStateSync:syncFromKindle(cde_key, source_path, doc_settings)
    if not self:isEnabled() then
        return false
    end

    local kindle_state = self:readKindleState(cde_key, source_path)
    if not kindle_state or not kindle_state.percent_read then
        return false
    end

    -- Don't pull from Kindle if the book has never been opened there
    if kindle_state.kindle_status == 0 or kindle_state.percent_read == 0 then
        logger.dbg("KindlePlugin: Skipping sync FROM Kindle - book unopened for:", cde_key)
        return false
    end

    local kr_timestamp = getValidatedKOReaderTimestamp(
        doc_settings.data and doc_settings.data.doc_path
    )

    if kindle_state.timestamp <= kr_timestamp then
        logger.dbg("KindlePlugin: KOReader is more recent, keeping KOReader value")
        return false
    end

    local epub_path = doc_settings.data and doc_settings.data.doc_path
    local exact_xpointer, position_error = self:getAuthoritativeKindleXPointer(
        cde_key, source_path, epub_path
    )
    if not exact_xpointer then
        logger.warn("KindlePlugin: exact native progress pull failed:", position_error)
        return false
    end
    local applied = self:applyKindleStateToKOReader(
        kindle_state, doc_settings, kr_timestamp
    )
    if applied then
        doc_settings:saveSetting("last_xpointer", exact_xpointer)
    end
    return applied
end

---
--- Applies Kindle state to KOReader settings.
--- @param kindle_state table: Kindle reading state.
--- @param doc_settings table: Document settings instance.
--- @param kr_timestamp number: KOReader timestamp for logging.
--- @return boolean: True if state was applied.
function ReadingStateSync:applyKindleStateToKOReader(kindle_state, doc_settings, kr_timestamp)
    local koreader_percent = kindle_state.percent_read / 100.0

    logger.info(
        "KindlePlugin: Syncing FROM Kindle - Kindle is more recent:",
        "Kindle timestamp:",
        kindle_state.timestamp,
        "vs KOReader:",
        kr_timestamp,
        "percent:",
        kindle_state.percent_read
    )

    doc_settings:saveSetting("percent_finished", koreader_percent)
    doc_settings:saveSetting("last_percent", koreader_percent)

    local summary = doc_settings:readSetting("summary") or {}
    summary.status = kindle_state.status

    if kindle_state.percent_read >= 100 then
        summary.status = "complete"
    end

    doc_settings:saveSetting("summary", summary)

    return true
end

---
--- Sync reading state from KOReader to Kindle (PUSH).
--- @param cde_key string|nil: Book cdeKey.
--- @param source_path string|nil: Real file path on device.
--- @param doc_settings table: Document settings instance.
--- @return boolean: True if write succeeded.
function ReadingStateSync:syncToKindle(cde_key, source_path, doc_settings, epub_path)
    if not self:isEnabled() then
        return false
    end

    local kr_percent = doc_settings:readSetting("percent_finished") or 0
    local summary = doc_settings:readSetting("summary") or {}
    local kr_status = summary.status or "reading"
    local current_timestamp = os.time()

    logger.info(
        "KindlePlugin: Syncing TO Kindle - writing KOReader progress:",
        string.format("%.2f%%", kr_percent * 100),
        "timestamp:",
        current_timestamp,
        "source_path:",
        source_path
    )

    local native_percent = self:saveAuthoritativeNativePosition(
        cde_key, source_path, epub_path, doc_settings
    )
    if not native_percent then
        return false
    end
    return self:writeKindleState(
        cde_key, source_path, native_percent, current_timestamp, kr_status
    )
end

--- Sync the native Kindle state into KOReader during an automatic open.
--- Unlike syncFromKindle(), this honors automatic-sync and all configured
--- direction choices, including an explicitly allowed older Kindle state.
function ReadingStateSync:syncFromKindleAutomatic(cde_key, source_path, doc_settings, epub_path)
    if not self:isAutomaticSyncEnabled() then
        return false
    end

    local kindle_state = self:readKindleState(cde_key, source_path)
    if not kindle_state or not kindle_state.percent_read then
        return false
    end
    if kindle_state.kindle_status == 0 or kindle_state.percent_read == 0 then
        return false
    end

    local doc_path = doc_settings.data and doc_settings.data.doc_path
    local kr_timestamp = getValidatedKOReaderTimestamp(doc_path)
    local kr_percent = doc_settings:readSetting("percent_finished") or 0
    local summary = doc_settings:readSetting("summary") or {}
    local kr_status = summary.status or "reading"
    -- A shelf percentage is not a position. Two locations within the same
    -- displayed percentage can have different KFX EID/offset coordinates, so
    -- the timestamp and authoritative LPR must decide the open-time pull.
    local kindle_is_newer = kindle_state.timestamp > kr_timestamp
    local sync_details = {
        book_title = self:getBookTitle(cde_key, doc_settings),
        source_percent = kindle_state.percent_read,
        dest_percent = kr_percent * 100,
        source_time = kindle_state.timestamp,
        dest_time = kr_timestamp,
    }

    local sync_completed = false
    self:syncIfApproved(true, kindle_is_newer, function()
        local exact_xpointer, position_error = self:getAuthoritativeKindleXPointer(
            cde_key, source_path, epub_path
        )
        if not exact_xpointer then
            logger.warn("KindlePlugin: exact native progress pull failed:", position_error)
            return
        end
        local same_position = doc_settings:readSetting("last_xpointer") == exact_xpointer
        local same_status = kr_status == kindle_state.status
            or (kr_percent >= 1 and kindle_state.percent_read >= 100)
        if same_position and same_status then
            return
        end
        sync_completed = self:applyKindleStateToKOReader(
            kindle_state,
            doc_settings,
            kr_timestamp
        )
        if sync_completed then
            doc_settings:saveSetting("last_xpointer", exact_xpointer)
            doc_settings:flush()
        end
    end, sync_details)
    return sync_completed
end

--- Sync KOReader state into Kindle's authoritative ReaderSDK state during close.
--- This honors automatic-sync and all configured direction choices.
function ReadingStateSync:syncToKindleAutomatic(cde_key, source_path, doc_settings, epub_path)
    if not self:isAutomaticSyncEnabled() then
        return false
    end

    local kr_percent = doc_settings:readSetting("percent_finished") or 0
    local summary = doc_settings:readSetting("summary") or {}
    local kr_status = summary.status or "reading"
    local doc_path = doc_settings.data and doc_settings.data.doc_path
    local kr_timestamp = getValidatedKOReaderTimestamp(doc_path)
    if kr_timestamp == 0 then
        return false
    end

    local kindle_state = self:readKindleState(cde_key, source_path) or {
        percent_read = 0,
        timestamp = 0,
        status = "",
        kindle_status = 0,
    }
    if SyncDecisionMaker.areBothSidesComplete(kindle_state, kr_percent, kr_status) then
        return false
    end
    local koreader_is_newer = kr_timestamp >= (kindle_state.timestamp or 0)
    local sync_details = {
        book_title = self:getBookTitle(cde_key, doc_settings),
        source_percent = kr_percent * 100,
        dest_percent = kindle_state.percent_read or 0,
        source_time = kr_timestamp,
        dest_time = kindle_state.timestamp or 0,
    }

    local sync_completed = false
    self:syncIfApproved(false, koreader_is_newer, function()
        local current_timestamp = os.time()
        -- Always verify the exact LPR even when the rounded shelf percentage
        -- already matches; two positions inside one percentage point differ.
        local native_percent = self:saveAuthoritativeNativePosition(
            cde_key, source_path, epub_path, doc_settings
        )
        if native_percent then
            sync_completed = self:writeKindleState(
                cde_key,
                source_path,
                native_percent,
                current_timestamp,
                kr_status
            )
        end
    end, sync_details)
    return sync_completed
end

---
--- Executes sync FROM Kindle to KOReader (PULL scenario).
function ReadingStateSync:executePullFromKindle(cde_key, source_path, doc_settings, kindle_state, kr_percent, kr_timestamp)
    logger.info(
        "KindlePlugin: Kindle is more recent - PULL scenario:",
        "Kindle:",
        kindle_state.percent_read,
        "% (",
        kindle_state.timestamp,
        ")",
        "KOReader:",
        kr_percent * 100,
        "% (",
        kr_timestamp,
        ")"
    )

    if kindle_state.kindle_status == 0 and kindle_state.percent_read == 0 then
        return false
    end

    local sync_details = {
        book_title = self:getBookTitle(cde_key, doc_settings),
        source_percent = kindle_state.percent_read,
        dest_percent = kr_percent * 100,
        source_time = kindle_state.timestamp,
        dest_time = kr_timestamp,
    }

    local sync_completed = false
    self:syncIfApproved(true, true, function()
        local epub_path = doc_settings.data and doc_settings.data.doc_path
        local exact_xpointer, position_error = self:getAuthoritativeKindleXPointer(
            cde_key, source_path, epub_path
        )
        if not exact_xpointer then
            logger.warn("KindlePlugin: exact native progress pull failed:", position_error)
            return
        end
        local koreader_percent = kindle_state.percent_read / 100.0
        logger.info("KindlePlugin: Syncing FROM Kindle (PULL)")
        doc_settings:saveSetting("percent_finished", koreader_percent)
        doc_settings:saveSetting("last_percent", koreader_percent)

        local summary = doc_settings:readSetting("summary") or {}
        summary.status = kindle_state.status
        if kindle_state.percent_read >= 100 then
            summary.status = "complete"
        end
        doc_settings:saveSetting("summary", summary)
        doc_settings:saveSetting("last_xpointer", exact_xpointer)
        doc_settings:flush()

        sync_completed = true
    end, sync_details)

    return sync_completed
end

---
--- Executes sync FROM KOReader to Kindle (PUSH scenario).
function ReadingStateSync:executePushToKindle(cde_key, source_path, doc_settings, kindle_state, kr_percent, kr_timestamp)
    logger.info(
        "KindlePlugin: KOReader is more recent - PUSH scenario:",
        "KOReader:",
        kr_percent * 100,
        "% (",
        kr_timestamp,
        ")",
        "Kindle:",
        kindle_state.percent_read,
        "% (",
        kindle_state.timestamp,
        ")"
    )

    if kr_timestamp == 0 then
        return false
    end

    local sync_details = {
        book_title = self:getBookTitle(cde_key, doc_settings),
        source_percent = kr_percent * 100,
        dest_percent = kindle_state.percent_read,
        source_time = kr_timestamp,
        dest_time = kindle_state.timestamp,
    }

    local sync_completed = false
    self:syncIfApproved(false, true, function()
        local summary = doc_settings:readSetting("summary") or {}
        local kr_status = summary.status or "reading"
        local current_timestamp = os.time()

        logger.info("KindlePlugin: Syncing TO Kindle (PUSH)")
        local epub_path = doc_settings.data and doc_settings.data.doc_path
        local native_percent = self:saveAuthoritativeNativePosition(
            cde_key, source_path, epub_path, doc_settings
        )
        if native_percent then
            sync_completed = self:writeKindleState(
                cde_key, source_path, native_percent, current_timestamp, kr_status
            )
        end
    end, sync_details)

    return sync_completed
end

---
--- Bidirectional sync - used when showing virtual library.
--- Winner is whoever was read more recently.
--- @param cde_key string|nil: Book cdeKey.
--- @param source_path string|nil: Real file path on device.
--- @param doc_settings table: Document settings instance.
--- @return boolean: True if sync was performed.
function ReadingStateSync:syncBidirectional(cde_key, source_path, doc_settings)
    if not self:isEnabled() then
        return false
    end

    local kindle_state = self:readKindleState(cde_key, source_path)
    if not kindle_state then
        return false
    end

    local kr_percent = doc_settings:readSetting("percent_finished") or 0
    local doc_path = doc_settings.data and doc_settings.data.doc_path
    local kr_timestamp = getValidatedKOReaderTimestamp(doc_path)

    local summary = doc_settings:readSetting("summary") or {}
    local kr_status = summary.status

    if SyncDecisionMaker.areBothSidesComplete(kindle_state, kr_percent, kr_status) then
        logger.dbg("KindlePlugin: Both sides complete, skipping sync for:", cde_key or source_path)
        return false
    end

    if kindle_state.timestamp > kr_timestamp then
        return self:executePullFromKindle(cde_key, source_path, doc_settings, kindle_state, kr_percent, kr_timestamp)
    end

    return self:executePushToKindle(cde_key, source_path, doc_settings, kindle_state, kr_percent, kr_timestamp)
end

---
--- Syncs a single book during manual sync.
--- @param book table: Book info with path and cde_key.
--- @return boolean: True if sync was successful.
function ReadingStateSync:syncBook(book)
    local source_path = book.real_path or book.filepath or book.location
    if not source_path then
        return false
    end

    -- Exact KFX coordinates can only be translated using the converted EPUB
    -- that KOReader actually reads. Never open a second sidecar against KFX.
    local mapped_book = self.virtual_library and self.virtual_library:getBook(source_path)
    local cache_manager = self.virtual_library and self.virtual_library.cache_manager
    if not mapped_book or not cache_manager then
        logger.dbg("KindlePlugin: manual sync skipped; no virtual mapping for", source_path)
        return false
    end
    local fresh, epub_path = cache_manager:isFresh(mapped_book)
    if not fresh then
        logger.dbg("KindlePlugin: manual sync skipped; book has no fresh cached EPUB:", source_path)
        return false
    end

    local doc_settings = DocSettings:open(epub_path)
    if not doc_settings then
        return false
    end

    local cde_key = book.cde_key
    if not cde_key then
        -- Try to find the ASIN from the file path
        cde_key = source_path:match("_(B[A-Z0-9]+)%.%w+$")
    end

    return self:syncBidirectional(cde_key, source_path, doc_settings)
end

---
--- Invalidates book metadata caches and broadcasts refresh events.
function ReadingStateSync:invalidateMetadataCaches()
    logger.info("KindlePlugin: Invalidating all book metadata caches after sync")
    BookList.book_info_cache = {}
    UIManager:broadcastEvent(Event:new("BookMetadataChanged"))
    UIManager:broadcastEvent(Event:new("InvalidateMetadataCache"))
end

---
--- Sync all accessible books in the library (manually triggered).
--- @return number: Number of books successfully synced.
function ReadingStateSync:syncAllBooksManual()
    if not self:isEnabled() then
        logger.warn("KindlePlugin: Sync is disabled")
        return 0
    end

    local result = 0
    Trapper:wrap(function()
        result = self:syncAllBooks()
    end)

    return result
end

---
--- Internal: Sync all accessible books (called from within Trapper:wrap context).
--- @return number: Number of books successfully synced.
function ReadingStateSync:syncAllBooks()
    if not self:isEnabled() then
        return 0
    end

    if self.virtual_library then
        local mapped, map_error = self.virtual_library:buildMappings(false)
        if not mapped then
            logger.warn("KindlePlugin: cannot build mappings for manual sync:", map_error)
            return 0
        end
    end

    -- Read all books from cc.db
    local all_books = KindleStateReader.readAllProgress()
    if not all_books or #all_books == 0 then
        logger.info("KindlePlugin: No books found in cc.db")
        return 0
    end

    logger.info("KindlePlugin: Starting manual sync for", #all_books, "books")

    Trapper:setPausedText(_("Do you want to abort sync?"), _("Abort"), _("Continue"))

    local go_on = Trapper:info(_("Scanning books..."))
    if not go_on then
        return 0
    end

    local synced_count = 0
    for i, book in ipairs(all_books) do
        go_on = Trapper:info(T(_("Syncing: %1 / %2"), i, #all_books))
        if not go_on then
            logger.info("KindlePlugin: Manual sync aborted at book", i, "of", #all_books)
            Trapper:clear()
            return synced_count
        end

        if self:syncBook(book) then
            synced_count = synced_count + 1
        end
    end

    logger.info("KindlePlugin: Manual sync completed -", synced_count, "books synced")

    if synced_count > 0 then
        self:invalidateMetadataCaches()
    end

    ffiUtil.sleep(2)
    Trapper:info(T(_("Synced %1 books"), synced_count))
    ffiUtil.sleep(2)
    Trapper:clear()

    return synced_count
end

return ReadingStateSync
