#!/usr/bin/env python3
"""Generate tiny, controlled EPUB fixtures for Kindle Previewer semantic probes."""

from __future__ import annotations

import argparse
from pathlib import Path
import zipfile


CONTAINER_XML = """<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>
"""


FIXTURE_NAMES = [
    "minimal",
    "footnote",
    "table",
    "fixed-layout",
    "vertical-ruby",
    "link",
    "bidi",
    "list",
    "svg",
    "dropcap",
    "image-figure",
    "first-line",
]


def probe_png() -> bytes:
    """Deterministic 60x40 RGB gradient PNG for the image-figure fixture."""
    import zlib

    width, height = 60, 40
    rows = bytearray()
    for y in range(height):
        rows.append(0)  # PNG filter type 0 (None)
        for x in range(width):
            rows += bytes((int(255 * x / (width - 1)), int(255 * y / (height - 1)), 128))

    def chunk(tag: bytes, payload: bytes) -> bytes:
        data = tag + payload
        return len(payload).to_bytes(4, "big") + data + zlib.crc32(data).to_bytes(4, "big")

    ihdr = width.to_bytes(4, "big") + height.to_bytes(4, "big") + bytes((8, 2, 0, 0, 0))
    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", ihdr)
        + chunk(b"IDAT", zlib.compress(bytes(rows), 9))
        + chunk(b"IEND", b"")
    )


