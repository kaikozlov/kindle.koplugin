---
--- Document provider extensions for Kindle KFX files.
--- Monkey patches DocumentRegistry to handle virtual Kindle library paths.
--- Follows the kobo.koplugin pattern: only patches hasProvider, getProvider,
--- and openDocument. Does NOT patch closeDocument or getReferenceCount,
--- since showreader_ext resolves virtual→real before the document is opened.

local InfoMessage = require("ui/widget/infomessage")
local UIManager = require("ui/uimanager")
local logger = require("logger")

local DocumentExt = {}

local function showFailure(text)
    UIManager:show(InfoMessage:new({
        text = text,
        timeout = 4,
    }))
end

function DocumentExt:init(virtual_library, cache_manager)
    self.virtual_library = virtual_library
    self.cache_manager = cache_manager
    self.original_methods = {}
end

function DocumentExt:apply(DocumentRegistry)
    self.original_methods.hasProvider = DocumentRegistry.hasProvider
    self.original_methods.getProvider = DocumentRegistry.getProvider
    self.original_methods.openDocument = DocumentRegistry.openDocument

    logger.info("KindlePlugin: applying DocumentRegistry patches")

    DocumentRegistry.hasProvider = function(dr_self, file, mimetype, include_aux)
        if self.virtual_library:isVirtualPath(file) then
            logger.dbg("KindlePlugin: hasProvider virtual path:", file)
            return true
        end

        return self.original_methods.hasProvider(dr_self, file, mimetype, include_aux)
    end

    DocumentRegistry.getProvider = function(dr_self, file, include_aux)
        if self.virtual_library:isVirtualPath(file) then
            logger.dbg("KindlePlugin: getProvider virtual path:", file)
            return require("document/credocument")
        end

        return self.original_methods.getProvider(dr_self, file, include_aux)
    end

    DocumentRegistry.openDocument = function(dr_self, file, provider)
        if not self.virtual_library:isVirtualPath(file) then
            return self.original_methods.openDocument(dr_self, file, provider)
        end

        -- Virtual path — resolve to real file and open via original
        logger.info("KindlePlugin: openDocument virtual path:", file)

        local book = self.virtual_library:getBook(file)
        if not book then
            logger.warn("KindlePlugin: book not found for virtual path:", file)
            showFailure("Book entry is no longer available.")
            return nil
        end

        if book.open_mode == "blocked" then
            showFailure(self.virtual_library:getBlockedReasonText(book))
            return nil
        end

        -- Resolve to real file path (may trigger conversion/cache)
        local actual_file, err = self.virtual_library:resolveBookPath(book)
        if not actual_file then
            logger.warn("KindlePlugin: failed to resolve book path:", err or "unknown")
            showFailure(self.virtual_library:getBlockedReasonText({
                block_reason = err or "conversion_failed",
            }))
            return nil
        end

        logger.info("KindlePlugin: resolved virtual path to:", actual_file)

        if not provider then
            provider = require("document/credocument")
        end

        -- Open via the original openDocument with the real file path.
        -- The document is registered under the real path in DocumentRegistry,
        -- so closeDocument/getReferenceCount work natively without patching.
        local doc = self.original_methods.openDocument(dr_self, actual_file, provider)
        if doc then
            doc.virtual_path = file
            logger.info("KindlePlugin: document opened successfully:", file, "->", actual_file)
        end

        return doc
    end
end

function DocumentExt:unapply(DocumentRegistry)
    logger.info("KindlePlugin: removing DocumentRegistry patches")
    for method_name, original_method in pairs(self.original_methods) do
        DocumentRegistry[method_name] = original_method
    end
    self.original_methods = {}
end

return DocumentExt
