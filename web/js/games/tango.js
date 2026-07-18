// web/js/games/tango.js
//
// Tango: 6x6 grid, every cell holds a sun or a moon. Rules (row/column
// balance, no-three-in-a-row, = / x edge constraints) are entirely the
// engine's job -- this module only renders board.cells/board.givens/
// board.hEdges/board.vEdges and calls api.violations/api.solved. See
// web/js/api.md's Tango section and web/js/games/README.md for the module
// contract.
//
// Pointer/touch: the board renders a real 0.6rem gutter track between every
// pair of cells (for the =/x edge-constraint glyphs) -- a tap landing in
// that gutter has no cell element under it at all. So the grid uses a
// single grid-level pointerdown/up listener (see web/js/games/README.md's
// "Pointer/touch: dead zones") resolving every point via api.cellAt rather
// than per-cell listeners, which would silently drop any tap that lands in
// a gutter (or on a cell-boundary sub-pixel seam).

export const id = "tango";

const GLYPH = { 1: "☀", 2: "☽" }; // 1 = sun (☀), 2 = moon (☽)
const CELL_TRACK = "1fr";
const GUTTER_TRACK = "0.6rem";
const LONG_PRESS_MS = 500;
const HINT_PULSE_MS = 900;

export function create(container, api, bundle) {
  const board = bundle.board;
  const rows = board.rows;
  const cols = board.cols;

  let cursor = { row: 0, col: 0 };

  container.innerHTML = "";
  const grid = document.createElement("div");
  grid.className = "puzzle-grid tango-grid";
  grid.style.gridTemplateColumns = trackList(cols);
  grid.style.gridTemplateRows = trackList(rows);
  grid.style.touchAction = "none"; // the drag/tap surface only -- rest of the page keeps scrolling
  grid.setAttribute("role", "grid");
  grid.setAttribute("aria-label", "Tango puzzle board");

  const cellEls = [];
  for (let r = 0; r < rows; r++) {
    const rowEls = [];
    for (let c = 0; c < cols; c++) {
      const cell = document.createElement("button");
      cell.type = "button";
      cell.className = "cell tango-cell";
      cell.style.gridColumn = String(2 * c + 1);
      cell.style.gridRow = String(2 * r + 1);
      cell.dataset.row = String(r);
      cell.dataset.col = String(c);
      cell.setAttribute("role", "gridcell");
      if (board.givens[r][c]) cell.classList.add("given");
      grid.appendChild(cell);
      rowEls.push(cell);
    }
    cellEls.push(rowEls);
  }

  for (let r = 0; r < rows; r++) {
    for (let c = 0; c < cols - 1; c++) {
      const v = board.hEdges[r][c];
      if (!v) continue;
      grid.appendChild(makeEdgeMarker(v, 2 * r + 1, 2 * c + 2));
    }
  }
  for (let r = 0; r < rows - 1; r++) {
    for (let c = 0; c < cols; c++) {
      const v = board.vEdges[r][c];
      if (!v) continue;
      grid.appendChild(makeEdgeMarker(v, 2 * r + 2, 2 * c + 1));
    }
  }

  container.appendChild(grid);

  render();
  runChecks();

  function makeEdgeMarker(value, gridRow, gridCol) {
    const marker = document.createElement("span");
    marker.className = "edge-marker";
    marker.style.gridRow = String(gridRow);
    marker.style.gridColumn = String(gridCol);
    marker.textContent = value === 1 ? "=" : "×";
    marker.setAttribute("aria-hidden", "true");
    return marker;
  }

  function trackList(n) {
    const tracks = [];
    for (let i = 0; i < n; i++) {
      tracks.push(CELL_TRACK);
      if (i < n - 1) tracks.push(GUTTER_TRACK);
    }
    return tracks.join(" ");
  }

  function setCursor(r, c) {
    cursor = { row: r, col: c };
    render();
  }

  // Keyboard primary/secondary: exactly the two-handed scheme from
  // web/js/api.md -- Space toggles sun, Shift+Space (or the `m` fallback)
  // toggles moon. Kept byte-for-byte the same as before so physical-keyboard
  // play never regresses.
  function applySunToggle(r, c) {
    if (board.givens[r][c]) return;
    board.cells[r][c] = board.cells[r][c] === 1 ? 0 : 1;
    render();
    runChecks();
  }

  function applyMoonToggle(r, c) {
    if (board.givens[r][c]) return;
    board.cells[r][c] = board.cells[r][c] === 2 ? 0 : 2;
    render();
    runChecks();
  }

  // Pointer (mouse + touch) interaction intentionally departs from the
  // keyboard's primary/secondary split: a touch user has no Shift key, so
  // the original "tap = sun, shift/long-press/right-click = moon" mapping
  // left moon completely unreachable by touch (the reported bug). Mirroring
  // LinkedIn's mobile Tango, a tap/click now cycles empty -> sun -> moon ->
  // empty; long-press/right-click/shift-click ("secondary", below) clears
  // the cell outright instead of placing moon.
  function applyPointerPrimary(r, c) {
    if (board.givens[r][c]) return;
    const v = board.cells[r][c];
    board.cells[r][c] = v === 0 ? 1 : v === 1 ? 2 : 0;
    render();
    runChecks();
  }

  function applyPointerSecondary(r, c) {
    if (board.givens[r][c]) return;
    board.cells[r][c] = 0;
    render();
    runChecks();
  }

  // Grid-level pointer state machine (see the header comment and
  // web/js/games/README.md's "Pointer/touch: dead zones"): a single
  // pointerdown/up pair on the grid container itself, resolving the
  // touched/clicked cell via api.cellAt (rect math) rather than a listener
  // per cell button -- so a tap landing in the gutter track between two
  // cells (rendered for the =/x edge glyphs) still resolves to whichever
  // cell it's closer to, instead of being silently dropped.
  let pressCell = null;
  let pressTimer = null;
  let longPressed = false;

  function clearPressTimer() {
    if (pressTimer) {
      window.clearTimeout(pressTimer);
      pressTimer = null;
    }
  }

  function endPress() {
    clearPressTimer();
    pressCell = null;
    longPressed = false;
  }

  function onPointerDown(ev) {
    if (typeof ev.button === "number" && ev.button !== 0) return;
    const hit = api.cellAt(grid, rows, cols, ev.clientX, ev.clientY);
    ev.preventDefault();
    pressCell = hit;
    longPressed = false;
    clearPressTimer();
    pressTimer = window.setTimeout(() => {
      longPressed = true;
      setCursor(hit.row, hit.col);
      applyPointerSecondary(hit.row, hit.col);
    }, LONG_PRESS_MS);
  }

  function onPointerUp(ev) {
    const cell = pressCell;
    const wasLongPress = longPressed;
    endPress();
    if (!cell) return;
    if (wasLongPress) return; // the long-press timer already acted
    setCursor(cell.row, cell.col);
    if (ev.shiftKey) applyPointerSecondary(cell.row, cell.col);
    else applyPointerPrimary(cell.row, cell.col);
  }

  function onPointerLeaveOrCancel() {
    endPress();
  }

  function onContextMenu(ev) {
    ev.preventDefault();
    endPress();
    const hit = api.cellAt(grid, rows, cols, ev.clientX, ev.clientY);
    setCursor(hit.row, hit.col);
    applyPointerSecondary(hit.row, hit.col);
  }

  grid.addEventListener("pointerdown", onPointerDown);
  grid.addEventListener("pointerup", onPointerUp);
  grid.addEventListener("pointerleave", onPointerLeaveOrCancel);
  grid.addEventListener("pointercancel", onPointerLeaveOrCancel);
  grid.addEventListener("contextmenu", onContextMenu);

  function pulseHintCells(cells) {
    for (const c of cells) {
      const el = cellEls[c.row] && cellEls[c.row][c.col];
      if (!el) continue;
      el.classList.add("hint-pulse");
      window.setTimeout(() => el.classList.remove("hint-pulse"), HINT_PULSE_MS);
    }
  }

  // See web/js/games/README.md's "Hints" section: apply the one recorded
  // move api.hint() returns, then re-check/re-render exactly like any other
  // move.
  async function performHint() {
    const result = await api.hint(board);
    if (!result) return null;
    if (result.done) {
      api.onHint(result.message);
      return result;
    }
    for (const w of result.apply.cells) {
      board.cells[w.row][w.col] = w.value;
    }
    if (result.cells.length > 0) setCursor(result.cells[0].row, result.cells[0].col);
    render();
    pulseHintCells(result.cells);
    await runChecks();
    api.onHint(result.message);
    return result;
  }

  function render() {
    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        const cellEl = cellEls[r][c];
        const v = board.cells[r][c];
        cellEl.textContent = v === 1 || v === 2 ? GLYPH[v] : "";
        cellEl.classList.toggle("sun", v === 1);
        cellEl.classList.toggle("moon", v === 2);
        cellEl.classList.toggle("cursor", cursor.row === r && cursor.col === c);
      }
    }
  }

  async function runChecks() {
    const viols = await api.violations(board);
    const bad = new Set();
    for (const v of viols) {
      for (const cell of v.cells) bad.add(cell.row + "," + cell.col);
    }
    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        cellEls[r][c].classList.toggle("invalid", bad.has(r + "," + c));
      }
    }
    if (await api.solved(board)) {
      api.onSolved();
    }
  }

  function handleKey(event) {
    // Checked before api.cursorMove: lowercase `h` is vim-motion "left"
    // (api.cursorMove lowercases before its lookup, so it would otherwise
    // swallow *both* `h` and `H` as a cursor move first). Requiring
    // shiftKey here means Shift+H is the hint shortcut and bare `h` is
    // untouched.
    if (event.shiftKey && (event.key === "h" || event.key === "H")) {
      event.preventDefault();
      performHint();
      return true;
    }

    const delta = api.cursorMove(event);
    if (delta) {
      event.preventDefault();
      cursor = {
        row: clamp(cursor.row + delta.dr, 0, rows - 1),
        col: clamp(cursor.col + delta.dc, 0, cols - 1),
      };
      render();
      return true;
    }

    if (event.code === "Space" || event.key === " ") {
      event.preventDefault();
      if (event.shiftKey) applyMoonToggle(cursor.row, cursor.col);
      else applySunToggle(cursor.row, cursor.col);
      return true;
    }

    if (event.key === "m" || event.key === "M") {
      event.preventDefault();
      applyMoonToggle(cursor.row, cursor.col);
      return true;
    }

    return false;
  }

  function destroy() {
    endPress();
    grid.removeEventListener("pointerdown", onPointerDown);
    grid.removeEventListener("pointerup", onPointerUp);
    grid.removeEventListener("pointerleave", onPointerLeaveOrCancel);
    grid.removeEventListener("pointercancel", onPointerLeaveOrCancel);
    grid.removeEventListener("contextmenu", onContextMenu);
    container.innerHTML = "";
  }

  return { handleKey, destroy, hint: performHint };
}

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
}
