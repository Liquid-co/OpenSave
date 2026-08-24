package api

import (
	"os"
	"path/filepath"
)

// Steam has usually already downloaded the art, and the file is sitting on
// this disk.
//
// Its client keeps library images under appcache/librarycache/<appid>/, using
// the same two names the CDN serves: header.jpg and library_600x900.jpg. For a
// game in the user's Steam library that makes the first and best source local
// — no network, no third party, no rate limit, and it works on a plane.
//
// It is not a replacement for fetching. Measured against a real library it
// answered for 14 of 129 titles that have an App ID: the cache holds what Steam
// itself has shown you, so a game installed outside Steam — a repack, a GOG or
// Epic copy — is not in it however well known the game is. It is a cheap first
// look, not a source in its own right.

// steamLibraryCacheDirs returns the librarycache folders to look in.
func (s *Server) steamLibraryCacheDirs() []string {
	if s.SteamCacheDirs != nil {
		return s.SteamCacheDirs
	}
	var out []string
	for _, root := range steamInstallRoots() {
		dir := filepath.Join(root, "appcache", "librarycache")
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			out = append(out, dir)
		}
	}
	return out
}

// steamInstallRoots lists the usual Steam install locations. Deliberately
// simple: this is an optimisation, and a machine whose Steam lives somewhere
// unusual simply falls through to fetching, which already works.
func steamInstallRoots() []string {
	var out []string
	for _, env := range []string{"PROGRAMFILES(X86)", "PROGRAMFILES"} {
		if v := os.Getenv(env); v != "" {
			out = append(out, filepath.Join(v, "Steam"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		// Linux and the Steam Deck.
		out = append(out,
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".local", "share", "Steam"))
	}
	return out
}

// localSteamCover returns art Steam has already downloaded for an App ID, or
// nil when there is none.
//
// Portrait prefers the tall library image and falls back to the landscape
// header, matching what the fetching path does — a caller asking for a portrait
// would rather have the wrong shape than nothing.
func (s *Server) localSteamCover(appID string, portrait bool) []byte {
	if !isNumericID(appID) {
		return nil
	}
	names := []string{"header.jpg"}
	if portrait {
		names = []string{"library_600x900.jpg", "header.jpg"}
	}
	for _, dir := range s.steamLibraryCacheDirs() {
		for _, name := range names {
			data, err := os.ReadFile(filepath.Join(dir, appID, name))
			if err == nil && len(data) > 0 {
				return data
			}
		}
	}
	return nil
}
