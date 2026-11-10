"""Drive the WebUI in a headless Chromium over the DevTools protocol.

Standalone, needs only websockets 11 or later.  Clicks are real mouse
events and fill() types, so an element that covers the page, a page left
inert by a modal, or a form that only reacts to input events, fails the
way it fails a user.  errors is every uncaught exception, console.error,
htmx request failure and HTTP response of 400 or above, each tagged
with the page it happened on.  The forwarder on localhost is for a
link-local address, Chromium refuses a zone id in a URL.
"""
import base64
import json
import os
import shutil
import ssl
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

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


HOOK = """
for (const name of ['htmx:responseError', 'htmx:sendError', 'htmx:swapError', 'htmx:timeout'])
    document.addEventListener(name, evt => {
        const d = evt.detail || {}, x = d.xhr || {};
        console.error(name,
                      d.requestConfig ? d.requestConfig.verb + ' ' + d.requestConfig.path : '',
                      x.status || '', (x.responseText || '').slice(0, 200));
    });
"""


class Browser:
    """One headless Chromium with one page, errors is what the page and the server said"""

    def __init__(self, host, username="admin", password="admin", port=443, scheme="https",
                 interface=None):
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
        self.dialogs = []
        self.requests = {}
        self.accept_dialogs = True
        self.seq = 0

        exe = next(filter(shutil.which, CHROMIUM), None)
        if not exe:
            raise RuntimeError(f"none of {CHROMIUM} in PATH")
        self.profile = tempfile.mkdtemp(prefix="infamy-chromium-")
        self.downloads = os.path.join(self.profile, "downloads")
        os.mkdir(self.downloads)
        cmd = [exe, *FLAGS, f"--user-data-dir={self.profile}", "about:blank"]
        self.proc = subprocess.Popen(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.ws = connect(self._browser_url(), max_size=None)
        target = self._send("Target.createTarget", url="about:blank")["targetId"]
        attached = self._send("Target.attachToTarget", targetId=target, flatten=True)
        self.session = attached["sessionId"]
        self.page = "about:blank"
        for method in ("Page.enable", "Runtime.enable", "Log.enable", "Network.enable"):
            self.call(method)
        self.call("Security.setIgnoreCertificateErrors", ignore=True)
        self.call("Page.addScriptToEvaluateOnNewDocument", source=HOOK)
        self._send("Browser.setDownloadBehavior", behavior="allow", downloadPath=self.downloads)

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
        end = time.time() + 60
        while True:
            try:
                reply = json.loads(self.ws.recv(timeout=max(end - time.time(), 0.1)))
            except TimeoutError:
                raise TimeoutError(f"no reply to {method} {json.dumps(params)[:200]} "
                                   "within 60s") from None
            if reply.get("id") == self.seq:
                if "error" in reply:
                    raise RuntimeError(f"{method}: {reply['error']}")
                return reply.get("result", {})
            self._event(reply)

    def _event(self, msg):
        method, params = msg.get("method"), msg.get("params", {})
        if method == "Page.frameNavigated" and not params["frame"].get("parentId"):
            self.page = params["frame"]["url"].removeprefix(self.base)
        elif method == "Page.navigatedWithinDocument":
            self.page = params["url"].removeprefix(self.base)
        elif method == "Runtime.exceptionThrown":
            det = params["exceptionDetails"]
            text = det.get("exception", {}).get("description") or det.get("text", "exception")
            self._error(text.splitlines()[0])
        elif method == "Runtime.consoleAPICalled" and params.get("type") == "error":
            self._error(" ".join(str(a.get("value", a.get("description"))) for a in params["args"]))
        elif method == "Network.responseReceived" and params["response"]["status"] >= 400:
            res = params["response"]
            self._error(f"HTTP {res['status']} {res['url'].removeprefix(self.base)}")
        elif method == "Network.requestWillBeSent":
            self.requests[params["requestId"]] = params["request"]["url"].removeprefix(self.base)
        elif method == "Network.loadingFailed" and not params.get("canceled"):
            url = self.requests.get(params["requestId"], "?")
            self._error(f"request failed: {params.get('errorText')} {url}")
        elif method == "Page.javascriptDialogOpening":
            self.dialogs.append(params.get("message", ""))
            answer = {"id": 0, "method": "Page.handleJavaScriptDialog",
                      "params": {"accept": self.accept_dialogs}, "sessionId": self.session}
            self.ws.send(json.dumps(answer))

    def _error(self, text):
        self.errors.append(f"{self.page}: {text}")

    def call(self, method, **params):
        """A DevTools method on the page"""
        return self._send(method, session=self.session, **params)

    def eval(self, expr):
        """Value of a JavaScript expression in the page"""
        res = self.call("Runtime.evaluate", expression=expr, returnByValue=True, awaitPromise=True)
        if "exceptionDetails" in res:
            detail = res["exceptionDetails"].get("exception", {})
            raise RuntimeError(detail.get("description", expr))
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
        self.wait(f"location.href.startsWith({json.dumps(self.base)}) "
                  "&& document.readyState === 'complete'", timeout)

    def login(self):
        self.goto("/login")
        self.fill("#username", self.username)
        self.fill("#password", self.password)
        self.click("form button[type=submit]")
        try:
            self.wait("document.readyState === 'complete' && location.pathname !== '/login'")
        except TimeoutError as err:
            page = self.eval("document.body ? document.body.innerText.slice(0, 200) : ''")
            raise RuntimeError(f"login as {self.username} failed, the page says: {page!r}") from err

    def click(self, selector, text=None):
        """A real mouse click on the first rendered match of selector, holding text when given"""
        rect = self.eval(f"""(() => {{
            const text = {json.dumps(text)};
            const el = [...document.querySelectorAll({json.dumps(selector)})]
                .find(e => {{
                    const r = e.getBoundingClientRect();
                    return r.width > 0 && r.height > 0
                        && e.checkVisibility({{contentVisibilityAuto: true,
                                               visibilityProperty: true}})
                        && (text === null || e.textContent.includes(text));
                }});
            if (!el) return null;
            el.scrollIntoView({{block: 'center'}});
            const r = el.getBoundingClientRect();
            return {{x: r.x + r.width / 2, y: r.y + r.height / 2}};
        }})()""")
        if not rect:
            raise LookupError(f"no rendered element {selector!r} with text {text!r}")
        for kind in ("mouseMoved", "mousePressed", "mouseReleased"):
            self.call("Input.dispatchMouseEvent", type=kind, x=rect["x"], y=rect["y"],
                      button="left", clickCount=1)

    def fill(self, selector, text):
        """Type text into the control matching selector, as keystrokes, replacing what is there"""
        found = self.eval(f"""(() => {{
            const el = document.querySelector({json.dumps(selector)});
            if (!el) return false;
            el.scrollIntoView({{block: 'center'}});
            el.focus();
            if (el.select) el.select();
            return document.activeElement === el;
        }})()""")
        if not found:
            raise LookupError(f"no focusable element {selector!r}")
        self.call("Input.dispatchKeyEvent", type="keyDown", key="Delete", windowsVirtualKeyCode=46)
        self.call("Input.dispatchKeyEvent", type="keyUp", key="Delete", windowsVirtualKeyCode=46)
        self.call("Input.insertText", text=text)
        self.eval(f"document.querySelector({json.dumps(selector)})"
                  ".dispatchEvent(new Event('change', {bubbles: true}))")

    def upload(self, selector, path):
        """Choose the local file at path in the file input matching selector, as a user would"""
        root = self.call("DOM.getDocument", depth=0)["root"]["nodeId"]
        node = self.call("DOM.querySelector", nodeId=root, selector=selector)["nodeId"]
        if not node:
            raise LookupError(f"no file input {selector!r}")
        self.call("DOM.setFileInputFiles", nodeId=node, files=[os.path.abspath(path)])

    def invalid(self, selector="form"):
        """Names of the controls that keep the form matching selector from submitting"""
        return self.eval(f"""(() => {{
            const form = document.querySelector({json.dumps(selector)});
            if (!form) return null;
            return [...form.querySelectorAll(':invalid:not(:disabled)')]
                .map(e => (e.name || e.id || e.tagName) + (e.offsetParent ? '' : ' (hidden)'));
        }})()""")

    def download(self, selector, text=None, timeout=60):
        """Click the element and return the path of the file that arrived, with its size"""
        before = set(os.listdir(self.downloads))
        self.click(selector, text)
        end = time.time() + timeout
        while time.time() < end:
            arrived = set(os.listdir(self.downloads)) - before
            new = [f for f in arrived if not f.endswith(".crdownload")]
            if new:
                path = os.path.join(self.downloads, new[0])
                return path, os.path.getsize(path)
            time.sleep(0.5)
        raise TimeoutError(f"no download within {timeout}s from {selector!r} {text!r}")

    def screenshot(self, path):
        """Write a PNG of the viewport to path"""
        data = self.call("Page.captureScreenshot", format="png")["data"]
        with open(path, "wb") as f:
            f.write(base64.b64decode(data))
        return path

    def restconf(self, path, datastore="running"):
        """The JSON at /restconf/ds/ietf-datastores:<datastore>/<path> on the box, None on 404"""
        url = f"{self.base}/restconf/ds/ietf-datastores:{datastore}/{path}"
        token = base64.b64encode(f"{self.username}:{self.password}".encode()).decode()
        req = urllib.request.Request(url, headers={"Authorization": f"Basic {token}",
                                                   "Accept": "application/yang-data+json"})
        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        try:
            with urllib.request.urlopen(req, context=ctx, timeout=30) as res:
                return json.load(res)
        except urllib.error.HTTPError as err:
            if err.code == 404:
                return None
            raise

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
