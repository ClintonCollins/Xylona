package main

import "embed"

//go:embed sql/migrations/*
var EmbeddedMigrations embed.FS
