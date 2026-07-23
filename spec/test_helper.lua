require("busted.runner")()

--- Shared test support layered on koplugin-dev's real headless KOReader.
---
--- Framework modules are never replaced here. Tests use real KOReader modules
--- and patch individual functions with save/restore when they need observation
--- or fault injection. Only external boundaries are virtualized:
---   * selected filesystem paths
---   * io.open handles
---   * Kindle cc.db SQLite connections

local M = {}

local UIManager = require("ui/uimanager")
local lfs = require("libs/libkoreader-lfs")

M.state = {
    uimanager_show_calls = {},
    uimanager_shown_widgets = {},
    uimanager_close_calls = {},
}

-- =============================================================================
-- Transparent UIManager spies
-- =============================================================================

local ui_spies_installed = false
local original_ui_show
local original_ui_close

local function installUIManagerSpies()
    if not ui_spies_installed then
        ui_spies_installed = true
        original_ui_show = UIManager.show
        original_ui_close = UIManager.close
    end

    UIManager.show = function(self, widget, ...)
        table.insert(UIManager._show_calls, {
            widget = widget,
            text = widget and widget.text or nil,
        })
        table.insert(UIManager._shown_widgets, widget)
        -- Real widgets should still see KOReader's lifecycle. Some unit specs
        -- intentionally pass small target doubles, so rendering remains best-effort.
        pcall(original_ui_show, UIManager, widget, ...)
    end

    UIManager.close = function(self, widget, ...)
        table.insert(UIManager._close_calls, { widget = widget })
        pcall(original_ui_close, UIManager, widget, ...)
    end

    UIManager._show_calls = UIManager._show_calls or {}
    UIManager._shown_widgets = UIManager._shown_widgets or {}
    UIManager._close_calls = UIManager._close_calls or {}
    UIManager._reset = function()
        UIManager._show_calls = {}
        UIManager._shown_widgets = {}
        UIManager._close_calls = {}
    end
end

-- =============================================================================
-- Real LFS with a controllable path overlay
-- =============================================================================

local lfs_overlay_installed = false
local original_lfs_attributes = lfs.attributes
local original_lfs_dir = lfs.dir
local file_states = {}
local directory_contents = {}

local function installLfsOverlay()
    if lfs_overlay_installed then
        return
    end
    lfs_overlay_installed = true

    lfs.attributes = function(path, attr_name)
        local state = file_states[path]
        if state ~= nil then
            if state.exists == false then
                return nil
            end
            local attrs = state.attributes or state
            if attr_name then
                return attrs[attr_name]
            end
            return attrs
        end
        return original_lfs_attributes(path, attr_name)
    end

    lfs.dir = function(path)
        local contents = directory_contents[path]
        if contents == nil then
            return original_lfs_dir(path)
        end
        local index = 0
        return function()
            index = index + 1
            return contents[index]
        end
    end

    lfs._setFileState = function(path, state)
        file_states[path] = state
    end
    lfs._setDirectoryContents = function(path, contents)
        directory_contents[path] = contents
    end
    lfs._clearFileStates = function()
        file_states = {}
        directory_contents = {}
    end
end

-- =============================================================================
-- io.open boundary
-- =============================================================================

local original_io_open = io.open

local function createIOOpenMocker()
    local mock_files = {}
    local IO_OPEN_FAIL = {}
    local installed = false

    return {
        install = function()
            if installed then
                return
            end
            installed = true
            rawset(io, "open", function(path, mode)
                local mock_file = mock_files[path]
                if mock_file ~= nil then
                    if mock_file == IO_OPEN_FAIL then
                        return nil
                    end
                    return mock_file
                end
                return original_io_open(path, mode)
            end)
        end,
        uninstall = function()
            if not installed then
                return
            end
            rawset(io, "open", original_io_open)
            installed = false
            mock_files = {}
        end,
        setMockFile = function(path, file_mock)
            mock_files[path] = file_mock
        end,
        setMockFileFailure = function(path)
            mock_files[path] = IO_OPEN_FAIL
        end,
        clear = function()
            mock_files = {}
        end,
    }
end

_G.createIOOpenMocker = createIOOpenMocker
_G._test_real_io_open = original_io_open

-- =============================================================================
-- Explicit SQLite boundary controls
-- =============================================================================

local sqlite_module_name = "lua-ljsqlite3/init"
local original_sqlite_preload = package.preload[sqlite_module_name]

local function restoreSqliteModule()
    package.loaded[sqlite_module_name] = nil
    package.preload[sqlite_module_name] = original_sqlite_preload
end

function M.install_sqlite_unavailable()
    package.loaded[sqlite_module_name] = nil
    package.preload[sqlite_module_name] = function()
        error("ljsqlite3 intentionally unavailable for this test")
    end
end

function M.install_sqlite_mock()
    local rows
    local row_count = 0
    local opened_path

    local connection = {}
    function connection:exec()
        return rows, row_count
    end
    function connection:close() end

    local SQ3 = {}
    function SQ3.open(path)
        opened_path = path
        return connection
    end
    function SQ3._setMockResults(new_rows, new_row_count)
        rows = new_rows
        row_count = new_row_count or 0
    end
    function SQ3._getMockDbPath()
        return opened_path
    end
    function SQ3._reset()
        rows = nil
        row_count = 0
        opened_path = nil
    end

    package.loaded[sqlite_module_name] = SQ3
    package.preload[sqlite_module_name] = function()
        return SQ3
    end
    return SQ3
end

-- =============================================================================
-- Setup and reset
-- =============================================================================

function M.setup_complete()
    installUIManagerSpies()
    installLfsOverlay()
end

function M.reset_state()
    UIManager:_reset()

    if G_reader_settings and G_reader_settings.data then
        for key in pairs(G_reader_settings.data) do
            G_reader_settings:delSetting(key)
        end
    end

    lfs._clearFileStates()
    restoreSqliteModule()
end

function M.before_each()
    M.reset_state()
end

function _G.resetAllMocks()
    M.reset_state()
end

function M.get_lfs()
    return lfs
end

function M.get_uimanager()
    return UIManager
end

return M
