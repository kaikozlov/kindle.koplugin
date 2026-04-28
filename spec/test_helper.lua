-- test_helper.lua
-- Thin test helper for kindle.koplugin, layered on top of commonrequire.lua
--
-- Usage (inside busted specs):
--   require 'busted.runner'()
--   local helper = require("spec/test_helper")
--
--   describe("MyFeature", function()
--       setup(function()
--           helper.setup_complete()
--       end)
--       before_each(function()
--           helper.before_each()
--       end)
--   end)
--
-- What commonrequire.lua provides (real KOReader):
--   - Device, Screen, Input (headless)
--   - G_reader_settings (isolated LuaSettings)
--   - G_defaults (isolated LuaDefaults)
--   - DataStorage, logger, ffi/util, UIManager, WidgetContainer
--   - package.unload/replace/reload, load_plugin, disable_plugins
--
-- What this helper adds:
--   - Mock lfs with test helpers (_setFileState, _setDirectoryContents, _clearFileStates)
--   - Kindle-specific path overrides (datastorage → /mnt/us/koreader/...)
--   - UIManager show/close tracking wrappers
--   - io.open mocker utility
--   - resetAllMocks helper

local M = {}

-- =============================================================================
-- State storage (accessible across tests)
-- =============================================================================
M.state = {
    settings = {},
    uimanager_show_calls = {},
    uimanager_shown_widgets = {},
    uimanager_close_calls = {},
}

-- =============================================================================
-- Mock lfs (libs/libkoreader-lfs) with test helpers
-- =============================================================================
-- The real KOReader lfs module works on the host filesystem, but specs need
-- controllable file state without touching real files.

local file_states = {}
local directory_contents = {}

local mock_lfs = {
    attributes = function(path, attr_name)
        if file_states[path] ~= nil then
            if file_states[path].exists == false then
                return nil
            end
            local attrs = file_states[path].attributes
            if attrs then
                if attr_name then
                    return attrs[attr_name]
                end
                return attrs
            end
        end
        local default_attrs = { size = 100, mode = "file", modification = 1000000000 }
        if attr_name then
            return default_attrs[attr_name]
        end
        return default_attrs
    end,

    dir = function(path)
        local contents = directory_contents[path]
        if not contents then
            return function() return nil end
        end
        local index = 0
        return function()
            index = index + 1
            if index <= #contents then
                return contents[index]
            end
            return nil
        end
    end,

    mkdir = function(path)
        return true
    end,

    -- Test helpers
    _setFileState = function(path, state)
        file_states[path] = state
    end,
    _setDirectoryContents = function(path, contents)
        directory_contents[path] = contents
    end,
    _clearFileStates = function()
        file_states = {}
        directory_contents = {}
    end,
}

-- =============================================================================
-- UIManager tracking wrapper
-- =============================================================================
-- commonrequire gives us the real UIManager. We wrap show/close to track calls
-- while keeping the real behavior available.

local UIManager -- will be set in setup_complete

local function wrapUIManager()
    UIManager = require("ui/uimanager")

    -- Replace show/close with tracking-only wrappers.
    -- We do NOT call through to the real UIManager.show because it expects
    -- real widgets with handleEvent etc. Tests only need call tracking.
    UIManager.show = function(self, widget, ...)
        table.insert(UIManager._show_calls, {
            widget = widget,
            text = widget and widget.text or nil,
        })
        table.insert(UIManager._shown_widgets, widget)
    end

    UIManager.close = function(self, widget, ...)
        table.insert(UIManager._close_calls, { widget = widget })
    end

    -- Tracking arrays on UIManager (specs access UIManager._show_calls etc.)
    UIManager._show_calls = {}
    UIManager._shown_widgets = {}
    UIManager._close_calls = {}

    -- Keep other real UIManager methods available (scheduleIn, etc.)

    -- Add _reset helper for test cleanup
    UIManager._reset = function()
        UIManager._show_calls = {}
        UIManager._shown_widgets = {}
        UIManager._close_calls = {}
    end
