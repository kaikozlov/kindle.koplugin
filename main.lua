---
--- Kindle Plugin Entry Point.
--- Provides access to Kindle native library books in KOReader.

local DataStorage = require("datastorage")
local Device = require("device")
local ConfirmBox = require("ui/widget/confirmbox")
local InfoMessage = require("ui/widget/infomessage")
local PathChooser = require("ui/widget/pathchooser")
local UIManager = require("ui/uimanager")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local _ = require("gettext")
local logger = require("logger")
local T = require("ffi/util").template
local util = require("util")

local CacheManager = require("lua/cache_manager")
local FileChooserExt = require("lua/filechooser_ext")
local HelperClient = require("lua/helper_client")
local KindleLibrary = require("lua/kindle_library")
local LibraryIndex = require("lua/library_index")
local OpenFileExt = require("lua/open_file_ext")
local ReadingStateSync = require("lua/reading_state_sync")
local SyncDecisionMaker = require("lua/lib/sync_decision_maker")
local VirtualLibrary = require("lua/virtual_library")

local SYNC_DIRECTION = {
    PROMPT = 1,
    SILENT = 2,
    NEVER = 3,
}

--- Gets localized name for a sync direction.
--- @param direction number: SYNC_DIRECTION constant.
--- @return string: Localized direction name.
local function getNameDirection(direction)
    if direction == SYNC_DIRECTION.PROMPT then
        return _("Prompt")
    end
    if direction == SYNC_DIRECTION.SILENT then
        return _("Silent")
    end
    return _("Never")
end

local default_settings = {
    enable_virtual_library = true,
    virtual_library_cover_path = "",
    documents_root = "/mnt/us/documents",
    cache_dir = DataStorage:getFullDataDir() .. "/cache/kindle.koplugin",
    index_ttl_seconds = 300,
    sync_reading_state = false,
    enable_auto_sync = true,
    enable_sync_from_kindle = false,
    enable_sync_to_kindle = true,
    sync_from_kindle_newer = SYNC_DIRECTION.PROMPT,
    sync_from_kindle_older = SYNC_DIRECTION.NEVER,
    sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
    sync_to_kindle_older = SYNC_DIRECTION.NEVER,
}

local helper_client = HelperClient:new()
local library_index = LibraryIndex:new()
local virtual_library = VirtualLibrary:new(library_index)
local cache_manager = CacheManager:new(helper_client)
local reading_state_sync = ReadingStateSync:new(helper_client)
local kindle_library = KindleLibrary:new(virtual_library, cache_manager)
virtual_library:setCacheManager(cache_manager)
reading_state_sync:setVirtualLibrary(virtual_library)

local KindlePlugin = WidgetContainer:extend({
    name = "kindle_plugin",
    settings_key = "kindle_plugin",
    is_doc_only = false,
    default_settings = default_settings,
})

local function getMappedBook(document)
    local path = document and document.file
    return path and virtual_library:getBook(path) or nil
end

local function getBookCdeKey(book, doc_settings)
    if not book then
        return nil
    end
    return book.cde_key or reading_state_sync:extractCdeKey(book.source_path, doc_settings) or reading_state_sync:extractCdeKey(nil, doc_settings)
end

