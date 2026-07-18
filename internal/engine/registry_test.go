package engine

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"testing"
)

// stubEntry is the trivial game used by the Phase 0 exit gate: it must
// round-trip generate → encode → verify through the registry.
func stubEntry(id GameID) Entry {
	return Entry{
		ID:   id,
		Name: "Stub",
		Generate: func(diff Difficulty, r *rand.Rand) (Generated, error) {
			payload := []byte{byte(diff), byte(r.IntN(256))}
			return Generated{
				Puzzle:      payload,
				Solution:    payload,
				Encoded:     payload,
				Fingerprint: FingerprintBytes(payload),
			}, nil
		},
		Verify: func(encoded []byte) error {
			if len(encoded) != 2 {
				return errors.New("stub: bad payload")
			}
			return nil
		},
	}
}

func TestRegistry_StubGameRoundTrips(t *testing.T) {
	e := stubEntry("stub-roundtrip")
	Register(e)

	got, ok := Lookup("stub-roundtrip")
	if !ok {
		t.Fatal("Lookup failed after Register")
	}
	gen, err := got.Generate(Medium, NewRand(42))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := got.Verify(gen.Encoded); err != nil {
		t.Fatalf("Verify rejected generated puzzle: %v", err)
	}

	// Determinism: same seed, same puzzle.
	gen2, _ := got.Generate(Medium, NewRand(42))
	if !bytes.Equal(gen.Encoded, gen2.Encoded) {
		t.Error("same seed produced different puzzles")
	}
	gen3, _ := got.Generate(Medium, NewRand(43))
	if bytes.Equal(gen.Encoded, gen3.Encoded) {
		t.Error("different seeds produced identical puzzles (suspicious for stub)")
	}
}

func TestRegister_PanicsOnDuplicate(t *testing.T) {
	Register(stubEntry("stub-dup"))
	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	Register(stubEntry("stub-dup"))
}

func TestParseDifficulty(t *testing.T) {
	for want, name := range map[Difficulty]string{Easy: "easy", Medium: "Medium", Hard: "HARD", Expert: "expert"} {
		got, err := ParseDifficulty(name)
		if err != nil || got != want {
			t.Errorf("ParseDifficulty(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
	if _, err := ParseDifficulty("nightmare"); err == nil {
		t.Error("ParseDifficulty accepted unknown difficulty")
	}
}

func TestCanonicalMin(t *testing.T) {
	got := CanonicalMin([][]byte{[]byte("b"), []byte("ab"), []byte("aa")})
	if string(got) != "aa" {
		t.Errorf("CanonicalMin = %q, want %q", got, "aa")
	}
}
