---
--- BookInfoManager extensions for Kindle virtual library files.
--- Patches CoverBrowser's BookInfoManager to extract metadata from
--- cached EPUBs instead of trying to open KFX files directly.
---
--- When CoverBrowser asks to extract metadata for a virtual path:
--- 1. Resolve to the cached EPUB
--- 2. Extract metadata from the real EPUB (title, authors, cover)
--- 3. Insert a copy into the database under the virtual path keys
---
--- When CoverBrowser asks for bookinfo on a virtual path:
--- Return the copied entry with full metadata and cover.

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

    local vl = self.virtual_library

    -- Patch extractBookInfo: for virtual paths, extract from cached EPUB
    -- and store result under the virtual path.
    self.original_methods.extractBookInfo = BookInfoManager.extractBookInfo
    local orig_extract = BookInfoManager.extractBookInfo

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

        -- Resolve to cached EPUB (may trigger conversion)
        local real_path = vl:resolveBookPath(book)
        if not real_path or not lfs.attributes(real_path, "mode") then
            logger.warn("KindlePlugin: cannot resolve virtual path for extraction:", filepath)
            return nil
        end

        -- Extract metadata from the real cached EPUB file.
        -- This stores bookinfo in the DB under real_path's directory/filename.
        logger.info("KindlePlugin: extracting metadata from cached EPUB:", real_path)
        local ok, result = pcall(orig_extract, bim_self, real_path, cover_specs)
        if not ok then
            logger.warn("KindlePlugin: metadata extraction failed:", result)
            return nil
        end

        -- Copy the DB entry from the real path to the virtual path
        self:cloneBookInfoEntry(BookInfoManager, filepath, real_path)

        return result
    end

    -- Patch getBookInfo: return bookinfo for virtual paths
    self.original_methods.getBookInfo = BookInfoManager.getBookInfo
    local orig_getBookInfo = BookInfoManager.getBookInfo

    BookInfoManager.getBookInfo = function(bim_self, filepath, get_cover)
        if not vl:isVirtualPath(filepath) then
            return orig_getBookInfo(bim_self, filepath, get_cover)
        end

        -- Try virtual path first (may have a cloned entry from extraction)
        local bookinfo = orig_getBookInfo(bim_self, filepath, get_cover)
        if bookinfo then
            return bookinfo
        end

        -- No entry yet. Try looking up the cached EPUB directly.
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

        -- Adapt: rewrite directory/filename to match virtual path
        local directory, filename = util.splitFilePathName(filepath)
        real_bookinfo.directory = directory
        real_bookinfo.filename = filename

        return real_bookinfo
    end

    logger.info("KindlePlugin: BookInfoManager patches applied")
end

--- Clone a bookinfo DB entry from real_path to virtual_path.
--- Uses INSERT OR REPLACE to create a new row with the virtual path's
--- directory/filename but all other columns copied from the real entry.
function BookInfoManagerExt:cloneBookInfoEntry(BookInfoManager, virtual_path, real_path)
    BookInfoManager:openDbConnection()

    local real_dir, real_fn = util.splitFilePathName(real_path)
    local virt_dir, virt_fn = util.splitFilePathName(virtual_path)

    -- Read the real entry
    local row = BookInfoManager.get_stmt:bind(real_dir, real_fn):step()
    if not row then
        BookInfoManager.get_stmt:clearbind():reset()
        logger.dbg("KindlePlugin: no bookinfo entry to clone for:", real_path)
        return
    end

    -- Build new row: same columns but with virtual directory/filename
    local cols = {
        "directory", "filename", "filesize", "filemtime",
        "in_progress", "unsupported", "cover_fetched", "has_meta",
        "has_cover", "cover_sizetag", "ignore_meta", "ignore_cover",
        "pages", "title", "authors", "series", "series_index",
        "language", "keywords", "description",
        "cover_w", "cover_h", "cover_bb_type", "cover_bb_stride",
        "cover_bb_data",
    }

    local dbrow = {}
    for num, col in ipairs(cols) do
        if col == "directory" then
            dbrow[num] = virt_dir
        elseif col == "filename" then
            dbrow[num] = virt_fn
        else
            dbrow[num] = row[num]
        end
    end

    BookInfoManager.get_stmt:clearbind():reset()

    -- Insert with virtual path keys using the prepared set_stmt (INSERT OR REPLACE)
    for num, _ in ipairs(cols) do
        BookInfoManager.set_stmt:bind1(num, dbrow[num])
    end
    BookInfoManager.set_stmt:step()
    BookInfoManager.set_stmt:clearbind():reset()

    logger.info("KindlePlugin: cloned bookinfo from", real_path, "to virtual path")
end

function BookInfoManagerExt:unapply(BookInfoManager)
    if not BookInfoManager or not next(self.original_methods) then
        return
    end

    logger.info("KindlePlugin: removing BookInfoManager patches")
    for method_name, original_method in pairs(self.original_methods) do
        BookInfoManager[method_name] = original_method
    end
    self.original_methods = {}
end

return BookInfoManagerExt
