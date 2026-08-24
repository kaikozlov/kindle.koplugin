require("busted.runner")()
local helper = require("spec/test_helper")

describe("CoverBrowserExt", function()
    local CoverBrowserExt

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/coverbrowser_ext"] = nil
        CoverBrowserExt = require("lua/coverbrowser_ext")
    end)

    after_each(function()
        package.preload["bookinfomanager"] = nil
        package.loaded["bookinfomanager"] = nil
        package.preload["covermenu"] = nil
        package.loaded["covermenu"] = nil
        package.preload["listmenu"] = nil
        package.loaded["listmenu"] = nil
        package.preload["mosaicmenu"] = nil
        package.loaded["mosaicmenu"] = nil
    end)

    local function installCoverBrowser(display_mode, settings)
        settings = settings or {}
        local stored = { kindle_library_display_mode = display_mode }
        local BookInfoManager = {}
        function BookInfoManager:getSetting(key)
            if key == "kindle_library_display_mode" then
                return stored.kindle_library_display_mode
            end
            return settings[key]
        end
        package.preload["bookinfomanager"] = function()
            return BookInfoManager
        end
        package.loaded["bookinfomanager"] = BookInfoManager
        package.preload["covermenu"] = function()
            return {
                updateItems = function() end,
                onCloseWidget = function() end,
            }
        end
        package.preload["listmenu"] = function()
            return {
                _recalculateDimen = function() end,
                _updateItemsBuildUI = function() end,
            }
        end
        package.preload["mosaicmenu"] = function()
            return {
                _recalculateDimen = function() end,
                _updateItemsBuildUI = function() end,
            }
        end
        return stored
    end

    it("stays in classic mode without CoverBrowser", function()
        assert.is_nil(CoverBrowserExt.displayMode())
        assert.is_false(CoverBrowserExt.apply({}))
    end)

    it("mirrors the filemanager display mode by default", function()
        installCoverBrowser(nil, {
            filemanager_display_mode = "mosaic_image",
        })

        assert.is_not_nil(CoverBrowserExt.displayMode())
        assert.equals("mosaic_image", CoverBrowserExt.displayMode())

        local menu = {}
        assert.is_true(CoverBrowserExt.apply(menu))
        assert.equals("mosaic", menu.display_mode_type)
        assert.is_true(menu._do_cover_images)
        assert.is_true(menu._do_center_partial_rows)
        assert.is_function(menu.getBookInfo)
        assert.is_function(menu.updateItems)
    end)

    it("applies a dedicated kindle library list mode", function()
        installCoverBrowser("list_image_meta", {
            filemanager_display_mode = "mosaic_image",
        })

        local menu = {}
        assert.is_true(CoverBrowserExt.apply(menu))
        assert.equals("list", menu.display_mode_type)
        assert.is_true(menu._do_cover_images)
        assert.is_false(menu._do_filename_only)
    end)

    it("degrades gracefully when CoverBrowser modules are missing", function()
        installCoverBrowser("list_image_meta")
        package.preload["listmenu"] = nil
        package.loaded["listmenu"] = nil

        assert.is_false(CoverBrowserExt.apply({}))
    end)
end)

describe("VirtualLibrary CoverBrowser entries", function()
    local VirtualLibrary

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/virtual_library"] = nil
        VirtualLibrary = require("lua/virtual_library")
    end)

    it("points fresh converted books at their cached EPUB for covers", function()
        local books = {
            {
                id = "fresh",
                source_path = "/documents/fresh.kfx",
                open_mode = "convert",
                display_name = "Fresh",
            },
            {
                id = "unprepared",
                source_path = "/documents/unprepared.kfx",
                open_mode = "convert",
                display_name = "Unprepared",
            },
            {
                id = "direct",
                source_path = "/documents/direct.pdf",
                open_mode = "direct",
                display_name = "Direct",
            },
        }
        local vlib = VirtualLibrary:new({ getBooks = function() return books end })
        vlib:setSettings({ cache_dir = "/cache" })
        vlib:setCacheManager({
            getCachePaths = function(_, book) return "/cache/" .. book.id .. ".epub" end,
            isFresh = function(_, book) return book.id == "fresh" end,
        })

        local entries = vlib:getBookEntries(false)

        assert.equals("/cache/fresh.epub", entries[1].file)
        assert.equals("/documents/unprepared.kfx", entries[2].file)
        assert.equals("/documents/direct.pdf", entries[3].file)
        assert.equals("/documents/fresh.kfx", entries[1].path)
    end)
end)
