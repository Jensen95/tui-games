// web/tests/e2e.mjs
//
// End-to-end smoke + regression suite for the web build (the WASM PWA under
// web/). It is intentionally self-contained: it starts its own static file
// server for web/, drives a headless Chromium via Playwright, and asserts the
// behaviours that have bitten us or that users asked for:
//
//   - the WASM engine loads and the game menu renders all games
//   - every game starts and renders a board of the right size
//   - the light/dark theme toggle switches, persists across reload, and
//     updates the theme-color meta
//   - the seed persists across reloads (does not reroll on every load)
//   - hints show a "why" explanation and then lock the Hint button for a
//     cooldown; the Shift+H shortcut is gated during the cooldown too
//   - Queens touch input: a jittery tap that drifts across a cell border is
//     still treated as a tap (cycles one cell) rather than a stray drag-paint,
//     while a real drag still paints X marks
//
// Prerequisites: the wasm bridge must be built first (the suite checks and
// tells you how if it is missing):
//   GOOS=js GOARCH=wasm go build -o web/lig.wasm ./web/wasm
//   cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/js/wasm_exec.js
//
// Run it with:  node web/tests/e2e.mjs   (or `task test:web`)
// Playwright + a Chromium build must be available; in this repo's environment
// they live under /opt (PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers). Exit code
// is non-zero if any assertion fails.

import { createRequire } from "node:module";
import { createServer } from "node:http";
import { readFile, stat } from "node:fs/promises";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, normalize } from "node:path";

const require = createRequire(import.meta.url);
const HERE = dirname(fileURLToPath(import.meta.url));
const WEB_ROOT = normalize(join(HERE, ".."));
const PORT = Number(process.env.LIG_TEST_PORT || 8399);
const BASE = `http://127.0.0.1:${PORT}`;

// ---- Playwright resolution (installed pkg, else the environment's global) ---
function loadChromium() {
  for (const spec of ["playwright", "/opt/node22/lib/node_modules/playwright"]) {
    try {
      return require(spec).chromium;
    } catch {
      /* try next */
    }
  }
  throw new Error(
    "Could not load Playwright. Install it (`npm i -D playwright`) or set NODE_PATH " +
      "to a global install, and ensure a Chromium build is available " +
      "(PLAYWRIGHT_BROWSERS_PATH)."
  );
}

// ---- tiny static server for web/ (correct wasm/js/css/svg MIME types) -------
const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".webmanifest": "application/manifest+json; charset=utf-8",
  ".wasm": "application/wasm",
  ".svg": "image/svg+xml",
  ".png": "image/png",
};

function startServer() {
  const server = createServer(async (req, res) => {
    try {
      let urlPath = decodeURIComponent(new URL(req.url, BASE).pathname);
      if (urlPath.endsWith("/")) urlPath += "index.html";
      // Contain within WEB_ROOT (no path traversal).
      const filePath = normalize(join(WEB_ROOT, urlPath));
      if (!filePath.startsWith(WEB_ROOT)) {
        res.writeHead(403).end("forbidden");
        return;
      }
      const info = await stat(filePath).catch(() => null);
      if (!info || !info.isFile()) {
        res.writeHead(404).end("not found");
        return;
      }
      const ext = filePath.slice(filePath.lastIndexOf("."));
      res.writeHead(200, { "content-type": MIME[ext] || "application/octet-stream" });
      res.end(await readFile(filePath));
    } catch (err) {
      res.writeHead(500).end(String(err));
    }
  });
  return new Promise((resolve) => server.listen(PORT, "127.0.0.1", () => resolve(server)));
}

// ---- minimal assertion harness --------------------------------------------
const results = [];
function ok(name, cond, extra = "") {
  results.push({ pass: !!cond, name, extra });
  console.log(`${cond ? "PASS" : "FAIL"} ${name}${extra ? "  — " + extra : ""}`);
}

// Dispatch a synthetic touch tap/drag on the Queens grid at (sx,sy) with a
// given (dx,dy) pointer travel, mirroring how a finger tap/drag arrives.
async function pointerStroke(page, sx, sy, dx, dy, id, steps = 1) {
  await page.evaluate(
    ({ sx, sy, dx, dy, id, steps }) => {
      const grid = document.querySelector(".queens-grid");
      const o = (cx, cy) => ({
        clientX: cx,
        clientY: cy,
        bubbles: true,
        cancelable: true,
        pointerId: id,
        pointerType: "touch",
        button: 0,
        isPrimary: true,
      });
      grid.dispatchEvent(new PointerEvent("pointerdown", o(sx, sy)));
      for (let i = 1; i <= steps; i++) {
        window.dispatchEvent(new PointerEvent("pointermove", o(sx + (dx * i) / steps, sy + (dy * i) / steps)));
      }
      window.dispatchEvent(new PointerEvent("pointerup", o(sx + dx, sy + dy)));
    },
    { sx, sy, dx, dy, id, steps }
  );
}

