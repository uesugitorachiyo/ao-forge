package foundation

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run git %v in %s: %v (output: %q)", args, dir, err, string(out))
	}
}

func TestDoctorVerifiesBaselineStates(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping integration tests")
	}

	tmp, err := os.MkdirTemp("", "doctor-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	compNames := []string{"repo1", "repo2"}
	compPaths := make(map[string]string)
	compCommits := make(map[string]string)

	for _, name := range compNames {
		path := filepath.Join(tmp, name)
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		compPaths[name] = path

		runGitCmd(t, path, "init")
		runGitCmd(t, path, "config", "user.name", "Test User")
		runGitCmd(t, path, "config", "user.email", "test@example.com")
		runGitCmd(t, path, "config", "commit.gpgsign", "false")

		dummyFile := filepath.Join(path, "file.txt")
		if err := os.WriteFile(dummyFile, []byte("content"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		runGitCmd(t, path, "add", "file.txt")
		runGitCmd(t, path, "commit", "-m", "initial commit")

		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = path
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("rev-parse HEAD: %v", err)
		}
		commit := strings.TrimSpace(string(out))
		compCommits[name] = commit

		runGitCmd(t, path, "tag", "-m", "release", "v1.0.0")
	}

	baseline := Baseline{
		SchemaVersion: "ao.forge.foundation-baseline.v0.1",
		VerifiedAt:    "2026-06-17",
		Status:        "ready_for_ao_forge_phase_0",
		Components: []Component{
			{
				Name:       "repo1",
				Role:       "r1",
				Repository: "repo1",
				LocalPath:  "repo1",
				Branch:     "master",
				Commit:     compCommits["repo1"],
				Release:    "v1.0.0",
			},
			{
				Name:       "repo2",
				Role:       "r2",
				Repository: "repo2",
				LocalPath:  "repo2",
				Branch:     "master",
				Commit:     compCommits["repo2"],
				Release:    "v1.0.0",
			},
		},
	}

	for i, comp := range baseline.Components {
		actualBranch, err := runGit(compPaths[comp.Name], "rev-parse", "--abbrev-ref", "HEAD")
		if err == nil {
			baseline.Components[i].Branch = actualBranch
		}
	}

	baselineBytes, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}

	baselinePath := filepath.Join(tmp, "baseline.json")
	if err := os.WriteFile(baselinePath, baselineBytes, 0644); err != nil {
		t.Fatalf("write baseline file: %v", err)
	}

	// 1. Success case
	res, err := RunDoctor(baselinePath)
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if res.Status != "passed" {
		t.Fatalf("expected status passed, got failed (problem: %q)", res.Problem)
	}

	// 2. Wrong commit case
	baseline.Components[0].Commit = "0000000000000000000000000000000000000000"
	baselineBytes, _ = json.Marshal(baseline)
	os.WriteFile(baselinePath, baselineBytes, 0644)
	res, err = RunDoctor(baselinePath)
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if res.Status != "failed" || !strings.Contains(res.Problem, "HEAD commit") {
		t.Fatalf("expected wrong commit failure, got status %q, problem %q", res.Status, res.Problem)
	}

	baseline.Components[0].Commit = compCommits["repo1"]

	// 3. Wrong branch case
	baseline.Components[0].Branch = "nonexistent-branch"
	baselineBytes, _ = json.Marshal(baseline)
	os.WriteFile(baselinePath, baselineBytes, 0644)
	res, err = RunDoctor(baselinePath)
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if res.Status != "failed" || !strings.Contains(res.Problem, "is on branch") {
		t.Fatalf("expected wrong branch failure, got status %q, problem %q", res.Status, res.Problem)
	}

	actualBranch, _ := runGit(compPaths["repo1"], "rev-parse", "--abbrev-ref", "HEAD")
	baseline.Components[0].Branch = actualBranch

	// 4. Dirty worktree case
	dummyFile := filepath.Join(compPaths["repo1"], "dirty.txt")
	os.WriteFile(dummyFile, []byte("dirty"), 0644)
	baselineBytes, _ = json.Marshal(baseline)
	os.WriteFile(baselinePath, baselineBytes, 0644)
	res, err = RunDoctor(baselinePath)
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if res.Status != "failed" || !strings.Contains(res.Problem, "worktree is dirty") {
		t.Fatalf("expected dirty worktree failure, got status %q, problem %q", res.Status, res.Problem)
	}

	os.Remove(dummyFile)

	// 5. Missing component case
	baseline.Components[0].LocalPath = "nonexistent-dir"
	baselineBytes, _ = json.Marshal(baseline)
	os.WriteFile(baselinePath, baselineBytes, 0644)
	res, err = RunDoctor(baselinePath)
	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}
	if res.Status != "failed" || !strings.Contains(res.Problem, "does not exist") {
		t.Fatalf("expected missing component failure, got status %q, problem %q", res.Status, res.Problem)
	}
}
