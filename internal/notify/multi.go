package notify

// MultiSink fans a Notification out to every wrapped Sink, so the coalescer's
// throttling decision (whether a notification fires at all) is made once and
// then delivered identically to every delivery channel — e.g. a macOS banner
// and a Web Push, from the exact same policy decision.
type MultiSink struct {
	sinks []Sink
}

// NewMultiSink wraps sinks into one Sink. A nil or empty sinks list is a
// valid, silent sink.
func NewMultiSink(sinks ...Sink) MultiSink {
	return MultiSink{sinks: sinks}
}

// Notify forwards n to every wrapped sink, in order. Each sink is
// responsible for its own non-blocking/error-swallowing behavior per the
// Sink contract, so this simply fans out.
func (m MultiSink) Notify(n Notification) {
	for _, s := range m.sinks {
		s.Notify(n)
	}
}