def fixture(name: str) -> tuple[str, str, str, dict[str, bytes]]:
    """Return (opf, xhtml, language, extra_files) for a fixture name."""
    if name == "minimal":
        metadata = ""
        language = "en"
        body = "<h1>Hello</h1><p>KFX probe.</p>"
    elif name == "footnote":
        metadata = ""
        language = "en"
        body = (
            '<p>Main text<a epub:type="noteref" href="#fn1">1</a>.</p>'
            '<aside epub:type="footnote" id="fn1"><p>Footnote text.</p></aside>'
        )
    elif name == "table":
        metadata = ""
        language = "en"
        body = (
            '<table id="probe-table">'
            '<caption>Probe table</caption>'
            '<thead><tr><th>H1</th><th>H2</th></tr></thead>'
            '<tbody>'
            '<tr><td rowspan="2">A</td><td>B</td></tr>'
            '<tr><td>C</td></tr>'
            '<tr><td colspan="2">D</td></tr>'
            '</tbody></table>'
        )
    elif name == "fixed-layout":
        metadata = (
            '<meta property="rendition:layout">pre-paginated</meta>'
            '<meta name="fixed-layout" content="true"/>'
            '<meta name="original-resolution" content="600x800"/>'
        )
        language = "en"
        body = (
            '<div style="position:absolute; left:60px; top:80px; width:240px; height:160px; '
            'border:2px solid black">Fixed-layout region</div>'
        )
    elif name == "vertical-ruby":
        metadata = '<meta name="primary-writing-mode" content="vertical-rl"/>'
        language = "ja"
        body = (
            '<h1>見出し</h1>'
            '<p><ruby style="-webkit-ruby-position: over"><rb>漢</rb><rt>かん</rt></ruby>字と'
            '<span style="-webkit-text-emphasis-style: filled dot">強調</span>。</p>'
        )
    elif name == "link":
        metadata = ""
        language = "en"
        body = (
            '<p><a href="#target">Jump to target</a>.</p>'
            '<h2 id="target">Target heading</h2><p>Destination.</p>'
        )
    elif name == "bidi":
        metadata = ""
        language = "ar"
        body = (
            '<p dir="rtl" style="direction: rtl">مرحبا بالعالم '
            '<span dir="ltr" style="direction: ltr; unicode-bidi: isolate">ABC 123</span></p>'
        )
    elif name == "list":
        metadata = ""
        language = "en"
        body = (
            '<ol start="3"><li>Three</li><li>Four<ul><li>Nested</li></ul></li></ol>'
        )
    elif name == "svg":
        metadata = ""
        language = "en"
        body = (
            '<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80" viewBox="0 0 120 80">'
            '<rect x="5" y="5" width="110" height="70" fill="none" stroke="black"/>'
            '<circle cx="30" cy="40" r="12" fill="black"/>'
            '<text x="52" y="45">SVG probe</text></svg>'
        )
    elif name == "dropcap":
        # Canonical Previewer drop-cap source form: a block whose first inline
        # child is floated left with an enlarged font. The PhantomJS preprocessing
        # pass measures cap height vs paragraph line height, rewrites the source
        # to dropcap_lines/dropcap_chars attributes plus a synthetic inline span,
        # and the paragraph adapter emits $125/$126 style properties (see
        # coreprocessor.js DropCap handling and C3940e.o reading the
        # dropcap_lines/dropcap_chars attributes). Geometry below yields
        # dropcap_lines=4, dropcap_chars=1.
        metadata = ""
        language = "en"
        body = (
            '<h1>Drop cap probe</h1>'
            '<p style="font-size: 12pt; line-height: 14pt; margin: 0;">'
            '<span style="float: left; font-size: 42pt; line-height: 42pt;">T</span>'
            'he opening paragraph carries a floated initial so the preprocessor '
            'recognizes a drop cap and records its measured line span.</p>'
            '<p>A following paragraph in the default style.</p>'
        )
    elif name == "image-figure":
        # Ordinary reflowable raster image behavior: figure + img + figcaption,
        # distinct from the fixed-layout fixture. The producer carries the PNG
        # resource through untouched (res/rsrcN), which exercises PNG image
        # handling on the consumer side (all real-book fixtures are JPEG).
        metadata = ""
        language = "en"
        body = (
            '<h1>Image probe</h1>'
            '<figure>'
            '<img src="images/probe.png" alt="Probe gradient" style="width: 50%;"/>'
            '<figcaption>Probe figure caption.</figcaption>'
            '</figure>'
            '<p>Text after the figure.</p>'
        )
    elif name == "first-line":
        # ::first-line and ::first-letter pseudo-element semantics. The
        # preprocessor materializes ::first-line as a data-first-line-style
        # attribute (adapter -> $622 yj.n_style) and ::first-letter as an inline
        # span with relativized styles (-> $142 style events).
        metadata = ""
        language = "en"
        body = (
            '<p class="firstline">The first line of this paragraph should render in '
            'small capitals with letter spacing, while subsequent lines return to the '
            'normal inherited style.</p>'
            '<p class="opening">A paragraph whose initial letter is styled large and '
            'bold through the first-letter pseudo element selector.</p>'
        )
    else:
        raise ValueError(name)

    manifest_items = '<item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/>'
    extra_files: dict[str, bytes] = {}
    if name == "image-figure":
        manifest_items += '<item id="probe-img" href="images/probe.png" media-type="image/png"/>'
        extra_files["OEBPS/images/probe.png"] = probe_png()

    opf = f"""<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:kp3-{name}</dc:identifier>
    <dc:title>KAF Probe</dc:title>
    <dc:language>{language}</dc:language>
    {metadata}
  </metadata>
  <manifest>{manifest_items}</manifest>
  <spine><itemref idref="chapter"/></spine>
</package>
"""
    head_extra = ""
    if name == "first-line":
        head_extra = (
            '<style type="text/css">\n'
            'p.firstline::first-line { font-variant: small-caps; letter-spacing: 1pt; }\n'
            'p.opening::first-letter { font-size: 24pt; font-weight: bold; }\n'
            '</style>'
        )
    xhtml = f"""<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="{language}" lang="{language}">
<head><title>Probe</title>{head_extra}</head><body>{body}</body></html>
"""
    return opf, xhtml, language, extra_files


def write_epub(name: str, output: Path) -> None:
    opf, xhtml, _, extra_files = fixture(name)
    output.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(output, "w") as zf:
        zf.writestr("mimetype", "application/epub+zip", compress_type=zipfile.ZIP_STORED)
        zf.writestr("META-INF/container.xml", CONTAINER_XML, compress_type=zipfile.ZIP_DEFLATED)
        zf.writestr("OEBPS/content.opf", opf, compress_type=zipfile.ZIP_DEFLATED)
        zf.writestr("OEBPS/chapter.xhtml", xhtml, compress_type=zipfile.ZIP_DEFLATED)
        for path, data in extra_files.items():
            zf.writestr(path, data, compress_type=zipfile.ZIP_DEFLATED)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("fixture", choices=FIXTURE_NAMES)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    write_epub(args.fixture, args.output)
    print(args.output)


if __name__ == "__main__":
    main()
