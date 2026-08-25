require("busted.runner")()
local helper = require("spec/test_helper")

local function utf_body(value)
    local encoded = value
    return "\0" .. string.char(math.floor(#encoded / 256), #encoded % 256) .. encoded
end

local function value(tag, payload)
    return string.char(tag) .. payload
end

local function utf(text)
    return value(3, utf_body(text))
end

local function long(number)
    local lo = number % 4294967296
    local hi = (number - lo) / 4294967296
    return value(2, table.concat({
        string.char(math.floor(hi / 16777216) % 256, math.floor(hi / 65536) % 256,
            math.floor(hi / 256) % 256, hi % 256),
        string.char(math.floor(lo / 16777216) % 256, math.floor(lo / 65536) % 256,
            math.floor(lo / 256) % 256, lo % 256),
    }))
end

local function byte(number)
    return value(7, string.char(number))
end

local function boolean(flag)
    return value(0, string.char(flag and 1 or 0))
end

local function object_value(name, children)
    return value(254, utf_body(name) .. table.concat(children) .. "\255")
end

local SIGNATURE = "\0\0\0\0\0\26\177\38"

local function make_store(lpr, fpr)
    lpr = lpr or "AScEAAAAAAAA:3"
    fpr = fpr or "ASUKAAAJAgAA:252650"
    local objects = {
        object_value("updated_lpr", { utf(lpr), long(1000), long(-1), utf(""), utf("") }),
        object_value("erl", { utf(lpr) }),
        object_value("fpr", { utf(fpr), long(900), long(-1), utf(""), utf("") }),
        object_value("sync_lpr", { boolean(false) }),
        object_value("lpr", { byte(2), utf(lpr), long(1100) }),
        object_value("unknown.future.object", { byte(7), utf("preserve-me") }),
    }
    return SIGNATURE .. long(1) ..
        value(1, table.concat({
            string.char(math.floor(#objects / 16777216) % 256,
                math.floor(#objects / 65536) % 256,
                math.floor(#objects / 256) % 256,
                #objects % 256),
        })) .. table.concat(objects)
end

describe("KindleSidecar", function()
    local KindleSidecar

    setup(function()
        helper.setup_complete()
    end)

    before_each(function()
        package.loaded["lua/lib/kindle_sidecar"] = nil
        KindleSidecar = require("lua/lib/kindle_sidecar")
    end)

    it("round-trips unknown values byte-for-byte", function()
        local data = make_store()
        local store, err = KindleSidecar.parse(data)
        assert.is_not_nil(store, err)

        assert.equals(data, KindleSidecar.encode(store))
    end)

    it("reads the last-page position preferring lpr with timestamps", function()
        local store = assert(KindleSidecar.parse(make_store()))
        local updated = KindleSidecar.objects(store, "updated_lpr")[1]
        local position, timestamp = KindleSidecar.position_from_object(updated)
        assert.equals("AScEAAAAAAAA:3", position)
        assert.equals(1000, timestamp)
    end)

    it("updates lpr, updated_lpr, and erl positions", function()
        local store = assert(KindleSidecar.parse(make_store()))
        for _, name in ipairs({ "lpr", "updated_lpr", "erl" }) do
            local obj = KindleSidecar.objects(store, name)[1]
            assert.is_not_nil(obj, name)
            assert.is_true(KindleSidecar.set_object_position(obj, "ATwFAACbAAAA:442741", 2000))
        end
        local reencoded = assert(KindleSidecar.encode(store))
        local reparsed = assert(KindleSidecar.parse(reencoded))
        for _, name in ipairs({ "lpr", "updated_lpr" }) do
            local position, timestamp =
                KindleSidecar.position_from_object(KindleSidecar.objects(reparsed, name)[1])
            assert.equals("ATwFAACbAAAA:442741", position, name)
            assert.equals(2000, timestamp, name)
        end
    end)

    it("rejects a bad signature", function()
        local store, err = KindleSidecar.parse("nonsense")
        assert.is_nil(store)
        assert.is_truthy(err:find("signature"))
    end)

    it("rejects trailing data", function()
        local store, err = KindleSidecar.parse(make_store() .. "X")
        assert.is_nil(store)
        assert.is_truthy(err:find("extra data"))
    end)
end)
