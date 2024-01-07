package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func constructArgs(postId, outPath string) []string {
	return []string{"--dirname-pattern=" + outPath, "--filename-pattern={profile}-{shortcode}", "--no-metadata-json", "--", "-" + postId}
}

// download post via id; return the path to where it has been downloaded.
// TODO better error handling
func execInstaLoader(postId string) string {
	p := filepath.Join(dlPath, postId)
	err := os.Mkdir(p, 0750)

	// if the dir already exists then we skip the post and return
	// the existing path
	if errors.Is(err, os.ErrExist) {
		log.Println("skipping... already downloaded")
		return p
	} else if err != nil {
		log.Fatal(err)
	}

	args := constructArgs(postId, p)
	cmd := exec.Command("instaloader", args...)
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	return p
}
