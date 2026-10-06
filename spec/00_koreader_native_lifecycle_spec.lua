require("busted.runner")()

--- Smoke-test the plugin through KOReader's real PluginLoader, FileManager,
--- FileChooser, and ReaderUI. The Kindle content catalog is reduced to the
--- narrow LibraryIndex.getBooks boundary; everything else (row building,
--- dialogs, document opening, close lifecycle) is the real native stack.
---
--- Keep the tests free of spec helper stubs and restore every mutated global,
--- setting, and temporary document so the whole suite can run together.
describe("KindlePlugin native KOReader lifecycle", function()
    local UIManager = require("ui/uimanager")
    local DataStorage = require("datastorage")
    local Dispatcher = require("dispatcher")
    local FileManager = require("apps/filemanager/filemanager")
    local FileChooserExt = require("lua/filechooser_ext")
    local OpenFileExt = require("lua/open_file_ext")
    local LibraryIndex = require("lua/library_index")
    local PluginLoader = require("pluginloader")
    local ReaderUI = require("apps/reader/readerui")
    local ReadingStateSync = require("lua/reading_state_sync")
    local Screen = require("device").screen
    local ffiUtil = require("ffi/util")
    local lfs = require("libs/libkoreader-lfs")
    local filemanager
    local reader_file
    local original_lastfile
    local original_home_dir
    local original_lastdir
    local original_get_books
    local original_pull
    local original_push
    local temp_dirs = {}

    local function snapshot_settings()
        original_lastfile = G_reader_settings:readSetting("lastfile")
        original_home_dir = G_reader_settings:readSetting("home_dir")
        original_lastdir = G_reader_settings:readSetting("lastdir")
    end

    local function restore_settings()
        local keys = { "kindle_plugin", "home_dir", "lastdir", "lastfile" }
        local originals = {
            kindle_plugin = nil,
            home_dir = original_home_dir,
            lastdir = original_lastdir,
            lastfile = original_lastfile,
        }
        for _, key in ipairs(keys) do
            local value = originals[key]
            if value == nil then
                G_reader_settings:delSetting(key)
            else
                G_reader_settings:saveSetting(key, value)
            end
        end
    end

    local function make_temp_dir()
        local dir = os.tmpname()
        os.remove(dir)
        assert(lfs.mkdir(dir))
        table.insert(temp_dirs, dir)
        return ffiUtil.realpath(dir)
    end

    local function write_file(path, content)
        local file = assert(io.open(path, "wb"))
        file:write(content or "A real KOReader document used by the native lifecycle spec.\n")
        file:close()
    end

    local function stub_catalog(book_dir, book_file)
        LibraryIndex.getBooks = function()
            return {
                {
                    id = "direct",
                    cde_key = "B000000001",
                    source_path = book_file,
                    open_mode = "direct",
                    display_name = "Native Open Book",
                },
            }
        end
        return book_dir, book_file
    end

    local function find_item(file_chooser, predicate)
        for _, item in ipairs(file_chooser.item_table) do
            if predicate(item) then
                return item
            end
        end
    end

    before_each(function()
        disable_plugins()
        snapshot_settings()
        original_get_books = LibraryIndex.getBooks
        original_pull = ReadingStateSync.syncFromKindleAutomatic
        original_push = ReadingStateSync.syncToKindleAutomatic
        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = false,
        })
    end)

    after_each(function()
        LibraryIndex.getBooks = original_get_books
        ReadingStateSync.syncFromKindleAutomatic = original_pull
        ReadingStateSync.syncToKindleAutomatic = original_push
        local instance = PluginLoader:getPluginInstance("kindle")
        if instance and instance.stopPlugin then
            pcall(instance.stopPlugin, instance)
        end
        -- ReaderUI closes clear PluginLoader bookkeeping before stopPlugin
        -- can run, so unwind the module hooks directly as well.
        pcall(OpenFileExt.unapply, OpenFileExt)
        pcall(FileChooserExt.unapply, FileChooserExt, require("ui/widget/filechooser"))
        pcall(Dispatcher.removeAction, Dispatcher, "kindle_library")
        if ReaderUI.instance then
            pcall(ReaderUI.instance.onClose, ReaderUI.instance)
        end
        if FileManager.instance then
            pcall(FileManager.instance.onClose, FileManager.instance)
        end
        filemanager = nil
        if reader_file then
            require("docsettings"):open(reader_file):purge()
            os.remove(reader_file)
            reader_file = nil
        end
        for _, dir in ipairs(temp_dirs) do
            ffiUtil.purgeDir(dir)
        end
        temp_dirs = {}
        restore_settings()
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

    it("activates the catalog view from the native entry in the real FileChooser", function()
        local book_dir = make_temp_dir()
        local book_file = book_dir .. "/entry-catalog-book.txt"
        write_file(book_file)
        stub_catalog(book_dir, book_file)

        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = true,
        })
        G_reader_settings:saveSetting("home_dir", DataStorage:getDataDir())
        load_plugin("kindle.koplugin")

        filemanager = FileManager:new({
            dimen = Screen:getSize(),
            root_path = DataStorage:getDataDir(),
        })
        UIManager:show(filemanager)
        fastforward_ui_events()

        local library = require("lua/filechooser_ext").kindle_library
        local file_chooser = filemanager.file_chooser
        local original_path = file_chooser.path
        local kindle_entry = find_item(file_chooser, function(item)
            return item.is_kindle_library_folder
        end)
        assert.is_truthy(kindle_entry)
        assert.equals(original_path, kindle_entry.path)

        assert.is_true(file_chooser:onMenuSelect(kindle_entry))
        assert.is_truthy(library:isBrowsing(file_chooser))
        assert.equals(original_path, file_chooser.path)
        local rows = file_chooser.item_table
        assert.is_truthy(rows[1].is_kindle_library_return)
        assert.is_true(rows[1].is_go_up)
        local book_row = find_item(file_chooser, function(item)
            return item.kindle_book_id == "direct"
        end)
        assert.is_truthy(book_row)
        assert.equals("file", lfs.attributes(book_row.path, "mode"))
        assert.equals(book_file, book_row.path)

        local instance = assert(PluginLoader:getPluginInstance("kindle"))
        assert.is_true(instance:stopPlugin())
        assert.is_falsy(library:isBrowsing(file_chooser))
        assert.equals(original_path, file_chooser.path)
        assert.is_nil(find_item(file_chooser, function(item)
            return item.is_kindle_library_folder or item.is_kindle_library_return or item.kindle_book_id
        end))
    end)

    it("executes the dispatcher action through a real FileManager", function()
        local book_dir = make_temp_dir()
        local book_file = book_dir .. "/dispatcher-book.txt"
        write_file(book_file)
        stub_catalog(book_dir, book_file)

        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = true,
        })
        load_plugin("kindle.koplugin")

        filemanager = FileManager:new({
            dimen = Screen:getSize(),
            root_path = DataStorage:getDataDir(),
        })
        UIManager:show(filemanager)
        fastforward_ui_events()

        assert.equals("Kindle Library", Dispatcher:getNameFromItem("kindle_library", { kindle_library = true }))
        Dispatcher:execute({ kindle_library = true })
        local library = require("lua/filechooser_ext").kindle_library
        local file_chooser = filemanager.file_chooser
        assert.equals(filemanager, library.ui)
        assert.is_truthy(library:isBrowsing(file_chooser))
        assert.is_truthy(find_item(file_chooser, function(item)
            return item.kindle_book_id == "direct"
        end))
        assert.is_truthy(
            FileManager.file_dialog_added_buttons
                and FileManager.file_dialog_added_buttons.index
                and FileManager.file_dialog_added_buttons.index.kindle_library
        )

        local instance = assert(PluginLoader:getPluginInstance("kindle"))
        assert.is_true(instance:stopPlugin())
        assert.equals("Unknown item", Dispatcher:getNameFromItem("kindle_library", { kindle_library = true }))
        Dispatcher:execute({ kindle_library = true })
        assert.is_falsy(library:isBrowsing(file_chooser))
        assert.is_falsy(
            FileManager.file_dialog_added_buttons
                and FileManager.file_dialog_added_buttons.index
                and FileManager.file_dialog_added_buttons.index.kindle_library
        )
    end)

    it("survives consumed ReaderUI lifecycle events and auto-syncs on real close", function()
        reader_file = DataStorage:getDataDir() .. "/kindle-consumed-docsettings.txt"
        local file = assert(io.open(reader_file, "wb"))
        file:write("A real KOReader document used to test consumed DocSettingsLoad.\n")
        file:close()

        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = true,
            cache_dir = DataStorage:getDataDir(),
            sync_reading_state = true,
            enable_auto_sync = true,
            enable_sync_from_kindle = true,
            enable_sync_to_kindle = true,
            sync_from_kindle_newer = 1,
            sync_from_kindle_older = 3,
            sync_to_kindle_newer = 2,
            sync_to_kindle_older = 3,
        })
        LibraryIndex.getBooks = function()
            return {
                {
                    id = "cc:test",
                    cde_key = "B000000001",
                    source_path = reader_file,
                    open_mode = "direct",
                },
            }
        end

        local pulls = 0
        local pushed
        ReadingStateSync.syncFromKindleAutomatic = function()
            pulls = pulls + 1
            return false
        end
        ReadingStateSync.syncToKindleAutomatic = function(_, cde_key, source_path, doc_settings, document_path)
            pushed = {
                cde_key = cde_key,
                source_path = source_path,
                percent = doc_settings:readSetting("percent_finished"),
                document_path = document_path,
            }
            return true
        end

        -- This stock KOReader plugin loads before kindle.koplugin and
        -- unconditionally consumes DocSettingsLoad. Also make it consume the
        -- two close events to prove Kindle's direct post-ReaderReady and
        -- CloseWidget recovery boundaries do not depend on plugin ordering.
        load_plugin("docsettingtweak.koplugin")
        for _, plugin in ipairs(PluginLoader.enabled_plugins) do
            if plugin.name == "docsettingtweak" then
                plugin.onCloseDocument = function()
                    return true
                end
                plugin.onSaveSettings = function()
                    return true
                end
                break
            end
        end
        load_plugin("kindle.koplugin")

        ReaderUI:doShowReader(reader_file)
        local reader = assert(ReaderUI.instance)
        local kindle = assert(reader.kindle)
        assert.equals(1, pulls)
        assert.equals(reader_file, kindle._automatic_sync_open_document)

        reader:onClose(false)

        assert.is_nil(ReaderUI.instance)
        assert.is_truthy(pushed)
        assert.equals("B000000001", pushed.cde_key)
        assert.equals(reader_file, pushed.source_path)
        assert.equals(reader_file, pushed.document_path)
    end)

    it("exits a real ReaderUI before showing the library in FileManager", function()
        local book_dir = make_temp_dir()
        reader_file = book_dir .. "/kindle-dispatcher-reader.txt"
        write_file(reader_file)
        stub_catalog(book_dir, reader_file)
        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = true,
        })
        load_plugin("kindle.koplugin")

        ReaderUI:doShowReader(reader_file)
        local reader = assert(ReaderUI.instance)
        assert.equals(reader_file, reader.document.file)

        Dispatcher:execute({ kindle_library = true })

        filemanager = assert(FileManager.instance)
        assert.is_nil(reader.document)
        assert.is_nil(ReaderUI.instance)
        local library = require("lua/filechooser_ext").kindle_library
        assert.is_truthy(library:isBrowsing(filemanager.file_chooser))
        assert.equals(ffiUtil.realpath(book_dir), filemanager.file_chooser.path)
    end)

    it("opens a catalog row into a real ReaderUI and returns to the catalog on close", function()
        local book_dir = make_temp_dir()
        local book_file = book_dir .. "/native-open-book.txt"
        write_file(book_file)
        stub_catalog(book_dir, book_file)

        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = true,
        })
        G_reader_settings:saveSetting("home_dir", book_dir)
        load_plugin("kindle.koplugin")

        filemanager = FileManager:new({
            dimen = Screen:getSize(),
            root_path = book_dir,
        })
        UIManager:show(filemanager)
        fastforward_ui_events()

        local library = require("lua/filechooser_ext").kindle_library
        local file_chooser = filemanager.file_chooser
        local kindle_entry = find_item(file_chooser, function(item)
            return item.is_kindle_library_folder
        end)
        assert.is_truthy(kindle_entry)
        assert.is_true(file_chooser:onMenuSelect(kindle_entry))
        assert.is_truthy(library:isBrowsing(file_chooser))

        local book_row = find_item(file_chooser, function(item)
            return item.kindle_book_id == "direct"
        end)
        assert.is_truthy(book_row)
        assert.is_true(file_chooser:onMenuSelect(book_row))
        fastforward_ui_events()

        -- The real native open chain ran: FileManager closed, ReaderUI owns
        -- the document, the catalog was left without a repaint, and exactly
        -- one return receipt is pending for the origin directory.
        local reader = assert(ReaderUI.instance)
        assert.equals(book_file, reader.document.file)
        assert.is_nil(FileManager.instance)
        assert.is_falsy(library:isBrowsing(file_chooser))
        assert.is_truthy(library.return_to_library_request)
        assert.equals(ffiUtil.realpath(book_dir), library.return_to_library_request.origin_path)

        -- Closing the reader through the native Home gesture rebuilds the
        -- file browser, restores the origin, and re-enters the catalog.
        reader:onHome()
        fastforward_ui_events()

        local returned = assert(FileManager.instance)
        assert.are_not_equal(filemanager, returned)
        assert.is_nil(ReaderUI.instance)
        local returned_chooser = returned.file_chooser
        assert.equals(ffiUtil.realpath(book_dir), returned_chooser.path)
        assert.is_nil(library.return_to_library_request)
        assert.is_truthy(library:isBrowsing(returned_chooser))
        assert.is_truthy(returned_chooser.item_table[1].is_kindle_library_return)
        assert.is_truthy(find_item(returned_chooser, function(item)
            return item.kindle_book_id == "direct"
        end))
    end)

    it("live stop while browsing restores the directory, navigation, and hooks", function()
        local book_dir = make_temp_dir()
        local book_file = book_dir .. "/stop-book.txt"
        write_file(book_file)
        local subdir = book_dir .. "/plain"
        assert(lfs.mkdir(subdir))
        stub_catalog(book_dir, book_file)

        G_reader_settings:saveSetting("kindle_plugin", {
            enable_virtual_library = true,
        })
        G_reader_settings:saveSetting("home_dir", book_dir)
        load_plugin("kindle.koplugin")

        filemanager = FileManager:new({
            dimen = Screen:getSize(),
            root_path = book_dir,
        })
        UIManager:show(filemanager)
        fastforward_ui_events()

        local library = require("lua/filechooser_ext").kindle_library
        local file_chooser = filemanager.file_chooser
        local kindle_entry = find_item(file_chooser, function(item)
            return item.is_kindle_library_folder
        end)
        assert.is_truthy(kindle_entry)
        assert.is_true(file_chooser:onMenuSelect(kindle_entry))
        assert.is_truthy(library:isBrowsing(file_chooser))

        local instance = assert(PluginLoader:getPluginInstance("kindle"))
        assert.is_true(instance:stopPlugin())

        assert.is_falsy(library:isBrowsing(file_chooser))
        assert.equals(ffiUtil.realpath(book_dir), file_chooser.path)
        assert.is_truthy(find_item(file_chooser, function(item)
            return item.path == book_file and item.is_file
        end))
        assert.is_nil(find_item(file_chooser, function(item)
            return item.is_kindle_library_folder or item.is_kindle_library_return or item.kindle_book_id
        end))

        -- Native navigation works again without the catalog dispatch.
        file_chooser:changeToPath(subdir)
        assert.equals(ffiUtil.realpath(subdir), file_chooser.path)
        assert.is_truthy(find_item(file_chooser, function(item)
            return item.is_go_up
        end))

        assert.equals("Unknown item", Dispatcher:getNameFromItem("kindle_library", { kindle_library = true }))
        assert.is_falsy(
            FileManager.file_dialog_added_buttons
                and FileManager.file_dialog_added_buttons.index
                and FileManager.file_dialog_added_buttons.index.kindle_library
        )
    end)
end)
