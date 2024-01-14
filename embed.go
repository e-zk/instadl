package main

import "embed"

//go:embed index.tmpl.html
var indexTmplFS embed.FS
