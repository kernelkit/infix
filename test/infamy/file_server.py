"""
Basic file server over HTTP
"""
from infamy.util import until
from .netns import NetnsService

class FileServer(NetnsService):
    """Open web server on (address, port) serving files from directory"""
    def __init__(self, netns, address, port, directory):
        super().__init__(netns)
        self.address = address
        self.port = port
        self.directory = directory

    def argv(self):
        return f"httpd -p {self.address}:{self.port} -f -h {self.directory}".split(" ")

    def ready(self):
        if self.address == "[::]":
            check_address = "::1"
        elif self.address == "0.0.0.0":
            check_address = "127.0.0.1"
        else:
            check_address = self.address
        cmd = f"nc -z {check_address} {self.port}".split()
        until(lambda: self.netns.run(cmd).returncode == 0)
