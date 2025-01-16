from . import container
from . import link

def operational(ifname=None):
    return {
        "ietf-interfaces:interfaces": {
            "interface":
            link.interfaces(ifname) +
            container.interfaces(ifname),
        },
    }
