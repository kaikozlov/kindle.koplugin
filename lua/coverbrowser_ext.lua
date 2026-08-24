local logger = require("logger")

-- Optional CoverBrowser integration for the Kindle Library BookList.
--
-- CoverBrowser itself patches only KOReader's own BookList owners (history,
-- collections, file search). This module applies the same instance overrides
-- to our native BookList when the CoverBrowser plugin is loaded, so cached
-- converted EPUBs get real cover thumbnails through BookInfoManager exactly
-- like History does. Everything degrades to the classic list when
-- CoverBrowser is absent or in classic mode.
local CoverBrowserExt = {}

local DISPLAY_MODES = {
    mosaic_image = true,
    mosaic_text = true,
    list_image_meta = true,
    list_only_meta = true,
    list_image_filename = true,
}

function CoverBrowserExt.available()
    local ok, BookInfoManager = pcall(require, "bookinfomanager")
    if not ok then
        return false
    end
    local ok_modes = DISPLAY_MODES[BookInfoManager:getSetting("kindle_library_display_mode") or ""]
    local unified = BookInfoManager:getSetting("unified_display_mode")
    local fm_mode = BookInfoManager:getSetting("filemanager_display_mode")
    return (ok_modes or (unified and DISPLAY_MODES[fm_mode or ""])) and true or false
end

function CoverBrowserExt.displayMode()
    local ok, BookInfoManager = pcall(require, "bookinfomanager")
    if not ok then
        return nil
    end
    local mode = BookInfoManager:getSetting("kindle_library_display_mode")
    if not DISPLAY_MODES[mode or ""] then
        if BookInfoManager:getSetting("unified_display_mode") then
            mode = BookInfoManager:getSetting("filemanager_display_mode")
        else
            mode = nil
        end
    end
    return DISPLAY_MODES[mode or ""] and mode or nil, BookInfoManager
end

local function initGrid(menu, BookInfoManager, display_mode)
    if menu.nb_cols_portrait == nil then
        menu.nb_cols_portrait = BookInfoManager:getSetting("nb_cols_portrait") or 3
        menu.nb_cols_landscape = BookInfoManager:getSetting("nb_cols_landscape") or 3
        menu.nb_rows_portrait = BookInfoManager:getSetting("nb_rows_portrait") or 3
        menu.nb_rows_landscape = BookInfoManager:getSetting("nb_rows_landscape") or 2
        menu.files_per_page = BookInfoManager:getSetting("files_per_page")
    end
    menu.display_mode_type = display_mode and display_mode:gsub("_.*", "")
end

--- Apply CoverBrowser display overrides to one native BookList instance.
--- @return boolean: True when a cover display mode was applied.
function CoverBrowserExt.apply(booklist_menu)
    if not booklist_menu then
        return false
    end
    local display_mode = CoverBrowserExt.displayMode()
    if not display_mode then
        return false
    end

    local ok, err = pcall(function()
        local BookInfoManager = require("bookinfomanager")
        local CoverMenu = require("covermenu")
        local BookList = require("ui/widget/booklist")

        booklist_menu.updateItems = CoverMenu.updateItems
        booklist_menu.onCloseWidget = CoverMenu.onCloseWidget
        -- ListMenu/MosaicMenu items ask their owning menu for reading-state
        -- metadata through this instance method.
        booklist_menu.getBookInfo = function(_, file)
            return BookList.getBookInfo(file)
        end

        initGrid(booklist_menu, BookInfoManager, display_mode)
        if booklist_menu.display_mode_type == "mosaic" then
            local MosaicMenu = require("mosaicmenu")
            booklist_menu._recalculateDimen = MosaicMenu._recalculateDimen
            booklist_menu._updateItemsBuildUI = MosaicMenu._updateItemsBuildUI
            booklist_menu._do_cover_images = display_mode ~= "mosaic_text"
            booklist_menu._do_center_partial_rows = true
        else
            local ListMenu = require("listmenu")
            booklist_menu._recalculateDimen = ListMenu._recalculateDimen
            booklist_menu._updateItemsBuildUI = ListMenu._updateItemsBuildUI
            booklist_menu._do_cover_images = display_mode ~= "list_only_meta"
            booklist_menu._do_filename_only = display_mode == "list_image_filename"
        end
        booklist_menu._do_hint_opened = true
    end)

    if not ok then
        logger.warn("KindlePlugin: CoverBrowser integration unavailable:", err)
        return false
    end
    return true
end

return CoverBrowserExt
