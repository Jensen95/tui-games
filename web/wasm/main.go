//go:build js && wasm

// Command main is the WASM engine bridge. Build it with:
//
//	GOOS=js GOARCH=wasm go build -o lig.wasm ./web/wasm
//
// It exposes exactly one global object, globalThis.ligEngine, with four
// synchronous, string-in/string-out functions — games, generate,
// violations, solved — then signals readiness via globalThis.ligEngineReady
// (and globalThis.onLigEngineReady(), if the host page defined it) and
// blocks forever so the Go runtime stays alive to service further calls.
//
// See web/js/api.md for the full JSON contract every function implements.
// No exposed function ever panics or throws across the JS boundary: every
// call site recovers and returns {"error": "..."} instead.
package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/Jensen95/tui-games/internal/engine"
	_ "github.com/Jensen95/tui-games/internal/games/all"
)

func main() {
	ligEngine := js.Global().Get("Object").New()
	ligEngine.Set("games", wrapJS(gamesHandler))
	ligEngine.Set("generate", wrapJS(generateHandler))
	ligEngine.Set("violations", wrapJS(violationsHandler))
	ligEngine.Set("solved", wrapJS(solvedHandler))
	ligEngine.Set("hint", wrapJS(hintHandler))
	js.Global().Set("ligEngine", ligEngine)

	js.Global().Set("ligEngineReady", js.ValueOf(true))
	if cb := js.Global().Get("onLigEngineReady"); cb.Type() == js.TypeFunction {
		cb.Invoke()
	}

	select {}
}

// wrapJS adapts a (args []js.Value) (string, error) handler into a
// js.Func: it recovers any panic, and turns any returned error (including a
// recovered panic) into the {"error": "..."} JSON shape instead of ever
// letting Go's panic or a thrown JS exception cross the boundary.
func wrapJS(fn func(args []js.Value) (string, error)) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) any {
		result, err := safeCall(fn, args)
		if err != nil {
			return errorJSON(err)
		}
		return result
	})
}

// safeCall runs fn, converting any panic into an error return.
func safeCall(fn func(args []js.Value) (string, error), args []js.Value) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn(args)
}

func errorJSON(err error) string {
	b, merr := json.Marshal(map[string]string{"error": err.Error()})
	if merr != nil {
		// Marshaling a plain string map cannot realistically fail; this is
		// just belt-and-suspenders so errorJSON itself never panics.
		return `{"error":"internal error"}`
	}
	return string(b)
}

// gameListEntry is the wire shape of one games() entry.
type gameListEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// gamesHandler implements globalThis.ligEngine.games(): every registered
// game's id and human-readable name, from engine.All().
func gamesHandler(args []js.Value) (string, error) {
	entries := engine.All()
	out := make([]gameListEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, gameListEntry{ID: string(e.ID), Name: e.Name})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// generateHandler implements
// globalThis.ligEngine.generate(gameId, difficulty, seed).
func generateHandler(args []js.Value) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("generate: want 3 args (gameId, difficulty, seed), got %d", len(args))
	}
	gameID := args[0].String()
	diffStr := args[1].String()
	seed := int64(args[2].Float())

	a, err := lookupAdapter(gameID)
	if err != nil {
		return "", err
	}
	diff, err := engine.ParseDifficulty(diffStr)
	if err != nil {
		return "", err
	}
	r := engine.NewRand(seed)
	res, err := a.generate(diff, r)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// violationsHandler implements
// globalThis.ligEngine.violations(gameId, puzzleJSON, boardJSON).
func violationsHandler(args []js.Value) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("violations: want 3 args (gameId, puzzleJSON, boardJSON), got %d", len(args))
	}
	gameID := args[0].String()
	puzzleJSON := args[1].String()
	boardJSON := args[2].String()

	a, err := lookupAdapter(gameID)
	if err != nil {
		return "", err
	}
	viols, err := a.violations([]byte(puzzleJSON), []byte(boardJSON))
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(viols)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// solvedHandler implements
// globalThis.ligEngine.solved(gameId, puzzleJSON, boardJSON).
func solvedHandler(args []js.Value) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("solved: want 3 args (gameId, puzzleJSON, boardJSON), got %d", len(args))
	}
	gameID := args[0].String()
	puzzleJSON := args[1].String()
	boardJSON := args[2].String()

	a, err := lookupAdapter(gameID)
	if err != nil {
		return "", err
	}
	ok, err := a.solved([]byte(puzzleJSON), []byte(boardJSON))
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(map[string]bool{"solved": ok})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// hintHandler implements
// globalThis.ligEngine.hint(gameId, puzzleJSON, boardJSON, solutionJSON).
func hintHandler(args []js.Value) (string, error) {
	if len(args) < 4 {
		return "", fmt.Errorf("hint: want 4 args (gameId, puzzleJSON, boardJSON, solutionJSON), got %d", len(args))
	}
	gameID := args[0].String()
	puzzleJSON := args[1].String()
	boardJSON := args[2].String()
	solutionJSON := args[3].String()

	a, err := lookupAdapter(gameID)
	if err != nil {
		return "", err
	}
	res, err := a.hint([]byte(puzzleJSON), []byte(boardJSON), []byte(solutionJSON))
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
