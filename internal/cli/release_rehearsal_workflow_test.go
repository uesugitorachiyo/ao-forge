package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCandidateBuildIdentity(t *testing.T) {
	oldVersion, oldSource := buildVersion, buildSourceCommit
	buildVersion, buildSourceCommit = "0.1.4", strings.Repeat("a", 40)
	t.Cleanup(func() {
		buildVersion, buildSourceCommit = oldVersion, oldSource
	})

	code, stdout, stderr := runCLI("--version")
	if code != 0 {
		t.Fatalf("--version exit code = %d, stderr = %s", code, stderr)
	}
	want := "ao-forge version=0.1.4 source_sha=" + strings.Repeat("a", 40) + "\n"
	if stdout != want {
		t.Fatalf("--version output = %q, want %q", stdout, want)
	}
}

func TestReleaseRehearsalWorkflowIsReadOnlyNativeAndExactlyBound(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release-rehearsal.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)

	for _, want := range []string{
		"workflow_dispatch:",
		"source_commit:",
		"approved_manifest_base64:",
		"approved_manifest_digest:",
		"contents: read",
		"ubuntu-24.04",
		"linux-x86_64",
		"macos-15",
		"macos-aarch64",
		"windows-2025",
		"windows-x86_64",
		"scripts/discover-release-candidate-version.py",
		"scripts/validate-release-rehearsal-manifest.py",
		"scripts/build-release-rehearsal-candidate.py",
		"scripts/verify-release-rehearsal.py",
		"AO_FORGE_BUILD_VERSION",
		"AO_FORGE_BUILD_SOURCE_COMMIT",
		"candidate-summary.json",
		"provenance.json",
		"smoke-summary.json",
		"SHA256SUMS",
		"LICENSE",
		"NOTICE",
		"immutable-promotion-plan.json",
		"ao-forge-release-rehearsal-plan-${{ inputs.source_commit }}",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release rehearsal workflow missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"push:",
		"tags:",
		"contents: write",
		"id-token: write",
		"deployments: write",
		"GITHUB_TOKEN:",
		"GH_TOKEN:",
		"gh release",
		"git tag",
		"git push",
		"environment:",
		"release-publish",
		"release-rollback",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("release rehearsal workflow contains forbidden capability %q", forbidden)
		}
	}
}

func TestReleaseRehearsalRegressionSuite(t *testing.T) {
	root := repoRoot(t)
	command := exec.Command("python3", filepath.Join(root, "scripts", "test_release_rehearsal.py"))
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("release rehearsal regression suite failed: %v\n%s", err, output)
	}
}
