-- Status format conversion between Kindle and KOReader.
-- Handles bidirectional conversion of reading status values.
-- Copied from kobo.koplugin/src/lib/status_converter.lua (adapted for Kindle).

local StatusConverter = {}

---
--- Converts Kindle p_readState value to KOReader status string.
--- Kindle uses numeric values: NULL=unopened, 6=opened/reading.
--- KOReader uses strings: abandoned, reading, complete.
--- @param kindle_status number|nil: Kindle p_readState value.
--- @return string: KOReader status string.
function StatusConverter.kindleToKoreader(kindle_status)
    local status_num = tonumber(kindle_status) or 0

    if status_num == 0 then
        return ""
    end

    if status_num == 6 then
        -- Kindle uses 6 for "has been opened / reading"
        return "reading"
    end

    return "reading"
end

---
--- Converts KOReader status string to Kindle p_readState value.
--- KOReader uses strings: abandoned, reading, complete, on-hold.
--- Kindle uses numeric values: NULL=unopened, 6=opened/reading.
--- @param kr_status string: KOReader status string.
--- @return number: Kindle p_readState value.
function StatusConverter.koreaderToKindle(kr_status)
    if kr_status == "reading" or kr_status == "in-progress" then
        return 6
    end

    if kr_status == "complete" or kr_status == "finished" then
        return 6
    end

    return 6
end

return StatusConverter
