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

// localStorage key the current seed is persisted under, so a reload replays
// the same puzzle instead of silently rerolling. See initialSeed()/setSeed().
const SEED_STORAGE_KEY = "lig-seed";

// After each hint the shared Hint button (and the Shift+H shortcut) is locked
// out for this long, with a live countdown, so hints can't be spammed. Pick a
// short, forgiving window -- long enough to nudge deliberate use, short enough
// not to annoy. The base button label is what the countdown suffix hangs off.
const HINT_COOLDOWN_MS = 15_000;
const HINT_LABEL = "\u{1F4A1} Hint"; // 💡 Hint -- matches play.html's #hint-btn-label

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
  hintBtn: document.getElementById("hint-btn"),
  hintLabel: document.getElementById("hint-btn-label"),
  hintStatus: document.getElementById("hint-status"),
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
  seed: initialSeed(),
  games: [], // [{id, name}]
  moduleCache: new Map(), // gameId -> module namespace | null (null = unavailable)
};

// The live puzzle session (only set while state.screen === "play").
let session = null;

function randomSeed() {
  return Math.floor(Math.random() * 1_000_000_000);
}

// ---------- seed persistence ----------

// The seed the app boots with: whatever was last stored (so a reload replays
// the same puzzle), or a fresh random one on the very first visit -- which we
// persist immediately so it too survives the next reload.
function initialSeed() {
  const stored = loadStoredSeed();
  if (stored !== null) return stored;
  const seed = randomSeed();
  storeSeed(seed);
  return seed;
}

function loadStoredSeed() {
  try {
    const raw = localStorage.getItem(SEED_STORAGE_KEY);
    if (raw !== null) {
      const parsed = Number.parseInt(raw, 10);
      if (Number.isFinite(parsed)) return parsed;
    }
  } catch (err) {
    /* localStorage unavailable (private mode / disabled) -- fall through */
  }
  return null;
}

function storeSeed(seed) {
  try {
    localStorage.setItem(SEED_STORAGE_KEY, String(seed));
  } catch (err) {
    /* non-fatal: the seed just won't persist across reloads */
  }
}

// Single funnel for every seed change: keeps state, the number input, and the
// persisted value in lockstep so a reload always restores what's on screen.
function setSeed(seed) {
  state.seed = seed;
  el.seedInput.value = String(seed);
  storeSeed(seed);
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

  // If the page already had a controlling worker when it loaded, a later
  // controllerchange means a freshly-activated worker has taken over (the
  // user accepted the update) -- reload once onto the new app shell. On a
  // first-ever visit there's no controller yet, so the clients.claim() in the
  // worker's activate handler also fires controllerchange; don't reload for
  // that, it's just the initial worker taking control.
  const hadController = Boolean(navigator.serviceWorker.controller);
  let reloading = false;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (!hadController || reloading) return;
    reloading = true;
    window.location.reload();
  });

  window.addEventListener("load", () => {
    navigator.serviceWorker
      .register("./sw.js")
      .then((reg) => watchForWorkerUpdate(reg))
      .catch(() => {
        /* offline support is a progressive enhancement -- ignore failures */
      });
  });
}

// Watch a registration for an updated worker reaching the "installed" state
// while an existing worker is still in control -- the signal that a new
// version is downloaded and waiting. Bumping sw.js's CACHE_VERSION is what
// makes the browser fetch a byte-different worker and trigger this.
function watchForWorkerUpdate(reg) {
  // A worker may already be waiting from a previous load that never activated.
  if (reg.waiting && navigator.serviceWorker.controller) {
    showUpdateBanner(reg.waiting);
  }
  reg.addEventListener("updatefound", () => {
    const installing = reg.installing;
    if (!installing) return;
    installing.addEventListener("statechange", () => {
      if (installing.state === "installed" && navigator.serviceWorker.controller) {
        showUpdateBanner(installing);
      }
    });
  });
}

// Small, non-intrusive toast telling the player a new version is ready. Its
// Refresh button asks the waiting worker to skipWaiting(); the resulting
// controllerchange (wired in registerServiceWorker) reloads the page.
function showUpdateBanner(worker) {
  if (document.getElementById("update-banner")) return; // only ever show one
  const banner = document.createElement("div");
  banner.id = "update-banner";
  banner.className = "update-banner";
  banner.setAttribute("role", "status");
  banner.innerHTML = `
    <span>A new version is available.</span>
    <button type="button" class="btn btn-primary">Refresh</button>
  `;
  const btn = banner.querySelector("button");
  btn.addEventListener("click", () => {
    worker.postMessage({ type: "SKIP_WAITING" });
    btn.disabled = true;
    btn.textContent = "Updating…";
  });
  document.body.appendChild(banner);
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
    setSeed(Number.isFinite(parsed) ? parsed : state.seed);
  });

  el.rerollBtn.addEventListener("click", () => {
    setSeed(randomSeed());
  });

  el.menuBtn.addEventListener("click", () => goToMenu());
  el.winMenuBtn.addEventListener("click", () => goToMenu());
  el.resetBtn.addEventListener("click", () => restartSession());
  el.winNewBtn.addEventListener("click", () => restartSession());
  el.hintBtn.addEventListener("click", () => requestHint());
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
  clearHintStatus();
  clearHintCooldown();

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

  const api = buildAPI(gameId, bundleData.puzzle, bundleData.solution, () => onSessionSolved(session));
  session.instance = mod.create(el.boardContainer, api, bundle);

  startTimer(session);
}

function restartSession() {
  if (!session) return;
  setSeed(randomSeed());
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
  clearHintStatus();
  clearHintCooldown();
  session = null;
}

