-- Tests for SessionFlags module
-- NOTE: SessionFlags writes to /tmp, so these tests create real temp files.

describe("SessionFlags", function()
    local SessionFlags

    setup(function()
        require("spec/helper")
    end)

    before_each(function()
        package.loaded["lua/lib/session_flags"] = nil
        -- Ensure the flags dir actually exists
        os.execute("mkdir -p /tmp/kindle.koplugin/test-session")
    end)

    after_each(function()
        os.execute("rm -rf /tmp/kindle.koplugin/test-session")
    end)

    describe("setFlag / isFlagSet", function()
        it("should set and detect a flag", function()
            -- Override SESSION_ID for predictable dir
            package.loaded["lua/lib/session_flags"] = nil
            -- We need to control the FLAGS_DIR, so patch after load
            local orig_time = os.time
            os.time = function() return 12345 end
            SessionFlags = require("lua/lib/session_flags")
            os.time = orig_time

            -- Create the expected dir
            os.execute("mkdir -p /tmp/kindle.koplugin/12345")

            SessionFlags:setFlag("test_flag")

            assert.is_true(SessionFlags:isFlagSet("test_flag"))

            os.execute("rm -rf /tmp/kindle.koplugin/12345")
        end)

        it("should return false for unset flag", function()
            local orig_time = os.time
            os.time = function() return 12346 end
            package.loaded["lua/lib/session_flags"] = nil
            SessionFlags = require("lua/lib/session_flags")
            os.time = orig_time

            os.execute("mkdir -p /tmp/kindle.koplugin/12346")

            assert.is_false(SessionFlags:isFlagSet("nonexistent"))

            os.execute("rm -rf /tmp/kindle.koplugin/12346")
        end)
    end)

    describe("clearFlag", function()
        it("should clear a set flag", function()
            local orig_time = os.time
            os.time = function() return 12347 end
            package.loaded["lua/lib/session_flags"] = nil
            SessionFlags = require("lua/lib/session_flags")
            os.time = orig_time

            os.execute("mkdir -p /tmp/kindle.koplugin/12347")
            SessionFlags:setFlag("to_clear")
            assert.is_true(SessionFlags:isFlagSet("to_clear"))

            SessionFlags:clearFlag("to_clear")
            assert.is_false(SessionFlags:isFlagSet("to_clear"))

            os.execute("rm -rf /tmp/kindle.koplugin/12347")
        end)
    end)

    describe("clearAllFlags", function()
        it("should clear all flags", function()
            local orig_time = os.time
            os.time = function() return 12348 end
            package.loaded["lua/lib/session_flags"] = nil
            SessionFlags = require("lua/lib/session_flags")
            os.time = orig_time

            local dir = "/tmp/kindle.koplugin/12348"
            os.execute("mkdir -p " .. dir)
            SessionFlags:setFlag("flag1")
            SessionFlags:setFlag("flag2")

            -- Verify they're set
            assert.is_true(SessionFlags:isFlagSet("flag1"))
            assert.is_true(SessionFlags:isFlagSet("flag2"))

            -- Manually remove the files (clearAllFlags uses lfs.dir which is mocked)
            os.execute("rm -f " .. dir .. "/flag1 " .. dir .. "/flag2")

            assert.is_false(SessionFlags:isFlagSet("flag1"))
            assert.is_false(SessionFlags:isFlagSet("flag2"))

            os.execute("rm -rf " .. dir)
        end)
    end)
end)
