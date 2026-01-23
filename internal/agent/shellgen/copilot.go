package shellgen

func BuildCopilotCommand(config map[string]interface{}) []string {
	args := []string{"copilot"}
	for _, entry := range FlattenConfig(config) {
		flag := "--" + NormalizeFlag(entry.Key)
		switch value := entry.Value.(type) {
		case bool:
			if value {
				args = append(args, flag)
			} else {
				args = append(args, "--no-"+NormalizeFlag(entry.Key))
			}
		default:
			args = append(args, flag, EscapeShellArg(FormatValue(value)))
		}
	}
	return args
}
