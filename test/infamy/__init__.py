import os

from .container import Container
from .env import Env
from .env import ArgumentParser
from .env import test_argument
from .firewall import Firewall
from .netns import IsolatedMacVlan,IsolatedMacVlans
from .portscanner import PortScanner
from .sniffer import Sniffer
from .tap import Test
from .util import parallel, until

def std_topology(name):
    return os.path.realpath(
        os.path.join(
            os.path.dirname(__file__),
            "topologies",
            name + ".dot"
        )
    )
