from .common import LOG
from .host import HOST


def operational():
    modems = HOST.run_json(['/usr/libexec/modemd/modem-info'], [])

    return {
        "infix-modem:modems": {
            "modem": [m for m in modems]
        }
    }
