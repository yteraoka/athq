package main

import "github.com/yteraoka/athq/internal/athq"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	athq.SetVersionInfo(version, commit, date)
	athq.Execute()
}
