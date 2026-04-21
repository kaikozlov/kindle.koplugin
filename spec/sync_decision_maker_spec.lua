-- Tests for SyncDecisionMaker module

describe("SyncDecisionMaker", function()
    local SyncDecisionMaker

    setup(function()
        require("spec/helper")
    end)

    before_each(function()
        package.loaded["lua/lib/sync_decision_maker"] = nil
        SyncDecisionMaker = require("lua/lib/sync_decision_maker")
        resetAllMocks()
    end)

    describe("areBothSidesComplete", function()
        it("should return false for nil state", function()
            assert.is_false(SyncDecisionMaker.areBothSidesComplete(nil, 0.5, "reading"))
        end)

        it("should return true when both sides are complete", function()
            local kindle_state = { status = "complete", percent_read = 100 }
            assert.is_true(SyncDecisionMaker.areBothSidesComplete(kindle_state, 1.0, "complete"))
        end)

        it("should return true when both percent >= 100 and status is finished", function()
            local kindle_state = { status = "reading", percent_read = 100 }
            assert.is_true(SyncDecisionMaker.areBothSidesComplete(kindle_state, 1.0, nil))
        end)

        it("should return true when kr status is finished even with lower percent", function()
            local kindle_state = { status = "complete", percent_read = 95 }
            assert.is_true(SyncDecisionMaker.areBothSidesComplete(kindle_state, 0.9, "finished"))
        end)

        it("should return false when only one side is complete", function()
            local kindle_state = { status = "complete", percent_read = 100 }
            assert.is_false(SyncDecisionMaker.areBothSidesComplete(kindle_state, 0.5, "reading"))
        end)
    end)

    describe("syncIfApproved", function()
        local SYNC_DIRECTION

        before_each(function()
            SYNC_DIRECTION = { PROMPT = 1, SILENT = 2, NEVER = 3 }
        end)

        it("should deny sync when plugin is nil", function()
            assert.is_false(SyncDecisionMaker.syncIfApproved(nil, SYNC_DIRECTION, true, true, function() end))
        end)

        it("should deny sync when sync_direction is nil", function()
            assert.is_false(
                SyncDecisionMaker.syncIfApproved({ settings = {} }, nil, true, true, function() end)
            )
        end)

        it("should silently execute when direction is SILENT", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_from_kindle = true,
                    sync_from_kindle_newer = SYNC_DIRECTION.SILENT,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, true, true,
                function() executed = true end
            )

            assert.is_true(ok)
            assert.is_true(executed)
        end)

        it("should deny sync when direction is NEVER", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_from_kindle = true,
                    sync_from_kindle_newer = SYNC_DIRECTION.NEVER,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, true, true,
                function() executed = true end
            )

            assert.is_false(ok)
            assert.is_false(executed)
        end)

        it("should deny FROM sync when enable_sync_from_kindle is false", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_from_kindle = false,
                    sync_from_kindle_newer = SYNC_DIRECTION.SILENT,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, true, true,
                function() executed = true end
            )

            assert.is_false(ok)
            assert.is_false(executed)
        end)

        it("should deny TO sync when enable_sync_to_kindle is false", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_to_kindle = false,
                    sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, false, true,
                function() executed = true end
            )

            assert.is_false(ok)
            assert.is_false(executed)
        end)

        it("should prompt user when direction is PROMPT", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_from_kindle = true,
                    sync_from_kindle_newer = SYNC_DIRECTION.PROMPT,
                },
            }

            -- In test env, Trapper:confirm returns true by default
            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, true, true,
                function() executed = true end
            )

            assert.is_true(ok)
            -- ConfirmBox path is async so the callback may not execute in tests
        end)

        it("should respect sync_from_kindle_older = SILENT", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_from_kindle = true,
                    sync_from_kindle_older = SYNC_DIRECTION.SILENT,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, true, false, -- FROM Kindle, older
                function() executed = true end
            )

            assert.is_true(ok)
            assert.is_true(executed)
        end)

        it("should respect sync_from_kindle_older = NEVER", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_from_kindle = true,
                    sync_from_kindle_older = SYNC_DIRECTION.NEVER,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, true, false,
                function() executed = true end
            )

            assert.is_false(ok)
            assert.is_false(executed)
        end)

        it("should respect sync_to_kindle_newer = SILENT", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_to_kindle = true,
                    sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, false, true, -- TO Kindle, newer
                function() executed = true end
            )

            assert.is_true(ok)
            assert.is_true(executed)
        end)

        it("should respect sync_to_kindle_newer = NEVER", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_to_kindle = true,
                    sync_to_kindle_newer = SYNC_DIRECTION.NEVER,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, false, true,
                function() executed = true end
            )

            assert.is_false(ok)
            assert.is_false(executed)
        end)

        it("should respect sync_to_kindle_older = SILENT", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_to_kindle = true,
                    sync_to_kindle_older = SYNC_DIRECTION.SILENT,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, false, false, -- TO Kindle, older
                function() executed = true end
            )

            assert.is_true(ok)
            assert.is_true(executed)
        end)

        it("should respect sync_to_kindle_older = NEVER", function()
            local executed = false
            local plugin = {
                settings = {
                    enable_sync_to_kindle = true,
                    sync_to_kindle_older = SYNC_DIRECTION.NEVER,
                },
            }

            local ok = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, false, false,
                function() executed = true end
            )

            assert.is_false(ok)
            assert.is_false(executed)
        end)

        it("should allow PULL newer but deny PUSH older in same session", function()
            -- Scenario: user allows sync FROM Kindle when Kindle is newer,
            -- but prevents pushing older progress TO Kindle
            local plugin = {
                settings = {
                    enable_sync_from_kindle = true,
                    enable_sync_to_kindle = true,
                    sync_from_kindle_newer = SYNC_DIRECTION.SILENT,
                    sync_from_kindle_older = SYNC_DIRECTION.NEVER,
                    sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
                    sync_to_kindle_older = SYNC_DIRECTION.NEVER,
                },
            }

            -- PULL newer: should succeed
            local pull_executed = false
            local ok1 = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, true, true,
                function() pull_executed = true end
            )
            assert.is_true(ok1)
            assert.is_true(pull_executed)

            -- PUSH older: should be denied
            local push_executed = false
            local ok2 = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, false, false,
                function() push_executed = true end
            )
            assert.is_false(ok2)
            assert.is_false(push_executed)
        end)

        it("should only sync FROM Kindle when TO Kindle is disabled", function()
            local plugin = {
                settings = {
                    enable_sync_from_kindle = true,
                    enable_sync_to_kindle = false,
                    sync_from_kindle_newer = SYNC_DIRECTION.SILENT,
                    sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
                },
            }

            -- FROM Kindle: should succeed
            local from_executed = false
            local ok1 = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, true, true,
                function() from_executed = true end
            )
            assert.is_true(ok1)
            assert.is_true(from_executed)

            -- TO Kindle: should be denied
            local to_executed = false
            local ok2 = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, false, true,
                function() to_executed = true end
            )
            assert.is_false(ok2)
            assert.is_false(to_executed)
        end)

        it("should only sync TO Kindle when FROM Kindle is disabled", function()
            local plugin = {
                settings = {
                    enable_sync_from_kindle = false,
                    enable_sync_to_kindle = true,
                    sync_from_kindle_newer = SYNC_DIRECTION.SILENT,
                    sync_to_kindle_newer = SYNC_DIRECTION.SILENT,
                },
            }

            -- FROM Kindle: should be denied
            local from_executed = false
            local ok1 = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, true, true,
                function() from_executed = true end
            )
            assert.is_false(ok1)
            assert.is_false(from_executed)

            -- TO Kindle: should succeed
            local to_executed = false
            local ok2 = SyncDecisionMaker.syncIfApproved(
                plugin, SYNC_DIRECTION, false, true,
                function() to_executed = true end
            )
            assert.is_true(ok2)
            assert.is_true(to_executed)
        end)
    end)
end)
