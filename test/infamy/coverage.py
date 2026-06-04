"""XPath coverage tracker for infamy test runs.

Writes covered xpaths to $NINEPM_LOG_PATH/xpath_coverage.log, one per line.
Call track_xpath(), track_module(), or track_dict() from transport methods.
"""

import os
import threading

_lock = threading.Lock()
_tracked: set[str] = set()


def _log_path() -> str | None:
    log_dir = os.environ.get("NINEPM_LOG_PATH")
    if not log_dir:
        return None
    return os.path.join(log_dir, "xpath_coverage.log")


def _flush(xpaths: list[str]) -> None:
    path = _log_path()
    if not path or not xpaths:
        return
    try:
        with open(path, "a", encoding="utf-8") as f:
            for x in xpaths:
                f.write(x + "\n")
    except OSError:
        pass


def _record(xpaths: list[str]) -> None:
    new: list[str] = []
    with _lock:
        for x in xpaths:
            if x not in _tracked:
                _tracked.add(x)
                new.append(x)
    _flush(new)


def _normalize(xpath: str) -> str:
    """Strip module prefix from every segment except the root.

    pyang and the CSV always use /mod:top/child (no mid-path prefixes).
    Tests sometimes pass /mod:top/other-mod:child; we canonicalize here.
    """
    if not xpath or ":" not in xpath:
        return xpath
    segs = xpath.lstrip("/").split("/")
    out = []
    for i, seg in enumerate(segs):
        out.append(seg if i == 0 else seg.split(":", 1)[-1])
    return "/" + "/".join(out)


def track_xpath(xpath: str | None) -> None:
    """Record a direct xpath access (get_data, delete_xpath, call_action, ...)."""
    if xpath:
        _record([_normalize(xpath)])


def track_module(modname: str | None) -> None:
    """Record a module-level access (call_dict with no specific path)."""
    if modname:
        _record([f"/{modname}:*"])


def track_dict(modname: str | None, data: object) -> None:
    """Record xpaths implied by a config/rpc dict (put_config_dicts, ...)."""
    if not modname or not isinstance(data, dict):
        return
    _record(list(_dict_to_xpaths(modname, data)))


def _dict_to_xpaths(modname: str, data: object, prefix: str = "") -> set[str]:
    xpaths: set[str] = set()

    if isinstance(data, dict):
        for key, value in data.items():
            node = key.split(":", 1)[-1]

            if not prefix:
                xpath = f"/{modname}:{node}"
            else:
                xpath = f"{prefix}/{node}"

            xpaths.add(xpath)
            xpaths.update(_dict_to_xpaths(modname, value, xpath))
    elif isinstance(data, list):
        for item in data:
            xpaths.update(_dict_to_xpaths(modname, item, prefix))
            
    return xpaths
