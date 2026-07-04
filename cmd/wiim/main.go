// Command wiim is a CLI for discovering, inspecting and controlling WiiM audio
// streamers on the local network. It supports device status, playback control,
// volume management, input switching, presets, Google Cast metadata, Spotify
// Connect, and local file serving.
package main

import (
	"fmt"
	"os"

	"github.com/zzwong/wiim-cli/internal/wiim"
)

func main() {
	if err := wiim.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(wiim.ExitCode(err))
	}
}
