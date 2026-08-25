-- Exact-position translation over a conversion-time position map.
--
-- The Python helper emits <cache-id>.positions.json next to each converted
-- EPUB (python/position_map.py). This module performs both translation
-- directions in-process from that map, so sync never spawns the bundled
-- interpreter.

local json = require("json")

local PositionMap = {}

local XPOINTER_RE = "^(/.*)%.(%d+)$"
local STEP_RE = "^([^%[]+)%[(%d+)%]$"

local function canonical_step(step)
    local name, index = step:match(STEP_RE)
    if not name then
        return step
    end
    if tonumber(index) == 1 then
        return name
    end
    return step
end

local cache = {}

function PositionMap.load(epub_path)
    local map_path = epub_path:gsub("%.epub$", ".positions.json")
    local attr = require("libs/libkoreader-lfs").attributes(map_path)
    if not attr or attr.mode ~= "file" then
        return nil, "position map is missing for " .. epub_path
    end
    local cached = cache[epub_path]
    if cached and cached.mtime == attr.modification and cached.size == attr.size then
        return cached.map
    end
    local handle = io.open(map_path, "rb")
    if not handle then
        return nil, "cannot open position map"
    end
    local payload = handle:read("*a")
    handle:close()
    local ok, decoded = pcall(json.decode, payload)
    if not ok or type(decoded) ~= "table" or decoded.version ~= 1 then
        return nil, "invalid position map for " .. epub_path
    end

    -- Index anchors by EID for reverse translation.
    decoded._eid_index = {}
    for fi, fragment in ipairs(decoded.fragments) do
        for ai, anchor in ipairs(fragment.anchors) do
            decoded._eid_index[tostring(anchor.eid)] = { fi = fi, ai = ai }
        end
    end

    cache[epub_path] = {
        map = decoded,
        mtime = attr.modification,
        size = attr.size,
    }
    return decoded
end

function PositionMap.forget(epub_path)
    cache[epub_path] = nil
end

local function encode_long(eid, offset)
    -- Mirrors Python: b64(0x01 + eid_le32 + offset_le32).
    local raw = string.char(
        1,
        eid % 256,
        math.floor(eid / 256) % 256,
        math.floor(eid / 65536) % 256,
        math.floor(eid / 16777216) % 256,
        offset % 256,
        math.floor(offset / 256) % 256,
        math.floor(offset / 65536) % 256,
        math.floor(offset / 16777216) % 256
    )
    local mime = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
    return (
        raw:gsub("...", function(chunk)
            local b1, b2, b3 = chunk:byte(1, 3)
            local n = b1 * 65536 + b2 * 256 + b3
            return table.concat({
                mime:sub(math.floor(n / 262144) % 64 + 1, math.floor(n / 262144) % 64 + 1),
                mime:sub(math.floor(n / 4096) % 64 + 1, math.floor(n / 4096) % 64 + 1),
                mime:sub(math.floor(n / 64) % 64 + 1, math.floor(n / 64) % 64 + 1),
                mime:sub(n % 64 + 1, n % 64 + 1),
            })
        end)
    )
end

