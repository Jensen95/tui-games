// web/js/engine.js
//
// Thin promise-friendly wrapper around globalThis.ligEngine, the
// synchronous WASM bridge documented in web/js/api.md. This module:
//
//   1. Loads ./wasm_exec.js (the Go js/wasm runtime support script -- a
//      classic, non-module script that defines globalThis.Go) and the
//      compiled ../lig.wasm module.
//   2. Waits for the bridge to signal readiness (globalThis.ligEngineReady).
//   3. Exposes a handful of async functions that JSON-encode/decode at the
//      boundary and turn `{"error": "..."}` responses into thrown Errors,
//      so callers never touch raw JSON strings or globalThis.ligEngine
//      directly.
//
// All paths are resolved relative to *this module's own URL*
// (import.meta.url), not the page's URL, so this works correctly however
// deep the importing page lives under the site's root -- required since
// the site is deployed at a subpath (https://jensen95.github.io/tui-games/).

let readyPromise = null;

function loadClassicScript(url) {
  return new Promise((resolve, reject) => {
    const el = document.createElement("script");
    el.src = url;
    el.onload = () => resolve();
    el.onerror = () =>
      reject(
        new Error(
          `failed to load ${url} -- has the wasm bridge been built? ` +
            `(GOOS=js GOARCH=wasm go build -o web/lig.wasm ./web/wasm && ` +
            `cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/js/wasm_exec.js)`
        )
      );
    document.head.appendChild(el);
  });
}

async function loadInstance() {
  const scriptURL = new URL("./wasm_exec.js", import.meta.url);
  const wasmURL = new URL("../lig.wasm", import.meta.url);

  await loadClassicScript(scriptURL.href);
  if (typeof globalThis.Go !== "function") {
    throw new Error("wasm_exec.js loaded but did not define globalThis.Go");
  }
  const go = new globalThis.Go();

  let instance = null;
  if (typeof WebAssembly.instantiateStreaming === "function") {
    try {
      const result = await WebAssembly.instantiateStreaming(fetch(wasmURL.href), go.importObject);
      instance = result.instance;
    } catch (err) {
      // Falls through to the arrayBuffer path below -- some static file
      // servers mislabel the response Content-Type (not application/wasm),
      // which makes instantiateStreaming reject even though the bytes are
      // fine.
      instance = null;
    }
  }
  if (!instance) {
    const res = await fetch(wasmURL.href);
    if (!res.ok) {
      throw new Error(`failed to fetch ${wasmURL.href}: HTTP ${res.status}`);
    }
    const bytes = await res.arrayBuffer();
    const result = await WebAssembly.instantiate(bytes, go.importObject);
    instance = result.instance;
  }

  // go.run() returns a promise that only resolves once the Go program's
  // main() returns -- main() here blocks forever (select{}) to keep
  // servicing calls, so this is intentionally fire-and-forget, never
  // awaited.
  go.run(instance);

  await new Promise((resolve) => {
    if (globalThis.ligEngineReady) {
      resolve();
      return;
    }
    globalThis.onLigEngineReady = resolve;
  });

  return globalThis.ligEngine;
}

/** Resolves once globalThis.ligEngine is loaded and ready to call. Safe to
 * call repeatedly -- the load only happens once. */
export function ready() {
  if (!readyPromise) readyPromise = loadInstance();
  return readyPromise;
}

function unwrap(raw) {
  const parsed = JSON.parse(raw);
  if (
    parsed &&
    typeof parsed === "object" &&
    !Array.isArray(parsed) &&
    Object.prototype.hasOwnProperty.call(parsed, "error")
  ) {
    throw new Error(parsed.error);
  }
  return parsed;
}

function asJSONString(value) {
  return typeof value === "string" ? value : JSON.stringify(value);
}

/** -> Promise<Array<{id: string, name: string}>> */
export async function listGames() {
  const eng = await ready();
  return unwrap(eng.games());
}

/**
 * generate(gameId, difficulty, seed) -> Promise<{puzzle, solution, board}>
 *
 * `puzzle` comes back as a parsed JS value -- an opaque token per api.md,
 * never read fields out of it, just pass it straight into violations()/
 * solved() below (they JSON.stringify it again for you).
 */
export async function generate(gameId, difficulty, seed) {
  const eng = await ready();
  return unwrap(eng.generate(gameId, difficulty, Number(seed)));
}

/** violations(gameId, puzzle, board) -> Promise<Array<Violation>> */
export async function violations(gameId, puzzle, board) {
  const eng = await ready();
  return unwrap(eng.violations(gameId, asJSONString(puzzle), asJSONString(board)));
}

/** solved(gameId, puzzle, board) -> Promise<boolean> */
export async function solved(gameId, puzzle, board) {
  const eng = await ready();
  const result = unwrap(eng.solved(gameId, asJSONString(puzzle), asJSONString(board)));
  return Boolean(result.solved);
}

/**
 * hint(gameId, puzzle, board, solution) -> Promise<HintResult>
 *
 * See web/js/api.md's hint() section for the full shape:
 * {done, message, technique, cells, apply}. `solution` is the `solution`
 * value generate() returned for this puzzle.
 */
export async function hint(gameId, puzzle, board, solution) {
  const eng = await ready();
  return unwrap(eng.hint(gameId, asJSONString(puzzle), asJSONString(board), asJSONString(solution)));
}
