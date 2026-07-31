package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/uesugitorachiyo/ao-forge/internal/foundation"
)

const (
	briefSchemaVersion                   = "ao.forge.factory-brief.v0.1"
	planSchemaVersion                    = "ao.forge.factory-plan.v0.1"
	packetSchemaVersion                  = "ao.forge.factory-packet.v0.1"
	decisionFixtureSchemaVersion         = "ao.forge.covenant-decision-fixture.v0.1"
	gateResultSchemaVersion              = "ao.forge.covenant-gate-result.v0.1"
	releaseCandidateVersion              = "ao.forge.release-candidate.v0.1"
	releaseCandidateSchemaPath           = "docs/contracts/release-candidate-v0.1.schema.json"
	releasePreviewAuditVersion           = "ao.forge.release-preview-audit.v0.1"
	releasePreviewInspectVersion         = "ao.forge.release-preview-inspect.v0.1"
	productionReadinessVersion           = "ao.forge.production-readiness-audit.v0.1"
	goalRunSchemaVersion                 = "ao.forge.goal-run.v0.1"
	goalRunSchemaPath                    = "docs/contracts/goal-run-v0.1.schema.json"
	goalContextHandoffVersion            = "ao.forge.goal-run-context-handoff.v0.1"
	goalContextHandoffSchemaPath         = "docs/contracts/goal-run-context-handoff-v0.1.schema.json"
	goalVerificationVersion              = "ao.forge.goal-run-verification.v0.1"
	goalVerificationSchemaPath           = "docs/contracts/goal-run-verification-v0.1.schema.json"
	goalRunUpdateAuditVersion            = "ao.forge.goal-run-update-audit.v0.1"
	goalRunUpdateAuditSchemaPath         = "docs/contracts/goal-run-update-audit-v0.1.schema.json"
	goalRetainedEvidenceVersion          = "ao.forge.goal-run-retained-evidence.v0.1"
	goalRetainedEvidencePath             = "docs/contracts/goal-run-retained-evidence-v0.1.schema.json"
	goalEvidenceCleanupVersion           = "ao.forge.goal-run-retained-evidence-cleanup.v0.1"
	maxRetainedEvidenceCleanupFiles      = 4096
	maxRetainedEvidenceCleanupFileBytes  = 8 * 1024 * 1024
	maxRetainedEvidenceCleanupTotalBytes = 64 * 1024 * 1024
	liveDocsGuardVersion                 = "ao.forge.live-docs-execution-guard.v0.1"
)

var (
	buildVersion                = "dev"
	buildSourceCommit           = "unknown"
	planIDPattern               = regexp.MustCompile(`^forge-plan-[a-f0-9]{12}$`)
	windowsAbsolutePathPattern  = regexp.MustCompile(`^[A-Za-z]:/`)
	liveDocsPermittedClasses    = []string{"docs_only_single_file", "docs_only_multi_file"}
	liveTestPermittedClasses    = []string{"test_only"}
	liveLowRiskPermittedClasses = []string{"low_risk_code"}
	liveDocsLegacyAliases       = []string{"tiny_documentation_change"}
	liveDocsDeniedClasses       = []string{"docs_config_only", "test_only", "low_risk_code", "multi_repo_low_risk", "complex_repo_mutation"}
	liveTestDeniedClasses       = []string{"docs_config_only", "low_risk_code", "multi_repo_low_risk", "complex_repo_mutation"}
	liveLowRiskDeniedClasses    = []string{"docs_config_only", "multi_repo_low_risk", "complex_repo_mutation"}
)

type factoryBrief struct {
	SchemaVersion string `json:"schema_version"`
	Objective     struct {
		Text        string `json:"text"`
		Workspace   string `json:"workspace"`
		ReleaseMode bool   `json:"release_mode"`
	} `json:"objective"`
	Constraints struct {
		LocalFirst                  bool `json:"local_first"`
		AllowNetwork                bool `json:"allow_network"`
		AllowReleaseMutation        bool `json:"allow_release_mutation"`
		RequireControlPlaneReadback bool `json:"require_control_plane_readback"`
	} `json:"constraints"`
	ExpectedWorkcells []briefWorkcell `json:"expected_workcells"`
	ExpectedEvidence  []string        `json:"expected_evidence"`
}

type workcellRubric struct {
	RequiredPatterns  []string `json:"required_patterns,omitempty"`
	ForbiddenPatterns []string `json:"forbidden_patterns,omitempty"`
	MinCoverage       *float64 `json:"min_coverage,omitempty"`
}

type briefWorkcell struct {
	WorkcellID string          `json:"workcell_id"`
	Kind       string          `json:"kind"`
	Workspace  string          `json:"workspace,omitempty"`
	Executor   string          `json:"executor,omitempty"`
	Peers      int             `json:"peers,omitempty"`
	MaxRepairs int             `json:"max_repairs,omitempty"`
	Task       string          `json:"task,omitempty"`
	DependsOn  []string        `json:"depends_on"`
	Rubric     *workcellRubric `json:"rubric,omitempty"`
}

type factoryPlan struct {
	SchemaVersion    string             `json:"schema_version"`
	PlanID           string             `json:"plan_id"`
	Objective        factoryObjective   `json:"objective"`
	Constraints      factoryConstraints `json:"constraints"`
	ExecutionEnabled bool               `json:"execution_enabled"`
	PolicyGate       policyGate         `json:"policy_gate"`
	Workcells        []planWorkcell     `json:"workcells"`
	ExpectedEvidence []string           `json:"expected_evidence"`
	NextActions      []nextAction       `json:"next_actions"`
}

type factoryObjective struct {
	Text        string `json:"text"`
	Workspace   string `json:"workspace"`
	ReleaseMode bool   `json:"release_mode"`
}

type factoryConstraints struct {
	LocalFirst                  bool `json:"local_first"`
	AllowNetwork                bool `json:"allow_network"`
	AllowReleaseMutation        bool `json:"allow_release_mutation"`
	RequireControlPlaneReadback bool `json:"require_control_plane_readback"`
}

type policyGate struct {
	Required    bool   `json:"required"`
	Status      string `json:"status"`
	Explanation string `json:"explanation"`
}

type planWorkcell struct {
	WorkcellID string          `json:"workcell_id"`
	Kind       string          `json:"kind"`
	Workspace  string          `json:"workspace,omitempty"`
	Executor   string          `json:"executor,omitempty"`
	Peers      int             `json:"peers,omitempty"`
	MaxRepairs int             `json:"max_repairs,omitempty"`
	Task       string          `json:"task,omitempty"`
	Status     string          `json:"status"`
	DependsOn  []string        `json:"depends_on"`
	Rubric     *workcellRubric `json:"rubric,omitempty"`
}

type nextAction struct {
	ActionID    string `json:"action_id"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type releaseCandidate struct {
	SchemaVersion    string                  `json:"schema_version"`
	CandidateID      string                  `json:"candidate_id"`
	Status           string                  `json:"status"`
	Repository       releaseCandidateRepo    `json:"repository"`
	Gates            []releaseCandidateGate  `json:"gates"`
	PromotionHandoff releasePromotionHandoff `json:"promotion_handoff"`
	NextActions      []nextAction            `json:"next_actions"`
}

type releaseCandidateRepo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
}

type releaseCandidateGate struct {
	GateID   string   `json:"gate_id"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
}

type releasePromotionHandoff struct {
	Status   string   `json:"status"`
	Workflow string   `json:"workflow"`
	Requires []string `json:"requires"`
}

type covenantDecisionFixture struct {
	SchemaVersion string `json:"schema_version"`
	DecisionID    string `json:"decision_id"`
	TargetPlanID  string `json:"target_plan_id"`
	Decision      string `json:"decision"`
	Explanation   string `json:"explanation"`
	Source        string `json:"source"`
}

type covenantGateResult struct {
	SchemaVersion    string                  `json:"schema_version"`
	Status           string                  `json:"status"`
	PlanID           string                  `json:"plan_id"`
	ExecutionEnabled bool                    `json:"execution_enabled"`
	Decision         covenantDecisionFixture `json:"decision"`
	Problem          string                  `json:"problem,omitempty"`
	NextActions      []nextAction            `json:"next_actions"`
}

type factoryPacket struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Objective     struct {
		Text        string `json:"text"`
		Workspace   string `json:"workspace"`
		ReleaseMode bool   `json:"release_mode"`
	} `json:"objective"`
	FactoryPlan struct {
		PlanID        string `json:"plan_id"`
		WorkcellCount int    `json:"workcell_count"`
	} `json:"factory_plan"`
	PolicyDecisions []struct {
		DecisionID  string `json:"decision_id"`
		Target      string `json:"target"`
		Decision    string `json:"decision"`
		Explanation string `json:"explanation"`
		Source      string `json:"source"`
	} `json:"policy_decisions"`
	Workcells []struct {
		WorkcellID       string   `json:"workcell_id"`
		Kind             string   `json:"kind"`
		Workspace        string   `json:"workspace,omitempty"`
		Executor         string   `json:"executor,omitempty"`
		Peers            int      `json:"peers,omitempty"`
		MaxRepairs       int      `json:"max_repairs,omitempty"`
		Task             string   `json:"task,omitempty"`
		Status           string   `json:"status"`
		DependsOn        []string `json:"depends_on"`
		AO2Run           string   `json:"ao2_run"`
		Summary          string   `json:"summary"`
		RepairsAttempted int      `json:"repairs_attempted,omitempty"`
	} `json:"workcells"`
	Evidence []struct {
		Label         string `json:"label"`
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Path          string `json:"path"`
		SHA256        string `json:"sha256"`
	} `json:"evidence"`
	TrustBoundary struct {
		LocalFirst               bool `json:"local_first"`
		MutatesReleases          bool `json:"mutates_releases"`
		StoresCredentials        bool `json:"stores_credentials"`
		ControlPlaneApprovesWork bool `json:"control_plane_approves_work"`
	} `json:"trust_boundary"`
	NextActions []nextAction `json:"next_actions"`
}

type goalRun struct {
	SchemaVersion      string   `json:"schema_version"`
	GoalID             string   `json:"goal_id"`
	Repo               string   `json:"repo"`
	Objective          string   `json:"objective"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	AllowedScope       []string `json:"allowed_scope"`
	StopConditions     []string `json:"stop_conditions"`
	CurrentPhase       string   `json:"current_phase"`
	NextTask           string   `json:"next_task"`
	LastVerifiedAt     string   `json:"last_verified_at"`
	ContinuationPrompt string   `json:"continuation_prompt"`
	LoopOwner          struct {
		StateOwner string `json:"state_owner"`
		Executor   string `json:"executor"`
		Scheduler  string `json:"scheduler"`
	} `json:"loop_owner"`
	NextActionGuard struct {
		MustReadLatestGoalRun         bool   `json:"must_read_latest_goal_run"`
		MustMatchAllowedScope         bool   `json:"must_match_allowed_scope"`
		MustSatisfyAcceptanceCriteria bool   `json:"must_satisfy_acceptance_criteria"`
		OnMismatch                    string `json:"on_mismatch"`
	} `json:"next_action_guard"`
	LastIteration *goalRunIteration `json:"last_iteration,omitempty"`
}

type goalRunIteration struct {
	Status   string            `json:"status"`
	Summary  string            `json:"summary"`
	Evidence []goalRunEvidence `json:"evidence"`
}

type goalRunEvidence struct {
	Label  string `json:"label"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

type goalRetainedEvidenceArtifact struct {
	SchemaVersion   string `json:"schema_version"`
	GoalID          string `json:"goal_id"`
	Iteration       string `json:"iteration"`
	Phase           string `json:"phase"`
	Summary         string `json:"summary"`
	CapturedOutputs []struct {
		Label               string   `json:"label"`
		Command             string   `json:"command"`
		SchemaVersion       string   `json:"schema_version,omitempty"`
		GeneratedBy         string   `json:"generated_by,omitempty"`
		Status              string   `json:"status"`
		BaselineScore       *float64 `json:"baseline_score,omitempty"`
		CandidateScore      *float64 `json:"candidate_score,omitempty"`
		RequiredImprovement *float64 `json:"required_improvement_percent,omitempty"`
		ActualImprovement   *float64 `json:"actual_improvement_percent,omitempty"`
		AutonomousClaim     string   `json:"autonomous_claim,omitempty"`
		RSIMode             string   `json:"rsi_mode,omitempty"`
		RSICapability       string   `json:"rsi_capability,omitempty"`
		OperatorMode        string   `json:"operator_mode,omitempty"`
		ClaimLevels         []struct {
			Claim    string `json:"claim"`
			Decision string `json:"decision"`
			Status   string `json:"status"`
		} `json:"claim_levels,omitempty"`
		MutatesRepositories *bool `json:"mutates_repositories,omitempty"`
		Families            []struct {
			Family string `json:"family"`
			Status string `json:"status"`
			Passed bool   `json:"passed"`
		} `json:"families,omitempty"`
		EvidenceMarkers []string `json:"evidence_markers,omitempty"`
	} `json:"captured_outputs,omitempty"`
	RetentionPolicy struct {
		Layout                                 string `json:"layout"`
		TemporaryPathsAllowed                  bool   `json:"temporary_paths_allowed"`
		MinimumRetentionDaysAfterTerminalPhase int    `json:"minimum_retention_days_after_terminal_phase"`
	} `json:"retention_policy"`
	RetentionMetadata struct {
		RetainedAt             string   `json:"retained_at"`
		RetentionClass         string   `json:"retention_class"`
		RetainWhileGoalActive  bool     `json:"retain_while_goal_active"`
		DeletionRequiresReview bool     `json:"deletion_requires_review"`
		CleanupChangeMustName  []string `json:"cleanup_change_must_name"`
	} `json:"retention_metadata"`
}

var goalRunPhaseTransitions = map[string][]string{
	"planning":       {"implementation", "blocked", "backoff", "stopped"},
	"implementation": {"verification", "blocked", "backoff", "stopped"},
	"verification":   {"implementation", "complete", "blocked", "backoff", "stopped"},
	"blocked":        {"planning", "stopped"},
	"backoff":        {"planning", "stopped"},
	"stopped":        {},
	"complete":       {},
}

type planFlags struct {
	briefPath string
	outPath   string
	dynamic   bool
}

func parsePlanFlags(args []string) (planFlags, error) {
	var flags planFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--brief":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return planFlags{}, fmt.Errorf("--brief requires a value")
			}
			flags.briefPath = args[i+1]
			i++
		case "--out":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return planFlags{}, fmt.Errorf("--out requires a value")
			}
			flags.outPath = args[i+1]
			i++
		case "--dynamic":
			flags.dynamic = true
		case "--help", "-h":
			return planFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(arg, "--") {
				return planFlags{}, fmt.Errorf("unknown flag %s", arg)
			}
			return planFlags{}, fmt.Errorf("unexpected argument %s", arg)
		}
	}
	if flags.briefPath == "" {
		return planFlags{}, fmt.Errorf("missing required --brief")
	}
	return flags, nil
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	flags, err := parsePlanFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge plan: %v\n", err)
		return 2
	}
	if flags.dynamic && !archivedAgySwarmsEnabled() {
		fmt.Fprintln(stderr, "forge plan: --dynamic uses archived agy-swarms compatibility; set AO_FORGE_ENABLE_ARCHIVED_AGY_SWARMS=1 to opt in")
		return 1
	}

	brief, canonical, err := readBrief(flags.briefPath, flags.dynamic)
	if err != nil {
		fmt.Fprintf(stderr, "forge plan: %v\n", err)
		return 1
	}

	var plan factoryPlan
	if flags.dynamic {
		plan, err = buildDynamicPlan(context.Background(), brief, canonical)
		if err != nil {
			fmt.Fprintf(stderr, "forge plan: dynamic planning failed: %v\n", err)
			return 1
		}
	} else {
		plan = buildPlan(brief, canonical)
	}

	if err := validatePlan(plan); err != nil {
		fmt.Fprintf(stderr, "forge plan: generated plan failed contract validation: %v\n", err)
		return 1
	}
	encoded, err := marshalIndented(plan)
	if err != nil {
		fmt.Fprintf(stderr, "forge plan: encode plan: %v\n", err)
		return 1
	}

	if flags.outPath != "" {
		if err := writeFile(flags.outPath, encoded); err != nil {
			fmt.Fprintf(stderr, "forge plan: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "factory_plan=%s\n", flags.outPath)
		return 0
	}

	_, _ = stdout.Write(encoded)
	return 0
}

func archivedAgySwarmsEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("AO_FORGE_ENABLE_ARCHIVED_AGY_SWARMS")))
	return value == "1" || value == "true" || value == "yes"
}

func buildDynamicPlan(ctx context.Context, brief factoryBrief, canonicalBrief []byte) (factoryPlan, error) {
	cmdArgs, err := resolveAgySwarmsCommand()
	if err != nil {
		return factoryPlan{}, fmt.Errorf("failed to resolve agy-swarms command: %v", err)
	}

	workspacePath := brief.Objective.Workspace

	prompt := fmt.Sprintf(
		"Analyze the target workspace structure (files, directories, package managers) to understand the codebase.\n"+
			"Decompose the objective into a sequence of workcells to achieve the objective.\n\n"+
			"Objective: %s\n\n"+
			"Each workcell must follow this JSON schema:\n"+
			"{\n"+
			"  \"workcell_id\": \"string (unique identifier)\",\n"+
			"  \"kind\": \"prepare\" | \"execute\" | \"verify\" | \"close\",\n"+
			"  \"workspace\": \"string (optional path relative to workspace root)\",\n"+
			"  \"executor\": \"ao2\" | \"agy-swarms\",\n"+
			"  \"peers\": integer (optional),\n"+
			"  \"max_repairs\": integer (optional),\n"+
			"  \"task\": \"string\",\n"+
			"  \"depends_on\": [\"array of dependency workcell_ids\"],\n"+
			"  \"rubric\": {\n"+
			"    \"required_patterns\": [\"array of strings\"],\n"+
			"    \"forbidden_patterns\": [\"array of strings\"],\n"+
			"    \"min_coverage\": number\n"+
			"  }\n"+
			"}\n\n"+
			"Write the resulting list of workcells as a JSON array to 'dynamic-plan-workcells.json' in the current workspace directory. Do not output anything else.",
		brief.Objective.Text,
	)

	taskSpec := map[string]interface{}{
		"task": prompt,
		"model_pins": map[string]string{
			"default": "gemini-3.5-flash",
		},
	}
	taskData, err := json.MarshalIndent(taskSpec, "", "  ")
	if err != nil {
		return factoryPlan{}, fmt.Errorf("failed to marshal dynamic planning task: %v", err)
	}

	tempTask, err := os.CreateTemp("", "agy-dynamic-task-*.json")
	if err != nil {
		return factoryPlan{}, fmt.Errorf("failed to create temp task: %v", err)
	}
	defer os.Remove(tempTask.Name())

	if _, err := tempTask.Write(taskData); err != nil {
		tempTask.Close()
		return factoryPlan{}, fmt.Errorf("failed to write temp task: %v", err)
	}
	tempTask.Close()

	tempReport, err := os.CreateTemp("", "agy-dynamic-report-*.json")
	if err != nil {
		return factoryPlan{}, fmt.Errorf("failed to create temp report: %v", err)
	}
	tempReport.Close()
	defer os.Remove(tempReport.Name())

	args := append(cmdArgs[1:], "run", "--task", tempTask.Name(), "--report", tempReport.Name(), "--allow-local-commands", "--reviewer", "agy", "--closer", "agy")

	cmdName := cmdArgs[0]
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = workspacePath

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	if runErr != nil {
		return factoryPlan{}, fmt.Errorf("dynamic planner swarm execution failed: %v (stderr: %q)", runErr, stderrBuf.String())
	}

	outputPath := filepath.Join(workspacePath, "dynamic-plan-workcells.json")
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return factoryPlan{}, fmt.Errorf("failed to read dynamic planner output 'dynamic-plan-workcells.json': %v", err)
	}
	defer os.Remove(outputPath)

	var parsedWorkcells []planWorkcell
	if err := json.Unmarshal(outputData, &parsedWorkcells); err != nil {
		return factoryPlan{}, fmt.Errorf("failed to parse dynamic planner output: %v (raw data: %q)", err, string(outputData))
	}

	for i := range parsedWorkcells {
		if parsedWorkcells[i].Status == "" {
			parsedWorkcells[i].Status = "planned"
		}
		if parsedWorkcells[i].DependsOn == nil {
			parsedWorkcells[i].DependsOn = []string{}
		}
	}

	plan := factoryPlan{
		SchemaVersion: planSchemaVersion,
		PlanID:        planID(canonicalBrief),
		Objective: factoryObjective{
			Text:        brief.Objective.Text,
			Workspace:   brief.Objective.Workspace,
			ReleaseMode: brief.Objective.ReleaseMode,
		},
		Constraints: factoryConstraints{
			LocalFirst:                  brief.Constraints.LocalFirst,
			AllowNetwork:                brief.Constraints.AllowNetwork,
			AllowReleaseMutation:        brief.Constraints.AllowReleaseMutation,
			RequireControlPlaneReadback: brief.Constraints.RequireControlPlaneReadback,
		},
		ExecutionEnabled: false,
		PolicyGate: policyGate{
			Required:    true,
			Status:      "not_requested",
			Explanation: "Slice 0.2 creates deterministic plans only; Covenant gate execution is introduced in slice 0.3.",
		},
		Workcells:        parsedWorkcells,
		ExpectedEvidence: cloneStrings(brief.ExpectedEvidence),
		NextActions: []nextAction{
			{
				ActionID:    "run-covenant-gate",
				Description: "Implement or invoke the Covenant gate before AO2 execution is enabled.",
				Required:    true,
			},
		},
	}

	return plan, nil
}

