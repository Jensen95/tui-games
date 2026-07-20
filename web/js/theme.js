// web/js/theme.js
//
// The site-wide light/dark theme toggle, shared by index.html and play.html
// (both load this as a plain deferred script). All the actual color values
// live in css/style.css, which already defines three layers:
//
//   :root                              -> dark (the site's default identity)
//   @media (prefers-color-scheme:light) -> light, when the OS asks for it
//   :root[data-theme="light"|"dark"]   -> an explicit override that wins both
//
// So switching themes is just a matter of stamping (or clearing) data-theme on
// <html> and remembering the choice. A tiny inline <head> script applies the
// stored choice before first paint (avoiding a flash of the wrong theme); this
// module wires the button and keeps the on-screen state honest afterwards.

(function () {
  "use strict";

  var STORAGE_KEY = "lig-theme";
  // Matches css/style.css's --bg for each theme, so the mobile browser chrome
  // (address bar etc.) tracks the page instead of staying stuck on the dark
  // default baked into the static <meta name="theme-color">.
  var THEME_COLORS = { dark: "#0a0a0b", light: "#ffffff" };

  var darkQuery =
    typeof window.matchMedia === "function"
      ? window.matchMedia("(prefers-color-scheme: dark)")
      : null;

  function storedChoice() {
    try {
      var v = localStorage.getItem(STORAGE_KEY);
      return v === "light" || v === "dark" ? v : null;
    } catch (e) {
      return null; // storage can throw in private mode / when blocked
    }
  }

  function systemTheme() {
    return darkQuery && darkQuery.matches ? "dark" : "light";
  }

  // The theme actually in force: an explicit choice if one exists, else
  // whatever the OS preference resolves to.
  function effectiveTheme() {
    return storedChoice() || systemTheme();
  }

  function applyExplicit(theme) {
    document.documentElement.setAttribute("data-theme", theme);
    try {
      localStorage.setItem(STORAGE_KEY, theme);
    } catch (e) {
      /* persistence is best-effort; the in-page toggle still works */
    }
    syncUI();
  }

  function updateThemeColorMeta(theme) {
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta && THEME_COLORS[theme]) meta.setAttribute("content", THEME_COLORS[theme]);
  }

  // Reflect the effective theme onto every toggle button (icon + a11y label)
  // and the theme-color meta. Called on load, after a click, and whenever the
  // OS preference changes while we're still following it.
  function syncUI() {
    var theme = effectiveTheme();
    updateThemeColorMeta(theme);
    var next = theme === "dark" ? "light" : "dark";
    var buttons = document.querySelectorAll("[data-theme-toggle]");
    for (var i = 0; i < buttons.length; i++) {
      var btn = buttons[i];
      btn.setAttribute("data-effective", theme);
      btn.setAttribute("aria-label", "Switch to " + next + " theme");
      btn.setAttribute("title", "Switch to " + next + " theme");
    }
  }

  function toggle() {
    // Flip relative to whatever is currently showing, then pin it explicitly.
    applyExplicit(effectiveTheme() === "dark" ? "light" : "dark");
  }

  function init() {
    var buttons = document.querySelectorAll("[data-theme-toggle]");
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].addEventListener("click", toggle);
    }

    // While the user hasn't made an explicit choice, keep following the OS: if
    // it flips, update the button icon (the CSS media query already reskins the
    // page on its own). Once a choice is stored, data-theme pins it and this is
    // a no-op for the page, only refreshing the icon defensively.
    if (darkQuery) {
      var onChange = function () {
        if (!storedChoice()) syncUI();
      };
      if (typeof darkQuery.addEventListener === "function") {
        darkQuery.addEventListener("change", onChange);
      } else if (typeof darkQuery.addListener === "function") {
        darkQuery.addListener(onChange); // older Safari
      }
    }

    syncUI();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
