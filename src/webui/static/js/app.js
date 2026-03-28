(function () {
  function getCSRFToken() {
    var meta = document.querySelector('meta[name="csrf-token"]');
    return meta ? meta.getAttribute('content') : '';
  }

  function initCSRF() {
    var token = getCSRFToken();
    if (!token || !window.htmx || !document.body) {
      return;
    }
    document.body.addEventListener('htmx:configRequest', function (evt) {
      evt.detail.headers['X-CSRF-Token'] = token;
    });
  }

  function initDeviceStatusBanner() {
    var banner = document.getElementById('conn-banner');
    if (!banner) {
      return;
    }
    var down = false;
    function check() {
      fetch('/device-status').then(function (r) {
        if (!r.ok) {
          throw r;
        }
        if (down) {
          down = false;
          banner.hidden = true;
        }
      }).catch(function () {
        if (!down) {
          down = true;
          banner.hidden = false;
        }
      });
    }
    setInterval(check, 10000);
  }

  function initProgressBars(root) {
    var scope = root || document;
    var bars = scope.querySelectorAll('.progress-bar[data-progress]');
    bars.forEach(function (bar) {
      var val = parseInt(bar.getAttribute('data-progress'), 10);
      if (!isNaN(val) && val >= 0) {
        bar.style.width = Math.min(val, 100) + '%';
      }
    });
  }

  function startRebootOverlay(root) {
    var scope = root || document;
    var overlay = scope.querySelector('.reboot-overlay');
    if (!overlay || overlay.dataset.init === 'true') {
      return;
    }
    overlay.dataset.init = 'true';

    var timeout = parseInt(overlay.getAttribute('data-timeout'), 10);
    if (isNaN(timeout) || timeout <= 0) {
      timeout = 120000;
    }
    var interval = parseInt(overlay.getAttribute('data-interval'), 10);
    if (isNaN(interval) || interval <= 0) {
      interval = 2000;
    }
    var start = Date.now();
    var status = overlay.querySelector('#reboot-status');
    var returnTo = window.location.pathname + window.location.search;

    function waitDown() {
      if (Date.now() - start > timeout) {
        if (status) {
          status.textContent = 'Timeout - device did not shut down within 2 minutes.';
          status.classList.add('is-error');
        }
        return;
      }
      fetch('/device-status').then(function (r) {
        if (r.ok) {
          setTimeout(waitDown, interval);
        } else {
          if (status) {
            status.textContent = 'Device is down, waiting for it to come back...';
          }
          setTimeout(waitUp, interval);
        }
      }).catch(function () {
        if (status) {
          status.textContent = 'Device is down, waiting for it to come back...';
        }
        setTimeout(waitUp, interval);
      });
    }

    function waitUp() {
      if (Date.now() - start > timeout) {
        if (status) {
          status.textContent = 'Timeout - device did not respond within 2 minutes.';
          status.classList.add('is-error');
        }
        return;
      }
      fetch('/device-status').then(function (r) {
        if (r.ok) {
          window.location = returnTo || '/';
        } else {
          setTimeout(waitUp, interval);
        }
      }).catch(function () {
        setTimeout(waitUp, interval);
      });
    }

    setTimeout(waitDown, interval);
  }

  function initDynamicUI(root) {
    initProgressBars(root);
    startRebootOverlay(root);
  }

  document.addEventListener('DOMContentLoaded', function () {
    initCSRF();
    initDeviceStatusBanner();
    initDynamicUI(document);
  });

  if (window.htmx && document.body) {
    document.body.addEventListener('htmx:afterSwap', function (evt) {
      initDynamicUI(evt.target);
    });
  }
})();

