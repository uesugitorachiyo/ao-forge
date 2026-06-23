package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionReadinessOpsWorkflowRunsBranchProtectionVerifier(t *testing.T) {
	root := repoRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "production-readiness-ops.yml")

	workflow, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read production readiness ops workflow: %v", err)
	}
	content := string(workflow)

	for _, want := range []string{
		"name: Production Readiness Ops",
		"workflow_dispatch:",
		"schedule:",
		`cron: "17 10 * * *"`,
		"permissions:",
		"contents: read",
		"name: Branch protection drift",
		"runs-on: ubuntu-latest",
		"GH_TOKEN: ${{ github.token }}",
		"scripts/verify-branch-protection.sh",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("production readiness ops workflow missing %q\n%s", want, content)
		}
	}

	for _, forbidden := range []string{
		"contents: write",
		"pull-requests: write",
		"id-token: write",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("production readiness ops workflow must stay read-only, found %q\n%s", forbidden, content)
		}
	}
}
