package main

import "embed"

// webBuildFS embeds the SvelteKit build output.
// The web_build directory is a symlink or copy created by the Makefile.
//
//go:embed all:web_build
var webBuildFS embed.FS

// sdkDistFS embeds the SDK JS bundle.
// The sdk_dist directory is a symlink or copy created by the Makefile.
//
//go:embed all:sdk_dist
var sdkDistFS embed.FS
