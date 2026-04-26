local ButtonDialog = require("ui/widget/buttondialog")
local Device = require("device")
local InfoMessage = require("ui/widget/infomessage")
local PatternUtils = require("lua/lib/pattern_utils")
local UIManager = require("ui/uimanager")
local logger = require("logger")
local _ = require("gettext")

local FileChooserExt = {}

local function findInsertPosition(item_table)
    for i, item in ipairs(item_table) do
        if not item.is_go_up and not item.path:match("/%.$") then
            return i
        end
    end

    return #item_table + 1
end

local function shouldAddVirtualFolder(path)
    if path == "/" then
        return true
    end

    local home_dir = G_reader_settings:readSetting("home_dir") or Device.home_dir
    return home_dir and path == home_dir
end

local function showInfo(text)
    UIManager:show(InfoMessage:new({
        text = text,
        timeout = 4,
    }))
end

local function createBackEntry()
    local home_dir = G_reader_settings:readSetting("home_dir") or Device.home_dir or "/"
    local virtual_prefix = "KINDLE_VIRTUAL://"
    local escaped_prefix = PatternUtils.escape(virtual_prefix)

    -- Avoid using a home_dir that points into the virtual library or cache,
    -- otherwise the back button would loop back into the virtual library.
    if home_dir == virtual_prefix or home_dir == virtual_prefix .. "/"
        or home_dir:match("^" .. escaped_prefix) then
        logger.dbg("KindlePlugin: home_dir points to virtual library, using Device.home_dir for back entry")
        home_dir = Device.home_dir or "/"
    end

    return {
        text = "⬆ ../",
        path = home_dir,
        is_go_up = true,
    }
end

local function openItem(fc_self, item)
    if item.kindle_block_reason then
        showInfo(item.kindle_block_reason_text or "This book cannot be opened yet.")
        return
    end

    local filemanagerutil = require("apps/filemanager/filemanagerutil")
    filemanagerutil.openFile(fc_self.ui, item.path)
end

function FileChooserExt:init(virtual_library, cache_manager, reading_state_sync)
    self.virtual_library = virtual_library
    self.cache_manager = cache_manager
    self.reading_state_sync = reading_state_sync
    self.original_methods = {}
end

function FileChooserExt:showBookDialog(fc_self, item)
    local book = self.virtual_library:getBook(item.path)
    local details = (book and book.source_path or item.kindle_source_path or "") .. "\n\n"
    if book and book.open_mode == "blocked" then
        details = details .. self.virtual_library:getBlockedReasonText(book)
    else
        details = details .. _("Ready to open through the Kindle virtual library.")
    end

    local dialog
    dialog = ButtonDialog:new({
        title = details,
        buttons = {
            {
                {
                    text = _("Open"),
                    callback = function()
                        UIManager:close(dialog)
                        openItem(fc_self, item)
                    end,
                },
                {
                    text = _("Refresh"),
                    callback = function()
                        UIManager:close(dialog)
                        self.virtual_library:refresh(true)
                        fc_self:showKindleVirtualLibrary()
                    end,
                },
            },
            {
                {
                    text = _("Clear Cache"),
                    callback = function()
                        UIManager:close(dialog)
                        local resolved_book = self.virtual_library:getBook(item.path)
                        if resolved_book and self.cache_manager then
                            self.cache_manager:clearBookCache(resolved_book)
                        end
                    end,
                },
                {
                    text = _("Clear Metadata"),
                    callback = function()
                        UIManager:close(dialog)
                        -- Delete the CoverBrowser bookinfo DB row for this virtual path
                        local BookInfoManager = require("bookinfomanager")
                        if BookInfoManager and BookInfoManager.deleteBookInfo then
                            BookInfoManager:deleteBookInfo(item.path)
                            logger.info("KindlePlugin: cleared bookinfo for", item.path)
                        end
                        -- Refresh the list so CoverBrowser re-extracts metadata
                        fc_self:updateItems(1, true)
                    end,
                },
            },
            {
                {
                    text = _("Show Info"),
                    callback = function()
                        UIManager:close(dialog)
                        showInfo(details)
                    end,
                },
            },
        },
    })

    UIManager:show(dialog)
