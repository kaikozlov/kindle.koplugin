import collections
import datetime
import io
import re
import zipfile

from .message_logging import log
from .resources import (
    combine_image_tiles, convert_image_to_pdf, convert_jxr_to_jpeg_or_png, convert_pdf_page_to_image,
    crop_image, ImageResource, PdfImageResource, pypdf, SYMBOL_FORMATS)
from .utilities import (json_serialize_compact, list_counts)
from .yj_to_epub import KFX_EPUB

__license__ = "GPL v3"
__copyright__ = "2016-2025, John Howell <jhowell@acm.org>"

USE_HIGHEST_RESOLUTION_IMAGE_VARIANT = True
DEBUG_VARIANTS = False


class KFX_IMAGE_BOOK(object):
    def __init__(self, book):
        self.book = book

    def convert_book_to_cbz(self, split_landscape_comic_images, progress):
        kfx_epub = KFX_EPUB(self.book, metadata_only=True)
        is_rtl = kfx_epub.page_progression_direction == "rtl"
        ordered_images = self.get_ordered_images(split_landscape_comic_images, kfx_epub.is_comic, is_rtl, progress)[0]

        yj_metadata = self.book.get_yj_metadata_from_book()
        comic_book_info = {}

        for ci_name, yj_value in [
                ("title", yj_metadata.title),
                ("publisher", yj_metadata.publisher),
                ("language", yj_metadata.language),
                ("lang", yj_metadata.language),
                ("comments", yj_metadata.description)]:
            if yj_value:
                comic_book_info[ci_name] = yj_value

        if yj_metadata.authors:
            comic_book_info["credits"] = [{"person": author, "role": "Writer"} for author in yj_metadata.authors]

        if yj_metadata.issue_date:
            try:
                pubdate = datetime.date.fromisoformat(yj_metadata.issue_date)
            except Exception:
                pass
            else:
                comic_book_info["publicationMonth"] = pubdate.month
                comic_book_info["publicationYear"] = pubdate.year

        cbz_metadata = {"ComicBookInfo/1.0": comic_book_info} if comic_book_info else None
        return combine_images_into_cbz(ordered_images, cbz_metadata)

    def convert_book_to_pdf(self, split_landscape_comic_images, progress):
        kfx_epub = KFX_EPUB(self.book, metadata_only=True)
        is_rtl = kfx_epub.page_progression_direction == "rtl"
        ordered_images, ordered_image_pids, content_pos_info = self.get_ordered_images(
            split_landscape_comic_images, kfx_epub.is_comic, is_rtl, progress)

        yj_metadata = self.book.get_yj_metadata_from_book()
        current_date = datetime.datetime.now().strftime("D\072%Y%m%d%H%M%S")
        pdf_metadata = {}

        for name, value in [
                ("/Title", yj_metadata.title),
                ("/CreationDate", current_date),
                ("/ModDate", current_date)]:
            if value:
                pdf_metadata[name] = value

        if yj_metadata.authors:
            pdf_metadata["/Author"] = " & ".join(yj_metadata.authors)

        outline1 = []

        def add_pages_nums_to_toc(toc):
            for toc_entry in toc:
                toc_entry.page_num = None
                eid, eid_offset = kfx_epub.position_of_anchor(toc_entry.anchor)
                if eid is not None:
                    toc_pid = self.book.pid_for_eid(eid, eid_offset, content_pos_info)
                    if toc_pid is not None:
                        toc_entry.page_num = 0
                        for ordered_page_num, ordered_page_pid in enumerate(ordered_image_pids):
                            toc_entry.page_num = ordered_page_num
                            if toc_pid <= ordered_page_pid:
                                break
                    else:
                        log.error("Failed to locate pid toc_entry: label=%s, eid=%s, eid_offset=%s" % (
                            toc_entry.title, repr(eid), eid_offset))

                outline1.append((toc_entry.title, toc_entry.page_num))
                add_pages_nums_to_toc(toc_entry.children)

        add_pages_nums_to_toc(kfx_epub.ncx_toc)
        return combine_images_into_pdf(ordered_images, pdf_metadata, is_rtl, kfx_epub.ncx_toc)

    def get_ordered_images(self, split_landscape_comic_images=False, is_comic=False, is_rtl=False, progress=None):

        ordered_image_resources, ordered_image_resource_pids, content_pos_info = self.book.get_ordered_image_resources()

        if progress is not None:
            progress.set_limit(len(ordered_image_resources) + 1)

        ordered_images = []
        ordered_image_pids = []
        split_image_count = 0
        for cnt, (fid, pid) in enumerate(zip(ordered_image_resources, ordered_image_resource_pids)):
            image_resource = self.get_resource_image(fid)
            if image_resource is not None:
                if (
                        split_landscape_comic_images and image_resource is not None and is_comic and image_resource.format != "$565" and
                        image_resource.width > image_resource.height):
                    split_image_count += 1

                    new_width = image_resource.width // 2
                    left_image = crop_image(
                        image_resource.raw_media, image_resource.location, image_resource.width, image_resource.height,
                        0, new_width, 0, 0)
                    left = ImageResource(
                        image_resource.format, suffix_location(image_resource.location, "-L"), left_image, image_resource.height, new_width)

                    right_image = crop_image(
                        image_resource.raw_media, image_resource.location, image_resource.width, image_resource.height,
                        new_width, 0, 0, 0)
                    right = ImageResource(
                        image_resource.format, suffix_location(image_resource.location, "-R"), right_image, image_resource.height, new_width)

                    if not is_rtl:
                        ordered_images.append(left)
                        ordered_image_pids.append(pid)
                        image_resource = right
                    else:
                        ordered_images.append(right)
                        ordered_image_pids.append(pid)
                        image_resource = left

                ordered_images.append(image_resource)
                ordered_image_pids.append(pid)

            if progress is not None:
                progress.update_count(cnt)

        if split_image_count:
            log.warning("Split %d landscape comic images into left/right image pairs" % split_image_count)

        num_pages = self.book.get_page_count()

        if num_pages and len(ordered_images) < num_pages:
            log.warning("Expected %d pages but found only %d page images in book" % (num_pages, len(ordered_images)))

        return (ordered_images, ordered_image_pids, content_pos_info)

    def get_resource_image(self, resource_name, ignore_variants=False):
        fragment = self.book.fragments.get(ftype="$164", fid=resource_name)
        if fragment is None:
            return None

        resource = fragment.value
        resource_format = resource.get("$161")
        resource_height = resource.get("$423", None) or resource.get("$67", None)
        resource_width = resource.get("$422", None) or resource.get("$66", None)
        page_index = resource.get("$564", 0)

        if "$636" in resource:
            yj_tiles = resource.get("$636")
            tile_height = resource.get("$638")
            tile_width = resource.get("$637")
            tile_padding = resource.get("$797", 0)
            location = yj_tiles[0][0].partition("-tile")[0]

            tiles_raw_media = []
            for row in yj_tiles:
                for tile_location in row:
                    tile_raw_media_frag = self.book.fragments.get(ftype="$417", fid=tile_location)
                    tiles_raw_media.append(None if tile_raw_media_frag is None else tile_raw_media_frag.value)

            raw_media, resource_format = combine_image_tiles(
                resource_name, resource_height, resource_width, resource_format, tile_height, tile_width, tile_padding,
                yj_tiles, tiles_raw_media, ignore_variants)
        else:
            location = resource.get("$165")
            if location is not None:
                raw_media = self.book.fragments.get(ftype="$417", fid=location)
                if raw_media is not None:
                    raw_media = raw_media.value
            else:
                raw_media = None

        if resource_format != "$565" and not ignore_variants:
            for rr in resource.get("$635", []):
                variant = self.get_resource_image(rr, ignore_variants=True)

                if (USE_HIGHEST_RESOLUTION_IMAGE_VARIANT and variant is not None and
                        variant.width > resource_width and variant.height > resource_height):
                    if DEBUG_VARIANTS:
                        log.info("Replacing image %s (%dx%d) with variant %s (%dx%d)" % (
                                location, resource_width, resource_height, variant.location, variant.width, variant.height))

                    location, resource_format, raw_media, resource_width, resource_height = (
                        variant.location, variant.format, variant.raw_media, variant.width, variant.height)

        if raw_media is None:
            return None

        if resource_format == "$565":
            return PdfImageResource(location, raw_media, page_index, None)

        return ImageResource(resource_format, location, raw_media, resource_height, resource_width)


