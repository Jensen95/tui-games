package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// listPuzzleFiles reads and decodes every *.json file directly under dir.
func listPuzzleFiles(t *testing.T, dir string) []puzzleFile {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var out []puzzleFile
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", e.Name(), err)
		}
		var pf puzzleFile
		if err := json.Unmarshal(data, &pf); err != nil {
			t.Fatalf("decode %s: %v", e.Name(), err)
		}
		out = append(out, pf)
	}
	return out
}

// TestGenerateCorpusDedupAcrossRuns mirrors the in-batch dedup loop but
// across two separate `generate` invocations sharing a --corpus directory:
// the second run, given the same base seed, must reject every puzzle the
// first run already produced and instead fall through to fresh seeds.
func TestGenerateCorpusDedupAcrossRuns(t *testing.T) {
	corpusDir := t.TempDir()
	out1 := t.TempDir()
	out2 := t.TempDir()

	args1 := []string{
		"--game", "tango",
		"--difficulty", "easy",
		"--count", "3",
		"--seed", "1",
		"--out", out1,
		"--corpus", corpusDir,
	}
	if err := runGenerate(args1); err != nil {
		t.Fatalf("first runGenerate: %v", err)
	}

	first := listPuzzleFiles(t, out1)
	if len(first) != 3 {
		t.Fatalf("first run: got %d puzzle files, want 3", len(first))
	}
	firstSeeds := map[int64]bool{}
	for _, pf := range first {
		firstSeeds[pf.Seed] = true
	}

	// Re-run with the identical base seed and count against the same
	// corpus. Every fingerprint the first run recorded must be rejected as a
	// dup, so the second run must fall through to seeds the first run never
	// used, yet still succeed in producing `count` fresh, distinct puzzles.
	args2 := []string{
		"--game", "tango",
		"--difficulty", "easy",
		"--count", "3",
		"--seed", "1",
		"--out", out2,
		"--corpus", corpusDir,
	}
	if err := runGenerate(args2); err != nil {
		t.Fatalf("second runGenerate: %v", err)
	}

	second := listPuzzleFiles(t, out2)
	if len(second) != 3 {
		t.Fatalf("second run: got %d puzzle files, want 3", len(second))
	}
	for _, pf := range second {
		if firstSeeds[pf.Seed] {
			t.Fatalf("second run reused seed %d from first run: corpus dedup did not skip it", pf.Seed)
		}
	}

	// And the payloads themselves must differ (belt-and-suspenders: seeds
	// differing is necessary but the real invariant is content dedup).
	firstPayloads := map[string]bool{}
	for _, pf := range first {
		firstPayloads[string(pf.Payload)] = true
	}
	for _, pf := range second {
		if firstPayloads[string(pf.Payload)] {
			t.Fatalf("second run emitted a payload identical to one from the first run")
		}
	}
}

// TestGenerateCorpusPersistsAcrossOpens simulates two independent processes:
// each calls runGenerate separately (a fresh corpus.Store Open/Close per
// call), sharing only the on-disk directory.
func TestGenerateCorpusPersistsAcrossOpens(t *testing.T) {
	corpusDir := t.TempDir()
	out1 := t.TempDir()
	out2 := t.TempDir()

	if err := runGenerate([]string{
		"--game", "zip",
		"--difficulty", "easy",
		"--count", "1",
		"--seed", "5",
		"--out", out1,
		"--corpus", corpusDir,
	}); err != nil {
		t.Fatalf("first runGenerate: %v", err)
	}
	first := listPuzzleFiles(t, out1)
	if len(first) != 1 {
		t.Fatalf("first run: got %d files, want 1", len(first))
	}

	// Same exact seed again, brand-new process-equivalent invocation: must
	// not reproduce the same puzzle, proving the corpus persisted to disk
	// and was reloaded.
	if err := runGenerate([]string{
		"--game", "zip",
		"--difficulty", "easy",
		"--count", "1",
		"--seed", "5",
		"--out", out2,
		"--corpus", corpusDir,
	}); err != nil {
		t.Fatalf("second runGenerate: %v", err)
	}
	second := listPuzzleFiles(t, out2)
	if len(second) != 1 {
		t.Fatalf("second run: got %d files, want 1", len(second))
	}
	if first[0].Seed == second[0].Seed {
		t.Fatalf("second run reused seed %d: corpus did not persist across separate runs", first[0].Seed)
	}
}

// TestGenerateWithoutCorpusFlagUnaffected ensures the new --corpus flag is
// purely additive: omitting it keeps the old in-batch-only behavior (no
// corpus directory is created or required).
func TestGenerateWithoutCorpusFlagUnaffected(t *testing.T) {
	out := t.TempDir()
	if err := runGenerate([]string{
		"--game", "tango",
		"--difficulty", "easy",
		"--count", "2",
		"--seed", "100",
		"--out", out,
	}); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}
	if got := len(listPuzzleFiles(t, out)); got != 2 {
		t.Fatalf("got %d puzzle files, want 2", got)
	}
}