--- Initializes the plugin and registers only the minimal runtime integration
--- needed by the current KOReader context.
function KindlePlugin:init()
    self:loadSettings()
    self.ui.menu:registerToMainMenu(self)

    reading_state_sync:setPlugin(self, SYNC_DIRECTION)
    reading_state_sync:setEnabled(self.settings.sync_reading_state == true)

    -- FileManager constructs its FileChooser after plugin instances are
    -- initialized, so installing this small reversible class hook here keeps
    -- PathChooser/ReaderUI/DocumentRegistry entirely native.
    if self.settings.enable_virtual_library ~= false or self.settings.sync_reading_state then
        OpenFileExt:init(virtual_library, cache_manager)
        OpenFileExt:apply()
    end
    if self.settings.enable_virtual_library ~= false then
        if self.ui and not self.ui.document then
            kindle_library:setUI(self.ui)
            local FileChooser = require("ui/widget/filechooser")
            FileChooserExt:init(virtual_library, kindle_library)
            FileChooserExt:apply(FileChooser)
            local return_request = kindle_library:takeReturnToLibraryRequest()
            if return_request then
                local filemanager_ui = self.ui
                filemanager_ui:registerPostInitCallback(function()
                    local origin_path = return_request["origin_path"]
                    if type(origin_path) == "string" and filemanager_ui.file_chooser and filemanager_ui.file_chooser.changeToPath then
                        filemanager_ui.file_chooser:changeToPath(origin_path)
                    end
                    UIManager:nextTick(function()
                        local FileManager = require("apps/filemanager/filemanager")
                        if FileManager.instance ~= filemanager_ui then
                            return
                        end
                        logger.info("KindlePlugin: returning to native Kindle Library")
                        kindle_library:setUI(filemanager_ui)
                        kindle_library:show(filemanager_ui, false)
                    end)
                end)
            end
        end
    end
end

local function stagePagingPercent(ui, doc_settings, percent)
    local paging = ui and ui.paging
    local pages = paging and tonumber(paging.number_of_pages)
    if not pages or pages <= 0 or type(percent) ~= "number" then
        return false
    end
    local page = math.floor(pages * percent)
    if page < 1 then
        page = 1
    end
    if page > pages then
        page = pages
    end
    doc_settings:saveSetting("last_page", page)
    return true
end

local function configuredDirection(settings, is_pull_from_kindle, is_newer)
    if is_pull_from_kindle then
        return is_newer and settings.sync_from_kindle_newer or settings.sync_from_kindle_older
    end
    return is_newer and settings.sync_to_kindle_newer or settings.sync_to_kindle_older
end

local function applyPullToLiveReader(plugin, document, doc_settings, sync_fn)
    local before_xpointer = doc_settings:readSetting("last_xpointer")
    local before_percent = doc_settings:readSetting("percent_finished") or 0
    sync_fn()
    local after_xpointer = doc_settings:readSetting("last_xpointer")
    local after_percent = doc_settings:readSetting("percent_finished") or 0
    local reader_is_active = plugin.ui and plugin.ui.document == document

    if reader_is_active and plugin.ui.rolling then
        if after_xpointer and after_xpointer ~= before_xpointer and plugin.ui.rolling.onGotoXPointer then
            plugin.ui.rolling:onGotoXPointer(after_xpointer)
        elseif after_percent ~= before_percent and plugin.ui.rolling.onGotoPercent then
            plugin.ui.rolling:onGotoPercent(after_percent * 100)
        end
        reading_state_sync:verifyOpenedKOReaderPosition(plugin.ui, document.file)
    elseif reader_is_active and plugin.ui.paging and after_percent ~= before_percent and plugin.ui.paging.onGotoPercent then
        plugin.ui.paging:onGotoPercent(after_percent * 100)
    else
        -- The choice may outlive a quickly closed reader. Preserve the accepted
        -- state, but do not acknowledge an exact pull without live readback.
        stagePagingPercent(plugin.ui, doc_settings, after_percent)
        doc_settings:flush()
        reading_state_sync:discardOpenPositionVerification(document.file)
    end
end