def combine_images_into_pdf(ordered_images, metadata=None, is_rtl=False, outline=None):
    if len(ordered_images) == 0:
        return None

    image_resource_formats = collections.defaultdict(set)
    combined_pdf_images = []
    for image_resource in ordered_images:
        image_resource_formats[SYMBOL_FORMATS[image_resource.format].upper()].add(image_resource.location)

        if image_resource.format == "$565":
            if combined_pdf_images and combined_pdf_images[-1].format == "$565" and combined_pdf_images[-1].location == image_resource.location:
                combined_pdf_images[-1].page_nums.extend(image_resource.page_nums)
            else:
                pdf = pypdf.PdfReader(io.BytesIO(image_resource.raw_media))
                image_resource.total_pages = len(pdf.pages)
                combined_pdf_images.append(image_resource)
        else:
            combined_pdf_images.append(convert_image_to_pdf(image_resource))

    if len(combined_pdf_images) == 1 and combined_pdf_images[0].entire_resource_used():
        combined = False
        pdf_data = combined_pdf_images[0].raw_media

        if not (metadata or is_rtl or outline):
            return pdf_data

        try:
            writer = pypdf.PdfWriter(clone_from=io.BytesIO(pdf_data))
        except Exception as e:
            log.error("pypdf PdfWriter error in clone_from %s: %s" % (combined_pdf_images[0].location, repr(e)))
            return None
    else:
        combined = True
        writer = pypdf.PdfWriter()

        for image_resource in combined_pdf_images:
            try:
                if image_resource.entire_resource_used():
                    writer.append(fileobj=io.BytesIO(image_resource.raw_media))
                else:
                    log.warning("Using PDF %s pages %s of %d" % (
                        image_resource.location, repr(image_resource.page_nums), image_resource.total_pages))

                    for page_range in image_resource.page_number_ranges():
                        writer.append(fileobj=io.BytesIO(image_resource.raw_media), pages=page_range)
            except Exception as e:
                log.error("pypdf PdfWriter error appending %s: %s" % (image_resource.location, repr(e)))
                return None

    try:
        if metadata:
            writer.add_metadata(metadata)

        if is_rtl:
            if writer.viewer_preferences is None:
                writer.create_viewer_preferences()

            writer.viewer_preferences.direction = "/R2L"

        if outline:
            if len(writer.outline) == 0:
                add_pdf_outline(writer, outline)
            else:
                log.warning("Existing PDF outline left unchanged")

        writer.compress_identical_objects(remove_identicals=True, remove_orphans=True)

        updated_file = io.BytesIO()
        writer.write(updated_file)
        pdf_data = updated_file.getvalue()
        updated_file.close()
    except Exception as e:
        log.error("pypdf error: %s" % repr(e))
        return None

    if combined:
        log.info("Combined %s resources into a %d page PDF file" % (list_counts(image_resource_formats), len(ordered_images)))

    return pdf_data