func runGate(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGateFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge gate: %v\n", err)
		return 2
	}
	plan, err := readPlan(flags.planPath)
	if err != nil {
		result := blockedGateResult("", covenantDecisionFixture{}, err.Error())
		return writeGateResult(result, flags.outPath, stdout, stderr, 1)
	}

	// Determine if covenantPath points to a JSON file or a binary
	isJSON := false
	if info, err := os.Stat(flags.covenantPath); err == nil && !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(flags.covenantPath), ".json") {
			isJSON = true
		} else {
			if data, err := os.ReadFile(flags.covenantPath); err == nil {
				trimmed := strings.TrimSpace(string(data))
				if strings.HasPrefix(trimmed, "{") {
					isJSON = true
				}
			}
		}
	}

	var decision covenantDecisionFixture

	if isJSON {
		decision, err = readDecisionFixture(flags.covenantPath)
		if err != nil {
			result := blockedGateResult(plan.PlanID, covenantDecisionFixture{Decision: "invalid"}, err.Error())
			return writeGateResult(result, flags.outPath, stdout, stderr, 1)
		}
		if err := validateDecisionFixture(plan, decision); err != nil {
			decision.Decision = "invalid"
			result := blockedGateResult(plan.PlanID, decision, err.Error())
			return writeGateResult(result, flags.outPath, stdout, stderr, 1)
		}
	} else {
		// Verify if the covenant binary executes successfully by running "<covenant> version --json"
		cmd := exec.Command(flags.covenantPath, "version", "--json")
		var cmdOut, cmdErr bytes.Buffer
		cmd.Stdout = &cmdOut
		cmd.Stderr = &cmdErr
		if err := cmd.Run(); err != nil {
			errStr := fmt.Sprintf("Covenant binary is unavailable: %v", err)
			if cmdErr.Len() > 0 {
				errStr = fmt.Sprintf("Covenant binary is unavailable: %v (stderr: %q)", err, cmdErr.String())
			}
			result := blockedGateResult(plan.PlanID, covenantDecisionFixture{Decision: "invalid"}, errStr)
			return writeGateResult(result, flags.outPath, stdout, stderr, 1)
		}

		var verInfo struct {
			SchemaVersion string `json:"schema_version"`
		}
		if err := json.Unmarshal(cmdOut.Bytes(), &verInfo); err != nil || verInfo.SchemaVersion != "covenant.version-result.v1" {
			result := blockedGateResult(plan.PlanID, covenantDecisionFixture{Decision: "invalid"}, "Covenant binary version check failed")
			return writeGateResult(result, flags.outPath, stdout, stderr, 1)
		}

		decision = covenantDecisionFixture{
			SchemaVersion: decisionFixtureSchemaVersion,
			TargetPlanID:  plan.PlanID,
			Source:        "live-covenant-adapter",
		}

		if plan.Objective.ReleaseMode {
			workspaces := []string{plan.Objective.Workspace}
			seen := map[string]bool{plan.Objective.Workspace: true}
			for _, wc := range plan.Workcells {
				if wc.Workspace != "" && !seen[wc.Workspace] {
					workspaces = append(workspaces, wc.Workspace)
					seen[wc.Workspace] = true
				}
			}
			for _, ws := range workspaces {
				if err := verifyReleaseWorkspace(ws); err != nil {
					decision.DecisionID = "deny-dirty-release-workspace"
					decision.Decision = "deny"
					decision.Explanation = fmt.Sprintf("Release workspace validation failed for %q: %v", ws, err)
					result := blockedGateResult(plan.PlanID, decision, decision.Explanation)
					return writeGateResult(result, flags.outPath, stdout, stderr, 1)
				}
			}
		}

		if plan.Constraints.AllowReleaseMutation {
			if !plan.Objective.ReleaseMode {
				decision.DecisionID = "deny-non-release-mutation"
				decision.Decision = "deny"
				decision.Explanation = "The plan requests release mutation, but Release Mode is disabled."
			} else {
				decision.DecisionID = "indeterminate-release-mutation"
				decision.Decision = "indeterminate"
				decision.Explanation = "The plan requests release mutation in Release Mode, which requires explicit operator override."
			}
		} else if plan.Constraints.AllowNetwork {
			decision.DecisionID = "indeterminate-network-access"
			decision.Decision = "indeterminate"
			decision.Explanation = "The plan requests network access, which requires operator approval and is indeterminate under standard policy."
		} else {
			decision.DecisionID = "allow-local-plan"
			decision.Decision = "allow"
			decision.Explanation = "The plan is local-first, does not allow network access, and does not mutate releases. Covenant binary version check passed."
		}
	}

	switch decision.Decision {
	case "allow":
		result := covenantGateResult{
			SchemaVersion:    gateResultSchemaVersion,
			Status:           "allowed",
			PlanID:           plan.PlanID,
			ExecutionEnabled: false,
			Decision:         decision,
			NextActions: []nextAction{
				{
					ActionID:    "implement-ao2-adapter",
					Description: "The local Covenant gate allowed the plan; AO2 execution remains disabled until Slice 0.4.",
					Required:    true,
				},
			},
		}
		return writeGateResult(result, flags.outPath, stdout, stderr, 0)
	case "deny":
		result := covenantGateResult{
			SchemaVersion:    gateResultSchemaVersion,
			Status:           "denied",
			PlanID:           plan.PlanID,
			ExecutionEnabled: false,
			Decision:         decision,
			NextActions: []nextAction{
				{
					ActionID:    "revise-plan-or-stop",
					Description: "The Covenant gate denied the plan; do not execute AO2 work.",
					Required:    true,
				},
			},
		}
		return writeGateResult(result, flags.outPath, stdout, stderr, 1)
	case "indeterminate":
		result := covenantGateResult{
			SchemaVersion:    gateResultSchemaVersion,
			Status:           "blocked",
			PlanID:           plan.PlanID,
			ExecutionEnabled: false,
			Decision:         decision,
			NextActions: []nextAction{
				{
					ActionID:    "request-operator-approval",
					Description: "The Covenant gate returned indeterminate decision; request operator review or attach approval ticket.",
					Required:    true,
				},
			},
		}
		return writeGateResult(result, flags.outPath, stdout, stderr, 1)
	default:
		result := blockedGateResult(plan.PlanID, covenantDecisionFixture{Decision: "invalid"}, "decision must be allow, deny, or indeterminate")
		return writeGateResult(result, flags.outPath, stdout, stderr, 1)
	}
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	packetPath, _, err := parsePathFlags(args, "--packet", "")
	if err != nil {
		fmt.Fprintf(stderr, "forge inspect: %v\n", err)
		return 2
	}
	if packetPath == "" {
		fmt.Fprintln(stderr, "forge inspect: missing required --packet")
		return 2
	}

	packet, err := readPacket(packetPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge inspect: %v\n", err)
		return 1
	}
	displayPath := displayPath(packetPath)
	fmt.Fprintf(stdout, "factory_packet=%s\n", displayPath)
	fmt.Fprintf(stdout, "schema_version=%s\n", packet.SchemaVersion)
	fmt.Fprintf(stdout, "status=%s\n", packet.Status)
	fmt.Fprintf(stdout, "objective=%s\n", packet.Objective.Text)
	fmt.Fprintf(stdout, "workspace=%s\n", packet.Objective.Workspace)
	fmt.Fprintf(stdout, "plan_id=%s\n", packet.FactoryPlan.PlanID)
	fmt.Fprintf(stdout, "workcells=%d\n", len(packet.Workcells))
	fmt.Fprintf(stdout, "policy_decisions=%d\n", len(packet.PolicyDecisions))
	fmt.Fprintf(stdout, "evidence=%d\n", len(packet.Evidence))
	for _, action := range packet.NextActions {
		fmt.Fprintf(stdout, "next_action=%s required=%t description=%s\n", action.ActionID, action.Required, action.Description)
	}
	return 0
}

type gateFlags struct {
	planPath     string
	covenantPath string
	outPath      string
}

func parseGateFlags(args []string) (gateFlags, error) {
	var flags gateFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--plan":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return gateFlags{}, fmt.Errorf("--plan requires a value")
			}
			flags.planPath = args[i+1]
			i++
		case "--covenant":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return gateFlags{}, fmt.Errorf("--covenant requires a value")
			}
			flags.covenantPath = args[i+1]
			i++
		case "--out":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return gateFlags{}, fmt.Errorf("--out requires a value")
			}
			flags.outPath = args[i+1]
			i++
		default:
			if strings.HasPrefix(arg, "--") {
				return gateFlags{}, fmt.Errorf("unknown flag %s", arg)
			}
			return gateFlags{}, fmt.Errorf("unexpected argument %s", arg)
		}
	}
	if flags.planPath == "" {
		return gateFlags{}, fmt.Errorf("missing required --plan")
	}
	if flags.covenantPath == "" {
		return gateFlags{}, fmt.Errorf("missing required --covenant")
	}
	return flags, nil
}

func parsePathFlags(args []string, required string, optional string) (string, string, error) {
	var requiredValue string
	var optionalValue string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case required:
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", "", fmt.Errorf("%s requires a value", required)
			}
			requiredValue = args[i+1]
			i++
		case optional:
			if optional == "" {
				return "", "", fmt.Errorf("unknown flag %s", arg)
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", "", fmt.Errorf("%s requires a value", optional)
			}
			optionalValue = args[i+1]
			i++
		case "--help", "-h":
			return "", "", fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(arg, "--") {
				return "", "", fmt.Errorf("unknown flag %s", arg)
			}
			return "", "", fmt.Errorf("unexpected argument %s", arg)
		}
	}
	return requiredValue, optionalValue, nil
}

func readBrief(path string, allowEmptyWorkcells bool) (factoryBrief, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return factoryBrief{}, nil, fmt.Errorf("read brief: %w", err)
	}

	var brief factoryBrief
	var canonical []byte

	if strings.HasSuffix(strings.ToLower(path), ".md") {
		var parseErr error
		brief, parseErr = parseMarkdownBrief(data)
		if parseErr != nil {
			return factoryBrief{}, nil, fmt.Errorf("parse markdown brief: %w", parseErr)
		}
		rawBytes, err := json.Marshal(brief)
		if err != nil {
			return factoryBrief{}, nil, fmt.Errorf("marshal parsed brief: %w", err)
		}
		var raw any
		if err := json.Unmarshal(rawBytes, &raw); err != nil {
			return factoryBrief{}, nil, fmt.Errorf("canonicalize parsed brief JSON: %w", err)
		}
		canonical, err = json.Marshal(raw)
		if err != nil {
			return factoryBrief{}, nil, fmt.Errorf("canonicalize parsed brief: %w", err)
		}
	} else {
		var raw any
		if err := json.Unmarshal(data, &raw); err != nil {
			return factoryBrief{}, nil, fmt.Errorf("parse brief JSON: %w", err)
		}
		var err error
		canonical, err = json.Marshal(raw)
		if err != nil {
			return factoryBrief{}, nil, fmt.Errorf("canonicalize brief JSON: %w", err)
		}
		if err := validateBriefRequiredFields(data, allowEmptyWorkcells); err != nil {
			return factoryBrief{}, nil, err
		}
		if err := decodeJSONStrict(data, &brief); err != nil {
			return factoryBrief{}, nil, fmt.Errorf("decode brief: %w", err)
		}
	}

	if brief.SchemaVersion != briefSchemaVersion {
		return factoryBrief{}, nil, fmt.Errorf("unsupported brief schema_version %q", brief.SchemaVersion)
	}
	if strings.TrimSpace(brief.Objective.Text) == "" {
		return factoryBrief{}, nil, fmt.Errorf("brief objective.text is required")
	}
	if strings.TrimSpace(brief.Objective.Workspace) == "" {
		return factoryBrief{}, nil, fmt.Errorf("brief objective.workspace is required")
	}
	if !allowEmptyWorkcells && len(brief.ExpectedWorkcells) == 0 {
		return factoryBrief{}, nil, fmt.Errorf("brief expected_workcells must not be empty")
	}
	return brief, canonical, nil
}

func validateBriefRequiredFields(data []byte, allowEmptyWorkcells bool) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse brief JSON: %w", err)
	}

	var missing []string
	if allowEmptyWorkcells {
		requireFields(&missing, root, "", "schema_version", "objective", "constraints", "expected_evidence")
	} else {
		requireFields(&missing, root, "", "schema_version", "objective", "constraints", "expected_workcells", "expected_evidence")
	}

	if raw, ok := root["objective"]; ok {
		objective, err := decodeObject(raw, "brief objective")
		if err != nil {
			return err
		}
		requireFields(&missing, objective, "objective", "text", "workspace", "release_mode")
	}
	if raw, ok := root["constraints"]; ok {
		constraints, err := decodeObject(raw, "brief constraints")
		if err != nil {
			return err
		}
		requireFields(&missing, constraints, "constraints", "local_first", "allow_network", "allow_release_mutation", "require_control_plane_readback")
	}
	if raw, ok := root["expected_workcells"]; ok {
		var workcells []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &workcells); err != nil || workcells == nil {
			return fmt.Errorf("brief expected_workcells must be an array")
		}
		for i, workcell := range workcells {
			if workcell == nil {
				return fmt.Errorf("brief expected_workcells[%d] must be an object", i)
			}
			requireFields(&missing, workcell, fmt.Sprintf("expected_workcells[%d]", i), "workcell_id", "kind", "depends_on")
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("%s", strings.Join(missing, "; "))
	}
	return nil
}

func decodeObject(raw json.RawMessage, label string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be an object", label)
	}
	return object, nil
}

func requireFields(missing *[]string, object map[string]json.RawMessage, prefix string, fields ...string) {
	for _, field := range fields {
		if _, ok := object[field]; ok {
			continue
		}
		path := field
		if prefix != "" {
			path = prefix + "." + field
		}
		*missing = append(*missing, fmt.Sprintf("brief %s is required", path))
	}
}

func readPlan(path string) (factoryPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return factoryPlan{}, fmt.Errorf("read plan: %w", err)
	}
	var plan factoryPlan
	if err := decodeJSONStrict(data, &plan); err != nil {
		return factoryPlan{}, fmt.Errorf("parse plan JSON: %w", err)
	}
	if err := validatePlan(plan); err != nil {
		return factoryPlan{}, err
	}
	return plan, nil
}

func validatePlan(plan factoryPlan) error {
	if plan.SchemaVersion != planSchemaVersion {
		return fmt.Errorf("unsupported plan schema_version %q", plan.SchemaVersion)
	}
	if !planIDPattern.MatchString(plan.PlanID) {
		return fmt.Errorf("plan_id must match forge-plan-<12 lowercase hex chars>")
	}
	if strings.TrimSpace(plan.Objective.Text) == "" {
		return fmt.Errorf("objective.text is required")
	}
	if strings.TrimSpace(plan.Objective.Workspace) == "" {
		return fmt.Errorf("objective.workspace is required")
	}
	if plan.ExecutionEnabled {
		return fmt.Errorf("execution_enabled must remain false until the AO2 adapter slice")
	}
	if !plan.PolicyGate.Required {
		return fmt.Errorf("policy_gate.required must be true")
	}
	if strings.TrimSpace(plan.PolicyGate.Status) == "" {
		return fmt.Errorf("policy_gate.status is required")
	}
	if strings.TrimSpace(plan.PolicyGate.Explanation) == "" {
		return fmt.Errorf("policy_gate.explanation is required")
	}
	if len(plan.Workcells) == 0 {
		return fmt.Errorf("plan workcells must not be empty")
	}
	for _, workcell := range plan.Workcells {
		if strings.TrimSpace(workcell.WorkcellID) == "" {
			return fmt.Errorf("workcell_id is required")
		}
		if !validWorkcellKind(workcell.Kind) {
			return fmt.Errorf("workcell %s kind must be prepare, execute, verify, or close", workcell.WorkcellID)
		}
		if strings.TrimSpace(workcell.Status) == "" {
			return fmt.Errorf("workcell %s status is required", workcell.WorkcellID)
		}
		if workcell.DependsOn == nil {
			return fmt.Errorf("workcell %s depends_on must be an array", workcell.WorkcellID)
		}
	}

	// Validate dependencies exist and detect cycles
	cellMap := make(map[string][]string)
	for _, wc := range plan.Workcells {
		cellMap[wc.WorkcellID] = wc.DependsOn
	}

	for _, wc := range plan.Workcells {
		for _, dep := range wc.DependsOn {
			if _, exists := cellMap[dep]; !exists {
				return fmt.Errorf("workcell %q depends on non-existent workcell %q", wc.WorkcellID, dep)
			}
		}
	}

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(node string) bool
	dfs = func(node string) bool {
		if recStack[node] {
			return true
		}
		if visited[node] {
			return false
		}
		recStack[node] = true
		for _, dep := range cellMap[node] {
			if dfs(dep) {
				return true
			}
		}
		recStack[node] = false
		visited[node] = true
		return false
	}

	for _, wc := range plan.Workcells {
		if dfs(wc.WorkcellID) {
			return fmt.Errorf("cyclic dependency detected involving workcell %q", wc.WorkcellID)
		}
	}

	if len(plan.ExpectedEvidence) == 0 {
		return fmt.Errorf("expected_evidence must not be empty")
	}
	for _, evidence := range plan.ExpectedEvidence {
		if strings.TrimSpace(evidence) == "" {
			return fmt.Errorf("expected_evidence entries must not be empty")
		}
	}
	if len(plan.NextActions) == 0 {
		return fmt.Errorf("next_actions must not be empty")
	}
	for _, action := range plan.NextActions {
		if strings.TrimSpace(action.ActionID) == "" {
			return fmt.Errorf("next action action_id is required")
		}
		if strings.TrimSpace(action.Description) == "" {
			return fmt.Errorf("next action %s description is required", action.ActionID)
		}
	}
	return nil
}

func validWorkcellKind(kind string) bool {
	switch kind {
	case "prepare", "execute", "verify", "close":
		return true
	default:
		return false
	}
}

func readDecisionFixture(path string) (covenantDecisionFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return covenantDecisionFixture{}, fmt.Errorf("read decision fixture: %w", err)
	}
	var decision covenantDecisionFixture
	if err := decodeJSONStrict(data, &decision); err != nil {
		return covenantDecisionFixture{}, fmt.Errorf("parse decision fixture JSON")
	}
	if decision.SchemaVersion != decisionFixtureSchemaVersion {
		return covenantDecisionFixture{}, fmt.Errorf("unsupported decision fixture schema_version %q", decision.SchemaVersion)
	}
	return decision, nil
}

func validateDecisionFixture(plan factoryPlan, decision covenantDecisionFixture) error {
	if strings.TrimSpace(decision.DecisionID) == "" {
		return fmt.Errorf("decision_id is required")
	}
	if decision.TargetPlanID != plan.PlanID {
		return fmt.Errorf("decision target_plan_id does not match plan_id")
	}
	if decision.Decision != "allow" && decision.Decision != "deny" && decision.Decision != "indeterminate" {
		return fmt.Errorf("decision must be allow, deny, or indeterminate")
	}
	if strings.TrimSpace(decision.Explanation) == "" {
		return fmt.Errorf("explanation is required")
	}
	return nil
}

func blockedGateResult(planID string, decision covenantDecisionFixture, problem string) covenantGateResult {
	if strings.TrimSpace(decision.Decision) == "" {
		decision.Decision = "invalid"
	}
	return covenantGateResult{
		SchemaVersion:    gateResultSchemaVersion,
		Status:           "blocked",
		PlanID:           planID,
		ExecutionEnabled: false,
		Decision:         decision,
		Problem:          problem,
		NextActions: []nextAction{
			{
				ActionID:    "fix-covenant-gate",
				Description: "Ensure the Covenant binary is available, the plan is valid, and any decision is resolved.",
				Required:    true,
			},
		},
	}
}

func writeGateResult(result covenantGateResult, outPath string, stdout, stderr io.Writer, code int) int {
	encoded, err := marshalIndented(result)
	if err != nil {
		fmt.Fprintf(stderr, "forge gate: encode result: %v\n", err)
		return 1
	}
	if outPath != "" {
		if err := writeFile(outPath, encoded); err != nil {
			fmt.Fprintf(stderr, "forge gate: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "covenant_gate=%s\n", outPath)
	} else {
		_, _ = stdout.Write(encoded)
	}
	if code != 0 {
		if result.Status == "denied" {
			fmt.Fprintf(stderr, "forge gate: decision denied: %s\n", result.Decision.Explanation)
		} else {
			fmt.Fprintf(stderr, "forge gate: %s\n", result.Problem)
		}
	}
	return code
}

func buildPlan(brief factoryBrief, canonicalBrief []byte) factoryPlan {
	workcells := make([]planWorkcell, 0, len(brief.ExpectedWorkcells))
	for _, cell := range brief.ExpectedWorkcells {
		workcells = append(workcells, planWorkcell{
			WorkcellID: cell.WorkcellID,
			Kind:       cell.Kind,
			Workspace:  cell.Workspace,
			Executor:   cell.Executor,
			Peers:      cell.Peers,
			MaxRepairs: cell.MaxRepairs,
			Task:       cell.Task,
			Status:     "planned",
			DependsOn:  cloneStrings(cell.DependsOn),
			Rubric:     cell.Rubric,
		})
	}
	return factoryPlan{
		SchemaVersion: planSchemaVersion,
		PlanID:        planID(canonicalBrief),
		Objective: factoryObjective{
			Text:        brief.Objective.Text,
			Workspace:   brief.Objective.Workspace,
			ReleaseMode: brief.Objective.ReleaseMode,
		},
		Constraints: factoryConstraints{
			LocalFirst:                  brief.Constraints.LocalFirst,
			AllowNetwork:                brief.Constraints.AllowNetwork,
			AllowReleaseMutation:        brief.Constraints.AllowReleaseMutation,
			RequireControlPlaneReadback: brief.Constraints.RequireControlPlaneReadback,
		},
		ExecutionEnabled: false,
		PolicyGate: policyGate{
			Required:    true,
			Status:      "not_requested",
			Explanation: "Slice 0.2 creates deterministic plans only; Covenant gate execution is introduced in slice 0.3.",
		},
		Workcells:        workcells,
		ExpectedEvidence: cloneStrings(brief.ExpectedEvidence),
		NextActions: []nextAction{
			{
				ActionID:    "run-covenant-gate",
				Description: "Implement or invoke the Covenant gate before AO2 execution is enabled.",
				Required:    true,
			},
		},
	}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string{}, values...)
}

func planID(canonicalBrief []byte) string {
	sum := sha256.Sum256(canonicalBrief)
	return "forge-plan-" + hex.EncodeToString(sum[:])[:12]
}

func readPacket(path string) (factoryPacket, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return factoryPacket{}, fmt.Errorf("read packet: %w", err)
	}
	var packet factoryPacket
	if err := decodeJSONStrict(data, &packet); err != nil {
		return factoryPacket{}, fmt.Errorf("parse packet JSON: %w", err)
	}
	if packet.SchemaVersion != packetSchemaVersion {
		return factoryPacket{}, fmt.Errorf("unsupported packet schema_version %q", packet.SchemaVersion)
	}
	if strings.TrimSpace(packet.Status) == "" {
		return factoryPacket{}, fmt.Errorf("packet status is required")
	}
	if strings.TrimSpace(packet.Objective.Text) == "" {
		return factoryPacket{}, fmt.Errorf("packet objective.text is required")
	}
	return packet, nil
}

