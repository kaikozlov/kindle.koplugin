require("busted.runner")()
local helper = require("spec/test_helper")

describe("KindlePlugin", function()
    local UIManager
    local instances

    setup(function()
        helper.setup_complete()
        UIManager = require("ui/uimanager")
    end)

    before_each(function()
        helper.before_each()
        UIManager:_reset()
        package.loaded["main"] = nil
        instances = {}
    end)

    after_each(function()
        for _, instance in ipairs(instances) do
            pcall(function() instance:stopPlugin() end)
        end
    end)

    local function newPlugin(settings, ui)
        if settings then
            G_reader_settings:saveSetting("kindle_plugin", settings)
        end
        local KindlePlugin = require("main")
        local instance = KindlePlugin:new({
            ui = ui or {
                menu = { registerToMainMenu = function() end },
            },
        })
        table.insert(instances, instance)
        return instance
    end

    it("does not patch KOReader document/filesystem/reader APIs at module load", function()
        local lfs = require("libs/libkoreader-lfs")
        local DocumentRegistry = require("document/documentregistry")
        local ReaderUI = require("apps/reader/readerui")
        local attrs = lfs.attributes
        local open_document = DocumentRegistry.openDocument
        local show_reader = ReaderUI.showReader

        require("main")

        assert.equals(attrs, lfs.attributes)
        assert.equals(open_document, DocumentRegistry.openDocument)
        assert.equals(show_reader, ReaderUI.showReader)
    end)

    it("loads defaults while preserving explicit settings", function()
        local instance = newPlugin({
            enable_virtual_library = false,
            drm_initialized = true,
            custom_setting = "preserved",
        })
        assert.is_false(instance.settings.enable_virtual_library)
        assert.is_true(instance.settings.drm_initialized)
        assert.equals("preserved", instance.settings.custom_setting)
        assert.is_not_nil(instance.settings.documents_root)
        assert.is_not_nil(instance.settings.cache_dir)
    end)

    it("repairs the obsolete virtual HOME URI", function()
        G_reader_settings:saveSetting("home_dir", "KINDLE_VIRTUAL://")
        newPlugin({ enable_virtual_library = true })
        assert.not_equals("KINDLE_VIRTUAL://", G_reader_settings:readSetting("home_dir"))
    end)

    it("registers its menu and can be live-stopped", function()
        local registered = false
        local instance = newPlugin(nil, {
            menu = {
                registerToMainMenu = function()
                    registered = true
                end,
            },
        })
        assert.is_true(registered)
        assert.is_true(instance:stopPlugin())
    end)

    it("keeps the menu available while the library view is disabled", function()
        local instance = newPlugin({ enable_virtual_library = false })
        instance.ui = { document = nil }
        local menu_items = {}
        instance:addToMainMenu(menu_items)
        assert.is_truthy(menu_items.kindle_plugin)
        assert.is_false(menu_items.kindle_plugin.sub_item_table[1].enabled_func())
        assert.is_false(menu_items.kindle_plugin.sub_item_table[2].enabled_func())
    end)

    it("does not add FileManager-only menu items in ReaderUI", function()
        local instance = newPlugin(nil, {
            document = { file = "/tmp/book.epub" },
            doc_settings = {},
            menu = { registerToMainMenu = function() end },
        })
        local menu_items = {}
        instance:addToMainMenu(menu_items)
        assert.is_nil(menu_items.kindle_plugin)
    end)
end)
