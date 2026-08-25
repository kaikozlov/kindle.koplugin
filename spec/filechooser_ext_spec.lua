require("busted.runner")()
local helper = require("spec/test_helper")

describe("FileChooserExt native library entry", function()
    local FileChooserExt

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/filechooser_ext"] = nil
        FileChooserExt = require("lua/filechooser_ext")
    end)

    local function makeFixture()
        local show_calls = 0
        local shown_ui
        local vlib = {
            isActive = function()
                return true
            end,
            createVirtualFolderEntry = function(_, parent)
                return {
                    text = "Kindle Library/",
                    path = parent,
                    attr = { mode = "directory" },
                    is_kindle_library_folder = true,
                }
            end,
        }
        local library = {
            setUI = function(_, ui)
                shown_ui = ui
            end,
            show = function(_, ui)
                show_calls = show_calls + 1
                shown_ui = ui
                return true
            end,
        }
        local delegated_select = 0
        local delegated_hold = 0
        local fc = {
            genItemTable = function(_, _, _, path)
                return { { text = "normal", path = path .. "/normal" } }
            end,
            onMenuSelect = function()
                delegated_select = delegated_select + 1
                return false
            end,
            onMenuHold = function()
                delegated_hold = delegated_hold + 1
                return false
            end,
        }
        FileChooserExt:init(vlib, library)
        FileChooserExt:apply(fc)
        return fc, function()
            return show_calls, shown_ui, delegated_select, delegated_hold
        end
    end

    after_each(function()
        -- Every fixture is a private class table, but clear module state so a
        -- failure cannot leak an applied hook into the following spec.
        FileChooserExt.applied = false
        FileChooserExt.original_methods = {}
    end)

    it("adds the synthetic entry only to the real FileManager at HOME", function()
        G_reader_settings:saveSetting("home_dir", "/mnt/us")
        local fc = makeFixture()

        local fm = setmetatable({ name = "filemanager", path = "/mnt/us" }, { __index = fc })
        local items = fm:genItemTable({}, {}, "/mnt/us")
        assert.equals(2, #items)
        assert.is_true(items[1].is_kindle_library_folder)
        assert.equals("/mnt/us", items[1].path)

        local pathchooser = setmetatable({ path = "/mnt/us" }, { __index = fc })
        local chooser_items = pathchooser:genItemTable({}, {}, "/mnt/us")
        assert.equals(1, #chooser_items)

        local elsewhere = setmetatable({ name = "filemanager", path = "/tmp" }, { __index = fc })
        assert.equals(1, #elsewhere:genItemTable({}, {}, "/tmp"))
    end)

    it("opens a BookList without changing the FileChooser filesystem path", function()
        G_reader_settings:saveSetting("home_dir", "/mnt/us")
        local fc, state = makeFixture()
        local ui = { marker = "fm" }
        local fm = setmetatable({ name = "filemanager", path = "/mnt/us", ui = ui }, { __index = fc })
        local item = fm:genItemTable({}, {}, "/mnt/us")[1]

        assert.is_true(fm:onMenuSelect(item))
        local calls, shown_ui = state()
        assert.equals(1, calls)
        assert.equals(ui, shown_ui)
        assert.equals("/mnt/us", fm.path)
    end)

    it("delegates ordinary selections and holds unchanged", function()
        local fc, state = makeFixture()
        local fm = setmetatable({ name = "filemanager", path = "/mnt/us" }, { __index = fc })
        assert.is_false(fm:onMenuSelect({ path = "/mnt/us/book.epub" }))
        assert.is_false(fm:onMenuHold({ path = "/mnt/us/book.epub" }))
        local _, _, select_calls, hold_calls = state()
        assert.equals(1, select_calls)
        assert.equals(1, hold_calls)
    end)

    it("restores KOReader methods on live plugin stop", function()
        local original_gen = function()
            return {}
        end
        local original_select = function()
            return false
        end
        local original_hold = function()
            return false
        end
        local fc = {
            genItemTable = original_gen,
            onMenuSelect = original_select,
            onMenuHold = original_hold,
        }
        FileChooserExt:init({
            isActive = function()
                return true
            end,
        }, { show = function() end })
        FileChooserExt:apply(fc)
        assert.not_equals(original_gen, fc.genItemTable)

        FileChooserExt:unapply(fc)
        assert.equals(original_gen, fc.genItemTable)
        assert.equals(original_select, fc.onMenuSelect)
        assert.equals(original_hold, fc.onMenuHold)
    end)
end)
