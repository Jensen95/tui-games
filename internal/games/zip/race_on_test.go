//go:build race

package zip

// raceEnabled reports whether this test binary was built with the race
// detector. Latency-budget assertions are skipped under race: instrumentation
// slows deep backtracking by 10-50x, which both invalidates the measurement
// and blows the package test timeout in CI.
const raceEnabled = true
