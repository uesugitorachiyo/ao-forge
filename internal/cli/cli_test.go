package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func runCLI(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func assertJSONOutputEqual(t *testing.T, label string, want []byte, got string) {
	t.Helper()
	var compactWant, compactGot bytes.Buffer
	if err := json.Compact(&compactWant, want); err != nil {
		t.Fatalf("compact expected %s JSON: %v", label, err)
	}
	if err := json.Compact(&compactGot, []byte(got)); err != nil {
		t.Fatalf("compact actual %s JSON: %v\n%s", label, err, got)
	}
	if compactGot.String() != compactWant.String() {
		t.Fatalf("%s output drifted\nwant:\n%s\ngot:\n%s", label, string(want), got)
	}
}

func TestHelpExplainsFactoryTermsWithoutMarketingCopy(t *testing.T) {
	code, stdout, stderr := runCLI("--help")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	for _, want := range []string{
		"AO Forge",
		"factory brief",
		"workcell",
		"factory packet",
		"forge plan --brief",
		"forge inspect --packet",
		"dry-run execution",
		"forge artifact checksums",
		"forge release-preview inspect --audit <release-preview-audit.json> [--json]",
		"Slice 2.7 status:",
		"release mutation",
		"GitHub publishing",
		"release preview audits",
		"release preview enforcement",
		"release preview audit inspection",
		"artifact checksums",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help missing %q\n%s", want, stdout)
		}
	}
	for _, forbidden := range []string{
		"revolutionary",
		"magical",
		"world-class",
		"best-in-class",
	} {
		if strings.Contains(strings.ToLower(stdout), forbidden) {
			t.Fatalf("help contains marketing copy %q\n%s", forbidden, stdout)
		}
	}
}

func TestReleasePreviewAuditFlagRequiresValue(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "run", args: []string{"run", "--release-preview-audit"}, want: "forge run: --release-preview-audit requires a value"},
		{name: "resume", args: []string{"resume", "--release-preview-audit"}, want: "forge resume: --release-preview-audit requires a value"},
		{name: "once", args: []string{"once", "--release-preview-audit"}, want: "forge once: --release-preview-audit requires a value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runCLI(tc.args...)
			if code != 2 {
				t.Fatalf("expected usage error code 2, got %d", code)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Fatalf("stderr missing %q: %s", tc.want, stderr)
			}
		})
	}
}

func TestFactoryBriefAndPlanSchemasAreLinkedAndStrict(t *testing.T) {
	root := repoRoot(t)
	readText := func(path ...string) string {
		t.Helper()
		bytes, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(path...), err)
		}
		return string(bytes)
	}

	readme := readText("README.md")
	briefSchema := readText("docs", "contracts", "factory-brief-v0.1.schema.json")
	planSchema := readText("docs", "contracts", "factory-plan-v0.1.schema.json")
	releasePreviewSchema := readText("docs", "contracts", "release-preview-audit-v0.1.schema.json")
	releasePreviewInspectSchema := readText("docs", "contracts", "release-preview-inspect-v0.1.schema.json")

	for _, check := range []struct {
		name string
		doc  string
		want string
	}{
		{name: "README brief schema link", doc: readme, want: "[Factory Brief v0.1 Schema](docs/contracts/factory-brief-v0.1.schema.json)"},
		{name: "README plan schema link", doc: readme, want: "[Factory Plan v0.1 Schema](docs/contracts/factory-plan-v0.1.schema.json)"},
		{name: "README release preview schema link", doc: readme, want: "[Release Preview Audit v0.1 Schema](docs/contracts/release-preview-audit-v0.1.schema.json)"},
		{name: "README release preview inspect schema link", doc: readme, want: "[Release Preview Inspect v0.1 Schema](docs/contracts/release-preview-inspect-v0.1.schema.json)"},
		{name: "brief schema id", doc: briefSchema, want: `"ao.forge.factory-brief.v0.1"`},
		{name: "brief strict root", doc: briefSchema, want: `"additionalProperties": false`},
		{name: "brief objective", doc: briefSchema, want: `"objective"`},
		{name: "brief constraints", doc: briefSchema, want: `"constraints"`},
		{name: "brief expected workcells", doc: briefSchema, want: `"expected_workcells"`},
		{name: "plan schema id", doc: planSchema, want: `"ao.forge.factory-plan.v0.1"`},
		{name: "plan strict root", doc: planSchema, want: `"additionalProperties": false`},
		{name: "plan id pattern", doc: planSchema, want: `"^forge-plan-[a-f0-9]{12}$"`},
		{name: "plan execution disabled", doc: planSchema, want: `"execution_enabled"`},
		{name: "plan policy gate", doc: planSchema, want: `"policy_gate"`},
		{name: "plan workcells", doc: planSchema, want: `"workcells"`},
		{name: "plan next actions", doc: planSchema, want: `"next_actions"`},
		{name: "release preview schema id", doc: releasePreviewSchema, want: `"ao.forge.release-preview-audit.v0.1"`},
		{name: "release preview strict root", doc: releasePreviewSchema, want: `"additionalProperties": false`},
		{name: "release preview checks", doc: releasePreviewSchema, want: `"checks"`},
		{name: "release preview check min items", doc: releasePreviewSchema, want: `"minItems": 1`},
		{name: "release preview artifacts", doc: releasePreviewSchema, want: `"artifacts"`},
		{name: "release preview checksum pattern", doc: releasePreviewSchema, want: `"^[a-f0-9]{64}$"`},
		{name: "release preview inspect schema id", doc: releasePreviewInspectSchema, want: `"ao.forge.release-preview-inspect.v0.1"`},
		{name: "release preview inspect strict root", doc: releasePreviewInspectSchema, want: `"additionalProperties": false`},
		{name: "release preview inspect schema version field", doc: releasePreviewInspectSchema, want: `"inspect_schema_version"`},
		{name: "release preview inspect audit schema field", doc: releasePreviewInspectSchema, want: `"schema_version"`},
		{name: "release preview inspect failed checks", doc: releasePreviewInspectSchema, want: `"failed_checks"`},
		{name: "release preview inspect artifact details", doc: releasePreviewInspectSchema, want: `"artifact_details"`},
		{name: "release preview inspect next actions", doc: releasePreviewInspectSchema, want: `"next_actions"`},
	} {
		if !strings.Contains(check.doc, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}

	for _, doc := range []struct {
		name string
		text string
	}{
		{name: "brief schema", text: briefSchema},
		{name: "plan schema", text: planSchema},
		{name: "release preview schema", text: releasePreviewSchema},
		{name: "release preview inspect schema", text: releasePreviewInspectSchema},
	} {
		var decoded any
		if err := json.Unmarshal([]byte(doc.text), &decoded); err != nil {
			t.Fatalf("%s is not valid JSON: %v", doc.name, err)
		}
	}
}

func TestReleaseThreatModelIsPublicAndMapsAttacksToControls(t *testing.T) {
	root := repoRoot(t)
	readText := func(path ...string) string {
		t.Helper()
		bytes, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(path...), err)
		}
		return string(bytes)
	}

	readme := readText("README.md")
	previewRunbook := readText("docs", "release", "PREVIEW-RELEASE.md")
	threatModel := readText("docs", "security", "RELEASE-THREAT-MODEL.md")

	for _, check := range []struct {
		name string
		doc  string
		want string
	}{
		{name: "README threat model link", doc: readme, want: "[Release Threat Model](docs/security/RELEASE-THREAT-MODEL.md)"},
		{name: "preview runbook threat model link", doc: previewRunbook, want: "../security/RELEASE-THREAT-MODEL.md"},
		{name: "threat model title", doc: threatModel, want: "# AO Forge Release Threat Model"},
		{name: "threat model scope", doc: threatModel, want: "## Scope"},
		{name: "threat model attack table", doc: threatModel, want: "## Release Attack Map"},
		{name: "tag spoofing attack", doc: threatModel, want: "Tag spoofing"},
		{name: "artifact tampering attack", doc: threatModel, want: "Artifact tampering"},
		{name: "credential exposure attack", doc: threatModel, want: "Credential exposure"},
		{name: "workflow permission escalation attack", doc: threatModel, want: "Workflow permission escalation"},
		{name: "stale evidence attack", doc: threatModel, want: "Stale release evidence"},
		{name: "dirty workspace attack", doc: threatModel, want: "Dirty workspace release"},
		{name: "control release preview", doc: threatModel, want: "`forge release-preview`"},
		{name: "control checksum command", doc: threatModel, want: "`forge artifact checksums`"},
		{name: "control inspect json", doc: threatModel, want: "`forge release-preview inspect --json`"},
		{name: "control read permissions", doc: threatModel, want: "`contents: read`"},
		{name: "public privacy guidance", doc: threatModel, want: "Do not commit ad hoc local release audits"},
	} {
		if !strings.Contains(check.doc, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}
}

func TestBranchProtectionRunbookDocumentsRequiredChecks(t *testing.T) {
	root := repoRoot(t)
	readText := func(path ...string) string {
		t.Helper()
		bytes, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(path...), err)
		}
		return string(bytes)
	}

	readme := readText("README.md")
	threatModel := readText("docs", "security", "RELEASE-THREAT-MODEL.md")
	runbook := readText("docs", "release", "BRANCH-PROTECTION.md")

	for _, check := range []struct {
		name string
		doc  string
		want string
	}{
		{name: "README branch protection link", doc: readme, want: "[Branch Protection Runbook](docs/release/BRANCH-PROTECTION.md)"},
		{name: "threat model branch protection link", doc: threatModel, want: "../release/BRANCH-PROTECTION.md"},
		{name: "runbook title", doc: runbook, want: "# AO Forge Branch Protection Runbook"},
		{name: "main branch", doc: runbook, want: "`main`"},
		{name: "require PR", doc: runbook, want: "Require a pull request before merging"},
		{name: "stale reviews", doc: runbook, want: "Dismiss stale pull request approvals"},
		{name: "required status checks", doc: runbook, want: "Require status checks to pass before merging"},
		{name: "ubuntu check", doc: runbook, want: "`Go ubuntu-latest`"},
		{name: "macos check", doc: runbook, want: "`Go macos-latest`"},
		{name: "windows check", doc: runbook, want: "`Go windows-latest`"},
		{name: "release preview check", doc: runbook, want: "`Release preview dry-run audit`"},
		{name: "linear history", doc: runbook, want: "Require linear history"},
		{name: "admin guidance", doc: runbook, want: "Do not bypass the required checks for public releases"},
		{name: "local fallback", doc: runbook, want: "go test ./... -count=1"},
		{name: "secret scan", doc: runbook, want: "gitleaks detect --source . --redact --verbose"},
	} {
		if !strings.Contains(check.doc, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}
}

func TestReleasePreviewWorkflowPublishesDryRunAuditArtifacts(t *testing.T) {
	root := repoRoot(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "release-preview.yml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read release preview workflow: %v", err)
	}
	workflow := string(data)
	for _, want := range []string{
		"name: Release Preview",
		"workflow_dispatch:",
		"pull_request:",
		"push:",
		"scripts/release-preview-dry-run.sh",
		"release-preview inspect --audit",
		"actions/upload-artifact@v7",
		"release-preview-audit",
		"release-preview-inspect",
		"release-preview-inspect-json",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("release preview workflow missing %q\n%s", want, workflow)
		}
	}
	for _, forbidden := range []string{
		"--confirm-release",
		"--live",
		"GITHUB_TOKEN:",
		"contents: write",
		"actions/upload-artifact@v4",
		"actions/upload-artifact@v5",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release preview workflow must not contain %q\n%s", forbidden, workflow)
		}
	}

	scriptPath := filepath.Join(root, "scripts", "release-preview-dry-run.sh")
	scriptData, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read release preview script: %v", err)
	}
	script := string(scriptData)
	for _, want := range []string{
		"set -euo pipefail",
		"git status --porcelain",
		"go build",
		"release-preview",
		"--tag",
		"--artifact",
		"artifact checksums",
		"release-preview-audit.json",
		"release-preview-inspect.txt",
		"release-preview-inspect.json",
		"--json",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("release preview script missing %q\n%s", want, script)
		}
	}
	for _, forbidden := range []string{
		"shasum -a 256",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("release preview script must use forge artifact checksums, found %q\n%s", forbidden, script)
		}
	}
}

func TestReleaseRehearsalWorkflowValidatesTaggedEvidenceWithoutPublishing(t *testing.T) {
	root := repoRoot(t)
	readText := func(path ...string) string {
		t.Helper()
		bytes, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(path...), err)
		}
		return string(bytes)
	}

	workflow := readText(".github", "workflows", "release-rehearsal.yml")
	previewRunbook := readText("docs", "release", "PREVIEW-RELEASE.md")
	readme := readText("README.md")

	for _, check := range []struct {
		name string
		doc  string
		want string
	}{
		{name: "workflow name", doc: workflow, want: "name: Release Rehearsal"},
		{name: "manual trigger", doc: workflow, want: "workflow_dispatch:"},
		{name: "tag push trigger", doc: workflow, want: "tags:"},
		{name: "version tag glob", doc: workflow, want: "'v*'"},
		{name: "read only permissions", doc: workflow, want: "contents: read"},
		{name: "tag env", doc: workflow, want: "AO_FORGE_RELEASE_PREVIEW_TAG"},
		{name: "dry run script", doc: workflow, want: "scripts/release-preview-dry-run.sh"},
		{name: "validate evidence step", doc: workflow, want: "Validate rehearsal evidence"},
		{name: "audit schema validation", doc: workflow, want: "ao.forge.release-preview-audit.v0.1"},
		{name: "inspect schema validation", doc: workflow, want: "ao.forge.release-preview-inspect.v0.1"},
		{name: "upload evidence artifact", doc: workflow, want: "release-rehearsal-evidence"},
		{name: "runbook section", doc: previewRunbook, want: "## Tagged Release Rehearsal"},
		{name: "runbook workflow name", doc: previewRunbook, want: "`Release Rehearsal`"},
		{name: "runbook artifact name", doc: previewRunbook, want: "`release-rehearsal-evidence`"},
		{name: "readme mentions rehearsal", doc: readme, want: "tagged release rehearsal"},
	} {
		if !strings.Contains(check.doc, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}

	for _, forbidden := range []string{
		"contents: write",
		"GITHUB_TOKEN:",
		"--confirm-release",
		"--live",
		"gh release create",
		"softprops/action-gh-release",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("release rehearsal workflow must not contain %q\n%s", forbidden, workflow)
		}
	}
}

func TestArtifactChecksumsWritesStableSHA256Manifest(t *testing.T) {
	tmpDir := t.TempDir()
	firstPath := filepath.Join(tmpDir, "ao-forge_Darwin_arm64.tar.gz")
	secondPath := filepath.Join(tmpDir, "ao-forge_Windows_x86_64.zip")
	outPath := filepath.Join(tmpDir, "checksums.txt")
	firstData := []byte("darwin artifact\n")
	secondData := []byte("windows artifact\n")
	if err := os.WriteFile(firstPath, firstData, 0644); err != nil {
		t.Fatalf("write first artifact: %v", err)
	}
	if err := os.WriteFile(secondPath, secondData, 0644); err != nil {
		t.Fatalf("write second artifact: %v", err)
	}

	code, stdout, stderr := runCLI("artifact", "checksums", "--artifact", firstPath, "--artifact", secondPath, "--out", outPath)
	if code != 0 {
		t.Fatalf("artifact checksums failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "artifact_checksums="+displayPath(outPath)) {
		t.Fatalf("stdout missing checksums output path: %s", stdout)
	}

	firstSum := sha256.Sum256(firstData)
	secondSum := sha256.Sum256(secondData)
	wantManifest := fmt.Sprintf("%s  %s\n%s  %s\n",
		hex.EncodeToString(firstSum[:]),
		displayPath(firstPath),
		hex.EncodeToString(secondSum[:]),
		displayPath(secondPath),
	)
	gotManifest, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read checksum manifest: %v", err)
	}
	if string(gotManifest) != wantManifest {
		t.Fatalf("checksum manifest drifted\nwant:\n%sgot:\n%s", wantManifest, string(gotManifest))
	}

	code, stdout, stderr = runCLI("artifact", "checksums", "--artifact", firstPath)
	if code != 0 {
		t.Fatalf("artifact checksums stdout mode failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if stdout != fmt.Sprintf("%s  %s\n", hex.EncodeToString(firstSum[:]), displayPath(firstPath)) {
		t.Fatalf("stdout checksum manifest drifted: %s", stdout)
	}
}

func TestArtifactChecksumsFailsClosedOnMissingArtifact(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing-artifact.tar.gz")

	code, stdout, stderr := runCLI("artifact", "checksums", "--artifact", missingPath)
	if code != 1 {
		t.Fatalf("expected missing artifact to fail with code 1, got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("missing artifact should not write stdout: %s", stdout)
	}
	for _, want := range []string{
		"forge artifact checksums:",
		"missing",
		displayPath(missingPath),
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q: %s", want, stderr)
		}
	}
}

func TestVerifiedFoundationBaselineIsLinkedAndMachineReadable(t *testing.T) {
	root := repoRoot(t)
	readFile := func(path ...string) []byte {
		t.Helper()
		bytes, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(path...), err)
		}
		return bytes
	}

	readme := string(readFile("README.md"))
	if !strings.Contains(readme, "[Verified Foundation Baseline](docs/foundation/VERIFIED-BASELINE.md)") {
		t.Fatalf("README missing verified foundation baseline link")
	}
	if !strings.Contains(readme, "[Foundation Baseline JSON](docs/foundation/foundation-baseline.v0.1.json)") {
		t.Fatalf("README missing foundation baseline JSON link")
	}

	var baseline struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Components    []struct {
			Name      string `json:"name"`
			LocalPath string `json:"local_path"`
			Commit    string `json:"commit"`
			Release   string `json:"release"`
			LatestCI  struct {
				Status        string `json:"status"`
				CompletedJobs int    `json:"completed_jobs"`
				TotalJobs     int    `json:"total_jobs"`
			} `json:"latest_ci"`
		} `json:"components"`
		ForgeStartPolicy struct {
			MayStartPhase0                        bool `json:"may_start_phase_0"`
			MayStartExecutionAdapter              bool `json:"may_start_execution_adapter"`
			MustKeepControlPlaneObserverOnly      bool `json:"must_keep_control_plane_observer_only"`
			MustFailClosedWithoutCovenantDecision bool `json:"must_fail_closed_without_covenant_decision"`
		} `json:"forge_start_policy"`
	}
	if err := json.Unmarshal(readFile("docs", "foundation", "foundation-baseline.v0.1.json"), &baseline); err != nil {
		t.Fatalf("foundation baseline is not valid JSON: %v", err)
	}
	if baseline.SchemaVersion != "ao.forge.foundation-baseline.v0.1" {
		t.Fatalf("schema_version = %q", baseline.SchemaVersion)
	}
	if baseline.Status != "ready_for_ao_forge_phase_0" {
		t.Fatalf("status = %q", baseline.Status)
	}
	if len(baseline.Components) != 3 {
		t.Fatalf("component count = %d", len(baseline.Components))
	}

	want := map[string]struct {
		localPath string
		commit    string
		release   string
	}{
		"ao2": {
			localPath: "../ao2",
			commit:    "fbdea7d4c8d0546e52103b7d3c0cdf01d2013670",
			release:   "v0.4.80",
		},
		"ao2-control-plane": {
			localPath: "../ao2-control-plane",
			commit:    "de4e865ef8a3fe00005d27b165aab319e99c6ba1",
			release:   "v0.1.13",
		},
		"ao-covenant": {
			localPath: "../ao-covenant",
			commit:    "ef815b35d1166b1f26ded2b482f15d088281c568",
			release:   "v0.1.0",
		},
	}
	for _, component := range baseline.Components {
		expected, ok := want[component.Name]
		if !ok {
			t.Fatalf("unexpected component %q", component.Name)
		}
		if component.LocalPath != expected.localPath {
			t.Fatalf("%s local_path = %q", component.Name, component.LocalPath)
		}
		if component.Commit != expected.commit {
			t.Fatalf("%s commit = %q", component.Name, component.Commit)
		}
		if component.Release != expected.release {
			t.Fatalf("%s release = %q", component.Name, component.Release)
		}
		if component.LatestCI.Status != "success" {
			t.Fatalf("%s latest CI status = %q", component.Name, component.LatestCI.Status)
		}
		if component.LatestCI.CompletedJobs != component.LatestCI.TotalJobs {
			t.Fatalf("%s CI jobs = %d/%d", component.Name, component.LatestCI.CompletedJobs, component.LatestCI.TotalJobs)
		}
		delete(want, component.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing components: %+v", want)
	}
	if !baseline.ForgeStartPolicy.MayStartPhase0 || !baseline.ForgeStartPolicy.MayStartExecutionAdapter {
		t.Fatalf("forge start policy does not allow phase 0 execution adapter work")
	}
	if !baseline.ForgeStartPolicy.MustKeepControlPlaneObserverOnly {
		t.Fatalf("forge start policy must keep control plane observer-only")
	}
	if !baseline.ForgeStartPolicy.MustFailClosedWithoutCovenantDecision {
		t.Fatalf("forge start policy must fail closed without Covenant decision")
	}
}

func TestPlanBriefProducesDeterministicFactoryPlan(t *testing.T) {
	brief := filepath.Join(repoRoot(t), "examples", "vertical-slices", "risky-pr-factory.factory.json")
	expectedPath := filepath.Join(repoRoot(t), "examples", "plans", "risky-pr-factory-plan.json")

	code, stdout, stderr := runCLI("plan", "--brief", brief)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected plan fixture: %v", err)
	}
	assertJSONOutputEqual(t, "plan", expected, stdout)

	var plan struct {
		SchemaVersion string `json:"schema_version"`
		PlanID        string `json:"plan_id"`
		Objective     struct {
			Text        string `json:"text"`
			Workspace   string `json:"workspace"`
			ReleaseMode bool   `json:"release_mode"`
		} `json:"objective"`
		ExecutionEnabled bool `json:"execution_enabled"`
		PolicyGate       struct {
			Required bool   `json:"required"`
			Status   string `json:"status"`
		} `json:"policy_gate"`
		Workcells []struct {
			WorkcellID string   `json:"workcell_id"`
			Kind       string   `json:"kind"`
			Status     string   `json:"status"`
			DependsOn  []string `json:"depends_on"`
		} `json:"workcells"`
		NextActions []struct {
			ActionID    string `json:"action_id"`
			Description string `json:"description"`
			Required    bool   `json:"required"`
		} `json:"next_actions"`
	}
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("plan output is not JSON: %v\n%s", err, stdout)
	}
	if plan.SchemaVersion != "ao.forge.factory-plan.v0.1" {
		t.Fatalf("schema_version = %q", plan.SchemaVersion)
	}
	if plan.PlanID != "forge-plan-efedbfb309b1" {
		t.Fatalf("plan_id = %q", plan.PlanID)
	}
	if plan.Objective.Text != "Run a governed risky PR improvement against the fixture repository." {
		t.Fatalf("objective text = %q", plan.Objective.Text)
	}
	if plan.Objective.Workspace != "fixtures/discount-service" {
		t.Fatalf("workspace = %q", plan.Objective.Workspace)
	}
	if plan.ExecutionEnabled {
		t.Fatalf("execution_enabled = true, want false for slice 0.2")
	}
	if !plan.PolicyGate.Required || plan.PolicyGate.Status != "not_requested" {
		t.Fatalf("policy gate = %+v", plan.PolicyGate)
	}
	if len(plan.Workcells) != 3 {
		t.Fatalf("workcell count = %d", len(plan.Workcells))
	}
	expectedIDs := []string{"prepare-fixture", "run-ao2-risky-pr", "close-factory-packet"}
	for i, want := range expectedIDs {
		if plan.Workcells[i].WorkcellID != want {
			t.Fatalf("workcell[%d] id = %q, want %q", i, plan.Workcells[i].WorkcellID, want)
		}
		if plan.Workcells[i].Status != "planned" {
			t.Fatalf("workcell[%d] status = %q", i, plan.Workcells[i].Status)
		}
	}
	if len(plan.NextActions) == 0 || plan.NextActions[0].ActionID != "run-covenant-gate" {
		t.Fatalf("next actions = %+v", plan.NextActions)
	}
}

func TestPlanRejectsBriefsOutsideFactoryBriefSchema(t *testing.T) {
	briefPath := filepath.Join(t.TempDir(), "invalid-brief.json")
	brief := `{
  "schema_version": "ao.forge.factory-brief.v0.1",
  "objective": {
    "text": "Run a governed risky PR improvement against the fixture repository.",
    "workspace": "fixtures/discount-service",
    "release_mode": false
  },
  "constraints": {
    "local_first": true,
    "allow_network": false,
    "allow_release_mutation": false,
    "require_control_plane_readback": false
  },
  "expected_workcells": [
    {
      "workcell_id": "prepare-fixture",
      "kind": "prepare",
      "depends_on": []
    }
  ],
  "expected_evidence": ["factory packet"],
  "unexpected_field": true
}`
	if err := os.WriteFile(briefPath, []byte(brief), 0o644); err != nil {
		t.Fatalf("write invalid brief: %v", err)
	}

	code, stdout, stderr := runCLI("plan", "--brief", briefPath)
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	if !strings.Contains(stderr, `unknown field "unexpected_field"`) {
		t.Fatalf("stderr missing unknown field error: %s", stderr)
	}
}

func TestPlanRejectsBriefsMissingRequiredFactoryBriefFields(t *testing.T) {
	briefPath := filepath.Join(t.TempDir(), "missing-required-brief.json")
	brief := `{
  "schema_version": "ao.forge.factory-brief.v0.1",
  "objective": {
    "text": "Run a governed risky PR improvement against the fixture repository.",
    "workspace": "fixtures/discount-service",
    "release_mode": false
  },
  "constraints": {
    "local_first": true,
    "allow_release_mutation": false,
    "require_control_plane_readback": false
  },
  "expected_workcells": [
    {
      "workcell_id": "prepare-fixture",
      "kind": "prepare"
    }
  ],
  "expected_evidence": ["factory packet"]
}`
	if err := os.WriteFile(briefPath, []byte(brief), 0o644); err != nil {
		t.Fatalf("write invalid brief: %v", err)
	}

	code, stdout, stderr := runCLI("plan", "--brief", briefPath)
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	for _, want := range []string{
		"brief constraints.allow_network is required",
		"brief expected_workcells[0].depends_on is required",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q: %s", want, stderr)
		}
	}
}

func TestFactoryPlanContractRejectsMalformedPlans(t *testing.T) {
	plan := factoryPlan{
		SchemaVersion: planSchemaVersion,
		PlanID:        "forge-plan-efedbfb309b1",
		Objective: factoryObjective{
			Text:        "Run a governed risky PR improvement against the fixture repository.",
			Workspace:   "fixtures/discount-service",
			ReleaseMode: false,
		},
		Constraints: factoryConstraints{
			LocalFirst:                  true,
			AllowNetwork:                false,
			AllowReleaseMutation:        false,
			RequireControlPlaneReadback: false,
		},
		ExecutionEnabled: false,
		PolicyGate: policyGate{
			Required:    true,
			Status:      "not_requested",
			Explanation: "gate not requested",
		},
		Workcells: []planWorkcell{
			{
				WorkcellID: "prepare-fixture",
				Kind:       "prepare",
				Status:     "planned",
				DependsOn:  nil,
			},
		},
		ExpectedEvidence: []string{"factory packet"},
		NextActions: []nextAction{
			{
				ActionID:    "run-covenant-gate",
				Description: "Run the Covenant gate before AO2 execution.",
				Required:    true,
			},
		},
	}

	err := validatePlan(plan)
	if err == nil {
		t.Fatalf("validatePlan accepted malformed plan")
	}
	if !strings.Contains(err.Error(), "workcell prepare-fixture depends_on must be an array") {
		t.Fatalf("error = %v", err)
	}
}

