import os

from .container import Container
from .env import Env
from .furl import Furl
from .netns import IsolatedMacVlan
from .sniffer import Sniffer
from .tap import Test
from .util import until

def std_topology(name):
    return os.path.realpath(
        os.path.join(
            os.path.dirname(__file__),
            "topologies",
            name + ".dot"
        )
    )
