package filetools

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"
)

const (
	defaultReadLimit       = 200
	maxReadLimit           = 600
	maxPreviewLines        = 40
	maxListEntries         = 200
	maxGlobMatches         = 200
	maxGrepMatches         = 120
	maxFileSizeBytes int64 = 2 * 1024 * 1024
)

// Service exposes workspace-scoped file exploration and editing helpers.
type Service struct {
	root string
}

// Entry describes one filesystem item returned by ListDirectory.
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// ListDirectoryResult is the structured output of ListDirectory.
type ListDirectoryResult struct {
	Path      string  `json:"path"`
	Entries   []Entry `json:"entries"`
	Count     int     `json:"count"`
	Truncated bool    `json:"truncated"`
}

// GlobFilesResult is the structured output of GlobFiles.
type GlobFilesResult struct {
	BasePath  string   `json:"base_path"`
	Pattern   string   `json:"pattern"`
	Matches   []string `json:"matches"`
	Count     int      `json:"count"`
	Truncated bool     `json:"truncated"`
}

// GrepMatch describes one grep match.
type GrepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// GrepFilesResult is the structured output of GrepFiles.
type GrepFilesResult struct {
	BasePath  string      `json:"base_path"`
	Pattern   string      `json:"pattern"`
	Include   string      `json:"include,omitempty"`
	Matches   []GrepMatch `json:"matches"`
	Count     int         `json:"count"`
	Truncated bool        `json:"truncated"`
}

// ReadFileResult is the structured output of ReadFile.
type ReadFileResult struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated"`
}

