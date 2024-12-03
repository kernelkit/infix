"""
Basic file server over HTTP
"""
import concurrent.futures
import functools
import http.server
import socket
from infamy.util import until

class FileServer:
    """Open web server on (address, port) serving files from directory"""
    def __init__(self, netns, address, port, directory):
        self.address = address
        self.port = port
        self.directory = directory
        self.netns = netns
        self.process = None
        self.check_addres = None


    def start(self):
        """start HTTP file server"""
        cmd = f"httpd -p {self.address}:{self.port} -f -h {self.directory}"
        self.process = self.netns.popen(cmd.split(" "))
        if self.address == "[::]":
            check_address = "::1"
        elif self.address == "0.0.0.0":
            check_address = "127.0.0.1"
        else:
            check_address=self.address
        cmd = f"nc -z {check_address} {self.port}".split()
        until(lambda: self.netns.run(cmd).returncode == 0)


    def stop(self):
        """Stop HTTP file server"""
        if self.process:
            self.process.terminate()
            self.process.wait()
            self.process = None

    def __enter__(self):
        self.start()

    def __exit__(self, _, __, ___):
        self.stop()
