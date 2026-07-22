package notify

import "testing"

func TestMultiSinkFansOutToEverySink(t *testing.T) {
	a := &fakeSink{}
	b := &fakeSink{}
	m := NewMultiSink(a, b)

	n := Notification{NodeID: "n1", Title: "t"}
	m.Notify(n)

	if got := a.all(); len(got) != 1 || got[0] != n {
		t.Errorf("sink a = %+v, want [%+v]", got, n)
	}
	if got := b.all(); len(got) != 1 || got[0] != n {
		t.Errorf("sink b = %+v, want [%+v]", got, n)
	}
}

func TestMultiSinkEmptyIsSilent(t *testing.T) {
	NewMultiSink().Notify(Notification{Body: "ignored"}) // must not panic
}