--- KOReader emits DocSettingsLoad after all reader plugins are instantiated and
--- before ReadSettings. Silent pulls therefore update the settings ReaderRolling
--- is about to consume. PROMPT pulls must be asynchronous: blocking/yielding here
--- would let ReadSettings continue before the user's answer. Their callback
--- applies the accepted state to the already-live ReaderUI instead.
function KindlePlugin:onDocSettingsLoad(doc_settings, document)
    if not self.settings.sync_reading_state or not reading_state_sync:isAutomaticSyncEnabled() then
        return
    end

    local book = getMappedBook(document)
    if not book or not book.source_path then
        return
    end

    local cde_key = getBookCdeKey(book, doc_settings)
    local before_sync_percent = doc_settings:readSetting("percent_finished") or 0
    local approval_handler = function(plugin, sync_direction, is_pull_from_kindle, is_newer, sync_fn, sync_details)
        local setting = configuredDirection(self.settings, is_pull_from_kindle, is_newer)
        if setting ~= SYNC_DIRECTION.PROMPT then
            return SyncDecisionMaker.syncIfApproved(plugin, sync_direction, is_pull_from_kindle, is_newer, sync_fn, sync_details)
        end

        UIManager:nextTick(function()
            SyncDecisionMaker.syncIfApproved(plugin, sync_direction, is_pull_from_kindle, is_newer, function()
                applyPullToLiveReader(self, document, doc_settings, sync_fn)
            end, sync_details, true)
        end)
        return true
    end
    local conflict_handler = function(details, use_kindle_fn, use_koreader_fn)
        UIManager:nextTick(function()
            SyncDecisionMaker.promptForConflict(details, function()
                applyPullToLiveReader(self, document, doc_settings, use_kindle_fn)
            end, use_koreader_fn, true)
        end)
        return true
    end

    reading_state_sync:syncFromKindleAutomatic(cde_key, book.source_path, doc_settings, document.file, approval_handler, conflict_handler)

    -- ReaderPaging restores last_page, not percent_finished. Silent pulls run
    -- before ReadSettings, so stage the equivalent page using the same floor +
    -- clamp semantics as ReaderPaging:onGotoPercent().
    local after_sync_percent = doc_settings:readSetting("percent_finished") or 0
    if after_sync_percent ~= before_sync_percent then
        stagePagingPercent(self.ui, doc_settings, after_sync_percent)
    end
end

--- ReaderReady runs after ReaderRolling has restored and rendered last_xpointer.
--- This is the earliest native lifecycle point where an automatic exact pull can
--- be acknowledged without confusing "requested" with "actually displayed".
function KindlePlugin:onReaderReady()
    if self.ui and self.ui.document then
        reading_state_sync:verifyOpenedKOReaderPosition(self.ui, self.ui.document.file)
    end
end

local function runPendingCloseSync(pending)
    -- The same document may have been reopened before the deferred push ran
    -- (fast back-to-back open). That session owns its position now; its own
    -- close will push, and receipt reconciliation covers the gap.
    local ReaderUI = require("apps/reader/readerui")
    local active = ReaderUI.instance and ReaderUI.instance.document
    if active and active.file == pending.epub_path then
        logger.info("KindlePlugin: skipping deferred close sync; document reopened")
        return
    end

    local approval_handler = function(plugin, sync_direction, is_pull_from_kindle, is_newer, sync_fn, sync_details)
        local is_prompt = configuredDirection(plugin.settings, is_pull_from_kindle, is_newer) == SYNC_DIRECTION.PROMPT
        if is_prompt then
            UIManager:nextTick(function()
                SyncDecisionMaker.syncIfApproved(plugin, sync_direction, is_pull_from_kindle, is_newer, sync_fn, sync_details, true)
            end)
            return true
        end
        return SyncDecisionMaker.syncIfApproved(plugin, sync_direction, is_pull_from_kindle, is_newer, sync_fn, sync_details)
    end
    local conflict_handler = function(details, use_kindle_fn, use_koreader_fn)
        UIManager:nextTick(function()
            SyncDecisionMaker.promptForConflict(details, use_kindle_fn, use_koreader_fn, true)
        end)
        return true
    end

    -- Close sync is fully in-process (position map + KRDS sidecar codec),
    -- so it runs to completion without spawning the bundled helper.
    local ok, err = pcall(function()
        reading_state_sync:syncToKindleAutomatic(
            pending.cde_key,
            pending.source_path,
            pending.doc_settings,
            pending.epub_path,
            approval_handler,
            conflict_handler
        )
    end)
    if not ok then
        logger.warn("KindlePlugin: deferred close sync failed:", err)
    end
end

function KindlePlugin:syncPendingClose()
    local pending = self._pending_close_sync
    if not pending then
        return
    end
    self._pending_close_sync = nil

    -- Run the push after the reader widget has closed so the exit gesture
    -- returns to the library immediately. An interrupted run is recovered by
    -- the receipt system on the next open.
    UIManager:scheduleIn(0.1, function()
        runPendingCloseSync(pending)
    end)
end

