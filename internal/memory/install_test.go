package memory

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestInstallSelectsPreferredChannel(t *testing.T) {
	// uv is preferred over pip when both are present.
	fb := newFakeBin(t, fakeOpts{tools: []string{"uv", "pip"}})
	var out bytes.Buffer
	if err := fb.env().Install(context.Background(), InstallOptions{Out: &out}); err != nil {
		t.Fatalf("Install: %v\n%s", err, out.String())
	}
	rec := fb.record(t)
	if !strings.Contains(rec, "uv tool install "+BinaryName+"=="+PinnedVersion) {
		t.Errorf("expected uv install, record = %q", rec)
	}
	if strings.Contains(rec, "pip ") {
		t.Errorf("pip should not have run, record = %q", rec)
	}
	// Post-install verification must find the binary the installer dropped.
	st, _ := fb.env().Detect(context.Background())
	if !st.Installed {
		t.Errorf("Detect after install: not installed")
	}
}

func TestInstallFallsBackToPip(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{tools: []string{"pip"}})
	if err := fb.env().Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	rec := fb.record(t)
	if !strings.Contains(rec, "pip install --user "+BinaryName+"=="+PinnedVersion) {
		t.Errorf("expected pip --user install, record = %q", rec)
	}
}

func TestInstallForcedChannel(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{tools: []string{"uv", "pipx"}})
	if err := fb.env().Install(context.Background(), InstallOptions{Channel: "pipx"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	rec := fb.record(t)
	if !strings.HasPrefix(rec, "pipx install ") {
		t.Errorf("expected forced pipx, record = %q", rec)
	}
}

func TestInstallForcedChannelUnavailable(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{tools: []string{"uv"}})
	err := fb.env().Install(context.Background(), InstallOptions{Channel: "pipx"})
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("err = %v, want 'unavailable'", err)
	}
}

func TestInstallNoChannel(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{}) // no installers on PATH
	err := fb.env().Install(context.Background(), InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "no install channel") {
		t.Fatalf("err = %v, want 'no install channel'", err)
	}
}

func TestInstallIdempotentWhenPresent(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, tools: []string{"uv"}, version: "3.6.0"})
	var out bytes.Buffer
	if err := fb.env().Install(context.Background(), InstallOptions{Out: &out}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if rec := fb.record(t); rec != "" {
		t.Errorf("installer ran despite existing install, record = %q", rec)
	}
	if !strings.Contains(out.String(), "already installed") {
		t.Errorf("output = %q, want 'already installed'", out.String())
	}
}

func TestInstallUpgradeForces(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{installed: true, tools: []string{"uv"}})
	if err := fb.env().Install(context.Background(), InstallOptions{Upgrade: true}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if rec := fb.record(t); !strings.Contains(rec, "tool install --force") {
		t.Errorf("expected forced upgrade, record = %q", rec)
	}
}

func TestInstallVersionOverride(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{tools: []string{"uv"}})
	if err := fb.env().Install(context.Background(), InstallOptions{Version: "3.5.1"}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if rec := fb.record(t); !strings.Contains(rec, BinaryName+"==3.5.1") {
		t.Errorf("expected pinned override 3.5.1, record = %q", rec)
	}
}

func TestInstallFailurePropagates(t *testing.T) {
	fb := newFakeBin(t, fakeOpts{tools: []string{"uv"}, installFail: true})
	err := fb.env().Install(context.Background(), InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "install via uv") {
		t.Fatalf("err = %v, want 'install via uv'", err)
	}
}
