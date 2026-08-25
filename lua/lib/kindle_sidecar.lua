-- Kindle Reader Data Store sidecar codec.
--
-- Pure-Lua port of the KRDS TLV format (signature 00*4 00 1A B1 26) used by
-- .yjf/.yjr reading-position sidecars, so exact-position sync reads and writes
-- the device's own state in-process without spawning the bundled interpreter.

local KindleSidecar = {}

local SIGNATURE = "\0\0\0\0\0\26\177\38"

local TAG_BOOLEAN = 0
local TAG_INT = 1
local TAG_LONG = 2
local TAG_UTF = 3
local TAG_DOUBLE = 4
local TAG_SHORT = 5
local TAG_FLOAT = 6
local TAG_BYTE = 7
local TAG_CHAR = 9
local TAG_OBJECT_BEGIN = -2
local TAG_OBJECT_END = -1

local function signed(b)
    if b >= 128 then
        return b - 256
    end
    return b
end

local function tag_byte(tag)
    return string.char(tag < 0 and tag + 256 or tag)
end

local Cursor = {}
Cursor.__index = Cursor

function Cursor:new(data)
    return setmetatable({ data = data, offset = 1 }, self)
end

function Cursor:remaining()
    return #self.data - self.offset + 1
end

function Cursor:read(count)
    if self:remaining() < count then
        return nil, "truncated KRDS value"
    end
    local chunk = self.data:sub(self.offset, self.offset + count - 1)
    self.offset = self.offset + count
    return chunk
end

function Cursor:read_i8()
    local chunk = self:read(1)
    if not chunk then
        return nil, chunk
    end
    return signed(chunk:byte())
end

function Cursor:read_i16()
    local chunk = self:read(2)
    if not chunk then
        return nil, chunk
    end
    local b1, b2 = chunk:byte(1, 2)
    return signed(b1) * 256 + b2
end

function Cursor:read_i32()
    local chunk = self:read(4)
    if not chunk then
        return nil, chunk
    end
    local b1, b2, b3, b4 = chunk:byte(1, 4)
    return signed(b1) * 16777216 + b2 * 65536 + b3 * 256 + b4
end

function Cursor:read_u32()
    local chunk = self:read(4)
    if not chunk then
        return nil, chunk
    end
    local b1, b2, b3, b4 = chunk:byte(1, 4)
    return ((b1 * 256 + b2) * 256 + b3) * 256 + b4
end

function Cursor:read_i64()
    local hi = self:read_i32()
    local lo = self:read_u32()
    if not hi then
        return nil, lo
    end
    return hi * 4294967296 + lo
end

function Cursor:read_utf_body()
    local empty = self:read_i8()
    if not empty then
        return nil, empty
    end
    if empty == 1 then
        return ""
    end
    if empty ~= 0 then
        return nil, "invalid KRDS UTF empty marker"
    end
    local chunk = self:read(2)
    if not chunk then
        return nil, chunk
    end
    local size = chunk:byte(1) * 256 + chunk:byte(2)
    local text = self:read(size)
    if not text then
        return nil, text
    end
    return text, false
end

local function parse_value(cursor)
    local tag = cursor:read_i8()
    if not tag then
        return nil, tag
    end
    if tag == TAG_BOOLEAN then
        local value = cursor:read_i8()
        if not value then
            return nil, value
        end
        if value ~= 0 and value ~= 1 then
            return nil, "invalid KRDS boolean"
        end
        return { tag = tag, value = value == 1 }
    elseif tag == TAG_INT then
        local value = cursor:read_i32()
        if not value then
            return nil, value
        end
        return { tag = tag, value = value }
    elseif tag == TAG_LONG then
        local value = cursor:read_i64()
        if not value then
            return nil, value
        end
        return { tag = tag, value = value }
    elseif tag == TAG_UTF then
        local value, empty = cursor:read_utf_body()
        if not value then
            return nil, empty
        end
        return { tag = tag, value = value, empty = empty }
    elseif tag == TAG_DOUBLE or tag == TAG_FLOAT then
        -- Raw byte passthrough; sync never reads floating point values.
        local size = tag == TAG_DOUBLE and 8 or 4
        local chunk = cursor:read(size)
        if not chunk then
            return nil, chunk
        end
        return { tag = tag, value = chunk }
    elseif tag == TAG_SHORT then
        local value = cursor:read_i16()
        if not value then
            return nil, value
        end
        return { tag = tag, value = value }
    elseif tag == TAG_BYTE then
        local value = cursor:read_i8()
        if not value then
            return nil, value
        end
        return { tag = tag, value = value }
    elseif tag == TAG_CHAR then
        local chunk = cursor:read(1)
        if not chunk then
            return nil, chunk
        end
        return { tag = tag, value = chunk }
    elseif tag == TAG_OBJECT_BEGIN then
        local name = cursor:read_utf_body()
        if not name then
            return nil, name
        end
        local values = {}
        while true do
            if cursor:remaining() < 1 then
                return nil, "unterminated KRDS object"
            end
            local next_tag = signed(cursor.data:byte(cursor.offset))
            if next_tag == TAG_OBJECT_END then
                cursor.offset = cursor.offset + 1
                break
            end
            local child, child_error = parse_value(cursor)
            if not child then
                return nil, child_error
            end
            table.insert(values, child)
        end
        return { tag = tag, value = { name = name, values = values } }
    end
    return nil, "unknown KRDS datatype " .. tag
end

