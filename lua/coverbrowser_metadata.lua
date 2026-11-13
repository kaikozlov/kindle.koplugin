local RenderImage = require("ui/renderimage")

-- CoverBrowser reads metadata by real path rather than from FileChooser rows.
-- Supply the Kindle catalog's existing metadata without opening unsupported
-- sources or replacing the user's browser, document provider, or cache.
local CoverBrowserMetadata = {}

function CoverBrowserMetadata:apply(virtual_library)
    self.virtual_library = virtual_library
    if self.installed then
        return
    end
    local manager = package.loaded.bookinfomanager
    if not manager then -- CoverBrowser is optional.
        return
    end
    local original = manager.getBookInfo
    local installed
    installed = function(owner, filepath, get_cover)
        local info = original(owner, filepath, get_cover)
        if self.installed ~= installed then
            return info
        end
        -- Use mappings already built by the catalog. Ordinary file browsing
        -- must not scan cc.db, and converted EPUBs keep native metadata.
        local book = self.virtual_library.books_by_real_path[filepath]
        if not book or filepath ~= book.source_path then
            return info
        end
        -- A supported document without a completed cover lookup still needs
        -- CoverBrowser's normal extraction, which may find an embedded cover.
        if not info or not info.cover_fetched then
            return info
        end
        if info._no_provider then
            -- These flags are CoverBrowser's unsupported-format defaults,
            -- but explicit preferences can still exist in its SQLite cache.
            owner:openDbConnection()
            local stmt = owner.db_conn:prepare("SELECT ignore_meta, ignore_cover FROM bookinfo WHERE directory=? AND filename=?;")
            local prefs = stmt:bind(info.directory, info.filename):step()
            info.ignore_meta = prefs and prefs[1] or nil
            info.ignore_cover = prefs and prefs[2] or nil
            stmt:close()
        end
        if not info.ignore_meta then
            -- CREngine can return an error document as successfully loaded
            -- metadata, making this message the displayed title of an AZW.
            if not info.title or info.title == "ERROR: Error reading PDB format" then
                info.title = book.title or book.display_name
            end
            if not info.authors and book.authors and #book.authors > 0 then
                info.authors = table.concat(book.authors, "\n")
            end
            info.has_meta = info.title and "Y" or info.has_meta
        end
        if get_cover and not info.has_cover and not info.ignore_cover and book.thumbnail and book.thumbnail ~= "" then
            local image = RenderImage:renderImageFile(book.thumbnail)
            if image then
                -- CoverBrowser transfers this buffer to ImageWidget or frees
                -- it itself; every lookup must return its own allocation.
                info.cover_bb = image
                info.cover_w = image:getWidth()
                info.cover_h = image:getHeight()
                info.cover_sizetag = info.cover_w .. "x" .. info.cover_h
                info.has_cover = "Y"
            end
        end
        return info
    end
    self.manager = manager
    self.original = original
    self.installed = installed
    manager.getBookInfo = installed
end

function CoverBrowserMetadata:unapply()
    if self.manager and self.manager.getBookInfo == self.installed then
        self.manager.getBookInfo = self.original
    end
    self.installed = nil
    self.original = nil
    self.manager = nil
    self.virtual_library = nil
end

return CoverBrowserMetadata
