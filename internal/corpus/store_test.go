package corpus

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jensen95/tui-games/internal/engine"
)

func fp(b byte) [32]byte {
	var out [32]byte
	out[0] = b
	out[31] = b
	return out
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	const game = engine.GameID("tango")
	a, b := fp(1), fp(2)

	if s.Seen(game, a) {
		t.Fatalf("Seen(a) = true before Add")
	}
	if err := s.Add(game, a); err != nil {
		t.Fatalf("Add(a): %v", err)
	}
	if !s.Seen(game, a) {
		t.Fatalf("Seen(a) = false after Add")
	}
	if s.Seen(game, b) {
		t.Fatalf("Seen(b) = true, want false (never added)")
	}

	// Adding an already-seen fingerprint is a no-op, not an error.
	if err := s.Add(game, a); err != nil {
		t.Fatalf("Add(a) again: %v", err)
	}
}

func TestStoreDedupAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	const game = engine.GameID("zip")
	a := fp(42)

	s1, err := Open(dir)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	if err := s1.Add(game, a); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close 1: %v", err)
	}

	// A brand-new Store (simulating a separate process/run) against the same
	// directory must see the fingerprint recorded by the first.
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	defer s2.Close()
	if !s2.Seen(game, a) {
		t.Fatalf("Seen(a) = false in second Open, want true (persisted)")
	}

	b := fp(43)
	if s2.Seen(game, b) {
		t.Fatalf("Seen(b) = true, want false")
	}
}

func TestStoreCorruptLineTolerance(t *testing.T) {
	dir := t.TempDir()
	const game = engine.GameID("queens")
	good := fp(7)

	// Hand-write a file with a mix of a valid line, garbage, a short/partial
	// hex line (simulating a killed process mid-write), and a blank line.
	path := filepath.Join(dir, string(game)+".fp")
	contents := hexOf(good) + "\n" +
		"not-hex-at-all\n" +
		hexOf(good)[:10] + "\n" +
		"\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if !s.Seen(game, good) {
		t.Fatalf("Seen(good) = false, want true (valid line among corrupt ones)")
	}

	other := fp(8)
	if s.Seen(game, other) {
		t.Fatalf("Seen(other) = true, want false")
	}

	// The store must still be writable after loading a file with corrupt
	// lines.
	if err := s.Add(game, other); err != nil {
		t.Fatalf("Add after corrupt load: %v", err)
	}
	if !s.Seen(game, other) {
		t.Fatalf("Seen(other) = false after Add")
	}
}

func TestStorePerGameIsolation(t *testing.T) {
	dir := t.TempDir()
	a := fp(1)

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.Add("tango", a); err != nil {
		t.Fatalf("Add tango: %v", err)
	}
	if s.Seen("zip", a) {
		t.Fatalf("Seen(zip, a) = true, want false: fingerprints are per-game")
	}
	if !s.Seen("tango", a) {
		t.Fatalf("Seen(tango, a) = false, want true")
	}
}

func TestOpenRejectsEmptyDir(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatalf("Open(\"\") = nil error, want error")
	}
}

func hexOf(fp [32]byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 0, 64)
	for _, b := range fp {
		out = append(out, hexDigits[b>>4], hexDigits[b&0xf])
	}
	return string(out)
}
