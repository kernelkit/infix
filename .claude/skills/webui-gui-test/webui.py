"""Drive the WebUI in a headless Chromium over the DevTools protocol.

Standalone, needs only websockets 11 or later.  Clicks are real mouse
events, so an element that covers the page, or a page left inert by a
modal, fails the way it fails a user.  The forwarder on localhost is for
a link-local address, Chromium refuses a zone id in a URL.
"""
import json
import shutil
import socket
import subprocess
import tempfile
import threading
import time

from websockets.sync.client import connect

CHROMIUM = ("chromium", "chromium-browser")
FLAGS = ("--headless=new", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
         "--ignore-certificate-errors", "--remote-debugging-port=0", "--window-size=1280,1024")


class Forwarder:
    """TCP forwarder from a port on 127.0.0.1 to host on interface"""

    def __init__(self, host, port, interface):
        if "%" not in host and interface:
            host = f"{host}%{interface}"
        self.dst = (host, port)
        self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.sock.bind(("127.0.0.1", 0))
        self.sock.listen(16)
        self.port = self.sock.getsockname()[1]
        threading.Thread(target=self._accept, daemon=True).start()

    def _accept(self):
        while True:
            try:
                client, _ = self.sock.accept()
            except OSError:
                return
            try:
                server = socket.create_connection(self.dst, timeout=60)
            except OSError:
                client.close()
                continue
            for src, dst in ((client, server), (server, client)):
                threading.Thread(target=self._pump, args=(src, dst), daemon=True).start()

    @staticmethod
    def _pump(src, dst):
        try:
            while data := src.recv(65536):
                dst.sendall(data)
        except OSError:
            pass
        finally:
            for sock in (src, dst):
                try:
                    sock.shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass

    def close(self):
        self.sock.close()


class Browser:
    """One headless Chromium with one page, errors is what the page logged"""

    def __init__(self, host, username="admin", password="admin", port=443, scheme="https", interface=None):
        self.fwd = None
        if interface:
            self.fwd = Forwarder(host, port, interface)
            host, port = "127.0.0.1", self.fwd.port
        if ":" in host and not host.startswith("["):
            host = f"[{host}]"
        default = {"https": 443, "http": 80}.get(scheme)
        self.base = f"{scheme}://{host}" if port == default else f"{scheme}://{host}:{port}"
        self.username = username
        self.password = password
        self.errors = []
        self.seq = 0

        exe = next(filter(shutil.which, CHROMIUM), None)
        if not exe:
            raise RuntimeError(f"none of {CHROMIUM} in PATH")
        self.profile = tempfile.mkdtemp(prefix="infamy-chromium-")
        self.proc = subprocess.Popen([exe, *FLAGS, f"--user-data-dir={self.profile}", "about:blank"],
                                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.ws = connect(self._browser_url(), max_size=None)
        target = self._send("Target.createTarget", url="about:blank")["targetId"]
        self.session = self._send("Target.attachToTarget", targetId=target, flatten=True)["sessionId"]
        for method in ("Page.enable", "Runtime.enable", "Log.enable"):
            self.call(method)
        self.call("Security.setIgnoreCertificateErrors", ignore=True)

    def _browser_url(self):
        path = f"{self.profile}/DevToolsActivePort"
        for _ in range(100):
            try:
                port, suffix = open(path, encoding="utf-8").read().split()
                return f"ws://127.0.0.1:{port}{suffix}"
            except (OSError, ValueError):
                time.sleep(0.1)
        raise RuntimeError("Chromium did not open its DevTools port")

    def _send(self, method, session=None, **params):
        self.seq += 1
        msg = {"id": self.seq, "method": method, "params": params}
        if session:
            msg["sessionId"] = session
        self.ws.send(json.dumps(msg))
        while True:
            reply = json.loads(self.ws.recv())
            if reply.get("id") == self.seq:
                if "error" in reply:
                    raise RuntimeError(f"{method}: {reply['error']}")
                return reply.get("result", {})
            self._event(reply)

    def _event(self, msg):
        method, params = msg.get("method"), msg.get("params", {})
        if method == "Runtime.exceptionThrown":
            self.errors.append(params["exceptionDetails"].get("text", "exception"))
        elif method == "Runtime.consoleAPICalled" and params.get("type") == "error":
            self.errors.append(" ".join(str(a.get("value", a.get("description"))) for a in params["args"]))

    def call(self, method, **params):
        """A DevTools method on the page"""
        return self._send(method, session=self.session, **params)

    def eval(self, expr):
        """Value of a JavaScript expression in the page"""
        res = self.call("Runtime.evaluate", expression=expr, returnByValue=True, awaitPromise=True)
        if "exceptionDetails" in res:
            raise RuntimeError(res["exceptionDetails"].get("exception", {}).get("description", expr))
        return res.get("result", {}).get("value")

    def wait(self, expr, timeout=30):
        """Value of expr once it is truthy, within timeout seconds"""
        end = time.time() + timeout
        while True:
            val = self.eval(expr)
            if val:
                return val
            if time.time() > end:
                raise TimeoutError(f"still false after {timeout}s: {expr}")
            time.sleep(0.5)

    def goto(self, path, timeout=30):
        """Navigate to path and wait for a page of the site to load"""
        self.call("Page.navigate", url=self.base + path)
        self.wait(f"location.href.startsWith({json.dumps(self.base)}) && document.readyState === 'complete'", timeout)

    def login(self):
        self.goto("/login")
        self.eval(f"""(() => {{
            document.querySelector('#username').value = {json.dumps(self.username)};
            document.querySelector('#password').value = {json.dumps(self.password)};
            document.querySelector('form').submit();
        }})()""")
        try:
            self.wait("document.readyState === 'complete' && location.pathname !== '/login'")
        except TimeoutError as err:
            page = self.eval("document.body ? document.body.innerText.slice(0, 200) : ''")
            raise RuntimeError(f"login as {self.username} failed, the page says: {page!r}") from err

    def click(self, selector, text=None):
        """A mouse click on the element matching selector, the one with text in it when given"""
        rect = self.eval(f"""(() => {{
            const text = {json.dumps(text)};
            const el = [...document.querySelectorAll({json.dumps(selector)})]
                .find(e => text === null || e.textContent.includes(text));
            if (!el) return null;
            el.scrollIntoView({{block: 'center'}});
            const r = el.getBoundingClientRect();
            return {{x: r.x + r.width / 2, y: r.y + r.height / 2}};
        }})()""")
        if not rect:
            raise LookupError(f"no element {selector!r} with text {text!r}")
        for kind in ("mouseMoved", "mousePressed", "mouseReleased"):
            self.call("Input.dispatchMouseEvent", type=kind, x=rect["x"], y=rect["y"],
                      button="left", clickCount=1)

    def close(self):
        try:
            self.ws.close()
        finally:
            self.proc.terminate()
            self.proc.wait(10)
            shutil.rmtree(self.profile, ignore_errors=True)
            if self.fwd:
                self.fwd.close()

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()
