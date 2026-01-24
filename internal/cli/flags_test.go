package cli

import (
	"flag"
	"io"
	"testing"
)

func TestHelpFlag(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	flags := AddHelpVersionFlags(fs, "", "")

	if err := fs.Parse([]string{"-h"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !flags.Help {
		t.Fatalf("expected help flag set")
	}
}

func TestVersionFlag(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	flags := AddHelpVersionFlags(fs, "", "")

	if err := fs.Parse([]string{"--version"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !flags.Version {
		t.Fatalf("expected version flag set")
	}
}