func marshalIndented(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func decodeJSONStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func displayPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	if root, ok := findRepoRoot(); ok {
		if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func findRepoRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	var foundationPath string
	var isJson bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--foundation":
			if i+1 < len(args) {
				foundationPath = args[i+1]
				i++
			} else {
				fmt.Fprintln(stderr, "forge doctor: --foundation requires a value")
				return 2
			}
		case "--json":
			isJson = true
		case "--help", "-h":
			fmt.Fprintln(stderr, "help is available with `forge --help`")
			return 2
		default:
			if strings.HasPrefix(args[i], "--") {
				fmt.Fprintf(stderr, "forge doctor: unknown flag %s\n", args[i])
				return 2
			}
			fmt.Fprintf(stderr, "forge doctor: unexpected argument %s\n", args[i])
			return 2
		}
	}

	if foundationPath == "" {
		fmt.Fprintln(stderr, "forge doctor: missing required --foundation")
		return 2
	}

	result, err := foundation.RunDoctor(foundationPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge doctor: %v\n", err)
		return 2
	}

	if isJson {
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "forge doctor: json marshal: %v\n", err)
			return 2
		}
		_, _ = stdout.Write(encoded)
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintf(stdout, "AO Forge Foundation Doctor Status: %s\n", strings.ToUpper(result.Status))
		for _, comp := range result.Components {
			statusStr := "PASSED"
			if !comp.Exists {
				statusStr = "MISSING"
			} else if !comp.GitDir {
				statusStr = "INVALID (Not Git)"
			} else if !comp.BranchOK || !comp.CommitOK || !comp.WorktreeClean || !comp.ReleaseTagExists {
				var checks []string
				if !comp.BranchOK {
					checks = append(checks, fmt.Sprintf("branch=%s(expected %s)", comp.Branch, comp.ExpectedBranch))
				}
				if !comp.CommitOK {
					checks = append(checks, fmt.Sprintf("commit=%s(expected %s)", truncateCommit(comp.Commit, 12), truncateCommit(comp.ExpectedCommit, 12)))
				}
				if !comp.WorktreeClean {
					checks = append(checks, "dirty")
				}
				if !comp.ReleaseTagExists {
					checks = append(checks, fmt.Sprintf("tag %s missing", comp.ExpectedRelease))
				}
				statusStr = fmt.Sprintf("FAILED (%s)", strings.Join(checks, ", "))
			}
			fmt.Fprintf(stdout, "  - %s: %s\n", comp.Name, statusStr)
		}
		if result.Status == "failed" {
			fmt.Fprintf(stdout, "\nProblem: %s\n", result.Problem)
		}
	}

	if result.Status == "passed" {
		return 0
	}
	return 1
}

func parseMarkdownBrief(data []byte) (factoryBrief, error) {
	var brief factoryBrief
	brief.SchemaVersion = "ao.forge.factory-brief.v0.1"
	brief.ExpectedWorkcells = []briefWorkcell{}
	brief.ExpectedEvidence = []string{}

	lines := strings.Split(string(data), "\n")
	currentSection := ""

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		if strings.HasPrefix(trimmedLine, "#") {
			headerText := strings.ToLower(strings.TrimSpace(strings.TrimLeft(trimmedLine, "#")))
			switch headerText {
			case "objective":
				currentSection = "objective"
			case "workspace":
				currentSection = "workspace"
			case "constraints":
				currentSection = "constraints"
			case "expected workcells", "expected_workcells", "workcells":
				currentSection = "workcells"
			case "expected evidence", "expected_evidence", "evidence":
				currentSection = "evidence"
			default:
				currentSection = ""
			}
			continue
		}

		switch currentSection {
		case "objective":
			if brief.Objective.Text == "" {
				brief.Objective.Text = trimmedLine
			} else {
				brief.Objective.Text += " " + trimmedLine
			}
		case "workspace":
			brief.Objective.Workspace = trimmedLine
		case "constraints":
			if strings.HasPrefix(trimmedLine, "-") || strings.HasPrefix(trimmedLine, "*") {
				content := strings.TrimSpace(strings.TrimLeft(trimmedLine, "-*"))
				parts := strings.SplitN(content, ":", 2)
				if len(parts) == 2 {
					key := strings.ToLower(strings.TrimSpace(parts[0]))
					val := strings.ToLower(strings.TrimSpace(parts[1]))
					boolVal := (val == "true")
					switch key {
					case "local first", "local_first":
						brief.Constraints.LocalFirst = boolVal
					case "allow network", "allow_network":
						brief.Constraints.AllowNetwork = boolVal
					case "allow release mutation", "allow_release_mutation":
						brief.Constraints.AllowReleaseMutation = boolVal
					case "require control plane readback", "require_control_plane_readback":
						brief.Constraints.RequireControlPlaneReadback = boolVal
					case "release mode", "release_mode":
						brief.Objective.ReleaseMode = boolVal
					}
				}
			}
		case "workcells":
			if strings.HasPrefix(trimmedLine, "-") || strings.HasPrefix(trimmedLine, "*") {
				prefixRegex := regexp.MustCompile(`^\s*[-\*]\s*([a-zA-Z0-9_-]+)\s*\((prepare|execute|verify|close)\)(.*)$`)
				matches := prefixRegex.FindStringSubmatch(trimmedLine)
				if len(matches) >= 3 {
					wcID := matches[1]
					wcKind := matches[2]
					remainder := matches[3]

					wcTask := ""
					if idx := strings.Index(remainder, `task: "`); idx != -1 {
						start := idx + len(`task: "`)
						if end := strings.Index(remainder[start:], `"`); end != -1 {
							wcTask = remainder[start : start+end]
							remainder = remainder[:idx] + remainder[start+end+1:]
						}
					}

					wcExecutor := ""
					if execMatches := regexp.MustCompile(`\bexecutor:\s*([a-zA-Z0-9_-]+)`).FindStringSubmatch(remainder); len(execMatches) > 1 {
						wcExecutor = execMatches[1]
					}

					wcPeers := 0
					if peersMatches := regexp.MustCompile(`\bpeers:\s*([0-9]+)`).FindStringSubmatch(remainder); len(peersMatches) > 1 {
						if p, err := strconv.Atoi(peersMatches[1]); err == nil {
							wcPeers = p
						}
					}

					wcMaxRepairs := 0
					if maxRepairsMatches := regexp.MustCompile(`\b(?:max_repairs|max-repairs):\s*([0-9]+)`).FindStringSubmatch(remainder); len(maxRepairsMatches) > 1 {
						if r, err := strconv.Atoi(maxRepairsMatches[1]); err == nil {
							wcMaxRepairs = r
						}
					}

					wcWorkspace := ""
					if wsMatches := regexp.MustCompile(`\bworkspace:\s*([^\s\(\)]+)`).FindStringSubmatch(remainder); len(wsMatches) > 1 {
						wcWorkspace = wsMatches[1]
					}

					deps := []string{}
					if depMatches := regexp.MustCompile(`\b(?:depends\s+on|depends_on):\s*([a-zA-Z0-9_,\s-]+)`).FindStringSubmatch(remainder); len(depMatches) > 1 {
						depList := strings.Split(depMatches[1], ",")
						for _, d := range depList {
							trimmedDep := strings.TrimSpace(d)
							if trimmedDep != "" {
								deps = append(deps, trimmedDep)
							}
						}
					}

					brief.ExpectedWorkcells = append(brief.ExpectedWorkcells, briefWorkcell{
						WorkcellID: wcID,
						Kind:       wcKind,
						Executor:   wcExecutor,
						Peers:      wcPeers,
						MaxRepairs: wcMaxRepairs,
						Workspace:  wcWorkspace,
						Task:       wcTask,
						DependsOn:  deps,
					})
				}
			}
		case "evidence":
			if strings.HasPrefix(trimmedLine, "-") || strings.HasPrefix(trimmedLine, "*") {
				evidenceItem := strings.TrimSpace(strings.TrimLeft(trimmedLine, "-*"))
				if evidenceItem != "" {
					brief.ExpectedEvidence = append(brief.ExpectedEvidence, evidenceItem)
				}
			}
		}
	}

	return brief, nil
}

func writeMarkdownPacket(outPath string, packet factoryPacket) error {
	if outPath == "" {
		return nil
	}
	packetDir := filepath.Dir(outPath)
	mdPath := filepath.Join(packetDir, "packet.md")

	var buf bytes.Buffer
	buf.WriteString("# AO Forge Factory Packet\n\n")
	fmt.Fprintf(&buf, "- **Status**: %s\n", strings.ToUpper(packet.Status))
	fmt.Fprintf(&buf, "- **Plan ID**: %s\n", packet.FactoryPlan.PlanID)
	fmt.Fprintf(&buf, "- **Workcell Count**: %d\n\n", packet.FactoryPlan.WorkcellCount)

	buf.WriteString("## Objective\n\n")
	fmt.Fprintf(&buf, "%s\n", packet.Objective.Text)
	fmt.Fprintf(&buf, "- **Workspace**: %s\n", packet.Objective.Workspace)
	fmt.Fprintf(&buf, "- **Release Mode**: %t\n\n", packet.Objective.ReleaseMode)

	buf.WriteString("## Policy Decisions\n\n")
	buf.WriteString("| Decision ID | Target | Decision | Explanation | Source |\n")
	buf.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, d := range packet.PolicyDecisions {
		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s |\n", d.DecisionID, d.Target, d.Decision, d.Explanation, d.Source)
	}
	buf.WriteString("\n")

	buf.WriteString("## Workcells\n\n")
	buf.WriteString("| Workcell ID | Kind | Executor | Status | Workspace | Run Mode | Summary |\n")
	buf.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, wc := range packet.Workcells {
		runMode := wc.AO2Run
		if runMode == "" {
			runMode = "none"
		}
		wsDisplay := wc.Workspace
		if wsDisplay == "" {
			wsDisplay = packet.Objective.Workspace
		}
		execDisplay := wc.Executor
		if execDisplay == "" {
			execDisplay = "ao2"
		}
		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s | %s | %s |\n", wc.WorkcellID, wc.Kind, execDisplay, wc.Status, wsDisplay, runMode, wc.Summary)
	}
	buf.WriteString("\n")

	buf.WriteString("## Evidence\n\n")
	buf.WriteString("| Label | Schema Version | Status | Path | SHA-256 |\n")
	buf.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, ev := range packet.Evidence {
		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s |\n", ev.Label, ev.SchemaVersion, ev.Status, ev.Path, ev.SHA256)
	}
	buf.WriteString("\n")

	buf.WriteString("## Trust Boundary\n\n")
	fmt.Fprintf(&buf, "- **Local First**: %t\n", packet.TrustBoundary.LocalFirst)
	fmt.Fprintf(&buf, "- **Mutates Releases**: %t\n", packet.TrustBoundary.MutatesReleases)
	fmt.Fprintf(&buf, "- **Stores Credentials**: %t\n", packet.TrustBoundary.StoresCredentials)
	fmt.Fprintf(&buf, "- **Control Plane Approves Work**: %t\n\n", packet.TrustBoundary.ControlPlaneApprovesWork)

	buf.WriteString("## Next Actions\n\n")
	for NaIndex, na := range packet.NextActions {
		if NaIndex > 0 {
			buf.WriteString("\n")
		}
		fmt.Fprintf(&buf, "- **%s**: %s (Required: %t)\n", na.ActionID, na.Description, na.Required)
	}

	return writeFile(mdPath, buf.Bytes())
}

func executeSingleWorkcell(ctx context.Context, plan factoryPlan, wcState *workcellRunState, ao2Path string, liveMode bool) error {
	if wcState.Peers > 1 && wcState.Executor != "agy-swarms" {
		return fmt.Errorf("parallel peer execution is only supported for executor 'agy-swarms', got %q", wcState.Executor)
	}

	workspace := newDirectWorkspaceSession(plan, wcState)
	defer workspace.Close()
	repoPath := workspace.Path()

	for {
		wcState.ResetOutputs()

		var err error
		if wcState.Executor == "agy-swarms" {
			err = runAgySwarmsWorkcell(ctx, plan, wcState, repoPath, liveMode)
		} else {
			err = runAo2Workcell(ctx, plan, wcState, repoPath, ao2Path, liveMode)
		}

		if err == nil {
			err = checkRubric(wcState)
		}

		if err == nil {
			return nil
		}

		// Self-healing / repair check
		if wcState.MaxRepairs > 0 && wcState.RepairsAttempted < wcState.MaxRepairs {
			wcState.RepairsAttempted++
			repairErr := runRepairSwarm(ctx, plan, wcState, repoPath, err, liveMode)
			if repairErr != nil {
				return fmt.Errorf("repair attempt %d failed: %v; original error: %w", wcState.RepairsAttempted, repairErr, err)
			}
			continue
		}

		return err
	}
}

func runAgySwarmsWorkcell(ctx context.Context, plan factoryPlan, wcState *workcellRunState, repoPath string, liveMode bool) error {
	cmdArgs, err := resolveAgySwarmsCommand()
	if err != nil {
		return fmt.Errorf("failed to resolve agy-swarms command: %v", err)
	}

	taskPrompt := wcState.Task
	if taskPrompt == "" {
		taskPrompt = plan.Objective.Text
	}
	taskSpec := map[string]interface{}{
		"task": taskPrompt,
		"model_pins": map[string]string{
			"default": "gemini-3.5-flash",
		},
	}
	taskData, err := json.MarshalIndent(taskSpec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal agy-swarms task for %s: %v", wcState.ID, err)
	}

	specSum := sha256.Sum256(taskData)
	wcState.SpecSHA256 = hex.EncodeToString(specSum[:])

	if wcState.Peers > 1 {
		// CONCURRENT PEER EXECUTION
		wcState.PeerStates = make([]*peerRunState, wcState.Peers)
		for i := 0; i < wcState.Peers; i++ {
			wcState.PeerStates[i] = &peerRunState{
				stateMu: &sync.Mutex{},
				Index:   i,
				Status:  "pending",
			}
		}

		var wg sync.WaitGroup
		peerErrs := make([]error, wcState.Peers)

		for idx := 0; idx < wcState.Peers; idx++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()

				pState := wcState.PeerStates[i]
				pState.stateMu.Lock()
				pState.Status = "running"
				pState.stateMu.Unlock()

				tempTask, err := os.CreateTemp("", fmt.Sprintf("agy-task-%s-peer-%d-*.json", wcState.ID, i))
				if err != nil {
					peerErrs[i] = fmt.Errorf("failed to create temp task: %w", err)
					pState.stateMu.Lock()
					pState.Status = "failed"
					pState.stateMu.Unlock()
					return
				}
				defer os.Remove(tempTask.Name())

				if _, err := tempTask.Write(taskData); err != nil {
					tempTask.Close()
					peerErrs[i] = fmt.Errorf("failed to write temp task: %w", err)
					pState.stateMu.Lock()
					pState.Status = "failed"
					pState.stateMu.Unlock()
					return
				}
				tempTask.Close()

				tempReport, err := os.CreateTemp("", fmt.Sprintf("agy-report-%s-peer-%d-*.json", wcState.ID, i))
				if err != nil {
					peerErrs[i] = fmt.Errorf("failed to create temp report: %w", err)
					pState.stateMu.Lock()
					pState.Status = "failed"
					pState.stateMu.Unlock()
					return
				}
				tempReport.Close()
				defer os.Remove(tempReport.Name())

				args := append(cmdArgs[1:], "run", "--task", tempTask.Name(), "--report", tempReport.Name(), "--allow-local-commands")
				if liveMode {
					args = append(args, "--reviewer", "agy", "--closer", "agy")
				} else {
					args = append(args, "--dry-run")
				}

				cmdName := cmdArgs[0]
				cmd := exec.CommandContext(ctx, cmdName, args...)
				cmd.Dir = repoPath
				cmd.Env = append(os.Environ(), fmt.Sprintf("AO_FORGE_PEER_INDEX=%d", i))

				cmd.Stdout = &realTimeWriter{appendFunc: func(data string) {
					pState.stateMu.Lock()
					pState.Stdout += data
					pState.stateMu.Unlock()
				}}
				cmd.Stderr = &realTimeWriter{appendFunc: func(data string) {
					pState.stateMu.Lock()
					pState.Stderr += data
					pState.stateMu.Unlock()
				}}

				runErr := cmd.Run()

				if reportData, err := os.ReadFile(tempReport.Name()); err == nil {
					var report map[string]interface{}
					if err := json.Unmarshal(reportData, &report); err == nil {
						_ = os.WriteFile(fmt.Sprintf("agy-swarms-report-%s-peer-%d.json", wcState.ID, i), reportData, 0644)
						statusStr, _ := report["status"].(string)
						spentTokens := 0.0
						if st, ok := report["spent_tokens"].(float64); ok {
							spentTokens = st
						}
						spentUSD := 0.0
						if su, ok := report["spent_usd"].(float64); ok {
							spentUSD = su
						}

						pState.stateMu.Lock()
						pState.Tokens = spentTokens
						pState.Cost = spentUSD
						if statusStr == "succeeded" {
							pState.Summary = fmt.Sprintf("Swarm execution succeeded (Tokens: %.0f, Cost: $%.2f)", spentTokens, spentUSD)
						} else {
							pState.Summary = fmt.Sprintf("Swarm execution failed (Tokens: %.0f, Cost: $%.2f)", spentTokens, spentUSD)
						}
						pState.stateMu.Unlock()
					}
				}

				pState.stateMu.Lock()
				if runErr != nil {
					peerErrs[i] = fmt.Errorf("agy-swarms execution failed for peer %d: %v", i, runErr)
					pState.Status = "failed"
				} else {
					pState.Status = "passed"
				}
				pState.stateMu.Unlock()
			}(idx)
		}

		wg.Wait()

		// ADVERSARIAL EVALUATION
		type candidateGrade struct {
			Index    int
			IsValid  bool
			Coverage float64
		}
		grades := make([]candidateGrade, wcState.Peers)
		for i := 0; i < wcState.Peers; i++ {
			pState := wcState.PeerStates[i]
			pState.stateMu.Lock()
			pStdout := pState.Stdout
			pStderr := pState.Stderr
			pStatus := pState.Status
			pState.stateMu.Unlock()

			grades[i] = candidateGrade{
				Index:   i,
				IsValid: true,
			}

			if peerErrs[i] != nil || pStatus != "passed" {
				grades[i].IsValid = false
				continue
			}

			combined := pStdout + "\n" + pStderr

			// Verify rubric patterns
			if wcState.Rubric != nil {
				for _, pattern := range wcState.Rubric.RequiredPatterns {
					if !strings.Contains(combined, pattern) {
						grades[i].IsValid = false
						break
					}
				}
				if !grades[i].IsValid {
					continue
				}

				for _, pattern := range wcState.Rubric.ForbiddenPatterns {
					if strings.Contains(combined, pattern) {
						grades[i].IsValid = false
						break
					}
				}
				if !grades[i].IsValid {
					continue
				}
			}

			// Parse coverage
			var parsedCoverage *float64
			prefixRegex := regexp.MustCompile(`(?i)(?:coverage\s*:\s*|coverage\s+is\s+|coverage\s+of\s+)([0-9.]+)\s*%`)
			matches := prefixRegex.FindStringSubmatch(combined)
			if len(matches) >= 2 {
				val, err := strconv.ParseFloat(matches[1], 64)
				if err == nil {
					parsedCoverage = &val
				}
			}
			if parsedCoverage == nil {
				fallbackRegex := regexp.MustCompile(`([0-9.]+)\s*%`)
				lines := strings.Split(combined, "\n")
				for _, line := range lines {
					if strings.Contains(strings.ToLower(line), "coverage") {
						m := fallbackRegex.FindStringSubmatch(line)
						if len(m) >= 2 {
							val, err := strconv.ParseFloat(m[1], 64)
							if err == nil {
								parsedCoverage = &val
								break
							}
						}
					}
				}
			}

			// Check min coverage
			if wcState.Rubric != nil && wcState.Rubric.MinCoverage != nil {
				if parsedCoverage == nil {
					grades[i].IsValid = false
					continue
				}
				if *parsedCoverage < *wcState.Rubric.MinCoverage {
					grades[i].IsValid = false
					continue
				}
			}

			if parsedCoverage != nil {
				grades[i].Coverage = *parsedCoverage
			}
		}

		// Select winner
		winnerIdx := -1
		maxCoverage := -1.0
		for _, g := range grades {
			if !g.IsValid {
				continue
			}
			if winnerIdx == -1 || g.Coverage > maxCoverage {
				winnerIdx = g.Index
				maxCoverage = g.Coverage
			}
		}

		if winnerIdx == -1 {
			var errMsgs []string
			for i, err := range peerErrs {
				if err != nil {
					errMsgs = append(errMsgs, fmt.Sprintf("peer %d: %v", i, err))
				} else {
					errMsgs = append(errMsgs, fmt.Sprintf("peer %d: rubric failed", i))
				}
			}
			return fmt.Errorf("all %d peer candidates failed: %s", wcState.Peers, strings.Join(errMsgs, "; "))
		}

		// Promote winner to main workcell result
		winnerState := wcState.PeerStates[winnerIdx]
		winnerState.stateMu.Lock()
		wcState.Stdout = winnerState.Stdout
		wcState.Stderr = winnerState.Stderr
		winnerSummary := winnerState.Summary
		winnerState.stateMu.Unlock()

		wcState.Summary = fmt.Sprintf("%s (Winner: Peer %d)", winnerSummary, winnerIdx)

		winnerReportPath := fmt.Sprintf("agy-swarms-report-%s-peer-%d.json", wcState.ID, winnerIdx)
		if reportData, err := os.ReadFile(winnerReportPath); err == nil {
			_ = os.WriteFile(fmt.Sprintf("agy-swarms-report-%s.json", wcState.ID), reportData, 0644)
		}
	} else {
		// SINGLE EXECUTION
		tempTask, err := os.CreateTemp("", "agy-task-"+wcState.ID+"-*.json")
		if err != nil {
			return fmt.Errorf("failed to create temp task for %s: %v", wcState.ID, err)
		}
		defer os.Remove(tempTask.Name())

		if _, err := tempTask.Write(taskData); err != nil {
			tempTask.Close()
			return fmt.Errorf("failed to write temp task for %s: %v", wcState.ID, err)
		}
		tempTask.Close()

		tempReport, err := os.CreateTemp("", "agy-report-"+wcState.ID+"-*.json")
		if err != nil {
			return fmt.Errorf("failed to create temp report for %s: %v", wcState.ID, err)
		}
		tempReport.Close()
		defer os.Remove(tempReport.Name())

		args := append(cmdArgs[1:], "run", "--task", tempTask.Name(), "--report", tempReport.Name(), "--allow-local-commands")
		if liveMode {
			args = append(args, "--reviewer", "agy", "--closer", "agy")
		} else {
			args = append(args, "--dry-run")
		}

		cmdName := cmdArgs[0]
		cmd := exec.CommandContext(ctx, cmdName, args...)
		cmd.Dir = repoPath

		cmd.Stdout = &realTimeWriter{appendFunc: wcState.AppendStdout}
		cmd.Stderr = &realTimeWriter{appendFunc: wcState.AppendStderr}

		runErr := cmd.Run()

		if reportData, err := os.ReadFile(tempReport.Name()); err == nil {
			var report map[string]interface{}
			if err := json.Unmarshal(reportData, &report); err == nil {
				_ = os.WriteFile(fmt.Sprintf("agy-swarms-report-%s.json", wcState.ID), reportData, 0644)
				statusStr, _ := report["status"].(string)
				spentTokens := 0.0
				if st, ok := report["spent_tokens"].(float64); ok {
					spentTokens = st
				}
				spentUSD := 0.0
				if su, ok := report["spent_usd"].(float64); ok {
					spentUSD = su
				}
				if statusStr == "succeeded" {
					wcState.SetSummary(fmt.Sprintf("Swarm execution succeeded (Tokens: %.0f, Cost: $%.2f)", spentTokens, spentUSD))
				} else {
					wcState.SetSummary(fmt.Sprintf("Swarm execution failed (Tokens: %.0f, Cost: $%.2f)", spentTokens, spentUSD))
				}
			}
		}

		if runErr != nil {
			return fmt.Errorf("agy-swarms execution failed for %s: %v (stderr: %q)", wcState.ID, runErr, wcState.GetStderr())
		}
	}
	return nil
}