func TestPlanWritesOutFileWhenRequested(t *testing.T) {
	brief := filepath.Join(repoRoot(t), "examples", "vertical-slices", "risky-pr-factory.factory.json")
	out := filepath.Join(t.TempDir(), "factory-plan.json")

	code, stdout, stderr := runCLI("plan", "--brief", brief, "--out", out)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "factory_plan="+out {
		t.Fatalf("stdout = %q", stdout)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read plan output: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("written plan is not JSON:\n%s", string(data))
	}
}

func TestPlanRequiresBriefPath(t *testing.T) {
	code, stdout, stderr := runCLI("plan")
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	if !strings.Contains(stderr, "missing required --brief") {
		t.Fatalf("stderr missing brief error: %s", stderr)
	}
}

func TestInspectPacketPrintsOperatorSummary(t *testing.T) {
	packet := filepath.Join(repoRoot(t), "examples", "packets", "risky-pr-factory-packet.json")

	code, stdout, stderr := runCLI("inspect", "--packet", packet)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	for _, want := range []string{
		"factory_packet=examples/packets/risky-pr-factory-packet.json",
		"status=blocked",
		"objective=Run a governed risky PR improvement against the fixture repository.",
		"workcells=3",
		"policy_decisions=1",
		"evidence=1",
		"next_action=run-covenant-gate required=true",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("inspect output missing %q\n%s", want, stdout)
		}
	}
}

func TestPacketFixtureEvidenceDigestsMatchFiles(t *testing.T) {
	root := repoRoot(t)
	packetPath := filepath.Join(root, "examples", "packets", "risky-pr-factory-packet.json")
	data, err := os.ReadFile(packetPath)
	if err != nil {
		t.Fatalf("read packet fixture: %v", err)
	}
	var packet struct {
		Evidence []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal(data, &packet); err != nil {
		t.Fatalf("parse packet fixture: %v", err)
	}
	if len(packet.Evidence) == 0 {
		t.Fatalf("packet fixture has no evidence")
	}
	for _, evidence := range packet.Evidence {
		if evidence.SHA256 == "" {
			t.Fatalf("evidence %q missing sha256", evidence.Path)
		}
		evidenceBytes, err := os.ReadFile(filepath.Join(root, evidence.Path))
		if err != nil {
			t.Fatalf("read evidence %q: %v", evidence.Path, err)
		}
		sum := sha256.Sum256(evidenceBytes)
		if actual := hex.EncodeToString(sum[:]); actual != evidence.SHA256 {
			t.Fatalf("evidence %q sha256 = %s, want %s", evidence.Path, evidence.SHA256, actual)
		}
	}
}

func TestGateAllowFixtureProducesAllowedGateResult(t *testing.T) {
	root := repoRoot(t)
	plan := filepath.Join(root, "examples", "plans", "risky-pr-factory-plan.json")
	decision := filepath.Join(root, "examples", "decisions", "allow-local-plan.decision.json")
	expectedPath := filepath.Join(root, "examples", "gates", "allow-local-plan.gate.json")

	code, stdout, stderr := runCLI("gate", "--plan", plan, "--covenant", decision)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected gate fixture: %v", err)
	}
	assertJSONOutputEqual(t, "gate", expected, stdout)
}

func TestGateDenyFixtureFailsClosed(t *testing.T) {
	root := repoRoot(t)
	plan := filepath.Join(root, "examples", "plans", "risky-pr-factory-plan.json")
	decision := filepath.Join(root, "examples", "decisions", "deny-release-mutation.decision.json")

	code, stdout, stderr := runCLI("gate", "--plan", plan, "--covenant", decision)
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	if !strings.Contains(stderr, "decision denied") {
		t.Fatalf("stderr missing deny message: %s", stderr)
	}
	var result struct {
		Status           string `json:"status"`
		ExecutionEnabled bool   `json:"execution_enabled"`
		Decision         struct {
			Decision    string `json:"decision"`
			Explanation string `json:"explanation"`
		} `json:"decision"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("deny output is not JSON: %v\n%s", err, stdout)
	}
	if result.Status != "denied" || result.ExecutionEnabled {
		t.Fatalf("result = %+v", result)
	}
	if result.Decision.Decision != "deny" || strings.TrimSpace(result.Decision.Explanation) == "" {
		t.Fatalf("decision = %+v", result.Decision)
	}
}

func TestGateIndeterminateFixtureFailsClosed(t *testing.T) {
	root := repoRoot(t)
	plan := filepath.Join(root, "examples", "plans", "risky-pr-factory-plan.json")
	decision := filepath.Join(root, "examples", "decisions", "indeterminate.decision.json")

	code, stdout, _ := runCLI("gate", "--plan", plan, "--covenant", decision)
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	var result struct {
		Status           string `json:"status"`
		ExecutionEnabled bool   `json:"execution_enabled"`
		Decision         struct {
			Decision    string `json:"decision"`
			Explanation string `json:"explanation"`
		} `json:"decision"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("indeterminate output is not JSON: %v\n%s", err, stdout)
	}
	if result.Status != "blocked" || result.ExecutionEnabled {
		t.Fatalf("result = %+v", result)
	}
	if result.Decision.Decision != "indeterminate" || strings.TrimSpace(result.Decision.Explanation) == "" {
		t.Fatalf("decision = %+v", result.Decision)
	}
}

func TestGateRequiresDecisionExplanation(t *testing.T) {
	root := repoRoot(t)
	plan := filepath.Join(root, "examples", "plans", "risky-pr-factory-plan.json")
	decision := filepath.Join(root, "examples", "decisions", "missing-explanation.decision.json")
	out := filepath.Join(t.TempDir(), "gate-result.json")

	code, stdout, stderr := runCLI("gate", "--plan", plan, "--covenant", decision, "--out", out)
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	if strings.TrimSpace(stdout) != "covenant_gate="+out {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "explanation is required") {
		t.Fatalf("stderr missing explanation error: %s", stderr)
	}
	resultBytes, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read gate result: %v", err)
	}
	var result struct {
		Status   string `json:"status"`
		Problem  string `json:"problem"`
		Decision struct {
			Decision string `json:"decision"`
		} `json:"decision"`
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("gate result is not JSON: %v\n%s", err, string(resultBytes))
	}
	if result.Status != "blocked" || result.Decision.Decision != "invalid" {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.Problem, "explanation is required") {
		t.Fatalf("problem = %q", result.Problem)
	}
}

func TestGateMalformedFixtureFailsClosedAndWritesResult(t *testing.T) {
	root := repoRoot(t)
	plan := filepath.Join(root, "examples", "plans", "risky-pr-factory-plan.json")
	decision := filepath.Join(root, "examples", "decisions", "malformed.decision.invalid")
	out := filepath.Join(t.TempDir(), "gate-result.json")

	code, stdout, stderr := runCLI("gate", "--plan", plan, "--covenant", decision, "--out", out)
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	if strings.TrimSpace(stdout) != "covenant_gate="+out {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "parse decision fixture JSON") {
		t.Fatalf("stderr missing malformed error: %s", stderr)
	}
	resultBytes, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read gate result: %v", err)
	}
	var result struct {
		SchemaVersion    string `json:"schema_version"`
		Status           string `json:"status"`
		ExecutionEnabled bool   `json:"execution_enabled"`
		Problem          string `json:"problem"`
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("gate result is not JSON: %v\n%s", err, string(resultBytes))
	}
	if result.SchemaVersion != "ao.forge.covenant-gate-result.v0.1" || result.Status != "blocked" || result.ExecutionEnabled {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.Problem, "parse decision fixture JSON") {
		t.Fatalf("problem = %q", result.Problem)
	}
}

func TestGateLiveCovenantBinary(t *testing.T) {
	root := repoRoot(t)

	// Compile a dummy binary that acts as covenant
	tmpDir := t.TempDir()
	dummySrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}

	srcContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" && len(os.Args) > 2 && os.Args[2] == "--json" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	// 1. Allow case: plan with AllowReleaseMutation = false and AllowNetwork = false
	planPath := filepath.Join(root, "examples", "plans", "risky-pr-factory-plan.json")
	code, stdout, stderr := runCLI("gate", "--plan", planPath, "--covenant", dummyBin)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}

	var result struct {
		Status   string `json:"status"`
		Decision struct {
			Decision    string `json:"decision"`
			Explanation string `json:"explanation"`
		} `json:"decision"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("unmarshal allow result: %v", err)
	}
	if result.Status != "allowed" || result.Decision.Decision != "allow" {
		t.Fatalf("expected allowed/allow, got: %+v", result)
	}

	// 2. Deny case: plan with AllowReleaseMutation = true
	denyPlanPath := filepath.Join(tmpDir, "deny-plan.json")
	denyPlan := `{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {
			"text": "test",
			"workspace": "test",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": true,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "not_requested",
			"explanation": "test"
		},
		"workcells": [
			{"workcell_id": "cell1", "kind": "prepare", "status": "planned", "depends_on": []}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`
	if err := os.WriteFile(denyPlanPath, []byte(denyPlan), 0644); err != nil {
		t.Fatalf("write deny plan: %v", err)
	}

	code, stdout, stderr = runCLI("gate", "--plan", denyPlanPath, "--covenant", dummyBin)
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("unmarshal deny result: %v", err)
	}
	if result.Status != "denied" || result.Decision.Decision != "deny" {
		t.Fatalf("expected denied/deny, got: %+v", result)
	}

	// 3. Indeterminate case: plan with AllowNetwork = true
	indetPlanPath := filepath.Join(tmpDir, "indet-plan.json")
	indetPlan := `{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {
			"text": "test",
			"workspace": "test",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": true,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "not_requested",
			"explanation": "test"
		},
		"workcells": [
			{"workcell_id": "cell1", "kind": "prepare", "status": "planned", "depends_on": []}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`
	if err := os.WriteFile(indetPlanPath, []byte(indetPlan), 0644); err != nil {
		t.Fatalf("write indet plan: %v", err)
	}

	code, stdout, stderr = runCLI("gate", "--plan", indetPlanPath, "--covenant", dummyBin)
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("unmarshal indeterminate result: %v", err)
	}
	if result.Status != "blocked" || result.Decision.Decision != "indeterminate" {
		t.Fatalf("expected blocked/indeterminate, got: %+v", result)
	}

	// 4. Unavailable case: binary not found
	code, stdout, stderr = runCLI("gate", "--plan", planPath, "--covenant", "nonexistent-covenant-bin")
	if code == 0 {
		t.Fatalf("exit code = 0, stdout = %s", stdout)
	}
	var blockedResult struct {
		Status  string `json:"status"`
		Problem string `json:"problem"`
	}
	if err := json.Unmarshal([]byte(stdout), &blockedResult); err != nil {
		t.Fatalf("unmarshal unavailable result: %v", err)
	}
	if blockedResult.Status != "blocked" || !strings.Contains(blockedResult.Problem, "unavailable") {
		t.Fatalf("expected blocked/unavailable, got: %+v", blockedResult)
	}
}

func TestRunLiveAo2Binary(t *testing.T) {
	root := repoRoot(t)

	// Compile a dummy binary that acts as ao2
	tmpDir := t.TempDir()
	dummySrc := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyBin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}

	srcContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		// check for --dry-run and --spec
		hasDryRun := false
		specPath := ""
		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "--dry-run" {
				hasDryRun = true
			} else if os.Args[i] == "--spec" && i+1 < len(os.Args) {
				specPath = os.Args[i+1]
			}
		}
		if hasDryRun && specPath != "" {
			fmt.Println("status=dry_run_accepted")
			fmt.Println("schema_version=ao2.run/v1")
			fmt.Println("plan_id=forge-plan-efedbfb309b1")
			fmt.Println("task_count=3")
			fmt.Println("target_repo=fixtures/discount-service")
			fmt.Println("control_plane_role=read_only_observer")
			fmt.Println("mutates_ao_artifacts=false")
			fmt.Println("factory_v3_drives_workflow=false")
			fmt.Println("spec_sha256=abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
			os.Exit(0)
		}
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}

	// Set AO2_PATH env var so resolver finds it
	t.Setenv("AO2_PATH", dummyBin)

	// Setup mock plans and gate results
	planPath := filepath.Join(root, "examples", "plans", "risky-pr-factory-plan.json")

	// 1. Fails closed with missing gate result
	code, stdout, stderr := runCLI("run", "--plan", planPath, "--gate-result", "nonexistent-gate-result.json")
	if code == 0 {
		t.Fatalf("expected failure exit code for missing gate result")
	}
	var packet struct {
		Status    string `json:"status"`
		Workcells []struct {
			Status string `json:"status"`
		} `json:"workcells"`
	}
	if err := json.Unmarshal([]byte(stdout), &packet); err != nil {
		t.Fatalf("failed to unmarshal output: %v (stdout: %s)", err, stdout)
	}
	if packet.Status != "blocked" {
		t.Fatalf("expected status blocked, got %s", packet.Status)
	}
	for _, wc := range packet.Workcells {
		if wc.Status != "blocked" {
			t.Fatalf("expected workcell status blocked, got %s", wc.Status)
		}
	}

	// 2. Fails closed with denied gate result
	denyGatePath := filepath.Join(tmpDir, "deny-gate.json")
	denyGateContent := `{
		"schema_version": "ao.forge.covenant-gate-result.v0.1",
		"status": "denied",
		"plan_id": "forge-plan-efedbfb309b1",
		"execution_enabled": false,
		"decision": {
			"schema_version": "ao.forge.covenant-decision-fixture.v0.1",
			"decision_id": "deny-release-mutation",
			"target_plan_id": "forge-plan-efedbfb309b1",
			"decision": "deny",
			"explanation": "denied by policy",
			"source": "test"
		}
	}`
	if err := os.WriteFile(denyGatePath, []byte(denyGateContent), 0644); err != nil {
		t.Fatalf("write deny gate: %v", err)
	}
	code, stdout, stderr = runCLI("run", "--plan", planPath, "--gate-result", denyGatePath)
	if code == 0 {
		t.Fatalf("expected failure exit code for denied gate result")
	}
	if err := json.Unmarshal([]byte(stdout), &packet); err != nil {
		t.Fatalf("failed to unmarshal output: %v (stdout: %s)", err, stdout)
	}
	if packet.Status != "denied" {
		t.Fatalf("expected status denied, got %s", packet.Status)
	}
	for _, wc := range packet.Workcells {
		if wc.Status != "denied" {
			t.Fatalf("expected workcell status denied, got %s", wc.Status)
		}
	}

	// 3. Succeeds with allowed gate result
	allowGatePath := filepath.Join(root, "examples", "gates", "allow-local-plan.gate.json")
	outPacketPath := filepath.Join(tmpDir, "packet-out.json")
	code, stdout, stderr = runCLI("run", "--plan", planPath, "--gate-result", allowGatePath, "--out", outPacketPath)
	if code != 0 {
		t.Fatalf("expected success exit code for run, got %d (stderr: %s)", code, stderr)
	}

	packetBytes, err := os.ReadFile(outPacketPath)
	if err != nil {
		t.Fatalf("failed to read out packet: %v", err)
	}
	var passedPacket struct {
		Status    string `json:"status"`
		Workcells []struct {
			Status string `json:"status"`
			AO2Run string `json:"ao2_run"`
		} `json:"workcells"`
		Evidence []struct {
			Label string `json:"label"`
			Path  string `json:"path"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal(packetBytes, &passedPacket); err != nil {
		t.Fatalf("failed to unmarshal passed packet: %v", err)
	}
	if passedPacket.Status != "passed" {
		t.Fatalf("expected packet status passed, got %s", passedPacket.Status)
	}
	if len(passedPacket.Workcells) != 3 {
		t.Fatalf("expected 3 workcells, got %d", len(passedPacket.Workcells))
	}
	for _, wc := range passedPacket.Workcells {
		if wc.Status != "passed" || wc.AO2Run != "dry-run" {
			t.Fatalf("unexpected workcell: %+v", wc)
		}
	}
	if len(passedPacket.Evidence) != 6 {
		t.Fatalf("expected 6 evidence items, got %d", len(passedPacket.Evidence))
	}
}

func TestOnceSucceedsFromBrief(t *testing.T) {
	root := repoRoot(t)
	tmpDir := t.TempDir()

	// Compile a dummy binary that acts as covenant
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" && len(os.Args) > 2 && os.Args[2] == "--json" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy cov src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	// Compile dummy ao2
	dummyAo2Src := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyAo2Bin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyAo2Bin += ".exe"
	}
	ao2Content := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		fmt.Println("status=dry_run_accepted")
		fmt.Println("schema_version=ao2.run/v1")
		fmt.Println("plan_id=forge-plan-efedbfb309b1")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyAo2Src, []byte(ao2Content), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}
	cmdAo2 := exec.Command("go", "build", "-o", dummyAo2Bin, dummyAo2Src)
	if out, err := cmdAo2.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}

	t.Setenv("AO2_PATH", dummyAo2Bin)

	briefPath := filepath.Join(root, "examples", "vertical-slices", "risky-pr-factory.factory.json")
	outPacket := filepath.Join(tmpDir, "once-packet.json")

	code, _, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
	if code != 0 {
		t.Fatalf("once command failed: code=%d, stderr=%q", code, stderr)
	}

	packetBytes, err := os.ReadFile(outPacket)
	if err != nil {
		t.Fatalf("failed to read once packet: %v", err)
	}
	var packet struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(packetBytes, &packet); err != nil {
		t.Fatalf("unmarshal once packet failed: %v", err)
	}
	if packet.Status != "passed" {
		t.Fatalf("expected status passed, got %q", packet.Status)
	}
}

func TestDoctorCLI(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping integration tests")
	}

	// 1. Missing --foundation
	code, _, stderr := runCLI("doctor")
	if code != 2 {
		t.Fatalf("expected code 2 for missing --foundation, got %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "missing required --foundation") {
		t.Fatalf("expected missing required --foundation in stderr, got: %s", stderr)
	}

	// 2. Nonexistent baseline file
	code, _, stderr = runCLI("doctor", "--foundation", "nonexistent.json")
	if code != 2 {
		t.Fatalf("expected code 2 for nonexistent file, got %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "read baseline file") || !strings.Contains(stderr, "nonexistent.json") {
		t.Fatalf("expected file error, got: %s", stderr)
	}

	// 3. Valid mock baseline integration
	tmp, err := os.MkdirTemp("", "cli-doctor-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	repoPath := filepath.Join(tmp, "dummy-repo")
	os.MkdirAll(repoPath, 0755)

	runGitInTest := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("run git %v: %v (output: %q)", args, err, string(out))
		}
	}

	runGitInTest("init")
	runGitInTest("config", "user.name", "Test User")
	runGitInTest("config", "user.email", "test@example.com")
	runGitInTest("config", "commit.gpgsign", "false")
	os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("ok"), 0644)
	runGitInTest("add", "file.txt")
	runGitInTest("commit", "-m", "init")
	runGitInTest("tag", "-m", "rel", "v1.0.0")

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	commitBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	commit := strings.TrimSpace(string(commitBytes))

	cmdBranch := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmdBranch.Dir = repoPath
	branchBytes, err := cmdBranch.Output()
	if err != nil {
		t.Fatalf("rev-parse branch: %v", err)
	}
	branch := strings.TrimSpace(string(branchBytes))

	mockBaseline := map[string]any{
		"schema_version": "ao.forge.foundation-baseline.v0.1",
		"verified_at":    "2026-06-17",
		"status":         "ready_for_ao_forge_phase_0",
		"components": []map[string]any{
			{
				"name":       "dummy",
				"role":       "role",
				"repository": "dummy-repo",
				"local_path": "dummy-repo",
				"branch":     branch,
				"commit":     commit,
				"release":    "v1.0.0",
			},
		},
	}
	baselineBytes, err := json.Marshal(mockBaseline)
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}
	baselineFile := filepath.Join(tmp, "baseline.json")
	os.WriteFile(baselineFile, baselineBytes, 0644)

	code, stdout, stderr := runCLI("doctor", "--foundation", baselineFile, "--json")
	if code != 0 {
		t.Fatalf("expected code 0, got %d (stderr: %s)", code, stderr)
	}

	var result struct {
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Components    []struct {
			Name             string `json:"name"`
			Exists           bool   `json:"exists"`
			GitDir           bool   `json:"git_dir"`
			BranchOK         bool   `json:"branch_ok"`
			CommitOK         bool   `json:"commit_ok"`
			WorktreeClean    bool   `json:"worktree_clean"`
			ReleaseTagExists bool   `json:"release_tag_exists"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("unmarshal json: %v (stdout: %s)", err, stdout)
	}
	if result.SchemaVersion != "ao.forge.foundation-doctor-result.v0.1" {
		t.Fatalf("unexpected schema version: %s", result.SchemaVersion)
	}
	if result.Status != "passed" {
		t.Fatalf("expected status passed, got %s", result.Status)
	}
	if len(result.Components) != 1 || result.Components[0].Name != "dummy" || !result.Components[0].Exists {
		t.Fatalf("unexpected component results: %+v", result.Components)
	}
}

func compileDummyAo2(t *testing.T, tmpDir string) string {
	t.Helper()
	dummySrc := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyBin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}
	srcContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		hasDryRun := false
		for _, arg := range os.Args {
			if arg == "--dry-run" {
				hasDryRun = true
			}
		}
		if hasDryRun {
			fmt.Println("status=dry_run_accepted")
		} else {
			fmt.Println("status=governed_run_started")
		}
		fmt.Println("schema_version=ao2.run/v1")
		fmt.Println("plan_id=forge-plan-efedbfb309b1")
		fmt.Println("task_count=1")
		fmt.Println("target_repo=fixtures/discount-service")
		fmt.Println("control_plane_role=read_only_observer")
		fmt.Println("mutates_ao_artifacts=false")
		fmt.Println("factory_v3_drives_workflow=false")
		fmt.Println("spec_sha256=abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
		if out := os.Getenv("AO2_MOCK_STDOUT"); out != "" {
			fmt.Print(out)
		}
		if errOut := os.Getenv("AO2_MOCK_STDERR"); errOut != "" {
			fmt.Fprint(os.Stderr, errOut)
		}
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}
	return dummyBin
}

func TestRunSucceedsWithControlPlaneReadback(t *testing.T) {
	tmpDir := t.TempDir()
	dummyBin := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyBin)

	// Set API token env var
	t.Setenv("AO2_CP_API_TOKEN", "valid-token")

	// Setup mock plan and gate result
	planPath := filepath.Join(tmpDir, "plan.json")
	planContent := `{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {
			"text": "test",
			"workspace": "test",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": true
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "not_requested",
			"explanation": "test"
		},
		"workcells": [
			{"workcell_id": "cell1", "kind": "prepare", "status": "planned", "depends_on": []}
		],
		"expected_evidence": ["factory plan"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	gatePath := filepath.Join(tmpDir, "gate.json")
	gateContent := `{
		"schema_version": "ao.forge.covenant-gate-result.v0.1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b1",
		"execution_enabled": false,
		"decision": {
			"schema_version": "ao.forge.covenant-decision-fixture.v0.1",
			"decision_id": "allow-local-plan",
			"target_plan_id": "forge-plan-efedbfb309b1",
			"decision": "allow",
			"explanation": "Covenant decision allowed",
			"source": "test"
		},
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate result: %v", err)
	}

	// Mock control plane endpoints
	var uploadedPacket []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		if r.Method == "POST" && r.URL.Path == "/api/v1/operator-packet/signed" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			// Verify signature schema and upload schema
			if body["schema_version"] != "ao2.cp-operator-packet-signed-upload.v1" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Decode and store the operator packet (decoded from base64 string or packet)
			b64Str, ok := body["operator_packet_b64"].(string)
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			decoded, err := base64.StdEncoding.DecodeString(b64Str)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			uploadedPacket = decoded

			// Return ingest receipt
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"schema_version": "ao2.cp-ingest-receipt.v1",
				"sha256": "fake-sha-256",
				"stored_at": "2026-06-17T12:00:00Z",
				"ingested_schema_version": "ao2.operator-evidence-packet.v1"
			}`))
			return
		}

		if r.Method == "GET" && r.URL.Path == "/api/v1/operator-packet/fake-sha-256" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(uploadedPacket)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	outPath := filepath.Join(tmpDir, "packet-out.json")
	code, _, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--control-plane", ts.URL)
	if code != 0 {
		t.Fatalf("expected code 0, got %d (stderr: %s)", code, stderr)
	}

	// Verify control-plane-receipt.json exists and final packet has it in evidence
	receiptPath := filepath.Join(tmpDir, "control-plane-receipt.json")
	receiptData, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("failed to read receipt: %v", err)
	}
	var receipt struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(receiptData, &receipt); err != nil {
		t.Fatalf("receipt not JSON: %v", err)
	}
	if receipt.SchemaVersion != "ao2.cp-ingest-receipt.v1" {
		t.Fatalf("unexpected receipt schema: %s", receipt.SchemaVersion)
	}

	packetBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read out packet: %v", err)
	}
	var passedPacket struct {
		Status   string `json:"status"`
		Evidence []struct {
			Label         string `json:"label"`
			SchemaVersion string `json:"schema_version"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal(packetBytes, &passedPacket); err != nil {
		t.Fatalf("packet not JSON: %v", err)
	}
	if passedPacket.Status != "passed" {
		t.Fatalf("packet status expected passed, got %s", passedPacket.Status)
	}

	foundReceipt := false
	for _, ev := range passedPacket.Evidence {
		if ev.Label == "control plane readback receipt" && ev.SchemaVersion == "ao2.cp-ingest-receipt.v1" {
			foundReceipt = true
		}
	}
	if !foundReceipt {
		t.Fatalf("evidence list missing control plane readback receipt: %+v", passedPacket.Evidence)
	}
}

func TestRunFailsClosedWhenControlPlaneUnavailable(t *testing.T) {
	tmpDir := t.TempDir()
	dummyBin := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyBin)
	t.Setenv("AO2_CP_API_TOKEN", "valid-token")

	planPath := filepath.Join(tmpDir, "plan.json")
	planContent := `{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {"text": "test", "workspace": "test", "release_mode": false},
		"constraints": {"local_first": true, "allow_network": false, "allow_release_mutation": false, "require_control_plane_readback": true},
		"execution_enabled": false,
		"policy_gate": {"required": true, "status": "not_requested", "explanation": "test"},
		"workcells": [{"workcell_id": "cell1", "kind": "prepare", "status": "planned", "depends_on": []}],
		"expected_evidence": ["factory plan"],
		"next_actions": [{"action_id": "test", "description": "test", "required": true}]
	}`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gatePath := filepath.Join(tmpDir, "gate.json")
	gateContent := `{
		"schema_version": "ao.forge.covenant-gate-result.v0.1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b1",
		"execution_enabled": false,
		"decision": {"schema_version": "ao.forge.covenant-decision-fixture.v0.1", "decision_id": "allow-local-plan", "target_plan_id": "forge-plan-efedbfb309b1", "decision": "allow", "explanation": "test", "source": "test"},
		"next_actions": [{"action_id": "test", "description": "test", "required": true}]
	}`
	os.WriteFile(gatePath, []byte(gateContent), 0644)

	// Call using a non-existent control plane server URL
	outPath := filepath.Join(tmpDir, "packet-out.json")
	code, _, _ := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--control-plane", "http://127.0.0.1:48744")
	if code == 0 {
		t.Fatalf("expected failure code, got %d", code)
	}

	packetBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read out packet: %v", err)
	}
	var blockedPacket struct {
		Status string `json:"status"`
	}
	json.Unmarshal(packetBytes, &blockedPacket)
	if blockedPacket.Status != "blocked" {
		t.Fatalf("expected status blocked, got %s", blockedPacket.Status)
	}
}

func TestRunFailsClosedWhenControlPlaneTokenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	dummyBin := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyBin)

	// Unset token
	t.Setenv("AO2_CP_API_TOKEN", "")
	t.Setenv("AO_FORGE_CP_API_TOKEN", "")
	t.Setenv("AO2_CP_AUTH_VALUE", "")

	planPath := filepath.Join(tmpDir, "plan.json")
	planContent := `{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {"text": "test", "workspace": "test", "release_mode": false},
		"constraints": {"local_first": true, "allow_network": false, "allow_release_mutation": false, "require_control_plane_readback": true},
		"execution_enabled": false,
		"policy_gate": {"required": true, "status": "not_requested", "explanation": "test"},
		"workcells": [{"workcell_id": "cell1", "kind": "prepare", "status": "planned", "depends_on": []}],
		"expected_evidence": ["factory plan"],
		"next_actions": [{"action_id": "test", "description": "test", "required": true}]
	}`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gatePath := filepath.Join(tmpDir, "gate.json")
	gateContent := `{
		"schema_version": "ao.forge.covenant-gate-result.v0.1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b1",
		"execution_enabled": false,
		"decision": {"schema_version": "ao.forge.covenant-decision-fixture.v0.1", "decision_id": "allow-local-plan", "target_plan_id": "forge-plan-efedbfb309b1", "decision": "allow", "explanation": "test", "source": "test"},
		"next_actions": [{"action_id": "test", "description": "test", "required": true}]
	}`
	os.WriteFile(gatePath, []byte(gateContent), 0644)

	outPath := filepath.Join(tmpDir, "packet-out.json")
	code, _, _ := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--control-plane", "http://127.0.0.1:8744")
	if code == 0 {
		t.Fatalf("expected failure code, got %d", code)
	}

	packetBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read out packet: %v", err)
	}
	var blockedPacket struct {
		Status string `json:"status"`
	}
	json.Unmarshal(packetBytes, &blockedPacket)
	if blockedPacket.Status != "blocked" {
		t.Fatalf("expected status blocked, got %s", blockedPacket.Status)
	}
}

func TestRunFailsClosedWhenControlPlaneReadbackPayloadDiffers(t *testing.T) {
	tmpDir := t.TempDir()
	dummyBin := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyBin)
	t.Setenv("AO2_CP_API_TOKEN", "valid-token")

	planPath := filepath.Join(tmpDir, "plan.json")
	planContent := `{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {"text": "test", "workspace": "test", "release_mode": false},
		"constraints": {"local_first": true, "allow_network": false, "allow_release_mutation": false, "require_control_plane_readback": true},
		"execution_enabled": false,
		"policy_gate": {"required": true, "status": "not_requested", "explanation": "test"},
		"workcells": [{"workcell_id": "cell1", "kind": "prepare", "status": "planned", "depends_on": []}],
		"expected_evidence": ["factory plan"],
		"next_actions": [{"action_id": "test", "description": "test", "required": true}]
	}`
	os.WriteFile(planPath, []byte(planContent), 0644)

	gatePath := filepath.Join(tmpDir, "gate.json")
	gateContent := `{
		"schema_version": "ao.forge.covenant-gate-result.v0.1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b1",
		"execution_enabled": false,
		"decision": {"schema_version": "ao.forge.covenant-decision-fixture.v0.1", "decision_id": "allow-local-plan", "target_plan_id": "forge-plan-efedbfb309b1", "decision": "allow", "explanation": "test", "source": "test"},
		"next_actions": [{"action_id": "test", "description": "test", "required": true}]
	}`
	os.WriteFile(gatePath, []byte(gateContent), 0644)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/operator-packet/signed" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"schema_version": "ao2.cp-ingest-receipt.v1",
				"sha256": "fake-sha-256",
				"stored_at": "2026-06-17T12:00:00Z",
				"ingested_schema_version": "ao2.operator-evidence-packet.v1"
			}`))
			return
		}

		if r.Method == "GET" && r.URL.Path == "/api/v1/operator-packet/fake-sha-256" {
			w.Header().Set("Content-Type", "application/json")
			// Return a different payload (status is denied) to simulate drift/tampering
			w.Write([]byte(`{
				"schema_version": "ao2.operator-evidence-packet.v1",
				"run_id": "forge-plan-efedbfb309b1",
				"status": "denied",
				"operator_id": "ao-forge-operator",
				"generated_at_utc": "2026-06-17T12:00:00Z",
				"summary": {"recommended_task": "verify signed operator packet readback", "evidence_count": 3},
				"evidence": [],
				"trust_boundary": {"control_plane_role": "read_only_observer", "mutates_ao2": false}
			}`))
			return
		}
	}))
	defer ts.Close()

	outPath := filepath.Join(tmpDir, "packet-out.json")
	code, _, _ := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--control-plane", ts.URL)
	if code == 0 {
		t.Fatalf("expected failure code, got %d", code)
	}

	packetBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read out packet: %v", err)
	}
	var blockedPacket struct {
		Status string `json:"status"`
	}
	json.Unmarshal(packetBytes, &blockedPacket)
	if blockedPacket.Status != "blocked" {
		t.Fatalf("expected status blocked, got %s", blockedPacket.Status)
	}
}

