local BD = require("ui/bidi")
local ConfirmBox = require("ui/widget/confirmbox")
local InfoMessage = require("ui/widget/infomessage")
local Trapper = require("ui/trapper")
local UIManager = require("ui/uimanager")
local filemanagerutil = require("apps/filemanager/filemanagerutil")
local logger = require("logger")
local _ = require("gettext")
local T = require("ffi/util").template

-- Narrow open-boundary hook for derived Kindle documents.
--
-- KOReader persists real cached EPUB paths in History/Collections. A persisted
-- path may later be stale (source changed/converter bumped) or missing (cache
-- cleared). filemanagerutil.openFile() is the last common boundary before
-- FileManager/ReaderUI provider selection, so refresh the real cache there and
-- then delegate to KOReader unchanged. No virtual path or provider is exposed.
local OpenFileExt = {
    applied = false,
    original_open_file = nil,
    virtual_library = nil,
    cache_manager = nil,
}

function OpenFileExt:init(virtual_library, cache_manager)
    self.virtual_library = virtual_library
    self.cache_manager = cache_manager
end

local function showFailure(text)
    UIManager:show(InfoMessage:new({
        text = text,
        timeout = 4,
    }))
end

function OpenFileExt:prepareKnownKindlePath(file)
    local book = self.virtual_library and self.virtual_library:getBook(file) or nil
    if not book then
        return file
    end

    if book.open_mode == "blocked" then
        return nil, self.virtual_library:getBlockedReasonText(book)
    end

    local expected_cache
    if self.cache_manager and book.open_mode ~= "direct" then
        expected_cache = self.cache_manager:getCachePaths(book)
    end

    local needs_prepare = book.open_mode ~= "direct"
        and (file == expected_cache
            or (file == book.source_path and self.virtual_library:isActive()))

    if needs_prepare then
        -- CacheManager compares against the real source file's size/mtime, so
        -- there is no need to rescan cc.db on every open. Refresh the catalog
        -- only when the mapped source itself disappeared (for example after a
        -- Kindle redownload moved it to a new location).
        local source = book.source_path and io.open(book.source_path, "rb") or nil
        if source then
            source:close()
        else
            local refreshed = self.virtual_library:refresh(true)
            if refreshed then
                book = self.virtual_library:getBook(file)
                    or self.virtual_library:getBook(book.id)
                    or book
            end
        end

        local fresh, cache_path = self.cache_manager:isFresh(book)
        if not fresh or file ~= cache_path then
            local resolved, err
            Trapper:wrap(function()
                local title = book.display_name or book.title or _("book")
                Trapper:info(T(_("Preparing %1…\nThis may take a moment."), title))
                resolved, err = self.virtual_library:resolveBookPath(book)
                Trapper:clear()
            end)
            if not resolved then
                logger.warn("KindlePlugin: failed to refresh persisted Kindle path:", err or "unknown")
                return nil, self.virtual_library:getBlockedReasonText({
                    block_reason = err or "conversion_failed",
                })
            end
            file = resolved
        end
    end

    return file
end

function OpenFileExt:apply()
    if self.applied then
        return
    end

    self.original_open_file = filemanagerutil.openFile

    local function openResolved(ui, file, caller_pre_callback)
        local resolved, err = self:prepareKnownKindlePath(file)
        if not resolved then
            showFailure(err or _("Failed to prepare this book for reading."))
            return
        end
        -- The confirmation, if any, has already happened at this point.
        return self.original_open_file(ui, resolved, caller_pre_callback, true)
    end

    filemanagerutil.openFile = function(ui, file, caller_pre_callback, no_dialog)
        local book = self.virtual_library and self.virtual_library:getBook(file) or nil
        if book and not no_dialog and G_reader_settings:isTrue("file_ask_to_open") then
            UIManager:show(ConfirmBox:new({
                text = _("Open this file?") .. "\n\n" .. BD.filename(file:match("([^/]+)$")),
                ok_text = _("Open"),
                ok_callback = function()
                    openResolved(ui, file, caller_pre_callback)
                end,
            }))
            return
        end

        if book then
            return openResolved(ui, file, caller_pre_callback)
        end
        return self.original_open_file(ui, file, caller_pre_callback, no_dialog)
    end

    self.applied = true
    logger.info("KindlePlugin: installed real-path Kindle open resolver")
end

function OpenFileExt:unapply()
    if not self.applied then
        return
    end
    filemanagerutil.openFile = self.original_open_file
    self.original_open_file = nil
    self.applied = false
    logger.info("KindlePlugin: removed real-path Kindle open resolver")
end

return OpenFileExt
