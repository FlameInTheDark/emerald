package skills

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultPollInterval = 2 * time.Second

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
	Content     string `json:"content"`
}

type Summary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
}

type Reader interface {
	List() []Summary
	SummaryText() string
	GetByName(name string) (Skill, bool)
}

type ManagedReader interface {
	Reader
	Start(parent context.Context) error
	Stop()
}

type fileState struct {
	Size    int64
	ModTime time.Time
}

type Store struct {
	dir          string
	dirResolver  func() (string, error)
	pollInterval time.Duration

	mu      sync.RWMutex
	skills  map[string]Skill
	summary []Summary
	files   map[string]fileState

	stopOnce sync.Once
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewStore(dir string, pollInterval time.Duration) *Store {
	return NewResolvingStore(func() (string, error) {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			return "", fmt.Errorf("skills directory is required")
		}

		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return "", fmt.Errorf("resolve skills directory: %w", err)
		}
		return abs, nil
	}, pollInterval)
}

func NewResolvingStore(dirResolver func() (string, error), pollInterval time.Duration) *Store {
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	return &Store{
		dirResolver:  dirResolver,
		pollInterval: pollInterval,
		skills:       make(map[string]Skill),
		summary:      make([]Summary, 0),
		files:        make(map[string]fileState),
	}
}

func (s *Store) Start(parent context.Context) error {
	if _, err := s.refreshDirectory(); err != nil {
		return err
	}
	if err := s.reload(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.wg.Add(1)
	go s.watch(ctx)

	return nil
}

func (s *Store) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	})
}

func (s *Store) List() []Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Summary, len(s.summary))
	copy(result, s.summary)
	return result
}

func (s *Store) SummaryText() string {
	items := s.List()
	if len(items) == 0 {
		return ""
	}

	lines := make([]string, 0, len(items))
	for _, item := range items {
		line := "- " + item.Name
		if description := strings.TrimSpace(item.Description); description != "" {
			line += ": " + description
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (s *Store) GetByName(name string) (Skill, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	skill, ok := s.skills[normalizeName(name)]
	if !ok {
		return Skill{}, false
	}

	return skill, true
}

func (s *Store) watch(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dirChanged, err := s.refreshDirectory()
			if err != nil {
				continue
			}

			changed := dirChanged
			if !changed {
				changed, err = s.hasChanges()
			}
			if err != nil || !changed {
				continue
			}
			_ = s.reload()
		}
	}
}

func (s *Store) hasChanges() (bool, error) {
	dir, err := s.currentDirectory()
	if err != nil {
		return false, err
	}

	files, err := scanSkillFiles(dir)
	if err != nil {
		return false, fmt.Errorf("scan skill files: %w", err)
	}

	s.mu.RLock()
	current := make(map[string]fileState, len(s.files))
	for path, state := range s.files {
		current[path] = state
	}
	s.mu.RUnlock()

	if len(files) != len(current) {
		return true, nil
	}

	for path, state := range files {
		existing, ok := current[path]
		if !ok {
			return true, nil
		}
		if existing.Size != state.Size || !existing.ModTime.Equal(state.ModTime) {
			return true, nil
		}
	}

	return false, nil
}

func (s *Store) reload() error {
	dir, err := s.currentDirectory()
	if err != nil {
		return err
	}

	skills, files, err := loadSkills(dir)
	if err != nil {
		return fmt.Errorf("load skills: %w", err)
	}

	summaries := make([]Summary, 0, len(skills))
	for _, skill := range skills {
		summaries = append(summaries, Summary{
			Name:        skill.Name,
			Description: skill.Description,
			Path:        skill.Path,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
	})

	s.mu.Lock()
	s.skills = skills
	s.summary = summaries
	s.files = files
	s.mu.Unlock()

	return nil
}

func (s *Store) refreshDirectory() (bool, error) {
	if s == nil {
		return false, fmt.Errorf("skill store is required")
	}

	resolved, err := s.resolveDirectory()
	if err != nil {
		return false, err
	}

	s.mu.RLock()
	current := s.dir
	s.mu.RUnlock()
	if pathsEqual(current, resolved) {
		return false, nil
	}

	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return false, fmt.Errorf("ensure skills directory: %w", err)
	}

	s.mu.Lock()
	s.dir = resolved
	s.mu.Unlock()
	return true, nil
}

func (s *Store) resolveDirectory() (string, error) {
	if s == nil || s.dirResolver == nil {
		return "", fmt.Errorf("skills directory resolver is not configured")
	}

	dir, err := s.dirResolver()
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return "", fmt.Errorf("skills directory is required")
	}

	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve skills directory: %w", err)
	}
	return abs, nil
}

func (s *Store) currentDirectory() (string, error) {
	s.mu.RLock()
	dir := s.dir
	s.mu.RUnlock()
	if strings.TrimSpace(dir) == "" {
		if _, err := s.refreshDirectory(); err != nil {
			return "", err
		}
		s.mu.RLock()
		dir = s.dir
		s.mu.RUnlock()
	}
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("skills directory is required")
	}
	return dir, nil
}

func loadSkills(dir string) (map[string]Skill, map[string]fileState, error) {
	files, err := scanSkillFiles(dir)
	if err != nil {
		return nil, nil, err
	}

	skills := make(map[string]Skill)
	for path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		skill := parseSkillFile(path, content)
		if strings.TrimSpace(skill.Name) == "" {
			continue
		}

		skills[normalizeName(skill.Name)] = skill
	}

	return skills, files, nil
}

func scanSkillFiles(dir string) (map[string]fileState, error) {
	files := make(map[string]fileState)

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() || !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		files[path] = fileState{
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

func parseSkillFile(path string, content []byte) Skill {
	name := strings.TrimSpace(filepath.Base(filepath.Dir(path)))
	description := ""

	scanner := bufio.NewScanner(bytes.NewReader(content))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return Skill{
			Name:        name,
			Description: description,
			Path:        path,
			Content:     string(content),
		}
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			break
		}
		if strings.HasPrefix(line, "name:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			value = strings.Trim(value, `"'`)
			if value != "" {
				name = value
			}
			continue
		}
		if strings.HasPrefix(line, "description:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			description = strings.Trim(value, `"'`)
		}
	}

	return Skill{
		Name:        name,
		Description: description,
		Path:        path,
		Content:     string(content),
	}
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
