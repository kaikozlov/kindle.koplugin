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
    self.original_methods.closeDocument = DocumentRegistry.closeDocument
    self.original_methods.getReferenceCount = DocumentRegistry.getReferenceCount

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

        logger.info("KindlePlugin: openDocument virtual path:", file)

        -- Already opened? Return cached document.
        if dr_self.registry[file] then
            logger.dbg("KindlePlugin: returning cached document for:", file)
            dr_self.registry[file].refs = dr_self.registry[file].refs + 1
            return dr_self.registry[file].doc
        end

        local book = self.virtual_library:getBook(file)
        if not book then
            logger.warn("KindlePlugin: book not found for virtual path:", file)
            showFailure("Book entry is no longer available.")
            return nil
        end

        logger.dbg("KindlePlugin: book found:", book.title or book.display_name,
            "open_mode:", book.open_mode, "source:", book.source_path)

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

        -- Register the mapping so close/getReference work correctly
        self.virtual_library:registerOpenAlias(actual_file, file)

        -- Open via the original openDocument with the real file path
        if not provider then
            provider = require("document/credocument")
        end

        local doc = self.original_methods.openDocument(dr_self, actual_file, provider)
        if not doc then
            logger.warn("KindlePlugin: original openDocument returned nil for:", actual_file)
            return nil
        end

        -- Store reference under the virtual path for future lookups
        dr_self.registry[file] = {
            doc = doc,
            refs = 1,
        }
        doc.virtual_path = file

        logger.info("KindlePlugin: document opened successfully:", file, "->", actual_file)

        return doc
    end

    DocumentRegistry.closeDocument = function(dr_self, file)
        local canonical = self.virtual_library:getCanonicalPath(file)
        logger.dbg("KindlePlugin: closeDocument:", file, "canonical:", canonical)
        local refs = self.original_methods.closeDocument(dr_self, canonical)
        if refs == 0 then
            self.virtual_library:clearOpenAlias(canonical)
        end
        return refs
    end

    DocumentRegistry.getReferenceCount = function(dr_self, file)
        return self.original_methods.getReferenceCount(dr_self, self.virtual_library:getCanonicalPath(file))
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
