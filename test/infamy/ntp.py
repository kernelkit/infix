"""
NTP client helper
"""


def _get_ntp(target):
    xpath = "/ietf-system:system-state/infix-system:ntp"
    data = target.get_data(xpath)

    if data is None:
        return None

    return data["system-state"].get("infix-system:ntp", None) or data["system-state"].get("ntp", None)


def _get_ntp_sources(target):
    ntp = _get_ntp(target)

    if ntp is None:
        return []

    return ntp["sources"]["source"]


def any_source_selected(target):
    sources = _get_ntp_sources(target)

    for source in sources:
        if source["state"] == "selected":
            return True

    return False


def number_of_sources(target):
    sources = _get_ntp_sources(target)

    return len(sources)
