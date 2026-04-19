---
--- BookInfoManager extensions for Kindle virtual library files.
--- Patches CoverBrowser's BookInfoManager to provide metadata and covers
--- for virtual library entries from cached EPUB files.
---
--- Strategy (following kobo.koplugin's approach):
--- - extractBookInfo: for virtual paths, completely bypass the original
---   extraction. Open the cached EPUB with crengine to get metadata/cover,
---   build a bookinfo row, and write it directly to the database.
--- - getBookInfo: for virtual paths, return whatever is in the database.
---   If nothing is there yet, return nil (which triggers extraction).
---
--- This avoids the "too many crashes" blacklist that occurs when the
--- original extractBookInfo tries to lfs.attributes() a virtual path.

local DataStorage = require("datastorage")
local logger = require("logger")
local util = require("util")
local lfs = require("libs/libkoreader-lfs")

local BookInfoManagerExt = {}

-- Column set matching BookInfoManager's BOOKINFO_COLS_SET exactly.
local BOOKINFO_COLS_SET = {
    "directory", "filename", "filesize", "filemtime",
    "in_progress", "unsupported", "cover_fetched", "has_meta",
    "has_cover", "cover_sizetag", "ignore_meta", "ignore_cover",
    "pages", "title", "authors", "series", "series_index",
    "language", "keywords", "description",
    "cover_w", "cover_h", "cover_bb_type", "cover_bb_stride",
    "cover_bb_data",
}

local function buildInsertSql()
    local placeholders = {}
    for _ = 1, #BOOKINFO_COLS_SET do
        table.insert(placeholders, "?")
    end
    return "INSERT OR REPLACE INTO bookinfo ("
        .. table.concat(BOOKINFO_COLS_SET, ",")
        .. ") VALUES ("
        .. table.concat(placeholders, ",")
        .. ");"
end

function BookInfoManagerExt:init(virtual_library, cache_manager)
    self.virtual_library = virtual_library
    self.cache_manager = cache_manager
    self.original_methods = {}
    self.db_location = DataStorage:getSettingsDir() .. "/bookinfo_cache.sqlite3"
end

function BookInfoManagerExt:apply(BookInfoManager)
    if not BookInfoManager then
        logger.dbg("KindlePlugin: CoverBrowser not loaded, skipping BookInfoManager patches")
        return
    end

    logger.info("KindlePlugin: applying BookInfoManager patches")

    local vl = self.virtual_library

    -- Patch getBookInfo: just let the database lookup work for virtual paths.
    -- We write entries under the virtual path, so the original lookup works.
    self.original_methods.getBookInfo = BookInfoManager.getBookInfo
    local orig_getBookInfo = BookInfoManager.getBookInfo

    BookInfoManager.getBookInfo = function(bim_self, filepath, get_cover)
        if not vl:isVirtualPath(filepath) then
            return orig_getBookInfo(bim_self, filepath, get_cover)
        end

        -- Try virtual path first
        local info = orig_getBookInfo(bim_self, filepath, get_cover)
        if info then
            return info
        end

        -- For direct-mode books, try looking up the real source path
        local book = vl:getBook(filepath)
        if book and book.open_mode == "direct" and book.source_path then
            return orig_getBookInfo(bim_self, book.source_path, get_cover)
        end

        return nil
    end

    -- Patch extractBookInfo: completely bypass original for virtual paths.
    -- Build bookinfo from the cached EPUB and write directly to DB.
    self.original_methods.extractBookInfo = BookInfoManager.extractBookInfo
    local orig_extract = BookInfoManager.extractBookInfo

    BookInfoManager.extractBookInfo = function(bim_self, filepath, cover_specs)
        if not vl:isVirtualPath(filepath) then
            return orig_extract(bim_self, filepath, cover_specs)
        end

        logger.info("KindlePlugin: extractBookInfo for virtual path:", filepath)

        local book = vl:getBook(filepath)
        if not book then
            logger.warn("KindlePlugin: book not found for virtual path:", filepath)
            return nil
        end

        -- For direct-mode books (AZW, PDF, etc.), let KOReader's native
        -- extraction handle them by passing the real source path.
        if book.open_mode == "direct" then
            logger.info("KindlePlugin: delegating to native extraction for direct book:", book.source_path)
            return orig_extract(bim_self, book.source_path, cover_specs)
        end

        -- First, insert a "in progress" row to prevent the original extractor
        -- from trying again and hitting "too many attempts"
        self:writeInProgressRow(filepath)

        -- Resolve to cached EPUB
        local real_path = vl:resolveBookPath(book)
        if not real_path or not lfs.attributes(real_path, "mode") then
            logger.warn("KindlePlugin: cannot resolve virtual path:", filepath)
            self:writeUnsupportedRow(filepath, "conversion_failed")
            return nil
        end

        -- Build bookinfo from the cached EPUB
        local bookinfo = self:buildBookInfoFromEpub(filepath, real_path, cover_specs ~= nil)
        if not bookinfo then
            logger.warn("KindlePlugin: failed to build bookinfo for:", filepath)
            self:writeUnsupportedRow(filepath, "metadata_extraction_failed")
            return nil
        end

        -- Write the final bookinfo to the database
        bookinfo.in_progress = 0
        bookinfo.cover_fetched = "Y"
        self:writeBookInfoToDb(filepath, bookinfo)

        logger.info("KindlePlugin: wrote bookinfo for", filepath,
            "title:", bookinfo.title, "authors:", bookinfo.authors,
            "has_cover:", bookinfo.has_cover)

        -- Return a truthy value so CoverBrowser knows extraction succeeded
        return true
    end

    logger.info("KindlePlugin: BookInfoManager patches applied")
end

--- Open the cached EPUB with crengine and extract metadata + cover.
function BookInfoManagerExt:buildBookInfoFromEpub(virtual_filepath, real_epub_path, get_cover)
    local directory, filename = util.splitFilePathName(virtual_filepath)
    local file_attr = lfs.attributes(real_epub_path)

    local bookinfo = {
        directory = directory,
        filename = filename,
        filesize = file_attr and file_attr.size or 0,
        filemtime = file_attr and file_attr.modification or 0,
        in_progress = 0,
        unsupported = nil,
        cover_fetched = "Y",
        has_meta = nil,
        has_cover = nil,
        cover_sizetag = nil,
        ignore_meta = nil,
        ignore_cover = nil,
        title = nil,
        authors = nil,
        series = nil,
        series_index = nil,
        language = nil,
        keywords = nil,
        description = nil,
        pages = nil,
    }

    -- Open the EPUB with crengine to get metadata
    local ok, document = pcall(function()
        local DocumentRegistry = require("document/documentregistry")
        local CreDocument = require("document/credocument")
        return DocumentRegistry:openDocument(real_epub_path, CreDocument)
    end)

    if not ok or not document then
        logger.warn("KindlePlugin: failed to open cached EPUB for metadata:", real_epub_path, ok, tostring(document))
        return bookinfo
    end
    logger.info("KindlePlugin: opened EPUB for metadata:", real_epub_path)

    -- Load metadata (not full render)
    if document.loadDocument then
        local load_ok, load_err = pcall(function() document:loadDocument(false) end)
        logger.info("KindlePlugin: loadDocument result:", load_ok, tostring(load_err))
    end

    local ok2, props = pcall(function()
        return document:getProps()
    end)

    if not ok2 then
        logger.warn("KindlePlugin: getProps failed:", tostring(props))
    elseif props then
        logger.info("KindlePlugin: got props:", tostring(props.title), tostring(props.authors))
        if next(props) then
            bookinfo.has_meta = "Y"
            for k, v in pairs(props) do
                bookinfo[k] = v
            end
        end
    else
        logger.warn("KindlePlugin: getProps returned nil")
    end

    -- Extract cover image if requested
    if get_cover then
        local ok3, cover_bb = pcall(function()
            local FileManagerBookInfo = require("apps/filemanager/filemanagerbookinfo")
            return FileManagerBookInfo:getCoverImage(document)
        end)

        if ok3 and cover_bb then
            bookinfo.has_cover = "Y"
            bookinfo.cover_bb = cover_bb
            bookinfo.cover_w = cover_bb:getWidth()
            bookinfo.cover_h = cover_bb:getHeight()
            bookinfo.cover_sizetag = string.format("%dx%d", bookinfo.cover_w, bookinfo.cover_h)
            logger.info("KindlePlugin: got cover:", bookinfo.cover_w, "x", bookinfo.cover_h)
        else
            logger.warn("KindlePlugin: cover extraction failed:", ok3, tostring(cover_bb))
        end
    end

    -- Close the document
    pcall(function() document:close() end)

    return bookinfo
end

--- Write bookinfo directly to the SQLite database.
--- Uses the same INSERT OR REPLACE approach as BookInfoDatabase in kobo.koplugin.
function BookInfoManagerExt:writeBookInfoToDb(filepath, bookinfo)
    local SQ3 = require("lua-ljsqlite3/init")
    local zstd = require("ffi/zstd")

    local db_conn = SQ3.open(self.db_location)
    if not db_conn then
        logger.warn("KindlePlugin: failed to open bookinfo database:", self.db_location)
        return false
    end
    db_conn:set_busy_timeout(5000)

    local directory, filename = util.splitFilePathName(filepath)

    -- Build the row values in column order.
    -- Use explicit numeric keys to avoid Lua nil-hole issues with # and ipairs.
    local dbrow = {
        [1]  = directory,
        [2]  = filename,
        [3]  = bookinfo.filesize or 0,
        [4]  = bookinfo.filemtime or 0,
        [5]  = bookinfo.in_progress or 0,
        [6]  = bookinfo.unsupported,
        [7]  = bookinfo.cover_fetched or "N",
        [8]  = bookinfo.has_meta,
        [9]  = bookinfo.has_cover,
        [10] = bookinfo.cover_sizetag,
        [11] = bookinfo.ignore_meta,
        [12] = bookinfo.ignore_cover,
        [13] = bookinfo.pages,
        [14] = bookinfo.title,
        [15] = bookinfo.authors,
        [16] = bookinfo.series,
        [17] = bookinfo.series_index,
        [18] = bookinfo.language,
        [19] = bookinfo.keywords,
        [20] = bookinfo.description,
    }

    -- Handle cover blob compression (columns 21-25)
    if bookinfo.cover_bb then
        local cover_bb = bookinfo.cover_bb
        local cover_size = cover_bb.stride * cover_bb.h
        local ok, cover_zst_ptr, cover_zst_size = pcall(zstd.zstd_compress, cover_bb.data, cover_size)
        if ok and cover_zst_ptr then
            dbrow[21] = cover_bb.w
            dbrow[22] = cover_bb.h
            dbrow[23] = cover_bb:getType()
            dbrow[24] = tonumber(cover_bb.stride)
            dbrow[25] = SQ3.blob(cover_zst_ptr, cover_zst_size)
        end
        cover_bb:free()
    end

    local insert_sql = buildInsertSql()
    local stmt = db_conn:prepare(insert_sql)
    if not stmt then
        logger.warn("KindlePlugin: failed to prepare INSERT statement")
        db_conn:close()
        return false
    end

    for num = 1, #BOOKINFO_COLS_SET do
        stmt:bind1(num, dbrow[num])
    end

    local ok = pcall(function() stmt:step() end)
    stmt:clearbind():reset()
    db_conn:close()

    if not ok then
        logger.warn("KindlePlugin: failed to write bookinfo to database for:", filepath)
        return false
    end

    return true
end

--- Write a minimal "in progress" row to prevent the original extractor
--- from trying and failing on this virtual path.
function BookInfoManagerExt:writeInProgressRow(filepath)
    local SQ3 = require("lua-ljsqlite3/init")
    local db_conn = SQ3.open(self.db_location)
    if not db_conn then return end
    db_conn:set_busy_timeout(5000)

    local directory, filename = util.splitFilePathName(filepath)

    -- Check existing in_progress count
    local stmt = db_conn:prepare("SELECT in_progress FROM bookinfo WHERE directory=? AND filename=?;")
    if not stmt then db_conn:close(); return end

    stmt:bind(directory, filename)
    local row = stmt:step()
    stmt:clearbind():reset()

    local prev_tries = 0
    if row then
        prev_tries = tonumber(row[1]) or 0
    end

    -- Only write if no existing entry
    if prev_tries == 0 then
        local insert_sql = buildInsertSql()
        local insert_stmt = db_conn:prepare(insert_sql)
        if insert_stmt then
            local dbrow = {}
            dbrow[1] = directory
            dbrow[2] = filename
            for i = 3, 25 do
                if i == 5 then -- in_progress column
                    dbrow[i] = 1
                elseif i == 7 then -- cover_fetched
                    dbrow[i] = "N"
                else
                    dbrow[i] = nil
                end
            end
            for num = 1, #BOOKINFO_COLS_SET do
                insert_stmt:bind1(num, dbrow[num])
            end
            pcall(function() insert_stmt:step() end)
            insert_stmt:clearbind():reset()
        end
    end

    db_conn:close()
end

--- Write an "unsupported" row so CoverBrowser stops trying.
function BookInfoManagerExt:writeUnsupportedRow(filepath, reason)
    local bookinfo = {
        filesize = 0,
        filemtime = 0,
        in_progress = 0,
        unsupported = reason,
        cover_fetched = "Y",
        has_meta = nil,
        has_cover = nil,
    }
    self:writeBookInfoToDb(filepath, bookinfo)
end

--- Clear stale database entries for virtual paths that have unsupported/unsupported
--- status from previous failed extraction attempts.
function BookInfoManagerExt:clearStaleVirtualEntries(BookInfoManager)
    local SQ3 = require("lua-ljsqlite3/init")
    local ok, db_conn = pcall(SQ3.open, self.db_location)
    if not ok or not db_conn then return end
    db_conn:set_busy_timeout(5000)

    -- Delete all entries under the virtual path directory
    local stmt = db_conn:prepare("DELETE FROM bookinfo WHERE directory LIKE 'KINDLE_VIRTUAL://%';")
    if stmt then
        stmt:step()
        stmt:clearbind():reset()
    end

    db_conn:close()
    logger.info("KindlePlugin: cleared stale virtual path bookinfo entries")
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