end

function FileChooserExt:apply(FileChooser)
    logger.info("KindlePlugin: applying FileChooser patches")
    self.original_methods.init = FileChooser.init
    self.original_methods.changeToPath = FileChooser.changeToPath
    self.original_methods.refreshPath = FileChooser.refreshPath
    self.original_methods.genItemTable = FileChooser.genItemTable
    self.original_methods.onMenuSelect = FileChooser.onMenuSelect
    self.original_methods.onMenuHold = FileChooser.onMenuHold

    -- Patch FileManager:updateTitleBarPath to show "Kindle Library" instead
    -- of "KINDLE_VIRTUAL://" in the title bar subtitle.
    local FileManager = require("apps/filemanager/filemanager")
    if not self.original_methods.updateTitleBarPath then
        self.original_methods.updateTitleBarPath = FileManager.updateTitleBarPath
    end
    local orig_updateTitleBarPath = self.original_methods.updateTitleBarPath
    local vl = self.virtual_library

    FileManager.updateTitleBarPath = function(fm_self, path)
        if path and path:match("^KINDLE_VIRTUAL://") then
            fm_self.title_bar:setSubTitle(vl.VIRTUAL_LIBRARY_NAME)
        else
            orig_updateTitleBarPath(fm_self, path)
        end
    end
    FileManager.onPathChanged = FileManager.updateTitleBarPath

    local cache_dir = self.cache_manager and self.cache_manager:getCacheDir() or ""
    logger.info("KindlePlugin: FileChooser cache_dir =", cache_dir)

    -- Patch init: when a NEW FileManager is created pointing at the cache
    -- directory (happens when reader closes and creates a fresh FileManager),
    -- redirect to the virtual library. This is the Kobo plugin's approach.
    FileChooser.init = function(fc_self)
        self.original_methods.init(fc_self)

        -- Respect bypass flag set by PathChooserExt
        if self.virtual_library._file_chooser_bypass_active then
            return
        end

        if cache_dir ~= "" and fc_self.path and fc_self.path:sub(1, #cache_dir) == cache_dir then
            logger.info("KindlePlugin: FileChooser initialized with cache path, showing virtual library")
            fc_self:showKindleVirtualLibrary()
        end
    end

    FileChooser.changeToPath = function(fc_self, new_path, ...)
        -- Respect bypass flag set by PathChooserExt
        if self.virtual_library._file_chooser_bypass_active then
            return self.original_methods.changeToPath(fc_self, new_path, ...)
        end

        if new_path and new_path:match("^KINDLE_VIRTUAL://") then
            fc_self:showKindleVirtualLibrary()
            return
        end

        -- Intercept navigation to the cache directory only when returning from
        -- a virtual library book (flag set in showreader_ext). When the flag is
        -- false, the user explicitly navigated here and wants to see cached files.
        if cache_dir ~= "" and new_path and new_path:sub(1, #cache_dir) == cache_dir then
            if self.virtual_library._return_to_virtual_pending then
                logger.info("KindlePlugin: returning from virtual library book, showing virtual library")
                self.virtual_library._return_to_virtual_pending = false
                fc_self:showKindleVirtualLibrary()
                return
            end
            logger.dbg("KindlePlugin: explicit navigation to cache dir, not redirecting")
        end

        return self.original_methods.changeToPath(fc_self, new_path, ...)
    end

    FileChooser.refreshPath = function(fc_self)
        if fc_self.path and fc_self.path:match("^KINDLE_VIRTUAL://") then
            fc_self:showKindleVirtualLibrary()
            return
        end

        return self.original_methods.refreshPath(fc_self)
    end

    FileChooser.genItemTable = function(fc_self, dirs, files, path)
        local item_table = self.original_methods.genItemTable(fc_self, dirs, files, path)

        -- Respect bypass flag set by PathChooserExt
        if self.virtual_library._file_chooser_bypass_active then
            return item_table
        end

        if not shouldAddVirtualFolder(path) then
            return item_table
        end

        local entry = self.virtual_library:createVirtualFolderEntry(path)
        table.insert(item_table, findInsertPosition(item_table), entry)
        return item_table
    end

    FileChooser.onMenuSelect = function(fc_self, item)
        if item.is_kindle_virtual_folder then
            fc_self:showKindleVirtualLibrary()
            return true
        end

        if fc_self.path and fc_self.path:match("^KINDLE_VIRTUAL://") and item.is_go_up then
            fc_self:changeToPath(item.path)
            return true
        end

        if item.path and item.path:match("^KINDLE_VIRTUAL://") and item.kindle_block_reason then
            showInfo(item.kindle_block_reason_text or "This book cannot be opened yet.")
            return true
        end

        if item.path and item.path:match("^KINDLE_VIRTUAL://") then
            openItem(fc_self, item)
            return true
        end

        return self.original_methods.onMenuSelect(fc_self, item)
    end

    FileChooser.onMenuHold = function(fc_self, item)
        if item.path and item.path:match("^KINDLE_VIRTUAL://") then
            self:showBookDialog(fc_self, item)
            return true
        end

        return self.original_methods.onMenuHold(fc_self, item)
    end

    FileChooser.showKindleVirtualLibrary = function(fc_self)
        logger.info("KindlePlugin: showing virtual library")
        fc_self.path = "KINDLE_VIRTUAL://"

        -- Lazy-patch BookInfoManager on first virtual library open to avoid
        -- issues with early loading. Follows kobo.koplugin's approach.
        if not self.bookinfomanager_patched then
            local BookInfoManager = package.loaded["bookinfomanager"]
            if BookInfoManager then
                local BookInfoManagerExt = require("lua/bookinfomanager_ext")
                local bim_ext = BookInfoManagerExt
                bim_ext:init(self.virtual_library, self.cache_manager)
                bim_ext:apply(BookInfoManager)
                logger.info("KindlePlugin: BookInfoManager patches applied (lazy)")
                self.bookinfomanager_patched = true
            end
        end

        self.virtual_library:buildPathMappings()

        local book_entries, err = self.virtual_library:getBookEntries(true)
        if not book_entries then
            showInfo(_("Failed to build Kindle library:\n") .. err)
            return
        end

        if #book_entries == 0 then
            showInfo(_("No Kindle books were found in the configured documents root."))
            return
        end

        for _, item in ipairs(book_entries) do
            local book = self.virtual_library:getBook(item.path)
            if book and book.open_mode == "blocked" then
                item.kindle_block_reason_text = self.virtual_library:getBlockedReasonText(book)
            end
        end

        -- Check if home_dir is locked to the virtual library — if so,
        -- skip the back entry to avoid a navigation loop.
        local virtual_prefix = "KINDLE_VIRTUAL://"
        local escaped_prefix = PatternUtils.escape(virtual_prefix)
        local current_home_dir = G_reader_settings:readSetting("home_dir")
        local home_is_virtual = current_home_dir
            and (current_home_dir == virtual_prefix
                or current_home_dir == virtual_prefix .. "/"
                or current_home_dir:match("^" .. escaped_prefix))
        local locked_at_home = G_reader_settings:isTrue("lock_home_folder") and home_is_virtual

        if not locked_at_home then
            table.insert(book_entries, 1, createBackEntry())
        end

        fc_self:switchItemTable(nil, book_entries, 1, nil, self.virtual_library.VIRTUAL_LIBRARY_NAME)
    end
end

function FileChooserExt:unapply(FileChooser)
    for method_name, original_method in pairs(self.original_methods) do
        FileChooser[method_name] = original_method
    end

    FileChooser.showKindleVirtualLibrary = nil
    self.original_methods = {}
end

return FileChooserExt
