require("busted.runner")()

--- Smoke-test the plugin through KOReader's real PluginLoader and FileManager.
--- Keep the virtual library disabled here so the test verifies widget lifecycle
--- without leaving global monkey patches installed for the remaining specs.
describe("KindlePlugin native KOReader lifecycle", function()
    local UIManager = require("ui/uimanager")
    local DataStorage = require("datastorage")
    local Dispatcher = require("dispatcher")
    local FileManager = require("apps/filemanager/filemanager")
    local KindleLibrary = require("lua/kindle_library")
    local PluginLoader = require("pluginloader")
    local ReaderUI = require("apps/reader/readerui")
    local Screen = require("device").screen
    local ffiUtil = require("ffi/util")
    local util = require("util")
    local filemanager
    local reader_file
    local original_lastfile
    local original_build_entries

    before_each(function()
        disable_plugins()
        original_lastfile = G_reader_settings:readSetting("lastfile")
        original_build_entries = KindleLibrary.buildEntries
        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = false,
        })
    end)

    after_each(function()
        KindleLibrary.buildEntries = original_build_entries
        local instance = PluginLoader:getPluginInstance("kindle")
        if instance and instance.stopPlugin then
            pcall(instance.stopPlugin, instance)
        end
        if ReaderUI.instance then
            ReaderUI.instance:onClose()
        end
        if FileManager.instance then
            FileManager.instance:onClose()
        end
        filemanager = nil
        if reader_file then
            require("docsettings"):open(reader_file):purge()
            os.remove(reader_file)
            reader_file = nil
        end
        G_reader_settings:delSetting("kindle_plugin")
        if original_lastfile == nil then
            G_reader_settings:delSetting("lastfile")
        else
            G_reader_settings:saveSetting("lastfile", original_lastfile)
        end
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

        local build_count = 0
        KindleLibrary.buildEntries = function()
            build_count = build_count + 1
            return { { text = "Test Kindle book", kindle_book_id = "test-book" } }
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
        assert.equals(1, build_count)
        assert.is_truthy(require("lua/filechooser_ext").kindle_library.booklist_menu)
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
    end)

    it("executes the dispatcher action through a real FileManager", function()
        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = true,
        })
        load_plugin("kindle.koplugin")

        local build_force
        local build_count = 0
        KindleLibrary.buildEntries = function(_, force)
            build_force = force
            build_count = build_count + 1
            return { { text = "Test Kindle book", kindle_book_id = "test-book" } }
        end

        filemanager = FileManager:new({
            dimen = Screen:getSize(),
            root_path = DataStorage:getDataDir(),
        })
        UIManager:show(filemanager)
        fastforward_ui_events()

        assert.equals("Kindle Library", Dispatcher:getNameFromItem("kindle_library", { kindle_library = true }))
        Dispatcher:execute({ kindle_library = true })
        local library = require("lua/filechooser_ext").kindle_library
        assert.equals(filemanager, library.ui)
        assert.is_truthy(library.booklist_menu)
        assert.is_true(build_force)
        assert.equals(1, build_count)

        local instance = assert(PluginLoader:getPluginInstance("kindle"))
        assert.is_true(instance:stopPlugin())
        assert.equals("Unknown item", Dispatcher:getNameFromItem("kindle_library", { kindle_library = true }))
        Dispatcher:execute({ kindle_library = true })
        assert.equals(1, build_count)
    end)

    it("exits a real ReaderUI before showing the library in FileManager", function()
        reader_file = DataStorage:getDataDir() .. "/kindle-dispatcher-reader.txt"
        local file = assert(io.open(reader_file, "wb"))
        file:write("A real KOReader document used to test the dispatcher lifecycle.\n")
        file:close()
        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = true,
        })
        load_plugin("kindle.koplugin")

        local build_force
        KindleLibrary.buildEntries = function(_, force)
            build_force = force
            return { { text = "Test Kindle book", kindle_book_id = "test-book" } }
        end

        ReaderUI:doShowReader(reader_file)
        local reader = assert(ReaderUI.instance)
        assert.equals(reader_file, reader.document.file)
        G_reader_settings:saveSetting("lastfile", "/tmp/not-the-open-book.epub")

        Dispatcher:execute({ kindle_library = true })

        filemanager = assert(FileManager.instance)
        assert.is_nil(reader.document)
        assert.is_nil(ReaderUI.instance)
        local library = require("lua/filechooser_ext").kindle_library
        assert.equals(filemanager, library.ui)
        assert.is_truthy(library.booklist_menu)
        assert.is_true(build_force)
        local book_dir = util.splitFilePathName(reader_file)
        assert.equals(ffiUtil.realpath(book_dir), filemanager.file_chooser.path)
    end)
end)
