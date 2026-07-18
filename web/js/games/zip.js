// web/js/games/zip.js
//
// Zip: R x C grid, draw one continuous path through every cell, passing
// through numbered waypoints 1..K in ascending order, moving only
// orthogonally, never crossing a wall. Rule checking (revisit,
// non-adjacent-step, wall-crossing, waypoint-order, wrong-start) is
// entirely the engine's job via api.violations/api.solved -- this module
// only renders board.waypoints/hWalls/vWalls and mutates board.path (the
// one field the bridge reads back), per web/js/api.md's Zip section and
// web/js/games/README.md's module contract.
//
// The defining interaction (per docs/plan/docs/03-tui-design.md) is a
// pointer drag from the path's current end through orthogonally adjacent
// cells; per the module README, a drag surface like this is wired with a
// hand-rolled pointerdown/move/up state machine rather than
// api.bindPointer (that helper is for "tap a single cell" games).
//
// The path is rendered as a continuous line via an absolutely-positioned
// SVG overlay (a polyline through cell centers, in viewBox units so it
// scales with the grid); waypoint numbers are bold text baked into each
// cell; walls are short bars pinned to the boundary between two cells.
//
// Every pointerdown/move resolves its cell via api.cellAt (rect math)
// rather than elementFromPoint/event.target, per web/js/games/README.md's
// "Pointer/touch: dead zones" -- a fast drag has no gap to fall through.

export const id = "zip";
const HINT_PULSE_MS = 900;

let stylesInjected = false;
function ensureStyles() {
  if (stylesInjected || document.getElementById("zip-module-styles")) {
    stylesInjected = true;
    return;
  }
  const style = document.createElement("style");
  style.id = "zip-module-styles";
  style.textContent = `
    .zip-grid { touch-action: none; }
    .zip-cell { position: relative; }
    .zip-num { position: relative; z-index: 4; font-weight: 800; pointer-events: none; }
    .zip-cell.on-path { background: color-mix(in srgb, var(--accent) 16%, var(--surface)); }
    .zip-cell.path-start, .zip-cell.path-end {
      outline: 2px solid var(--accent);
      outline-offset: -2px;
    }
    .zip-cell.cursor.pen { box-shadow: inset 0 0 0 3px var(--warning); }
    .zip-wall-bar { position: absolute; background: var(--text); pointer-events: none; z-index: 5; }
    .zip-wall-bar.wall-right { top: 0; bottom: 0; right: -2px; width: 4px; }
    .zip-wall-bar.wall-bottom { left: 0; right: 0; bottom: -2px; height: 4px; }
    .zip-path-line { fill: none; stroke: var(--accent); stroke-width: 0.16; stroke-linecap: round; stroke-linejoin: round; }
  `;
  document.head.appendChild(style);
  stylesInjected = true;
}