// FileChangeResult is the structured output of EditFile and WriteFile.
type FileChangeResult struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Diff      string `json:"diff"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changed   bool   `json:"changed"`
}

// New creates a workspace-scoped file service rooted at the supplied directory.
func New(root string) (*Service, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return nil, fmt.Errorf("workspace root is required")
	}

	absoluteRoot, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("resolve workspace symlinks: %w", err)
		}
		resolvedRoot = absoluteRoot
	}

	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return nil, fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root %q is not a directory", resolvedRoot)
	}

	return &Service{root: filepath.Clean(resolvedRoot)}, nil
}

// Root returns the normalized workspace root directory.
func (s *Service) Root() string {
	return s.root
}

// ListDirectory returns directory entries under the workspace root.
func (s *Service) ListDirectory(path string) (*ListDirectoryResult, error) {
	target, rel, err := s.resolveExistingPath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", rel, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", rel)
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, fmt.Errorf("read directory %q: %w", rel, err)
	}

	items := make([]Entry, 0, minInt(len(entries), maxListEntries))
	for _, entry := range entries {
		entryRel := cleanRelPath(filepath.ToSlash(filepath.Join(rel, entry.Name())))
		entryType := "file"
		if entry.IsDir() {
			entryType = "directory"
		} else if entry.Type()&os.ModeSymlink != 0 {
			entryType = "symlink"
		}
		items = append(items, Entry{
			Name: entry.Name(),
			Path: entryRel,
			Type: entryType,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Type == items[j].Type {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		if items[i].Type == "directory" {
			return true
		}
		if items[j].Type == "directory" {
			return false
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	result := items
	truncated := false
	if len(result) > maxListEntries {
		result = result[:maxListEntries]
		truncated = true
	}

	return &ListDirectoryResult{
		Path:      rel,
		Entries:   result,
		Count:     len(items),
		Truncated: truncated,
	}, nil
}

// GlobFiles finds files that match the supplied pattern under the workspace root.
func (s *Service) GlobFiles(pattern string, basePath string) (*GlobFilesResult, error) {
	normalizedPattern := strings.TrimSpace(pattern)
	if normalizedPattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	base, rel, err := s.resolveExistingPath(basePath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(base)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", rel, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", rel)
	}

	matcher, err := compileGlobMatcher(normalizedPattern)
	if err != nil {
		return nil, err
	}

	matches := make([]string, 0, 32)
	truncated := false
	err = filepath.WalkDir(base, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}

		currentRel, relErr := filepath.Rel(base, current)
		if relErr != nil {
			return relErr
		}
		currentRel = filepath.ToSlash(filepath.Clean(currentRel))
		if !matcher.MatchString(currentRel) {
			return nil
		}

		workspaceRel, relErr := filepath.Rel(s.root, current)
		if relErr != nil {
			return relErr
		}
		matches = append(matches, cleanRelPath(filepath.ToSlash(workspaceRel)))
		if len(matches) >= maxGlobMatches {
			truncated = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk files for %q: %w", rel, err)
	}

	sort.Strings(matches)
	return &GlobFilesResult{
		BasePath:  rel,
		Pattern:   normalizedPattern,
		Matches:   matches,
		Count:     len(matches),
		Truncated: truncated,
	}, nil
}

// GrepFiles searches text files for matches under the workspace root.
func (s *Service) GrepFiles(pattern string, basePath string, include string) (*GrepFilesResult, error) {
	normalizedPattern := strings.TrimSpace(pattern)
	if normalizedPattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	searchPattern, err := regexp.Compile(normalizedPattern)
	if err != nil {
		return nil, fmt.Errorf("compile pattern: %w", err)
	}

	base, rel, err := s.resolveExistingPath(basePath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(base)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", rel, err)
	}

	var includeMatcher *regexp.Regexp
	if strings.TrimSpace(include) != "" {
		includeMatcher, err = compileGlobMatcher(include)
		if err != nil {
			return nil, err
		}
	}

	matches := make([]GrepMatch, 0, 32)
	truncated := false
	visitFile := func(current string, entry fs.DirEntry) error {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if currentInfo, statErr := entry.Info(); statErr == nil && currentInfo.Size() > maxFileSizeBytes {
			return nil
		}
		if isBinary, binErr := pathLooksBinary(current); binErr == nil && isBinary {
			return nil
		}

		relativeFromBase, relErr := filepath.Rel(base, current)
		if relErr != nil {
			return relErr
		}
		relativeFromBase = filepath.ToSlash(filepath.Clean(relativeFromBase))
		if includeMatcher != nil && !includeMatcher.MatchString(relativeFromBase) {
			return nil
		}

		content, readErr := os.ReadFile(current)
		if readErr != nil {
			return readErr
		}

		workspaceRel, relErr := filepath.Rel(s.root, current)
		if relErr != nil {
			return relErr
		}
		workspaceRel = cleanRelPath(filepath.ToSlash(workspaceRel))

		lines := splitLinesPreserveFinalLine(string(content))
		for index, line := range lines {
			if !searchPattern.MatchString(line) {
				continue
			}
			matches = append(matches, GrepMatch{
				Path: workspaceRel,
				Line: index + 1,
				Text: line,
			})
			if len(matches) >= maxGrepMatches {
				truncated = true
				return fs.SkipAll
			}
		}
		return nil
	}

	if info.IsDir() {
		err = filepath.WalkDir(base, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			return visitFile(current, entry)
		})
	} else {
		entry := fs.FileInfoToDirEntry(info)
		err = visitFile(base, entry)
		if err == fs.SkipAll {
			err = nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("grep files in %q: %w", rel, err)
	}

	return &GrepFilesResult{
		BasePath:  rel,
		Pattern:   normalizedPattern,
		Include:   strings.TrimSpace(include),
		Matches:   matches,
		Count:     len(matches),
		Truncated: truncated,
	}, nil
}

// ReadFile reads a text file from the workspace root using 1-indexed line windows.
func (s *Service) ReadFile(path string, offset int, limit int) (*ReadFileResult, error) {
	target, rel, err := s.resolveExistingPath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", rel, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%q is a directory; use list_directory instead", rel)
	}
	if info.Size() > maxFileSizeBytes {
		return nil, fmt.Errorf("%q is larger than %d bytes", rel, maxFileSizeBytes)
	}

	isBinary, err := pathLooksBinary(target)
	if err != nil {
		return nil, fmt.Errorf("inspect %q: %w", rel, err)
	}
	if isBinary {
		return nil, fmt.Errorf("%q looks like a binary file", rel)
	}

	contentBytes, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", rel, err)
	}

	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		limit = defaultReadLimit
	}
	if limit > maxReadLimit {
		limit = maxReadLimit
	}

	lines := splitLinesPreserveFinalLine(string(contentBytes))
	totalLines := len(lines)
	if totalLines == 0 {
		totalLines = 1
		lines = []string{""}
	}
	if offset > totalLines {
		return nil, fmt.Errorf("offset %d is out of range for %q (%d lines)", offset, rel, totalLines)
	}

	end := minInt(totalLines, offset+limit-1)
	window := lines[offset-1 : end]
	numbered := make([]string, 0, len(window))
	for index, line := range window {
		numbered = append(numbered, fmt.Sprintf("%d: %s", offset+index, line))
	}

	return &ReadFileResult{
		Path:       rel,
		Content:    strings.Join(numbered, "\n"),
		StartLine:  offset,
		EndLine:    end,
		TotalLines: totalLines,
		Truncated:  end < totalLines,
	}, nil
}

// EditFile replaces oldString with newString in a workspace file and returns a unified diff.
func (s *Service) EditFile(path string, oldString string, newString string, replaceAll bool) (*FileChangeResult, error) {
	if strings.TrimSpace(oldString) == "" {
		return nil, fmt.Errorf("old_string is required")
	}
	if oldString == newString {
		return nil, fmt.Errorf("old_string and new_string must be different")
	}

	target, rel, err := s.resolveExistingPath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", rel, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%q is a directory; use write_file for file creation", rel)
	}
	if info.Size() > maxFileSizeBytes {
		return nil, fmt.Errorf("%q is larger than %d bytes", rel, maxFileSizeBytes)
	}

	isBinary, err := pathLooksBinary(target)
	if err != nil {
		return nil, fmt.Errorf("inspect %q: %w", rel, err)
	}
	if isBinary {
		return nil, fmt.Errorf("%q looks like a binary file", rel)
	}

	currentBytes, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", rel, err)
	}
	current := string(currentBytes)

	lineEnding := detectLineEnding(current)
	normalizedOld := applyLineEnding(normalizeLineEndings(oldString), lineEnding)
	normalizedNew := applyLineEnding(normalizeLineEndings(newString), lineEnding)

	occurrences := strings.Count(current, normalizedOld)
	if occurrences == 0 {
		return nil, fmt.Errorf("old_string was not found in %q", rel)
	}
	if occurrences > 1 && !replaceAll {
		return nil, fmt.Errorf("old_string matched %d times in %q; set replace_all to true or use a more specific old_string", occurrences, rel)
	}

	updated := strings.Replace(current, normalizedOld, normalizedNew, replaceCount(replaceAll))
	change, err := s.writeChange(target, rel, current, updated, "updated")
	if err != nil {
		return nil, err
	}
	if !change.Changed {
		return nil, fmt.Errorf("edit produced no changes for %q", rel)
	}
	return change, nil
}

// WriteFile creates or replaces a workspace file and returns a unified diff.
func (s *Service) WriteFile(path string, content string) (*FileChangeResult, error) {
	target, rel, existed, err := s.resolveWritablePath(path)
	if err != nil {
		return nil, err
	}

	current := ""
	if existed {
		info, statErr := os.Stat(target)
		if statErr != nil {
			return nil, fmt.Errorf("stat %q: %w", rel, statErr)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%q is a directory", rel)
		}
		if info.Size() > maxFileSizeBytes {
			return nil, fmt.Errorf("%q is larger than %d bytes", rel, maxFileSizeBytes)
		}
		isBinary, binErr := pathLooksBinary(target)
		if binErr != nil {
			return nil, fmt.Errorf("inspect %q: %w", rel, binErr)
		}
		if isBinary {
			return nil, fmt.Errorf("%q looks like a binary file", rel)
		}

		currentBytes, readErr := os.ReadFile(target)
		if readErr != nil {
			return nil, fmt.Errorf("read %q: %w", rel, readErr)
		}
		current = string(currentBytes)
	}

	operation := "created"
	if existed {
		operation = "updated"
	}
	return s.writeChange(target, rel, current, content, operation)
}

func (s *Service) writeChange(target string, rel string, before string, after string, operation string) (*FileChangeResult, error) {
	diff, err := buildUnifiedDiff(rel, before, after)
	if err != nil {
		return nil, err
	}
	additions, deletions := diffStats(diff)
	changed := before != after
	if changed {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("create parent directory for %q: %w", rel, err)
		}
		if err := os.WriteFile(target, []byte(after), 0o644); err != nil {
			return nil, fmt.Errorf("write %q: %w", rel, err)
		}
	}

	return &FileChangeResult{
		Path:      rel,
		Operation: operation,
		Diff:      diff,
		Additions: additions,
		Deletions: deletions,
		Changed:   changed,
	}, nil
}

func (s *Service) resolveExistingPath(path string) (string, string, error) {
	target, rel, err := s.resolvePath(path, false)
	if err != nil {
		return "", "", err
	}
	return target, rel, nil
}

func (s *Service) resolveWritablePath(path string) (string, string, bool, error) {
	target, rel, err := s.resolvePath(path, true)
	if err != nil {
		return "", "", false, err
	}
	_, statErr := os.Stat(target)
	if statErr == nil {
		return target, rel, true, nil
	}
	if os.IsNotExist(statErr) {
		return target, rel, false, nil
	}
	return "", "", false, fmt.Errorf("stat %q: %w", rel, statErr)
}

func (s *Service) resolvePath(path string, allowMissing bool) (string, string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		trimmed = "."
	}
	if filepath.IsAbs(trimmed) {
		return "", "", fmt.Errorf("absolute paths are not allowed; use workspace-relative paths")
	}

	candidate := filepath.Clean(filepath.Join(s.root, trimmed))
	if !isWithinRoot(s.root, candidate) {
		return "", "", fmt.Errorf("path %q escapes the workspace root", path)
	}

	resolved, err := s.resolveSymlinksWithinRoot(candidate, allowMissing)
	if err != nil {
		return "", "", err
	}

	rel, err := filepath.Rel(s.root, resolved)
	if err != nil {
		return "", "", fmt.Errorf("resolve relative path: %w", err)
	}
	return resolved, cleanRelPath(filepath.ToSlash(rel)), nil
}

func (s *Service) resolveSymlinksWithinRoot(candidate string, allowMissing bool) (string, error) {
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		if !isWithinRoot(s.root, resolved) {
			return "", fmt.Errorf("path %q resolves outside the workspace root", candidate)
		}
		return filepath.Clean(resolved), nil
	} else if !allowMissing || !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve path %q: %w", candidate, err)
	}

	parent := filepath.Dir(candidate)
	parentResolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		if os.IsNotExist(err) {
			parentResolved = parent
		} else {
			return "", fmt.Errorf("resolve parent path %q: %w", parent, err)
		}
	}
	if !isWithinRoot(s.root, parentResolved) {
		return "", fmt.Errorf("path %q resolves outside the workspace root", candidate)
	}
	return candidate, nil
}

func isWithinRoot(root string, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	relative = filepath.Clean(relative)
	return relative == "." || (!strings.HasPrefix(relative, "..") && relative != ".." && !filepath.IsAbs(relative))
}

func compileGlobMatcher(pattern string) (*regexp.Regexp, error) {
	normalized := filepath.ToSlash(strings.TrimSpace(pattern))
	if normalized == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	var builder strings.Builder
	builder.WriteString("^")
	for index := 0; index < len(normalized); index++ {
		char := normalized[index]
		switch char {
		case '*':
			if index+1 < len(normalized) && normalized[index+1] == '*' {
				if index+2 < len(normalized) && normalized[index+2] == '/' {
					builder.WriteString(`(?:.*/)?`)
					index += 2
				} else {
					builder.WriteString(".*")
					index++
				}
			} else {
				builder.WriteString(`[^/]*`)
			}
		case '?':
			builder.WriteString(`[^/]`)
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			builder.WriteByte('\\')
			builder.WriteByte(char)
		default:
			builder.WriteByte(char)
		}
	}
	builder.WriteString("$")

	matcher, err := regexp.Compile(builder.String())
	if err != nil {
		return nil, fmt.Errorf("compile glob pattern %q: %w", pattern, err)
	}
	return matcher, nil
}

func pathLooksBinary(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = file.Close()
	}()

	buffer := make([]byte, 8192)
	count, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false, err
	}
	return looksBinary(buffer[:count]), nil
}

func looksBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	return !utf8.Valid(data)
}

func buildUnifiedDiff(path string, before string, after string) (string, error) {
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(normalizeLineEndings(before)),
		B:        difflib.SplitLines(normalizeLineEndings(after)),
		FromFile: path,
		ToFile:   path,
		Context:  3,
	})
	if err != nil {
		return "", fmt.Errorf("build unified diff for %q: %w", path, err)
	}
	return strings.TrimSpace(diff), nil
}

func diffStats(diff string) (int, int) {
	additions := 0
	deletions := 0
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "@@") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			additions++
			continue
		}
		if strings.HasPrefix(line, "-") {
			deletions++
		}
	}
	return additions, deletions
}

func splitLinesPreserveFinalLine(value string) []string {
	normalized := normalizeLineEndings(value)
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func normalizeLineEndings(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func detectLineEnding(value string) string {
	if strings.Contains(value, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func applyLineEnding(value string, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(value, "\n", "\r\n")
	}
	return value
}

func replaceCount(replaceAll bool) int {
	if replaceAll {
		return -1
	}
	return 1
}

func cleanRelPath(value string) string {
	cleaned := strings.TrimSpace(filepath.ToSlash(filepath.Clean(value)))
	if cleaned == "" || cleaned == "." {
		return "."
	}
	return cleaned
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
