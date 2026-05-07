-- Luacheck config for kindle.koplugin
-- Target: LuaJIT (Lua 5.1) as used by KOReader

std = "luajit"

-- unused args are normal in KOReader plugin callbacks
unused_args = false

-- ignore implicit self
self = false

-- KOReader-provided globals
globals = {
    "G_reader_settings",
    "G_defaults",
    "einkfb",
    "table.pack",
    "table.unpack",
}

read_globals = {
    "_ENV",
}

-- REFERENCE/, build/, dot-dirs are not project source
exclude_files = {
    "REFERENCE/",
    "build/",
    ".luarocks/",
    ".luajitrocks/",
    "python/",
}

-- spec files use busted + our custom helpers
files["spec/"].std = "+busted"
files["spec/"].globals = {
    "match",
    "package",
    "TEST_DATA_DIR",
    "PLUGIN_PATH",
    "createIOOpenMocker",
    "resetAllMocks",
    "_test_real_io_open",
    "load_plugin",
    "disable_plugins",
    "fastforward_ui_events",
    "get_test_data_dir",
    "get_plugin_path",
}

-- 211 - unused local starting with __ (placeholder)
-- 231 - unused __ self vararg
-- 631 - line too long
ignore = {
    "211/__*",
    "231/__",
    "631",
}
