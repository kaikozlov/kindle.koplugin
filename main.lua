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
local DocSettingsExt = require("lua/docsettings_ext")
local DocumentExt = require("lua/document_ext")
local FileChooserExt = require("lua/filechooser_ext")
local FilesystemExt = require("lua/filesystem_ext")
local HelperClient = require("lua/helper_client")
local LibraryIndex = require("lua/library_index")
local BookInfoManagerExt = require("lua/bookinfomanager_ext")
local PathChooserExt = require("lua/pathchooser_ext")
local ReaderUIExt = require("lua/readerui_ext")
local ReadingStateSync = require("lua/reading_state_sync")
local ShowReaderExt = require("lua/showreader_ext")
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
    last_scan_at = 0,
    drm_initialized = false,
    sync_reading_state = false,
    enable_auto_sync = true,
    enable_sync_from_kindle = false,
    enable_sync_to_kindle = true,
    sync_from_kindle_newer = SYNC_DIRECTION.PROMPT,
    sync_from_kindle_older = SYNC_DIRECTION.NEVER,
    sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
    sync_to_kindle_older = SYNC_DIRECTION.NEVER,
}

local function cloneDefaults()
    local copy = {}
    for key, value in pairs(default_settings) do
        copy[key] = value
    end
    return copy
end

local helper_client = HelperClient:new()
local library_index = LibraryIndex:new(helper_client)
local virtual_library = VirtualLibrary:new(library_index)
local cache_manager = CacheManager:new(helper_client, virtual_library)
local reading_state_sync = ReadingStateSync:new(helper_client)
virtual_library:setCacheManager(cache_manager)

--- Applies ShowReader extensions for virtual library support.
local function applyShowReaderExtensions()
    local ext = ShowReaderExt
    ext:init(virtual_library, reading_state_sync)
    ext:apply()
end

--- Applies document provider extensions for KFX/AZW files.
local function applyDocumentExtensions()
    local DocumentRegistry = require("document/documentregistry")
    local ext = DocumentExt
    ext:init(virtual_library, cache_manager)
    ext:apply(DocumentRegistry)
end

--- Applies DocSettings extensions for sidecar file support.
local function applyDocSettingsExtensions()
    local DocSettings = require("docsettings")
    local ext = DocSettingsExt
    ext:init(virtual_library)
    ext:apply(DocSettings)
end

--- Applies FileChooser extensions for virtual library.
local function applyFileChooserExtensions()
    local FileChooser = require("ui/widget/filechooser")
    local ext = FileChooserExt
    ext:init(virtual_library, cache_manager, reading_state_sync)
    ext:apply(FileChooser)
end

--- Applies BookInfoManager extensions for CoverBrowser integration.
local function applyBookInfoManagerExtensions()
    local ok, BookInfoManager = pcall(require, "bookinfomanager")
    if ok and BookInfoManager then
        local ext = BookInfoManagerExt
        ext:init(virtual_library, cache_manager)
        ext:apply(BookInfoManager)
        ext:clearStaleVirtualEntries(BookInfoManager)
    else
        logger.dbg("KindlePlugin: CoverBrowser plugin not loaded, skipping BookInfoManager patches")
    end
end

--- Applies filesystem extensions for virtual path support.
local function applyFilesystemExtensions()
    local ext = FilesystemExt
    ext:init(virtual_library)
    ext:apply()
end

--- Applies ReaderUI extensions for Kindle navigation on close.
local function applyReaderUIExtensions()
    local ok_rui, ReaderUI = pcall(require, "apps/reader/readerui")
    if not ok_rui or not ReaderUI then
        return
    end

    local ext = ReaderUIExt
    ext:init(virtual_library, reading_state_sync)
    ext:apply(ReaderUI)
    logger.info("KindlePlugin: ReaderUI patches applied")
end

--- Applies PathChooser extensions for bypassing virtual library.
local function applyPathChooserExtensions()
    local ext = PathChooserExt
    ext:init({ virtual_library = virtual_library })
    ext:apply()
end

local plugin_settings = G_reader_settings:readSetting("kindle_plugin") or cloneDefaults()
helper_client:setSettings(plugin_settings)
library_index:setSettings(plugin_settings)
virtual_library:setSettings(plugin_settings)
cache_manager:setSettings(plugin_settings)

