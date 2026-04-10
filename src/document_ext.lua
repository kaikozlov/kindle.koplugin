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

local function getDirectProvider(book, original_get_provider, dr_self)
    if book.format == "azw3" then
        return require("document/credocument")
    end

    return original_get_provider(dr_self, book.source_path)
end

function DocumentExt:apply(DocumentRegistry)
    self.original_methods.hasProvider = DocumentRegistry.hasProvider
    self.original_methods.getProvider = DocumentRegistry.getProvider
    self.original_methods.openDocument = DocumentRegistry.openDocument
    self.original_methods.closeDocument = DocumentRegistry.closeDocument
    self.original_methods.getReferenceCount = DocumentRegistry.getReferenceCount

    DocumentRegistry.hasProvider = function(dr_self, file, mimetype, include_aux)
        if self.virtual_library:isVirtualPath(file) then
            return true
        end

        return self.original_methods.hasProvider(dr_self, file, mimetype, include_aux)
    end

    DocumentRegistry.getProvider = function(dr_self, file, include_aux)
        if not self.virtual_library:isVirtualPath(file) then
            return self.original_methods.getProvider(dr_self, file, include_aux)
        end

        local book = self.virtual_library:getBook(file)
        if not book then
            return nil
        end

        if book.open_mode == "blocked" then
            return nil
        end

        if book.open_mode == "convert" then
            return require("document/credocument")
        end

        return getDirectProvider(book, self.original_methods.getProvider, dr_self)
    end

    DocumentRegistry.openDocument = function(dr_self, file, provider)
        if not self.virtual_library:isVirtualPath(file) then
            return self.original_methods.openDocument(dr_self, file, provider)
        end

        if dr_self.registry[file] then
            dr_self.registry[file].refs = dr_self.registry[file].refs + 1
            return dr_self.registry[file].doc
        end

        collectgarbage()
        collectgarbage()

        local book = self.virtual_library:getBook(file)
        if not book then
            showFailure("Book entry is no longer available.")
            return nil
        end

        if book.open_mode == "blocked" then
            showFailure(self.virtual_library:getBlockedReasonText(book))
            return nil
        end

        local actual_file, err = self.virtual_library:resolveBookPath(book)
        if not actual_file then
            showFailure(self.virtual_library:getBlockedReasonText({
                block_reason = err or "conversion_failed",
            }))
            return nil
        end

        if not provider then
            provider = dr_self:getProvider(file)
        end
        if not provider then
            showFailure("No KOReader document provider is available for this book.")
            return nil
        end

        local ok, doc = pcall(provider.new, provider, { file = actual_file })
        if not ok then
            logger.warn("KindlePlugin: failed to open document", actual_file, doc)
            showFailure("Failed to open resolved book path.")
            return nil
        end

        dr_self.registry[file] = {
            doc = doc,
            refs = 1,
        }
        doc.virtual_path = file
        doc.real_path = actual_file

        self.virtual_library:registerOpenAlias(actual_file, file)

        return doc
    end

    DocumentRegistry.closeDocument = function(dr_self, file)
        local canonical = self.virtual_library:getCanonicalPath(file)
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
    for method_name, original_method in pairs(self.original_methods) do
        DocumentRegistry[method_name] = original_method
    end
    self.original_methods = {}
end

return DocumentExt