--- CloseDocument happens before the final UIManager-driven SaveSettings in the
--- normal ReaderUI teardown. Capture identity here, then push from onSaveSettings
--- after ReaderRolling has written its final XPointer/percent. In the uncommon
--- ReaderUI path that saves before CloseDocument, push immediately instead.
function KindlePlugin:onCloseDocument()
    if
        not self.settings.sync_reading_state
        or not reading_state_sync:isAutomaticSyncEnabled()
        or not self.ui
        or not self.ui.document
        or not self.ui.doc_settings
    then
        return
    end

    local book = getMappedBook(self.ui.document)
    if not book or not book.source_path then
        return
    end

    self._pending_close_sync = {
        cde_key = getBookCdeKey(book, self.ui.doc_settings),
        source_path = book.source_path,
        doc_settings = self.ui.doc_settings,
        epub_path = self.ui.document.file,
    }

    if self.ui.dialog ~= self.ui then
        self:syncPendingClose()
    end
end

--- ReaderRolling is registered before plugins, so its onSaveSettings handler has
--- already captured the final reading position when this plugin sees the event.
function KindlePlugin:onSaveSettings()
    self:syncPendingClose()
end

function KindlePlugin:stopPlugin()
    local FileChooser = require("ui/widget/filechooser")
    FileChooserExt:unapply(FileChooser)
    OpenFileExt:unapply()
    kindle_library:close()
    if self.ui and self.ui.file_chooser and self.ui.file_chooser.refreshPath then
        self.ui.file_chooser:refreshPath()
    end
    return true
end

--- Loads plugin settings from persistent storage.
function KindlePlugin:loadSettings()
    self.settings = G_reader_settings:readSetting(self.settings_key) or {}

    -- v0.0.4 and earlier could persist the legacy synthetic URI as KOReader's
    -- HOME. It is not a filesystem path, so repair that setting.
    local home_dir = G_reader_settings:readSetting("home_dir")
    if type(home_dir) == "string" and home_dir:sub(1, 17) == "KINDLE_VIRTUAL://" then
        G_reader_settings:saveSetting("home_dir", Device.home_dir)
    end

    for key, value in pairs(self.default_settings) do
        if self.settings[key] == nil then
            self.settings[key] = value
        end
    end

    helper_client:setSettings(self.settings)
    library_index:setSettings(self.settings)
    virtual_library:setSettings(self.settings)
    cache_manager:setSettings(self.settings)
end

--- Saves plugin settings to persistent storage.
function KindlePlugin:saveSettings()
    G_reader_settings:saveSetting(self.settings_key, self.settings)
end

--- Shows an InfoMessage to the user.
--- @param text string: Message text.
--- @param timeout number|nil: Auto-dismiss timeout in seconds.
function KindlePlugin:showInfo(text, timeout)
    UIManager:show(InfoMessage:new({
        text = text,
        timeout = timeout,
    }))
end

-- ---------------------------------------------------------------------------
-- Menu item builders (extracted methods, matching kobo.koplugin pattern)
-- ---------------------------------------------------------------------------

--- Creates virtual library enable/disable menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createVirtualLibraryToggleMenuItem()
    return {
        text = _("Virtual Library Enabled"),
        checked_func = function()
            return self.settings.enable_virtual_library ~= false
        end,
        callback = function()
            self.settings.enable_virtual_library = self.settings.enable_virtual_library == false
            self:saveSettings()
            UIManager:askForRestart()
        end,
        separator = true,
    }
end

--- Creates virtual library cover path configuration menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createVirtualLibraryCoverMenuItem()
    return {
        text = _("Virtual Library Folder Cover"),
        help_text = _(
            "Select a custom cover image for the virtual library folder. "
                .. "Used by CoverBrowser plugin. "
                .. "If not set, no cover will be shown (falls back to generated covers)."
        ),
        enabled_func = function()
            return self.settings.enable_virtual_library ~= false
        end,
        callback = function()
            local path_chooser = PathChooser:new({
                select_file = true,
                select_directory = false,
                path = self.settings.virtual_library_cover_path ~= "" and util.splitFilePathName(self.settings.virtual_library_cover_path)
                    or Device.home_dir
                    or "/mnt/us",
                onConfirm = function(file_path)
                    self.settings.virtual_library_cover_path = file_path
                    self:saveSettings()
                    self:showInfo(T(_("Cover set to: %1"), file_path), 2)
                end,
            })
            UIManager:show(path_chooser)
        end,
    }
