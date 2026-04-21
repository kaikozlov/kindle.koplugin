-- Test helper module that provides mock dependencies for all test files
-- Adapted from kobo.koplugin/spec/helper.lua
-- This module sets up package.preload mocks before any tests require actual modules

-- Remove luarocks searcher to prevent it from trying to load packages
if package.searchers ~= nil then
    for i = #package.searchers, 1, -1 do
        local searcher_info = debug.getinfo(package.searchers[i], "S")
        if searcher_info and searcher_info.source and searcher_info.source:match("luarocks") then
            table.remove(package.searchers, i)
        end
    end
end

-- Adjust package path to find plugin modules
package.path = package.path .. ";./lua/?.lua;./lua/lib/?.lua;./?.lua"

-- Mock gettext module
if not package.preload["gettext"] then
    package.preload["gettext"] = function()
        return function(text)
            return text
        end
    end
end

-- Mock logger module
if not package.preload["logger"] then
    package.preload["logger"] = function()
        return {
            info = function(...) end,
            dbg = function(...) end,
            warn = function(...) end,
            err = function(...) end,
        }
    end
end

-- Mock datastorage module
if not package.preload["datastorage"] then
    package.preload["datastorage"] = function()
        return {
            getDocSettingsDir = function()
                return "/mnt/us/koreader/docsettings"
            end,
            getDocSettingsHashDir = function()
                return "/mnt/us/koreader/hash"
            end,
            getSettingsDir = function()
                return "/mnt/us/koreader"
            end,
            getFullDataDir = function()
                return "/mnt/us/koreader"
            end,
            getHistoryDir = function()
                return "/mnt/us/koreader/history"
            end,
        }
    end
end

-- Mock G_reader_settings global
if not _G.G_reader_settings then
    _G.G_reader_settings = {
        _settings = {},
        readSetting = function(self, key)
            return self._settings[key]
        end,
        saveSetting = function(self, key, value)
            self._settings[key] = value
        end,
        isTrue = function(self, key)
            return self._settings[key] == true
        end,
        flush = function(self)
            -- No-op in tests
        end,
    }
end

-- Mock ui/bidi module
if not package.preload["ui/bidi"] then
    package.preload["ui/bidi"] = function()
        return {
            filename = function(text)
                return text
            end,
            directory = function(text)
                return text
            end,
        }
    end
end

-- Mock device module
if not package.preload["device"] then
    package.preload["device"] = function()
        local Device = {
            home_dir = "/mnt/us",
        }
        function Device:isKindle()
            return true
        end
        return Device
    end
end