func runAo2Workcell(ctx context.Context, plan factoryPlan, wcState *workcellRunState, repoPath, ao2Path string, liveMode bool) error {
	specTask := runTask{
		ID:        wcState.ID,
		Kind:      mapWorkcellKind(wcState.Kind),
		Deps:      nil,
		Rationale: "ao-forge workcell " + wcState.ID,
	}

	runSpec := ao2RunSpec{
		APIVersion: "ao2.run/v1",
		Kind:       "Run",
		Metadata: runMetadata{
			Name:        plan.PlanID,
			Description: plan.Objective.Text,
		},
		Spec: runSpecDetails{
			Source: runSource{
				SchemaVersion: "ao2.sdd-plan.v1",
				PlanID:        plan.PlanID,
			},
			PlanKind: "build",
			Goal:     plan.Objective.Text,
			Target: runTarget{
				RepoPath: repoPath,
			},
			TrustBoundary: trustBoundary{
				ControlPlaneRole:   "read_only_observer",
				MutatesAoArtifacts: false,
			},
			Tasks:        []runTask{specTask},
			ExitCriteria: buildAo2ExitCriteria(wcState),
		},
	}

	specData, err := json.MarshalIndent(runSpec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ao2 run spec for %s: %v", wcState.ID, err)
	}

	specSum := sha256.Sum256(specData)
	wcState.SpecSHA256 = hex.EncodeToString(specSum[:])

	tempSpec, err := os.CreateTemp("", "ao2-runspec-"+wcState.ID+"-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp spec for %s: %v", wcState.ID, err)
	}
	defer os.Remove(tempSpec.Name())

	if _, err := tempSpec.Write(specData); err != nil {
		tempSpec.Close()
		return fmt.Errorf("failed to write temp spec for %s: %v", wcState.ID, err)
	}
	tempSpec.Close()

	var cmd *exec.Cmd
	if liveMode {
		tempPrompt, err := os.CreateTemp("", "ao2-provider-prompt-"+wcState.ID+"-*.sh")
		if err != nil {
			return fmt.Errorf("failed to create provider prompt for %s: %v", wcState.ID, err)
		}
		defer os.Remove(tempPrompt.Name())
		if _, err := tempPrompt.WriteString(buildAo2ProviderPrompt(wcState)); err != nil {
			tempPrompt.Close()
			return fmt.Errorf("failed to write provider prompt for %s: %v", wcState.ID, err)
		}
		if err := tempPrompt.Close(); err != nil {
			return fmt.Errorf("failed to close provider prompt for %s: %v", wcState.ID, err)
		}
		cmd = exec.CommandContext(ctx, ao2Path, "run", "--spec", tempSpec.Name(), "--provider", "scripted", "--provider-prompt-file", tempPrompt.Name())
	} else {
		cmd = exec.CommandContext(ctx, ao2Path, "run", "--dry-run", "--spec", tempSpec.Name())
	}
	cmd.Stdout = &realTimeWriter{appendFunc: wcState.AppendStdout}
	cmd.Stderr = &realTimeWriter{appendFunc: wcState.AppendStderr}

	runErr := cmd.Run()

	if runErr != nil {
		return fmt.Errorf("ao2 run failed for %s: %v (stderr: %q)", wcState.ID, runErr, wcState.GetStderr())
	}

	if liveMode {
		if !strings.Contains(wcState.GetStdout(), "status=governed_run_started") &&
			!strings.Contains(wcState.GetStdout(), "status=governed_provider_run_started") &&
			!strings.Contains(wcState.GetStdout(), "status=Accepted") &&
			!strings.Contains(wcState.GetStdout(), "status=accepted") {
			return fmt.Errorf("ao2 run output for %s did not confirm acceptance: %q (stderr: %q)", wcState.ID, wcState.GetStdout(), wcState.GetStderr())
		}
	} else {
		if !strings.Contains(wcState.GetStdout(), "status=dry_run_accepted") {
			return fmt.Errorf("ao2 run output for %s did not confirm acceptance: %q (stderr: %q)", wcState.ID, wcState.GetStdout(), wcState.GetStderr())
		}
	}

	return nil
}

func buildAo2ProviderPrompt(wcState *workcellRunState) string {
	task := strings.TrimSpace(wcState.Task)
	var b strings.Builder
	b.WriteString("set -eu\n")
	b.WriteString("printf 'Summary: AO Forge delegated workcell ")
	b.WriteString(shellSingleQuoteContent(wcState.ID))
	b.WriteString(" through AO2 scripted provider\\n'\n")
	b.WriteString("printf 'Changed files: none\\n'\n")
	if task != "" && (wcState.Kind == "verify" || looksLikeVerificationCommand(task)) {
		b.WriteString(task)
		b.WriteString("\n")
		b.WriteString("printf 'Verification: ")
		b.WriteString(shellSingleQuoteContent(task))
		b.WriteString(" passed\\n'\n")
	} else {
		b.WriteString("printf 'Verification: no verifier command configured for this workcell\\n'\n")
	}
	b.WriteString("printf 'Concern: none\\n'\n")
	b.WriteString("printf 'Blocker: none\\n'\n")
	return b.String()
}

func shellSingleQuoteContent(value string) string {
	return strings.ReplaceAll(value, "'", "'\"'\"'")
}

func buildAo2ExitCriteria(wcState *workcellRunState) exitCriteria {
	criteria := exitCriteria{
		Tests:  []string{},
		Gates:  []string{},
		Manual: []string{},
	}
	task := strings.TrimSpace(wcState.Task)
	if wcState.Kind == "verify" && task != "" {
		criteria.Tests = append(criteria.Tests, task)
		return criteria
	}
	if task != "" && looksLikeVerificationCommand(task) {
		criteria.Tests = append(criteria.Tests, task)
		return criteria
	}
	criteria.Gates = append(criteria.Gates, "ao-forge workcell "+wcState.ID+" accepted")
	return criteria
}

func looksLikeVerificationCommand(task string) bool {
	lower := strings.ToLower(strings.TrimSpace(task))
	return strings.HasPrefix(lower, "go test ") ||
		strings.HasPrefix(lower, "go test") ||
		strings.HasPrefix(lower, "cargo test") ||
		strings.HasPrefix(lower, "npm test") ||
		strings.HasPrefix(lower, "pytest") ||
		strings.HasPrefix(lower, "python -m pytest") ||
		strings.HasPrefix(lower, "uv run") && strings.Contains(lower, "pytest")
}

func checkRubric(wcState *workcellRunState) error {
	if wcState.Rubric == nil {
		return nil
	}
	combined := wcState.GetStdout() + "\n" + wcState.GetStderr()
	for _, pattern := range wcState.Rubric.RequiredPatterns {
		if !strings.Contains(combined, pattern) {
			return fmt.Errorf("rubric validation failed for %s: required pattern %q not found in output", wcState.ID, pattern)
		}
	}
	for _, pattern := range wcState.Rubric.ForbiddenPatterns {
		if strings.Contains(combined, pattern) {
			return fmt.Errorf("rubric validation failed for %s: forbidden pattern %q found in output", wcState.ID, pattern)
		}
	}
	if wcState.Rubric.MinCoverage != nil {
		var parsedCoverage *float64
		prefixRegex := regexp.MustCompile(`(?i)(?:coverage\s*:\s*|coverage\s+is\s+|coverage\s+of\s+)([0-9.]+)\s*%`)
		matches := prefixRegex.FindStringSubmatch(combined)
		if len(matches) >= 2 {
			val, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				parsedCoverage = &val
			}
		}
		if parsedCoverage == nil {
			fallbackRegex := regexp.MustCompile(`([0-9.]+)\s*%`)
			lines := strings.Split(combined, "\n")
			for _, line := range lines {
				if strings.Contains(strings.ToLower(line), "coverage") {
					m := fallbackRegex.FindStringSubmatch(line)
					if len(m) >= 2 {
						val, err := strconv.ParseFloat(m[1], 64)
						if err == nil {
							parsedCoverage = &val
							break
						}
					}
				}
			}
		}
		if parsedCoverage == nil {
			return fmt.Errorf("rubric validation failed for %s: coverage metric not found in output", wcState.ID)
		}
		if *parsedCoverage < *wcState.Rubric.MinCoverage {
			return fmt.Errorf("rubric validation failed for %s: coverage %0.1f%% is below minimum %0.1f%%", wcState.ID, *parsedCoverage, *wcState.Rubric.MinCoverage)
		}
	}
	return nil
}

func runRepairSwarm(ctx context.Context, plan factoryPlan, wcState *workcellRunState, repoPath string, execErr error, liveMode bool) error {
	cmdArgs, err := resolveAgySwarmsCommand()
	if err != nil {
		return fmt.Errorf("failed to resolve agy-swarms command: %v", err)
	}

	taskPrompt := wcState.Task
	if taskPrompt == "" {
		taskPrompt = plan.Objective.Text
	}

	diagnosticPrompt := fmt.Sprintf(
		"You are in self-healing/repair mode. The previous execution of the workcell failed.\n\n"+
			"Original Task:\n%s\n\n"+
			"Stdout Log:\n%s\n\n"+
			"Stderr Log:\n%s\n\n"+
			"Failure Detail:\n%s\n\n"+
			"Please inspect the workspace files and make necessary modifications to repair the codebase so that execution and rubric checks pass on the next attempt.",
		taskPrompt, wcState.GetStdout(), wcState.GetStderr(), execErr.Error(),
	)

	taskSpec := map[string]interface{}{
		"task": diagnosticPrompt,
		"model_pins": map[string]string{
			"default": "gemini-3.5-flash",
		},
	}
	taskData, err := json.MarshalIndent(taskSpec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal agy-swarms repair task: %v", err)
	}

	tempTask, err := os.CreateTemp("", "agy-repair-task-"+wcState.ID+"-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp repair task: %v", err)
	}
	defer os.Remove(tempTask.Name())

	if _, err := tempTask.Write(taskData); err != nil {
		tempTask.Close()
		return fmt.Errorf("failed to write temp repair task: %v", err)
	}
	tempTask.Close()

	tempReport, err := os.CreateTemp("", "agy-repair-report-"+wcState.ID+"-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp repair report: %v", err)
	}
	tempReport.Close()
	defer os.Remove(tempReport.Name())

	args := append(cmdArgs[1:], "run", "--task", tempTask.Name(), "--report", tempReport.Name(), "--allow-local-commands")
	if liveMode {
		args = append(args, "--reviewer", "agy", "--closer", "agy")
	} else {
		args = append(args, "--dry-run")
	}

	cmdName := cmdArgs[0]
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = repoPath

	var repairStdout, repairStderr bytes.Buffer
	cmd.Stdout = &repairStdout
	cmd.Stderr = &repairStderr

	runErr := cmd.Run()

	if reportData, err := os.ReadFile(tempReport.Name()); err == nil {
		_ = os.WriteFile(fmt.Sprintf("agy-swarms-report-%s-repair-attempt-%d.json", wcState.ID, wcState.RepairsAttempted), reportData, 0644)
	}

	if runErr != nil {
		return fmt.Errorf("repair swarm execution failed: %w (stderr: %q)", runErr, repairStderr.String())
	}

	return nil
}

func verifyReleaseWorkspace(workspacePath string) error {
	info, err := os.Stat(workspacePath)
	if err != nil {
		return fmt.Errorf("workspace directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace path is not a directory")
	}

	cmd := exec.Command("git", "-C", workspacePath, "rev-parse", "--is-inside-work-tree")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("workspace is not a git repository: %w", err)
	}

	statusCmd := exec.Command("git", "-C", workspacePath, "status", "--porcelain")
	var statusOut bytes.Buffer
	statusCmd.Stdout = &statusOut
	if err := statusCmd.Run(); err != nil {
		return fmt.Errorf("failed to check git status of workspace: %w", err)
	}

	if statusOut.Len() > 0 {
		return fmt.Errorf("workspace has uncommitted changes (dirty worktree)")
	}

	return nil
}

func runInit(args []string, stdout, stderr io.Writer) int {
	dotForge := ".forge"
	if err := os.MkdirAll(filepath.Join(dotForge, "runs"), 0755); err != nil {
		fmt.Fprintf(stderr, "forge init: failed to create runs directory: %v\n", err)
		return 1
	}

	configPath := filepath.Join(dotForge, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := map[string]any{
			"default_covenant":      "docs/foundation/foundation-baseline.v0.1.json",
			"default_control_plane": "http://127.0.0.1:8744",
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err == nil {
			_ = os.WriteFile(configPath, data, 0644)
		}
	}

	fmt.Fprintln(stdout, "AO Forge repository state initialized under .forge/")
	return 0
}

func runPacketCommand(args []string, stdout, stderr io.Writer) int {
	var runID, outPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--run":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge packet: --run requires a value")
				return 2
			}
			runID = args[i+1]
			i++
		case "--out":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge packet: --out requires a value")
				return 2
			}
			outPath = args[i+1]
			i++
		default:
			fmt.Fprintf(stderr, "forge packet: unexpected argument %s\n", args[i])
			return 2
		}
	}

	if runID == "" {
		fmt.Fprintln(stderr, "forge packet: missing required --run")
		return 2
	}

	packetPath := filepath.Join(".forge", "runs", runID, "factory-packet.json")
	data, err := os.ReadFile(packetPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge packet: run %q not found or packet is missing: %v\n", runID, err)
		return 1
	}

	if outPath != "" {
		if err := writeFile(outPath, data); err != nil {
			fmt.Fprintf(stderr, "forge packet: failed to write packet output: %v\n", err)
			return 1
		}
		var pkt factoryPacket
		if err := json.Unmarshal(data, &pkt); err == nil {
			_ = writeMarkdownPacket(outPath, pkt)
		}
		fmt.Fprintf(stdout, "factory_packet=%s\n", displayPath(outPath))
	} else {
		_, _ = stdout.Write(data)
	}

	return 0
}

func readStdinLine(r io.Reader) (string, error) {
	reader := bufio.NewReader(r)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func resolveGitPath() string {
	if gitBin := os.Getenv("GIT_PATH"); gitBin != "" {
		return gitBin
	}
	return "git"
}

type contractValidateFlags struct {
	schemaPath   string
	documentPath string
	json         bool
}

type contractValidationSummary struct {
	Schema   string   `json:"schema"`
	Document string   `json:"document"`
	Status   string   `json:"status"`
	Errors   []string `json:"errors"`
}

type liveDocsGuardFlags struct {
	planPath            string
	approvalGatePath    string
	ticketPath          string
	sentinelPath        string
	commandReadbackPath string
	outPath             string
}

type liveDocsGuardEvidence struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	SHA256        string `json:"sha256"`
}

type liveDocsGuardResult struct {
	SchemaVersion           string                  `json:"schema_version"`
	Status                  string                  `json:"status"`
	SafeToRequest           bool                    `json:"safe_to_request"`
	SafeToExecute           bool                    `json:"safe_to_execute"`
	AllowedNextAction       string                  `json:"allowed_next_action"`
	FirstFailingCheck       string                  `json:"first_failing_check"`
	BlockingNextActions     []string                `json:"blocking_next_actions"`
	MaintenanceSuggestions  []string                `json:"maintenance_suggestions"`
	MutationClassPolicy     liveDocsClassPolicy     `json:"mutation_class_policy"`
	PatchLimits             *liveDocsPatchLimits    `json:"patch_limits,omitempty"`
	ApprovedScope           map[string]any          `json:"approved_scope"`
	SourceHashes            []liveDocsGuardEvidence `json:"source_hashes"`
	RequiredEvidence        []liveDocsGuardEvidence `json:"required_evidence"`
	Guards                  map[string]string       `json:"guards"`
	MutatesRepositories     bool                    `json:"mutates_repositories"`
	ExecutesWork            bool                    `json:"executes_work"`
	CallsProviders          bool                    `json:"calls_providers"`
	ReleaseOrPublishAllowed bool                    `json:"release_or_publish_allowed"`
}

type liveDocsClassPolicy struct {
	CurrentClass      string   `json:"current_class"`
	PermittedClasses  []string `json:"permitted_classes"`
	LegacyAliases     []string `json:"legacy_aliases"`
	DeniedClasses     []string `json:"denied_classes"`
	AuthorityBoundary string   `json:"authority_boundary"`
	DenialReason      string   `json:"denial_reason"`
}

type liveDocsPatchLimits struct {
	MutationClass                string   `json:"mutation_class"`
	MaxSourceFiles               int      `json:"max_source_files"`
	MaxTestFiles                 int      `json:"max_test_files"`
	MaxChangedFiles              int      `json:"max_changed_files"`
	RequiresRollbackPatch        bool     `json:"requires_rollback_patch"`
	RequiresVerificationCommands bool     `json:"requires_verification_commands"`
	DeniedPathClasses            []string `json:"denied_path_classes"`
}

func runContract(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "forge contract: missing subcommand")
		return 2
	}
	switch args[0] {
	case "validate":
		return runContractValidate(args[1:], stdout, stderr)
	case "--help", "-h":
		fmt.Fprintln(stderr, "forge contract: use `forge contract validate --schema <schema.json> --document <document.json> [--json]`")
		return 0
	default:
		fmt.Fprintf(stderr, "forge contract: unknown subcommand %q\n", args[0])
		return 2
	}
}

func runLiveDocs(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "forge live-docs: missing subcommand")
		return 2
	}
	switch args[0] {
	case "guard":
		return runLiveDocsGuard(args[1:], stdout, stderr)
	case "--help", "-h":
		fmt.Fprintln(stderr, "forge live-docs: use `forge live-docs guard --plan <dry-run-plan.json> --approval-gate <approval-gate.json> --ticket <ticket.json> --sentinel <sentinel.json> --command-readback <command-readback.json> --out <guard.json>`")
		return 0
	default:
		fmt.Fprintf(stderr, "forge live-docs: unknown subcommand %q\n", args[0])
		return 2
	}
}

