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

export const id = "queens";

const REGION_TUNED = 6; // how many hand-tuned colors CSS provides (--region-0..5)

export function create(container, api, bundle) {
  const board = bundle.board;
  const n = board.n;

  let cursor = { row: 0, col: 0 };
  const marks = Array.from({ length: n }, () => Array(n).fill(false));
  const unbindFns = [];

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

      unbindFns.push(
        api.bindPointer(cell, {
          onPrimary: () => {
            setCursor(r, c);
            applyPrimary(r, c);
          },
          onSecondary: () => {
            setCursor(r, c);
            applySecondary(r, c);
          },
        })
      );
    }
    cellEls.push(rowEls);
  }

  container.appendChild(grid);

  render();
  runChecks();

  function setCursor(r, c) {
    cursor = { row: r, col: c };
    render();
  }

  // Primary = toggle the UI-only "X" mark (never touches board.cells).
  function applyPrimary(r, c) {
    if (board.givens[r][c]) return;
    marks[r][c] = !marks[r][c];
    render();
  }

  // Secondary = toggle an actual queen (the only thing violations/solved
  // ever look at for this game).
  function applySecondary(r, c) {
    if (board.givens[r][c]) return;
    board.cells[r][c] = board.cells[r][c] === 1 ? 0 : 1;
    render();
    runChecks();
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
      if (event.shiftKey) applySecondary(cursor.row, cursor.col);
      else applyPrimary(cursor.row, cursor.col);
      return true;
    }

    if (event.key === "x" || event.key === "X") {
      event.preventDefault();
      applyPrimary(cursor.row, cursor.col);
      return true;
    }

    return false;
  }

  function destroy() {
    for (const unbind of unbindFns) unbind();
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
