"""Shared DRMION parsing helpers for Kindle book content."""

from io import BytesIO

from dedrm.ion import BinaryIonParser, DrmIon, TID_LIST, TID_SYMBOL, addprottable


DRMION_SIGNATURE = b"\xeaDRMION\xee"
CONT_SIGNATURE = b"CONT"


def encryption_key_ids(data):
    """Return stable EnvelopeMetadata encryption-key identifiers."""
    if not data.startswith(DRMION_SIGNATURE):
        return []

    parser = BinaryIonParser(BytesIO(data[8:-8]))
    addprottable(parser)
    parser.reset()

    if not parser.hasnext() or parser.next() != TID_SYMBOL:
        raise ValueError("DRMION doctype symbol is missing")
    if not parser.hasnext() or parser.next() != TID_LIST:
        raise ValueError("DRMION envelope list is missing")

    key_ids = []
    while True:
        if parser.gettypename() == "enddoc":
            break
        parser.stepin()
        while parser.hasnext():
            parser.next()
            if parser.gettypename() not in (
                "com.amazon.drm.EnvelopeMetadata@1.0",
                "com.amazon.drm.EnvelopeMetadata@2.0",
            ):
                continue
            parser.stepin()
            while parser.hasnext():
                parser.next()
                if parser.getfieldname() == "encryption_key":
                    key_id = parser.stringvalue()
                    if key_id and key_id not in key_ids:
                        key_ids.append(key_id)
            parser.stepout()
        parser.stepout()
        if not parser.hasnext():
            break
        parser.next()

    return key_ids


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