end

-- =============================================================================
-- io.open mocker
-- =============================================================================
-- Store the real io.open (set by commonrequire or the runtime)
local _original_io_open = io.open

local function createIOOpenMocker()
    local mock_files = {}
    local IO_OPEN_FAIL = {}
    local mock_active = false

    local function install()
        if mock_active then return end
        mock_active = true
        io.open = function(path, mode)
            local mock_file = mock_files[path]
            if mock_file ~= nil then
                if mock_file == IO_OPEN_FAIL then return nil end
                return mock_file
            end
            return _original_io_open(path, mode)
        end
    end

    local function uninstall()
        if not mock_active then return end
        io.open = _original_io_open
        mock_active = false
        mock_files = {}
    end

    local function setMockFile(path, file_mock)
        mock_files[path] = file_mock
    end

    local function setMockFileFailure(path)
        mock_files[path] = IO_OPEN_FAIL
    end

    local function clear()
        mock_files = {}
    end

    return {
        install = install,
        uninstall = uninstall,
        setMockFile = setMockFile,
        setMockFileFailure = setMockFileFailure,
        clear = clear,
    }
end

-- Expose globally (matches existing spec pattern)
_G.createIOOpenMocker = createIOOpenMocker
_G._test_real_io_open = _original_io_open

-- =============================================================================
-- Module stubs for things that need Kindle-specific behavior
-- =============================================================================
-- These are installed via package.loaded to override real KOReader modules
-- where we need controlled test behavior.