func parseLiveDocsGuardFlags(args []string) (liveDocsGuardFlags, error) {
	var flags liveDocsGuardFlags
	for i := 0; i < len(args); i++ {
		readValue := func(flag string) (string, error) {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", fmt.Errorf("%s requires a value", flag)
			}
			i++
			return args[i], nil
		}
		var err error
		switch args[i] {
		case "--plan":
			flags.planPath, err = readValue(args[i])
		case "--approval-gate":
			flags.approvalGatePath, err = readValue(args[i])
		case "--ticket":
			flags.ticketPath, err = readValue(args[i])
		case "--sentinel":
			flags.sentinelPath, err = readValue(args[i])
		case "--command-readback":
			flags.commandReadbackPath, err = readValue(args[i])
		case "--out":
			flags.outPath, err = readValue(args[i])
		case "--help", "-h":
			return liveDocsGuardFlags{}, fmt.Errorf("help is available with `forge live-docs --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return liveDocsGuardFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return liveDocsGuardFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
		if err != nil {
			return liveDocsGuardFlags{}, err
		}
	}
	missing := []string{}
	if flags.planPath == "" {
		missing = append(missing, "--plan")
	}
	if flags.approvalGatePath == "" {
		missing = append(missing, "--approval-gate")
	}
	if flags.ticketPath == "" {
		missing = append(missing, "--ticket")
	}
	if flags.sentinelPath == "" {
		missing = append(missing, "--sentinel")
	}
	if flags.commandReadbackPath == "" {
		missing = append(missing, "--command-readback")
	}
	if flags.outPath == "" {
		missing = append(missing, "--out")
	}
	if len(missing) > 0 {
		return liveDocsGuardFlags{}, fmt.Errorf("missing required %s", strings.Join(missing, ", "))
	}
	return flags, nil
}

func runLiveDocsGuard(args []string, stdout, stderr io.Writer) int {
	flags, err := parseLiveDocsGuardFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge live-docs guard: %v\n", err)
		return 2
	}

	result, err := buildLiveDocsGuardResult(flags)
	if err != nil {
		fmt.Fprintf(stderr, "forge live-docs guard: %v\n", err)
		return 1
	}
	data, err := marshalIndented(result)
	if err != nil {
		fmt.Fprintf(stderr, "forge live-docs guard: marshal result: %v\n", err)
		return 1
	}
	if err := writeFile(flags.outPath, data); err != nil {
		fmt.Fprintf(stderr, "forge live-docs guard: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "live_docs_execution_guard=%s\n", result.Status)
	fmt.Fprintf(stdout, "safe_to_request=%t\n", result.SafeToRequest)
	fmt.Fprintf(stdout, "safe_to_execute=%t\n", result.SafeToExecute)
	fmt.Fprintf(stdout, "guard=%s\n", displayPath(flags.outPath))
	if result.FirstFailingCheck != "" {
		fmt.Fprintf(stdout, "first_failing_check=%s\n", result.FirstFailingCheck)
	}
	return 0
}

func buildLiveDocsGuardResult(flags liveDocsGuardFlags) (liveDocsGuardResult, error) {
	plan, planEvidence, err := readLiveDocsGuardEvidence("dry_run_plan", flags.planPath)
	if err != nil {
		return liveDocsGuardResult{}, err
	}
	approvalGate, approvalEvidence, err := readLiveDocsGuardEvidence("approval_gate", flags.approvalGatePath)
	if err != nil {
		return liveDocsGuardResult{}, err
	}
	ticket, ticketEvidence, err := readLiveDocsGuardEvidence("covenant_ticket", flags.ticketPath)
	if err != nil {
		return liveDocsGuardResult{}, err
	}
	sentinel, sentinelEvidence, err := readLiveDocsGuardEvidence("sentinel_no_hold", flags.sentinelPath)
	if err != nil {
		return liveDocsGuardResult{}, err
	}
	commandReadback, commandEvidence, err := readLiveDocsGuardEvidence("command_readback", flags.commandReadbackPath)
	if err != nil {
		return liveDocsGuardResult{}, err
	}

	mutationClass := liveDocsNestedString(plan, "target", "mutation_class")
	evidence := []liveDocsGuardEvidence{planEvidence, approvalEvidence, ticketEvidence, sentinelEvidence, commandEvidence}
	result := liveDocsGuardResult{
		SchemaVersion:          liveDocsGuardVersion,
		Status:                 "ready",
		SafeToRequest:          true,
		SafeToExecute:          liveDocsClassHasExecutionAuthority(mutationClass),
		AllowedNextAction:      liveDocsAllowedNextAction(mutationClass),
		FirstFailingCheck:      "",
		BlockingNextActions:    []string{},
		MaintenanceSuggestions: liveDocsMaintenanceSuggestions(mutationClass),
		MutationClassPolicy:    liveDocsMutationClassPolicy(mutationClass),
		PatchLimits:            liveDocsPatchLimitsForClass(mutationClass),
		ApprovedScope: map[string]any{
			"repo":               liveDocsNestedString(plan, "target", "repo"),
			"allowed_path_class": liveDocsNestedString(plan, "target", "allowed_path_class"),
			"allowed_paths":      liveDocsNestedArray(plan, "target", "allowed_paths"),
			"mutation_class":     mutationClass,
			"branch":             liveDocsNestedString(plan, "worktree_isolation", "branch"),
		},
		SourceHashes:     evidence,
		RequiredEvidence: evidence,
		Guards: map[string]string{
			"dry_run_plan":        "passed",
			"approval_gate":       "passed",
			"covenant_ticket":     "passed",
			"mutation_class":      "passed",
			"docs_only_allowlist": "passed",
			"clean_worktree":      "planned",
			"rollback_plan":       "planned",
			"sentinel_no_hold":    "passed",
			"command_readback":    "passed",
		},
		MutatesRepositories:     false,
		ExecutesWork:            false,
		CallsProviders:          false,
		ReleaseOrPublishAllowed: false,
	}

	if failure := liveDocsMutationClassFailure(mutationClass); failure != "" {
		return blockedLiveDocsGuard(result, "mutation_class", failure), nil
	}
	if failure := liveDocsPlanFailure(plan); failure != "" {
		return blockedLiveDocsGuard(result, "dry_run_plan", failure), nil
	}
	if failure := liveDocsApprovalGateFailure(approvalGate, mutationClass); failure != "" {
		return blockedLiveDocsGuard(result, "approval_gate", failure), nil
	}
	if failure := liveDocsTicketFailure(ticket, mutationClass); failure != "" {
		return blockedLiveDocsGuard(result, "covenant_ticket", failure), nil
	}
	if failure := liveDocsSentinelFailure(sentinel, mutationClass); failure != "" {
		return blockedLiveDocsGuard(result, "sentinel_no_hold", failure), nil
	}
	if failure := liveDocsCommandReadbackFailure(commandReadback, mutationClass); failure != "" {
		return blockedLiveDocsGuard(result, "command_readback", failure), nil
	}

	return result, nil
}

func blockedLiveDocsGuard(result liveDocsGuardResult, check string, reason string) liveDocsGuardResult {
	result.Status = "blocked"
	result.SafeToRequest = false
	result.SafeToExecute = false
	result.AllowedNextAction = "resolve_live_docs_guard_blocker"
	result.FirstFailingCheck = check
	result.BlockingNextActions = []string{reason}
	result.Guards[check] = "blocked"
	return result
}

func readLiveDocsGuardEvidence(name, path string) (map[string]any, liveDocsGuardEvidence, error) {
	doc, err := readJSONAny(path)
	if err != nil {
		return nil, liveDocsGuardEvidence{}, fmt.Errorf("read %s: %w", name, err)
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return nil, liveDocsGuardEvidence{}, fmt.Errorf("%s must be a JSON object", name)
	}
	sum, err := sha256File(path)
	if err != nil {
		return nil, liveDocsGuardEvidence{}, fmt.Errorf("hash %s: %w", name, err)
	}
	status := liveDocsString(obj, "status")
	if status == "" {
		status = liveDocsString(obj, "approval_state")
	}
	if status == "" {
		status = "unknown"
	}
	return obj, liveDocsGuardEvidence{
		Name:          name,
		Path:          displayPath(path),
		SchemaVersion: liveDocsString(obj, "schema_version"),
		Status:        status,
		SHA256:        sum,
	}, nil
}

func liveDocsMutationClassPolicy(mutationClass string) liveDocsClassPolicy {
	if liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) {
		return liveDocsClassPolicy{
			CurrentClass:      mutationClass,
			PermittedClasses:  append([]string{}, liveLowRiskPermittedClasses...),
			LegacyAliases:     append([]string{}, liveDocsLegacyAliases...),
			DeniedClasses:     append([]string{}, liveLowRiskDeniedClasses...),
			AuthorityBoundary: "low_risk_code_dry_run_only",
			DenialReason:      "low_risk_code may only request dry-run evidence here; live code execution requires later rollback, hold, promotion, CI, and readback proof",
		}
	}
	if liveDocsClassIn(mutationClass, liveTestPermittedClasses) {
		return liveDocsClassPolicy{
			CurrentClass:      mutationClass,
			PermittedClasses:  append([]string{}, liveTestPermittedClasses...),
			LegacyAliases:     append([]string{}, liveDocsLegacyAliases...),
			DeniedClasses:     append([]string{}, liveTestDeniedClasses...),
			AuthorityBoundary: "test_only_class_only",
			DenialReason:      "code and multi-repo mutation classes require later rollback, hold, promotion, CI, and readback evidence before Forge may permit them",
		}
	}
	return liveDocsClassPolicy{
		CurrentClass:      mutationClass,
		PermittedClasses:  append([]string{}, liveDocsPermittedClasses...),
		LegacyAliases:     append([]string{}, liveDocsLegacyAliases...),
		DeniedClasses:     append([]string{}, liveDocsDeniedClasses...),
		AuthorityBoundary: "docs_only_classes_only",
		DenialReason:      "non-docs and code mutation classes require later rollback, hold, promotion, CI, and readback evidence before Forge may permit them",
	}
}

func liveDocsMutationClassFailure(mutationClass string) string {
	switch {
	case mutationClass == "":
		return "dry-run plan must name a mutation_class"
	case liveDocsClassIn(mutationClass, liveDocsPermittedClasses), liveDocsClassIn(mutationClass, liveTestPermittedClasses), liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses), liveDocsClassIn(mutationClass, liveDocsLegacyAliases):
		return ""
	case liveDocsClassIn(mutationClass, liveDocsDeniedClasses):
		return fmt.Sprintf("mutation class %s is denied until later slices add class-specific execution evidence", mutationClass)
	default:
		return fmt.Sprintf("mutation class %s is not permitted by the docs-only Forge guard", mutationClass)
	}
}

func liveDocsClassIn(mutationClass string, values []string) bool {
	for _, value := range values {
		if mutationClass == value {
			return true
		}
	}
	return false
}

func liveDocsUsesLegacyClass(mutationClass string) bool {
	return liveDocsClassIn(mutationClass, liveDocsLegacyAliases)
}

func liveDocsClassHasExecutionAuthority(mutationClass string) bool {
	return !liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses)
}

func liveDocsAllowedNextAction(mutationClass string) string {
	if liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) {
		return "request_low_risk_code_dry_run_only"
	}
	if liveDocsClassIn(mutationClass, liveTestPermittedClasses) {
		return "prepare_isolated_test_only_branch"
	}
	return "prepare_isolated_docs_only_branch"
}

func liveDocsMaintenanceSuggestions(mutationClass string) []string {
	if liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) {
		return []string{
			"Keep low_risk_code limited to dry-run packet evidence; this guard does not mutate repositories.",
			"Live low-risk execution and higher mutation classes remain denied until later slices add promotion evidence.",
		}
	}
	if liveDocsClassIn(mutationClass, liveTestPermittedClasses) {
		return []string{
			"Keep execution behind the explicit class-bound live mutation path; this guard does not mutate repositories.",
			"Code and multi-repo mutation classes remain denied until later slices add class-specific evidence.",
		}
	}
	return []string{
		"Keep execution behind the explicit live docs approval path; this guard does not mutate repositories.",
		"Code and non-doc mutation classes remain denied until later slices add class-specific evidence.",
	}
}

func liveDocsPlanFailure(plan map[string]any) string {
	mutationClass := liveDocsNestedString(plan, "target", "mutation_class")
	allowedPathClass := liveDocsNestedString(plan, "target", "allowed_path_class")
	allowedPaths := liveDocsNestedArray(plan, "target", "allowed_paths")
	switch {
	case liveDocsString(plan, "schema_version") != "ao.forge.live-mutation-dry-run-plan.v0.1":
		return "dry-run plan schema_version must be ao.forge.live-mutation-dry-run-plan.v0.1"
	case liveDocsString(plan, "mode") != "dry_run_only":
		return "dry-run plan mode must remain dry_run_only"
	case liveDocsAllowedPathClassFailure(mutationClass, allowedPathClass) != "":
		return liveDocsAllowedPathClassFailure(mutationClass, allowedPathClass)
	case len(allowedPaths) == 0:
		return "dry-run plan must include allowed paths"
	case !liveDocsAllowedPathCount(mutationClass, len(allowedPaths)):
		return fmt.Sprintf("dry-run plan allowed path count exceeds %s limit", mutationClass)
	case !liveDocsAllowedPathsForClass(mutationClass, allowedPaths):
		return fmt.Sprintf("dry-run plan allowed paths must stay within %s paths", mutationClass)
	case liveDocsNestedBool(plan, "worktree_isolation", "isolated_worktree") != true:
		return "dry-run plan must require isolated worktree"
	case liveDocsNestedBool(plan, "worktree_isolation", "clean_worktree_required") != true:
		return "dry-run plan must require clean worktree"
	case liveDocsNestedBool(plan, "worktree_isolation", "reuse_existing_worktree") != false:
		return "dry-run plan must forbid reused worktrees"
	case liveDocsNestedString(plan, "rollback_rehearsal", "status") != "planned":
		return "dry-run plan must include rollback rehearsal plan"
	case liveDocsNestedString(plan, "rollback_rehearsal", "rollback_plan") == "":
		return "dry-run plan must include rollback plan evidence"
	case len(liveDocsNestedArray(plan, "verification", "commands")) == 0:
		return "dry-run plan must include verification commands"
	case liveDocsNestedBool(plan, "provider_boundary", "provider_calls_allowed") != false:
		return "dry-run plan must forbid provider calls"
	case liveDocsNestedBool(plan, "execution_boundary", "mutates_live_repo") != false:
		return "dry-run plan must keep live repo mutation disabled"
	case liveDocsNestedBool(plan, "execution_boundary", "operator_kill_switch_required") != true:
		return "dry-run plan must require operator kill switch"
	default:
		return ""
	}
}

func liveDocsAllowedPathCount(mutationClass string, count int) bool {
	switch mutationClass {
	case "tiny_documentation_change", "docs_only_single_file":
		return count == 1
	case "docs_only_multi_file":
		return count >= 1 && count <= 2
	case "test_only":
		return count == 1
	case "low_risk_code":
		return count >= 1 && count <= 2
	default:
		return false
	}
}

func liveDocsAllowedPathClassFailure(mutationClass, allowedPathClass string) string {
	switch {
	case liveDocsClassIn(mutationClass, liveDocsPermittedClasses), liveDocsClassIn(mutationClass, liveDocsLegacyAliases):
		if allowedPathClass != "docs_only" {
			return "dry-run plan docs mutation target must be docs_only"
		}
	case liveDocsClassIn(mutationClass, liveTestPermittedClasses):
		if allowedPathClass != "test_only" {
			return "dry-run plan test mutation target must be test_only"
		}
	case liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses):
		if allowedPathClass != "low_risk_code" {
			return "dry-run plan low-risk code mutation target must be low_risk_code"
		}
	default:
		return "dry-run plan target has no permitted path class for mutation_class"
	}
	return ""
}

func liveDocsAllowedPathsForClass(mutationClass string, paths []any) bool {
	if liveDocsClassIn(mutationClass, liveTestPermittedClasses) {
		return liveDocsAllowedPathsAreTestOnly(paths)
	}
	if liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) {
		return liveDocsAllowedPathsAreLowRiskCode(paths)
	}
	return liveDocsAllowedPathsAreDocsOnly(paths)
}

func liveDocsAllowedPathsAreDocsOnly(paths []any) bool {
	for _, item := range paths {
		path, ok := item.(string)
		if !ok || path == "" {
			return false
		}
		if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~") || strings.Contains(path, "..") || windowsAbsolutePathPattern.MatchString(path) {
			return false
		}
		if path == "README.md" || path == "REFERENCE.md" || path == "CHANGELOG.md" || strings.HasPrefix(path, "docs/") {
			continue
		}
		return false
	}
	return true
}

func liveDocsAllowedPathsAreTestOnly(paths []any) bool {
	for _, item := range paths {
		path, ok := item.(string)
		if !ok || path == "" {
			return false
		}
		if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~") || strings.Contains(path, "..") || windowsAbsolutePathPattern.MatchString(path) {
			return false
		}
		if !strings.HasSuffix(path, "_test.go") {
			return false
		}
	}
	return true
}

func liveDocsAllowedPathsAreLowRiskCode(paths []any) bool {
	sourceFiles := 0
	testFiles := 0
	for _, item := range paths {
		path, ok := item.(string)
		if !ok || path == "" {
			return false
		}
		if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~") || strings.Contains(path, "..") || windowsAbsolutePathPattern.MatchString(path) {
			return false
		}
		if liveDocsLowRiskForbiddenPath(path) || !strings.HasPrefix(path, "internal/") || !strings.HasSuffix(path, ".go") {
			return false
		}
		if strings.HasSuffix(path, "_test.go") {
			testFiles++
			continue
		}
		sourceFiles++
	}
	return sourceFiles <= 1 && testFiles <= 1 && sourceFiles+testFiles > 0
}

func liveDocsLowRiskForbiddenPath(path string) bool {
	for _, forbidden := range []string{
		".github/",
		"cmd/",
		"config/",
		"configs/",
		"docs/",
		"examples/",
		"release/",
		"releases/",
		"schemas/",
		"scripts/",
		"secrets/",
		"providers/",
		"internal/provider/",
		"internal/providers/",
		"internal/secrets/",
		"go.mod",
		"go.sum",
	} {
		if path == strings.TrimSuffix(forbidden, "/") || strings.HasPrefix(path, forbidden) {
			return true
		}
	}
	return false
}

func liveDocsPatchLimitsForClass(mutationClass string) *liveDocsPatchLimits {
	if !liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) {
		return nil
	}
	return &liveDocsPatchLimits{
		MutationClass:                mutationClass,
		MaxSourceFiles:               1,
		MaxTestFiles:                 1,
		MaxChangedFiles:              2,
		RequiresRollbackPatch:        true,
		RequiresVerificationCommands: true,
		DeniedPathClasses: []string{
			"scripts",
			"ci_workflows",
			"release",
			"secrets",
			"config_expansion",
			"provider_paths",
			"broad_refactors",
		},
	}
}

func liveDocsApprovalGateFailure(gate map[string]any, mutationClass string) string {
	switch liveDocsString(gate, "schema_version") {
	case "ao.foundry.live-docs-approval-gate.v0.1":
		if !liveDocsUsesLegacyClass(mutationClass) {
			return "legacy live-docs approval gate may only approve the legacy single-file docs class"
		}
		switch {
		case liveDocsString(gate, "status") != "ready":
			return "approval gate is not ready"
		case liveDocsBool(gate, "safe_to_execute") != true:
			return "approval gate did not grant safe_to_execute"
		case liveDocsBool(gate, "mutates_repositories") != false:
			return "approval gate must not mutate repositories"
		default:
			return ""
		}
	case "ao.foundry.mutation-class-gate.v0.1":
		switch {
		case liveDocsString(gate, "status") != "ready":
			return "mutation-class gate is not ready"
		case liveDocsString(gate, "mutation_class") != mutationClass:
			return "mutation-class gate class does not match dry-run plan"
		case liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) && liveDocsBool(gate, "safe_to_request") != true:
			return "mutation-class gate did not grant safe_to_request"
		case liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) && liveDocsBool(gate, "safe_to_execute") != false:
			return "mutation-class gate must keep low_risk_code safe_to_execute false"
		case !liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) && liveDocsBool(gate, "safe_to_execute") != true:
			return "mutation-class gate did not grant safe_to_execute"
		case liveDocsString(gate, "authority_boundary") != "single_class_only":
			return "mutation-class gate must remain single_class_only"
		case liveDocsBool(gate, "mutates_repositories") != false:
			return "mutation-class gate must not mutate repositories"
		case liveDocsBool(gate, "executes_work") != false:
			return "mutation-class gate must not execute work"
		default:
			return ""
		}
	default:
		return "approval gate schema_version must be ao.foundry.live-docs-approval-gate.v0.1 or ao.foundry.mutation-class-gate.v0.1"
	}
}

func liveDocsTicketFailure(ticket map[string]any, mutationClass string) string {
	switch liveDocsString(ticket, "schema_version") {
	case "covenant.live-docs-approval-ticket.v1":
		if !liveDocsUsesLegacyClass(mutationClass) {
			return "legacy live-docs approval ticket may only approve the legacy single-file docs class"
		}
	case "covenant.mutation-class-authority-ticket.v1":
		return liveDocsClassTicketFailure(ticket, mutationClass)
	default:
		return "approval ticket schema_version must be covenant.live-docs-approval-ticket.v1 or covenant.mutation-class-authority-ticket.v1"
	}
	state := liveDocsString(ticket, "approval_state")
	if state == "" {
		state = liveDocsString(ticket, "status")
	}
	if state != "approved" {
		return "approval ticket is not approved"
	}
	if liveDocsString(ticket, "approver") == "" {
		return "approval ticket must include approver identity"
	}
	if liveDocsBool(ticket, "consumed") {
		return "approval ticket is already consumed"
	}
	expiresAt := liveDocsString(ticket, "expires_at")
	if expiresAt == "" {
		return "approval ticket must include expires_at"
	}
	parsedExpiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return "approval ticket expires_at must be RFC3339"
	}
	if !parsedExpiry.After(time.Now().UTC()) {
		return "approval ticket is expired"
	}
	return ""
}

func liveDocsClassTicketFailure(ticket map[string]any, mutationClass string) string {
	state := liveDocsString(ticket, "approval_state")
	if state == "" {
		state = liveDocsString(ticket, "status")
	}
	switch {
	case state != "approved":
		return "mutation-class authority ticket is not approved"
	case liveDocsString(ticket, "approver_identity") == "":
		return "mutation-class authority ticket must include approver identity"
	case liveDocsBool(ticket, "consumed"):
		return "mutation-class authority ticket is already consumed"
	case liveDocsString(ticket, "mutation_class") != mutationClass:
		return "mutation-class authority ticket class does not match dry-run plan"
	}
	expiresAt := liveDocsString(ticket, "expires_at")
	if expiresAt == "" {
		return "mutation-class authority ticket must include expires_at"
	}
	parsedExpiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return "mutation-class authority ticket expires_at must be RFC3339"
	}
	if !parsedExpiry.After(time.Now().UTC()) {
		return "mutation-class authority ticket is expired"
	}
	boundaries, _ := ticket["authority_boundaries"].(map[string]any)
	switch {
	case !liveDocsBool(boundaries, "exact_scope"):
		return "mutation-class authority ticket must be exact-scope"
	case !liveDocsBool(boundaries, "class_bound"):
		return "mutation-class authority ticket must be class-bound"
	case !liveDocsBool(boundaries, "digest_bound"):
		return "mutation-class authority ticket must be digest-bound"
	case !liveDocsBool(boundaries, "single_use"):
		return "mutation-class authority ticket must be single-use"
	case liveDocsBool(boundaries, "live_mutation_grant"):
		return "mutation-class authority ticket must not be a broad live mutation grant"
	case liveDocsBool(boundaries, "provider_calls_allowed"):
		return "mutation-class authority ticket must forbid provider calls"
	case liveDocsBool(boundaries, "release_or_publish_allowed"):
		return "mutation-class authority ticket must forbid release or publish"
	}
	approvedScope, _ := ticket["approved_scope"].(map[string]any)
	if len(approvedScope) > 0 && liveDocsString(approvedScope, "mutation_class") != mutationClass {
		return "mutation-class authority ticket approved_scope class does not match dry-run plan"
	}
	if liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) {
		switch {
		case liveDocsBool(approvedScope, "safe_to_request") != true:
			return "mutation-class authority ticket must allow low_risk_code request only"
		case liveDocsBool(approvedScope, "safe_to_execute") != false:
			return "mutation-class authority ticket must keep low_risk_code safe_to_execute false"
		}
	}
	return ""
}

