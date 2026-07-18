// web/js/games/minisudoku.js
//
// Mini Sudoku: 6x6 grid, digits 1-6, 2x3 boxes. Rule checking is entirely
// api.violations/api.solved's job -- this module only renders board.cells/
// board.givens plus its own local pencil-note state (no engine
// representation, per web/js/api.md's Mini Sudoku section) and forwards
// input.
//
// Unlike Tango/Queens, Sudoku's primary action is inherently digit-based --
// there's no single symbol to tap-cycle on a cell -- so a tap only moves
// the cursor/selection; digits are entered via the keyboard (1-6, Shift+1-6
// for notes) or the on-screen keypad rendered below the grid so the game is
// fully playable on a phone with no physical keyboard.
//
// The board's own tap-to-move-cursor is resolved via a single grid-level
// pointerdown listener + api.cellAt (rect math), not a listener per cell
// button -- see web/js/games/README.md's "Pointer/touch: dead zones". The
// on-screen keypad/tools buttons below the grid have no such dead-zone risk
// (nothing else is ever rendered between them) so they stay plain click
// listeners.

export const id = "minisudoku";
const HINT_PULSE_MS = 900;

export function create(container, api, bundle) {
  const board = bundle.board;
  const rows = board.rows;
  const cols = board.cols;
  const boxRows = board.boxRows;
  const boxCols = board.boxCols;
  // The digit range is read from the board JSON (rows === cols === the
  // puzzle's size) rather than hardcoded, per web/js/games/README.md -- a
  // future clone with a different grid size just works.
  const size = rows;
  const digits = Array.from({ length: size }, (_, i) => i + 1);

  let cursor = { row: 0, col: 0 };
  let noteMode = false;
  const notes = Array.from({ length: rows }, () => Array.from({ length: cols }, () => new Set()));

  container.innerHTML = "";
  const shell = document.createElement("div");
  shell.className = "board-shell";

  const grid = document.createElement("div");
  grid.className = "puzzle-grid sudoku-grid";
  grid.style.gridTemplateColumns = `repeat(${cols}, 1fr)`;
  grid.style.gridTemplateRows = `repeat(${rows}, 1fr)`;
  grid.setAttribute("role", "grid");
  grid.setAttribute("aria-label", "Mini Sudoku puzzle board");

  const cellEls = [];
  for (let r = 0; r < rows; r++) {
    const rowEls = [];
    for (let c = 0; c < cols; c++) {
      const cell = document.createElement("button");
      cell.type = "button";
      cell.className = "cell sudoku-cell";
      cell.dataset.row = String(r);
      cell.dataset.col = String(c);
      cell.setAttribute("role", "gridcell");
      if (board.givens[r][c]) cell.classList.add("given");
      if (c < cols - 1 && (c + 1) % boxCols === 0) cell.classList.add("box-edge-right");
      if (r < rows - 1 && (r + 1) % boxRows === 0) cell.classList.add("box-edge-bottom");

      const notesEl = document.createElement("div");
      notesEl.className = "sudoku-notes";
      // boxCols/boxRows (from the board JSON, not hardcoded) shape the
      // pencil-mark mini-grid so it always matches the puzzle's own box
      // geometry, even if a future clone varies it.
      notesEl.style.gridTemplateColumns = `repeat(${boxCols}, 1fr)`;
      notesEl.style.gridTemplateRows = `repeat(${boxRows}, 1fr)`;
      for (const d of digits) {
        const span = document.createElement("span");
        span.dataset.digit = String(d);
        notesEl.appendChild(span);
      }
      cell.appendChild(notesEl);

      const valueEl = document.createElement("span");
      valueEl.className = "cell-value";
      cell.appendChild(valueEl);

      grid.appendChild(cell);
      rowEls.push({ el: cell, notesEl, valueEl });
    }
    cellEls.push(rowEls);
  }
  shell.appendChild(grid);

  // Grid-level tap-to-move-cursor -- see the header comment. A plain click
  // (or tap) anywhere in the grid resolves to the nearest cell via
  // api.cellAt, never a per-cell listener.
  function onGridPointerDown(ev) {
    if (typeof ev.button === "number" && ev.button !== 0) return;
    const hit = api.cellAt(grid, rows, cols, ev.clientX, ev.clientY);
    setCursor(hit.row, hit.col);
  }
  grid.addEventListener("pointerdown", onGridPointerDown);

  // On-screen keypad: always rendered (not just on coarse-pointer/narrow
  // viewports -- keeping it up on desktop too is simplest and harmless, per
  // web/js/games/README.md) so the game is fully playable with no physical
  // keyboard. Digit buttons run 1..size, read from the board above, never
  // hardcoded.
  const keypad = document.createElement("div");
  keypad.className = "keypad";
  keypad.style.gridTemplateColumns = `repeat(${size}, 1fr)`;
  const digitButtons = [];
  for (const d of digits) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = String(d);
    btn.setAttribute("aria-label", `Enter ${d}`);
    btn.addEventListener("click", (ev) => {
      applyDigit(cursor.row, cursor.col, d, noteMode || ev.shiftKey);
    });
    keypad.appendChild(btn);
    digitButtons.push(btn);
  }
  shell.appendChild(keypad);

  const tools = document.createElement("div");
  tools.className = "keypad-tools";
  const noteBtn = document.createElement("button");
  noteBtn.type = "button";
  noteBtn.textContent = "✏ Notes";
  noteBtn.setAttribute("aria-pressed", "false");
  noteBtn.addEventListener("click", () => {
    noteMode = !noteMode;
    noteBtn.classList.toggle("active", noteMode);
    noteBtn.setAttribute("aria-pressed", String(noteMode));
  });
  const clearBtn = document.createElement("button");
  clearBtn.type = "button";
  clearBtn.textContent = "Erase";
  clearBtn.addEventListener("click", () => clearCell(cursor.row, cursor.col));
  tools.appendChild(noteBtn);
  tools.appendChild(clearBtn);
  shell.appendChild(tools);

  container.appendChild(shell);

  render();
  runChecks();

  function setCursor(r, c) {
    cursor = { row: r, col: c };
    render();
  }

  function applyDigit(r, c, digit, asNote) {
    if (board.givens[r][c]) return;
    if (asNote) {
      if (board.cells[r][c] !== 0) return; // notes only make sense on an empty cell
      const set = notes[r][c];
      if (set.has(digit)) set.delete(digit);
      else set.add(digit);
      render();
      return;
    }
    board.cells[r][c] = board.cells[r][c] === digit ? 0 : digit;
    if (board.cells[r][c] !== 0) notes[r][c].clear();
    render();
    runChecks();
  }

  function clearCell(r, c) {
    if (board.givens[r][c]) return;
    board.cells[r][c] = 0;
    notes[r][c].clear();
    render();
    runChecks();
  }

  function render() {
    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        const { el: cellEl, notesEl, valueEl } = cellEls[r][c];
        const v = board.cells[r][c];
        valueEl.textContent = v === 0 ? "" : String(v);
        notesEl.style.display = v === 0 && notes[r][c].size > 0 ? "grid" : "none";
        for (const span of notesEl.children) {
          const d = Number(span.dataset.digit);
          span.textContent = notes[r][c].has(d) ? String(d) : "";
        }
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
        cellEls[r][c].el.classList.toggle("invalid", bad.has(r + "," + c));
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
      cursor = {
        row: clamp(cursor.row + delta.dr, 0, rows - 1),
        col: clamp(cursor.col + delta.dc, 0, cols - 1),
      };
      render();
      return true;
    }

    if (event.key >= "1" && event.key <= "9") {
      const d = Number(event.key);
      if (d <= size) {
        event.preventDefault();
        applyDigit(cursor.row, cursor.col, d, noteMode || event.shiftKey);
        return true;
      }
    }

    if (event.key === "0" || event.key === "Backspace") {
      event.preventDefault();
      clearCell(cursor.row, cursor.col);
      return true;
    }

    if (event.key === "e" || event.key === "E") {
      event.preventDefault();
      noteMode = !noteMode;
      noteBtn.classList.toggle("active", noteMode);
      noteBtn.setAttribute("aria-pressed", String(noteMode));
      return true;
    }

    return false;
  }

  function pulseHintCells(cells) {
    for (const c of cells) {
      const entry = cellEls[c.row] && cellEls[c.row][c.col];
      if (!entry) continue;
      entry.el.classList.add("hint-pulse");
      window.setTimeout(() => entry.el.classList.remove("hint-pulse"), HINT_PULSE_MS);
    }
  }

  // See web/js/games/README.md's "Hints" section.
  async function performHint() {
    const result = await api.hint(board);
    if (!result) return null;
    if (result.done) {
      api.onHint(result.message);
      return result;
    }
    for (const w of result.apply.cells) {
      board.cells[w.row][w.col] = w.value;
      notes[w.row][w.col].clear();
    }
    if (result.cells.length > 0) setCursor(result.cells[0].row, result.cells[0].col);
    render();
    pulseHintCells(result.cells);
    await runChecks();
    api.onHint(result.message);
    return result;
  }

  function destroy() {
    grid.removeEventListener("pointerdown", onGridPointerDown);
    container.innerHTML = "";
  }

  return { handleKey, destroy, hint: performHint };
}

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
}
