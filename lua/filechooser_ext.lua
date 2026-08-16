local Device = require("device")
local logger = require("logger")

-- Minimal FileChooser integration: expose one synthetic Kindle Library folder
-- from the real file browser. The chooser's path always remains a real path;
-- selecting the synthetic entry opens a BookList owned by kindle_library.lua.
local FileChooserExt = {
    applied = false,
    original_methods = {},
    virtual_library = nil,
    kindle_library = nil,
}

local function shouldAddLibraryFolder(fc_self, path)
    if not fc_self or fc_self.name ~= "filemanager" then
        return false
    end
    if not FileChooserExt.virtual_library or not FileChooserExt.virtual_library:isActive() then
        return false
    end
    if path == "/" then
        return true
    end
    local home_dir = G_reader_settings:readSetting("home_dir") or Device.home_dir
    return home_dir ~= nil and path == home_dir
end

local function findInsertPosition(item_table)
    for i, item in ipairs(item_table) do
        if not item.is_go_up then
            return i
        end
    end
    return #item_table + 1
end

function FileChooserExt:init(virtual_library, kindle_library)
    self.virtual_library = virtual_library
    self.kindle_library = kindle_library
end

function FileChooserExt:apply(FileChooser)
    if self.applied then
        return
    end

    self.original_methods.genItemTable = FileChooser.genItemTable
    self.original_methods.onMenuSelect = FileChooser.onMenuSelect
    self.original_methods.onMenuHold = FileChooser.onMenuHold

    FileChooser.genItemTable = function(fc_self, dirs, files, path)
        local item_table = self.original_methods.genItemTable(fc_self, dirs, files, path)
        if shouldAddLibraryFolder(fc_self, path) then
            local entry = self.virtual_library:createVirtualFolderEntry(path)
            table.insert(item_table, findInsertPosition(item_table), entry)
        end
        return item_table
    end

    FileChooser.onMenuSelect = function(fc_self, item)
        if item and item.is_kindle_library_folder then
            self.kindle_library:setUI(fc_self.ui)
            self.kindle_library:show(fc_self.ui, true)
            return true
        end
        return self.original_methods.onMenuSelect(fc_self, item)
    end

    FileChooser.onMenuHold = function(fc_self, item)
        if item and item.is_kindle_library_folder then
            self.kindle_library:setUI(fc_self.ui)
            self.kindle_library:show(fc_self.ui, true)
            return true
        end
        return self.original_methods.onMenuHold(fc_self, item)
    end

    self.applied = true
    logger.info("KindlePlugin: installed native FileChooser Kindle Library entry")
end

function FileChooserExt:unapply(FileChooser)
    if not self.applied then
        return
    end
    for name, method in pairs(self.original_methods) do
        FileChooser[name] = method
    end
    self.original_methods = {}
    self.applied = false
    logger.info("KindlePlugin: removed FileChooser Kindle Library entry")
end

return FileChooserExt