func TestParseMarkdownBrief(t *testing.T) {
	tmpDir := t.TempDir()
	mdPath := filepath.Join(tmpDir, "brief.md")
	mdContent := `# Objective
Run a governed risky PR improvement against the fixture repository.

# Workspace
fixtures/discount-service

# Constraints
- Local First: true
- Allow Network: false
- Allow Release Mutation: false
- Require Control Plane Readback: true
- Release Mode: false

# Expected Workcells
- prepare-fixture (prepare)
- run-ao2-risky-pr (execute) depends on: prepare-fixture
- close-factory-packet (close) depends on: run-ao2-risky-pr

# Expected Evidence
- ao2 run summary
- covenant policy decision
- factory packet
- control plane readback receipt
`
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		t.Fatalf("failed to write md brief: %v", err)
	}

	brief, canonical, err := readBrief(mdPath, false)
	if err != nil {
		t.Fatalf("failed to read md brief: %v", err)
	}

	if brief.Objective.Text != "Run a governed risky PR improvement against the fixture repository." {
		t.Fatalf("unexpected objective text: %s", brief.Objective.Text)
	}
	if brief.Objective.Workspace != "fixtures/discount-service" {
		t.Fatalf("unexpected workspace: %s", brief.Objective.Workspace)
	}
	if !brief.Constraints.LocalFirst || brief.Constraints.AllowNetwork || brief.Constraints.AllowReleaseMutation || !brief.Constraints.RequireControlPlaneReadback {
		t.Fatalf("unexpected constraints: %+v", brief.Constraints)
	}
	if len(brief.ExpectedWorkcells) != 3 {
		t.Fatalf("expected 3 workcells, got %d", len(brief.ExpectedWorkcells))
	}
	if brief.ExpectedWorkcells[1].WorkcellID != "run-ao2-risky-pr" || brief.ExpectedWorkcells[1].Kind != "execute" || len(brief.ExpectedWorkcells[1].DependsOn) != 1 || brief.ExpectedWorkcells[1].DependsOn[0] != "prepare-fixture" {
		t.Fatalf("unexpected workcells: %+v", brief.ExpectedWorkcells)
	}
	if len(brief.ExpectedEvidence) != 4 || brief.ExpectedEvidence[3] != "control plane readback receipt" {
		t.Fatalf("unexpected evidence: %+v", brief.ExpectedEvidence)
	}

	// Verify plan determinism matches JSON brief plan
	jsonPath := filepath.Join(tmpDir, "brief.json")
	jsonContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "Run a governed risky PR improvement against the fixture repository.",
			"workspace": "fixtures/discount-service",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": true
		},
		"expected_workcells": [
			{"workcell_id": "prepare-fixture", "kind": "prepare", "depends_on": []},
			{"workcell_id": "run-ao2-risky-pr", "kind": "execute", "depends_on": ["prepare-fixture"]},
			{"workcell_id": "close-factory-packet", "kind": "close", "depends_on": ["run-ao2-risky-pr"]}
		],
		"expected_evidence": [
			"ao2 run summary",
			"covenant policy decision",
			"factory packet",
			"control plane readback receipt"
		]
	}`
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write json brief: %v", err)
	}

	_, jsonCanonical, err := readBrief(jsonPath, false)
	if err != nil {
		t.Fatalf("failed to read json brief: %v", err)
	}

	// Canonical bytes should be exactly equal
	if string(canonical) != string(jsonCanonical) {
		t.Fatalf("canonical JSON bytes mismatch:\nmd:   %s\njson: %s", string(canonical), string(jsonCanonical))
	}

	// Deterministic plan ID should match
	mdPlanID := planID(canonical)
	jsonPlanID := planID(jsonCanonical)
	if mdPlanID != jsonPlanID {
		t.Fatalf("deterministic plan IDs drifted: md %s, json %s", mdPlanID, jsonPlanID)
	}
}

func TestOnceSucceedsWithWorkspaceOverride(t *testing.T) {
	tmpDir := t.TempDir()

	// Compile dummy ao2
	dummyAo2 := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyAo2)

	// Compile dummy covenant
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" && len(os.Args) > 2 && os.Args[2] == "--json" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	os.WriteFile(dummyCovSrc, []byte(covContent), 0644)
	exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc).Run()

	briefPath := filepath.Join(tmpDir, "brief.json")
	briefContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "test override",
			"workspace": "original-workspace",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_workcells": [
			{"workcell_id": "cell1", "kind": "prepare", "depends_on": []}
		],
		"expected_evidence": ["test"]
	}`
	os.WriteFile(briefPath, []byte(briefContent), 0644)

	outPacket := filepath.Join(tmpDir, "packet.json")
	code, _, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket, "--workspace", "overridden-workspace")
	if code != 0 {
		t.Fatalf("once with workspace override failed: %s", stderr)
	}

	packetBytes, err := os.ReadFile(outPacket)
	if err != nil {
		t.Fatalf("failed to read out packet: %v", err)
	}
	var packet struct {
		Objective struct {
			Workspace string `json:"workspace"`
		} `json:"objective"`
	}
	if err := json.Unmarshal(packetBytes, &packet); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if packet.Objective.Workspace != "overridden-workspace" {
		t.Fatalf("expected workspace overridden-workspace, got %s", packet.Objective.Workspace)
	}
}

func TestOnceEmitsMarkdownPacket(t *testing.T) {
	tmpDir := t.TempDir()

	// Compile dummy ao2
	dummyAo2 := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyAo2)

	// Compile dummy covenant
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" && len(os.Args) > 2 && os.Args[2] == "--json" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	os.WriteFile(dummyCovSrc, []byte(covContent), 0644)
	exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc).Run()

	briefPath := filepath.Join(tmpDir, "brief.json")
	briefContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "test md",
			"workspace": "test-ws",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_workcells": [
			{"workcell_id": "cell1", "kind": "prepare", "depends_on": []}
		],
		"expected_evidence": ["test"]
	}`
	os.WriteFile(briefPath, []byte(briefContent), 0644)

	outPacket := filepath.Join(tmpDir, "factory-packet.json")
	code, _, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
	if code != 0 {
		t.Fatalf("once failed: %s", stderr)
	}

	// Verify packet.md exists in the same directory as factory-packet.json
	mdPath := filepath.Join(tmpDir, "packet.md")
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("failed to read packet.md: %v", err)
	}
	mdContent := string(mdBytes)

	for _, want := range []string{
		"# AO Forge Factory Packet",
		"- **Status**: PASSED",
		"## Objective",
		"test md",
		"- **Workspace**: test-ws",
		"## Workcells",
		"| Workcell ID | Kind | Executor | Status | Workspace | Run Mode | Summary |",
		"| cell1 | prepare | ao2 | passed | test-ws | dry-run | Dry-run accepted by ao2 |",
		"## Evidence",
		"## Next Actions",
	} {
		if !strings.Contains(mdContent, want) {
			t.Fatalf("packet.md missing %q:\n%s", want, mdContent)
		}
	}
}

func TestSchedulerCycleDetection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "forge-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	briefPath := filepath.Join(tmpDir, "cyclic-brief.json")
	briefContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "test cycle",
			"workspace": "test-ws",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_workcells": [
			{"workcell_id": "cell1", "kind": "prepare", "depends_on": ["cell2"]},
			{"workcell_id": "cell2", "kind": "execute", "depends_on": ["cell1"]}
		],
		"expected_evidence": ["test"]
	}`
	if err := os.WriteFile(briefPath, []byte(briefContent), 0644); err != nil {
		t.Fatalf("write cyclic brief: %v", err)
	}

	code, _, stderr := runCLI("plan", "--brief", briefPath)
	if code != 1 {
		t.Fatalf("expected code 1, got %d (stderr: %q)", code, stderr)
	}
	if !strings.Contains(stderr, "cyclic dependency detected") {
		t.Fatalf("expected cyclic dependency error in stderr, got: %q", stderr)
	}
}

func TestSchedulerConcurrentExecution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "forge-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	traceFile := filepath.Join(tmpDir, "trace.log")

	// Compile a mock ao2 that sleeps for 500ms and writes timestamps to a trace file
	dummyAo2Src := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyAo2Bin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyAo2Bin += ".exe"
	}
	ao2Content := fmt.Sprintf(`package main
import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		var specFile string
		for i := 0; i < len(os.Args); i++ {
			if os.Args[i] == "--spec" && i+1 < len(os.Args) {
				specFile = os.Args[i+1]
			}
		}
		data, _ := os.ReadFile(specFile)
		var spec map[string]any
		json.Unmarshal(data, &spec)
		specObj := spec["spec"].(map[string]any)
		tasks := specObj["tasks"].([]any)
		task := tasks[0].(map[string]any)
		taskID := task["id"].(string)

		start := time.Now().UnixNano() / int64(time.Millisecond)
		time.Sleep(200 * time.Millisecond)
		end := time.Now().UnixNano() / int64(time.Millisecond)

		f, _ := os.OpenFile(%q, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(f, "%%s: %%d - %%d\n", taskID, start, end)
		f.Close()

		fmt.Println("status=dry_run_accepted")
		fmt.Println("schema_version=ao2.run/v1")
		fmt.Println("plan_id=forge-plan-efedbfb309b1")
		os.Exit(0)
	}
	os.Exit(1)
}`, traceFile)

	if err := os.WriteFile(dummyAo2Src, []byte(ao2Content), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}
	cmdAo2 := exec.Command("go", "build", "-o", dummyAo2Bin, dummyAo2Src)
	if out, err := cmdAo2.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}
	t.Setenv("AO2_PATH", dummyAo2Bin)

	// Write mock covenant decision fixture
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	briefPath := filepath.Join(tmpDir, "brief.json")
	briefContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "test concurrent execution",
			"workspace": "test-ws",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_workcells": [
			{"workcell_id": "wc1", "kind": "prepare", "depends_on": []},
			{"workcell_id": "wc2", "kind": "execute", "depends_on": []}
		],
		"expected_evidence": ["test"]
	}`
	if err := os.WriteFile(briefPath, []byte(briefContent), 0644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	outPacket := filepath.Join(tmpDir, "packet.json")
	start := time.Now()
	code, stdoutVal, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
	duration := time.Since(start)

	if code != 0 {
		t.Fatalf("once failed: %s (stdout: %q)", stderr, stdoutVal)
	}

	// Print trace file contents
	traceBytes, traceErr := os.ReadFile(traceFile)
	if traceErr != nil {
		t.Fatalf("Failed to read trace log: %v", traceErr)
	}
	t.Logf("Trace log:\n%s", string(traceBytes))

	// Parse trace log to verify concurrent overlap
	lines := strings.Split(strings.TrimSpace(string(traceBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 lines in trace log, got %d", len(lines))
	}
	var start1, end1, start2, end2 int64
	for _, line := range lines {
		var name string
		var s, e int64
		if _, err := fmt.Sscanf(line, "%s %d - %d", &name, &s, &e); err != nil {
			t.Fatalf("failed to parse trace line %q: %v", line, err)
		}
		if name == "wc1:" {
			start1, end1 = s, e
		} else if name == "wc2:" {
			start2, end2 = s, e
		}
	}
	if start1 >= end2 || start2 >= end1 {
		t.Fatalf("expected executions to overlap (be concurrent), but wc1 ran %d-%d and wc2 ran %d-%d", start1, end1, start2, end2)
	}

	// Verify duration is reasonable (failsafe to ensure it did not hang)
	if duration >= 2500*time.Millisecond {
		t.Fatalf("execution took too long, expected under 2500ms, took %v", duration)
	}

	// Verify both workcells are passed in packet
	data, err := os.ReadFile(outPacket)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(data, &packet); err != nil {
		t.Fatalf("failed to parse packet: %v", err)
	}

	if len(packet.Workcells) != 2 {
		t.Fatalf("expected 2 workcells, got %d", len(packet.Workcells))
	}
	for _, wc := range packet.Workcells {
		if wc.Status != "passed" {
			t.Fatalf("expected workcell %s to be passed, got %s", wc.WorkcellID, wc.Status)
		}
	}
}

func TestSchedulerDependencyOrchestration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "forge-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	traceFile := filepath.Join(tmpDir, "trace.log")

	// Compile a mock ao2 that appends the executed task ID to a trace file
	dummyAo2Src := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyAo2Bin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyAo2Bin += ".exe"
	}
	ao2Content := fmt.Sprintf(`package main
import (
	"encoding/json"
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		// Read spec to find task ID
		var specFile string
		for i := 0; i < len(os.Args); i++ {
			if os.Args[i] == "--spec" && i+1 < len(os.Args) {
				specFile = os.Args[i+1]
			}
		}
		data, _ := os.ReadFile(specFile)
		var spec map[string]any
		json.Unmarshal(data, &spec)
		specObj := spec["spec"].(map[string]any)
		tasks := specObj["tasks"].([]any)
		task := tasks[0].(map[string]any)
		taskID := task["id"].(string)

		f, _ := os.OpenFile(%q, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		f.WriteString(taskID + "\n")
		f.Close()

		fmt.Println("status=dry_run_accepted")
		fmt.Println("schema_version=ao2.run/v1")
		fmt.Println("plan_id=forge-plan-efedbfb309b1")
		os.Exit(0)
	}
	os.Exit(1)
}`, traceFile)

	if err := os.WriteFile(dummyAo2Src, []byte(ao2Content), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}
	cmdAo2 := exec.Command("go", "build", "-o", dummyAo2Bin, dummyAo2Src)
	if out, err := cmdAo2.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}
	t.Setenv("AO2_PATH", dummyAo2Bin)

	// Write mock covenant decision fixture
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	briefPath := filepath.Join(tmpDir, "brief.json")
	briefContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "test dependency orchestration",
			"workspace": "test-ws",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_workcells": [
			{"workcell_id": "wc1", "kind": "prepare", "depends_on": ["wc2"]},
			{"workcell_id": "wc2", "kind": "execute", "depends_on": []}
		],
		"expected_evidence": ["test"]
	}`
	if err := os.WriteFile(briefPath, []byte(briefContent), 0644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	outPacket := filepath.Join(tmpDir, "packet.json")
	code, _, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
	if code != 0 {
		t.Fatalf("once failed: %s", stderr)
	}

	traceBytes, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("failed to read trace: %v", err)
	}
	traceLines := strings.Split(strings.TrimSpace(string(traceBytes)), "\n")
	if len(traceLines) != 2 {
		t.Fatalf("expected 2 executed workcells, got %d (trace: %q)", len(traceLines), string(traceBytes))
	}
	if traceLines[0] != "wc2" || traceLines[1] != "wc1" {
		t.Fatalf("expected execution order wc2 -> wc1, got: %v", traceLines)
	}
}

func TestSchedulerFailurePropagation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "forge-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Compile a mock ao2 that fails for wc_fail
	dummyAo2Src := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyAo2Bin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyAo2Bin += ".exe"
	}
	ao2Content := `package main
import (
	"encoding/json"
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		var specFile string
		for i := 0; i < len(os.Args); i++ {
			if os.Args[i] == "--spec" && i+1 < len(os.Args) {
				specFile = os.Args[i+1]
			}
		}
		data, _ := os.ReadFile(specFile)
		var spec map[string]any
		json.Unmarshal(data, &spec)
		specObj := spec["spec"].(map[string]any)
		tasks := specObj["tasks"].([]any)
		task := tasks[0].(map[string]any)
		taskID := task["id"].(string)

		if taskID == "wc_fail" {
			os.Exit(1)
		}
		fmt.Println("status=dry_run_accepted")
		fmt.Println("schema_version=ao2.run/v1")
		fmt.Println("plan_id=forge-plan-efedbfb309b1")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyAo2Src, []byte(ao2Content), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}
	cmdAo2 := exec.Command("go", "build", "-o", dummyAo2Bin, dummyAo2Src)
	if out, err := cmdAo2.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}
	t.Setenv("AO2_PATH", dummyAo2Bin)

	// Write mock covenant decision fixture
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	briefPath := filepath.Join(tmpDir, "brief.json")
	briefContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "test failure propagation",
			"workspace": "test-ws",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_workcells": [
			{"workcell_id": "wc_fail", "kind": "prepare", "depends_on": []},
			{"workcell_id": "wc_dep", "kind": "execute", "depends_on": ["wc_fail"]}
		],
		"expected_evidence": ["test"]
	}`
	if err := os.WriteFile(briefPath, []byte(briefContent), 0644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	outPacket := filepath.Join(tmpDir, "packet.json")
	code, _, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
	if code != 1 {
		t.Fatalf("expected once to fail with exit code 1, got %d (stderr: %q)", code, stderr)
	}

	data, err := os.ReadFile(outPacket)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(data, &packet); err != nil {
		t.Fatalf("failed to parse packet: %v", err)
	}

	if packet.Status != "failed" {
		t.Fatalf("expected packet status to be failed, got %q", packet.Status)
	}
	if len(packet.Workcells) != 2 {
		t.Fatalf("expected 2 workcells, got %d", len(packet.Workcells))
	}

	// wc_fail should be failed, wc_dep should be skipped
	for _, wc := range packet.Workcells {
		if wc.WorkcellID == "wc_fail" {
			if wc.Status != "failed" {
				t.Fatalf("expected wc_fail status to be failed, got %q", wc.Status)
			}
		} else if wc.WorkcellID == "wc_dep" {
			if wc.Status != "skipped" {
				t.Fatalf("expected wc_dep status to be skipped, got %q", wc.Status)
			}
		}
	}
}

