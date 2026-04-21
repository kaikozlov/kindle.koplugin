-- Tests for FilesystemExt module

describe("FilesystemExt", function()
    local FilesystemExt
    local VirtualLibrary

    setup(function()
        require("spec/helper")
        FilesystemExt = require("lua/filesystem_ext")
    end)

    before_each(function()
        package.loaded["lua/filesystem_ext"] = nil
        FilesystemExt = require("lua/filesystem_ext")
        resetAllMocks()
    end)

    describe("initialization", function()
        it("should initialize with virtual_library", function()
            local ext = FilesystemExt

            ext:init({ isVirtualPath = function() return false end })

            assert.is_not_nil(ext.virtual_library)
        end)
    end)

    describe("apply", function()
        local mock_virtual_library
        local original_lfs_attributes

        before_each(function()
            original_lfs_attributes = require("libs/libkoreader-lfs").attributes

            mock_virtual_library = {
                isActive = function() return true end,
                isVirtualPath = function(self, path)
                    return type(path) == "string" and path:match("^KINDLE_VIRTUAL://") ~= nil
                end,
                getRealPath = function(self, path)
                    if path == "KINDLE_VIRTUAL://b1/Book.epub" then
                        return "/cache/b1.epub"
                    end
                    return nil
                end,
            }

            FilesystemExt:init(mock_virtual_library)
        end)

        after_each(function()
            FilesystemExt:unapply()
        end)

        it("should not patch when virtual library is not active", function()
            mock_virtual_library.isActive = function() return false end

            FilesystemExt:apply()

            -- lfs.attributes should be unchanged
            local lfs = require("libs/libkoreader-lfs")
            assert.equals(original_lfs_attributes, lfs.attributes)
        end)

        it("should patch lfs.attributes for virtual root as directory", function()
            FilesystemExt:apply()

            local lfs = require("libs/libkoreader-lfs")

            local mode = lfs.attributes("KINDLE_VIRTUAL://", "mode")
            assert.equals("directory", mode)
        end)

        it("should return directory table for virtual root", function()
            FilesystemExt:apply()

            local lfs = require("libs/libkoreader-lfs")

            local attrs = lfs.attributes("KINDLE_VIRTUAL://")
            assert.is_table(attrs)
            assert.equals("directory", attrs.mode)
        end)

        it("should redirect virtual book path to real path", function()
            FilesystemExt:apply()

            local lfs = require("libs/libkoreader-lfs")
            lfs._setFileState("/cache/b1.epub", {
                exists = true,
                attributes = { mode = "file", size = 999 },
            })

            local size = lfs.attributes("KINDLE_VIRTUAL://b1/Book.epub", "size")
            assert.equals(999, size)
        end)

        it("should return nil for unmapped virtual path", function()
            FilesystemExt:apply()

            local lfs = require("libs/libkoreader-lfs")

            local result = lfs.attributes("KINDLE_VIRTUAL://nonexistent/file.epub", "mode")
            assert.is_nil(result)
        end)

        it("should pass through non-virtual paths unchanged", function()
            FilesystemExt:apply()

            local lfs = require("libs/libkoreader-lfs")
            lfs._setFileState("/regular/file.epub", {
                exists = true,
                attributes = { mode = "file", size = 42 },
            })

            local size = lfs.attributes("/regular/file.epub", "size")
            assert.equals(42, size)
        end)
    end)

    describe("unapply", function()
        it("should restore original lfs.attributes", function()
            local lfs = require("libs/libkoreader-lfs")
            local original = lfs.attributes

            mock_virtual_library = {
                isActive = function() return true end,
                isVirtualPath = function() return false end,
                getRealPath = function() return nil end,
            }

            FilesystemExt:init(mock_virtual_library)
            FilesystemExt:apply()

            assert.is_not.equals(original, lfs.attributes)

            FilesystemExt:unapply()

            assert.equals(original, lfs.attributes)
        end)
    end)
end)
