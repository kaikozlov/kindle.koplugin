-- Tests for DocSettingsExt module

describe("DocSettingsExt", function()
    local DocSettingsExt

    setup(function()
        require("spec/helper")
        DocSettingsExt = require("lua/docsettings_ext")
    end)

    before_each(function()
        package.loaded["lua/docsettings_ext"] = nil
        DocSettingsExt = require("lua/docsettings_ext")
        resetAllMocks()
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
                real_to_virtual = {},
            }

            mock_docsettings = {
                getSidecarDir = function(self, doc_path, force_location)
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
            it("should return virtual sidecar dir for virtual paths", function()
                -- Make getBook return a book for the virtual path
                mock_virtual_library.getBook = function(self, path)
                    if path and path:match("^KINDLE_VIRTUAL://") then
                        return { id = "test_book_id" }
                    end
                    return nil
                end

                DocSettingsExt:apply(mock_docsettings)

                local result = mock_docsettings:getSidecarDir("KINDLE_VIRTUAL://test_id/Book.epub")

                assert.is_true(result:match("kindle_virtual") ~= nil)
                assert.is_true(result:match("%.sdr$") ~= nil)
            end)

            it("should fall through to original for non-virtual paths", function()
                DocSettingsExt:apply(mock_docsettings)

                local result = mock_docsettings:getSidecarDir("/regular/path.epub")

                assert.equals("/regular/path.epub.sdr", result)
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
end)
