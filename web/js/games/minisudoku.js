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

export const id = "minisudoku";

export function create(container, api, bundle) {
  const board = bundle.board;
  const rows = board.rows;
  const cols = board.cols;
  const boxRows = board.boxRows;
  const boxCols = board.boxCols;

  let cursor = { row: 0, col: 0 };
  let noteMode = false;
  const notes = Array.from({ length: rows }, () => Array.from({ length: cols }, () => new Set()));
  const unbindFns = [];

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
      for (let d = 1; d <= 6; d++) {
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

      unbindFns.push(
        api.bindPointer(cell, {
          onPrimary: () => setCursor(r, c),
          onSecondary: () => setCursor(r, c),
        })
      );
    }
    cellEls.push(rowEls);
  }
  shell.appendChild(grid);

  const keypad = document.createElement("div");
  keypad.className = "keypad";
  const digitButtons = [];
  for (let d = 1; d <= 6; d++) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = String(d);
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
  noteBtn.addEventListener("click", () => {
    noteMode = !noteMode;
    noteBtn.classList.toggle("active", noteMode);
  });
  const clearBtn = document.createElement("button");
  clearBtn.type = "button";
  clearBtn.textContent = "Clear";
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

    if (event.key >= "1" && event.key <= "6") {
      event.preventDefault();
      applyDigit(cursor.row, cursor.col, Number(event.key), noteMode || event.shiftKey);
      return true;
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

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
}
