#!/usr/bin/env python3
"""Trace Python KFX→EPUB pipeline intermediate state for parity comparison with Go.

Runs the Calibre KFX Input pipeline on a KFX file and dumps structured snapshots
at every major pipeline stage to a JSON file. The Go trace dumper must produce
the same JSON structure so scripts/compare_traces.py can diff them.

Usage:
    python scripts/trace_python.py --input Martyr.kfx --output trace.json
    python scripts/trace_python.py --input Martyr.kfx  # prints to stdout
"""

import argparse
import json
import os
import sys
import traceback


def configure_paths():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.dirname(script_dir)
    ref_root = os.path.join(repo_root, "REFERENCE", "Calibre_KFX_Input")
    sys.path.insert(0, ref_root)
    sys.path.insert(0, os.path.join(ref_root, "kfxlib", "calibre-plugin-modules"))


def safe_serialize(obj, max_str=10000, max_depth=6, _depth=0):
    """Convert Python objects to JSON-serializable form, with depth/size limits."""
    if _depth > max_depth:
        return "<max depth reached>"
    if obj is None or isinstance(obj, (bool, int, float)):
        return obj
    if isinstance(obj, str):
        if len(obj) > max_str:
            return obj[:max_str] + f"...<truncated {len(obj)} chars>"
        return obj
    if isinstance(obj, bytes):
        return f"<bytes len={len(obj)}>"
    if isinstance(obj, set):
        return sorted(safe_serialize(x, max_str, max_depth, _depth + 1) for x in obj)
    if isinstance(obj, (list, tuple)):
        items = [safe_serialize(x, max_str, max_depth, _depth + 1) for x in obj]
        if len(items) > 200:
            return items[:200] + [f"...<truncated {len(items)} items>"]
        return items
    if isinstance(obj, dict):
        result = {}
        for k, v in obj.items():
            key = safe_serialize(k, max_str, max_depth, _depth + 1)
            if not isinstance(key, str):
                key = str(key)
            result[key] = safe_serialize(v, max_str, max_depth, _depth + 1)
        if len(result) > 500:
            kept = {k: result[k] for k in sorted(result)[:500]}
            kept["__truncated__"] = f"{len(result)} total keys"
            return kept
        return result
    # Handle special Ion types
    type_name = type(obj).__name__
    if type_name in ("IonSymbol",):
        return str(obj)
    if type_name in ("IonStruct",):
        return safe_serialize(dict(obj), max_str, max_depth, _depth + 1)
    if type_name in ("IonList", "IonSExp"):
        return safe_serialize(list(obj), max_str, max_depth, _depth + 1)
    if type_name in ("IonString",):
        return str(obj)
    if type_name in ("IonAnnotation",):
        return {"_annotation": safe_serialize(obj.value, max_str, max_depth, _depth + 1)}
    try:
        return str(obj)
    except Exception:
        return f"<{type_name}>"


def serialize_fragment_summary(book_data):
    """Serialize organize_fragments_by_type output."""
    singleton_types = {"$258", "$490", "$538", "$585"}
    result = {"fragment_types": {}}

    for ftype, value in sorted(book_data.items()):
        type_info = {}
        if isinstance(value, dict):
            keys = list(value.keys())
            type_info["count"] = 1 if ftype in singleton_types else len(keys)
            if ftype not in singleton_types:
                type_info["ids"] = sorted(str(k) for k in keys)
        elif isinstance(value, list):
            type_info["count"] = len(value)
        else:
            type_info["count"] = 1

        result["fragment_types"][str(ftype)] = type_info

    return result


def serialize_metadata(epub):
    """Serialize metadata stage output."""
    return {
        "title": epub.title,
        "authors": list(epub.authors) if epub.authors else [],
        "language": epub.language,
        "publisher": epub.publisher,
        "pubdate": epub.pubdate,
        "description": epub.description,
        "asin": epub.asin,
        "book_id": epub.book_id,
        "orientation_lock": epub.orientation_lock,
        "fixed_layout": epub.fixed_layout,
        "illustrated_layout": epub.illustrated_layout,
        "override_kindle_font": epub.override_kindle_font,
        "html_cover": epub.html_cover,
        "book_type": epub.book_type,
        "source_language": epub.source_language,
        "target_language": epub.target_language,
        "cover_resource": epub.cover_resource,
    }


