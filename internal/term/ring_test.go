package term

import (
	"sync"
	"testing"
)

func TestRingWrap(t *testing.T) {
	r := NewRing(4)
	r.Write([]byte("ab"))
	if got := string(r.Bytes()); got != "ab" {
		t.Fatalf("Bytes = %q, want ab", got)
	}
	r.Write([]byte("cde")) // total 5, wraps: last 4 bytes are "bcde"
	if got := string(r.Bytes()); got != "bcde" {
		t.Fatalf("Bytes after wrap = %q, want bcde", got)
	}
	if r.Total() != 5 {
		t.Fatalf("Total = %d, want 5", r.Total())
	}
	r.Write([]byte("XYZWVU")) // write larger than capacity: last 4 are "ZWVU"
	if got := string(r.Bytes()); got != "ZWVU" {
		t.Fatalf("Bytes after oversized write = %q, want ZWVU", got)
	}
	if r.Total() != 11 {
		t.Fatalf("Total = %d, want 11", r.Total())
	}
}

func TestRingEmptyAndPartial(t *testing.T) {
	r := NewRing(8)
	if len(r.Bytes()) != 0 || r.Total() != 0 {
		t.Fatalf("fresh ring not empty: %q total=%d", r.Bytes(), r.Total())
	}
	r.Write([]byte("hi"))
	if got := string(r.Bytes()); got != "hi" {
		t.Fatalf("partial Bytes = %q, want hi", got)
	}
}

func TestRingDefaultSize(t *testing.T) {
	if r := NewRing(0); len(r.buf) != DefaultRingSize {
		t.Fatalf("NewRing(0) size = %d, want %d", len(r.buf), DefaultRingSize)
	}
	if r := NewRing(-5); len(r.buf) != DefaultRingSize {
		t.Fatalf("NewRing(-5) size = %d, want %d", len(r.buf), DefaultRingSize)
	}
}

func TestRingConcurrent(t *testing.T) {
	const (
		size    = 1024
		writers = 8
		iters   = 2000
		chunk   = "abcd"
	)
	r := NewRing(size)

	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			for range iters {
				r.Write([]byte(chunk))
			}
		})
	}
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 3 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					if got := len(r.Bytes()); got > size {
						t.Errorf("Bytes len %d exceeds capacity %d", got, size)
						return
					}
					_ = r.Total()
				}
			}
		})
	}
	wg.Wait()
	close(stop)
	readers.Wait()

	if want := int64(writers * iters * len(chunk)); r.Total() != want {
		t.Fatalf("Total = %d, want %d", r.Total(), want)
	}
	if got := len(r.Bytes()); got != size {
		t.Fatalf("Bytes len = %d, want full %d", got, size)
	}
}
