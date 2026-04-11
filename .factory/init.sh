#!/bin/bash
# Mission init script — idempotent
cd /home/kai/gitrepos/kobo.koplugin/kindle.koplugin
go build ./internal/kfx/... 2>/dev/null

# Generate reference EPUBs if missing
if [ ! -f /tmp/martyr_python_ref.epub ]; then
  python3 scripts/convert_kfx_python.py --input ../REFERENCE/kfx_examples/Martyr_5AFAFAA13FFE43ECBE78F0FF3761814C.kfx --output /tmp/martyr_python_ref.epub 2>/dev/null
fi
if [ ! -f "/tmp/Elvis and the Underdogs_B009NG3090_calibre.epub" ]; then
  python3 scripts/convert_kfx_python.py --input "../REFERENCE/KFX_DRM/decrypted/Elvis and the Underdogs_B009NG3090_decrypted.kfx-zip" --output "/tmp/Elvis and the Underdogs_B009NG3090_calibre.epub" 2>/dev/null
fi
if [ ! -f "/tmp/The Hunger Games Trilogy_B004XJRQUQ_calibre.epub" ]; then
  python3 scripts/convert_kfx_python.py --input "../REFERENCE/KFX_DRM/decrypted/The Hunger Games Trilogy_B004XJRQUQ_decrypted.kfx-zip" --output "/tmp/The Hunger Games Trilogy_B004XJRQUQ_calibre.epub" 2>/dev/null
fi
if [ ! -f "/tmp/Three Below (Floors #2)_B008PL1YQ0_calibre.epub" ]; then
  python3 scripts/convert_kfx_python.py --input "../REFERENCE/KFX_DRM/decrypted/Three Below (Floors #2)_B008PL1YQ0_decrypted.kfx-zip" --output "/tmp/Three Below (Floors #2)_B008PL1YQ0_calibre.epub" 2>/dev/null
fi
exit 0
