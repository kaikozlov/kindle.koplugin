-- Tests for CcDbScanner module

require('busted.runner')()
local helper = require("spec/test_helper")

describe("CcDbScanner", function()
    local CcDbScanner
    local SQ3
    local lfs

    setup(function()
        helper.setup_complete()
        CcDbScanner = require("lua/ccdb_scanner")
        lfs = require("libs/libkoreader-lfs")
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/ccdb_scanner"] = nil
        SQ3 = helper.install_sqlite_mock()
        CcDbScanner = require("lua/ccdb_scanner")
    end)

    describe("initialization", function()
        it("should create a new scanner instance", function()
            local scanner = CcDbScanner:new()
            assert.is_not_nil(scanner)
            assert.equals(CcDbScanner.CC_DB_PATH, "/var/local/cc.db")
        end)
    end)

    describe("isAvailable", function()
        it("should return true when cc.db exists", function()
            lfs._setFileState("/var/local/cc.db", {
                exists = true,
                attributes = { mode = "file" },
            })
            local scanner = CcDbScanner:new()
            assert.is_true(scanner:isAvailable())
            lfs._clearFileStates()
        end)

        it("should return false when cc.db does not exist", function()
            lfs._setFileState("/var/local/cc.db", { exists = false })
            local scanner = CcDbScanner:new()
            assert.is_false(scanner:isAvailable())
            lfs._clearFileStates()
        end)
    end)

    describe("scan", function()
        it("should parse KFX book entries correctly", function()
            SQ3._setMockResults({
                p_uuid = { "abc-123" },
                p_location = { "/mnt/us/documents/Downloads/Items01/Book_B00TEST.kfx" },
                p_titles_0_nominal = { "The Test Book" },
                j_credits = { '[{"name":{"display":"Test Author"},"kind":"Author"}]' },
                p_mimeType = { "application/x-kfx-ebook" },
                p_cdeKey = { "B00TEST" },
                p_cdeType = { "EBOK" },
                p_isDRMProtected = { "1" },
                p_isArchived = { "0" },
                p_percentFinished = { "42" },
                p_thumbnail = { "/mnt/us/documents/Downloads/Items01/Book_B00TEST.kfx.sdr/icon.png" },
                p_diskUsage = { "1048576" },
                p_contentSize = { "900000" },
                p_modificationTime = { "1700000000" },
            }, 1)

            local scanner = CcDbScanner:new()
            local books = scanner:scan()

            assert.is_not_nil(books)
            assert.equals(1, #books)

            local book = books[1]
            assert.equals("cc:abc-123", book.id)
            assert.equals("/mnt/us/documents/Downloads/Items01/Book_B00TEST.kfx", book.source_path)
            assert.equals("The Test Book", book.title)
            assert.same({ "Test Author" }, book.authors)
            assert.equals("kfx", book.format)
            assert.equals("epub", book.logical_ext)
            assert.equals("convert", book.open_mode)
            assert.equals("B00TEST", book.cde_key)
            assert.equals("EBOK", book.cde_type)
            assert.equals(42, book.percent_finished)
            assert.is_nil(book.block_reason)
        end)

        it("should block books without a local file (cloud-only)", function()
            SQ3._setMockResults({
                p_uuid = { "cloud-1" },
                p_location = { "" },
                p_titles_0_nominal = { "Cloud Book" },
                j_credits = { "" },
                p_mimeType = { "application/x-mobipocket-ebook" },
                p_cdeKey = { "B00CLOUD" },
                p_cdeType = { "EBOK" },
                p_isDRMProtected = { nil },
                p_isArchived = { "0" },
                p_percentFinished = { nil },
                p_thumbnail = { "" },
                p_diskUsage = { "0" },
                p_contentSize = { "0" },
                p_modificationTime = { "1700000000" },
            }, 1)

            local scanner = CcDbScanner:new()
            local books = scanner:scan()

            assert.equals(1, #books)
            assert.equals("blocked", books[1].open_mode)
            assert.equals("missing_source", books[1].block_reason)
        end)

        it("should block DRM-protected mobipocket books", function()
            SQ3._setMockResults({
                p_uuid = { "drm-azw" },
                p_location = { "/mnt/us/documents/book.azw" },
                p_titles_0_nominal = { "DRM Book" },
                j_credits = { "" },
                p_mimeType = { "application/x-mobipocket-ebook" },
                p_cdeKey = { "B00DRM" },
                p_cdeType = { "EBOK" },
                p_isDRMProtected = { "1" },
                p_isArchived = { "0" },
                p_percentFinished = { nil },
                p_thumbnail = { "" },
                p_diskUsage = { "500000" },
                p_contentSize = { "400000" },
                p_modificationTime = { "1700000000" },
            }, 1)

            local scanner = CcDbScanner:new()
            local books = scanner:scan()

            assert.equals(1, #books)
            assert.equals("blocked", books[1].open_mode)
            assert.equals("drm", books[1].block_reason)
        end)

        it("should allow DRM-free mobipocket books as direct", function()
            SQ3._setMockResults({
                p_uuid = { "free-azw" },
                p_location = { "/mnt/us/documents/free.mobi" },
                p_titles_0_nominal = { "Free Book" },
                j_credits = { "" },
                p_mimeType = { "application/x-mobipocket-ebook" },
                p_cdeKey = { "B00FREE" },
                p_cdeType = { "PDOC" },
                p_isDRMProtected = { nil },
                p_isArchived = { "0" },
                p_percentFinished = { "0" },
                p_thumbnail = { "" },
                p_diskUsage = { "300000" },
                p_contentSize = { "250000" },
                p_modificationTime = { "1700000000" },
            }, 1)

            local scanner = CcDbScanner:new()
            local books = scanner:scan()

            assert.equals("direct", books[1].open_mode)
            assert.is_nil(books[1].block_reason)
        end)

        it("should return empty table when no rows found", function()
            SQ3._setMockResults(nil, 0)

            local scanner = CcDbScanner:new()
            local books = scanner:scan()

            assert.is_not_nil(books)
            assert.equals(0, #books)
        end)

        it("should handle multiple entries", function()
            SQ3._setMockResults({
                p_uuid = { "id1", "id2", "id3" },
                p_location = { "/path/book1.kfx", "/path/book2.kfx", "/path/book.azw" },
                p_titles_0_nominal = { "Book One", "Book Three", "Book Two" },
                j_credits = { "", "", "" },
                p_mimeType = { "application/x-kfx-ebook", "application/x-kfx-ebook", "application/x-mobipocket-ebook" },
                p_cdeKey = { "B001", "B002", "B003" },
                p_cdeType = { "EBOK", "EBOK", "PDOC" },
                p_isDRMProtected = { "1", "1", nil },
                p_isArchived = { "0", "0", "0" },
                p_percentFinished = { "10", "0", "0" },
                p_thumbnail = { "", "", "" },
                p_diskUsage = { "1000000", "800000", "500000" },
                p_contentSize = { "900000", "700000", "400000" },
                p_modificationTime = { "1700000000", "1700000001", "1700000002" },
            }, 3)

            local scanner = CcDbScanner:new()
            local books = scanner:scan()

            assert.equals(3, #books)
            assert.equals("convert", books[1].open_mode)
            assert.equals("convert", books[2].open_mode)
            assert.equals("direct", books[3].open_mode)
        end)
    end)
end)