if plugin_settings.enable_virtual_library ~= false then
    logger.info("KindlePlugin: enabling virtual library patches")
    applyFilesystemExtensions()
    applyShowReaderExtensions()
    applyDocumentExtensions()
    applyDocSettingsExtensions()
    applyFileChooserExtensions()
    applyBookInfoManagerExtensions()
    applyReaderUIExtensions()
    applyPathChooserExtensions()
end

local KindlePlugin = WidgetContainer:extend({
    name = "kindle_plugin",
    is_doc_only = false,
    default_settings = default_settings,
})

--- Initializes the plugin and loads settings.
function KindlePlugin:init()
    self:loadSettings()
    self.ui.menu:registerToMainMenu(self)

    reading_state_sync:setPlugin(self, SYNC_DIRECTION)
    if self.settings.sync_reading_state then
        reading_state_sync:setEnabled(true)
    end
end

--- Loads plugin settings from persistent storage.
function KindlePlugin:loadSettings()
    self.settings = G_reader_settings:readSetting("kindle_plugin") or {}

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
    G_reader_settings:saveSetting("kindle_plugin", self.settings)
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
            self.settings.enable_virtual_library = not (self.settings.enable_virtual_library ~= false)
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
                path = self.settings.virtual_library_cover_path ~= ""
                        and util.splitFilePathName(self.settings.virtual_library_cover_path)
                    or Device.home_dir or "/mnt/us",
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
                enabled
                        and _("Reading state sync enabled\n\nKOReader and Kindle reading positions will be synced.")
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

--- Creates DRM setup menu item.
--- @return table: Menu item configuration.
function KindlePlugin:createDrmSetupMenuItem()
    return {
        text = _("Setup DRM Decryption"),
        help_text = _(
            "Extract decryption keys from the device to open DRM-protected Kindle books. "
                .. "This runs the device JVM with an interception hook to capture AES keys."
        ),
        callback = function()
            self:showInfo(_("Setting up DRM decryption…\nThis may take a moment."), 0)
            UIManager:scheduleIn(0.1, function()
                local result, err = helper_client:drmInit()
                if not result then
                    UIManager:show(InfoMessage:new({
                        text = _("DRM setup failed:\n") .. (err or _("unknown error")),
                        timeout = 5,
                    }))
                    return
                end
                if not result.ok then
                    UIManager:show(InfoMessage:new({
                        text = _("DRM setup failed:\n") .. (result.message or _("unknown error")),
                        timeout = 5,
                    }))
                    return
                end
                self.settings.drm_initialized = true
                self:saveSettings()
                virtual_library:refresh(true)
                local msg = _("DRM setup complete.\n")
                    .. string.format(_("Books found: %d\nKeys extracted: %d"), result.books_found, result.keys_found)
                UIManager:show(InfoMessage:new({ text = msg, timeout = 5 }))
            end)
        end,
        separator = true,
    }
end

--- Creates clear cache menu item with stats confirmation.
--- @return table: Menu item configuration.
function KindlePlugin:createClearCacheMenuItem()
    return {
        text = _("Clear Kindle Cache"),
        help_text = _("Removes all cached converted EPUBs. Books will be re-converted on next access."),
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

            local drm_status = _("Not set up")
            if self.settings.drm_initialized then
                drm_status = _("Ready")
            end

            local cache_stats = cache_manager:getCacheStats()

            local msg = string.format(
                _([[Kindle Virtual Library

Total books: %d
  DRM-protected: %d
  Convertible: %d
  Direct open: %d
  Blocked: %d

DRM: %s
Cached EPUBs: %d (%s)
Root: %s
Cache: %s]]),
                total, drm_count, convert_count, direct_count, blocked_count,
                drm_status,
                cache_stats.count, util.getFriendlySize(cache_stats.total_size),
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
        callback = function()
            local _, err = virtual_library:refresh(true)
            self.settings.last_scan_at = os.time()
            self:saveSettings()
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
        callback = function()
            if self.ui and self.ui.file_chooser and self.ui.file_chooser.showKindleVirtualLibrary then
                self.ui.file_chooser:showKindleVirtualLibrary()
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
    if not virtual_library:isActive() then
        return
    end

    if self.ui.document then
        return
    end

    local sub_item_table = {
        self:createBrowseLibraryMenuItem(),
        self:createRefreshLibraryMenuItem(),
        self:createDrmSetupMenuItem(),
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