func liveDocsSentinelFailure(sentinel map[string]any, mutationClass string) string {
	switch liveDocsString(sentinel, "schema_version") {
	case "ao.sentinel.live-docs-mutation-hold.v0.1":
		if !liveDocsUsesLegacyClass(mutationClass) {
			return "legacy Sentinel live-docs hold may only approve the legacy single-file docs class"
		}
		switch {
		case liveDocsString(sentinel, "status") != "pass":
			return "sentinel verdict did not pass"
		case liveDocsBool(sentinel, "hold") != false:
			return "sentinel hold is active"
		default:
			return ""
		}
	case "ao.sentinel.mutation-class-hold.v0.1":
		switch {
		case liveDocsString(sentinel, "status") != "no_hold":
			return "sentinel mutation-class verdict did not report no_hold"
		case liveDocsString(sentinel, "mutation_class") != mutationClass:
			return "sentinel mutation-class verdict class does not match dry-run plan"
		case liveDocsBool(sentinel, "hold") != false:
			return "sentinel hold is active"
		default:
			return ""
		}
	default:
		return "sentinel verdict schema_version must be ao.sentinel.live-docs-mutation-hold.v0.1 or ao.sentinel.mutation-class-hold.v0.1"
	}
}

func liveDocsCommandReadbackFailure(readback map[string]any, mutationClass string) string {
	switch liveDocsString(readback, "schema_version") {
	case "ao.command.live-mutation-approval-status.v0.1":
		if !liveDocsUsesLegacyClass(mutationClass) {
			return "legacy command live-mutation readback may only approve the legacy single-file docs class"
		}
		switch {
		case liveDocsString(readback, "status") != "approved":
			return "command readback is not approved"
		case liveDocsBool(readback, "safe_to_execute") != true:
			return "command readback did not show safe_to_execute"
		case liveDocsString(readback, "operator_mode") != "read_only":
			return "command readback must be read_only"
		case liveDocsBool(readback, "mutates_repositories") != false:
			return "command readback must not mutate repositories"
		default:
			return ""
		}
	case "ao.command.atlas-authority-ladder.v0.1":
		switch {
		case liveDocsString(readback, "readback_status") != "ready":
			return "authority ladder readback is not ready"
		case liveDocsString(readback, "operator_mode") != "read_only":
			return "authority ladder readback must be read_only"
		case liveDocsBool(readback, "mutates_repositories") != false:
			return "authority ladder readback must not mutate repositories"
		case liveDocsString(readback, "current_class") != mutationClass && liveDocsString(readback, "next_class") != mutationClass:
			return "authority ladder readback does not expose the requested mutation class"
		case liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) && liveDocsBool(readback, "safe_to_request") != true:
			return "authority ladder readback did not show safe_to_request"
		case liveDocsClassIn(mutationClass, liveLowRiskPermittedClasses) && liveDocsBool(readback, "safe_to_execute") != false:
			return "authority ladder readback must keep low_risk_code safe_to_execute false"
		default:
			return ""
		}
	default:
		return "command readback schema_version must be ao.command.live-mutation-approval-status.v0.1 or ao.command.atlas-authority-ladder.v0.1"
	}
}

func liveDocsString(obj map[string]any, key string) string {
	val, _ := obj[key].(string)
	return val
}

func liveDocsBool(obj map[string]any, key string) bool {
	val, _ := obj[key].(bool)
	return val
}

func liveDocsNestedString(obj map[string]any, outer, inner string) string {
	nested, _ := obj[outer].(map[string]any)
	return liveDocsString(nested, inner)
}

func liveDocsNestedBool(obj map[string]any, outer, inner string) bool {
	nested, _ := obj[outer].(map[string]any)
	return liveDocsBool(nested, inner)
}

func liveDocsNestedArray(obj map[string]any, outer, inner string) []any {
	nested, _ := obj[outer].(map[string]any)
	values, _ := nested[inner].([]any)
	if values == nil {
		return []any{}
	}
	return values
}

func parseContractValidateFlags(args []string) (contractValidateFlags, error) {
	var flags contractValidateFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--schema":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return contractValidateFlags{}, fmt.Errorf("--schema requires a value")
			}
			flags.schemaPath = args[i+1]
			i++
		case "--document":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return contractValidateFlags{}, fmt.Errorf("--document requires a value")
			}
			flags.documentPath = args[i+1]
			i++
		case "--json":
			flags.json = true
		case "--help", "-h":
			return contractValidateFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return contractValidateFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return contractValidateFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.schemaPath == "" {
		return contractValidateFlags{}, fmt.Errorf("missing required --schema")
	}
	if flags.documentPath == "" {
		return contractValidateFlags{}, fmt.Errorf("missing required --document")
	}
	return flags, nil
}

func runContractValidate(args []string, stdout, stderr io.Writer) int {
	flags, err := parseContractValidateFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge contract validate: %v\n", err)
		return 2
	}

	summary := contractValidationSummary{
		Schema:   displayPath(flags.schemaPath),
		Document: displayPath(flags.documentPath),
		Status:   "passed",
		Errors:   []string{},
	}
	if err := validateJSONSchemaDocument(flags.schemaPath, flags.documentPath); err != nil {
		summary.Status = "failed"
		summary.Errors = []string{err.Error()}
		writeContractValidationSummary(stdout, summary, flags.json)
		fmt.Fprintf(stderr, "forge contract validate: schema validation failed: %v\n", err)
		return 1
	}

	writeContractValidationSummary(stdout, summary, flags.json)
	return 0
}

func writeContractValidationSummary(stdout io.Writer, summary contractValidationSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"status\":\"failed\",\"errors\":[%q]}\n", err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "contract_validation=%s\n", summary.Status)
	fmt.Fprintf(stdout, "schema=%s\n", summary.Schema)
	fmt.Fprintf(stdout, "document=%s\n", summary.Document)
	for _, validationErr := range summary.Errors {
		fmt.Fprintf(stdout, "error=%s\n", validationErr)
	}
}

func validateJSONSchemaDocument(schemaPath, documentPath string) error {
	schemaDoc, err := readJSONAny(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	document, err := readJSONAny(documentPath)
	if err != nil {
		return fmt.Errorf("read document: %w", err)
	}
	return validateJSONSchemaValueWithSchema(schemaDoc, document)
}

func validateJSONSchemaValue(schemaPath string, document any) error {
	schemaDoc, err := readJSONAny(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	return validateJSONSchemaValueWithSchema(schemaDoc, document)
}

func validateJSONSchemaValueWithSchema(schemaDoc any, document any) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schemaDoc); err != nil {
		return fmt.Errorf("load schema: %w", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		return err
	}
	return nil
}

func readJSONAny(path string) (any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("multiple JSON values")
	}
	return decoded, nil
}

type artifactChecksumFlags struct {
	artifactPaths []string
	outPath       string
}

type artifactVerifyChecksumFlags struct {
	manifestPath string
}

func runArtifact(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "forge artifact: missing subcommand checksums or verify-checksums")
		return 2
	}
	switch args[0] {
	case "checksums":
		return runArtifactChecksums(args[1:], stdout, stderr)
	case "verify-checksums":
		return runArtifactVerifyChecksums(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "forge artifact: unknown subcommand %q\n", args[0])
		return 2
	}
}

func parseArtifactChecksumFlags(args []string) (artifactChecksumFlags, error) {
	var flags artifactChecksumFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--artifact":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return artifactChecksumFlags{}, fmt.Errorf("--artifact requires a value")
			}
			flags.artifactPaths = append(flags.artifactPaths, args[i+1])
			i++
		case "--out":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return artifactChecksumFlags{}, fmt.Errorf("--out requires a value")
			}
			flags.outPath = args[i+1]
			i++
		case "--help", "-h":
			return artifactChecksumFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return artifactChecksumFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return artifactChecksumFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if len(flags.artifactPaths) == 0 {
		return artifactChecksumFlags{}, fmt.Errorf("missing required --artifact")
	}
	return flags, nil
}

func parseArtifactVerifyChecksumFlags(args []string) (artifactVerifyChecksumFlags, error) {
	var flags artifactVerifyChecksumFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--manifest":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return artifactVerifyChecksumFlags{}, fmt.Errorf("--manifest requires a value")
			}
			flags.manifestPath = args[i+1]
			i++
		case "--help", "-h":
			return artifactVerifyChecksumFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return artifactVerifyChecksumFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return artifactVerifyChecksumFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.manifestPath == "" {
		return artifactVerifyChecksumFlags{}, fmt.Errorf("missing required --manifest")
	}
	return flags, nil
}

func runArtifactChecksums(args []string, stdout, stderr io.Writer) int {
	flags, err := parseArtifactChecksumFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge artifact checksums: %v\n", err)
		return 2
	}

	manifest, err := buildArtifactChecksumManifest(flags.artifactPaths)
	if err != nil {
		fmt.Fprintf(stderr, "forge artifact checksums: %v\n", err)
		return 1
	}

	if flags.outPath != "" {
		if err := writeFile(flags.outPath, []byte(manifest)); err != nil {
			fmt.Fprintf(stderr, "forge artifact checksums: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "artifact_checksums=%s\n", displayPath(flags.outPath))
		return 0
	}

	fmt.Fprint(stdout, manifest)
	return 0
}

func runArtifactVerifyChecksums(args []string, stdout, stderr io.Writer) int {
	flags, err := parseArtifactVerifyChecksumFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge artifact verify-checksums: %v\n", err)
		return 2
	}

	verified, err := verifyArtifactChecksumManifest(flags.manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge artifact verify-checksums: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "artifact_checksums_verified=%d\n", verified)
	fmt.Fprintf(stdout, "manifest=%s\n", displayPath(flags.manifestPath))
	return 0
}

func buildArtifactChecksumManifest(paths []string) (string, error) {
	var manifest strings.Builder
	for _, path := range paths {
		artifact := auditReleasePreviewArtifact(path)
		if artifact.Status != "present" {
			return "", fmt.Errorf("artifact path is %s: %s", artifact.Status, artifact.Path)
		}
		fmt.Fprintf(&manifest, "%s  %s\n", artifact.SHA256, filepath.ToSlash(path))
	}
	return manifest.String(), nil
}

