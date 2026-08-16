require("busted.runner")()
local helper = require("spec/test_helper")

describe("legacy Kindle sidecar migration", function()
    local DataStorage
    local DocSettings
    local util
    local tmp_root

    setup(function()
        helper.setup_complete()
        DataStorage = require("datastorage")
        DocSettings = require("docsettings")
        util = require("util")
    end)

    before_each(function()
        helper.before_each()
        tmp_root = "/tmp/kindle-sidecar-native-" .. tostring(os.time()) .. "-" .. tostring(math.random(1000000))
        util.makePath(tmp_root)
    end)

    after_each(function()
        os.execute("rm -rf " .. tmp_root)
        os.execute("rm -rf " .. DataStorage:getDocSettingsDir() .. "/kindle_virtual/test_native_sidecar.sdr")
    end)

    it("copies legacy metadata once, then leaves doc/dir migration to KOReader", function()
        local real_path = tmp_root .. "/book.epub"
        assert.is_true(util.writeToFile("epub", real_path))

        local book = {
            id = "test_native_sidecar",
            logical_ext = "epub",
            source_path = "/mnt/us/documents/book.kfx",
            open_mode = "convert",
        }
        local legacy_dir = DataStorage:getDocSettingsDir() .. "/kindle_virtual/test_native_sidecar.sdr"
        assert.is_true(util.makePath(legacy_dir))
        local legacy_file = legacy_dir .. "/metadata.epub.lua"
        assert.is_true(util.writeToFile(
            "return { percent_finished = 0.42, summary = { status = 'reading' } }\n",
            legacy_file
        ))

        G_reader_settings:saveSetting("document_metadata_folder", "doc")
        local Migration = require("lua/legacy_sidecar_migration")
        local migration = Migration:new({})
        assert.is_true(migration:migrate(book, real_path))
        assert.is_false(migration:migrate(book, real_path))

        local settings = DocSettings:open(real_path)
        assert.equals(0.42, settings:readSetting("percent_finished"))
        local sidecar, location = DocSettings:findSidecarFile(real_path)
        assert.is_truthy(sidecar)
        assert.equals("doc", location)

        -- Changing KOReader's global metadata preference must keep working with
        -- no Kindle-specific getSidecarDir override in the way.
        G_reader_settings:saveSetting("document_metadata_folder", "dir")
        DocSettings.updateLocation(real_path, real_path, false)
        local moved_sidecar, moved_location = DocSettings:findSidecarFile(real_path)
        assert.is_truthy(moved_sidecar)
        assert.equals("dir", moved_location)
        assert.equals(0.42, DocSettings:open(real_path):readSetting("percent_finished"))
    end)

    it("makes a first hash-location migration visible immediately", function()
        local real_path = tmp_root .. "/hash-book.epub"
        assert.is_true(util.writeToFile("epub", real_path))
        local book = {
            id = "test_native_sidecar",
            logical_ext = "epub",
            open_mode = "convert",
        }
        local legacy_dir = DataStorage:getDocSettingsDir() .. "/kindle_virtual/test_native_sidecar.sdr"
        assert.is_true(util.makePath(legacy_dir))
        assert.is_true(util.writeToFile(
            "return { percent_finished = 0.37 }\n",
            legacy_dir .. "/metadata.epub.lua"
        ))

        G_reader_settings:saveSetting("document_metadata_folder", "hash")
        -- Reproduce a process that checked hash support before its first hash
        -- sidecar directory existed.
        DocSettings.setIsHashLocationEnabled(false)
        local Migration = require("lua/legacy_sidecar_migration")
        assert.is_true(Migration:new({}):migrate(book, real_path))

        local reopened = DocSettings:open(real_path)
        assert.equals(0.37, reopened:readSetting("percent_finished"))
        local _, location = DocSettings:findSidecarFile(real_path)
        assert.equals("hash", location)
        reopened:purge()
        DocSettings.setIsHashLocationEnabled(nil)
    end)

    it("never overwrites a newer native KOReader sidecar", function()
        local real_path = tmp_root .. "/book.epub"
        assert.is_true(util.writeToFile("epub", real_path))
        G_reader_settings:saveSetting("document_metadata_folder", "doc")

        local native = DocSettings:open(real_path)
        native:saveSetting("percent_finished", 0.9)
        native:flush()

        local legacy_dir = DataStorage:getDocSettingsDir() .. "/kindle_virtual/test_native_sidecar.sdr"
        assert.is_true(util.makePath(legacy_dir))
        assert.is_true(util.writeToFile(
            "return { percent_finished = 0.1 }\n",
            legacy_dir .. "/metadata.epub.lua"
        ))

        local Migration = require("lua/legacy_sidecar_migration")
        local migration = Migration:new({})
        assert.is_false(migration:migrate({ id = "test_native_sidecar", logical_ext = "epub" }, real_path))
        assert.equals(0.9, DocSettings:open(real_path):readSetting("percent_finished"))
    end)
end)
