// web/js/app.js
//
// The play-shell: menu (game + difficulty + seed picker) -> generates a
// puzzle via web/js/engine.js -> hands it to the matching game module under
// web/js/games/<id>.js -> live violation styling + win detection are driven
// entirely by that module calling back into the shell through the `api`
// object built in startGame() below. See web/js/games/README.md for the
// exact module/api contract this file implements the "shell" side of.

import * as engine from "./engine.js";

// Cosmetic-only descriptions for the menu cards; games() (the source of
// truth for which games exist) only gives {id, name}. An id with no entry
// here still renders fine with a generic fallback line.
const GAME_BLURBS = {
  tango: { glyphs: "☀☽", blurb: "Balance suns and moons under equals/cross edge constraints." },
  queens: { glyphs: "♕", blurb: "One queen per row, column and region -- none touching." },
  zip: { glyphs: "┐┘", blurb: "Draw one path through every cell, in numbered order." },
  patches: { glyphs: "▓", blurb: "Tile the grid with rectangles that match every clue." },
  minisudoku: { glyphs: "#", blurb: "Classic Sudoku rules, distilled to a smaller, faster grid." },
};

const DIFFICULTIES = ["easy", "medium", "hard", "expert"];

const el = {
  engineStatus: document.getElementById("engine-status"),
  difficultyGroup: document.getElementById("difficulty-group"),
  seedInput: document.getElementById("seed-input"),
  rerollBtn: document.getElementById("reroll-btn"),
  gameList: document.getElementById("game-list"),
  screenMenu: document.getElementById("screen-menu"),
  screenPlay: document.getElementById("screen-play"),
  menuBtn: document.getElementById("menu-btn"),
  resetBtn: document.getElementById("reset-btn"),
  playTitle: document.getElementById("play-title"),
  playDifficulty: document.getElementById("play-difficulty"),
  playSeed: document.getElementById("play-seed"),
  playTimer: document.getElementById("play-timer"),
  boardContainer: document.getElementById("board-container"),
  winOverlay: document.getElementById("win-overlay"),
  winTime: document.getElementById("win-time"),
  winNewBtn: document.getElementById("win-new-btn"),
  winMenuBtn: document.getElementById("win-menu-btn"),
};

const state = {
  screen: "menu", // "menu" | "play"
  difficulty: "easy",
  seed: randomSeed(),
  games: [], // [{id, name}]
  moduleCache: new Map(), // gameId -> module namespace | null (null = unavailable)
};

// The live puzzle session (only set while state.screen === "play").
let session = null;

function randomSeed() {
  return Math.floor(Math.random() * 1_000_000_000);
}

// ---------- boot ----------

registerServiceWorker();
wireMenuControls();
boot();

async function boot() {
  try {
    await engine.ready();
  } catch (err) {
    showEngineError(err);
    return;
  }
  let games;
  try {
    games = await engine.listGames();
  } catch (err) {
    showEngineError(err);
    return;
  }
  state.games = games;
  await renderGameList(games);
}

function registerServiceWorker() {
  if (!("serviceWorker" in navigator)) return;
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("./sw.js").catch(() => {
      /* offline support is a progressive enhancement -- ignore failures */
    });
  });
}

// ---------- menu ----------

function wireMenuControls() {
  el.seedInput.value = String(state.seed);

  el.difficultyGroup.addEventListener("click", (event) => {
    const btn = event.target.closest("[data-difficulty]");
    if (!btn) return;
    setDifficulty(btn.dataset.difficulty);
  });
  setDifficulty(state.difficulty);

  el.seedInput.addEventListener("change", () => {
    const parsed = Number.parseInt(el.seedInput.value, 10);
    state.seed = Number.isFinite(parsed) ? parsed : state.seed;
    el.seedInput.value = String(state.seed);
  });

  el.rerollBtn.addEventListener("click", () => {
    state.seed = randomSeed();
    el.seedInput.value = String(state.seed);
  });

  el.menuBtn.addEventListener("click", () => goToMenu());
  el.winMenuBtn.addEventListener("click", () => goToMenu());
  el.resetBtn.addEventListener("click", () => restartSession());
  el.winNewBtn.addEventListener("click", () => restartSession());
}

function setDifficulty(difficulty) {
  if (!DIFFICULTIES.includes(difficulty)) return;
  state.difficulty = difficulty;
  for (const btn of el.difficultyGroup.querySelectorAll("[data-difficulty]")) {
    btn.classList.toggle("active", btn.dataset.difficulty === difficulty);
  }
}