func TestOnceEmitsWorkcellEvidence(t *testing.T) {
	tmpDir := t.TempDir()

	// Compile a dummy binary that acts as ao2, outputting different lines to stdout
	dummyAo2Src := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyAo2Bin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyAo2Bin += ".exe"
	}
	ao2Content := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		fmt.Println("status=dry_run_accepted")
		fmt.Println("schema_version=ao2.run/v1")
		fmt.Println("plan_id=forge-plan-efedbfb309b1")
		fmt.Println("task_count=2")
		fmt.Println("target_repo=fixtures/discount-service")
		fmt.Println("control_plane_role=read_only_observer")
		fmt.Println("mutates_ao_artifacts=false")
		fmt.Println("factory_v3_drives_workflow=false")
		fmt.Println("spec_sha256=abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyAo2Src, []byte(ao2Content), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}
	cmdAo2 := exec.Command("go", "build", "-o", dummyAo2Bin, dummyAo2Src)
	if out, err := cmdAo2.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}
	t.Setenv("AO2_PATH", dummyAo2Bin)

	// Compile a dummy binary that acts as covenant
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	briefPath := filepath.Join(tmpDir, "brief.json")
	briefContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "test dynamic evidence",
			"workspace": "test-ws",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_workcells": [
			{"workcell_id": "wc1", "kind": "prepare", "depends_on": []},
			{"workcell_id": "wc2", "kind": "execute", "depends_on": []}
		],
		"expected_evidence": ["test"]
	}`
	if err := os.WriteFile(briefPath, []byte(briefContent), 0644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	outPacket := filepath.Join(tmpDir, "packet.json")
	code, stdoutVal, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
	if code != 0 {
		t.Fatalf("once failed: %s (stdout: %q)", stderr, stdoutVal)
	}

	// 1. Verify ao2-run-summary.json exists and does NOT contain abcdef...
	summaryPath := filepath.Join(tmpDir, "ao2-run-summary.json")
	summaryBytes, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read run summary: %v", err)
	}
	var runSummary map[string]any
	if err := json.Unmarshal(summaryBytes, &runSummary); err != nil {
		t.Fatalf("failed to parse run summary: %v", err)
	}
	specSHA := runSummary["spec_sha256"].(string)
	if specSHA == "" || specSHA == "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd" {
		t.Fatalf("expected dynamically computed spec_sha256, got: %q", specSHA)
	}

	// 2. Verify individual workcell evidence files were written and are valid
	wc1EvPath := filepath.Join(tmpDir, "ao2-wc-wc1-evidence.json")
	wc1EvBytes, err := os.ReadFile(wc1EvPath)
	if err != nil {
		t.Fatalf("failed to read wc1 evidence: %v", err)
	}
	var wc1Ev map[string]any
	if err := json.Unmarshal(wc1EvBytes, &wc1Ev); err != nil {
		t.Fatalf("failed to parse wc1 evidence: %v", err)
	}
	if wc1Ev["schema_version"] != "ao2.workcell-evidence.v1" {
		t.Fatalf("expected schema ao2.workcell-evidence.v1, got: %q", wc1Ev["schema_version"])
	}
	if wc1Ev["workcell_id"] != "wc1" || wc1Ev["status"] != "passed" {
		t.Fatalf("unexpected wc1 content: %+v", wc1Ev)
	}
	if !strings.Contains(wc1Ev["stdout"].(string), "status=dry_run_accepted") {
		t.Fatalf("wc1 stdout should contain status=dry_run_accepted, got: %q", wc1Ev["stdout"])
	}

	wc2EvPath := filepath.Join(tmpDir, "ao2-wc-wc2-evidence.json")
	if _, err := os.Stat(wc2EvPath); err != nil {
		t.Fatalf("failed to read wc2 evidence: %v", err)
	}

	// 3. Verify packet contains 5 evidence items
	data, err := os.ReadFile(outPacket)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(data, &packet); err != nil {
		t.Fatalf("failed to parse packet: %v", err)
	}
	if len(packet.Evidence) != 5 {
		t.Fatalf("expected 5 evidence items, got %d", len(packet.Evidence))
	}

	foundWC1, foundWC2 := false, false
	for _, ev := range packet.Evidence {
		if ev.Label == "workcell wc1 evidence" {
			foundWC1 = true
			if ev.SchemaVersion != "ao2.workcell-evidence.v1" {
				t.Fatalf("unexpected wc1 evidence schema: %q", ev.SchemaVersion)
			}
		}
		if ev.Label == "workcell wc2 evidence" {
			foundWC2 = true
			if ev.SchemaVersion != "ao2.workcell-evidence.v1" {
				t.Fatalf("unexpected wc2 evidence schema: %q", ev.SchemaVersion)
			}
		}
	}
	if !foundWC1 || !foundWC2 {
		t.Fatalf("did not find both wc1 and wc2 evidence in packet")
	}
}

func TestGateReleaseModeValidation(t *testing.T) {
	tmpDir := t.TempDir()

	// Compile a dummy binary that acts as covenant
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	// Helper to create a plan
	makePlan := func(workspace string, releaseMode bool, allowMutation bool) string {
		plan := factoryPlan{
			SchemaVersion: "ao.forge.factory-plan.v0.1",
			PlanID:        "forge-plan-efedbfb309b1",
			Objective: factoryObjective{
				Text:        "test release validation",
				Workspace:   workspace,
				ReleaseMode: releaseMode,
			},
			Constraints: factoryConstraints{
				LocalFirst:           true,
				AllowNetwork:         false,
				AllowReleaseMutation: allowMutation,
			},
			PolicyGate: policyGate{
				Required:    true,
				Status:      "not_requested",
				Explanation: "test",
			},
			Workcells: []planWorkcell{
				{WorkcellID: "wc1", Kind: "prepare", Status: "planned", DependsOn: []string{}},
			},
			ExpectedEvidence: []string{"test"},
			NextActions: []nextAction{
				{ActionID: "test", Description: "test", Required: true},
			},
		}
		data, _ := json.Marshal(plan)
		planPath := filepath.Join(tmpDir, fmt.Sprintf("plan-%t-%t.json", releaseMode, allowMutation))
		_ = os.WriteFile(planPath, data, 0644)
		return planPath
	}

	// 1. Workspace does not exist -> Denied
	plan1 := makePlan(filepath.Join(tmpDir, "nonexistent"), true, false)
	code, stdout, stderr := runCLI("gate", "--plan", plan1, "--covenant", dummyCovBin)
	if code != 1 {
		t.Fatalf("expected code 1, got %d (stderr: %q)", code, stderr)
	}
	var gateRes1 covenantGateResult
	if err := json.Unmarshal([]byte(stdout), &gateRes1); err != nil {
		t.Fatalf("failed to parse gate result: %v", err)
	}
	if gateRes1.Status != "blocked" || gateRes1.Decision.DecisionID != "deny-dirty-release-workspace" {
		t.Fatalf("unexpected gate result for nonexistent workspace: %+v", gateRes1)
	}

	// 2. Workspace exists but is not a git repo -> Denied
	noGitDir := filepath.Join(tmpDir, "nogit")
	_ = os.Mkdir(noGitDir, 0755)
	plan2 := makePlan(noGitDir, true, false)
	code, stdout, stderr = runCLI("gate", "--plan", plan2, "--covenant", dummyCovBin)
	if code != 1 {
		t.Fatalf("expected code 1, got %d (stderr: %q)", code, stderr)
	}
	var gateRes2 covenantGateResult
	if err := json.Unmarshal([]byte(stdout), &gateRes2); err != nil {
		t.Fatalf("failed to parse gate result: %v", err)
	}
	if gateRes2.Status != "blocked" || gateRes2.Decision.DecisionID != "deny-dirty-release-workspace" {
		t.Fatalf("unexpected gate result for non-git workspace: %+v", gateRes2)
	}

	// 3. Workspace is a dirty git repo -> Denied
	dirtyGitDir := filepath.Join(tmpDir, "dirtygit")
	_ = os.Mkdir(dirtyGitDir, 0755)
	runGit := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit(dirtyGitDir, "init")
	_ = os.WriteFile(filepath.Join(dirtyGitDir, "dirty.txt"), []byte("dirty"), 0644)
	// Not committed, so dirty

	plan3 := makePlan(dirtyGitDir, true, false)
	code, stdout, stderr = runCLI("gate", "--plan", plan3, "--covenant", dummyCovBin)
	if code != 1 {
		t.Fatalf("expected code 1, got %d (stderr: %q)", code, stderr)
	}
	var gateRes3 covenantGateResult
	if err := json.Unmarshal([]byte(stdout), &gateRes3); err != nil {
		t.Fatalf("failed to parse gate result: %v", err)
	}
	if gateRes3.Status != "blocked" || gateRes3.Decision.DecisionID != "deny-dirty-release-workspace" {
		t.Fatalf("unexpected gate result for dirty git workspace: %+v", gateRes3)
	}

	// 4. Workspace is a clean git repo -> Success (Allowed)
	cleanGitDir := filepath.Join(tmpDir, "cleangit")
	_ = os.Mkdir(cleanGitDir, 0755)
	runGit(cleanGitDir, "init")
	runGit(cleanGitDir, "config", "user.email", "test@example.com")
	runGit(cleanGitDir, "config", "user.name", "Test")
	runGit(cleanGitDir, "config", "commit.gpgSign", "false")
	_ = os.WriteFile(filepath.Join(cleanGitDir, "clean.txt"), []byte("clean"), 0644)
	runGit(cleanGitDir, "add", "clean.txt")
	runGit(cleanGitDir, "commit", "-m", "init")

	plan4 := makePlan(cleanGitDir, true, false)
	code, stdout, stderr = runCLI("gate", "--plan", plan4, "--covenant", dummyCovBin)
	if code != 0 {
		t.Fatalf("expected code 0, got %d (stderr: %q)", code, stderr)
	}
	var gateRes4 covenantGateResult
	if err := json.Unmarshal([]byte(stdout), &gateRes4); err != nil {
		t.Fatalf("failed to parse gate result: %v", err)
	}
	if gateRes4.Status != "allowed" || gateRes4.Decision.DecisionID != "allow-local-plan" {
		t.Fatalf("unexpected gate result for clean git workspace: %+v", gateRes4)
	}

	// 5. AllowReleaseMutation: true, ReleaseMode: false -> Denied
	plan5 := makePlan(cleanGitDir, false, true)
	code, stdout, stderr = runCLI("gate", "--plan", plan5, "--covenant", dummyCovBin)
	if code != 1 {
		t.Fatalf("expected code 1, got %d (stderr: %q)", code, stderr)
	}
	var gateRes5 covenantGateResult
	if err := json.Unmarshal([]byte(stdout), &gateRes5); err != nil {
		t.Fatalf("failed to parse gate result: %v", err)
	}
	if gateRes5.Status != "denied" || gateRes5.Decision.DecisionID != "deny-non-release-mutation" {
		t.Fatalf("unexpected gate result for non-release mutation request: %+v", gateRes5)
	}

	// 6. AllowReleaseMutation: true, ReleaseMode: true (and clean workspace) -> Blocked (Indeterminate)
	plan6 := makePlan(cleanGitDir, true, true)
	code, stdout, stderr = runCLI("gate", "--plan", plan6, "--covenant", dummyCovBin)
	if code != 1 {
		t.Fatalf("expected code 1, got %d (stderr: %q)", code, stderr)
	}
	var gateRes6 covenantGateResult
	if err := json.Unmarshal([]byte(stdout), &gateRes6); err != nil {
		t.Fatalf("failed to parse gate result: %v", err)
	}
	if gateRes6.Status != "blocked" || gateRes6.Decision.DecisionID != "indeterminate-release-mutation" {
		t.Fatalf("unexpected gate result for indeterminate release mutation request: %+v", gateRes6)
	}
}

func TestGateLiveExecutionMode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 123456, "tag_name": "v1.0.0"}`))
	}))
	defer ts.Close()
	t.Setenv("AO_FORGE_MOCK_GITHUB_API", ts.URL)
	t.Setenv("AO_FORGE_RELEASE_TAG", "v1.0.0")
	t.Setenv("GITHUB_TOKEN", "mock-github-token")

	tmpDir := t.TempDir()
	t.Setenv("GIT_PATH", buildGitPushWrapper(t, tmpDir))

	// Compile a dummy binary that acts as covenant
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	// Compile a dummy binary that acts as ao2
	dummyAo2Src := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyAo2Bin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyAo2Bin += ".exe"
	}
	ao2Content := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		hasDryRun := false
		specPath := ""
		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "--dry-run" {
				hasDryRun = true
			} else if os.Args[i] == "--spec" && i+1 < len(os.Args) {
				specPath = os.Args[i+1]
			}
		}
		if specPath != "" {
			if hasDryRun {
				fmt.Println("status=dry_run_accepted")
			} else {
				fmt.Println("status=governed_run_started")
			}
			os.Exit(0)
		}
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyAo2Src, []byte(ao2Content), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}
	cmdAo2 := exec.Command("go", "build", "-o", dummyAo2Bin, dummyAo2Src)
	if out, err := cmdAo2.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}

	// Set AO2_PATH env var so resolver finds it
	t.Setenv("AO2_PATH", dummyAo2Bin)

	// Helper to create a plan
	makePlan := func(workspace string, releaseMode bool, allowMutation bool) string {
		plan := factoryPlan{
			SchemaVersion: "ao.forge.factory-plan.v0.1",
			PlanID:        "forge-plan-efedbfb309b1",
			Objective: factoryObjective{
				Text:        "test live mode",
				Workspace:   workspace,
				ReleaseMode: releaseMode,
			},
			Constraints: factoryConstraints{
				LocalFirst:           true,
				AllowNetwork:         false,
				AllowReleaseMutation: allowMutation,
			},
			PolicyGate: policyGate{
				Required:    true,
				Status:      "not_requested",
				Explanation: "test",
			},
			Workcells: []planWorkcell{
				{WorkcellID: "wc1", Kind: "prepare", Status: "planned", DependsOn: []string{}},
			},
			ExpectedEvidence: []string{"test"},
			NextActions: []nextAction{
				{ActionID: "test", Description: "test", Required: true},
			},
		}
		data, _ := json.Marshal(plan)
		planPath := filepath.Join(tmpDir, fmt.Sprintf("plan-%t-%t.json", releaseMode, allowMutation))
		_ = os.WriteFile(planPath, data, 0644)
		return planPath
	}

	// Helper to create an allowed gate result
	makeGateResult := func(planID string, status string, decisionID string) string {
		res := covenantGateResult{
			SchemaVersion:    "ao.forge.covenant-gate-result.v0.1",
			Status:           status,
			PlanID:           planID,
			ExecutionEnabled: true,
			Decision: covenantDecisionFixture{
				SchemaVersion: "ao.forge.covenant-decision-fixture.v0.1",
				TargetPlanID:  planID,
				Decision:      status,
				DecisionID:    decisionID,
				Explanation:   "Approved",
				Source:        "live-covenant-adapter",
			},
		}
		data, _ := json.Marshal(res)
		gatePath := filepath.Join(tmpDir, fmt.Sprintf("gate-%s.json", status))
		_ = os.WriteFile(gatePath, data, 0644)
		return gatePath
	}

	cleanGitDir := filepath.Join(tmpDir, "cleangit")
	_ = os.Mkdir(cleanGitDir, 0755)
	runGit := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit(cleanGitDir, "init")
	runGit(cleanGitDir, "config", "user.email", "test@example.com")
	runGit(cleanGitDir, "config", "user.name", "Test")
	runGit(cleanGitDir, "config", "commit.gpgSign", "false")
	runGit(cleanGitDir, "remote", "add", "origin", "git@github.com:test-owner/test-repo.git")
	_ = os.WriteFile(filepath.Join(cleanGitDir, "clean.txt"), []byte("clean"), 0644)
	runGit(cleanGitDir, "add", "clean.txt")
	runGit(cleanGitDir, "commit", "-m", "init")

	releasePreviewAuditPath := filepath.Join(tmpDir, "release-preview-live-mode.json")
	previewCode, previewStdout, previewStderr := runCLI("release-preview", "--workspace", cleanGitDir, "--tag", "v1.0.0", "--out", releasePreviewAuditPath)
	if previewCode != 0 {
		t.Fatalf("release-preview failed with code %d\nstdout: %s\nstderr: %s", previewCode, previewStdout, previewStderr)
	}

	// 1. Safe Plan, --live mode (not release mode) -> succeeds
	plan1 := makePlan(cleanGitDir, false, false)
	gate1 := makeGateResult("forge-plan-efedbfb309b1", "allowed", "allow-local-plan")
	outPath1 := filepath.Join(tmpDir, "packet1.json")
	code1, stdout1, stderr1 := runCLI("run", "--plan", plan1, "--gate-result", gate1, "--out", outPath1, "--live")
	if code1 != 0 {
		t.Fatalf("expected code 0, got %d (stdout: %q, stderr: %q)", code1, stdout1, stderr1)
	}
	packetData, err := os.ReadFile(outPath1)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var pkt1 factoryPacket
	if err := json.Unmarshal(packetData, &pkt1); err != nil {
		t.Fatalf("failed to parse packet: %v", err)
	}
	if pkt1.Status != "passed" || pkt1.Workcells[0].AO2Run != "live" {
		t.Fatalf("unexpected packet properties for safe live run: %+v", pkt1)
	}

	// 2. Release Mode Plan, --live mode, WITHOUT --confirm-release -> fails closed (blocked)
	plan2 := makePlan(cleanGitDir, true, false)
	gate2 := makeGateResult("forge-plan-efedbfb309b1", "allowed", "allow-local-plan")
	outPath2 := filepath.Join(tmpDir, "packet2.json")
	code2, _, _ := runCLI("run", "--plan", plan2, "--gate-result", gate2, "--out", outPath2, "--live")
	if code2 != 1 {
		t.Fatalf("expected code 1, got %d", code2)
	}
	packetData2, err := os.ReadFile(outPath2)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var pkt2 factoryPacket
	if err := json.Unmarshal(packetData2, &pkt2); err != nil {
		t.Fatalf("failed to parse packet: %v", err)
	}
	if pkt2.Status != "blocked" || pkt2.PolicyDecisions[0].DecisionID != "release-confirmation-required" {
		t.Fatalf("unexpected packet properties for unconfirmed live release mode run: %+v", pkt2)
	}

	// 3. Release Mode Plan, --live mode, WITH --confirm-release -> succeeds
	outPath3 := filepath.Join(tmpDir, "packet3.json")
	code3, _, stderr3 := runCLI("run", "--plan", plan2, "--gate-result", gate2, "--out", outPath3, "--live", "--confirm-release", "--release-preview-audit", releasePreviewAuditPath)
	if code3 != 0 {
		t.Fatalf("expected code 0, got %d (stderr: %q)", code3, stderr3)
	}
	packetData3, err := os.ReadFile(outPath3)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var pkt3 factoryPacket
	if err := json.Unmarshal(packetData3, &pkt3); err != nil {
		t.Fatalf("failed to parse packet: %v", err)
	}
	if pkt3.Status != "passed" || pkt3.Workcells[0].AO2Run != "live" || !pkt3.TrustBoundary.MutatesReleases {
		t.Fatalf("unexpected packet properties for confirmed live release mode run: %+v", pkt3)
	}

	// 4. Gate Result is indeterminate-release-mutation, WITH --confirm-release -> succeeds (operator override)
	gate4 := makeGateResult("forge-plan-efedbfb309b1", "blocked", "indeterminate-release-mutation")
	outPath4 := filepath.Join(tmpDir, "packet4.json")
	code4, _, stderr4 := runCLI("run", "--plan", plan2, "--gate-result", gate4, "--out", outPath4, "--live", "--confirm-release", "--release-preview-audit", releasePreviewAuditPath)
	if code4 != 0 {
		t.Fatalf("expected code 0, got %d (stderr: %q)", code4, stderr4)
	}
	packetData4, err := os.ReadFile(outPath4)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var pkt4 factoryPacket
	if err := json.Unmarshal(packetData4, &pkt4); err != nil {
		t.Fatalf("failed to parse packet: %v", err)
	}
	if pkt4.Status != "passed" || pkt4.Workcells[0].AO2Run != "live" || !pkt4.TrustBoundary.MutatesReleases {
		t.Fatalf("unexpected packet properties for overridden indeterminate run: %+v", pkt4)
	}
}

func TestForgeInitAndPacketPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Compile a dummy binary that acts as covenant
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	cleanGitDir := filepath.Join(tmpDir, "cleangit")
	_ = os.Mkdir(cleanGitDir, 0755)
	runGit := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit(cleanGitDir, "init")
	runGit(cleanGitDir, "config", "user.email", "test@example.com")
	runGit(cleanGitDir, "config", "user.name", "Test")
	runGit(cleanGitDir, "config", "commit.gpgSign", "false")
	_ = os.WriteFile(filepath.Join(cleanGitDir, "clean.txt"), []byte("clean"), 0644)
	runGit(cleanGitDir, "add", "clean.txt")
	runGit(cleanGitDir, "commit", "-m", "init")

	// Change current working directory to tmpDir to test local .forge creation
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	// 1. Run init command
	code, stdout, stderr := runCLI("init")
	if code != 0 {
		t.Fatalf("expected code 0 from init, got %d (stderr: %q)", code, stderr)
	}
	if !strings.Contains(stdout, "initialized under .forge") {
		t.Fatalf("unexpected init stdout: %q", stdout)
	}

	// Verify .forge and .forge/runs exist
	if info, err := os.Stat(".forge"); err != nil || !info.IsDir() {
		t.Fatalf(".forge directory was not created")
	}
	if info, err := os.Stat(filepath.Join(".forge", "runs")); err != nil || !info.IsDir() {
		t.Fatalf(".forge/runs directory was not created")
	}

	// 2. Run once command to execute and verify archiving
	// Create a dummy brief pointing to cleanGitDir as workspace
	briefContent := fmt.Sprintf(`# Objective
test persistence once

# Workspace
%s

# Constraints
- local_first: true

# Expected Workcells
- wc1 (prepare)

# Expected Evidence
- test
`, strings.ReplaceAll(cleanGitDir, "\\", "/"))
	briefPath := filepath.Join(tmpDir, "brief.md")
	if err := os.WriteFile(briefPath, []byte(briefContent), 0644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	// Compile a dummy binary that acts as ao2
	dummyAo2Src := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyAo2Bin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyAo2Bin += ".exe"
	}
	ao2Content := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		fmt.Println("status=dry_run_accepted")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyAo2Src, []byte(ao2Content), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}
	cmdAo2 := exec.Command("go", "build", "-o", dummyAo2Bin, dummyAo2Src)
	if out, err := cmdAo2.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}

	t.Setenv("AO2_PATH", dummyAo2Bin)

	// Run once to auto-generate plan and execute
	outPath := filepath.Join(tmpDir, "packet_once.json")
	code, stdout, stderr = runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPath)
	if code != 0 {
		t.Fatalf("expected code 0 from once, got %d (stderr: %q)", code, stderr)
	}

	// Read packet to find the plan ID
	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read once packet: %v", err)
	}
	var pkt factoryPacket
	if err := json.Unmarshal(packetData, &pkt); err != nil {
		t.Fatalf("failed to unmarshal once packet: %v", err)
	}
	planID := pkt.FactoryPlan.PlanID
	if planID == "" {
		t.Fatalf("plan ID was not generated")
	}

	// Verify files were archived under .forge/runs/<planID>/
	archiveDir := filepath.Join(".forge", "runs", planID)
	if _, err := os.Stat(filepath.Join(archiveDir, "plan.json")); err != nil {
		t.Fatalf("plan.json was not archived")
	}
	if _, err := os.Stat(filepath.Join(archiveDir, "gate_result.json")); err != nil {
		t.Fatalf("gate_result.json was not archived")
	}
	if _, err := os.Stat(filepath.Join(archiveDir, "factory-packet.json")); err != nil {
		t.Fatalf("factory-packet.json was not archived")
	}

	// 3. Retrieve packet using forge packet subcommand
	code, stdout, stderr = runCLI("packet", "--run", planID)
	if code != 0 {
		t.Fatalf("expected code 0 from packet command, got %d (stderr: %q)", code, stderr)
	}
	var retrievedPkt factoryPacket
	if err := json.Unmarshal([]byte(stdout), &retrievedPkt); err != nil {
		t.Fatalf("failed to parse retrieved packet: %v", err)
	}
	if retrievedPkt.FactoryPlan.PlanID != planID {
		t.Fatalf("retrieved plan ID mismatch: expected %q, got %q", planID, retrievedPkt.FactoryPlan.PlanID)
	}
}

func TestLiveRunControlPlaneOptionalSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dummyBin := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyBin)

	// Set API token env var
	t.Setenv("AO2_CP_API_TOKEN", "valid-token")

	// Setup mock plan and gate result
	planPath := filepath.Join(tmpDir, "plan.json")
	planContent := `{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b2",
		"objective": {
			"text": "test-optional-success",
			"workspace": "test",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "not_requested",
			"explanation": "test"
		},
		"workcells": [
			{"workcell_id": "cell1", "kind": "prepare", "status": "planned", "depends_on": []}
		],
		"expected_evidence": ["factory plan"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	gatePath := filepath.Join(tmpDir, "gate.json")
	gateContent := `{
		"schema_version": "ao.forge.covenant-gate-result.v0.1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b2",
		"execution_enabled": false,
		"decision": {
			"schema_version": "ao.forge.covenant-decision-fixture.v0.1",
			"decision_id": "allow-local-plan",
			"target_plan_id": "forge-plan-efedbfb309b2",
			"decision": "allow",
			"explanation": "Covenant decision allowed",
			"source": "test"
		},
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate result: %v", err)
	}

	// Mock control plane endpoints
	var uploadedPacket []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid-token" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		if r.Method == "POST" && r.URL.Path == "/api/v1/operator-packet/signed" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			uploadedPacket, _ = json.Marshal(body["operator_packet"])

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"schema_version": "ao2.cp-ingest-receipt.v1",
				"sha256": "fake-receipt-sha256",
				"stored_at": "2026-06-17T00:00:00Z",
				"ingested_schema_version": "ao2.operator-evidence-packet.v1"
			}`))
			return
		}

		if r.Method == "GET" && r.URL.Path == "/api/v1/operator-packet/fake-receipt-sha256" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(uploadedPacket)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	outPath := filepath.Join(tmpDir, "packet.json")
	code, _, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--control-plane", ts.URL, "--live")
	if code != 0 {
		t.Fatalf("expected code 0, got %d (stderr: %q)", code, stderr)
	}

	// Verify receipt is added to the evidence list of the final packet
	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read final packet: %v", err)
	}
	var passedPacket factoryPacket
	if err := json.Unmarshal(packetData, &passedPacket); err != nil {
		t.Fatalf("failed to parse final packet: %v", err)
	}

	hasReceipt := false
	for _, ev := range passedPacket.Evidence {
		if ev.Label == "control plane readback receipt" && ev.SchemaVersion == "ao2.cp-ingest-receipt.v1" {
			hasReceipt = true
			break
		}
	}
	if !hasReceipt {
		t.Fatalf("optional control plane upload succeeded, but receipt was not added to evidence list")
	}
}

func TestLiveRunControlPlaneOptionalWarning(t *testing.T) {
	tmpDir := t.TempDir()
	dummyBin := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyBin)

	// Setup mock plan and gate result
	planPath := filepath.Join(tmpDir, "plan.json")
	planContent := `{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b3",
		"objective": {
			"text": "test-optional-warning",
			"workspace": "test",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "not_requested",
			"explanation": "test"
		},
		"workcells": [
			{"workcell_id": "cell1", "kind": "prepare", "status": "planned", "depends_on": []}
		],
		"expected_evidence": ["factory plan"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	gatePath := filepath.Join(tmpDir, "gate.json")
	gateContent := `{
		"schema_version": "ao.forge.covenant-gate-result.v0.1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b3",
		"execution_enabled": false,
		"decision": {
			"schema_version": "ao.forge.covenant-decision-fixture.v0.1",
			"decision_id": "allow-local-plan",
			"target_plan_id": "forge-plan-efedbfb309b3",
			"decision": "allow",
			"explanation": "Covenant decision allowed",
			"source": "test"
		},
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate result: %v", err)
	}

	// 1. Missing control plane token: should print a warning but succeed
	t.Setenv("AO2_CP_API_TOKEN", "")
	t.Setenv("AO_FORGE_CP_API_TOKEN", "")
	t.Setenv("AO2_CP_AUTH_VALUE", "")

	outPath := filepath.Join(tmpDir, "packet_warn1.json")
	code, _, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--live")
	if code != 0 {
		t.Fatalf("expected code 0, got %d (stderr: %q)", code, stderr)
	}
	if !strings.Contains(stderr, "Warning: Control plane API token is missing") {
		t.Fatalf("stderr missing warning about missing token: %q", stderr)
	}

	// 2. Control plane is unavailable: should print a warning but succeed
	t.Setenv("AO2_CP_API_TOKEN", "some-token")
	outPath2 := filepath.Join(tmpDir, "packet_warn2.json")
	// Using a bad control plane URL
	code, _, stderr = runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath2, "--control-plane", "http://127.0.0.1:9999", "--live")
	if code != 0 {
		t.Fatalf("expected code 0, got %d (stderr: %q)", code, stderr)
	}
	if !strings.Contains(stderr, "Warning: Optional control plane readback failed") {
		t.Fatalf("stderr missing warning about optional control plane readback failure: %q", stderr)
	}
}

func TestWorkcellRubricValidationSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dummyBin := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyBin)

	// Write mock covenant
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	// Set env vars for mock ao2 output
	t.Setenv("AO2_MOCK_STDOUT", "BUILD SUCCESSFUL\ntests passed\ncoverage: 85.3% of statements\n")
	t.Setenv("AO2_MOCK_STDERR", "")

	briefPath := filepath.Join(tmpDir, "brief.json")
	briefContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "test rubric success",
			"workspace": "test-ws",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_workcells": [
			{
				"workcell_id": "cell1",
				"kind": "prepare",
				"depends_on": [],
				"rubric": {
					"required_patterns": ["BUILD SUCCESSFUL", "tests passed"],
					"forbidden_patterns": ["ERROR", "FATAL"],
					"min_coverage": 80.0
				}
			}
		],
		"expected_evidence": ["test"]
	}`
	if err := os.WriteFile(briefPath, []byte(briefContent), 0644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	outPacket := filepath.Join(tmpDir, "packet.json")
	code, stdout, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
	if code != 0 {
		t.Fatalf("expected once to succeed with exit code 0, got %d (stdout: %q, stderr: %q)", code, stdout, stderr)
	}

	data, err := os.ReadFile(outPacket)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(data, &packet); err != nil {
		t.Fatalf("failed to parse packet: %v", err)
	}

	if packet.Status != "passed" {
		t.Fatalf("expected packet status to be passed, got %q", packet.Status)
	}
	if len(packet.Workcells) != 1 {
		t.Fatalf("expected 1 workcell, got %d", len(packet.Workcells))
	}
	if packet.Workcells[0].Status != "passed" {
		t.Fatalf("expected workcell status to be passed, got %q (summary: %q)", packet.Workcells[0].Status, packet.Workcells[0].Summary)
	}
}

func TestWorkcellRubricValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dummyBin := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyBin)

	// Write mock covenant
	dummyCovSrc := filepath.Join(tmpDir, "dummy_covenant.go")
	dummyCovBin := filepath.Join(tmpDir, "dummy_covenant")
	if os.PathSeparator == '\\' {
		dummyCovBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyCovSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy covenant src: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", dummyCovBin, dummyCovSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy covenant: %v (output: %q)", err, string(out))
	}

	briefPath := filepath.Join(tmpDir, "brief.json")
	briefContent := `{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "test rubric failure",
			"workspace": "test-ws",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_workcells": [
			{
				"workcell_id": "cell1",
				"kind": "prepare",
				"depends_on": [],
				"rubric": {
					"required_patterns": ["BUILD SUCCESSFUL"],
					"forbidden_patterns": ["ERROR"],
					"min_coverage": 80.0
				}
			}
		],
		"expected_evidence": ["test"]
	}`
	if err := os.WriteFile(briefPath, []byte(briefContent), 0644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	t.Run("missing required pattern", func(t *testing.T) {
		t.Setenv("AO2_MOCK_STDOUT", "BUILD FAILED\ntests passed\ncoverage: 85.3% of statements\n")
		t.Setenv("AO2_MOCK_STDERR", "")

		outPacket := filepath.Join(tmpDir, "packet_fail1.json")
		code, _, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
		if code != 1 {
			t.Fatalf("expected once to fail with exit code 1, got %d (stderr: %q)", code, stderr)
		}

		data, err := os.ReadFile(outPacket)
		if err != nil {
			t.Fatalf("failed to read packet: %v", err)
		}
		var packet factoryPacket
		if err := json.Unmarshal(data, &packet); err != nil {
			t.Fatalf("failed to parse packet: %v", err)
		}

		if packet.Status != "failed" {
			t.Fatalf("expected packet status to be failed, got %q", packet.Status)
		}
		if packet.Workcells[0].Status != "failed" {
			t.Fatalf("expected workcell status to be failed, got %q", packet.Workcells[0].Status)
		}
		if !strings.Contains(packet.Workcells[0].Summary, `required pattern "BUILD SUCCESSFUL" not found`) {
			t.Fatalf("unexpected failure summary: %q", packet.Workcells[0].Summary)
		}
	})

	t.Run("forbidden pattern present", func(t *testing.T) {
		t.Setenv("AO2_MOCK_STDOUT", "BUILD SUCCESSFUL\ntests passed\nERROR: compile failed\ncoverage: 85.3% of statements\n")
		t.Setenv("AO2_MOCK_STDERR", "")

		outPacket := filepath.Join(tmpDir, "packet_fail2.json")
		code, _, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
		if code != 1 {
			t.Fatalf("expected once to fail with exit code 1, got %d (stderr: %q)", code, stderr)
		}

		data, err := os.ReadFile(outPacket)
		if err != nil {
			t.Fatalf("failed to read packet: %v", err)
		}
		var packet factoryPacket
		if err := json.Unmarshal(data, &packet); err != nil {
			t.Fatalf("failed to parse packet: %v", err)
		}

		if packet.Status != "failed" {
			t.Fatalf("expected packet status to be failed, got %q", packet.Status)
		}
		if packet.Workcells[0].Status != "failed" {
			t.Fatalf("expected workcell status to be failed, got %q", packet.Workcells[0].Status)
		}
		if !strings.Contains(packet.Workcells[0].Summary, `forbidden pattern "ERROR" found`) {
			t.Fatalf("unexpected failure summary: %q", packet.Workcells[0].Summary)
		}
	})

	t.Run("low coverage", func(t *testing.T) {
		t.Setenv("AO2_MOCK_STDOUT", "BUILD SUCCESSFUL\ntests passed\ncoverage: 75.3% of statements\n")
		t.Setenv("AO2_MOCK_STDERR", "")

		outPacket := filepath.Join(tmpDir, "packet_fail3.json")
		code, _, stderr := runCLI("once", "--brief", briefPath, "--covenant", dummyCovBin, "--out", outPacket)
		if code != 1 {
			t.Fatalf("expected once to fail with exit code 1, got %d (stderr: %q)", code, stderr)
		}

		data, err := os.ReadFile(outPacket)
		if err != nil {
			t.Fatalf("failed to read packet: %v", err)
		}
		var packet factoryPacket
		if err := json.Unmarshal(data, &packet); err != nil {
			t.Fatalf("failed to parse packet: %v", err)
		}

		if packet.Status != "failed" {
			t.Fatalf("expected packet status to be failed, got %q", packet.Status)
		}
		if packet.Workcells[0].Status != "failed" {
			t.Fatalf("expected workcell status to be failed, got %q", packet.Workcells[0].Status)
		}
		if !strings.Contains(packet.Workcells[0].Summary, `coverage 75.3% is below minimum 80.0%`) {
			t.Fatalf("unexpected failure summary: %q", packet.Workcells[0].Summary)
		}
	})
}

