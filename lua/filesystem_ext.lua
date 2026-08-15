-- Filesystem virtualization for Kindle virtual library files
-- Monkey patches lfs and related filesystem functions to make virtual files appear real
-- Adapted from kobo.koplugin/src/filesystem_ext.lua

local lfs = require("libs/libkoreader-lfs")
local logger = require("logger")

local FilesystemExt = {}

---
--- Initializes the filesystem extension with a virtual library instance.
--- @param virtual_library table: VirtualLibrary instance for path translation.
function FilesystemExt:init(virtual_library)
    self.virtual_library = virtual_library
    self.original_functions = {}
end

---
--- Creates the patched lfs.attributes function with virtual path support.
--- @param virtual_library table: VirtualLibrary instance for path translation.
--- @param original_lfs_attributes function: Original lfs.attributes function.
--- @return function: Patched attributes function that handles virtual paths.
local function createPatchedAttributesFunction(virtual_library, original_lfs_attributes)
    return function(filepath, attribute_name)
        if type(filepath) == "string" and virtual_library:isVirtualPath(filepath) then
            logger.dbg("KindlePlugin: lfs.attributes intercepted virtual path:", filepath)

            if filepath == "KINDLE_VIRTUAL://" or filepath == "KINDLE_VIRTUAL:///" then
                logger.dbg("KindlePlugin: Returning directory attributes for virtual library root")

                if attribute_name then
                    if attribute_name == "mode" then
                        return "directory"
                    end

                    return nil
                end

                return { mode = "directory" }
            end

            local real_path = virtual_library:getRealPath(filepath)
            if real_path then
                logger.dbg("KindlePlugin: Redirecting to real path:", real_path)

                return original_lfs_attributes(real_path, attribute_name)
            end

            logger.dbg("KindlePlugin: Virtual path has no real counterpart:", filepath)

            return nil
        end

        local attributes = original_lfs_attributes(filepath, attribute_name)
        if attributes ~= nil or type(filepath) ~= "string" then
            return attributes
        end

        -- Bookshelf validates persisted paths before calling ReaderUI. When a
        -- generated EPUB has been cleared, let that exact known cache path
        -- continue to showReader, where the normal resolver will regenerate it.
        -- Never synthesize attributes for missing source or arbitrary files.
        local virtual_path = virtual_library:getVirtualPath(filepath)
        local book = virtual_path and virtual_library:getBook(virtual_path) or nil
        local cache_manager = virtual_library.cache_manager
        local cached_path = book and cache_manager and cache_manager:getCachePaths(book) or nil
        if cached_path ~= filepath then
            return nil
        end

        if attribute_name then
            if attribute_name == "mode" then
                return "file"
            elseif attribute_name == "size" then
                return book.source_size or 0
            end
            return nil
        end
        return {
            mode = "file",
            size = book.source_size or 0,
        }
    end
end

---
--- Applies filesystem virtualization patches.
--- Monkey-patches lfs.attributes to transparently redirect virtual paths to real files.
--- Only applies patches if the virtual library is active.
function FilesystemExt:apply()
    if not self.virtual_library:isActive() then
        logger.info("KindlePlugin: virtual library not active, skipping filesystem patches")
        return
    end

    logger.info("KindlePlugin: Applying filesystem virtualization for Kindle virtual library")

    local virtual_library = self.virtual_library
    local original_lfs_attributes = lfs.attributes

    self.original_functions.lfs_attributes = original_lfs_attributes
    lfs.attributes = createPatchedAttributesFunction(virtual_library, original_lfs_attributes)
end

---
--- Removes filesystem virtualization patches.
--- Restores original lfs functions to their unpatched state.
function FilesystemExt:unapply()
    logger.info("KindlePlugin: Removing filesystem virtualization")

    if self.original_functions.lfs_attributes then
        lfs.attributes = self.original_functions.lfs_attributes
    end

    self.original_functions = {}
end

return FilesystemExt
