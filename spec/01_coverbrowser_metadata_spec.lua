require("busted.runner")()

-- Exercise the real CoverBrowser lookup and renderer: plain FileChooser tests
-- cannot catch its fallback from catalog titles to unsupported KFX filenames.
-- Run with the native lifecycle suite before unit fixtures install SQLite mocks.
describe("Kindle catalog CoverBrowser metadata", function()
    local BD = require("ui/bidi")
    local Blitbuffer = require("ffi/blitbuffer")
    local FileManager = require("apps/filemanager/filemanager")
    local HelperClient = require("lua/helper_client")
    local LibraryIndex = require("lua/library_index")
    local PluginLoader = require("pluginloader")
    local Screen = require("device").screen
    local UIManager = require("ui/uimanager")
    local ffiUtil = require("ffi/util")
    local lfs = require("libs/libkoreader-lfs")
    local util = require("util")
    local BookInfoManager
    local original_get_books
    local original_convert
    local conversions
    local original_settings
    local original_package_path
    local original_display_mode
    local original_lookup
    local original_lastdir
    local tmpdir
    local source
    local thumbnail
    local filemanager
    local book
    local cached_paths

    local function write_file(path, content)
        local file = assert(io.open(path, "wb"))
        file:write(content or "Unopened document fixture")
        file:close()
    end

    local function cache_info(path, title, ignore_meta, ignore_cover)
        local directory, filename = util.splitFilePathName(path)
        BookInfoManager:openDbConnection()
        local stmt = BookInfoManager.db_conn:prepare([[
            INSERT OR REPLACE INTO bookinfo
                (directory, filename, title, has_meta, cover_fetched, in_progress, ignore_meta, ignore_cover)
            VALUES (?, ?, ?, 'Y', 'Y', 0, ?, ?);
        ]])
        stmt:bind(directory, filename, title, ignore_meta, ignore_cover):step()
        stmt:close()
        cached_paths[#cached_paths + 1] = path
    end

    local function find_rendered_item(widget)
        if widget.entry and widget.entry.kindle_book_id == "catalog-book" then
            return widget
        end
        for _, child in ipairs(widget) do
            if type(child) == "table" then
                local item = find_rendered_item(child)
                if item then
                    return item
                end
            end
        end
    end

    local function has_rendered_text(widget, text)
        if widget.text == text then
            return true
        end
        for _, child in ipairs(widget) do
            if type(child) == "table" and has_rendered_text(child, text) then
                return true
            end
        end
        return false
    end

    before_each(function()
        disable_plugins()
        -- commonrequire's load_plugin restores package.path after loading the
        -- class; CoverBrowser also imports its shipped siblings during init.
        original_package_path = package.path
        package.path = package.path .. ";plugins/coverbrowser.koplugin/?.lua"
        original_settings = G_reader_settings:readSetting("kindle_plugin")
        original_lastdir = G_reader_settings:readSetting("lastdir")
        original_get_books = LibraryIndex.getBooks
        original_convert = HelperClient.convert
        conversions = 0
        HelperClient.convert = function()
            conversions = conversions + 1
            error("Catalog browsing must not convert books")
        end
        tmpdir = os.tmpname()
        os.remove(tmpdir)
        assert(lfs.mkdir(tmpdir))
        cached_paths = {}
        source = tmpdir .. "/book_with_delivery_identifier.kfx"
        write_file(source)
        thumbnail = tmpdir .. "/thumbnail.png"
        local image = Blitbuffer.new(60, 90, Blitbuffer.TYPE_BB8)
        image:paintRect(0, 0, 60, 90, Blitbuffer.COLOR_WHITE)
        image:writePNG(thumbnail)
        image:free()
        book = {
            id = "catalog-book",
            title = "Catalog title",
            display_name = "Catalog title",
            authors = { "Catalog author" },
            thumbnail = thumbnail,
            source_path = source,
            open_mode = "convert",
        }
        LibraryIndex.getBooks = function()
            return { book }
        end
        G_reader_settings:saveSetting("kindle_plugin", { enable_virtual_library = true })
        load_plugin("coverbrowser.koplugin")
        load_plugin("kindle.koplugin")
        BookInfoManager = require("bookinfomanager")
        original_lookup = BookInfoManager.getBookInfo
        original_display_mode = BookInfoManager:getSetting("filemanager_display_mode")
        BookInfoManager:saveSetting("filemanager_display_mode", "list_image_meta")
        filemanager = FileManager:new({ dimen = Screen:getSize(), root_path = tmpdir })
        UIManager:show(filemanager)
        fastforward_ui_events()
        assert.is_truthy(PluginLoader:getPluginInstance("coverbrowser"))
        require("lua/filechooser_ext").kindle_library:show(filemanager, true)
    end)

    after_each(function()
        local plugin = PluginLoader:getPluginInstance("kindle")
        if plugin then
            plugin:stopPlugin()
        end
        if filemanager then
            filemanager:onClose()
            filemanager = nil
        end
        BookInfoManager:saveSetting("filemanager_display_mode", original_display_mode)
        for _, path in ipairs(cached_paths) do
            BookInfoManager:deleteBookInfo(path)
        end
        LibraryIndex.getBooks = original_get_books
        HelperClient.convert = original_convert
        if original_settings then
            G_reader_settings:saveSetting("kindle_plugin", original_settings)
        else
            G_reader_settings:delSetting("kindle_plugin")
        end
        G_reader_settings:saveSetting("lastdir", original_lastdir)
        ffiUtil.purgeDir(tmpdir)
        UIManager:quit()
        package.path = original_package_path
    end)

    it("renders catalog metadata and an existing thumbnail before KFX conversion", function()
        assert.is_false(require("document/documentregistry"):hasProvider(source))
        local info = BookInfoManager:getBookInfo(source, true)
        assert.equals("Catalog title", info.title)
        assert.equals("Catalog author", info.authors)
        assert.is_falsy(info.ignore_meta)
        assert.is_truthy(info.has_cover)
        assert.equals(60, info.cover_bb:getWidth())
        info.cover_bb:free()

        local item = assert(find_rendered_item(filemanager.file_chooser.item_group))
        assert.is_true(item._has_cover_image)
        assert.is_true(has_rendered_text(item[1], BD.auto("Catalog title")))
        assert.is_true(has_rendered_text(item[1], BD.auto("Catalog author")))
        assert.equals(source, item.entry.path)
        assert.equals(0, conversions)
    end)

    it("honors explicit metadata and cover opt-outs even for unsupported KFX", function()
        cache_info(source, "User cache title", "Y", "Y")
        local info = BookInfoManager:getBookInfo(source, true)
        assert.equals("Y", info.ignore_meta)
        assert.equals("Y", info.ignore_cover)
        assert.is_nil(info.cover_bb)
        assert.is_nil(info.title)
    end)

    it("replaces a cached PDB error title with catalog metadata without rewriting the cache", function()
        source = tmpdir .. "/failed.azw"
        write_file(source)
        book.source_path = source
        book.open_mode = "direct"
        require("lua/filechooser_ext").virtual_library:refresh(true)
        cache_info(source, "ERROR: Error reading PDB format")

        local info = BookInfoManager:getBookInfo(source, true)
        assert.equals("Catalog title", info.title)
        assert.equals("Catalog author", info.authors)
        assert.is_truthy(info.cover_bb)
        info.cover_bb:free()
        assert.equals("ERROR: Error reading PDB format", original_lookup(BookInfoManager, source).title)
        assert.equals(0, conversions)
    end)

    it("leaves supported uncached sources available for native metadata and cover extraction", function()
        source = tmpdir .. "/not_extracted.azw"
        write_file(source)
        book.source_path = source
        book.open_mode = "direct"
        require("lua/filechooser_ext").virtual_library:refresh(true)
        assert.is_nil(BookInfoManager:getBookInfo(source, true))

        cache_info(source, "Embedded title")
        BookInfoManager:setBookInfoProperties(source, { cover_fetched = false })
        local info = BookInfoManager:getBookInfo(source, true)
        assert.equals("Embedded title", info.title)
        assert.is_nil(info.cover_fetched)
        assert.is_nil(info.cover_bb)
    end)

    it("delegates unrelated books and restores the native lookup when disabled", function()
        local other = tmpdir .. "/unrelated.kfx"
        write_file(other)
        local info = BookInfoManager:getBookInfo(other, true)
        assert.is_nil(info.title)
        assert.is_nil(info.cover_bb)
        assert.equals("Y", info.ignore_meta)

        PluginLoader:getPluginInstance("kindle"):stopPlugin()
        assert.equals(original_lookup, BookInfoManager.getBookInfo)
        assert.is_nil(BookInfoManager:getBookInfo(source, true).title)
    end)
end)