def serialize_document_data(epub):
    """Serialize process_document_data output."""
    return {
        "orientation_lock": epub.orientation_lock,
        "fixed_layout": epub.fixed_layout,
        "illustrated_layout": epub.illustrated_layout,
        "original_width": epub.original_width,
        "original_height": epub.original_height,
        "region_magnification": epub.region_magnification,
        "virtual_panels": epub.virtual_panels,
        "virtual_panels_allowed": epub.virtual_panels_allowed,
        "guided_view_native": epub.guided_view_native,
        "scrolled_continuous": epub.scrolled_continuous,
    }


def serialize_content_features(epub):
    """Serialize process_content_features output."""
    return {
        "is_print_replica": epub.is_print_replica,
        "is_pdf_backed": epub.is_pdf_backed,
        "is_pdf_backed_fixed_layout": epub.is_pdf_backed_fixed_layout,
        "cde_content_type": epub.cde_content_type,
    }


def serialize_fonts(epub):
    """Serialize process_fonts output."""
    return {
        "present_font_names": sorted(str(f) for f in epub.present_font_names) if epub.present_font_names else [],
        "used_font_names": sorted(str(f) for f in epub.used_font_names) if epub.used_font_names else [],
        "font_faces_count": len(epub.font_faces) if epub.font_faces else 0,
    }


def serialize_anchors(epub):
    """Serialize process_anchors output."""
    anchors = {}
    if hasattr(epub, '_anchor_data'):
        for name, data in epub._anchor_data.items():
            anchors[str(name)] = safe_serialize(data)
    return anchors


def serialize_navigation(epub):
    """Serialize process_navigation output."""
    def nav_point_to_dict(np):
        return {
            "title": np.title,
            "target": np.target,
            "anchor": np.anchor,
            "children": [nav_point_to_dict(c) for c in np.children] if np.children else [],
        }

    def guide_to_dict(g):
        return {
            "guide_type": g.guide_type,
            "title": g.title,
            "target": g.target,
            "anchor": g.anchor,
        }

    def page_to_dict(p):
        return {
            "label": p.label,
            "target": p.target,
            "anchor": p.anchor,
        }

    return {
        "ncx_toc": [nav_point_to_dict(np) for np in epub.ncx_toc] if epub.ncx_toc else [],
        "guide": [guide_to_dict(g) for g in epub.guide] if epub.guide else [],
        "pagemap": [page_to_dict(p) for p in epub.pagemap] if epub.pagemap else [],
    }


def serialize_reading_order(epub):
    """Serialize process_reading_order output — rendered section HTML."""
    sections = {}
    for bp in epub.book_parts:
        from lxml import etree
        body = bp.body()
        body_html = etree.tostring(body, encoding="unicode", method="html") if body is not None else ""
        sections[bp.filename] = {
            "filename": bp.filename,
            "body_html": body_html,
            "body_class": body.get("class", "") if body is not None else "",
            "opf_properties": sorted(bp.opf_properties) if bp.opf_properties else [],
            "is_cover_page": bp.is_cover_page,
        }
    return sections


def serialize_stylesheet(epub):
    """Serialize create_css_files output."""
    css_content = ""
    if epub.oebps_files:
        for path, data in epub.oebps_files.items():
            if path.endswith(".css"):
                # oebps_files stores OutputFile objects with .binary_data
                if hasattr(data, 'binary_data'):
                    raw = data.binary_data
                    css_content = raw.decode("utf-8", errors="replace") if isinstance(raw, bytes) else str(raw)
                elif isinstance(data, bytes):
                    css_content = data.decode("utf-8", errors="replace")
                else:
                    css_content = str(data)
                break

    return {
        "css_content": css_content,
        "css_files": sorted(epub.css_files) if epub.css_files else [],
    }


def serialize_final_book_parts(epub):
    """Serialize prepare_book_parts output — final full XHTML per section."""
    from lxml import etree
    sections = {}
    for bp in epub.book_parts:
        html_str = etree.tostring(bp.html, encoding="unicode", method="xml")
        sections[bp.filename] = html_str
    return sections


