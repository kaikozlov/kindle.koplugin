require("busted.runner")()
local helper = require("spec/test_helper")

--- Exercises the catalog controller against a real FileManager FileChooser
--- with real temporary document files. LibraryIndex.getBooks is the only
--- stubbed boundary: the rows reaching FileChooser are built by the real
--- VirtualLibrary and the real native FileChooser item machinery.
describe("KindleLibrary native catalog controller", function()
    local lfs = require("libs/libkoreader-lfs")
    local Device = require("device")
    local Screen = Device.screen
    local UIManager = require("ui/uimanager")
    local FileManager = require("apps/filemanager/filemanager")
    local FileChooser = require("ui/widget/filechooser")
    local FileChooserExt = require("lua/filechooser_ext")
    local filemanagerutil = require("apps/filemanager/filemanagerutil")
    local ffiUtil = require("ffi/util")
    local KindleLibrary = require("lua/kindle_library")
    local LibraryIndex = require("lua/library_index")
    local VirtualLibrary = require("lua/virtual_library")

    local original_get_books
    local original_open_file
    local temp_dirs = {}
    local fixtures = {}

    local LONG_TITLE = string.rep("A very long Kindle catalog title ", 14)
    local LONG_AUTHOR = string.rep("Long Author Name ", 18)

    setup(function()
        helper.setup_complete()
        original_open_file = filemanagerutil.openFile
    end)

    before_each(function()
        helper.before_each()
        disable_plugins()
        original_get_books = LibraryIndex.getBooks
        temp_dirs = {}
        fixtures = {}
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
        LibraryIndex.getBooks = original_get_books
        filemanagerutil.openFile = original_open_file
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

    local function write_file(path, content)
        local file = assert(io.open(path, "wb"))
        file:write(content or "placeholder document text\n")
        file:close()
    end

    -- books(dir) must return catalog books whose source files really exist.
    local function build_fixture(books_for, options)
        options = options or {}
        local dir = make_temp_dir()
        local direct_path = dir .. "/direct-book.txt"
        write_file(direct_path)
        local convert_path = dir .. "/convert-book.kfx"
        write_file(convert_path)
        local subdir = dir .. "/subdir"
        assert(lfs.mkdir(subdir))

        local books = books_for({
            dir = dir,
            direct_path = direct_path,
            convert_path = convert_path,
            long_title = LONG_TITLE,
            long_author = LONG_AUTHOR,
        })
        local get_books_calls = {}
        LibraryIndex.getBooks = function(_, force)
            get_books_calls[#get_books_calls + 1] = force
            return books
        end

        local vlib = VirtualLibrary:new(LibraryIndex)
        vlib:setSettings({
            enable_virtual_library = true,
            cache_dir = options.cache_dir,
        })
        local library = KindleLibrary:new(vlib, options.cache_manager)
        local filemanager = FileManager:new({
            dimen = Screen:getSize(),
            root_path = dir,
        })
        UIManager:show(filemanager)
        library:setUI(filemanager)
        FileChooserExt:init(vlib, library)
        FileChooserExt:apply(FileChooser)

        local fixture = {
            library = library,
            vlib = vlib,
            filemanager = filemanager,
            file_chooser = filemanager.file_chooser,
            dir = dir,
            direct_path = direct_path,
            convert_path = convert_path,
            get_books_calls = get_books_calls,
        }
        table.insert(fixtures, fixture)
        return fixture
    end

    local function standard_books(parts)
        return {
            {
                id = "direct",
                source_path = parts.direct_path,
                open_mode = "direct",
                display_name = "Direct Book",
                authors = { "Direct Author" },
                source_size = lfs.attributes(parts.direct_path, "size") or 0,
            },
            {
                id = "convert",
                source_path = parts.convert_path,
                open_mode = "convert",
                display_name = "Convert Book",
                source_size = lfs.attributes(parts.convert_path, "size") or 0,
            },
            {
                id = "blocked",
                open_mode = "blocked",
                block_reason = "drm",
                display_name = "Blocked Book",
            },
        }
    end

    local function find_row(file_chooser, predicate)
        for _, item in ipairs(file_chooser.item_table) do
            if predicate(item) then
                return item
            end
        end
    end

    local function catalog_row(file_chooser, book_id)
        return find_row(file_chooser, function(item)
            return item.kindle_book_id == book_id and not item.kindle_unavailable
        end)
    end

    local function has_normal_file_row(file_chooser, path)
        return find_row(file_chooser, function(item)
            return item.path == path and item.is_file
        end)
    end

    local function latest_info_text()
        for index = #UIManager._shown_widgets, 1, -1 do
            local widget = UIManager._shown_widgets[index]
            if widget and widget.text then
                return widget.text
            end
        end
    end

    it("anchors the catalog in the same real FileChooser directory", function()
        local fixture = build_fixture(standard_books)
        local fc = fixture.file_chooser
        assert.is_true(fixture.library:show(fixture.filemanager, true))
        -- show scans the catalog: the scan boundary sees the forced refresh.
        assert.is_truthy(fixture.get_books_calls[1])
        assert.is_truthy(fixture.library:isBrowsing(fc))
        assert.is_true(fc == fixture.filemanager.file_chooser)
        assert.equals(fixture.dir, fc.path)

        local rows = fc.item_table
        assert.is_truthy(rows[1].is_kindle_library_return)
        assert.is_true(rows[1].is_go_up)
        assert.equals(fixture.dir, rows[1].path)

        local direct = catalog_row(fc, "direct")
        assert.is_truthy(direct)
        assert.equals("file", lfs.attributes(direct.path, "mode"))
        assert.equals(fixture.direct_path, direct.path)
        local convert = catalog_row(fc, "convert")
        assert.is_truthy(convert)
        assert.equals(fixture.convert_path, convert.path)

        assert.is_true(fixture.library:show(fixture.filemanager, false))
        -- The refresh boundary keeps seeing scans, now unforced.
        assert.is_truthy(#fixture.get_books_calls > 1)
        assert.is_falsy(fixture.get_books_calls[#fixture.get_books_calls])
        assert.is_truthy(fixture.library:isBrowsing(fc))
    end)

    it("keeps the normal browser on catalog errors and empty scans", function()
        local fixture = build_fixture(standard_books)
        local fc = fixture.file_chooser

        LibraryIndex.getBooks = function()
            return nil, "catalog boom"
        end
        UIManager:_reset()
        assert.is_false(fixture.library:show(fixture.filemanager, true))
        assert.is_falsy(fixture.library:isBrowsing(fc))
        assert.is_truthy(latest_info_text() and latest_info_text():find("catalog boom", 1, true))
        assert.is_truthy(has_normal_file_row(fc, fixture.direct_path))
        assert.is_nil(find_row(fc, function(item)
            return item.is_kindle_library_return
        end))

        LibraryIndex.getBooks = function()
            return {}
        end
        UIManager:_reset()
        assert.is_false(fixture.library:show(fixture.filemanager, true))
        assert.is_falsy(fixture.library:isBrowsing(fc))
        assert.is_truthy(latest_info_text())
        assert.is_truthy(has_normal_file_row(fc, fixture.direct_path))
    end)

    it("paints long titles and authors in classic mode via native mandatory", function()
        local fixture = build_fixture(function(parts)
            local books = standard_books(parts)
            books[1].display_name = parts.long_title
            books[1].authors = { parts.long_author, "Second Author" }
            return books
        end)
        local fc = fixture.file_chooser
        assert.is_true(fixture.library:show(fixture.filemanager, true))

        local row = catalog_row(fc, "direct")
        assert.is_truthy(row)
        -- Regression for the classic title width bug: the long author string
        -- must never ride along in the native mandatory column.
        assert.is_falsy(row.mandatory and row.mandatory:find(LONG_AUTHOR, 1, true))
        assert.is_falsy(row.mandatory and row.mandatory:find("Second Author", 1, true))

        -- The original failure only surfaced on real painting, so paint the
        -- FileManager for real and inspect the rendered title widgets.
        fixture.filemanager:paintTo(Screen.bb, 0, 0)

        local title_chunk = LONG_TITLE:sub(1, 24)
        local rendered_titles = {}
        local function walk(widget)
            if type(widget) ~= "table" then
                return
            end
            if type(widget.text) == "string" and widget.text:find(title_chunk, 1, true) then
                table.insert(rendered_titles, widget)
            end
            for _, child in ipairs(widget) do
                walk(child)
            end
        end
        walk(fixture.filemanager)
        assert.is_truthy(#rendered_titles >= 1, "long title was not rendered")
        for _, title_widget in ipairs(rendered_titles) do
            local width = title_widget:getSize().w
            assert.is_truthy(width > 0)
            assert.is_truthy(width <= fc.dimen.w)
        end
    end)

    it("closes on native back and restores the real directory listing", function()
        local fixture = build_fixture(standard_books)
        local fc = fixture.file_chooser
        assert.is_true(fixture.library:show(fixture.filemanager, true))
        assert.is_truthy(fixture.library:isBrowsing(fc))

        assert.is_true(fc:onBack())
        assert.is_falsy(fixture.library:isBrowsing(fc))
        assert.equals(fixture.dir, fc.path)
        assert.is_truthy(has_normal_file_row(fc, fixture.direct_path))
        assert.is_nil(find_row(fc, function(item)
            return item.is_kindle_library_return or item.kindle_book_id
        end))
    end)

    it("restores the normal folder on Home when HOME equals the origin", function()
        local fixture = build_fixture(standard_books)
        local fc = fixture.file_chooser
        local home = fixture.dir
        G_reader_settings:saveSetting("home_dir", home)
        assert.equals(home, fc.path)
        assert.is_true(fixture.library:show(fixture.filemanager, true))
        assert.is_truthy(fixture.library:isBrowsing(fc))

        assert.is_true(fc:goHome())
        assert.is_falsy(fixture.library:isBrowsing(fc))
        assert.equals(home, fc.path)
        assert.is_truthy(has_normal_file_row(fc, fixture.direct_path))
        assert.is_nil(find_row(fc, function(item)
            return item.is_kindle_library_return or item.kindle_book_id
        end))
    end)

    it("leaves the catalog when native navigation changes the directory", function()
        local fixture = build_fixture(standard_books)
        local fc = fixture.file_chooser
        local parent = ffiUtil.realpath(fixture.dir .. "/..")
        assert.is_true(fixture.library:show(fixture.filemanager, true))
        assert.is_truthy(fixture.library:isBrowsing(fc))

        fc:changeToPath(parent)
        assert.is_falsy(fixture.library:isBrowsing(fc))
        assert.equals(parent, fc.path)
        assert.is_truthy(find_row(fc, function(item)
            return item.path == fixture.dir and not item.is_file and not item.is_kindle_library_folder
        end))
    end)

    it("schedules the library return only after the accepted open begins", function()
        local fixture = build_fixture(standard_books)
        local fc = fixture.file_chooser
        local opened = {}
        filemanagerutil.openFile = function(ui, path, callback)
            opened[#opened + 1] = { ui = ui, path = path, callback = callback }
        end

        assert.is_true(fixture.library:show(fixture.filemanager, true))
        local row = catalog_row(fc, "direct")
        assert.is_true(fc:onMenuSelect(row))

        assert.equals(1, #opened)
        assert.equals(fixture.filemanager, opened[1].ui)
        assert.equals(fixture.direct_path, opened[1].path)
        assert.is_truthy(fixture.library:isBrowsing(fc))
        assert.is_nil(fixture.library:takeReturnToLibraryRequest())

        opened[1].callback()
        assert.same({ origin_path = fixture.dir }, fixture.library:takeReturnToLibraryRequest())
        assert.is_nil(fixture.library:takeReturnToLibraryRequest())
        assert.is_falsy(fixture.library:isBrowsing(fc))

        -- A callback firing after the catalog was left behind must not arm a
        -- stale return receipt.
        assert.is_true(fixture.library:show(fixture.filemanager, false))
        local captured
        filemanagerutil.openFile = function(_, _, callback)
            captured = callback
        end
        fc:onMenuSelect(catalog_row(fc, "convert"))
        assert.is_function(captured)
        fixture.library:leave()
        captured()
        assert.is_nil(fixture.library:takeReturnToLibraryRequest())
    end)

    it("explains unavailable rows instead of native file actions", function()
        local fixture = build_fixture(standard_books)
        local fc = fixture.file_chooser
        local opened = 0
        filemanagerutil.openFile = function()
            opened = opened + 1
        end

        assert.is_true(fixture.library:show(fixture.filemanager, true))
        local blocked = find_row(fc, function(item)
            return item.kindle_book_id == "blocked"
        end)
        assert.is_truthy(blocked)
        assert.is_true(blocked.kindle_unavailable)
        assert.is_true(blocked.dim)
        assert.is_nil(blocked.is_file)
        assert.is_nil(blocked.file)
        assert.equals(fixture.dir, blocked.path)

        UIManager:_reset()
        assert.is_true(fc:onMenuSelect(blocked))
        assert.equals(0, opened)
        assert.is_truthy(latest_info_text())
        assert.is_truthy(fixture.library:isBrowsing(fc))

        UIManager:_reset()
        assert.is_true(fc:onMenuHold(blocked))
        assert.equals(0, opened)
        assert.is_truthy(latest_info_text())
        assert.is_truthy(fixture.library:isBrowsing(fc))
    end)

    it("opens real rows through the native boundary and delegates select mode", function()
        local fixture = build_fixture(standard_books)
        local fc = fixture.file_chooser
        local opened = {}
        filemanagerutil.openFile = function(ui, path)
            opened[#opened + 1] = { ui = ui, path = path }
        end

        assert.is_true(fixture.library:show(fixture.filemanager, true))
        local row = catalog_row(fc, "direct")

        -- Native select mode keeps its own behavior inside the catalog.
        fixture.filemanager.selected_files = {}
        assert.is_true(fc:onMenuSelect(row))
        assert.equals(0, #opened)
        assert.is_truthy(fixture.filemanager.selected_files[fixture.direct_path])
        assert.is_true(row.dim)
        assert.is_truthy(fixture.library:isBrowsing(fc))

        fixture.filemanager.selected_files = nil
        row.dim = nil
        assert.is_true(fc:onMenuSelect(row))
        assert.equals(1, #opened)
        assert.equals(fixture.filemanager, opened[1].ui)
        assert.equals(fixture.direct_path, opened[1].path)
    end)

    it("adds Kindle actions to the native file dialog and retains the view on cache failure", function()
        local cache_failures = 0
        local fixture = build_fixture(standard_books, {
            cache_manager = {
                clearBookCache = function()
                    cache_failures = cache_failures + 1
                    return false, "permission denied"
                end,
            },
        })
        local fc = fixture.file_chooser
        assert.is_true(fixture.library:show(fixture.filemanager, true))
        local row = catalog_row(fc, "convert")

        -- Exercise the real added action through the native file dialog.
        assert.is_true(fc:onMenuHold(row))
        local dialog = fc.file_dialog
        assert.is_truthy(dialog)
        local clear_cache
        for _, buttons in ipairs(dialog.buttons) do
            for _, button in ipairs(buttons) do
                if button.text == "Clear Kindle cache" then
                    clear_cache = button
                end
            end
        end
        assert.is_truthy(clear_cache)

        -- Direct books expose nothing to clear.
        local direct_buttons = fixture.library:fileDialogButtons(fixture.direct_path, true)
        assert.is_falsy(direct_buttons[2].enabled)
        -- Folders never get Kindle actions.
        assert.is_nil(fixture.library:fileDialogButtons(fixture.dir, false))

        assert.is_truthy(clear_cache.enabled)
        UIManager:_reset()
        local calls_before = #fixture.get_books_calls
        clear_cache.callback()
        assert.equals(1, cache_failures)
        assert.is_truthy(latest_info_text():find("permission denied", 1, true))
        assert.equals(calls_before, #fixture.get_books_calls)
        assert.is_truthy(fixture.library:isBrowsing(fc))
        assert.is_truthy(catalog_row(fc, "convert"))
    end)

    it("swaps owners clearing browsing but preserves a pending return receipt", function()
        local fixture = build_fixture(standard_books)
        local library = fixture.library
        assert.is_true(library:show(fixture.filemanager, true))
        library:requestReturnToLibrary(fixture.dir)

        UIManager:close(fixture.filemanager)
        local second_dir = make_temp_dir()
        local second = FileManager:new({
            dimen = Screen:getSize(),
            root_path = second_dir,
        })
        UIManager:show(second)
        library:setUI(second)
        assert.is_falsy(library:isBrowsing(fixture.file_chooser))
        assert.is_falsy(library:isBrowsing(second.file_chooser))
        assert.same({ origin_path = fixture.dir }, library:takeReturnToLibraryRequest())

        -- The same owner must not be treated as a takeover.
        assert.is_true(library:show(second, true))
        assert.is_truthy(library:isBrowsing(second.file_chooser))
        library:setUI(second)
        assert.is_truthy(library:isBrowsing(second.file_chooser))

        library:close()
        UIManager:close(second)
    end)

    it("closes into the normal folder while FileManager stays alive", function()
        local fixture = build_fixture(standard_books)
        local fc = fixture.file_chooser
        assert.is_true(fixture.library:show(fixture.filemanager, true))

        fixture.library:close()
        assert.is_falsy(fixture.library:isBrowsing(fc))
        assert.equals(FileManager.instance, fixture.filemanager)
        assert.equals(fixture.dir, fc.path)
        assert.is_truthy(has_normal_file_row(fc, fixture.direct_path))
        assert.is_nil(find_row(fc, function(item)
            return item.is_kindle_library_return or item.kindle_book_id
        end))
    end)
end)