end

--- Creates sync enable/disable menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createSyncToggleMenuItem()
    return {
        text = _("Sync reading state with Kindle"),
        checked_func = function()
            return self.settings.sync_reading_state == true
        end,
        callback = function()
            local enabled = not self.settings.sync_reading_state
            self.settings.sync_reading_state = enabled
            reading_state_sync:setEnabled(enabled)
            self:saveSettings()
            self:showInfo(
                enabled and _("Reading state sync enabled\n\nKOReader and Kindle reading positions will be synced.")
                    or _("Reading state sync disabled"),
                4
            )
        end,
        separator = true,
    }
end

--- Creates manual sync menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createManualSyncMenuItem()
    return {
        text = _("Sync all books now"),
        enabled_func = function()
            return self.settings.sync_reading_state == true
        end,
        callback = function()
            if not reading_state_sync:isEnabled() then
                self:showInfo(_("Reading progress sync is not enabled."))
                return
            end
            reading_state_sync:syncAllBooksManual()
        end,
        separator = true,
    }
end

--- Creates sync direction choice submenu.
--- @param direction_key string: Settings key ('sync_from_kindle_newer', etc.).
--- @param label string: Menu label (may contain %1 for current direction name).
--- @param help_text string|nil: Help text.
--- @return table: Menu item configuration.
function KindlePlugin:createSyncDirectionChoiceMenu(direction_key, label, help_text)
    return {
        text_func = function()
            return T(label, getNameDirection(self.settings[direction_key]))
        end,
        help_text = help_text,
        sub_item_table = {
            {
                text = _("Always sync"),
                checked_func = function()
                    return self.settings[direction_key] == SYNC_DIRECTION.SILENT
                end,
                callback = function()
                    self.settings[direction_key] = SYNC_DIRECTION.SILENT
                    self:saveSettings()
                end,
            },
            {
                text = _("Ask me"),
                checked_func = function()
                    return self.settings[direction_key] == SYNC_DIRECTION.PROMPT
                end,
                callback = function()
                    self.settings[direction_key] = SYNC_DIRECTION.PROMPT
                    self:saveSettings()
                end,
            },
            {
                text = _("Never"),
                checked_func = function()
                    return self.settings[direction_key] == SYNC_DIRECTION.NEVER
                end,
                callback = function()
                    self.settings[direction_key] = SYNC_DIRECTION.NEVER
                    self:saveSettings()
                end,
            },
        },
    }
end

--- Creates FROM Kindle sync settings submenu.
--- @return table: Menu item configuration.
function KindlePlugin:createFromKindleSyncSettingsMenu()
    return {
        text = _("FROM Kindle sync settings"),
        enabled_func = function()
            return self.settings.enable_sync_from_kindle == true
        end,
        sub_item_table = {
            self:createSyncDirectionChoiceMenu(
                "sync_from_kindle_newer",
                _("Sync to a newer state (%1)"),
                _("What to do when Kindle has newer progress than KOReader.")
            ),
            self:createSyncDirectionChoiceMenu(
                "sync_from_kindle_older",
                _("Sync to an older state (%1)"),
                _("What to do when Kindle has older progress than KOReader.")
            ),
        },
    }
end

--- Creates TO Kindle sync settings submenu.
--- @return table: Menu item configuration.
function KindlePlugin:createToKindleSyncSettingsMenu()
    return {
        text = _("TO Kindle sync settings"),
        enabled_func = function()
            return self.settings.enable_sync_to_kindle == true
        end,
        sub_item_table = {
            self:createSyncDirectionChoiceMenu(
                "sync_to_kindle_newer",
                _("Sync to a newer state (%1)"),
                _("What to do when KOReader has newer progress than Kindle.")
            ),
            self:createSyncDirectionChoiceMenu(
                "sync_to_kindle_older",
                _("Sync to an older state (%1)"),
                _("What to do when KOReader has older progress than Kindle.")
            ),
        },
    }
