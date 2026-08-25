require("busted.runner")()
local helper = require("spec/test_helper")
local PositionMap = require("lua/lib/position_map")

-- Fixture mirrors python/tests/test_position_map.py: one spine document with
-- a div anchor wrapping an inner p anchor (em/text/tail) plus an unanchored
-- p and a trailing top-level p anchor.
local MAP = {
    version = 1,
    max_pid = 27200 + 16,
    fragments = {
        {
            path = "OEBPS/one.xhtml",
            elements = {
                ["div"] = { a = 1, s = 0, l = {} },
                ["div/p"] = { a = 2, s = 0, l = { 6, 10 } },
                ["div/p/em"] = { a = 2, s = 6, l = { 5 } },
                ["div/p[2]"] = { a = 1, s = 21, l = { 11 } },
                ["p"] = { a = 3, s = 0, l = { 16 } },
            },
            anchors = {
                { p = "div", eid = 1138, pid = 27100, t = 32,
                  nodes = { { p = "div/p[2]", n = 1, c = 21, v = 11 } } },
                { p = "div/p", eid = 1139, pid = 27115, t = 21,
                  nodes = {
                      { p = "div/p", n = 1, c = 0, v = 6 },
                      { p = "div/p/em", n = 1, c = 6, v = 5 },
                      { p = "div/p", n = 2, c = 11, v = 10 },
                  } },
                { p = "p", eid = 1140, pid = 27200, t = 16,
                  nodes = { { p = "p", n = 1, c = 0, v = 16 } } },
            },
        },
    },
}

describe("PositionMap", function()
    setup(function()
        helper.setup_complete()
    end)

    it("translates an XPointer into native coordinates", function()
        local result, err = PositionMap.translate_xpointer(
            MAP, "/body/DocFragment/body/div/p/em/text().4")
        assert.is_not_nil(result, err)
        assert.equals(1139, result.eid)
        assert.equals(10, result.eid_offset) -- 6 chars before em text + 4
        assert.equals(27125, result.pid)
        assert.is_number(result.percent)
        assert.truthy(result.percent > 0 and result.percent <= 100)
    end)

    it("translates a tail text node through its parent element", function()
        local result, err = PositionMap.translate_xpointer(
            MAP, "/body/DocFragment/body/div/p/text()[2].3")
        assert.is_not_nil(result, err)
        assert.equals(14, result.eid_offset) -- 6 + 5 + 3
        assert.equals(27129, result.pid)
    end)

    it("rejects XPointers on unanchored elements", function()
        local result = PositionMap.translate_xpointer(
            MAP, "/body/DocFragment/body/p[2]/text().1")
        assert.is_nil(result)
    end)

    it("round-trips a native position back to the same XPointer", function()
        local forward = assert(PositionMap.translate_xpointer(
            MAP, "/body/DocFragment/body/div/p/em/text().4"))
        local restored, err = PositionMap.translate_native(MAP, forward.long)
        assert.is_not_nil(restored, err)
        assert.equals(forward.long, restored.long)
        assert.equals("/body/DocFragment/body/div/p/em/text().4", restored.xpointer)
        assert.equals(forward.pid, restored.pid)
    end)

    it("round-trips a tail node", function()
        local forward = assert(PositionMap.translate_xpointer(
            MAP, "/body/DocFragment/body/div/p/text()[2].3"))
        local restored = assert(PositionMap.translate_native(MAP, forward.long))
        assert.equals("/body/DocFragment/body/div/p/text()[2].3", restored.xpointer)
    end)

    it("rejects native positions whose element is absent", function()
        local result, err = PositionMap.translate_native(MAP, "AAAAAAAAAA")
        assert.is_nil(result)
        assert.is_truthy(err)
    end)
end)