func compileTestAo2(t *testing.T, tmpDir string, tracePath string) string {
	t.Helper()
	dummySrc := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyBin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}
	srcContent := fmt.Sprintf(`package main
import (
	"encoding/json"
	"fmt"
	"os"
)

type runTarget struct {
	RepoPath string "json:\"repo_path\""
}

type runSpecDetails struct {
	Target runTarget "json:\"target\""
	Tasks []struct {
		ID string "json:\"id\""
	} "json:\"tasks\""
}

type ao2RunSpec struct {
	Spec runSpecDetails "json:\"spec\""
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		var specPath string
		for i, arg := range os.Args {
			if arg == "--spec" && i+1 < len(os.Args) {
				specPath = os.Args[i+1]
			}
		}
		if specPath != "" {
			data, err := os.ReadFile(specPath)
			if err == nil {
				var spec ao2RunSpec
				if err := json.Unmarshal(data, &spec); err == nil && len(spec.Spec.Tasks) > 0 {
					taskID := spec.Spec.Tasks[0].ID
					repoPath := spec.Spec.Target.RepoPath
					f, err := os.OpenFile(%q, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if err == nil {
						fmt.Fprintf(f, "%%s:%%s\n", taskID, repoPath)
						f.Close()
					}
				}
			}
		}

		hasDryRun := false
		for _, arg := range os.Args {
			if arg == "--dry-run" {
				hasDryRun = true
			}
		}
		if hasDryRun {
			fmt.Println("status=dry_run_accepted")
		} else {
			fmt.Println("status=governed_run_started")
		}
		fmt.Println("schema_version=ao2.run/v1")
		fmt.Println("plan_id=forge-plan-efedbfb309b1")
		fmt.Println("task_count=1")
		fmt.Println("target_repo=fixtures/discount-service")
		fmt.Println("control_plane_role=read_only_observer")
		fmt.Println("mutates_ao_artifacts=false")
		fmt.Println("factory_v3_drives_workflow=false")
		fmt.Println("spec_sha256=abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
		os.Exit(0)
	}
	os.Exit(1)
}`, tracePath)
	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy ao2 src: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}
	return dummyBin
}

func TestForgeResumeSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Set up working directory inside tmpDir to avoid polluting local files
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	// 2. Compile dummy ao2 binary with trace logging
	traceFile := filepath.Join(tmpDir, "trace.log")
	dummyBin := compileTestAo2(t, tmpDir, traceFile)
	t.Setenv("AO2_PATH", dummyBin)

	// 3. Initialize .forge directory structure
	if code, _, stderr := runCLI("init"); code != 0 {
		t.Fatalf("forge init failed with code %d: %s", code, stderr)
	}

	runID := "forge-plan-efedbfb309b1"
	runDir := filepath.Join(".forge", "runs", runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatalf("mkdir runs/%s: %v", runID, err)
	}

	// 4. Write mock files simulating a failed run where wc1 passed and wc2 failed
	planPath := filepath.Join(runDir, "plan.json")
	planContent := `{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {
			"text": "test resume",
			"workspace": "test-ws",
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "allowed",
			"explanation": "gate is allowed"
		},
		"workcells": [
			{
				"workcell_id": "wc1",
				"kind": "prepare",
				"status": "planned",
				"depends_on": []
			},
			{
				"workcell_id": "wc2",
				"kind": "execute",
				"status": "planned",
				"depends_on": ["wc1"]
			}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	gatePath := filepath.Join(runDir, "gate_result.json")
	gateContent := `{
		"schema_version": "covenant.version-result.v1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b1",
		"execution_enabled": true,
		"decision": {
			"schema_version": "covenant.decision.v1",
			"decision_id": "test-decision",
			"target_plan_id": "forge-plan-efedbfb309b1",
			"decision": "allow",
			"explanation": "test allowed",
			"source": "test-covenant"
		}
	}`
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate result: %v", err)
	}

	packetPath := filepath.Join(runDir, "factory-packet.json")
	packetContent := `{
		"schema_version": "ao.forge.factory-packet.v0.1",
		"status": "failed",
		"objective": {
			"text": "test resume",
			"workspace": "test-ws",
			"release_mode": false
		},
		"factory_plan": {
			"plan_id": "forge-plan-efedbfb309b1",
			"workcell_count": 2
		},
		"workcells": [
			{
				"workcell_id": "wc1",
				"kind": "prepare",
				"status": "passed",
				"depends_on": [],
				"ao2_run": "dry-run",
				"summary": "Dry-run accepted by ao2"
			},
			{
				"workcell_id": "wc2",
				"kind": "execute",
				"status": "failed",
				"depends_on": ["wc1"],
				"ao2_run": "none",
				"summary": "some error"
			}
		]
	}`
	if err := os.WriteFile(packetPath, []byte(packetContent), 0644); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	wc1EvidencePath := filepath.Join(runDir, "ao2-wc-wc1-evidence.json")
	wc1EvidenceContent := `{
		"schema_version": "ao2.workcell-evidence.v1",
		"workcell_id": "wc1",
		"status": "passed",
		"stdout": "wc1 stdout",
		"stderr": "wc1 stderr",
		"spec_sha256": "wc1_spec_sha"
	}`
	if err := os.WriteFile(wc1EvidencePath, []byte(wc1EvidenceContent), 0644); err != nil {
		t.Fatalf("write wc1 evidence: %v", err)
	}

	// 5. Run resume command
	outPath := filepath.Join(tmpDir, "final-packet.json")
	code, stdout, stderr := runCLI("resume", "--run", runID, "--out", outPath)
	if code != 0 {
		t.Fatalf("expected resume to succeed, got %d. stderr: %s, stdout: %s", code, stderr, stdout)
	}

	// 6. Verify only wc2 was executed
	traceData, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	executedTasks := strings.Fields(string(traceData))
	if len(executedTasks) != 1 {
		t.Fatalf("expected exactly 1 task to execute, executed: %v", executedTasks)
	}
	if !strings.HasPrefix(executedTasks[0], "wc2:") {
		t.Fatalf("expected wc2 to execute, got: %s", executedTasks[0])
	}

	// 7. Verify the final packet has both wc1 and wc2 as passed, and preserves wc1 evidence
	finalData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read final packet: %v", err)
	}
	var finalPacket factoryPacket
	if err := json.Unmarshal(finalData, &finalPacket); err != nil {
		t.Fatalf("unmarshal final packet: %v", err)
	}

	if finalPacket.Status != "passed" {
		t.Fatalf("expected final packet status to be passed, got %s", finalPacket.Status)
	}
	if len(finalPacket.Workcells) != 2 {
		t.Fatalf("expected 2 workcells in final packet, got %d", len(finalPacket.Workcells))
	}
	if finalPacket.Workcells[0].WorkcellID != "wc1" || finalPacket.Workcells[0].Status != "passed" {
		t.Fatalf("expected wc1 to be passed, got %+v", finalPacket.Workcells[0])
	}
	if finalPacket.Workcells[1].WorkcellID != "wc2" || finalPacket.Workcells[1].Status != "passed" {
		t.Fatalf("expected wc2 to be passed, got %+v", finalPacket.Workcells[1])
	}

	// Make sure evidence files are archived in the runs directory
	wc1ArchivedEv := filepath.Join(runDir, "ao2-wc-wc1-evidence.json")
	wc2ArchivedEv := filepath.Join(runDir, "ao2-wc-wc2-evidence.json")
	if _, err := os.Stat(wc1ArchivedEv); err != nil {
		t.Fatalf("expected wc1 evidence to exist in run archive: %v", err)
	}
	if _, err := os.Stat(wc2ArchivedEv); err != nil {
		t.Fatalf("expected wc2 evidence to exist in run archive: %v", err)
	}
}

func TestForgeResumeFailure(t *testing.T) {
	// 1. Missing --run option
	t.Run("missing --run option", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		defer func() {
			_ = os.Chdir(oldWd)
		}()

		// Running forge resume without --run should exit with code 2
		code, _, stderr := runCLI("resume")
		if code != 2 {
			t.Fatalf("expected exit code 2 when --run is missing, got %d. stderr: %q", code, stderr)
		}
		if !strings.Contains(stderr, "missing required --run") {
			t.Fatalf("expected error message to contain 'missing required --run', got %q", stderr)
		}
	})

	// 2. Missing .forge directory
	t.Run("missing .forge directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		defer func() {
			_ = os.Chdir(oldWd)
		}()

		// Running forge resume with --run but no .forge should exit with code 1
		code, _, stderr := runCLI("resume", "--run", "forge-plan-efedbfb309b1")
		if code != 1 {
			t.Fatalf("expected exit code 1 when .forge is missing, got %d. stderr: %q", code, stderr)
		}
		if !strings.Contains(stderr, "local state directory .forge not found") {
			t.Fatalf("expected error message to contain 'local state directory .forge not found', got %q", stderr)
		}
	})

	// 3. Missing run directory
	t.Run("missing run directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		defer func() {
			_ = os.Chdir(oldWd)
		}()

		// Create .forge but not the run directory
		if err := os.MkdirAll(filepath.Join(".forge", "runs"), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		code, _, stderr := runCLI("resume", "--run", "forge-plan-efedbfb309b1")
		if code != 1 {
			t.Fatalf("expected exit code 1 when run directory is missing, got %d. stderr: %q", code, stderr)
		}
		if !strings.Contains(stderr, `run ID "forge-plan-efedbfb309b1" not found`) {
			t.Fatalf("expected error message to contain 'run ID \"forge-plan-efedbfb309b1\" not found', got %q", stderr)
		}
	})

	// 4. Missing plan.json or gate_result.json
	t.Run("missing plan.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		defer func() {
			_ = os.Chdir(oldWd)
		}()

		runID := "forge-plan-efedbfb309b1"
		runDir := filepath.Join(".forge", "runs", runID)
		if err := os.MkdirAll(runDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Leave plan.json missing
		code, _, stderr := runCLI("resume", "--run", runID)
		if code != 1 {
			t.Fatalf("expected exit code 1 when plan.json is missing, got %d. stderr: %q", code, stderr)
		}
		if !strings.Contains(stderr, "plan.json not found in run directory") {
			t.Fatalf("expected error message to contain 'plan.json not found in run directory', got %q", stderr)
		}
	})

	t.Run("missing gate_result.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		oldWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		defer func() {
			_ = os.Chdir(oldWd)
		}()

		runID := "forge-plan-efedbfb309b1"
		runDir := filepath.Join(".forge", "runs", runID)
		if err := os.MkdirAll(runDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Write a valid plan.json but leave gate_result.json missing
		planPath := filepath.Join(runDir, "plan.json")
		planContent := `{
			"schema_version": "ao.forge.factory-plan.v0.1",
			"plan_id": "forge-plan-efedbfb309b1",
			"objective": {
				"text": "test resume failure",
				"workspace": "test-ws",
				"release_mode": false
			},
			"constraints": {
				"local_first": true,
				"allow_network": false,
				"allow_release_mutation": false,
				"require_control_plane_readback": false
			},
			"execution_enabled": false,
			"policy_gate": {
				"required": true,
				"status": "allowed",
				"explanation": "gate is allowed"
			},
			"workcells": [
				{
					"workcell_id": "wc1",
					"kind": "prepare",
					"status": "planned",
					"depends_on": []
				}
			],
			"expected_evidence": ["test"],
			"next_actions": [
				{"action_id": "test", "description": "test", "required": true}
			]
		}`
		if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
			t.Fatalf("write plan: %v", err)
		}

		code, _, stderr := runCLI("resume", "--run", runID)
		if code != 1 {
			t.Fatalf("expected exit code 1 when gate_result.json is missing, got %d. stderr: %q", code, stderr)
		}
		if !strings.Contains(stderr, "gate_result.json not found in run directory") {
			t.Fatalf("expected error message to contain 'gate_result.json not found in run directory', got %q", stderr)
		}
	})
}

func TestParseMarkdownBriefWithWorkcellWorkspaces(t *testing.T) {
	mdBrief := `# Objective
test parsing multi-workspace brief

# Workspace
default-ws

# Constraints
- Local First: true
- Allow Network: false
- Allow Release Mutation: false
- Require Control Plane Readback: false
- Release Mode: false

# Expected Workcells
- wc1 (prepare) workspace: custom-ws-1
- wc2 (execute) workspace: custom-ws-2 depends on: wc1
- wc3 (verify) depends on: wc2

# Expected Evidence
- test-evidence
`
	brief, err := parseMarkdownBrief([]byte(mdBrief))
	if err != nil {
		t.Fatalf("parseMarkdownBrief: %v", err)
	}

	if brief.Objective.Workspace != "default-ws" {
		t.Fatalf("expected default workspace 'default-ws', got %q", brief.Objective.Workspace)
	}

	if len(brief.ExpectedWorkcells) != 3 {
		t.Fatalf("expected 3 workcells, got %d", len(brief.ExpectedWorkcells))
	}

	wc1 := brief.ExpectedWorkcells[0]
	if wc1.WorkcellID != "wc1" || wc1.Kind != "prepare" || wc1.Workspace != "custom-ws-1" {
		t.Fatalf("unexpected wc1: %+v", wc1)
	}

	wc2 := brief.ExpectedWorkcells[1]
	if wc2.WorkcellID != "wc2" || wc2.Kind != "execute" || wc2.Workspace != "custom-ws-2" || len(wc2.DependsOn) != 1 || wc2.DependsOn[0] != "wc1" {
		t.Fatalf("unexpected wc2: %+v", wc2)
	}

	wc3 := brief.ExpectedWorkcells[2]
	if wc3.WorkcellID != "wc3" || wc3.Kind != "verify" || wc3.Workspace != "" || len(wc3.DependsOn) != 1 || wc3.DependsOn[0] != "wc2" {
		t.Fatalf("unexpected wc3: %+v", wc3)
	}
}

func TestGateReleaseModeMultiWorkspaceValidation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create ws1 (clean git repo)
	ws1Path := filepath.Join(tmpDir, "ws1")
	if err := os.Mkdir(ws1Path, 0755); err != nil {
		t.Fatalf("mkdir ws1: %v", err)
	}
	runGitCmd := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGitCmd(ws1Path, "init")
	runGitCmd(ws1Path, "config", "user.email", "test@example.com")
	runGitCmd(ws1Path, "config", "user.name", "Test")
	runGitCmd(ws1Path, "config", "commit.gpgSign", "false")
	if err := os.WriteFile(filepath.Join(ws1Path, "clean.txt"), []byte("clean"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGitCmd(ws1Path, "add", "clean.txt")
	runGitCmd(ws1Path, "commit", "-m", "initial commit")

	// Create ws2 (dirty git repo)
	ws2Path := filepath.Join(tmpDir, "ws2")
	if err := os.Mkdir(ws2Path, 0755); err != nil {
		t.Fatalf("mkdir ws2: %v", err)
	}
	runGitCmd(ws2Path, "init")
	runGitCmd(ws2Path, "config", "user.email", "test@example.com")
	runGitCmd(ws2Path, "config", "user.name", "Test")
	runGitCmd(ws2Path, "config", "commit.gpgSign", "false")
	if err := os.WriteFile(filepath.Join(ws2Path, "clean.txt"), []byte("clean"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGitCmd(ws2Path, "add", "clean.txt")
	runGitCmd(ws2Path, "commit", "-m", "initial commit")
	// Make it dirty
	if err := os.WriteFile(filepath.Join(ws2Path, "clean.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatalf("make dirty: %v", err)
	}

	// Compile a covenant binary
	covSrc := filepath.Join(tmpDir, "dummy_cov.go")
	covBin := filepath.Join(tmpDir, "dummy_cov")
	if os.PathSeparator == '\\' {
		covBin += ".exe"
	}
	covContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("{\"schema_version\": \"covenant.version-result.v1\", \"version\": \"v0.1.0\"}")
		os.Exit(0)
	}
	os.Exit(0)
}`
	if err := os.WriteFile(covSrc, []byte(covContent), 0644); err != nil {
		t.Fatalf("write dummy cov: %v", err)
	}
	cmdCov := exec.Command("go", "build", "-o", covBin, covSrc)
	if out, err := cmdCov.CombinedOutput(); err != nil {
		t.Fatalf("build dummy cov: %v (output: %q)", err, string(out))
	}

	// Construct plan with ws1 as default workspace and ws2 as workcell workspace
	planPath := filepath.Join(tmpDir, "plan.json")
	planContent := fmt.Sprintf(`{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {
			"text": "test release safety",
			"workspace": %q,
			"release_mode": true
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "not_requested",
			"explanation": "test"
		},
		"workcells": [
			{
				"workcell_id": "wc1",
				"kind": "prepare",
				"workspace": %q,
				"status": "planned",
				"depends_on": []
			}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`, ws1Path, ws2Path)
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	// Evaluated policy should deny because ws2 is dirty
	code, stdout, stderr := runCLI("gate", "--plan", planPath, "--covenant", covBin)
	if code != 1 {
		t.Fatalf("expected code 1, got %d (stdout: %q, stderr: %q)", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "deny-dirty-release-workspace") {
		t.Fatalf("expected deny-dirty-release-workspace in output, got: %s", stdout)
	}
}

func TestMultiWorkspaceExecution(t *testing.T) {
	tmpDir := t.TempDir()

	// Compile dummy ao2 with trace logging
	traceFile := filepath.Join(tmpDir, "trace.log")
	dummyBin := compileTestAo2(t, tmpDir, traceFile)
	t.Setenv("AO2_PATH", dummyBin)

	// Set up directories for default workspace and custom workcell workspace
	defaultWS := filepath.Join(tmpDir, "default-ws")
	customWS := filepath.Join(tmpDir, "custom-ws")
	if err := os.MkdirAll(defaultWS, 0755); err != nil {
		t.Fatalf("mkdir default-ws: %v", err)
	}
	if err := os.MkdirAll(customWS, 0755); err != nil {
		t.Fatalf("mkdir custom-ws: %v", err)
	}

	// Change wd to tmpDir to support .forge init and run commands locally
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	// Init forge
	if code, _, stderr := runCLI("init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	// Write plan with wc1 using default workspace, and wc2 using custom workspace
	planContent := fmt.Sprintf(`{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {
			"text": "test multi-workspace execution",
			"workspace": %q,
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "allowed",
			"explanation": "gate is allowed"
		},
		"workcells": [
			{
				"workcell_id": "wc1",
				"kind": "prepare",
				"status": "planned",
				"depends_on": []
			},
			{
				"workcell_id": "wc2",
				"kind": "execute",
				"workspace": %q,
				"status": "planned",
				"depends_on": ["wc1"]
			}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`, defaultWS, customWS)

	planPath := filepath.Join(tmpDir, "plan.json")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	// Write allowed gate result
	gateContent := `{
		"schema_version": "covenant.version-result.v1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b1",
		"execution_enabled": true,
		"decision": {
			"schema_version": "covenant.decision.v1",
			"decision_id": "test-decision",
			"target_plan_id": "forge-plan-efedbfb309b1",
			"decision": "allow",
			"explanation": "test allowed",
			"source": "test-covenant"
		}
	}`
	gatePath := filepath.Join(tmpDir, "gate_result.json")
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate: %v", err)
	}

	outPacket := filepath.Join(tmpDir, "packet.json")
	code, stdout, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPacket)
	if code != 0 {
		t.Fatalf("expected run to succeed, got %d (stdout: %q, stderr: %q)", code, stdout, stderr)
	}

	// Verify trace file has correct RepoPath for both workcells
	traceData, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(traceData)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 workcell executions in trace log, got %d: %q", len(lines), lines)
	}

	// wc1 should use defaultWS
	if lines[0] != fmt.Sprintf("wc1:%s", defaultWS) {
		t.Fatalf("expected wc1 to execute in defaultWS %q, got: %s", defaultWS, lines[0])
	}

	// wc2 should use customWS
	if lines[1] != fmt.Sprintf("wc2:%s", customWS) {
		t.Fatalf("expected wc2 to execute in customWS %q, got: %s", customWS, lines[1])
	}

	// Verify final packet workcells have workspaces correctly recorded
	finalData, err := os.ReadFile(outPacket)
	if err != nil {
		t.Fatalf("read final packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(finalData, &packet); err != nil {
		t.Fatalf("unmarshal final packet: %v", err)
	}

	if packet.Workcells[0].Workspace != "" {
		t.Fatalf("expected wc1 workspace to be empty/omitted in packet, got %q", packet.Workcells[0].Workspace)
	}
	if packet.Workcells[1].Workspace != customWS {
		t.Fatalf("expected wc2 workspace to be %q, got %q", customWS, packet.Workcells[1].Workspace)
	}
}

func TestParseMarkdownBriefWithExecutorAndTask(t *testing.T) {
	mdBrief := `# Objective
test parsing agy-swarms brief

# Workspace
default-ws

# Constraints
- Local First: true
- Allow Network: false
- Allow Release Mutation: false
- Require Control Plane Readback: false
- Release Mode: false

# Expected Workcells
- wc1 (prepare) executor: agy-swarms workspace: custom-ws-1 task: "Refactor model schemas to v2"
- wc2 (execute) executor: ao2 workspace: custom-ws-2 depends on: wc1
- wc3 (verify) depends on: wc2

# Expected Evidence
- test-evidence
`
	brief, err := parseMarkdownBrief([]byte(mdBrief))
	if err != nil {
		t.Fatalf("parseMarkdownBrief: %v", err)
	}

	if len(brief.ExpectedWorkcells) != 3 {
		t.Fatalf("expected 3 workcells, got %d", len(brief.ExpectedWorkcells))
	}

	wc1 := brief.ExpectedWorkcells[0]
	if wc1.WorkcellID != "wc1" || wc1.Kind != "prepare" || wc1.Executor != "agy-swarms" || wc1.Workspace != "custom-ws-1" || wc1.Task != "Refactor model schemas to v2" {
		t.Fatalf("unexpected wc1: %+v", wc1)
	}

	wc2 := brief.ExpectedWorkcells[1]
	if wc2.WorkcellID != "wc2" || wc2.Kind != "execute" || wc2.Executor != "ao2" || wc2.Workspace != "custom-ws-2" || wc2.Task != "" {
		t.Fatalf("unexpected wc2: %+v", wc2)
	}

	wc3 := brief.ExpectedWorkcells[2]
	if wc3.WorkcellID != "wc3" || wc3.Kind != "verify" || wc3.Executor != "" || wc3.Workspace != "" || wc3.Task != "" {
		t.Fatalf("unexpected wc3: %+v", wc3)
	}
}