async function renderGameList(games) {
  el.gameList.innerHTML = "";
  for (const game of games) {
    const meta = GAME_BLURBS[game.id] || { glyphs: "■", blurb: "" };
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "game-pick";
    btn.innerHTML = `
      <span class="glyphs" aria-hidden="true">${meta.glyphs}</span>
      <h3>${escapeHTML(game.name)}</h3>
      <p>${escapeHTML(meta.blurb)}</p>
    `;
    el.gameList.appendChild(btn);

    // Probe module availability now (a second agent may not have added
    // zip.js/patches.js yet) so the card can show "coming soon" up front
    // rather than failing silently after a click. Dynamic import() results
    // are cached by the browser, so this costs nothing extra later.
    const mod = await loadGameModule(game.id);
    if (!mod) {
      btn.disabled = true;
      const p = btn.querySelector("p");
      p.textContent = p.textContent ? `${p.textContent} (coming soon)` : "Coming soon.";
    } else {
      btn.addEventListener("click", () => startGame(game.id));
    }
  }
}

async function loadGameModule(gameId) {
  if (state.moduleCache.has(gameId)) return state.moduleCache.get(gameId);
  let mod = null;
  try {
    mod = await import(`./games/${gameId}.js`);
  } catch (err) {
    mod = null;
  }
  state.moduleCache.set(gameId, mod);
  return mod;
}

function escapeHTML(str) {
  const d = document.createElement("div");
  d.textContent = str;
  return d.innerHTML;
}

// ---------- engine status banner ----------

function showEngineError(err) {
  el.engineStatus.hidden = false;
  el.engineStatus.classList.remove("info");
  el.engineStatus.textContent = `Puzzle engine error: ${err && err.message ? err.message : err}`;
  el.gameList.innerHTML = "";
}

function showEngineInfo(message) {
  el.engineStatus.hidden = false;
  el.engineStatus.classList.add("info");
  el.engineStatus.textContent = message;
}

function clearEngineStatus() {
  el.engineStatus.hidden = true;
  el.engineStatus.textContent = "";
}

// ---------- starting / restarting / ending a game ----------

async function startGame(gameId) {
  clearEngineStatus();
  const mod = await loadGameModule(gameId);
  if (!mod) {
    showEngineInfo(`"${gameId}" isn't available yet.`);
    return;
  }

  let bundleData;
  try {
    bundleData = await engine.generate(gameId, state.difficulty, state.seed);
  } catch (err) {
    showEngineInfo(`Could not generate a ${gameId} puzzle: ${err.message}`);
    return;
  }

  const gameName = (state.games.find((g) => g.id === gameId) || {}).name || gameId;
  endSession();
  showScreen("play");

  el.playTitle.textContent = gameName;
  el.playDifficulty.textContent = capitalize(state.difficulty);
  el.playSeed.textContent = `seed ${state.seed}`;
  el.boardContainer.innerHTML = "";
  el.winOverlay.hidden = true;

  const bundle = {
    puzzle: bundleData.puzzle,
    solution: bundleData.solution,
    board: bundleData.board,
    difficulty: state.difficulty,
    seed: state.seed,
    gameId,
  };

  session = {
    gameId,
    difficulty: state.difficulty,
    seed: state.seed,
    instance: null,
    startedAt: Date.now(),
    solvedAt: null,
    timerHandle: null,
  };

  const api = buildAPI(gameId, bundleData.puzzle, () => onSessionSolved(session));
  session.instance = mod.create(el.boardContainer, api, bundle);

  startTimer(session);
}

function restartSession() {
  if (!session) return;
  state.seed = randomSeed();
  el.seedInput.value = String(state.seed);
  startGame(session.gameId);
}

function goToMenu() {
  endSession();
  showScreen("menu");
}

function endSession() {
  if (!session) return;
  stopTimer(session);
  if (session.instance && typeof session.instance.destroy === "function") {
    try {
      session.instance.destroy();
    } catch (err) {
      /* a misbehaving module shouldn't be able to wedge the shell */
    }
  }
  el.boardContainer.innerHTML = "";
  el.winOverlay.hidden = true;
  session = null;
}

function showScreen(screen) {
  state.screen = screen;
  el.screenMenu.hidden = screen !== "menu";
  el.screenPlay.hidden = screen !== "play";
}

function buildAPI(gameId, puzzle, onSolvedCallback) {
  let solvedReported = false;
  return {
    gameId,
    difficulty: state.difficulty,
    seed: state.seed,

    async violations(board) {
      try {
        return await engine.violations(gameId, puzzle, board);
      } catch (err) {
        reportBridgeError(err);
        return [];
      }
    },

    async solved(board) {
      try {
        return await engine.solved(gameId, puzzle, board);
      } catch (err) {
        reportBridgeError(err);
        return false;
      }
    },

    onSolved() {
      if (solvedReported) return;
      solvedReported = true;
      onSolvedCallback();
    },

    onError(err) {
      reportBridgeError(err);
    },

    cursorMove,
    bindPointer,
  };
}