func verifyArtifactChecksumManifest(manifestPath string) (int, error) {
	file, err := os.Open(manifestPath)
	if err != nil {
		return 0, fmt.Errorf("read manifest: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	verified := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		expected, artifactPath, err := parseChecksumManifestLine(line)
		if err != nil {
			return 0, fmt.Errorf("manifest line %d: %w", lineNumber, err)
		}

		resolvedPath := resolveManifestArtifactPath(manifestPath, artifactPath)
		artifact := auditReleasePreviewArtifact(resolvedPath)
		if artifact.Status != "present" {
			return 0, fmt.Errorf("artifact path is %s: %s", artifact.Status, artifact.Path)
		}
		if artifact.SHA256 != expected {
			return 0, fmt.Errorf("checksum mismatch for %s: expected %s, got %s", artifact.Path, expected, artifact.SHA256)
		}
		verified++
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read manifest: %w", err)
	}
	if verified == 0 {
		return 0, fmt.Errorf("manifest has no checksum entries: %s", displayPath(manifestPath))
	}
	return verified, nil
}

func parseChecksumManifestLine(line string) (string, string, error) {
	parts := strings.SplitN(line, "  ", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected '<sha256>  <artifact-path>'")
	}

	expected := strings.ToLower(strings.TrimSpace(parts[0]))
	artifactPath := strings.TrimSpace(parts[1])
	if !isSHA256Hex(expected) {
		return "", "", fmt.Errorf("invalid SHA-256 digest %q", parts[0])
	}
	if artifactPath == "" {
		return "", "", fmt.Errorf("missing artifact path")
	}
	return expected, artifactPath, nil
}

func isSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func resolveManifestArtifactPath(manifestPath, artifactPath string) string {
	if filepath.IsAbs(artifactPath) {
		return artifactPath
	}
	if _, err := os.Stat(artifactPath); err == nil {
		return artifactPath
	}
	return filepath.Join(filepath.Dir(manifestPath), artifactPath)
}

type releasePreviewFlags struct {
	workspacePath string
	tagName       string
	artifactPaths []string
	outPath       string
}

type releasePreviewCheck struct {
	CheckID string `json:"check_id"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

type releasePreviewArtifact struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256,omitempty"`
	SizeBytes  int64  `json:"size_bytes"`
	Status     string `json:"status"`
	Provenance string `json:"provenance"`
}

type releasePreviewAudit struct {
	SchemaVersion       string                   `json:"schema_version"`
	Status              string                   `json:"status"`
	GeneratedAtUTC      string                   `json:"generated_at_utc"`
	Workspace           string                   `json:"workspace"`
	GitHubRepo          string                   `json:"github_repo,omitempty"`
	Tag                 string                   `json:"tag,omitempty"`
	HeadCommit          string                   `json:"head_commit,omitempty"`
	MutatesReleases     bool                     `json:"mutates_releases"`
	NetworkRequired     bool                     `json:"network_required"`
	Checks              []releasePreviewCheck    `json:"checks"`
	Artifacts           []releasePreviewArtifact `json:"artifacts"`
	ReleaseNotesPreview string                   `json:"release_notes_preview"`
	NextActions         []nextAction             `json:"next_actions"`
}

type releasePreviewInspectFlags struct {
	auditPath string
	json      bool
}

type releasePreviewInspectSummary struct {
	InspectSchemaVersion string                   `json:"inspect_schema_version"`
	ReleasePreviewAudit  string                   `json:"release_preview_audit"`
	SchemaVersion        string                   `json:"schema_version"`
	Status               string                   `json:"status"`
	Workspace            string                   `json:"workspace"`
	GitHubRepo           string                   `json:"github_repo"`
	Tag                  string                   `json:"tag"`
	HeadCommit           string                   `json:"head_commit"`
	MutatesReleases      bool                     `json:"mutates_releases"`
	NetworkRequired      bool                     `json:"network_required"`
	Checks               int                      `json:"checks"`
	FailedChecks         int                      `json:"failed_checks"`
	Artifacts            int                      `json:"artifacts"`
	ArtifactDetails      []releasePreviewArtifact `json:"artifact_details"`
	NextActions          []nextAction             `json:"next_actions"`
}

type productionReadinessFlags struct {
	json bool
}

type releaseCandidateFlags struct {
	candidatePath string
}

type productionReadinessGate struct {
	GateID   string   `json:"gate_id"`
	Category string   `json:"category"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Evidence []string `json:"evidence"`
}

type productionReadinessAudit struct {
	SchemaVersion    string                    `json:"schema_version"`
	Status           string                    `json:"status"`
	GeneratedAtUTC   string                    `json:"generated_at_utc"`
	ReadinessPercent int                       `json:"readiness_percent"`
	PassedGates      int                       `json:"passed_gates"`
	TotalGates       int                       `json:"total_gates"`
	Gates            []productionReadinessGate `json:"gates"`
	NextActions      []nextAction              `json:"next_actions"`
}

type productionReadinessGateSpec struct {
	GateID   string
	Category string
	Summary  string
	Evidence []string
	Requires []productionReadinessRequirement
}

type productionReadinessRequirement struct {
	Path     string
	Pattern  string
	Optional bool
}

func runReleaseCandidate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "forge release-candidate: missing subcommand")
		return 2
	}
	switch args[0] {
	case "validate":
		return runReleaseCandidateValidate(args[1:], stdout, stderr)
	case "--help", "-h":
		fmt.Fprintln(stderr, "forge release-candidate: use `forge release-candidate validate --candidate <release-candidate.json>`")
		return 0
	default:
		fmt.Fprintf(stderr, "forge release-candidate: unknown subcommand %q\n", args[0])
		return 2
	}
}

func parseReleaseCandidateValidateFlags(args []string) (releaseCandidateFlags, error) {
	var flags releaseCandidateFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--candidate":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return releaseCandidateFlags{}, fmt.Errorf("--candidate requires a value")
			}
			flags.candidatePath = args[i+1]
			i++
		case "--help", "-h":
			return releaseCandidateFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return releaseCandidateFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return releaseCandidateFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.candidatePath == "" {
		return releaseCandidateFlags{}, fmt.Errorf("missing required --candidate")
	}
	return flags, nil
}

func runReleaseCandidateValidate(args []string, stdout, stderr io.Writer) int {
	flags, err := parseReleaseCandidateValidateFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge release-candidate validate: %v\n", err)
		return 2
	}
	candidate, err := loadReleaseCandidate(flags.candidatePath)
	if err != nil {
		fmt.Fprintf(stderr, "forge release-candidate validate: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "release_candidate=%s\n", candidate.CandidateID)
	fmt.Fprintf(stdout, "status=%s\n", candidate.Status)
	fmt.Fprintf(stdout, "repository=%s\n", candidate.Repository.ID)
	fmt.Fprintf(stdout, "gates=%d\n", len(candidate.Gates))
	fmt.Fprintf(stdout, "promotion_handoff=%s\n", candidate.PromotionHandoff.Status)
	return 0
}

func loadReleaseCandidate(path string) (releaseCandidate, error) {
	var candidate releaseCandidate
	data, err := os.ReadFile(path)
	if err != nil {
		return candidate, fmt.Errorf("read candidate: %w", err)
	}
	if err := decodeJSONStrict(data, &candidate); err != nil {
		return candidate, fmt.Errorf("decode candidate: %w", err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return candidate, fmt.Errorf("decode candidate document: %w", err)
	}
	if err := validateJSONSchemaValue(resolveDefaultContractPath(releaseCandidateSchemaPath), document); err != nil {
		return candidate, fmt.Errorf("schema validation failed: %w", err)
	}
	return candidate, validateReleaseCandidate(candidate)
}

func validateReleaseCandidate(candidate releaseCandidate) error {
	if candidate.SchemaVersion != releaseCandidateVersion {
		return fmt.Errorf("invalid schema_version %q", candidate.SchemaVersion)
	}
	if candidate.CandidateID == "" {
		return fmt.Errorf("candidate_id is required")
	}
	if candidate.Status != "ready" {
		return fmt.Errorf("status must be ready")
	}
	if candidate.Repository.ID != "ao-forge" {
		return fmt.Errorf("repository must be ao-forge")
	}
	if candidate.Repository.Status != "ready" {
		return fmt.Errorf("repository status must be ready")
	}
	if candidate.Repository.Name == "" || candidate.Repository.Role == "" || len(candidate.Repository.Evidence) == 0 {
		return fmt.Errorf("repository name, role, and evidence are required")
	}
	requiredGates := map[string]bool{
		"production_readiness":    false,
		"release_preview_dry_run": false,
		"release_attestation":     false,
		"public_repo_policy":      false,
		"promotion_handoff":       false,
	}
	if len(candidate.Gates) != len(requiredGates) {
		return fmt.Errorf("candidate requires exactly %d gates", len(requiredGates))
	}
	for _, gate := range candidate.Gates {
		seen, ok := requiredGates[gate.GateID]
		if !ok {
			return fmt.Errorf("unexpected gate %q", gate.GateID)
		}
		if seen {
			return fmt.Errorf("duplicate gate %q", gate.GateID)
		}
		if gate.Status != "ready" {
			return fmt.Errorf("gate %q must be ready", gate.GateID)
		}
		if len(gate.Evidence) == 0 {
			return fmt.Errorf("gate %q requires evidence", gate.GateID)
		}
		requiredGates[gate.GateID] = true
	}
	for gateID, seen := range requiredGates {
		if !seen {
			return fmt.Errorf("missing gate %q", gateID)
		}
	}
	if candidate.PromotionHandoff.Status != "manual_required" {
		return fmt.Errorf("promotion_handoff status must be manual_required")
	}
	if candidate.PromotionHandoff.Workflow != "Production Promotion" {
		return fmt.Errorf("promotion_handoff workflow must be Production Promotion")
	}
	for _, required := range []string{"Release Verify", "Release Install Verify", "Release Rollback", "production-promotion-audit"} {
		if !containsString(candidate.PromotionHandoff.Requires, required) {
			return fmt.Errorf("promotion_handoff missing requirement %q", required)
		}
	}
	return nil
}

func runProductionReadiness(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "forge production-readiness: missing subcommand")
		return 2
	}
	switch args[0] {
	case "audit":
		return runProductionReadinessAudit(args[1:], stdout, stderr)
	case "--help", "-h":
		fmt.Fprintln(stderr, "forge production-readiness: use `forge production-readiness audit [--json]`")
		return 0
	default:
		fmt.Fprintf(stderr, "forge production-readiness: unknown subcommand %q\n", args[0])
		return 2
	}
}

func parseProductionReadinessAuditFlags(args []string) (productionReadinessFlags, error) {
	var flags productionReadinessFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			flags.json = true
		case "--help", "-h":
			return productionReadinessFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return productionReadinessFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return productionReadinessFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	return flags, nil
}

func runProductionReadinessAudit(args []string, stdout, stderr io.Writer) int {
	flags, err := parseProductionReadinessAuditFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge production-readiness audit: %v\n", err)
		return 2
	}

	audit := buildProductionReadinessAudit()
	writeProductionReadinessAudit(stdout, audit, flags.json)
	if audit.Status != "passed" {
		fmt.Fprintf(stderr, "forge production-readiness audit: %d/%d gates passed\n", audit.PassedGates, audit.TotalGates)
		return 1
	}
	return 0
}

func buildProductionReadinessAudit() productionReadinessAudit {
	audit := productionReadinessAudit{
		SchemaVersion:  productionReadinessVersion,
		Status:         "passed",
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		Gates:          []productionReadinessGate{},
		NextActions:    []nextAction{},
	}

	for _, spec := range productionReadinessGateSpecs() {
		gate := evaluateProductionReadinessGate(spec)
		audit.Gates = append(audit.Gates, gate)
		if gate.Status == "passed" {
			audit.PassedGates++
		} else {
			audit.Status = "failed"
			audit.NextActions = append(audit.NextActions, nextAction{
				ActionID:    "fix-" + gate.GateID,
				Description: "Restore production-readiness gate: " + gate.Summary,
				Required:    true,
			})
		}
	}
	audit.TotalGates = len(audit.Gates)
	if audit.TotalGates > 0 {
		audit.ReadinessPercent = audit.PassedGates * 100 / audit.TotalGates
	}
	return audit
}

func productionReadinessGateSpecs() []productionReadinessGateSpec {
	return []productionReadinessGateSpec{
		{
			GateID:   "contract.production_readiness_audit",
			Category: "contracts",
			Summary:  "production-readiness audit output has a strict JSON contract",
			Evidence: []string{"docs/contracts/production-readiness-audit-v0.1.schema.json"},
			Requires: []productionReadinessRequirement{
				{Path: "docs/contracts/production-readiness-audit-v0.1.schema.json", Pattern: productionReadinessVersion},
				{Path: "docs/contracts/production-readiness-audit-v0.1.schema.json", Pattern: `"additionalProperties": false`},
			},
		},
		{
			GateID:   "ci.required_checks_documented",
			Category: "ci",
			Summary:  "branch protection runbook documents all required merge checks",
			Evidence: []string{"docs/release/BRANCH-PROTECTION.md"},
			Requires: []productionReadinessRequirement{
				{Path: "docs/release/BRANCH-PROTECTION.md", Pattern: "Go ubuntu-latest"},
				{Path: "docs/release/BRANCH-PROTECTION.md", Pattern: "Go macos-26"},
				{Path: "docs/release/BRANCH-PROTECTION.md", Pattern: "Go windows-latest"},
				{Path: "docs/release/BRANCH-PROTECTION.md", Pattern: "Workflow lint"},
				{Path: "docs/release/BRANCH-PROTECTION.md", Pattern: "GoalRun fixture smoke"},
				{Path: "docs/release/BRANCH-PROTECTION.md", Pattern: "Production readiness audit"},
				{Path: "docs/release/BRANCH-PROTECTION.md", Pattern: "Release preview dry-run audit"},
			},
		},
		{
			GateID:   "ci.workflow_required_jobs",
			Category: "ci",
			Summary:  "CI workflow defines Go, workflow lint, GoalRun fixture smoke, and production-readiness audit jobs",
			Evidence: []string{".github/workflows/ci.yml"},
			Requires: []productionReadinessRequirement{
				{Path: ".github/workflows/ci.yml", Pattern: "Go ${{ matrix.os }}"},
				{Path: ".github/workflows/ci.yml", Pattern: "Workflow lint"},
				{Path: ".github/workflows/ci.yml", Pattern: "GoalRun fixture smoke"},
				{Path: ".github/workflows/ci.yml", Pattern: "scripts/verify-goal-fixtures.sh"},
				{Path: ".github/workflows/ci.yml", Pattern: "Production readiness audit"},
				{Path: ".github/workflows/ci.yml", Pattern: "production-readiness audit --json"},
				{Path: ".github/workflows/ci.yml", Pattern: "production-readiness-audit-v0.1.schema.json"},
				{Path: ".github/workflows/ci.yml", Pattern: "goal evidence cleanup --dry-run --json"},
				{Path: ".github/workflows/ci.yml", Pattern: "goal-run-retained-evidence-cleanup-v0.1.schema.json"},
				{Path: ".github/workflows/ci.yml", Pattern: "goal-run-retained-evidence-cleanup.json"},
				{Path: ".github/workflows/ci.yml", Pattern: "production-readiness-audit"},
			},
		},
		{
			GateID:   "release.candidate_handoff",
			Category: "release",
			Summary:  "release candidate handoff is schema-backed and validated in CI before preview, publish, or promotion",
			Evidence: []string{"docs/contracts/release-candidate-v0.1.schema.json", "examples/release-preview/release-candidate.v0.1.example.json", ".github/workflows/ci.yml", "REFERENCE.md", "docs/README.md"},
			Requires: []productionReadinessRequirement{
				{Path: "docs/contracts/release-candidate-v0.1.schema.json", Pattern: releaseCandidateVersion},
				{Path: "docs/contracts/release-candidate-v0.1.schema.json", Pattern: `"additionalProperties": false`},
				{Path: "examples/release-preview/release-candidate.v0.1.example.json", Pattern: `"candidate_id": "ao-forge-active-spine-2026-06-23"`},
				{Path: "examples/release-preview/release-candidate.v0.1.example.json", Pattern: `"id": "ao-forge"`},
				{Path: "examples/release-preview/release-candidate.v0.1.example.json", Pattern: `"status": "manual_required"`},
				{Path: ".github/workflows/ci.yml", Pattern: "release-candidate validate --candidate examples/release-preview/release-candidate.v0.1.example.json"},
				{Path: "REFERENCE.md", Pattern: "release-candidate validate --candidate examples/release-preview/release-candidate.v0.1.example.json"},
				{Path: "docs/README.md", Pattern: "[Release Candidate v0.1 Schema](contracts/release-candidate-v0.1.schema.json)"},
			},
		},
		{
			GateID:   "release.preview_read_only",
			Category: "release",
			Summary:  "release preview workflow runs non-mutating audit and validates preview contracts",
			Evidence: []string{".github/workflows/release-preview.yml", "docs/contracts/release-preview-audit-v0.1.schema.json", "docs/contracts/release-preview-inspect-v0.1.schema.json"},
			Requires: []productionReadinessRequirement{
				{Path: ".github/workflows/release-preview.yml", Pattern: "Release preview dry-run audit"},
				{Path: ".github/workflows/release-preview.yml", Pattern: "release-preview-audit-v0.1.schema.json"},
				{Path: ".github/workflows/release-preview.yml", Pattern: "release-preview-inspect-v0.1.schema.json"},
				{Path: "docs/contracts/release-preview-audit-v0.1.schema.json", Pattern: "ao.forge.release-preview-audit.v0.1"},
				{Path: "docs/contracts/release-preview-inspect-v0.1.schema.json", Pattern: "ao.forge.release-preview-inspect.v0.1"},
			},
		},
		{
			GateID:   "release.publish_attested_draft",
			Category: "release",
			Summary:  "release publish requires rehearsal evidence, checksums, release evidence bundle, and attestation verification",
			Evidence: []string{".github/workflows/release-publish.yml", "docs/contracts/release-evidence-bundle-v0.1.schema.json"},
			Requires: []productionReadinessRequirement{
				{Path: ".github/workflows/release-publish.yml", Pattern: "release-evidence-bundle.json"},
				{Path: ".github/workflows/release-publish.yml", Pattern: "gh attestation verify"},
				{Path: ".github/workflows/release-publish.yml", Pattern: "draft"},
				{Path: "docs/contracts/release-evidence-bundle-v0.1.schema.json", Pattern: "ao.forge.release-evidence-bundle.v0.1"},
			},
		},
		{
			GateID:   "release.verify_and_install",
			Category: "release",
			Summary:  "release verification and install verification workflows emit contract-valid read-only audits",
			Evidence: []string{".github/workflows/release-verify.yml", ".github/workflows/release-install-verify.yml", "docs/contracts/release-verify-audit-v0.1.schema.json", "docs/contracts/release-install-verify-audit-v0.1.schema.json"},
			Requires: []productionReadinessRequirement{
				{Path: ".github/workflows/release-verify.yml", Pattern: "release-verify-audit.json"},
				{Path: ".github/workflows/release-verify.yml", Pattern: "require_evidence_bundle"},
				{Path: ".github/workflows/release-install-verify.yml", Pattern: "release-install-verify-audit.json"},
				{Path: "docs/contracts/release-verify-audit-v0.1.schema.json", Pattern: "ao.forge.release-verify-audit.v0.1"},
				{Path: "docs/contracts/release-install-verify-audit-v0.1.schema.json", Pattern: "ao.forge.release-install-verify.v0.1"},
			},
		},
		{
			GateID:   "release.rollback_guarded",
			Category: "release",
			Summary:  "rollback workflow has read-only audit mode and guarded mutation modes",
			Evidence: []string{".github/workflows/release-rollback.yml", "docs/contracts/release-rollback-audit-v0.1.schema.json"},
			Requires: []productionReadinessRequirement{
				{Path: ".github/workflows/release-rollback.yml", Pattern: "audit-only"},
				{Path: ".github/workflows/release-rollback.yml", Pattern: "production-release"},
				{Path: ".github/workflows/release-rollback.yml", Pattern: "release-rollback-audit.json"},
				{Path: "docs/contracts/release-rollback-audit-v0.1.schema.json", Pattern: "ao.forge.release-rollback-audit.v0.1"},
			},
		},
		{
			GateID:   "release.production_promotion",
			Category: "release",
			Summary:  "production-stable promotion is read-only and requires verify, install, rollback, and soak evidence",
			Evidence: []string{".github/workflows/production-promotion.yml", "docs/release/PRODUCTION-STABLE-PROMOTION.md", "docs/contracts/production-promotion-audit-v0.1.schema.json"},
			Requires: []productionReadinessRequirement{
				{Path: ".github/workflows/production-promotion.yml", Pattern: "production-promotion-audit.json"},
				{Path: ".github/workflows/production-promotion.yml", Pattern: "min_soak_hours"},
				{Path: ".github/workflows/production-promotion.yml", Pattern: "release_rollback_audit_run_id"},
				{Path: "docs/release/PRODUCTION-STABLE-PROMOTION.md", Pattern: "Do not describe a release as production-stable"},
				{Path: "docs/contracts/production-promotion-audit-v0.1.schema.json", Pattern: "ao.forge.production-promotion-audit.v0.1"},
			},
		},
		{
			GateID:   "goalrun.contracts_and_loop",
			Category: "goalrun",
			Summary:  "GoalRun schemas, context handoffs, verification evidence, AO2 Pulse readiness entrypoint, and durable loop documentation are present",
			Evidence: []string{"docs/contracts/goal-run-v0.1.schema.json", "docs/contracts/goal-run-context-handoff-v0.1.schema.json", "docs/contracts/goal-run-verification-v0.1.schema.json", "docs/contracts/goal-run-readiness-audit-v0.1.schema.json", "examples/goals/ao2-weekend-hardening.context-handoff.json", "examples/goals/ao2-weekend-hardening.goal-run-verification.json", "scripts/ao2-pulse-goal-readiness.sh", "scripts/verify-goal-fixtures.sh", "docs/design/GOAL-RUNS.md", "docs/design/AO2-PULSE-GOAL-RUN-LOOP.md"},
			Requires: []productionReadinessRequirement{
				{Path: "docs/contracts/goal-run-v0.1.schema.json", Pattern: "ao.forge.goal-run.v0.1"},
				{Path: "docs/contracts/goal-run-context-handoff-v0.1.schema.json", Pattern: "ao.forge.goal-run-context-handoff.v0.1"},
				{Path: "docs/contracts/goal-run-verification-v0.1.schema.json", Pattern: "ao.forge.goal-run-verification.v0.1"},
				{Path: "examples/goals/ao2-weekend-hardening.goal-run-verification.json", Pattern: `"phase": "security_scan"`},
				{Path: "examples/goals/ao2-weekend-hardening.context-handoff.json", Pattern: "must_run_goal_readiness"},
				{Path: "docs/contracts/goal-run-readiness-audit-v0.1.schema.json", Pattern: "ao.forge.goal-run-readiness-audit.v0.1"},
				{Path: "scripts/verify-goal-fixtures.sh", Pattern: "goal verification validate"},
				{Path: "docs/design/GOAL-RUNS.md", Pattern: "forge goal verification validate"},
				{Path: "scripts/ao2-pulse-goal-readiness.sh", Pattern: "goal readiness --goal-run"},
				{Path: "docs/design/AO2-PULSE-GOAL-RUN-LOOP.md", Pattern: "An external scheduler may invoke AO2 Pulse on a schedule"},
			},
		},
		{
			GateID:   "goalrun.evidence_retention",
			Category: "goalrun",
			Summary:  "retained evidence policy includes cleanup dry-run plus release/promotion provenance protection",
			Evidence: []string{"docs/contracts/goal-run-retained-evidence-v0.1.schema.json", "docs/contracts/goal-run-retained-evidence-audit-v0.1.schema.json", "docs/contracts/goal-run-retained-evidence-cleanup-v0.1.schema.json", "docs/evidence/goals/ao2-weekend-hardening/20260101T000000Z-complete/release-provenance-retention-proof.json", "docs/evidence/goals/ao2-weekend-hardening/20260101T020000Z-complete/promotion-provenance-retention-proof.json"},
			Requires: []productionReadinessRequirement{
				{Path: "docs/contracts/goal-run-retained-evidence-v0.1.schema.json", Pattern: "release_provenance"},
				{Path: "docs/contracts/goal-run-retained-evidence-v0.1.schema.json", Pattern: "promotion_provenance"},
				{Path: "docs/contracts/goal-run-retained-evidence-audit-v0.1.schema.json", Pattern: "not_eligible_public_provenance"},
				{Path: "docs/contracts/goal-run-retained-evidence-cleanup-v0.1.schema.json", Pattern: goalEvidenceCleanupVersion},
				{Path: "docs/contracts/goal-run-retained-evidence-cleanup-v0.1.schema.json", Pattern: `"mode"`},
				{Path: "scripts/verify-goal-fixtures.sh", Pattern: "goal evidence cleanup"},
				{Path: "scripts/verify-goal-fixtures.sh", Pattern: "not_eligible_public_provenance"},
				{Path: "docs/evidence/goals/ao2-weekend-hardening/20260101T000000Z-complete/release-provenance-retention-proof.json", Pattern: `"retention_class": "release_provenance"`},
				{Path: "docs/evidence/goals/ao2-weekend-hardening/20260101T020000Z-complete/promotion-provenance-retention-proof.json", Pattern: `"retention_class": "promotion_provenance"`},
				{Path: "docs/evidence/goals/ao2-weekend-hardening/20260101T010000Z-complete/old-loop-retention-proof.json", Pattern: `"retention_class": "loop_evidence"`},
			},
		},
		{
			GateID:   "goalrun.provenance_negative_fixtures",
			Category: "goalrun",
			Summary:  "readiness audit tampering and provenance mismatch fixtures are checked in",
			Evidence: []string{"examples/goals/invalid/tampered-readiness-audit.goal-run-readiness-audit.invalid.json", "examples/goals/invalid/mismatched-provenance-readiness-audit.goal-run-readiness-audit.provenance-invalid.json", "scripts/verify-goal-fixtures.sh"},
			Requires: []productionReadinessRequirement{
				{Path: "examples/goals/invalid/tampered-readiness-audit.goal-run-readiness-audit.invalid.json", Pattern: "tampered_after_validation"},
				{Path: "examples/goals/invalid/mismatched-provenance-readiness-audit.goal-run-readiness-audit.provenance-invalid.json", Pattern: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
				{Path: "scripts/verify-goal-fixtures.sh", Pattern: "goal_run_readiness_provenance_invalid_fixtures_rejected"},
			},
		},
		{
			GateID:   "goalrun.architecture_rsi_pin_readback",
			Category: "goalrun",
			Summary:  "AO Architecture pins Forge retained RSI proofs and AO Command enforces those pins",
			Evidence: []string{"docs/contracts/architecture-rsi-pin-readback-v0.1.schema.json", "docs/evidence/architecture/ao-architecture-rsi-pin-readback.json", "REFERENCE.md", "docs/README.md"},
			Requires: []productionReadinessRequirement{
				{Path: "docs/contracts/architecture-rsi-pin-readback-v0.1.schema.json", Pattern: "ao.forge.architecture-rsi-pin-readback.v0.1"},
				{Path: "docs/contracts/architecture-rsi-pin-readback-v0.1.schema.json", Pattern: `"additionalProperties": false`},
				{Path: "docs/evidence/architecture/ao-architecture-rsi-pin-readback.json", Pattern: `"status": "passed"`},
				{Path: "docs/evidence/architecture/ao-architecture-rsi-pin-readback.json", Pattern: "ao-command-rsi-manifest-retention-proof.json"},
				{Path: "docs/evidence/architecture/ao-architecture-rsi-pin-readback.json", Pattern: "bounded-rsi-improvement-chain-retention-proof.json"},
				{Path: "docs/evidence/architecture/ao-architecture-rsi-pin-readback.json", Pattern: `"architecture_prs": [13, 14, 15]`},
				{Path: "docs/evidence/architecture/ao-architecture-rsi-pin-readback.json", Pattern: `"command_validator_pr": 32`},
				{Path: "REFERENCE.md", Pattern: "ao-architecture-rsi-pin-readback.json"},
				{Path: "docs/README.md", Pattern: "[AO Architecture RSI Pin Readback](evidence/architecture/ao-architecture-rsi-pin-readback.json)"},
			},
		},
		{
			GateID:   "security.release_threat_model",
			Category: "security",
			Summary:  "release threat model covers artifact tampering, attestations, and promotion evidence",
			Evidence: []string{"docs/security/RELEASE-THREAT-MODEL.md"},
			Requires: []productionReadinessRequirement{
				{Path: "docs/security/RELEASE-THREAT-MODEL.md", Pattern: "Artifact tampering"},
				{Path: "docs/security/RELEASE-THREAT-MODEL.md", Pattern: "GitHub Artifact Attestation"},
				{Path: "docs/security/RELEASE-THREAT-MODEL.md", Pattern: "Production Promotion"},
			},
		},
		{
			GateID:   "security.public_repo_policy_scan",
			Category: "security",
			Summary:  "public repo policy scan checks tracked files for private paths and credential-shaped leaks",
			Evidence: []string{"scripts/check-public-repo-policy.sh", ".github/workflows/ci.yml", "docs/security/PUBLIC-REPO-POLICY.md"},
			Requires: []productionReadinessRequirement{
				{Path: "scripts/check-public-repo-policy.sh", Pattern: "public repo policy check passed"},
				{Path: "scripts/check-public-repo-policy.sh", Pattern: "git ls-files"},
				{Path: "scripts/check-public-repo-policy.sh", Pattern: "PRIVATE KEY"},
				{Path: "scripts/check-public-repo-policy.sh", Pattern: "credential-like assignment"},
				{Path: "scripts/check-public-repo-policy.sh", Pattern: "machine-local home path"},
				{Path: ".github/workflows/ci.yml", Pattern: "scripts/check-public-repo-policy.sh"},
				{Path: ".github/workflows/ci.yml", Pattern: "public-repo-policy-check.txt"},
				{Path: "docs/security/PUBLIC-REPO-POLICY.md", Pattern: "scripts/check-public-repo-policy.sh"},
			},
		},
	}
}

func evaluateProductionReadinessGate(spec productionReadinessGateSpec) productionReadinessGate {
	gate := productionReadinessGate{
		GateID:   spec.GateID,
		Category: spec.Category,
		Status:   "passed",
		Summary:  spec.Summary,
		Evidence: spec.Evidence,
	}
	for _, requirement := range spec.Requires {
		if err := productionReadinessRequirementSatisfied(requirement); err != nil {
			gate.Status = "failed"
			gate.Summary = spec.Summary + ": " + err.Error()
			return gate
		}
	}
	return gate
}

func productionReadinessRequirementSatisfied(requirement productionReadinessRequirement) error {
	data, err := os.ReadFile(resolveDefaultContractPath(requirement.Path))
	if err != nil {
		if requirement.Optional {
			return nil
		}
		return fmt.Errorf("%s is not readable: %v", requirement.Path, err)
	}
	if requirement.Pattern != "" && !strings.Contains(string(data), requirement.Pattern) {
		return fmt.Errorf("%s missing %q", requirement.Path, requirement.Pattern)
	}
	return nil
}

func writeProductionReadinessAudit(stdout io.Writer, audit productionReadinessAudit, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(audit)
		if err != nil {
			fmt.Fprintf(stdout, "{\"schema_version\":\"%s\",\"status\":\"failed\",\"readiness_percent\":0,\"passed_gates\":0,\"total_gates\":0,\"gates\":[],\"next_actions\":[{\"action_id\":\"marshal-production-readiness-audit\",\"description\":%q,\"required\":true}]}\n", productionReadinessVersion, err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}

	fmt.Fprintf(stdout, "production_readiness=%s\n", audit.Status)
	fmt.Fprintf(stdout, "readiness_percent=%d\n", audit.ReadinessPercent)
	fmt.Fprintf(stdout, "gates_passed=%d\n", audit.PassedGates)
	fmt.Fprintf(stdout, "gates_total=%d\n", audit.TotalGates)
	for _, gate := range audit.Gates {
		fmt.Fprintf(stdout, "gate=%s category=%s status=%s summary=%s\n", gate.GateID, gate.Category, gate.Status, gate.Summary)
	}
	for _, action := range audit.NextActions {
		fmt.Fprintf(stdout, "next_action=%s required=%t description=%s\n", action.ActionID, action.Required, action.Description)
	}
}

func parseReleasePreviewInspectFlags(args []string) (releasePreviewInspectFlags, error) {
	var flags releasePreviewInspectFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--audit":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return releasePreviewInspectFlags{}, fmt.Errorf("--audit requires a value")
			}
			flags.auditPath = args[i+1]
			i++
		case "--json":
			flags.json = true
		case "--help", "-h":
			return releasePreviewInspectFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return releasePreviewInspectFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return releasePreviewInspectFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.auditPath == "" {
		return releasePreviewInspectFlags{}, fmt.Errorf("missing required --audit")
	}
	return flags, nil
}

func parseReleasePreviewFlags(args []string) (releasePreviewFlags, error) {
	var flags releasePreviewFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--workspace":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return releasePreviewFlags{}, fmt.Errorf("--workspace requires a value")
			}
			flags.workspacePath = args[i+1]
			i++
		case "--tag":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return releasePreviewFlags{}, fmt.Errorf("--tag requires a value")
			}
			flags.tagName = args[i+1]
			i++
		case "--artifact":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return releasePreviewFlags{}, fmt.Errorf("--artifact requires a value")
			}
			flags.artifactPaths = append(flags.artifactPaths, args[i+1])
			i++
		case "--out":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return releasePreviewFlags{}, fmt.Errorf("--out requires a value")
			}
			flags.outPath = args[i+1]
			i++
		case "--help", "-h":
			return releasePreviewFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return releasePreviewFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return releasePreviewFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.workspacePath == "" {
		return releasePreviewFlags{}, fmt.Errorf("missing required --workspace")
	}
	return flags, nil
}

func runReleasePreview(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "inspect" {
		return runReleasePreviewInspect(args[1:], stdout, stderr)
	}

	flags, err := parseReleasePreviewFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge release-preview: %v\n", err)
		return 2
	}

	audit := buildReleasePreviewAudit(flags)
	data, err := marshalIndented(audit)
	if err != nil {
		fmt.Fprintf(stderr, "forge release-preview: marshal audit: %v\n", err)
		return 1
	}

	if flags.outPath != "" {
		if err := writeFile(flags.outPath, data); err != nil {
			fmt.Fprintf(stderr, "forge release-preview: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "release_preview_audit=%s\n", displayPath(flags.outPath))
	} else {
		_, _ = stdout.Write(data)
	}

	if audit.Status != "passed" {
		fmt.Fprintf(stderr, "forge release-preview: %s\n", releasePreviewFailureSummary(audit))
		return 1
	}
	return 0
}

func runReleasePreviewInspect(args []string, stdout, stderr io.Writer) int {
	flags, err := parseReleasePreviewInspectFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge release-preview inspect: %v\n", err)
		return 2
	}

	audit, err := readReleasePreviewAudit(flags.auditPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge release-preview inspect: %v\n", err)
		return 1
	}

	failedChecks := 0
	for _, check := range audit.Checks {
		if check.Status != "passed" {
			failedChecks++
		}
	}

	if flags.json {
		summary := releasePreviewInspectSummary{
			InspectSchemaVersion: releasePreviewInspectVersion,
			ReleasePreviewAudit:  displayPath(flags.auditPath),
			SchemaVersion:        audit.SchemaVersion,
			Status:               audit.Status,
			Workspace:            audit.Workspace,
			GitHubRepo:           audit.GitHubRepo,
			Tag:                  audit.Tag,
			HeadCommit:           audit.HeadCommit,
			MutatesReleases:      audit.MutatesReleases,
			NetworkRequired:      audit.NetworkRequired,
			Checks:               len(audit.Checks),
			FailedChecks:         failedChecks,
			Artifacts:            len(audit.Artifacts),
			ArtifactDetails:      audit.Artifacts,
			NextActions:          audit.NextActions,
		}
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stderr, "forge release-preview inspect: marshal summary: %v\n", err)
			return 1
		}
		_, _ = stdout.Write(data)
		return 0
	}

	fmt.Fprintf(stdout, "release_preview_audit=%s\n", displayPath(flags.auditPath))
	fmt.Fprintf(stdout, "schema_version=%s\n", audit.SchemaVersion)
	fmt.Fprintf(stdout, "status=%s\n", audit.Status)
	fmt.Fprintf(stdout, "workspace=%s\n", audit.Workspace)
	fmt.Fprintf(stdout, "github_repo=%s\n", audit.GitHubRepo)
	fmt.Fprintf(stdout, "tag=%s\n", audit.Tag)
	fmt.Fprintf(stdout, "head_commit=%s\n", audit.HeadCommit)
	fmt.Fprintf(stdout, "mutates_releases=%t\n", audit.MutatesReleases)
	fmt.Fprintf(stdout, "network_required=%t\n", audit.NetworkRequired)
	fmt.Fprintf(stdout, "checks=%d\n", len(audit.Checks))
	fmt.Fprintf(stdout, "failed_checks=%d\n", failedChecks)
	fmt.Fprintf(stdout, "artifacts=%d\n", len(audit.Artifacts))
	for _, artifact := range audit.Artifacts {
		fmt.Fprintf(stdout, "artifact=%s status=%s size_bytes=%d sha256=%s provenance=%s\n", artifact.Path, artifact.Status, artifact.SizeBytes, artifact.SHA256, artifact.Provenance)
	}
	for _, action := range audit.NextActions {
		fmt.Fprintf(stdout, "next_action=%s required=%t description=%s\n", action.ActionID, action.Required, action.Description)
	}
	return 0
}

func buildReleasePreviewAudit(flags releasePreviewFlags) releasePreviewAudit {
	audit := releasePreviewAudit{
		SchemaVersion:   releasePreviewAuditVersion,
		Status:          "passed",
		GeneratedAtUTC:  time.Now().UTC().Format(time.RFC3339),
		Workspace:       displayPath(flags.workspacePath),
		MutatesReleases: false,
		NetworkRequired: false,
		Checks:          []releasePreviewCheck{},
		Artifacts:       []releasePreviewArtifact{},
	}
	addCheck := func(id, status, summary string) {
		audit.Checks = append(audit.Checks, releasePreviewCheck{CheckID: id, Status: status, Summary: summary})
		if status != "passed" {
			audit.Status = "blocked"
		}
	}

	if info, err := os.Stat(flags.workspacePath); err != nil {
		addCheck("workspace-exists", "failed", fmt.Sprintf("workspace directory is unavailable: %v", err))
		audit.NextActions = releasePreviewNextActions(audit.Status)
		return audit
	} else if !info.IsDir() {
		addCheck("workspace-exists", "failed", "workspace path is not a directory")
		audit.NextActions = releasePreviewNextActions(audit.Status)
		return audit
	}
	addCheck("workspace-exists", "passed", "workspace directory exists")

	gitBin := resolveGitPath()
	if out, err := exec.Command(gitBin, "-C", flags.workspacePath, "rev-parse", "--is-inside-work-tree").CombinedOutput(); err != nil {
		addCheck("git-repository", "failed", fmt.Sprintf("workspace is not a git repository: %v (output: %q)", err, string(out)))
		audit.NextActions = releasePreviewNextActions(audit.Status)
		return audit
	}
	addCheck("git-repository", "passed", "workspace is a git repository")

	statusOut, err := exec.Command(gitBin, "-C", flags.workspacePath, "status", "--porcelain").Output()
	if err != nil {
		addCheck("clean-worktree", "failed", fmt.Sprintf("failed to inspect workspace status: %v", err))
	} else if strings.TrimSpace(string(statusOut)) != "" {
		addCheck("clean-worktree", "failed", "dirty release workspace: uncommitted changes are present")
	} else {
		addCheck("clean-worktree", "passed", "release workspace is clean")
	}

	if headOut, err := exec.Command(gitBin, "-C", flags.workspacePath, "rev-parse", "HEAD").Output(); err != nil {
		addCheck("head-commit", "failed", fmt.Sprintf("failed to resolve HEAD commit: %v", err))
	} else {
		audit.HeadCommit = strings.TrimSpace(string(headOut))
		addCheck("head-commit", "passed", "HEAD commit resolved")
	}

	if repo, err := getGitHubRepo(flags.workspacePath); err != nil {
		addCheck("github-remote", "failed", fmt.Sprintf("failed to resolve GitHub repository: %v", err))
	} else {
		audit.GitHubRepo = repo
		addCheck("github-remote", "passed", "origin remote resolves to GitHub repository "+repo)
	}

	tagName := strings.TrimSpace(flags.tagName)
	if tagName == "" {
		var err error
		tagName, err = extractReleaseTag("", flags.workspacePath)
		if err != nil {
			addCheck("release-tag", "failed", fmt.Sprintf("failed to resolve release tag: %v", err))
		}
	}
	audit.Tag = tagName
	if tagName != "" {
		tagOut, err := exec.Command(gitBin, "-C", flags.workspacePath, "rev-parse", tagName+"^{commit}").Output()
		if err != nil {
			addCheck("release-tag", "passed", "release tag "+tagName+" is available for creation")
		} else if strings.TrimSpace(string(tagOut)) == audit.HeadCommit {
			addCheck("release-tag", "passed", "release tag "+tagName+" already points to HEAD")
		} else {
			addCheck("release-tag", "failed", "release tag "+tagName+" already points to a different commit")
		}
	}

	if len(flags.artifactPaths) == 0 {
		addCheck("artifact-audit", "passed", "no release artifacts declared for checksum audit")
	}
	for _, artifactPath := range flags.artifactPaths {
		artifact := auditReleasePreviewArtifact(artifactPath)
		audit.Artifacts = append(audit.Artifacts, artifact)
		status := "passed"
		if artifact.Status != "present" {
			status = "failed"
		}
		addCheck("artifact:"+displayPath(artifactPath), status, artifact.Status+" release artifact "+displayPath(artifactPath))
	}

	addCheck("non-mutating-preview", "passed", "preview did not create tags, push refs, publish releases, or require network access")
	audit.ReleaseNotesPreview = buildReleaseNotesPreview(audit)
	audit.NextActions = releasePreviewNextActions(audit.Status)
	return audit
}

func auditReleasePreviewArtifact(path string) releasePreviewArtifact {
	artifact := releasePreviewArtifact{
		Path:       displayPath(path),
		Status:     "missing",
		Provenance: "local-release-preview",
	}
	info, err := os.Stat(path)
	if err != nil {
		return artifact
	}
	if info.IsDir() {
		artifact.Status = "directory"
		return artifact
	}
	data, err := os.ReadFile(path)
	if err != nil {
		artifact.Status = "unreadable"
		return artifact
	}
	sum := sha256.Sum256(data)
	artifact.SHA256 = hex.EncodeToString(sum[:])
	artifact.SizeBytes = info.Size()
	artifact.Status = "present"
	return artifact
}

func buildReleaseNotesPreview(audit releasePreviewAudit) string {
	var notes strings.Builder
	notes.WriteString("## Release Preview\n\n")
	if audit.Tag != "" {
		notes.WriteString("Tag: " + audit.Tag + "\n\n")
	}
	if audit.HeadCommit != "" {
		notes.WriteString("Commit: " + audit.HeadCommit + "\n\n")
	}
	notes.WriteString("## Artifact Checksums\n")
	if len(audit.Artifacts) == 0 {
		notes.WriteString("No artifacts declared.\n")
	} else {
		notes.WriteString("| Artifact | Size | SHA-256 |\n")
		notes.WriteString("| --- | ---: | --- |\n")
		for _, artifact := range audit.Artifacts {
			notes.WriteString(fmt.Sprintf("| `%s` | %d | `%s` |\n", artifact.Path, artifact.SizeBytes, artifact.SHA256))
		}
	}
	return notes.String()
}

func releasePreviewNextActions(status string) []nextAction {
	if status == "passed" {
		return []nextAction{
			{ActionID: "review-release-preview-audit", Description: "Review the release preview audit, artifact checksums, and release notes preview before live release mutation.", Required: true},
			{ActionID: "run-confirmed-release", Description: "Run the live release path only after operator approval and final artifact verification.", Required: true},
		}
	}
	return []nextAction{
		{ActionID: "fix-release-preview-blockers", Description: "Resolve failed release preview checks before creating tags, pushing refs, or publishing a GitHub release.", Required: true},
	}
}

func releasePreviewFailureSummary(audit releasePreviewAudit) string {
	for _, check := range audit.Checks {
		if check.Status != "passed" {
			return check.Summary
		}
	}
	return "release preview audit blocked"
}

func validateReleasePreviewAuditForPlan(path string, plan factoryPlan) (struct {
	Label         string `json:"label"`
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
}, error) {
	var evidence struct {
		Label         string `json:"label"`
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Path          string `json:"path"`
		SHA256        string `json:"sha256"`
	}

	audit, data, err := readReleasePreviewAuditWithData(path)
	if err != nil {
		return evidence, err
	}
	if audit.SchemaVersion != releasePreviewAuditVersion {
		return evidence, fmt.Errorf("unsupported audit schema_version %q", audit.SchemaVersion)
	}
	if audit.Status != "passed" {
		return evidence, fmt.Errorf("audit status must be passed, got %q", audit.Status)
	}
	if audit.MutatesReleases {
		return evidence, fmt.Errorf("audit must be non-mutating")
	}
	if audit.NetworkRequired {
		return evidence, fmt.Errorf("audit must not require network access")
	}
	if len(audit.Checks) == 0 {
		return evidence, fmt.Errorf("audit must include at least one check")
	}

	expectedWorkspace := displayPath(plan.Objective.Workspace)
	if audit.Workspace != expectedWorkspace {
		return evidence, fmt.Errorf("audit workspace %q does not match plan workspace %q", audit.Workspace, expectedWorkspace)
	}

	expectedTag, err := extractReleaseTag(plan.Objective.Text, plan.Objective.Workspace)
	if err != nil {
		return evidence, fmt.Errorf("resolve expected release tag: %w", err)
	}
	if audit.Tag != expectedTag {
		return evidence, fmt.Errorf("audit tag %q does not match expected tag %q", audit.Tag, expectedTag)
	}

	gitBin := resolveGitPath()
	headOut, err := exec.Command(gitBin, "-C", plan.Objective.Workspace, "rev-parse", "HEAD").Output()
	if err != nil {
		return evidence, fmt.Errorf("resolve current HEAD: %w", err)
	}
	currentHead := strings.TrimSpace(string(headOut))
	if audit.HeadCommit != currentHead {
		return evidence, fmt.Errorf("audit head_commit %q does not match current HEAD %q", audit.HeadCommit, currentHead)
	}

	for _, check := range audit.Checks {
		if check.Status != "passed" {
			return evidence, fmt.Errorf("audit check %q is %q: %s", check.CheckID, check.Status, check.Summary)
		}
	}

	sum := sha256.Sum256(data)
	evidence = struct {
		Label         string `json:"label"`
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Path          string `json:"path"`
		SHA256        string `json:"sha256"`
	}{
		Label:         "release preview audit",
		SchemaVersion: audit.SchemaVersion,
		Status:        "passed",
		Path:          displayPath(path),
		SHA256:        hex.EncodeToString(sum[:]),
	}
	return evidence, nil
}

func readReleasePreviewAudit(path string) (releasePreviewAudit, error) {
	audit, _, err := readReleasePreviewAuditWithData(path)
	return audit, err
}

func readReleasePreviewAuditWithData(path string) (releasePreviewAudit, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return releasePreviewAudit{}, nil, fmt.Errorf("read audit: %w", err)
	}
	var audit releasePreviewAudit
	if err := decodeJSONStrict(data, &audit); err != nil {
		return releasePreviewAudit{}, nil, fmt.Errorf("parse audit JSON: %w", err)
	}
	return audit, data, nil
}

func extractReleaseTag(objectiveText string, workspacePath string) (string, error) {
	if tag := os.Getenv("AO_FORGE_RELEASE_TAG"); tag != "" {
		return tag, nil
	}

	re := regexp.MustCompile(`v[0-9]+(?:\.[0-9]+)+(?:-[a-zA-Z0-9.]+)?`)
	if match := re.FindString(objectiveText); match != "" {
		return match, nil
	}

	versionFile := filepath.Join(workspacePath, "VERSION")
	if data, err := os.ReadFile(versionFile); err == nil {
		version := strings.TrimSpace(string(data))
		if version != "" {
			if strings.HasPrefix(version, "v") {
				return version, nil
			}
			return "v" + version, nil
		}
	}

	return "", fmt.Errorf("could not resolve version tag name from objective, environment, or VERSION file")
}

func getGitHubRepo(workspacePath string) (string, error) {
	gitBin := resolveGitPath()
	cmd := exec.Command(gitBin, "-C", workspacePath, "remote", "get-url", "origin")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get git remote url: %w", err)
	}
	urlStr := strings.TrimSpace(out.String())
	if strings.Contains(urlStr, "github.com") {
		parts := strings.SplitN(urlStr, "github.com", 2)
		if len(parts) == 2 {
			path := parts[1]
			path = strings.TrimPrefix(path, ":")
			path = strings.TrimPrefix(path, "/")
			path = strings.TrimSuffix(path, ".git")
			return path, nil
		}
	}
	return "", fmt.Errorf("git remote URL does not contain github.com: %s", urlStr)
}

func publishReleaseViaAPI(repo, token, tagName, commitHash, notesBody string) error {
	apiURL := "https://api.github.com"
	if mockURL := os.Getenv("AO_FORGE_MOCK_GITHUB_API"); mockURL != "" {
		apiURL = mockURL
	}
	url := fmt.Sprintf("%s/repos/%s/releases", apiURL, repo)

	reqBody, err := json.Marshal(map[string]any{
		"tag_name":         tagName,
		"target_commitish": commitHash,
		"name":             "Release " + tagName,
		"body":             notesBody,
		"draft":            true,
		"prerelease":       false,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal release request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api returned status %s: %s", resp.Status, string(bodyBytes))
	}

	return nil
}

func performReleaseMutation(
	plan factoryPlan,
	outPath string,
	evidence []struct {
		Label         string `json:"label"`
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Path          string `json:"path"`
		SHA256        string `json:"sha256"`
	},
	stdout, stderr io.Writer,
) error {
	workspacePath := plan.Objective.Workspace
	tagName, err := extractReleaseTag(plan.Objective.Text, workspacePath)
	if err != nil {
		return fmt.Errorf("failed to determine release tag: %w", err)
	}

	repo, err := getGitHubRepo(workspacePath)
	if err != nil {
		return fmt.Errorf("failed to resolve GitHub repository path: %w", err)
	}

	gitBin := resolveGitPath()

	// Get current HEAD commit hash
	headCmd := exec.Command(gitBin, "-C", workspacePath, "rev-parse", "HEAD")
	headOut, err := headCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get HEAD commit: %w", err)
	}
	commitHash := strings.TrimSpace(string(headOut))

	// Check if the tag already exists
	tagExists := false
	tagCheckCmd := exec.Command(gitBin, "-C", workspacePath, "rev-parse", tagName+"^{commit}")
	tagCheckOut, err := tagCheckCmd.Output()
	if err == nil {
		tagExists = true
		existingCommit := strings.TrimSpace(string(tagCheckOut))
		if existingCommit != commitHash {
			return fmt.Errorf("release tag %q already exists locally but points to a different commit (%s) than HEAD (%s)", tagName, existingCommit, commitHash)
		}
		fmt.Fprintf(stdout, "Release tag %q already exists locally and points to HEAD commit\n", tagName)
	}

	// Create tag if it doesn't exist
	if !tagExists {
		tagCreateCmd := exec.Command(gitBin, "-C", workspacePath, "tag", "-a", tagName, "-m", "Release "+tagName+" via AO Forge")
		if out, err := tagCreateCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to create local git tag %q: %v (output: %q)", tagName, err, string(out))
		}
		fmt.Fprintf(stdout, "Created local git tag %q\n", tagName)
	}

	// Push must succeed before any public release publishing can proceed.
	pushCmd := exec.Command(gitBin, "-C", workspacePath, "push", "origin", tagName)
	if out, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push git tag %q to remote origin: %v (output: %q)", tagName, err, string(out))
	}
	fmt.Fprintf(stdout, "Successfully pushed git tag %s to remote origin\n", tagName)

	// Compile changelog
	var changelog string
	prevTagCmd := exec.Command(gitBin, "-C", workspacePath, "describe", "--tags", "--abbrev=0", tagName+"^")
	prevTagOut, err := prevTagCmd.Output()
	if err == nil {
		prevTag := strings.TrimSpace(string(prevTagOut))
		logCmd := exec.Command(gitBin, "-C", workspacePath, "log", prevTag+".."+tagName, "--oneline")
		logOut, err := logCmd.Output()
		if err == nil {
			changelog = strings.TrimSpace(string(logOut))
		}
	}
	if changelog == "" {
		logCmd := exec.Command(gitBin, "-C", workspacePath, "log", "--oneline", "-n", "50")
		logOut, err := logCmd.Output()
		if err == nil {
			changelog = strings.TrimSpace(string(logOut))
		}
	}

	// Compile notes
	var notes strings.Builder
	notes.WriteString("## Objective\n")
	notes.WriteString(plan.Objective.Text + "\n\n")

	notes.WriteString("## Summary\n")
	notes.WriteString("Release succeeded and verified via AO Forge.\n\n")

	notes.WriteString("## Changelog\n")
	if changelog != "" {
		notes.WriteString("```text\n" + changelog + "\n```\n\n")
	} else {
		notes.WriteString("No git commit history found.\n\n")
	}

	notes.WriteString("## Verified Evidence\n")
	notes.WriteString("| Label | File Path | SHA-256 Digest |\n")
	notes.WriteString("| --- | --- | --- |\n")
	for _, ev := range evidence {
		notes.WriteString(fmt.Sprintf("| %s | `%s` | `%s` |\n", ev.Label, ev.Path, ev.SHA256))
	}

	// Try to publish draft release
	githubToken := os.Getenv("AO_FORGE_GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = os.Getenv("GITHUB_TOKEN")
	}

	ghBin := os.Getenv("GH_PATH")
	if ghBin == "" {
		var err error
		ghBin, err = exec.LookPath("gh")
		if err != nil {
			ghBin = ""
		}
	}

	releasePublished := false
	var ghErr error

	if ghBin != "" {
		tempNotes, err := os.CreateTemp("", "gh-release-notes-*.md")
		if err == nil {
			defer os.Remove(tempNotes.Name())
			if _, err := tempNotes.Write([]byte(notes.String())); err == nil {
				tempNotes.Close()
				cmd := exec.Command(ghBin, "release", "create", tagName,
					"--repo", repo,
					"--title", "Release "+tagName,
					"--notes-file", tempNotes.Name(),
					"--draft",
				)
				cmd.Dir = workspacePath
				cmd.Env = os.Environ()
				if out, err := cmd.CombinedOutput(); err != nil {
					ghErr = fmt.Errorf("gh release create failed: %v (output: %q)", err, string(out))
				} else {
					fmt.Fprintf(stdout, "Successfully created GitHub draft release %s via gh CLI\n", tagName)
					releasePublished = true
				}
			} else {
				tempNotes.Close()
				ghErr = fmt.Errorf("failed to write temp notes: %w", err)
			}
		} else {
			ghErr = fmt.Errorf("failed to create temp notes file: %w", err)
		}
	}

	if !releasePublished {
		if githubToken != "" {
			fmt.Fprintln(stdout, "Attempting fallback to GitHub API for release creation...")
			if err := publishReleaseViaAPI(repo, githubToken, tagName, commitHash, notes.String()); err != nil {
				return fmt.Errorf("GitHub release creation failed (gh CLI err: %v, API err: %v)", ghErr, err)
			}
			fmt.Fprintf(stdout, "Successfully created GitHub draft release %s via HTTP API\n", tagName)
			releasePublished = true
		} else {
			if ghErr != nil {
				return fmt.Errorf("GitHub release creation failed (gh CLI err: %v, and GITHUB_TOKEN is missing)", ghErr)
			}
			return fmt.Errorf("GitHub release creation failed: gh CLI is not available and GITHUB_TOKEN is missing")
		}
	}

	return nil
}
