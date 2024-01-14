package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

var ErrPostExists error = errors.New("post already exists")

func constructArgs(postId, outPath string) []string {
	return []string{"--dirname-pattern=" + outPath, "--filename-pattern={profile}-{shortcode}", "--no-metadata-json", "--", "-" + postId}
}

// download post via id; return the path to where it has been downloaded.
// TODO better error handling (does not care if download fails)
func execInstaLoader(postId string) (string, error) {
	p := filepath.Join(dlPath, postId)

	err := os.Mkdir(p, 0750)
	// if the dir already exists then we skip the post and return
	// the existing path
	if errors.Is(err, os.ErrExist) {
		return p, ErrPostExists
	} else if err != nil {
		return p, err
	}

	args := constructArgs(postId, p)
	cmd := exec.Command("instaloader", args...)
	if err := cmd.Run(); err != nil {
		return p, err
	}

	return p, nil
}
