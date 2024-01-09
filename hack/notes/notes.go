package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

/*
This tool generates release notes from the commits between two Git references
*/

var (
	warnings      = ":warning: Breaking Changes"
	features      = ":sparkles: New Features"
	bugs          = ":bug: Bug Fixes"
	documentation = ":book: Documentation"
	others        = ":seedling: Others"
	unknown       = ":question: Sort these by hand"

	outputOrder = []string{
		warnings,
		features,
		bugs,
		documentation,
		others,
		unknown,
	}
)

func main() {
	from := flag.String("from", "", "Include commits starting from this Git reference")
	to := flag.String("to", "", "Include commits up to and including this Git reference. Defaults to HEAD")
	flag.Parse()

	err := run(*from, *to)
	if err != nil {
		os.Exit(1)
	}
}

func run(from, to string) error {
	if to == "" {
		to = "HEAD"
	}
	var err error
	if from == "" {
		from, err = previousTag(to)
		if err != nil {
			return err
		}
	}

	cmd := exec.Command("git", "rev-list", fmt.Sprintf("%s..%s", from, to), "--pretty=format:%B")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	commits := map[string][]string{}
	for _, output := range outputOrder {
		commits[output] = []string{}
	}
	outLines := strings.Split(string(out), "\n")
	for i, line := range outLines {
		// If we have found a commit then we pick the next line
		if !strings.HasPrefix(line, "commit ") {
			continue
		}
		title := outLines[i+1]
		var key string
		switch {
		case strings.HasPrefix(title, "⚠️"):
			key = warnings
			title = strings.TrimPrefix(title, "⚠️")
		case strings.HasPrefix(title, "✨"):
			key = features
			title = strings.TrimPrefix(title, "✨")
		case strings.HasPrefix(title, "🐛"):
			key = bugs
			title = strings.TrimPrefix(title, "🐛")
		case strings.HasPrefix(title, "📖"):
			key = documentation
			title = strings.TrimPrefix(title, "📖")
		case strings.HasPrefix(title, "🌱"):
			key = others
			title = strings.TrimPrefix(title, "🌱")
		default:
			key = unknown
		}
		title = strings.TrimSpace(title)
		commits[key] = append(commits[key], title)
	}

	fmt.Printf("## Changes since [%s](https://github.com/hsbc/cost-manager/releases/%s)\n\n", from, from)
	for _, key := range outputOrder {
		commits := commits[key]
		if len(commits) == 0 {
			continue
		}

		fmt.Printf("### %s\n\n", key)
		for _, commit := range commits {
			fmt.Printf("- %s\n", commit)
		}
		fmt.Println()
	}

	return nil
}

func previousTag(to string) (string, error) {
	cmd := exec.Command("git", "describe", "--abbrev=0", "--tags", fmt.Sprintf("%s^", to))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
