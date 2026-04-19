---
--- BookInfoManager extensions for Kindle virtual library files.
--- Patches CoverBrowser's BookInfoManager to extract metadata from
--- cached EPUBs instead of trying to open KFX files directly.
---
--- Strategy: when CoverBrowser asks to extract metadata for a virtual
--- path, we resolve it to the cached EPUB and extract from that instead.
--- We store the result under the virtual path's directory/filename keys
--- so CoverBrowser finds it on future lookups.

local logger = require("logger")
local util = require("util")
local lfs = require("libs/libkoreader-lfs")

local BookInfoManagerExt = {}

function BookInfoManagerExt:init(virtual_library, cache_manager)
    self.virtual_library = virtual_library
    self.cache_manager = cache_manager
    self.original_methods = {}
end

function BookInfoManagerExt:apply(BookInfoManager)
    -- CoverBrowser plugin may not be loaded
    if not BookInfoManager then
        logger.dbg("KindlePlugin: CoverBrowser not loaded, skipping BookInfoManager patches")
        return
    end

    logger.info("KindlePlugin: applying BookInfoManager patches")

    -- Patch extractBookInfo: for virtual paths, extract metadata from the
    -- cached EPUB file instead of the KFX original.
    self.original_methods.extractBookInfo = BookInfoManager.extractBookInfo
    local orig_extract = BookInfoManager.extractBookInfo
    local vl = self.virtual_library

    BookInfoManager.extractBookInfo = function(bim_self, filepath, cover_specs)
        if not vl:isVirtualPath(filepath) then
            return orig_extract(bim_self, filepath, cover_specs)
        end

        logger.dbg("KindlePlugin: extractBookInfo for virtual path:", filepath)

        local book = vl:getBook(filepath)
        if not book then
            logger.warn("KindlePlugin: book not found for virtual path:", filepath)
            return nil
        end

        -- Resolve to cached EPUB (may trigger conversion if not cached)
        local real_path = vl:resolveBookPath(book)
        if not real_path then
            logger.warn("KindlePlugin: cannot resolve virtual path for extraction:", filepath)
            return nil
        end

        if not lfs.attributes(real_path, "mode") then
            logger.warn("KindlePlugin: cached EPUB does not exist:", real_path)
            return nil
        end

        -- Extract metadata from the real cached EPUB file.
        -- This gives us a proper bookinfo row with cover, title, authors, etc.
        logger.info("KindlePlugin: extracting metadata from cached EPUB:", real_path)
        local ok, result = pcall(orig_extract, bim_self, real_path, cover_specs)
        if not ok then
            logger.warn("KindlePlugin: metadata extraction failed:", result)
            return nil
        end

        -- Now the metadata is stored in CoverBrowser's database under the
        -- real EPUB's directory/filename. We need to copy that entry to also
        -- be indexed under the virtual path so future getBookInfo calls find it.
        self:copyBookInfoToVirtualPath(BookInfoManager, filepath, real_path)

        return result
    end

    -- Patch getBookInfo: try the database under the virtual path first,
    -- then fall back to looking up by the cached EPUB's path.
    self.original_methods.getBookInfo = BookInfoManager.getBookInfo
    local orig_getBookInfo = BookInfoManager.getBookInfo

    BookInfoManager.getBookInfo = function(bim_self, filepath, get_cover)
        if not vl:isVirtualPath(filepath) then
            return orig_getBookInfo(bim_self, filepath, get_cover)
        end

        -- Try virtual path first (may already have a copied entry)
        local bookinfo = orig_getBookInfo(bim_self, filepath, get_cover)
        if bookinfo then
            return bookinfo
        end

        -- Fall back: look up the cached EPUB's entry and adapt
        local book = vl:getBook(filepath)
        if not book then
            return nil
        end

        local real_path = vl:resolveBookPath(book)
        if not real_path then
            return nil
        end

        local real_bookinfo = orig_getBookInfo(bim_self, real_path, get_cover)
        if not real_bookinfo then
            return nil
        end

        -- Adapt the real bookinfo to have the virtual path's directory/filename
        -- so CoverBrowser sees it as belonging to the virtual path
        local directory, filename = util.splitFilePathName(filepath)
        real_bookinfo.directory = directory
        real_bookinfo.filename = filename

        return real_bookinfo
    end

    logger.info("KindlePlugin: BookInfoManager patches applied")
end

--- Copy a bookinfo entry from the real EPUB path to the virtual path
--- in CoverBrowser's SQLite database.
function BookInfoManagerExt:copyBookInfoToVirtualPath(BookInfoManager, virtual_path, real_path)
    local ok, bookinfo = pcall(function()
        return self.original_methods.getBookInfo(BookInfoManager, real_path, false)
    end)
    if not ok or not bookinfo then
        logger.dbg("KindlePlugin: no bookinfo to copy for:", real_path)
        return
    end

    -- Rewrite directory/filename to match the virtual path
    local directory, filename = util.splitFilePathName(virtual_path)
    bookinfo.directory = directory
    bookinfo.filename = filename

    -- Write via setBookInfoProperties-style approach
    local props = {}
    local text_cols = {
        "filesize", "filemtime", "in_progress", "unsupported",
        "cover_fetched", "has_meta", "has_cover", "cover_sizetag",
        "ignore_meta", "ignore_cover", "pages",
        "title", "authors", "series", "series_index", "language",
        "keywords", "description",
        "cover_w", "cover_h", "cover_bb_type", "cover_bb_stride",
    }

    for _, col in ipairs(text_cols) do
        if bookinfo[col] ~= nil then
            props[col] = bookinfo[col]
        end
    end

    -- Use setBookInfoProperties to write under virtual path
    local ok2, err = pcall(function()
        BookInfoManager:setBookInfoProperties(virtual_path, props)
    end)
    if not ok2 then
        logger.dbg("KindlePlugin: failed to copy bookinfo to virtual path:", err)
    else
        logger.dbg("KindlePlugin: copied bookinfo from", real_path, "to virtual path")
    end
end

function BookInfoManagerExt:unapply(BookInfoManager)
    if not BookInfoManager or not self.original_methods then
        return
    end

    logger.info("KindlePlugin: removing BookInfoManager patches")
    for method_name, original_method in pairs(self.original_methods) do
        BookInfoManager[method_name] = original_method
    end
    self.original_methods = {}
end

return BookInfoManagerExt
