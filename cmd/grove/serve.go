package main

import "errors"

// runServe starts the daemon. Wired up together with the server package.
func runServe(args []string) error {
	_ = args
	return errors.New("the daemon is not wired up yet (coming with the server package)")
}

// runOpen launches the browser pointed at the local daemon with the auth token.
func runOpen(args []string) error {
	_ = args
	return errors.New("not implemented yet")
}
