// web/js/games/queens.js
//
// Queens: NxN grid (N varies per puzzle), divided into N colored regions.
// Exactly one queen per row/column/region, no two queens 8-adjacent. Rule
// checking is entirely api.violations/api.solved's job.
//
// "X" marks are explicitly a UI-only concept (see web/js/api.md's Queens
// section) -- they never touch board.cells, they live in this module's own
// `marks` array. Region color is a cosmetic channel only; the *required*
// accessibility channel is the heavier region-boundary border (always on)
// plus a per-region letter tag, per docs/plan/docs/08-theme-style-guide.md.
//
// Pointer/touch interaction: the keyboard keeps the documented two-handed
// scheme (Space places/clears an X mark, Shift+Space places/clears a queen,
// `x` is the X fallback -- see applyMarkToggle/applyQueenToggle below,
// wired verbatim into handleKey so keyboard play never regresses). Pointer
// input (mouse *and* touch) instead follows LinkedIn's mobile Queens model,
// which needs its own hand-rolled pointerdown/move/up state machine rather
// than api.bindPointer (per web/js/games/README.md, that helper is only for
// "tap a single cell" games -- Queens' defining mouse/touch interaction is a
// drag):
//   - tap (press+release with no movement) cycles a cell
//     empty -> X -> queen -> empty.
//   - press-and-drag paints X marks across every non-given, non-queen cell
//     the pointer crosses (mirrors LinkedIn's drag-to-mark-X), independent
//     of any single cell's tap-cycle.
//   - long-press (~500ms), right-click, or shift-click clears a cell
//     (both its X mark and any queen) outright.

export const id = "queens";

const REGION_TUNED = 6; // how many hand-tuned colors CSS provides (--region-0..5)
const LONG_PRESS_MS = 500;

let stylesInjected = false;
function ensureStyles() {
  if (stylesInjected || document.getElementById("queens-module-styles")) {
    stylesInjected = true;
    return;
  }
  const style = document.createElement("style");
  style.id = "queens-module-styles";
  style.textContent = `
    /* The board is a drag surface (drag-to-paint X marks), so it must not
       hand touch gestures to the browser for scrolling -- ONLY the board,
       the rest of the page still scrolls normally. */
    .queens-grid { touch-action: none; }
  `;
  document.head.appendChild(style);
  stylesInjected = true;
}