def add_pdf_outline(pdf_writer, outline_entries, parent=None):
    for outline_entry in outline_entries:
        new_entry = pdf_writer.add_outline_item(outline_entry.title, outline_entry.page_num, parent=parent)

        if outline_entry.children:
            add_pdf_outline(pdf_writer, outline_entry.children, new_entry)


def combine_images_into_cbz(ordered_images, metadata=None):
    if len(ordered_images) == 0:
        return None

    image_resource_formats = collections.defaultdict(set)
    reported_pdf_errors = set()
    page_images = []
    for image_resource in ordered_images:
        image_resource_formats[SYMBOL_FORMATS[image_resource.format].upper()].add(image_resource.location)

        if image_resource.format in {"$286", "$285", "$284"}:
            page_images.append(image_resource)
        elif image_resource.format == "$565":
            for page_num in image_resource.page_nums:
                image_data, fmt = convert_pdf_page_to_image(
                    image_resource.location, image_resource.raw_media, page_num, reported_errors=reported_pdf_errors)
                page_images.append(ImageResource(fmt, None, image_data))
        elif image_resource.format == "$548":
            image_data, fmt = convert_jxr_to_jpeg_or_png(image_resource.raw_media, image_resource.location)
            page_images.append(ImageResource(fmt, None, image_data))
        else:
            raise Exception("Unexpected image format: %s" % image_resource.format)

    cbz_file = io.BytesIO()

    with zipfile.ZipFile(cbz_file, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for i, image_resource in enumerate(page_images):
            zf.writestr("%04d.%s" % (i + 1, SYMBOL_FORMATS[image_resource.format]), image_resource.raw_media)

        if metadata:
            comment = json_serialize_compact(metadata).encode("utf-8")
            if len(comment) <= 65535:
                zf.comment = comment
            else:
                log.warning("Discarding CBZ metadata -- too long for ZIP comment")

    cbz_data = cbz_file.getvalue()
    cbz_file.close()

    log.info("Combined %s resources into a %d page CBZ file" % (
        list_counts(image_resource_formats), len(ordered_images)))

    return cbz_data


def suffix_location(location, suffix):
    if "." in location:
        return re.sub("\\.", suffix + ".", location, count=1)

    return location + suffix
