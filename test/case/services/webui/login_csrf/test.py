#!/usr/bin/env python3
"""Web UI login and CSRF protection

Black-box test of the web management interface (nginx :443 -> Go webui on
127.0.0.1:10000).  Drives the UI exactly like a browser would, using a
cookie-preserving HTTP session, and verifies the security-critical parts of
the login flow:

1. A protected page is not served to an unauthenticated client; the request
   is redirected to /login.
2. Login with a bad password is rejected (no session is created).
3. Login with the correct credentials establishes a session, and the session
   cookie is flagged HttpOnly.
4. An authenticated page load succeeds.
5. A state-changing POST without the CSRF token is rejected with 403, even
   with a valid session cookie.
6. The same POST with the CSRF token (X-CSRF-Token header) succeeds.
7. After logout the session cookie no longer grants access.

The device is reached over its IPv6 link-local mgmt address, which carries a
zone id (fe80::.../%ifname).  requests/urllib3 percent-encode the '%' to
'%25' while preparing the request, so we substitute it back before sending —
the same workaround infamy.restconf uses.
"""
import re
import warnings

import requests
from urllib3.exceptions import InsecureRequestWarning

import infamy
from infamy import until

# The device uses a self-signed certificate; silence the verify=False noise.
warnings.simplefilter("ignore", InsecureRequestWarning)


class WebUI:
    """Cookie-preserving HTTP client for the web UI, link-local aware."""

    def __init__(self, host, username, password):
        self.base = f"https://[{host}]"
        self.username = username
        self.password = password
        self.session = requests.Session()

    def _send(self, method, path, **kwargs):
        req = requests.Request(method, f"{self.base}{path}", **kwargs)
        prepared = self.session.prepare_request(req)
        # Undo urllib3's percent-encoding of the IPv6 zone id separator.
        prepared.url = re.sub(r"%25", "%", prepared.url)
        # http.cookiejar's host normalisation for IPv6 link-local URLs is
        # inconsistent across Python versions — the csrf cookie set on
        # GET /login can fail to match the same URL on the following POST,
        # producing a 403 from csrfMiddleware that's awfully hard to
        # diagnose.  Rebuild the Cookie header from the jar directly so
        # whatever we received gets sent back verbatim.
        jar = "; ".join(f"{c.name}={c.value}" for c in self.session.cookies)
        if jar:
            prepared.headers["Cookie"] = jar
        # Never auto-follow redirects: requests would rebuild the target URL
        # internally and re-mangle the link-local zone id, breaking the
        # connection.  We assert on the 3xx directly instead.
        return self.session.send(prepared, verify=False, allow_redirects=False)

    def get(self, path, **kwargs):
        return self._send("GET", path, **kwargs)

    def post(self, path, **kwargs):
        return self._send("POST", path, **kwargs)

    def csrf_token(self):
        """Return the current CSRF token from the session cookie jar."""
        return self.session.cookies.get("csrf")

    def login(self):
        """Fetch /login, scrape the CSRF token, and POST credentials.

        Returns the POST response.  On success the server answers 303 to /
        and sets the session cookie; on failure it re-renders /login (200)
        with no session cookie.
        """
        page = self.get("/login")
        assert page.status_code == 200, f"GET /login: {page.status_code}"
        m = re.search(r'name="csrf-token"\s+content="([^"]+)"', page.text)
        assert m, "no csrf-token meta tag on /login"
        token = m.group(1)

        return self.post("/login", data={
            "username": self.username,
            "password": self.password,
            "csrf": token,
        })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        # Attach over NETCONF only to learn the mgmt address and admin
        # password; the web UI itself is exercised over plain HTTPS below.
        target = env.attach("target", "mgmt", protocol="netconf")
        host = target.location.host
        password = target.location.password

    with test.step("Wait for the web UI to come up"):
        def webui_ready():
            try:
                r = WebUI(host, "admin", password).get("/login")
                return r.status_code == 200 and "csrf-token" in r.text
            except Exception:
                return False

        until(webui_ready, attempts=60, interval=2)

    with test.step("Unauthenticated access to a protected page redirects to /login"):
        anon = WebUI(host, "admin", password)
        r = anon.get("/")
        assert r.status_code == 303, f"want 303, got {r.status_code}"
        assert r.headers.get("Location") == "/login", \
            f"want redirect to /login, got {r.headers.get('Location')}"

    with test.step("Login with a bad password is rejected"):
        bad = WebUI(host, "admin", "definitely-not-the-password")
        r = bad.login()
        assert r.status_code == 200, \
            f"bad login should re-render /login (200), got {r.status_code}"
        assert "session" not in bad.session.cookies, \
            "session cookie set despite wrong password"
        # And a protected page is still denied.
        r = bad.get("/")
        assert r.status_code == 303, \
            f"bad login still authenticated? got {r.status_code}"

    with test.step("Login with correct credentials establishes a session"):
        ui = WebUI(host, "admin", password)
        r = ui.login()
        assert r.status_code == 303, f"login should redirect (303), got {r.status_code}"
        assert r.headers.get("Location") == "/", \
            f"login should redirect to /, got {r.headers.get('Location')}"
        assert "session" in ui.session.cookies, "no session cookie after login"

    with test.step("Session cookie is flagged HttpOnly"):
        sess = next(c for c in ui.session.cookies if c.name == "session")
        assert sess.has_nonstandard_attr("HttpOnly"), \
            "session cookie missing HttpOnly flag"

    with test.step("Authenticated page load succeeds"):
        r = ui.get("/interfaces")
        assert r.status_code == 200, f"GET /interfaces: {r.status_code}"
        # base.html ships an htmx-config meta tag on every authenticated page;
        # its absence means the layout chrome didn't render even though we got 200.
        assert 'name="htmx-config"' in r.text, "htmx-config meta missing from authenticated page"

    with test.step("State-changing POST without CSRF token is rejected"):
        # Valid session cookie, but no X-CSRF-Token / csrf form field.
        r = ui.post("/logout")
        assert r.status_code == 403, \
            f"CSRF-less POST not rejected, got {r.status_code}"

    with test.step("State-changing POST with CSRF token succeeds (logout)"):
        r = ui.post("/logout", headers={"X-CSRF-Token": ui.csrf_token()})
        assert r.status_code in (200, 204, 303), \
            f"logout failed, got {r.status_code}"

    with test.step("After logout the session no longer grants access"):
        r = ui.get("/")
        assert r.status_code == 303, \
            f"still authenticated after logout, got {r.status_code}"

    test.succeed()
