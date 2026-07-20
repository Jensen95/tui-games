//go:build race

package queens

// raceEnabled reports whether this test binary was built with the race
// detector. Queens generation runs the complete solver over N=9..11 boards,
// which the race instrumentation slows by roughly 10-50x; seedCount() uses this
// to cap property-batch sizes under race so the package stays within the CI
// test timeout. The uninstrumented LIG_SEEDS run is the full-coverage pass.
const raceEnabled = true
