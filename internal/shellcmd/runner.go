package shellcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const DefaultTimeout = 60 * time.Second

type Request struct {
	Command          string
	WorkingDirectory string
	Timeout          time.Duration
}

type Result struct {
	Command          string `json:"command"`
	WorkingDirectory string `json:"working_directory"`
	Stdout           string `json:"stdout,omitempty"`
	Stderr           string `json:"stderr,omitempty"`
	ExitCode         int    `json:"exit_code"`
	Success          bool   `json:"success"`
	DurationMs       int64  `json:"duration_ms"`
	TimedOut         bool   `json:"timed_out,omitempty"`
}

type Runner interface {
	Run(ctx context.Context, req Request) (*Result, error)
}

type ExecRunner struct {
	baseDir string
}

func NewRunner(baseDir string) *ExecRunner {
	return &ExecRunner{baseDir: baseDir}
}

func (r *ExecRunner) WorkspaceRoot() (string, error) {
	return r.resolveWorkingDirectory("")
}

func (r *ExecRunner) Run(ctx context.Context, req Request) (*Result, error) {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	workingDir, err := r.resolveWorkingDirectory(req.WorkingDirectory)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(runCtx, shellExecutable(), shellArgs(command)...)
	cmd.Dir = workingDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startedAt := time.Now()
	runErr := cmd.Run()
	duration := time.Since(startedAt)

	result := &Result{
		Command:          command,
		WorkingDirectory: workingDir,
		Stdout:           stdout.String(),
		Stderr:           stderr.String(),
		ExitCode:         0,
		Success:          runErr == nil,
		DurationMs:       duration.Milliseconds(),
	}

	if runErr == nil {
		return result, nil
	}

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.Success = false
		if exitErr := new(exec.ExitError); errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		result.Success = false
		return result, nil
	}

	return nil, fmt.Errorf("run shell command: %w", runErr)
}

func (r *ExecRunner) resolveWorkingDirectory(raw string) (string, error) {
	base := r.baseDir
	if strings.TrimSpace(base) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
		base = cwd
	}

	if strings.TrimSpace(raw) == "" {
		return filepath.Clean(base), nil
	}

	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}

	return filepath.Clean(filepath.Join(base, raw)), nil
}

type workspaceRootProvider interface {
	WorkspaceRoot() (string, error)
}

// ResolveWorkspaceRoot returns the workspace directory shared by local tooling.
func ResolveWorkspaceRoot(runner Runner) (string, error) {
	if provider, ok := runner.(workspaceRootProvider); ok {
		return provider.WorkspaceRoot()
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	return filepath.Clean(cwd), nil
}

func shellExecutable() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}

	return "sh"
}

func shellArgs(command string) []string {
	if runtime.GOOS == "windows" {
		return []string{"-NoProfile", "-NonInteractive", "-Command", command}
	}

	return []string{"-lc", command}
}