func compileTestAgySwarms(t *testing.T, tmpDir string, tracePath string) string {
	t.Helper()
	dummySrc := filepath.Join(tmpDir, "dummy_agy.go")
	dummyBin := filepath.Join(tmpDir, "dummy_agy")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}
	srcContent := fmt.Sprintf(`package main
import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	var taskPath, reportPath string
	for i, arg := range os.Args {
		if arg == "--task" && i+1 < len(os.Args) {
			taskPath = os.Args[i+1]
		}
		if arg == "--report" && i+1 < len(os.Args) {
			reportPath = os.Args[i+1]
		}
	}
	_ = taskPath

	// Log execution trace (argv) to trace file for verification
	if %q != "" {
		traceData := strings.Join(os.Args, " ") + "\n"
		f, err := os.OpenFile(%q, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			f.WriteString(traceData)
			f.Close()
		}
	}

	if reportPath != "" {
		// Mock a successful local-runner-report JSON output
		report := map[string]interface{}{
			"status": "succeeded",
			"spent_tokens": 1250,
			"spent_usd": 0.02,
			"states": map[string]string{
				"worker_0": "succeeded",
			},
			"blockers": []interface{}{},
			"concerns": []interface{}{},
			"changed_files": []string{},
			"results": map[string]interface{}{
				"worker_0": map[string]interface{}{
					"status": "succeeded",
					"error_class": "",
					"artifact": map[string]interface{}{},
					"stdout": "coverage is 85.0%%\nsuccess pattern present",
					"stderr": "",
					"exit_code": 0,
				},
			},
		}
		data, err := json.Marshal(report)
		if err == nil {
			os.WriteFile(reportPath, data, 0644)
		}
	}

	// Write mock stdout output for command success check and rubric matching
	fmt.Println("coverage is 85.0%%")
	fmt.Println("success pattern present")
	os.Exit(0)
}`, tracePath, tracePath)

	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy agy src: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy agy: %v (output: %q)", err, string(out))
	}
	return dummyBin
}

func TestAgySwarmsExecutionDryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Compile dummy agy-swarms binary and dummy ao2 binary
	traceFile := filepath.Join(tmpDir, "trace.log")
	dummyAgy := compileTestAgySwarms(t, tmpDir, traceFile)
	t.Setenv("AGY_SWARMS_PATH", dummyAgy)

	dummyAo2 := compileTestAo2(t, tmpDir, filepath.Join(tmpDir, "ao2-trace.log"))
	t.Setenv("AO2_PATH", dummyAo2)

	defaultWS := filepath.Join(tmpDir, "default-ws")
	if err := os.MkdirAll(defaultWS, 0755); err != nil {
		t.Fatalf("mkdir default-ws: %v", err)
	}

	// Change wd to tmpDir to support .forge init and run commands locally
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if code, _, stderr := runCLI("init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	// Construct plan with wc1 using agy-swarms, and wc2 using ao2
	planContent := fmt.Sprintf(`{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {
			"text": "test agy-swarms execution",
			"workspace": %q,
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "allowed",
			"explanation": "gate allowed"
		},
		"workcells": [
			{
				"workcell_id": "wc1",
				"kind": "prepare",
				"executor": "agy-swarms",
				"task": "Perform swarm tasks",
				"status": "planned",
				"depends_on": [],
				"rubric": {
					"required_patterns": ["success pattern present"],
					"min_coverage": 80.0
				}
			},
			{
				"workcell_id": "wc2",
				"kind": "execute",
				"status": "planned",
				"depends_on": ["wc1"]
			}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`, defaultWS)

	planPath := filepath.Join(tmpDir, "plan.json")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	gateContent := `{
		"schema_version": "covenant.version-result.v1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b1",
		"execution_enabled": true,
		"decision": {
			"schema_version": "covenant.decision.v1",
			"decision_id": "test-decision",
			"target_plan_id": "forge-plan-efedbfb309b1",
			"decision": "allow",
			"explanation": "test allowed",
			"source": "test-covenant"
		}
	}`
	gatePath := filepath.Join(tmpDir, "gate_result.json")
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate: %v", err)
	}

	outPacket := filepath.Join(tmpDir, "packet.json")
	code, stdout, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPacket)
	if code != 0 {
		t.Fatalf("expected run to succeed, got %d (stdout: %q, stderr: %q)", code, stdout, stderr)
	}

	// Verify trace file has correct commands for agy-swarms (wc1)
	traceData, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	traceStr := strings.TrimSpace(string(traceData))
	if !strings.Contains(traceStr, "--dry-run") {
		t.Fatalf("expected dry-run args in agy-swarms call, got: %q", traceStr)
	}
	if !strings.Contains(traceStr, "--allow-local-commands") {
		t.Fatalf("expected --allow-local-commands in agy-swarms call, got: %q", traceStr)
	}

	// Verify final packet workcells have correct status, executor, and summary
	finalData, err := os.ReadFile(outPacket)
	if err != nil {
		t.Fatalf("read final packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(finalData, &packet); err != nil {
		t.Fatalf("unmarshal final packet: %v", err)
	}

	if packet.Workcells[0].Executor != "agy-swarms" {
		t.Fatalf("expected wc1 executor to be agy-swarms, got %q", packet.Workcells[0].Executor)
	}
	if packet.Workcells[0].Task != "Perform swarm tasks" {
		t.Fatalf("expected wc1 task to be 'Perform swarm tasks', got %q", packet.Workcells[0].Task)
	}
	if !strings.Contains(packet.Workcells[0].Summary, "Swarm execution succeeded (Tokens: 1250, Cost: $0.02)") {
		t.Fatalf("unexpected summary for wc1: %q", packet.Workcells[0].Summary)
	}

	// Verify that agy-swarms report file was written in workspace
	reportPath := filepath.Join(tmpDir, "agy-swarms-report-wc1.json")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("expected agy-swarms report file to exist, but got err: %v", err)
	}
}

func TestAgySwarmsExecutionLive(t *testing.T) {
	tmpDir := t.TempDir()

	// Compile dummy agy-swarms binary and dummy ao2 binary
	traceFile := filepath.Join(tmpDir, "trace.log")
	dummyAgy := compileTestAgySwarms(t, tmpDir, traceFile)
	t.Setenv("AGY_SWARMS_PATH", dummyAgy)

	dummyAo2 := compileTestAo2(t, tmpDir, filepath.Join(tmpDir, "ao2-trace.log"))
	t.Setenv("AO2_PATH", dummyAo2)

	defaultWS := filepath.Join(tmpDir, "default-ws")
	if err := os.MkdirAll(defaultWS, 0755); err != nil {
		t.Fatalf("mkdir default-ws: %v", err)
	}

	// Change wd to tmpDir to support .forge init and run commands locally
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if code, _, stderr := runCLI("init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	// Construct plan with wc1 using agy-swarms
	planContent := fmt.Sprintf(`{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-efedbfb309b1",
		"objective": {
			"text": "test agy-swarms live execution",
			"workspace": %q,
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "allowed",
			"explanation": "gate allowed"
		},
		"workcells": [
			{
				"workcell_id": "wc1",
				"kind": "prepare",
				"executor": "agy-swarms",
				"status": "planned",
				"depends_on": []
			}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`, defaultWS)

	planPath := filepath.Join(tmpDir, "plan.json")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	gateContent := `{
		"schema_version": "covenant.version-result.v1",
		"status": "allowed",
		"plan_id": "forge-plan-efedbfb309b1",
		"execution_enabled": true,
		"decision": {
			"schema_version": "covenant.decision.v1",
			"decision_id": "test-decision",
			"target_plan_id": "forge-plan-efedbfb309b1",
			"decision": "allow",
			"explanation": "test allowed",
			"source": "test-covenant"
		}
	}`
	gatePath := filepath.Join(tmpDir, "gate_result.json")
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate: %v", err)
	}

	outPacket := filepath.Join(tmpDir, "packet.json")
	code, stdout, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPacket, "--live")
	if code != 0 {
		t.Fatalf("expected run to succeed, got %d (stdout: %q, stderr: %q)", code, stdout, stderr)
	}

	// Verify trace file shows that agy-swarms was run with live args
	traceData, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	traceStr := strings.TrimSpace(string(traceData))
	if strings.Contains(traceStr, "--dry-run") {
		t.Fatalf("did not expect dry-run args in live agy-swarms call, got: %q", traceStr)
	}
	if !strings.Contains(traceStr, "--reviewer agy") || !strings.Contains(traceStr, "--closer agy") {
		t.Fatalf("expected live reviewer/closer args in live agy-swarms call, got: %q", traceStr)
	}
}

func TestInteractiveGateOverrideApprove(t *testing.T) {
	tmpDir := t.TempDir()

	plan := factoryPlan{
		PlanID: "override-plan-id",
		Objective: factoryObjective{
			Text:      "test override",
			Workspace: tmpDir,
		},
		Constraints: factoryConstraints{
			LocalFirst: true,
		},
		Workcells: []planWorkcell{
			{
				WorkcellID: "wc1",
				Kind:       "prepare",
				Status:     "planned",
			},
		},
	}

	planPath := filepath.Join(tmpDir, "plan.json")
	planData, _ := json.Marshal(plan)
	_ = os.WriteFile(planPath, planData, 0644)

	gate := covenantGateResult{
		SchemaVersion: "covenant.gate-result.v0.1",
		Status:        "blocked", // representing indeterminate
		PlanID:        "override-plan-id",
		Decision: covenantDecisionFixture{
			DecisionID:  "indeterminate-network-access",
			Decision:    "indeterminate",
			Explanation: "Plan requires network access, requiring override",
		},
	}
	gatePath := filepath.Join(tmpDir, "gate.json")
	gateData, _ := json.Marshal(gate)
	_ = os.WriteFile(gatePath, gateData, 0644)

	dummyAo2 := compileTestAo2(t, tmpDir, filepath.Join(tmpDir, "ao2-trace.log"))
	t.Setenv("AO2_PATH", dummyAo2)

	outPath := filepath.Join(tmpDir, "packet.json")

	stdinMock := strings.NewReader("y\n")
	var stdoutBuf, stderrBuf bytes.Buffer

	code := executePlanRun(plan, planPath, gatePath, outPath, "", "", false, false, false, true, stdinMock, nil, &stdoutBuf, &stderrBuf)
	if code != 0 {
		t.Fatalf("expected executePlanRun to succeed (exit code 0), got %d. stderr: %s", code, stderrBuf.String())
	}

	// Verify operator override evidence is in the packet
	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(packetData, &packet); err != nil {
		t.Fatalf("failed to unmarshal packet: %v", err)
	}

	if packet.Status != "passed" {
		t.Fatalf("expected packet status to be passed, got %q", packet.Status)
	}

	foundOverride := false
	for _, ev := range packet.Evidence {
		if ev.Label == "operator override approval evidence" {
			foundOverride = true
			if ev.SchemaVersion != "ao2.operator-override-evidence.v1" {
				t.Errorf("unexpected schema version: %q", ev.SchemaVersion)
			}
			// Verify override JSON file exists and contains correct content
			overrideFilePath := filepath.Join(tmpDir, "operator-override.json")
			overrideData, err := os.ReadFile(overrideFilePath)
			if err != nil {
				t.Fatalf("override file not found: %v", err)
			}
			var overrideObj map[string]any
			if err := json.Unmarshal(overrideData, &overrideObj); err != nil {
				t.Fatalf("failed to unmarshal override JSON: %v", err)
			}
			if overrideObj["approved"] != true {
				t.Errorf("expected approved to be true, got %v", overrideObj["approved"])
			}
			if overrideObj["gate_decision_id"] != "indeterminate-network-access" {
				t.Errorf("expected decision ID to be 'indeterminate-network-access', got %v", overrideObj["gate_decision_id"])
			}
		}
	}
	if !foundOverride {
		t.Fatal("expected operator override evidence to be present in packet")
	}
}

func TestInteractiveGateOverrideDeny(t *testing.T) {
	tmpDir := t.TempDir()

	plan := factoryPlan{
		PlanID: "override-plan-id",
		Objective: factoryObjective{
			Text:      "test override deny",
			Workspace: tmpDir,
		},
		Constraints: factoryConstraints{
			LocalFirst: true,
		},
		Workcells: []planWorkcell{
			{
				WorkcellID: "wc1",
				Kind:       "prepare",
				Status:     "planned",
			},
		},
	}
	planPath := filepath.Join(tmpDir, "plan.json")
	planData, _ := json.Marshal(plan)
	_ = os.WriteFile(planPath, planData, 0644)

	gate := covenantGateResult{
		SchemaVersion: "covenant.gate-result.v0.1",
		Status:        "blocked",
		PlanID:        "override-plan-id",
		Decision: covenantDecisionFixture{
			DecisionID:  "indeterminate-network-access",
			Decision:    "indeterminate",
			Explanation: "Plan requires network access, requiring override",
		},
	}
	gatePath := filepath.Join(tmpDir, "gate.json")
	gateData, _ := json.Marshal(gate)
	_ = os.WriteFile(gatePath, gateData, 0644)

	outPath := filepath.Join(tmpDir, "packet.json")

	stdinMock := strings.NewReader("n\n")
	var stdoutBuf, stderrBuf bytes.Buffer

	code := executePlanRun(plan, planPath, gatePath, outPath, "", "", false, false, false, true, stdinMock, nil, &stdoutBuf, &stderrBuf)
	if code != 1 {
		t.Fatalf("expected executePlanRun to fail with exit code 1, got %d", code)
	}

	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(packetData, &packet); err != nil {
		t.Fatalf("failed to unmarshal packet: %v", err)
	}
	if packet.Status != "blocked" {
		t.Fatalf("expected packet status to be blocked, got %q", packet.Status)
	}
}

func compileStatefulTestAo2(t *testing.T, tmpDir string, stateFile string) string {
	t.Helper()
	dummySrc := filepath.Join(tmpDir, "dummy_ao2_stateful.go")
	dummyBin := filepath.Join(tmpDir, "dummy_ao2_stateful")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}
	srcContent := fmt.Sprintf(`package main
import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		count := 0
		if data, err := os.ReadFile(%q); err == nil {
			fmt.Sscanf(string(data), "%%d", &count)
		}
		count++
		_ = os.WriteFile(%q, []byte(fmt.Sprintf("%%d", count)), 0644)

		if count == 1 {
			fmt.Fprintln(os.Stderr, "Execution failed first attempt")
			os.Exit(1)
		}

		fmt.Println("status=dry_run_accepted")
		fmt.Println("schema_version=ao2.run/v1")
		fmt.Println("plan_id=forge-plan-efedbfb309b1")
		fmt.Println("task_count=1")
		fmt.Println("target_repo=fixtures/discount-service")
		fmt.Println("control_plane_role=read_only_observer")
		fmt.Println("mutates_ao_artifacts=false")
		fmt.Println("factory_v3_drives_workflow=false")
		fmt.Println("spec_sha256=abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
		os.Exit(0)
	}
	os.Exit(1)
}`, stateFile, stateFile)
	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy src: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy bin: %v (output: %q)", err, string(out))
	}
	return dummyBin
}

func TestInteractiveWorkcellFailureRetry(t *testing.T) {
	tmpDir := t.TempDir()

	plan := factoryPlan{
		PlanID: "retry-plan-id",
		Objective: factoryObjective{
			Text:      "test retry flow",
			Workspace: tmpDir,
		},
		Constraints: factoryConstraints{
			LocalFirst: true,
		},
		Workcells: []planWorkcell{
			{
				WorkcellID: "wc1",
				Kind:       "prepare",
				Status:     "planned",
			},
		},
	}
	planPath := filepath.Join(tmpDir, "plan.json")
	planData, _ := json.Marshal(plan)
	_ = os.WriteFile(planPath, planData, 0644)

	gate := covenantGateResult{
		SchemaVersion: "covenant.gate-result.v0.1",
		Status:        "allowed",
		PlanID:        "retry-plan-id",
		Decision: covenantDecisionFixture{
			DecisionID:  "allow-local",
			Decision:    "allow",
			Explanation: "local allowed",
		},
	}
	gatePath := filepath.Join(tmpDir, "gate.json")
	gateData, _ := json.Marshal(gate)
	_ = os.WriteFile(gatePath, gateData, 0644)

	stateFile := filepath.Join(tmpDir, "wc1-run.count")
	dummyAo2 := compileStatefulTestAo2(t, tmpDir, stateFile)
	t.Setenv("AO2_PATH", dummyAo2)

	outPath := filepath.Join(tmpDir, "packet.json")

	stdinMock := strings.NewReader("r\n")
	var stdoutBuf, stderrBuf bytes.Buffer

	code := executePlanRun(plan, planPath, gatePath, outPath, "", "", false, false, false, true, stdinMock, nil, &stdoutBuf, &stderrBuf)
	if code != 0 {
		t.Fatalf("expected executePlanRun to succeed (exit code 0), got %d. stderr: %s", code, stderrBuf.String())
	}

	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(packetData, &packet); err != nil {
		t.Fatalf("failed to unmarshal packet: %v", err)
	}
	if packet.Status != "passed" {
		t.Fatalf("expected packet status to be passed, got %q", packet.Status)
	}
	if packet.Workcells[0].Status != "passed" {
		t.Fatalf("expected wc1 status to be passed, got %q", packet.Workcells[0].Status)
	}
}

func compileSelectiveTestAo2(t *testing.T, tmpDir string) string {
	t.Helper()
	dummySrc := filepath.Join(tmpDir, "dummy_ao2_selective.go")
	dummyBin := filepath.Join(tmpDir, "dummy_ao2_selective")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}
	srcContent := `package main
import (
	"encoding/json"
	"fmt"
	"os"
)

type runTarget struct {
	RepoPath string "json:\"repo_path\""
}

type runSpecDetails struct {
	Target runTarget "json:\"target\""
	Tasks []struct {
		ID string "json:\"id\""
	} "json:\"tasks\""
}

type ao2RunSpec struct {
	Spec runSpecDetails "json:\"spec\""
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		var specPath string
		for i, arg := range os.Args {
			if arg == "--spec" && i+1 < len(os.Args) {
				specPath = os.Args[i+1]
			}
		}
		if specPath != "" {
			data, err := os.ReadFile(specPath)
			if err == nil {
				var spec ao2RunSpec
				if err := json.Unmarshal(data, &spec); err == nil && len(spec.Spec.Tasks) > 0 {
					taskID := spec.Spec.Tasks[0].ID
					if taskID == "wc1" {
						fmt.Fprintln(os.Stderr, "wc1 failed selectively")
						os.Exit(1)
					}
				}
			}
		}
		fmt.Println("status=dry_run_accepted")
		fmt.Println("schema_version=ao2.run/v1")
		fmt.Println("plan_id=forge-plan-efedbfb309b1")
		fmt.Println("task_count=1")
		fmt.Println("target_repo=fixtures/discount-service")
		fmt.Println("control_plane_role=read_only_observer")
		fmt.Println("mutates_ao_artifacts=false")
		fmt.Println("factory_v3_drives_workflow=false")
		fmt.Println("spec_sha256=abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy src: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy bin: %v (output: %q)", err, string(out))
	}
	return dummyBin
}

func TestInteractiveWorkcellFailureSkip(t *testing.T) {
	tmpDir := t.TempDir()

	plan := factoryPlan{
		PlanID: "skip-plan-id",
		Objective: factoryObjective{
			Text:      "test skip flow",
			Workspace: tmpDir,
		},
		Constraints: factoryConstraints{
			LocalFirst: true,
		},
		Workcells: []planWorkcell{
			{
				WorkcellID: "wc1",
				Kind:       "prepare",
				Status:     "planned",
			},
			{
				WorkcellID: "wc2",
				Kind:       "prepare",
				Status:     "planned",
			},
		},
	}
	planPath := filepath.Join(tmpDir, "plan.json")
	planData, _ := json.Marshal(plan)
	_ = os.WriteFile(planPath, planData, 0644)

	gate := covenantGateResult{
		SchemaVersion: "covenant.gate-result.v0.1",
		Status:        "allowed",
		PlanID:        "skip-plan-id",
		Decision: covenantDecisionFixture{
			DecisionID:  "allow-local",
			Decision:    "allow",
			Explanation: "local allowed",
		},
	}
	gatePath := filepath.Join(tmpDir, "gate.json")
	gateData, _ := json.Marshal(gate)
	_ = os.WriteFile(gatePath, gateData, 0644)

	dummyAo2 := compileSelectiveTestAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyAo2)

	outPath := filepath.Join(tmpDir, "packet.json")

	stdinMock := strings.NewReader("s\n")
	var stdoutBuf, stderrBuf bytes.Buffer

	code := executePlanRun(plan, planPath, gatePath, outPath, "", "", false, false, false, true, stdinMock, nil, &stdoutBuf, &stderrBuf)
	if code != 0 {
		t.Fatalf("expected executePlanRun to succeed (exit code 0) after skip, got %d. stderr: %s", code, stderrBuf.String())
	}

	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(packetData, &packet); err != nil {
		t.Fatalf("failed to unmarshal packet: %v", err)
	}

	if packet.Status != "passed" {
		t.Fatalf("expected packet status to be passed, got %q", packet.Status)
	}

	var wc1Status, wc2Status string
	for _, wc := range packet.Workcells {
		if wc.WorkcellID == "wc1" {
			wc1Status = wc.Status
		}
		if wc.WorkcellID == "wc2" {
			wc2Status = wc.Status
		}
	}
	if wc1Status != "skipped" {
		t.Errorf("expected wc1 status to be 'skipped', got %q", wc1Status)
	}
	if wc2Status != "passed" {
		t.Errorf("expected wc2 status to be 'passed', got %q", wc2Status)
	}
}

func TestInteractiveWorkcellFailureAbort(t *testing.T) {
	tmpDir := t.TempDir()

	plan := factoryPlan{
		PlanID: "abort-plan-id",
		Objective: factoryObjective{
			Text:      "test abort flow",
			Workspace: tmpDir,
		},
		Constraints: factoryConstraints{
			LocalFirst: true,
		},
		Workcells: []planWorkcell{
			{
				WorkcellID: "wc1",
				Kind:       "prepare",
				Status:     "planned",
			},
		},
	}
	planPath := filepath.Join(tmpDir, "plan.json")
	planData, _ := json.Marshal(plan)
	_ = os.WriteFile(planPath, planData, 0644)

	gate := covenantGateResult{
		SchemaVersion: "covenant.gate-result.v0.1",
		Status:        "allowed",
		PlanID:        "abort-plan-id",
		Decision: covenantDecisionFixture{
			DecisionID:  "allow-local",
			Decision:    "allow",
			Explanation: "local allowed",
		},
	}
	gatePath := filepath.Join(tmpDir, "gate.json")
	gateData, _ := json.Marshal(gate)
	_ = os.WriteFile(gatePath, gateData, 0644)

	dummyAo2 := compileSelectiveTestAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyAo2)

	outPath := filepath.Join(tmpDir, "packet.json")

	stdinMock := strings.NewReader("a\n")
	var stdoutBuf, stderrBuf bytes.Buffer

	code := executePlanRun(plan, planPath, gatePath, outPath, "", "", false, false, false, true, stdinMock, nil, &stdoutBuf, &stderrBuf)
	if code != 1 {
		t.Fatalf("expected executePlanRun to fail (exit code 1), got %d. stderr: %s", code, stderrBuf.String())
	}

	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read packet: %v", err)
	}
	var packet factoryPacket
	if err := json.Unmarshal(packetData, &packet); err != nil {
		t.Fatalf("failed to unmarshal packet: %v", err)
	}

	if packet.Status != "failed" {
		t.Fatalf("expected packet status to be failed, got %q", packet.Status)
	}
	if packet.Workcells[0].Status != "failed" {
		t.Fatalf("expected wc1 status to be failed, got %q", packet.Workcells[0].Status)
	}
}

func TestTerminalDetectionMock(t *testing.T) {
	// Simple sanity test for terminal check
	isTerm := isTerminal(os.Stdout.Fd())
	t.Logf("isTerminal(stdout) = %t", isTerm)
	width, height, err := getTerminalSize(os.Stdout.Fd())
	t.Logf("getTerminalSize(stdout) = %d x %d (err: %v)", width, height, err)
}

func TestDashboardDisabledInNonInteractive(t *testing.T) {
	tmpDir := t.TempDir()
	plan := factoryPlan{
		PlanID: "dash-dis-plan",
		Objective: factoryObjective{
			Text:      "test dashboard bypass",
			Workspace: tmpDir,
		},
		Workcells: []planWorkcell{
			{
				WorkcellID: "wc1",
				Kind:       "prepare",
				Status:     "planned",
			},
		},
	}
	planPath := filepath.Join(tmpDir, "plan.json")
	planData, _ := json.Marshal(plan)
	_ = os.WriteFile(planPath, planData, 0644)
	gatePath := filepath.Join(tmpDir, "gate.json")
	gate := covenantGateResult{
		Status: "allowed",
		PlanID: "dash-dis-plan",
	}
	gateData, _ := json.Marshal(gate)
	_ = os.WriteFile(gatePath, gateData, 0644)

	dummyAo2 := compileDummyAo2(t, tmpDir)
	t.Setenv("AO2_PATH", dummyAo2)

	outPath := filepath.Join(tmpDir, "packet.json")
	var stdoutBuf, stderrBuf bytes.Buffer

	// executePlanRun with noDashboard = true
	code := executePlanRun(plan, planPath, gatePath, outPath, "", "", false, false, false, true, strings.NewReader(""), nil, &stdoutBuf, &stderrBuf)
	if code != 0 {
		t.Fatalf("run failed: %d", code)
	}

	if strings.Contains(stderrBuf.String(), "\033[?1049h") {
		t.Fatal("TUI alternate screen characters should not be emitted when noDashboard is true")
	}
}

func TestDashboardLogStreaming(t *testing.T) {
	state := &workcellRunState{
		stateMu: &sync.Mutex{},
		ID:      "wc1",
		Status:  "pending",
	}
	state.AppendStdout("line 1\n")
	if state.GetStdout() != "line 1\n" {
		t.Fatalf("expected stdout 'line 1\n', got %q", state.GetStdout())
	}
	state.AppendStderr("error 1\n")
	if state.GetStderr() != "error 1\n" {
		t.Fatalf("expected stderr 'error 1\n', got %q", state.GetStderr())
	}
}

func TestDashboardRenderingUnits(t *testing.T) {
	plan := factoryPlan{
		PlanID:    "tui-plan-id",
		Objective: factoryObjective{Text: "Test rendering of TUI dashboard"},
		Workcells: []planWorkcell{
			{WorkcellID: "wc1", Kind: "prepare"},
			{WorkcellID: "wc2", Kind: "execute"},
		},
	}
	states := map[string]*workcellRunState{
		"wc1": {stateMu: &sync.Mutex{}, ID: "wc1", Kind: "prepare", Status: "passed", Summary: "Succeeded cleanly"},
		"wc2": {stateMu: &sync.Mutex{}, ID: "wc2", Kind: "execute", Status: "running", Stdout: "Task progress: 50%\n"},
	}
	var buf bytes.Buffer
	d := &dashboard{
		plan:      plan,
		states:    states,
		mu:        &sync.Mutex{},
		startTime: time.Now().Add(-10 * time.Second),
		writer:    &buf,
	}

	// We pass a dummy FD value of 0 since getTerminalSize won't succeed on it and will fallback to 80 width
	d.render(0)

	rendered := buf.String()
	if !strings.Contains(rendered, "[AO Forge Factory Dashboard]") {
		t.Fatal("Header tag not found in TUI render output")
	}
	if !strings.Contains(rendered, "tui-plan-id") {
		t.Fatal("Plan ID not found in TUI render output")
	}
	if !strings.Contains(rendered, "Test rendering of TUI dashboard") {
		t.Fatal("Objective not found in TUI render output")
	}
	if !strings.Contains(rendered, "wc1 (prepare) -> \033[32mPASSED\033[0m") {
		t.Fatal("wc1 PASSED status line not formatted correctly")
	}
	if !strings.Contains(rendered, "wc2 (execute) -> \033[36mRUNNING\033[0m") {
		t.Fatal("wc2 RUNNING status line not formatted correctly")
	}
	if !strings.Contains(rendered, "Task progress: 50%") {
		t.Fatal("Log tail preview not printed in TUI output")
	}
}

func compileTestAgySwarmsWithPeers(t *testing.T, tmpDir string, tracePath string) string {
	t.Helper()
	dummySrc := filepath.Join(tmpDir, "dummy_agy.go")
	dummyBin := filepath.Join(tmpDir, "dummy_agy")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}
	srcContent := fmt.Sprintf(`package main
