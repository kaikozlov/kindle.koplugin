---
--- ReaderUI extensions for Kindle virtual library.
--- Patches ReaderUI to navigate back to virtual library after closing books
--- and update BookList cache with final reading progress.
--- Adapted from kobo.koplugin/src/readerui_ext.lua (reading_state_sync removed).

local logger = require("logger")

local ReaderUIExt = {}

---
--- Creates a new ReaderUIExt instance.
--- @return table: A new ReaderUIExt instance.
function ReaderUIExt:new()
    local o = {}
    setmetatable(o, self)
    self.__index = self
    return o
end

---
--- Updates BookList cache with final reading progress.
--- @param virtual_path string: Virtual library path.
--- @param doc_settings table: Document settings instance.
local function updateBookListCache(virtual_path, doc_settings)
    if not doc_settings then
        return
    end

    local percent_finished = doc_settings:readSetting("percent_finished")
    if not percent_finished then
        return
    end

    local BookList = require("ui/widget/booklist")
    logger.dbg(
        "KindlePlugin: Updating BookList cache for virtual path:",
        virtual_path,
        "percent:",
        percent_finished * 100
    )
    BookList.setBookInfoCacheProperty(virtual_path, "percent_finished", percent_finished)
end

---
--- Initializes the ReaderUIExt module.
--- @param virtual_library table: Virtual library instance.
--- @param reading_state_sync table|nil: Reading state sync instance (optional).
function ReaderUIExt:init(virtual_library, reading_state_sync)
    self.virtual_library = virtual_library
    self.reading_state_sync = reading_state_sync
    self.original_methods = {}
end

---
--- Applies monkey patches to ReaderUI.
--- Patches showFileManager and onClose for virtual library navigation.
--- @param ReaderUI table: ReaderUI module to patch.
function ReaderUIExt:apply(ReaderUI)
    if not self.virtual_library:isActive() then
        logger.info("KindlePlugin: virtual library not active, skipping ReaderUI patches")
        return
    end

    logger.info("KindlePlugin: Applying ReaderUI monkey patches for virtual library navigation")

    self.original_methods.showFileManager = ReaderUI.showFileManager
    ReaderUI.showFileManager = function(reader_self, file, selected_files)
        if not file or not self.virtual_library:isVirtualPath(file) then
            return self.original_methods.showFileManager(reader_self, file, selected_files)
        end

        logger.info("KindlePlugin: Navigating to virtual library for virtual path:", file)
        return self.original_methods.showFileManager(reader_self, file, selected_files)
    end

    self.original_methods.onClose = ReaderUI.onClose
    ReaderUI.onClose = function(reader_self, full_refresh)
        -- Try document.virtual_path first (set by document_ext for virtual paths)
        -- Fall back to looking up via the open alias (set by showreader_ext)
        local virtual_path = reader_self.document and reader_self.document.virtual_path
        if not virtual_path and reader_self.document and reader_self.document.file then
            virtual_path = self.virtual_library:getVirtualPath(reader_self.document.file)
        end

        self.original_methods.onClose(reader_self, full_refresh)

        if virtual_path and reader_self.doc_settings then
            updateBookListCache(virtual_path, reader_self.doc_settings)

            -- Sync progress to Kindle on book close
            if self.reading_state_sync and self.reading_state_sync:isEnabled() then
                local cde_key = self.reading_state_sync:extractCdeKey(virtual_path, reader_self.doc_settings)
                -- Resolve source_path from the virtual library book data
                local book = self.virtual_library:getBook(virtual_path)
                local source_path = book and book.source_path
                if cde_key or source_path then
                    logger.info("KindlePlugin: Auto-syncing progress to Kindle on book close:",
                        "cde_key:", cde_key, "source_path:", source_path)
                    self.reading_state_sync:syncToKindle(cde_key, source_path, reader_self.doc_settings)
                end
            end
        end
    end
end

---
--- Removes all monkey patches and restores original methods.
--- @param ReaderUI table: ReaderUI module to restore.
function ReaderUIExt:unapply(ReaderUI)
    logger.info("KindlePlugin: Removing ReaderUI monkey patches")

    for method_name, original_method in pairs(self.original_methods) do
        ReaderUI[method_name] = original_method
    end

    self.original_methods = {}
end

return ReaderUIExt
