-- Tests for firmware-aligned direct cc.db connection setup.

require("busted.runner")()
local helper = require("spec/test_helper")

describe("KindleCatalogDb", function()
    local CatalogDb

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        helper.before_each()
        package.loaded["lua/lib/kindle_catalog_db"] = nil
        CatalogDb = require("lua/lib/kindle_catalog_db")
    end)

    after_each(function()
        CatalogDb._test_icu_installer = nil
        helper.reset_state()
    end)

    local function fakeConnection(results, callbacks)
        return {
            rowexec = function(_, sql)
                local value = results[sql]
                if type(value) == "table" and value.error then
                    error(value.error)
                end
                return value
            end,
            setscalar = callbacks and function(_, name, callback)
                callbacks[name] = callback
            end or nil,
        }
    end

    it("does no compatibility work when the instantiated schema needs none", function()
        local q = CatalogDb._queries
        local callbacks = {}
        local conn = fakeConnection({
            [q.entry_trigger_count] = 0,
            [q.last_access_index] = nil,
        }, callbacks)

        local context, err = CatalogDb.prepareWriteConnection(conn)

        assert.is_nil(err)
        assert.is_true(context.write_last_access)
        assert.same({}, callbacks)
        context.close()
    end)

    it("detects ICU inherited by EntriesLastAccessIndex from the column schema", function()
        local q = CatalogDb._queries
        local installed = false
        local cleaned = false
        CatalogDb._test_icu_installer = function()
            installed = true
            return true, function()
                cleaned = true
            end
        end
        local conn = fakeConnection({
            [q.entry_trigger_count] = 0,
            [q.last_access_index] = "CREATE INDEX EntriesLastAccessIndex ON Entries (p_lastAccess DESC, p_titles_0_collation)",
            [q.entries_table] = "CREATE TABLE Entries (p_titles_0_collation COLLATE icu, p_lastAccess)",
        })

        local context = assert(CatalogDb.prepareWriteConnection(conn))

        assert.is_true(installed)
        assert.is_true(context.write_last_access)
        context.close()
        assert.is_true(cleaned)
    end)

    it("falls back to progress-only writes if native ICU setup raises", function()
        local q = CatalogDb._queries
        CatalogDb._test_icu_installer = function()
            error("missing firmware symbol")
        end
        local conn = fakeConnection({
            [q.entry_trigger_count] = 0,
            [q.last_access_index] = "CREATE INDEX EntriesLastAccessIndex ON Entries (p_lastAccess DESC, p_titles_0_collation)",
            [q.entries_table] = "CREATE TABLE Entries (p_titles_0_collation COLLATE icu, p_lastAccess)",
        })

        local context = assert(CatalogDb.prepareWriteConnection(conn))

        assert.is_false(context.write_last_access)
    end)

    it("leaves p_lastAccess untouched instead of faking ICU when native registration fails", function()
        local q = CatalogDb._queries
        CatalogDb._test_icu_installer = function()
            return false, "native ICU unavailable"
        end
        local conn = fakeConnection({
            [q.entry_trigger_count] = 0,
            [q.last_access_index] = "CREATE INDEX EntriesLastAccessIndex ON Entries (p_lastAccess DESC, p_titles_0_collation)",
            [q.entries_table] = "CREATE TABLE Entries (p_titles_0_collation COLLATE icu, p_lastAccess)",
        })

        local context = assert(CatalogDb.prepareWriteConnection(conn))

        assert.is_false(context.write_last_access)
    end)

    it("registers real firmware audit functions only when Entries triggers exist", function()
        local q = CatalogDb._queries
        local callbacks = {}
        local conn = fakeConnection({
            [q.entry_trigger_count] = 1,
            [q.last_access_index] = nil,
        }, callbacks)

        assert(CatalogDb.prepareWriteConnection(conn))

        assert.is_function(callbacks.get_entry_external_id)
        assert.is_function(callbacks.get_entry_change_type)
        assert.is_function(callbacks.get_companion_relation_external_id)
        assert.is_function(callbacks.build_merge_changes)
        assert.is_function(callbacks.build_merge_changes_delta)
        assert.equals("B007N6JEII!!EBOK", callbacks.get_entry_external_id("Entry:Item", "uuid", "B007N6JEII", "EBOK", nil, "Title"))
        assert.equals("LIBRARY_ITEM", callbacks.get_entry_change_type("Entry:Item"))
        assert.equals("B007N6JEII!!EBOK!!HAS_COMPANION", callbacks.get_companion_relation_external_id("B007N6JEII", "EBOK"))

        local json = require("json")
        local delta = json.decode(callbacks.build_merge_changes_delta("Entry:Item", nil, nil, 200, 100, 6, 1))
        assert.equals(200, delta.p_lastAccess)
        assert.equals(6, delta.p_readState)
        assert.is_nil(delta.p_conversionStatus)
    end)

    it("fails closed when trigger semantics cannot be installed", function()
        local q = CatalogDb._queries
        local conn = fakeConnection({
            [q.entry_trigger_count] = 1,
            [q.last_access_index] = nil,
        })

        local context, err = CatalogDb.prepareWriteConnection(conn)

        assert.is_nil(context)
        assert.is_string(err)
        assert.is_truthy(err:find("scalar registration", 1, true))
    end)

    it("fails closed when the catalog schema cannot be inspected", function()
        local q = CatalogDb._queries
        local conn = fakeConnection({
            [q.entry_trigger_count] = { error = "schema unavailable" },
        })

        local context, err = CatalogDb.prepareWriteConnection(conn)

        assert.is_nil(context)
        assert.is_truthy(err:find("cannot inspect Kindle catalog triggers", 1, true))
    end)
end)
