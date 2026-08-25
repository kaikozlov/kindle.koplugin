require("busted.runner")()

--- Smoke-test the plugin through KOReader's real PluginLoader and FileManager.
--- Keep the virtual library disabled here so the test verifies widget lifecycle
--- without leaving global monkey patches installed for the remaining specs.
describe("KindlePlugin native KOReader lifecycle", function()
    local UIManager = require("ui/uimanager")
    local DataStorage = require("datastorage")
    local FileManager = require("apps/filemanager/filemanager")
    local PluginLoader = require("pluginloader")
    local Screen = require("device").screen
    local filemanager

    before_each(function()
        disable_plugins()
        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = false,
        })
    end)

    after_each(function()
        local instance = PluginLoader:getPluginInstance("kindle")
        if instance and instance.stopPlugin then
            pcall(instance.stopPlugin, instance)
        end
        if filemanager then
            filemanager:onClose()
            filemanager = nil
        end
        G_reader_settings:delSetting("kindle_plugin")
        UIManager:quit()
    end)

    it("is discovered by the real PluginLoader", function()
        local discovered
        for _, plugin in ipairs(PluginLoader:_discover()) do
            if plugin.name == "kindle" or plugin.name == "kindle.koplugin" then
                discovered = plugin
                break
            end
        end

        assert.is_truthy(discovered, "kindle.koplugin should be discovered")
        assert.is_false(discovered.disabled)
        assert.is_truthy(discovered.main:match("kindle%.koplugin/main%.lua$"))
    end)

    it("instantiates through FileManager and registers its menu", function()
        load_plugin("kindle.koplugin")

        filemanager = FileManager:new({
            dimen = Screen:getSize(),
            root_path = DataStorage:getDataDir(),
        })
        UIManager:show(filemanager)
        fastforward_ui_events()

        local instance = PluginLoader:getPluginInstance("kindle")
        assert.is_truthy(instance, "kindle.koplugin should instantiate")
        assert.is_truthy(instance.ui)
        assert.is_truthy(instance.ui.menu)

        local menu_items = {}
        instance:addToMainMenu(menu_items)
        assert.is_truthy(menu_items.kindle_plugin)
        assert.is_truthy(menu_items.kindle_plugin.sub_item_table)

        -- KOReader recreates plugin widgets while switching views. Verify a
        -- second real WidgetContainer instance receives ui before init and
        -- reads current settings without relying on the first instance.
        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = false,
            cache_dir = "/tmp/recreated-kindle-cache",
        })
        local KindlePlugin = getmetatable(instance)
        local registered = false
        local recreated = KindlePlugin:new({
            ui = {
                menu = {
                    registerToMainMenu = function()
                        registered = true
                    end,
                },
            },
        })
        assert.is_true(registered)
        assert.are.equal("/tmp/recreated-kindle-cache", recreated.settings.cache_dir)
    end)

    it("adds a native Kindle Library entry without replacing FileChooser.path", function()
        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = true,
        })
        G_reader_settings:saveSetting("home_dir", DataStorage:getDataDir())
        load_plugin("kindle.koplugin")

        local KindleLibrary = require("lua/kindle_library")
        local original_show = KindleLibrary.show
        local shown = false
        KindleLibrary.show = function()
            shown = true
            return true
        end

        filemanager = FileManager:new({
            dimen = Screen:getSize(),
            root_path = DataStorage:getDataDir(),
        })
        UIManager:show(filemanager)
        fastforward_ui_events()

        local original_path = filemanager.file_chooser.path
        local kindle_entry
        for _, item in ipairs(filemanager.file_chooser.item_table) do
            if item.is_kindle_library_folder then
                kindle_entry = item
                break
            end
        end
        assert.is_truthy(kindle_entry)
        assert.equals(original_path, kindle_entry.path)
        assert.is_true(filemanager.file_chooser:onMenuSelect(kindle_entry))
        assert.is_true(shown)
        assert.equals(original_path, filemanager.file_chooser.path)

        local instance = PluginLoader:getPluginInstance("kindle")
        assert.is_truthy(instance)
        assert.is_true(instance:stopPlugin())
        local still_present = false
        for _, item in ipairs(filemanager.file_chooser.item_table) do
            if item.is_kindle_library_folder then
                still_present = true
                break
            end
        end
        assert.is_false(still_present)
        assert.equals(original_path, filemanager.file_chooser.path)

        KindleLibrary.show = original_show
    end)
end)
