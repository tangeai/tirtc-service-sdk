package tirtcx

import "gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcclib"

// Init maps to TiRtcInit and registers SDK callbacks through Start.
func Init() error {
	return tirtcclib.TiRtcInit()
}

// Start maps to TiRtcStart in WHIP service mode and installs Conn callbacks.
func Start() error {
	return tirtcclib.TiRtcStart("", callbacks())
}

// Stop maps to TiRtcStop.
func Stop() {
	tirtcclib.TiRtcStop()
}

// Uninit maps to TiRtcUninit.
func Uninit() {
	tirtcclib.TiRtcUninit()
}

// GetVersion maps to TiRtcGetVersion.
func GetVersion() string {
	return tirtcclib.TiRtcGetVersion()
}
