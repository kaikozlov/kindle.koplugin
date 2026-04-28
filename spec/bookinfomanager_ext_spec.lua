-- Tests for Kindle thumbnail handling in BookInfoManagerExt.

require('busted.runner')()
local helper = require("spec/test_helper")

describe("BookInfoManagerExt", function()
    local BookInfoManagerExt
    local lfs

    setup(function()
        helper.setup_complete()
        lfs = require("libs/libkoreader-lfs")
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/bookinfomanager_ext"] = nil
        BookInfoManagerExt = require("lua/bookinfomanager_ext")
    end)

    describe("Kindle thumbnails", function()
        it("should load an existing Kindle thumbnail", function()
            local thumbnail_path = "/mnt/us/system/thumbnails/thumbnail.jpg"
            local rendered_path
            local cover = {
                getWidth = function() return 120 end,
                getHeight = function() return 180 end,
            }

            lfs._setFileState(thumbnail_path, {
                exists = true,
                attributes = { mode = "file" },
            })
            package.loaded["ui/renderimage"] = {
                renderImageFile = function(_, path)
                    rendered_path = path
                    return cover
                end,
            }

            local bookinfo = {}
            BookInfoManagerExt:tryLoadKindleThumbnail(bookinfo, thumbnail_path)

            assert.equals(thumbnail_path, rendered_path)
            assert.equals("Y", bookinfo.has_cover)
            assert.equals("Y", bookinfo.cover_fetched)
            assert.equals(cover, bookinfo.cover_bb)
            assert.equals(120, bookinfo.cover_w)
            assert.equals(180, bookinfo.cover_h)
            assert.equals("120x180", bookinfo.cover_sizetag)
        end)

        it("should ignore a missing Kindle thumbnail", function()
            local thumbnail_path = "/mnt/us/system/thumbnails/missing.jpg"
            lfs._setFileState(thumbnail_path, { exists = false })

            local bookinfo = {}
            BookInfoManagerExt:tryLoadKindleThumbnail(bookinfo, thumbnail_path)

            assert.is_nil(bookinfo.has_cover)
            assert.is_nil(bookinfo.cover_bb)
        end)

        it("should prefer a Kindle thumbnail over sidecar extraction", function()
            local thumbnail_calls = 0
            local sidecar_calls = 0
            local virtual_library = {
                resolveBookPath = function() return nil end,
            }
            local cache_manager = { helper_client = {} }
            BookInfoManagerExt:init(virtual_library, cache_manager)
            BookInfoManagerExt.tryLoadKindleThumbnail = function(_, bookinfo, path)
                thumbnail_calls = thumbnail_calls + 1
                assert.equals("/mnt/us/system/thumbnails/book.jpg", path)
                bookinfo.has_cover = "Y"
            end
            BookInfoManagerExt.tryExtractCoverFromSidecar = function()
                sidecar_calls = sidecar_calls + 1
            end

            local bookinfo = BookInfoManagerExt:buildBookInfoFromScanAndEpub(
                "KINDLE_VIRTUAL://book/Book.epub",
                {
                    id = "book",
                    title = "Book",
                    source_path = "/mnt/us/documents/book.kfx",
                    thumbnail_path = "/mnt/us/system/thumbnails/book.jpg",
                },
                true
            )

            assert.is_not_nil(bookinfo)
            assert.equals(1, thumbnail_calls)
            assert.equals(0, sidecar_calls)
            assert.equals("Y", bookinfo.has_cover)
        end)
    end)
end)
