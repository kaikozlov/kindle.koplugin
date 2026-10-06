require("busted.runner")()
local helper = require("spec/test_helper")

--- Verifies the native FileChooser hook set: instance-dispatched catalog rows,
--- synthetic entry placement, dynamic delegation for ordinary directories,
--- and complete unwinding. All fixtures use the real FileChooser machinery
--- with real temporary directories; LibraryIndex.getBooks is the only stub.
describe("FileChooserExt native catalog hooks", function()
    local lfs = require("libs/libkoreader-lfs")
    local Device = require("device")
    local Screen = Device.screen
    local UIManager = require("ui/uimanager")
    local FileManager = require("apps/filemanager/filemanager")
    local FileChooser = require("ui/widget/filechooser")
    local PathChooser = require("ui/widget/pathchooser")
    local FileChooserExt = require("lua/filechooser_ext")
    local ffiUtil = require("ffi/util")
    local KindleLibrary = require("lua/kindle_library")
    local LibraryIndex = require("lua/library_index")
    local VirtualLibrary = require("lua/virtual_library")

    local original_get_books
    local temp_dirs = {}
    local fixtures = {}
    local class_wraps = {}

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        disable_plugins()
        original_get_books = LibraryIndex.getBooks
        temp_dirs = {}
        fixtures = {}
        class_wraps = {}
    end)

    after_each(function()
        for _, fixture in ipairs(fixtures) do
            pcall(function()
                FileChooserExt:unapply(FileChooser)
            end)
            if FileManager.instance == fixture.filemanager then
                pcall(function()
                    UIManager:close(fixture.filemanager)
                end)
            end
        end
        for _, wrap in ipairs(class_wraps) do
            pcall(function()
                FileChooser[wrap.name] = wrap.original
            end)
        end
        LibraryIndex.getBooks = original_get_books
        for _, dir in ipairs(temp_dirs) do
            ffiUtil.purgeDir(dir)
        end
    end)

    local function make_temp_dir()
        local dir = os.tmpname()
        os.remove(dir)
        assert(lfs.mkdir(dir))
        table.insert(temp_dirs, dir)
        return ffiUtil.realpath(dir)
    end

    local function write_file(path)
        local file = assert(io.open(path, "wb"))
        file:write("real catalog document\n")
        file:close()
    end

    local function wrap_class_method(name, wrapper)
        local original = FileChooser[name]
        table.insert(class_wraps, { name = name, original = original })
        FileChooser[name] = wrapper
        return original
    end

    local function build_fixture(options)
        options = options or {}
        local dir = make_temp_dir()
        local direct_path = dir .. "/direct-book.txt"
        write_file(direct_path)
        local subdir = dir .. "/nested"
        assert(lfs.mkdir(subdir))

        LibraryIndex.getBooks = function()
            return {
                {
                    id = "direct",
                    source_path = direct_path,
                    open_mode = "direct",
                    display_name = "Direct Book",
                },
            }
        end
        local vlib = VirtualLibrary:new(LibraryIndex)
        vlib:setSettings({ enable_virtual_library = true })
        local library = KindleLibrary:new(vlib, options.cache_manager)

        local filemanager = FileManager:new({
            dimen = Screen:getSize(),
            root_path = options.root_path or dir,
        })
        UIManager:show(filemanager)
        library:setUI(filemanager)
        FileChooserExt:init(vlib, library)
        FileChooserExt:apply(FileChooser)

        local fixture = {
            library = library,
            vlib = vlib,
            filemanager = filemanager,
            file_chooser = filemanager and filemanager.file_chooser,
            dir = dir,
            direct_path = direct_path,
            subdir = subdir,
        }
        table.insert(fixtures, fixture)
        return fixture
    end

    local function folder_entry(file_chooser)
        for _, item in ipairs(file_chooser.item_table) do
            if item.is_kindle_library_folder then
                return item
            end
        end
    end

    local function launcher_count(file_chooser)
        local count = 0
        for _, item in ipairs(file_chooser.item_table) do
            if item.is_kindle_library_folder then
                count = count + 1
            end
        end
        return count
    end

    local function find_file_row(file_chooser, path)
        for _, item in ipairs(file_chooser.item_table) do
            if item.path == path and item.is_file then
                return item
            end
        end
    end

    it("adds the synthetic entry only at HOME or root in FileManager choosers", function()
        local home = make_temp_dir()
        G_reader_settings:saveSetting("home_dir", home)
        local fixture = build_fixture({ root_path = home })
        local fc = fixture.file_chooser

        fc:refreshPath()
        local entry = folder_entry(fc)
        assert.is_truthy(entry)
        assert.equals(home, entry.path)

        fc:changeToPath(fixture.subdir)
        assert.is_nil(folder_entry(fc))

        fc:changeToPath("/")
        assert.is_truthy(folder_entry(fc))
        assert.is_nil(folder_entry(fc).is_file)

        -- PathChooser shares the FileChooser class but must stay untouched.
        local chooser = PathChooser:new({
            dimen = Screen:getSize(),
            select_file = true,
            select_directory = false,
        })
        UIManager:show(chooser)
        assert.is_nil(folder_entry(chooser))
        chooser:refreshPath()
        assert.is_nil(folder_entry(chooser))
        UIManager:close(chooser)
    end)

    it("opens the catalog from the synthetic entry select and hold", function()
        local home = make_temp_dir()
        G_reader_settings:saveSetting("home_dir", home)
        local fixture = build_fixture({ root_path = home })
        local fc = fixture.file_chooser
        fc:refreshPath()
        local entry = folder_entry(fc)
        assert.is_truthy(entry)

        assert.is_true(fc:onMenuSelect(entry))
        assert.is_truthy(fixture.library:isBrowsing(fc))
        fixture.library:close()

        assert.is_true(fc:onMenuHold(entry))
        assert.is_truthy(fixture.library:isBrowsing(fc))
    end)

    it("attaches choosers created after apply", function()
        local fixture = build_fixture()
        local first = fixture.filemanager
        UIManager:close(first)
        if FileManager.instance == first then
            FileManager.instance = nil
        end

        local second = FileManager:new({
            dimen = Screen:getSize(),
            root_path = fixture.dir,
        })
        UIManager:show(second)
        fixture.library:setUI(second)
        assert.is_truthy(fixture.library:show(second, true))
        assert.is_truthy(fixture.library:isBrowsing(second.file_chooser))

        UIManager:close(second)
        FileManager.instance = nil
    end)

    it("bypasses a memoizing directory cache for the catalog without leakage", function()
        local fixture = build_fixture()
        local fc = fixture.file_chooser
        G_reader_settings:saveSetting("home_dir", fixture.dir)

        -- A representative outer directory cache wrapping the class method.
        local real_gen = FileChooser.genItemTableFromPath
        local cached_listings = {}
        wrap_class_method("genItemTableFromPath", function(chooser, path)
            local cached = cached_listings[path]
            if cached then
                return cached
            end
            local rows = real_gen(chooser, path)
            cached_listings[path] = rows
            return rows
        end)

        -- Populate the cache with the normal listing of the origin.
        fc:refreshPath()
        local normal_rows = cached_listings[fc.path]
        assert.is_truthy(normal_rows)
        assert.is_truthy(find_file_row(fc, fixture.direct_path))
        assert.is_truthy(folder_entry(fc))
        assert.is_nil(folder_entry({ item_table = normal_rows }))

        -- Catalog rows must not come from, or be written to, the directory cache.
        assert.is_true(fixture.library:show(fixture.filemanager, true))
        assert.is_truthy(fixture.library:isBrowsing(fc))
        -- The cached entry still holds the normal listing: no catalog rows
        -- leaked into the cache, and none came out of it.
        assert.equals(normal_rows, cached_listings[fc.path])
        assert.is_truthy(fc.item_table[1].is_kindle_library_return)
        assert.is_falsy(normal_rows[1].is_kindle_library_return)

        fc:refreshPath()
        assert.is_truthy(fc.item_table[1].is_kindle_library_return)

        -- Closing falls back to the cached normal listing: no catalog rows
        -- leak through, and the cache is actually used again.
        fixture.library:close()
        assert.is_truthy(find_file_row(fc, fixture.direct_path))
        for _, item in ipairs(fc.item_table) do
            assert.is_nil(item.kindle_book_id)
            assert.is_nil(item.is_kindle_library_return)
        end
        assert.is_truthy(folder_entry(fc))
        assert.is_nil(folder_entry({ item_table = normal_rows }))

        -- The transient launcher must not survive live disable via a warm cache.
        FileChooserExt:unapply(FileChooser)
        fc:refreshPath()
        assert.is_nil(folder_entry(fc))
        assert.is_truthy(find_file_row(fc, fixture.direct_path))
    end)

    it("evicts a contaminated directory snapshot before adding a fresh launcher", function()
        local fixture = build_fixture()
        local fc = fixture.file_chooser
        G_reader_settings:saveSetting("home_dir", fixture.dir)
        local real_folder = fixture.dir .. "/Kindle Library"
        assert(lfs.mkdir(real_folder))

        local real_gen = FileChooser.genItemTableFromPath
        local legacy_rows = real_gen(fc, fixture.dir)
        table.insert(legacy_rows, fixture.vlib:createVirtualFolderEntry(fixture.dir))
        local other_rows = real_gen(fc, fixture.subdir)
        local cached_listings = {
            [fixture.dir] = legacy_rows,
            [fixture.subdir] = other_rows,
        }
        wrap_class_method("genItemTableFromPath", function(chooser, path)
            if not cached_listings[path] then
                cached_listings[path] = real_gen(chooser, path)
            end
            return cached_listings[path]
        end)
        fc._zen_invalidate_item_table_path = function(_, path)
            cached_listings[path] = nil
        end

        fc:refreshPath()
        assert.equals(1, launcher_count(fc))
        assert.is_truthy(find_file_row(fc, fixture.direct_path))
        local found_real_folder = false
        for _, item in ipairs(fc.item_table) do
            if item.path == real_folder then
                assert.is_nil(item.is_kindle_library_folder)
                found_real_folder = true
            end
        end
        assert.is_true(found_real_folder)
        assert.equals(other_rows, cached_listings[fixture.subdir])
        assert.equals(0, launcher_count({ item_table = cached_listings[fixture.dir] }))
        -- Discard the old snapshot through its owner, not by editing its rows.
        assert.equals(1, launcher_count({ item_table = legacy_rows }))

        assert.is_true(fc:onMenuSelect(folder_entry(fc)))
        assert.is_truthy(fixture.library:isBrowsing(fc))
        fixture.library:close()
        fc:changeToPath(fixture.subdir)
        fc:changeToPath(fixture.dir)
        fc:refreshPath()
        assert.equals(1, launcher_count(fc))

        FileChooserExt:unapply(FileChooser)
        fc:refreshPath()
        assert.equals(0, launcher_count(fc))
        assert.is_truthy(find_file_row(fc, fixture.direct_path))
        FileChooserExt:apply(FileChooser)
        fc:refreshPath()
        assert.equals(1, launcher_count(fc))
    end)

    it("filters cached launcher markers without an invalidation API or matching their labels", function()
        local fixture = build_fixture()
        local fc = fixture.file_chooser
        G_reader_settings:saveSetting("home_dir", fixture.dir)
        local real_gen = FileChooser.genItemTableFromPath
        local legacy_rows = real_gen(fc, fixture.dir)
        for _, label in ipairs({ "Kindle Library/", "Bibliothèque Kindle/" }) do
            local entry = fixture.vlib:createVirtualFolderEntry(fixture.dir)
            entry.text = label
            table.insert(legacy_rows, entry)
        end
        wrap_class_method("genItemTableFromPath", function(chooser, path)
            return path == fixture.dir and legacy_rows or real_gen(chooser, path)
        end)

        fc:refreshPath()
        assert.equals(1, launcher_count(fc))
        fc:refreshPath()
        assert.equals(1, launcher_count(fc))
        assert.is_truthy(find_file_row(fc, fixture.direct_path))
        assert.equals(2, launcher_count({ item_table = legacy_rows }))

        -- A cached launcher is also stale when this directory is no longer HOME.
        G_reader_settings:saveSetting("home_dir", fixture.subdir)
        fc:refreshPath()
        assert.equals(0, launcher_count(fc))
        assert.is_truthy(find_file_row(fc, fixture.direct_path))
        assert.equals(2, launcher_count({ item_table = legacy_rows }))
    end)

    it("unwinds class hooks, tracked instance methods, and dialog buttons", function()
        local originals = {}
        for _, name in ipairs({
            "init",
            "genItemTable",
            "refreshPath",
            "onMenuSelect",
            "onMenuHold",
            "changeToPath",
            "goHome",
            "onBack",
            "onFolderUp",
        }) do
            originals[name] = FileChooser[name]
        end

        local fixture = build_fixture()
        local fc = fixture.file_chooser
        assert.is_true(fixture.library:show(fixture.filemanager, true))

        assert.is_truthy(rawget(fc, "genItemTableFromPath"))
        assert.is_truthy(FileManager.file_dialog_added_buttons)

        -- A replacement browser installing its own hook after ours must not
        -- be clobbered by the unwind. Teardown is registered against the true
        -- pre-plugin method, not against our installed hook.
        local foreign = function() end
        FileChooser.changeToPath = foreign
        table.insert(class_wraps, {
            name = "changeToPath",
            original = originals.changeToPath,
        })

        FileChooserExt:unapply(FileChooser)
        FileChooserExt:unapply(FileChooser) -- idempotent

        for name, original in pairs(originals) do
            if name ~= "changeToPath" then
                assert.equals(original, FileChooser[name], name)
            end
        end
        assert.equals(foreign, FileChooser.changeToPath)

        assert.is_nil(rawget(fc, "genItemTableFromPath"))
        assert.is_falsy(
            FileManager.file_dialog_added_buttons
                and FileManager.file_dialog_added_buttons.index
                and FileManager.file_dialog_added_buttons.index.kindle_library
        )

        assert.is_falsy(fixture.library:isBrowsing(fc))
        assert.equals(fixture.dir, fc.path)
        assert.is_truthy(find_file_row(fc, fixture.direct_path))
        assert.is_nil(folder_entry(fc))
    end)

    it("keeps FileManager navigation working after unapply", function()
        local fixture = build_fixture()
        local fc = fixture.file_chooser
        assert.is_true(fixture.library:show(fixture.filemanager, true))
        FileChooserExt:unapply(FileChooser)

        fc:changeToPath(fixture.subdir)
        assert.equals(fixture.subdir, fc.path)
        fc:changeToPath(fixture.dir)
        assert.equals(fixture.dir, fc.path)
        assert.is_true(fc:onMenuSelect({ path = fixture.subdir }))
        assert.equals(fixture.subdir, fc.path)
    end)
end)
