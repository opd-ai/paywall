//go:build !race

package paywall

// raceDetectorEnabled is false when not built with -race flag
const raceDetectorEnabled = false