local function installModuleStubs()
    -- Override lfs with our mock (has test helpers)
    package.loaded["libs/libkoreader-lfs"] = mock_lfs

    -- FIXME: ljsqlite3 mock — needs investigation
    --
    -- The koplugin-dev container has real ljsqlite3 available, but tests don't have
    -- a real Kindle cc.db. If ljsqlite3 is left available, kindle_state_reader and
    -- kindle_state_writer will attempt the SQ3 code path, which either crashes
    -- (SQ3.open creates an empty db, then fails on missing Entries table) or
    -- produces unexpected results.
    --
    -- We mock it out so the reader/writer modules fall through to their CLI
    -- (sqlite3 command / io.popen) code paths, which tests mock at a higher level.
    --
    -- This works but means we're NOT testing the SQ3 code path at all. On a real
    -- Kindle device, ljsqlite3 IS available and cc.db exists, so the SQ3 path is
    -- what actually runs in production. We should investigate:
    --   1. Creating a fixture cc.db with the Entries table so we can test SQ3 for real
    --   2. Or parameterizing the reader/writer to accept a db path override in tests
    package.loaded["lua-ljsqlite3/init"] = nil
    package.preload["lua-ljsqlite3/init"] = function()
        error("ljsqlite3 not available in tests")
    end

    -- docsettings — our specs need Kindle-specific path behavior
    package.loaded["docsettings"] = {
        getSidecarDir = function(self, doc_path, force_location)
            return doc_path .. ".sdr"
        end,
        getSidecarFilename = function(doc_path)
            return "metadata." .. doc_path:match("([^/]+)$") .. ".lua"
        end,
        getHistoryPath = function(self, doc_path)
            return "/mnt/us/koreader/history/" .. doc_path:match("([^/]+)$") .. ".lua"
        end,
    }

    -- json — use a simple mock to avoid dependencies on real json library behavior
    -- (some specs rely on the simplified encode/decode)
    package.loaded["json"] = (function()
        local json = {}
        function json.encode(data)
            if type(data) == "string" then
                return '"' .. data .. '"'
            elseif type(data) == "number" then
                return tostring(data)
            elseif type(data) == "boolean" then
                return tostring(data)
            elseif type(data) == "table" then
                local parts = {}
                for k, v in pairs(data) do
                    local key = type(k) == "number" and "" or '"' .. k .. '":'
                    if type(v) == "string" then
                        table.insert(parts, key .. '"' .. v .. '"')
                    elseif type(v) == "number" then
                        table.insert(parts, key .. tostring(v))
                    elseif type(v) == "boolean" then
                        table.insert(parts, key .. tostring(v))
                    end
                end
                return "{" .. table.concat(parts, ",") .. "}"
            end
            return "null"
        end

        function json.decode(str)
            if not str or str == "" then return nil end
            if str:match('^"(.*)"$') then
                return str:match('^"(.*)"$')
            elseif str == "true" then
                return true
            elseif str == "false" then
                return false
            elseif tonumber(str) then
                return tonumber(str)
            end
            return nil
        end

        return json
    end)()

    -- readhistory — specs need controllable history
    package.loaded["readhistory"] = {
        hist = {},
    }

    -- ui/trapper — no real trapping in tests
    package.loaded["ui/trapper"] = {
        wrap = function(self, fn) fn() end,
        info = function(self, msg) return true end,
        confirm = function(self, msg) return true end,
        clear = function(self) end,
        setPausedText = function(self) end,
        isWrapped = function(self) return false end,
    }

    -- ui/event
    package.loaded["ui/event"] = {
        new = function(self, name) return { name = name } end,
    }

    -- ui/widget/confirmbox
    package.loaded["ui/widget/confirmbox"] = {
        new = function(self, opts) return opts or {} end,
    }

    -- ui/widget/pathchooser
    package.loaded["ui/widget/pathchooser"] = {
        new = function(self, opts) return opts or {} end,
    }

    -- ui/widget/infomessage
    package.loaded["ui/widget/infomessage"] = {
        new = function(self, args)
            return {
                text = args and args.text or "",
                timeout = args and args.timeout,
            }
        end,
    }

    -- ui/widget/buttondialog
    package.loaded["ui/widget/buttondialog"] = {
        new = function(self, args)
            return {
                title = args and args.title or "",
                buttons = args and args.buttons or {},
            }
        end,
    }

    -- ui/widget/filechooser — method stubs
    package.loaded["ui/widget/filechooser"] = {
        init = function() end,
        changeToPath = function() end,
        refreshPath = function() end,
        genItemTable = function() return {} end,
        onMenuSelect = function() return false end,
        onMenuHold = function() return false end,
    }

    -- ui/widget/booklist
    package.loaded["ui/widget/booklist"] = {
        setBookInfoCacheProperty = function() end,
    }

    -- apps/reader/readerui
    package.loaded["apps/reader/readerui"] = {
        showReader = function() end,
        onClose = function() end,
        instance = {
            document = nil,
        },
    }

    -- apps/filemanager/filemanager
    package.loaded["apps/filemanager/filemanager"] = {
        updateTitleBarPath = function() end,
    }

    -- apps/filemanager/filemanagerutil
    package.loaded["apps/filemanager/filemanagerutil"] = {
        openFile = function() end,
        abbreviate = function(path) return path end,
    }

    -- apps/filemanager/filemanagerbookinfo
    package.loaded["apps/filemanager/filemanagerbookinfo"] = {}

    -- document/documentregistry
    package.loaded["document/documentregistry"] = {
        hasProvider = function() return false end,
        getProvider = function() return nil end,
        openDocument = function() return nil end,
    }

    -- document/credocument (needed by showreader_ext)
    package.loaded["document/credocument"] = {
        is_cre = true,
    }

    -- ui/renderimage
    package.loaded["ui/renderimage"] = {
        renderImageFile = function() return nil end,
    }
end

-- =============================================================================
-- Optional ljsqlite3 mock
-- =============================================================================

