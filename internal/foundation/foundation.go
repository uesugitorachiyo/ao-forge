package foundation

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Baseline struct {
	SchemaVersion string      `json:"schema_version"`
	VerifiedAt    string      `json:"verified_at"`
	Status        string      `json:"status"`
	Components    []Component `json:"components"`
}

type Component struct {
	Name       string `json:"name"`
	Role       string `json:"role"`
	Repository string `json:"repository"`
	LocalPath  string `json:"local_path"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	Release    string `json:"release"`
}

type ComponentResult struct {
	Name             string `json:"name"`
	LocalPath        string `json:"local_path"`
	Exists           bool   `json:"exists"`
	GitDir           bool   `json:"git_dir"`
	Branch           string `json:"branch"`
	ExpectedBranch   string `json:"expected_branch"`
	BranchOK         bool   `json:"branch_ok"`
	Commit           string `json:"commit"`
	ExpectedCommit   string `json:"expected_commit"`
	CommitOK         bool   `json:"commit_ok"`
	WorktreeClean    bool   `json:"worktree_clean"`
	ReleaseTagExists bool   `json:"release_tag_exists"`
	ExpectedRelease  string `json:"expected_release"`
}

type DoctorResult struct {
	SchemaVersion string            `json:"schema_version"`
	Status        string            `json:"status"`
	Components    []ComponentResult `json:"components"`
	Problem       string            `json:"problem,omitempty"`
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !fi.IsDir() {
			return dir, nil
		}
		if fi, err := os.Stat(filepath.Join(dir, ".git")); err == nil && fi.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return os.Getwd()
}

func runGit(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v failed: %v (stderr: %q)", args, err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func VerifyComponent(comp Component, repoRoot string, baselinePath string) ComponentResult {
	res := ComponentResult{
		Name:            comp.Name,
		LocalPath:       comp.LocalPath,
		ExpectedBranch:  comp.Branch,
		ExpectedCommit:  comp.Commit,
		ExpectedRelease: comp.Release,
	}

	absPath := filepath.Join(repoRoot, comp.LocalPath)
	if fi, err := os.Stat(absPath); err != nil || !fi.IsDir() {
		altPath := filepath.Join(filepath.Dir(baselinePath), comp.LocalPath)
		if fiAlt, errAlt := os.Stat(altPath); errAlt == nil && fiAlt.IsDir() {
			absPath = altPath
		}
	}
	absPath = filepath.Clean(absPath)

	fi, err := os.Stat(absPath)
	if err != nil || !fi.IsDir() {
		return res
	}
	res.Exists = true

	gitDir := filepath.Join(absPath, ".git")
	fiGit, errGit := os.Stat(gitDir)
	if errGit == nil && fiGit.IsDir() {
		res.GitDir = true
	} else if errGit == nil && !fiGit.IsDir() {
		if bytes, errRead := os.ReadFile(gitDir); errRead == nil {
			if strings.HasPrefix(string(bytes), "gitdir:") {
				res.GitDir = true
			}
		}
	}

	if !res.GitDir {
		return res
	}

	branch, err := runGit(absPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		res.Branch = branch
		res.BranchOK = (branch == comp.Branch)
	}

	commit, err := runGit(absPath, "rev-parse", "HEAD")
	if err == nil {
		res.Commit = commit
		res.CommitOK = (strings.ToLower(commit) == strings.ToLower(comp.Commit))
	}

	status, err := runGit(absPath, "status", "--porcelain")
	if err == nil {
		res.WorktreeClean = (len(strings.TrimSpace(status)) == 0)
	}

	tagOutput, err := runGit(absPath, "tag", "--list", comp.Release)
	if err == nil && strings.TrimSpace(tagOutput) == comp.Release {
		res.ReleaseTagExists = true
	} else {
		lsRemoteOutput, errRemote := runGit(absPath, "ls-remote", "--tags", "origin", "refs/tags/"+comp.Release)
		if errRemote == nil && strings.Contains(lsRemoteOutput, "refs/tags/"+comp.Release) {
			res.ReleaseTagExists = true
		}
	}

	return res
}

func RunDoctor(baselinePath string) (*DoctorResult, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git executable not found in PATH")
	}

	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return nil, fmt.Errorf("read baseline file: %v", err)
	}

	var base Baseline
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("unmarshal baseline: %v", err)
	}

	if base.SchemaVersion != "ao.forge.foundation-baseline.v0.1" {
		return nil, fmt.Errorf("unsupported baseline schema version: %q", base.SchemaVersion)
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return nil, err
	}

	var compResults []ComponentResult
	passed := true
	var problems []string

	for _, comp := range base.Components {
		res := VerifyComponent(comp, repoRoot, baselinePath)
		compResults = append(compResults, res)

		if !res.Exists {
			passed = false
			problems = append(problems, fmt.Sprintf("component %q path %q does not exist", comp.Name, res.LocalPath))
		} else if !res.GitDir {
			passed = false
			problems = append(problems, fmt.Sprintf("component %q is not a git repository", comp.Name))
		} else {
			if !res.BranchOK {
				passed = false
				problems = append(problems, fmt.Sprintf("component %q is on branch %q (expected %q)", comp.Name, res.Branch, comp.Branch))
			}
			if !res.CommitOK {
				passed = false
				problems = append(problems, fmt.Sprintf("component %q HEAD commit is %s (expected %s)", comp.Name, truncate(res.Commit, 12), truncate(comp.Commit, 12)))
			}
			if !res.WorktreeClean {
				passed = false
				problems = append(problems, fmt.Sprintf("component %q worktree is dirty", comp.Name))
			}
			if !res.ReleaseTagExists {
				passed = false
				problems = append(problems, fmt.Sprintf("component %q release tag %q does not exist", comp.Name, comp.Release))
			}
		}
	}

	status := "passed"
	if !passed {
		status = "failed"
	}

	problemStr := ""
	if len(problems) > 0 {
		problemStr = strings.Join(problems, "; ")
	}

	return &DoctorResult{
		SchemaVersion: "ao.forge.foundation-doctor-result.v0.1",
		Status:        status,
		Components:    compResults,
		Problem:       problemStr,
	}, nil
}

func truncate(s string, limit int) string {
	if len(s) > limit {
		return s[:limit]
	}
	return s
}
