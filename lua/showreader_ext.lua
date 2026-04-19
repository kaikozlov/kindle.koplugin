---
--- Patch ReaderUI:showReader to handle Kindle virtual library paths.
--- Intercepts before lfs.attributes check, resolves to cached EPUB,
--- and delegates to the original showReader with a real file path.

local InfoMessage = require("ui/widget/infomessage")
local UIManager = require("ui/uimanager")
local logger = require("logger")

local ShowReaderExt = {
    original_showReader = nil,
}

function ShowReaderExt:init(virtual_library)
    self.virtual_library = virtual_library
end

function ShowReaderExt:apply()
    local ReaderUI = require("apps/reader/readerui")

    if not self.original_showReader then
        self.original_showReader = ReaderUI.showReader
    end
    if not self.original_showFileManager then
        self.original_showFileManager = ReaderUI.showFileManager
    end

    local virtual_library = self.virtual_library
    local original_showReader = self.original_showReader
    local original_showFileManager = self.original_showFileManager

    ReaderUI.showReader = function(reader_self, file, provider, seamless, is_provider_forced, after_open_callback)
        if not virtual_library:isVirtualPath(file) then
            return original_showReader(reader_self, file, provider, seamless, is_provider_forced, after_open_callback)
        end

        logger.info("KindlePlugin: showReader intercepting virtual path:", file)

        local book = virtual_library:getBook(file)
        if not book then
            logger.warn("KindlePlugin: book not found for virtual path:", file)
            UIManager:show(InfoMessage:new({
                text = "Book not found in Kindle library index.",
                timeout = 3,
            }))
            return
        end

        if book.open_mode == "blocked" then
            local reason = virtual_library:getBlockedReasonText(book)
            logger.warn("KindlePlugin: book is blocked:", reason)
            UIManager:show(InfoMessage:new({
                text = reason,
                timeout = 4,
            }))
            return
        end

        -- Resolve to real file (may trigger KFX→EPUB conversion + caching)
        local real_file, err = virtual_library:resolveBookPath(book)
        if not real_file then
            logger.warn("KindlePlugin: failed to resolve book:", err or "unknown")
            UIManager:show(InfoMessage:new({
                text = virtual_library:getBlockedReasonText({
                    block_reason = err or "conversion_failed",
                }),
                timeout = 4,
            }))
            return
        end

        logger.info("KindlePlugin: resolved virtual path to:", real_file)

        -- Register the alias so DocumentRegistry/closeDocument can find it
        virtual_library:registerOpenAlias(real_file, file)

        -- Use credocument provider for converted/DRM books
        if not provider then
            provider = require("document/credocument")
        end

        -- Delegate to original showReader with the real file
        return original_showReader(reader_self, real_file, provider, seamless, is_provider_forced, after_open_callback)
    end

    -- Patch showFileManager: when closing a book that was opened from the
    -- virtual library, schedule returning to the Kindle Library on next tick.
    ReaderUI.showFileManager = function(reader_self, file, selected_files)
        if file and virtual_library:isOpenAlias(file) then
            logger.info("KindlePlugin: scheduling return to Kindle Library after close")
            UIManager:scheduleIn(0.1, function()
                local FileChooser = require("ui/widget/filechooser")
                local FileManager = require("apps/filemanager/filemanager")
                if FileChooser.showKindleVirtualLibrary and FileManager.instance then
                    FileManager.instance.path = "KINDLE_VIRTUAL://"
                    FileChooser.showKindleVirtualLibrary(FileManager.instance.file_chooser or FileManager.instance)
                end
            end)
        end
        return original_showFileManager(reader_self, file, selected_files)
    end

    logger.info("KindlePlugin: patched ReaderUI:showReader for virtual library paths")
end

function ShowReaderExt:unapply()
    if not self.original_showReader then
        return
    end

    local ReaderUI = require("apps/reader/readerui")
    ReaderUI.showReader = self.original_showReader
    if self.original_showFileManager then
        ReaderUI.showFileManager = self.original_showFileManager
    end
    logger.info("KindlePlugin: restored original ReaderUI:showReader")
    self.original_showReader = nil
    self.original_showFileManager = nil
end

return ShowReaderExt
