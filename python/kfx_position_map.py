"""Text-free KFX coordinate helpers used by the position adapter."""


__license__ = "GPL v3"

KFX_EID_ATTRIBUTE = "data-kfx-eid"
KFX_PID_ATTRIBUTE = "data-kfx-pid"


def _plain_value(value):
    if isinstance(value, int):
        return int(value)
    if value is None:
        return None
    return str(value)


def unique_eid_base_pids(chunks):
    """Return unambiguous ``eid -> base pid`` values from kfxlib chunks.

    A KFX entity can appear in more than one position domain.  Only publish a
    base PID when every observed chunk for that EID agrees; otherwise exact
    translation must fail rather than invent a misleading coordinate.
    """

    bases_by_eid = {}
    for chunk in chunks:
        eid = _plain_value(chunk.eid)
        base_pid = int(chunk.pid) - int(chunk.eid_offset)
        bases_by_eid.setdefault(eid, set()).add(base_pid)

    return {
        eid: next(iter(bases))
        for eid, bases in bases_by_eid.items()
        if len(bases) == 1
    }


def tag_position_element(elem, eid, unique_bases):
    plain_eid = _plain_value(eid)
    elem.set(KFX_EID_ATTRIBUTE, str(plain_eid))
    if plain_eid in unique_bases:
        elem.set(KFX_PID_ATTRIBUTE, str(unique_bases[plain_eid]))
