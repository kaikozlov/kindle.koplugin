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
        if filemanager then
            filemanager:onClose()
            filemanager = nil
        end
        G_reader_settings:delSetting("kindle_plugin")
        UIManager:quit()
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
    end)
end)