export function create(container, api, bundle) {
  ensureStyles();

  const board = bundle.board;
  const rows = board.rows;
  const cols = board.cols;
  const path = board.path; // mutated in place; same reference sent to api.*

  let cursor = initialCursor();
  let penDown = false;
  let dragging = false;

  container.innerHTML = "";
  const shell = document.createElement("div");
  shell.className = "board-shell";

  const wrap = document.createElement("div");
  wrap.className = "puzzle-grid zip-wrap";
  wrap.style.position = "relative";
  wrap.style.display = "block"; // override .puzzle-grid's display:grid -- cells live in the nested grid below

  const grid = document.createElement("div");
  grid.className = "zip-grid";
  grid.style.display = "grid";
  grid.style.gridTemplateColumns = `repeat(${cols}, 1fr)`;
  grid.style.gridTemplateRows = `repeat(${rows}, 1fr)`;
  grid.style.width = "100%";
  grid.style.height = "100%";
  grid.setAttribute("role", "grid");
  grid.setAttribute("aria-label", "Zip puzzle board");

  const cellEls = [];
  for (let r = 0; r < rows; r++) {
    const rowEls = [];
    for (let c = 0; c < cols; c++) {
      const cell = document.createElement("button");
      cell.type = "button";
      cell.className = "cell zip-cell";
      cell.dataset.row = String(r);
      cell.dataset.col = String(c);
      cell.setAttribute("role", "gridcell");

      const wp = board.waypoints[r][c];
      if (wp) {
        const num = document.createElement("span");
        num.className = "zip-num";
        num.textContent = String(wp);
        cell.appendChild(num);
      }

      if (c < cols - 1 && board.hWalls[r][c]) {
        const bar = document.createElement("span");
        bar.className = "zip-wall-bar wall-right";
        bar.setAttribute("aria-hidden", "true");
        cell.appendChild(bar);
      }
      if (r < rows - 1 && board.vWalls[r][c]) {
        const bar = document.createElement("span");
        bar.className = "zip-wall-bar wall-bottom";
        bar.setAttribute("aria-hidden", "true");
        cell.appendChild(bar);
      }

      grid.appendChild(cell);
      rowEls.push(cell);
    }
    cellEls.push(rowEls);
  }
  wrap.appendChild(grid);

  const svgNS = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(svgNS, "svg");
  svg.setAttribute("viewBox", `0 0 ${cols} ${rows}`);
  svg.setAttribute("preserveAspectRatio", "none");
  svg.style.position = "absolute";
  svg.style.inset = "0";
  svg.style.width = "100%";
  svg.style.height = "100%";
  svg.style.pointerEvents = "none";
  svg.style.zIndex = "3";
  const pathLine = document.createElementNS(svgNS, "polyline");
  pathLine.setAttribute("class", "zip-path-line");
  svg.appendChild(pathLine);
  wrap.appendChild(svg);

  shell.appendChild(wrap);
  container.appendChild(shell);

  grid.addEventListener("pointerdown", onPointerDown);

  render();
  runChecks();

  function initialCursor() {
    if (path.length > 0) {
      const last = path[path.length - 1];
      return { row: last.row, col: last.col };
    }
    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        if (board.waypoints[r][c] === 1) return { row: r, col: c };
      }
    }
    return { row: 0, col: 0 };
  }

  function key(r, c) {
    return r + "," + c;
  }

  function pathIndexOf(r, c) {
    return path.findIndex((p) => p.row === r && p.col === c);
  }

  function samePos(a, b) {
    return a.row === b.row && a.col === b.col;
  }

  function isAdjacent(a, b) {
    return (
      (a.row === b.row && Math.abs(a.col - b.col) === 1) ||
      (a.col === b.col && Math.abs(a.row - b.row) === 1)
    );
  }

  function wallBetween(r1, c1, r2, c2) {
    if (r1 === r2) return Boolean(board.hWalls[r1][Math.min(c1, c2)]);
    if (c1 === c2) return Boolean(board.vWalls[Math.min(r1, r2)][c1]);
    return false;
  }

  function pushCell(r, c) {
    path.push({ row: r, col: c });
  }

  function onPointerDown(ev) {
    if (typeof ev.button === "number" && ev.button > 0) return;
    const { row, col } = api.cellAt(grid, rows, cols, ev.clientX, ev.clientY);
    try {
      grid.setPointerCapture(ev.pointerId);
    } catch {
      /* not every pointer type supports capture -- harmless if it throws */
    }

    if (path.length === 0) {
      pushCell(row, col);
    } else {
      const idx = pathIndexOf(row, col);
      if (idx !== -1) {
        path.length = idx + 1; // pressing back onto the path truncates here
      } else {
        const last = path[path.length - 1];
        if (!isAdjacent(last, { row, col }) || wallBetween(last.row, last.col, row, col)) {
          return; // not a legal place to (re)start dragging from
        }
        pushCell(row, col);
      }
    }

    penDown = true;
    dragging = true;
    cursor = { row, col };
    ev.preventDefault();
    render();
    runChecks();

    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", onPointerUp);
    window.addEventListener("pointercancel", onPointerUp);
  }

  function onPointerMove(ev) {
    if (!dragging || path.length === 0) return;
    const { row, col } = api.cellAt(grid, rows, cols, ev.clientX, ev.clientY);
    const last = path[path.length - 1];
    if (row === last.row && col === last.col) return;

    if (path.length >= 2 && samePos(path[path.length - 2], { row, col })) {
      path.pop();
      cursor = { row, col };
      render();
      runChecks();
      return;
    }

    const idx = pathIndexOf(row, col);
    if (idx !== -1) {
      if (idx < path.length - 1) {
        path.length = idx + 1;
        cursor = { row, col };
        render();
        runChecks();
      }
      return;
    }

    if (!isAdjacent(last, { row, col }) || wallBetween(last.row, last.col, row, col)) return;

    pushCell(row, col);
    cursor = { row, col };
    render();
    runChecks();
  }

  function onPointerUp() {
    dragging = false;
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    window.removeEventListener("pointercancel", onPointerUp);
  }

  function togglePen() {
    if (!penDown) {
      penDown = true;
      if (path.length === 0) {
        pushCell(cursor.row, cursor.col);
        runChecks();
      } else {
        cursor = { ...path[path.length - 1] };
      }
    } else {
      penDown = false;
    }
    render();
  }

  function stepPen(delta) {
    const nr = clamp(cursor.row + delta.dr, 0, rows - 1);
    const nc = clamp(cursor.col + delta.dc, 0, cols - 1);
    if (nr === cursor.row && nc === cursor.col) return;

    if (path.length === 0) {
      cursor = { row: nr, col: nc };
      render();
      return;
    }

    if (path.length >= 2 && samePos(path[path.length - 2], { row: nr, col: nc })) {
      path.pop();
      cursor = { row: nr, col: nc };
      render();
      runChecks();
      return;
    }

    const last = path[path.length - 1];
    if (!isAdjacent(last, { row: nr, col: nc })) return;
    if (wallBetween(last.row, last.col, nr, nc)) return;
    if (pathIndexOf(nr, nc) !== -1) return; // already used elsewhere in the path

    pushCell(nr, nc);
    cursor = { row: nr, col: nc };
    render();
    runChecks();
  }

  function eraseLast() {
    if (path.length === 0) return;
    path.pop();
    if (path.length > 0) cursor = { ...path[path.length - 1] };
    render();
    runChecks();
  }

  function render() {
    const idxByPos = new Map();
    path.forEach((p, i) => idxByPos.set(key(p.row, p.col), i));

    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        const cellEl = cellEls[r][c];
        const pos = idxByPos.get(key(r, c));
        const onPath = pos !== undefined;
        const isCursor = cursor.row === r && cursor.col === c;
        cellEl.classList.toggle("on-path", onPath);
        cellEl.classList.toggle("path-start", onPath && pos === 0);
        cellEl.classList.toggle("path-end", onPath && pos === path.length - 1);
        cellEl.classList.toggle("cursor", isCursor);
        cellEl.classList.toggle("pen", isCursor && penDown);
      }
    }

    const points = path.map((p) => `${p.col + 0.5},${p.row + 0.5}`).join(" ");
    pathLine.setAttribute("points", points);
  }

  async function runChecks() {
    const viols = await api.violations(board);
    const bad = new Set();
    for (const v of viols) {
      for (const cell of v.cells) bad.add(key(cell.row, cell.col));
    }
    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        cellEls[r][c].classList.toggle("invalid", bad.has(key(r, c)));
      }
    }
    if (await api.solved(board)) {
      api.onSolved();
    }
  }

  function handleKey(event) {
    // Checked before api.cursorMove -- see games/tango.js's handleKey for
    // why (lowercase `h` is vim-motion "left"; Shift+H is the hint
    // shortcut).
    if (event.shiftKey && (event.key === "h" || event.key === "H")) {
      event.preventDefault();
      performHint();
      return true;
    }

    const delta = api.cursorMove(event);
    if (delta) {
      event.preventDefault();
      if (penDown) {
        stepPen(delta);
      } else {
        cursor = {
          row: clamp(cursor.row + delta.dr, 0, rows - 1),
          col: clamp(cursor.col + delta.dc, 0, cols - 1),
        };
        render();
      }
      return true;
    }

    if (event.code === "Space" || event.key === " ") {
      event.preventDefault();
      if (event.shiftKey) eraseLast();
      else togglePen();
      return true;
    }

    if (event.key === "Backspace") {
      event.preventDefault();
      eraseLast();
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

  // See web/js/games/README.md's "Hints" section. apply.path is the full
  // replacement path (board.path always round-trips as a whole array, per
  // api.md) -- mutate the existing `path` array in place rather than
  // reassigning board.path, since `path` above is an alias every other
  // function in this module already closes over.
  async function performHint() {
    const result = await api.hint(board);
    if (!result) return null;
    if (result.done) {
      api.onHint(result.message);
      return result;
    }
    path.length = 0;
    for (const p of result.apply.path) path.push({ row: p.row, col: p.col });
    penDown = true;
    if (path.length > 0) cursor = { ...path[path.length - 1] };
    render();
    pulseHintCells(result.cells);
    await runChecks();
    api.onHint(result.message);
    return result;
  }

  function destroy() {
    grid.removeEventListener("pointerdown", onPointerDown);
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    window.removeEventListener("pointercancel", onPointerUp);
    container.innerHTML = "";
  }

  return { handleKey, destroy, hint: performHint };
}

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
}