end

--- Creates sync behavior menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createSyncBehaviorMenuItem()
    return {
        text = _("Sync behavior"),
        enabled_func = function()
            return self.settings.sync_reading_state == true
        end,
        sub_item_table = {
            {
                text = _("Automatic sync on book open and close"),
                checked_func = function()
                    return self.settings.enable_auto_sync == true
                end,
                callback = function()
                    self.settings.enable_auto_sync = not self.settings.enable_auto_sync
                    self:saveSettings()
                end,
                separator = true,
            },
            {
                text = _("Enable sync FROM Kindle TO KOReader"),
                checked_func = function()
                    return self.settings.enable_sync_from_kindle == true
                end,
                callback = function()
                    self.settings.enable_sync_from_kindle = not self.settings.enable_sync_from_kindle
                    self:saveSettings()
                end,
            },
            {
                text = _("Enable sync FROM KOReader TO Kindle"),
                checked_func = function()
                    return self.settings.enable_sync_to_kindle == true
                end,
                callback = function()
                    self.settings.enable_sync_to_kindle = not self.settings.enable_sync_to_kindle
                    self:saveSettings()
                end,
                separator = true,
            },
            self:createFromKindleSyncSettingsMenu(),
            self:createToKindleSyncSettingsMenu(),
        },
        separator = true,
    }
end

--- Creates clear book keys menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createClearKeysMenuItem()
    return {
        text = _("Clear Book Keys"),
        help_text = _("Removes cached book access keys. Required keys will be extracted again when a book is next opened."),
        callback = function()
            local keys_path = cache_manager:getDrmKeysPath()
            local f = io.open(keys_path, "rb")
            if not f then
                self:showInfo(_("No book keys found."), 2)
                return
            end
            f:close()

            UIManager:show(ConfirmBox:new({
                text = _("Clear all cached book keys? Required keys will be extracted again when a book is next opened."),
                ok_text = _("Clear keys"),
                ok_callback = function()
                    local ok, err = os.remove(keys_path)
                    if not ok then
                        self:showInfo(_("Failed to clear book keys:\n") .. (err or _("unknown error")))
                        return
                    end
                    self:showInfo(_("Book keys cleared."), 2)
                end,
            }))
        end,
    }
end

--- Creates clear cache menu item with stats confirmation.
--- @return table: Menu item configuration.
function KindlePlugin:createClearCacheMenuItem()
    return {
        text = _("Clear Kindle Cache"),
        help_text = _(
            "Removes cached converted EPUBs and position metadata. Book access keys are preserved. " .. "Books will be re-converted on next access."
        ),
        callback = function()
            local stats = cache_manager:getCacheStats()

            if stats.count == 0 then
                self:showInfo(_("Cache is already empty."), 2)
                return
            end

            UIManager:show(ConfirmBox:new({
                text = T(_("Clear %1 cached books (%2)?"), stats.count, util.getFriendlySize(stats.total_size)),
                ok_text = _("Clear cache"),
                ok_callback = function()
                    local ok, err = cache_manager:clearAllCache()
                    if not ok then
                        self:showInfo(_("Failed to clear cache:\n") .. (err or _("unknown error")))
                        return
                    end
                    self:showInfo(T(_("Cleared %1 books from cache."), stats.count), 3)
                end,
            }))
        end,
    }
end

--- Creates documents root directory picker menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createDocumentsRootMenuItem()
    return {
        text_func = function()
            return T(_("Documents root: %1"), self.settings.documents_root or default_settings.documents_root)
        end,
        callback = function()
            local path_chooser = PathChooser:new({
                title = _("Select documents root directory"),
                select_file = false,
                select_directory = true,
                path = self.settings.documents_root or default_settings.documents_root,
                onConfirm = function(path)
                    if not path or path == "" then
                        return
                    end
                    self.settings.documents_root = path
                    self:saveSettings()
                    self:showInfo(T(_("Documents root set to:\n%1\n\nRestart KOReader to apply."), path), 4)
                end,
            })
            UIManager:show(path_chooser)
        end,
    }
