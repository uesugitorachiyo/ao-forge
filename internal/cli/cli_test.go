package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
		"execution remains disabled",
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

	for _, check := range []struct {
		name string
		doc  string
		want string
	}{
		{name: "README brief schema link", doc: readme, want: "[Factory Brief v0.1 Schema](docs/contracts/factory-brief-v0.1.schema.json)"},
		{name: "README plan schema link", doc: readme, want: "[Factory Plan v0.1 Schema](docs/contracts/factory-plan-v0.1.schema.json)"},
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
	} {
		var decoded any
		if err := json.Unmarshal([]byte(doc.text), &decoded); err != nil {
			t.Fatalf("%s is not valid JSON: %v", doc.name, err)
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

	code, stdout, stderr := runCLI("gate", "--plan", plan, "--decision-fixture", decision)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected gate fixture: %v", err)
	}
	assertJSONOutputEqual(t, "gate", expected, stdout)
	var result struct {
		SchemaVersion    string `json:"schema_version"`
		Status           string `json:"status"`
		PlanID           string `json:"plan_id"`
		ExecutionEnabled bool   `json:"execution_enabled"`
		Decision         struct {
			DecisionID  string `json:"decision_id"`
			Decision    string `json:"decision"`
			Explanation string `json:"explanation"`
		} `json:"decision"`
		NextActions []struct {
			ActionID string `json:"action_id"`
			Required bool   `json:"required"`
		} `json:"next_actions"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("gate output is not JSON: %v\n%s", err, stdout)
	}
	if result.SchemaVersion != "ao.forge.covenant-gate-result.v0.1" {
		t.Fatalf("schema_version = %q", result.SchemaVersion)
	}
	if result.Status != "allowed" {
		t.Fatalf("status = %q", result.Status)
	}
	if result.PlanID != "forge-plan-efedbfb309b1" {
		t.Fatalf("plan_id = %q", result.PlanID)
	}
	if result.ExecutionEnabled {
		t.Fatalf("execution_enabled = true, want false until AO2 adapter slice")
	}
	if result.Decision.Decision != "allow" || strings.TrimSpace(result.Decision.Explanation) == "" {
		t.Fatalf("decision = %+v", result.Decision)
	}
	if len(result.NextActions) == 0 || result.NextActions[0].ActionID != "implement-ao2-adapter" {
		t.Fatalf("next actions = %+v", result.NextActions)
	}
}

func TestGateDenyFixtureFailsClosed(t *testing.T) {
	root := repoRoot(t)
	plan := filepath.Join(root, "examples", "plans", "risky-pr-factory-plan.json")
	decision := filepath.Join(root, "examples", "decisions", "deny-release-mutation.decision.json")

	code, stdout, stderr := runCLI("gate", "--plan", plan, "--decision-fixture", decision)
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

func TestGateRequiresDecisionExplanation(t *testing.T) {
	root := repoRoot(t)
	plan := filepath.Join(root, "examples", "plans", "risky-pr-factory-plan.json")
	decision := filepath.Join(root, "examples", "decisions", "missing-explanation.decision.json")
	out := filepath.Join(t.TempDir(), "gate-result.json")

	code, stdout, stderr := runCLI("gate", "--plan", plan, "--decision-fixture", decision, "--out", out)
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

	code, stdout, stderr := runCLI("gate", "--plan", plan, "--decision-fixture", decision, "--out", out)
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

func TestRunAndOnceStayDisabledInSlice03(t *testing.T) {
	for _, command := range []string{"run", "once"} {
		code, stdout, stderr := runCLI(command)
		if code == 0 {
			t.Fatalf("%s exit code = 0, stdout = %s", command, stdout)
		}
		if !strings.Contains(stderr, "execution is disabled") {
			t.Fatalf("%s stderr missing disabled message: %s", command, stderr)
		}
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