function PositionMap.decode_long(long_position)
    local raw = {}
    local mime = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
    long_position = long_position:gsub("=+$", "")
    for ch in long_position:gmatch(".") do
        local v = mime:find(ch, 1, true)
        if not v then
            return nil, "invalid native long position"
        end
        table.insert(raw, v - 1)
    end
    local bits = {}
    local bitcount = 0
    for _, v in ipairs(raw) do
        for i = 5, 0, -1 do
            table.insert(bits, math.floor(v / 2 ^ i) % 2)
        end
        bitcount = bitcount + 6
    end
    local bytes = {}
    for i = 0, math.floor(#bits / 8) - 1 do
        local b = 0
        for j = 1, 8 do
            b = b * 2 + bits[i * 8 + j]
        end
        table.insert(bytes, b)
    end
    if #bytes ~= 9 or bytes[1] ~= 1 then
        return nil, "unsupported native long position"
    end
    local eid = bytes[2] + bytes[3] * 256 + bytes[4] * 65536 + bytes[5] * 16777216
    local offset = bytes[6] + bytes[7] * 256 + bytes[8] * 65536 + bytes[9] * 16777216
    return eid, offset
end

--- Translate KOReader's normalized XPointer into native coordinates.
function PositionMap.translate_xpointer(map, xpointer)
    if type(xpointer) ~= "string" then
        return nil, "invalid normalized XPointer"
    end
    local path, offset_text = xpointer:match(XPOINTER_RE)
    if not path then
        return nil, "invalid normalized XPointer"
    end
    local character_offset = tonumber(offset_text)

    local steps = {}
    for step in path:gmatch("[^/]+") do
        table.insert(steps, step)
    end
    if #steps < 3 or steps[1] ~= "body" then
        return nil, "unsupported normalized XPointer root"
    end
    local fragment_step = steps[2]
    local fragment_name = fragment_step:match("^([^%[]+)")
    local fragment_index = tonumber(fragment_step:match("%[(%d+)%]")) or 1
    if fragment_name ~= "DocFragment" then
        return nil, "XPointer does not identify an EPUB document"
    end

    local fragment = map.fragments[fragment_index]
    if not fragment then
        return nil, "XPointer document is outside EPUB spine"
    end

    -- Walk element steps below body, building the canonical body-relative
    -- path exactly as the map generator recorded it.
    local current_path = ""
    local started = false
    local target_node = nil
    for i = 3, #steps do
        local step = steps[i]
        -- Lua patterns treat () as captures; compare the literal prefix.
        if step == "text()" or step:sub(1, 7) == "text()[" then
            target_node = step
        else
            if step == "body" and not started then
                started = true
            else
                started = true
                if current_path == "" then
                    current_path = canonical_step(step)
                else
                    current_path = current_path .. "/" .. canonical_step(step)
                end
            end
        end
    end
    local entry = fragment.elements[current_path]
    if entry == nil and current_path ~= "" then
        -- An XPointer may stop on <body> itself; that has no anchor below.
        entry = fragment.elements[""]
    end
    if not entry then
        return nil, "XPointer element is missing from position map"
    end
    if entry.a == 0 then
        return nil, "XPointer has no KFX position anchor"
    end
    local anchor = fragment.anchors[entry.a]

    local eid_offset
    if target_node then
        local node_index = tonumber(target_node:match("%[(%d+)%]")) or 1
        -- The anchor's node list carries each text node's offset within the
        -- anchor (children's subtree text included); the element's own list
        -- alone cannot express that.
        local node
        for _, candidate in ipairs(anchor.nodes) do
            if candidate.p == current_path and candidate.n == node_index then
                node = candidate
                break
            end
        end
        if not node then
            return nil, "XPointer text node is missing"
        end
        if character_offset > node.v then
            return nil, "XPointer character offset is outside text node"
        end
        eid_offset = node.c + character_offset
    else
        local node
        for _, candidate in ipairs(anchor.nodes) do
            if candidate.p == current_path and candidate.n == 1 then
                node = candidate
                break
            end
        end
        if not node then
            return nil, "XPointer text node is outside KFX element"
        end
        if character_offset > node.v then
            return nil, "XPointer character offset is outside text node"
        end
        eid_offset = node.c + character_offset
    end

    local pid = anchor.pid + eid_offset
    return {
        eid = anchor.eid,
        eid_offset = eid_offset,
        pid = pid,
        long = encode_long(anchor.eid, eid_offset),
        percent = PositionMap.percent(map, pid),
    }
end

function PositionMap.percent(map, pid)
    if map.max_pid <= 0 then
        return nil, "position map has no KFX position range"
    end
    local value = pid * 100.0 / map.max_pid
    if value < 0 then
        value = 0
    end
    if value > 100 then
        value = 100
    end
    return value
end

--- Translate a native long position back to KOReader's XPointer.
function PositionMap.translate_native(map, long_position)
    local eid, offset = PositionMap.decode_long(long_position)
    if not eid then
        return nil, offset
    end
    if not map._eid_index then
        map._eid_index = {}
        for fi, fragment in ipairs(map.fragments) do
            for ai, anchor in ipairs(fragment.anchors) do
                map._eid_index[tostring(anchor.eid)] = { fi = fi, ai = ai }
            end
        end
    end
    local location = map._eid_index[tostring(eid)]
    if not location then
        return nil, "native KFX element is missing from position map"
    end
    local fragment = map.fragments[location.fi]
    local anchor = fragment.anchors[location.ai]

    local node
    for _, candidate in ipairs(anchor.nodes) do
        if offset > candidate.c and offset <= candidate.c + candidate.v then
            node = candidate
            break
        end
    end
    if not node then
        if offset == 0 and anchor.nodes[1] and anchor.nodes[1].c == 0 then
            node = anchor.nodes[1]
        else
            return nil, "native offset is outside KFX element"
        end
    end
    local text_offset = offset - node.c

    local fragment_step = "DocFragment"
    if location.fi > 1 then
        fragment_step = "DocFragment[" .. location.fi .. "]"
    end
    local node_step = "text()"
    if node.n > 1 then
        node_step = "text()[" .. node.n .. "]"
    end
    local xpointer = "/body/" .. fragment_step .. "/body/" .. node.p .. "/" .. node_step .. "." .. text_offset

    local verified = PositionMap.translate_xpointer(map, xpointer)
    if not verified or verified.long ~= long_position then
        return nil, "reverse position verification failed"
    end
    return {
        xpointer = xpointer,
        eid = eid,
        eid_offset = offset,
        pid = verified.pid,
        long = long_position,
        percent = PositionMap.percent(map, verified.pid),
    }
end
PositionMap.encode_long = encode_long

return PositionMap
