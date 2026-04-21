-- User patch to add separator to last menu item in filemanager_settings
-- This provides visual separation for plugin-added menu items
-- Adapted from kobo.koplugin/patches/2-filemanager-menu-separator.lua
-- Priority: 2

local logger = require("logger")

local function patchFileManagerMenu()
    local ok, FileManagerMenu = pcall(require, "apps/filemanager/filemanagermenu")
    if not ok then
        logger.warn("KindlePlugin: Could not load FileManagerMenu for patching")
        return
    end

    local original_getStartWithMenuTable = FileManagerMenu.getStartWithMenuTable

    function FileManagerMenu:getStartWithMenuTable()
        local menu_table = original_getStartWithMenuTable(self)
        menu_table.separator = true
        return menu_table
    end

    logger.info("KindlePlugin: Added separator to filemanager_settings menu")
end

patchFileManagerMenu()

return true
