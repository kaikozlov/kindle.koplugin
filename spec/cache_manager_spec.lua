-- Tests for CacheManager module

require("busted.runner")()
local helper = require("spec/test_helper")

describe("CacheManager", function()
    local CacheManager
    local io_mocker

    setup(function()
        helper.setup_complete()
        CacheManager = require("lua/cache_manager")
    end)

    before_each(function()
        package.loaded["lua/cache_manager"] = nil
        CacheManager = require("lua/cache_manager")
        io_mocker = createIOOpenMocker()
        io_mocker.install()
        helper.before_each()
    end)

    after_each(function()
        io_mocker.uninstall()
    end)

    describe("initialization", function()
        it("should create a new instance with helper and virtual_library", function()
            local cm = CacheManager:new({}, {})

            assert.is_not_nil(cm)
            assert.is_table(cm)
        end)
    end)

    describe("setSettings", function()
        it("should store settings", function()
            local cm = CacheManager:new({}, {})
            local settings = { cache_dir = "/test/cache" }

            cm:setSettings(settings)

            assert.equals(settings, cm.settings)
        end)

        it("should default to empty table for nil", function()
            local cm = CacheManager:new({}, {})

            cm:setSettings(nil)

            assert.is_table(cm.settings)
        end)
    end)

    describe("getCacheDir", function()
        it("should return settings cache_dir", function()
            local cm = CacheManager:new({}, {})
            cm:setSettings({ cache_dir = "/custom/cache" })

            assert.equals("/custom/cache", cm:getCacheDir())
        end)

        it("should return default when not set", function()
            local cm = CacheManager:new({}, {})
            cm:setSettings({})

            assert.equals("/tmp/kindle.koplugin.cache", cm:getCacheDir())
        end)
    end)

    describe("getCachePaths", function()
        it("should generate epub and json paths from book id", function()
            local cm = CacheManager:new({}, {})
            local book = { id = "test_book_id" }

            local epub_path, meta_path = cm:getCachePaths(book)

            assert.is_true(epub_path:match("test_book_id%.epub$") ~= nil)
            assert.is_true(meta_path:match("test_book_id%.json$") ~= nil)
        end)

        it("should sanitize special chars in id", function()
            local cm = CacheManager:new({}, {})
            local book = { id = "book/with:special" }

            local epub_path = cm:getCachePaths(book)

            -- The id gets sanitized (non-word chars replaced with _)
            assert.is_true(epub_path:match("book_with_special") ~= nil)
        end)
    end)

    describe("isFresh", function()
        it("should return false when epub is missing", function()
            local cm = CacheManager:new({}, {})
            local book = { id = "b1", source_mtime = 1000, source_size = 42 }

            -- epub doesn't exist (no mock file set)
            local fresh = cm:isFresh(book)

            assert.is_false(fresh)
        end)

        it("should return false when metadata is missing", function()
            local cm = CacheManager:new({}, {})
            cm:setSettings({ cache_dir = "/cache" })
            local book = { id = "b1", source_mtime = 1000, source_size = 42 }

            -- Set epub file but not metadata
            local epub_path = cm:getCachePaths(book)
            io_mocker.setMockFile(epub_path, {
                read = function()
                    return ""
                end,
                close = function() end,
            })

            local fresh = cm:isFresh(book)

            assert.is_false(fresh)
        end)

        it("should return false when converter version changed", function()
            local cm = CacheManager:new({}, {})
            cm:setSettings({ cache_dir = "/cache" })
            local book = { id = "b1", source_mtime = 1000, source_size = 42 }

            local epub_path, meta_path = cm:getCachePaths(book)

            io_mocker.setMockFile(epub_path, {
                read = function()
                    return ""
                end,
                close = function() end,
            })
            -- Version 2 predates exact-position metadata in converted EPUBs.
            io_mocker.setMockFile(meta_path, {
                read = function()
                    return '{"converter_version":"2","source_mtime":1000,"source_size":42}'
                end,
                close = function() end,
            })

            -- Current CONVERTER_VERSION is "5".
            local fresh = cm:isFresh(book)

            assert.is_false(fresh)
        end)

        it("should return false when source mtime changed", function()
            local cm = CacheManager:new({}, {})
            cm:setSettings({ cache_dir = "/cache" })
            local book = { id = "b1", source_mtime = 2000, source_size = 42 }

            local epub_path, meta_path = cm:getCachePaths(book)

            io_mocker.setMockFile(epub_path, {
                read = function()
                    return ""
                end,
                close = function() end,
            })
            io_mocker.setMockFile(meta_path, {
                read = function()
                    return '{"converter_version":"3","source_mtime":1000,"source_size":42}'
                end,
                close = function() end,
            })

            local fresh = cm:isFresh(book)

            assert.is_false(fresh)
        end)

        it("should prefer the real source file signature over stale catalog metadata", function()
            local source_path = "/tmp/kindle-cache-source.kfx"
            local source = assert(_test_real_io_open(source_path, "wb"))
            source:write("actual-source")
            source:close()
            local lfs = require("libs/libkoreader-lfs")
            local attr = assert(lfs.attributes(source_path))

            local cm = CacheManager:new({}, {})
            cm:setSettings({ cache_dir = "/cache" })
            local book = {
                id = "b1",
                source_path = source_path,
                source_mtime = 1,
                source_size = 1,
            }
            local epub_path, meta_path = cm:getCachePaths(book)
            io_mocker.setMockFile(epub_path, {
                read = function()
                    return ""
                end,
                close = function() end,
            })
            io_mocker.setMockFile(epub_path:gsub("%.epub$", ".positions.json"), {
                read = function()
                    return "{}"
                end,
                close = function() end,
            })
            io_mocker.setMockFile(meta_path, {
                read = function()
                    return string.format('{"converter_version":"5","source_mtime":%d,"source_size":%d}', attr.modification, attr.size)
                end,
                close = function() end,
            })

            assert.is_true(cm:isFresh(book))
            os.remove(source_path)
        end)

        it("should return true when cache is valid", function()
            local cm = CacheManager:new({}, {})
            cm:setSettings({ cache_dir = "/cache" })
            local book = { id = "b1", source_mtime = 1000, source_size = 42 }

            local epub_path, meta_path = cm:getCachePaths(book)

            io_mocker.setMockFile(epub_path, {
                read = function()
                    return ""
                end,
                close = function() end,
            })
            io_mocker.setMockFile(epub_path:gsub("%.epub$", ".positions.json"), {
                read = function()
                    return "{}"
                end,
                close = function() end,
            })
            -- The real json.decode is used, so provide valid JSON that decodes to the right table
            io_mocker.setMockFile(meta_path, {
                read = function()
                    return '{"converter_version":"5","source_mtime":1000,"source_size":42}'
                end,
                close = function() end,
            })

            local fresh, ret_epub, ret_meta = cm:isFresh(book)

            assert.is_true(fresh)
            assert.equals(epub_path, ret_epub)
            assert.equals(meta_path, ret_meta)
        end)
    end)

    describe("ensureCachedEpub DRM failures", function()
        local function newDrmManager(extract_result, extract_err, retry_result, retry_err)
            local converts = 0
            local helper_client = {
                convert = function()
                    converts = converts + 1
                    if converts == 1 then
                        return { ok = false, code = "drm", message = "no cached page key" }
                    end
                    return retry_result, retry_err
                end,
                extractBookKey = function()
                    return extract_result, extract_err
                end,
            }
            local cm = CacheManager:new(helper_client, {})
            cm:setSettings({ cache_dir = "/cache" })
            cm.isFresh = function()
                return false, "/cache/book.epub", "/cache/book.json"
            end
            cm.ensureCacheDir = function()
                return true
            end
            return cm
        end

        it("preserves an unavailable-extractor error for the UI", function()
            local cm = newDrmManager({
                ok = false,
                code = "drm_extractor_unavailable",
                message = "no extractor",
            })

            local path, err = cm:ensureCachedEpub({
                id = "book",
                source_path = "/documents/book.kfx",
                open_mode = "convert",
            })

            assert.is_nil(path)
            assert.equals("drm_extractor_unavailable", err)
        end)

        it("uses a stable error when the extraction helper itself fails", function()
            local cm = newDrmManager(nil, "invalid helper JSON")

            local path, err = cm:ensureCachedEpub({
                id = "book",
                source_path = "/documents/book.kfx",
                open_mode = "convert",
            })

            assert.is_nil(path)
            assert.equals("drm_key_extraction_failed", err)
        end)

        it("distinguishes decryption failure after a key was extracted", function()
            local cm = newDrmManager({ ok = true, book_id = "book" }, nil, { ok = false, code = "drm", message = "still encrypted" })

            local path, err = cm:ensureCachedEpub({
                id = "book",
                source_path = "/documents/book.kfx",
                open_mode = "convert",
            })

            assert.is_nil(path)
            assert.equals("drm_after_key_extraction", err)
        end)
    end)

    describe("getDrmKeysPath", function()
        it("should return path under cache dir", function()
            local cm = CacheManager:new({}, {})
            cm:setSettings({ cache_dir = "/test/cache" })

            assert.equals("/test/cache/drm_keys.json", cm:getDrmKeysPath())
        end)
    end)

    describe("clearBookCache", function()
        it("should remove epub and metadata files", function()
            local cm = CacheManager:new({}, {})
            cm:setSettings({ cache_dir = "/cache" })
            local book = { id = "b1" }

            -- os.remove will just return nil in test env, but we verify it doesn't crash
            local ok = cm:clearBookCache(book)

            assert.is_true(ok)
        end)
    end)

    describe("CONVERTER_VERSION", function()
        it("should be a string", function()
            assert.is_string(CacheManager.CONVERTER_VERSION)
        end)

        it("should be non-empty", function()
            assert.is_true(#CacheManager.CONVERTER_VERSION > 0)
        end)
    end)
end)
