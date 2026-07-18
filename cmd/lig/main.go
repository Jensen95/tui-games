// lig is the LinkedIn-games TUI binary. It has two faces sharing one engine:
//
//	lig                                  # interactive TUI (game picker)
//	lig generate --game zip --difficulty hard --count 10 --seed 7 --out dir/
//	lig verify file.json [file2.json ...]
//	lig games                            # list registered games
//
// The headless generate/verify path exercises the engine with zero TUI
// dependencies — it is used by CI fuzzing and corpus builds, and doubles as
// proof that the engine stays TUI-independent.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Jensen95/tui-games/internal/corpus"
	"github.com/Jensen95/tui-games/internal/engine"
	_ "github.com/Jensen95/tui-games/internal/games/all"
)

// puzzleFile is the on-disk envelope for a generated puzzle. Payload is the
// game's own stable serialization (engine.Generated.Encoded).
type puzzleFile struct {
	SchemaVersion int             `json:"schemaVersion"`
	Game          string          `json:"game"`
	Difficulty    string          `json:"difficulty"`
	Seed          int64           `json:"seed"`
	Payload       json.RawMessage `json:"payload"`
}

const schemaVersion = 1

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lig:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runTUI()
	}
	switch args[0] {
	case "generate":
		return runGenerate(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "games":
		return runGames()
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  lig                    launch the TUI
  lig games              list registered games
  lig generate --game <id> [--difficulty easy|medium|hard|expert]
               [--count N] [--seed S] [--out DIR] [--corpus DIR]
  lig verify <file.json> [...]`)
}

func runGames() error {
	games := engine.All()
	if len(games) == 0 {
		fmt.Println("no games registered yet")
		return nil
	}
	for _, e := range games {
		fmt.Printf("%-12s %s\n", e.ID, e.Name)
	}
	return nil
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	game := fs.String("game", "", "game id (see `lig games`)")
	diffStr := fs.String("difficulty", "medium", "easy|medium|hard|expert")
	count := fs.Int("count", 1, "number of distinct puzzles to generate")
	seed := fs.Int64("seed", 1, "base seed; puzzle i uses seed+i")
	out := fs.String("out", "", "output directory (default: stdout)")
	corpusDir := fs.String("corpus", "", "cross-run corpus directory: dedup against and record into it (default: off)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *game == "" {
		return errors.New("generate: --game is required")
	}
	entry, ok := engine.Lookup(engine.GameID(*game))
	if !ok {
		return fmt.Errorf("generate: unknown game %q (see `lig games`)", *game)
	}
	diff, err := engine.ParseDifficulty(*diffStr)
	if err != nil {
		return err
	}
	if *out != "" {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			return err
		}
	}

	var store *corpus.Store
	if *corpusDir != "" {
		store, err = corpus.Open(*corpusDir)
		if err != nil {
			return fmt.Errorf("generate: %w", err)
		}
		defer store.Close()
	}

	// In-batch dedup: retry with fresh seeds until count distinct fingerprints.
	// When a corpus store is configured, also dedup and record against it, so
	// repeated `generate` runs never emit a puzzle already amassed in the
	// corpus (docs/plan/docs/02-engine-and-generation.md, "Deduplication").
	seen := map[[32]byte]struct{}{}
	produced := 0
	for s := *seed; produced < *count; s++ {
		if s-*seed > int64(*count)*100 {
			return fmt.Errorf("generate: gave up after %d attempts (%d/%d distinct)", s-*seed, produced, *count)
		}
		gen, err := entry.Generate(diff, engine.NewRand(s))
		if err != nil {
			return fmt.Errorf("generate: seed %d: %w", s, err)
		}
		if _, dup := seen[gen.Fingerprint]; dup {
			continue
		}
		if store != nil && store.Seen(entry.ID, gen.Fingerprint) {
			continue
		}
		seen[gen.Fingerprint] = struct{}{}
		if store != nil {
			if err := store.Add(entry.ID, gen.Fingerprint); err != nil {
				return fmt.Errorf("generate: corpus add: %w", err)
			}
		}

		env := puzzleFile{
			SchemaVersion: schemaVersion,
			Game:          *game,
			Difficulty:    diff.String(),
			Seed:          s,
			Payload:       json.RawMessage(gen.Encoded),
		}
		data, err := json.MarshalIndent(env, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if *out == "" {
			os.Stdout.Write(data)
		} else {
			name := fmt.Sprintf("%s-%s-%d.json", *game, diff, s)
			if err := os.WriteFile(filepath.Join(*out, name), data, 0o644); err != nil {
				return err
			}
			fmt.Println(filepath.Join(*out, name))
		}
		produced++
	}
	return nil
}

func runVerify(files []string) error {
	if len(files) == 0 {
		return errors.New("verify: no files given")
	}
	var failed int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var env puzzleFile
		if err := json.Unmarshal(data, &env); err != nil {
			return fmt.Errorf("verify: %s: bad envelope: %w", f, err)
		}
		entry, ok := engine.Lookup(engine.GameID(env.Game))
		if !ok {
			return fmt.Errorf("verify: %s: unknown game %q", f, env.Game)
		}
		if err := entry.Verify(env.Payload); err != nil {
			fmt.Printf("FAIL %s: %v\n", f, err)
			failed++
			continue
		}
		fmt.Printf("OK   %s\n", f)
	}
	if failed > 0 {
		return fmt.Errorf("verify: %d of %d puzzles failed", failed, len(files))
	}
	return nil
}
