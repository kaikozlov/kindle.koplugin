local DataStorage = require("datastorage")
local Device = require("device")
local InfoMessage = require("ui/widget/infomessage")
local UIManager = require("ui/uimanager")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local _ = require("gettext")
local logger = require("logger")

local CacheManager = require("src/cache_manager")
local DocSettingsExt = require("src/docsettings_ext")
local DocumentExt = require("src/document_ext")
local FileChooserExt = require("src/filechooser_ext")
local HelperClient = require("src/helper_client")
local LibraryIndex = require("src/library_index")
local VirtualLibrary = require("src/virtual_library")

local default_settings = {
    enable_virtual_library = true,
    documents_root = "/mnt/us/documents",
    cache_dir = DataStorage:getFullDataDir() .. "/cache/kindle.koplugin",
    index_ttl_seconds = 300,
    last_scan_at = 0,
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

local plugin_settings = G_reader_settings:readSetting("kindle_plugin") or cloneDefaults()
helper_client:setSettings(plugin_settings)
library_index:setSettings(plugin_settings)
virtual_library:setSettings(plugin_settings)
cache_manager:setSettings(plugin_settings)

if plugin_settings.enable_virtual_library ~= false then
    logger.info("KindlePlugin: enabling virtual library patches")
    applyDocumentExtensions()
    applyDocSettingsExtensions()
    applyFileChooserExtensions()
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
