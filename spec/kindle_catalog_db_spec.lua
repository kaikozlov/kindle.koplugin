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

    it("disables JIT on ljsqlite3's shared step function before installing callbacks", function()
        local jit = require("jit")
        local original_jit_off = jit.off
        local original_jit_flush = jit.flush
        local guarded_step
        local flush_count = 0
        local prepare_count = 0
        local statement_closed = false
        local step = function() end
        local conn = {
            prepare = function(_, sql)
                prepare_count = prepare_count + 1
                assert.equals("SELECT 1", sql)
                return {
                    _step = step,
                    close = function()
                        statement_closed = true
                    end,
                }
            end,
        }

        jit.off = function(func)
            guarded_step = func
        end
        jit.flush = function()
            flush_count = flush_count + 1
        end
        local ok, result, err = pcall(CatalogDb._disable_sqlite_step_jit, conn)
        local ok_second, result_second, err_second = pcall(CatalogDb._disable_sqlite_step_jit, conn)
        jit.off = original_jit_off
        jit.flush = original_jit_flush

        assert.is_true(ok, result)
        assert.is_true(result, err)
        assert.is_true(ok_second, result_second)
        assert.is_true(result_second, err_second)
        assert.equals(step, guarded_step)
        assert.equals(1, flush_count)
        assert.equals(1, prepare_count)
        assert.is_true(statement_closed)
    end)

    it("mirrors the firmware locale-prefix mapping table", function()
        assert.equals("Lroot", CatalogDb._matching_locale_mapping("en_US_POSIX"))
        assert.equals("Lroot", CatalogDb._matching_locale_mapping("en_US_POSIX.UTF-8"))
        assert.equals("Lzh:Hani", CatalogDb._matching_locale_mapping("zh_CN.utf8"))
        assert.equals("Lja_S4_HO:Hira,Kana,Hani", CatalogDb._matching_locale_mapping("ja_JP.utf8"))
        assert.equals("Lru:y.Cyrl", CatalogDb._matching_locale_mapping("ru_RU.utf8"))
        assert.is_nil(CatalogDb._matching_locale_mapping("en_US.utf8"))
    end)

    it("pairs ICU i18n/core libraries by major instead of trusting the highest filename", function()
        local candidates = CatalogDb._build_icu_candidates({
            "libicui18n.so.72",
            "libicuuc.so.71",
            "libicui18n.so.65.1",
            "libicui18n.so.65",
            "libicuuc.so.65.1",
            "libicuuc.so.65",
            "libicui18n.so.60.2",
            "libicuuc.so.60.3",
        })

        assert.equals(2, #candidates)
        assert.same({
            major = 65,
            i18n = "/usr/lib/libicui18n.so.65",
            uc = "/usr/lib/libicuuc.so.65",
        }, candidates[1])
        assert.same({
            major = 60,
            i18n = "/usr/lib/libicui18n.so.60.2",
            uc = "/usr/lib/libicuuc.so.60.3",
        }, candidates[2])
    end)

    it("matches firmware behavior when Japanese explicit reorder codes are rejected", function()
        local script_codes = { Hira = 20, Kana = 22, Hani = 17 }
        local applied
        local icuuc = {
            uscript_getCode_65 = function(name, output, _, status)
                output[0] = assert(script_codes[name])
                status[0] = 0
                return 1
            end,
        }
        local icui18n = {
            ucol_setReorderCodes_65 = function(_, codes, count, status)
                applied = {}
                for i = 0, count - 1 do
                    table.insert(applied, tonumber(codes[i]))
                end
                status[0] = 1 -- U_ILLEGAL_ARGUMENT_ERROR on ICU 65 for this vector.
            end,
        }

        local ok, err = CatalogDb._apply_short_string_reorder({}, "Hira,Kana,Hani", icui18n, icuuc, 65)

        assert.is_true(ok, err)
        assert.same({ 20, 22, 17, 0x1004 }, applied)
    end)

    it("installs ICU directly without scanning catalog indexes", function()
        local installed = false
        local cleaned = false
        CatalogDb._test_icu_installer = function()
            installed = true
            return true, function()
                cleaned = true
            end
        end
        local callbacks = {}
        local conn = fakeConnection({}, callbacks)
        conn.rowexec = function()
            error("unexpected catalog scan")
        end

        local context, err = CatalogDb.prepareWriteConnection(conn)

        assert.is_nil(err)
        assert.is_true(installed)
        assert.is_true(context.write_last_access)
        assert.is_function(callbacks.get_entry_external_id)
        assert.is_function(callbacks.get_entry_change_type)
        assert.is_function(callbacks.get_companion_relation_external_id)
        assert.is_function(callbacks.build_merge_changes)
        assert.is_function(callbacks.build_merge_changes_delta)
        context.close()
        assert.is_true(cleaned)
    end)

    it("falls back to progress-only writes if native ICU setup raises", function()
        CatalogDb._test_icu_installer = function()
            error("missing firmware symbol")
        end
        local conn = fakeConnection({})

        local context = assert(CatalogDb.prepareWriteConnection(conn))

        assert.is_false(context.write_last_access)
    end)

    it("leaves p_lastAccess untouched instead of faking ICU when native registration fails", function()
        CatalogDb._test_icu_installer = function()
            return false, "native ICU unavailable"
        end
        local conn = fakeConnection({})

        local context = assert(CatalogDb.prepareWriteConnection(conn))

        assert.is_false(context.write_last_access)
    end)

    it("registers the firmware trigger functions without a schema scan", function()
        CatalogDb._test_icu_installer = function()
            return true, function() end
        end
        local callbacks = {}
        local conn = fakeConnection({}, callbacks)

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
end)