function showScreen(screen) {
  state.screen = screen;
  el.screenMenu.hidden = screen !== "menu";
  el.screenPlay.hidden = screen !== "play";
}

function buildAPI(gameId, puzzle, solution, onSolvedCallback) {
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

    // See web/js/api.md's hint() section for the {done, message, technique,
    // cells, apply} shape. Resolves to `null` on an engine-level error
    // (already reported via onError/reportBridgeError below), mirroring
    // violations()/solved()'s error handling -- callers don't need a
    // try/catch at every call site.
    async hint(board) {
      try {
        return await engine.hint(gameId, puzzle, board, solution);
      } catch (err) {
        reportBridgeError(err);
        return null;
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

    // Called by a game module every time it performs a hint (whether
    // triggered by the shared Hint button or the `H` key), so the shell's
    // shared status line can show the move/technique without every module
    // reimplementing that UI.
    onHint(message) {
      showHintStatus(message);
      // Every hint routes through here (button and Shift+H alike), so this is
      // where the shared cooldown starts.
      startHintCooldown();
    },

    cursorMove,
    bindPointer,
    cellAt,
  };
}

function reportBridgeError(err) {
  // Surfaced softly (a live-region info line) rather than yanking the
  // player out of the game they're mid-puzzle on.
  showEngineInfo(`Engine error: ${err && err.message ? err.message : err}`);
}

// ---------- hint status line ----------

function showHintStatus(message) {
  el.hintStatus.hidden = false;
  el.hintStatus.textContent = message;
}

function clearHintStatus() {
  el.hintStatus.hidden = true;
  el.hintStatus.textContent = "";
}

async function requestHint() {
  // The button is set disabled during the cooldown, but guard here too so the
  // Shift+H path and any stray programmatic click are gated identically.
  if (hintOnCooldown()) return;
  if (!session || !session.instance || typeof session.instance.hint !== "function") return;
  await session.instance.hint();
}

// ---------- hint cooldown ----------
//
// After any hint (button or Shift+H) the button is locked out for
// HINT_COOLDOWN_MS with a live countdown baked into its label. The cooldown is
// actually started from the api.onHint() callback -- the single funnel every
// hint (button-driven or keyboard-driven) passes through -- so both paths are
// covered without the shell needing to know which one fired.

let hintCooldownHandle = null;
let hintCooldownEndsAt = 0;

function hintOnCooldown() {
  return hintCooldownHandle !== null;
}

function startHintCooldown() {
  clearHintCooldown();
  hintCooldownEndsAt = Date.now() + HINT_COOLDOWN_MS;
  updateHintCooldownLabel();
  // 250ms is smooth enough for a whole-second countdown without busy-looping.
  hintCooldownHandle = window.setInterval(updateHintCooldownLabel, 250);
}

function updateHintCooldownLabel() {
  const remainingMs = hintCooldownEndsAt - Date.now();
  if (remainingMs <= 0) {
    clearHintCooldown();
    return;
  }
  const secs = Math.ceil(remainingMs / 1000);
  el.hintBtn.disabled = true;
  el.hintBtn.setAttribute("aria-disabled", "true");
  el.hintLabel.textContent = `${HINT_LABEL} (${secs}s)`;
}

function clearHintCooldown() {
  if (hintCooldownHandle !== null) {
    window.clearInterval(hintCooldownHandle);
    hintCooldownHandle = null;
  }
  hintCooldownEndsAt = 0;
  el.hintBtn.disabled = false;
  el.hintBtn.removeAttribute("aria-disabled");
  el.hintLabel.textContent = HINT_LABEL;
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

// cellAt resolves ANY point (in viewport/client coordinates, e.g. a
// PointerEvent's clientX/clientY) to the nearest {row, col} inside a
// `rows` x `cols` CSS-grid element -- pure position math
// (getBoundingClientRect + floor division), never DOM hit-testing
// (elementFromPoint/event.target). This is what makes every point inside
// (or even just outside, e.g. a fast drag that briefly overshoots) the
// grid's bounds resolve to *some* cell: there is no gap a tap can land in
// and get silently swallowed -- not on a cell border, not in a gutter
// track some games render between cells (Tango's edge-marker gutters), not
// on the 1fr-rounding sub-pixel seams CSS Grid can leave between tracks.
// Points outside [0,1] on either axis clamp to the nearest edge cell
// rather than resolving to nothing.
//
// Any game whose defining pointer interaction is grid-level (Queens'
// drag-to-paint, Zip/Patches' drag-to-draw) should resolve every
// pointerdown/pointermove hit through this instead of
// elementFromPoint/event.target.closest(...).
function cellAt(gridEl, rows, cols, clientX, clientY) {
  const rect = gridEl.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return { row: 0, col: 0 };
  let fx = (clientX - rect.left) / rect.width;
  let fy = (clientY - rect.top) / rect.height;
  fx = Math.min(Math.max(fx, 0), 0.999999);
  fy = Math.min(Math.max(fy, 0), 0.999999);
  return {
    row: clamp(Math.floor(fy * rows), 0, rows - 1),
    col: clamp(Math.floor(fx * cols), 0, cols - 1),
  };
}

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
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

  // Gate the Shift+H hint shortcut through the same cooldown as the button.
  // The game module's handleKey would otherwise call its hint() (and thus
  // api.hint) directly, bypassing the button's disabled state -- so swallow
  // the event here before forwarding while the cooldown is running.
  if (event.shiftKey && (event.key === "H" || event.key === "h") && hintOnCooldown()) {
    event.preventDefault();
    return;
  }

  if (typeof session.instance.handleKey === "function") {
    session.instance.handleKey(event);
  }
});
