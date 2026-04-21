-- User patch to fix position restoration when using last_percent from sync.
-- When a book is opened after syncing progress from Kindle, we set
-- last_percent in doc_settings. KOReader's ReaderRolling handles this
-- in a backward-compatibility branch. This patch ensures page mode
-- position restoration works correctly after sync.
-- Adapted from kobo.koplugin/patches/2-readerrolling-position-fix.lua
-- Priority: 2

local logger = require("logger")

local original_require = require
_G.require = function(modname)
    local result = original_require(modname)

    if modname == "apps/reader/modules/readerrolling" and type(result) == "table" then
        local ReaderRolling = result

        if not ReaderRolling._patched_kindle_position_fix then
            local original_onReadSettings = ReaderRolling.onReadSettings
            ReaderRolling.onReadSettings = function(self, config)
                original_onReadSettings(self, config)

                local original_setupXpointer = self.setupXpointer
                if original_setupXpointer then
                    local last_per = config:readSetting("last_percent")
                    -- Only patch if we have last_percent and we're in page mode
                    -- and last_xpointer was not set (our sync case)
                    if last_per and not config:readSetting("last_xpointer")
                        and self.view and self.view.view_mode == "page" then
                        self.setupXpointer = function()
                            logger.dbg("KindlePlugin: Position fix - restoring from last_percent:", last_per * 100, "%")
                            self:_gotoPercent(last_per * 100)
                            self.ui.document:gotoPos(self.current_pos)
                            self.xpointer = self.ui.document:getXPointer()
                        end
                    end
                end
            end
            ReaderRolling._patched_kindle_position_fix = true
            logger.info("KindlePlugin: Applied ReaderRolling position restoration fix")
        end
    end

    return result
end
