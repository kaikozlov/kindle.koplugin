require("busted.runner")()
local helper = require("spec/test_helper")

describe("KindlePlugin", function()
    local FileManager
    local KindleLibrary
    local UIManager
    local instances
    local original_filemanager_instance
    local original_library_show
    local original_next_tick

    setup(function()
        helper.setup_complete()
        UIManager = require("ui/uimanager")
        FileManager = require("apps/filemanager/filemanager")
        KindleLibrary = require("lua/kindle_library")
        original_filemanager_instance = FileManager.instance
        original_library_show = KindleLibrary.show
        original_next_tick = UIManager.nextTick
    end)

    before_each(function()
        helper.before_each()
        UIManager:_reset()
        package.loaded["main"] = nil
        instances = {}
    end)

    after_each(function()
        for _, instance in ipairs(instances) do
            pcall(function()
                instance:stopPlugin()
            end)
        end
        FileManager.instance = original_filemanager_instance
        KindleLibrary.show = original_library_show
        UIManager.nextTick = original_next_tick
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
            custom_setting = "preserved",
        })
        assert.is_false(instance.settings.enable_virtual_library)
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

    it("restores the file browser before returning to the native library", function()
        local first = newPlugin({ enable_virtual_library = true })
        local library = require("lua/filechooser_ext").kindle_library
        library:requestReturnToLibrary("/mnt/us")
        first:stopPlugin()

        local post_init
        local next_tick
        local changed_to
        local returned_ui = {
            document = nil,
            menu = { registerToMainMenu = function() end },
            file_chooser = {
                changeToPath = function(_, path)
                    changed_to = path
                end,
            },
            registerPostInitCallback = function(_, callback)
                post_init = callback
            end,
        }
        UIManager.nextTick = function(_, callback)
            next_tick = callback
        end
        local shown_ui
        local refresh
        KindleLibrary.show = function(_, ui, force)
            shown_ui = ui
            refresh = force
            return true
        end

        newPlugin(nil, returned_ui)
        assert.is_function(post_init)
        post_init()
        assert.equals("/mnt/us", changed_to)
        assert.is_function(next_tick)
        FileManager.instance = returned_ui
        next_tick()

        assert.equals(returned_ui, shown_ui)
        assert.is_false(refresh)
        assert.is_nil(library:takeReturnToLibraryRequest())
    end)

    it("clears book keys only after confirmation", function()
        local cache_dir = os.tmpname()
        os.remove(cache_dir)
        assert.equals(0, os.execute("mkdir -p " .. cache_dir))
        local keys_path = cache_dir .. "/drm_keys.json"
        local keys_file = assert(io.open(keys_path, "wb"))
        keys_file:write("{}")
        keys_file:close()

        local instance = newPlugin({ cache_dir = cache_dir })
        local menu_item = instance:createClearKeysMenuItem()
        UIManager:_reset()
        menu_item.callback()

        local confirm = UIManager._shown_widgets[#UIManager._shown_widgets]
        assert.is_truthy(confirm)
        assert.is_function(confirm.ok_callback)
        confirm.ok_callback()

        assert.is_nil(io.open(keys_path, "rb"))
        local info = UIManager._shown_widgets[#UIManager._shown_widgets]
        assert.is_truthy(info.text:match("Book keys cleared"))
        os.execute("rm -rf " .. cache_dir)
    end)

    it("reports a book-key removal failure", function()
        local cache_dir = os.tmpname()
        os.remove(cache_dir)
        assert.equals(0, os.execute("mkdir -p " .. cache_dir))
        local keys_path = cache_dir .. "/drm_keys.json"
        local keys_file = assert(io.open(keys_path, "wb"))
        keys_file:write("{}")
        keys_file:close()

        local instance = newPlugin({ cache_dir = cache_dir })
        local menu_item = instance:createClearKeysMenuItem()
        UIManager:_reset()
        menu_item.callback()
        local confirm = UIManager._shown_widgets[#UIManager._shown_widgets]

        local original_remove = os.remove
        rawset(os, "remove", function(path)
            if path == keys_path then
                return nil, "permission denied"
            end
            return original_remove(path)
        end)
        confirm.ok_callback()
        rawset(os, "remove", original_remove)

        local still_there = assert(io.open(keys_path, "rb"))
        still_there:close()
        local info = UIManager._shown_widgets[#UIManager._shown_widgets]
        assert.is_truthy(info.text:match("Failed to clear book keys"))
        assert.is_truthy(info.text:match("permission denied"))
        os.execute("rm -rf " .. cache_dir)
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
