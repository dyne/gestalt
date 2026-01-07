package main

import (
	"io/fs"
	"path"
	"strings"
)

// configFS routes config reads to external or embedded sources per subdirectory.
type configFS struct {
	embedded     fs.FS
	external     fs.FS
	externalRoot string
	useExternal  map[string]bool
}

func (c configFS) Open(name string) (fs.File, error) {
	fsys, target := c.selectFS(name)
	return fsys.Open(target)
}

func (c configFS) ReadDir(name string) ([]fs.DirEntry, error) {
	fsys, target := c.selectFS(name)
	if reader, ok := fsys.(fs.ReadDirFS); ok {
		return reader.ReadDir(target)
	}
	return fs.ReadDir(fsys, target)
}

func (c configFS) Stat(name string) (fs.FileInfo, error) {
	fsys, target := c.selectFS(name)
	if statter, ok := fsys.(fs.StatFS); ok {
		return statter.Stat(target)
	}
	return fs.Stat(fsys, target)
}

func (c configFS) selectFS(name string) (fs.FS, string) {
	cleaned := path.Clean(name)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		cleaned = "."
	}

	if cleaned == "config/agents" || strings.HasPrefix(cleaned, "config/agents/") {
		return c.pickFS("agents", cleaned)
	}
	if cleaned == "config/prompts" || strings.HasPrefix(cleaned, "config/prompts/") {
		return c.pickFS("prompts", cleaned)
	}
	if cleaned == "config/skills" || strings.HasPrefix(cleaned, "config/skills/") {
		return c.pickFS("skills", cleaned)
	}

	return c.embedded, cleaned
}

func (c configFS) pickFS(kind, cleaned string) (fs.FS, string) {
	if c.useExternal[kind] {
		return c.external, path.Join(c.externalRoot, cleaned)
	}
	return c.embedded, cleaned
}