--- Install an in-memory ljsqlite3 mock for specs that exercise cc.db queries.
--- Other specs keep the default unavailable-module stub so state sync uses its
--- existing CLI fallback.
function M.install_sqlite_mock()
    local mock_rows = nil
    local mock_nrow = 0
    local mock_db_path = nil

    local conn_mt = {}
    conn_mt.__index = conn_mt

    function conn_mt:exec()
        if mock_rows then
            return mock_rows, mock_nrow
        end
        return nil, 0
    end

    function conn_mt:close() end

    local SQ3 = {}

    function SQ3.open(path)
        mock_db_path = path
        return setmetatable({}, conn_mt)
    end

    function SQ3._setMockResults(rows, nrow)
        mock_rows = rows
        mock_nrow = nrow or 0
    end

    function SQ3._getMockDbPath()
        return mock_db_path
    end

    function SQ3._reset()
        mock_rows = nil
        mock_nrow = 0
        mock_db_path = nil
    end

    package.loaded["lua-ljsqlite3/init"] = SQ3
    package.preload["lua-ljsqlite3/init"] = function()
        return SQ3
    end
    return SQ3
end

-- =============================================================================
-- Setup and teardown
-- =============================================================================

--- Complete setup — call once in setup() or at the top of your spec.
-- Wraps UIManager for tracking, installs module stubs, configures Kindle paths.
function M.setup_complete(opts)
    opts = opts or {}

    -- Install our module stubs (overrides real modules where needed)
    installModuleStubs()

    -- Wrap UIManager for show/close tracking
    wrapUIManager()

    -- Ensure device reports as Kindle
    local Device = require("device")
    if not Device.isKindle then
        Device.isKindle = function() return true end
    else
        -- Wrap the real isKindle to always return true
        local orig_isKindle = Device.isKindle
        Device.isKindle = function()
            return true
        end
    end

    -- Override DataStorage to return Kindle paths
    local DataStorage = require("datastorage")
    DataStorage.getFullDataDir = function()
        return "/mnt/us/koreader"
    end
    DataStorage.getDocSettingsDir = function()
        return "/mnt/us/koreader/docsettings"
    end
    DataStorage.getSettingsDir = function()
        return "/mnt/us/koreader"
    end
    DataStorage.getHistoryDir = function()
        return "/mnt/us/koreader/history"
    end

    -- Set device home_dir
    Device.home_dir = "/mnt/us"
end

--- Reset state between tests — call in before_each().
-- Clears all tracked state, resets mocks.
function M.before_each()
    M.reset_state()
    -- FIXME: Keep ljsqlite3 mocked out across test resets.
    -- Specs may re-cache the real module via require() during their runs.
    -- See the FIXME in installModuleStubs() for the full explanation.
    package.loaded["lua-ljsqlite3/init"] = nil
end

--- Reset all tracked state
function M.reset_state()
    -- Reset UIManager tracking arrays
    local ok, ui = pcall(require, "ui/uimanager")
    if ok then
        if ui._show_calls then ui._show_calls = {} end
        if ui._shown_widgets then ui._shown_widgets = {} end
        if ui._close_calls then ui._close_calls = {} end
    end

    -- Reset G_reader_settings
    if _G.G_reader_settings then
        -- commonrequire's G_reader_settings is a real LuaSettings
        -- Clear all data
        if _G.G_reader_settings.data then
            for k in pairs(_G.G_reader_settings.data) do
                _G.G_reader_settings.data[k] = nil
            end
        end
    end

    -- Clear lfs mock state
    mock_lfs._clearFileStates()
end

--- Compatibility shim: resetAllMocks() — matches existing spec pattern.
-- Specs call _G.resetAllMocks() — redirect to our reset.
function _G.resetAllMocks()
    M.reset_state()
end

--- Convenience: get a fresh reference to the mock lfs
function M.get_lfs()
    return mock_lfs
end

--- Convenience: get UIManager (tracked wrapper)
function M.get_uimanager()
    return require("ui/uimanager")
end

return M
