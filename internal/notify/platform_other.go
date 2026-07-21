//go:build !darwin

package notify

import "log/slog"

// newPlatformSink returns a no-op sink: desktop notifications are macOS-only in
// v1, so other platforms drop banners silently.
func newPlatformSink(_ *slog.Logger) Sink {
	return NopSink{}
}
