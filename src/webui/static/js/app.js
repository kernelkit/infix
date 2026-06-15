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

  // HTMX page navigation swaps only #content, leaving the <title> in <head>
  // stale.  Each rendered page emits a setPageTitle HX-Trigger event (set in
  // handlers/common.go newPageData) carrying the full "Page · Context" title
  // string ready to apply.
  function initPageTitle() {
    document.body.addEventListener('setPageTitle', function (evt) {
      if (evt.detail && typeof evt.detail.value === 'string') {
        document.title = evt.detail.value;
      }
    });
  }

  // The banner reflects htmx request outcomes — both user interactions and
  // the `hx-trigger="every 5s"` watchdog div in base.html.
  //
  // The watchdog request gets a 5 s timeout (via configRequest), bounding
  // disconnect detection at ~5 s + 5 s. Without it the XHR would sit on the
  // OS TCP timeout (1–2 min) before any error fires.
  //
  // Once we see a failure, a dead-man timer counts down to a forced
  // navigation to /login.  The device may have been rebooted or upgraded
  // out of band; in that case its in-memory session is gone and the next
  // poll would 401 → HX-Redirect → /login automatically.  But if the
  // device is truly unreachable (cable pulled, route flap), no response
  // ever comes, so we time out client-side instead of letting the user
  // stare at a stale page.
  function initDeviceStatusBanner() {
    var banner = document.getElementById('conn-banner');
    if (!banner) return;

    var REDIRECT_AFTER_MS = 90 * 1000;
    var firstFailureMs = null;
    var tickerId = null;

    function tick() {
      var remaining = Math.max(0, Math.ceil(
        (firstFailureMs + REDIRECT_AFTER_MS - Date.now()) / 1000
      ));
      banner.textContent =
        'Device unreachable — returning to login in ' + remaining + ' s';
      if (remaining === 0) {
        clearInterval(tickerId);
        window.location.replace('/login');
      }
    }

    function show() {
      banner.hidden = false;
      if (tickerId !== null) return;
      firstFailureMs = Date.now();
      tick();
      tickerId = setInterval(tick, 1000);
    }

    // Reconnect: the device is answering again after a disconnect.  It may
    // have rebooted or been upgraded out of band, in which case
    // running-config reverted to startup (activated-but-unsaved changes
    // are gone), operational data is stale, and the in-memory session may
    // no longer exist.  So we do NOT resume the page the user was on —
    // we reload it.  A live session re-fetches true post-reboot state; a
    // dead one makes the server 303-redirect the navigation to /login.
    // The watchdog only fires `show()` after a >=5 s outage, so this never
    // triggers on a sub-second blip — only on outages long enough to
    // plausibly be a reboot.
    function reconnect() {
      if (firstFailureMs === null) return; // we were never down
      banner.hidden = false;
      banner.textContent = 'Device back online — reloading…';
      if (tickerId !== null) {
        clearInterval(tickerId);
        tickerId = null;
      }
      window.location.reload();
    }

    document.addEventListener('htmx:configRequest', function (evt) {
      if (evt.detail && evt.detail.path === '/device-status') {
        evt.detail.timeout = 5000;
      }
    });
    document.addEventListener('htmx:sendError', show);
    document.addEventListener('htmx:timeout',   show);
    document.addEventListener('htmx:responseError', function (evt) {
      var s = evt.detail && evt.detail.xhr && evt.detail.xhr.status;
      if (s === 502 || s === 503 || s === 504) show();
    });
    document.addEventListener('htmx:afterRequest', function (evt) {
      if (!evt.detail) return;
      var xhr = evt.detail.xhr;
      var status = xhr ? xhr.status : 0;
      // Any HTTP response < 500 means the server is reachable again.
      // status === 0 = no response; leave it to sendError / timeout.
      if (status > 0 && status < 500) reconnect();
    });
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
    swBootInit(root);
    swUploadInit(root);
    initRestoreCheckbox(root);
    initYangTree(root);
    initMultiDropdown(root);
    initLogsFilter(root);
    initLogsTail(root);
  }

  // Active EventSource for the live-tail stream, if any.  Module-scoped
  // so the htmx:afterSwap handler can tear it down on tab change — the
  // old tail-checkbox is replaced by the new card, but the underlying
  // stream would otherwise keep running until the server times out.
  var logsTailES = null;

  // Latched true between a Load earlier click and the swap completing.
  // Stays true for a 500ms grace window after the click, then auto-clears
  // — htmx can fire htmx:afterSwap more than once per request (OOB swaps,
  // settle, etc.), and consuming the flag on the first fire would leave
  // a subsequent fire to scroll-to-bottom and undo the preservation.
  // The grace window comfortably covers a normal request RTT but expires
  // quickly enough that a real tab swap right afterwards behaves correctly.
  var loadEarlierInFlight = false;
  var loadEarlierClearTimer = null;

  function teardownLogsTail() {
    if (logsTailES) {
      logsTailES.close();
      logsTailES = null;
    }
  }

  // Helper: returns true when the user is at (or very near) the bottom
  // of a scrollable element.  Used so live-tail only auto-scrolls when
  // the user is already reading the tail; if they've scrolled up to
  // inspect old context, new lines append silently in the background.
  function nearBottom(el) {
    return (el.scrollHeight - el.scrollTop - el.clientHeight) < 40;
  }

  function setTailStatus(label, text, cls) {
    if (!label) return;
    var status = label.querySelector('.logs-tail-status');
    if (!status) return;
    status.textContent = text;
    status.className = 'logs-tail-status' + (cls ? ' ' + cls : '');
  }

  function initLogsTail(root) {
    var scope = root || document;
    scope.querySelectorAll('.logs-tail-cb:not([data-init])').forEach(function (cb) {
      cb.dataset.init = 'true';
      var label = cb.closest('label');

      cb.addEventListener('change', function () {
        teardownLogsTail();
        if (!cb.checked) {
          setTailStatus(label, '', '');
          return;
        }

        var body = cb.closest('.logs-body');
        var srcKey = body && body.getAttribute('data-source');
        var content = body && body.querySelector('.logs-content');
        if (!srcKey || !content) {
          // Surface the failure rather than silently no-op — empty log
          // files have no .logs-content element, and any future markup
          // tweak that drops the data-source attribute would otherwise
          // make tail appear broken without any clue why.
          console.warn('[logs] live-tail aborted', { body: !!body, srcKey: srcKey, content: !!content });
          setTailStatus(label, 'nothing to tail', 'err');
          cb.checked = false;
          return;
        }

        // Toggling tail on always jumps to the end — the user opted in
        // to "show me new stuff," even if they were scrolled up.  Defer
        // to the next frame so the jump lands after the layout settles
        // (the status pill flips to "connecting…" on the same tick).
        var stickToBottom = true;
        requestAnimationFrame(function () {
          content.scrollTop = content.scrollHeight;
        });

        setTailStatus(label, 'connecting…', 'pending');

        logsTailES = new EventSource('/maintenance/logs/' + encodeURIComponent(srcKey) + '/tail');

        // The native open event fires on successful HTTP handshake,
        // which is enough to confirm the TCP/TLS path works even if no
        // log lines are streaming yet.  Useful when nginx buffering
        // would otherwise leave the user staring at a blank tail.
        logsTailES.onopen = function () {
          setTailStatus(label, 'live', 'live');
        };
        logsTailES.addEventListener('lines', function (evt) {
          // Stick to the bottom for the first batch unconditionally (the
          // user just opted in), then fall back to the "only follow if
          // already at the bottom" rule so scrolling up to read pauses
          // the auto-follow.
          var follow = stickToBottom || nearBottom(content);
          stickToBottom = false;
          content.insertAdjacentHTML('beforeend', evt.data);
          if (follow) {
            content.scrollTop = content.scrollHeight;
          }
          setTailStatus(label, 'live', 'live');
        });
        logsTailES.onerror = function () {
          // EventSource auto-reconnects on transient drops — fired
          // commonly under nginx proxy in front of our handler.  Only
          // touch the status when the browser has given up (CLOSED is
          // terminal); leave "live" alone during the auto-reconnect
          // cycle so the badge doesn't flicker between green and amber
          // while new lines are still arriving correctly.
          if (logsTailES && logsTailES.readyState === EventSource.CLOSED) {
            teardownLogsTail();
            cb.checked = false;
            setTailStatus(label, 'disconnected', 'err');
          }
        };
      });
    });
  }

  // Pin the log buffer to the bottom on initial render and after every
  // htmx swap (tab change).  The Load earlier button sits at the top of
  // the same scroll container, so this naturally hides it until the user
  // scrolls back up.  Always query against `document`: with our
  // outerHTML tab swap, htmx hands us the OLD .logs-card as the scope,
  // which is detached from the DOM by the time afterSwap fires.  Note
  // for the SSE wiring step: when the user toggles Live tail on, call
  // this here too.
  function scrollLogsToBottom() {
    document.querySelectorAll('.logs-content').forEach(function (el) {
      el.scrollTop = el.scrollHeight;
    });
  }

  // Move keyboard focus onto the log buffer so PageUp / PageDown /
  // Home / End / arrow keys all work without the user having to click
  // first.  Uses preventScroll because the buffer was just scrolled to
  // the bottom and the default focus() would scroll it back to make
  // the element-top visible.
  function focusLogsContent() {
    var content = document.querySelector('.logs-content');
    if (!content) return;
    try { content.focus({ preventScroll: true }); } catch (_) { content.focus(); }
  }

  // Steal Ctrl-F (and ⌘-F on macOS) when a log filter is on-screen so
  // searching jumps into the filter input rather than opening the
  // browser's native find — the filter actually understands the
  // structured log buffer (regex, severity colors stay aligned).  When
  // no filter is in the DOM the binding does nothing and the browser
  // behaves normally.
  document.addEventListener('keydown', function (evt) {
    if (!evt.key || evt.key.toLowerCase() !== 'f') return;
    if (!(evt.ctrlKey || evt.metaKey)) return;
    if (evt.altKey || evt.shiftKey) return;
    var filter = document.querySelector('.logs-filter');
    if (!filter) return;
    evt.preventDefault();
    filter.focus();
    filter.select();
  });

  // Filter the visible log buffer client-side.  Plain substring by default;
  // /…/ delimiters switch to JS regex (case-insensitive).  Empty query
  // shows everything.  Debounced so per-keystroke regex compile doesn't
  // hitch the UI on a big buffer.
  function initLogsFilter(root) {
    var scope = root || document;
    scope.querySelectorAll('.logs-filter:not([data-init])').forEach(function (input) {
      input.dataset.init = 'true';
      var content = input.closest('.logs-body') && input.closest('.logs-body').querySelector('.logs-content');
      if (!content) return;

      function apply() {
        var q = input.value;
        var re = null;
        var trimmed = q.trim();
        if (trimmed.length >= 2 && trimmed.charAt(0) === '/' && trimmed.charAt(trimmed.length - 1) === '/') {
          try { re = new RegExp(trimmed.slice(1, -1), 'i'); } catch (_) { re = null; }
        }
        var sub = q.toLowerCase();
        content.querySelectorAll('.logs-line').forEach(function (line) {
          var t = line.textContent;
          var show;
          if (re)        show = re.test(t);
          else if (q)    show = t.toLowerCase().indexOf(sub) !== -1;
          else           show = true;
          line.style.display = show ? '' : 'none';
        });
      }

      var timer = null;
      input.addEventListener('input', function () {
        clearTimeout(timer);
        timer = setTimeout(apply, 120);
      });
    });
  }

  // The <details>-based multi-select dropdown needs JS to behave like a
  // real <select>: refresh the summary text when checkboxes change, and
  // close when the user clicks outside the widget.
  function initMultiDropdown(root) {
    var scope = root || document;
    scope.querySelectorAll('.cfg-multi:not([data-init])').forEach(function (det) {
      det.dataset.init = 'true';
      var summary = det.querySelector('.cfg-multi-summary');
      var body = det.querySelector('.cfg-multi-body');
      if (!summary || !body) return;

      var allBox = body.querySelector('input[data-multi-all]');
      var itemBoxes = body.querySelectorAll('input[type="checkbox"]:not([data-multi-all])');

      function refreshSummary() {
        var labels = [];
        itemBoxes.forEach(function (cb) {
          if (!cb.checked) return;
          var lab = cb.closest('label');
          labels.push(lab ? lab.textContent.trim() : cb.value);
        });
        summary.textContent = labels.length ? labels.join(', ') : '(All)';
      }
      body.addEventListener('change', function (evt) {
        if (evt.target === allBox && allBox.checked) {
          // (All) was just checked — clear specific selections.
          itemBoxes.forEach(function (cb) { cb.checked = false; });
        } else if (evt.target !== allBox && evt.target.checked && allBox) {
          // A specific PMD was checked — uncheck (All).
          allBox.checked = false;
        } else if (evt.target !== allBox && allBox) {
          // A specific PMD was unchecked — if none remain, re-check (All).
          var anyChecked = false;
          itemBoxes.forEach(function (cb) { if (cb.checked) anyChecked = true; });
          if (!anyChecked) allBox.checked = true;
        }
        refreshSummary();
      });
    });
  }

  // Close any open .cfg-multi when the user clicks outside it.  Single
  // top-level listener — cheaper than per-widget bindings.
  document.addEventListener('click', function (evt) {
    document.querySelectorAll('.cfg-multi[open]').forEach(function (det) {
      if (!det.contains(evt.target)) det.removeAttribute('open');
    });
  });

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

  function swUploadInit(scope) {
    var btn = (scope || document).querySelector('#sw-upload-btn');
    if (!btn || btn.dataset.init) return;
    btn.dataset.init = 'true';
    btn.addEventListener('click', window.swUpload);
  }

  function initRestoreCheckbox(scope) {
    var cb = (scope || document).querySelector('#restore-startup-cb');
    if (!cb || cb.dataset.init) return;
    cb.dataset.init = 'true';
    cb.addEventListener('change', function () { window.scRestoreCheckbox(cb); });
  }

  // ─── Software: boot order drag-and-drop ──────────────────────────────────
  function swBootInit(scope) {
    var slots = (scope || document).querySelector('#sw-boot-slots');
    if (!slots || slots.dataset.dndInit) return;
    slots.dataset.dndInit = 'true';

    // Stash the page-load order so Reset can restore it without a page refresh.
    // Slot names are a fixed enum (primary | secondary | net), so comma-joining is safe.
    var initialOrder = [];
    slots.querySelectorAll('.sw-boot-badge').forEach(function (b) { initialOrder.push(b.dataset.slot); });
    slots.dataset.originalOrder = initialOrder.join(',');

    var dragging = null;
    var insertRef = undefined; // node to insertBefore; undefined = not set, null = append

    function clearIndicators() {
      slots.querySelectorAll('.sw-boot-drop-before').forEach(function (el) {
        el.classList.remove('sw-boot-drop-before');
      });
    }

    slots.addEventListener('dragstart', function (e) {
      dragging = e.target.closest('.sw-boot-badge');
      if (!dragging) return;
      dragging.classList.add('sw-boot-dragging');
      e.dataTransfer.effectAllowed = 'move';
    });

    slots.addEventListener('dragend', function () {
      if (dragging) dragging.classList.remove('sw-boot-dragging');
      dragging = null;
      insertRef = undefined;
      clearIndicators();
    });

    slots.addEventListener('dragenter', function (e) { e.preventDefault(); });

    slots.addEventListener('dragover', function (e) {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      if (!dragging) return;
      var target = e.target.closest('.sw-boot-badge');
      clearIndicators();
      if (!target || target === dragging) return;
      var rect = target.getBoundingClientRect();
      if (e.clientX < rect.left + rect.width / 2) {
        insertRef = target;
        target.classList.add('sw-boot-drop-before');
      } else {
        insertRef = target.nextElementSibling || null;
        if (insertRef && insertRef.classList.contains('sw-boot-badge')) {
          insertRef.classList.add('sw-boot-drop-before');
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

    var saveBtn = document.getElementById('sw-boot-save-btn');
    if (saveBtn) saveBtn.addEventListener('click', function () { window.swBootSave(saveBtn); });

    var resetBtn = document.getElementById('sw-boot-reset-btn');
    if (resetBtn) resetBtn.addEventListener('click', window.swBootReset);
  }

  // Restore the boot-order row to the page-load order — i.e. what RAUC
  // reported as the current device boot order before any drag/drop.
  // Set will push the displayed order to the device; Reset just undoes
  // local rearrangement without a server round-trip.
  window.swBootReset = function () {
    var slots = document.getElementById('sw-boot-slots');
    if (!slots) return;
    var original = (slots.dataset.originalOrder || '').split(',');
    var existing = {};
    slots.querySelectorAll('.sw-boot-badge').forEach(function (b) {
      existing[b.dataset.slot] = b;
    });
    original.forEach(function (slot) {
      if (existing[slot]) slots.appendChild(existing[slot]);
    });
  };

  window.swBootSave = function (btn) {
    var badges = document.querySelectorAll('#sw-boot-slots .sw-boot-badge');
    var params = new URLSearchParams();
    badges.forEach(function (b) { params.append('boot-order', b.dataset.slot); });

    function btnSet(text, disabled) {
      if (!btn) return;
      btn.textContent = text;
      btn.disabled = disabled;
    }

    btnSet('Setting\u2026', true);

    fetch('/software/boot-order', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-CSRF-Token': getCSRFToken(),
      },
      body: params.toString(),
    }).then(function (r) {
      if (r.ok) {
        // Device now holds the displayed order; refresh the Reset baseline
        // so a follow-up \u21ba doesn't revert to a state the device no longer has.
        var slots = document.getElementById('sw-boot-slots');
        if (slots) {
          var saved = [];
          badges.forEach(function (b) { saved.push(b.dataset.slot); });
          slots.dataset.originalOrder = saved.join(',');
        }
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

  // ─── Configure > Date & Time card ────────────────────────────────────────
  // Two pieces working together:
  //   1. #sc-dt-input is pre-filled with the browser's current local time so
  //      "Set now" defaults to "synchronize device to my clock".
  //   2. #sc-system-time is a JS-driven live clock seeded from the server's
  //      data-server-time attribute (ISO string).  We compute the offset
  //      between device clock and browser clock once, then tick locally and
  //      render via toLocaleString() so each user sees their own locale.
  //      After a successful POST /maintenance/system/datetime we recompute
  //      the offset from the value the user submitted so the visible clock
  //      jumps to the new time and continues ticking.
  var systemTimeOffsetMs = null; // device - browser

  function localDatetimeLocal() {
    // Format Date as YYYY-MM-DDTHH:MM:SS in *local* time, matching what
    // <input type="datetime-local"> stores in its .value field.
    var d = new Date();
    return d.getFullYear() + '-' +
      String(d.getMonth() + 1).padStart(2, '0') + '-' +
      String(d.getDate()).padStart(2, '0') + 'T' +
      String(d.getHours()).padStart(2, '0') + ':' +
      String(d.getMinutes()).padStart(2, '0') + ':' +
      String(d.getSeconds()).padStart(2, '0');
  }

  function renderSystemTime(span) {
    if (systemTimeOffsetMs === null) return;
    var d = new Date(Date.now() + systemTimeOffsetMs);
    span.textContent = d.toLocaleString();
  }

  function initDatetimePicker(root) {
    // Always pre-fill an empty picker; htmx swaps replace #content so each
    // visit gets a fresh element, and overwriting an empty value is a no-op.
    var input = (root || document).querySelector('#sc-dt-input');
    if (input && !input.value) {
      input.value = localDatetimeLocal();
    }

    var span = (root || document).querySelector('#sc-system-time');
    if (span && span.dataset.serverTime) {
      // Server omits the timezone offset; treat the value as the device's
      // local wall clock, which is what the Timezone row tells the user.
      var deviceMs = new Date(span.dataset.serverTime).getTime();
      systemTimeOffsetMs = deviceMs - Date.now();
      renderSystemTime(span);
      // Cancel any prior tick from an earlier htmx navigation — htmx
      // replaces #content but doesn't notify JS to clean up timers,
      // so without this each visit would leak a 1 Hz interval pinning
      // an orphaned span.
      if (window.scSystemTimeTimer) clearInterval(window.scSystemTimeTimer);
      window.scSystemTimeTimer = setInterval(function () { renderSystemTime(span); }, 1000);
    }
  }

  // The <input type="datetime-local"> reports its value in the user's local
  // time without a zone suffix; the server appends "+00:00" unconditionally,
  // so without this hook a CEST user typing 18:55 would set the device to
  // 18:55 UTC instead of 16:55 UTC.  Convert the parameter to UTC just
  // before it goes on the wire so the device clock matches the moment the
  // user actually selected.  Attached to `document` (not document.body)
  // because this code runs at IIFE parse time before <body> exists.
  document.addEventListener('htmx:configRequest', function (evt) {
    if (!evt.detail || evt.detail.path !== '/maintenance/system/datetime') return;
    var local = evt.detail.parameters && evt.detail.parameters.datetime;
    if (!local) return;
    var d = new Date(local);
    if (isNaN(d.getTime())) return;
    // toISOString gives "YYYY-MM-DDTHH:MM:SS.sssZ"; strip millis so the
    // server's "+00:00" append produces valid RFC 3339.
    evt.detail.parameters.datetime = d.toISOString().slice(0, 19);
  });

  // After a successful Set POST, re-sync the live clock to the value the
  // user just submitted so the display jumps instead of waiting for the next
  // page load.
  document.addEventListener('htmx:afterRequest', function (evt) {
    if (!evt.detail || !evt.detail.successful) return;
    var path = evt.detail.requestConfig && evt.detail.requestConfig.path;
    if (path !== '/maintenance/system/datetime') return;
    var input = document.getElementById('sc-dt-input');
    var span = document.getElementById('sc-system-time');
    if (!input || !span || !input.value) return;
    var submittedMs = new Date(input.value).getTime();
    if (isNaN(submittedMs)) return;
    systemTimeOffsetMs = submittedMs - Date.now();
    renderSystemTime(span);
  });

  window.scRestoreCheckbox = function (cb) {
    var form = document.getElementById('restore-form');
    if (!form) return;
    form.setAttribute('hx-confirm', cb.checked
      ? 'Save configuration to startup? Reboot required to apply.'
      : 'Apply this configuration to the running system?');
  };

  // Locate or inject the shared #sw-progress-card. Server-rendered when the
  // page loads with ?installing=1; injected here when the upload flow starts
  // from /software. The same DOM element later receives SSE-driven swaps.
  function ensureProgressCard(headerText, message) {
    var card = document.getElementById('sw-progress-card');
    if (!card) {
      card = document.createElement('section');
      card.id = 'sw-progress-card';
      card.className = 'info-card';
      var grid = document.querySelector('.sw-install-grid');
      var parent = grid && grid.parentNode;
      if (parent) parent.insertBefore(card, grid);
      else (document.getElementById('content') || document.body).appendChild(card);
    }
    card.innerHTML =
      '<div class="card-header">' + headerText + '</div>' +
      '<div class="card-body">' +
      '  <div class="progress-bar-wrap progress-bar-wrap--flush">' +
      '    <div class="progress-bar" style="width:0%"></div>' +
      '  </div>' +
      '  <p class="progress-text">' + message + '</p>' +
      '</div>';
    // Force re-init of the SSE stream if one was previously attached.
    delete card.dataset.sseInit;
    return card;
  }

  window.swUpload = function () {
    var fileInput = document.getElementById('sw-file');
    if (!fileInput || !fileInput.files.length) return;
    if (!confirm('Upload and install this software bundle? The current installation may be overwritten.')) return;

    var file       = fileInput.files[0];
    var autoReboot = !!document.querySelector('#sw-upload-auto-reboot:checked');
    var btn        = document.getElementById('sw-upload-btn');

    var sseURL = new URL((btn && btn.getAttribute('data-sse-url')) || '/software/progress', window.location.origin);
    if (autoReboot) sseURL.searchParams.set('auto-reboot', '1');

    var formData = new FormData();
    formData.append('pkg', file);
    if (autoReboot) formData.append('auto-reboot', '1');

    if (btn) btn.disabled = true;
    var card = ensureProgressCard('Uploading Bundle', 'Uploading bundle… 0%');
    var bar  = card.querySelector('.progress-bar');
    var text = card.querySelector('.progress-text');

    var xhr = new XMLHttpRequest();

    xhr.upload.onprogress = function (e) {
      if (!e.lengthComputable) return;
      var pct = Math.round(e.loaded / e.total * 100);
      bar.style.width = pct + '%';
      text.textContent = 'Uploading bundle\u2026 ' + pct + '%';
    };

    xhr.onload = function () {
      if (xhr.status !== 200) {
        text.textContent = 'Upload failed: ' + (xhr.responseText.replace(/<[^>]*>/g, '').trim() || 'unknown error');
        if (btn) btn.disabled = false;
        return;
      }
      // pushState lets a mid-install reload resume the progress card.
      var target = xhr.responseText.trim() || '/software?installing=1';
      if (window.history && window.history.pushState) {
        window.history.pushState({}, '', target);
      }
      text.textContent = 'Starting installation\u2026';
      card.setAttribute('data-sse-src', sseURL.pathname + sseURL.search);
      initSoftwareProgress(document);
    };

    xhr.onerror = function () {
      text.textContent = 'Upload failed \u2014 network error.';
      if (btn) btn.disabled = false;
    };

    xhr.open('POST', '/software/upload');
    xhr.setRequestHeader('X-CSRF-Token', getCSRFToken());
    xhr.send(formData);
  };

  // SSE-driven software install progress card.
  // The Go server polls RESTCONF and streams rendered HTML fragments; we just
  // swap them into the card and let the server close the stream when done.
  var swEventSource = null;

  function initSoftwareProgress(root) {
    var scope = root || document;
    var card = scope.querySelector('#sw-progress-card[data-sse-src]');
    if (!card || card.dataset.sseInit) return;
    card.dataset.sseInit = 'true';

    var src = card.getAttribute('data-sse-src');
    if (swEventSource) { swEventSource.close(); }
    swEventSource = new EventSource(src);

    function swap(html) {
      card.innerHTML = html;
      initProgressBars(card);
      if (window.htmx) htmx.process(card);
    }

    function endStream() {
      swEventSource.close();
      swEventSource = null;
      // Drop the stream URL and re-arm the upload button so a follow-up
      // install can run on the same page without a reload.
      card.removeAttribute('data-sse-src');
      delete card.dataset.sseInit;
      var btn = document.getElementById('sw-upload-btn');
      if (btn) btn.disabled = false;
    }

    swEventSource.addEventListener('progress', function(e) { swap(e.data); });

    swEventSource.addEventListener('done', function(e) {
      swap(e.data);
      endStream();
    });

    swEventSource.addEventListener('reboot', function(e) {
      swap(e.data);
      endStream();
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

    // Let EventSource auto-reconnect on transient errors. Only tear down
    // when the browser gives up (readyState === CLOSED) — otherwise an
    // nginx read-timeout or a brief network blip would silently kill the
    // stream and leave the progress card stuck on stale data.
    swEventSource.onerror = function () {
      if (swEventSource && swEventSource.readyState === EventSource.CLOSED) {
        endStream();
      }
    };
  }

  document.addEventListener('DOMContentLoaded', function() {
    initCSRF();
    initPageTitle();
    initDeviceStatusBanner();
    initDynamicUI(document);
    initSoftwareProgress(document);
    // On first paint we want the buffer pinned to the most recent entries
    // and keyboard focus on the buffer so PageUp/PageDown work straight away.
    scrollLogsToBottom();
    focusLogsContent();

    // Attach the htmx swap listener here, not at IIFE parse time — at parse
    // time the script runs inside <head> before <body> exists, so the
    // document.body guard would silently skip the listener and subsequent
    // navigations wouldn't re-init dynamic UI.
    // Set the Load earlier flag at click capture time — runs before htmx
    // sees the click, before any other listener, and works regardless of
    // whether htmx's beforeRequest event detail matches our expectations.
    // While we're here, snapshot the scroll state on the .logs-content so
    // afterSwap can preserve the user's visual position: the line they
    // were looking at should stay put after new (older) content gets
    // prepended, not jump to top or bottom.
    document.body.addEventListener('click', function (evt) {
      var btn = evt.target && evt.target.closest && evt.target.closest('.logs-load-earlier');
      if (!btn) return;
      loadEarlierInFlight = true;
      if (loadEarlierClearTimer) clearTimeout(loadEarlierClearTimer);
      loadEarlierClearTimer = setTimeout(function () {
        loadEarlierInFlight = false;
        loadEarlierClearTimer = null;
      }, 500);
      var content = btn.closest('.logs-content');
      if (content) {
        content._preLoadHeight = content.scrollHeight;
        content._preLoadTop = content.scrollTop;
      }
    }, true);

    if (window.htmx) {
      document.body.addEventListener('htmx:afterSwap', function (evt) {
        // Close any open SSE stream if the software install progress card is no longer present.
        if (swEventSource && !document.getElementById('sw-progress-card')) {
          swEventSource.close();
          swEventSource = null;
        }

        // Read but DON'T reset — the timeout in the click handler clears
        // it after the grace window, so successive htmx:afterSwap fires
        // within one Load earlier round trip all take the same branch.
        var isLoadEarlier = loadEarlierInFlight;

        // Tab change replaces the whole logs card, which removes the
        // tail checkbox without giving us a chance to react.  Kill any
        // open stream so it doesn't run on after navigation.  Skip on
        // Load earlier — that swap stays within the current tab and
        // shouldn't touch the live-tail stream.
        if (!isLoadEarlier) {
          teardownLogsTail();
        }

        // Scope to document, not evt.detail.target: tab swaps use
        // hx-swap="outerHTML", so afterSwap hands us the OLD (now detached)
        // .logs-card as the target.  Initialising against it misses the new
        // tab's Live-tail checkbox — which is why tailing only worked on the
        // tab rendered at page load.  The init helpers all guard on
        // :not([data-init]), so re-scanning the document is idempotent.
        var scope = document;
        initDynamicUI(scope);

        if (isLoadEarlier) {
          // Content-anchored scroll preservation: keep the line the user
          // was looking at in the same vertical position by shifting
          // scrollTop by exactly the height of the newly-prepended
          // content.  The pre-swap measurements were captured in the
          // click-capture handler so they reflect the buffer's state
          // before htmx mutated it.  Without this, the browser falls
          // back to scrollTop=0 because the OLD anchor element (the
          // clicked button) no longer exists in the DOM.  Idempotent
          // across multiple afterSwap fires — recomputing from saved
          // snapshots always yields the same scrollTop.
          document.querySelectorAll('.logs-content').forEach(function (content) {
            if (typeof content._preLoadHeight !== 'number') return;
            var added = content.scrollHeight - content._preLoadHeight;
            content.scrollTop = content._preLoadTop + added;
          });
        } else {
          scrollLogsToBottom();
          focusLogsContent();
        }

        initSoftwareProgress(scope);
      });
    }
  });
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

// Interface page glue (replaces inline hx-on / inline <script>, which CSP
// blocks under the `script-src 'self'` policy in middleware.go).
(function () {
  // A checkbox marked [data-fold-target="<id>"] toggles the matching <details>
  // (or any element with that id) immediately on change. Used for the DHCP /
  // DHCPv6 settings foldouts on Configure → Interface: the foldout is always
  // in the DOM but starts hidden when the client isn't enabled. This lets the
  // user see the settings form before clicking Save IPvX Settings and confirms
  // the section exists even when DHCP is off.
  document.addEventListener('change', function (evt) {
    var cb = evt.target;
    if (!cb || !cb.matches || !cb.matches('input[type="checkbox"][data-fold-target]')) return;
    var target = document.getElementById(cb.getAttribute('data-fold-target'));
    if (target) target.hidden = !cb.checked;
  });

  // Add Interface modal — open via data-show-modal, close via
  // data-close-modal. Mirrors the existing data-show/data-hide vocabulary
  // for action-on-target attributes. Native <dialog>.showModal() gives us
  // focus trap, ESC, and backdrop for free.
  document.addEventListener('click', function (e) {
    var open = e.target.closest && e.target.closest('[data-show-modal]');
    if (open) {
      var dlg = document.getElementById(open.getAttribute('data-show-modal'));
      if (dlg && dlg.showModal) {
        // Reset to server-rendered defaults so the dialog is consistent
        // on every open (a previous pick + Cancel would otherwise leave
        // the wrong fieldset enabled).
        var form = dlg.querySelector('form');
        if (form) form.reset();
        var vlanName = dlg.querySelector('#add-iface-vlan-name');
        if (vlanName) delete vlanName.dataset.userEdited;
        var wifiName = dlg.querySelector('#add-iface-wifi-name');
        if (wifiName) delete wifiName.dataset.userEdited;
        dlg.showModal();
        var sel = dlg.querySelector('#add-iface-type-select');
        if (sel) sel.dispatchEvent(new Event('change', { bubbles: true }));
      }
    }
    var close = e.target.closest && e.target.closest('[data-close-modal]');
    if (close) {
      var dlg2 = document.getElementById(close.getAttribute('data-close-modal'));
      if (dlg2 && dlg2.close) dlg2.close();
    }
  });

  // Add Interface modal, WiFi fieldset: mode (Station/AP) drives which
  // security-mode optgroup is exposed, which AP-only rows are visible,
  // whether the PSK row applies (open/disabled hide it), and an -ap
  // suffix on the Name.
  function refreshWifiSec() {
    var modeAP = document.getElementById('add-iface-wifi-mode-ap');
    var isAP = !!(modeAP && modeAP.checked);
    var sec = document.getElementById('add-iface-wifi-sec');
    if (sec) {
      var groups = sec.querySelectorAll('optgroup[data-mode-group]');
      groups.forEach(function (g) {
        var match = g.getAttribute('data-mode-group') === (isAP ? 'access-point' : 'station');
        g.hidden = !match;
        g.disabled = !match;
      });
      // Reset to the first visible option of the active group if the
      // current value belongs to the other mode.
      var active = sec.options[sec.selectedIndex];
      if (!active || active.parentElement.disabled) {
        for (var i = 0; i < sec.options.length; i++) {
          if (!sec.options[i].parentElement.disabled) { sec.selectedIndex = i; break; }
        }
      }
    }
    var hiddenRow = document.getElementById('add-iface-wifi-hidden-row');
    if (hiddenRow) hiddenRow.hidden = !isAP;
    refreshWifiPSK();
    refreshWifiName();
  }
  // Auto-suffix the Name with "-ap" in AP mode and strip it in Station,
  // unless the user has manually edited the Name. Matches the in-tree
  // wifi naming convention (wifi0 / wifi0-ap).
  function refreshWifiName() {
    var name = document.getElementById('add-iface-wifi-name');
    if (!name || name.dataset.userEdited === '1') return;
    var modeAP = document.getElementById('add-iface-wifi-mode-ap');
    var isAP = !!(modeAP && modeAP.checked);
    var base = name.value.replace(/-ap$/, '');
    name.value = isAP ? base + '-ap' : base;
  }
  function refreshWifiPSK() {
    var sec = document.getElementById('add-iface-wifi-sec');
    var pskRow = document.getElementById('add-iface-wifi-psk-row');
    var psk = document.getElementById('add-iface-wifi-psk');
    if (!sec || !pskRow || !psk) return;
    var v = sec.value;
    var needPSK = v !== 'disabled' && v !== 'open';
    pskRow.hidden = !needPSK;
    psk.required = needPSK;
    psk.disabled = !needPSK;
  }
  document.addEventListener('change', function (e) {
    var t = e.target;
    if (t.id === 'add-iface-wifi-mode-sta' || t.id === 'add-iface-wifi-mode-ap') {
      refreshWifiSec();
    } else if (t.id === 'add-iface-wifi-sec') {
      refreshWifiPSK();
    }
  });
  // User-typed Name pins it (stops auto-suffix on mode change).
  document.addEventListener('input', function (e) {
    if (e.target.id === 'add-iface-wifi-name') {
      e.target.dataset.userEdited = '1';
    }
  });

  // CSP-safe successor to inline hx-on:htmx:after-request. Save buttons
  // in the inline "+ New keystore key" forms carry
  // data-ks-create-success="<form-id>"; on a successful HTMX swap, hide
  // the named form and clear its inputs so the user sees the picker
  // with their new key selected.
  document.addEventListener('htmx:afterRequest', function (evt) {
    if (!evt.detail || !evt.detail.successful) return;
    var btn = evt.detail.elt;
    if (!btn || !btn.hasAttribute || !btn.hasAttribute('data-ks-create-success')) return;
    var form = document.getElementById(btn.getAttribute('data-ks-create-success'));
    if (!form) return;
    form.hidden = true;
    form.querySelectorAll('input, textarea').forEach(function (i) { i.value = ''; });
    resetMaskedInputs(form);
  });

  // Wizard "Edit" / "+ Add" buttons that share an inline create form.
  //   - data-add-form="<form-id>" clears every input/textarea before
  //     showing the form (handled here; data-show on the same button
  //     then reveals it).
  //   - data-edit-form="<form-id>" pre-fills the form from the picker's
  //     selected option. data-edit-source points at the <select>;
  //     data-edit-name-target (optional) names the input that should
  //     receive the picker's value (for keystore forms whose name input
  //     mirrors the picker). For the radio form, every <input>/<select>
  //     whose name matches a data-<name> attribute on the picker option
  //     is filled from that attribute (e.g. data-country fills
  //     name="country-code", data-band fills name="band", …).
  function clearKsForm(formId) {
    var form = document.getElementById(formId);
    if (!form) return;
    form.querySelectorAll('input, textarea').forEach(function (i) { i.value = ''; });
    form.querySelectorAll('select').forEach(function (s) { s.selectedIndex = 0; });
    resetMaskedInputs(form);
  }
  // Strip the `.cfg-secret-shown` (eye-toggled "show") class from any
  // masked inputs inside `root`. Called whenever a fold-out form
  // transitions (open / clear / prefill / after-save) so a previous
  // session's "shown" state never leaks across the next interaction.
  function resetMaskedInputs(root) {
    if (!root) return;
    root.querySelectorAll('.cfg-input-mask.cfg-secret-shown').forEach(function (i) {
      i.classList.remove('cfg-secret-shown');
    });
  }
  function prefillKsForm(btn) {
    var formId = btn.getAttribute('data-edit-form');
    var srcId  = btn.getAttribute('data-edit-source');
    var form = document.getElementById(formId);
    var src  = document.getElementById(srcId);
    if (!form || !src) return;
    resetMaskedInputs(form);
    var opt = src.options[src.selectedIndex];
    if (!opt) return;
    var ksNameTarget = btn.getAttribute('data-edit-name-target');
    if (ksNameTarget) {
      var nameInput = document.getElementById(ksNameTarget);
      if (nameInput) nameInput.value = opt.value;
    }
    // Generic mapping: every form field whose [name=X] matches a
    // data-X attribute on the option gets that attribute's value.
    form.querySelectorAll('[name]').forEach(function (field) {
      var attr = 'data-' + field.getAttribute('name');
      if (opt.hasAttribute(attr)) field.value = opt.getAttribute(attr) || '';
    });
    // For the radio form, the picker's selected radio name needs to be
    // present in the inner radio-name <select> so the Save POST carries
    // it. The available-radios dropdown only lists unconfigured radios,
    // so inject the selected name as an extra option if missing.
    var radioNameSel = form.querySelector('select[name="radio-name"]');
    if (radioNameSel && opt.value) {
      var found = false;
      for (var i = 0; i < radioNameSel.options.length; i++) {
        if (radioNameSel.options[i].value === opt.value) {
          radioNameSel.selectedIndex = i;
          found = true;
          break;
        }
      }
      if (!found) {
        var injected = document.createElement('option');
        injected.value = opt.value;
        injected.textContent = opt.value + ' (currently configured)';
        injected.dataset.injected = '1';
        radioNameSel.insertBefore(injected, radioNameSel.firstChild);
        radioNameSel.selectedIndex = 0;
      }
    }
  }
  document.addEventListener('click', function (e) {
    var addBtn = e.target.closest && e.target.closest('[data-add-form]');
    if (addBtn) {
      clearKsForm(addBtn.getAttribute('data-add-form'));
    }
    var editBtn = e.target.closest && e.target.closest('[data-edit-form]');
    if (editBtn) {
      // Clear any injected "currently configured" options from prior
      // edits before re-prefilling.
      var form = document.getElementById(editBtn.getAttribute('data-edit-form'));
      if (form) form.querySelectorAll('option[data-injected]').forEach(function (o) { o.remove(); });
      prefillKsForm(editBtn);
    }
  });

  // Show / hide the WiFi PSK passphrase. The input is type="text" with
  // CSS masking (to dodge browser password-manager prompts); the eye
  // button toggles a `.cfg-secret-shown` class that removes the mask.
  document.addEventListener('click', function (e) {
    var btn = e.target.closest && e.target.closest('[data-toggle-mask]');
    if (!btn) return;
    var input = document.getElementById(btn.getAttribute('data-toggle-mask'));
    if (!input) return;
    input.classList.toggle('cfg-secret-shown');
  });

  // Hide the Edit button when its picker has no real selection
  // (placeholder option, or empty). Re-evaluated on every change to the
  // picker, including the HTMX swap that follows a successful Save.
  function syncKsEditButtons(root) {
    var scope = root || document;
    scope.querySelectorAll('[data-edit-source]').forEach(function (btn) {
      var src = document.getElementById(btn.getAttribute('data-edit-source'));
      if (!src) { btn.hidden = true; return; }
      var opt = src.options[src.selectedIndex];
      btn.hidden = !opt || !opt.value || opt.disabled;
    });
  }
  document.addEventListener('change', function (e) {
    if (e.target && e.target.tagName === 'SELECT') syncKsEditButtons();
  });
  document.addEventListener('htmx:afterSwap', function () { syncKsEditButtons(); });
  document.addEventListener('DOMContentLoaded', function () { syncKsEditButtons(); });
  // Sync once when the dialog opens so initial state matches the default
  // mode (Station). The fieldset toggle below already fires on dialog
  // open, so piggyback there.

  // Add Interface modal, VLAN fieldset: keep the Name input synced to
  // <parent>.<vid> as the user changes either side. The user can still
  // override by typing into Name; once they type a custom value, we stop
  // auto-syncing for that session (tracked via .dataset.userEdited).
  document.addEventListener('input', function (e) {
    var nameInput = document.getElementById('add-iface-vlan-name');
    if (!nameInput) return;
    var t = e.target;
    if (t === nameInput) {
      nameInput.dataset.userEdited = '1';
      return;
    }
    if (t.id !== 'add-iface-vlan-parent' && t.id !== 'add-iface-vlan-vid') return;
    if (nameInput.dataset.userEdited === '1') return;
    var parent = document.getElementById('add-iface-vlan-parent');
    var vid = document.getElementById('add-iface-vlan-vid');
    if (parent && vid) {
      nameInput.value = (parent.value || '') + '.' + (vid.value || '');
    }
  });

  // On type select change in the Add Interface modal, show/enable the
  // matching <fieldset> and hide/disable the rest. Disabling siblings is
  // what keeps their inputs out of the form submission so only one
  // `name="name"` reaches the server. Falls back to the "unsupported"
  // panel for types whose Create path isn't implemented yet.
  document.addEventListener('change', function (e) {
    var sel = e.target.closest && e.target.closest('#add-iface-type-select');
    if (!sel) return;
    var opt = sel.options[sel.selectedIndex];
    var slug = (opt && opt.getAttribute('data-slug')) || '';
    var fields = document.querySelectorAll('.iface-fields');
    var matched = null;
    fields.forEach(function (fs) {
      var isMatch = fs.id === 'iface-fields-' + slug;
      fs.hidden = !isMatch;
      fs.disabled = !isMatch;
      if (isMatch) matched = fs;
    });
    if (!matched) {
      var unsupported = document.getElementById('iface-fields-unsupported');
      if (unsupported) {
        unsupported.hidden = false;
        unsupported.disabled = false;
      }
    }
    var createBtn = document.getElementById('add-iface-create-btn');
    if (createBtn) createBtn.disabled = !matched;
    if (matched && matched.id === 'iface-fields-wifi') refreshWifiSec();
  });

  // Configure > Hardware "+ Add hardware" picker: sync the hidden class
  // input from the selected option's data-class and reveal class-specific
  // fields (currently WiFi country-code).
  document.addEventListener('change', function (e) {
    var sel = e.target.closest && e.target.closest('#add-hw-picker');
    if (!sel) return;
    var opt = sel.options[sel.selectedIndex];
    var cls = (opt && opt.getAttribute('data-class')) || '';
    var classInput = document.getElementById('add-hw-class');
    if (classInput) classInput.value = cls;
    var wifi = document.getElementById('add-hw-wifi-fields');
    if (wifi) wifi.hidden = (cls !== 'wifi');
    var country = document.getElementById('add-hw-wifi-country');
    if (country) country.required = (cls === 'wifi');
  });
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

  // Follow the OS when on 'auto'.  Colours already track via CSS @media +
  // color-scheme; re-applying on change keeps the menu indicators and any
  // theme-derived state in sync.  Ignored when a theme is pinned explicitly.
  try {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function () {
      if (!getTheme()) applyTheme(null);
    });
  } catch (_) {}

  document.addEventListener('DOMContentLoaded', function() {
    // Re-apply now that the DOM exists: applyTheme() ran in <head> before the
    // dropdown and login toggle were parsed, so their indicators weren't set
    // (the login toggle was stuck on its default 'auto' glyph).
    applyTheme(getTheme());

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

    // Persistence + Configure enter-hook.
    //
    // Toggling a section header just expands or collapses that section —
    // it does NOT close sibling sections, and it does NOT auto-navigate
    // to the first page in the section. This lets the user browse the
    // available pages under a header without paying for a navigation
    // they didn't ask for. The actual mutual-exclusion (other sections
    // collapse) happens in updateActiveNav after a real page-link click.
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
        if (d.open && d.id === 'nav-configure') {
          enterConfigure();
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

// ─── data-autofocus: WM-friendly focus on visible ──────────────────────────
// Focus the [data-autofocus] element only when the page is actually
// visible — calling .focus() on a background tab/desktop is treated
// as an attention hint by Cinnamon/Muffin (and similar X11 WMs)
// and pulls the whole browser window to the foreground.
(function() {
  function focusAutofocusWhenVisible() {
    var el = document.querySelector('[data-autofocus]');
    if (!el) return;
    var apply = function () {
      if (document.visibilityState !== 'hidden') el.focus({ preventScroll: true });
    };
    if (document.visibilityState !== 'hidden') {
      apply();
      return;
    }
    var handler = function () {
      if (document.visibilityState !== 'hidden') {
        document.removeEventListener('visibilitychange', handler);
        apply();
      }
    };
    document.addEventListener('visibilitychange', handler);
  }
  document.addEventListener('DOMContentLoaded', focusAutofocusWhenVisible);
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
// openModal shows the shared confirm dialog. With opts.alert it becomes a
// single-button alert (one "Dismiss", no Cancel) for errors the user can only
// acknowledge — e.g. a rejected Apply. The Cancel button and OK label are
// restored on close so the dialog returns to confirm() defaults.
function openModal(message, onConfirm, opts) {
  opts = opts || {};
  var dlg = document.getElementById('confirm-dialog');
  if (!dlg) { // fallback if dialog missing
    if (opts.alert) window.alert(message);
    else if (onConfirm) onConfirm();
    return;
  }
  var msg = document.getElementById('dialog-message');
  var ok  = document.getElementById('dialog-confirm');
  var no  = document.getElementById('dialog-cancel');
  msg.textContent = message;

  var okLabel = ok.textContent;
  if (opts.alert) { no.hidden = true; ok.textContent = 'Dismiss'; }

  function cleanup() {
    ok.removeEventListener('click', handleOK);
    no.removeEventListener('click', handleNo);
    if (opts.alert) { no.hidden = false; ok.textContent = okLabel; }
    dlg.close();
  }
  function handleOK()  { cleanup(); if (onConfirm) onConfirm(); }
  function handleNo()  { cleanup(); }

  ok.addEventListener('click', handleOK);
  no.addEventListener('click', handleNo);
  dlg.showModal();
}

// ─── data-close-detail: collapse a key-detail-row from inside ───────────────
(function() {
  document.addEventListener('click', function(e) {
    var btn = e.target.closest('[data-close-detail]');
    if (!btn) return;
    var target = btn.getAttribute('data-close-detail');
    var row = document.getElementById('key-detail-' + target);
    if (row) row.classList.remove('is-open');
    var toggle = document.querySelector('[data-target="' + target + '"]');
    if (toggle) toggle.setAttribute('aria-expanded', 'false');
  });
})();

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

// ─── YANG tree: selected-node highlight + deep-link reveal ──────────────────
// Two related orientation fixes for the tree editor:
//   1. Clicking a node loads it in the right pane but left no cue of where you
//      are — mark the clicked summary .yt-active.
//   2. A status-page "Configure →" deep link (/configure/tree?path=…) only
//      filled the right pane, leaving the left tree collapsed and the user
//      disoriented.  Walk the ancestor chain, opening each <details> in turn —
//      children load lazily over htmx on toggle, so wait for each level's swap
//      before descending — then highlight and scroll the target into view.
(function() {
  function setActive(summary) {
    document.querySelectorAll('.yt-label.yt-active').forEach(function (s) {
      if (s !== summary) s.classList.remove('yt-active');
    });
    if (summary) summary.classList.add('yt-active');
  }

  document.addEventListener('click', function (e) {
    var summary = e.target.closest && e.target.closest('summary[data-yang-path]');
    if (summary) setActive(summary);
  });

  // Load a node's children into its .yt-children with a direct htmx.ajax
  // request, resolving with the children <ul>.  We deliberately do NOT route
  // through the node's lazy "toggle once" trigger: the toggle event from a
  // programmatic .open is async and, on a freshly htmx-swapped tree, did not
  // reliably reach htmx — the node opened but its body stayed empty until a
  // manual reload.  htmx.ajax sidesteps all of that.
  function loadChildren(details) {
    var childUl = details.querySelector(':scope > .yt-children');
    if (!childUl || childUl.children.length) return Promise.resolve(childUl);
    var url = details.getAttribute('hx-get');
    if (!url || !window.htmx || !window.htmx.ajax) return Promise.resolve(childUl);
    return window.htmx.ajax('GET', url, { target: childUl, swap: 'innerHTML' })
      .then(function () { return childUl; });
  }

  function step(ul, target) {
    var best = null;
    ul.querySelectorAll(':scope > li > details.yt-node > summary[data-yang-path]').forEach(function (s) {
      var p = s.getAttribute('data-yang-path');
      if (p === target || target.indexOf(p + '/') === 0) {
        if (!best || p.length > best.getAttribute('data-yang-path').length) best = s;
      }
    });
    if (!best) return;

    var details  = best.parentElement;
    var isTarget = best.getAttribute('data-yang-path') === target;
    details.open = true; // chevron + reveal the children area

    if (isTarget) {
      // Highlight and scroll right away — neither depends on the child load —
      // then pull in the target's own children so its subtree shows.
      setActive(best);
      best.scrollIntoView({ block: 'center' });
      loadChildren(details);
      return;
    }
    // Ancestor: load its children, then descend toward the target.
    loadChildren(details).then(function (childUl) {
      if (childUl) step(childUl, target);
    });
  }

  function initReveal() {
    var tree = document.querySelector('.yang-tree[data-initial-path]');
    if (!tree) return;
    var target = tree.getAttribute('data-initial-path');
    tree.removeAttribute('data-initial-path'); // process once
    var rootChildren = tree.querySelector('.yt-children');
    if (!target || !rootChildren) return;
    // Defer a tick: on an htmx navigation the tree was just swapped in and htmx
    // is still settling, so a child load triggered now is dropped (the node
    // then highlights only after a manual reload).  On a full reload it's
    // already settled, so the tick is harmless.
    setTimeout(function () { step(rootChildren, target); }, 0);
  }

  document.addEventListener('DOMContentLoaded', initReveal);
  document.addEventListener('htmx:afterSwap', initReveal);
})();

// ─── ⓘ field-info tooltip (position:fixed to escape overflow clipping) ───────
(function() {
  var tip = null;

  function getTip() {
    if (!tip) {
      tip = document.createElement('div');
      tip.id = 'field-tip';
    }
    return tip;
  }

  // A native <dialog>.showModal() renders in the browser's top layer,
  // which is above z-index entirely; a tooltip appended to <body> ends
  // up *below* the dialog regardless of z-index. Re-parent the tooltip
  // into the active dialog so it shares the same top-layer context.
  function parentFor(el) {
    return el.closest('dialog[open]') || document.body;
  }

  document.addEventListener('mouseover', function(e) {
    var el = e.target.closest('.field-info[data-tip]');
    if (!el) return;
    var t = getTip();
    var parent = parentFor(el);
    if (t.parentElement !== parent) parent.appendChild(t);
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

  // findSaveStatusSpan locates the status span associated with the form that
  // triggered an htmx event. Lookup order:
  //   1. .cfg-save-status inside the form
  //   2. [data-cfg-status-for="<form-id>"] anywhere on the page — used when the
  //      Save button is bound to the form via the HTML5 `form` attribute and
  //      lives outside the form element
  //   3. .cfg-save-status inside the enclosing .info-card (shared feedback slot)
  function findSaveStatusSpan(e) {
    var form = e.target && e.target.closest('form');
    if (form) {
      var inside = form.querySelector('.cfg-save-status');
      if (inside) return inside;
      if (form.id) {
        var bound = document.querySelector('.cfg-save-status[data-cfg-status-for="' + form.id + '"]');
        if (bound) return bound;
      }
    }
    var card = e.target && e.target.closest('.info-card');
    return card ? card.querySelector('.cfg-save-status') : null;
  }

  // cfgError: show error message in the .cfg-save-status span of the submitting form.
  // Falls back to the enclosing .info-card when the form itself has no status
  // span — useful when multiple forms in one card share a single feedback slot
  // (e.g. Date & Time's Set-now and Save-timezone forms).
  document.addEventListener('cfgError', function(e) {
    var msg = e.detail && e.detail.value ? e.detail.value : 'Save failed';
    var span = findSaveStatusSpan(e);
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
    var LS_KEY = 'sw-url-history';
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
      var dl = document.getElementById('sw-url-history');
      if (!dl) return;
      dl.innerHTML = loadHistory().map(function(u) {
        return '<option value="' + u.replace(/&/g, '&amp;').replace(/"/g, '&quot;') + '">';
      }).join('');
    }

    document.addEventListener('DOMContentLoaded', populateDatalist);
    document.addEventListener('htmx:afterSettle', populateDatalist);

    document.addEventListener('submit', function(e) {
      var form = e.target.closest('.software-form');
      if (!form) return;
      var input = form.querySelector('input[name="url"]');
      if (input && input.value) saveURL(input.value);
    });
  })();

  // cfgLogged records a save in the activity panel only — used when the
  // handler swaps the whole block via outerHTML and renders its own inline
  // confirmation, so there's no span for the page JS to chase.
  document.addEventListener('cfgLogged', function(e) {
    cfgLog('ok', e.detail && e.detail.value ? e.detail.value : 'Saved');
  });

  // cfgApplyError fires when Apply / Apply & Save reaches the device but the
  // candidate is rejected (e.g. deleting a still-referenced interface). The
  // backend returns 200 + this trigger instead of 502 so the connection
  // monitor stays quiet — the box is fine, the config isn't. The toolbar
  // optimistically logged success before issuing the request; drop that entry
  // and show the real reason.
  document.addEventListener('cfgApplyError', function(e) {
    var msg = (e.detail && e.detail.value) || 'The device rejected the configuration.';
    if (cfgLogEntries.length && cfgLogEntries[cfgLogEntries.length - 1].level === 'ok') {
      cfgLogEntries.pop();
    }
    cfgLog('error', msg);
    openModal('Configuration not applied: ' + msg, null, { alert: true });
  });

  document.addEventListener('cfgSaved', function(e) {
    var msg = e.detail && e.detail.value ? e.detail.value : 'Saved';
    cfgLog('ok', msg);
    var span = findSaveStatusSpan(e);
    if (span) {
      span.textContent = '✓ ' + msg;
      span.classList.add('saved');
      setTimeout(function() {
        span.textContent = '';
        span.classList.remove('saved');
      }, 3000);
    }

    // Re-render the whole current tree page from the (now fresh) candidate.
    // The save handlers only WRITE — they don't echo back what confd
    // inferred from the change (DHCP option lists, related leaves,
    // normalised values), and inference can land outside the edited node.
    // Re-fetching the page the user is on surfaces all of it.  Guarded on
    // #yang-detail so curated pages that also emit cfgSaved are unaffected.
    var detail = document.getElementById('yang-detail');
    if (detail && window.htmx) {
      var node = (history.state && history.state.yangDetailPath) ||
                 new URLSearchParams(window.location.search).get('node');
      if (node) {
        htmx.ajax('GET', '/configure/tree/node?path=' + encodeURIComponent(node),
          { target: '#yang-detail', swap: 'innerHTML' });
      }
    }
  });
})();

// ─── Inactivity auto-logout ────────────────────────────────────────────────
// Submits POST /logout after a configurable period of inactivity.
// The chosen timeout is stored in localStorage; default is 15 min.
// Reset by deliberate user input and by HTMX requests that aren't
// background pollers.
(function() {
  var LS_KEY = 'auto-logout';
  var DEFAULT = '900';
  var timerId = null;

  function getMs() {
    var n = parseInt(localStorage.getItem(LS_KEY) || DEFAULT, 10);
    return isNaN(n) ? parseInt(DEFAULT, 10) * 1000 : n * 1000;
  }

  function updateOpts() {
    var current = localStorage.getItem(LS_KEY) || DEFAULT;
    document.querySelectorAll('.timeout-opt').forEach(function(btn) {
      btn.classList.toggle('is-active', btn.getAttribute('data-timeout') === current);
    });
  }

  function doLogout() {
    // Tear down the server-side session in the background so it can't
    // outlive the navigation. keepalive lets the request finish even
    // if the user is mid-tab-close. getCSRFToken() from the early IIFE
    // isn't in scope here — read the meta tag directly.
    var fd = new FormData();
    var meta = document.querySelector('meta[name="csrf-token"]');
    if (meta) fd.append('csrf', meta.getAttribute('content') || '');
    fetch('/logout', {
      method: 'POST',
      body: fd,
      credentials: 'same-origin',
      keepalive: true,
    }).catch(function () {});
    window.location.replace('/login');
  }

  function reset() {
    clearTimeout(timerId);
    var ms = getMs();
    if (ms > 0) timerId = setTimeout(doLogout, ms);
  }

  // Mirror the chosen idle timeout to the server so the server-side session
  // expiry matches the menu — including "Off" (0 = never).  The server starts
  // each session at its 1 h default; this re-asserts the user's preference on
  // load and whenever they change it.
  function syncServerTimeout(secs) {
    var fd = new FormData();
    fd.append('timeout', secs);
    var meta = document.querySelector('meta[name="csrf-token"]');
    if (meta) fd.append('csrf', meta.getAttribute('content') || '');
    fetch('/session/timeout', { method: 'POST', body: fd, credentials: 'same-origin' }).catch(function () {});
  }

  // Background pollers (watchdog, *counters refresh) must NOT extend
  // the idle window — otherwise the timer never reaches the threshold.
  function isPollingPath(p) {
    return p === '/device-status' || (p && p.indexOf('/counters') !== -1);
  }

  document.addEventListener('DOMContentLoaded', function() {
    if (document.querySelector('form[action="/login"]')) return;

    updateOpts();
    reset();
    syncServerTimeout(localStorage.getItem(LS_KEY) || DEFAULT);

    // mousemove deliberately omitted: trackpad jitter from a neighbouring
    // window would keep the session alive indefinitely.
    ['mousedown', 'keypress', 'touchstart', 'scroll', 'click'].forEach(function(ev) {
      window.addEventListener(ev, reset, { passive: true });
    });
    document.addEventListener('htmx:afterRequest', function(evt) {
      var cfg = evt && evt.detail && evt.detail.requestConfig;
      if (cfg && isPollingPath(cfg.path)) return;
      reset();
    });

    document.addEventListener('click', function(e) {
      var btn = e.target.closest('.timeout-opt');
      if (!btn) return;
      localStorage.setItem(LS_KEY, btn.getAttribute('data-timeout'));
      updateOpts();
      reset();
      syncServerTimeout(btn.getAttribute('data-timeout'));
    });

    // Session expired server-side while we were idle: any HX request
    // now returns 401. Send the user to /login so the page reflects
    // reality instead of failing silently on every click.
    document.addEventListener('htmx:responseError', function(evt) {
      var s = evt && evt.detail && evt.detail.xhr && evt.detail.xhr.status;
      if (s === 401) window.location.replace('/login');
    });

    // Route the manual Logout button through doLogout so the session is
    // torn down server-side before navigation, not after.
    var logoutForm = document.querySelector('form[action="/logout"]');
    if (logoutForm) {
      logoutForm.addEventListener('submit', function(e) {
        e.preventDefault();
        doLogout();
      });
    }
  });
})();

// ─── Maintenance > Diagnostics ─────────────────────────────────────────────
// Ping / traceroute / mtr / DNS lookup.  Tabs are client-side: switching
// tool only changes which option fields and which output surface show.
// Run opens an EventSource to /maintenance/diagnostics/run and renders
// streamed `line` / `hop` / `done` events; Stop closes it, which cancels
// the server request context and kills the spawned tool.  DNS lookup is a
// one-shot fetch instead of a stream.
(function () {
  var diagES = null; // active stream, module-scoped so afterSwap can kill it

  function diagTeardown() {
    if (diagES) {
      diagES.close();
      diagES = null;
    }
  }

  // Tools that take a network target + source interface (everything but
  // DNS, which resolves a name and has no source-interface concept here).
  function isNetTool(tool) {
    return tool === 'ping' || tool === 'traceroute' || tool === 'mtr' || tool === 'nmap';
  }

  function setActiveTool(card, tool) {
    card.dataset.active = tool;

    card.querySelectorAll('.diag-tab').forEach(function (t) {
      t.classList.toggle('active', t.getAttribute('data-tool') === tool);
    });

    // Show only the option fields belonging to this tool.
    card.querySelectorAll('.diag-opt').forEach(function (o) {
      o.hidden = o.getAttribute('data-tool') !== tool;
    });

    // Source interface only applies to the network tools.
    card.querySelectorAll('.diag-only-net').forEach(function (el) {
      el.hidden = !isNetTool(tool);
    });

    // mtr's cycle/size fields only make sense when not running forever.
    if (tool === 'mtr') applyMtrContinuous(card);

    // Retarget the input label/placeholder per tool.
    var label = card.querySelector('[data-target-label]');
    var input = card.querySelector('.diag-target');
    if (tool === 'dns') {
      if (label) label.textContent = 'Name to resolve';
      if (input) input.placeholder = 'hostname';
    } else if (tool === 'nmap') {
      if (label) label.textContent = 'Target host, address, or CIDR';
      if (input) input.placeholder = 'host, IP, or 192.168.0.0/24';
    } else {
      if (label) label.textContent = 'Target host or address';
      if (input) input.placeholder = 'hostname or IP';
    }

    // Reset to the ready state: kill any stream, hide all output
    // surfaces, show the placeholder hint.  The matching surface is
    // revealed on Run.  Switching tool is a context change, so dropping
    // the previous tool's output is the expected "fresh start".
    diagTeardown();
    card.querySelector('.diag-text').hidden = true;
    card.querySelector('.diag-mtr').hidden = true;
    card.querySelector('.diag-dns').hidden = true;
    card.querySelector('.diag-placeholder').hidden = false;
    diagSetStatus(card, '', '');
    diagSetRunning(card, false);
  }

  function diagSetStatus(card, text, cls) {
    var s = card.querySelector('.diag-status');
    if (!s) return;
    s.textContent = text || '';
    s.className = 'diag-status' + (cls ? ' ' + cls : '');
  }

  function diagSetRunning(card, running) {
    card.querySelector('.diag-run').hidden = running;
    card.querySelector('.diag-stop').hidden = !running;
  }

  // Return focus to the target field when a run finishes so the user can
  // type the next address immediately.  Guard on visibility so a run that
  // completes while the tab is backgrounded doesn't yank the window
  // forward (the run was started, then the user looked away).
  function diagFocusTarget(card) {
    var input = card.querySelector('.diag-target');
    if (input && document.visibilityState !== 'hidden') {
      input.focus({ preventScroll: true });
    }
  }

  // Hide mtr's Cycles/Size fields while "Run continuously" is checked.
  function applyMtrContinuous(card) {
    var cb = card.querySelector('.diag-mtr-continuous');
    var continuous = cb ? cb.checked : true;
    card.querySelectorAll('.diag-mtr-param').forEach(function (el) {
      el.hidden = continuous;
    });
  }

  // ── Target history (shared across tools, persisted in localStorage) ──
  var DIAG_HIST_KEY = 'infix.diag.history';

  function diagLoadHistory() {
    try { return JSON.parse(localStorage.getItem(DIAG_HIST_KEY)) || []; }
    catch (_) { return []; }
  }

  function diagRenderHistory() {
    var dl = document.getElementById('diag-history');
    if (!dl) return;
    dl.innerHTML = '';
    diagLoadHistory().forEach(function (h) {
      var o = document.createElement('option');
      o.value = h;
      dl.appendChild(o);
    });
  }

  function diagPushHistory(target) {
    if (!target) return;
    var list = diagLoadHistory().filter(function (h) { return h !== target; });
    list.unshift(target);
    if (list.length > 25) list = list.slice(0, 25);
    try { localStorage.setItem(DIAG_HIST_KEY, JSON.stringify(list)); } catch (_) {}
    diagRenderHistory();
  }

  // Upsert one mtr hop row keyed by hop index.
  function diagUpsertHop(tbody, hop) {
    var row = tbody.querySelector('tr[data-idx="' + hop.idx + '"]');
    if (!row) {
      row = document.createElement('tr');
      row.setAttribute('data-idx', hop.idx);
      row.innerHTML =
        '<td class="diag-mtr-hop"></td><td class="diag-mtr-host"></td>' +
        '<td class="diag-mtr-loss"></td><td></td><td></td><td></td><td></td><td></td>';
      // Keep rows ordered by hop index even if events arrive out of order.
      var next = null;
      tbody.querySelectorAll('tr').forEach(function (r) {
        if (next === null && parseInt(r.getAttribute('data-idx'), 10) > hop.idx) next = r;
      });
      tbody.insertBefore(row, next);
    }
    var c = row.children;
    c[0].textContent = hop.idx + 1;
    c[1].textContent = hop.host;
    c[2].textContent = hop.loss.toFixed(1);
    c[2].classList.toggle('diag-loss-bad', hop.loss > 0);
    c[3].textContent = hop.snt;
    c[4].textContent = hop.last.toFixed(1);
    c[5].textContent = hop.avg.toFixed(1);
    c[6].textContent = hop.best.toFixed(1);
    c[7].textContent = hop.worst.toFixed(1);
  }

  function diagBuildURL(card) {
    var tool = card.dataset.active;
    var q = new URLSearchParams();
    q.set('tool', tool);
    q.set('target', card.querySelector('.diag-target').value.trim());
    q.set('family', card.querySelector('.diag-family').value);
    if (isNetTool(tool)) {
      q.set('iface', card.querySelector('.diag-iface').value);
    }
    if (tool === 'ping') {
      var count = card.querySelector('.diag-count');
      var size = card.querySelector('.diag-size');
      if (count) q.set('count', count.value);
      if (size) q.set('size', size.value);
    } else if (tool === 'traceroute') {
      var maxhops = card.querySelector('.diag-maxhops');
      if (maxhops) q.set('maxhops', maxhops.value);
    } else if (tool === 'mtr') {
      // Omitting count tells the backend to run forever; only send the
      // cycle/size knobs when the user has opted out of continuous mode.
      var cont = card.querySelector('.diag-mtr-continuous');
      if (cont && !cont.checked) {
        var mc = card.querySelector('.diag-mtr-count');
        var ms = card.querySelector('.diag-mtr-size');
        if (mc) q.set('count', mc.value);
        if (ms) q.set('size', ms.value);
      }
    } else if (tool === 'nmap') {
      var scan = card.querySelector('.diag-scan');
      if (scan) q.set('scan', scan.value);
    }
    return '/maintenance/diagnostics/run?' + q.toString();
  }

  function diagRunDNS(card) {
    var name = card.querySelector('.diag-target').value.trim();
    if (!name) {
      diagSetStatus(card, 'enter a name', 'err');
      return;
    }
    var dns = card.querySelector('.diag-dns');
    var family = card.querySelector('.diag-family').value;
    diagSetStatus(card, 'resolving…', 'pending');
    card.querySelector('.diag-placeholder').hidden = true;
    dns.hidden = false;
    fetch('/maintenance/diagnostics/resolve?name=' + encodeURIComponent(name) +
          '&family=' + encodeURIComponent(family), { headers: { 'HX-Request': 'true' } })
      .then(function (r) { return r.text(); })
      .then(function (html) { dns.innerHTML = html; diagSetStatus(card, '', ''); diagFocusTarget(card); })
      .catch(function () { diagSetStatus(card, 'lookup failed', 'err'); });
  }

  function diagRunStream(card) {
    var tool = card.dataset.active;
    var target = card.querySelector('.diag-target').value.trim();
    if (!target) {
      diagSetStatus(card, 'enter a target', 'err');
      return;
    }

    diagTeardown();
    card.querySelector('.diag-placeholder').hidden = true;

    var textPane = card.querySelector('.diag-text');
    var mtrTable = card.querySelector('.diag-mtr');
    var mtrBody = mtrTable.querySelector('tbody');
    if (tool === 'mtr') {
      mtrBody.innerHTML = '';
      mtrTable.hidden = false;
      textPane.hidden = true;
    } else {
      textPane.textContent = '';
      textPane.hidden = false;
      mtrTable.hidden = true;
    }

    diagSetStatus(card, 'running…', 'pending');
    diagSetRunning(card, true);

    diagES = new EventSource(diagBuildURL(card));
    diagES.addEventListener('line', function (evt) {
      textPane.textContent += evt.data + '\n';
      textPane.scrollTop = textPane.scrollHeight;
    });
    diagES.addEventListener('hop', function (evt) {
      try { diagUpsertHop(mtrBody, JSON.parse(evt.data)); } catch (_) {}
    });
    diagES.addEventListener('done', function () {
      diagTeardown();
      diagSetStatus(card, 'done', '');
      diagSetRunning(card, false);
      diagFocusTarget(card);
    });
    diagES.onerror = function () {
      // Transient drops auto-reconnect; only react to a terminal close.
      if (diagES && diagES.readyState === EventSource.CLOSED) {
        diagTeardown();
        diagSetStatus(card, 'disconnected', 'err');
        diagSetRunning(card, false);
      }
    };
  }

  function diagRun(card) {
    var input = card.querySelector('.diag-target');
    var target = input.value.trim();
    if (target) diagPushHistory(target);
    // Blur the target so Firefox dismisses the datalist popup, which it
    // otherwise leaves hovering over the results after a selection.
    if (input) input.blur();
    if (card.dataset.active === 'dns') {
      diagRunDNS(card);
    } else {
      diagRunStream(card);
    }
  }

  function diagStop(card) {
    diagTeardown();
    diagSetStatus(card, 'stopped', '');
    diagSetRunning(card, false);
    diagFocusTarget(card);
  }

  function diagClear(card) {
    card.querySelector('.diag-text').textContent = '';
    card.querySelector('.diag-mtr tbody').innerHTML = '';
    card.querySelector('.diag-dns').innerHTML = '';
    card.querySelector('.diag-text').hidden = true;
    card.querySelector('.diag-mtr').hidden = true;
    card.querySelector('.diag-dns').hidden = true;
    card.querySelector('.diag-placeholder').hidden = false;
    diagSetStatus(card, '', '');
  }

  function initDiagnostics(scope) {
    var root = scope || document;
    root.querySelectorAll('.diag-card:not([data-init])').forEach(function (card) {
      card.dataset.init = 'true';

      card.querySelectorAll('.diag-tab').forEach(function (tab) {
        tab.addEventListener('click', function () {
          setActiveTool(card, tab.getAttribute('data-tool'));
          // Focus the target field so the user can type straight away.
          var input = card.querySelector('.diag-target');
          if (input) input.focus();
        });
      });

      card.querySelector('.diag-run').addEventListener('click', function () { diagRun(card); });
      card.querySelector('.diag-stop').addEventListener('click', function () { diagStop(card); });
      card.querySelector('.diag-clear').addEventListener('click', function () { diagClear(card); });

      var cont = card.querySelector('.diag-mtr-continuous');
      if (cont) cont.addEventListener('change', function () { applyMtrContinuous(card); });

      // Enter in ANY form field (target, count, family, …) runs the
      // active tool — Run is the natural default action.  Buttons are
      // excluded so Enter on Run/Stop/Clear keeps their own behavior
      // and doesn't fire twice.
      card.querySelector('.diag-form').addEventListener('keydown', function (e) {
        if (e.key === 'Enter' && e.target.tagName !== 'BUTTON') {
          e.preventDefault();
          diagRun(card);
        }
      });

      diagRenderHistory();
      setActiveTool(card, card.dataset.active || 'ping');

      // Focus the target on load too (tab clicks already do).  Guard on
      // visibility so a background load doesn't yank the window forward
      // under X11 WMs — same reasoning as the data-autofocus handler.
      var target = card.querySelector('.diag-target');
      if (target && document.visibilityState !== 'hidden') {
        target.focus({ preventScroll: true });
      }
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    initDiagnostics(document);
    if (window.htmx) {
      document.body.addEventListener('htmx:afterSwap', function (evt) {
        // Navigating away from the diagnostics page removes the card;
        // make sure any running stream is torn down server-side.
        if (!document.querySelector('.diag-card')) diagTeardown();
        var s = (evt.detail && evt.detail.target) || document;
        initDiagnostics(s);
      });
    }
  });
})();

// ─── Maintenance > Backup & Support: support bundle download ────────────────
// `support collect` takes up to a minute and returns the whole archive at
// once, so a plain <a download> would just hang with no feedback.  Instead
// fetch the bundle as a blob with a visible "generating…" state, then save
// it client-side using the filename the server set in Content-Disposition.
(function () {
  function filenameFromDisposition(cd, fallback) {
    var m = /filename="?([^";]+)"?/.exec(cd || '');
    return m ? m[1] : fallback;
  }

  function initSupportBundle(scope) {
    (scope || document).querySelectorAll('.support-generate:not([data-init])').forEach(function (btn) {
      btn.dataset.init = 'true';
      var card = btn.closest('.info-card');
      var pass = card.querySelector('.support-pass');
      var status = card.querySelector('.support-status');

      function setStatus(text, cls) {
        if (!status) return;
        status.textContent = text || '';
        status.className = 'support-status' + (cls ? ' ' + cls : '');
      }

      btn.addEventListener('click', function () {
        btn.disabled = true;
        setStatus('Collecting… (up to a minute)', 'pending');

        var body = new FormData();
        if (pass && pass.value) body.append('password', pass.value);

        fetch('/maintenance/support-bundle', {
          method: 'POST',
          headers: { 'X-CSRF-Token': btn.getAttribute('data-csrf') || '' },
          body: body
        }).then(function (r) {
          if (!r.ok) throw new Error('HTTP ' + r.status);
          var name = filenameFromDisposition(r.headers.get('Content-Disposition'), 'support-bundle.tar.gz');
          return r.blob().then(function (blob) { return { blob: blob, name: name }; });
        }).then(function (res) {
          var url = URL.createObjectURL(res.blob);
          var a = document.createElement('a');
          a.href = url;
          a.download = res.name;
          document.body.appendChild(a);
          a.click();
          a.remove();
          setTimeout(function () { URL.revokeObjectURL(url); }, 10000);
          setStatus('Downloaded ' + res.name, '');
          if (pass) pass.value = '';
        }).catch(function (e) {
          setStatus('Failed to generate bundle', 'err');
          if (window.console) console.warn('[support]', e);
        }).finally(function () {
          btn.disabled = false;
        });
      });
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    initSupportBundle(document);
    if (window.htmx) {
      document.body.addEventListener('htmx:afterSwap', function (evt) {
        initSupportBundle((evt.detail && evt.detail.target) || document);
      });
    }
  });
})();

// ─── Web console (ttyd) link ────────────────────────────────────────────────
// The console runs on its own HTTPS port (7681), so the link target can't be
// a relative path — build it from the current hostname at load time.  CSP
// (script-src 'self') forbids an inline onclick, hence this lives here.  The
// topbar/sidebar only render on full page loads, so DOMContentLoaded covers it.
(function () {
  document.addEventListener('DOMContentLoaded', function () {
    var links = document.querySelectorAll('[data-console-link]');
    if (!links.length) return;
    var url = 'https://' + window.location.hostname + ':7681/';
    links.forEach(function (a) { a.href = url; });
  });
})();
