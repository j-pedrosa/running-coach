package web

import "embed"

//go:embed all:static
var Content embed.FS