function reportBridgeError(err) {
  // Surfaced softly (a live-region info line) rather than yanking the
  // player out of the game they're mid-puzzle on.
  showEngineInfo(`Engine error: ${err && err.message ? err.message : err}`);
}

// ---------- timer ----------

function startTimer(sess) {
  updateTimerLabel(sess);
  sess.timerHandle = window.setInterval(() => updateTimerLabel(sess), 250);
}

function stopTimer(sess) {
  if (sess.timerHandle) {
    window.clearInterval(sess.timerHandle);
    sess.timerHandle = null;
  }
}

function updateTimerLabel(sess) {
  const end = sess.solvedAt || Date.now();
  el.playTimer.textContent = formatElapsed(end - sess.startedAt);
}

function formatElapsed(ms) {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  const m = Math.floor(totalSeconds / 60);
  const s = totalSeconds % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

function capitalize(str) {
  return str.charAt(0).toUpperCase() + str.slice(1);
}

// ---------- win ----------

function onSessionSolved(sess) {
  if (sess !== session) return; // a stale/destroyed session's late callback
  sess.solvedAt = Date.now();
  stopTimer(sess);
  updateTimerLabel(sess);
  el.winTime.textContent = `Solved in ${formatElapsed(sess.solvedAt - sess.startedAt)}.`;
  el.winOverlay.hidden = false;
}

// ---------- shared keyboard/pointer helpers passed to game modules ----------

const MOVE_KEYS = {
  ArrowUp: { dr: -1, dc: 0 },
  ArrowDown: { dr: 1, dc: 0 },
  ArrowLeft: { dr: 0, dc: -1 },
  ArrowRight: { dr: 0, dc: 1 },
  w: { dr: -1, dc: 0 },
  s: { dr: 1, dc: 0 },
  a: { dr: 0, dc: -1 },
  d: { dr: 0, dc: 1 },
  k: { dr: -1, dc: 0 },
  j: { dr: 1, dc: 0 },
  h: { dr: 0, dc: -1 },
  l: { dr: 0, dc: 1 },
};

function cursorMove(event) {
  const key = event.key.length === 1 ? event.key.toLowerCase() : event.key;
  const delta = MOVE_KEYS[key];
  return delta ? { ...delta } : null;
}

const LONG_PRESS_MS = 500;

function bindPointer(elm, { onPrimary, onSecondary }) {
  let pressTimer = null;
  let longPressed = false;

  function clearTimer() {
    if (pressTimer) {
      window.clearTimeout(pressTimer);
      pressTimer = null;
    }
  }

  function onPointerDown(ev) {
    if (typeof ev.button === "number" && ev.button !== 0) return;
    longPressed = false;
    clearTimer();
    pressTimer = window.setTimeout(() => {
      longPressed = true;
      onSecondary();
    }, LONG_PRESS_MS);
  }

  function onPointerUp() {
    clearTimer();
  }

  function onClick(ev) {
    if (longPressed) {
      // The long-press timer already fired the secondary action; swallow
      // the click that follows pointerup so we don't double-act.
      longPressed = false;
      return;
    }
    if (ev.shiftKey) {
      onSecondary();
    } else {
      onPrimary();
    }
  }

  function onContextMenu(ev) {
    ev.preventDefault();
    clearTimer();
    onSecondary();
  }

  elm.addEventListener("pointerdown", onPointerDown);
  elm.addEventListener("pointerup", onPointerUp);
  elm.addEventListener("pointerleave", onPointerUp);
  elm.addEventListener("pointercancel", onPointerUp);
  elm.addEventListener("click", onClick);
  elm.addEventListener("contextmenu", onContextMenu);

  return function unbind() {
    clearTimer();
    elm.removeEventListener("pointerdown", onPointerDown);
    elm.removeEventListener("pointerup", onPointerUp);
    elm.removeEventListener("pointerleave", onPointerUp);
    elm.removeEventListener("pointercancel", onPointerUp);
    elm.removeEventListener("click", onClick);
    elm.removeEventListener("contextmenu", onContextMenu);
  };
}

// ---------- global keyboard forwarding ----------

window.addEventListener("keydown", (event) => {
  if (state.screen !== "play" || !session || !session.instance) return;
  const target = event.target;
  if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA")) return;

  if (event.key === "Escape") {
    goToMenu();
    return;
  }

  if (typeof session.instance.handleKey === "function") {
    session.instance.handleKey(event);
  }
});
