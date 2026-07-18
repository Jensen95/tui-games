# 07 — Android Future (Portability Plan)

You may later want these five games as an Android app. This plan makes that a **UI project, not a rewrite** — provided one rule is honored from day one.

## The one rule that makes this cheap

**The engine stays pure Go with zero TUI/IO dependencies.** All rules, generation, solving, uniqueness, and fingerprinting live in `internal/engine` + `internal/games/*` and import nothing terminal-specific (enforced by the Phase-0 dependency guard). If that holds, every bit of the actual *puzzle logic* is reusable on Android unchanged. Only the presentation layer is new.

## Options for the Android UI (pick when you get there)

### Option A — `gomobile bind` (Go core as a library, native Android UI) — recommended
- Use `gomobile bind` to compile the engine into an **`.aar`** and call it from **Kotlin + Jetpack Compose**.
- The Compose app owns rendering, touch, animation, theming, and platform integration; it calls into the Go engine for `Generate`, `Validate`, `Solve`, `Hint`, `Fingerprint`.
- Pros: one shared, well-tested engine across TUI *and* Android; idiomatic, modern Android UI; smallest logic-duplication.
- Cons: a thin, hand-maintained binding surface (keep it small — pass simple serializable structs/bytes across the boundary, not rich object graphs); gomobile has some type-marshaling constraints to design around.
- **Design implication now:** keep a *narrow, serialization-friendly* facade over the engine (e.g. functions that take/return JSON or protobuf-ish flat structs). Promote `internal/engine` to an exported module (`.../engine`) when needed — mechanical because of the seam.

### Option B — Full Go UI on mobile (e.g. Fyne / Ebiten / Gio)
- Write the mobile UI *also* in Go (Fyne or Gio for widget-style UIs; Ebiten if you want a game-canvas feel), sharing the engine directly (no binding layer).
- Pros: single language end-to-end; no binding marshaling; reuse Go skills.
- Cons: less "native" look/feel than Compose; larger binaries; some platform-integration friction.

### Option C — Rewrite the UI natively, reuse only algorithms as reference
- Reimplement generation/solving in Kotlin using the Go engine as the spec/oracle.
- Pros: fully native, no Go runtime on device.
- Cons: you re-do (and must re-test) the hardest, most bug-prone code. Only choose this if a Go runtime on device is unacceptable. If you go here, the Go engine's **property tests double as a conformance suite** for the Kotlin port (run the same seeds through both and diff).

## What ports directly vs. what's new

| Layer | Reused on Android? |
|---|---|
| Rules / validators | ✅ as-is (Option A/B) or as spec+oracle (Option C) |
| Generators | ✅ |
| Solvers (complete + logic) | ✅ |
| Uniqueness / fingerprinting / dedup | ✅ |
| Difficulty labeling | ✅ |
| Property/fuzz test suites | ✅ (also a conformance oracle for Option C) |
| Bubble Tea shell / adapters / Lip Gloss theme | ❌ replaced by Compose (A/C) or a Go mobile UI (B) |
| Mouse hit-testing | ❌ replaced by touch handling (but the *cell-from-coordinate* math is the same idea) |

## Interaction mapping (mouse → touch)

The TUI's drag interactions were chosen partly because they map naturally to touch:
- **Zip:** click-drag to draw the path → **touch-drag** to draw the path (near-identical UX).
- **Patches:** click-drag a rectangle → **touch-drag** a rectangle.
- **Queens:** click / click-drag to mark → **tap / long-press-drag** to mark; tap-again for queen.
- **Tango / Sudoku:** click a cell → **tap** a cell; on-screen number pad already fits mobile.

Designing these as drag/tap flows now means the Android UX is a re-skin of interactions the engine already supports.

## Recommendation

Build v1 as the TUI with the pure engine and the narrow, serialization-friendly seam. When Android time comes, start with **Option A** (gomobile + Compose) and only fall back to B or C if a concrete constraint forces it. Either way, the expensive, correctness-critical code — generation, solving, uniqueness — is written and tested **once**.
