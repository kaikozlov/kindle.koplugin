local DataStorage = require("datastorage")
local Device = require("device")
local InfoMessage = require("ui/widget/infomessage")
local UIManager = require("ui/uimanager")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local _ = require("gettext")
local logger = require("logger")

local CacheManager = require("lua/cache_manager")
local DocSettingsExt = require("lua/docsettings_ext")
local DocumentExt = require("lua/document_ext")
local FileChooserExt = require("lua/filechooser_ext")
local HelperClient = require("lua/helper_client")
local LibraryIndex = require("lua/library_index")
local BookInfoManagerExt = require("lua/bookinfomanager_ext")
local ShowReaderExt = require("lua/showreader_ext")
local VirtualLibrary = require("lua/virtual_library")

local default_settings = {
    enable_virtual_library = true,
    documents_root = "/mnt/us/documents",
    cache_dir = DataStorage:getFullDataDir() .. "/cache/kindle.koplugin",
    index_ttl_seconds = 300,
    last_scan_at = 0,
    drm_initialized = false,
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
virtual_library:setCacheManager(cache_manager)

local function applyShowReaderExtensions()
    local ext = ShowReaderExt
    ext:init(virtual_library)
    ext:apply()
end

local function applyDocumentExtensions()
    local DocumentRegistry = require("document/documentregistry")
    local ext = DocumentExt
    ext:init(virtual_library, cache_manager)
    ext:apply(DocumentRegistry)
end

local function applyDocSettingsExtensions()
    local DocSettings = require("docsettings")
    local ext = DocSettingsExt
    ext:init(virtual_library)
    ext:apply(DocSettings)
end

local function applyFileChooserExtensions()
    local FileChooser = require("ui/widget/filechooser")
    local ext = FileChooserExt
    ext:init(virtual_library, cache_manager)
    ext:apply(FileChooser)
end

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

local plugin_settings = G_reader_settings:readSetting("kindle_plugin") or cloneDefaults()
helper_client:setSettings(plugin_settings)
library_index:setSettings(plugin_settings)
virtual_library:setSettings(plugin_settings)
cache_manager:setSettings(plugin_settings)

if plugin_settings.enable_virtual_library ~= false then
    logger.info("KindlePlugin: enabling virtual library patches")
    applyShowReaderExtensions()
    applyDocumentExtensions()
    applyDocSettingsExtensions()
    applyFileChooserExtensions()
    applyBookInfoManagerExtensions()
end

local KindlePlugin = WidgetContainer:extend({
    name = "kindle_plugin",
    is_doc_only = false,
    default_settings = default_settings,
})

function KindlePlugin:init()
    self:loadSettings()
    self.ui.menu:registerToMainMenu(self)
end

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

function KindlePlugin:saveSettings()
    G_reader_settings:saveSetting("kindle_plugin", self.settings)
end

function KindlePlugin:showInfo(text, timeout)
    UIManager:show(InfoMessage:new({
        text = text,
        timeout = timeout,
    }))
end

function KindlePlugin:refreshLibrary()
    local _, err = virtual_library:refresh(true)
    self.settings.last_scan_at = os.time()
    self:saveSettings()

    if err then
        self:showInfo(_("Failed to refresh Kindle library:\n") .. err)
        return
    end

    self:showInfo(_("Kindle library refreshed."), 2)
end

function KindlePlugin:openVirtualLibrary()
    if self.ui and self.ui.file_chooser and self.ui.file_chooser.showKindleVirtualLibrary then
        self.ui.file_chooser:showKindleVirtualLibrary()
        return
    end

    self:showInfo(_("Open the file browser to access Kindle Library."))
end

function KindlePlugin:clearAllCache()
    local ok, err = cache_manager:clearAllCache()
    if not ok then
        self:showInfo(_("Failed to clear cache:\n") .. (err or _("unknown error")))
        return
    end

    self:showInfo(_("Kindle cache cleared."), 2)
end

function KindlePlugin:setupDRM()
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

        -- Refresh the library to pick up new DRM book statuses
        virtual_library:refresh(true)

        local msg = _("DRM setup complete.\n")
        msg = msg .. string.format(_("Books found: %d\nKeys extracted: %d"), result.books_found, result.keys_found)

        UIManager:show(InfoMessage:new({
            text = msg,
            timeout = 5,
        }))
    end)
end

function KindlePlugin:showAbout()
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

    local msg = string.format(
        _([[Kindle Virtual Library

Total books: %d
  DRM-protected: %d
  Convertible: %d
  Direct open: %d
  Blocked: %d

DRM: %s
Root: %s
Cache: %s]]),
        total, drm_count, convert_count, direct_count, blocked_count,
        drm_status,
        self.settings.documents_root or default_settings.documents_root,
        self.settings.cache_dir or default_settings.cache_dir
    )

    UIManager:show(InfoMessage:new({ text = msg }))
end

function KindlePlugin:addToMainMenu(menu_items)
    menu_items.kindle_plugin = {
        text = _("Kindle Library"),
        sorting_hint = "more_tools",
        sub_item_table = {
            {
                text = _("Browse Kindle Library"),
                callback = function()
                    self:openVirtualLibrary()
                end,
            },
            {
                text = _("Refresh Kindle Index"),
                callback = function()
                    self:refreshLibrary()
                end,
            },
            {
                text = _("Setup DRM Decryption"),
                help_text = _("Extract decryption keys from the device to open DRM-protected Kindle books."),
                callback = function()
                    self:setupDRM()
                end,
                separator = true,
            },
            {
                text = _("Clear Kindle Cache"),
                callback = function()
                    self:clearAllCache()
                end,
            },
            {
                text = _("Virtual Library Enabled"),
                checked_func = function()
                    return self.settings.enable_virtual_library ~= false
                end,
                callback = function()
                    self.settings.enable_virtual_library = not (self.settings.enable_virtual_library ~= false)
                    self:saveSettings()
                    self:showInfo(
                        _("Restart KOReader to apply the new Kindle virtual library setting."),
                        3
                    )
                end,
            },
            {
                text_func = function()
                    local root = self.settings.documents_root or default_settings.documents_root
                    return _("Root: ") .. root
                end,
                enabled_func = function()
                    return false
                end,
            },
            {
                text_func = function()
                    local cache_dir = self.settings.cache_dir or default_settings.cache_dir
                    return _("Cache: ") .. cache_dir
                end,
                enabled_func = function()
                    return false
                end,
                separator = true,
            },
            {
                text = _("About Kindle Library"),
                callback = function()
                    self:showAbout()
                end,
            },
        },
    }
end

function KindlePlugin:onFlushSettings()
    self:saveSettings()
end

if Device.isKindle ~= nil and Device:isKindle() == false then
    logger.info("KindlePlugin: running on a non-Kindle device")
end

return KindlePlugin
