require("busted.runner")()
local helper = require("spec/test_helper")

describe("VirtualLibrary real-path model", function()
    local VirtualLibrary

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/virtual_library"] = nil
        VirtualLibrary = require("lua/virtual_library")
    end)

    it("indexes source and deterministic cache paths without exposing virtual files", function()
        local books = {
            {
                id = "b1",
                display_name = "Book One",
                source_path = "/documents/book.kfx",
                open_mode = "convert",
            },
        }
        local vlib = VirtualLibrary:new({
            getBooks = function()
                return books
            end,
        })
        vlib:setCacheManager({
            getCachePaths = function()
                return "/cache/b1.epub", "/cache/b1.json"
            end,
        })

        vlib:buildMappings(false)

        assert.equals(books[1], vlib:getBook("b1"))
        assert.equals(books[1], vlib:getBook("/documents/book.kfx"))
        assert.equals(books[1], vlib:getBook("/cache/b1.epub"))
        assert.is_nil(books[1].virtual_path)
    end)

    it("lazily rebuilds mappings for a cold-start cached EPUB", function()
        local calls = 0
        local books = {
            { id = "b1", source_path = "/documents/book.kfx", open_mode = "convert" },
        }
        local vlib = VirtualLibrary:new({
            getBooks = function()
                calls = calls + 1
                return books
            end,
        })
        vlib:setSettings({ cache_dir = "/cache" })
        vlib:setCacheManager({
            getCachePaths = function()
                return "/cache/b1.epub"
            end,
        })

        assert.equals(books[1], vlib:getBook("/cache/b1.epub"))
        assert.equals(1, calls)
    end)

    it("does not scan the Kindle library for an unrelated KOReader path", function()
        local calls = 0
        local vlib = VirtualLibrary:new({
            getBooks = function()
                calls = calls + 1
                return {}
            end,
        })
        vlib:setSettings({ cache_dir = "/cache" })

        assert.is_nil(vlib:getBook("/mnt/us/books/unrelated.epub"))
        assert.equals(0, calls)
    end)

    it("does not repeat a failed lazy mapping probe until an explicit refresh", function()
        local calls = 0
        local vlib = VirtualLibrary:new({
            getBooks = function()
                calls = calls + 1
                return nil, "scan failed"
            end,
        })
        vlib:setSettings({ cache_dir = "/cache" })
        vlib:setCacheManager({
            getCachePaths = function()
                return "/cache/book.epub"
            end,
        })

        assert.is_nil(vlib:getBook("/cache/book.epub"))
        assert.is_nil(vlib:getBook("/cache/book.epub"))
        assert.equals(1, calls)

        local books, err = vlib:refresh(true)
        assert.is_nil(books)
        assert.equals("scan failed", err)
        assert.equals(2, calls)
    end)

    it("keeps cloud-only catalog entries without using nil as a path key", function()
        local book = {
            id = "cc:cloud",
            source_path = nil,
            open_mode = "blocked",
            block_reason = "missing_source",
        }
        local vlib = VirtualLibrary:new({
            getBooks = function()
                return { book }
            end,
        })
        local result = vlib:buildMappings(false)
        assert.equals(1, #result)
        assert.equals(book, vlib:getBook("cc:cloud"))
        assert.is_nil(vlib:getBook("cc:unknown"))
    end)

    it("creates a synthetic folder entry whose path remains a real directory", function()
        local vlib = VirtualLibrary:new({})
        vlib:setSettings({})
        local entry = vlib:createVirtualFolderEntry("/mnt/us")
        assert.is_true(entry.is_kindle_library_folder)
        assert.equals("/mnt/us", entry.path)
        assert.equals("directory", entry.attr.mode)
    end)

    it("formats actionable DRM preparation failures", function()
        local vlib = VirtualLibrary:new({})

        assert.is_truthy(vlib:getBlockedReasonText({
            block_reason = "drm_extractor_unavailable",
        }):match("kfxdedrm"))
        assert.is_truthy(vlib:getBlockedReasonText({
            block_reason = "drm_key_extraction_failed",
        }):match("debug log"))
        assert.is_truthy(vlib:getBlockedReasonText({
            block_reason = "drm_after_key_extraction",
        }):match("re%-downloading"))
    end)

    it("returns direct paths unchanged and converts only on explicit open", function()
        local direct = { id = "pdf", source_path = "/documents/book.pdf", open_mode = "direct" }
        local convert = { id = "kfx", source_path = "/documents/book.kfx", open_mode = "convert" }
        local conversions = 0
        local vlib = VirtualLibrary:new({
            getBooks = function()
                return { direct, convert }
            end,
        })
        vlib:setCacheManager({
            isFresh = function(_, book)
                if book.id == "kfx" then
                    return false, "/cache/kfx.epub"
                end
                return false
            end,
            getCachePaths = function(_, book)
                return "/cache/" .. book.id .. ".epub"
            end,
            ensureCachedEpub = function()
                conversions = conversions + 1
                return "/cache/kfx.epub"
            end,
        })
        vlib:buildMappings(false)

        assert.equals("/documents/book.pdf", vlib:resolveBookPath(direct))
        assert.equals(0, conversions)
        assert.equals("/cache/kfx.epub", vlib:resolveBookPath(convert))
        assert.equals(1, conversions)
    end)
end)

