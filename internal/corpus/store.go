// Package corpus persists puzzle fingerprints across separate `lig generate`
// runs (and processes) so a growing library of provably-distinct puzzles can
// be amassed over time, per docs/plan/docs/02-engine-and-generation.md
// ("Deduplication", level 2 — across runs).
//
// It lives outside internal/engine and internal/games on purpose: it needs
// os for file I/O, and those packages are depguard-restricted to stdlib +
// internal/engine (no os). corpus only imports internal/engine, so the
// engine seam (docs/plan/docs/01-architecture.md) stays intact.
package corpus

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Jensen95/tui-games/internal/engine"
)

// Store persists seen puzzle fingerprints to a directory, one file per game.
// The on-disk format is deliberately simple and append-friendly: one
// hex-encoded fingerprint per line. Multiple runs (even multiple processes,
// serialized) just keep appending; a corrupt or partial trailing line — e.g.
// left by a process killed mid-write — is tolerated and skipped on load
// rather than treated as fatal.
//
// A Store is safe for concurrent use by multiple goroutines within a single
// process (guarded by an internal mutex). It is not safe for concurrent use
// by multiple processes against the same directory.
type Store struct {
	dir string

	mu    sync.Mutex
	sets  map[engine.GameID]map[[32]byte]struct{}
	files map[engine.GameID]*os.File
}

// Open opens (creating if necessary) a corpus store rooted at dir. Per-game
// files are not eagerly scanned; a game's fingerprints are loaded lazily the
// first time that game is touched via Seen or Add.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("corpus: empty dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("corpus: %w", err)
	}
	return &Store{
		dir:   dir,
		sets:  make(map[engine.GameID]map[[32]byte]struct{}),
		files: make(map[engine.GameID]*os.File),
	}, nil
}

// path returns the on-disk file backing game's fingerprints.
func (s *Store) path(game engine.GameID) string {
	return filepath.Join(s.dir, string(game)+".fp")
}

// load returns the in-memory fingerprint set for game, reading it from disk
// on first access. Caller must hold s.mu.
func (s *Store) load(game engine.GameID) (map[[32]byte]struct{}, error) {
	if set, ok := s.sets[game]; ok {
		return set, nil
	}
	set := make(map[[32]byte]struct{})

	f, err := os.Open(s.path(game))
	if err != nil {
		if os.IsNotExist(err) {
			s.sets[game] = set
			return set, nil
		}
		return nil, fmt.Errorf("corpus: open %s: %w", s.path(game), err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		b, decErr := hex.DecodeString(line)
		if decErr != nil || len(b) != 32 {
			// Corrupt or partial line (e.g. a killed process mid-write):
			// tolerate it and move on rather than failing the whole load.
			continue
		}
		var fp [32]byte
		copy(fp[:], b)
		set[fp] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("corpus: read %s: %w", s.path(game), err)
	}

	s.sets[game] = set
	return set, nil
}

// Seen reports whether fp has already been recorded for game, either in this
// process or a previous run that persisted to the same directory.
func (s *Store) Seen(game engine.GameID, fp [32]byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	set, err := s.load(game)
	if err != nil {
		// Dedup is best-effort: a load failure (e.g. unreadable file) should
		// not make Generate spuriously reject everything as "seen". Add will
		// still surface a hard error if the directory is truly unusable.
		return false
	}
	_, ok := set[fp]
	return ok
}

// Add records fp as seen for game: appends it to the on-disk file (fsynced
// for durability) and updates the in-memory set. Adding an already-recorded
// fingerprint is a no-op.
func (s *Store) Add(game engine.GameID, fp [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	set, err := s.load(game)
	if err != nil {
		return err
	}
	if _, ok := set[fp]; ok {
		return nil
	}

	f, err := s.fileFor(game)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(hex.EncodeToString(fp[:]) + "\n"); err != nil {
		return fmt.Errorf("corpus: write %s: %w", s.path(game), err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("corpus: sync %s: %w", s.path(game), err)
	}

	set[fp] = struct{}{}
	return nil
}

// fileFor returns (opening lazily if needed) the append-mode file handle for
// game. Caller must hold s.mu.
func (s *Store) fileFor(game engine.GameID) (*os.File, error) {
	if f, ok := s.files[game]; ok {
		return f, nil
	}
	f, err := os.OpenFile(s.path(game), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("corpus: open %s: %w", s.path(game), err)
	}
	s.files[game] = f
	return f, nil
}

// Close closes any open per-game file handles. It is safe to call multiple
// times.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var firstErr error
	for game, f := range s.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("corpus: close %s: %w", s.path(game), err)
		}
		delete(s.files, game)
	}
	return firstErr
}