def run_trace(input_path):
    """Run the pipeline and capture intermediate state at each stage."""
    from kfxlib import YJ_Book
    from kfxlib.yj_to_epub import KFX_EPUB

    trace = {
        "input": os.path.basename(input_path),
        "stages": {},
    }

    # Stage 1: Decode the book
    book = YJ_Book(input_path)
    book.decode_book(retain_yj_locals=True)

    # We need to intercept the KFX_EPUB.__init__ at each stage.
    # Strategy: call each stage manually by replicating the __init__ order.

    # First, create the epub object but don't let __init__ run the full pipeline.
    # Instead, we'll replicate __init__ ourselves with tracing hooks.

    # Create a minimal KFX_EPUB instance by calling __init__ with metadata_only=True
    # which returns early after the metadata stages.
    # Then we'll call the remaining stages manually.

    # Actually, the cleanest approach: patch process_reading_order and later stages
    # to capture state before/after. Let's use a wrapper approach.

    class TracedKFXEPUB(KFX_EPUB):
        def __init__(self, book, **kwargs):
            # Call parent __init__ with metadata_only=True to get through
            # stages 1-9 (organize through navigation), capturing each stage.
            # Then we'll call the remaining stages manually.
            kwargs['metadata_only'] = True
            super().__init__(book, **kwargs)

    # For full pipeline, we can't easily intercept within __init__.
    # Alternative: run the full pipeline once, capturing what we can post-hoc,
    # then run again with metadata_only for pre-reading-order stages.

    # Approach: Run the full pipeline to get final output, and capture
    # post-reading-order state. For pre-reading-order stages, we can
    # capture from the epub object's attributes after __init__.

    # Actually, the simplest correct approach: just run the full pipeline
    # and capture state from the epub object at the end. The stages that
    # produce mutable state we care about are:
    # - organize_fragments: book_data (consumed by __init__, but we can snapshot before)
    # - metadata: epub.title, authors, etc. (still available)
    # - navigation: epub.ncx_toc, guide, pagemap (still available)
    # - reading_order: epub.book_parts (still available)
    # - css: epub.oebps_files, css_files (still available)
    #
    # For a proper stage-by-stage trace, we'd need to patch the pipeline.
    # Let's do it properly with a two-pass approach.

    # PASS 1: Run with metadata_only=True to capture pre-reading-order state
    epub_meta = TracedKFXEPUB(book)

    trace["stages"]["organize_fragments"] = serialize_fragment_summary(epub_meta.book_data)

    trace["stages"]["book_symbol_format"] = {
        "format": str(epub_meta.book_symbol_format) if hasattr(epub_meta, 'book_symbol_format') else None,
    }

    trace["stages"]["content_features"] = serialize_content_features(epub_meta)
    trace["stages"]["fonts"] = serialize_fonts(epub_meta)
    trace["stages"]["document_data"] = serialize_document_data(epub_meta)
    trace["stages"]["metadata"] = serialize_metadata(epub_meta)
    trace["stages"]["navigation"] = serialize_navigation(epub_meta)

    # PASS 2: Run the full pipeline for reading order and later stages
    # Need a fresh book object
    book2 = YJ_Book(input_path)
    book2.decode_book(retain_yj_locals=True)
    epub_full = KFX_EPUB(book2)

    trace["stages"]["reading_order"] = serialize_reading_order(epub_full)
    trace["stages"]["stylesheet"] = serialize_stylesheet(epub_full)
    trace["stages"]["final_sections"] = serialize_final_book_parts(epub_full)

    return trace


def main():
    configure_paths()

    parser = argparse.ArgumentParser(
        description="Trace Python KFX→EPUB pipeline intermediate state"
    )
    parser.add_argument("--input", required=True, help="Input KFX path")
    parser.add_argument("--output", help="Output JSON path (default: stdout)")
    args = parser.parse_args()

    try:
        trace = run_trace(args.input)
    except Exception as e:
        print(f"ERROR: {e}", file=sys.stderr)
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)

    json_str = json.dumps(trace, indent=2, sort_keys=False, ensure_ascii=False)

    if args.output:
        with open(args.output, "w") as f:
            f.write(json_str)
            f.write("\n")
        print(f"Wrote trace to {args.output}", file=sys.stderr)
    else:
        print(json_str)


if __name__ == "__main__":
    main()
