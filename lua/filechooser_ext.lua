local Device = require("device")
local logger = require("logger")

-- Catalog data and navigation only. FileManager keeps its own FileChooser,
-- real directory path, row builder, dialogs, and whichever renderer is active.
local FileChooserExt = {
    applied = false,
    original_methods = {},
    installed_methods = {},
    instances = setmetatable({}, { __mode = "k" }),
    virtual_library = nil,
    kindle_library = nil,
}

local function shouldAddLibraryFolder(fc_self, path)
    if not FileChooserExt.applied or not fc_self or fc_self.name ~= "filemanager" then
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

function FileChooserExt:attach(file_chooser, FileChooser)
    if not file_chooser or file_chooser.name ~= "filemanager" or self.instances[file_chooser] then
        return
    end
    local original = rawget(file_chooser, "genItemTableFromPath")
    local installation = self.installed_methods
    local function getItemTable(chooser, path)
        if self.applied and self.installed_methods == installation and path == chooser.path and self.kindle_library:isBrowsing(chooser) then
            local entries = self.kindle_library:buildEntries(chooser, false)
            if entries then
                return entries
            end
            self.kindle_library:leave()
        end
        -- Dispatch at call time: replacement browsers may wrap the class
        -- method later. Catalog rows must never enter their directory caches.
        local generate = original or FileChooser.genItemTableFromPath
        local items = generate(chooser, path)
        if not self.applied or self.installed_methods ~= installation then
            return items
        end

        local stale_launcher = false
        for _, item in ipairs(items) do
            if item.is_kindle_library_folder then
                stale_launcher = true
                -- Older versions let Zen persist this transient row. Evict
                -- that path through the cache owner so it stays gone on disable.
                if type(chooser._zen_invalidate_item_table_path) == "function" then
                    chooser:_zen_invalidate_item_table_path(path)
                    items = generate(chooser, path)
                end
                break
            end
        end

        local add_launcher = shouldAddLibraryFolder(chooser, path)
        if stale_launcher or add_launcher then
            -- Never mutate a browser-owned array. Normalize legacy markers
            -- even when the browser has no cache invalidation API.
            local entries = {}
            for _, item in ipairs(items) do
                if not item.is_kindle_library_folder then
                    entries[#entries + 1] = item
                end
            end
            if add_launcher then
                table.insert(entries, findInsertPosition(entries), self.virtual_library:createVirtualFolderEntry(path))
            end
            return entries
        end
        return items
    end
    self.instances[file_chooser] = { original = original, installed = getItemTable }
    file_chooser.genItemTableFromPath = getItemTable
end

function FileChooserExt:apply(FileChooser)
    if self.applied then
        return
    end
    local FileManager = require("apps/filemanager/filemanager")
    local original = self.original_methods
    local installation = self.installed_methods
    local function isInstalled()
        return self.applied and self.installed_methods == installation
    end
    local function install(name, method)
        original[name] = FileChooser[name]
        self.installed_methods[name] = method
        FileChooser[name] = method
    end

    install("init", function(chooser, ...)
        if isInstalled() then
            self:attach(chooser, FileChooser)
        end
        return original.init(chooser, ...)
    end)

    install("refreshPath", function(chooser, ...)
        local result = original.refreshPath(chooser, ...)
        if isInstalled() then
            self.kindle_library:updateTitle(chooser)
        end
        return result
    end)

    install("onMenuSelect", function(chooser, item)
        if isInstalled() and chooser.name == "filemanager" and item then
            if item.is_kindle_library_folder then
                self.kindle_library:show(chooser.ui, true)
                return true
            elseif self.kindle_library:isBrowsing(chooser) then
                if item.is_kindle_library_return then
                    self.kindle_library:close()
                    return true
                elseif item.kindle_book_id and (item.kindle_unavailable or not chooser.ui.selected_files) then
                    return self.kindle_library:openItem(item)
                end
            end
        end
        return original.onMenuSelect(chooser, item)
    end)

    install("onMenuHold", function(chooser, item)
        if isInstalled() and chooser.name == "filemanager" and item then
            if item.is_kindle_library_folder then
                self.kindle_library:show(chooser.ui, true)
                return true
            elseif self.kindle_library:isBrowsing(chooser) and item.kindle_unavailable then
                return self.kindle_library:openItem(item)
            end
        end
        return original.onMenuHold(chooser, item)
    end)

    install("changeToPath", function(chooser, ...)
        if isInstalled() and self.kindle_library:isBrowsing(chooser) then
            self.kindle_library:leave()
        end
        return original.changeToPath(chooser, ...)
    end)

    install("goHome", function(chooser, ...)
        if isInstalled() and self.kindle_library:isBrowsing(chooser) then
            self.kindle_library:close()
        end
        return original.goHome(chooser, ...)
    end)

    for _, name in ipairs({ "onBack", "onFolderUp" }) do
        install(name, function(chooser, ...)
            if isInstalled() and self.kindle_library:isBrowsing(chooser) then
                self.kindle_library:close()
                return true
            end
            return original[name](chooser, ...)
        end)
    end

    FileManager:addFileDialogButtons("kindle_library", function(file, is_file)
        return self.kindle_library:fileDialogButtons(file, is_file)
    end)
    self.applied = true
    local ui = self.kindle_library.ui
    self:attach(ui and ui.file_chooser, FileChooser)
    logger.info("KindlePlugin: installed native FileChooser catalog integration")
end

function FileChooserExt:unapply(FileChooser)
    if not self.applied then
        return
    end
    self.applied = false
    require("apps/filemanager/filemanager"):removeFileDialogButtons("kindle_library")
    for chooser, methods in pairs(self.instances) do
        if chooser.genItemTableFromPath == methods.installed then
            chooser.genItemTableFromPath = methods.original
        end
    end
    for name, method in pairs(self.original_methods) do
        -- Do not remove a replacement browser's subsequently installed hook.
        -- Any wrapper retaining ours now delegates without Kindle behavior.
        if FileChooser[name] == self.installed_methods[name] then
            FileChooser[name] = method
        end
    end
    self.instances = setmetatable({}, { __mode = "k" })
    self.original_methods = {}
    self.installed_methods = {}
    self.kindle_library:close()
    logger.info("KindlePlugin: removed native FileChooser catalog integration")
end

return FileChooserExt