import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	var taskPath, reportPath string
	for i, arg := range os.Args {
		if arg == "--task" && i+1 < len(os.Args) {
			taskPath = os.Args[i+1]
		}
		if arg == "--report" && i+1 < len(os.Args) {
			reportPath = os.Args[i+1]
		}
	}
	_ = taskPath

	// Log execution trace
	if %q != "" {
		traceData := strings.Join(os.Args, " ") + "\n"
		f, err := os.OpenFile(%q, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			f.WriteString(traceData)
			f.Close()
		}
	}

	// Determine peer run index deterministically
	runIdx := 0
	peerIdxStr := os.Getenv("AO_FORGE_PEER_INDEX")
	if peerIdxStr != "" {
		if idx, err := strconv.Atoi(peerIdxStr); err == nil {
			runIdx = idx
		}
	}

	// Outputs based on index
	var stdoutText string
	var spentTokens float64
	var spentUSD float64

	switch runIdx {
	case 0:
		// Invalid due to forbidden pattern
		stdoutText = "coverage is 98.0%%\nsuccess pattern present\nforbidden pattern found"
		spentTokens = 1000
		spentUSD = 0.01
	case 1:
		// Winner (highest coverage among valid)
		stdoutText = "coverage is 95.0%%\nsuccess pattern present"
		spentTokens = 1200
		spentUSD = 0.02
	case 2:
		// Valid but lower coverage
		stdoutText = "coverage is 85.0%%\nsuccess pattern present"
		spentTokens = 800
		spentUSD = 0.015
	default:
		stdoutText = "coverage is 50.0%%\nsuccess pattern present"
		spentTokens = 500
		spentUSD = 0.005
	}

	if reportPath != "" {
		report := map[string]interface{}{
			"status": "succeeded",
			"spent_tokens": spentTokens,
			"spent_usd": spentUSD,
			"states": map[string]string{
				"worker_0": "succeeded",
			},
			"blockers": []interface{}{},
			"concerns": []interface{}{},
			"changed_files": []string{},
			"results": map[string]interface{}{
				"worker_0": map[string]interface{}{
					"status": "succeeded",
					"error_class": "",
					"artifact": map[string]interface{}{},
					"stdout": stdoutText,
					"stderr": "",
					"exit_code": 0,
				},
			},
		}
		data, err := json.Marshal(report)
		if err == nil {
			os.WriteFile(reportPath, data, 0644)
		}
	}

	fmt.Println(stdoutText)
	os.Exit(0)
}`, tracePath, tracePath)

	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy agy src: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy agy: %v (output: %q)", err, string(out))
	}
	return dummyBin
}

func TestParallelSwarmsPeerReview(t *testing.T) {
	tmpDir := t.TempDir()

	traceFile := filepath.Join(tmpDir, "trace.log")
	dummyAgy := compileTestAgySwarmsWithPeers(t, tmpDir, traceFile)
	t.Setenv("AGY_SWARMS_PATH", dummyAgy)

	dummyAo2 := compileTestAo2(t, tmpDir, filepath.Join(tmpDir, "ao2-trace.log"))
	t.Setenv("AO2_PATH", dummyAo2)

	defaultWS := filepath.Join(tmpDir, "default-ws")
	if err := os.MkdirAll(defaultWS, 0755); err != nil {
		t.Fatalf("mkdir default-ws: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if code, _, stderr := runCLI("init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	// Construct plan with wc1 using agy-swarms with 3 peers, and wc2 using ao2
	planContent := fmt.Sprintf(`{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-1234567890ab",
		"objective": {
			"text": "test parallel peers execution",
			"workspace": %q,
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "allowed",
			"explanation": "gate allowed"
		},
		"workcells": [
			{
				"workcell_id": "wc1",
				"kind": "prepare",
				"executor": "agy-swarms",
				"peers": 3,
				"task": "Perform peer tasks",
				"status": "planned",
				"depends_on": [],
				"rubric": {
					"required_patterns": ["success pattern present"],
					"forbidden_patterns": ["forbidden pattern found"],
					"min_coverage": 80.0
				}
			},
			{
				"workcell_id": "wc2",
				"kind": "execute",
				"status": "planned",
				"depends_on": ["wc1"]
			}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`, defaultWS)

	planPath := filepath.Join(tmpDir, "plan.json")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	gatePath := filepath.Join(tmpDir, "gate_result.json")
	gateContent := `{
		"status": "allowed",
		"plan_id": "forge-plan-1234567890ab",
		"decision": {
			"decision_id": "covenant-allow-local-safe",
			"target": "factory-plan",
			"decision": "allow",
			"explanation": "Covenant policy verification passed locally",
			"source": "covenant-gate"
		}
	}`
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate_result: %v", err)
	}

	outPath := filepath.Join(tmpDir, "packet-out.json")
	code, stdout, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--no-dashboard", "--non-interactive")
	if code != 0 {
		packetData, _ := os.ReadFile(outPath)
		t.Fatalf("run failed with code %d: %s\nStderr: %s\nPacket: %s", code, stdout, stderr, string(packetData))
	}

	// Read packet to assert chosen winner
	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	var packet map[string]interface{}
	if err := json.Unmarshal(packetData, &packet); err != nil {
		t.Fatalf("unmarshal packet: %v", err)
	}

	workcells, ok := packet["workcells"].([]interface{})
	if !ok || len(workcells) < 2 {
		t.Fatalf("unexpected workcells structure: %+v", packet["workcells"])
	}

	wc1 := workcells[0].(map[string]interface{})
	summary, _ := wc1["summary"].(string)

	// Winner must be Peer 1 (95% coverage)
	if !strings.Contains(summary, "Winner: Peer 1") {
		t.Fatalf("expected Winner: Peer 1, got summary: %q", summary)
	}

	// Verify evidence contains peer evidence files
	evidence, ok := packet["evidence"].([]interface{})
	if !ok {
		t.Fatalf("unexpected evidence structure")
	}

	foundPeer0 := false
	foundPeer1 := false
	foundPeer2 := false
	for _, ev := range evidence {
		evMap := ev.(map[string]interface{})
		label, _ := evMap["label"].(string)
		if strings.Contains(label, "wc1 peer 0 evidence") {
			foundPeer0 = true
		}
		if strings.Contains(label, "wc1 peer 1 evidence") {
			foundPeer1 = true
		}
		if strings.Contains(label, "wc1 peer 2 evidence") {
			foundPeer2 = true
		}
	}

	if !foundPeer0 || !foundPeer1 || !foundPeer2 {
		t.Fatalf("missing peer evidence files in packet evidence list: foundPeer0=%t, foundPeer1=%t, foundPeer2=%t", foundPeer0, foundPeer1, foundPeer2)
	}

	// Verify files are created in the runs archive
	runID := packet["factory_plan"].(map[string]interface{})["plan_id"].(string)
	archiveDir := filepath.Join(tmpDir, ".forge", "runs", runID)

	for idx := 0; idx < 3; idx++ {
		pEvName := fmt.Sprintf("ao2-wc-wc1-peer-%d-evidence.json", idx)
		pEvPath := filepath.Join(archiveDir, pEvName)
		if _, err := os.Stat(pEvPath); os.IsNotExist(err) {
			t.Fatalf("peer evidence file not archived: %s", pEvName)
		}
	}

	// Verify main report is promoted
	mainReportPath := filepath.Join(tmpDir, "agy-swarms-report-wc1.json")
	if _, err := os.Stat(mainReportPath); os.IsNotExist(err) {
		t.Fatal("main report file agy-swarms-report-wc1.json was not promoted")
	}
}

func TestParallelSwarmsAllFail(t *testing.T) {
	tmpDir := t.TempDir()

	traceFile := filepath.Join(tmpDir, "trace.log")
	dummyAgy := compileTestAgySwarmsWithPeers(t, tmpDir, traceFile)
	t.Setenv("AGY_SWARMS_PATH", dummyAgy)

	dummyAo2 := compileTestAo2(t, tmpDir, filepath.Join(tmpDir, "ao2-trace.log"))
	t.Setenv("AO2_PATH", dummyAo2)

	defaultWS := filepath.Join(tmpDir, "default-ws")
	if err := os.MkdirAll(defaultWS, 0755); err != nil {
		t.Fatalf("mkdir default-ws: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if code, _, stderr := runCLI("init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	// Construct plan with wc1 using agy-swarms with 2 peers, both of which will fail the rubric
	// Peer 0 will have "forbidden pattern found", and Peer 1 will have 95% coverage but we require 99% coverage!
	planContent := fmt.Sprintf(`{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-abcdef123456",
		"objective": {
			"text": "test parallel peers execution failure",
			"workspace": %q,
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "allowed",
			"explanation": "gate allowed"
		},
		"workcells": [
			{
				"workcell_id": "wc1",
				"kind": "prepare",
				"executor": "agy-swarms",
				"peers": 2,
				"task": "Perform peer tasks",
				"status": "planned",
				"depends_on": [],
				"rubric": {
					"required_patterns": ["success pattern present"],
					"forbidden_patterns": ["forbidden pattern found"],
					"min_coverage": 99.0
				}
			}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`, defaultWS)

	planPath := filepath.Join(tmpDir, "plan.json")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	gatePath := filepath.Join(tmpDir, "gate_result.json")
	gateContent := `{
		"status": "allowed",
		"plan_id": "forge-plan-abcdef123456",
		"decision": {
			"decision_id": "covenant-allow-local-safe",
			"target": "factory-plan",
			"decision": "allow",
			"explanation": "Covenant policy verification passed locally",
			"source": "covenant-gate"
		}
	}`
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate_result: %v", err)
	}

	outPath := filepath.Join(tmpDir, "packet-out.json")
	code, stdout, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--no-dashboard", "--non-interactive")
	if code == 0 {
		t.Fatal("expected run to fail but it succeeded")
	}

	// Verify packet-out.json status is failed
	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read packet: %v\nExitCode: %d\nStdout: %s\nStderr: %s", err, code, stdout, stderr)
	}
	var packet map[string]interface{}
	if err := json.Unmarshal(packetData, &packet); err != nil {
		t.Fatalf("unmarshal packet: %v", err)
	}

	status, _ := packet["status"].(string)
	if status != "failed" {
		t.Fatalf("expected packet status to be failed, got: %q", status)
	}

	// Verify that evidence files are still archived for all peers
	runID := packet["factory_plan"].(map[string]interface{})["plan_id"].(string)
	archiveDir := filepath.Join(tmpDir, ".forge", "runs", runID)

	for idx := 0; idx < 2; idx++ {
		pEvName := fmt.Sprintf("ao2-wc-wc1-peer-%d-evidence.json", idx)
		pEvPath := filepath.Join(archiveDir, pEvName)
		if _, err := os.Stat(pEvPath); os.IsNotExist(err) {
			t.Fatalf("failed peer evidence file not archived: %s", pEvName)
		}
	}
}

func compileTestRepairAgySwarms(t *testing.T, tmpDir string, tracePath string) string {
	t.Helper()
	dummySrc := filepath.Join(tmpDir, "dummy_repair_agy.go")
	dummyBin := filepath.Join(tmpDir, "dummy_repair_agy")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}
	srcContent := fmt.Sprintf(`package main
import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	var taskPath, reportPath string
	for i, arg := range os.Args {
		if arg == "--task" && i+1 < len(os.Args) {
			taskPath = os.Args[i+1]
		}
		if arg == "--report" && i+1 < len(os.Args) {
			reportPath = os.Args[i+1]
		}
	}

	if %q != "" {
		traceData := strings.Join(os.Args, " ") + "\n"
		f, err := os.OpenFile(%q, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			f.WriteString(traceData)
			f.Close()
		}
	}

	var taskData map[string]interface{}
	if taskPath != "" {
		data, _ := os.ReadFile(taskPath)
		_ = json.Unmarshal(data, &taskData)
	}
	taskText, _ := taskData["task"].(string)

	isRepair := strings.Contains(taskText, "self-healing/repair mode")
	hasRepairedFile := false
	if _, err := os.Stat("repaired.txt"); err == nil {
		hasRepairedFile = true
	}

	if isRepair {
		// Perform the repair
		_ = os.WriteFile("repaired.txt", []byte("success"), 0644)
		if reportPath != "" {
			report := map[string]interface{}{
				"status": "succeeded",
				"results": map[string]interface{}{
					"worker_0": map[string]interface{}{
						"status": "succeeded",
						"exit_code": 0,
					},
				},
			}
			data, _ := json.Marshal(report)
			_ = os.WriteFile(reportPath, data, 0644)
		}
		fmt.Println("Repair run completed successfully")
		os.Exit(0)
	}

	if hasRepairedFile {
		if reportPath != "" {
			report := map[string]interface{}{
				"status": "succeeded",
				"results": map[string]interface{}{
					"worker_0": map[string]interface{}{
						"status": "succeeded",
						"stdout": "success pattern present\ncoverage is 90.0%%\n",
						"exit_code": 0,
					},
				},
			}
			data, _ := json.Marshal(report)
			_ = os.WriteFile(reportPath, data, 0644)
		}
		fmt.Println("success pattern present")
		fmt.Println("coverage is 90.0%%")
		os.Exit(0)
	} else {
		// First run: fail
		if reportPath != "" {
			report := map[string]interface{}{
				"status": "failed",
				"results": map[string]interface{}{
					"worker_0": map[string]interface{}{
						"status": "failed",
						"stdout": "failed execution\n",
						"exit_code": 1,
					},
				},
			}
			data, _ := json.Marshal(report)
			_ = os.WriteFile(reportPath, data, 0644)
		}
		fmt.Fprintln(os.Stderr, "failed execution")
		os.Exit(1)
	}
}`, tracePath, tracePath)

	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy src: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy bin: %v (output: %q)", err, string(out))
	}
	return dummyBin
}

func TestSelfHealingRepair(t *testing.T) {
	tmpDir := t.TempDir()

	dummyAo2 := compileTestAo2(t, tmpDir, filepath.Join(tmpDir, "ao2-trace.log"))
	t.Setenv("AO2_PATH", dummyAo2)

	traceFile := filepath.Join(tmpDir, "trace.log")
	dummyAgy := compileTestRepairAgySwarms(t, tmpDir, traceFile)
	t.Setenv("AGY_SWARMS_PATH", dummyAgy)

	defaultWS := filepath.Join(tmpDir, "default-ws")
	if err := os.MkdirAll(defaultWS, 0755); err != nil {
		t.Fatalf("mkdir default-ws: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(defaultWS); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if code, _, stderr := runCLI("init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	// Construct plan with wc1 using agy-swarms with max_repairs: 2
	planContent := fmt.Sprintf(`{
		"schema_version": "ao.forge.factory-plan.v0.1",
		"plan_id": "forge-plan-9876543210ab",
		"objective": {
			"text": "test self healing repair",
			"workspace": %q,
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"execution_enabled": false,
		"policy_gate": {
			"required": true,
			"status": "allowed",
			"explanation": "gate allowed"
		},
		"workcells": [
			{
				"workcell_id": "wc1",
				"kind": "prepare",
				"executor": "agy-swarms",
				"max_repairs": 2,
				"task": "Fix the bug",
				"status": "planned",
				"depends_on": [],
				"rubric": {
					"required_patterns": ["success pattern present"],
					"min_coverage": 80.0
				}
			}
		],
		"expected_evidence": ["test"],
		"next_actions": [
			{"action_id": "test", "description": "test", "required": true}
		]
	}`, defaultWS)

	planPath := filepath.Join(tmpDir, "plan.json")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	gatePath := filepath.Join(tmpDir, "gate_result.json")
	gateContent := `{
		"status": "allowed",
		"plan_id": "forge-plan-9876543210ab",
		"decision": {
			"decision_id": "covenant-allow-local-safe",
			"target": "factory-plan",
			"decision": "allow",
			"explanation": "Covenant policy verification passed locally",
			"source": "covenant-gate"
		}
	}`
	if err := os.WriteFile(gatePath, []byte(gateContent), 0644); err != nil {
		t.Fatalf("write gate_result: %v", err)
	}

	outPath := filepath.Join(tmpDir, "packet-out.json")
	code, stdout, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--no-dashboard", "--non-interactive")
	if code != 0 {
		packetData, readErr := os.ReadFile(outPath)
		t.Fatalf("run failed: %d\nStdout: %s\nStderr: %s\nPacket Data (err=%v): %s", code, stdout, stderr, readErr, string(packetData))
	}

	// Read packet to verify that wc1 succeeded and repairs_attempted was recorded as 1
	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	var packet map[string]interface{}
	if err := json.Unmarshal(packetData, &packet); err != nil {
		t.Fatalf("unmarshal packet: %v", err)
	}

	workcells, ok := packet["workcells"].([]interface{})
	if !ok || len(workcells) == 0 {
		t.Fatalf("expected workcells in packet, got: %v", packet)
	}

	wc1 := workcells[0].(map[string]interface{})
	status, _ := wc1["status"].(string)
	repairsAttempted, _ := wc1["repairs_attempted"].(float64)

	if status != "passed" {
		t.Fatalf("expected wc1 status to be passed after self-healing, got: %q", status)
	}

	if repairsAttempted != 1 {
		t.Fatalf("expected repairs_attempted to be 1, got: %f", repairsAttempted)
	}

	// Verify that the repair swarm report file was written
	repairReportPath := filepath.Join(defaultWS, "agy-swarms-report-wc1-repair-attempt-1.json")
	if _, err := os.Stat(repairReportPath); os.IsNotExist(err) {
		t.Fatalf("expected repair swarm report to exist: %s", repairReportPath)
	}
}

func compileTestDynamicAgySwarms(t *testing.T, tmpDir string) string {
	t.Helper()
	dummySrc := filepath.Join(tmpDir, "dummy_dynamic_agy.go")
	dummyBin := filepath.Join(tmpDir, "dummy_dynamic_agy")
	if os.PathSeparator == '\\' {
		dummyBin += ".exe"
	}
	srcContent := `package main
import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	var taskPath, reportPath string
	for i, arg := range os.Args {
		if arg == "--task" && i+1 < len(os.Args) {
			taskPath = os.Args[i+1]
		}
		if arg == "--report" && i+1 < len(os.Args) {
			reportPath = os.Args[i+1]
		}
	}

	var taskData map[string]interface{}
	if taskPath != "" {
		data, _ := os.ReadFile(taskPath)
		_ = json.Unmarshal(data, &taskData)
	}
	taskText, _ := taskData["task"].(string)

	isDynamic := strings.Contains(taskText, "dynamic-plan-workcells.json")
	if isDynamic {
		// Output the mock workcells to dynamic-plan-workcells.json in current directory
		workcells := []map[string]interface{}{
			{
				"workcell_id": "wc-prep",
				"kind":        "prepare",
				"executor":    "agy-swarms",
				"task":        "Perform prepare steps",
				"depends_on":  []string{},
			},
			{
				"workcell_id": "wc-exec",
				"kind":        "execute",
				"executor":    "agy-swarms",
				"task":        "Execute tests",
				"depends_on":  []string{"wc-prep"},
			},
		}
		data, _ := json.Marshal(workcells)
		_ = os.WriteFile("dynamic-plan-workcells.json", data, 0644)

		if reportPath != "" {
			report := map[string]interface{}{
				"status": "succeeded",
			}
			rData, _ := json.Marshal(report)
			_ = os.WriteFile(reportPath, rData, 0644)
		}
		fmt.Println("Dynamic planning completed successfully")
		os.Exit(0)
	}

	fmt.Println("Mock execution success")
	os.Exit(0)
}`

	if err := os.WriteFile(dummySrc, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write dummy dynamic agy src: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", dummyBin, dummySrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy dynamic agy: %v (output: %q)", err, string(out))
	}
	return dummyBin
}

func TestDynamicPlanGeneration(t *testing.T) {
	tmpDir := t.TempDir()

	dummyAgy := compileTestDynamicAgySwarms(t, tmpDir)
	t.Setenv("AGY_SWARMS_PATH", dummyAgy)

	defaultWS := filepath.Join(tmpDir, "default-ws")
	if err := os.MkdirAll(defaultWS, 0755); err != nil {
		t.Fatalf("mkdir default-ws: %v", err)
	}

	// Create a brief JSON that does NOT have expected_workcells (relaxing requirement)
	briefContent := fmt.Sprintf(`{
		"schema_version": "ao.forge.factory-brief.v0.1",
		"objective": {
			"text": "Decompose this dynamically",
			"workspace": %q,
			"release_mode": false
		},
		"constraints": {
			"local_first": true,
			"allow_network": false,
			"allow_release_mutation": false,
			"require_control_plane_readback": false
		},
		"expected_evidence": ["factory packet"]
	}`, defaultWS)

	briefPath := filepath.Join(tmpDir, "brief.json")
	if err := os.WriteFile(briefPath, []byte(briefContent), 0644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	// Change wd to defaultWS to support init
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(defaultWS); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if code, _, stderr := runCLI("init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	outPath := filepath.Join(tmpDir, "plan-out.json")
	code, stdout, stderr := runCLI("plan", "--brief", briefPath, "--dynamic", "--out", outPath)
	if code != 0 {
		t.Fatalf("plan failed: %d\nStdout: %s\nStderr: %s", code, stdout, stderr)
	}

	// Verify that the output plan has the dynamic workcells
	planData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated plan: %v", err)
	}

	var plan map[string]interface{}
	if err := json.Unmarshal(planData, &plan); err != nil {
		t.Fatalf("unmarshal generated plan: %v", err)
	}

	workcells, ok := plan["workcells"].([]interface{})
	if !ok || len(workcells) != 2 {
		t.Fatalf("expected 2 workcells in generated plan, got: %v", plan)
	}

	wc0 := workcells[0].(map[string]interface{})
	wc1 := workcells[1].(map[string]interface{})

	if wc0["workcell_id"].(string) != "wc-prep" || wc0["status"].(string) != "planned" {
		t.Fatalf("unexpected first workcell: %+v", wc0)
	}
	if wc1["workcell_id"].(string) != "wc-exec" || wc1["status"].(string) != "planned" {
		t.Fatalf("unexpected second workcell: %+v", wc1)
	}

	// Verify temporary file is cleaned up
	tempPlanPath := filepath.Join(defaultWS, "dynamic-plan-workcells.json")
	if _, err := os.Stat(tempPlanPath); err == nil {
		t.Fatalf("expected dynamic-plan-workcells.json to be cleaned up, but it still exists")
	}
}

func buildGitPushWrapper(t *testing.T, tmpDir string) string {
	t.Helper()

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}

	wrapperSrc := filepath.Join(tmpDir, "dummy_git.go")
	wrapperBin := filepath.Join(tmpDir, "dummy_git")
	if os.PathSeparator == '\\' {
		wrapperBin += ".exe"
	}
	source := fmt.Sprintf(`package main
import (
	"fmt"
	"os"
	"os/exec"
)
func main() {
	for _, arg := range os.Args[1:] {
		if arg == "push" {
			if os.Getenv("AO_FORGE_TEST_GIT_PUSH_FAIL") == "1" {
				fmt.Fprintln(os.Stderr, "simulated git push failure")
				os.Exit(1)
			}
			fmt.Fprintln(os.Stdout, "simulated git push ok")
			os.Exit(0)
		}
	}
	cmd := exec.Command(%q, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
`, realGit)
	if err := os.WriteFile(wrapperSrc, []byte(source), 0644); err != nil {
		t.Fatalf("write dummy git: %v", err)
	}
	cmdBuild := exec.Command("go", "build", "-o", wrapperBin, wrapperSrc)
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		t.Fatalf("build dummy git: %v (output: %q)", err, string(out))
	}
	return wrapperBin
}

func TestReleaseMutationRequiresPreviewAudit(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")
	if err := os.WriteFile(filepath.Join(workspaceDir, "VERSION"), []byte("1.2.7"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	runGit("add", "VERSION")
	runGit("commit", "-m", "version 1.2.7")

	dummyAo2 := compileTestAo2(t, tmpDir, filepath.Join(tmpDir, "ao2-trace.log"))
	t.Setenv("AO2_PATH", dummyAo2)
	t.Setenv("GIT_PATH", buildGitPushWrapper(t, tmpDir))

	plan := factoryPlan{
		SchemaVersion: "ao.forge.factory-plan.v0.1",
		PlanID:        "forge-plan-1234567890ab",
		Objective: factoryObjective{
			Text:        "Deploy release version",
			Workspace:   workspaceDir,
			ReleaseMode: true,
		},
		Constraints: factoryConstraints{
			LocalFirst:           true,
			AllowNetwork:         false,
			AllowReleaseMutation: true,
		},
		PolicyGate: policyGate{
			Required:    true,
			Status:      "allowed",
			Explanation: "approved",
		},
		Workcells: []planWorkcell{
			{WorkcellID: "wc1", Kind: "prepare", Status: "planned", DependsOn: []string{}},
		},
		ExpectedEvidence: []string{"test"},
		NextActions:      []nextAction{{ActionID: "test", Description: "test", Required: true}},
	}
	planData, _ := json.Marshal(plan)
	planPath := filepath.Join(tmpDir, "plan.json")
	_ = os.WriteFile(planPath, planData, 0644)

	gateResult := covenantGateResult{
		SchemaVersion:    "ao.forge.covenant-gate-result.v0.1",
		Status:           "allowed",
		PlanID:           plan.PlanID,
		ExecutionEnabled: true,
		Decision: covenantDecisionFixture{
			SchemaVersion: "ao.forge.covenant-decision-fixture.v0.1",
			TargetPlanID:  plan.PlanID,
			Decision:      "allow",
			DecisionID:    "allow-safe",
			Explanation:   "Approved",
			Source:        "test",
		},
	}
	gateData, _ := json.Marshal(gateResult)
	gatePath := filepath.Join(tmpDir, "gate.json")
	_ = os.WriteFile(gatePath, gateData, 0644)

	outPath := filepath.Join(tmpDir, "packet.json")
	code, _, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--live", "--confirm-release")
	if code == 0 {
		t.Fatalf("expected confirmed release run without preview audit to fail closed")
	}
	if !strings.Contains(stderr, "release preview audit is required") {
		t.Fatalf("stderr missing preview audit requirement: %s", stderr)
	}

	var packet factoryPacket
	packetData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if err := json.Unmarshal(packetData, &packet); err != nil {
		t.Fatalf("unmarshal packet: %v", err)
	}
	if packet.Status != "blocked" || len(packet.PolicyDecisions) == 0 || packet.PolicyDecisions[0].DecisionID != "release-preview-audit-required" {
		t.Fatalf("expected release-preview-audit-required blocked packet, got: %+v", packet)
	}
}

func TestReleaseMutationDraftPublishing(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Set up mock git repository
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")

	// Write VERSION file
	versionPath := filepath.Join(workspaceDir, "VERSION")
	if err := os.WriteFile(versionPath, []byte("1.2.3"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}

	runGit("add", "VERSION")
	runGit("commit", "-m", "initial version")

	previewAuditPath := filepath.Join(tmpDir, "release-preview.json")
	previewCode, previewStdout, previewStderr := runCLI("release-preview", "--workspace", workspaceDir, "--artifact", versionPath, "--out", previewAuditPath)
	if previewCode != 0 {
		t.Fatalf("release-preview failed with code %d\nStdout: %s\nStderr: %s", previewCode, previewStdout, previewStderr)
	}

	// 2. Set up mock HTTP server for GitHub API fallback
	var apiCalled bool
	var receivedBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/repos/test-owner/test-repo/releases" {
			apiCalled = true
			decoder := json.NewDecoder(r.Body)
			_ = decoder.Decode(&receivedBody)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id": 123, "tag_name": "v1.2.3"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	// Compile a dummy gh CLI that always fails (to force fallback)
	dummyGhSrc := filepath.Join(tmpDir, "dummy_gh.go")
	dummyGhBin := filepath.Join(tmpDir, "dummy_gh")
	if os.PathSeparator == '\\' {
		dummyGhBin += ".exe"
	}
	ghSrcContent := `package main
import "os"
func main() {
	// Exit 1 to simulate gh CLI failure or lack of authentication
	os.Exit(1)
}`
	if err := os.WriteFile(dummyGhSrc, []byte(ghSrcContent), 0644); err != nil {
		t.Fatalf("write dummy gh: %v", err)
	}
	cmdBuild := exec.Command("go", "build", "-o", dummyGhBin, dummyGhSrc)
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		t.Fatalf("build dummy gh: %v (output: %q)", err, string(out))
	}

	t.Setenv("GH_PATH", dummyGhBin)
	t.Setenv("AO_FORGE_MOCK_GITHUB_API", ts.URL)
	t.Setenv("GITHUB_TOKEN", "mock-token")
	t.Setenv("GIT_PATH", buildGitPushWrapper(t, tmpDir))

	// Compile a dummy ao2
	dummyAo2Src := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyAo2Bin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyAo2Bin += ".exe"
	}
	ao2SrcContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		fmt.Println("status=governed_run_started")
		os.Exit(0)
	}
	os.Exit(1)
}`
	if err := os.WriteFile(dummyAo2Src, []byte(ao2SrcContent), 0644); err != nil {
		t.Fatalf("write dummy ao2: %v", err)
	}
	cmdBuildAo2 := exec.Command("go", "build", "-o", dummyAo2Bin, dummyAo2Src)
	if out, err := cmdBuildAo2.CombinedOutput(); err != nil {
		t.Fatalf("build dummy ao2: %v (output: %q)", err, string(out))
	}
	t.Setenv("AO2_PATH", dummyAo2Bin)

	// Set up factory plan
	plan := factoryPlan{
		SchemaVersion: "ao.forge.factory-plan.v0.1",
		PlanID:        "forge-plan-1234567890ab",
		Objective: factoryObjective{
			Text:        "Deploy release version",
			Workspace:   workspaceDir,
			ReleaseMode: true,
		},
		Constraints: factoryConstraints{
			LocalFirst:           true,
			AllowNetwork:         false,
			AllowReleaseMutation: true,
		},
		PolicyGate: policyGate{
			Required:    true,
			Status:      "allowed",
			Explanation: "approved",
		},
		Workcells: []planWorkcell{
			{WorkcellID: "wc1", Kind: "prepare", Status: "planned", DependsOn: []string{}},
		},
		ExpectedEvidence: []string{"test"},
		NextActions: []nextAction{
			{ActionID: "test", Description: "test", Required: true},
		},
	}
	planData, _ := json.Marshal(plan)
	planPath := filepath.Join(tmpDir, "plan.json")
	_ = os.WriteFile(planPath, planData, 0644)

	gateResult := covenantGateResult{
		SchemaVersion:    "ao.forge.covenant-gate-result.v0.1",
		Status:           "allowed",
		PlanID:           "forge-plan-1234567890ab",
		ExecutionEnabled: true,
		Decision: covenantDecisionFixture{
			SchemaVersion: "ao.forge.covenant-decision-fixture.v0.1",
			TargetPlanID:  "plan-id-12345",
			Decision:      "allow",
			DecisionID:    "allow-safe",
			Explanation:   "Approved",
			Source:        "test",
		},
	}
	gateData, _ := json.Marshal(gateResult)
	gatePath := filepath.Join(tmpDir, "gate.json")
	_ = os.WriteFile(gatePath, gateData, 0644)

	outPath := filepath.Join(tmpDir, "packet.json")
	code, stdout, stderr := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--live", "--confirm-release", "--release-preview-audit", previewAuditPath)
	if code != 0 {
		t.Fatalf("run failed with code %d\nStdout: %s\nStderr: %s", code, stdout, stderr)
	}

	if !apiCalled {
		t.Fatalf("expected GitHub API fallback to be called")
	}

	if receivedBody["tag_name"] != "v1.2.3" || receivedBody["name"] != "Release v1.2.3" {
		t.Fatalf("unexpected API body received: %+v", receivedBody)
	}

	// Verify local tag was created pointing to HEAD
	tagCheck := exec.Command("git", "rev-parse", "v1.2.3^{commit}")
	tagCheck.Dir = workspaceDir
	out, err := tagCheck.Output()
	if err != nil {
		t.Fatalf("git tag check failed: %v", err)
	}
	tagCommit := strings.TrimSpace(string(out))

	headCheck := exec.Command("git", "rev-parse", "HEAD")
	headCheck.Dir = workspaceDir
	outHead, err := headCheck.Output()
	if err != nil {
		t.Fatalf("git HEAD check failed: %v", err)
	}
	headCommit := strings.TrimSpace(string(outHead))

	if tagCommit != headCommit {
		t.Fatalf("expected tag commit %s to match HEAD commit %s", tagCommit, headCommit)
	}
}

func TestReleaseMutationFailsClosedWhenTagPushFails(t *testing.T) {
	tmpDir := t.TempDir()

	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")

	versionPath := filepath.Join(workspaceDir, "VERSION")
	if err := os.WriteFile(versionPath, []byte("1.2.4"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}

	runGit("add", "VERSION")
	runGit("commit", "-m", "version 1.2.4")

	t.Setenv("GIT_PATH", buildGitPushWrapper(t, tmpDir))
	t.Setenv("AO_FORGE_TEST_GIT_PUSH_FAIL", "1")

	plan := factoryPlan{
		SchemaVersion: "ao.forge.factory-plan.v0.1",
		PlanID:        "forge-plan-1234567890ab",
		Objective: factoryObjective{
			Text:        "Deploy release version",
			Workspace:   workspaceDir,
			ReleaseMode: true,
		},
		Constraints: factoryConstraints{
			LocalFirst:           true,
			AllowNetwork:         false,
			AllowReleaseMutation: true,
		},
	}

	var stdout, stderr bytes.Buffer
	err := performReleaseMutation(plan, filepath.Join(tmpDir, "packet.json"), nil, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected release mutation to fail closed when git tag push fails")
	}
	if !strings.Contains(err.Error(), "failed to push git tag") {
		t.Fatalf("expected git tag push failure, got: %v", err)
	}
	if strings.Contains(stdout.String(), "Published GitHub release") {
		t.Fatalf("release publishing should not continue after tag push failure; stdout: %s", stdout.String())
	}
}

func TestReleaseMutationMissingTokenFailsClosed(t *testing.T) {
	tmpDir := t.TempDir()

	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")

	versionPath := filepath.Join(workspaceDir, "VERSION")
	_ = os.WriteFile(versionPath, []byte("2.0.0"), 0644)
	runGit("add", "VERSION")
	runGit("commit", "-m", "version 2.0.0")

	// Hide gh CLI and GITHUB_TOKEN to force authentication failure
	t.Setenv("GH_PATH", "/non-existent/gh")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("AO_FORGE_GITHUB_TOKEN", "")
	t.Setenv("GIT_PATH", buildGitPushWrapper(t, tmpDir))

	// Compile a dummy ao2
	dummyAo2Src := filepath.Join(tmpDir, "dummy_ao2.go")
	dummyAo2Bin := filepath.Join(tmpDir, "dummy_ao2")
	if os.PathSeparator == '\\' {
		dummyAo2Bin += ".exe"
	}
	ao2SrcContent := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		fmt.Println("status=governed_run_started")
		os.Exit(0)
	}
	os.Exit(1)
}`
	_ = os.WriteFile(dummyAo2Src, []byte(ao2SrcContent), 0644)
	cmdBuildAo2 := exec.Command("go", "build", "-o", dummyAo2Bin, dummyAo2Src)
	_ = cmdBuildAo2.Run()
	t.Setenv("AO2_PATH", dummyAo2Bin)

	plan := factoryPlan{
		SchemaVersion: "ao.forge.factory-plan.v0.1",
		PlanID:        "forge-plan-1234567890ab",
		Objective: factoryObjective{
			Text:        "Release v2.0.0",
			Workspace:   workspaceDir,
			ReleaseMode: true,
		},
		Constraints: factoryConstraints{
			LocalFirst:           true,
			AllowNetwork:         false,
			AllowReleaseMutation: true,
		},
		PolicyGate: policyGate{
			Required:    true,
			Status:      "allowed",
			Explanation: "approved",
		},
		Workcells: []planWorkcell{
			{WorkcellID: "wc1", Kind: "prepare", Status: "planned", DependsOn: []string{}},
		},
		ExpectedEvidence: []string{"test"},
		NextActions: []nextAction{
			{ActionID: "test", Description: "test", Required: true},
		},
	}
	planData, _ := json.Marshal(plan)
	planPath := filepath.Join(tmpDir, "plan.json")
	_ = os.WriteFile(planPath, planData, 0644)

	gateResult := covenantGateResult{
		SchemaVersion:    "ao.forge.covenant-gate-result.v0.1",
		Status:           "allowed",
		PlanID:           "forge-plan-1234567890ab",
		ExecutionEnabled: true,
		Decision: covenantDecisionFixture{
			SchemaVersion: "ao.forge.covenant-decision-fixture.v0.1",
			TargetPlanID:  "forge-plan-1234567890ab",
			Decision:      "allow",
			DecisionID:    "allow-safe",
			Explanation:   "Approved",
			Source:        "test",
		},
	}
	gateData, _ := json.Marshal(gateResult)
	gatePath := filepath.Join(tmpDir, "gate.json")
	_ = os.WriteFile(gatePath, gateData, 0644)

	outPath := filepath.Join(tmpDir, "packet.json")
	code, _, _ := runCLI("run", "--plan", planPath, "--gate-result", gatePath, "--out", outPath, "--live", "--confirm-release")
	if code == 0 {
		t.Fatalf("expected run to fail closed due to missing github authentication, but it exited 0")
	}
}

func TestReleasePreviewAuditWritesMachineReadableBundle(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")

	versionPath := filepath.Join(workspaceDir, "VERSION")
	artifactPath := filepath.Join(workspaceDir, "dist.txt")
	if err := os.WriteFile(versionPath, []byte("1.2.5"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("release artifact"), 0644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	runGit("add", "VERSION", "dist.txt")
	runGit("commit", "-m", "version 1.2.5")

	outPath := filepath.Join(tmpDir, "release-preview.json")
	code, stdout, stderr := runCLI("release-preview", "--workspace", workspaceDir, "--artifact", artifactPath, "--out", outPath)
	if code != 0 {
		t.Fatalf("release-preview failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "release_preview_audit=") {
		t.Fatalf("stdout missing audit path: %s", stdout)
	}

	var audit struct {
		SchemaVersion   string `json:"schema_version"`
		Status          string `json:"status"`
		Workspace       string `json:"workspace"`
		GitHubRepo      string `json:"github_repo"`
		Tag             string `json:"tag"`
		HeadCommit      string `json:"head_commit"`
		MutatesReleases bool   `json:"mutates_releases"`
		NetworkRequired bool   `json:"network_required"`
		Checks          []struct {
			CheckID string `json:"check_id"`
			Status  string `json:"status"`
			Summary string `json:"summary"`
		} `json:"checks"`
		Artifacts []struct {
			Path       string `json:"path"`
			SHA256     string `json:"sha256"`
			SizeBytes  int64  `json:"size_bytes"`
			Status     string `json:"status"`
			Provenance string `json:"provenance"`
		} `json:"artifacts"`
		NextActions []nextAction `json:"next_actions"`
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatalf("unmarshal audit: %v", err)
	}
	if audit.SchemaVersion != "ao.forge.release-preview-audit.v0.1" {
		t.Fatalf("unexpected schema: %q", audit.SchemaVersion)
	}
	if audit.Status != "passed" || audit.Tag != "v1.2.5" || audit.GitHubRepo != "test-owner/test-repo" {
		t.Fatalf("unexpected audit identity: %+v", audit)
	}
	if audit.MutatesReleases || audit.NetworkRequired {
		t.Fatalf("preview audit must be non-mutating/local-only: %+v", audit)
	}
	if len(audit.Artifacts) != 1 || audit.Artifacts[0].SHA256 == "" || audit.Artifacts[0].SizeBytes == 0 || audit.Artifacts[0].Status != "present" {
		t.Fatalf("artifact audit missing checksum/size/status: %+v", audit.Artifacts)
	}
	if len(audit.Checks) < 5 {
		t.Fatalf("expected release preview checks, got: %+v", audit.Checks)
	}
}

func TestReleasePreviewInspectPrintsOperatorSummary(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")
	versionPath := filepath.Join(workspaceDir, "VERSION")
	artifactPath := filepath.Join(workspaceDir, "dist.txt")
	if err := os.WriteFile(versionPath, []byte("1.2.8"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("release artifact"), 0644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	runGit("add", "VERSION", "dist.txt")
	runGit("commit", "-m", "version 1.2.8")

	auditPath := filepath.Join(tmpDir, "release-preview.json")
	code, stdout, stderr := runCLI("release-preview", "--workspace", workspaceDir, "--artifact", artifactPath, "--out", auditPath)
	if code != 0 {
		t.Fatalf("release-preview failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	code, stdout, stderr = runCLI("release-preview", "inspect", "--audit", auditPath)
	if code != 0 {
		t.Fatalf("release-preview inspect failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{
		"release_preview_audit=" + displayPath(auditPath),
		"schema_version=ao.forge.release-preview-audit.v0.1",
		"status=passed",
		"workspace=" + displayPath(workspaceDir),
		"github_repo=test-owner/test-repo",
		"tag=v1.2.8",
		"mutates_releases=false",
		"network_required=false",
		"failed_checks=0",
		"artifacts=1",
		"artifact=" + displayPath(artifactPath) + " status=present",
		"next_action=review-release-preview-audit required=true",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("release preview inspect output missing %q\n%s", want, stdout)
		}
	}
}

func TestReleasePreviewInspectPrintsJSONSummary(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")
	versionPath := filepath.Join(workspaceDir, "VERSION")
	artifactPath := filepath.Join(workspaceDir, "dist.txt")
	if err := os.WriteFile(versionPath, []byte("1.2.10"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("release artifact"), 0644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	runGit("add", "VERSION", "dist.txt")
	runGit("commit", "-m", "version 1.2.10")

	auditPath := filepath.Join(tmpDir, "release-preview.json")
	code, stdout, stderr := runCLI("release-preview", "--workspace", workspaceDir, "--artifact", artifactPath, "--out", auditPath)
	if code != 0 {
		t.Fatalf("release-preview failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	code, stdout, stderr = runCLI("release-preview", "inspect", "--audit", auditPath, "--json")
	if code != 0 {
		t.Fatalf("release-preview inspect --json failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("release-preview inspect --json wrote stderr: %s", stderr)
	}

	var summary struct {
		InspectSchemaVersion string `json:"inspect_schema_version"`
		ReleasePreviewAudit  string `json:"release_preview_audit"`
		SchemaVersion        string `json:"schema_version"`
		Status               string `json:"status"`
		Workspace            string `json:"workspace"`
		GitHubRepo           string `json:"github_repo"`
		Tag                  string `json:"tag"`
		HeadCommit           string `json:"head_commit"`
		MutatesReleases      bool   `json:"mutates_releases"`
		NetworkRequired      bool   `json:"network_required"`
		Checks               int    `json:"checks"`
		FailedChecks         int    `json:"failed_checks"`
		Artifacts            int    `json:"artifacts"`
		ArtifactDetails      []struct {
			Path       string `json:"path"`
			SHA256     string `json:"sha256"`
			SizeBytes  int64  `json:"size_bytes"`
			Status     string `json:"status"`
			Provenance string `json:"provenance"`
		} `json:"artifact_details"`
		NextActions []nextAction `json:"next_actions"`
	}
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("inspect --json did not produce valid JSON: %v\n%s", err, stdout)
	}
	if summary.InspectSchemaVersion != "ao.forge.release-preview-inspect.v0.1" {
		t.Fatalf("unexpected inspect schema version: %+v", summary)
	}
	if summary.ReleasePreviewAudit != displayPath(auditPath) || summary.SchemaVersion != releasePreviewAuditVersion || summary.Status != "passed" {
		t.Fatalf("unexpected JSON summary identity: %+v", summary)
	}
	if summary.Workspace != displayPath(workspaceDir) || summary.GitHubRepo != "test-owner/test-repo" || summary.Tag != "v1.2.10" {
		t.Fatalf("unexpected JSON summary release fields: %+v", summary)
	}
	if summary.MutatesReleases || summary.NetworkRequired || summary.Checks < 5 || summary.FailedChecks != 0 {
		t.Fatalf("unexpected JSON summary checks: %+v", summary)
	}
	if summary.Artifacts != 1 || len(summary.ArtifactDetails) != 1 || summary.ArtifactDetails[0].Path != displayPath(artifactPath) || summary.ArtifactDetails[0].Status != "present" || summary.ArtifactDetails[0].SHA256 == "" {
		t.Fatalf("unexpected JSON artifact details: %+v", summary.ArtifactDetails)
	}
	if len(summary.NextActions) == 0 || summary.NextActions[0].ActionID != "review-release-preview-audit" {
		t.Fatalf("unexpected JSON next actions: %+v", summary.NextActions)
	}
}

func TestReleasePreviewFixtureArtifactsDriveInspectJSON(t *testing.T) {
	root := repoRoot(t)
	readText := func(path ...string) string {
		t.Helper()
		bytes, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Join(path...), err)
		}
		return string(bytes)
	}
	fixtureDir := filepath.Join(root, "examples", "release-preview")
	artifactPath := filepath.Join(fixtureDir, "ao-forge-preview-artifact.txt")
	checksumsPath := filepath.Join(fixtureDir, "checksums.txt")
	expectedArtifactsPath := filepath.Join(fixtureDir, "inspect-artifacts.expected.json")

	for _, check := range []struct {
		name string
		doc  string
		want string
	}{
		{name: "README release preview fixture link", doc: readText("README.md"), want: "[Release Preview Fixtures](examples/release-preview/)"},
		{name: "runbook release preview fixture link", doc: readText("docs", "release", "PREVIEW-RELEASE.md"), want: "../../examples/release-preview/"},
	} {
		if !strings.Contains(check.doc, check.want) {
			t.Fatalf("%s missing %q", check.name, check.want)
		}
	}

	expectedManifest, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatalf("read checksum fixture: %v", err)
	}
	code, stdout, stderr := runCLI("artifact", "checksums", "--artifact", artifactPath)
	if code != 0 {
		t.Fatalf("artifact checksums fixture failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if stdout != string(expectedManifest) {
		t.Fatalf("checksum fixture drifted\nwant:\n%sgot:\n%s", string(expectedManifest), stdout)
	}

	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")
	if err := os.WriteFile(filepath.Join(workspaceDir, "VERSION"), []byte("1.2.11"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	runGit("add", "VERSION")
	runGit("commit", "-m", "version 1.2.11")

	auditPath := filepath.Join(tmpDir, "release-preview.json")
	code, stdout, stderr = runCLI("release-preview", "--workspace", workspaceDir, "--artifact", artifactPath, "--artifact", checksumsPath, "--out", auditPath)
	if code != 0 {
		t.Fatalf("release-preview fixture failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	code, stdout, stderr = runCLI("release-preview", "inspect", "--audit", auditPath, "--json")
	if code != 0 {
		t.Fatalf("release-preview inspect fixture failed with code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	var summary struct {
		InspectSchemaVersion string                   `json:"inspect_schema_version"`
		Status               string                   `json:"status"`
		Tag                  string                   `json:"tag"`
		MutatesReleases      bool                     `json:"mutates_releases"`
		NetworkRequired      bool                     `json:"network_required"`
		Artifacts            int                      `json:"artifacts"`
		ArtifactDetails      []releasePreviewArtifact `json:"artifact_details"`
	}
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("unmarshal inspect fixture JSON: %v\n%s", err, stdout)
	}
	if summary.InspectSchemaVersion != "ao.forge.release-preview-inspect.v0.1" {
		t.Fatalf("unexpected fixture inspect schema version: %+v", summary)
	}
	if summary.Status != "passed" || summary.Tag != "v1.2.11" || summary.MutatesReleases || summary.NetworkRequired {
		t.Fatalf("unexpected release preview fixture summary: %+v", summary)
	}
	if summary.Artifacts != 2 || len(summary.ArtifactDetails) != 2 {
		t.Fatalf("expected two fixture artifact details, got: %+v", summary.ArtifactDetails)
	}

	expectedArtifacts, err := os.ReadFile(expectedArtifactsPath)
	if err != nil {
		t.Fatalf("read expected inspect artifact fixture: %v", err)
	}
	actualArtifacts, err := marshalIndented(summary.ArtifactDetails)
	if err != nil {
		t.Fatalf("marshal actual fixture artifacts: %v", err)
	}
	assertJSONOutputEqual(t, "release preview fixture artifact details", expectedArtifacts, string(actualArtifacts))
}

func TestReleasePreviewAuditValidationRejectsEmptyChecks(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")
	if err := os.WriteFile(filepath.Join(workspaceDir, "VERSION"), []byte("1.2.9"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	runGit("add", "VERSION")
	runGit("commit", "-m", "version 1.2.9")

	audit := buildReleasePreviewAudit(releasePreviewFlags{workspacePath: workspaceDir})
	audit.Checks = nil
	auditPath := filepath.Join(tmpDir, "release-preview-empty-checks.json")
	data, err := marshalIndented(audit)
	if err != nil {
		t.Fatalf("marshal audit: %v", err)
	}
	if err := os.WriteFile(auditPath, data, 0644); err != nil {
		t.Fatalf("write audit: %v", err)
	}

	plan := factoryPlan{
		Objective: factoryObjective{
			Text:        "Release v1.2.9",
			Workspace:   workspaceDir,
			ReleaseMode: true,
		},
	}
	_, err = validateReleasePreviewAuditForPlan(auditPath, plan)
	if err == nil || !strings.Contains(err.Error(), "audit must include at least one check") {
		t.Fatalf("expected empty checks validation error, got %v", err)
	}
}

func TestReleasePreviewAuditValidationRejectsMismatchedEvidence(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")
	if err := os.WriteFile(filepath.Join(workspaceDir, "VERSION"), []byte("1.3.0"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	runGit("add", "VERSION")
	runGit("commit", "-m", "version 1.3.0")

	plan := factoryPlan{
		Objective: factoryObjective{
			Text:        "Release v1.3.0",
			Workspace:   workspaceDir,
			ReleaseMode: true,
		},
	}
	baseAudit := buildReleasePreviewAudit(releasePreviewFlags{workspacePath: workspaceDir})

	cases := []struct {
		name    string
		mutate  func(*releasePreviewAudit)
		wantErr string
	}{
		{name: "blocked status", mutate: func(a *releasePreviewAudit) { a.Status = "blocked" }, wantErr: "audit status must be passed"},
		{name: "mutating preview", mutate: func(a *releasePreviewAudit) { a.MutatesReleases = true }, wantErr: "audit must be non-mutating"},
		{name: "network preview", mutate: func(a *releasePreviewAudit) { a.NetworkRequired = true }, wantErr: "audit must not require network access"},
		{name: "wrong workspace", mutate: func(a *releasePreviewAudit) { a.Workspace = "other-workspace" }, wantErr: "does not match plan workspace"},
		{name: "wrong tag", mutate: func(a *releasePreviewAudit) { a.Tag = "v9.9.9" }, wantErr: "does not match expected tag"},
		{name: "stale head", mutate: func(a *releasePreviewAudit) { a.HeadCommit = strings.Repeat("0", 40) }, wantErr: "does not match current HEAD"},
		{name: "failed check", mutate: func(a *releasePreviewAudit) { a.Checks[0].Status = "failed" }, wantErr: "is \"failed\""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			audit := baseAudit
			audit.Checks = append([]releasePreviewCheck(nil), baseAudit.Checks...)
			audit.Artifacts = append([]releasePreviewArtifact(nil), baseAudit.Artifacts...)
			audit.NextActions = append([]nextAction(nil), baseAudit.NextActions...)
			tc.mutate(&audit)

			auditPath := filepath.Join(tmpDir, strings.ReplaceAll(tc.name, " ", "-")+".json")
			data, err := marshalIndented(audit)
			if err != nil {
				t.Fatalf("marshal audit: %v", err)
			}
			if err := os.WriteFile(auditPath, data, 0644); err != nil {
				t.Fatalf("write audit: %v", err)
			}
			_, err = validateReleasePreviewAuditForPlan(auditPath, plan)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected validation error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestReleasePreviewAuditFailsClosedOnDirtyWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = workspaceDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %q)", args, err, string(out))
		}
	}
	runGit("init")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test User")
	runGit("config", "commit.gpgSign", "false")
	runGit("remote", "add", "origin", "git@github.com:test-owner/test-repo.git")
	if err := os.WriteFile(filepath.Join(workspaceDir, "VERSION"), []byte("1.2.6"), 0644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}
	runGit("add", "VERSION")
	runGit("commit", "-m", "version 1.2.6")
	if err := os.WriteFile(filepath.Join(workspaceDir, "dirty.txt"), []byte("not committed"), 0644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	outPath := filepath.Join(tmpDir, "release-preview.json")
	code, _, stderr := runCLI("release-preview", "--workspace", workspaceDir, "--out", outPath)
	if code == 0 {
		t.Fatalf("expected dirty workspace to fail closed")
	}
	if !strings.Contains(stderr, "dirty release workspace") {
		t.Fatalf("stderr missing dirty workspace explanation: %s", stderr)
	}

	var audit struct {
		Status string `json:"status"`
		Checks []struct {
			CheckID string `json:"check_id"`
			Status  string `json:"status"`
		} `json:"checks"`
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatalf("unmarshal audit: %v", err)
	}
	if audit.Status != "blocked" {
		t.Fatalf("expected blocked audit, got: %+v", audit)
	}
	var foundDirty bool
	for _, check := range audit.Checks {
		if check.CheckID == "clean-worktree" && check.Status == "failed" {
			foundDirty = true
		}
	}
	if !foundDirty {
		t.Fatalf("expected failed clean-worktree check, got: %+v", audit.Checks)
	}
}
