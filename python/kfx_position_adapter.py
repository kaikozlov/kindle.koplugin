"""Plugin-owned KFX position metadata adapter.

The vendored :mod:`kfxlib` tree is intentionally kept pristine.  Exact Kindle
position sync needs two text-free coordinates on generated EPUB elements,
though: the KFX entity id and that entity's base position id.  This module
adds those attributes at runtime by temporarily wrapping kfxlib's public
conversion class, then restores the class immediately after conversion.

No KFX parsing or conversion logic is reimplemented here; kfxlib remains the
source of truth for both the EPUB and content-position records.
"""

from contextlib import contextmanager

from kfx_position_map import tag_position_element, unique_eid_base_pids


def _position_bases(book):
    return unique_eid_base_pids(book.collect_content_position_info())


@contextmanager
def position_metadata_conversion(epub_class=None):
    """Temporarily annotate generated EPUB elements with KFX coordinates.

    ``YJ_Book.convert_to_epub`` imports ``KFX_EPUB`` after decoding the book,
    so wrapping its constructor here observes the same decoded book that
    upstream conversion uses.  The wrapper only adds ``data-kfx-*`` metadata;
    all structural/content conversion remains in the unmodified vendored
    implementation.
    """

    if epub_class is None:
        from kfxlib.yj_to_epub import KFX_EPUB as epub_class

    original_init = epub_class.__init__
    original_process_position = epub_class.process_position

    def wrapped_init(self, book, *args, **kwargs):
        self._kindle_position_bases = _position_bases(book)
        return original_init(self, book, *args, **kwargs)

    def wrapped_process_position(self, eid, offset, elem):
        if offset == 0:
            tag_position_element(elem, eid, getattr(self, "_kindle_position_bases", {}))
        return original_process_position(self, eid, offset, elem)

    epub_class.__init__ = wrapped_init
    epub_class.process_position = wrapped_process_position
    try:
        yield
    finally:
        epub_class.__init__ = original_init
        epub_class.process_position = original_process_position
