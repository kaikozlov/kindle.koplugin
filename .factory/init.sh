#!/bin/bash
# Mission init script — idempotent
cd /home/kai/gitrepos/kobo.koplugin/kindle.koplugin
go build ./internal/kfx/... 2>/dev/null
exit 0