local function encode_utf(value, empty_sentinel)
    if empty_sentinel and value == "" then
        return "\1"
    end
    if #value > 65535 then
        return nil, "KRDS UTF value is too long"
    end
    return "\0" .. string.char(math.floor(#value / 256), #value % 256) .. value
end

local function be_bytes(number, count)
    local bytes = {}
    for i = count - 1, 0, -1 do
        local shift = 256 ^ i
        table.insert(bytes, string.char(math.floor(number / shift) % 256))
    end
    return table.concat(bytes)
end

local function encode_value(node)
    local tag = node.tag
    if tag == TAG_BOOLEAN then
        return tag_byte(tag) .. string.char(node.value and 1 or 0)
    elseif tag == TAG_INT then
        return tag_byte(tag) .. be_bytes(node.value, 4)
    elseif tag == TAG_LONG then
        local lo = node.value % 4294967296
        local hi = (node.value - lo) / 4294967296
        return tag_byte(tag) .. be_bytes(hi, 4) .. be_bytes(lo, 4)
    elseif tag == TAG_UTF then
        local body = encode_utf(node.value, node.empty)
        if not body then
            return nil, body
        end
        return tag_byte(tag) .. body
    elseif tag == TAG_DOUBLE or tag == TAG_FLOAT then
        return tag_byte(tag) .. node.value
    elseif tag == TAG_SHORT then
        local v = node.value
        return tag_byte(tag) .. string.char(math.floor(v / 256) % 256, v % 256)
    elseif tag == TAG_BYTE then
        return tag_byte(tag) .. string.char(node.value)
    elseif tag == TAG_CHAR then
        return tag_byte(tag) .. node.value
    elseif tag == TAG_OBJECT_BEGIN then
        local body = encode_utf(node.value.name)
        if not body then
            return nil, body
        end
        local encoded = { tag_byte(tag), body }
        for _, child in ipairs(node.value.values) do
            local chunk = encode_value(child)
            if not chunk then
                return nil, chunk
            end
            table.insert(encoded, chunk)
        end
        table.insert(encoded, tag_byte(TAG_OBJECT_END))
        return table.concat(encoded)
    end
    return nil, "unknown KRDS datatype " .. tag
end

function KindleSidecar.parse(data)
    if data:sub(1, #SIGNATURE) ~= SIGNATURE then
        return nil, "invalid KRDS signature"
    end
    local cursor = Cursor:new(data)
    cursor.offset = #SIGNATURE + 1
    local first = parse_value(cursor)
    if not first then
        return nil, first
    end
    local count = parse_value(cursor)
    if not count then
        return nil, count
    end
    if first.value ~= 1 or count.tag ~= TAG_INT or count.value < 0 then
        return nil, "invalid KRDS header"
    end
    local values = {}
    for _ = 1, count.value do
        local parsed, error = parse_value(cursor)
        if not parsed then
            return nil, error
        end
        table.insert(values, parsed)
    end
    if cursor:remaining() ~= 0 then
        return nil, "extra data after KRDS store"
    end
    return { first = first, count = count, values = values }
end

function KindleSidecar.encode(store)
    if store.count.value ~= #store.values then
        return nil, "KRDS object count mismatch"
    end
    local encoded = { SIGNATURE }
    for _, node in ipairs({ store.first, store.count }) do
        local chunk = encode_value(node)
        if not chunk then
            return nil, chunk
        end
        table.insert(encoded, chunk)
    end
    for _, value in ipairs(store.values) do
        local chunk = encode_value(value)
        if not chunk then
            return nil, chunk
        end
        table.insert(encoded, chunk)
    end
    return table.concat(encoded)
end

function KindleSidecar.objects(store, name)
    local result = {}
    local function visit(node)
        if node.tag ~= TAG_OBJECT_BEGIN then
            return
        end
        if name == nil or node.value.name == name then
            table.insert(result, node.value)
        end
        for _, child in ipairs(node.value.values) do
            visit(child)
        end
    end
    for _, value in ipairs(store.values) do
        visit(value)
    end
    return result
end

function KindleSidecar.position_from_object(obj)
    local values = obj.values
    if obj.name == "lpr" then
        if values[1] and values[1].tag == TAG_UTF then
            return values[1].value, nil
        end
        if #values >= 3 and values[1].tag == TAG_BYTE and values[1].value <= 2
            and values[2].tag == TAG_UTF and values[3].tag == TAG_LONG
        then
            return values[2].value, values[3].value
        end
    elseif obj.name == "updated_lpr" or obj.name == "fpr" then
        if #values >= 2 and values[1].tag == TAG_UTF and values[2].tag == TAG_LONG then
            return values[1].value, values[2].value
        end
    elseif obj.name == "erl" then
        if values[1] and values[1].tag == TAG_UTF then
            return values[1].value, nil
        end
    end
    return nil, "unsupported " .. obj.name .. " object"
end

function KindleSidecar.set_object_position(obj, position, timestamp_ms)
    local values = obj.values
    if obj.name == "lpr" then
        if values[1] and values[1].tag == TAG_UTF then
            values[1].value = position
            return true
        end
        if #values >= 3 and values[1].tag == TAG_BYTE and values[1].value <= 2 then
            values[2].value = position
            values[3].value = timestamp_ms
            return true
        end
    elseif obj.name == "updated_lpr" or obj.name == "fpr" then
        if #values >= 2 and values[1].tag == TAG_UTF then
            values[1].value = position
            values[2].value = timestamp_ms
            return true
        end
    elseif obj.name == "erl" then
        if values[1] and values[1].tag == TAG_UTF then
            values[1].value = position
            return true
        end
    end
    return nil, "unsupported " .. obj.name .. " object"
end

return KindleSidecar
