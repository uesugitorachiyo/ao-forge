package issuerepair

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const stateDirectoryName = ".forge-issue-repair"
const gitBoundaryTimeout = 30 * time.Second

type Workspace struct {
	Root      string `json:"root"`
	StateRoot string `json:"state_root"`
	StatePath string `json:"state_path"`
	LockPath  string `json:"lock_path"`
}

func openWorkspace(repositoryRoot, stateRoot string) (Workspace, error) {
	root, err := existingCanonicalDirectory(repositoryRoot)
	if err != nil {
		return Workspace{}, err
	}
	if stateRoot == "" {
		return Workspace{}, errors.New("issue-repair state root is required")
	}
	stateAbsolute, err := filepath.Abs(stateRoot)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve issue-repair state root: %w", err)
	}
	if info, err := os.Lstat(stateAbsolute); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return Workspace{}, ErrUnsafeWorkspace
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Workspace{}, fmt.Errorf("inspect issue-repair state root: %w", err)
	} else if err := os.MkdirAll(stateAbsolute, 0o700); err != nil {
		return Workspace{}, fmt.Errorf("create issue-repair state root: %w", err)
	}
	stateCanonical, err := filepath.EvalSymlinks(stateAbsolute)
	if err != nil {
		return Workspace{}, fmt.Errorf("canonicalize issue-repair state root: %w", err)
	}
	if pathWithin(root, stateCanonical) || pathWithin(stateCanonical, root) {
		return Workspace{}, ErrUnsafeWorkspace
	}

	stateDirectory := filepath.Join(stateCanonical, stateDirectoryName)
	if info, err := os.Lstat(stateDirectory); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return Workspace{}, ErrUnsafeWorkspace
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Workspace{}, fmt.Errorf("inspect issue-repair state directory: %w", err)
	} else if err := os.Mkdir(stateDirectory, 0o700); err != nil {
		return Workspace{}, fmt.Errorf("create issue-repair state directory: %w", err)
	}

	return Workspace{
		Root:      root,
		StateRoot: stateCanonical,
		StatePath: filepath.Join(stateDirectory, "state.json"),
		LockPath:  filepath.Join(stateDirectory, "state.lock"),
	}, nil
}

func (workspace Workspace) verifyClone(expectedBase string) (string, error) {
	topLevel, err := boundedGit(workspace.Root, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrUnsafeWorkspace
	}
	canonicalTop, err := filepath.EvalSymlinks(strings.TrimSpace(topLevel))
	if err != nil || canonicalTop != workspace.Root {
		return "", ErrUnsafeWorkspace
	}
	head, err := boundedGit(workspace.Root, "rev-parse", "HEAD")
	if err != nil {
		return "", ErrUnsafeWorkspace
	}
	if strings.TrimSpace(head) != expectedBase {
		return "", ErrStaleBase
	}
	status, err := boundedGit(workspace.Root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || len(bytes.TrimSpace([]byte(status))) != 0 {
		return "", ErrUnsafeWorkspace
	}
	sum := sha256.Sum256([]byte(workspace.Root + "\n" + strings.TrimSpace(head) + "\nclean\n"))
	return hex.EncodeToString(sum[:]), nil
}

func (workspace Workspace) patchStatusDigest() (string, error) {
	status, err := boundedGit(workspace.Root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || len(bytes.TrimSpace([]byte(status))) == 0 {
		return "", ErrUnsafeWorkspace
	}
	sum := sha256.Sum256([]byte(status))
	return hex.EncodeToString(sum[:]), nil
}

func boundedGit(root string, args ...string) (string, error) {
	return runBoundedCommand(root, gitBoundaryTimeout, "git", append([]string{"-C", root}, args...)...)
}

func runBoundedCommand(root string, timeout time.Duration, executable string, args ...string) (string, error) {
	commandContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(commandContext, executable, args...)
	command.Dir = root
	var output limitedBuffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		if commandContext.Err() != nil {
			return "", ErrUnsafeWorkspace
		}
		return "", err
	}
	if output.overflow {
		return "", ErrUnsafeWorkspace
	}
	return output.String(), nil
}

func existingCanonicalDirectory(path string) (string, error) {
	if path == "" {
		return "", errors.New("issue-repair repository workspace is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve issue-repair repository workspace: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect issue-repair repository workspace: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", ErrUnsafeWorkspace
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonicalize issue-repair repository workspace: %w", err)
	}
	return canonical, nil
}

func pathWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