end

--- Creates cache directory info menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createCacheInfoMenuItem()
    return {
        text_func = function()
            return T(_("Cache: %1"), self.settings.cache_dir or default_settings.cache_dir)
        end,
        enabled_func = function()
            return false
        end,
        separator = true,
    }
end

--- Creates about menu item showing library statistics.
--- @return table: Menu item configuration.
function KindlePlugin:createAboutMenuItem()
    return {
        text = _("About Kindle Library"),
        callback = function()
            local books, _ = library_index:getBooks(false)
            local total = books and #books or 0
            local drm_count = 0
            local convert_count = 0
            local direct_count = 0
            local blocked_count = 0

            if books then
                for _, book in ipairs(books) do
                    if book.open_mode == "drm" then
                        drm_count = drm_count + 1
                    elseif book.open_mode == "convert" then
                        convert_count = convert_count + 1
                    elseif book.open_mode == "direct" then
                        direct_count = direct_count + 1
                    elseif book.open_mode == "blocked" then
                        blocked_count = blocked_count + 1
                    end
                end
            end

            local cache_stats = cache_manager:getCacheStats()

            local msg = string.format(
                _([[Kindle Virtual Library

Total books: %d
  DRM-protected: %d
  Convertible: %d
  Direct open: %d
  Blocked: %d

Cached EPUBs: %d (%s)
Root: %s
Cache: %s]]),
                total,
                drm_count,
                convert_count,
                direct_count,
                blocked_count,
                cache_stats.count,
                util.getFriendlySize(cache_stats.total_size),
                self.settings.documents_root or default_settings.documents_root,
                self.settings.cache_dir or default_settings.cache_dir
            )

            UIManager:show(InfoMessage:new({ text = msg }))
        end,
    }
end

--- Creates refresh library menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createRefreshLibraryMenuItem()
    return {
        text = _("Refresh Kindle Index"),
        enabled_func = function()
            return self.settings.enable_virtual_library ~= false
        end,
        callback = function()
            local _, err = virtual_library:refresh(true)
            if err then
                self:showInfo(_("Failed to refresh Kindle library:\n") .. err)
                return
            end
            self:showInfo(_("Kindle library refreshed."), 2)
        end,
    }
end

--- Creates browse virtual library menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createBrowseLibraryMenuItem()
    return {
        text = _("Browse Kindle Library"),
        enabled_func = function()
            return self.settings.enable_virtual_library ~= false and self.ui and not self.ui.document
        end,
        callback = function()
            if self.ui and not self.ui.document then
                kindle_library:setUI(self.ui)
                kindle_library:show(self.ui, true)
                return
            end
            self:showInfo(_("Open the file browser to access Kindle Library."))
        end,
    }
end

-- ---------------------------------------------------------------------------
-- Main menu registration
-- ---------------------------------------------------------------------------

--- Adds plugin menu items to the file manager main menu.
--- @param menu_items table: Main menu items table to populate.
function KindlePlugin:addToMainMenu(menu_items)
    if self.ui.document then
        return
    end

    local sub_item_table = {
        self:createBrowseLibraryMenuItem(),
        self:createRefreshLibraryMenuItem(),
        self:createClearKeysMenuItem(),
        self:createClearCacheMenuItem(),
        self:createVirtualLibraryToggleMenuItem(),
        self:createVirtualLibraryCoverMenuItem(),
        self:createSyncToggleMenuItem(),
        self:createManualSyncMenuItem(),
        self:createSyncBehaviorMenuItem(),
        self:createDocumentsRootMenuItem(),
        self:createCacheInfoMenuItem(),
        self:createAboutMenuItem(),
    }

    menu_items.kindle_plugin = {
        text = _("Kindle Library"),
        sorting_hint = "more_tools",
        separator = true,
        sub_item_table = sub_item_table,
    }
end

--- Called when settings need to be flushed.
function KindlePlugin:onFlushSettings()
    self:saveSettings()
end

if Device.isKindle ~= nil and Device:isKindle() == false then
    logger.info("KindlePlugin: running on a non-Kindle device")
end

return KindlePlugin