export function create(container, api, bundle) {
  ensureStyles();

  const board = bundle.board;
  const n = board.n;

  let cursor = { row: 0, col: 0 };
  const marks = Array.from({ length: n }, () => Array(n).fill(false));

  // Drag-to-paint state, see the header comment for the full interaction.
  let dragAnchor = null; // {row, col} while a pointer is down on a non-given cell
  let dragMoved = false;
  let dragPaintValue = false;
  let longPressTimer = null;
  let longPressFired = false;

  container.innerHTML = "";
  const grid = document.createElement("div");
  grid.className = "puzzle-grid queens-grid";
  grid.style.gridTemplateColumns = `repeat(${n}, 1fr)`;
  grid.style.gridTemplateRows = `repeat(${n}, 1fr)`;
  grid.setAttribute("role", "grid");
  grid.setAttribute("aria-label", "Queens puzzle board");

  const cellEls = [];
  for (let r = 0; r < n; r++) {
    const rowEls = [];
    for (let c = 0; c < n; c++) {
      const cell = document.createElement("button");
      cell.type = "button";
      cell.className = "cell queens-cell";
      const region = board.regions[r][c];
      cell.dataset.row = String(r);
      cell.dataset.col = String(c);
      cell.style.background = regionColor(region);
      if (region >= REGION_TUNED) {
        // The hue-rotation fallback (see regionColor()) picks a mid-dark
        // tone that isn't theme-aware, so pin readable text on top of it
        // rather than relying on --text (which flips light/dark).
        cell.style.color = "#f2f3f5";
      }
      cell.setAttribute("role", "gridcell");
      if (board.givens[r][c]) cell.classList.add("given");
      if (c < n - 1 && board.regions[r][c] !== board.regions[r][c + 1]) {
        cell.classList.add("region-edge-right");
      }
      if (r < n - 1 && board.regions[r][c] !== board.regions[r + 1][c]) {
        cell.classList.add("region-edge-bottom");
      }

      const tag = document.createElement("span");
      tag.className = "region-tag";
      tag.textContent = regionLetter(board.regions[r][c]);
      tag.setAttribute("aria-hidden", "true");
      cell.appendChild(tag);

      const glyph = document.createElement("span");
      glyph.className = "cell-glyph";
      cell.appendChild(glyph);

      grid.appendChild(cell);
      rowEls.push({ el: cell, glyph });
    }
    cellEls.push(rowEls);
  }

  container.appendChild(grid);

  grid.addEventListener("pointerdown", onPointerDown);
  grid.addEventListener("contextmenu", onContextMenu);

  render();
  runChecks();

  function setCursor(r, c) {
    cursor = { row: r, col: c };
    render();
  }

  // Clears both the X mark and the queen -- long-press/right-click/
  // shift-click's "secondary" action.
  function clearCellState(r, c) {
    if (board.givens[r][c]) return;
    board.cells[r][c] = 0;
    marks[r][c] = false;
    render();
    runChecks();
  }

  // Tap-cycle: empty -> X -> queen -> empty.
  function cycleCellState(r, c) {
    if (board.givens[r][c]) return;
    if (board.cells[r][c] === 1) {
      board.cells[r][c] = 0;
      marks[r][c] = false;
    } else if (marks[r][c]) {
      board.cells[r][c] = 1;
      marks[r][c] = false;
    } else {
      marks[r][c] = true;
    }
    render();
    runChecks();
  }

  // Drag-paint: sets (or clears) just the X mark, never touches a queen
  // cell -- a drag stroke shouldn't destructively overwrite a placed queen.
  function paintMark(r, c, value) {
    if (board.givens[r][c] || board.cells[r][c] === 1) return;
    if (marks[r][c] === value) return;
    marks[r][c] = value;
    render();
  }

  // Keyboard primary/secondary: exactly the two-handed scheme from
  // web/js/api.md -- Space (and the `x` fallback) toggles the X mark,
  // Shift+Space toggles the queen. Kept exactly as before so physical-
  // keyboard play never regresses.
  function applyMarkToggle(r, c) {
    if (board.givens[r][c]) return;
    marks[r][c] = !marks[r][c];
    render();
  }

  function applyQueenToggle(r, c) {
    if (board.givens[r][c]) return;
    board.cells[r][c] = board.cells[r][c] === 1 ? 0 : 1;
    render();
    runChecks();
  }

  function clearLongPressTimer() {
    if (longPressTimer) {
      window.clearTimeout(longPressTimer);
      longPressTimer = null;
    }
  }

  function cellAtPoint(x, y) {
    const el = document.elementFromPoint(x, y);
    const cellEl = el && el.closest ? el.closest(".queens-cell") : null;
    if (!cellEl) return null;
    return { row: Number(cellEl.dataset.row), col: Number(cellEl.dataset.col) };
  }

  function endDrag() {
    clearLongPressTimer();
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    window.removeEventListener("pointercancel", onPointerCancel);
    dragAnchor = null;
    dragMoved = false;
  }

  function onPointerDown(ev) {
    if (typeof ev.button === "number" && ev.button > 0) return;
    const cellEl = ev.target.closest(".queens-cell");
    if (!cellEl) return;
    const row = Number(cellEl.dataset.row);
    const col = Number(cellEl.dataset.col);
    if (board.givens[row][col]) return;

    ev.preventDefault();
    setCursor(row, col);

    if (ev.shiftKey) {
      clearCellState(row, col);
      return;
    }

    try {
      cellEl.setPointerCapture(ev.pointerId);
    } catch {
      /* not every pointer type supports capture -- harmless if it throws */
    }

    dragAnchor = { row, col };
    dragMoved = false;
    dragPaintValue = !marks[row][col];
    longPressFired = false;
    clearLongPressTimer();
    longPressTimer = window.setTimeout(() => {
      longPressFired = true;
      clearCellState(row, col);
    }, LONG_PRESS_MS);

    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", onPointerUp);
    window.addEventListener("pointercancel", onPointerCancel);
  }

  function onPointerMove(ev) {
    if (!dragAnchor) return;
    const hit = cellAtPoint(ev.clientX, ev.clientY);
    if (!hit) return;
    if (hit.row === dragAnchor.row && hit.col === dragAnchor.col && !dragMoved) return;

    if (!dragMoved) {
      dragMoved = true;
      clearLongPressTimer();
      paintMark(dragAnchor.row, dragAnchor.col, dragPaintValue);
    }
    setCursor(hit.row, hit.col);
    paintMark(hit.row, hit.col, dragPaintValue);
  }

  function onPointerUp() {
    const anchor = dragAnchor;
    const moved = dragMoved;
    const wasLongPress = longPressFired;
    endDrag();
    // A drag only ever paints X marks (UI-only, no engine representation --
    // see the header comment), so there's nothing to re-check with the
    // engine unless this was a plain tap-cycle, which mutates board.cells.
    if (anchor && !moved && !wasLongPress) {
      cycleCellState(anchor.row, anchor.col);
    }
  }

  function onPointerCancel() {
    endDrag();
  }

  function onContextMenu(ev) {
    const cellEl = ev.target.closest(".queens-cell");
    if (!cellEl) return;
    ev.preventDefault();
    const row = Number(cellEl.dataset.row);
    const col = Number(cellEl.dataset.col);
    endDrag();
    clearCellState(row, col);
  }

  function render() {
    for (let r = 0; r < n; r++) {
      for (let c = 0; c < n; c++) {
        const { el: cellEl, glyph } = cellEls[r][c];
        const hasQueen = board.cells[r][c] === 1;
        glyph.textContent = hasQueen ? "♛" : marks[r][c] ? "✕" : "";
        glyph.classList.toggle("queen", hasQueen);
        glyph.classList.toggle("mark", !hasQueen && marks[r][c]);
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
    for (let r = 0; r < n; r++) {
      for (let c = 0; c < n; c++) {
        cellEls[r][c].el.classList.toggle("invalid", bad.has(r + "," + c));
      }
    }
    if (await api.solved(board)) {
      api.onSolved();
    }
  }

  function handleKey(event) {
    const delta = api.cursorMove(event);
    if (delta) {
      event.preventDefault();
      cursor = {
        row: clamp(cursor.row + delta.dr, 0, n - 1),
        col: clamp(cursor.col + delta.dc, 0, n - 1),
      };
      render();
      return true;
    }

    if (event.code === "Space" || event.key === " ") {
      event.preventDefault();
      if (event.shiftKey) applyQueenToggle(cursor.row, cursor.col);
      else applyMarkToggle(cursor.row, cursor.col);
      return true;
    }

    if (event.key === "x" || event.key === "X") {
      event.preventDefault();
      applyMarkToggle(cursor.row, cursor.col);
      return true;
    }

    return false;
  }

  function destroy() {
    endDrag();
    grid.removeEventListener("pointerdown", onPointerDown);
    grid.removeEventListener("contextmenu", onContextMenu);
    container.innerHTML = "";
  }

  return { handleKey, destroy };
}

// Indices 0-5 use the hand-tuned, colorblind-checked palette from the theme
// style guide (CSS vars --region-0..5). Larger boards (Queens' N can reach
// 11) fall back to a hue rotation -- a reasonable-looking but NOT
// colorblind-reviewed extension; the region border + letter tag (always on,
// regardless of index) are what actually carries the distinction, per
// docs/plan/docs/08-theme-style-guide.md's accessibility notes.
function regionColor(idx) {
  if (idx < REGION_TUNED) return `var(--region-${idx})`;
  const hue = (idx * 47) % 360;
  return `hsl(${hue} 45% 42%)`;
}

function regionLetter(idx) {
  return String.fromCharCode(65 + (idx % 26));
}

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
}
