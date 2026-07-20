package gemini

import (
	"errors"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

func TestFormatPromptUnsupported(t *testing.T) {
	d := New()
	b, err := d.FormatPrompt("hello there")
	if b != nil {
		t.Errorf("FormatPrompt() bytes = %v, want nil", b)
	}
	if !errors.Is(err, driver.ErrUnsupported) {
		t.Errorf("FormatPrompt() error = %v, want %v", err, driver.ErrUnsupported)
	}
}