-- Mock util module
if not package.preload["util"] then
    package.preload["util"] = function()
        local util = {}

        function util.shell_escape(args)
            if type(args) == "table" then
                local parts = {}
                for _, arg in ipairs(args) do
                    table.insert(parts, "'" .. tostring(arg):gsub("'", "'\\''") .. "'")
                end
                return table.concat(parts, " ")
            end
            return "'" .. tostring(args):gsub("'", "'\\''") .. "'"
        end

        function util.getFriendlySize(size)
            return tostring(size) .. " B"
        end

        function util.splitFilePathName(filepath)
            if not filepath or type(filepath) ~= "string" then
                return "", ""
            end
            local directory = filepath:match("(.*/)")
            if directory then
                directory = directory:sub(1, -2)
                local filename = filepath:sub(#directory + 2)
                return directory, filename
            end
            return "", filepath
        end

        function util.partialMD5(filepath)
            if not filepath then return nil end
            return "a1b2c3d4e5f6"
        end

        function util.tableDeepCopy(orig)
            local copy
            if type(orig) == "table" then
                copy = {}
                for k, v in pairs(orig) do
                    copy[k] = type(v) == "table" and util.tableDeepCopy(v) or v
                end
            else
                copy = orig
            end
            return copy
        end

        return util
    end
end

-- Mock json module
if not package.preload["json"] then
    package.preload["json"] = function()
        local json = {}

        function json.encode(data)
            if type(data) == "string" then
                return '"' .. data .. '"'
            elseif type(data) == "number" then
                return tostring(data)
            elseif type(data) == "boolean" then
                return tostring(data)
            elseif type(data) == "table" then
                -- Simple table encoding for test purposes
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
            -- For tests, we'll use a simple decoder
            -- Real tests should mock at a higher level when complex JSON is needed
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
    end
end

-- Mock libs/libkoreader-lfs module
if not package.preload["libs/libkoreader-lfs"] then
    package.preload["libs/libkoreader-lfs"] = function()
        local file_states = {}
        local directory_contents = {}

        local lfs = {
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

        return lfs
    end
end

-- Mock ffi/util module
if not package.preload["ffi/util"] then
    package.preload["ffi/util"] = function()
        return {
            template = function(template_str, ...)
                local args = { ... }
                if #args == 0 then return template_str end
                local result = template_str
                for i, value in ipairs(args) do
                    result = result:gsub("%%(" .. i .. ")", tostring(value))
                end
                return result
            end,
        }
    end
end

-- Mock ui/widget/container/widgetcontainer module
if not package.preload["ui/widget/container/widgetcontainer"] then
    package.preload["ui/widget/container/widgetcontainer"] = function()
        local WidgetContainer = {}
        function WidgetContainer:extend(subclass)
            local o = subclass or {}
            setmetatable(o, self)
            self.__index = self
            return o
        end
        function WidgetContainer:new(opts)
            local o = opts or {}
            setmetatable(o, self)
            self.__index = self
            return o
        end
        return WidgetContainer
    end
end

-- Mock ui/uimanager module
if not package.preload["ui/uimanager"] then
    package.preload["ui/uimanager"] = function()
        local UIManager = {
            _show_calls = {},
            _shown_widgets = {},
            _close_calls = {},
        }

        function UIManager:show(widget)
            table.insert(self._show_calls, { widget = widget, text = widget and widget.text or nil })
            table.insert(self._shown_widgets, widget)
        end

        function UIManager:close(widget)
            table.insert(self._close_calls, { widget = widget })
        end

        function UIManager:forceRePaint() end

        function UIManager:scheduleIn(time, callback)
            -- Execute immediately in tests
            callback()
        end

        function UIManager:_reset()
            self._show_calls = {}
            self._shown_widgets = {}
            self._close_calls = {}
        end

        return UIManager
    end
end

-- Mock ui/widget/infomessage module
if not package.preload["ui/widget/infomessage"] then
    package.preload["ui/widget/infomessage"] = function()
        local InfoMessage = {}
        function InfoMessage:new(args)
            return {
                text = args and args.text or "",
                timeout = args and args.timeout,
            }
        end
        return InfoMessage
    end
end

-- Mock ui/widget/buttondialog module
if not package.preload["ui/widget/buttondialog"] then
    package.preload["ui/widget/buttondialog"] = function()
        local ButtonDialog = {}
        function ButtonDialog:new(args)
            return {
                title = args and args.title or "",
                buttons = args and args.buttons or {},
            }
        end
        return ButtonDialog
    end
end

-- Mock ui/widget/filechooser module
if not package.preload["ui/widget/filechooser"] then
    package.preload["ui/widget/filechooser"] = function()
        return {
            init = function() end,
            changeToPath = function() end,
            refreshPath = function() end,
            genItemTable = function() return {} end,
            onMenuSelect = function() return false end,
            onMenuHold = function() return false end,
        }
    end
end

-- Mock apps/reader/readerui module (required by readerui_ext, showreader_ext)
if not package.preload["apps/reader/readerui"] then
    package.preload["apps/reader/readerui"] = function()
        return {
            showReader = function() end,
            onClose = function() end,
            instance = {
                document = nil,
            },
        }
    end
end

-- Mock document/documentregistry module
if not package.preload["document/documentregistry"] then
    package.preload["document/documentregistry"] = function()
        return {
            hasProvider = function() return false end,
            getProvider = function() return nil end,
            openDocument = function() return nil end,
        }
    end
end

-- Mock apps/filemanager/filemanager module
if not package.preload["apps/filemanager/filemanager"] then
    package.preload["apps/filemanager/filemanager"] = function()
        return {
            updateTitleBarPath = function() end,
        }
    end
end

-- Mock apps/filemanager/filemanagerutil module
if not package.preload["apps/filemanager/filemanagerutil"] then
    package.preload["apps/filemanager/filemanagerutil"] = function()
        return {
            openFile = function() end,
            abbreviate = function(path) return path end,
        }
    end
end

-- Mock docsettings module
if not package.preload["docsettings"] then
    package.preload["docsettings"] = function()
        return {
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
    end
end

-- Mock ui/widget/booklist module
if not package.preload["ui/widget/booklist"] then
    package.preload["ui/widget/booklist"] = function()
        return {
            setBookInfoCacheProperty = function() end,
        }
    end
end

-- Mock frontend/util module
if not package.preload["frontend/util"] then
    package.preload["frontend/util"] = function()
        return {
            makePath = function(path)
                -- No-op in tests, just pretend success
            end,
        }
    end
end

-- Mock readhistory
if not package.preload["readhistory"] then
    package.preload["readhistory"] = function()
        return {
            hist = {},
        }
    end
end

-- Mock ui/trapper
if not package.preload["ui/trapper"] then
    package.preload["ui/trapper"] = function()
        return {
            wrap = function(self, fn) fn() end,
            info = function(self, msg) return true end,
            confirm = function(self, msg) return true end,
            clear = function(self) end,
            setPausedText = function(self) end,
            isWrapped = function(self) return false end,
        }
    end
end

-- Mock ui/widget/confirmbox
if not package.preload["ui/widget/confirmbox"] then
    package.preload["ui/widget/confirmbox"] = function()
        return {
            new = function(self, opts) return opts end,
        }
    end
end

-- Mock ui/event
if not package.preload["ui/event"] then
    package.preload["ui/event"] = function()
        return {
            new = function(self, name) return { name = name } end,
        }
    end
end

-- Store original io.open for io.open mocker
local _original_io_open
if not _G._test_real_io_open then
    _original_io_open = io.open
    _G._test_real_io_open = _original_io_open
else
    _original_io_open = _G._test_real_io_open
end

-- Helper function to create localized io.open mocks
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

-- Expose globally for test files
_G.createIOOpenMocker = createIOOpenMocker

-- Reset helper for before_each
local function resetAllMocks()
    _G.G_reader_settings._settings = {}
    local ok, UIManager = pcall(require, "ui/uimanager")
    if ok and UIManager._reset then
        UIManager:_reset()
    end
end
_G.resetAllMocks = resetAllMocks

-- Mock ui/widget/confirmbox module (required by main.lua top-level)
if not package.preload["ui/widget/confirmbox"] then
    package.preload["ui/widget/confirmbox"] = function()
        return {
            new = function(self, opts)
                return opts or {}
            end,
        }
    end
end

-- Mock ui/widget/pathchooser module (required by main.lua top-level)
if not package.preload["ui/widget/pathchooser"] then
    package.preload["ui/widget/pathchooser"] = function()
        return {
            new = function(self, opts)
                return opts or {}
            end,
        }
    end
end

-- Mock apps/filemanager/filemanagerbookinfo (required by bookinfomanager_ext)
if not package.preload["apps/filemanager/filemanagerbookinfo"] then
    package.preload["apps/filemanager/filemanagerbookinfo"] = function()
        return {}
    end
end

-- Mock ui/renderimage (required by bookinfomanager_ext)
if not package.preload["ui/renderimage"] then
    package.preload["ui/renderimage"] = function()
        return {
            renderImageFile = function() return nil end,
        }
    end
end
