"""
Basic file server over HTTP
"""
import concurrent.futures
import functools
import http.server
import socket


class FileServer(http.server.HTTPServer):
    """Open web server on (address, port) serving files from directory"""
    class RequestHandler(http.server.SimpleHTTPRequestHandler):
        def log_message(self, *args, **kwargs):
            pass

    address_family = socket.AF_INET6

    def __init__(self, server_address, directory):
        rh = functools.partial(FileServer.RequestHandler, directory=directory)
        self.__tp = concurrent.futures.ThreadPoolExecutor(max_workers=1)
        super().__init__(server_address, rh)

    def __enter__(self):
        self.__tp.submit(self.serve_forever)

    def __exit__(self, _, __, ___):
        self.shutdown()
        self.__tp.shutdown()
