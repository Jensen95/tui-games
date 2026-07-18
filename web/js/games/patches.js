// web/js/games/patches.js
//
// Patches: fixed 5x5 grid, partition into one axis-aligned rectangle per
// clue (matching the clue's cell-count and shape). Rule checking
// (exact-cover, one-clue, area, shape) is entirely the engine's job via
// api.violations/api.solved -- this module only renders board.clues and
// mutates board.labels (the one field the bridge reads back), per
// web/js/api.md's Patches section and web/js/games/README.md's module
// contract.
//
// The defining interaction (per docs/plan/docs/03-tui-design.md) is:
// press a clue cell and drag to the opposite corner to preview a
// rectangle (live outline), release commits it; clicking a placed
// rectangle removes it. Per the module README, a drag surface like this
// is wired with a hand-rolled pointerdown/move/up state machine rather
// than api.bindPointer (that helper is for "tap a single cell" games).
//
// Touch-friendly alternative to dragging: a plain tap-and-release with no
// movement anchors a rectangle without committing it (a 1-cell dashed
// preview stays visible); a second, separate tap elsewhere names the
// opposite corner and commits (or, tapping the same anchor cell again,
// confirms a 1x1 rectangle). Tapping a covered cell while a two-tap
// rectangle is pending abandons the pending anchor and removes that
// rectangle instead. Dragging remains the primary gesture and always
// commits immediately on release.
//
// `labels` values are opaque per api.md -- any integer not currently used
// by another rectangle works, so this module hands out a simple ever-
// incrementing counter.
//
// A rectangle's drag anchor is NOT required to be its clue's own cell:
// internal/games/patches/generator.go's deriveClues() places each clue at
// a *uniformly random* cell inside its rectangle, not necessarily a
// corner. Since a two-point drag's bounding box always has the anchor at
// one of its four corners (a plain fact of bounding-box geometry), forcing
// the press to land exactly on the clue would make any placement where
// the clue isn't a corner of its own rectangle unreachable by dragging.
// So: any uncovered cell can start a rectangle; the engine's own
// exact-cover/one-clue checks (via api.violations/api.solved) are what
// confirm the player put the right clue inside it, exactly like every
// other rule in this game. For cosmetic region coloring (mirrors
// games/queens.js's regionColor()), each placed rectangle looks up which
// clue (if any) its label's bounding box actually contains.
//
// Every pointerdown/move resolves its cell via api.cellAt (rect math)
// rather than elementFromPoint/event.target, per web/js/games/README.md's
// "Pointer/touch: dead zones" -- a fast drag has no gap to fall through.

export const id = "patches";
const HINT_PULSE_MS = 900;

const REGION_TUNED = 6; // how many hand-tuned colors CSS provides (--region-0..5)
const SHAPES = ["square", "wide", "tall", "free"];
const SHAPE_LABELS = { square: "Square", wide: "Wide", tall: "Tall", free: "Free" };

let stylesInjected = false;
function ensureStyles() {
  if (stylesInjected || document.getElementById("patches-module-styles")) {
    stylesInjected = true;
    return;
  }
  const style = document.createElement("style");
  style.id = "patches-module-styles";
  style.textContent = `
    .patches-cell { touch-action: none; position: relative; }
    .patches-clue {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 0.15em;
      pointer-events: none;
      position: relative;
      z-index: 4;
    }
    .clue-area { font-weight: 800; line-height: 1; }
    .shape-swatch { display: inline-block; background: currentColor; opacity: 0.85; }
    .shape-swatch.shape-square { width: 0.55em; height: 0.55em; }
    .shape-swatch.shape-wide { width: 0.95em; height: 0.42em; }
    .shape-swatch.shape-tall { width: 0.42em; height: 0.95em; }
    .shape-swatch.shape-free { width: 0.55em; height: 0.55em; border-radius: 50%; }
    .patches-preview {
      position: absolute;
      box-sizing: border-box;
      border: 2px dashed var(--accent);
      border-radius: 4px;
      pointer-events: none;
      z-index: 6;
      display: none;
    }
    .patches-legend {
      display: flex;
      flex-wrap: wrap;
      gap: 0.9rem;
      justify-content: center;
      width: min(100%, 26rem);
      font-size: 0.8rem;
      color: var(--dim);
    }
    .patches-legend .legend-item { display: flex; align-items: center; gap: 0.4em; }
    .patches-legend .shape-swatch { color: var(--text); }
  `;
  document.head.appendChild(style);
  stylesInjected = true;
}

