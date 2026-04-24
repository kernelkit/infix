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

    var status = overlay.querySelector('#reboot-status');

    // Hard cap: redirect home after 60 s regardless of poll outcome.
    setTimeout(function () { window.location.replace('/'); }, 60000);

    // Wait 4 s for the device to shut down, then start polling for it to come back.
    // This avoids the race where a fast reboot never shows as "down" on the 1 s poll.
    setTimeout(function () {
      if (status) status.textContent = 'Waiting for device to come back\u2026';
      function poll() {
        fetch('/device-status').then(function (r) {
          if (r.ok) {
            window.location.replace('/');
          } else {
            setTimeout(poll, 2000);
          }
        }).catch(function () {
          setTimeout(poll, 2000);
        });
      }
      poll();
    }, 8000);
  }

  function initDynamicUI(root) {
    initProgressBars(root);
    startRebootOverlay(root);
    initDatetimePicker(root);
    fwBootInit(root);
    fwUploadInit(root);
    initRestoreCheckbox(root);
    initYangTree(root);
  }

  function initYangTree(scope) {
    var root = scope || document;
    // Stop <details> from toggling when clicking interactive elements inside <summary>.
    // Also stops row-click navigation when action buttons inside rows are clicked.
    root.querySelectorAll('.yt-sp').forEach(function (el) {
      if (el.dataset.init) return;
      el.dataset.init = 'true';
      el.addEventListener('click', function (e) { e.stopPropagation(); });
    });
    // Binary content show/hide eye button.
    root.querySelectorAll('.yt-binary-eye').forEach(function (btn) {
      if (btn.dataset.init) return;
      btn.dataset.init = 'true';
      btn.addEventListener('click', function () {
        var wrap = btn.closest('.yt-binary-wrap');
        if (wrap) wrap.classList.toggle('yt-revealed');
      });
    });
  }

  function fwUploadInit(scope) {
    var btn = (scope || document).getElementById('fw-upload-btn');
    if (!btn || btn.dataset.init) return;
    btn.dataset.init = 'true';
    btn.addEventListener('click', window.fwUpload);
  }

  function initRestoreCheckbox(scope) {
    var cb = (scope || document).getElementById('restore-startup-cb');
    if (!cb || cb.dataset.init) return;
    cb.dataset.init = 'true';
    cb.addEventListener('change', function () { window.scRestoreCheckbox(cb); });
  }

  // ─── Firmware: boot order drag-and-drop ──────────────────────────────────
  function fwBootInit(scope) {
    var slots = (scope || document).querySelector('#fw-boot-slots');
    if (!slots || slots.dataset.dndInit) return;
    slots.dataset.dndInit = 'true';

    var dragging = null;
    var insertRef = undefined; // node to insertBefore; undefined = not set, null = append

    function clearIndicators() {
      slots.querySelectorAll('.fw-boot-drop-before').forEach(function (el) {
        el.classList.remove('fw-boot-drop-before');
      });
    }

    slots.addEventListener('dragstart', function (e) {
      dragging = e.target.closest('.fw-boot-badge');
      if (!dragging) return;
      dragging.classList.add('fw-boot-dragging');
      e.dataTransfer.effectAllowed = 'move';
    });

    slots.addEventListener('dragend', function () {
      if (dragging) dragging.classList.remove('fw-boot-dragging');
      dragging = null;
      insertRef = undefined;
      clearIndicators();
    });

    slots.addEventListener('dragenter', function (e) { e.preventDefault(); });

    slots.addEventListener('dragover', function (e) {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      if (!dragging) return;
      var target = e.target.closest('.fw-boot-badge');
      clearIndicators();
      if (!target || target === dragging) return;
      var rect = target.getBoundingClientRect();
      if (e.clientX < rect.left + rect.width / 2) {
        insertRef = target;
        target.classList.add('fw-boot-drop-before');
      } else {
        insertRef = target.nextElementSibling || null;
        if (insertRef && insertRef.classList.contains('fw-boot-badge')) {
          insertRef.classList.add('fw-boot-drop-before');
        }
      }
    });

    slots.addEventListener('drop', function (e) {
      e.preventDefault();
      if (!dragging || insertRef === undefined) return;
      slots.insertBefore(dragging, insertRef); // insertRef===null appends to end
      clearIndicators();
      insertRef = undefined;
    });

    var saveBtn = document.getElementById('fw-boot-save-btn');
    if (saveBtn) saveBtn.addEventListener('click', function () { window.fwBootSave(saveBtn); });
  }

  window.fwBootSave = function (btn) {
    var badges = document.querySelectorAll('#fw-boot-slots .fw-boot-badge');
    var params = new URLSearchParams();
    badges.forEach(function (b) { params.append('boot-order', b.dataset.slot); });

    function btnSet(text, disabled) {
      if (!btn) return;
      btn.textContent = text;
      btn.disabled = disabled;
    }

    btnSet('Setting\u2026', true);

    fetch('/firmware/boot-order', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-CSRF-Token': getCSRFToken(),
      },
      body: params.toString(),
    }).then(function (r) {
      if (r.ok) {
        btnSet('\u2713 Set', true);
        setTimeout(function () { btnSet('Set', false); }, 2000);
        return;
      }
      return r.text().then(function (t) {
        btnSet('\u2717 ' + (t.replace(/<[^>]*>/g, '').trim() || 'Failed'), false);
        setTimeout(function () { btnSet('Set', false); }, 4000);
      });
    }).catch(function () {
      btnSet('\u2717 Failed', false);
      setTimeout(function () { btnSet('Set', false); }, 4000);
    });
  };

  // ─── System Control: datetime picker ─────────────────────────────────────
  // Pre-fills #sc-dt-input with the browser's current UTC time on page load
  // and after HTMX swaps.  Also exposed as window.scSyncTime() for the
  // "Browser time" button.
  function utcDatetimeLocal() {
    var d = new Date();
    return d.getUTCFullYear() + '-' +
      String(d.getUTCMonth() + 1).padStart(2, '0') + '-' +
      String(d.getUTCDate()).padStart(2, '0') + 'T' +
      String(d.getUTCHours()).padStart(2, '0') + ':' +
      String(d.getUTCMinutes()).padStart(2, '0');
  }

  function initDatetimePicker(root) {
    var el = (root || document).querySelector('#sc-dt-input:not([data-init])');
    if (!el) return;
    el.dataset.init = 'true';
    el.value = utcDatetimeLocal();
    var btn = (root || document).getElementById('sc-sync-time-btn');
    if (btn) btn.addEventListener('click', function () { el.value = utcDatetimeLocal(); });
  }

  window.scRestoreCheckbox = function (cb) {
    var form = document.getElementById('restore-form');
    if (!form) return;
    form.setAttribute('hx-confirm', cb.checked
      ? 'Save configuration to startup? Reboot required to apply.'
      : 'Apply this configuration to the running system?');
  };

  window.fwUpload = function () {
    var fileInput = document.getElementById('fw-file');
    if (!fileInput || !fileInput.files.length) return;
    if (!confirm('Upload and install this firmware? The current installation may be overwritten.')) return;

    var autoReboot = document.getElementById('fw-upload-auto-reboot');
    var btn        = document.getElementById('fw-upload-btn');
    var wrap       = document.getElementById('fw-upload-progress');
    var bar        = document.getElementById('fw-upload-bar');
    var text       = document.getElementById('fw-upload-text');

    var formData = new FormData();
    formData.append('pkg', fileInput.files[0]);
    if (autoReboot && autoReboot.checked) formData.append('auto-reboot', '1');

    var xhr = new XMLHttpRequest();

    xhr.upload.onprogress = function (e) {
      if (e.lengthComputable) {
        var pct = Math.round(e.loaded / e.total * 100);
        bar.style.width = pct + '%';
        text.textContent = 'Uploading\u2026 ' + pct + '%';
      }
    };

    xhr.onload = function () {
      if (xhr.status === 200) {
        window.location.href = xhr.responseText.trim();
      } else {
        text.textContent = 'Failed: ' + xhr.responseText;
        btn.disabled = false;
      }
    };

    xhr.onerror = function () {
      text.textContent = 'Upload failed \u2014 network error.';
      btn.disabled = false;
    };

    xhr.open('POST', '/firmware/upload');
    xhr.setRequestHeader('X-CSRF-Token', getCSRFToken());

    btn.disabled = true;
    wrap.hidden = false;
    bar.style.width = '0%';
    text.textContent = 'Uploading\u2026';
    xhr.send(formData);
  };

  // SSE-driven firmware progress card.
  // The Go server polls RESTCONF and streams rendered HTML fragments; we just
  // swap them into the card and let the server close the stream when done.
  var fwEventSource = null;

  function initFirmwareProgress(root) {
    var scope = root || document;
    var card = scope.querySelector('#fw-progress-card[data-sse-src]');
    if (!card || card.dataset.sseInit) return;
    card.dataset.sseInit = 'true';

    var src = card.getAttribute('data-sse-src');
    if (fwEventSource) { fwEventSource.close(); }
    fwEventSource = new EventSource(src);

    function swap(html) {
      card.innerHTML = html;
      initProgressBars(card);
      if (window.htmx) htmx.process(card);
    }

    fwEventSource.addEventListener('progress', function(e) { swap(e.data); });

    fwEventSource.addEventListener('done', function(e) {
      swap(e.data);
      fwEventSource.close();
      fwEventSource = null;
    });

    fwEventSource.addEventListener('reboot', function(e) {
      swap(e.data);
      fwEventSource.close();
      fwEventSource = null;
      // Auto-reboot: POST /reboot and show the reboot overlay.
      fetch('/reboot', { method: 'POST', headers: { 'X-CSRF-Token': getCSRFToken() } })
        .then(function(r) { return r.text(); })
        .then(function(html) {
          var content = document.getElementById('content');
          if (content) {
            content.innerHTML = html;
            initDynamicUI(content);
          }
        });
    });

    fwEventSource.onerror = function() {
      fwEventSource.close();
      fwEventSource = null;
    };
  }

  document.addEventListener('DOMContentLoaded', function() {
    initCSRF();
    initDeviceStatusBanner();
    initDynamicUI(document);
    initFirmwareProgress(document);
  });

  if (window.htmx && document.body) {
    document.body.addEventListener('htmx:afterSwap', function (evt) {
      // Close any open SSE stream if the firmware progress card is no longer present.
      if (fwEventSource && !document.getElementById('fw-progress-card')) {
        fwEventSource.close();
        fwEventSource = null;
      }
      initDynamicUI(evt.target);
      initFirmwareProgress(evt.target);
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
          // Don't auto-open Configure: its toggle handler fires enterConfigure(),
          // which must only happen on explicit user interaction, not on URL sync.
          if (d.id !== 'nav-configure' && !d.open) d.open = true;
        } else {
          if (d.open) d.open = false;
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
  var bar, showTimer, hideTimer, inFlight = 0, visible = false;

  function getBar() {
    if (!bar) bar = document.getElementById('page-progress');
    return bar;
  }

  function show() {
    visible = true;
    var b = getBar();
    if (!b) return;
    clearTimeout(hideTimer); hideTimer = null;
    b.style.transition = 'none';
    b.style.width = '0%';
    b.style.opacity = '1';
    b.offsetWidth; // force reflow
    b.style.transition = 'width 6s cubic-bezier(0.05, 0.8, 0.4, 1)';
    b.style.width = '85%';
  }

  function hide() {
    visible = false;
    var b = getBar();
    if (!b) return;
    clearTimeout(hideTimer);
    b.style.transition = 'width 0.15s ease';
    b.style.width = '100%';
    hideTimer = setTimeout(function() {
      b.style.transition = 'opacity 0.25s ease';
      b.style.opacity = '0';
      hideTimer = setTimeout(function() {
        b.style.transition = 'none';
        b.style.width = '0%';
        hideTimer = null;
      }, 270);
    }, 120);
  }

  function start() {
    inFlight++;
    if (inFlight === 1 && !showTimer) {
      // Only show after 150 ms — fast requests (saves, etc.) won't trigger it.
      showTimer = setTimeout(function() {
        showTimer = null;
        if (inFlight > 0) show();
      }, 150);
    }
  }

  function finish() {
    inFlight = Math.max(0, inFlight - 1);
    if (inFlight > 0) return; // other requests still in flight
    if (showTimer) { clearTimeout(showTimer); showTimer = null; return; }
    if (visible) hide();
  }

  document.addEventListener('htmx:beforeSend',    start);
  // afterRequest fires for all types including hx-swap="none"; afterSettle does not.
  document.addEventListener('htmx:afterRequest',  finish);
  document.addEventListener('htmx:responseError', finish);
  document.addEventListener('htmx:sendError',     finish);
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
// Status and Maintenance persist open/closed state in localStorage.
// Configure is excluded from persistence: its state is controlled by the
// server template.  Configure's Enter POST (running→candidate) is fired
// from JS the first time the accordion opens per page lifecycle.
(function() {
  document.addEventListener('DOMContentLoaded', function() {
    var groups = document.querySelectorAll('details.nav-group-top');
    var configureEntered = false;

    // Fire Configure Enter POST once per page lifecycle (initialises the
    // candidate datastore).  Called when the accordion is opened or when the
    // page loads with the accordion already open (e.g. on a /configure/ page).
    function enterConfigure() {
      if (configureEntered) return;
      configureEntered = true;
      htmx.ajax('POST', '/configure/enter', {swap: 'none', target: document.body});
    }

    // If the Configure accordion is already open on page load, fire Enter now.
    var configureDetails = document.getElementById('nav-configure');
    if (configureDetails && configureDetails.open) {
      enterConfigure();
    }

    // Restore saved state — skip Configure so it never auto-reopens on page load.
    // When closing an accordion, don't close it if the active page is inside it
    // (updateActiveNav has already run at this point and set .nav-link.active).
    // Mark elements being programmatically restored so the toggle handler below
    // skips auto-navigation for these synthetic toggles (toggle fires async).
    groups.forEach(function(d) {
      if (d.id === 'nav-configure') return;
      var label = d.querySelector(':scope > summary');
      if (!label) return;
      var key = 'nav-top:' + label.textContent.trim();
      var saved = localStorage.getItem(key);
      if (saved === 'open') {
        d.dataset.navRestoring = 'true';
        d.setAttribute('open', '');
      } else if (saved === 'closed' && !d.querySelector('.nav-link.active')) {
        d.removeAttribute('open');
      }
    });

    // Mutual exclusion, persistence, and auto-navigation to first page link
    groups.forEach(function(d) {
      var label = d.querySelector(':scope > summary');
      if (!label) return;
      var key = 'nav-top:' + label.textContent.trim();
      d.addEventListener('toggle', function() {
        // Skip synthetic toggles fired during page-load state restoration.
        if (d.dataset.navRestoring) {
          delete d.dataset.navRestoring;
          return;
        }
        if (d.id !== 'nav-configure') {
          localStorage.setItem(key, d.open ? 'open' : 'closed');
        }
        if (d.open) {
          if (d.id === 'nav-configure') {
            enterConfigure();
          }
          groups.forEach(function(other) {
            if (other !== d && other.open) {
              other.removeAttribute('open');
              if (other.id !== 'nav-configure') {
                var otherLabel = other.querySelector(':scope > summary');
                if (otherLabel) {
                  localStorage.setItem('nav-top:' + otherLabel.textContent.trim(), 'closed');
                }
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

// Card collapsible — "Show more / Show less" toggle injected when content overflows
(function() {
  function initCollapsibles() {
    document.querySelectorAll('.card-collapsible:not([data-ci])').forEach(function(card) {
      card.setAttribute('data-ci', '1');
      var body = card.querySelector('.card-collapsible-body');
      if (!body || body.scrollHeight <= body.clientHeight + 4) return;

      var btn = document.createElement('button');
      btn.className = 'card-expand-btn';
      btn.type = 'button';
      btn.textContent = 'Show more \u25be';
      btn.style.display = 'block';
      card.appendChild(btn);

      btn.addEventListener('click', function() {
        var expanded = card.classList.toggle('is-expanded');
        btn.textContent = expanded ? 'Show less \u25b4' : 'Show more \u25be';
      });
    });
  }
  document.addEventListener('DOMContentLoaded', initCollapsibles);
  document.addEventListener('htmx:afterSettle', initCollapsibles);
})();

// Firewall zone matrix cell drill-down
(function() {
  var verdictLabel = { allow: '✓ Allow', deny: '✗ Deny', conditional: '⚠ Conditional' };

  document.addEventListener('click', function(e) {
    var td = e.target.closest('.matrix-cell[data-verdict]');
    var panel = document.getElementById('fw-flow-detail');
    if (!panel) return;

    if (!td) {
      panel.hidden = true;
      return;
    }
    panel.querySelector('.fw-detail-flow').textContent =
      td.getAttribute('data-from') + ' → ' + td.getAttribute('data-to');
    panel.querySelector('.fw-detail-verdict').textContent =
      verdictLabel[td.getAttribute('data-verdict')] || td.getAttribute('data-verdict');
    panel.querySelector('.fw-detail-text').textContent = td.getAttribute('data-detail') || '';
    panel.hidden = false;
  });
})();

// Keystore key detail row toggle
(function() {
  document.addEventListener('click', function(e) {
    var btn = e.target.closest('.key-row-toggle');
    if (!btn) return;
    var target = btn.getAttribute('data-target');
    var row = document.getElementById('key-detail-' + target);
    if (!row) return;
    var open = row.classList.toggle('is-open');
    btn.setAttribute('aria-expanded', open ? 'true' : 'false');
  });
})();

// mDNS extra-address row toggle
(function() {
  document.addEventListener('click', function(e) {
    var btn = e.target.closest('.mdns-addr-toggle');
    if (!btn) return;
    var group = btn.getAttribute('data-group');
    var expanded = btn.getAttribute('aria-expanded') === 'true';
    btn.setAttribute('aria-expanded', expanded ? 'false' : 'true');
    document.querySelectorAll('[data-mdns-extra="' + group + '"]').forEach(function(row) {
      row.style.display = expanded ? '' : 'table-row';
    });
  });
})();

// Bridge/LAG member row collapse toggle
(function() {
  document.addEventListener('click', function(e) {
    var btn = e.target.closest('.bridge-toggle');
    if (!btn) return;
    var group = btn.getAttribute('data-group');
    var parentRow = btn.closest('tr');
    var collapsed = parentRow.classList.toggle('bridge-collapsed');
    btn.setAttribute('aria-expanded', collapsed ? 'false' : 'true');
    document.querySelectorAll('[data-bridge-member="' + group + '"]').forEach(function(row) {
      row.style.display = collapsed ? 'none' : '';
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

// ─── Login page: progress bar on submit ────────────────────────────────────
// The login form is a native POST (not HTMX), so htmx:beforeSend never fires.
// Start the bar and disable the button to show the user something is happening.
(function() {
  document.addEventListener('DOMContentLoaded', function() {
    var form = document.querySelector('form[action="/login"]');
    if (!form) return;
    form.addEventListener('submit', function() {
      var bar = document.getElementById('page-progress');
      if (bar) {
        bar.style.transition = 'none';
        bar.style.width = '0%';
        bar.style.opacity = '1';
        bar.offsetWidth;
        bar.style.transition = 'width 8s cubic-bezier(0.05, 0.8, 0.4, 1)';
        bar.style.width = '85%';
      }
      var btn = form.querySelector('button[type="submit"]');
      if (btn) { btn.disabled = true; btn.textContent = 'Logging in\u2026'; }
    });
  });
})();

// ─── Confirm dialog ────────────────────────────────────────────────────────
// openModal(message, onConfirm) shows the shared <dialog> and calls onConfirm
// if the user clicks Confirm, or does nothing on Cancel.
function openModal(message, onConfirm) {
  var dlg = document.getElementById('confirm-dialog');
  var msg = document.getElementById('dialog-message');
  var ok  = document.getElementById('dialog-confirm');
  var no  = document.getElementById('dialog-cancel');
  if (!dlg) { onConfirm(); return; } // fallback if dialog missing
  msg.textContent = message;

  function cleanup() {
    ok.removeEventListener('click', handleOK);
    no.removeEventListener('click', handleNo);
    dlg.close();
  }
  function handleOK()  { cleanup(); onConfirm(); }
  function handleNo()  { cleanup(); }

  ok.addEventListener('click', handleOK);
  no.addEventListener('click', handleNo);
  dlg.showModal();
}

// ─── data-show / data-hide: toggle element visibility ──────────────────────
(function() {
  document.addEventListener('click', function(e) {
    var show = e.target.closest('[data-show]');
    if (show) {
      var el = document.getElementById(show.getAttribute('data-show'));
      if (el) { el.hidden = false; el.querySelector('input,select,textarea') && el.querySelector('input,select,textarea').focus(); }
    }
    var hide = e.target.closest('[data-hide]');
    if (hide) {
      var el = document.getElementById(hide.getAttribute('data-hide'));
      if (el) el.hidden = true;
    }
  });
})();

// ─── YANG tree: browser back/forward navigation for detail pane ─────────────
(function() {
  var pendingPath = null;
  var observedEl = null;
  var observer = null;

  // Track which node path the user intends to navigate to.
  document.addEventListener('click', function(e) {
    var el = e.target.closest('[data-yang-path]');
    if (el) pendingPath = el.getAttribute('data-yang-path');
  });

  // MutationObserver on #yang-detail: when its direct children change, a new
  // node was loaded.  Push a history entry only when a navigation click caused it.
  function onDetailMutated() {
    if (!pendingPath) return;
    var path = pendingPath;
    pendingPath = null;
    history.pushState({ yangDetailPath: path }, '',
      window.location.pathname + '?node=' + encodeURIComponent(path));
  }

  function attachObserver() {
    var detail = document.getElementById('yang-detail');
    if (!detail || detail === observedEl) return;
    if (observer) observer.disconnect();
    observedEl = detail;
    observer = new MutationObserver(onDetailMutated);
    observer.observe(detail, { childList: true });
  }

  window.addEventListener('popstate', function(e) {
    var detail = document.getElementById('yang-detail');
    if (!detail) return;
    if (e.state && e.state.yangDetailPath) {
      if (window.htmx) {
        htmx.ajax('GET', '/configure/tree/node?path=' + encodeURIComponent(e.state.yangDetailPath),
          { target: '#yang-detail', swap: 'innerHTML' });
      }
    } else {
      detail.innerHTML = '';
    }
  });

  document.addEventListener('DOMContentLoaded', function() {
    attachObserver();
    var detail = document.getElementById('yang-detail');
    if (!detail || !window.htmx) return;
    var node = new URLSearchParams(window.location.search).get('node');
    if (node) {
      htmx.ajax('GET', '/configure/tree/node?path=' + encodeURIComponent(node),
        { target: '#yang-detail', swap: 'innerHTML' });
    }
  });

  // Re-attach after HTMX page navigation recreates #content (and #yang-detail).
  document.addEventListener('htmx:afterSwap', attachObserver);
})();

// ─── Configure: dynamic list row add/delete ────────────────────────────────
// Handles .btn-add-row (data-table, data-template) and .cfg-delete-row buttons.
// Templates are keyed by data-template attribute value.
(function () {
  var rowTemplates = {
    'ntp': function(i) {
      return '<tr>' +
        '<td><input class="cfg-input" type="text" name="ntp_name_' + i + '" required></td>' +
        '<td><input class="cfg-input" type="text" name="ntp_addr_' + i + '"></td>' +
        '<td><input class="cfg-input cfg-input-sm" type="number" min="1" max="65535"' +
             ' name="ntp_port_' + i + '" placeholder="123"></td>' +
        '<td style="text-align:center"><input type="checkbox" name="ntp_prefer_' + i + '"></td>' +
        '<td>' + deleteBtn() + '</td>' +
        '</tr>';
    },
    'dns-search': function(i) {
      return '<tr>' +
        '<td><input class="cfg-input" type="text" name="dns_search_' + i + '"></td>' +
        '<td>' + deleteBtn() + '</td>' +
        '</tr>';
    },
    'dns-server': function(i) {
      return '<tr>' +
        '<td><input class="cfg-input" type="text" name="dns_name_' + i + '" required></td>' +
        '<td><input class="cfg-input" type="text" name="dns_addr_' + i + '"></td>' +
        '<td><input class="cfg-input cfg-input-sm" type="number" min="1" max="65535"' +
             ' name="dns_port_' + i + '" placeholder="53"></td>' +
        '<td>' + deleteBtn() + '</td>' +
        '</tr>';
    }
  };

  function deleteBtn() {
    return '<button type="button" class="btn-icon btn-icon-danger cfg-delete-row" title="Remove">' +
      '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"' +
      ' stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
      '<line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>' +
      '</svg></button>';
  }

  // Re-index all inputs in a tbody so names stay sequential after add/delete.
  function renumber(tbody) {
    tbody.querySelectorAll('tr').forEach(function(row, i) {
      row.querySelectorAll('input').forEach(function(inp) {
        inp.name = inp.name.replace(/_\d+$/, '_' + i);
      });
    });
  }

  document.addEventListener('click', function(e) {
    var addBtn = e.target.closest('.btn-add-row');
    if (addBtn) {
      var tbodyId  = addBtn.getAttribute('data-table');
      var tmplKey  = addBtn.getAttribute('data-template');
      var tbody    = document.getElementById(tbodyId);
      if (!tbody || !rowTemplates[tmplKey]) { return; }
      var idx = tbody.querySelectorAll('tr').length;
      tbody.insertAdjacentHTML('beforeend', rowTemplates[tmplKey](idx));
      var newInput = tbody.querySelector('tr:last-child input');
      if (newInput) { newInput.focus(); }
      return;
    }

    var delBtn = e.target.closest('.cfg-delete-row');
    if (delBtn) {
      var row   = delBtn.closest('tr');
      var tbody = row && row.closest('tbody');
      if (row) { row.remove(); }
      if (tbody) { renumber(tbody); }
    }
  });
})();

// ─── YANG tree accordion ───────────────────────────────────────────────────
// When any node opens, collapse its siblings at the same level so only one
// subtree is expanded at a time.  Works at every depth (top-level modules,
// list instances, nested containers).  toggle doesn't bubble — use capture.
(function() {
  document.addEventListener('toggle', function(e) {
    var node = e.target;
    if (!node.classList || !node.classList.contains('yt-node') || !node.open) return;
    var li = node.parentElement;
    var ul = li && li.parentElement;
    if (!ul) return;
    ul.querySelectorAll(':scope > li > details.yt-node').forEach(function(d) {
      if (d !== node && d.open) d.removeAttribute('open');
    });
  }, true);
})();

// ─── ⓘ field-info tooltip (position:fixed to escape overflow clipping) ───────
(function() {
  var tip = null;

  function getTip() {
    if (!tip) {
      tip = document.createElement('div');
      tip.id = 'field-tip';
      document.body.appendChild(tip);
    }
    return tip;
  }

  document.addEventListener('mouseover', function(e) {
    var el = e.target.closest('.field-info[data-tip]');
    if (!el) return;
    var t = getTip();
    t.textContent = el.getAttribute('data-tip');
    t.style.display = 'block';
    var r = el.getBoundingClientRect();
    // Position above the icon, centred; clamp to viewport edges.
    var left = r.left + r.width / 2 - t.offsetWidth / 2;
    var top  = r.top - t.offsetHeight - 6;
    if (left < 8) left = 8;
    if (left + t.offsetWidth > window.innerWidth - 8) left = window.innerWidth - t.offsetWidth - 8;
    if (top < 8) top = r.bottom + 6; // flip below if no room above
    t.style.left = left + 'px';
    t.style.top  = top  + 'px';
  });

  document.addEventListener('mouseout', function(e) {
    var el = e.target.closest('.field-info[data-tip]');
    if (!el) return;
    var t = getTip();
    t.style.display = 'none';
  });
})();

// ─── Configure interaction log ─────────────────────────────────────────────
// Persists save/error events in sessionStorage so they survive page reloads
// (Apply/Abort both do HX-Refresh).  Cleared on logout.
var cfgLogEntries = (function() {
  try { return JSON.parse(sessionStorage.getItem('cfgLog') || '[]'); } catch(e) { return []; }
})();

function cfgIsoNow() {
  var d = new Date();
  return d.getFullYear() + '-' +
    String(d.getMonth() + 1).padStart(2, '0') + '-' +
    String(d.getDate()).padStart(2, '0') + ' ' +
    String(d.getHours()).padStart(2, '0') + ':' +
    String(d.getMinutes()).padStart(2, '0') + ':' +
    String(d.getSeconds()).padStart(2, '0');
}

function cfgLog(level, msg) {
  cfgLogEntries.push({ level: level, msg: msg, ts: cfgIsoNow() });
  if (cfgLogEntries.length > 200) cfgLogEntries.shift();
  try { sessionStorage.setItem('cfgLog', JSON.stringify(cfgLogEntries)); } catch(e) {}
  var badge = document.getElementById('cfg-log-badge');
  if (badge && level === 'error') {
    badge.hidden = false;
    badge.textContent = cfgLogEntries.filter(function(e) { return e.level === 'error'; }).length;
  }
}

// Restore error badge count after a page reload (entries came from sessionStorage).
document.addEventListener('DOMContentLoaded', function() {
  var errCount = cfgLogEntries.filter(function(e) { return e.level === 'error'; }).length;
  if (errCount > 0) {
    var badge = document.getElementById('cfg-log-badge');
    if (badge) { badge.hidden = false; badge.textContent = errCount; }
  }
  // Clear log on logout so the next session starts fresh.
  document.addEventListener('submit', function(e) {
    if (e.target.closest('form[action="/logout"]')) {
      try { sessionStorage.removeItem('cfgLog'); } catch(e2) {}
    }
  });
});

function renderCfgLog() {
  var panel = document.getElementById('cfg-log-panel');
  if (!panel) return;
  if (cfgLogEntries.length === 0) {
    panel.querySelector('.cfg-log-list').innerHTML = '<li class="cfg-log-empty">No activity yet.</li>';
    return;
  }
  var html = cfgLogEntries.slice().reverse().map(function(e) {
    return '<li class="cfg-log-entry cfg-log-' + e.level + '">' +
           '<span class="cfg-log-ts">' + e.ts + '</span> ' +
           '<span class="cfg-log-msg">' + e.msg.replace(/</g, '&lt;') + '</span></li>';
  }).join('');
  panel.querySelector('.cfg-log-list').innerHTML = html;
}

// ─── Configure toolbar ─────────────────────────────────────────────────────
// Intercept HTMX's confirm event for toolbar buttons and show the custom modal.
// Apply / Apply & Save / Abort / Save-to-startup all respond with HX-Refresh.
(function () {
  var toolbarLabels = {
    'cfg-apply-btn':        'Applied staged changes to running config',
    'cfg-apply-save-btn':   'Applied and saved to startup config',
    'cfg-abort-btn':        'Aborted: candidate reset to running config',
    'cfg-unsaved-save-btn': 'Saved running config to startup',
  };
  document.addEventListener('htmx:confirm', function(e) {
    var btn = e.detail.elt;
    if (!btn) return;
    var label = null;
    for (var cls in toolbarLabels) {
      if (btn.classList.contains(cls)) { label = toolbarLabels[cls]; break; }
    }
    if (!label) return;
    e.preventDefault();
    openModal(e.detail.question, function() {
      cfgLog('ok', label);
      e.detail.issueRequest(true);
    });
  });

  // Log panel toggle / close.
  document.addEventListener('click', function(e) {
    if (e.target.closest('.cfg-log-close')) {
      var panel = document.getElementById('cfg-log-panel');
      if (panel) panel.hidden = true;
      return;
    }
    var btn = e.target.closest('#cfg-log-btn');
    if (!btn) return;
    var panel = document.getElementById('cfg-log-panel');
    if (!panel) return;
    panel.hidden = !panel.hidden;
    if (!panel.hidden) {
      renderCfgLog();
      var badge = document.getElementById('cfg-log-badge');
      if (badge) badge.hidden = true;
    }
  });

  // cfgError: show error message in the .cfg-save-status span of the submitting form.
  document.addEventListener('cfgError', function(e) {
    var msg = e.detail && e.detail.value ? e.detail.value : 'Save failed';
    var form = e.target && e.target.closest('form');
    var span = form ? form.querySelector('.cfg-save-status') : null;
    if (span) {
      span.textContent = '✗ ' + msg;
      span.classList.add('error');
      cfgLog('error', msg);
      // Keep error visible until dismissed (click) or 30 s timeout.
      var tid = setTimeout(function() {
        span.textContent = '';
        span.classList.remove('error');
      }, 30000);
      span.addEventListener('click', function once() {
        clearTimeout(tid);
        span.textContent = '';
        span.classList.remove('error');
        span.removeEventListener('click', once);
      });
    }
  });

  // Show "Saved ✓" feedback when a card Save succeeds.
  (function() {
    var LS_KEY = 'fw-url-history';
    var MAX_HIST = 10;

    function loadHistory() {
      try { return JSON.parse(localStorage.getItem(LS_KEY) || '[]'); }
      catch (e) { return []; }
    }

    function saveURL(url) {
      var hist = loadHistory().filter(function(u) { return u !== url; });
      hist.unshift(url);
      if (hist.length > MAX_HIST) hist = hist.slice(0, MAX_HIST);
      localStorage.setItem(LS_KEY, JSON.stringify(hist));
    }

    function populateDatalist() {
      var dl = document.getElementById('fw-url-history');
      if (!dl) return;
      dl.innerHTML = loadHistory().map(function(u) {
        return '<option value="' + u.replace(/&/g, '&amp;').replace(/"/g, '&quot;') + '">';
      }).join('');
    }

    document.addEventListener('DOMContentLoaded', populateDatalist);
    document.addEventListener('htmx:afterSettle', populateDatalist);

    document.addEventListener('submit', function(e) {
      var form = e.target.closest('.firmware-form');
      if (!form) return;
      var input = form.querySelector('input[name="url"]');
      if (input && input.value) saveURL(input.value);
    });
  })();

  document.addEventListener('cfgSaved', function(e) {
    var msg = e.detail && e.detail.value ? e.detail.value : 'Saved';
    cfgLog('ok', msg);
    // Find the status span closest to the form that triggered the event.
    var form = e.target && e.target.closest('form');
    var span = form ? form.querySelector('.cfg-save-status') : null;
    if (span) {
      span.textContent = '✓ ' + msg;
      span.classList.add('saved');
      setTimeout(function() {
        span.textContent = '';
        span.classList.remove('saved');
      }, 3000);
    }
  });
})();