// Theme (auto / light / dark) — shared by main app and login page
(function() {
  function getTheme() {
    var m = document.cookie.split(';').map(function(c){return c.trim();}).find(function(c){return c.indexOf('theme=')===0;});
    return m ? m.split('=')[1] : null;
  }

  function applyTheme(mode) {
    document.documentElement.classList.remove('dark', 'light');
    if (mode === 'dark') document.documentElement.classList.add('dark');
    else if (mode === 'light') document.documentElement.classList.add('light');
    updateDropdownCheck(mode || 'auto');
    updateLoginToggleIcon(mode || 'auto');
  }

  function updateDropdownCheck(mode) {
    document.querySelectorAll('.theme-opt').forEach(function(btn) {
      btn.classList.toggle('is-active', btn.getAttribute('data-theme') === mode);
    });
  }

  function updateLoginToggleIcon(mode) {
    // Login page floating toggle — cycle auto→light→dark
    var btn = document.getElementById('login-theme-toggle');
    if (!btn) return;
    var ids = {auto: 'lti-auto', light: 'lti-light', dark: 'lti-dark'};
    Object.keys(ids).forEach(function(k) {
      var el = btn.querySelector('#' + ids[k]);
      if (el) el.style.display = (k === mode) ? '' : 'none';
    });
  }

  function setTheme(mode) {
    if (mode) {
      document.cookie = 'theme=' + mode + '; path=/; max-age=31536000; samesite=lax';
    } else {
      document.cookie = 'theme=; path=/; max-age=0; samesite=lax';
    }
    applyTheme(mode);
  }

  // Apply saved theme immediately (before DOMContentLoaded to avoid flash)
  applyTheme(getTheme());

  document.addEventListener('DOMContentLoaded', function() {
    // Apply checkmarks now that DOM is ready
    updateDropdownCheck(getTheme() || 'auto');

    // Dropdown theme options (main app)
    document.querySelectorAll('.theme-opt').forEach(function(btn) {
      btn.addEventListener('click', function() {
        var t = btn.getAttribute('data-theme');
        setTheme(t === 'auto' ? null : t);
      });
    });

    // Login page floating toggle — cycles auto → light → dark → auto
    var loginBtn = document.getElementById('login-theme-toggle');
    if (loginBtn) {
      loginBtn.addEventListener('click', function() {
        var cur = getTheme();
        if (!cur || cur === 'auto') setTheme('light');
        else if (cur === 'light') setTheme('dark');
        else setTheme(null);
      });
    }
  });
})();

// Accordion nav group persistence
(function() {
  document.addEventListener('DOMContentLoaded', function() {
    document.querySelectorAll('details.nav-group').forEach(function(d) {
      var label = d.querySelector('summary');
      if (!label) return;
      var key = 'nav-group:' + label.textContent.trim();
      var saved = localStorage.getItem(key);
      if (saved === 'closed') {
        d.removeAttribute('open');
      } else if (saved === 'open') {
        d.setAttribute('open', '');
      }
      d.addEventListener('toggle', function() {
        localStorage.setItem(key, d.open ? 'open' : 'closed');
      });
    });
  });
})();

// User menu — hover handled by CSS; JS manages aria-expanded and keyboard
(function() {
  document.addEventListener('DOMContentLoaded', function() {
    var menu = document.getElementById('user-menu');
    var btn = document.getElementById('user-menu-btn');
    if (!menu || !btn) return;

    menu.addEventListener('mouseenter', function() {
      btn.setAttribute('aria-expanded', 'true');
    });
    menu.addEventListener('mouseleave', function() {
      btn.setAttribute('aria-expanded', 'false');
    });
    document.addEventListener('keydown', function(e) {
      if (e.key === 'Escape') btn.setAttribute('aria-expanded', 'false');
    });
  });
})();

// Sidebar toggle (mobile)
(function() {
  document.addEventListener('DOMContentLoaded', function() {
    var btn = document.getElementById('hamburger-btn');
    if (!btn) return;
    btn.addEventListener('click', function() {
      var open = document.body.classList.toggle('sidebar-open');
      btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
    // Close sidebar when clicking the overlay (::after pseudo)
    document.body.addEventListener('click', function(e) {
      if (document.body.classList.contains('sidebar-open') && e.target === document.body) {
        document.body.classList.remove('sidebar-open');
        btn.setAttribute('aria-expanded', 'false');
      }
    });
    // Close sidebar when a nav link is clicked (htmx navigation)
    document.addEventListener('htmx:beforeRequest', function() {
      if (document.body.classList.contains('sidebar-open')) {
        document.body.classList.remove('sidebar-open');
        if (btn) btn.setAttribute('aria-expanded', 'false');
      }
    });
  });
})();
