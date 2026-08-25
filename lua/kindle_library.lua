local BookList = require("ui/widget/booklist")
local ButtonDialog = require("ui/widget/buttondialog")
local InfoMessage = require("ui/widget/infomessage")
local UIManager = require("ui/uimanager")
local filemanagerutil = require("apps/filemanager/filemanagerutil")
local logger = require("logger")
local _ = require("gettext")
local CoverBrowserExt = require("lua/coverbrowser_ext")
local T = require("ffi/util").template

local KindleLibrary = {}
KindleLibrary.__index = KindleLibrary

function KindleLibrary:new(virtual_library, cache_manager)
    return setmetatable({
        virtual_library = virtual_library,
        cache_manager = cache_manager,
        ui = nil,
        booklist_menu = nil,
        return_to_library_request = nil,
    }, self)
end

function KindleLibrary:setUI(ui)
    self.ui = ui
end

function KindleLibrary:requestReturnToLibrary(origin_path)
    self.return_to_library_request = {
        origin_path = origin_path,
    }
end

function KindleLibrary:takeReturnToLibraryRequest()
    local request = self.return_to_library_request
    self.return_to_library_request = nil
    return request
end

local function showInfo(text, timeout)
    UIManager:show(InfoMessage:new({ text = text, timeout = timeout or 4 }))
end

function KindleLibrary:close()
    if self.booklist_menu then
        UIManager:close(self.booklist_menu)
        self.booklist_menu = nil
    end
end

function KindleLibrary:buildEntries(force)
    local entries, err = self.virtual_library:getBookEntries(force)
    if not entries then
        return nil, err
    end
    for _, item in ipairs(entries) do
        local book = self.virtual_library:getBook(item.kindle_book_id)
        if book and book.open_mode == "blocked" then
            item.text = item.text .. " [blocked]"
            item.mandatory = self.virtual_library:getBlockedReasonText(book)
        elseif book and book.open_mode == "convert" then
            local cache_state = self.virtual_library:isBookPrepared(book) and " · cached" or " · prepare on open"
            item.mandatory = item.mandatory .. cache_state
        end
    end
    return entries
end

function KindleLibrary:show(ui, force)
    self.ui = ui or self.ui
    if not self.ui then
        return false
    end
    if self.booklist_menu then
        UIManager:close(self.booklist_menu)
        self.booklist_menu = nil
    end

    local entries, err = self:buildEntries(force ~= false)
    if not entries then
        showInfo(_("Failed to build Kindle library:\n") .. (err or _("unknown error")))
        return false
    end
    if #entries == 0 then
        showInfo(_("No Kindle books were found in the Kindle content catalog."))
        return false
    end

    local manager = self
    self.booklist_menu = BookList:new({
        name = "kindle_library",
        title = self.virtual_library.VIRTUAL_LIBRARY_NAME,
        title_bar_left_icon = "appbar.menu",
        onLeftButtonTap = function()
            manager:close()
        end,
        onMenuSelect = function(_, item)
            return manager:openItem(item)
        end,
        onMenuHold = function(_, item)
            return manager:showBookDialog(item)
        end,
        ui = self.ui,
        _manager = self,
        _recreate_func = function()
            manager:show(manager.ui, true)
        end,
    })
    if CoverBrowserExt.apply(self.booklist_menu) then
        logger.info("KindlePlugin: Kindle Library uses a CoverBrowser display mode")
    end
    self.booklist_menu.close_callback = function()
        manager:close()
    end
    self.booklist_menu:switchItemTable(T(_("Kindle Library (%1)"), #entries), entries, -1)
    UIManager:show(self.booklist_menu)
    return true
end

function KindleLibrary:openItem(item)
    local book = item and self.virtual_library:getBook(item.kindle_book_id)
    if not book then
        showInfo(_("Book entry is no longer available."))
        return true
    end
    if book.open_mode == "blocked" then
        showInfo(self.virtual_library:getBlockedReasonText(book))
        return true
    end

    if not book.source_path then
        showInfo(self.virtual_library:getBlockedReasonText({ block_reason = "missing_source" }))
        return true
    end

    -- Request the real Kindle source path. open_file_ext resolves convertible
    -- books to their real cached EPUB only after KOReader's optional open
    -- confirmation has been accepted.
    logger.info("KindlePlugin: requesting native open for:", book.source_path)
    local close_callback = self.booklist_menu and self.booklist_menu.close_callback or nil
    filemanagerutil.openFile(self.ui, book.source_path, function()
        -- This callback runs only after confirmation and cache preparation
        -- succeed, immediately before KOReader opens the real document.
        local file_chooser = self.ui and self.ui.file_chooser
        self:requestReturnToLibrary(file_chooser and file_chooser.path or nil)
        if close_callback then
            close_callback()
        end
    end)
    return true
end

function KindleLibrary:showBookDialog(item)
    local book = item and self.virtual_library:getBook(item.kindle_book_id)
    if not book then
        return true
    end

    local details = book.source_path or _("Cloud-only Kindle entry")
    if book.open_mode == "blocked" then
        details = details .. "\n\n" .. self.virtual_library:getBlockedReasonText(book)
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
                        self:openItem(item)
                    end,
                    enabled = book.open_mode ~= "blocked",
                },
                {
                    text = _("Refresh"),
                    callback = function()
                        UIManager:close(dialog)
                        self.virtual_library:refresh(true)
                        self:show(self.ui, false)
                    end,
                },
            },
            {
                {
                    text = _("Clear Cache"),
                    callback = function()
                        UIManager:close(dialog)
                        if self.cache_manager then
                            local ok, err = self.cache_manager:clearBookCache(book)
                            if not ok then
                                showInfo(_("Failed to clear cache:\n") .. (err or _("unknown error")))
                                return
                            end
                        end
                        self:show(self.ui, false)
                    end,
                    enabled = book.open_mode ~= "direct",
                },
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
    return true
end

return KindleLibrary
