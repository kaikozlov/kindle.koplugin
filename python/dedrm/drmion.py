"""Shared DRMION parsing helpers for Kindle book content."""

from io import BytesIO

from dedrm.ion import DrmIon


DRMION_SIGNATURE = b"\xeaDRMION\xee"
CONT_SIGNATURE = b"CONT"


def decrypt(data, page_key):
    """Decode a DRMION envelope, optionally using a 16-byte page key."""
    if not data.startswith(DRMION_SIGNATURE):
        raise ValueError("Not a DRMION file")

    class Voucher:
        def __init__(self, key):
            self.secretkey = key

    output = BytesIO()
    drm = DrmIon(BytesIO(data[8:-8]), lambda name: Voucher(page_key))
    drm.parse(output)
    result = output.getvalue()

    if not result.startswith(CONT_SIGNATURE):
        raise ValueError("Decrypted data is not a valid CONT container")
    return result