export function create(container, api, bundle) {
  ensureStyles();

  const board = bundle.board;
  const rows = board.rows;
  const cols = board.cols;

  const clueIndex = Array.from({ length: rows }, () => Array(cols).fill(null));
  board.clues.forEach((clue, i) => {
    clueIndex[clue.row][clue.col] = i;
  });

  let cursor = { row: 0, col: 0 };
  let anchor = null; // {row, col} once a rectangle is being defined
  let dragCurrent = null; // {row, col}, the free corner while defining
  let dragging = false; // true only while the pointer is physically down
  // Two-tap rectangle mode (a touch-friendly alternative to drag): a plain
  // tap-and-release with no movement anchors a rectangle without
  // committing it; `awaitingSecondTap` is true while we're waiting for a
  // second, separate tap to name the opposite corner (or re-tap the same
  // cell to confirm a 1x1). See onPointerDown/onPointerUp below.
  let awaitingSecondTap = false;
  let labelCounter = 0; // next fresh label id to hand out on commit

  container.innerHTML = "";
  const shell = document.createElement("div");
  shell.className = "board-shell";

  const wrap = document.createElement("div");
  wrap.className = "puzzle-grid patches-wrap";
  wrap.style.position = "relative";
  wrap.style.display = "block"; // override .puzzle-grid's display:grid -- cells live in the nested grid below

  const grid = document.createElement("div");
  grid.className = "patches-grid";
  grid.style.display = "grid";
  grid.style.gridTemplateColumns = `repeat(${cols}, 1fr)`;
  grid.style.gridTemplateRows = `repeat(${rows}, 1fr)`;
  grid.style.width = "100%";
  grid.style.height = "100%";
  grid.setAttribute("role", "grid");
  grid.setAttribute("aria-label", "Patches puzzle board");

  const cellEls = [];
  for (let r = 0; r < rows; r++) {
    const rowEls = [];
    for (let c = 0; c < cols; c++) {
      const cell = document.createElement("button");
      cell.type = "button";
      cell.className = "cell patches-cell";
      cell.dataset.row = String(r);
      cell.dataset.col = String(c);
      cell.setAttribute("role", "gridcell");

      const clueIdx = clueIndex[r][c];
      if (clueIdx !== null) {
        const clue = board.clues[clueIdx];
        const wrapEl = document.createElement("span");
        wrapEl.className = "patches-clue";
        const areaEl = document.createElement("span");
        areaEl.className = "clue-area";
        areaEl.textContent = String(clue.area);
        wrapEl.appendChild(areaEl);
        wrapEl.appendChild(shapeSwatch(clue.shape));
        cell.appendChild(wrapEl);
      }

      grid.appendChild(cell);
      rowEls.push(cell);
    }
    cellEls.push(rowEls);
  }
  wrap.appendChild(grid);

  const preview = document.createElement("div");
  preview.className = "patches-preview";
  wrap.appendChild(preview);

  shell.appendChild(wrap);
  shell.appendChild(buildLegend());
  container.appendChild(shell);

  grid.addEventListener("pointerdown", onPointerDown);

  render();
  runChecks();

  function shapeSwatch(shape) {
    const span = document.createElement("span");
    span.className = `shape-swatch shape-${shape}`;
    span.setAttribute("aria-hidden", "true");
    return span;
  }

  function buildLegend() {
    const legend = document.createElement("div");
    legend.className = "patches-legend";
    for (const shape of SHAPES) {
      const item = document.createElement("span");
      item.className = "legend-item";
      item.appendChild(shapeSwatch(shape));
      const label = document.createElement("span");
      label.className = "legend-label";
      label.textContent = SHAPE_LABELS[shape];
      item.appendChild(label);
      legend.appendChild(item);
    }
    return legend;
  }

  // Which clue (if any) currently sits inside this label's placed cells --
  // used only for cosmetic coloring, never for rule checking.
  function ownerClueIndex(label) {
    for (let i = 0; i < board.clues.length; i++) {
      const clue = board.clues[i];
      if (board.labels[clue.row][clue.col] === label) return i;
    }
    return null;
  }

  function normBox(a, b) {
    return {
      r0: Math.min(a.row, b.row),
      r1: Math.max(a.row, b.row),
      c0: Math.min(a.col, b.col),
      c1: Math.max(a.col, b.col),
    };
  }

  function boxOverlapsExisting(box) {
    for (let r = box.r0; r <= box.r1; r++) {
      for (let c = box.c0; c <= box.c1; c++) {
        if (board.labels[r][c] !== -1) return true;
      }
    }
    return false;
  }

  function removeRectangle(label) {
    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        if (board.labels[r][c] === label) board.labels[r][c] = -1;
      }
    }
    render();
    runChecks();
  }

  function tryCommit() {
    if (!anchor) return;
    const box = normBox(anchor, dragCurrent);
    if (boxOverlapsExisting(box)) return; // leave anchor active -- user can reshape/cancel
    const label = labelCounter++;
    for (let r = box.r0; r <= box.r1; r++) {
      for (let c = box.c0; c <= box.c1; c++) {
        board.labels[r][c] = label;
      }
    }
    anchor = null;
    dragCurrent = null;
    awaitingSecondTap = false;
    render();
    runChecks();
  }

  function cancelAnchor() {
    anchor = null;
    dragCurrent = null;
    awaitingSecondTap = false;
    render();
  }

  function onPointerDown(ev) {
    if (typeof ev.button === "number" && ev.button > 0) return;
    const { row, col } = api.cellAt(grid, rows, cols, ev.clientX, ev.clientY);

    if (awaitingSecondTap) {
      // Completing (or redirecting) a two-tap rectangle: this pointerdown
      // is the second, separate tap naming the opposite corner (or re-
      // tapping the anchor cell itself to confirm a 1x1 rectangle).
      ev.preventDefault();
      const label = board.labels[row][col];
      if (label !== -1 && !(row === anchor.row && col === anchor.col)) {
        // Second tap landed on an already-placed rectangle instead --
        // abandon the pending anchor and let the normal "tap a placed
        // rectangle to remove" behavior run.
        cancelAnchor();
        removeRectangle(label);
        return;
      }
      cursor = { row, col };
      dragCurrent = { row, col };
      awaitingSecondTap = false;
      tryCommit();
      return;
    }

    cursor = { row, col };

    const label = board.labels[row][col];
    if (label !== -1) {
      // Tap on a placed rectangle removes it -- no drag involved.
      ev.preventDefault();
      removeRectangle(label);
      return;
    }

    // Any uncovered cell can start a rectangle (not just a clue cell -- see
    // this module's header comment on why the anchor isn't restricted to
    // clue cells).
    try {
      grid.setPointerCapture(ev.pointerId);
    } catch {
      /* not every pointer type supports capture -- harmless if it throws */
    }
    anchor = { row, col };
    dragCurrent = { row, col };
    dragging = true;
    ev.preventDefault();
    render();

    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", onPointerUp);
    window.addEventListener("pointercancel", onPointerCancel);
  }

  function onPointerMove(ev) {
    if (!dragging) return;
    const hit = api.cellAt(grid, rows, cols, ev.clientX, ev.clientY);
    dragCurrent = hit;
    cursor = { ...hit };
    render();
  }

  function onPointerUp() {
    if (!dragging) return;
    dragging = false;
    detachDragListeners();
    const moved = anchor && dragCurrent && (dragCurrent.row !== anchor.row || dragCurrent.col !== anchor.col);
    if (moved) {
      tryCommit();
    } else {
      // A plain tap-and-release with no movement: don't commit a 1x1
      // immediately (that made the two-tap flow below unreachable --
      // pressing to anchor a rectangle would instantly finish it before a
      // second tap could ever land). Leave the anchor active, showing its
      // 1-cell preview, awaiting a second tap elsewhere to name the
      // opposite corner. Dragging stays the primary/immediate gesture.
      awaitingSecondTap = true;
    }
  }

  function onPointerCancel() {
    dragging = false;
    detachDragListeners();
    cancelAnchor();
  }

  function detachDragListeners() {
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    window.removeEventListener("pointercancel", onPointerCancel);
  }

  function activeCell() {
    return anchor ? dragCurrent : cursor;
  }

  function render() {
    const box = anchor ? normBox(anchor, dragCurrent) : null;
    const overlap = box ? boxOverlapsExisting(box) : false;
    const active = activeCell();

    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        const cellEl = cellEls[r][c];
        const label = board.labels[r][c];
        const isCursor = active.row === r && active.col === c;

        const shadows = [];
        if (label !== -1) {
          if (c === 0 || board.labels[r][c - 1] !== label) shadows.push("inset 3px 0 0 var(--border)");
          if (c === cols - 1 || board.labels[r][c + 1] !== label) shadows.push("inset -3px 0 0 var(--border)");
          if (r === 0 || board.labels[r - 1][c] !== label) shadows.push("inset 0 3px 0 var(--border)");
          if (r === rows - 1 || board.labels[r + 1][c] !== label) shadows.push("inset 0 -3px 0 var(--border)");
        }
        if (isCursor) shadows.push("inset 0 0 0 3px var(--accent)");
        cellEl.style.boxShadow = shadows.join(", ");
        cellEl.classList.toggle("cursor", isCursor);
        if (label !== -1) {
          const colorIdx = ownerClueIndex(label);
          const idx = colorIdx !== null ? colorIdx : label;
          cellEl.style.background = regionColor(idx);
          cellEl.style.color = idx >= REGION_TUNED ? "#f2f3f5" : "";
        } else {
          cellEl.style.background = "";
          cellEl.style.color = "";
        }
      }
    }

    if (box) {
      preview.style.display = "block";
      preview.style.left = `${(box.c0 / cols) * 100}%`;
      preview.style.top = `${(box.r0 / rows) * 100}%`;
      preview.style.width = `${((box.c1 - box.c0 + 1) / cols) * 100}%`;
      preview.style.height = `${((box.r1 - box.r0 + 1) / rows) * 100}%`;
      preview.style.borderColor = overlap ? "var(--error)" : "var(--accent)";
    } else {
      preview.style.display = "none";
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

  function primarySpace() {
    if (anchor) {
      tryCommit();
      return;
    }
    if (board.labels[cursor.row][cursor.col] === -1) {
      anchor = { ...cursor };
      dragCurrent = { ...cursor };
      render();
    }
  }

  function cancelOrRemove() {
    if (anchor) {
      cancelAnchor();
      return;
    }
    const label = board.labels[cursor.row][cursor.col];
    if (label !== -1) removeRectangle(label);
  }

  function handleKey(event) {
    // Checked before api.cursorMove -- see games/tango.js's handleKey for
    // why (lowercase `h`/`x` are reserved elsewhere; Shift+H is the hint
    // shortcut).
    if (event.shiftKey && (event.key === "h" || event.key === "H")) {
      event.preventDefault();
      performHint();
      return true;
    }

    const delta = api.cursorMove(event);
    if (delta) {
      event.preventDefault();
      if (anchor) {
        dragCurrent = {
          row: clamp(dragCurrent.row + delta.dr, 0, rows - 1),
          col: clamp(dragCurrent.col + delta.dc, 0, cols - 1),
        };
      } else {
        cursor = {
          row: clamp(cursor.row + delta.dr, 0, rows - 1),
          col: clamp(cursor.col + delta.dc, 0, cols - 1),
        };
      }
      render();
      return true;
    }

    if (event.code === "Space" || event.key === " ") {
      event.preventDefault();
      if (event.shiftKey) cancelOrRemove();
      else primarySpace();
      return true;
    }

    if (event.key === "x" || event.key === "X") {
      event.preventDefault();
      cancelOrRemove();
      return true;
    }

    return false;
  }

  function pulseHintCells(cells) {
    for (const c of cells) {
      const cellEl = cellEls[c.row] && cellEls[c.row][c.col];
      if (!cellEl) continue;
      cellEl.classList.add("hint-pulse");
      window.setTimeout(() => cellEl.classList.remove("hint-pulse"), HINT_PULSE_MS);
    }
  }

  // See web/js/games/README.md's "Hints" section. apply is
  // {r0,c0,r1,c1} -- one rectangle's inclusive bounding box to reveal.
  // Mirrors internal/tui/boards/patches.go's applyHintRect: fully clear any
  // rectangle(s) currently overlapping the box (not just the overlapping
  // cells) before planting a fresh label across the whole box, and abandon
  // any pending anchor (a hint takes priority over an in-progress
  // two-tap/drag rectangle, exactly like tapping a placed rectangle does).
  async function performHint() {
    const result = await api.hint(board);
    if (!result) return null;
    if (result.done) {
      api.onHint(result.message);
      return result;
    }
    const { r0, c0, r1, c1 } = result.apply;
    const stale = new Set();
    for (let r = r0; r <= r1; r++) {
      for (let c = c0; c <= c1; c++) {
        const l = board.labels[r][c];
        if (l !== -1) stale.add(l);
      }
    }
    for (const l of stale) {
      for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
          if (board.labels[r][c] === l) board.labels[r][c] = -1;
        }
      }
    }
    const label = labelCounter++;
    for (let r = r0; r <= r1; r++) {
      for (let c = c0; c <= c1; c++) {
        board.labels[r][c] = label;
      }
    }
    anchor = null;
    dragCurrent = null;
    awaitingSecondTap = false;
    cursor = { row: r0, col: c0 };
    render();
    pulseHintCells(result.cells);
    await runChecks();
    api.onHint(result.message);
    return result;
  }

  function destroy() {
    grid.removeEventListener("pointerdown", onPointerDown);
    detachDragListeners();
    container.innerHTML = "";
  }

  return { handleKey, destroy, hint: performHint };
}

// Indices 0-5 use the hand-tuned, colorblind-checked palette from the theme
// style guide (CSS vars --region-0..5), shared with games/queens.js. Larger
// clue counts fall back to a hue rotation -- a reasonable-looking but NOT
// colorblind-reviewed extension; per
// docs/plan/docs/08-theme-style-guide.md, Patches' fills are cosmetic --
// the numbered clue + shape glyph (always rendered, regardless of fill)
// are the real, required non-color channel.
function regionColor(idx) {
  if (idx < REGION_TUNED) return `var(--region-${idx})`;
  const hue = (idx * 47) % 360;
  return `hsl(${hue} 45% 42%)`;
}

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
}
