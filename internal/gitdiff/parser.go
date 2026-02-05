package gitdiff

import (
	"regexp"
	"strings"
)

type Commit struct {
	Hash  string
	Files []DiffFile
}

func ParseGitLog(raw string) []Commit {
	var commits []Commit

	// Split by commit hash
	// Caveat: "commit " could appear in message, but at start of line it's usually the marker.
	// git log output always starts with "commit <hash>"

	commitChunks := strings.Split(raw, "\ncommit ")
	if len(commitChunks) > 0 && commitChunks[0] == "" {
		commitChunks = commitChunks[1:]
	}

	for _, chunk := range commitChunks {
		chunk = "commit " + chunk // Restore the "commit " prefix for first line parsing if needed, or just parse hash
		lines := strings.Split(chunk, "\n")
		if len(lines) == 0 {
			continue
		}

		hash := strings.TrimPrefix(lines[0], "commit ")
		hash = strings.Fields(hash)[0] // Handle potential extra info

		commit := Commit{
			Hash:  hash,
			Files: parseFiles(lines),
		}
		commits = append(commits, commit)
	}

	return commits
}

func parseFiles(lines []string) []DiffFile {
	var files []DiffFile
	var currentFile *DiffFile

	// We look for "diff --git a/path b/path"
	diffRe := regexp.MustCompile(`^diff --git a/(.*) b/(.*)`)
	newFileRe := regexp.MustCompile(`^new file mode`)
	deletedFileRe := regexp.MustCompile(`^deleted file mode`)

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			if currentFile != nil {
				files = append(files, *currentFile)
			}

			matches := diffRe.FindStringSubmatch(line)
			path := ""
			if len(matches) > 2 {
				path = matches[2] // Use b/path
			} else {
				// Fallback or complex filename handling
				parts := strings.Fields(line)
				if len(parts) >= 4 {
					path = strings.TrimPrefix(parts[3], "b/")
				}
			}

			currentFile = &DiffFile{
				Path:   path,
				IsTest: isTestFile(path),
			}
			continue
		}

		if currentFile == nil {
			continue
		}

		if newFileRe.MatchString(line) {
			currentFile.IsNew = true
		}
		if deletedFileRe.MatchString(line) {
			currentFile.IsDeleted = true
		}

		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			currentFile.Additions = append(currentFile.Additions, strings.TrimPrefix(line, "+"))
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			currentFile.Deletions = append(currentFile.Deletions, strings.TrimPrefix(line, "-"))
		}
	}

	if currentFile != nil {
		files = append(files, *currentFile)
	}

	return files
}
