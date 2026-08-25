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
]


def fixture(name: str) -> tuple[str, str, str]:
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
    else:
        raise ValueError(name)

    opf = f"""<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:kp3-{name}</dc:identifier>
    <dc:title>KAF Probe</dc:title>
    <dc:language>{language}</dc:language>
    {metadata}
  </metadata>
  <manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="chapter"/></spine>
</package>
"""
    xhtml = f"""<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="{language}" lang="{language}">
<head><title>Probe</title></head><body>{body}</body></html>
"""
    return opf, xhtml, language


def write_epub(name: str, output: Path) -> None:
    opf, xhtml, _ = fixture(name)
    output.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(output, "w") as zf:
        zf.writestr("mimetype", "application/epub+zip", compress_type=zipfile.ZIP_STORED)
        zf.writestr("META-INF/container.xml", CONTAINER_XML, compress_type=zipfile.ZIP_DEFLATED)
        zf.writestr("OEBPS/content.opf", opf, compress_type=zipfile.ZIP_DEFLATED)
        zf.writestr("OEBPS/chapter.xhtml", xhtml, compress_type=zipfile.ZIP_DEFLATED)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("fixture", choices=FIXTURE_NAMES)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    write_epub(args.fixture, args.output)
    print(args.output)


if __name__ == "__main__":
    main()
