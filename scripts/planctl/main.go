package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "current":
		runCurrent(os.Args[2:])
	case "list":
		runList(os.Args[2:])
	case "find":
		runFind(os.Args[2:])
	case "set":
		runSet(os.Args[2:])
	case "complete-l1":
		runCompleteL1(os.Args[2:])
	case "insert-l2":
		runInsertL2(os.Args[2:])
	case "show":
		runShow(os.Args[2:])
	case "lint":
		runLint(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	lines := []string{
		"planctl - PLAN.org helper",
		"",
		"Usage:",
		"  planctl <command> [options]",
		"",
		"Commands:",
		"  current      Show current WIP L1/L2",
		"  list         List headings",
		"  find         Find headings by title substring",
		"  set          Set status for a heading",
		"  complete-l1  Mark an L1 DONE and clear L2 status flags",
		"  insert-l2    Insert a TODO L2 under an L1",
		"  show         Print an L1 section",
		"  lint         Report multiple WIP headings",
		"",
		"Common options:",
		"  --file PATH  PLAN.org path (default: PLAN.org)",
	}
	fmt.Fprintln(os.Stderr, strings.Join(lines, "\n"))
}

func parseFlags(fs *flag.FlagSet, args []string) {
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
}

func loadPlanFromFile(path string) (Plan, error) {
	lines, err := readLines(path)
	if err != nil {
		return Plan{}, err
	}
	return parsePlan(lines), nil
}

func runCurrent(args []string) {
	fs := flag.NewFlagSet("current", flag.ContinueOnError)
	file := fs.String("file", "PLAN.org", "PLAN.org path")
	parseFlags(fs, args)

	plan, err := loadPlanFromFile(*file)
	if err != nil {
		die(err)
	}
	results, warnings := current(plan)
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, warning)
	}
	printResults(results)
}

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	file := fs.String("file", "PLAN.org", "PLAN.org path")
	level := fs.Int("level", 1, "heading level (0=all,1=L1,2=L2)")
	status := fs.String("status", "", "status filter (comma-separated TODO,WIP,DONE,NONE)")
	parseFlags(fs, args)

	plan, err := loadPlanFromFile(*file)
	if err != nil {
		die(err)
	}
	filter := statusFilter(*status)
	results := listHeadings(plan, *level, filter)
	printResults(results)
}

func runFind(args []string) {
	fs := flag.NewFlagSet("find", flag.ContinueOnError)
	file := fs.String("file", "PLAN.org", "PLAN.org path")
	level := fs.Int("level", 0, "heading level (0=all,1=L1,2=L2)")
	status := fs.String("status", "", "status filter (comma-separated TODO,WIP,DONE,NONE)")
	query := fs.String("query", "", "substring to search for")
	parseFlags(fs, args)

	plan, err := loadPlanFromFile(*file)
	if err != nil {
		die(err)
	}
	filter := statusFilter(*status)
	results := findHeadings(plan, *level, filter, *query)
	printResults(results)
}

func runSet(args []string) {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	file := fs.String("file", "PLAN.org", "PLAN.org path")
	level := fs.Int("level", 0, "heading level (1=L1,2=L2)")
	title := fs.String("title", "", "exact heading title")
	status := fs.String("status", "", "status to set (TODO,WIP,DONE,NONE)")
	parseFlags(fs, args)

	if *level == 0 {
		die(fmt.Errorf("level is required"))
	}
	if *title == "" {
		die(fmt.Errorf("title is required"))
	}
	if *status == "" {
		die(fmt.Errorf("status is required"))
	}

	lines, err := readLines(*file)
	if err != nil {
		die(err)
	}
	updated, err := setStatus(lines, *level, *title, *status)
	if err != nil {
		die(err)
	}
	if err := writeLines(*file, updated); err != nil {
		die(err)
	}
}

func runCompleteL1(args []string) {
	fs := flag.NewFlagSet("complete-l1", flag.ContinueOnError)
	file := fs.String("file", "PLAN.org", "PLAN.org path")
	title := fs.String("title", "", "exact L1 title")
	parseFlags(fs, args)

	if *title == "" {
		die(fmt.Errorf("title is required"))
	}
	lines, err := readLines(*file)
	if err != nil {
		die(err)
	}
	updated, err := completeL1(lines, *title)
	if err != nil {
		die(err)
	}
	if err := writeLines(*file, updated); err != nil {
		die(err)
	}
}

func runInsertL2(args []string) {
	fs := flag.NewFlagSet("insert-l2", flag.ContinueOnError)
	file := fs.String("file", "PLAN.org", "PLAN.org path")
	l1Title := fs.String("l1-title", "", "exact L1 title")
	title := fs.String("title", "", "new L2 title")
	priority := fs.String("priority", "", "priority letter (optional)")
	parseFlags(fs, args)

	if *l1Title == "" {
		die(fmt.Errorf("l1-title is required"))
	}
	if *title == "" {
		die(fmt.Errorf("title is required"))
	}
	lines, err := readLines(*file)
	if err != nil {
		die(err)
	}
	updated, err := insertL2(lines, *l1Title, *title, *priority)
	if err != nil {
		die(err)
	}
	if err := writeLines(*file, updated); err != nil {
		die(err)
	}
}

func runShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	file := fs.String("file", "PLAN.org", "PLAN.org path")
	title := fs.String("title", "", "exact L1 title")
	parseFlags(fs, args)

	if *title == "" {
		die(fmt.Errorf("title is required"))
	}
	lines, err := readLines(*file)
	if err != nil {
		die(err)
	}
	section, err := showL1(lines, *title)
	if err != nil {
		die(err)
	}
	for _, line := range section {
		fmt.Println(line)
	}
}

func runLint(args []string) {
	fs := flag.NewFlagSet("lint", flag.ContinueOnError)
	file := fs.String("file", "PLAN.org", "PLAN.org path")
	parseFlags(fs, args)

	plan, err := loadPlanFromFile(*file)
	if err != nil {
		die(err)
	}
	results, violations := lint(plan)
	printResults(results)
	if violations {
		os.Exit(1)
	}
}

func printResults(results []Result) {
	for _, result := range results {
		fmt.Println(result.Format())
	}
}

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
