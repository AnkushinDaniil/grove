package memory

import (
	"context"
	"fmt"
	"io"
)

// InstallOptions configures Install.
type InstallOptions struct {
	Out     io.Writer // installer output stream; nil => io.Discard
	Upgrade bool      // reinstall/upgrade even when already present
	Version string    // version to install; "" => PinnedVersion
	Channel string    // force a channel (uv|pipx|pip); "" => first available
}

// channel is a way to install the mempalace CLI. channels is ordered by
// preference; the first whose tool resolves on PATH wins.
//
// MemPalace ships on PyPI (https://pypi.org/project/mempalace/), so the real
// install channels are Python packagers — this deliberately diverges from the
// ORCHESTRATION.md §8 "npm/uvx/brew" sketch, which predated confirming the
// distribution. uv is preferred because `uv tool install` drops an isolated
// binary straight onto PATH; pipx is the classic equivalent; `pip install
// --user` is the always-available fallback. (brew's only role would be
// bootstrapping pipx, so it is not a direct channel.)
type channel struct {
	name  string
	tools []string // executable candidates, first found wins
}

var channels = []channel{
	{name: "uv", tools: []string{"uv"}},
	{name: "pipx", tools: []string{"pipx"}},
	{name: "pip", tools: []string{"pip", "pip3"}},
}

// installArgs builds the argv (after the tool name) for installing spec.
func (ch channel) installArgs(spec string, upgrade bool) []string {
	switch ch.name {
	case "uv":
		if upgrade {
			return []string{"tool", "install", "--force", spec}
		}
		return []string{"tool", "install", spec}
	case "pipx":
		if upgrade {
			return []string{"install", "--force", spec}
		}
		return []string{"install", spec}
	case "pip":
		if upgrade {
			return []string{"install", "--user", "--upgrade", spec}
		}
		return []string{"install", "--user", spec}
	default:
		return nil
	}
}

// selectChannel returns the channel to use and its resolved tool path. When
// force is set, only that channel is considered.
func (e Env) selectChannel(force string) (channel, string, error) {
	for _, ch := range channels {
		if force != "" && ch.name != force {
			continue
		}
		for _, tool := range ch.tools {
			if path, err := e.lookPath(tool); err == nil {
				return ch, path, nil
			}
		}
	}
	if force != "" {
		return channel{}, "", fmt.Errorf("requested install channel %q is unavailable (its tool is not on PATH)", force)
	}
	return channel{}, "", fmt.Errorf("no install channel available: put one of uv, pipx or pip on PATH (uv: https://docs.astral.sh/uv, or `brew install pipx`)")
}

// Install ensures the mempalace CLI is present. It is idempotent: an existing
// install is left as-is unless opts.Upgrade is set. Installer output streams to
// opts.Out, and the result is verified with Detect afterwards.
func (e Env) Install(ctx context.Context, opts InstallOptions) error {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	version := opts.Version
	if version == "" {
		version = PinnedVersion
	}

	st, err := e.Detect(ctx)
	if err != nil {
		return err
	}
	if st.Installed && !opts.Upgrade {
		fprintf(out, "MemPalace already installed: %s %s (%s)\n", BinaryName, orUnknown(st.Version), st.Path)
		return nil
	}

	ch, toolPath, err := e.selectChannel(opts.Channel)
	if err != nil {
		return err
	}
	spec := BinaryName + "==" + version
	args := ch.installArgs(spec, opts.Upgrade)

	fprintf(out, "Installing %s via %s (%s)...\n", spec, ch.name, toolPath)
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, installTimeout)
		defer cancel()
	}
	if err := e.stream(ctx, out, toolPath, args...); err != nil {
		if opts.Upgrade {
			return fmt.Errorf("install via %s: %w", ch.name, err)
		}
		// Detect said "not installed" yet a plain install failed — the classic
		// cause is a broken leftover (e.g. a dangling uv shim: "Executable
		// already exists"). Repair by retrying once with the force variant.
		fprintf(out, "Install failed; retrying with force to repair a broken previous install...\n")
		if err := e.stream(ctx, out, toolPath, ch.installArgs(spec, true)...); err != nil {
			return fmt.Errorf("install via %s (incl. force retry): %w", ch.name, err)
		}
	}

	st2, err := e.Detect(ctx)
	if err != nil {
		return err
	}
	if !st2.Installed {
		return fmt.Errorf("%s installed via %s but %q is still not on PATH — ensure the channel's bin dir (uv/pipx: ~/.local/bin) is on PATH", spec, ch.name, BinaryName)
	}
	fprintf(out, "Installed %s %s (%s)\n", BinaryName, orUnknown(st2.Version), st2.Path)
	return nil
}
