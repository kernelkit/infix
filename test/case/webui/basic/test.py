#!/usr/bin/env python3
"""
WebUI login and logout

Smoke test that the WebUI is reachable, accepts admin credentials,
serves the dashboard while authenticated, and clears the session on
logout.  Pure HTTP — no browser framework — so we only catch backend
and plumbing regressions (nginx down, Go service crashed, TLS or
RESTCONF auth broken, CSRF or session cookie logic broken, wrong
defaults).  JS, htmx, and visual issues are deliberately out of
scope.
"""
import re
import time
import infamy
import requests


with infamy.Test() as test:
    with test.step("Set up topology and wait for WebUI to come up"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        password = env.get_password("target")

        # Bracket the host so IPv6 link-local mgmt addresses
        # (e.g. fe80::...%eth0) form a valid URL.
        base = f"https://[{target.location.host}]"

        sess = requests.Session()
        sess.verify = False  # device ships a self-signed cert

        # env.attach() returns once NETCONF/RESTCONF is reachable, but
        # nginx + the Go webui process may still be coming up.  Poll
        # /login briefly to ride out the early-boot window where
        # default.conf's error_page would otherwise hand us /50x.html.
        for _ in range(15):
            try:
                r = sess.get(f"{base}/login", timeout=10)
                if r.status_code == 200 and 'name="csrf"' in r.text:
                    break
            except requests.RequestException:
                pass
            time.sleep(1)
        else:
            test.fail("WebUI did not come up within 15s")
        m = re.search(r'name="csrf"\s+value="([^"]+)"', r.text)
        if not m:
            test.fail("CSRF token field not found on /login")
        login_csrf = m.group(1)

    with test.step("Authenticate as admin and load the dashboard"):
        r = sess.post(
            f"{base}/login",
            data={"username": "admin", "password": password, "csrf": login_csrf},
            allow_redirects=False,
            timeout=10,
        )
        if r.status_code != 303:
            test.fail(f"POST /login: expected 303, got {r.status_code}")
        if "session" not in sess.cookies:
            test.fail("session cookie not set after successful login")

        r = sess.get(f"{base}/", timeout=10)
        if r.status_code != 200:
            test.fail(f"GET /: expected 200, got {r.status_code}")
        # base.html ships an htmx-config meta tag on every authenticated page.
        if 'name="htmx-config"' not in r.text:
            test.fail("htmx-config meta tag missing from /")
        m = re.search(r'name="csrf-token"\s+content="([^"]+)"', r.text)
        if not m:
            test.fail("csrf-token meta tag not found on dashboard")
        meta_csrf = m.group(1)

    with test.step("Logout clears the session cookie"):
        r = sess.post(
            f"{base}/logout",
            data={"csrf": meta_csrf},
            allow_redirects=False,
            timeout=10,
        )
        if r.status_code != 303:
            test.fail(f"POST /logout: expected 303, got {r.status_code}")
        if "session" in sess.cookies:
            test.fail("session cookie still present after logout")

    test.succeed()
