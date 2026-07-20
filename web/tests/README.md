# Web end-to-end tests

`e2e.mjs` is a self-contained browser regression suite for the web build (the
WASM PWA under `web/`). It starts its own static server for `web/`, drives a
headless Chromium through [Playwright](https://playwright.dev), and asserts the
behaviours we care about keeping working:

- the WASM engine loads and the menu renders all five games;
- every game starts and renders a board;
- the light/dark **theme toggle** switches, persists across reload, and updates
  the `theme-color` meta;
- the **seed persists** across reloads (it must not reroll on every load);
- **hints** show a "why" explanation and then lock the Hint button for a
  cooldown, and the `Shift+H` shortcut is gated during that cooldown;
- **Queens touch input**: a jittery tap that drifts across a cell border is
  still treated as a tap (cycles one cell) rather than a stray drag-paint, while
  a genuine drag still paints X marks.

## Running

```sh
task test:web            # builds the wasm bridge, then runs the suite
# ...or manually:
GOOS=js GOARCH=wasm go build -o web/lig.wasm ./web/wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/js/wasm_exec.js
node web/tests/e2e.mjs
```

The suite needs Playwright and a Chromium build on the machine. It resolves
Playwright from a local `node_modules` first, then from a global install; a
Chromium build is located via `PLAYWRIGHT_BROWSERS_PATH` (as configured in this
repo's dev environment). Exit code is non-zero if any check fails, and it prints
one `PASS`/`FAIL` line per check plus a summary.

Override the port with `LIG_TEST_PORT` if `8399` is taken.