async function startGame(page, gameNameRe, difficulty) {
  await page.waitForSelector(".game-pick:not([disabled])");
  if (difficulty) await page.locator(`[data-difficulty="${difficulty}"]`).click();
  const cards = page.locator(".game-pick");
  const n = await cards.count();
  for (let i = 0; i < n; i++) {
    if (gameNameRe.test(await cards.nth(i).innerText())) {
      await cards.nth(i).click();
      return true;
    }
  }
  return false;
}

// ---- the suite --------------------------------------------------------------
async function main() {
  if (!existsSync(join(WEB_ROOT, "lig.wasm")) || !existsSync(join(WEB_ROOT, "js", "wasm_exec.js"))) {
    console.error(
      "Missing web/lig.wasm or web/js/wasm_exec.js. Build them first:\n" +
        '  GOOS=js GOARCH=wasm go build -o web/lig.wasm ./web/wasm\n' +
        '  cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/js/wasm_exec.js'
    );
    process.exit(2);
  }

  const chromium = loadChromium();
  const server = await startServer();
  const browser = await chromium.launch();
  // Pin the OS preference to dark so the toggle assertions below are
  // deterministic (first toggle => light); the toggle logic itself is
  // symmetric and follows the OS preference until an explicit choice is made.
  const ctx = await browser.newContext({ colorScheme: "dark" });
  const page = await ctx.newPage();
  const pageErrors = [];
  page.on("pageerror", (e) => pageErrors.push(String(e)));

  try {
    // 1) Engine loads + menu renders every game.
    await page.goto(`${BASE}/play.html`, { waitUntil: "networkidle" });
    await page.waitForSelector(".game-pick", { timeout: 15000 });
    const enabled = await page.locator(".game-pick:not([disabled])").count();
    ok("engine loads and all 5 games are enabled", enabled === 5, `enabled=${enabled}`);

    // 2) Theme toggle: switch, apply, persist, meta.
    const initialTheme = await page.evaluate(() => document.documentElement.getAttribute("data-theme"));
    ok("theme initially follows OS (no data-theme)", initialTheme === null, `got=${initialTheme}`);
    await page.locator("[data-theme-toggle]").first().click();
    const afterLight = await page.evaluate(() => ({
      attr: document.documentElement.getAttribute("data-theme"),
      ls: localStorage.getItem("lig-theme"),
      bg: getComputedStyle(document.body).backgroundColor,
      meta: document.querySelector('meta[name="theme-color"]').getAttribute("content"),
    }));
    ok("toggle switches to light + persists + updates meta",
      afterLight.attr === "light" && afterLight.ls === "light" &&
        afterLight.bg === "rgb(255, 255, 255)" && afterLight.meta === "#ffffff",
      JSON.stringify(afterLight));
    await page.reload({ waitUntil: "domcontentloaded" });
    const persistedTheme = await page.evaluate(() => document.documentElement.getAttribute("data-theme"));
    ok("theme persists across reload", persistedTheme === "light", `got=${persistedTheme}`);
    // reset to dark for the rest of the run
    await page.locator("[data-theme-toggle]").first().click();

    // 3) Seed persistence across reload.
    await page.waitForSelector("#seed-input");
    const seed1 = await page.inputValue("#seed-input");
    const lsSeed = await page.evaluate(() => localStorage.getItem("lig-seed"));
    ok("seed stored in localStorage", lsSeed !== null && lsSeed === seed1, `input=${seed1} ls=${lsSeed}`);
    await page.reload({ waitUntil: "networkidle" });
    await page.waitForSelector("#seed-input");
    const seed2 = await page.inputValue("#seed-input");
    ok("seed does not reroll on reload", seed1 === seed2, `before=${seed1} after=${seed2}`);

    // 4) Every game starts and renders a board.
    const games = [
      { re: /tango/i, sel: ".puzzle-grid" },
      { re: /queens/i, sel: ".queens-grid" },
      { re: /zip/i, sel: ".puzzle-grid" },
      { re: /patches/i, sel: ".puzzle-grid" },
      { re: /sudoku/i, sel: ".puzzle-grid" },
    ];
    for (const g of games) {
      await page.goto(`${BASE}/play.html`, { waitUntil: "networkidle" });
      const started = await startGame(page, g.re, "easy");
      let cells = 0;
      if (started) {
        await page.waitForSelector(`${g.sel} .cell, ${g.sel} .queens-cell`, { timeout: 10000 }).catch(() => {});
        cells = await page.evaluate((sel) => document.querySelectorAll(`${sel} .cell, ${sel} .queens-cell`).length, g.sel);
      }
      ok(`${g.re.source} starts and renders a board`, started && cells > 0, `cells=${cells}`);
    }

    // 5) Hint shows a "why" explanation + starts the cooldown; Shift+H gated.
    await page.goto(`${BASE}/play.html`, { waitUntil: "networkidle" });
    await startGame(page, /tango/i, "easy");
    await page.waitForSelector(".puzzle-grid .cell");
    await page.locator("#hint-btn").click();
    await page.waitForSelector("#hint-status:not([hidden])", { timeout: 5000 }).catch(() => {});
    const hintMsg = await page.evaluate(() => (document.getElementById("hint-status") || {}).textContent || "");
    ok("hint message explains WHY (contains an em-dash reason)", /—/.test(hintMsg), JSON.stringify(hintMsg.slice(0, 120)));
    const btnState = await page.evaluate(() => {
      const b = document.getElementById("hint-btn");
      return { disabled: b.disabled || b.getAttribute("aria-disabled") === "true", label: b.textContent.trim() };
    });
    ok("hint button locks with a countdown after use", btnState.disabled && /\(\d+\s*s\)/i.test(btnState.label), JSON.stringify(btnState));
    // Shift+H during cooldown must not fire another hint (button stays disabled).
    await page.keyboard.press("Shift+H");
    await page.waitForTimeout(150);
    const stillDisabled = await page.evaluate(() => {
      const b = document.getElementById("hint-btn");
      return b.disabled || b.getAttribute("aria-disabled") === "true";
    });
    ok("Shift+H is gated during the hint cooldown", stillDisabled);
    // New puzzle clears the cooldown.
    await page.locator("#reset-btn").click();
    await page.waitForTimeout(150);
    const reenabled = await page.evaluate(() => {
      const b = document.getElementById("hint-btn");
      return !(b.disabled || b.getAttribute("aria-disabled") === "true");
    });
    ok("cooldown resets on New puzzle", reenabled);

    // 6) Queens touch: jittery boundary-crossing tap cycles (does not stray-paint).
    await page.goto(`${BASE}/play.html`, { waitUntil: "networkidle" });
    await startGame(page, /queens/i, "easy");
    await page.waitForSelector(".queens-grid .queens-cell");
    const info = await page.evaluate(() => {
      const grid = document.querySelector(".queens-grid");
      const cols = getComputedStyle(grid).gridTemplateColumns.split(" ").length;
      const all = [...document.querySelectorAll(".queens-cell")];
      for (const c of all) {
        const r = +c.dataset.row, col = +c.dataset.col;
        if (col >= cols - 1) continue;
        const right = all[r * cols + col + 1];
        if (!c.classList.contains("given") && !right.classList.contains("given")) {
          const rc = c.getBoundingClientRect();
          return { l: rc.left, t: rc.top, w: rc.width, h: rc.height, r, col, cols };
        }
      }
      return null;
    });
    ok("found a Queens cell with a free right neighbour", !!info);
    if (info) {
      const glyph = (r, c) => page.evaluate(({ r, c, cols }) => {
        const all = [...document.querySelectorAll(".queens-cell")];
        return all[r * cols + c].querySelector(".cell-glyph").textContent;
      }, { r, c, cols: info.cols });
      const sx = info.l + info.w - 2, sy = info.t + info.h / 2; // 2px inside the right edge
      await pointerStroke(page, sx, sy, 6, 0, 101); // 6px < 10px slop, crosses the border
      await page.waitForTimeout(40);
      const a1 = await glyph(info.r, info.col), nb1 = await glyph(info.r, info.col + 1);
      ok("jittery boundary tap #1 cycles anchor to X (not a drag)", a1 === "✕", `anchor=${a1}`);
      ok("neighbour NOT stray-painted", nb1 === "", `neighbour=${nb1}`);
      await pointerStroke(page, sx, sy, 6, 0, 102);
      await page.waitForTimeout(40);
      const a2 = await glyph(info.r, info.col);
      ok("jittery boundary tap #2 reaches a queen", a2 === "♛", `anchor=${a2}`);
      // A real drag (well past slop, across several cells) still paints marks.
      const marks = await page.evaluate(() => {
        const grid = document.querySelector(".queens-grid");
        const all = [...grid.querySelectorAll(".queens-cell")].filter((c) => !c.classList.contains("given"));
        const c0 = all[all.length - 1].getBoundingClientRect();
        const sx = c0.left + c0.width / 2, sy = c0.top + c0.height / 2, w = c0.width;
        const o = (cx, cy) => ({ clientX: cx, clientY: cy, bubbles: true, cancelable: true, pointerId: 103, pointerType: "touch", button: 0, isPrimary: true });
        grid.dispatchEvent(new PointerEvent("pointerdown", o(sx, sy)));
        for (let i = 1; i <= 6; i++) window.dispatchEvent(new PointerEvent("pointermove", o(sx - i * w, sy)));
        window.dispatchEvent(new PointerEvent("pointerup", o(sx - 6 * w, sy)));
        return [...grid.querySelectorAll(".queens-cell .cell-glyph")].filter((g) => g.textContent === "✕").length;
      });
      ok("a real drag still paints multiple X marks", marks >= 2, `marks=${marks}`);
    }

    ok("no uncaught page errors", pageErrors.length === 0, pageErrors.join(" | "));
  } finally {
    await browser.close();
    server.close();
  }

  const failed = results.filter((r) => !r.pass);
  console.log(`\n${results.length - failed.length}/${results.length} checks passed`);
  process.exit(failed.length ? 1 : 0);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