describe("VirtualLibrary native catalog rows", function()
    local VirtualLibrary
    local FileChooser
    local DocumentRegistry
    local ffiUtil
    local lfs
    local original_show_filter

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/virtual_library"] = nil
        VirtualLibrary = require("lua/virtual_library")
        FileChooser = require("ui/widget/filechooser")
        DocumentRegistry = require("document/documentregistry")
        ffiUtil = require("ffi/util")
        lfs = require("libs/libkoreader-lfs")
        -- Neutral class-level filter state for every test, restored afterwards
        -- without mutating the table other consumers may be holding.
        original_show_filter = FileChooser.show_filter
        FileChooser.show_filter = {}
    end)

    after_each(function()
        FileChooser.show_filter = original_show_filter
    end)

    local function makeTempDir()
        local dir = os.tmpname()
        os.remove(dir)
        assert.is_truthy(lfs.mkdir(dir))
        return dir
    end

    local function makeFile(path)
        local fh = assert(io.open(path, "w"))
        fh:write("kindle book bytes")
        fh:close()
    end

    -- Minimal instance fields; everything else resolves to the real native
    -- FileChooser class through the prototype chain.
    local function newFileChooser(path)
        return setmetatable({
            name = "filemanager",
            path = path,
            file_filter = function(filename)
                return DocumentRegistry:hasProvider(filename)
            end,
        }, { __index = FileChooser })
    end

    local function newLibrary(books, tmpdir)
        local vlib = VirtualLibrary:new({
            getBooks = function()
                return books
            end,
        })
        vlib:setSettings({ cache_dir = tmpdir .. "/cache" })
        return vlib
    end

    it("lists native rows at the fresh cached EPUB or the Kindle source", function()
        local tmpdir = makeTempDir()
        finally(function()
            ffiUtil.purgeDir(tmpdir)
        end)

        local sources = {
            fresh = tmpdir .. "/fresh.kfx",
            unprepared = tmpdir .. "/unprepared.kfx",
            direct = tmpdir .. "/direct.pdf",
        }
        local cached = tmpdir .. "/cache/fresh.epub"
        assert.is_truthy(lfs.mkdir(tmpdir .. "/cache"))
        makeFile(sources.fresh)
        makeFile(sources.unprepared)
        makeFile(sources.direct)
        makeFile(cached)

        local conversions = 0
        local vlib = newLibrary({
            { id = "fresh", display_name = "Fresh Book", source_path = sources.fresh, open_mode = "convert" },
            { id = "unprepared", display_name = "Unprepared Book", source_path = sources.unprepared, open_mode = "convert" },
            { id = "direct", display_name = "Direct Book", source_path = sources.direct, open_mode = "direct" },
        }, tmpdir)
        vlib:setCacheManager({
            getCachePaths = function(_, book)
                return tmpdir .. "/cache/" .. book.id .. ".epub"
            end,
            isFresh = function(_, book)
                return book.id == "fresh"
            end,
            ensureCachedEpub = function()
                conversions = conversions + 1
            end,
        })

        local fc = newFileChooser(tmpdir)
        fc.show_unsupported = true
        local files, err, unavailable = vlib:getBookEntries(fc, false)

        assert.is_nil(err)
        assert.equals(3, #files)
        assert.equals(0, #unavailable)
        assert.equals(0, conversions)

        assert.equals("Fresh Book", files[1].text)
        assert.equals(cached, files[1].path)
        assert.is_true(files[1].is_file)
        assert.equals("file", files[1].attr.mode)
        assert.is_string(files[1].mandatory)
        assert.equals("fresh", files[1].kindle_book_id)

        assert.equals(sources.unprepared, files[2].path)
        assert.equals("unprepared", files[2].kindle_book_id)
        assert.equals(sources.direct, files[3].path)
        assert.is_nil(files[1].file)
    end)

    it("keeps blocked books with a local source listed as normal rows", function()
        local tmpdir = makeTempDir()
        finally(function()
            ffiUtil.purgeDir(tmpdir)
        end)

        local source = tmpdir .. "/book.azw3"
        makeFile(source)
        local vlib = newLibrary({
            { id = "drm", display_name = "DRM Book", source_path = source, open_mode = "blocked", block_reason = "drm" },
        }, tmpdir)

        local fc = newFileChooser(tmpdir)
        fc.show_unsupported = true
        local files, err, unavailable = vlib:getBookEntries(fc, false)

        assert.is_nil(err)
        assert.equals(1, #files)
        assert.equals(source, files[1].path)
        assert.equals("drm", files[1].kindle_book_id)
        assert.equals(0, #unavailable)
    end)

    it("reports cloud-only and vanished books as unavailable actions", function()
        local tmpdir = makeTempDir()
        finally(function()
            ffiUtil.purgeDir(tmpdir)
        end)

        local present = tmpdir .. "/present.azw3"
        makeFile(present)
        local vlib = newLibrary({
            { id = "drm", display_name = "DRM Book", source_path = present, open_mode = "blocked", block_reason = "drm" },
            { id = "cc:cloud", display_name = "Cloud Book", source_path = nil, open_mode = "blocked", block_reason = "missing_source" },
            { id = "vanished", display_name = "Vanished Book", source_path = tmpdir .. "/gone.kfx", open_mode = "convert" },
        }, tmpdir)

        local fc = newFileChooser(tmpdir)
        fc.show_unsupported = true
        local files, err, unavailable = vlib:getBookEntries(fc, false)

        assert.is_nil(err)
        assert.equals(1, #files)
        assert.equals("drm", files[1].kindle_book_id)

        assert.equals(2, #unavailable)
        local cloud = unavailable[1]
        assert.equals("Cloud Book", cloud.text)
        assert.equals(tmpdir, cloud.path)
        assert.equals("cc:cloud", cloud.kindle_book_id)
        assert.is_true(cloud.kindle_unavailable)
        assert.is_true(cloud.dim)
        assert.is_nil(cloud.is_file)
        assert.is_nil(cloud.file)
        assert.is_nil(cloud.attr)
        assert.equals("vanished", unavailable[2].kindle_book_id)
    end)

    it("keeps Kindle sources listed without a provider but hides unsupported others", function()
        local tmpdir = makeTempDir()
        finally(function()
            ffiUtil.purgeDir(tmpdir)
        end)

        local source = tmpdir .. "/book.kfx"
        local other = tmpdir .. "/direct.kindle_test_unknown"
        makeFile(source)
        makeFile(other)
        local vlib = newLibrary({
            { id = "kfx", display_name = "KFX Book", source_path = source, open_mode = "convert" },
            { id = "unknown", display_name = "Unknown Book", source_path = other, open_mode = "direct" },
        }, tmpdir)

        local fc = newFileChooser(tmpdir)
        fc.show_unsupported = false
        assert.is_falsy(DocumentRegistry:hasProvider("book.kfx"))
        assert.is_falsy(DocumentRegistry:hasProvider("direct.kindle_test_unknown"))
        local files, err, unavailable = vlib:getBookEntries(fc, false)

        assert.is_nil(err)
        assert.equals(1, #files)
        assert.equals(source, files[1].path)
        assert.equals(0, #unavailable)
    end)

    it("hides status-filtered Kindle sources without turning them into unavailable actions", function()
        local tmpdir = makeTempDir()
        finally(function()
            ffiUtil.purgeDir(tmpdir)
        end)

        local source = tmpdir .. "/book.kfx"
        makeFile(source)
        local vlib = newLibrary({
            { id = "kfx", display_name = "KFX Book", source_path = source, open_mode = "convert" },
        }, tmpdir)

        FileChooser.show_filter = { status = { reading = true } }
        local fc = newFileChooser(tmpdir)
        fc.show_unsupported = false
        local files, err, unavailable = vlib:getBookEntries(fc, false)

        assert.is_nil(err)
        assert.equals(0, #files)
        assert.equals(0, #unavailable)
    end)

    it("still applies browser exclusion filters to provider-exempt Kindle sources", function()
        local tmpdir = makeTempDir()
        finally(function()
            ffiUtil.purgeDir(tmpdir)
        end)

        local listed = tmpdir .. "/book.kfx"
        local excluded = tmpdir .. "/scratch.kfx"
        makeFile(listed)
        makeFile(excluded)
        local vlib = newLibrary({
            { id = "keep", display_name = "Keep Book", source_path = listed, open_mode = "convert" },
            { id = "drop", display_name = "Drop Book", source_path = excluded, open_mode = "convert" },
        }, tmpdir)

        local fc = newFileChooser(tmpdir)
        fc.show_unsupported = false
        fc.exclude_files = { "^scratch%.kfx$" }
        local files, err, unavailable = vlib:getBookEntries(fc, false)

        assert.is_nil(err)
        assert.equals(1, #files)
        assert.equals(listed, files[1].path)
        assert.equals(0, #unavailable)
    end)
end)
