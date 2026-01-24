package cli

import "flag"

const (
	defaultHelpDesc    = "Show help"
	defaultVersionDesc = "Print version and exit"
)

type HelpVersionFlags struct {
	Help    bool
	Version bool
}

func AddHelpVersionFlags(fs *flag.FlagSet, helpDesc, versionDesc string) *HelpVersionFlags {
	if fs == nil {
		return &HelpVersionFlags{}
	}
	if helpDesc == "" {
		helpDesc = defaultHelpDesc
	}
	if versionDesc == "" {
		versionDesc = defaultVersionDesc
	}
	flags := &HelpVersionFlags{}
	fs.BoolVar(&flags.Help, "help", false, helpDesc)
	fs.BoolVar(&flags.Help, "h", false, helpDesc)
	fs.BoolVar(&flags.Version, "version", false, versionDesc)
	fs.BoolVar(&flags.Version, "v", false, versionDesc)
	return flags
}
