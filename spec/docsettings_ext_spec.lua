-- Tests for DocSettingsExt module

require('busted.runner')()
local helper = require("spec/test_helper")

describe("DocSettingsExt", function()
    local DocSettingsExt
    local util

    setup(function()
        helper.setup_complete()
        DocSettingsExt = require("lua/docsettings_ext")
        util = require("util")
    end)

    before_each(function()
        package.loaded["lua/docsettings_ext"] = nil
        DocSettingsExt = require("lua/docsettings_ext")
        helper.before_each()
    end)

    describe("apply", function()
        local mock_virtual_library
        local mock_docsettings

        before_each(function()
            mock_virtual_library = {
                isVirtualPath = function(self, path)
                    return type(path) == "string" and path:match("^KINDLE_VIRTUAL://") ~= nil
                end,
                getBook = function(self, path) return nil end,
                getCanonicalPath = function(self, path) return path end,
                getVirtualPath = function(self, path) return nil end,
                cache_manager = {
                    getCachePaths = function(self, book)
                        return "/cache/" .. book.id .. ".epub", "/cache/" .. book.id .. ".json"
                    end,
                },
                real_to_virtual = {},
            }

            mock_docsettings = {
                getSidecarDir = function(self, doc_path, force_location)
                    if force_location == "dir" then
                        return "/docsettings" .. doc_path .. ".sdr"
                    elseif force_location == "hash" then
                        return "/hashdocsettings/ab/hash.sdr"
                    end
                    return doc_path .. ".sdr"
                end,
                getSidecarFilename = function(doc_path)
                    return "metadata." .. doc_path:match("([^/]+)$") .. ".lua"
                end,
                getHistoryPath = function(self, doc_path)
                    return "/history/" .. doc_path:match("([^/]+)$") .. ".lua"
                end,
            }

            DocSettingsExt:init(mock_virtual_library)
        end)

        after_each(function()
            DocSettingsExt:unapply(mock_docsettings)
        end)

        describe("getSidecarDir", function()
            it("should return the cached EPUB sidecar dir for converted virtual paths", function()
                -- Make getBook return a book for the virtual path
                mock_virtual_library.getBook = function(self, path)
                    if path and path:match("^KINDLE_VIRTUAL://") then
                        return { id = "test_book_id", open_mode = "convert" }
                    end
                    return nil
                end

                DocSettingsExt:apply(mock_docsettings)

                local result = mock_docsettings:getSidecarDir("KINDLE_VIRTUAL://test_id/Book.epub")

                assert.equals("/cache/test_book_id.epub.sdr", result)
            end)

            it("should pin alternate location probes to the canonical sidecar", function()
                mock_virtual_library.getBook = function(self, path)
                    if path and path:match("^KINDLE_VIRTUAL://") then
                        return { id = "test_book_id", open_mode = "convert" }
                    end
                    return nil
                end

                DocSettingsExt:apply(mock_docsettings)

                assert.equals(
                    "/cache/test_book_id.epub.sdr",
                    mock_docsettings:getSidecarDir(
                        "KINDLE_VIRTUAL://test_id/Book.epub",
                        "dir"
                    )
                )
                assert.equals(
                    "/cache/test_book_id.epub.sdr",
                    mock_docsettings:getSidecarDir(
                        "KINDLE_VIRTUAL://test_id/Book.epub",
                        "hash"
                    )
                )
            end)

            it("should return the source sidecar dir for direct virtual books", function()
                mock_virtual_library.getBook = function(self, path)
                    if path and path:match("^KINDLE_VIRTUAL://") then
                        return {
                            id = "direct_book",
                            open_mode = "direct",
                            source_path = "/documents/direct.epub",
                        }
                    end
                    return nil
                end

                DocSettingsExt:apply(mock_docsettings)

                local result = mock_docsettings:getSidecarDir("KINDLE_VIRTUAL://direct_book/Book.epub")

                assert.equals("/documents/direct.epub.sdr", result)
            end)

            it("should fall through to original for non-virtual paths", function()
                DocSettingsExt:apply(mock_docsettings)

                local result = mock_docsettings:getSidecarDir("/regular/path.epub")

                assert.equals("/regular/path.epub.sdr", result)
            end)
        end)

        describe("getSidecarFilename", function()
            it("should use the prepared document filename", function()
                mock_virtual_library.getBook = function(self, path)
                    if path and path:match("^KINDLE_VIRTUAL://") then
                        return { id = "test_book_id", open_mode = "convert" }
                    end
                    return nil
                end

                DocSettingsExt:apply(mock_docsettings)

                local result = mock_docsettings.getSidecarFilename(
                    "KINDLE_VIRTUAL://test_book_id/Book.epub"
                )

                assert.equals("metadata.test_book_id.epub.lua", result)
            end)
        end)

        describe("getHistoryPath", function()
            it("should build history path for virtual paths", function()
                mock_virtual_library.getBook = function(self, path)
                    if path and path:match("^KINDLE_VIRTUAL://") then
                        return { id = "test_id" }
                    end
                    return nil
                end

                DocSettingsExt:apply(mock_docsettings)

                local result = mock_docsettings:getHistoryPath("KINDLE_VIRTUAL://test_id/Book.epub")

                assert.is_string(result)
                assert.is_true(#result > 0)
            end)

            it("should fall through to original for non-virtual paths", function()
                DocSettingsExt:apply(mock_docsettings)

                local result = mock_docsettings:getHistoryPath("/regular/path.epub")

                -- The original mock returns /history/<basename>.lua
                assert.is_true(result:match("path%.epub") ~= nil)
            end)
        end)
    end)

    describe("legacy migration", function()
        local original_make_path
        local tmp_dir

        before_each(function()
            original_make_path = util.makePath
            tmp_dir = os.tmpname()
            os.remove(tmp_dir)
            assert.is_true(os.execute("mkdir -p " .. tmp_dir) == 0)
            DocSettingsExt:init({})
            DocSettingsExt.original_methods.getSidecarFilename = function()
                return "metadata.epub.lua"
            end
        end)

        after_each(function()
            util.makePath = original_make_path
            os.execute("rm -rf " .. tmp_dir)
        end)

        it("should not reimport legacy metadata when the canonical file is readable", function()
            local preferred_dir = tmp_dir .. "/cache/test.sdr"
            local preferred = preferred_dir .. "/metadata.epub.lua"
            local make_path_called = false
            assert.is_true(os.execute("mkdir -p " .. preferred_dir) == 0)
            local canonical = assert(io.open(preferred, "wb"))
            canonical:write("canonical")
            canonical:close()
            util.makePath = function()
                make_path_called = true
                return true
            end

            DocSettingsExt:migrateLegacySidecar(
                { id = "test", open_mode = "convert" },
                "KINDLE_VIRTUAL://test/Book.epub",
                tmp_dir .. "/cache/test.epub",
                preferred_dir
            )

            assert.is_false(make_path_called)
            local current = assert(io.open(preferred, "rb"))
            assert.equals("canonical", current:read("*a"))
            current:close()
        end)

        it("should not reimport legacy metadata during canonical rotation", function()
            local preferred_dir = tmp_dir .. "/cache/test.sdr"
            local preferred_old = preferred_dir .. "/metadata.epub.lua.old"
            local make_path_called = false
            assert.is_true(os.execute("mkdir -p " .. preferred_dir) == 0)
            local rotated = assert(io.open(preferred_old, "wb"))
            rotated:write("canonical")
            rotated:close()
            util.makePath = function()
                make_path_called = true
                return true
            end

            DocSettingsExt:migrateLegacySidecar(
                { id = "test", open_mode = "convert" },
                "KINDLE_VIRTUAL://test/Book.epub",
                tmp_dir .. "/cache/test.epub",
                preferred_dir
            )

            assert.is_false(make_path_called)
        end)
    end)
end)
