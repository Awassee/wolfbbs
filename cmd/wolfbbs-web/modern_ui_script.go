package main

const modernUIScriptTag = `
<script id="wolfbbs-modern-ui-js">
(() => {
  function initWolfbbsModernUI() {
    if (document.documentElement.dataset.wolfbbsModernUi === "1") return;
    document.documentElement.dataset.wolfbbsModernUi = "1";

  const uiPrefsKey = "wolfbbs:ui:prefs:v2";
  const sectionStatePrefix = "wolfbbs:ui:section:";
  const favoritesKey = "wolfbbs:ui:favorites:v1";
  const tableViewPrefix = "wolfbbs:ui:tableview:";
  const notesKey = "wolfbbs:ui:quick-notes:v1";
  const focusKey = "wolfbbs:ui:focus-timer:v1";
  const replayPrefix = "wolfbbs:ui:replay:";
  const goalsPrefix = "wolfbbs:ui:goals:";
  const telemetryKey = "wolfbbs:ui:telemetry:v1";
  const paletteHistoryKey = "wolfbbs:ui:palette-history:v1";
  const focusModeKey = "wolfbbs:ui:focus-mode:v1";
  const guideStateKey = "wolfbbs:ui:guide-state:v1";
  const toastHistoryKey = "wolfbbs:ui:toast-history:v1";
  const undoStackKey = "wolfbbs:ui:undo-stack:v1";
  const workspaceKey = "wolfbbs:ui:workspaces:v1";
  const revisitKeyPrefix = "wolfbbs:ui:last-visit:";
  const kpiSnapshotKeyPrefix = "wolfbbs:ui:kpi-snapshot:";
  const spotlightKey = "wolfbbs:ui:spotlight:v1";
  const checkpointKey = "wolfbbs:ui:checkpoints:v1";
  const draftKeyPrefix = "wolfbbs:ui:draft:";
  const incidentKey = "wolfbbs:ui:incidents:v1";
  const playbookKeyPrefix = "wolfbbs:ui:playbook:";
  const reminderKey = "wolfbbs:ui:reminders:v1";
  const releaseGateKey = "wolfbbs:ui:release-gate:v1";
  const feedbackKey = "wolfbbs:ui:feedback:v1";
  const kpiWatchKeyPrefix = "wolfbbs:ui:kpi-watch:";
  const routePinsKey = "wolfbbs:ui:route-pins:v1";
  const sectionPinsKeyPrefix = "wolfbbs:ui:section-pins:";
  const sectionDoneKeyPrefix = "wolfbbs:ui:section-done:";
  const bugCaptureKey = "wolfbbs:ui:bug-capture:v1";
  const sessionStartKey = "wolfbbs:ui:session-start:v1";
  const sessionTrailKey = "wolfbbs:ui:session-trail:v1";
  const actionDockStateKey = "wolfbbs:ui:action-dock:v1";
  const syncChannelName = "wolfbbs-modern-ui-sync";
  const syncTabID = "tab-" + Math.random().toString(36).slice(2, 10);
  let wolfbbsSyncChannel = null;

  function readJSON(key, fallback) {
    try {
      const raw = localStorage.getItem(key);
      if (!raw) return fallback;
      const parsed = JSON.parse(raw);
      return parsed === null ? fallback : parsed;
    } catch (_) {
      return fallback;
    }
  }

  function writeJSON(key, value) {
    try {
      localStorage.setItem(key, JSON.stringify(value));
      window.dispatchEvent(new CustomEvent("wolfbbs-storage-updated", { detail: { key: key } }));
      if (wolfbbsSyncChannel) {
        wolfbbsSyncChannel.postMessage({
          type: "storage:update",
          key: key,
          at: new Date().toISOString(),
          source: syncTabID
        });
      }
      return true;
    } catch (_) {
      return false;
    }
  }

  function clamp(value, min, max) {
    return Math.min(max, Math.max(min, value));
  }

  function ensureToastRegion() {
    let region = document.getElementById("wolfbbsToastRegion");
    if (region) return region;
    region = document.createElement("div");
    region.id = "wolfbbsToastRegion";
    document.body.appendChild(region);
    return region;
  }

  function ensureLiveRegion() {
    let region = document.getElementById("wolfbbsLiveRegion");
    if (region) return region;
    region = document.createElement("div");
    region.id = "wolfbbsLiveRegion";
    region.className = "wolfbbs-live-region";
    region.setAttribute("role", "status");
    region.setAttribute("aria-live", "polite");
    region.setAttribute("aria-atomic", "true");
    document.body.appendChild(region);
    return region;
  }

  function announceLive(message) {
    const text = String(message || "").trim();
    if (!text) return;
    const region = ensureLiveRegion();
    region.textContent = "";
    window.setTimeout(() => {
      region.textContent = text;
    }, 12);
  }

  function showToast(message, kind) {
    const text = String(message || "").trim();
    if (!text) return;
    const region = ensureToastRegion();
    const item = document.createElement("div");
    item.className = "wolfbbs-toast";
    item.setAttribute("data-kind", kind || "ok");
    item.textContent = text;
    region.appendChild(item);
    window.requestAnimationFrame(() => item.classList.add("show"));
    window.setTimeout(() => {
      item.classList.remove("show");
      window.setTimeout(() => item.remove(), 170);
    }, 1600);
    announceLive(text);
    const history = readJSON(toastHistoryKey, []);
    history.unshift({
      text: text,
      kind: kind || "ok",
      at: new Date().toISOString()
    });
    writeJSON(toastHistoryKey, history.slice(0, 60));
  }

  function pushUndoAction(action) {
    if (!action || !action.type) return;
    const stack = readJSON(undoStackKey, []);
    stack.unshift(action);
    writeJSON(undoStackKey, stack.slice(0, 20));
  }

  function popUndoAction() {
    const stack = readJSON(undoStackKey, []);
    if (!stack.length) return null;
    const action = stack.shift();
    writeJSON(undoStackKey, stack);
    return action;
  }

  function loadTelemetry() {
    return Object.assign({
      routeHits: {},
      events: {},
      lastUpdatedAt: ""
    }, readJSON(telemetryKey, {}));
  }

  function saveTelemetry(next) {
    const payload = Object.assign({
      routeHits: {},
      events: {},
      lastUpdatedAt: ""
    }, next || {});
    payload.lastUpdatedAt = new Date().toISOString();
    writeJSON(telemetryKey, payload);
    return payload;
  }

  function trackTelemetry(eventKey) {
    const key = String(eventKey || "").trim();
    if (!key) return;
    const telemetry = loadTelemetry();
    telemetry.routeHits[currentRoute || "/"] = (telemetry.routeHits[currentRoute || "/"] || 0) + 1;
    telemetry.events[key] = (telemetry.events[key] || 0) + 1;
    saveTelemetry(telemetry);
    window.dispatchEvent(new Event("wolfbbs-telemetry-updated"));
  }

  function downloadJSONFile(filename, payload) {
    const body = JSON.stringify(payload, null, 2);
    const blob = new Blob([body], { type: "application/json;charset=utf-8" });
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    window.setTimeout(() => {
      URL.revokeObjectURL(link.href);
      link.remove();
    }, 0);
  }

  const uiPrefs = Object.assign({
    theme: "night",
    density: "comfortable",
    fontScale: 1,
    layout: "standard",
    accent: "blue",
    motion: "full"
  }, readJSON(uiPrefsKey, {}));

  function persistUIPrefs() {
    writeJSON(uiPrefsKey, uiPrefs);
  }

  function applyUIPrefs() {
    const theme = ["default", "contrast", "night"].includes(uiPrefs.theme) ? uiPrefs.theme : "night";
    const density = uiPrefs.density === "compact" ? "compact" : "comfortable";
    const fontScale = clamp(Number(uiPrefs.fontScale || 1), 0.9, 1.2);
    const layout = ["standard", "wide", "focus"].includes(uiPrefs.layout) ? uiPrefs.layout : "standard";
    const accent = ["blue", "teal", "amber"].includes(uiPrefs.accent) ? uiPrefs.accent : "blue";
    const motion = uiPrefs.motion === "reduced" ? "reduced" : "full";
    uiPrefs.theme = theme;
    uiPrefs.density = density;
    uiPrefs.fontScale = fontScale;
    uiPrefs.layout = layout;
    uiPrefs.accent = accent;
    uiPrefs.motion = motion;
    document.body.setAttribute("data-theme-mode", theme);
    document.body.setAttribute("data-density", density);
    document.body.setAttribute("data-layout-mode", layout);
    document.body.setAttribute("data-accent-mode", accent);
    document.body.setAttribute("data-motion-mode", motion);
    document.body.classList.toggle("wolfbbs-motion-reduced", motion === "reduced");
    document.documentElement.style.setProperty("--font-scale", String(fontScale));
  }

  applyUIPrefs();

  function normalizePath(raw) {
    const value = String(raw || "").trim();
    if (!value || value[0] !== "/") return "";
    const base = value.split("?")[0].replace(/\/+$/, "");
    return base || "/";
  }

  const title = (document.querySelector("h1") && document.querySelector("h1").textContent.trim()) || document.title || "WolfBBS";
  const currentPath = location.pathname + location.search;
  const currentRoute = normalizePath(currentPath);
  if (currentRoute) {
    document.body.setAttribute("data-route", currentRoute);
  }
  try {
    const startedAt = Number(readJSON(sessionStartKey, Date.now()) || Date.now());
    writeJSON(sessionStartKey, startedAt);
    const trail = Object.assign({ visited: [] }, readJSON(sessionTrailKey, {}));
    const nextVisited = [currentRoute || "/",].concat((trail.visited || []).filter((item) => item !== (currentRoute || "/"))).slice(0, 20);
    writeJSON(sessionTrailKey, {
      startedAt: startedAt,
      updatedAt: new Date().toISOString(),
      visited: nextVisited
    });
  } catch (_) {}
  trackTelemetry("route:view");
  function isCurrentNav(href) {
    const candidate = normalizePath(href);
    if (!candidate) return false;
    if (candidate === "/") return currentRoute === "/";
    return currentRoute === candidate || currentRoute.startsWith(candidate + "/");
  }

  const navRows = Array.from(document.querySelectorAll("p")).filter((p) => p.querySelectorAll("a").length >= 3 && p.textContent.includes("|"));
  navRows.forEach((row, index) => {
    row.classList.add("wolfbbs-nav-row");
    row.dataset.wolfbbsNavLevel = index === 0 ? "primary" : "secondary";
    row.dataset.wolfbbsNavCount = String(row.querySelectorAll("a[href]").length);
    row.querySelectorAll("a[href]").forEach((anchor) => {
      const href = anchor.getAttribute("href");
      if (isCurrentNav(href)) {
        anchor.classList.add("wolfbbs-nav-active");
      }
    });
  });
  function compactNavRows() {
    navRows.forEach((row) => {
      if (!row || row.dataset.wolfbbsNavCompact === "1") return;
      const items = Array.from(row.querySelectorAll("a[href]")).map((anchor) => ({
        href: anchor.getAttribute("href") || "",
        label: (anchor.textContent || "").replace(/\s+/g, " ").trim(),
        active: anchor.classList.contains("wolfbbs-nav-active")
      })).filter((item) => item.href && item.label);
      const maxVisible = row.dataset.wolfbbsNavLevel === "secondary" ? 3 : 5;
      if (items.length <= maxVisible + 1) {
        row.dataset.wolfbbsNavCompact = "1";
        return;
      }
      const keep = [];
      const seen = new Set();
      items.forEach((item, index) => {
        const mustKeep = index < maxVisible || item.active;
        if (!mustKeep) return;
        const key = item.href + "::" + item.label;
        if (seen.has(key)) return;
        seen.add(key);
        keep.push(item);
      });
      const overflow = items.filter((item) => !keep.some((entry) => entry.href === item.href && entry.label === item.label));
      row.innerHTML = "";
      keep.forEach((item) => {
        const link = document.createElement("a");
        link.href = item.href;
        link.textContent = item.label;
        if (item.active) link.classList.add("wolfbbs-nav-active");
        row.appendChild(link);
      });
      if (overflow.length) {
        const details = document.createElement("details");
        details.className = "wolfbbs-nav-more";
        const summary = document.createElement("summary");
        summary.textContent = "More (" + overflow.length + ")";
        const panel = document.createElement("div");
        panel.className = "wolfbbs-nav-more-panel";
        overflow.forEach((item) => {
          const link = document.createElement("a");
          link.href = item.href;
          link.textContent = item.label;
          if (item.active) link.classList.add("wolfbbs-nav-active");
          panel.appendChild(link);
        });
        details.appendChild(summary);
        details.appendChild(panel);
        row.appendChild(details);
      }
      row.dataset.wolfbbsNavCompact = "1";
    });
  }
  compactNavRows();
  function routeKind(pathname) {
    const p = String(pathname || "");
    if (p.startsWith("/admin")) return "admin";
    if (p === "/connect" || p === "/tour" || p === "/help" || p === "/login" || p === "/start") return "guest";
    return "caller";
  }

  function routeMatches(pathname, prefixes) {
    const path = normalizePath(pathname);
    if (!path) return false;
    return (prefixes || []).some((prefix) => path === prefix || path.indexOf(prefix + "/") === 0);
  }

  function routeProfile(pathname) {
    const denseRoutes = [
      "/today",
      "/boards",
      "/mail",
      "/doors",
      "/directory",
      "/admin/setup",
      "/admin/config",
      "/admin/users",
      "/admin/files",
      "/admin/mail",
      "/admin/ops",
    ];
    const visualRoutes = [
      "/today",
      "/boards",
      "/doors",
      "/admin/setup",
      "/admin/config",
    ];
    const dense = routeMatches(pathname, denseRoutes);
    return {
      dense: dense,
      compactGuide: dense,
      autoCollapseSections: dense,
      compactSectionNav: dense,
      mobileUtilityHub: dense || routeKind(pathname) === "admin",
      visualRegression: routeMatches(pathname, visualRoutes),
    };
  }

  function isCompactViewport() {
    return typeof window.matchMedia === "function" && window.matchMedia("(max-width: 820px)").matches;
  }

  const currentRouteProfile = routeProfile(currentRoute);
  document.body.setAttribute("data-route-profile", currentRouteProfile.dense ? "dense" : "standard");
  if (currentRouteProfile.visualRegression) {
    document.body.setAttribute("data-visual-regression", "1");
  }

  function routeMatchesPrefix(pathname, prefixes) {
    return (prefixes || []).some((prefix) => pathname === prefix || pathname.indexOf(prefix + "/") === 0);
  }

  function shouldMountPrimerSurface(pathname) {
    return routeMatchesPrefix(normalizePath(pathname || currentRoute || "/"), [
      "/start",
      "/help",
      "/connect",
      "/tour"
    ]);
  }

  function shouldMountRouteScaffolding(pathname) {
    return routeMatchesPrefix(normalizePath(pathname || currentRoute || "/"), [
      "/start",
      "/help",
      "/connect"
    ]);
  }

  function routeLabel(kind) {
    if (kind === "admin") return "Sysop Lane";
    if (kind === "guest") return "Guest Lane";
    return "Caller Lane";
  }

  function headingBaseLabel(heading) {
    if (!heading) return "";
    const clone = heading.cloneNode(true);
    Array.from(clone.querySelectorAll("a,button")).forEach((node) => node.remove());
    return (clone.textContent || "").replace(/\s+/g, " ").trim();
  }

  function buildPageShell() {
    const h1 = document.querySelector("h1");
    if (!h1 || h1.closest(".wolfbbs-page-hero")) return;
    const navRow = navRows.length ? navRows[0] : null;
    const lead = Array.from(document.querySelectorAll("body > p")).find((p) => {
      if (p === navRow) return false;
      if (p.classList.contains("wolfbbs-nav-row")) return false;
      if (p.querySelector("a")) return false;
      const text = (p.textContent || "").trim();
      return text.length >= 18;
    });

    const hero = document.createElement("header");
    hero.className = "wolfbbs-page-hero";
    const heroMain = document.createElement("div");
    heroMain.className = "wolfbbs-page-hero-main";
    const heroMeta = document.createElement("div");
    heroMeta.className = "wolfbbs-page-hero-meta";

    const kind = routeKind(currentRoute);
    const laneChip = document.createElement("span");
    laneChip.className = "wolfbbs-hero-chip";
    laneChip.setAttribute("data-kind", kind);
    laneChip.textContent = routeLabel(kind);
    heroMeta.appendChild(laneChip);

    h1.parentNode.insertBefore(hero, h1);
    hero.appendChild(heroMain);
    hero.appendChild(heroMeta);
    heroMain.appendChild(h1);
    if (lead && lead.parentNode === document.body) {
      heroMain.appendChild(lead);
    }

    if (document.querySelector("main.wolfbbs-main")) return;
    const scriptNodeRaw = document.getElementById("wolfbbs-modern-ui-js");
    const scriptNode = scriptNodeRaw && scriptNodeRaw.parentNode === document.body ? scriptNodeRaw : null;
    const contentStart = navRow ? navRow.nextSibling : hero.nextSibling;
    if (!contentStart) return;
    const main = document.createElement("main");
    main.className = "wolfbbs-main";
    main.id = "wolfbbs-main-anchor";
    let node = contentStart;
    while (node && node !== scriptNode) {
      const next = node.nextSibling;
      if (node.nodeType === 1 || (node.nodeType === 3 && node.textContent.trim())) {
        main.appendChild(node);
      }
      node = next;
    }
    if (scriptNode) {
      document.body.insertBefore(main, scriptNode);
    } else {
      document.body.appendChild(main);
    }
  }

  buildPageShell();

  function prefersReducedMotion() {
    return uiPrefs.motion === "reduced" || (typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
  }

  function setUIPref(key, value, toastLabel) {
    uiPrefs[key] = value;
    persistUIPrefs();
    applyUIPrefs();
    if (toastLabel) {
      showToast(toastLabel, "ok");
    }
  }

  function classifyBentoDensity(node) {
    if (!node) return "medium";
    if (node.matches && node.matches(".wolfbbs-primer,.wolfbbs-spatial-card,.wolfbbs-chat-layout,.wolfbbs-dashboard")) return "high";
    if (node.matches && node.matches(".wolfbbs-kpi-card")) return "low";
    const tableRows = node.querySelectorAll("table tr").length;
    const formFields = node.querySelectorAll("input,select,textarea,button").length;
    const links = node.querySelectorAll("a").length;
    const items = node.querySelectorAll("li,p,dt,dd").length;
    const bodyText = (node.textContent || "").replace(/\s+/g, " ").trim();
    const score = (tableRows * 5) + (formFields * 2) + links + Math.min(items, 12) + Math.min(Math.floor(bodyText.length / 180), 8);
    if (score >= 18) return "high";
    if (score <= 6) return "low";
    return "medium";
  }

  function applyBentoDensity(root) {
    const scope = root || document;
    const selectors = [
      ".wolfbbs-card",
      ".wolfbbs-helper-card",
      ".wolfbbs-kpi-card",
      ".wolfbbs-action-card",
      ".wolfbbs-primer",
      ".wolfbbs-spatial-card",
      "main.wolfbbs-main > section",
      "main.wolfbbs-main > article",
      "main.wolfbbs-main > form",
      "main.wolfbbs-main > .wolfbbs-chat-layout",
      "main.wolfbbs-main > .wolfbbs-table-wrap"
    ];
    Array.from(scope.querySelectorAll(selectors.join(","))).forEach((node) => {
      if (node.dataset.wolfbbsDensityLocked === "1") return;
      node.dataset.density = classifyBentoDensity(node);
    });
  }

  function markRevealTargets(root) {
    const scope = root || document;
    const selectors = [
      ".wolfbbs-page-hero",
      "main.wolfbbs-main > *",
      ".wolfbbs-card",
      ".wolfbbs-helper-card",
      ".wolfbbs-kpi-card",
      ".wolfbbs-action-card",
      ".wolfbbs-primer",
      ".wolfbbs-spatial-card"
    ];
    return Array.from(scope.querySelectorAll(selectors.join(","))).filter((node) => {
      if (node.dataset.wolfbbsReveal === "1") return false;
      node.dataset.wolfbbsReveal = "1";
      node.classList.add("wolfbbs-reveal");
      return true;
    });
  }

  function mountStructuralMotion() {
    const targets = markRevealTargets(document);
    if (!targets.length) return;
    if (prefersReducedMotion() || typeof IntersectionObserver !== "function") {
      targets.forEach((node) => node.classList.add("is-visible"));
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry, idx) => {
        if (!entry.isIntersecting) return;
        const node = entry.target;
        node.style.setProperty("--wolfbbs-reveal-delay", String(Math.min(idx, 6) * 40) + "ms");
        node.classList.add("is-visible");
        observer.unobserve(node);
      });
    }, {
      threshold: 0.12,
      rootMargin: "0px 0px -8% 0px"
    });
    targets.forEach((node) => observer.observe(node));
  }

  function mountMorphingUI() {
    if (document.body.dataset.wolfbbsMorphBound === "1") return;
    document.body.dataset.wolfbbsMorphBound = "1";
    document.addEventListener("pointerdown", (event) => {
      const target = event.target && event.target.closest ? event.target.closest("button, .wolfbbs-action-card, .wolfbbs-palette-item, .wolfbbs-omnibar-module, .wolfbbs-hero-chip") : null;
      if (!target) return;
      target.classList.add("wolfbbs-morph-active");
      window.setTimeout(() => target.classList.remove("wolfbbs-morph-active"), 220);
    }, true);
  }

  function mountKineticHero() {
    const hero = document.querySelector(".wolfbbs-page-hero");
    const titleNode = hero && hero.querySelector("h1");
    if (!hero || !titleNode || hero.dataset.wolfbbsKinetic === "1") return;
    hero.dataset.wolfbbsKinetic = "1";
    titleNode.classList.add("wolfbbs-kinetic-title");
    function syncFromScroll() {
      const limit = Math.max(220, hero.offsetHeight * 1.8);
      const depth = clamp(window.scrollY / limit, 0, 1);
      titleNode.style.setProperty("--wolfbbs-hero-weight", String(Math.round(810 - (depth * 170))));
      titleNode.style.setProperty("--wolfbbs-hero-scale", String((0.028 - (depth * 0.02)).toFixed(3)));
      titleNode.style.setProperty("--wolfbbs-hero-glow", String((1 - (depth * 0.65)).toFixed(3)));
      titleNode.style.setProperty("--wolfbbs-hero-shift", String((1 - depth).toFixed(3)));
    }
    function syncFromPointer(event) {
      const rect = hero.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      const x = ((event.clientX - rect.left) / rect.width) - 0.5;
      const y = ((event.clientY - rect.top) / rect.height) - 0.5;
      titleNode.style.setProperty("--wolfbbs-hero-x", String((x * 8).toFixed(2)));
      titleNode.style.setProperty("--wolfbbs-hero-y", String((y * 6).toFixed(2)));
    }
    function resetPointer() {
      titleNode.style.setProperty("--wolfbbs-hero-x", "0");
      titleNode.style.setProperty("--wolfbbs-hero-y", "0");
    }
    hero.addEventListener("pointermove", syncFromPointer);
    hero.addEventListener("pointerleave", resetPointer);
    window.addEventListener("scroll", syncFromScroll, { passive: true });
    syncFromScroll();
    resetPointer();
  }

  function isSpatialPreviewRoute() {
    return ["/start", "/showcase", "/connect", "/tour", "/help", "/admin/launch"].some((path) => currentRoute === path || currentRoute.indexOf(path + "/") === 0);
  }

  function syncSpatialCanvas(section) {
    const shell = section && section.querySelector(".wolfbbs-spatial-shell");
    const canvas = section && section.querySelector(".wolfbbs-webgl-canvas");
    if (!shell || !canvas) return;
    if (canvas.dataset.wolfbbsSpatial === "1") return;
    canvas.dataset.wolfbbsSpatial = "1";
    const gl = canvas.getContext("webgl", { alpha: true, antialias: true });
    const ctx2d = gl ? null : canvas.getContext("2d");
    const state = { x: 0, y: 0, active: false };
    function resize() {
      const rect = shell.getBoundingClientRect();
      const dpr = Math.max(1, window.devicePixelRatio || 1);
      canvas.width = Math.max(1, Math.round(rect.width * dpr));
      canvas.height = Math.max(1, Math.round(rect.height * dpr));
      canvas.style.width = rect.width + "px";
      canvas.style.height = rect.height + "px";
      if (gl) {
        gl.viewport(0, 0, canvas.width, canvas.height);
      }
    }
    function paint(timeMs) {
      const drift = (timeMs || 0) * 0.0012;
      if (gl) {
        const r = 0.03 + (Math.sin(drift) * 0.018) + 0.03;
        const g = 0.05 + (Math.cos(drift * 0.7) * 0.014) + 0.03;
        const b = 0.09 + (Math.sin(drift * 0.9) * 0.022) + 0.04;
        gl.clearColor(r, g, b, 0.92);
        gl.clear(gl.COLOR_BUFFER_BIT);
      } else if (ctx2d) {
        const width = canvas.width;
        const height = canvas.height;
        const grad = ctx2d.createLinearGradient(0, 0, width, height);
        grad.addColorStop(0, "rgba(31,93,226,0.88)");
        grad.addColorStop(0.55, "rgba(17,29,58,0.95)");
        grad.addColorStop(1, "rgba(255,83,213,0.82)");
        ctx2d.clearRect(0, 0, width, height);
        ctx2d.fillStyle = grad;
        ctx2d.fillRect(0, 0, width, height);
      }
      if (!prefersReducedMotion()) {
        window.requestAnimationFrame(paint);
      }
    }
    function syncOrbit(clientX, clientY) {
      const rect = shell.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      const x = clamp((((clientX - rect.left) / rect.width) - 0.5) * 18, -12, 12);
      const y = clamp((((clientY - rect.top) / rect.height) - 0.5) * -14, -10, 10);
      shell.style.setProperty("--wolfbbs-orbit-x", x.toFixed(2) + "deg");
      shell.style.setProperty("--wolfbbs-orbit-y", y.toFixed(2) + "deg");
    }
    shell.addEventListener("pointermove", (event) => {
      state.active = true;
      syncOrbit(event.clientX, event.clientY);
    });
    shell.addEventListener("pointerleave", () => {
      state.active = false;
      shell.style.setProperty("--wolfbbs-orbit-x", "0deg");
      shell.style.setProperty("--wolfbbs-orbit-y", "0deg");
    });
    window.addEventListener("resize", resize);
    resize();
    paint(0);
  }

  function mountSpatialPreview() {
    if (!isSpatialPreviewRoute() || document.querySelector(".wolfbbs-spatial-card")) return;
    const main = document.querySelector("main.wolfbbs-main");
    const hero = document.querySelector(".wolfbbs-page-hero");
    if (!main && !hero) return;
    const section = document.createElement("section");
    section.className = "wolfbbs-spatial-card";
    section.dataset.density = "high";
    section.innerHTML = '' +
      '<div class="wolfbbs-spatial-copy">' +
        '<span class="wolfbbs-hero-chip" data-kind="guest">Spatial Preview</span>' +
        '<h2>Spin through the board before you commit to a workflow.</h2>' +
        '<p>The shell adapts around what matters most, surfaces next-best actions, and keeps the retro terminal soul intact under a much more tactile interface.</p>' +
        '<ul>' +
          '<li>Bento cards stretch with data density instead of forcing every surface into the same box.</li>' +
          '<li>Liquid Glass layers keep status, tools, and actions readable without feeling flat.</li>' +
          '<li>The omnibar speaks the product language: routes, actions, focus modes, and operator tools.</li>' +
        '</ul>' +
      '</div>' +
      '<div class="wolfbbs-spatial-shell">' +
        '<canvas class="wolfbbs-webgl-canvas" aria-label="Interactive WolfBBS preview"></canvas>' +
        '<div class="wolfbbs-spatial-stage">' +
          '<div class="wolfbbs-spatial-terminal">' +
            '<div class="wolfbbs-spatial-terminal-face screen">' +
              '<div class="wolfbbs-spatial-screen-ui">' +
                '<div class="wolfbbs-spatial-screen-bar"><span>WolfBBS 2026</span><span>Node-ready + live lanes</span></div>' +
                '<div class="wolfbbs-spatial-screen-row">' +
                  '<div class="wolfbbs-spatial-screen-stack">' +
                    '<div class="wolfbbs-spatial-screen-panel"><strong>Today Brief</strong><span>Activity, alerts, and next best moves</span></div>' +
                    '<div class="wolfbbs-spatial-screen-panel"><strong>Live Lobby</strong><span>Chat, moderation, and presence at a glance</span></div>' +
                  '</div>' +
                  '<div class="wolfbbs-spatial-screen-stack">' +
                    '<div class="wolfbbs-spatial-screen-panel"><strong>Launch</strong><span>Release gate, smoke checks, and runtime status</span></div>' +
                    '<div class="wolfbbs-spatial-screen-panel"><strong>Omnibar</strong><span>Ask for routes, actions, or focus modes</span></div>' +
                  '</div>' +
                '</div>' +
              '</div>' +
            '</div>' +
            '<div class="wolfbbs-spatial-terminal-face back"></div>' +
            '<div class="wolfbbs-spatial-terminal-face top"></div>' +
            '<div class="wolfbbs-spatial-terminal-face bottom"></div>' +
            '<div class="wolfbbs-spatial-terminal-face side-left"></div>' +
            '<div class="wolfbbs-spatial-terminal-face side-right"></div>' +
          '</div>' +
        '</div>' +
        '<div class="wolfbbs-spatial-caption"><span>Interactive browser preview</span><strong>Drag to orbit. Ctrl+K opens the omnibar.</strong></div>' +
      '</div>';
    if (main && main.firstChild) {
      main.insertBefore(section, main.firstChild);
    } else if (main) {
      main.appendChild(section);
    } else if (hero && hero.parentNode) {
      hero.parentNode.insertBefore(section, hero.nextSibling);
    }
    syncSpatialCanvas(section);
  }

  const routeLabelOverrides = {
    "admin": "Admin",
    "setup": "Setup",
    "config": "Config",
    "launch": "Launch",
    "ops": "Ops",
    "system": "System",
    "events": "Events",
    "boards": "Boards",
    "mail": "Mail",
    "chat": "Chat",
    "doors": "Doors",
    "status": "Status",
    "today": "Today",
    "digest": "Digest",
    "attention": "Attention",
    "clubhouse": "Clubhouse",
    "radar": "Radar",
    "scores": "Scores",
    "tournaments": "Tournaments",
    "connect": "Connect",
    "tour": "Tour",
    "showcase": "Showcase",
    "help": "Help",
    "gateway": "Gateway",
    "offline": "Offline",
    "collections": "Collections",
    "directory": "Directory",
    "discover": "Discover",
    "first-call": "First Call"
  };

  function humanizeRoutePart(part) {
    const clean = String(part || "").trim().toLowerCase();
    if (!clean) return "";
    if (routeLabelOverrides[clean]) return routeLabelOverrides[clean];
    return clean.replace(/[-_]+/g, " ").replace(/\b\w/g, (ch) => ch.toUpperCase());
  }

  function mountSkipAndScrollUI() {
    if (!document.getElementById("wolfbbsSkipLink")) {
      const skip = document.createElement("a");
      skip.id = "wolfbbsSkipLink";
      skip.className = "wolfbbs-skip-link";
      skip.href = "#wolfbbs-main-anchor";
      skip.textContent = "Skip to content";
      document.body.insertBefore(skip, document.body.firstChild);
    }
    if (!document.getElementById("wolfbbsScrollProgress")) {
      const progress = document.createElement("div");
      progress.id = "wolfbbsScrollProgress";
      progress.innerHTML = "<span></span>";
      document.body.appendChild(progress);
    }
    const progressFill = document.querySelector("#wolfbbsScrollProgress span");
    function updateProgress() {
      const doc = document.documentElement;
      const total = Math.max(1, doc.scrollHeight - doc.clientHeight);
      const pct = clamp((doc.scrollTop / total) * 100, 0, 100);
      if (progressFill) {
        progressFill.style.width = pct.toFixed(2) + "%";
      }
      const back = document.getElementById("wolfbbsBackToTop");
      if (back) {
        back.classList.toggle("show", doc.scrollTop > 220);
      }
    }
    if (!document.getElementById("wolfbbsBackToTop")) {
      const back = document.createElement("button");
      back.id = "wolfbbsBackToTop";
      back.type = "button";
      back.textContent = "Top";
      back.addEventListener("click", () => {
        window.scrollTo({ top: 0, behavior: "smooth" });
      });
      document.body.appendChild(back);
    }
    window.addEventListener("scroll", updateProgress, { passive: true });
    updateProgress();
  }

  function mountBreadcrumbs() {
    if (document.querySelector(".wolfbbs-breadcrumbs")) return;
    const parts = currentRoute.split("/").filter(Boolean);
    if (!parts.length) return;
    const crumb = document.createElement("nav");
    crumb.className = "wolfbbs-breadcrumbs";
    crumb.setAttribute("aria-label", "Breadcrumb");
    const home = document.createElement("a");
    home.href = "/start";
    home.textContent = "Start";
    crumb.appendChild(home);
    let href = "";
    parts.forEach((part) => {
      href += "/" + part;
      const sep = document.createElement("span");
      sep.className = "wolfbbs-breadcrumb-sep";
      sep.textContent = "/";
      crumb.appendChild(sep);
      const link = document.createElement("a");
      link.href = href;
      link.textContent = humanizeRoutePart(part);
      crumb.appendChild(link);
    });
    const hero = document.querySelector(".wolfbbs-page-hero");
    if (hero && hero.parentNode) {
      hero.parentNode.insertBefore(crumb, hero);
      return;
    }
    const navRow = document.querySelector("p.wolfbbs-nav-row");
    if (navRow && navRow.parentNode) {
      navRow.parentNode.insertBefore(crumb, navRow);
    }
  }

  function applyGlossaryEnhancer() {
    const glossary = {
      "WFC": "Waiting For Caller operator console.",
      "watch tier": "Boards that escalate into attention workflows.",
      "digest tier": "Boards surfaced in lower-noise daily summaries.",
      "launch readiness": "Go-live health and configuration confidence state.",
      "sysop": "Primary board operator role."
    };
    const nodes = Array.from(document.querySelectorAll("p, li"));
    nodes.forEach((node) => {
      if (node.dataset.wolfbbsGlossary === "1") return;
      let html = node.innerHTML;
      let touched = false;
      Object.keys(glossary).forEach((term) => {
        const re = new RegExp("\\b" + term.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\$&") + "\\b", "gi");
        if (!re.test(html)) return;
        touched = true;
        html = html.replace(re, (match) => '<abbr title="' + glossary[term] + '">' + match + '</abbr>');
      });
      if (touched) {
        node.innerHTML = html;
        node.dataset.wolfbbsGlossary = "1";
      }
    });
  }

  function mountRevisitBanner() {
    if (document.querySelector(".wolfbbs-revisit-banner")) return;
    const key = revisitKeyPrefix + (currentRoute || "/");
    const last = readJSON(key, {});
    writeJSON(key, { at: new Date().toISOString() });
    if (!last || !last.at) return;
    const previous = new Date(last.at).getTime();
    if (!Number.isFinite(previous)) return;
    const elapsedSec = Math.max(1, Math.floor((Date.now() - previous) / 1000));
    const elapsedLabel = elapsedSec < 60 ? (elapsedSec + "s ago") : elapsedSec < 3600 ? (Math.floor(elapsedSec / 60) + "m ago") : (Math.floor(elapsedSec / 3600) + "h ago");
    const bar = document.createElement("div");
    bar.className = "wolfbbs-revisit-banner";
    bar.textContent = "Last visited this route " + elapsedLabel + ".";
    const dismiss = document.createElement("button");
    dismiss.type = "button";
    dismiss.className = "wolfbbs-section-toggle";
    dismiss.textContent = "Dismiss";
    dismiss.addEventListener("click", () => bar.remove());
    bar.appendChild(dismiss);
    const hero = document.querySelector(".wolfbbs-page-hero");
    if (hero && hero.parentNode) {
      hero.parentNode.insertBefore(bar, hero.nextSibling);
    }
  }

  function mountKPIDeltas() {
    const cards = Array.from(document.querySelectorAll(".wolfbbs-kpi-card"));
    if (!cards.length) return;
    const key = kpiSnapshotKeyPrefix + (currentRoute || "/");
    const previous = readJSON(key, {});
    const next = {};
    cards.forEach((card, idx) => {
      const strong = card.querySelector("strong");
      if (!strong) return;
      const value = parseNumericValue(strong.textContent || "");
      if (value === null) return;
      const slot = "kpi_" + idx;
      next[slot] = value;
      if (!(slot in previous)) return;
      const delta = value - Number(previous[slot] || 0);
      const badge = document.createElement("span");
      badge.className = "wolfbbs-kpi-delta";
      if (delta > 0) {
        badge.classList.add("up");
        badge.textContent = "▲ " + delta.toFixed(1);
      } else if (delta < 0) {
        badge.classList.add("down");
        badge.textContent = "▼ " + Math.abs(delta).toFixed(1);
      } else {
        badge.textContent = "• 0.0";
      }
      if (!card.querySelector(".wolfbbs-kpi-delta")) {
        strong.insertAdjacentElement("afterend", badge);
      }
    });
    writeJSON(key, next);
    evaluateKPIWatchers(next);
  }

  function loadFavorites() {
    return readJSON(favoritesKey, []).filter((item) => item && item.href && item.label);
  }

  function saveFavorites(rows) {
    const clean = rows
      .map((item) => ({
        href: normalizePath(item.href),
        label: String(item.label || "").trim() || "Route"
      }))
      .filter((item) => item.href)
      .slice(0, 10);
    writeJSON(favoritesKey, clean);
    return clean;
  }

  function loadRoutePins() {
    return readJSON(routePinsKey, []).filter((item) => item && item.href && item.label);
  }

  function saveRoutePins(rows) {
    const clean = (rows || [])
      .map((item) => ({
        href: normalizePath(item.href),
        label: String(item.label || "").trim() || "Route"
      }))
      .filter((item) => item.href)
      .slice(0, 12);
    writeJSON(routePinsKey, clean);
    return clean;
  }

  function isCurrentFavorite() {
    return loadFavorites().some((item) => normalizePath(item.href) === currentRoute);
  }

  function toggleFavoriteCurrentRoute() {
    const favorites = loadFavorites();
    const index = favorites.findIndex((item) => normalizePath(item.href) === currentRoute);
    if (index >= 0) {
      favorites.splice(index, 1);
      saveFavorites(favorites);
      showToast("Removed from favorites", "ok");
      return false;
    }
    favorites.unshift({ href: currentRoute, label: title });
    saveFavorites(favorites);
    showToast("Added to favorites", "ok");
    return true;
  }

  function renderFavoritesRail() {
    const existing = document.querySelector(".wolfbbs-favorites-rail");
    if (existing) existing.remove();
    const favorites = loadFavorites();
    if (!favorites.length) return;
    const rail = document.createElement("div");
    rail.className = "wolfbbs-favorites-rail";
    const heading = document.createElement("strong");
    heading.textContent = "Favorites";
    rail.appendChild(heading);
    favorites.forEach((item) => {
      const link = document.createElement("a");
      link.href = item.href;
      link.textContent = item.label;
      rail.appendChild(link);
    });
    const navRow = document.querySelector("p.wolfbbs-nav-row");
    const recentRail = document.querySelector(".wolfbbs-recent-rail");
    const hero = document.querySelector(".wolfbbs-page-hero");
    if (recentRail && recentRail.parentNode) {
      recentRail.parentNode.insertBefore(rail, recentRail.nextSibling);
      return;
    }
    if (navRow && navRow.parentNode) {
      navRow.parentNode.insertBefore(rail, navRow.nextSibling);
      return;
    }
    if (hero && hero.parentNode) {
      hero.parentNode.insertBefore(rail, hero.nextSibling);
    }
  }

  function loadFocusState() {
    return Object.assign({
      durationSeconds: 25 * 60,
      remainingSeconds: 25 * 60,
      running: false,
      startedAt: ""
    }, readJSON(focusKey, {}));
  }

  function saveFocusState(next) {
    const state = Object.assign({
      durationSeconds: 25 * 60,
      remainingSeconds: 25 * 60,
      running: false,
      startedAt: ""
    }, next || {});
    writeJSON(focusKey, state);
    return state;
  }

  function formatDuration(totalSeconds) {
    const value = Math.max(0, Number(totalSeconds || 0));
    const mins = Math.floor(value / 60);
    const secs = value % 60;
    return String(mins).padStart(2, "0") + ":" + String(secs).padStart(2, "0");
  }

  function attachFocusTimerUI(target, controls) {
    if (!target) return;
    const pill = document.createElement("span");
    pill.className = "wolfbbs-focus-pill";
    pill.setAttribute("data-state", "idle");
    target.appendChild(pill);

    const toggle = document.createElement("button");
    toggle.type = "button";
    controls.appendChild(toggle);

    const reset = document.createElement("button");
    reset.type = "button";
    reset.textContent = "Reset focus";
    controls.appendChild(reset);

    let state = loadFocusState();
    let ticker = null;

    function sync() {
      if (state.running && state.startedAt) {
        const started = new Date(state.startedAt).getTime();
        if (Number.isFinite(started)) {
          const elapsed = Math.floor((Date.now() - started) / 1000);
          state.remainingSeconds = Math.max(0, state.durationSeconds - elapsed);
          if (state.remainingSeconds === 0) {
            state.running = false;
            state.startedAt = "";
            showToast("Focus session complete", "ok");
            trackTelemetry("focus:complete");
          }
        }
      }
      pill.textContent = "Focus " + formatDuration(state.remainingSeconds);
      pill.setAttribute("data-state", state.running ? "running" : "idle");
      toggle.textContent = state.running ? "Pause focus" : "Start focus";
      saveFocusState(state);
    }

    function startTicker() {
      if (ticker) window.clearInterval(ticker);
      ticker = window.setInterval(sync, 1000);
    }

    toggle.addEventListener("click", () => {
      if (!state.running) {
        state.running = true;
        state.startedAt = new Date(Date.now() - (state.durationSeconds - state.remainingSeconds) * 1000).toISOString();
        trackTelemetry("focus:start");
      } else {
        state.running = false;
        state.startedAt = "";
        trackTelemetry("focus:pause");
      }
      sync();
    });

    reset.addEventListener("click", () => {
      state = {
        durationSeconds: 25 * 60,
        remainingSeconds: 25 * 60,
        running: false,
        startedAt: ""
      };
      sync();
      showToast("Focus timer reset", "ok");
      trackTelemetry("focus:reset");
    });

    sync();
    startTicker();
  }

  function readFocusMode() {
    return readJSON(focusModeKey, { enabled: false }).enabled === true;
  }

  function applyFocusMode(enabled) {
    document.body.classList.toggle("wolfbbs-focus-ui", Boolean(enabled));
    writeJSON(focusModeKey, { enabled: Boolean(enabled) });
  }

  function mountGuideMinimizeControl() {
    const guide = document.querySelector(".wolfbbs-guide-strip");
    if (!guide || guide.querySelector("[data-guide-toggle]")) return;
    const saved = readJSON(guideStateKey + ":" + currentRoute, {});
    const state = {
      minimized: saved && Object.prototype.hasOwnProperty.call(saved, "minimized")
        ? Boolean(saved.minimized)
        : Boolean(currentRouteProfile.compactGuide),
    };
    const button = document.createElement("button");
    button.type = "button";
    button.setAttribute("data-guide-toggle", "1");
    button.className = "wolfbbs-section-toggle";
    function sync() {
      guide.classList.toggle("wolfbbs-guide-minimized", Boolean(state.minimized));
      button.textContent = state.minimized ? "Show guide" : "Hide guide";
      writeJSON(guideStateKey + ":" + currentRoute, state);
    }
    button.addEventListener("click", () => {
      state.minimized = !state.minimized;
      sync();
      trackTelemetry("guide:toggle");
    });
    const host = guide.querySelector("strong") ? guide.querySelector("strong").parentNode : guide;
    host.appendChild(button);
    sync();
  }

  function mountNetworkStatusChip(heroMeta) {
    if (!heroMeta || heroMeta.querySelector('[data-kind="net-online"], [data-kind="net-offline"]')) return;
    const chip = document.createElement("span");
    chip.className = "wolfbbs-hero-chip";
    function sync() {
      const online = navigator.onLine;
      chip.setAttribute("data-kind", online ? "net-online" : "net-offline");
      chip.textContent = online ? "Online" : "Offline";
    }
    sync();
    window.addEventListener("online", () => {
      sync();
      announceLive("Network online");
      trackTelemetry("network:online");
    });
    window.addEventListener("offline", () => {
      sync();
      announceLive("Network offline");
      trackTelemetry("network:offline");
    });
    heroMeta.appendChild(chip);
  }

  function mountLatencyChip(heroMeta) {
    if (!heroMeta || heroMeta.querySelector('[data-kind="latency"]')) return;
    const chip = document.createElement("span");
    chip.className = "wolfbbs-hero-chip";
    chip.setAttribute("data-kind", "latency");
    chip.textContent = "Latency --";
    heroMeta.appendChild(chip);
    async function probe() {
      const started = Date.now();
      try {
        const res = await fetch("/healthz?ts=" + encodeURIComponent(String(started)), { cache: "no-store", credentials: "same-origin" });
        if (!res.ok) throw new Error("bad response");
        const ms = Math.max(0, Date.now() - started);
        chip.textContent = "Latency " + ms + "ms";
      } catch (_) {
        chip.textContent = "Latency n/a";
      }
    }
    probe();
    window.setInterval(probe, 60000);
  }

  function mountTelemetryChip(heroMeta) {
    if (!heroMeta || heroMeta.querySelector('[data-kind="telemetry"]')) return;
    const chip = document.createElement("span");
    chip.className = "wolfbbs-hero-chip";
    chip.setAttribute("data-kind", "telemetry");
    function sync() {
      const telemetry = loadTelemetry();
      const actions = Object.values(telemetry.events || {}).reduce((acc, value) => acc + Number(value || 0), 0);
      chip.textContent = "Actions " + actions;
    }
    sync();
    window.addEventListener("storage", (event) => {
      if (event && event.key === telemetryKey) sync();
    });
    window.addEventListener("wolfbbs-telemetry-updated", sync);
    heroMeta.appendChild(chip);
  }

  function mountSessionDurationChip(heroMeta) {
    if (!heroMeta || heroMeta.querySelector('[data-kind="session-time"]')) return;
    const chip = document.createElement("span");
    chip.className = "wolfbbs-hero-chip wolfbbs-session-chip";
    chip.setAttribute("data-kind", "session-time");
    function sync() {
      const startedAt = Number(readJSON(sessionStartKey, Date.now()) || Date.now());
      const elapsed = Math.max(0, Math.floor((Date.now() - startedAt) / 1000));
      const mm = String(Math.floor(elapsed / 60)).padStart(2, "0");
      const ss = String(elapsed % 60).padStart(2, "0");
      chip.textContent = "Session " + mm + ":" + ss;
    }
    sync();
    window.setInterval(sync, 1000);
    heroMeta.appendChild(chip);
  }

  function mountSessionTrailChip(heroMeta) {
    if (!heroMeta || heroMeta.querySelector('[data-kind="session-trail"]')) return;
    const chip = document.createElement("span");
    chip.className = "wolfbbs-hero-chip wolfbbs-session-chip";
    chip.setAttribute("data-kind", "session-trail");
    function sync() {
      const trail = Object.assign({ visited: [] }, readJSON(sessionTrailKey, {}));
      const visited = Array.isArray(trail.visited) ? trail.visited : [];
      chip.textContent = "Hops " + visited.length;
      chip.title = visited.join(" -> ");
    }
    sync();
    window.addEventListener("storage", (event) => {
      if (event && event.key === sessionTrailKey) sync();
    });
    window.addEventListener("wolfbbs-storage-updated", (event) => {
      if (event && event.detail && event.detail.key === sessionTrailKey) sync();
    });
    heroMeta.appendChild(chip);
  }

  function mountShortcutLegendOverlay() {
    if (document.getElementById("wolfbbsShortcutOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsShortcutOverlay";
    overlay.innerHTML = '<div id="wolfbbsShortcutPanel"><h3>Shortcut Legend</h3><ul><li><strong>Ctrl/Cmd+K</strong>: command palette</li><li><strong>?</strong>: palette quick open</li><li><strong>Alt+1..6</strong>: route jump macros</li><li><strong>Alt+0</strong>: macro help</li><li><strong>Alt+J / Alt+K</strong>: next/previous section</li><li><strong>Ctrl/Cmd+Shift+N</strong>: quick notes workspace</li><li><strong>Ctrl/Cmd+Shift+D</strong>: draft center</li><li><strong>Ctrl/Cmd+Shift+I</strong>: incident console</li><li><strong>Ctrl/Cmd+Shift+P</strong>: playbook runner</li><li><strong>Ctrl/Cmd+Shift+M</strong>: reminder scheduler</li><li><strong>Ctrl/Cmd+Shift+G</strong>: release gate</li><li><strong>Ctrl/Cmd+Shift+U</strong>: bug capture</li><li><strong>F1</strong>: macro help panel</li><li><strong>Ctrl/Cmd+Shift+/</strong>: this legend</li></ul><p class="wolfbbs-help-copy">Esc closes overlays.</p></div>';
    document.body.appendChild(overlay);
    function open() {
      overlay.classList.add("active");
      trackTelemetry("shortcut-legend:open");
    }
    function close() {
      overlay.classList.remove("active");
    }
    window.wolfbbsOpenShortcutLegend = open;
    window.wolfbbsCloseShortcutLegend = close;
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) close();
    });
    document.addEventListener("keydown", (event) => {
      if ((event.metaKey || event.ctrlKey) && event.shiftKey && event.key === "?") {
        event.preventDefault();
        open();
        return;
      }
      if ((event.metaKey || event.ctrlKey) && event.shiftKey && event.key === "/") {
        event.preventDefault();
        open();
        return;
      }
      if (event.key === "Escape" && overlay.classList.contains("active")) {
        event.preventDefault();
        close();
      }
    });
  }

  function mountPreferenceControls() {
    const heroMeta = document.querySelector(".wolfbbs-page-hero-meta");
    if (!heroMeta || heroMeta.querySelector(".wolfbbs-pref-controls")) return;
    applyFocusMode(readFocusMode());
    const controls = document.createElement("div");
    controls.className = "wolfbbs-pref-controls";
    const menuDetails = [];
    function closeMenus(except) {
      menuDetails.forEach((menu) => {
        if (menu !== except) menu.open = false;
      });
    }
    function buildMenu(label, description) {
      const details = document.createElement("details");
      details.className = "wolfbbs-pref-menu";
      const summary = document.createElement("summary");
      summary.textContent = label;
      summary.setAttribute("aria-label", label + " menu");
      const panel = document.createElement("div");
      panel.className = "wolfbbs-pref-menu-panel";
      if (description) {
        const note = document.createElement("p");
        note.className = "wolfbbs-pref-menu-note wolfbbs-help-copy";
        note.textContent = description;
        panel.appendChild(note);
      }
      details.appendChild(summary);
      details.appendChild(panel);
      details.addEventListener("toggle", () => {
        if (details.open) closeMenus(details);
      });
      controls.appendChild(details);
      menuDetails.push(details);
      return panel;
    }
    function mountMenuButton(panel, button) {
      if (!panel || !button) return button;
      button.setAttribute("data-control-priority", "secondary");
      panel.appendChild(button);
      return button;
    }
    function mountMenuGroup(panel, title) {
      const group = document.createElement("section");
      group.className = "wolfbbs-pref-menu-group";
      if (title) {
        const heading = document.createElement("strong");
        heading.className = "wolfbbs-pref-menu-group-title";
        heading.textContent = title;
        group.appendChild(heading);
      }
      panel.appendChild(group);
      return group;
    }
    const morePanel = buildMenu("More", "Display, session, and route tools live here when you need them.");
    const helpPanel = buildMenu("Help", "Guides, shortcuts, and reporting.");
    const statusPanel = mountMenuGroup(morePanel, "Session");
    const statusGrid = document.createElement("div");
    statusGrid.className = "wolfbbs-status-grid";
    statusPanel.appendChild(statusGrid);
    mountNetworkStatusChip(statusGrid);
    mountLatencyChip(statusGrid);
    mountTelemetryChip(statusGrid);
    mountSessionDurationChip(statusGrid);
    mountSessionTrailChip(statusGrid);
    const viewPanel = mountMenuGroup(morePanel, "Display");
    const routePanel = mountMenuGroup(morePanel, "This page");
    const toolsPanel = mountMenuGroup(morePanel, "Utilities");

    const profileKey = "wolfbbs:ui:profile:v1";
    let profileMode = String(readJSON(profileKey, "balanced") || "balanced");
    if (!["balanced", "reader", "operator"].includes(profileMode)) {
      profileMode = "balanced";
    }

    const theme = document.createElement("button");
    theme.type = "button";
    const themeModes = ["default", "contrast", "night"];
    function syncThemeLabel() {
      theme.textContent = "Theme: " + (uiPrefs.theme === "default" ? "Soft" : uiPrefs.theme === "contrast" ? "Contrast" : "Night");
    }
    theme.addEventListener("click", () => {
      const current = themeModes.indexOf(uiPrefs.theme);
      const prev = uiPrefs.theme;
      uiPrefs.theme = themeModes[(current + 1 + themeModes.length) % themeModes.length];
      persistUIPrefs();
      applyUIPrefs();
      syncThemeLabel();
      showToast("Theme updated", "ok");
      pushUndoAction({ type: "ui-theme", value: prev });
    });
    syncThemeLabel();
    mountMenuButton(viewPanel, theme);

    const density = document.createElement("button");
    density.type = "button";
    function syncDensityLabel() {
      density.textContent = "Density: " + (uiPrefs.density === "compact" ? "Compact" : "Comfort");
    }
    density.addEventListener("click", () => {
      const prev = uiPrefs.density;
      uiPrefs.density = uiPrefs.density === "compact" ? "comfortable" : "compact";
      persistUIPrefs();
      applyUIPrefs();
      syncDensityLabel();
      showToast("Density updated", "ok");
      pushUndoAction({ type: "ui-density", value: prev });
    });
    syncDensityLabel();
    mountMenuButton(viewPanel, density);

    const layout = document.createElement("button");
    layout.type = "button";
    const layoutModes = ["standard", "wide", "focus"];
    function syncLayoutLabel() {
      const label = uiPrefs.layout === "wide" ? "Wide" : uiPrefs.layout === "focus" ? "Focus" : "Standard";
      layout.textContent = "Layout: " + label;
    }
    layout.addEventListener("click", () => {
      const prev = uiPrefs.layout;
      const current = layoutModes.indexOf(uiPrefs.layout);
      uiPrefs.layout = layoutModes[(current + 1 + layoutModes.length) % layoutModes.length];
      persistUIPrefs();
      applyUIPrefs();
      syncLayoutLabel();
      showToast("Layout updated", "ok");
      pushUndoAction({ type: "ui-layout", value: prev });
    });
    syncLayoutLabel();
    mountMenuButton(viewPanel, layout);

    const accent = document.createElement("button");
    accent.type = "button";
    const accentModes = ["blue", "teal", "amber"];
    function syncAccentLabel() {
      const label = uiPrefs.accent === "teal" ? "Teal" : uiPrefs.accent === "amber" ? "Amber" : "Blue";
      accent.textContent = "Accent: " + label;
    }
    accent.addEventListener("click", () => {
      const prev = uiPrefs.accent;
      const current = accentModes.indexOf(uiPrefs.accent);
      uiPrefs.accent = accentModes[(current + 1 + accentModes.length) % accentModes.length];
      persistUIPrefs();
      applyUIPrefs();
      syncAccentLabel();
      showToast("Accent updated", "ok");
      pushUndoAction({ type: "ui-accent", value: prev });
    });
    syncAccentLabel();
    mountMenuButton(viewPanel, accent);

    const motion = document.createElement("button");
    motion.type = "button";
    function syncMotionLabel() {
      motion.textContent = uiPrefs.motion === "reduced" ? "Motion: Reduced" : "Motion: Full";
    }
    motion.addEventListener("click", () => {
      const prev = uiPrefs.motion;
      uiPrefs.motion = uiPrefs.motion === "reduced" ? "full" : "reduced";
      persistUIPrefs();
      applyUIPrefs();
      syncMotionLabel();
      showToast("Motion preference updated", "ok");
      pushUndoAction({ type: "ui-motion", value: prev });
    });
    syncMotionLabel();
    mountMenuButton(viewPanel, motion);

    const profile = document.createElement("button");
    profile.type = "button";
    function syncProfileLabel() {
      const label = profileMode === "reader" ? "Reader" : profileMode === "operator" ? "Operator" : "Balanced";
      profile.textContent = "Profile: " + label;
    }
    function applyProfile(targetMode) {
      const mode = ["balanced", "reader", "operator"].includes(targetMode) ? targetMode : "balanced";
      profileMode = mode;
      if (mode === "reader") {
        uiPrefs.layout = "wide";
        uiPrefs.motion = "reduced";
        uiPrefs.density = "comfortable";
        uiPrefs.accent = "blue";
      } else if (mode === "operator") {
        uiPrefs.layout = "standard";
        uiPrefs.motion = "full";
        uiPrefs.density = "compact";
        uiPrefs.accent = "teal";
      } else {
        uiPrefs.layout = "standard";
        uiPrefs.motion = "full";
        uiPrefs.density = "comfortable";
        uiPrefs.accent = "blue";
      }
      writeJSON(profileKey, profileMode);
      persistUIPrefs();
      applyUIPrefs();
      syncLayoutLabel();
      syncDensityLabel();
      syncAccentLabel();
      syncMotionLabel();
      syncProfileLabel();
    }
    profile.addEventListener("click", () => {
      const before = {
        profile: profileMode,
        prefs: {
          theme: uiPrefs.theme,
          density: uiPrefs.density,
          fontScale: uiPrefs.fontScale,
          layout: uiPrefs.layout,
          accent: uiPrefs.accent,
          motion: uiPrefs.motion
        }
      };
      if (profileMode === "balanced") {
        applyProfile("reader");
      } else if (profileMode === "reader") {
        applyProfile("operator");
      } else {
        applyProfile("balanced");
      }
      showToast("UI profile updated", "ok");
      pushUndoAction({ type: "ui-profile", value: before });
    });
    if (profileMode === "reader" || profileMode === "operator") {
      applyProfile(profileMode);
    } else {
      syncProfileLabel();
    }
    mountMenuButton(viewPanel, profile);

    const smaller = document.createElement("button");
    smaller.type = "button";
    smaller.textContent = "A-";
    smaller.title = "Smaller text";
    smaller.addEventListener("click", () => {
      const prev = uiPrefs.fontScale;
      uiPrefs.fontScale = clamp(Number(uiPrefs.fontScale || 1) - 0.05, 0.9, 1.2);
      persistUIPrefs();
      applyUIPrefs();
      showToast("Text size " + Math.round(uiPrefs.fontScale * 100) + "%", "ok");
      pushUndoAction({ type: "ui-font-scale", value: prev });
    });
    mountMenuButton(viewPanel, smaller);

    const larger = document.createElement("button");
    larger.type = "button";
    larger.textContent = "A+";
    larger.title = "Larger text";
    larger.addEventListener("click", () => {
      const prev = uiPrefs.fontScale;
      uiPrefs.fontScale = clamp(Number(uiPrefs.fontScale || 1) + 0.05, 0.9, 1.2);
      persistUIPrefs();
      applyUIPrefs();
      showToast("Text size " + Math.round(uiPrefs.fontScale * 100) + "%", "ok");
      pushUndoAction({ type: "ui-font-scale", value: prev });
    });
    mountMenuButton(viewPanel, larger);

    const favorite = document.createElement("button");
    favorite.type = "button";
    function syncFavoriteLabel() {
      favorite.textContent = isCurrentFavorite() ? "Unfavorite" : "Favorite";
    }
    favorite.addEventListener("click", () => {
      const prev = isCurrentFavorite();
      toggleFavoriteCurrentRoute();
      syncFavoriteLabel();
      renderFavoritesRail();
      pushUndoAction({ type: "ui-favorite-current", value: prev });
    });
    syncFavoriteLabel();
    mountMenuButton(routePanel, favorite);

    const pinRoute = document.createElement("button");
    pinRoute.type = "button";
    function syncPinRouteLabel() {
      const pinned = loadRoutePins().some((item) => normalizePath(item.href) === currentRoute);
      pinRoute.textContent = pinned ? "Unpin route" : "Pin route";
    }
    pinRoute.addEventListener("click", () => {
      const pins = loadRoutePins();
      const idx = pins.findIndex((item) => normalizePath(item.href) === currentRoute);
      const prev = idx >= 0;
      if (idx >= 0) {
        pins.splice(idx, 1);
      } else {
        pins.unshift({ href: currentRoute, label: title });
      }
      saveRoutePins(pins);
      syncPinRouteLabel();
      if (typeof window.wolfbbsRenderActionDock === "function") window.wolfbbsRenderActionDock();
      showToast(idx >= 0 ? "Route unpinned" : "Route pinned", "ok");
      pushUndoAction({ type: "ui-pin-route", value: prev });
    });
    syncPinRouteLabel();
    mountMenuButton(routePanel, pinRoute);

    const copyRoute = document.createElement("button");
    copyRoute.type = "button";
    copyRoute.textContent = "Copy link";
    copyRoute.addEventListener("click", () => {
      const href = location.origin + location.pathname + location.search + location.hash;
      copyText(href).then(() => {
        showToast("Route URL copied", "ok");
      }).catch(() => {
        showToast("Could not copy route URL", "error");
      });
      trackTelemetry("route:copy");
    });
    mountMenuButton(routePanel, copyRoute);

    const trailBack = document.createElement("button");
    trailBack.type = "button";
    function previousTrailRoute() {
      const trail = Object.assign({ visited: [] }, readJSON(sessionTrailKey, {}));
      const rows = Array.isArray(trail.visited) ? trail.visited : [];
      for (let i = 0; i < rows.length; i++) {
        const candidate = normalizePath(rows[i]);
        if (candidate && candidate !== currentRoute) {
          return candidate;
        }
      }
      return "";
    }
    function syncTrailBackLabel() {
      const prevRoute = previousTrailRoute();
      trailBack.textContent = "Back";
      if (!prevRoute) {
        trailBack.disabled = true;
        trailBack.title = "No previous route yet";
        return;
      }
      trailBack.disabled = false;
      trailBack.title = "Back to " + humanizeRoutePart(prevRoute.split("/").filter(Boolean).slice(-1)[0] || "start");
    }
    trailBack.addEventListener("click", () => {
      const prevRoute = previousTrailRoute();
      if (!prevRoute) return;
      location.assign(prevRoute);
    });
    syncTrailBackLabel();
    controls.insertBefore(trailBack, controls.firstChild);

    const shortcutsButton = document.createElement("button");
    shortcutsButton.type = "button";
    shortcutsButton.textContent = "Shortcuts";
    shortcutsButton.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenShortcutLegend === "function") {
        window.wolfbbsOpenShortcutLegend();
      }
    });
    mountMenuButton(helpPanel, shortcutsButton);

    const contextHelp = document.createElement("button");
    contextHelp.type = "button";
    contextHelp.textContent = "Page help";
    contextHelp.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenContextHelp === "function") {
        window.wolfbbsOpenContextHelp();
      }
    });
    mountMenuButton(helpPanel, contextHelp);

    const reset = document.createElement("button");
    reset.type = "button";
    reset.textContent = "Reset UI";
    reset.addEventListener("click", () => {
      const prev = {
        theme: uiPrefs.theme,
        density: uiPrefs.density,
        fontScale: uiPrefs.fontScale,
        layout: uiPrefs.layout,
        accent: uiPrefs.accent,
        motion: uiPrefs.motion
      };
      uiPrefs.theme = "default";
      uiPrefs.density = "comfortable";
      uiPrefs.fontScale = 1;
      uiPrefs.layout = "standard";
      uiPrefs.accent = "blue";
      uiPrefs.motion = "full";
      persistUIPrefs();
      applyUIPrefs();
      syncThemeLabel();
      syncDensityLabel();
      syncLayoutLabel();
      syncAccentLabel();
      syncMotionLabel();
      showToast("UI preferences reset", "ok");
      pushUndoAction({ type: "ui-pref-reset", value: prev });
    });
    mountMenuButton(viewPanel, reset);

    const focusUI = document.createElement("button");
    focusUI.type = "button";
    function syncFocusUILabel() {
      focusUI.textContent = document.body.classList.contains("wolfbbs-focus-ui") ? "Focus UI: On" : "Focus UI: Off";
    }
    focusUI.addEventListener("click", () => {
      const prev = document.body.classList.contains("wolfbbs-focus-ui");
      const next = !document.body.classList.contains("wolfbbs-focus-ui");
      applyFocusMode(next);
      syncFocusUILabel();
      trackTelemetry("focus-ui:toggle");
      pushUndoAction({ type: "ui-focus-mode", value: prev });
    });
    syncFocusUILabel();
    mountMenuButton(viewPanel, focusUI);

    const compactControlsKey = "wolfbbs:ui:compact-controls:v1";
    const compactControls = document.createElement("button");
    compactControls.type = "button";
    let compactEnabled = Boolean(readJSON(compactControlsKey, false));
    function syncCompactLabel() {
      compactControls.textContent = compactEnabled ? "Controls: Compact" : "Controls: Full";
    }
    function applyCompactControls() {
      document.body.classList.toggle("wolfbbs-controls-compact", compactEnabled);
    }
    compactControls.addEventListener("click", () => {
      compactEnabled = !compactEnabled;
      writeJSON(compactControlsKey, compactEnabled);
      applyCompactControls();
      syncCompactLabel();
      trackTelemetry("controls:compact-toggle");
    });
    applyCompactControls();
    syncCompactLabel();
    mountMenuButton(viewPanel, compactControls);

    const autoRefreshKey = "wolfbbs:ui:auto-refresh:v1";
    const autoRefreshButton = document.createElement("button");
    autoRefreshButton.type = "button";
    const autoRefreshState = Object.assign({ enabled: false, seconds: 30 }, readJSON(autoRefreshKey, {}));
    let autoRefreshTimer = null;
    function syncAutoRefreshLabel() {
      autoRefreshButton.textContent = autoRefreshState.enabled ? ("Auto " + autoRefreshState.seconds + "s") : "Auto refresh";
    }
    function armAutoRefresh() {
      if (autoRefreshTimer) window.clearInterval(autoRefreshTimer);
      if (!autoRefreshState.enabled) return;
      autoRefreshTimer = window.setInterval(() => {
        if (document.hidden) return;
        location.reload();
      }, Math.max(15, Number(autoRefreshState.seconds || 30)) * 1000);
    }
    autoRefreshButton.addEventListener("click", () => {
      autoRefreshState.enabled = !autoRefreshState.enabled;
      writeJSON(autoRefreshKey, autoRefreshState);
      syncAutoRefreshLabel();
      if (autoRefreshState.enabled) armAutoRefresh();
      trackTelemetry("auto-refresh:toggle");
    });
    syncAutoRefreshLabel();
    if (autoRefreshState.enabled) armAutoRefresh();
    mountMenuButton(toolsPanel, autoRefreshButton);

    const exportTelemetry = document.createElement("button");
    exportTelemetry.type = "button";
    exportTelemetry.textContent = "Export UX";
    exportTelemetry.addEventListener("click", () => {
      downloadJSONFile("wolfbbs-ux-telemetry.json", loadTelemetry());
      showToast("UX telemetry exported", "ok");
      trackTelemetry("telemetry:export");
    });
    mountMenuButton(toolsPanel, exportTelemetry);

    const exportDiag = document.createElement("button");
    exportDiag.type = "button";
    exportDiag.textContent = "Export diag";
    exportDiag.addEventListener("click", () => {
      downloadJSONFile("wolfbbs-ui-diagnostics.json", {
        route: currentRoute,
        generatedAt: new Date().toISOString(),
        prefs: readJSON(uiPrefsKey, {}),
        focusMode: readJSON(focusModeKey, {}),
        focusTimer: readJSON(focusKey, {}),
        favorites: readJSON(favoritesKey, []),
        notes: String(localStorage.getItem(notesKey) || ""),
        telemetry: loadTelemetry()
      });
      showToast("UI diagnostics exported", "ok");
      trackTelemetry("diagnostics:export");
    });
    mountMenuButton(toolsPanel, exportDiag);

    const toastCenter = document.createElement("button");
    toastCenter.type = "button";
    toastCenter.textContent = "Notifications";
    toastCenter.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenToastCenter === "function") {
        window.wolfbbsOpenToastCenter();
      }
    });
    mountMenuButton(toolsPanel, toastCenter);

    const quickNotes = document.createElement("button");
    quickNotes.type = "button";
    quickNotes.textContent = "Quick notes";
    quickNotes.addEventListener("click", () => {
      const trigger = document.getElementById("wolfbbsNotesButton");
      if (trigger) {
        trigger.click();
        return;
      }
      showToast("Quick notes unavailable", "error");
    });
    mountMenuButton(toolsPanel, quickNotes);

    const workspace = document.createElement("button");
    workspace.type = "button";
    workspace.textContent = "Workspaces";
    workspace.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenWorkspaceHub === "function") {
        window.wolfbbsOpenWorkspaceHub();
      }
    });
    mountMenuButton(toolsPanel, workspace);

    const checkpoints = document.createElement("button");
    checkpoints.type = "button";
    checkpoints.textContent = "Checkpoints";
    checkpoints.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenCheckpointHub === "function") {
        window.wolfbbsOpenCheckpointHub();
      }
    });
    mountMenuButton(toolsPanel, checkpoints);

    const spotlight = document.createElement("button");
    spotlight.type = "button";
    spotlight.textContent = "Spotlight";
    spotlight.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenSpotlight === "function") {
        window.wolfbbsOpenSpotlight();
      }
    });
    mountMenuButton(toolsPanel, spotlight);

    const drafts = document.createElement("button");
    drafts.type = "button";
    drafts.textContent = "Drafts";
    drafts.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenDraftCenter === "function") {
        window.wolfbbsOpenDraftCenter();
      }
    });
    mountMenuButton(toolsPanel, drafts);

    const kpiWatch = document.createElement("button");
    kpiWatch.type = "button";
    kpiWatch.textContent = "KPI watch";
    kpiWatch.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenKPIWatchCenter === "function") {
        window.wolfbbsOpenKPIWatchCenter();
      }
    });
    mountMenuButton(toolsPanel, kpiWatch);

    const incidents = document.createElement("button");
    incidents.type = "button";
    incidents.textContent = "Incidents";
    incidents.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenIncidentConsole === "function") {
        window.wolfbbsOpenIncidentConsole();
      }
    });
    mountMenuButton(toolsPanel, incidents);

    const playbook = document.createElement("button");
    playbook.type = "button";
    playbook.textContent = "Playbooks";
    playbook.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenPlaybookRunner === "function") {
        window.wolfbbsOpenPlaybookRunner();
      }
    });
    mountMenuButton(toolsPanel, playbook);

    const reminders = document.createElement("button");
    reminders.type = "button";
    reminders.textContent = "Reminders";
    reminders.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenReminderScheduler === "function") {
        window.wolfbbsOpenReminderScheduler();
      }
    });
    mountMenuButton(toolsPanel, reminders);

    const releaseGate = document.createElement("button");
    releaseGate.type = "button";
    releaseGate.textContent = "Release gate";
    releaseGate.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenReleaseGate === "function") {
        window.wolfbbsOpenReleaseGate();
      }
    });
    mountMenuButton(toolsPanel, releaseGate);

    const feedback = document.createElement("button");
    feedback.type = "button";
    feedback.textContent = "Feedback";
    feedback.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenFeedbackPulse === "function") {
        window.wolfbbsOpenFeedbackPulse();
      }
    });
    mountMenuButton(helpPanel, feedback);

    const bugReport = document.createElement("button");
    bugReport.type = "button";
    bugReport.textContent = "Bug report";
    bugReport.addEventListener("click", () => {
      if (typeof window.wolfbbsOpenBugCapture === "function") {
        window.wolfbbsOpenBugCapture();
      }
    });
    mountMenuButton(helpPanel, bugReport);

    const undoButton = document.createElement("button");
    undoButton.type = "button";
    undoButton.textContent = "Undo UI";
    undoButton.addEventListener("click", () => {
      const action = popUndoAction();
      if (!action) {
        showToast("Nothing to undo", "error");
        return;
      }
      switch (action.type) {
      case "ui-theme":
        uiPrefs.theme = action.value;
        persistUIPrefs();
        applyUIPrefs();
        syncThemeLabel();
        break;
      case "ui-density":
        uiPrefs.density = action.value;
        persistUIPrefs();
        applyUIPrefs();
        syncDensityLabel();
        break;
      case "ui-layout":
        uiPrefs.layout = action.value || "standard";
        persistUIPrefs();
        applyUIPrefs();
        syncLayoutLabel();
        break;
      case "ui-accent":
        uiPrefs.accent = action.value || "blue";
        persistUIPrefs();
        applyUIPrefs();
        syncAccentLabel();
        break;
      case "ui-motion":
        uiPrefs.motion = action.value === "reduced" ? "reduced" : "full";
        persistUIPrefs();
        applyUIPrefs();
        syncMotionLabel();
        break;
      case "ui-profile":
        if (action.value && action.value.prefs) {
          const previousPrefs = action.value.prefs;
          uiPrefs.theme = previousPrefs.theme || "default";
          uiPrefs.density = previousPrefs.density || "comfortable";
          uiPrefs.fontScale = Number(previousPrefs.fontScale || 1);
          uiPrefs.layout = previousPrefs.layout || "standard";
          uiPrefs.accent = previousPrefs.accent || "blue";
          uiPrefs.motion = previousPrefs.motion === "reduced" ? "reduced" : "full";
          profileMode = ["balanced", "reader", "operator"].includes(action.value.profile) ? action.value.profile : "balanced";
          writeJSON(profileKey, profileMode);
          persistUIPrefs();
          applyUIPrefs();
          syncThemeLabel();
          syncDensityLabel();
          syncLayoutLabel();
          syncAccentLabel();
          syncMotionLabel();
          syncProfileLabel();
        }
        break;
      case "ui-font-scale":
        uiPrefs.fontScale = action.value;
        persistUIPrefs();
        applyUIPrefs();
        break;
      case "ui-focus-mode":
        applyFocusMode(Boolean(action.value));
        syncFocusUILabel();
        break;
      case "ui-favorite-current":
        const hasCurrent = isCurrentFavorite();
        if (Boolean(action.value) !== hasCurrent) {
          toggleFavoriteCurrentRoute();
          syncFavoriteLabel();
          renderFavoritesRail();
        }
        break;
      case "ui-pin-route":
        const pins = loadRoutePins();
        const pinIdx = pins.findIndex((item) => normalizePath(item.href) === currentRoute);
        if (Boolean(action.value) && pinIdx < 0) {
          pins.unshift({ href: currentRoute, label: title });
          saveRoutePins(pins);
        }
        if (!Boolean(action.value) && pinIdx >= 0) {
          pins.splice(pinIdx, 1);
          saveRoutePins(pins);
        }
        syncPinRouteLabel();
        if (typeof window.wolfbbsRenderActionDock === "function") window.wolfbbsRenderActionDock();
        break;
      case "ui-pref-reset":
        if (action.value && typeof action.value === "object") {
          uiPrefs.theme = action.value.theme || "default";
          uiPrefs.density = action.value.density || "comfortable";
          uiPrefs.fontScale = Number(action.value.fontScale || 1);
          uiPrefs.layout = action.value.layout || "standard";
          uiPrefs.accent = action.value.accent || "blue";
          uiPrefs.motion = action.value.motion === "reduced" ? "reduced" : "full";
          persistUIPrefs();
          applyUIPrefs();
          syncThemeLabel();
          syncDensityLabel();
          syncLayoutLabel();
          syncAccentLabel();
          syncMotionLabel();
        }
        break;
      default:
        showToast("Undo item unsupported", "error");
        return;
      }
      showToast("UI action undone", "ok");
      trackTelemetry("ui:undo");
    });
    mountMenuButton(viewPanel, undoButton);

    document.addEventListener("click", (event) => {
      if (!controls.contains(event.target)) closeMenus(null);
    });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape") closeMenus(null);
    });

    heroMeta.appendChild(controls);
    attachFocusTimerUI(statusGrid, statusPanel);
  }

  function mountSectionToggles() {
    const sections = Array.from(document.querySelectorAll("main.wolfbbs-main > section, main.wolfbbs-main > article"));
    function shouldAutoCollapse(section, index) {
      if (!currentRouteProfile.autoCollapseSections) return false;
      if (index === 0) return false;
      if (section.querySelector("table")) return true;
      if (section.querySelector("form")) return index >= 1;
      return index >= 2;
    }
    sections.forEach((section, index) => {
      if (section.classList.contains("wolfbbs-section-collapsible")) return;
      const heading = section.querySelector(":scope > h2");
      if (!heading) return;
      section.classList.add("wolfbbs-section-collapsible");
      if (!heading.dataset.navLabel) {
        heading.dataset.navLabel = headingBaseLabel(heading);
      }
      const body = document.createElement("div");
      body.className = "wolfbbs-section-body";
      while (heading.nextSibling) {
        body.appendChild(heading.nextSibling);
      }
      section.appendChild(body);
      const key = sectionStatePrefix + currentRoute + ":" + (heading.id || index);
      const button = document.createElement("button");
      button.type = "button";
      button.className = "wolfbbs-section-toggle";
      heading.appendChild(button);
      function setState(collapsed) {
        section.classList.toggle("is-collapsed", collapsed);
        button.textContent = collapsed ? "Expand" : "Collapse";
        writeJSON(key, { collapsed: Boolean(collapsed) });
      }
      const saved = readJSON(key, {});
      const hasSavedState = saved && Object.prototype.hasOwnProperty.call(saved, "collapsed");
      setState(hasSavedState ? Boolean(saved.collapsed) : shouldAutoCollapse(section, index));
      button.addEventListener("click", () => {
        setState(!section.classList.contains("is-collapsed"));
      });
    });
  }

  const routeActionRegistry = {
    "/start": [{ label: "Connect", href: "/connect" }, { label: "Help", href: "/help" }],
    "/today": [{ label: "Attention", href: "/attention" }, { label: "Boards", href: "/boards?mode=watched" }],
    "/attention": [{ label: "Mail", href: "/mail?box=unread" }, { label: "Boards", href: "/boards?mode=mentions" }],
    "/boards": [{ label: "Create message", href: "/boards?compose=1" }, { label: "Open today", href: "/today" }],
    "/chat": [{ label: "Open #lobby", href: "/chat?channel=%23lobby" }, { label: "Clubhouse", href: "/clubhouse" }],
    "/mail": [{ label: "Compose", href: "/mail?compose=1" }, { label: "Directory", href: "/directory" }],
    "/admin/setup": [{ label: "Seed boards", href: "/admin/setup" }, { label: "Create users", href: "/admin/users" }],
    "/admin/launch": [{ label: "Ops center", href: "/admin/ops" }, { label: "Status", href: "/status" }],
    "/admin/ops": [{ label: "System", href: "/admin/system" }, { label: "Audit", href: "/admin/audit" }]
  };

  function actionsForRoute(pathname) {
    const path = normalizePath(pathname);
    if (!path) return [];
    if (routeActionRegistry[path]) return routeActionRegistry[path];
    const prefix = Object.keys(routeActionRegistry).find((key) => path.indexOf(key + "/") === 0);
    return prefix ? routeActionRegistry[prefix] : [{ label: "Start", href: "/start" }, { label: "Help", href: "/help" }];
  }

  function mountActionDock() {
    if (document.getElementById("wolfbbsActionDock")) return;
    const persistedState = Object.assign({ collapsed: true, side: "right" }, readJSON(actionDockStateKey, {}));
    const state = {
      collapsed: Boolean(persistedState.collapsed),
      side: persistedState.side === "left" ? "left" : "right"
    };
    const dock = document.createElement("aside");
    dock.id = "wolfbbsActionDock";
    const head = document.createElement("div");
    head.className = "wolfbbs-action-dock-head";
    const titleNode = document.createElement("strong");
    titleNode.textContent = "Quick Launch Dock";
    const headActions = document.createElement("div");
    headActions.className = "wolfbbs-inline-actions";
    const refreshButton = document.createElement("button");
    refreshButton.type = "button";
    refreshButton.textContent = "Refresh";
    const sideButton = document.createElement("button");
    sideButton.type = "button";
    const collapseButton = document.createElement("button");
    collapseButton.type = "button";
    headActions.appendChild(refreshButton);
    headActions.appendChild(sideButton);
    headActions.appendChild(collapseButton);
    head.appendChild(titleNode);
    head.appendChild(headActions);
    const body = document.createElement("div");
    body.className = "wolfbbs-action-dock-body";
    const searchWrap = document.createElement("label");
    searchWrap.className = "wolfbbs-action-dock-search";
    const searchInput = document.createElement("input");
    searchInput.type = "search";
    searchInput.placeholder = "Filter route actions";
    searchInput.setAttribute("aria-label", "Filter action dock links");
    searchWrap.appendChild(searchInput);
    dock.appendChild(head);
    dock.appendChild(body);
    document.body.appendChild(dock);
    let lastCollisionNoticeAt = 0;
    let scrollAdjustTimer = null;
    let manualExpandUntil = 0;

    function persistDockState() {
      writeJSON(actionDockStateKey, {
        collapsed: Boolean(state.collapsed),
        side: state.side === "left" ? "left" : "right"
      });
    }

    function rectsOverlap(a, b) {
      if (!a || !b) return false;
      return !(a.right <= b.left || a.left >= b.right || a.bottom <= b.top || a.top >= b.bottom);
    }

    function dockIntersectsCriticalControls() {
      if (state.collapsed) return false;
      const dockRect = dock.getBoundingClientRect();
      if (!dockRect || dockRect.width <= 0 || dockRect.height <= 0) return false;
      const criticalNodes = [];
      const prefControls = document.querySelector(".wolfbbs-pref-controls");
      const navRow = document.querySelector("p.wolfbbs-nav-row");
      const sectionNav = document.querySelector(".wolfbbs-section-nav");
      if (prefControls) criticalNodes.push(prefControls);
      if (navRow) criticalNodes.push(navRow);
      if (sectionNav) criticalNodes.push(sectionNav);
      return criticalNodes.some((node) => {
        const rect = node.getBoundingClientRect();
        return rect && rect.width > 0 && rect.height > 0 && rectsOverlap(dockRect, rect);
      });
    }

    function maybeAutoAdjustDock() {
      if (!dockIntersectsCriticalControls()) return;
      if (state.side !== "left") {
        state.side = "left";
      } else if (!state.collapsed && Date.now() >= manualExpandUntil) {
        state.collapsed = true;
      } else {
        return;
      }
      persistDockState();
      renderActionDock();
      const now = Date.now();
      if (now - lastCollisionNoticeAt > 3000) {
        showToast("Action dock adjusted to keep controls clickable", "ok");
        lastCollisionNoticeAt = now;
      }
      trackTelemetry("action-dock:collision-avoid");
    }

    function setCollapsed(collapsed, reason) {
      const next = Boolean(collapsed);
      if (state.collapsed === next) return;
      if (!next) {
        // Keep the dock expanded briefly after explicit user action or
        // route-pin auto-expand to avoid immediate auto-collapse.
        manualExpandUntil = Date.now() + 7000;
      } else if (next) {
        manualExpandUntil = 0;
      }
      state.collapsed = next;
      persistDockState();
      renderActionDock();
      if (reason) {
        trackTelemetry("action-dock:toggle");
      }
    }

    function renderGroup(label, rows, queryText) {
      const group = document.createElement("section");
      group.className = "wolfbbs-action-dock-group";
      const heading = document.createElement("strong");
      heading.className = "wolfbbs-action-dock-group-title";
      heading.textContent = label;
      const count = document.createElement("span");
      const sourceRows = Array.isArray(rows) ? rows.slice(0, 16) : [];
      const query = String(queryText || "").trim().toLowerCase();
      const filteredRows = query
        ? sourceRows.filter((item) => {
            const href = String(item && item.href || "");
            const itemLabel = String(item && item.label || "");
            return href.toLowerCase().includes(query) || itemLabel.toLowerCase().includes(query);
          })
        : sourceRows;
      count.textContent = String(filteredRows.length) + "/" + String(sourceRows.length);
      heading.appendChild(count);
      group.appendChild(heading);
      const links = document.createElement("div");
      links.className = "wolfbbs-action-dock-links";
      filteredRows.forEach((item) => {
        const link = document.createElement("a");
        link.href = item.href;
        link.textContent = item.label;
        links.appendChild(link);
      });
      if (!links.children.length) {
        const none = document.createElement("span");
        none.className = "wolfbbs-muted";
        none.textContent = "No items";
        links.appendChild(none);
      }
      group.appendChild(links);
      return group;
    }

    function renderActionDock() {
      body.innerHTML = "";
      body.appendChild(searchWrap);
      const actions = actionsForRoute(currentRoute).map((item) => ({
        href: item.href,
        label: item.label
      }));
      const pinned = loadRoutePins();
      const trail = Object.assign({ visited: [] }, readJSON(sessionTrailKey, {}));
      const recent = (trail.visited || [])
        .map((href) => ({ href: normalizePath(href), label: humanizeRoutePart(String(href).split("/").filter(Boolean).slice(-1)[0] || "start") }))
        .filter((item) => item.href && item.href !== currentRoute)
        .slice(0, 8);
      const queryText = String(searchInput.value || "");
      body.appendChild(renderGroup("Route actions", actions, queryText));
      body.appendChild(renderGroup("Pinned routes", pinned, queryText));
      body.appendChild(renderGroup("Recent route hops", recent, queryText));
      dock.setAttribute("data-collapsed", state.collapsed ? "true" : "false");
      dock.setAttribute("data-side", state.side);
      titleNode.textContent = state.collapsed ? "Quick Launch" : "Quick Launch Dock";
      refreshButton.hidden = state.collapsed;
      sideButton.hidden = state.collapsed;
      sideButton.textContent = state.side === "left" ? "Dock right" : "Dock left";
      collapseButton.textContent = state.collapsed ? "Open" : "Close";
      window.requestAnimationFrame(maybeAutoAdjustDock);
    }

    collapseButton.addEventListener("click", () => {
      setCollapsed(!state.collapsed, "manual");
    });
    refreshButton.addEventListener("click", () => {
      renderActionDock();
      showToast("Action dock refreshed", "ok");
      trackTelemetry("action-dock:refresh");
    });
    sideButton.addEventListener("click", () => {
      state.side = state.side === "left" ? "right" : "left";
      persistDockState();
      renderActionDock();
      trackTelemetry("action-dock:side-toggle");
    });
    searchInput.addEventListener("input", () => {
      renderActionDock();
    });
    window.addEventListener("resize", () => {
      window.requestAnimationFrame(maybeAutoAdjustDock);
    });
    window.addEventListener("scroll", () => {
      if (scrollAdjustTimer) window.clearTimeout(scrollAdjustTimer);
      scrollAdjustTimer = window.setTimeout(() => {
        maybeAutoAdjustDock();
      }, 80);
    }, { passive: true });
    window.wolfbbsRenderActionDock = renderActionDock;
    window.wolfbbsSetActionDockCollapsed = (collapsed) => {
      setCollapsed(collapsed, "external");
    };
    renderActionDock();
  }
  // The floating action dock created too much competing chrome for normal routes.

  const goalCoachRegistry = {
    "/start": [
      { id: "connect", label: "Pick a connection route", href: "/connect" },
      { id: "tour", label: "Open guided tour", href: "/tour" },
      { id: "signin", label: "Sign in with caller account", href: "/login" }
    ],
    "/today": [
      { id: "attention", label: "Review action queue", href: "/attention" },
      { id: "boards", label: "Check watched boards", href: "/boards?mode=watched" },
      { id: "events", label: "Review upcoming events", href: "/events" }
    ],
    "/admin/setup": [
      { id: "identity", label: "Save identity and safety settings", href: "/admin/setup" },
      { id: "boards", label: "Seed baseline boards", href: "/admin/setup" },
      { id: "caller", label: "Create non-sysop caller account", href: "/admin/users" }
    ],
    "/admin/launch": [
      { id: "checks", label: "Review launch readiness checks", href: "/admin/launch" },
      { id: "ops", label: "Verify ops center is clean", href: "/admin/ops" },
      { id: "walk", label: "Run real caller walkthrough", href: "/connect" }
    ],
    "/chat": [
      { id: "join", label: "Join #lobby", href: "/chat?channel=%23lobby" },
      { id: "send", label: "Send one message", href: "/chat" },
      { id: "presence", label: "Verify online list", href: "/chat" }
    ]
  };

  function goalsForRoute(pathname) {
    const path = normalizePath(pathname);
    if (!path) return [];
    if (goalCoachRegistry[path]) return goalCoachRegistry[path];
    const prefix = Object.keys(goalCoachRegistry).find((key) => path.indexOf(key + "/") === 0);
    return prefix ? goalCoachRegistry[prefix] : [];
  }

  function goalStorageKey(route) {
    return goalsPrefix + normalizePath(route || currentRoute || "/");
  }

  function mountGoalCoach() {
    if (document.querySelector(".wolfbbs-goal-coach")) return;
    const goals = goalsForRoute(currentRoute);
    if (!goals.length) return;
    const key = goalStorageKey(currentRoute);
    const state = Object.assign({}, readJSON(key, {}));
    const card = document.createElement("section");
    card.className = "wolfbbs-goal-coach";
    const head = document.createElement("div");
    head.className = "wolfbbs-goal-head";
    const titleNode = document.createElement("strong");
    titleNode.textContent = "Goal Coach";
    const progress = document.createElement("span");
    head.appendChild(titleNode);
    head.appendChild(progress);
    card.appendChild(head);
    const list = document.createElement("ul");
    list.className = "wolfbbs-goal-list";
    goals.forEach((goal) => {
      const item = document.createElement("li");
      const box = document.createElement("input");
      box.type = "checkbox";
      box.setAttribute("aria-label", goal.label);
      box.checked = Boolean(state[goal.id]);
      box.addEventListener("change", () => {
        state[goal.id] = box.checked;
        writeJSON(key, state);
        syncProgress();
        trackTelemetry("goals:toggle");
      });
      item.appendChild(box);
      const label = document.createElement("span");
      label.textContent = goal.label;
      item.appendChild(label);
      if (goal.href) {
        const link = document.createElement("a");
        link.href = goal.href;
        link.textContent = "open";
        item.appendChild(link);
      }
      list.appendChild(item);
    });
    card.appendChild(list);
    const actions = document.createElement("div");
    actions.className = "wolfbbs-goal-actions";
    const complete = document.createElement("button");
    complete.type = "button";
    complete.textContent = "Complete all";
    complete.addEventListener("click", () => {
      goals.forEach((goal) => { state[goal.id] = true; });
      writeJSON(key, state);
      list.querySelectorAll('input[type="checkbox"]').forEach((node) => { node.checked = true; });
      syncProgress();
      showToast("Goal coach completed", "ok");
      trackTelemetry("goals:complete-all");
    });
    actions.appendChild(complete);
    const reset = document.createElement("button");
    reset.type = "button";
    reset.textContent = "Reset goals";
    reset.addEventListener("click", () => {
      goals.forEach((goal) => { state[goal.id] = false; });
      writeJSON(key, state);
      list.querySelectorAll('input[type="checkbox"]').forEach((node) => { node.checked = false; });
      syncProgress();
      trackTelemetry("goals:reset");
    });
    actions.appendChild(reset);
    card.appendChild(actions);

    function syncProgress() {
      const done = goals.filter((goal) => Boolean(state[goal.id])).length;
      progress.textContent = done + "/" + goals.length + " complete";
    }
    syncProgress();
    const target = document.querySelector(".wolfbbs-guide-strip") || document.querySelector(".wolfbbs-recent-rail") || document.querySelector("main.wolfbbs-main");
    if (target && target.parentNode) {
      target.parentNode.insertBefore(card, target.nextSibling);
    }
  }

  function mountRouteScorecard() {
    if (document.querySelector(".wolfbbs-scorecard")) return;
    if (routeKind(currentRoute) !== "admin" || currentRouteProfile.dense) return;
    const forms = document.querySelectorAll("form").length;
    const tables = document.querySelectorAll("table").length;
    const links = document.querySelectorAll("a[href]").length;
    const headings = document.querySelectorAll("h2,h3").length;
    const actionable = Math.min(40, forms * 8 + tables * 6 + Math.min(links, 24));
    const guidance = Math.min(30, headings * 3);
    const structure = Math.min(30, document.querySelectorAll("section,article").length);
    const score = clamp(actionable + guidance + structure, 0, 100);
    const level = score >= 76 ? "strong" : score >= 56 ? "ok" : "warn";
    if (score >= 76) return;
    const bar = document.createElement("div");
    bar.className = "wolfbbs-scorecard";
    const scorePill = document.createElement("span");
    scorePill.className = "wolfbbs-score-pill";
    scorePill.setAttribute("data-level", level === "strong" ? "strong" : level === "warn" ? "warn" : "ok");
    scorePill.textContent = "Operator review " + score + "/100";
    bar.appendChild(scorePill);
    const note = document.createElement("p");
    note.className = "wolfbbs-scorecard-note";
    note.textContent = actionable + " actions • " + guidance + " guidance cues • " + structure + " structure blocks";
    bar.appendChild(note);
    const hero = document.querySelector(".wolfbbs-page-hero");
    if (hero && hero.parentNode) {
      hero.parentNode.insertBefore(bar, hero.nextSibling);
    }
  }

  function enhanceEmptyStates() {
    const candidates = Array.from(document.querySelectorAll("p,li,td")).filter((node) => {
      if (node.dataset.wolfbbsEmptyEnhanced === "1") return false;
      const text = (node.textContent || "").trim();
      return /^(no\b.*\b(yet|now)|no active\b|seasonal challenge is not configured)/i.test(text);
    });
    if (!candidates.length) return;
    const actions = actionsForRoute(currentRoute);
    candidates.slice(0, 4).forEach((node) => {
      const row = document.createElement("div");
      row.className = "wolfbbs-empty-actions";
      actions.slice(0, 2).forEach((action) => {
        const link = document.createElement("a");
        link.href = action.href;
        link.textContent = action.label;
        row.appendChild(link);
      });
      node.appendChild(row);
      node.dataset.wolfbbsEmptyEnhanced = "1";
    });
  }

  function mountQuickNotesWorkspace() {
    if (document.getElementById("wolfbbsNotesButton")) return;
    const button = document.createElement("button");
    button.id = "wolfbbsNotesButton";
    button.type = "button";
    button.textContent = "Quick Notes";
    document.body.appendChild(button);

    const overlay = document.createElement("div");
    overlay.id = "wolfbbsNotesOverlay";
    overlay.innerHTML = '<div id="wolfbbsNotesPanel"><div id="wolfbbsNotesHeader"><strong>Quick Notes Workspace</strong><button type="button" id="wolfbbsNotesClose">Close</button></div><textarea id="wolfbbsNotesArea" placeholder="Capture operator notes, release checks, and caller follow-up items."></textarea><div class="wolfbbs-notes-actions"><button type="button" id="wolfbbsNotesCopy">Copy notes</button><button type="button" id="wolfbbsNotesDownload">Download notes</button><button type="button" id="wolfbbsNotesClear">Clear notes</button></div></div>';
    document.body.appendChild(overlay);
    const area = overlay.querySelector("#wolfbbsNotesArea");
    const close = overlay.querySelector("#wolfbbsNotesClose");
    const copy = overlay.querySelector("#wolfbbsNotesCopy");
    const download = overlay.querySelector("#wolfbbsNotesDownload");
    const clear = overlay.querySelector("#wolfbbsNotesClear");
    area.value = String(localStorage.getItem(notesKey) || "");
    area.addEventListener("input", () => {
      localStorage.setItem(notesKey, area.value || "");
    });
    function open() {
      overlay.classList.add("active");
      window.setTimeout(() => area.focus(), 20);
      trackTelemetry("notes:open");
    }
    function shut() {
      overlay.classList.remove("active");
    }
    button.addEventListener("click", open);
    close.addEventListener("click", shut);
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) shut();
    });
    copy.addEventListener("click", () => {
      copyText(area.value || "").then(() => {
        showToast("Notes copied", "ok");
        trackTelemetry("notes:copy");
      }).catch(() => showToast("Copy failed", "error"));
    });
    download.addEventListener("click", () => {
      downloadJSONFile("wolfbbs-notes.json", {
        route: currentRoute,
        updatedAt: new Date().toISOString(),
        body: area.value || ""
      });
      showToast("Notes downloaded", "ok");
      trackTelemetry("notes:download");
    });
    clear.addEventListener("click", () => {
      area.value = "";
      localStorage.setItem(notesKey, "");
      showToast("Notes cleared", "ok");
      trackTelemetry("notes:clear");
    });
    document.addEventListener("keydown", (event) => {
      if ((event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "n") {
        event.preventDefault();
        open();
      }
    });
  }

  function mountUXDiagnosticsButton() {
    if (document.getElementById("wolfbbsUXDiagButton")) return;
    const button = document.createElement("button");
    button.id = "wolfbbsUXDiagButton";
    button.type = "button";
    button.textContent = "UX Diag";
    button.addEventListener("click", () => {
      downloadJSONFile("wolfbbs-ui-diagnostics.json", {
        route: currentRoute,
        generatedAt: new Date().toISOString(),
        prefs: readJSON(uiPrefsKey, {}),
        focusMode: readJSON(focusModeKey, {}),
        focusTimer: readJSON(focusKey, {}),
        favorites: readJSON(favoritesKey, []),
        notes: String(localStorage.getItem(notesKey) || ""),
        telemetry: loadTelemetry()
      });
      showToast("UI diagnostics exported", "ok");
      trackTelemetry("diagnostics:quick-export");
    });
    document.body.appendChild(button);
  }

  function loadWorkspaces() {
    return readJSON(workspaceKey, []).filter((item) => item && item.id && item.name);
  }

  function saveWorkspaces(rows) {
    const clean = rows.filter((item) => item && item.id && item.name).slice(0, 24);
    writeJSON(workspaceKey, clean);
    return clean;
  }

  function buildHandoffMarkdown() {
    const telemetry = loadTelemetry();
    const favorites = loadFavorites().map((item) => "- [" + item.label + "](" + item.href + ")").join("\n");
    const topEvents = Object.keys(telemetry.events || {})
      .map((key) => ({ key: key, value: telemetry.events[key] }))
      .sort((a, b) => b.value - a.value)
      .slice(0, 8)
      .map((row) => "- " + row.key + ": " + row.value)
      .join("\n");
    return [
      "# WolfBBS UI Handoff",
      "",
      "- Generated: " + new Date().toISOString(),
      "- Route: " + currentRoute,
      "",
      "## Favorites",
      favorites || "- (none)",
      "",
      "## Top UX Signals",
      topEvents || "- (none)",
      "",
      "## Notes",
      "~~~",
      String(localStorage.getItem(notesKey) || "").trim() || "(none)",
      "~~~"
    ].join("\n");
  }

  function mountWorkspaceHub() {
    if (document.getElementById("wolfbbsWorkspaceOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsWorkspaceOverlay";
    overlay.innerHTML = '<div id="wolfbbsWorkspacePanel"><div class="wolfbbs-inline-actions"><strong>Workspace Hub</strong><button type="button" id="wolfbbsWorkspaceClose">Close</button><button type="button" id="wolfbbsWorkspaceCreate">Save current as workspace</button><button type="button" id="wolfbbsWorkspaceHandoff">Export handoff</button></div><div id="wolfbbsWorkspaceList"></div></div>';
    document.body.appendChild(overlay);
    const list = overlay.querySelector("#wolfbbsWorkspaceList");
    const close = overlay.querySelector("#wolfbbsWorkspaceClose");
    const create = overlay.querySelector("#wolfbbsWorkspaceCreate");
    const handoff = overlay.querySelector("#wolfbbsWorkspaceHandoff");

    function render() {
      list.innerHTML = "";
      const rows = loadWorkspaces();
      if (!rows.length) {
        const empty = document.createElement("p");
        empty.textContent = "No workspaces saved yet.";
        list.appendChild(empty);
        return;
      }
      rows.forEach((row) => {
        const card = document.createElement("div");
        card.className = "wolfbbs-workspace-row";
        const title = document.createElement("strong");
        title.textContent = row.name;
        card.appendChild(title);
        const meta = document.createElement("p");
        meta.textContent = (row.routes || []).join(" | ");
        card.appendChild(meta);
        const actions = document.createElement("div");
        actions.className = "wolfbbs-inline-actions";
        const open = document.createElement("button");
        open.type = "button";
        open.textContent = "Open first route";
        open.addEventListener("click", () => {
          if (Array.isArray(row.routes) && row.routes.length) {
            location.href = row.routes[0];
          }
        });
        actions.appendChild(open);
        const remove = document.createElement("button");
        remove.type = "button";
        remove.textContent = "Delete workspace";
        remove.addEventListener("click", () => {
          const next = loadWorkspaces().filter((item) => item.id !== row.id);
          saveWorkspaces(next);
          render();
          trackTelemetry("workspace:delete");
        });
        actions.appendChild(remove);
        card.appendChild(actions);
        list.appendChild(card);
      });
    }

    function openHub() {
      overlay.classList.add("active");
      render();
      trackTelemetry("workspace:open");
    }
    function closeHub() {
      overlay.classList.remove("active");
    }

    window.wolfbbsOpenWorkspaceHub = openHub;
    close.addEventListener("click", closeHub);
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) closeHub();
    });
    create.addEventListener("click", () => {
      const name = window.prompt("Workspace name", title + " Workspace");
      if (!name) return;
      const rows = loadWorkspaces();
      rows.unshift({
        id: "ws-" + Math.random().toString(36).slice(2, 10),
        name: String(name).trim().slice(0, 60),
        createdAt: new Date().toISOString(),
        routes: [currentRoute].concat(loadFavorites().map((item) => item.href)).slice(0, 8)
      });
      saveWorkspaces(rows);
      render();
      showToast("Workspace saved", "ok");
      trackTelemetry("workspace:create");
    });
    handoff.addEventListener("click", () => {
      const markdown = buildHandoffMarkdown();
      copyText(markdown).then(() => {
        showToast("Handoff markdown copied", "ok");
      }).catch(() => showToast("Handoff copy failed", "error"));
      downloadJSONFile("wolfbbs-handoff.json", {
        generatedAt: new Date().toISOString(),
        route: currentRoute,
        markdown: markdown
      });
      trackTelemetry("handoff:export");
    });
  }

  function mountToastCenter() {
    if (document.getElementById("wolfbbsToastCenterOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsToastCenterOverlay";
    overlay.innerHTML = '<div id="wolfbbsToastCenterPanel"><div class="wolfbbs-inline-actions"><strong>Notification Center</strong><button type="button" id="wolfbbsToastCenterClose">Close</button><button type="button" id="wolfbbsToastCenterClear">Clear</button></div><ul id="wolfbbsToastCenterList"></ul></div>';
    document.body.appendChild(overlay);
    const list = overlay.querySelector("#wolfbbsToastCenterList");
    const close = overlay.querySelector("#wolfbbsToastCenterClose");
    const clear = overlay.querySelector("#wolfbbsToastCenterClear");
    function render() {
      list.innerHTML = "";
      const rows = readJSON(toastHistoryKey, []);
      if (!rows.length) {
        const li = document.createElement("li");
        li.textContent = "No notifications yet.";
        list.appendChild(li);
        return;
      }
      rows.slice(0, 40).forEach((row) => {
        const li = document.createElement("li");
        li.textContent = "[" + (row.kind || "ok") + "] " + row.text + " • " + row.at;
        list.appendChild(li);
      });
    }
    function openCenter() {
      overlay.classList.add("active");
      render();
      trackTelemetry("toast-center:open");
    }
    window.wolfbbsOpenToastCenter = openCenter;
    close.addEventListener("click", () => overlay.classList.remove("active"));
    clear.addEventListener("click", () => {
      writeJSON(toastHistoryKey, []);
      render();
      trackTelemetry("toast-center:clear");
    });
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.classList.remove("active");
    });
  }

  function mountSpotlightSearch() {
    if (document.getElementById("wolfbbsSpotlightOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsSpotlightOverlay";
    overlay.innerHTML = '<div id="wolfbbsSpotlightPanel"><div class="wolfbbs-inline-actions"><strong>Spotlight Search</strong><button type="button" id="wolfbbsSpotlightClose">Close</button></div><label>Find on page <input id="wolfbbsSpotlightInput" type="search" placeholder="Type to spotlight"></label><p id="wolfbbsSpotlightCount" class="wolfbbs-muted">0 matches</p><div class="wolfbbs-inline-actions"><button type="button" id="wolfbbsSpotlightClear">Clear spotlight</button></div></div>';
    document.body.appendChild(overlay);
    const input = overlay.querySelector("#wolfbbsSpotlightInput");
    const count = overlay.querySelector("#wolfbbsSpotlightCount");
    const close = overlay.querySelector("#wolfbbsSpotlightClose");
    const clear = overlay.querySelector("#wolfbbsSpotlightClear");
    function apply(query) {
      const q = String(query || "").trim().toLowerCase();
      let hits = 0;
      Array.from(document.querySelectorAll("main.wolfbbs-main p, main.wolfbbs-main li, main.wolfbbs-main td, main.wolfbbs-main h2, main.wolfbbs-main h3")).forEach((node) => {
        const match = q && (node.textContent || "").toLowerCase().includes(q);
        node.classList.toggle("wolfbbs-spotlight-hit", Boolean(match));
        if (match) hits += 1;
      });
      count.textContent = hits + " match(es)";
      writeJSON(spotlightKey, { query: q });
    }
    function open() {
      overlay.classList.add("active");
      const saved = readJSON(spotlightKey, {});
      input.value = saved.query || "";
      apply(input.value);
      window.setTimeout(() => input.focus(), 20);
      trackTelemetry("spotlight:open");
    }
    window.wolfbbsOpenSpotlight = open;
    input.addEventListener("input", () => apply(input.value));
    clear.addEventListener("click", () => {
      input.value = "";
      apply("");
    });
    close.addEventListener("click", () => overlay.classList.remove("active"));
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.classList.remove("active");
    });
    document.addEventListener("keydown", (event) => {
      const tag = event.target && event.target.tagName ? event.target.tagName.toLowerCase() : "";
      const editing = tag === "input" || tag === "textarea" || tag === "select" || event.target.isContentEditable;
      if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "f") {
        event.preventDefault();
        open();
      }
    });
  }

  function loadCheckpoints() {
    return readJSON(checkpointKey, []).filter((item) => item && item.id && item.name);
  }

  function saveCheckpoints(rows) {
    const clean = rows.filter((item) => item && item.id && item.name).slice(0, 24);
    writeJSON(checkpointKey, clean);
    return clean;
  }

  function captureCheckpointPayload() {
    return {
      prefs: readJSON(uiPrefsKey, {}),
      favorites: readJSON(favoritesKey, []),
      notes: String(localStorage.getItem(notesKey) || ""),
      focusMode: readJSON(focusModeKey, {}),
      spotlight: readJSON(spotlightKey, {}),
      route: currentRoute
    };
  }

  function applyCheckpointPayload(payload) {
    if (!payload || typeof payload !== "object") return;
    writeJSON(uiPrefsKey, payload.prefs || {});
    writeJSON(favoritesKey, payload.favorites || []);
    localStorage.setItem(notesKey, payload.notes || "");
    writeJSON(focusModeKey, payload.focusMode || { enabled: false });
    writeJSON(spotlightKey, payload.spotlight || { query: "" });
    window.location.reload();
  }

  function mountCheckpointHub() {
    if (document.getElementById("wolfbbsCheckpointOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsCheckpointOverlay";
    overlay.style.cssText = "position:fixed;inset:0;background:rgba(8,17,30,.45);display:none;z-index:68;padding:22px 16px;";
    overlay.innerHTML = '<div style="max-width:700px;margin:0 auto;background:#fff;border:1px solid #c8d7e8;border-radius:14px;box-shadow:0 20px 40px rgba(8,19,36,.28);padding:14px 15px;"><div class="wolfbbs-inline-actions"><strong>Session Checkpoints</strong><button type="button" id="wolfbbsCheckpointClose">Close</button><button type="button" id="wolfbbsCheckpointCreate">Save checkpoint</button></div><div id="wolfbbsCheckpointList" style="display:grid;gap:8px;margin-top:10px;"></div></div>';
    document.body.appendChild(overlay);
    const list = overlay.querySelector("#wolfbbsCheckpointList");
    const close = overlay.querySelector("#wolfbbsCheckpointClose");
    const create = overlay.querySelector("#wolfbbsCheckpointCreate");
    function render() {
      list.innerHTML = "";
      const rows = loadCheckpoints();
      if (!rows.length) {
        const p = document.createElement("p");
        p.textContent = "No checkpoints saved.";
        list.appendChild(p);
        return;
      }
      rows.forEach((row) => {
        const card = document.createElement("div");
        card.className = "wolfbbs-workspace-row";
        const title = document.createElement("strong");
        title.textContent = row.name;
        card.appendChild(title);
        const meta = document.createElement("p");
        meta.textContent = row.createdAt;
        card.appendChild(meta);
        const actions = document.createElement("div");
        actions.className = "wolfbbs-inline-actions";
        const restore = document.createElement("button");
        restore.type = "button";
        restore.textContent = "Restore checkpoint";
        restore.addEventListener("click", () => applyCheckpointPayload(row.payload));
        actions.appendChild(restore);
        const remove = document.createElement("button");
        remove.type = "button";
        remove.textContent = "Delete checkpoint";
        remove.addEventListener("click", () => {
          saveCheckpoints(loadCheckpoints().filter((item) => item.id !== row.id));
          render();
          trackTelemetry("checkpoint:delete");
        });
        actions.appendChild(remove);
        card.appendChild(actions);
        list.appendChild(card);
      });
    }
    function open() {
      overlay.style.display = "block";
      render();
      trackTelemetry("checkpoint:open");
    }
    window.wolfbbsOpenCheckpointHub = open;
    close.addEventListener("click", () => overlay.style.display = "none");
    create.addEventListener("click", () => {
      const name = window.prompt("Checkpoint name", "Checkpoint " + new Date().toLocaleTimeString());
      if (!name) return;
      const rows = loadCheckpoints();
      rows.unshift({
        id: "cp-" + Math.random().toString(36).slice(2, 10),
        name: String(name).trim().slice(0, 60),
        createdAt: new Date().toISOString(),
        payload: captureCheckpointPayload()
      });
      saveCheckpoints(rows);
      render();
      trackTelemetry("checkpoint:create");
    });
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.style.display = "none";
    });
  }

  function applyStorageUpdateKey(key) {
    const targetKey = String(key || "").trim();
    if (!targetKey) return;
    if (targetKey === uiPrefsKey) {
      const next = Object.assign({}, readJSON(uiPrefsKey, {}));
      if (next.theme) uiPrefs.theme = next.theme;
      if (next.density) uiPrefs.density = next.density;
      if (next.fontScale) uiPrefs.fontScale = next.fontScale;
      applyUIPrefs();
    }
    if (targetKey === favoritesKey) {
      renderFavoritesRail();
    }
    if (targetKey === notesKey) {
      const notesArea = document.getElementById("wolfbbsNotesArea");
      if (notesArea && notesArea !== document.activeElement) {
        notesArea.value = String(localStorage.getItem(notesKey) || "");
      }
    }
    if (targetKey === telemetryKey) {
      window.dispatchEvent(new Event("wolfbbs-telemetry-updated"));
    }
  }

  function mountCrossTabSync() {
    if (window.BroadcastChannel) {
      try {
        wolfbbsSyncChannel = new BroadcastChannel(syncChannelName);
        wolfbbsSyncChannel.onmessage = (event) => {
          const payload = event && event.data ? event.data : {};
          if (!payload || payload.source === syncTabID) return;
          if (payload.type === "storage:update") {
            applyStorageUpdateKey(payload.key);
          }
        };
      } catch (_) {
        wolfbbsSyncChannel = null;
      }
    }
    window.addEventListener("storage", (event) => {
      if (!event || !event.key) return;
      applyStorageUpdateKey(event.key);
    });
    window.addEventListener("wolfbbs-storage-updated", (event) => {
      const detail = event && event.detail ? event.detail : {};
      if (!detail || !detail.key) return;
      applyStorageUpdateKey(detail.key);
    });
  }

  function draftFormKey(form, index) {
    const action = normalizePath(form.getAttribute("action") || currentRoute || "/");
    const method = (form.getAttribute("method") || "get").toLowerCase();
    return draftKeyPrefix + normalizePath(currentRoute || "/") + ":" + method + ":" + action + ":" + String(index || 0);
  }

  function captureDraftPayload(form) {
    const fields = Array.from(form.querySelectorAll("input[name], textarea[name], select[name]"));
    const payload = {};
    fields.forEach((field) => {
      const type = (field.getAttribute("type") || "").toLowerCase();
      if (type === "hidden" || type === "password" || type === "file" || type === "submit" || type === "button") return;
      if (type === "checkbox" || type === "radio") {
        payload[field.name] = Boolean(field.checked);
        return;
      }
      payload[field.name] = field.value || "";
    });
    return payload;
  }

  function applyDraftPayload(form, values) {
    const payload = values && typeof values === "object" ? values : {};
    const fields = Array.from(form.querySelectorAll("input[name], textarea[name], select[name]"));
    fields.forEach((field) => {
      const type = (field.getAttribute("type") || "").toLowerCase();
      if (type === "hidden" || type === "password" || type === "file" || type === "submit" || type === "button") return;
      if (!(field.name in payload)) return;
      if (type === "checkbox" || type === "radio") {
        field.checked = Boolean(payload[field.name]);
        return;
      }
      field.value = payload[field.name];
    });
  }

  function listRouteDrafts() {
    const prefix = draftKeyPrefix + normalizePath(currentRoute || "/") + ":";
    const out = [];
    for (let i = 0; i < localStorage.length; i++) {
      const key = String(localStorage.key(i) || "");
      if (!key.startsWith(prefix)) continue;
      const value = readJSON(key, null);
      if (!value || typeof value !== "object" || !value.values) continue;
      out.push({
        key: key,
        value: value
      });
    }
    out.sort((a, b) => String(b.value.updatedAt || "").localeCompare(String(a.value.updatedAt || "")));
    return out.slice(0, 40);
  }

  function mountDraftCenter() {
    if (document.getElementById("wolfbbsDraftOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsDraftOverlay";
    overlay.innerHTML = '<div id="wolfbbsDraftPanel"><div class="wolfbbs-inline-actions"><strong>Draft Center</strong><button type="button" id="wolfbbsDraftClose">Close</button><button type="button" id="wolfbbsDraftRefresh">Refresh</button><button type="button" id="wolfbbsDraftExport">Export drafts</button></div><div id="wolfbbsDraftList"></div></div>';
    document.body.appendChild(overlay);
    const list = overlay.querySelector("#wolfbbsDraftList");
    const close = overlay.querySelector("#wolfbbsDraftClose");
    const refresh = overlay.querySelector("#wolfbbsDraftRefresh");
    const exportButton = overlay.querySelector("#wolfbbsDraftExport");
    function render() {
      list.innerHTML = "";
      const rows = listRouteDrafts();
      if (!rows.length) {
        const p = document.createElement("p");
        p.textContent = "No route drafts saved yet.";
        list.appendChild(p);
        return;
      }
      const forms = Array.from(document.querySelectorAll("form"));
      rows.forEach((row) => {
        const card = document.createElement("div");
        card.className = "wolfbbs-draft-row";
        const heading = document.createElement("strong");
        const idx = Number(row.value.formIndex || -1);
        heading.textContent = idx >= 0 ? ("Form #" + (idx + 1)) : "Form draft";
        card.appendChild(heading);
        const meta = document.createElement("p");
        meta.className = "wolfbbs-muted";
        meta.textContent = String(row.value.updatedAt || "");
        card.appendChild(meta);
        const actions = document.createElement("div");
        actions.className = "wolfbbs-inline-actions";
        const restore = document.createElement("button");
        restore.type = "button";
        restore.textContent = "Restore";
        restore.addEventListener("click", () => {
          const target = idx >= 0 && idx < forms.length ? forms[idx] : null;
          if (!target) {
            showToast("Draft form is not available on this page", "error");
            return;
          }
          applyDraftPayload(target, row.value.values);
          showToast("Draft restored", "ok");
          trackTelemetry("draft:center-restore");
          if (!target.querySelector(".wolfbbs-draft-banner")) {
            const banner = document.createElement("div");
            banner.className = "wolfbbs-restore-banner wolfbbs-draft-banner";
            banner.textContent = "Draft restored from Draft Center.";
            target.insertBefore(banner, target.firstChild);
          }
        });
        actions.appendChild(restore);
        const remove = document.createElement("button");
        remove.type = "button";
        remove.textContent = "Delete";
        remove.addEventListener("click", () => {
          localStorage.removeItem(row.key);
          render();
          trackTelemetry("draft:center-delete");
        });
        actions.appendChild(remove);
        card.appendChild(actions);
        list.appendChild(card);
      });
    }
    function open() {
      overlay.classList.add("active");
      render();
      trackTelemetry("draft:center-open");
    }
    window.wolfbbsOpenDraftCenter = open;
    close.addEventListener("click", () => overlay.classList.remove("active"));
    refresh.addEventListener("click", render);
    exportButton.addEventListener("click", () => {
      downloadJSONFile("wolfbbs-drafts.json", {
        route: currentRoute,
        generatedAt: new Date().toISOString(),
        drafts: listRouteDrafts()
      });
      showToast("Drafts exported", "ok");
      trackTelemetry("draft:center-export");
    });
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.classList.remove("active");
    });
  }

  function kpiWatchersKey(route) {
    return kpiWatchKeyPrefix + normalizePath(route || currentRoute || "/");
  }

  function loadKPIWatchers(route) {
    return Object.assign({}, readJSON(kpiWatchersKey(route), {}));
  }

  function saveKPIWatchers(route, payload) {
    const next = payload && typeof payload === "object" ? payload : {};
    writeJSON(kpiWatchersKey(route), next);
    return next;
  }

  function evaluateKPIWatchers(snapshot) {
    const watchers = loadKPIWatchers(currentRoute);
    let changed = false;
    Object.keys(watchers).forEach((slot) => {
      const row = watchers[slot];
      if (!row || !row.enabled) return;
      const value = Number(snapshot && snapshot[slot]);
      const threshold = Number(row.threshold);
      if (!Number.isFinite(value) || !Number.isFinite(threshold)) return;
      if (value >= threshold && Number(row.lastAlertValue || -1) !== value) {
        showToast("KPI watch: " + slot + " reached " + formatCompactNumber(value), "ok");
        row.lastAlertValue = value;
        watchers[slot] = row;
        changed = true;
        trackTelemetry("kpi-watch:trigger");
      }
    });
    if (changed) {
      saveKPIWatchers(currentRoute, watchers);
    }
  }

  function mountKPIWatchCenter() {
    if (document.getElementById("wolfbbsKPIWatchOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsKPIWatchOverlay";
    overlay.style.cssText = "position:fixed;inset:0;background:rgba(8,17,30,.45);display:none;z-index:69;padding:22px 16px;";
    overlay.innerHTML = '<div id="wolfbbsReleaseGatePanel"><div class="wolfbbs-inline-actions"><strong>KPI Watch Center</strong><button type="button" id="wolfbbsKPIWatchClose">Close</button><button type="button" id="wolfbbsKPIWatchReset">Reset</button></div><div id="wolfbbsKPIWatchList" style="display:grid;gap:8px;margin-top:10px;"></div></div>';
    document.body.appendChild(overlay);
    const list = overlay.querySelector("#wolfbbsKPIWatchList");
    const close = overlay.querySelector("#wolfbbsKPIWatchClose");
    const reset = overlay.querySelector("#wolfbbsKPIWatchReset");
    function render() {
      list.innerHTML = "";
      const cards = Array.from(document.querySelectorAll(".wolfbbs-kpi-card"));
      const watchers = loadKPIWatchers(currentRoute);
      if (!cards.length) {
        const p = document.createElement("p");
        p.textContent = "No KPI cards on this route.";
        list.appendChild(p);
        return;
      }
      cards.forEach((card, idx) => {
        const strong = card.querySelector("strong");
        const slot = "kpi_" + idx;
        const currentVal = parseNumericValue(strong ? strong.textContent : "");
        const row = document.createElement("div");
        row.className = "wolfbbs-release-gate-row";
        const title = document.createElement("strong");
        title.textContent = slot + " • current " + (currentVal === null ? "--" : formatCompactNumber(currentVal));
        row.appendChild(title);
        const actions = document.createElement("div");
        actions.className = "wolfbbs-inline-actions";
        const set = document.createElement("button");
        set.type = "button";
        set.textContent = watchers[slot] && watchers[slot].enabled ? ("Threshold " + watchers[slot].threshold) : "Set threshold";
        set.addEventListener("click", () => {
          const seed = watchers[slot] && watchers[slot].enabled ? String(watchers[slot].threshold) : (currentVal === null ? "1" : String(Math.ceil(currentVal)));
          const raw = window.prompt("Threshold for " + slot, seed);
          if (!raw) return;
          const value = Number(raw);
          if (!Number.isFinite(value)) {
            showToast("Threshold must be numeric", "error");
            return;
          }
          watchers[slot] = { enabled: true, threshold: value, updatedAt: new Date().toISOString() };
          saveKPIWatchers(currentRoute, watchers);
          render();
          trackTelemetry("kpi-watch:set");
        });
        actions.appendChild(set);
        const clear = document.createElement("button");
        clear.type = "button";
        clear.textContent = "Clear";
        clear.addEventListener("click", () => {
          delete watchers[slot];
          saveKPIWatchers(currentRoute, watchers);
          render();
          trackTelemetry("kpi-watch:clear");
        });
        actions.appendChild(clear);
        row.appendChild(actions);
        list.appendChild(row);
      });
    }
    function open() {
      overlay.style.display = "block";
      render();
      trackTelemetry("kpi-watch:open");
    }
    window.wolfbbsOpenKPIWatchCenter = open;
    close.addEventListener("click", () => overlay.style.display = "none");
    reset.addEventListener("click", () => {
      saveKPIWatchers(currentRoute, {});
      render();
      trackTelemetry("kpi-watch:reset");
    });
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.style.display = "none";
    });
  }

  function loadIncidents() {
    return readJSON(incidentKey, []).filter((row) => row && row.id && row.title);
  }

  function saveIncidents(rows) {
    const clean = rows.filter((row) => row && row.id && row.title).slice(0, 80);
    writeJSON(incidentKey, clean);
    return clean;
  }

  function mountIncidentConsole() {
    if (document.getElementById("wolfbbsIncidentOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsIncidentOverlay";
    overlay.innerHTML = '<div id="wolfbbsIncidentPanel"><div class="wolfbbs-inline-actions"><strong>Incident Console</strong><button type="button" id="wolfbbsIncidentClose">Close</button><button type="button" id="wolfbbsIncidentCreate">Create incident</button><button type="button" id="wolfbbsIncidentExport">Export</button></div><div id="wolfbbsIncidentList"></div></div>';
    document.body.appendChild(overlay);
    const list = overlay.querySelector("#wolfbbsIncidentList");
    const close = overlay.querySelector("#wolfbbsIncidentClose");
    const create = overlay.querySelector("#wolfbbsIncidentCreate");
    const exportButton = overlay.querySelector("#wolfbbsIncidentExport");
    function render() {
      list.innerHTML = "";
      const rows = loadIncidents();
      if (!rows.length) {
        const p = document.createElement("p");
        p.textContent = "No incidents logged.";
        list.appendChild(p);
        return;
      }
      rows.forEach((row) => {
        const card = document.createElement("div");
        card.className = "wolfbbs-incident-row";
        const title = document.createElement("strong");
        title.textContent = row.title;
        card.appendChild(title);
        const meta = document.createElement("p");
        meta.className = "wolfbbs-muted";
        meta.textContent = (row.status || "open") + " • " + (row.createdAt || "");
        card.appendChild(meta);
        const badge = document.createElement("span");
        badge.className = "wolfbbs-incident-badge";
        badge.setAttribute("data-severity", row.severity || "medium");
        badge.textContent = "severity " + (row.severity || "medium");
        card.appendChild(badge);
        const actions = document.createElement("div");
        actions.className = "wolfbbs-inline-actions";
        const toggle = document.createElement("button");
        toggle.type = "button";
        toggle.textContent = row.status === "resolved" ? "Reopen" : "Resolve";
        toggle.addEventListener("click", () => {
          const next = loadIncidents().map((item) => {
            if (item.id !== row.id) return item;
            const clone = Object.assign({}, item);
            clone.status = item.status === "resolved" ? "open" : "resolved";
            clone.resolvedAt = clone.status === "resolved" ? new Date().toISOString() : "";
            return clone;
          });
          saveIncidents(next);
          render();
          trackTelemetry("incident:toggle");
        });
        actions.appendChild(toggle);
        const remove = document.createElement("button");
        remove.type = "button";
        remove.textContent = "Delete";
        remove.addEventListener("click", () => {
          saveIncidents(loadIncidents().filter((item) => item.id !== row.id));
          render();
          trackTelemetry("incident:delete");
        });
        actions.appendChild(remove);
        card.appendChild(actions);
        list.appendChild(card);
      });
    }
    function open() {
      overlay.classList.add("active");
      render();
      trackTelemetry("incident:open");
    }
    window.wolfbbsOpenIncidentConsole = open;
    close.addEventListener("click", () => overlay.classList.remove("active"));
    create.addEventListener("click", () => {
      const title = String(window.prompt("Incident title", "Service health check") || "").trim();
      if (!title) return;
      const severityRaw = String(window.prompt("Severity (low, medium, high)", "medium") || "medium").toLowerCase();
      const severity = ["low", "medium", "high"].includes(severityRaw) ? severityRaw : "medium";
      const note = String(window.prompt("Optional note", "") || "").trim();
      const rows = loadIncidents();
      rows.unshift({
        id: "inc-" + Math.random().toString(36).slice(2, 10),
        title: title.slice(0, 100),
        severity: severity,
        note: note.slice(0, 500),
        status: "open",
        route: currentRoute,
        createdAt: new Date().toISOString()
      });
      saveIncidents(rows);
      render();
      trackTelemetry("incident:create");
      showToast("Incident logged", "ok");
    });
    exportButton.addEventListener("click", () => {
      downloadJSONFile("wolfbbs-incidents.json", {
        generatedAt: new Date().toISOString(),
        incidents: loadIncidents()
      });
      trackTelemetry("incident:export");
    });
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.classList.remove("active");
    });
  }

  const playbookRegistry = {
    "/admin/setup": [
      "Validate identity and public host",
      "Seed baseline boards and channels",
      "Create one non-sysop account",
      "Run status verification checks"
    ],
    "/admin/launch": [
      "Review launch readiness blockers",
      "Run caller journey smoke test",
      "Confirm alert channels are working",
      "Publish launch bulletin"
    ],
    "/admin/ops": [
      "Review unresolved incidents",
      "Check active sessions and rate limits",
      "Review admin audit tail",
      "Export diagnostics package"
    ]
  };

  function playbookForRoute(route) {
    const path = normalizePath(route || currentRoute || "/");
    if (playbookRegistry[path]) return playbookRegistry[path];
    const prefix = Object.keys(playbookRegistry).find((key) => path.indexOf(key + "/") === 0);
    return prefix ? playbookRegistry[prefix] : [
      "Review route objective",
      "Complete primary action",
      "Capture notes for handoff",
      "Mark route complete"
    ];
  }

  function playbookKey(route) {
    return playbookKeyPrefix + normalizePath(route || currentRoute || "/");
  }

  function mountPlaybookRunner() {
    if (document.getElementById("wolfbbsPlaybookOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsPlaybookOverlay";
    overlay.innerHTML = '<div id="wolfbbsPlaybookPanel"><div class="wolfbbs-inline-actions"><strong>Playbook Runner</strong><button type="button" id="wolfbbsPlaybookClose">Close</button><button type="button" id="wolfbbsPlaybookComplete">Complete all</button><button type="button" id="wolfbbsPlaybookReset">Reset</button></div><div id="wolfbbsPlaybookList"></div></div>';
    document.body.appendChild(overlay);
    const list = overlay.querySelector("#wolfbbsPlaybookList");
    const close = overlay.querySelector("#wolfbbsPlaybookClose");
    const complete = overlay.querySelector("#wolfbbsPlaybookComplete");
    const reset = overlay.querySelector("#wolfbbsPlaybookReset");
    function render() {
      list.innerHTML = "";
      const steps = playbookForRoute(currentRoute);
      const state = Object.assign({}, readJSON(playbookKey(currentRoute), {}));
      const head = document.createElement("p");
      const done = steps.filter((_, idx) => Boolean(state["step_" + idx])).length;
      head.className = "wolfbbs-muted";
      head.textContent = done + "/" + steps.length + " complete";
      list.appendChild(head);
      steps.forEach((step, idx) => {
        const row = document.createElement("div");
        row.className = "wolfbbs-playbook-row";
        const actions = document.createElement("div");
        actions.className = "wolfbbs-inline-actions";
        const box = document.createElement("input");
        box.type = "checkbox";
        box.setAttribute("aria-label", step);
        box.checked = Boolean(state["step_" + idx]);
        box.addEventListener("change", () => {
          state["step_" + idx] = box.checked;
          writeJSON(playbookKey(currentRoute), state);
          render();
          trackTelemetry("playbook:toggle");
        });
        actions.appendChild(box);
        const label = document.createElement("span");
        label.textContent = step;
        actions.appendChild(label);
        row.appendChild(actions);
        list.appendChild(row);
      });
    }
    function open() {
      overlay.classList.add("active");
      render();
      trackTelemetry("playbook:open");
    }
    window.wolfbbsOpenPlaybookRunner = open;
    close.addEventListener("click", () => overlay.classList.remove("active"));
    complete.addEventListener("click", () => {
      const steps = playbookForRoute(currentRoute);
      const state = {};
      steps.forEach((_, idx) => { state["step_" + idx] = true; });
      writeJSON(playbookKey(currentRoute), state);
      render();
      trackTelemetry("playbook:complete-all");
    });
    reset.addEventListener("click", () => {
      writeJSON(playbookKey(currentRoute), {});
      render();
      trackTelemetry("playbook:reset");
    });
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.classList.remove("active");
    });
  }

  function loadReminders() {
    return readJSON(reminderKey, []).filter((row) => row && row.id && row.text);
  }

  function saveReminders(rows) {
    const clean = rows.filter((row) => row && row.id && row.text).slice(0, 120);
    writeJSON(reminderKey, clean);
    return clean;
  }

  function mountReminderScheduler() {
    if (document.getElementById("wolfbbsReminderOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsReminderOverlay";
    overlay.innerHTML = '<div id="wolfbbsReminderPanel"><div class="wolfbbs-inline-actions"><strong>Reminder Scheduler</strong><button type="button" id="wolfbbsReminderClose">Close</button><button type="button" id="wolfbbsReminderCreate">Add reminder</button><button type="button" id="wolfbbsReminderClearDone">Clear done</button></div><div id="wolfbbsReminderList"></div></div>';
    document.body.appendChild(overlay);
    const list = overlay.querySelector("#wolfbbsReminderList");
    const close = overlay.querySelector("#wolfbbsReminderClose");
    const create = overlay.querySelector("#wolfbbsReminderCreate");
    const clearDone = overlay.querySelector("#wolfbbsReminderClearDone");
    function render() {
      list.innerHTML = "";
      const rows = loadReminders();
      if (!rows.length) {
        const p = document.createElement("p");
        p.textContent = "No reminders yet.";
        list.appendChild(p);
        return;
      }
      rows.forEach((row) => {
        const dueAt = new Date(row.dueAt || "").getTime();
        const due = Number.isFinite(dueAt) && dueAt <= Date.now() && row.done !== true;
        const card = document.createElement("div");
        card.className = "wolfbbs-reminder-row";
        card.setAttribute("data-due", due ? "true" : "false");
        const title = document.createElement("strong");
        title.textContent = row.text;
        card.appendChild(title);
        const meta = document.createElement("p");
        meta.className = "wolfbbs-muted";
        meta.textContent = (row.done ? "done" : "scheduled") + " • due " + String(row.dueAt || "");
        card.appendChild(meta);
        const actions = document.createElement("div");
        actions.className = "wolfbbs-inline-actions";
        const toggle = document.createElement("button");
        toggle.type = "button";
        toggle.textContent = row.done ? "Reopen" : "Done";
        toggle.addEventListener("click", () => {
          const next = loadReminders().map((item) => {
            if (item.id !== row.id) return item;
            const clone = Object.assign({}, item);
            clone.done = !item.done;
            clone.fired = clone.done;
            return clone;
          });
          saveReminders(next);
          render();
          trackTelemetry("reminder:toggle");
        });
        actions.appendChild(toggle);
        const remove = document.createElement("button");
        remove.type = "button";
        remove.textContent = "Delete";
        remove.addEventListener("click", () => {
          saveReminders(loadReminders().filter((item) => item.id !== row.id));
          render();
          trackTelemetry("reminder:delete");
        });
        actions.appendChild(remove);
        card.appendChild(actions);
        list.appendChild(card);
      });
    }
    function tick() {
      const rows = loadReminders();
      let changed = false;
      rows.forEach((row) => {
        const dueAt = new Date(row.dueAt || "").getTime();
        if (!Number.isFinite(dueAt) || row.done || row.fired) return;
        if (dueAt <= Date.now()) {
          row.fired = true;
          changed = true;
          showToast("Reminder: " + row.text, "ok");
          trackTelemetry("reminder:fired");
        }
      });
      if (changed) {
        saveReminders(rows);
      }
    }
    function open() {
      overlay.classList.add("active");
      render();
      trackTelemetry("reminder:open");
    }
    window.wolfbbsOpenReminderScheduler = open;
    close.addEventListener("click", () => overlay.classList.remove("active"));
    create.addEventListener("click", () => {
      const text = String(window.prompt("Reminder text", "Follow up on board replies") || "").trim();
      if (!text) return;
      const minutesRaw = String(window.prompt("Due in minutes", "30") || "30").trim();
      const minutes = Number(minutesRaw);
      if (!Number.isFinite(minutes) || minutes <= 0) {
        showToast("Minutes must be a positive number", "error");
        return;
      }
      const rows = loadReminders();
      rows.unshift({
        id: "rem-" + Math.random().toString(36).slice(2, 10),
        text: text.slice(0, 140),
        dueAt: new Date(Date.now() + minutes * 60 * 1000).toISOString(),
        done: false,
        fired: false,
        route: currentRoute,
        createdAt: new Date().toISOString()
      });
      saveReminders(rows);
      render();
      trackTelemetry("reminder:create");
    });
    clearDone.addEventListener("click", () => {
      saveReminders(loadReminders().filter((row) => !row.done));
      render();
      trackTelemetry("reminder:clear-done");
    });
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.classList.remove("active");
    });
    tick();
    window.setInterval(tick, 15000);
  }

  function mountReleaseGate() {
    if (document.getElementById("wolfbbsReleaseGateOverlay")) return;
    const checks = [
      { id: "tests", label: "All automated test suites are green" },
      { id: "smoke", label: "Smoke verification passed for target env" },
      { id: "auth", label: "Auth and RBAC behavior verified" },
      { id: "chat", label: "Chat and moderation flow verified" },
      { id: "docs", label: "Install and product docs are up to date" },
      { id: "observability", label: "Status and diagnostics reviewed" }
    ];
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsReleaseGateOverlay";
    overlay.innerHTML = '<div id="wolfbbsReleaseGatePanel"><div class="wolfbbs-inline-actions"><strong>Release Gate</strong><button type="button" id="wolfbbsReleaseGateClose">Close</button><button type="button" id="wolfbbsReleaseGateExport">Export gate</button><button type="button" id="wolfbbsReleaseGateReset">Reset</button></div><div id="wolfbbsReleaseGateSummary" class="wolfbbs-muted"></div><div id="wolfbbsReleaseGateList"></div></div>';
    document.body.appendChild(overlay);
    const summary = overlay.querySelector("#wolfbbsReleaseGateSummary");
    const list = overlay.querySelector("#wolfbbsReleaseGateList");
    const close = overlay.querySelector("#wolfbbsReleaseGateClose");
    const exportButton = overlay.querySelector("#wolfbbsReleaseGateExport");
    const reset = overlay.querySelector("#wolfbbsReleaseGateReset");
    function render() {
      const state = Object.assign({}, readJSON(releaseGateKey, {}));
      const done = checks.filter((row) => Boolean(state[row.id])).length;
      summary.textContent = "Release confidence " + done + "/" + checks.length;
      list.innerHTML = "";
      checks.forEach((row) => {
        const item = document.createElement("div");
        item.className = "wolfbbs-release-gate-row";
        const actions = document.createElement("div");
        actions.className = "wolfbbs-inline-actions";
        const box = document.createElement("input");
        box.type = "checkbox";
        box.setAttribute("aria-label", row.label);
        box.checked = Boolean(state[row.id]);
        box.addEventListener("change", () => {
          state[row.id] = box.checked;
          writeJSON(releaseGateKey, state);
          render();
          trackTelemetry("release-gate:toggle");
        });
        actions.appendChild(box);
        const label = document.createElement("span");
        label.textContent = row.label;
        actions.appendChild(label);
        item.appendChild(actions);
        list.appendChild(item);
      });
    }
    function open() {
      overlay.classList.add("active");
      render();
      trackTelemetry("release-gate:open");
    }
    window.wolfbbsOpenReleaseGate = open;
    close.addEventListener("click", () => overlay.classList.remove("active"));
    reset.addEventListener("click", () => {
      writeJSON(releaseGateKey, {});
      render();
      trackTelemetry("release-gate:reset");
    });
    exportButton.addEventListener("click", () => {
      downloadJSONFile("wolfbbs-release-gate.json", {
        route: currentRoute,
        generatedAt: new Date().toISOString(),
        checks: checks,
        state: readJSON(releaseGateKey, {})
      });
      trackTelemetry("release-gate:export");
    });
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.classList.remove("active");
    });
  }

  function mountFeedbackPulse() {
    if (document.getElementById("wolfbbsFeedbackButton")) return;
    const button = document.createElement("button");
    button.id = "wolfbbsFeedbackButton";
    button.type = "button";
    button.textContent = "Feedback";
    document.body.appendChild(button);
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsFeedbackOverlay";
    overlay.innerHTML = '<div id="wolfbbsFeedbackPanel"><div class="wolfbbs-inline-actions"><strong>Feedback Pulse</strong><button type="button" id="wolfbbsFeedbackClose">Close</button><button type="button" id="wolfbbsFeedbackExport">Export</button></div><p class="wolfbbs-muted">Rate this route and capture one note for product improvement.</p><div class="wolfbbs-rating-row" id="wolfbbsFeedbackRatings"></div><textarea id="wolfbbsFeedbackNote" placeholder="What should improve next?" style="width:100%;min-height:110px;margin-top:10px;"></textarea><div class="wolfbbs-inline-actions" style="margin-top:10px;"><button type="button" id="wolfbbsFeedbackSave">Save feedback</button></div></div>';
    document.body.appendChild(overlay);
    const ratings = overlay.querySelector("#wolfbbsFeedbackRatings");
    const close = overlay.querySelector("#wolfbbsFeedbackClose");
    const exportButton = overlay.querySelector("#wolfbbsFeedbackExport");
    const save = overlay.querySelector("#wolfbbsFeedbackSave");
    const note = overlay.querySelector("#wolfbbsFeedbackNote");
    let selected = 0;
    function syncRatings() {
      ratings.innerHTML = "";
      for (let i = 1; i <= 5; i++) {
        const chip = document.createElement("button");
        chip.type = "button";
        chip.textContent = String(i);
        chip.setAttribute("data-active", selected === i ? "true" : "false");
        chip.addEventListener("click", () => {
          selected = i;
          syncRatings();
        });
        ratings.appendChild(chip);
      }
    }
    function open() {
      overlay.classList.add("active");
      note.value = "";
      selected = 0;
      syncRatings();
      trackTelemetry("feedback:open");
    }
    window.wolfbbsOpenFeedbackPulse = open;
    button.addEventListener("click", open);
    close.addEventListener("click", () => overlay.classList.remove("active"));
    save.addEventListener("click", () => {
      if (selected <= 0) {
        showToast("Choose a rating before saving", "error");
        return;
      }
      const rows = readJSON(feedbackKey, []);
      rows.unshift({
        id: "fb-" + Math.random().toString(36).slice(2, 10),
        route: currentRoute,
        rating: selected,
        note: String(note.value || "").trim().slice(0, 600),
        createdAt: new Date().toISOString()
      });
      writeJSON(feedbackKey, rows.slice(0, 120));
      showToast("Feedback saved", "ok");
      trackTelemetry("feedback:save");
      overlay.classList.remove("active");
    });
    exportButton.addEventListener("click", () => {
      downloadJSONFile("wolfbbs-feedback.json", {
        generatedAt: new Date().toISOString(),
        feedback: readJSON(feedbackKey, [])
      });
      trackTelemetry("feedback:export");
    });
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) overlay.classList.remove("active");
    });
    syncRatings();
  }

  function mountMacroHelp() {
    if (document.getElementById("wolfbbsMacroHelpOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsMacroHelpOverlay";
    overlay.innerHTML = '<div id="wolfbbsMacroHelpPanel"><h3>Keyboard Macros</h3><ul><li><strong>Ctrl/Cmd+K</strong> open command palette</li><li><strong>?</strong> open command palette</li><li><strong>Alt+1..6</strong> quick route jump</li><li><strong>Alt+J / Alt+K</strong> section navigation</li><li><strong>Ctrl/Cmd+Shift+N</strong> quick notes</li><li><strong>Ctrl/Cmd+Shift+D / I / P / M / G</strong> drafts, incidents, playbooks, reminders, release gate</li><li><strong>Ctrl/Cmd+Shift+U</strong> bug capture</li><li><strong>F1</strong> keyboard help</li></ul><p class="wolfbbs-help-copy">Esc closes overlays.</p></div>';
    document.body.appendChild(overlay);
    function openHelp() {
      overlay.classList.add("active");
      trackTelemetry("macro:help-open");
    }
    function closeHelp() {
      overlay.classList.remove("active");
    }
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) closeHelp();
    });
    window.wolfbbsOpenMacroHelp = openHelp;
    document.addEventListener("keydown", (event) => {
      if (event.key === "F1") {
        event.preventDefault();
        openHelp();
        return;
      }
      if (event.key === "Escape" && overlay.classList.contains("active")) {
        event.preventDefault();
        closeHelp();
      }
    });
  }

  function buildBugCapturePayload() {
    const telemetry = loadTelemetry();
    const topEvents = Object.entries(telemetry.events || {})
      .sort((a, b) => Number(b[1] || 0) - Number(a[1] || 0))
      .slice(0, 15)
      .map((entry) => ({ key: entry[0], count: Number(entry[1] || 0) }));
    return {
      route: currentRoute,
      path: location.pathname + location.search + location.hash,
      title: document.title || title,
      generatedAt: new Date().toISOString(),
      userAgent: navigator.userAgent,
      uiPrefs: readJSON(uiPrefsKey, {}),
      focusMode: readJSON(focusModeKey, {}),
      focusTimer: readJSON(focusKey, {}),
      session: readJSON(sessionTrailKey, {}),
      notifications: readJSON(toastHistoryKey, []).slice(0, 25),
      telemetryTopEvents: topEvents,
      notesPreview: String(localStorage.getItem(notesKey) || "").slice(0, 800)
    };
  }

  function mountBugCapture() {
    if (document.getElementById("wolfbbsBugOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsBugOverlay";
    overlay.innerHTML = '<div id="wolfbbsBugPanel"><h3>Bug Report Capture</h3><p class="wolfbbs-muted">Captures route, UI state, telemetry summary, and notification history for reproducible bug reports.</p><label>Snapshot JSON<textarea id="wolfbbsBugPayload"></textarea></label><div class="wolfbbs-inline-actions"><button type="button" id="wolfbbsBugRefresh">Refresh</button><button type="button" id="wolfbbsBugCopy">Copy JSON</button><button type="button" id="wolfbbsBugDownload">Download JSON</button><button type="button" id="wolfbbsBugClose">Close</button></div></div>';
    document.body.appendChild(overlay);
    const payloadField = overlay.querySelector("#wolfbbsBugPayload");
    function render() {
      if (!payloadField) return;
      payloadField.value = JSON.stringify(buildBugCapturePayload(), null, 2);
    }
    function open() {
      render();
      overlay.classList.add("active");
      trackTelemetry("bug-capture:open");
    }
    function close() {
      overlay.classList.remove("active");
    }
    overlay.querySelector("#wolfbbsBugRefresh").addEventListener("click", () => {
      render();
      showToast("Bug snapshot refreshed", "ok");
      trackTelemetry("bug-capture:refresh");
    });
    overlay.querySelector("#wolfbbsBugCopy").addEventListener("click", () => {
      copyText(payloadField.value || "").then(() => {
        showToast("Bug snapshot copied", "ok");
      }).catch(() => {
        showToast("Could not copy bug snapshot", "error");
      });
      trackTelemetry("bug-capture:copy");
    });
    overlay.querySelector("#wolfbbsBugDownload").addEventListener("click", () => {
      const payload = buildBugCapturePayload();
      downloadJSONFile("wolfbbs-bug-capture.json", payload);
      writeJSON(bugCaptureKey, payload);
      showToast("Bug snapshot downloaded", "ok");
      trackTelemetry("bug-capture:download");
    });
    overlay.querySelector("#wolfbbsBugClose").addEventListener("click", close);
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) close();
    });
    const quickButton = document.createElement("button");
    quickButton.type = "button";
    quickButton.id = "wolfbbsBugButton";
    quickButton.textContent = "Bug capture";
    quickButton.addEventListener("click", open);
    document.body.appendChild(quickButton);
    window.wolfbbsOpenBugCapture = open;
    document.addEventListener("keydown", (event) => {
      const editingTag = event.target && event.target.tagName ? event.target.tagName.toLowerCase() : "";
      const editing = editingTag === "input" || editingTag === "textarea" || editingTag === "select" || (event.target && event.target.isContentEditable);
      if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "u") {
        event.preventDefault();
        open();
        return;
      }
      if (event.key === "Escape" && overlay.classList.contains("active")) {
        event.preventDefault();
        close();
      }
    });
  }

  function mountContextHelpDrawer() {
    if (document.getElementById("wolfbbsContextHelpOverlay")) return;
    const overlay = document.createElement("div");
    overlay.id = "wolfbbsContextHelpOverlay";
    overlay.innerHTML = '<div id="wolfbbsContextHelpPanel"><h3>Context Help</h3><p class="wolfbbs-help-copy" id="wolfbbsContextHelpBody"></p><div id="wolfbbsContextHelpActions" class="wolfbbs-action-dock-links"></div><div class="wolfbbs-inline-actions"><button type="button" id="wolfbbsContextHelpClose">Close</button></div></div>';
    document.body.appendChild(overlay);
    const bodyNode = overlay.querySelector("#wolfbbsContextHelpBody");
    const actionsNode = overlay.querySelector("#wolfbbsContextHelpActions");
    function render() {
      const primer = typeof primerForPath === "function" ? primerForPath(currentRoute) : null;
      const actions = typeof actionsForRoute === "function" ? actionsForRoute(currentRoute) : [];
      bodyNode.textContent = primer && primer.body ? String(primer.body).trim() : "Use this route as a focused step in the caller/sysop loop.";
      actionsNode.innerHTML = "";
      actions.slice(0, 8).forEach((item) => {
        const link = document.createElement("a");
        link.href = item.href;
        link.textContent = item.label;
        actionsNode.appendChild(link);
      });
      if (!actionsNode.children.length) {
        const fallback = document.createElement("a");
        fallback.href = "/help";
        fallback.textContent = "Open help";
        actionsNode.appendChild(fallback);
      }
    }
    function open() {
      render();
      overlay.classList.add("active");
      trackTelemetry("context-help:open");
    }
    function close() {
      overlay.classList.remove("active");
    }
    overlay.querySelector("#wolfbbsContextHelpClose").addEventListener("click", close);
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) close();
    });
    window.wolfbbsOpenContextHelp = open;
  }

  function mountMobileToolsHub() {
    if (document.getElementById("wolfbbsMobileToolsOverlay")) return;
    const button = document.createElement("button");
    button.id = "wolfbbsMobileToolsButton";
    button.type = "button";
    button.textContent = "Quick Tools";
    document.body.appendChild(button);

    const overlay = document.createElement("div");
    overlay.id = "wolfbbsMobileToolsOverlay";
    overlay.innerHTML = '<div id="wolfbbsMobileToolsPanel"><div class="wolfbbs-inline-actions"><strong>Quick Tools</strong><button type="button" id="wolfbbsMobileToolsClose">Close</button></div><p class="wolfbbs-help-copy">Fast actions tuned for dense pages and narrow viewports.</p><div id="wolfbbsMobileToolsList"></div></div>';
    document.body.appendChild(overlay);

    const list = overlay.querySelector("#wolfbbsMobileToolsList");
    const close = overlay.querySelector("#wolfbbsMobileToolsClose");

    function openNamedSurface(name) {
      if (typeof window[name] === "function") {
        window[name]();
        return true;
      }
      return false;
    }

    function hideHub() {
      overlay.classList.remove("active");
    }

    function showHub() {
      render();
      overlay.classList.add("active");
      trackTelemetry("mobile-tools:open");
    }

    function actionButton(label, handler) {
      const node = document.createElement("button");
      node.type = "button";
      node.textContent = label;
      node.addEventListener("click", () => {
        const handled = handler();
        if (handled !== false) {
          hideHub();
        }
      });
      return node;
    }

    function routeLink(label, href) {
      const link = document.createElement("a");
      link.href = href;
      link.textContent = label;
      return link;
    }

    function renderGroup(title, rows) {
      if (!rows.length) return null;
      const group = document.createElement("section");
      group.className = "wolfbbs-mobile-tools-group";
      const heading = document.createElement("strong");
      heading.textContent = title;
      group.appendChild(heading);
      const grid = document.createElement("div");
      grid.className = "wolfbbs-mobile-tools-grid";
      rows.forEach((row) => grid.appendChild(row));
      group.appendChild(grid);
      return group;
    }

    function render() {
      list.innerHTML = "";

      const utilityRows = [
        actionButton("Open omnibar", () => {
          const trigger = document.getElementById("wolfbbsCommandButton");
          if (trigger) {
            trigger.click();
            return true;
          }
          return false;
        }),
        actionButton("Quick notes", () => {
          const trigger = document.getElementById("wolfbbsNotesButton");
          if (trigger) {
            trigger.click();
            return true;
          }
          return false;
        }),
        actionButton("Context help", () => openNamedSurface("wolfbbsOpenContextHelp")),
        actionButton("Feedback", () => openNamedSurface("wolfbbsOpenFeedbackPulse")),
        actionButton("Bug report", () => openNamedSurface("wolfbbsOpenBugCapture")),
        actionButton("UI diagnostics", () => {
          const trigger = document.getElementById("wolfbbsUXDiagButton");
          if (trigger) {
            trigger.click();
            return true;
          }
          return false;
        }),
      ];

      const routeRows = actionsForRoute(currentRoute).map((item) => routeLink(item.label, item.href));
      const pinnedRows = loadRoutePins().slice(0, 6).map((item) => routeLink(item.label, item.href));
      const trail = Object.assign({ visited: [] }, readJSON(sessionTrailKey, {}));
      const recentRows = (trail.visited || [])
        .map((href) => normalizePath(href))
        .filter((href) => href && href !== currentRoute)
        .slice(0, 6)
        .map((href) => routeLink(humanizeRoutePart(String(href).split("/").filter(Boolean).slice(-1)[0] || "start"), href));

      [renderGroup("Utilities", utilityRows), renderGroup("Next routes", routeRows), renderGroup("Pinned", pinnedRows), renderGroup("Recent", recentRows)]
        .filter(Boolean)
        .forEach((group) => list.appendChild(group));
    }

    button.addEventListener("click", showHub);
    close.addEventListener("click", hideHub);
    overlay.addEventListener("click", (event) => {
      if (event.target === overlay) hideHub();
    });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && overlay.classList.contains("active")) {
        event.preventDefault();
        hideHub();
      }
      if (!isCompactViewport()) return;
      if (isEditingTarget(event.target)) return;
      if ((event.metaKey || event.ctrlKey) && String(event.key).toLowerCase() === ".") {
        event.preventDefault();
        showHub();
      }
    });
    window.wolfbbsOpenMobileTools = showHub;
  }

  function isEditingTarget(node) {
    if (!node) return false;
    const tag = node.tagName ? node.tagName.toLowerCase() : "";
    if (tag === "input" || tag === "textarea" || tag === "select") return true;
    return Boolean(node.isContentEditable);
  }

  function mountUXRound20Pass() {
    const main = document.querySelector("main.wolfbbs-main");
    const hero = document.querySelector(".wolfbbs-page-hero");
    const heroMeta = document.querySelector(".wolfbbs-page-hero-meta");
    const navRow = document.querySelector("p.wolfbbs-nav-row");
    const routeActions = actionsForRoute(currentRoute).slice(0, currentRouteProfile.dense ? 2 : 3);

    // 1-4: route compass, context path, route actions, and surface metrics.
    if (shouldMountRouteScaffolding(currentRoute) && !document.querySelector(".wolfbbs-ux20-compass")) {
      const compass = document.createElement("section");
      compass.className = "wolfbbs-ux20-compass";
      const left = document.createElement("div");
      const heading = document.createElement("strong");
      heading.textContent = "On this page";
      left.appendChild(heading);
      const sub = document.createElement("p");
      const path = document.createElement("span");
      path.className = "wolfbbs-ux20-path";
      path.textContent = currentRoute || "/";
      sub.appendChild(path);
      sub.appendChild(document.createTextNode(" \u2022 primary actions and structure at a glance"));
      left.appendChild(sub);
      if (routeActions.length) {
        const row = document.createElement("div");
        row.className = "wolfbbs-ux20-action-row";
        routeActions.forEach((action) => {
          const link = document.createElement("a");
          link.href = action.href;
          link.textContent = action.label;
          link.addEventListener("click", () => trackTelemetry("ux20:compass-action"));
          row.appendChild(link);
        });
        left.appendChild(row);
      }
      const stats = [
        { label: "Forms", value: document.querySelectorAll("form").length },
        { label: "Tables", value: document.querySelectorAll("table").length },
        { label: "Sections", value: document.querySelectorAll("main.wolfbbs-main > section, main.wolfbbs-main > article").length || document.querySelectorAll("section,article").length },
        { label: "Inputs", value: document.querySelectorAll("input,textarea,select").length }
      ];
      const metrics = document.createElement("p");
      metrics.className = "wolfbbs-ux20-metrics-summary";
      metrics.textContent = stats.map((row) => row.label + " " + row.value).join(" \u2022 ");
      left.appendChild(metrics);
      compass.appendChild(left);
      if (hero && hero.parentNode) {
        hero.parentNode.insertBefore(compass, hero.nextSibling);
      } else if (navRow && navRow.parentNode) {
        navRow.parentNode.insertBefore(compass, navRow.nextSibling);
      } else if (main && main.parentNode) {
        main.parentNode.insertBefore(compass, main);
      }
    }

    // 5-6: end-of-page next-step guide with route actions and trail.
    if (shouldMountRouteScaffolding(currentRoute) && main && !main.querySelector(".wolfbbs-ux20-next") && !currentRouteProfile.dense) {
      const next = document.createElement("section");
      next.className = "wolfbbs-ux20-next";
      const titleNode = document.createElement("strong");
      titleNode.textContent = "Recommended Next Steps";
      next.appendChild(titleNode);
      const list = document.createElement("ul");
      routeActions.slice(0, 3).forEach((action) => {
        const li = document.createElement("li");
        const link = document.createElement("a");
        link.href = action.href;
        link.textContent = action.label;
        li.appendChild(link);
        list.appendChild(li);
      });
      const trail = Object.assign({ visited: [] }, readJSON(sessionTrailKey, {}));
      const previous = (trail.visited || []).find((item) => normalizePath(item) && normalizePath(item) !== currentRoute);
      if (previous) {
        const li = document.createElement("li");
        li.appendChild(document.createTextNode("Return to "));
        const link = document.createElement("a");
        link.href = previous;
        link.textContent = humanizeRoutePart(String(previous).split("/").filter(Boolean).slice(-1)[0] || "start");
        li.appendChild(link);
        list.appendChild(li);
      }
      if (!list.children.length) {
        const li = document.createElement("li");
        li.textContent = "Open /start for the guided route map.";
        list.appendChild(li);
      }
      next.appendChild(list);
      main.appendChild(next);
    }

    // 7-9: section filter and jump-to-next-incomplete live inside the tools panel.
    const sectionNav = document.querySelector(".wolfbbs-section-nav");
    if (sectionNav && sectionNav.dataset.wolfbbsUx20Enhanced !== "1") {
      sectionNav.dataset.wolfbbsUx20Enhanced = "1";
      const toolsPanel = sectionNav.wolfbbsToolsPanel || sectionNav.querySelector(".wolfbbs-section-nav-tools-panel");
      if (toolsPanel) {
        const filter = document.createElement("input");
        filter.type = "search";
        filter.className = "wolfbbs-ux20-section-filter";
        filter.placeholder = "Find section";
        const jump = document.createElement("button");
        jump.type = "button";
        jump.className = "wolfbbs-toolbar-button wolfbbs-section-nav-tool-button";
        jump.textContent = "Next open";
        toolsPanel.insertBefore(filter, toolsPanel.firstChild || null);
        toolsPanel.insertBefore(jump, filter.nextSibling);
        filter.addEventListener("input", () => {
          const q = (filter.value || "").trim().toLowerCase();
          Array.from(sectionNav.querySelectorAll('a[href^="#"]')).forEach((link) => {
            const hit = !q || (link.textContent || "").toLowerCase().includes(q);
            link.style.display = hit ? "" : "none";
            link.classList.toggle("wolfbbs-ux20-highlight", Boolean(q) && hit);
          });
        });
        jump.addEventListener("click", () => {
          const nextHeading = Array.from(document.querySelectorAll("h2[id],h3[id]")).find((heading) => !heading.classList.contains("wolfbbs-section-done"));
          if (!nextHeading) {
            showToast("All visible sections marked done", "ok");
            return;
          }
          location.hash = "#" + nextHeading.id;
          nextHeading.scrollIntoView({ behavior: "smooth", block: "start" });
          trackTelemetry("ux20:section-next-open");
        });
      }
    }

    // 10-12: form scaffolding belongs only on primer routes, not dense product screens.
    if (shouldMountRouteScaffolding(currentRoute)) {
      Array.from(document.querySelectorAll("form")).forEach((form) => {
        if (form.closest("table")) return;
        const fields = Array.from(form.querySelectorAll("input[name], textarea[name], select[name]")).filter((field) => {
          const type = (field.getAttribute("type") || "").toLowerCase();
          if (type === "hidden" || type === "submit" || type === "button" || type === "file" || type === "password") return false;
          return !field.disabled;
        });
        if (fields.length < 3) return;
        let requiredFields = fields.filter((field) => field.hasAttribute("required"));
        if (!requiredFields.length) {
          requiredFields = fields.filter((field) => {
            const tag = field.tagName.toLowerCase();
            const type = (field.getAttribute("type") || "").toLowerCase();
            return tag === "textarea" || tag === "select" || ["text", "email", "url", "search", "number", "tel"].includes(type || "text");
          }).slice(0, Math.min(4, fields.length));
        }
        if (!requiredFields.length) return;
        requiredFields.forEach((field) => {
          const label = field.closest("label") || (field.id ? form.querySelector('label[for="' + field.id + '"]') : null);
          if (!label || label.querySelector(".wolfbbs-ux20-required")) return;
          const marker = document.createElement("span");
          marker.className = "wolfbbs-ux20-required";
          marker.textContent = "*";
          label.appendChild(marker);
        });
        if (fields.length >= 4 && !form.querySelector('[data-ux20-reset="1"]')) {
          const reset = document.createElement("button");
          reset.type = "button";
          reset.className = "wolfbbs-form-secondary";
          reset.setAttribute("data-ux20-reset", "1");
          reset.textContent = "Reset fields";
          reset.addEventListener("click", () => {
            fields.forEach((field) => {
              if (field.type === "checkbox" || field.type === "radio") {
                field.checked = false;
              } else if (field.tagName.toLowerCase() === "select") {
                field.selectedIndex = 0;
              } else {
                field.value = "";
              }
              field.dispatchEvent(new Event("input", { bubbles: true }));
              field.dispatchEvent(new Event("change", { bubbles: true }));
            });
            trackTelemetry("ux20:form-reset");
            showToast("Form fields reset", "ok");
          });
          form.appendChild(reset);
        }
      });
    }

    // 13: double-submit guard to prevent duplicate posts.
    if (document.body.dataset.wolfbbsUx20SubmitGuard !== "1") {
      document.body.dataset.wolfbbsUx20SubmitGuard = "1";
      document.addEventListener("submit", (event) => {
        const form = event.target;
        if (!form || form.tagName.toLowerCase() !== "form") return;
        const now = Date.now();
        const previous = Number(form.dataset.wolfbbsUx20SubmitAt || 0);
        if (now - previous < 3500) {
          event.preventDefault();
          showToast("Submission already in progress. Please wait.", "error");
          trackTelemetry("ux20:submit-guard");
          return;
        }
        form.dataset.wolfbbsUx20SubmitAt = String(now);
      }, true);
    }

    // 14: inline clear controls for search fields.
    Array.from(document.querySelectorAll('input[type="search"]')).forEach((input) => {
      if (input.dataset.wolfbbsUx20Clear === "1") return;
      if (input.id === "wolfbbsPaletteInput") return;
      input.dataset.wolfbbsUx20Clear = "1";
      const wrap = document.createElement("span");
      wrap.className = "wolfbbs-ux20-search-wrap";
      input.parentNode.insertBefore(wrap, input);
      wrap.appendChild(input);
      const clear = document.createElement("button");
      clear.type = "button";
      clear.className = "wolfbbs-ux20-clear";
      clear.textContent = "Clear";
      clear.addEventListener("click", () => {
        input.value = "";
        input.dispatchEvent(new Event("input", { bubbles: true }));
        input.focus();
      });
      wrap.appendChild(clear);
    });

    // 15: freeze-first-column toggle for enhanced tables.
    Array.from(document.querySelectorAll(".wolfbbs-table-toolbar")).forEach((toolbar) => {
      if (toolbar.dataset.wolfbbsUx20Freeze === "1") return;
      const wrap = toolbar.nextElementSibling;
      const table = wrap ? wrap.querySelector("table") : null;
      if (!table) return;
      toolbar.dataset.wolfbbsUx20Freeze = "1";
      const freeze = document.createElement("button");
      freeze.type = "button";
      freeze.className = "wolfbbs-toolbar-button";
      freeze.textContent = "Freeze col 1";
      freeze.addEventListener("click", () => {
        const enabled = table.classList.toggle("wolfbbs-ux20-freeze-col");
        freeze.textContent = enabled ? "Unfreeze col 1" : "Freeze col 1";
        trackTelemetry("ux20:table-freeze");
      });
      toolbar.appendChild(freeze);
    });

    // 16: alt+click table row to copy row values.
    Array.from(document.querySelectorAll("table")).forEach((table) => {
      if (table.dataset.wolfbbsUx20CopyRows === "1") return;
      table.dataset.wolfbbsUx20CopyRows = "1";
      Array.from(table.querySelectorAll("tr")).forEach((row) => {
        row.addEventListener("click", (event) => {
          if (!event.altKey) return;
          event.preventDefault();
          event.stopPropagation();
          const text = Array.from(row.querySelectorAll("th,td")).map((cell) => (cell.textContent || "").trim()).filter(Boolean).join(" | ");
          if (!text) return;
          copyText(text).then(() => showToast("Row copied", "ok")).catch(() => showToast("Could not copy row", "error"));
          trackTelemetry("ux20:table-row-copy");
        });
      });
    });

    // 17: slash shortcut focuses the first available search field.
    if (!window.wolfbbsUX20SlashBound) {
      window.wolfbbsUX20SlashBound = true;
      document.addEventListener("keydown", (event) => {
        if (event.defaultPrevented) return;
        if (event.key !== "/" || event.ctrlKey || event.metaKey || event.altKey) return;
        if (isEditingTarget(event.target)) return;
        const target = document.querySelector('.wolfbbs-section-nav input[type="search"], .wolfbbs-table-toolbar input[type="search"], input[type="search"]');
        if (!target) return;
        event.preventDefault();
        target.focus();
        if (typeof target.select === "function") target.select();
        announceLive("Search focused");
        trackTelemetry("ux20:slash-search");
      });
    }

    // 18: escape blurs active field when no overlay is open.
    if (!window.wolfbbsUX20EscBound) {
      window.wolfbbsUX20EscBound = true;
      document.addEventListener("keydown", (event) => {
        if (event.key !== "Escape") return;
        if (document.querySelector(".active#wolfbbsPaletteOverlay, .active#wolfbbsShortcutOverlay, .active#wolfbbsNotesOverlay")) return;
        const active = document.activeElement;
        if (!isEditingTarget(active)) return;
        active.blur();
        announceLive("Input focus cleared");
        trackTelemetry("ux20:escape-blur");
      });
    }

    // 19: two-key route macros: g + key.
    if (!window.wolfbbsUX20GoBound) {
      window.wolfbbsUX20GoBound = true;
      let armedAt = 0;
      const goMap = {
        h: "/help",
        s: "/start",
        t: "/today",
        a: "/attention",
        b: "/boards",
        c: "/chat",
        m: "/mail",
        d: "/doors"
      };
      document.addEventListener("keydown", (event) => {
        if (event.defaultPrevented) return;
        if (event.ctrlKey || event.metaKey || event.altKey) return;
        if (isEditingTarget(event.target)) return;
        const key = String(event.key || "").toLowerCase();
        const now = Date.now();
        if (key === "g") {
          armedAt = now;
          return;
        }
        if (now - armedAt > 1200) return;
        armedAt = 0;
        if (!goMap[key]) return;
        event.preventDefault();
        trackTelemetry("ux20:go-macro");
        location.assign(goMap[key]);
      });
    }

  }

  mountSkipAndScrollUI();
  mountRevisitBanner();
  mountBreadcrumbs();
  applyGlossaryEnhancer();
  mountCrossTabSync();
  mountPreferenceControls();
  mountKPIDeltas();
  mountSectionToggles();
  mountGoalCoach();
  if (shouldMountRouteScaffolding(currentRoute)) {
    mountRouteScorecard();
  }
  enhanceEmptyStates();
  mountQuickNotesWorkspace();
  mountUXDiagnosticsButton();
  mountToastCenter();
  mountWorkspaceHub();
  mountCheckpointHub();
  mountSpotlightSearch();
  mountDraftCenter();
  mountKPIWatchCenter();
  mountIncidentConsole();
  mountPlaybookRunner();
  mountReminderScheduler();
  mountReleaseGate();
  mountFeedbackPulse();
  mountMacroHelp();
  mountShortcutLegendOverlay();
  mountContextHelpDrawer();
  mountBugCapture();
  mountMobileToolsHub();
  mountUXRound20Pass();

  const primerRegistry = {
    "/help": {
      eyebrow: "Start here",
      title: "Use WolfBBS by intent",
      body: "This page is the route map. If you run the board, finish /admin/setup before treating the product as ready. If you are a caller, start with boards, chat, and doors.",
      bullets: [
        "Use /admin/setup before /admin/config when launching a fresh board.",
        "Use /chat or IRC for the same live conversation layer.",
        "Use SSH when you want the full ANSI board feel."
      ],
      actions: [
        { label: "Admin setup", href: "/admin/setup" },
        { label: "Boards", href: "/boards" },
        { label: "Chat", href: "/chat" }
      ]
    },
    "/admin/setup": {
      eyebrow: "Launch path",
      title: "Finish setup in this order",
      body: "Identity and safety first, bootstrap actions second, then validate the real caller surfaces.",
      bullets: [
        "Save Step 1 and Step 2 before inviting users.",
        "Seed boards and create a real non-sysop account.",
        "Check /status and /admin/system after bootstrap."
      ],
      actions: [
        { label: "Admin config", href: "/admin/config" },
        { label: "Users", href: "/admin/users" },
        { label: "Status", href: "/status" }
      ]
    },
    "/admin/launch": {
      eyebrow: "Operator flow",
      title: "Run the board like a product, not a scavenger hunt",
      body: "This page consolidates launch readiness, runtime health, operator commands, and the direct links you need when the board is almost ready but not obviously done.",
      bullets: [
        "Fix launch blockers first, polish second.",
        "Walk the real caller journey before announcing anything.",
        "Use this route as the home base for first-run and recovery."
      ],
      actions: [
        { label: "Setup wizard", href: "/admin/setup" },
        { label: "System", href: "/admin/system" },
        { label: "Users", href: "/admin/users" }
      ]
    },
    "/admin/config": {
      eyebrow: "Runtime controls",
      title: "Use config after setup, not instead of it",
      body: "This page is for runtime flags, identity details, and service exposure after the baseline setup wizard is complete.",
      bullets: [
        "Keep public-facing changes deliberate.",
        "Verify status after changing ports, proxies, or optional services."
      ],
      actions: [
        { label: "Setup wizard", href: "/admin/setup" },
        { label: "System", href: "/admin/system" },
        { label: "Help", href: "/help" }
      ]
    },
    "/start": {
      eyebrow: "First stop",
      title: "Use Start Center to pick the right lane",
      body: "This page exists so guests, callers, and sysops can get a concrete next move without route hunting.",
      actions: [
        { label: "Connect", href: "/connect" },
        { label: "Today", href: "/today" },
        { label: "Help", href: "/help" },
        { label: "Boards", href: "/boards" }
      ]
    },
    "/today": {
      eyebrow: "Daily loop",
      title: "Today Brief compresses the caller day",
      body: "Use this page when you want watched boards, direct follow-up, and upcoming events in one place before you drift into route hunting.",
      actions: [
        { label: "Attention", href: "/attention" },
        { label: "Digest", href: "/digest" },
        { label: "Events", href: "/events" },
        { label: "Boards", href: "/boards?mode=watched" }
      ]
    },
    "/digest": {
      eyebrow: "Low-noise summary",
      title: "Daily Digest is the calmer web loop",
      body: "Use this when you want a once-per-day summary of follow-up, digest-tier boards, and scheduled events without opening every route.",
      actions: [
        { label: "Today", href: "/today" },
        { label: "Settings", href: "/settings" },
        { label: "Boards", href: "/boards?mode=digest" }
      ]
    },
    "/attention": {
      eyebrow: "Action queue",
      title: "Attention Center is the short list",
      body: "Use this page for the things that actually need response now: mentions, replies, unread mail, and board movement.",
      actions: [
        { label: "Boards", href: "/boards?mode=mentions" },
        { label: "Mail", href: "/mail?box=unread" },
        { label: "Discover", href: "/discover" }
      ]
    },
    "/boards": {
      eyebrow: "Caller home base",
      title: "Boards are the long-form center of gravity",
      body: "Use boards for persistent discussion, unread scanning, and threaded replies. Pair this page with mail for private follow-up and radar for what changed.",
      actions: [
        { label: "Today", href: "/today" },
        { label: "Mail", href: "/mail" },
        { label: "Radar", href: "/radar" },
        { label: "Bulletins", href: "/bulletins" }
      ]
    },
    "/events": {
      eyebrow: "Scheduled return hooks",
      title: "Events turn the board into a place with a rhythm",
      body: "Use the calendar to make activity concrete: tournaments, nets, content drops, and social sessions should all have a time and a path to join.",
      actions: [
        { label: "Today", href: "/today" },
        { label: "Tournaments", href: "/tournaments" },
        { label: "Clubhouse", href: "/clubhouse" },
        { label: "Boards", href: "/boards" }
      ]
    },
    "/tournaments": {
      eyebrow: "Competitive layer",
      title: "Tournament Center keeps the bracket nights visible",
      body: "Use this route when you want ladders, score nights, and door competitions separated from the broader social calendar.",
      actions: [
        { label: "Events", href: "/events" },
        { label: "Scores", href: "/scores" },
        { label: "Doors", href: "/doors" }
      ]
    },
    "/chat": {
      eyebrow: "Shared live chat",
      title: "Web chat and IRC are the same conversation layer",
      body: "Use this page for quick live interaction. The default room is #lobby, and IRC users see the same channel state.",
      actions: [
        { label: "Clubhouse", href: "/clubhouse" },
        { label: "Help", href: "/help" },
        { label: "Status", href: "/status" }
      ]
    },
    "/doors": {
      eyebrow: "Games and stickiness",
      title: "Doors keep callers coming back",
      body: "Use favorites, recommendations, and score links to turn the door list into a daily destination instead of a dead catalog.",
      actions: [
        { label: "Scores", href: "/scores" },
        { label: "Clubhouse", href: "/clubhouse" },
        { label: "Boards", href: "/boards" }
      ]
    },
    "/status": {
      eyebrow: "Health snapshot",
      title: "Use status as the fast confidence check",
      body: "This is the quick answer to whether the board looks healthy. For sysop detail, follow through to the WFC dashboard and setup pages.",
      actions: [
        { label: "Admin system", href: "/admin/system" },
        { label: "Admin setup", href: "/admin/setup" },
        { label: "Launch center", href: "/admin/launch" },
        { label: "Help", href: "/help" }
      ]
    },
    "/mail": {
      eyebrow: "Private conversation",
      title: "Use mail for direct follow-up",
      body: "Boards are public, mail is direct. This is the right place for operator feedback, replies, and caller-to-caller private messages.",
      actions: [
        { label: "Boards", href: "/boards" },
        { label: "Directory", href: "/directory" },
        { label: "Help", href: "/help" }
      ]
    },
    "/radar": {
      eyebrow: "Mission control",
      title: "Radar shows what changed since the last call",
      body: "Use this view when you want a single-screen snapshot of pulse, callers, recommended doors, and recent activity.",
      actions: [
        { label: "Boards", href: "/boards" },
        { label: "Chat", href: "/chat" },
        { label: "Clubhouse", href: "/clubhouse" }
      ]
    },
    "/clubhouse": {
      eyebrow: "Social layer",
      title: "Clubhouse is where the board feels alive",
      body: "Use one-liners, BBS exchange, and social presence here to keep momentum between longer board posts.",
      actions: [
        { label: "Chat", href: "/chat" },
        { label: "Bulletins", href: "/bulletins" },
        { label: "Directory", href: "/directory" }
      ]
    },
    "/admin/ops": {
      eyebrow: "Operator triage",
      title: "Ops Center consolidates the decision surface",
      body: "Use this page when you need sessions, audits, errors, and launch health in one place before deciding what action is justified.",
      actions: [
        { label: "Launch center", href: "/admin/launch" },
        { label: "Events admin", href: "/admin/events" },
        { label: "System", href: "/admin/system" },
        { label: "Audit", href: "/admin/audit" }
      ]
    },
    "/admin/events": {
      eyebrow: "Retention operations",
      title: "Schedule the board like a real product",
      body: "This page exists so recurring reasons to return are managed deliberately instead of getting buried in one-off announcements.",
      actions: [
        { label: "Public calendar", href: "/events" },
        { label: "Today", href: "/today" },
        { label: "Ops center", href: "/admin/ops" }
      ]
    }
  };
  function primerForPath(pathname) {
    if (primerRegistry[pathname]) return primerRegistry[pathname];
    if (pathname.startsWith("/admin/launch")) return primerRegistry["/admin/launch"];
    if (pathname.startsWith("/admin/setup")) return primerRegistry["/admin/setup"];
    if (pathname.startsWith("/admin/config")) return primerRegistry["/admin/config"];
    if (pathname.startsWith("/admin/ops")) return primerRegistry["/admin/ops"];
    if (pathname.startsWith("/admin/events")) return primerRegistry["/admin/events"];
    return null;
  }
  const navLinks = [];
  const navSeen = new Set();
  Array.from(document.querySelectorAll("a[href]")).forEach((anchor) => {
    const href = anchor.getAttribute("href");
    if (!href || href[0] !== "/" || href.startsWith("/chat/") || href.startsWith("/mail/inbound")) return;
    const key = href + "|" + anchor.textContent.trim().toLowerCase();
    if (navSeen.has(key)) return;
    navSeen.add(key);
    navLinks.push({
      href: href,
      label: anchor.textContent.trim() || href,
      meta: (anchor.closest("p") ? "nav" : "page")
    });
  });

  try {
    const recentKey = "wolfbbsRecentPages";
    const recent = JSON.parse(localStorage.getItem(recentKey) || "[]").filter((item) => item && item.href);
    const next = [{href: currentPath, label: title}].concat(recent.filter((item) => item.href !== currentPath)).slice(0, 6);
    localStorage.setItem(recentKey, JSON.stringify(next));
    if (next.length > 1) {
      const rail = document.createElement("div");
      rail.className = "wolfbbs-recent-rail";
      next.slice(1).forEach((item) => {
        const a = document.createElement("a");
        a.href = item.href;
        a.textContent = item.label;
        rail.appendChild(a);
      });
      const navRow = document.querySelector("p.wolfbbs-nav-row");
      const hero = document.querySelector(".wolfbbs-page-hero");
      const shellMain = document.querySelector("main.wolfbbs-main");
      if (navRow && navRow.parentNode) {
        navRow.parentNode.insertBefore(rail, navRow.nextSibling);
      } else if (hero && hero.parentNode) {
        hero.parentNode.insertBefore(rail, hero.nextSibling);
      } else if (shellMain && shellMain.parentNode) {
        shellMain.parentNode.insertBefore(rail, shellMain);
      }
    }
  } catch (_) {}
  renderFavoritesRail();

  document.querySelectorAll('table').forEach((table) => {
    if (table.parentElement && table.parentElement.classList.contains('wolfbbs-table-wrap')) return;
    const wrap = document.createElement('div');
    wrap.className = 'wolfbbs-table-wrap';
    table.parentNode.insertBefore(wrap, table);
    wrap.appendChild(table);
  });

  document.querySelectorAll('[data-wolfbbs-flash]').forEach((banner) => {
    if (banner.querySelector('.wolfbbs-banner-close')) return;
    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'wolfbbs-banner-close';
    close.textContent = 'Dismiss';
    close.addEventListener('click', () => banner.remove());
    banner.appendChild(close);
  });

  function copyText(text) {
    const value = String(text || "");
    if (!value) return Promise.reject(new Error("empty"));
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(value);
    }
    const area = document.createElement("textarea");
    area.value = value;
    area.setAttribute("readonly", "readonly");
    area.style.position = "absolute";
    area.style.left = "-9999px";
    document.body.appendChild(area);
    area.select();
    try {
      document.execCommand("copy");
      document.body.removeChild(area);
      return Promise.resolve();
    } catch (err) {
      document.body.removeChild(area);
      return Promise.reject(err);
    }
  }

  document.addEventListener('click', (event) => {
    const trigger = event.target.closest('[data-copy-text]');
    if (!trigger) return;
    event.preventDefault();
    const text = trigger.getAttribute('data-copy-text') || '';
    const original = trigger.textContent;
    copyText(text).then(() => {
      trigger.textContent = 'Copied';
      showToast("Copied to clipboard", "ok");
      window.setTimeout(() => {
        trigger.textContent = original;
      }, 1200);
    }).catch(() => {
      trigger.textContent = 'Copy failed';
      showToast("Copy failed", "error");
      window.setTimeout(() => {
        trigger.textContent = original;
      }, 1200);
    });
  });

  document.querySelectorAll('pre').forEach((block) => {
    if (block.closest('.wolfbbs-command-block')) return;
    const wrapper = document.createElement('div');
    wrapper.className = 'wolfbbs-command-block';
    block.parentNode.insertBefore(wrapper, block);
    wrapper.appendChild(block);
    const copy = document.createElement('button');
    copy.type = 'button';
    copy.className = 'wolfbbs-copy-button';
    copy.textContent = 'Copy';
    copy.setAttribute('data-copy-text', block.textContent || '');
    wrapper.appendChild(copy);
  });

  document.querySelectorAll('form[data-filter-form]').forEach((form) => {
    const controls = Array.from(form.querySelectorAll('input[name], select[name]')).filter((control) => {
      const type = (control.getAttribute('type') || '').toLowerCase();
      if (type === 'hidden' || type === 'submit' || type === 'button') return false;
      if (control.disabled) return false;
      return true;
    });
    const active = controls.map((control) => {
      const value = (control.value || '').trim();
      if (!value) return null;
      const label = control.getAttribute('data-filter-label') || control.name.replace(/_/g, ' ');
      return { label, value };
    }).filter(Boolean);
    if (!active.length) return;
    const summary = document.createElement('div');
    summary.className = 'wolfbbs-active-filters';
    active.forEach((item) => {
      const chip = document.createElement('span');
      chip.className = 'wolfbbs-filter-chip';
      chip.textContent = item.label + ': ' + item.value;
      summary.appendChild(chip);
    });
    const reset = document.createElement('a');
    reset.className = 'wolfbbs-filter-reset';
    reset.href = form.getAttribute('data-filter-reset') || form.getAttribute('action') || location.pathname;
    reset.textContent = 'Reset filters';
    summary.appendChild(reset);
    form.insertAdjacentElement('afterend', summary);
  });

  const dirtyForms = new Set();
  const baseDocumentTitle = document.title || "WolfBBS";
  function refreshDirtyTitle() {
    if (dirtyForms.size > 0) {
      if (!document.title.startsWith("● ")) {
        document.title = "● " + baseDocumentTitle;
      }
      return;
    }
    document.title = baseDocumentTitle;
  }
  function draftFields(form) {
    return Array.from(form.querySelectorAll('textarea[name], input[name], select[name]')).filter((field) => {
      const type = (field.getAttribute('type') || '').toLowerCase();
      if (type === 'hidden' || type === 'submit' || type === 'button' || type === 'checkbox' || type === 'radio' || type === 'password') return false;
      return !field.disabled;
    });
  }

  document.querySelectorAll('form[data-draft-key]').forEach((form) => {
    const key = 'wolfbbs:draft:' + location.pathname + ':' + form.getAttribute('data-draft-key');
    const fields = draftFields(form);
    if (!fields.length) return;
    const note = document.createElement('div');
    note.className = 'wolfbbs-form-note';
    note.innerHTML = '<strong>Drafts:</strong> <span>Saved locally in this browser while you type.</span>';
    const status = document.createElement('span');
    status.className = 'wolfbbs-form-status';
    status.textContent = 'Draft idle';
    note.appendChild(status);
    const discard = document.createElement('button');
    discard.type = 'button';
    discard.className = 'wolfbbs-form-secondary';
    discard.textContent = 'Discard draft';
    note.appendChild(discard);
    form.insertBefore(note, form.firstChild);

    let dirty = false;
    let saveTimer = null;

    function formatSavedAt(raw) {
      try {
        const when = new Date(raw);
        if (Number.isNaN(when.getTime())) return '';
        return when.toLocaleString();
      } catch (_) {
        return '';
      }
    }

    function markClean(message) {
      dirty = false;
      dirtyForms.delete(form);
      refreshDirtyTitle();
      status.classList.remove('dirty', 'error');
      status.textContent = message || 'Draft idle';
    }

    function markDirty(message) {
      dirty = true;
      dirtyForms.add(form);
      refreshDirtyTitle();
      status.classList.remove('error');
      status.classList.add('dirty');
      status.textContent = message || 'Unsaved draft changes';
    }

    function serialize() {
      const payload = {};
      fields.forEach((field) => {
        payload[field.name] = field.value || '';
      });
      return payload;
    }

    function saveDraft() {
      try {
        const savedAt = new Date().toISOString();
        localStorage.setItem(key, JSON.stringify({
          savedAt: new Date().toISOString(),
          values: serialize()
        }));
        const label = formatSavedAt(savedAt);
        markClean(label ? 'Draft saved ' + label : 'Draft saved locally');
      } catch (_) {
        status.classList.remove('dirty');
        status.classList.add('error');
        status.textContent = 'Draft storage unavailable';
      }
    }

    try {
      const raw = localStorage.getItem(key);
      if (raw) {
        const saved = JSON.parse(raw);
        if (saved && saved.values) {
          fields.forEach((field) => {
            if (!field.value && saved.values[field.name]) {
              field.value = saved.values[field.name];
            }
          });
          const label = formatSavedAt(saved.savedAt);
          status.textContent = label ? 'Draft restored from ' + label : 'Draft restored';
        }
      }
    } catch (_) {}

    fields.forEach((field) => {
      field.addEventListener('input', () => {
        markDirty();
        if (saveTimer) window.clearTimeout(saveTimer);
        saveTimer = window.setTimeout(saveDraft, 400);
      });
      field.addEventListener('change', () => {
        markDirty();
        if (saveTimer) window.clearTimeout(saveTimer);
        saveTimer = window.setTimeout(saveDraft, 250);
      });
    });

    discard.addEventListener('click', () => {
      try {
        localStorage.removeItem(key);
      } catch (_) {}
      fields.forEach((field) => {
        field.value = '';
      });
      markClean('Draft discarded');
    });

    form.addEventListener('submit', (event) => {
      if (event.defaultPrevented) return;
      try {
        localStorage.removeItem(key);
      } catch (_) {}
      markClean('Submitting');
    });
  });

  function escapeHTML(value) {
    return String(value || '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function composePreviewHTML(value) {
    const text = String(value || '').trim();
    if (!text) {
      return '<p class="wolfbbs-muted">Nothing to preview yet.</p>';
    }
    return text.split(/\n{2,}/).map((block) => {
      return '<p>' + escapeHTML(block).replace(/\n/g, '<br>') + '</p>';
    }).join('');
  }

  const handleSuggestCache = new Map();

  async function fetchHandleSuggestions(query, mode) {
    const q = String(query || '').trim().toLowerCase();
    const cacheKey = (mode || 'mention') + ':' + q;
    if (!q) return [];
    if (handleSuggestCache.has(cacheKey)) {
      return handleSuggestCache.get(cacheKey);
    }
    try {
      const res = await fetch('/handles/suggest?q=' + encodeURIComponent(q) + '&mode=' + encodeURIComponent(mode || 'mention'), {
        credentials: 'same-origin'
      });
      if (!res.ok) return [];
      const payload = await res.json();
      const items = Array.isArray(payload.items) ? payload.items : [];
      handleSuggestCache.set(cacheKey, items);
      return items;
    } catch (_) {
      return [];
    }
  }

  function attachHandleAssist(control, options) {
    if (!control || control.dataset.handleAssistBound === '1') return;
    control.dataset.handleAssistBound = '1';
    const mode = options && options.mode ? options.mode : 'mention';
    const host = document.createElement('div');
    host.className = 'wolfbbs-handle-assist';
    control.insertAdjacentElement('afterend', host);
    let items = [];
    let activeIndex = 0;

    function closeAssist() {
      host.classList.remove('active');
      host.innerHTML = '';
      items = [];
      activeIndex = 0;
    }

    function mentionMatch() {
      const text = control.value || '';
      const caret = typeof control.selectionStart === 'number' ? control.selectionStart : text.length;
      const before = text.slice(0, caret);
      const match = before.match(/(^|\s)@([a-z0-9._-]{1,32})$/i);
      if (!match) return null;
      return {
        query: match[2],
        start: caret - match[2].length - 1,
        end: caret
      };
    }

    function currentQuery() {
      if (mode === 'recipient') {
        const query = String(control.value || '').trim();
        if (!query || query.includes('@')) return null;
        return { query: query, start: 0, end: control.value.length };
      }
      return mentionMatch();
    }

    function applySelection(handle) {
      const match = currentQuery();
      if (!match) return;
      const value = control.value || '';
      if (mode === 'recipient') {
        control.value = handle;
      } else {
        control.value = value.slice(0, match.start) + '@' + handle + ' ' + value.slice(match.end);
        const caret = match.start + handle.length + 2;
        if (typeof control.setSelectionRange === 'function') {
          control.setSelectionRange(caret, caret);
        }
      }
      control.dispatchEvent(new Event('input', { bubbles: true }));
      closeAssist();
      control.focus();
    }

    function renderAssist() {
      host.innerHTML = '';
      if (!items.length) {
        closeAssist();
        return;
      }
      host.classList.add('active');
      items.forEach((item, index) => {
        const button = document.createElement('button');
        button.type = 'button';
        button.textContent = mode === 'recipient' ? item : '@' + item;
        if (index === activeIndex) button.classList.add('active');
        button.addEventListener('mousedown', (event) => {
          event.preventDefault();
          applySelection(item);
        });
        host.appendChild(button);
      });
    }

    async function refreshAssist() {
      const match = currentQuery();
      if (!match || !match.query) {
        closeAssist();
        return;
      }
      items = await fetchHandleSuggestions(match.query, mode);
      activeIndex = 0;
      renderAssist();
    }

    control.addEventListener('input', refreshAssist);
    control.addEventListener('click', refreshAssist);
    control.addEventListener('blur', () => {
      window.setTimeout(closeAssist, 120);
    });
    control.addEventListener('keydown', (event) => {
      if (!items.length) return;
      if (event.key === 'ArrowDown') {
        event.preventDefault();
        activeIndex = (activeIndex + 1) % items.length;
        renderAssist();
        return;
      }
      if (event.key === 'ArrowUp') {
        event.preventDefault();
        activeIndex = (activeIndex - 1 + items.length) % items.length;
        renderAssist();
        return;
      }
      if ((event.key === 'Tab' || event.key === 'Enter') && host.classList.contains('active')) {
        event.preventDefault();
        applySelection(items[activeIndex]);
        return;
      }
      if (event.key === 'Escape') {
        closeAssist();
      }
    });
  }

  document.querySelectorAll('form[data-rich-compose]').forEach((form) => {
    if (form.querySelector('.wolfbbs-compose-toolbar')) return;
    const textarea = form.querySelector('textarea[name="body"]');
    if (!textarea) return;
    attachHandleAssist(textarea, { mode: 'mention' });
    const recipient = form.querySelector('input[name="to"]');
    if (recipient) {
      attachHandleAssist(recipient, { mode: 'recipient' });
    }
    const layoutKey = 'wolfbbs:compose:layout:' + location.pathname + ':' + (form.getAttribute('data-draft-key') || form.getAttribute('data-rich-compose') || textarea.name || 'body');

    const shell = document.createElement('div');
    shell.className = 'wolfbbs-compose-shell';
    const label = textarea.parentElement && textarea.parentElement.tagName.toLowerCase() === 'label' ? textarea.parentElement : null;
    if (label && label.parentNode) {
      if (!textarea.id) {
        textarea.id = 'wolfbbs-compose-' + Math.random().toString(36).slice(2, 10);
      }
      label.setAttribute('for', textarea.id);
      label.parentNode.insertBefore(shell, label.nextSibling);
    } else {
      textarea.parentNode.insertBefore(shell, textarea);
    }
    shell.appendChild(textarea);

    const toolbar = document.createElement('div');
    toolbar.className = 'wolfbbs-compose-toolbar';
    shell.insertBefore(toolbar, textarea);

    const previewButton = document.createElement('button');
    previewButton.type = 'button';
    previewButton.textContent = 'Preview';
    toolbar.appendChild(previewButton);

    const focusButton = document.createElement('button');
    focusButton.type = 'button';
    focusButton.textContent = 'Focus Mode';
    toolbar.appendChild(focusButton);

    const fullscreenButton = document.createElement('button');
    fullscreenButton.type = 'button';
    fullscreenButton.textContent = 'Fullscreen';
    toolbar.appendChild(fullscreenButton);

    const helpButton = document.createElement('button');
    helpButton.type = 'button';
    helpButton.textContent = 'Shortcuts';
    toolbar.appendChild(helpButton);

    const quoteSource = textarea.getAttribute('data-compose-quote') || form.getAttribute('data-compose-quote') || '';
    if (quoteSource.trim()) {
      const quoteButton = document.createElement('button');
      quoteButton.type = 'button';
      quoteButton.textContent = 'Quote Context';
      toolbar.appendChild(quoteButton);
      quoteButton.addEventListener('click', () => {
        const quoteText = quoteSource.trim();
        if (!quoteText) return;
        const separator = textarea.value.trim() ? '\n\n' : '';
        textarea.value = (textarea.value || '') + separator + quoteText;
        textarea.dispatchEvent(new Event('input', { bubbles: true }));
        textarea.focus();
      });
    }

    const signatureValue = (form.getAttribute('data-compose-signature') || '').trim();
    if (signatureValue) {
      const signatureButton = document.createElement('button');
      signatureButton.type = 'button';
      signatureButton.textContent = 'Insert Signature';
      toolbar.appendChild(signatureButton);
      signatureButton.addEventListener('click', () => {
        const signature = '\n\n-- \n' + signatureValue;
        if ((textarea.value || '').includes(signature.trim())) return;
        textarea.value = (textarea.value || '') + signature;
        textarea.dispatchEvent(new Event('input', { bubbles: true }));
        textarea.focus();
      });
    }

    const meta = document.createElement('span');
    meta.className = 'wolfbbs-compose-meta';
    toolbar.appendChild(meta);

    const preview = document.createElement('div');
    preview.className = 'wolfbbs-compose-preview';
    shell.appendChild(preview);

    const help = document.createElement('div');
    help.className = 'wolfbbs-compose-help';
    help.innerHTML = '<strong>Composer Shortcuts</strong><ul><li>Ctrl/Cmd+Enter: send</li><li>Ctrl/Cmd+Shift+P: preview</li><li>Ctrl/Cmd+Shift+F: fullscreen</li><li>Esc: exit focus or fullscreen</li></ul>';
    shell.appendChild(help);

    function persistLayout() {
      try {
        localStorage.setItem(layoutKey, JSON.stringify({
          preview: preview.classList.contains('active'),
          focus: form.classList.contains('wolfbbs-compose-focus'),
          fullscreen: form.classList.contains('wolfbbs-compose-fullscreen')
        }));
      } catch (_) {}
    }

    function syncMeta() {
      const text = textarea.value || '';
      const words = text.trim() ? text.trim().split(/\s+/).length : 0;
      meta.textContent = words + ' words / ' + text.length + ' chars / Ctrl+Enter sends';
    }

    function syncPreview() {
      preview.innerHTML = composePreviewHTML(textarea.value);
      syncMeta();
    }

    function togglePreview(force) {
      if (typeof force === 'boolean') {
        preview.classList.toggle('active', force);
      } else {
        preview.classList.toggle('active');
      }
      const open = preview.classList.contains('active');
      previewButton.textContent = open ? 'Hide Preview' : 'Preview';
      if (open) {
        syncPreview();
      }
      persistLayout();
    }

    function toggleFocus(force) {
      if (typeof force === 'boolean') {
        form.classList.toggle('wolfbbs-compose-focus', force);
      } else {
        form.classList.toggle('wolfbbs-compose-focus');
      }
      const open = form.classList.contains('wolfbbs-compose-focus');
      focusButton.textContent = open ? 'Exit Focus' : 'Focus Mode';
      if (open) {
        textarea.focus();
      }
      persistLayout();
    }

    function toggleFullscreen(force) {
      if (typeof force === 'boolean') {
        form.classList.toggle('wolfbbs-compose-fullscreen', force);
      } else {
        form.classList.toggle('wolfbbs-compose-fullscreen');
      }
      const open = form.classList.contains('wolfbbs-compose-fullscreen');
      document.body.classList.toggle('wolfbbs-compose-fullscreen-open', open);
      fullscreenButton.textContent = open ? 'Exit Fullscreen' : 'Fullscreen';
      if (open) {
        textarea.focus();
      }
      persistLayout();
    }

    function toggleHelp(force) {
      if (typeof force === 'boolean') {
        help.classList.toggle('active', force);
      } else {
        help.classList.toggle('active');
      }
      helpButton.textContent = help.classList.contains('active') ? 'Hide Shortcuts' : 'Shortcuts';
    }

    previewButton.addEventListener('click', () => {
      togglePreview();
    });

    focusButton.addEventListener('click', () => {
      toggleFocus();
    });

    fullscreenButton.addEventListener('click', () => {
      toggleFullscreen();
    });

    helpButton.addEventListener('click', () => {
      toggleHelp();
    });

    textarea.addEventListener('input', syncPreview);
    textarea.addEventListener('change', syncPreview);
    textarea.addEventListener('keydown', (event) => {
      const modifier = event.ctrlKey || event.metaKey;
      if (modifier && event.key === 'Enter') {
        event.preventDefault();
        if (typeof form.requestSubmit === 'function') {
          form.requestSubmit();
        } else {
          form.submit();
        }
        return;
      }
      if (modifier && event.shiftKey && String(event.key).toLowerCase() === 'p') {
        event.preventDefault();
        togglePreview();
        return;
      }
      if (modifier && event.shiftKey && String(event.key).toLowerCase() === 'f') {
        event.preventDefault();
        toggleFullscreen();
        return;
      }
      if (event.key === 'Escape' && help.classList.contains('active')) {
        event.preventDefault();
        toggleHelp(false);
        return;
      }
      if (event.key === 'Escape' && form.classList.contains('wolfbbs-compose-fullscreen')) {
        event.preventDefault();
        toggleFullscreen(false);
        return;
      }
      if (event.key === 'Escape' && form.classList.contains('wolfbbs-compose-focus')) {
        event.preventDefault();
        toggleFocus(false);
      }
    });
    try {
      const raw = localStorage.getItem(layoutKey);
      if (raw) {
        const saved = JSON.parse(raw);
        if (saved && saved.preview) togglePreview(true);
        if (saved && saved.focus) toggleFocus(true);
        if (saved && saved.fullscreen) toggleFullscreen(true);
      }
    } catch (_) {}
    syncPreview();
  });

  window.addEventListener('beforeunload', (event) => {
    if (!dirtyForms.size) return;
    event.preventDefault();
    event.returnValue = '';
  });

  const chatMessageInput = document.getElementById('message');
  if (chatMessageInput) {
    attachHandleAssist(chatMessageInput, { mode: 'mention' });
  }

  function inferredConfirmMessage(form, submitter) {
    const actionValue = submitter && submitter.getAttribute('value') ? submitter.getAttribute('value') : '';
    const hiddenAction = form.querySelector('input[name="action"]');
    const hiddenActionValue = hiddenAction && hiddenAction.value ? hiddenAction.value.toLowerCase() : '';
    const formAction = (form.getAttribute('action') || location.pathname || '').toLowerCase();
    const adminScoped = formAction.indexOf('/admin') === 0 || formAction.includes('/admin/');
    const text = [
      submitter && submitter.textContent,
      actionValue,
      hiddenActionValue,
      form.getAttribute('data-confirm')
    ].join(' ').toLowerCase();
    if (!text.trim()) return '';
    if (submitter && submitter.getAttribute('data-confirm')) return submitter.getAttribute('data-confirm');
    if (text.includes('delete') && adminScoped) return 'Delete this item? This cannot be undone.';
    if (text.includes('reset password') && (adminScoped || hiddenActionValue === 'reset')) return 'Reset this password and replace the current one?';
    if (text.includes('ban') && adminScoped) return 'Ban this account now?';
    if (text.includes('disable') && adminScoped) return 'Disable this item now?';
    if (text.includes('remove') && adminScoped) return 'Remove this item now?';
    if (text.includes('lock') && adminScoped) return 'Apply this lock now?';
    return '';
  }

  document.addEventListener('submit', (event) => {
    const form = event.target;
    if (!form || form.tagName.toLowerCase() !== 'form') return;
    const requiredInvalid = Array.from(form.querySelectorAll('[required]')).filter((node) => typeof node.checkValidity === 'function' && !node.checkValidity());
    if (requiredInvalid.length) {
      event.preventDefault();
      const first = requiredInvalid[0];
      if (typeof first.focus === 'function') first.focus();
      if (typeof first.reportValidity === 'function') first.reportValidity();
      showToast('Please fill required fields before submitting.', 'error');
      trackTelemetry('form:required-missing');
      return;
    }
    if (typeof form.checkValidity === 'function' && !form.checkValidity()) {
      event.preventDefault();
      const firstInvalid = form.querySelector(':invalid');
      if (firstInvalid && typeof firstInvalid.focus === 'function') firstInvalid.focus();
      if (firstInvalid && typeof firstInvalid.reportValidity === 'function') firstInvalid.reportValidity();
      showToast('Please fix invalid form fields.', 'error');
      trackTelemetry('form:invalid');
      return;
    }
    const submitter = event.submitter || form.querySelector('button[type="submit"], input[type="submit"]');
    const confirmMessage = inferredConfirmMessage(form, submitter);
    if (confirmMessage && !window.confirm(confirmMessage)) {
      event.preventDefault();
      return;
    }
    if (form.hasAttribute('data-no-auto-busy')) return;
    const method = (form.getAttribute('method') || 'get').toLowerCase();
    if (method !== 'post') return;
    const buttons = Array.from(form.querySelectorAll('button[type="submit"], input[type="submit"]'));
    buttons.forEach((button) => {
      if (button === submitter) {
        button.setAttribute('data-original-label', button.textContent || button.value || '');
        if (button.tagName.toLowerCase() === 'input') {
          button.value = 'Working...';
        } else {
          button.textContent = 'Working...';
        }
      }
      button.disabled = true;
      button.classList.add('wolfbbs-submit-busy');
    });
    trackTelemetry('form:submit');
  }, true);

  const primer = primerForPath(location.pathname);
  if (primer && shouldMountPrimerSurface(location.pathname)) {
    const panel = document.createElement("section");
    panel.className = "wolfbbs-guide-strip";
    const main = document.createElement("div");
    const heading = document.createElement("strong");
    const headingPrefix = primer.eyebrow ? primer.eyebrow + " \u2022 " : "";
    heading.textContent = headingPrefix + (primer.title || title);
    main.appendChild(heading);
    if (primer.body) {
      const body = document.createElement("p");
      const trimmed = String(primer.body).trim();
      body.textContent = trimmed.length > 190 ? trimmed.slice(0, 187) + "..." : trimmed;
      main.appendChild(body);
    }
    panel.appendChild(main);
    if (Array.isArray(primer.actions) && primer.actions.length) {
      const actions = document.createElement("div");
      actions.className = "wolfbbs-guide-actions";
      primer.actions.slice(0, 4).forEach((item) => {
        const link = document.createElement("a");
        link.href = item.href;
        link.textContent = item.label;
        actions.appendChild(link);
      });
      panel.appendChild(actions);
    }
    if (Array.isArray(primer.bullets) && primer.bullets.length) {
      const details = document.createElement("details");
      details.className = "wolfbbs-guide-details";
      const summary = document.createElement("summary");
      summary.textContent = currentRoute.startsWith("/admin") ? "Operator notes" : "Usage notes";
      details.appendChild(summary);
      const list = document.createElement("ul");
      primer.bullets.slice(0, 4).forEach((item) => {
        const li = document.createElement("li");
        li.textContent = item;
        list.appendChild(li);
      });
      details.appendChild(list);
      panel.appendChild(details);
    }
    const recentRail = document.querySelector(".wolfbbs-recent-rail");
    const shellMain = document.querySelector("main.wolfbbs-main");
    const navRow = document.querySelector("p.wolfbbs-nav-row");
    if (recentRail && recentRail.parentNode) {
      recentRail.parentNode.insertBefore(panel, recentRail.nextSibling);
    } else if (shellMain && shellMain.parentNode) {
      shellMain.parentNode.insertBefore(panel, shellMain);
    } else if (navRow && navRow.parentNode) {
      navRow.parentNode.insertBefore(panel, navRow.nextSibling);
    }
  }
  mountGuideMinimizeControl();

  function mountSectionNavCompaction(nav, sectionHeadings) {
    if (!nav || nav.dataset.wolfbbsSectionNavCompact === "1" || !currentRouteProfile.compactSectionNav) return;
    nav.dataset.wolfbbsSectionNavCompact = "1";
    const storageKey = "wolfbbs:ui:section-nav:v1:" + currentRoute;
    const saved = Object.assign({ expanded: false }, readJSON(storageKey, {}));
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "wolfbbs-section-toggle wolfbbs-section-nav-more";
    nav.insertBefore(toggle, nav.querySelector('a[href^="#"]') || null);

    function sync() {
      const activeHash = location.hash || "";
      const links = Array.from(nav.querySelectorAll('a[href^="#"]'));
      links.forEach((link, index) => {
        const keepVisible = saved.expanded || index < 5 || link.classList.contains("wolfbbs-nav-active") || link.getAttribute("href") === activeHash;
        link.classList.toggle("wolfbbs-section-nav-link-hidden", !keepVisible);
      });
      const hiddenCount = links.filter((link) => link.classList.contains("wolfbbs-section-nav-link-hidden")).length;
      nav.classList.toggle("wolfbbs-section-nav-compact", !saved.expanded);
      toggle.textContent = saved.expanded ? "Show less" : hiddenCount ? "More sections (" + hiddenCount + ")" : "Sections";
      writeJSON(storageKey, saved);
    }

    toggle.addEventListener("click", () => {
      saved.expanded = !saved.expanded;
      sync();
      trackTelemetry("sections:compact-toggle");
    });
    window.addEventListener("hashchange", sync);
    nav.wolfbbsCompactSync = sync;
    sync();
  }

  const headings = Array.from(document.querySelectorAll("h2, h3"));
  const sectionHeadings = headings
    .filter((heading) => heading.tagName === "H2")
    .filter((heading) => {
      const text = headingBaseLabel(heading);
      if (!text || text.length < 3) return false;
      if (heading.closest("table")) return false;
      return true;
    })
    .slice(0, 8);
  sectionHeadings.forEach((heading, idx) => {
    if (!heading.id) heading.id = "wolfbbs-section-" + idx;
    if (!heading.dataset.navLabel) heading.dataset.navLabel = headingBaseLabel(heading);
  });
  const legacyNavLinkCount = document.querySelectorAll("p.wolfbbs-nav-row a").length;
  const showSectionNav = sectionHeadings.length >= 3 && legacyNavLinkCount < 4 && navRows.length <= 1;
  if (showSectionNav) {
    const nav = document.createElement("nav");
    nav.className = "wolfbbs-section-nav wolfbbs-section-nav-minimal";
    const label = document.createElement("span");
    label.className = "wolfbbs-section-nav-label";
    label.textContent = "Jump to";
    nav.appendChild(label);
    const linkMap = new Map();
    sectionHeadings.forEach((heading, idx) => {
      if (!heading.id) heading.id = "wolfbbs-section-" + idx;
      const link = document.createElement("a");
      link.href = "#" + heading.id;
      link.textContent = heading.dataset.navLabel || headingBaseLabel(heading);
      nav.appendChild(link);
      linkMap.set(heading.id, link);
    });
    const shellMain = document.querySelector("main.wolfbbs-main");
    if (shellMain && shellMain.parentNode) {
      shellMain.parentNode.insertBefore(nav, shellMain);
    } else {
      const h1 = document.querySelector("h1");
      if (h1 && h1.parentNode) {
        h1.parentNode.insertBefore(nav, h1.nextSibling ? h1.nextSibling.nextSibling : null);
      }
    }
    mountSectionNavCompaction(nav, sectionHeadings);
    if ("IntersectionObserver" in window) {
      const observer = new IntersectionObserver((entries) => {
        entries.forEach((entry) => {
          const link = linkMap.get(entry.target.id);
          if (!link || !entry.isIntersecting) return;
          nav.querySelectorAll("a").forEach((item) => item.classList.remove("wolfbbs-nav-active"));
          link.classList.add("wolfbbs-nav-active");
          if (typeof nav.wolfbbsCompactSync === "function") nav.wolfbbsCompactSync();
        });
      }, { rootMargin: "-38% 0px -52% 0px", threshold: 0.05 });
      sectionHeadings.forEach((heading) => observer.observe(heading));
    }
    document.addEventListener("keydown", (event) => {
      const tag = event.target && event.target.tagName ? event.target.tagName.toLowerCase() : "";
      const editing = tag === "input" || tag === "textarea" || tag === "select" || (event.target && event.target.isContentEditable);
      if (editing || !event.altKey) return;
      const currentID = location.hash ? location.hash.replace("#", "") : "";
      let idx = sectionHeadings.findIndex((heading) => heading.id === currentID);
      if (idx < 0) idx = 0;
      if (String(event.key).toLowerCase() === "j") {
        event.preventDefault();
        idx = Math.min(sectionHeadings.length - 1, idx + 1);
        location.hash = "#" + sectionHeadings[idx].id;
        sectionHeadings[idx].scrollIntoView({ behavior: "smooth", block: "start" });
        trackTelemetry("sections:key-next");
      } else if (String(event.key).toLowerCase() === "k") {
        event.preventDefault();
        idx = Math.max(0, idx - 1);
        location.hash = "#" + sectionHeadings[idx].id;
        sectionHeadings[idx].scrollIntoView({ behavior: "smooth", block: "start" });
        trackTelemetry("sections:key-prev");
      }
    });
  }

  function parseNumericValue(raw) {
    const source = String(raw || "").replace(/,/g, "");
    const match = source.match(/-?\d+(?:\.\d+)?/);
    if (!match) return null;
    const value = Number(match[0]);
    if (!Number.isFinite(value)) return null;
    return value;
  }

  function formatCompactNumber(value) {
    const numeric = Number(value || 0);
    if (!Number.isFinite(numeric)) return "0";
    if (Math.abs(numeric) >= 1000) {
      return numeric.toLocaleString(undefined, { maximumFractionDigits: 1 });
    }
    if (Math.abs(numeric) >= 100) {
      return numeric.toFixed(0);
    }
    if (Math.abs(numeric) >= 10) {
      return numeric.toFixed(1);
    }
    return numeric.toFixed(2);
  }

  function average(values) {
    if (!values.length) return 0;
    return values.reduce((acc, value) => acc + value, 0) / values.length;
  }

  const WolfCharts = (() => {
    const registry = [];
    let resizeTimer = null;

    function withCanvas(canvas, draw) {
      if (!canvas) return;
      const rect = canvas.getBoundingClientRect();
      const width = Math.max(140, Math.floor(rect.width || 140));
      const height = Math.max(96, Math.floor(rect.height || 96));
      const dpr = window.devicePixelRatio || 1;
      canvas.width = Math.floor(width * dpr);
      canvas.height = Math.floor(height * dpr);
      const ctx = canvas.getContext("2d");
      if (!ctx) return;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      draw(ctx, width, height);
    }

    function register(drawFn) {
      registry.push(drawFn);
      drawFn();
    }

    function drawLine(canvas, values, opts) {
      const data = Array.isArray(values) ? values.slice() : [];
      const options = opts || {};
      register(() => {
        withCanvas(canvas, (ctx, width, height) => {
          ctx.clearRect(0, 0, width, height);
          if (data.length < 2) return;

          const min = Math.min.apply(null, data);
          const max = Math.max.apply(null, data);
          const span = max - min || 1;
          const left = 10;
          const right = width - 10;
          const top = 10;
          const bottom = height - 12;

          ctx.strokeStyle = "rgba(82,112,146,.25)";
          ctx.lineWidth = 1;
          for (let i = 0; i < 4; i++) {
            const y = top + ((bottom - top) * i) / 3;
            ctx.beginPath();
            ctx.moveTo(left, y);
            ctx.lineTo(right, y);
            ctx.stroke();
          }

          const points = data.map((value, index) => {
            const x = left + ((right - left) * index) / (data.length - 1);
            const y = bottom - ((value - min) / span) * (bottom - top);
            return { x, y };
          });

          const gradient = ctx.createLinearGradient(0, top, 0, bottom);
          gradient.addColorStop(0, options.fillTop || "rgba(16,104,205,.22)");
          gradient.addColorStop(1, "rgba(16,104,205,0)");

          ctx.beginPath();
          ctx.moveTo(points[0].x, bottom);
          points.forEach((point) => ctx.lineTo(point.x, point.y));
          ctx.lineTo(points[points.length - 1].x, bottom);
          ctx.closePath();
          ctx.fillStyle = gradient;
          ctx.fill();

          ctx.beginPath();
          points.forEach((point, index) => {
            if (index === 0) {
              ctx.moveTo(point.x, point.y);
            } else {
              ctx.lineTo(point.x, point.y);
            }
          });
          ctx.strokeStyle = options.stroke || "#0f63cb";
          ctx.lineWidth = 2.2;
          ctx.stroke();

          const end = points[points.length - 1];
          ctx.beginPath();
          ctx.arc(end.x, end.y, 3.2, 0, Math.PI * 2);
          ctx.fillStyle = options.stroke || "#0f63cb";
          ctx.fill();
          ctx.strokeStyle = "#ffffff";
          ctx.lineWidth = 1.3;
          ctx.stroke();
        });
      });
    }

    function drawBars(canvas, values, labels, opts) {
      const data = Array.isArray(values) ? values.slice() : [];
      const names = Array.isArray(labels) ? labels.slice() : [];
      const options = opts || {};
      register(() => {
        withCanvas(canvas, (ctx, width, height) => {
          ctx.clearRect(0, 0, width, height);
          if (!data.length) return;
          const max = Math.max.apply(null, data) || 1;
          const left = 8;
          const right = width - 8;
          const top = 8;
          const bottom = height - 20;
          const slotWidth = (right - left) / data.length;

          for (let i = 0; i < data.length; i++) {
            const value = data[i];
            const barHeight = ((bottom - top) * value) / max;
            const x = left + i * slotWidth + 3;
            const y = bottom - barHeight;
            const w = Math.max(6, slotWidth - 6);
            const radius = 4;

            const gradient = ctx.createLinearGradient(0, y, 0, bottom);
            gradient.addColorStop(0, options.barTop || "#1d78df");
            gradient.addColorStop(1, options.barBottom || "#0f4f95");
            ctx.fillStyle = gradient;
            ctx.beginPath();
            ctx.moveTo(x, bottom);
            ctx.lineTo(x, y + radius);
            ctx.quadraticCurveTo(x, y, x + radius, y);
            ctx.lineTo(x + w - radius, y);
            ctx.quadraticCurveTo(x + w, y, x + w, y + radius);
            ctx.lineTo(x + w, bottom);
            ctx.closePath();
            ctx.fill();

            if (names[i]) {
              ctx.save();
              ctx.fillStyle = "rgba(56,83,111,.86)";
              ctx.font = "10px sans-serif";
              ctx.textAlign = "center";
              ctx.fillText(String(names[i]).slice(0, 6), x + w / 2, height - 6);
              ctx.restore();
            }
          }
        });
      });
    }

    window.addEventListener("resize", () => {
      if (resizeTimer) window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(() => {
        registry.forEach((draw) => draw());
      }, 120);
    });

    return {
      line: drawLine,
      bars: drawBars
    };
  })();
  window.WolfCharts = WolfCharts;

  function inferTableTitle(table) {
    const container = table.closest("article,section,div");
    if (!container) return "Table";
    const heading = container.querySelector("h2,h3");
    if (!heading) return "Table";
    return headingBaseLabel(heading) || "Table";
  }

  function extractSeriesFromTable(table) {
    const rows = Array.from(table.querySelectorAll("tr")).filter((row) => row.querySelectorAll("th,td").length >= 2);
    if (rows.length < 4) return null;
    const headerCells = Array.from(rows[0].querySelectorAll("th,td")).map((cell) => cell.textContent.trim());
    const dataRows = rows.slice(1).map((row) => Array.from(row.querySelectorAll("th,td")).map((cell) => cell.textContent.trim()));
    const colCount = Math.max.apply(null, dataRows.map((row) => row.length).concat(headerCells.length));
    let best = null;

    for (let col = 1; col < colCount; col++) {
      const values = [];
      const labels = [];
      dataRows.forEach((cells, index) => {
        if (cells.length <= col) return;
        const parsed = parseNumericValue(cells[col]);
        if (parsed === null) return;
        values.push(parsed);
        labels.push((cells[0] || "Row " + (index + 1)).slice(0, 18));
      });
      if (values.length < 3) continue;
      const spread = Math.max.apply(null, values) - Math.min.apply(null, values);
      const score = values.length * 10 + spread;
      if (!best || score > best.score) {
        best = {
          score: score,
          values: values,
          labels: labels,
          column: headerCells[col] || "Value",
          title: inferTableTitle(table)
        };
      }
    }
    return best;
  }

  function mountDashboard() {
    const eligibleRoutes = [
      "/today",
      "/status",
      "/radar",
      "/boards",
      "/scores",
      "/tournaments",
      "/digest",
      "/attention",
      "/admin/ops",
      "/admin/events",
      "/admin/system",
      "/admin/analytics",
      "/admin/launch"
    ];
    const eligible = eligibleRoutes.some((prefix) => currentRoute === prefix || currentRoute.indexOf(prefix + "/") === 0);
    if (!eligible) return;

    const tables = Array.from(document.querySelectorAll("table")).filter((table) => table.querySelectorAll("tr").length >= 4);
    const seriesList = tables.map((table) => extractSeriesFromTable(table)).filter(Boolean);
    const kpiRows = Array.from(document.querySelectorAll(".wolfbbs-kpi-card")).map((card) => {
      const strong = card.querySelector("strong");
      const span = card.querySelector("span");
      const value = strong ? parseNumericValue(strong.textContent) : null;
      if (value === null) return null;
      return {
        label: span ? span.textContent.trim() : "KPI",
        value: value
      };
    }).filter(Boolean);

    if (!seriesList.length && kpiRows.length < 2) return;
    const primary = seriesList[0] || null;
    const secondary = seriesList[1] || null;

    const dashboard = document.createElement("section");
    dashboard.className = "wolfbbs-dashboard";

    const trendCard = document.createElement("article");
    trendCard.className = "wolfbbs-dashboard-card";
    const trendTitle = document.createElement("h3");
    trendTitle.textContent = primary ? "Data Trend" : "Activity Trend";
    trendCard.appendChild(trendTitle);
    const trendSub = document.createElement("p");
    trendSub.className = "wolfbbs-dashboard-sub";
    trendSub.textContent = primary ? (primary.title + " | " + primary.column) : "No table trend detected on this route.";
    trendCard.appendChild(trendSub);
    const trendShell = document.createElement("div");
    trendShell.className = "wolfbbs-chart-shell";
    const trendCanvas = document.createElement("canvas");
    trendCanvas.className = "wolfbbs-chart-canvas";
    trendShell.appendChild(trendCanvas);
    trendCard.appendChild(trendShell);
    const trendLegend = document.createElement("div");
    trendLegend.className = "wolfbbs-chart-legend";
    trendCard.appendChild(trendLegend);
    if (primary) {
      const trendValues = primary.values.slice(-24);
      WolfCharts.line(trendCanvas, trendValues, { stroke: "#0f63cb", fillTop: "rgba(16,99,203,.22)" });
      const trendMin = Math.min.apply(null, trendValues);
      const trendMax = Math.max.apply(null, trendValues);
      const trendLast = trendValues[trendValues.length - 1];
      [ "min " + formatCompactNumber(trendMin), "max " + formatCompactNumber(trendMax), "latest " + formatCompactNumber(trendLast) ].forEach((text) => {
        const chip = document.createElement("span");
        chip.textContent = text;
        trendLegend.appendChild(chip);
      });
    } else {
      const chip = document.createElement("span");
      chip.textContent = "No trend source";
      trendLegend.appendChild(chip);
    }

    const distCard = document.createElement("article");
    distCard.className = "wolfbbs-dashboard-card";
    const distTitle = document.createElement("h3");
    distTitle.textContent = primary ? "Top Distribution" : "Secondary Distribution";
    distCard.appendChild(distTitle);
    const distSub = document.createElement("p");
    distSub.className = "wolfbbs-dashboard-sub";
    distSub.textContent = primary ? "Top rows by " + primary.column : "Waiting for tabular numeric data.";
    distCard.appendChild(distSub);
    const distShell = document.createElement("div");
    distShell.className = "wolfbbs-chart-shell";
    const distCanvas = document.createElement("canvas");
    distCanvas.className = "wolfbbs-chart-canvas";
    distShell.appendChild(distCanvas);
    distCard.appendChild(distShell);
    const distLegend = document.createElement("div");
    distLegend.className = "wolfbbs-chart-legend";
    distCard.appendChild(distLegend);
    if (primary) {
      const sorted = primary.values.map((value, index) => {
        return {
          value: value,
          label: primary.labels[index] || ("row" + (index + 1))
        };
      }).sort((a, b) => b.value - a.value).slice(0, 8).reverse();
      WolfCharts.bars(distCanvas, sorted.map((item) => item.value), sorted.map((item) => item.label), { barTop: "#2e87e8", barBottom: "#0f4f93" });
      if (sorted.length) {
        const top = sorted[sorted.length - 1];
        const low = sorted[0];
        [ "top " + top.label + " " + formatCompactNumber(top.value), "floor " + low.label + " " + formatCompactNumber(low.value) ].forEach((text) => {
          const chip = document.createElement("span");
          chip.textContent = text;
          distLegend.appendChild(chip);
        });
      }
    } else {
      const chip = document.createElement("span");
      chip.textContent = "No distribution source";
      distLegend.appendChild(chip);
    }

    const metricCard = document.createElement("article");
    metricCard.className = "wolfbbs-dashboard-card";
    const metricTitle = document.createElement("h3");
    metricTitle.textContent = "Ops Snapshot";
    metricCard.appendChild(metricTitle);
    const metricSub = document.createElement("p");
    metricSub.className = "wolfbbs-dashboard-sub";
    metricSub.textContent = "Live rollup from visible KPIs and data tables.";
    metricCard.appendChild(metricSub);
    const metricStack = document.createElement("div");
    metricStack.className = "wolfbbs-dash-metrics";
    metricCard.appendChild(metricStack);

    const sourceValues = primary ? primary.values : [];
    const avg = sourceValues.length ? average(sourceValues) : 0;
    const latest = sourceValues.length ? sourceValues[sourceValues.length - 1] : 0;
    const delta = sourceValues.length > 1 ? (sourceValues[sourceValues.length - 1] - sourceValues[0]) : 0;

    [
      { label: "Current", value: formatCompactNumber(latest) },
      { label: "Average", value: formatCompactNumber(avg) },
      { label: "Net Change", value: (delta >= 0 ? "+" : "") + formatCompactNumber(delta) }
    ].forEach((item) => {
      const row = document.createElement("div");
      row.className = "wolfbbs-dash-metric";
      const rowLabel = document.createElement("label");
      rowLabel.textContent = item.label;
      const rowValue = document.createElement("strong");
      rowValue.textContent = item.value;
      row.appendChild(rowLabel);
      row.appendChild(rowValue);
      metricStack.appendChild(row);
    });

    const mini = document.createElement("div");
    mini.className = "wolfbbs-dash-mini";
    (kpiRows.length ? kpiRows : [{ label: secondary ? secondary.column : "Data", value: secondary ? average(secondary.values) : 0 }]).slice(0, 4).forEach((item) => {
      const chip = document.createElement("div");
      chip.className = "wolfbbs-dash-chip";
      const value = document.createElement("strong");
      value.textContent = formatCompactNumber(item.value);
      const label = document.createElement("span");
      label.textContent = item.label || "kpi";
      chip.appendChild(value);
      chip.appendChild(label);
      mini.appendChild(chip);
    });
    metricCard.appendChild(mini);

    dashboard.appendChild(trendCard);
    dashboard.appendChild(distCard);
    dashboard.appendChild(metricCard);

    const shellMain = document.querySelector("main.wolfbbs-main");
    const firstTableWrap = document.querySelector(".wolfbbs-table-wrap");
    if (firstTableWrap && firstTableWrap.parentNode) {
      firstTableWrap.parentNode.insertBefore(dashboard, firstTableWrap);
    } else if (shellMain && shellMain.firstChild) {
      shellMain.insertBefore(dashboard, shellMain.firstChild);
    } else if (shellMain) {
      shellMain.appendChild(dashboard);
    }
  }

  mountDashboard();

  function tableRows(table) {
    const bodyRows = Array.from(table.querySelectorAll("tbody tr"));
    if (bodyRows.length) {
      return bodyRows;
    }
    const rows = Array.from(table.querySelectorAll("tr"));
    if (rows.length <= 1) return [];
    return rows.slice(1);
  }

  function ensureTableEmptyRow(table) {
    let row = table.querySelector("tr.wolfbbs-empty-row");
    if (row) return row;
    const first = table.querySelector("tr");
    const colCount = first ? Math.max(1, first.querySelectorAll("th,td").length) : 1;
    row = document.createElement("tr");
    row.className = "wolfbbs-empty-row";
    const cell = document.createElement("td");
    cell.colSpan = colCount;
    cell.textContent = "No rows match this filter.";
    row.appendChild(cell);
    const targetParent = table.querySelector("tbody") || table;
    targetParent.appendChild(row);
    row.style.display = "none";
    return row;
  }

  function tableCellValue(row, colIndex) {
    const cells = row.querySelectorAll("th,td");
    if (!cells.length || colIndex < 0 || colIndex >= cells.length) return "";
    return (cells[colIndex].textContent || "").trim();
  }

  function updateTableCount(table, node) {
    const rows = tableRows(table).filter((row) => !row.classList.contains("wolfbbs-empty-row"));
    const visible = rows.filter((row) => row.style.display !== "none").length;
    if (node) {
      node.textContent = visible + " of " + rows.length + " rows";
    }
    const empty = ensureTableEmptyRow(table);
    empty.style.display = visible ? "none" : "";
  }

  function sortTable(table, colIndex, direction) {
    const rows = tableRows(table).filter((row) => !row.classList.contains("wolfbbs-empty-row"));
    if (!rows.length) return;
    const parent = rows[0].parentNode;
    rows.sort((a, b) => {
      const left = tableCellValue(a, colIndex);
      const right = tableCellValue(b, colIndex);
      const leftNum = parseNumericValue(left);
      const rightNum = parseNumericValue(right);
      if (leftNum !== null && rightNum !== null) {
        return (leftNum - rightNum) * direction;
      }
      return left.localeCompare(right, undefined, { sensitivity: "base", numeric: true }) * direction;
    });
    rows.forEach((row) => parent.appendChild(row));
  }

  function downloadTableCSV(table, filename) {
    const rows = Array.from(table.querySelectorAll("tr")).filter((row) => {
      if (row.classList.contains("wolfbbs-empty-row")) return false;
      return row.style.display !== "none";
    });
    const lines = rows.map((row) => {
      return Array.from(row.querySelectorAll("th,td")).map((cell) => {
        const text = (cell.textContent || "").replace(/\s+/g, " ").trim().replace(/"/g, "\"\"");
        return "\"" + text + "\"";
      }).join(",");
    });
    if (!lines.length) return;
    const blob = new Blob([lines.join("\n")], { type: "text/csv;charset=utf-8" });
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    window.setTimeout(() => {
      URL.revokeObjectURL(link.href);
      link.remove();
    }, 0);
  }

  function tableViewKey(index) {
    return tableViewPrefix + normalizePath(currentRoute || "/") + ":" + String(index || 0);
  }

  function loadTableView(index) {
    return Object.assign({
      query: "",
      sortCol: -1,
      sortDirection: 1
    }, readJSON(tableViewKey(index), {}));
  }

  function saveTableView(index, view) {
    const payload = Object.assign({
      query: "",
      sortCol: -1,
      sortDirection: 1
    }, view || {});
    writeJSON(tableViewKey(index), payload);
    return payload;
  }

  function enhanceTables() {
    Array.from(document.querySelectorAll("table")).forEach((table, index) => {
      if (table.dataset.wolfbbsEnhanced === "1") return;
      table.dataset.wolfbbsEnhanced = "1";
      const wrap = table.closest(".wolfbbs-table-wrap") || table.parentNode;
      if (!wrap || !wrap.parentNode) return;
      const toolbar = document.createElement("div");
      toolbar.className = "wolfbbs-table-toolbar";
      const heading = document.createElement("strong");
      heading.textContent = "Table tools";
      toolbar.appendChild(heading);
      const count = document.createElement("span");
      count.className = "wolfbbs-table-count";
      toolbar.appendChild(count);
      const search = document.createElement("input");
      search.type = "search";
      search.placeholder = "Filter rows";
      toolbar.appendChild(search);
      const exportButton = document.createElement("button");
      exportButton.type = "button";
      exportButton.textContent = "Export CSV";
      exportButton.addEventListener("click", () => {
        downloadTableCSV(table, "wolfbbs-table-" + (index + 1) + ".csv");
        showToast("CSV exported", "ok");
      });
      toolbar.appendChild(exportButton);
      const exportSelectedButton = document.createElement("button");
      exportSelectedButton.type = "button";
      exportSelectedButton.textContent = "Export selected";
      toolbar.appendChild(exportSelectedButton);
      const selectVisibleButton = document.createElement("button");
      selectVisibleButton.type = "button";
      selectVisibleButton.textContent = "Select visible";
      toolbar.appendChild(selectVisibleButton);
      const invertSelectionButton = document.createElement("button");
      invertSelectionButton.type = "button";
      invertSelectionButton.textContent = "Invert selected";
      toolbar.appendChild(invertSelectionButton);
      const clearSelectionButton = document.createElement("button");
      clearSelectionButton.type = "button";
      clearSelectionButton.textContent = "Clear selected";
      toolbar.appendChild(clearSelectionButton);
      const saveViewButton = document.createElement("button");
      saveViewButton.type = "button";
      saveViewButton.textContent = "Save view";
      toolbar.appendChild(saveViewButton);
      const restoreViewButton = document.createElement("button");
      restoreViewButton.type = "button";
      restoreViewButton.textContent = "Restore view";
      toolbar.appendChild(restoreViewButton);
      const copyJSONButton = document.createElement("button");
      copyJSONButton.type = "button";
      copyJSONButton.textContent = "Copy JSON";
      toolbar.appendChild(copyJSONButton);
      const copySelectedJSONButton = document.createElement("button");
      copySelectedJSONButton.type = "button";
      copySelectedJSONButton.textContent = "Copy selected JSON";
      toolbar.appendChild(copySelectedJSONButton);
      const columnToggle = document.createElement("div");
      columnToggle.className = "wolfbbs-column-toggle";
      const columnToggleButton = document.createElement("button");
      columnToggleButton.type = "button";
      columnToggleButton.textContent = "Columns";
      const columnPanel = document.createElement("div");
      columnPanel.className = "wolfbbs-column-toggle-panel";
      columnToggle.appendChild(columnToggleButton);
      columnToggle.appendChild(columnPanel);
      toolbar.appendChild(columnToggle);
      wrap.parentNode.insertBefore(toolbar, wrap);
      const rows = tableRows(table).filter((row) => !row.classList.contains("wolfbbs-empty-row"));
      rows.forEach((row) => {
        row.addEventListener("click", (event) => {
          if (event.metaKey || event.ctrlKey) {
            row.classList.toggle("wolfbbs-selected-row");
            trackTelemetry("table:select-row");
            return;
          }
          rows.forEach((item) => item.classList.remove("wolfbbs-row-active"));
          row.classList.add("wolfbbs-row-active");
        });
      });

      function selectedRows() {
        return rows.filter((row) => row.classList.contains("wolfbbs-selected-row"));
      }

      const headerCells = Array.from(table.querySelectorAll("tr:first-child th"));
      const sortState = {
        col: -1,
        direction: 1
      };
      headerCells.forEach((cell, colIndex) => {
        const text = cell.textContent.trim();
        if (!text) return;
        const trigger = document.createElement("button");
        trigger.type = "button";
        trigger.className = "wolfbbs-table-sort";
        trigger.innerHTML = "<span>" + text + "</span><span class=\"wolfbbs-table-sort-indicator\">↕</span>";
        let direction = 1;
        trigger.addEventListener("click", () => {
          sortTable(table, colIndex, direction);
          sortState.col = colIndex;
          sortState.direction = direction;
          direction = direction * -1;
          trigger.querySelector(".wolfbbs-table-sort-indicator").textContent = direction > 0 ? "↑" : "↓";
          updateTableCount(table, count);
          trackTelemetry("table:sort");
        });
        cell.innerHTML = "";
        cell.appendChild(trigger);

        const toggleRow = document.createElement("label");
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.setAttribute("aria-label", "Toggle column " + text);
        cb.checked = true;
        cb.addEventListener("change", () => {
          const visible = cb.checked;
          Array.from(table.querySelectorAll("tr")).forEach((row) => {
            const cells = row.querySelectorAll("th,td");
            if (!cells[colIndex]) return;
            cells[colIndex].style.display = visible ? "" : "none";
          });
          trackTelemetry("table:column-toggle");
        });
        toggleRow.appendChild(cb);
        const labelText = document.createElement("span");
        labelText.textContent = text;
        toggleRow.appendChild(labelText);
        columnPanel.appendChild(toggleRow);
      });

      columnToggleButton.addEventListener("click", () => {
        columnPanel.classList.toggle("active");
      });
      document.addEventListener("click", (event) => {
        if (!columnToggle.contains(event.target)) {
          columnPanel.classList.remove("active");
        }
      });

      search.addEventListener("input", () => {
        const query = search.value.trim().toLowerCase();
        const targetRows = tableRows(table).filter((row) => !row.classList.contains("wolfbbs-empty-row"));
        targetRows.forEach((row) => {
          const hit = !query || row.textContent.toLowerCase().includes(query);
          row.style.display = hit ? "" : "none";
          row.classList.toggle("wolfbbs-row-hit", Boolean(query) && hit);
        });
        updateTableCount(table, count);
        trackTelemetry("table:filter");
      });

      saveViewButton.addEventListener("click", () => {
        saveTableView(index, {
          query: search.value || "",
          sortCol: sortState.col,
          sortDirection: sortState.direction
        });
        showToast("Table view saved", "ok");
        trackTelemetry("table:view-save");
      });

      restoreViewButton.addEventListener("click", () => {
        const view = loadTableView(index);
        search.value = view.query || "";
        search.dispatchEvent(new Event("input"));
        if (view.sortCol >= 0) {
          sortTable(table, view.sortCol, view.sortDirection || 1);
          sortState.col = view.sortCol;
          sortState.direction = view.sortDirection || 1;
        }
        showToast("Table view restored", "ok");
        trackTelemetry("table:view-restore");
      });

      copyJSONButton.addEventListener("click", () => {
        const header = Array.from(table.querySelectorAll("tr:first-child th, tr:first-child td")).map((cell) => (cell.textContent || "").trim());
        const body = tableRows(table).filter((row) => !row.classList.contains("wolfbbs-empty-row") && row.style.display !== "none");
        const rowsAsJSON = body.map((row) => {
          const out = {};
          Array.from(row.querySelectorAll("th,td")).forEach((cell, idx) => {
            const key = header[idx] || ("col_" + idx);
            out[key] = (cell.textContent || "").trim();
          });
          return out;
        });
        copyText(JSON.stringify(rowsAsJSON, null, 2)).then(() => {
          showToast("Visible rows copied as JSON", "ok");
          trackTelemetry("table:copy-json");
        }).catch(() => {
          showToast("JSON copy failed", "error");
        });
      });

      exportSelectedButton.addEventListener("click", () => {
        const selected = selectedRows();
        if (!selected.length) {
          showToast("No rows selected. Ctrl/Cmd+Click rows first.", "error");
          return;
        }
        const header = Array.from(table.querySelectorAll("tr:first-child th, tr:first-child td")).map((cell) => (cell.textContent || "").trim());
        const lines = [header.map((item) => "\"" + item.replace(/"/g, "\"\"") + "\"").join(",")];
        selected.forEach((row) => {
          const line = Array.from(row.querySelectorAll("th,td")).map((cell) => "\"" + ((cell.textContent || "").replace(/\s+/g, " ").trim().replace(/"/g, "\"\"")) + "\"").join(",");
          lines.push(line);
        });
        const blob = new Blob([lines.join("\n")], { type: "text/csv;charset=utf-8" });
        const link = document.createElement("a");
        link.href = URL.createObjectURL(blob);
        link.download = "wolfbbs-selected-" + (index + 1) + ".csv";
        document.body.appendChild(link);
        link.click();
        window.setTimeout(() => {
          URL.revokeObjectURL(link.href);
          link.remove();
        }, 0);
        showToast("Selected rows exported", "ok");
        trackTelemetry("table:export-selected");
      });

      selectVisibleButton.addEventListener("click", () => {
        rows.forEach((row) => {
          if (row.style.display !== "none") {
            row.classList.add("wolfbbs-selected-row");
          }
        });
        showToast("Visible rows selected", "ok");
        trackTelemetry("table:select-visible");
      });

      invertSelectionButton.addEventListener("click", () => {
        rows.forEach((row) => {
          if (row.style.display !== "none") {
            row.classList.toggle("wolfbbs-selected-row");
          }
        });
        showToast("Selection inverted", "ok");
        trackTelemetry("table:invert-selection");
      });

      clearSelectionButton.addEventListener("click", () => {
        rows.forEach((row) => row.classList.remove("wolfbbs-selected-row"));
        showToast("Selection cleared", "ok");
        trackTelemetry("table:clear-selection");
      });

      copySelectedJSONButton.addEventListener("click", () => {
        const selected = selectedRows();
        if (!selected.length) {
          showToast("No rows selected", "error");
          return;
        }
        const header = Array.from(table.querySelectorAll("tr:first-child th, tr:first-child td")).map((cell) => (cell.textContent || "").trim());
        const payload = selected.map((row) => {
          const out = {};
          Array.from(row.querySelectorAll("th,td")).forEach((cell, idx) => {
            const key = header[idx] || ("col_" + idx);
            out[key] = (cell.textContent || "").trim();
          });
          return out;
        });
        copyText(JSON.stringify(payload, null, 2)).then(() => {
          showToast("Selected rows copied as JSON", "ok");
          trackTelemetry("table:copy-selected-json");
        }).catch(() => showToast("Selected JSON copy failed", "error"));
      });

      const bootView = loadTableView(index);
      if (bootView.query) {
        search.value = bootView.query;
        search.dispatchEvent(new Event("input"));
      }
      if (bootView.sortCol >= 0) {
        sortTable(table, bootView.sortCol, bootView.sortDirection || 1);
        sortState.col = bootView.sortCol;
        sortState.direction = bootView.sortDirection || 1;
      }
      updateTableCount(table, count);
    });
  }

  function replayFormKey(form, index) {
    const action = normalizePath(form.getAttribute("action") || currentRoute || "/");
    const method = (form.getAttribute("method") || "get").toLowerCase();
    return replayPrefix + normalizePath(currentRoute || "/") + ":" + method + ":" + action + ":" + String(index || 0);
  }

  function mountDraftRestoreBanner(form, key) {
    const saved = readJSON(key, null);
    if (!saved || !saved.values || form.querySelector(".wolfbbs-draft-banner")) return;
    const row = document.createElement("div");
    row.className = "wolfbbs-restore-banner wolfbbs-draft-banner";
    const note = document.createElement("span");
    note.textContent = "Draft autosave is available.";
    row.appendChild(note);
    const restore = document.createElement("button");
    restore.type = "button";
    restore.textContent = "Restore draft";
    restore.addEventListener("click", () => {
      applyDraftPayload(form, saved.values);
      showToast("Draft restored", "ok");
      trackTelemetry("draft:restore");
    });
    row.appendChild(restore);
    const clear = document.createElement("button");
    clear.type = "button";
    clear.textContent = "Dismiss";
    clear.addEventListener("click", () => {
      row.remove();
      localStorage.removeItem(key);
      trackTelemetry("draft:dismiss");
    });
    row.appendChild(clear);
    form.insertBefore(row, form.firstChild);
  }

  function captureReplayPayload(form) {
    const fields = Array.from(form.querySelectorAll("input[name], textarea[name], select[name]"));
    const payload = {};
    fields.forEach((field) => {
      const type = (field.getAttribute("type") || "").toLowerCase();
      if (type === "hidden" || type === "password" || type === "file" || type === "submit" || type === "button") return;
      if (type === "checkbox" || type === "radio") {
        payload[field.name] = Boolean(field.checked);
        return;
      }
      payload[field.name] = field.value || "";
    });
    return payload;
  }

  function mountReplayBanner(form, key) {
    const saved = readJSON(key, null);
    if (!saved || !saved.values || form.querySelector(".wolfbbs-restore-banner")) return;
    const row = document.createElement("div");
    row.className = "wolfbbs-restore-banner";
    const note = document.createElement("span");
    note.textContent = "Last submit snapshot available.";
    row.appendChild(note);
    const restore = document.createElement("button");
    restore.type = "button";
    restore.textContent = "Restore fields";
    restore.addEventListener("click", () => {
      const fields = Array.from(form.querySelectorAll("input[name], textarea[name], select[name]"));
      fields.forEach((field) => {
        const type = (field.getAttribute("type") || "").toLowerCase();
        if (type === "hidden" || type === "password" || type === "file" || type === "submit" || type === "button") return;
        if (!(field.name in saved.values)) return;
        if (type === "checkbox" || type === "radio") {
          field.checked = Boolean(saved.values[field.name]);
          return;
        }
        field.value = saved.values[field.name];
      });
      showToast("Previous form values restored", "ok");
      trackTelemetry("form:restore");
    });
    row.appendChild(restore);
    const clear = document.createElement("button");
    clear.type = "button";
    clear.textContent = "Dismiss";
    clear.addEventListener("click", () => {
      row.remove();
      localStorage.removeItem(key);
    });
    row.appendChild(clear);
    form.insertBefore(row, form.firstChild);
  }

  function enhanceForms() {
    Array.from(document.querySelectorAll("form")).forEach((form, index) => {
      if (form.dataset.wolfbbsFormEnhanced === "1") return;
      form.dataset.wolfbbsFormEnhanced = "1";
      if (form.closest("table")) return;
      const replayKey = replayFormKey(form, index);
      const draftKey = draftFormKey(form, index);
      mountReplayBanner(form, replayKey);
      mountDraftRestoreBanner(form, draftKey);
      let draftTimer = null;
      const draftFields = Array.from(form.querySelectorAll("input[name], textarea[name], select[name]")).filter((field) => {
        const type = (field.getAttribute("type") || "").toLowerCase();
        return !(type === "hidden" || type === "password" || type === "file" || type === "submit" || type === "button");
      });
      function queueDraftSave() {
        if (draftTimer) window.clearTimeout(draftTimer);
        draftTimer = window.setTimeout(() => {
          const values = captureDraftPayload(form);
          const hasAny = Object.values(values).some((value) => {
            if (typeof value === "boolean") return value;
            return String(value || "").trim() !== "";
          });
          if (!hasAny) return;
          writeJSON(draftKey, {
            updatedAt: new Date().toISOString(),
            route: currentRoute,
            formIndex: index,
            values: values
          });
          trackTelemetry("draft:autosave");
        }, 220);
      }
      draftFields.forEach((field) => {
        field.addEventListener("input", queueDraftSave);
        field.addEventListener("change", queueDraftSave);
      });
      const submitter = form.querySelector('button[type="submit"], input[type="submit"]');
      const stickySubmitEnabled = Boolean(submitter) && form.hasAttribute("data-sticky-submit");
      if (stickySubmitEnabled) {
        const dock = document.createElement("div");
        dock.className = "wolfbbs-form-actions-sticky";
        const action = document.createElement("button");
        action.type = "button";
        const submitLabel = String(submitter.textContent || submitter.value || "Submit").replace(/\s+/g, " ").trim() || "Submit";
        action.textContent = submitLabel;
        action.title = "Primary action: " + submitLabel;
        action.addEventListener("click", () => {
          if (typeof form.requestSubmit === "function") {
            form.requestSubmit(submitter);
            return;
          }
          submitter.click();
        });
        dock.appendChild(action);
        form.appendChild(dock);
      }
      if (!submitter) return;
      form.addEventListener("submit", () => {
        const values = captureReplayPayload(form);
        const hasAny = Object.values(values).some((value) => {
          if (typeof value === "boolean") return value;
          return String(value || "").trim() !== "";
        });
        if (!hasAny) return;
        writeJSON(replayKey, {
          updatedAt: new Date().toISOString(),
          values: values
        });
        trackTelemetry("form:snapshot");
      });
    });
  }

  function tableRound3Key(index) {
    return "wolfbbs:ui:table-round3:" + normalizePath(currentRoute || "/") + ":" + String(index || 0);
  }

  function enhanceTablesRound3() {
    Array.from(document.querySelectorAll(".wolfbbs-table-wrap")).forEach((wrap, index) => {
      if (wrap.dataset.wolfbbsRound3 === "1") return;
      wrap.dataset.wolfbbsRound3 = "1";
      const table = wrap.querySelector("table");
      if (!table) return;
      const state = Object.assign({ sticky: false, compact: false }, readJSON(tableRound3Key(index), {}));
      const toolbar = document.createElement("div");
      toolbar.className = "wolfbbs-inline-actions";
      const sticky = document.createElement("button");
      sticky.type = "button";
      const compact = document.createElement("button");
      compact.type = "button";
      function sync() {
        wrap.classList.toggle("wolfbbs-sticky-head", Boolean(state.sticky));
        wrap.classList.toggle("wolfbbs-table-compact", Boolean(state.compact));
        sticky.textContent = state.sticky ? "Sticky head: on" : "Sticky head: off";
        compact.textContent = state.compact ? "Compact rows: on" : "Compact rows: off";
        writeJSON(tableRound3Key(index), state);
      }
      sticky.addEventListener("click", () => {
        state.sticky = !state.sticky;
        sync();
        trackTelemetry("table:sticky-toggle");
      });
      compact.addEventListener("click", () => {
        state.compact = !state.compact;
        sync();
        trackTelemetry("table:compact-toggle");
      });
      toolbar.appendChild(sticky);
      toolbar.appendChild(compact);
      wrap.insertBefore(toolbar, wrap.firstChild);
      const inspector = document.createElement("div");
      inspector.className = "wolfbbs-row-inspector";
      inspector.innerHTML = "<strong>Row inspector</strong><p class=\"wolfbbs-muted\">Click a row to inspect key values.</p>";
      wrap.appendChild(inspector);
      const headers = Array.from(table.querySelectorAll("tr:first-child th, tr:first-child td")).map((cell) => (cell.textContent || "").trim());
      Array.from(table.querySelectorAll("tr")).slice(1).forEach((row) => {
        row.addEventListener("click", (event) => {
          if (event.metaKey || event.ctrlKey || row.classList.contains("wolfbbs-empty-row")) return;
          const cells = Array.from(row.querySelectorAll("th,td"));
          if (!cells.length) return;
          const dl = document.createElement("dl");
          cells.forEach((cell, idx) => {
            const dt = document.createElement("dt");
            dt.textContent = headers[idx] || ("Column " + (idx + 1));
            const dd = document.createElement("dd");
            dd.textContent = (cell.textContent || "").trim() || "—";
            dl.appendChild(dt);
            dl.appendChild(dd);
          });
          inspector.innerHTML = "<strong>Row inspector</strong>";
          inspector.appendChild(dl);
          trackTelemetry("table:inspect-row");
        });
      });
      sync();
    });
  }

  function enhanceFormsRound3() {
    Array.from(document.querySelectorAll("form")).forEach((form) => {
      if (form.dataset.wolfbbsRound3Form === "1") return;
      form.dataset.wolfbbsRound3Form = "1";
      if (form.closest("table")) return;
      const fields = Array.from(form.querySelectorAll("input[name], textarea[name], select[name]")).filter((field) => {
        const type = (field.getAttribute("type") || "").toLowerCase();
        return !(type === "hidden" || type === "password" || type === "file" || type === "submit" || type === "button");
      });
      if (!fields.length) return;
      const utilityForm = form.classList.contains("wolfbbs-inline-form")
        || (!form.querySelector("textarea") && fields.length <= 4 && form.querySelectorAll('button, input[type="submit"], input[type="button"]').length <= 2);
      if (utilityForm) {
        form.classList.add("wolfbbs-utility-form");
      }
      const baseline = JSON.stringify(captureReplayPayload(form));
      function syncDirtyState() {
        const now = JSON.stringify(captureReplayPayload(form));
        if (now !== baseline) {
          dirtyForms.add(form);
        } else {
          dirtyForms.delete(form);
        }
        refreshDirtyTitle();
      }
      fields.forEach((field) => {
        field.addEventListener("input", syncDirtyState);
        field.addEventListener("change", syncDirtyState);
      });
      form.addEventListener("submit", () => {
        dirtyForms.delete(form);
        refreshDirtyTitle();
      });
    });
  }

  function enhanceFieldValidation() {
    const fields = Array.from(document.querySelectorAll('input[type="email"], input[type="url"], input[type="number"]'));
    fields.forEach((field) => {
      if (field.dataset.wolfbbsValidated === "1") return;
      field.dataset.wolfbbsValidated = "1";
      const hint = document.createElement("small");
      hint.className = "wolfbbs-field-hint";
      hint.style.display = "none";
      field.insertAdjacentElement("afterend", hint);
      function validate() {
        if (!field.value.trim()) {
          field.classList.remove("wolfbbs-invalid");
          field.removeAttribute("aria-invalid");
          hint.style.display = "none";
          hint.textContent = "";
          return;
        }
        if (field.checkValidity()) {
          field.classList.remove("wolfbbs-invalid");
          field.removeAttribute("aria-invalid");
          hint.style.display = "none";
          hint.textContent = "";
          return;
        }
        field.classList.add("wolfbbs-invalid");
        field.setAttribute("aria-invalid", "true");
        hint.style.display = "block";
        hint.textContent = field.validationMessage || "Please check this value.";
      }
      field.addEventListener("input", validate);
      field.addEventListener("blur", validate);
    });
  }

  function enhanceTextCounters() {
    const fields = Array.from(document.querySelectorAll('textarea, input[type="text"], input[type="search"], input[type="email"], input[type="url"]'));
    fields.forEach((field) => {
      if (field.dataset.wolfbbsCounter === "1") return;
      if (field.closest(".wolfbbs-template-strip")) return;
      field.dataset.wolfbbsCounter = "1";
      const counter = document.createElement("small");
      counter.className = "wolfbbs-text-counter";
      function sync() {
        const value = field.value || "";
        const max = Number(field.getAttribute("maxlength") || 0);
        counter.textContent = max > 0 ? (value.length + " / " + max + " chars") : (value.length + " chars");
      }
      field.insertAdjacentElement("afterend", counter);
      field.addEventListener("input", sync);
      sync();
    });
  }

  function composeTemplatesForRoute() {
    if (currentRoute.indexOf("/admin/events") === 0) {
      return [
        { label: "Tournament invite", body: "Title: Friday Tournament Night\nWhen: Fri 20:00 local\nWhere: /tournaments\nWhy: Weekly bracket + score ladder." },
        { label: "Social net", body: "Title: Lobby Net\nWhen: Weekday 21:00 local\nWhere: #lobby\nPrompt: Share one win and one thing you need help with." },
        { label: "Ops review", body: "Title: Sysop Ops Review\nWhen: Weekly\nAgenda:\n- Launch blockers\n- Recent incidents\n- Next release goals" }
      ];
    }
    if (currentRoute.indexOf("/mail") === 0 || currentRoute.indexOf("/boards") === 0) {
      return [
        { label: "Welcome reply", body: "Welcome aboard.\n\nGreat to have you here. If you want a quick start, begin with /today and /boards." },
        { label: "Status update", body: "Quick update:\n- What changed\n- Why it matters\n- What is next\n\nReply if you want details." },
        { label: "Follow-up ask", body: "Following up on this thread.\n\nCould you share:\n1) what worked\n2) what blocked you\n3) what we should improve next" }
      ];
    }
    return [
      { label: "Announcement", body: "Headline:\n\nWhat changed:\n\nHow to use it:\n\nWhere to give feedback:" },
      { label: "Issue report", body: "Observed behavior:\nExpected behavior:\nSteps to reproduce:\nEnvironment:" },
      { label: "Release note", body: "Release summary:\n- Feature 1\n- Feature 2\n- Fixes\nValidation: tests + e2e passed." }
    ];
  }

  function enhanceComposeTemplates() {
    const templates = composeTemplatesForRoute();
    Array.from(document.querySelectorAll("form")).forEach((form) => {
      if (form.dataset.wolfbbsTemplatesEnhanced === "1") return;
      const textarea = form.querySelector('textarea[name="body"], textarea[name="description"], textarea[name="payload"]');
      if (!textarea) return;
      form.dataset.wolfbbsTemplatesEnhanced = "1";
      const strip = document.createElement("div");
      strip.className = "wolfbbs-template-strip";
      templates.slice(0, 3).forEach((tpl) => {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = tpl.label;
        button.addEventListener("click", () => {
          if ((textarea.value || "").trim()) {
            textarea.value = textarea.value + "\n\n" + tpl.body;
          } else {
            textarea.value = tpl.body;
          }
          textarea.dispatchEvent(new Event("input", { bubbles: true }));
          textarea.focus();
          showToast("Template inserted: " + tpl.label, "ok");
          trackTelemetry("compose:template");
        });
        strip.appendChild(button);
      });
      textarea.insertAdjacentElement("beforebegin", strip);
    });
  }

  enhanceTables();
  enhanceTablesRound3();
  enhanceForms();
  enhanceFormsRound3();
  enhanceFieldValidation();
  enhanceTextCounters();
  enhanceComposeTemplates();
  mountUXRound20Pass();
  mountSpatialPreview();
  applyBentoDensity();
  mountStructuralMotion();
  mountMorphingUI();
  mountKineticHero();

  const overlay = document.createElement("div");
  overlay.id = "wolfbbsPaletteOverlay";
  overlay.innerHTML = '<div id="wolfbbsPalette"><div id="wolfbbsPaletteHeader"><label><span id="wolfbbsOmnibarLabel">Omnibar / Ask WolfBBS</span><span class="wolfbbs-omnibar-hint">Try natural prompts like "take me to boards", "open release gate", or "focus mode".</span><input id="wolfbbsPaletteInput" type="search" placeholder="Ask for routes, actions, settings, or sysop tools..." style="width:100%"></label><div id="wolfbbsPaletteHistory" class="wolfbbs-palette-history"></div></div><div id="wolfbbsOmnibarModules" class="wolfbbs-omnibar-module-grid"></div><div id="wolfbbsPaletteList"></div></div>';
  document.body.appendChild(overlay);

  const paletteButton = document.createElement("button");
  paletteButton.id = "wolfbbsCommandButton";
  paletteButton.type = "button";
  paletteButton.textContent = "Search";
  const headerControls = document.querySelector(".wolfbbs-pref-controls");
  if (headerControls) {
    paletteButton.className = "wolfbbs-header-command";
    const firstMenu = headerControls.querySelector(".wolfbbs-pref-menu");
    headerControls.insertBefore(paletteButton, firstMenu || null);
  } else {
    document.body.appendChild(paletteButton);
  }

  const paletteInput = overlay.querySelector("#wolfbbsPaletteInput");
  const paletteList = overlay.querySelector("#wolfbbsPaletteList");
  const paletteHistory = overlay.querySelector("#wolfbbsPaletteHistory");
  const omnibarModules = overlay.querySelector("#wolfbbsOmnibarModules");

  function openNamedSurface(name) {
    if (typeof window[name] !== "function") return false;
    window[name]();
    return true;
  }

  function executeOmnibarItem(item) {
    if (!item) return false;
    if (item.href) {
      location.href = item.href;
      return true;
    }
    if (!item.action) return false;
    if (item.action === "theme:night") {
      setUIPref("theme", "night", "Theme set to Night");
      return true;
    }
    if (item.action === "theme:contrast") {
      setUIPref("theme", "contrast", "Theme set to Contrast");
      return true;
    }
    if (item.action === "layout:focus") {
      setUIPref("layout", "focus", "Layout set to Focus");
      return true;
    }
    if (item.action === "layout:wide") {
      setUIPref("layout", "wide", "Layout set to Wide");
      return true;
    }
    if (item.action === "density:compact") {
      setUIPref("density", "compact", "Density set to Compact");
      return true;
    }
    if (item.action === "density:comfortable") {
      setUIPref("density", "comfortable", "Density set to Comfortable");
      return true;
    }
    if (item.action === "motion:reduced") {
      setUIPref("motion", "reduced", "Motion set to Reduced");
      return true;
    }
    if (item.action === "motion:full") {
      setUIPref("motion", "full", "Motion set to Full");
      return true;
    }
    if (item.action === "workspace") return openNamedSurface("wolfbbsOpenWorkspaceHub");
    if (item.action === "spotlight") return openNamedSurface("wolfbbsOpenSpotlight");
    if (item.action === "release") return openNamedSurface("wolfbbsOpenReleaseGate");
    if (item.action === "bug") return openNamedSurface("wolfbbsOpenBugCapture");
    if (item.action === "toast") return openNamedSurface("wolfbbsOpenToastCenter");
    if (item.action === "checkpoint") return openNamedSurface("wolfbbsOpenCheckpointHub");
    if (item.action === "drafts") return openNamedSurface("wolfbbsOpenDraftCenter");
    if (item.action === "insights") return openNamedSurface("wolfbbsOpenKPIWatchCenter");
    if (item.action === "incident") return openNamedSurface("wolfbbsOpenIncidentConsole");
    if (item.action === "playbook") return openNamedSurface("wolfbbsOpenPlaybookRunner");
    if (item.action === "reminders") return openNamedSurface("wolfbbsOpenReminderScheduler");
    return false;
  }

  function routeModules() {
    if (routeKind(currentRoute) === "admin") {
      return [
        { label: "Release Gate", description: "Launch checks, evidence, and ship status.", action: "release", meta: "operator" },
        { label: "Ops Console", description: "Incidents, runtime drift, and next actions.", href: "/admin/ops", meta: "operator" },
        { label: "Config Center", description: "Identity, runtime flags, and gateways.", href: "/admin/setup", meta: "operator" },
        { label: "Bug Capture", description: "Log breakage without losing route context.", action: "bug", meta: "operator" },
        { label: "Focus Layout", description: "Trim chrome for deep operator work.", action: "layout:focus", meta: "ui" },
        { label: "Status Center", description: "Caller-facing health and uptime view.", href: "/status", meta: "operator" }
      ];
    }
    if (routeKind(currentRoute) === "guest") {
      return [
        { label: "Start Center", description: "Orientation, quick paths, and the core story.", href: "/start", meta: "guest" },
        { label: "Connect Guide", description: "SSH, web, IRC, and install paths.", href: "/connect", meta: "guest" },
        { label: "Showcase", description: "Feature tour, lanes, and product proof.", href: "/showcase", meta: "guest" },
        { label: "Spotlight", description: "Find routes and product surfaces fast.", action: "spotlight", meta: "guest" },
        { label: "Night Theme", description: "Lean into the dark glass default.", action: "theme:night", meta: "ui" },
        { label: "Wide Layout", description: "Stretch the shell for comparison work.", action: "layout:wide", meta: "ui" }
      ];
    }
    return [
      { label: "Today Brief", description: "Your live summary, momentum, and next moves.", href: "/today", meta: "caller" },
      { label: "Boards", description: "Browse conversations, digests, and watch tiers.", href: "/boards", meta: "caller" },
      { label: "Live Chat", description: "Jump into the lobby and current channels.", href: "/chat", meta: "caller" },
      { label: "Mail", description: "Private messages, drafts, and reply flows.", href: "/mail", meta: "caller" },
      { label: "Workspace Hub", description: "Sticky context, notes, and work surfaces.", action: "workspace", meta: "caller" },
      { label: "Compact Density", description: "Fit more signal into each view.", action: "density:compact", meta: "ui" }
    ];
  }

  function renderOmnibarModules(query) {
    if (!omnibarModules) return;
    const q = (query || "").trim().toLowerCase();
    omnibarModules.innerHTML = "";
    routeModules()
      .filter((item) => !q || item.label.toLowerCase().includes(q) || item.description.toLowerCase().includes(q) || String(item.meta || "").toLowerCase().includes(q))
      .slice(0, 6)
      .forEach((item) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "wolfbbs-omnibar-module";
        button.innerHTML = "<strong>" + item.label + "</strong><span>" + item.description + "</span>";
        button.addEventListener("click", () => {
          executeOmnibarItem(item);
          closePalette();
        });
        omnibarModules.appendChild(button);
      });
  }

  function resolveOmnibarIntent(rawQuery) {
    const q = String(rawQuery || "").trim().toLowerCase();
    if (!q) return null;
    if ((q.includes("night") || q.includes("dark")) && q.includes("theme")) return { label: "Switch to Night theme", meta: "appearance", action: "theme:night" };
    if (q.includes("contrast")) return { label: "Switch to Contrast theme", meta: "appearance", action: "theme:contrast" };
    if (q.includes("focus")) return { label: "Enable Focus layout", meta: "appearance", action: "layout:focus" };
    if (q.includes("wide")) return { label: "Enable Wide layout", meta: "appearance", action: "layout:wide" };
    if (q.includes("compact") || q.includes("dense")) return { label: "Use Compact density", meta: "appearance", action: "density:compact" };
    if (q.includes("comfortable")) return { label: "Use Comfortable density", meta: "appearance", action: "density:comfortable" };
    if (q.includes("reduced motion")) return { label: "Reduce motion", meta: "appearance", action: "motion:reduced" };
    if (q.includes("full motion")) return { label: "Restore full motion", meta: "appearance", action: "motion:full" };
    if (q.includes("workspace")) return { label: "Open Workspace Hub", meta: "surface", action: "workspace" };
    if (q.includes("spotlight") || q.includes("search everything")) return { label: "Open Spotlight", meta: "surface", action: "spotlight" };
    if (q.includes("release") || q.includes("ship") || q.includes("launch")) return { label: "Open Release Gate", meta: "surface", action: "release" };
    if (q.includes("bug") || q.includes("issue") || q.includes("report")) return { label: "Open Bug Capture", meta: "surface", action: "bug" };
    if (q.includes("ops") || q.includes("incident")) return { label: "Open Ops Console", meta: "navigate", href: "/admin/ops" };
    if (q.includes("status") || q.includes("health")) return { label: "Open Status Center", meta: "navigate", href: "/status" };
    if (q.includes("connect")) return { label: "Open Connect Guide", meta: "navigate", href: "/connect" };
    if (q.includes("showcase")) return { label: "Open Showcase", meta: "navigate", href: "/showcase" };
    if (q.includes("today")) return { label: "Open Today Brief", meta: "navigate", href: "/today" };
    if (q.includes("attention")) return { label: "Open Attention Center", meta: "navigate", href: "/attention" };
    if (q.includes("board")) return { label: "Open Boards", meta: "navigate", href: "/boards" };
    if (q.includes("chat") || q.includes("lobby")) return { label: "Open Live Chat", meta: "navigate", href: "/chat" };
    if (q.includes("mail") || q.includes("compose")) return { label: "Open Mail", meta: "navigate", href: "/mail" };
    if (q.includes("door")) return { label: "Open Doors", meta: "navigate", href: "/doors" };
    if (q.includes("config") || q.includes("setup")) return { label: "Open Config Center", meta: "navigate", href: routeKind(currentRoute) === "admin" ? "/admin/setup" : "/config" };
    if (q.includes("admin") || q.includes("sysop")) return { label: "Open Admin Deck", meta: "navigate", href: "/admin" };
    if (q.includes("start") || q.includes("home") || q.includes("dashboard")) return { label: "Open Start Center", meta: "navigate", href: "/start" };
    return null;
  }
  const commands = [
    { href: "/start", label: "Start Center", meta: "core" },
    { href: "/today", label: "Today Brief", meta: "core" },
    { href: "/attention", label: "Attention Center", meta: "core" },
    { href: "/boards", label: "Boards", meta: "core" },
    { href: "/chat", label: "Live Chat", meta: "core" },
    { href: "/mail", label: "Mail", meta: "core" },
    { href: "/status", label: "Status Center", meta: "core" },
    { href: "/showcase", label: "Showcase", meta: "core" }
  ].concat(loadFavorites().map((item) => ({
    href: item.href,
    label: item.label,
    meta: "favorite"
  }))).concat(navLinks).concat(headings.map((heading) => ({
    href: "#" + heading.id,
    label: heading.dataset.navLabel || headingBaseLabel(heading),
    meta: "section"
  })));

  function appendPaletteItem(item) {
    if (!item) return null;
    const element = document.createElement(item.href ? "a" : "button");
    element.className = "wolfbbs-palette-item";
    if (item.href) {
      element.href = item.href;
    } else {
      element.type = "button";
    }
    element.innerHTML = "<strong>" + item.label + "</strong><span class=\"wolfbbs-palette-meta\">" + item.meta + (item.href ? (" • " + item.href) : "") + "</span>";
    if (!item.href) {
      element.addEventListener("click", () => {
        executeOmnibarItem(item);
        closePalette();
      });
    }
    paletteList.appendChild(element);
    return element;
  }

  function renderPalette(query) {
    const q = (query || "").trim().toLowerCase();
    paletteList.innerHTML = "";
    renderOmnibarModules(query);
    const intent = resolveOmnibarIntent(query);
    if (intent) {
      appendPaletteItem(intent);
    }
    commands
      .filter((item) => !q || item.label.toLowerCase().includes(q) || item.href.toLowerCase().includes(q))
      .slice(0, 16)
      .forEach((item) => {
        appendPaletteItem(item);
      });
    if (!paletteList.children.length) {
      const empty = document.createElement("div");
      empty.className = "wolfbbs-palette-item";
      empty.innerHTML = "<strong>No matches</strong><span class=\"wolfbbs-palette-meta\">Try boards, release gate, focus mode, workspace, or status.</span>";
      paletteList.appendChild(empty);
    }
  }

  function loadPaletteHistory() {
    return readJSON(paletteHistoryKey, []).filter((item) => typeof item === "string" && item.trim() !== "").slice(0, 8);
  }

  function savePaletteHistoryEntry(raw) {
    const value = String(raw || "").trim();
    if (!value) return;
    const next = [value].concat(loadPaletteHistory().filter((item) => item.toLowerCase() !== value.toLowerCase())).slice(0, 8);
    writeJSON(paletteHistoryKey, next);
  }

  function renderPaletteHistory() {
    if (!paletteHistory) return;
    paletteHistory.innerHTML = "";
    const entries = loadPaletteHistory();
    if (!entries.length) return;
    entries.forEach((entry) => {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = entry;
      button.addEventListener("click", () => {
        paletteInput.value = entry;
        renderPalette(entry);
        paletteInput.focus();
      });
      paletteHistory.appendChild(button);
    });
  }

  function openPalette() {
    overlay.classList.add("active");
    renderPalette("");
    renderPaletteHistory();
    paletteInput.value = "";
    trackTelemetry("palette:open");
    window.setTimeout(() => paletteInput.focus(), 10);
  }

  function closePalette() {
    savePaletteHistoryEntry(paletteInput.value);
    renderPaletteHistory();
    overlay.classList.remove("active");
  }

  paletteButton.addEventListener("click", openPalette);
  paletteInput.addEventListener("input", () => renderPalette(paletteInput.value));
  paletteInput.addEventListener("keydown", (event) => {
    if (event.key !== "Enter") return;
    event.preventDefault();
    const intent = resolveOmnibarIntent(paletteInput.value);
    if (intent) {
      executeOmnibarItem(intent);
      closePalette();
      return;
    }
    const first = paletteList.querySelector(".wolfbbs-palette-item");
    if (!first) return;
    if (first.tagName && first.tagName.toLowerCase() === "a" && first.href) {
      location.href = first.getAttribute("href");
    } else {
      first.click();
    }
  });
  overlay.addEventListener("click", (event) => {
    if (event.target === overlay) closePalette();
    if (event.target && event.target.closest(".wolfbbs-palette-item")) {
      savePaletteHistoryEntry(paletteInput.value);
    }
  });
    document.addEventListener("keydown", (event) => {
    const tag = event.target && event.target.tagName ? event.target.tagName.toLowerCase() : "";
    const editing = tag === "input" || tag === "textarea" || tag === "select" || event.target.isContentEditable;
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
      event.preventDefault();
      openPalette();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "w") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenWorkspaceHub === "function") window.wolfbbsOpenWorkspaceHub();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "a") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenToastCenter === "function") window.wolfbbsOpenToastCenter();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "c") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenCheckpointHub === "function") window.wolfbbsOpenCheckpointHub();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "d") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenDraftCenter === "function") window.wolfbbsOpenDraftCenter();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "i") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenIncidentConsole === "function") window.wolfbbsOpenIncidentConsole();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "p") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenPlaybookRunner === "function") window.wolfbbsOpenPlaybookRunner();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "m") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenReminderScheduler === "function") window.wolfbbsOpenReminderScheduler();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "g") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenReleaseGate === "function") window.wolfbbsOpenReleaseGate();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "y") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenKPIWatchCenter === "function") window.wolfbbsOpenKPIWatchCenter();
      return;
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.shiftKey && String(event.key).toLowerCase() === "b") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenFeedbackPulse === "function") window.wolfbbsOpenFeedbackPulse();
      return;
    }
    if (!editing && event.altKey) {
      const macroRoutes = ["/start", "/today", "/attention", "/boards", "/chat", "/mail"];
      const idx = Number(event.key) - 1;
      if (Number.isInteger(idx) && idx >= 0 && idx < macroRoutes.length) {
        event.preventDefault();
        trackTelemetry("macro:route-jump");
        location.href = macroRoutes[idx];
        return;
      }
      if (event.key === "0" && typeof window.wolfbbsOpenMacroHelp === "function") {
        event.preventDefault();
        window.wolfbbsOpenMacroHelp();
        return;
      }
    }
    if (!editing && (event.metaKey || event.ctrlKey) && event.key === "/") {
      event.preventDefault();
      if (typeof window.wolfbbsOpenMacroHelp === "function") {
        window.wolfbbsOpenMacroHelp();
      }
      return;
    }
    if (event.key === "Escape" && overlay.classList.contains("active")) {
      event.preventDefault();
      closePalette();
      return;
    }
    if (!editing && event.key === "?") {
      event.preventDefault();
      openPalette();
    }
  });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initWolfbbsModernUI, { once: true });
    return;
  }
  initWolfbbsModernUI();
})();
</script>`
