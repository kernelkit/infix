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

// Active nav link — keep in sync with the current URL after htmx navigation.
// Also opens the top-level accordion section containing the active link,
// and immediately highlights the clicked link (optimistic active state).
(function() {
  function updateActiveNav() {
    var path = window.location.pathname;
    var activeTopGroup = null;

    document.querySelectorAll('.nav-link').forEach(function(link) {
      var href = link.getAttribute('href');
      var active = href === path;
      link.classList.toggle('active', active);
      if (active) {
        var topGroup = link.closest('details.nav-group-top');
        if (topGroup) activeTopGroup = topGroup;
      }
    });

    if (activeTopGroup) {
      document.querySelectorAll('details.nav-group-top').forEach(function(d) {
        if (d === activeTopGroup) {
          d.setAttribute('open', '');
        } else {
          d.removeAttribute('open');
        }
      });
    }
  }

  // Optimistic active state: highlight the link immediately on click so the
  // sidebar doesn't lag while waiting for the server to respond.
  document.addEventListener('click', function(e) {
    var link = e.target.closest('a.nav-link[hx-get]');
    if (!link) return;
    document.querySelectorAll('.nav-link').forEach(function(l) { l.classList.remove('active'); });
    link.classList.add('active');
  });

  document.addEventListener('DOMContentLoaded', updateActiveNav);
  document.addEventListener('htmx:pushedIntoHistory', updateActiveNav);
  window.addEventListener('popstate', updateActiveNav);
})();

// Top progress bar during htmx navigations
(function() {
  var bar, timer;

  function getBar() {
    if (!bar) bar = document.getElementById('page-progress');
    return bar;
  }

  function start() {
    var b = getBar();
    if (!b) return;
    if (timer) { clearTimeout(timer); timer = null; }
    b.style.transition = 'none';
    b.style.width = '0%';
    b.style.opacity = '1';
    b.offsetWidth; // force reflow so the transition below fires from 0
    b.style.transition = 'width 8s cubic-bezier(0.05, 0.8, 0.4, 1)';
    b.style.width = '85%';
  }

  function finish() {
    var b = getBar();
    if (!b) return;
    if (timer) clearTimeout(timer);
    b.style.transition = 'width 0.1s ease';
    b.style.width = '100%';
    timer = setTimeout(function() {
      b.style.transition = 'opacity 0.25s ease';
      b.style.opacity = '0';
      timer = setTimeout(function() {
        b.style.transition = 'none';
        b.style.width = '0%';
        timer = null;
      }, 260);
    }, 120);
  }

  document.addEventListener('htmx:beforeSend',     start);
  document.addEventListener('htmx:afterSettle',    finish);
  document.addEventListener('htmx:responseError',  finish);
  document.addEventListener('htmx:sendError',      finish);
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
// Top-level sections (Status / Configure / Maintenance) are mutually exclusive.
// State is persisted in localStorage under 'nav-top:Name' keys.
(function() {
  document.addEventListener('DOMContentLoaded', function() {
    var groups = document.querySelectorAll('details.nav-group-top');

    // Restore saved state (HTML default: Status open, others closed)
    groups.forEach(function(d) {
      var label = d.querySelector(':scope > summary');
      if (!label) return;
      var key = 'nav-top:' + label.textContent.trim();
      var saved = localStorage.getItem(key);
      if (saved === 'open') {
        d.setAttribute('open', '');
      } else if (saved === 'closed') {
        d.removeAttribute('open');
      }
    });

    // Mutual exclusion, persistence, and auto-navigation to first page link
    groups.forEach(function(d) {
      var label = d.querySelector(':scope > summary');
      if (!label) return;
      var key = 'nav-top:' + label.textContent.trim();
      d.addEventListener('toggle', function() {
        localStorage.setItem(key, d.open ? 'open' : 'closed');
        if (d.open) {
          groups.forEach(function(other) {
            if (other !== d && other.open) {
              other.removeAttribute('open');
              var otherLabel = other.querySelector(':scope > summary');
              if (otherLabel) {
                localStorage.setItem('nav-top:' + otherLabel.textContent.trim(), 'closed');
              }
            }
          });
          // Navigate to the first page link if none in this section is active
          if (!d.querySelector('.nav-link.active')) {
            var first = d.querySelector('a.nav-link[hx-get]');
            if (first) first.click();
          }
        }
      });
    });
  });
})();

// User menu — hover (desktop) + click-toggle (touch) + keyboard
(function() {
  document.addEventListener('DOMContentLoaded', function() {
    var menu = document.getElementById('user-menu');
    var btn = document.getElementById('user-menu-btn');
    if (!menu || !btn) return;

    function setExpanded(val) {
      btn.setAttribute('aria-expanded', val ? 'true' : 'false');
    }

    // Hover: keep aria-expanded in sync so CSS transition fires correctly
    menu.addEventListener('mouseenter', function() { setExpanded(true); });
    menu.addEventListener('mouseleave', function() { setExpanded(false); });

    // Click/tap: toggle (primary interaction on touch devices)
    btn.addEventListener('click', function(e) {
      e.stopPropagation();
      setExpanded(btn.getAttribute('aria-expanded') !== 'true');
    });

    // Close when clicking outside (touch: tap outside)
    document.addEventListener('click', function(e) {
      if (!menu.contains(e.target)) setExpanded(false);
    });

    document.addEventListener('keydown', function(e) {
      if (e.key === 'Escape') setExpanded(false);
    });
  });
})();

// Sidebar toggle (mobile)
(function() {
  var MOBILE_BP = 1024;

  function closeSidebar(btn) {
    document.body.classList.remove('sidebar-open');
    if (btn) btn.setAttribute('aria-expanded', 'false');
  }

  document.addEventListener('DOMContentLoaded', function() {
    var btn = document.getElementById('hamburger-btn');
    if (!btn) return;

    // Open/close on hamburger click
    btn.addEventListener('click', function() {
      var open = document.body.classList.toggle('sidebar-open');
      btn.setAttribute('aria-expanded', open ? 'true' : 'false');
    });

    // Close when clicking the overlay (::after pseudo-element covers body)
    document.body.addEventListener('click', function(e) {
      if (document.body.classList.contains('sidebar-open') && e.target === document.body) {
        closeSidebar(btn);
      }
    });

    // Close when navigating via htmx
    document.addEventListener('htmx:beforeRequest', function() {
      closeSidebar(btn);
    });

    // Close when viewport grows beyond the mobile breakpoint so the
    // sidebar-open class and overlay don't linger at desktop widths.
    var mq = window.matchMedia('(max-width: ' + MOBILE_BP + 'px)');
    function onBreakpointChange(e) {
      if (!e.matches) closeSidebar(btn);
    }
    if (mq.addEventListener) {
      mq.addEventListener('change', onBreakpointChange);
    } else {
      mq.addListener(onBreakpointChange); // Safari <14 fallback
    }
  });
})();
