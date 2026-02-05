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

	// Normalize input to ensure we can split reliably
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}

	// Split by "\ncommit " or just "commit " if it's the first one
	// A simple trick: add a newline at start so everyone matches \ncommit
	normalized := "\n" + raw
	commitChunks := strings.Split(normalized, "\ncommit ")

	for _, chunk := range commitChunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}

		lines := strings.Split(chunk, "\n")
		// The first line of the chunk is now JUST the hash (and maybe author info if log was diff)
		// But usually it's just the hash.
		hash := strings.Fields(lines[0])[0]
		if len(hash) > 5 {
			hash = hash[:5]
		}

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
