package shellgen

import "fmt"

func BuildCodexCommand(config map[string]interface{}) []string {
	args := []string{"codex"}
	for _, entry := range FlattenConfigPreserveArrays(config) {
		if entry.Key == "notify" {
			if single, ok := entry.Value.(string); ok {
				entry.Value = []string{single}
			}
		}
		value := FormatValue(entry.Value)
		args = append(args, "-c", EscapeShellArg(fmt.Sprintf("%s=%s", entry.Key, value)))
	}
	return args
}
