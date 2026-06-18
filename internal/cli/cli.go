package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/uesugitorachiyo/ao-forge/internal/foundation"
)

const (
	briefSchemaVersion           = "ao.forge.factory-brief.v0.1"
	planSchemaVersion            = "ao.forge.factory-plan.v0.1"
	packetSchemaVersion          = "ao.forge.factory-packet.v0.1"
	decisionFixtureSchemaVersion = "ao.forge.covenant-decision-fixture.v0.1"
	gateResultSchemaVersion      = "ao.forge.covenant-gate-result.v0.1"
)

var planIDPattern = regexp.MustCompile(`^forge-plan-[a-f0-9]{12}$`)

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

// Run executes the AO Forge CLI and returns a process-style exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printHelp(stdout)
		return 0
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "plan":
		return runPlan(args[1:], stdout, stderr)
	case "gate":
		return runGate(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "release-preview":
		return runReleasePreview(args[1:], stdout, stderr)
	case "run":
		return runRun(args[1:], stdout, stderr)
	case "once":
		return runOnce(args[1:], stdout, stderr)
	case "packet":
		return runPacketCommand(args[1:], stdout, stderr)
	case "resume":
		return runResume(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printHelp(stderr)
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `AO Forge

Usage:
  forge --help
  forge init
  forge plan --brief <factory-brief.json> [--out <factory-plan.json>] [--dynamic]
  forge gate --plan <factory-plan.json> --covenant <path-to-covenant-or-config> [--out <gate-result.json>]
  forge run --plan <factory-plan.json> --gate-result <gate-result.json> [--out <factory-packet.json>] [--live] [--confirm-release] [--release-preview-audit <release-preview-audit.json>]
  forge once --brief <factory-brief.json> --covenant <path-to-covenant-or-config> [--out <factory-packet.json>] [--live] [--confirm-release] [--release-preview-audit <release-preview-audit.json>]
  forge packet --run <run-id> [--out <factory-packet.json>]
  forge resume --run <run-id> [--out <factory-packet.json>] [--live] [--confirm-release] [--release-preview-audit <release-preview-audit.json>]
  forge inspect --packet <factory-packet.json>
  forge doctor --foundation <foundation-baseline.json> [--json]
  forge release-preview --workspace <git-workspace> [--tag <vX.Y.Z>] [--artifact <path> ...] [--out <release-preview-audit.json>]

Factory terms:
  factory brief   normalized operator objective and constraints
  workcell        bounded unit of factory work with dependencies and evidence
  factory packet  operator-ready JSON summary of plan, gates, evidence, and next actions

Slice 2.5 status:
  durable state persistence, live/dry-run execution orchestration, verification, run resumption, multi-workspace orchestration, worker swarm integration, interactive operator overrides, real-time TUI dashboard, parallel swarms peer review, closed-loop multi-agent repair & self-healing, dynamic LLM-first factory planning, release mutation, GitHub publishing, release preview audits, and release preview enforcement are enabled.
`)
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
			decision.Explanation = fmt.Sprintf("The plan is local-first, does not allow network access, and does not mutate releases. Covenant binary verified at %s.", flags.covenantPath)
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

func truncateCommit(s string, limit int) string {
	if len(s) > limit {
		return s[:limit]
	}
	return s
}

type ao2RunSpec struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   runMetadata    `json:"metadata"`
	Spec       runSpecDetails `json:"spec"`
}

type runMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type runSpecDetails struct {
	Source        runSource     `json:"source"`
	PlanKind      string        `json:"plan_kind"`
	Goal          string        `json:"goal"`
	Target        runTarget     `json:"target"`
	TrustBoundary trustBoundary `json:"trust_boundary"`
	Tasks         []runTask     `json:"tasks"`
}

type runSource struct {
	SchemaVersion string `json:"schema_version"`
	PlanID        string `json:"plan_id"`
}

type runTarget struct {
	RepoPath string `json:"repo_path"`
}

type trustBoundary struct {
	ControlPlaneRole   string `json:"control_plane_role"`
	MutatesAoArtifacts bool   `json:"mutates_ao_artifacts"`
}

type runTask struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Deps      []string `json:"deps"`
	Rationale string   `json:"rationale"`
}

func mapWorkcellKind(k string) string {
	switch k {
	case "prepare":
		return "create"
	case "execute":
		return "test"
	case "verify", "close":
		return "verify"
	default:
		return "verify"
	}
}

func resolveAo2Binary() (string, error) {
	if p := os.Getenv("AO2_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("AO2_PATH is set to %q but file does not exist", p)
	}

	if root, ok := findRepoRoot(); ok {
		candidates := []string{
			filepath.Join(root, "../ao2/target/release/ao2"),
			filepath.Join(root, "../ao2/target/debug/ao2"),
			filepath.Join(root, "../ao2/ao2"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}

	if p, err := exec.LookPath("ao2"); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("ao2 binary not found (checked AO2_PATH, sibling directory ../ao2, and PATH)")
}

func resolveAgySwarmsProjectDir() (string, bool) {
	if p := os.Getenv("AGY_SWARMS_PROJECT_PATH"); p != "" {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p, true
		}
	}
	if root, ok := findRepoRoot(); ok {
		sibling := filepath.Join(root, "../agy-swarms")
		if info, err := os.Stat(sibling); err == nil && info.IsDir() {
			return sibling, true
		}
	}
	return "", false
}

func resolveAgySwarmsCommand() ([]string, error) {
	if p := os.Getenv("AGY_SWARMS_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return []string{p}, nil
		}
		return nil, fmt.Errorf("AGY_SWARMS_PATH is set to %q but file does not exist", p)
	}

	if dir, ok := resolveAgySwarmsProjectDir(); ok {
		if uvPath, err := exec.LookPath("uv"); err == nil {
			return []string{uvPath, "run", "--project", dir, "agy-swarms"}, nil
		}
	}

	if p, err := exec.LookPath("agy-swarms"); err == nil {
		return []string{p}, nil
	}
	if p, err := exec.LookPath("agy"); err == nil {
		return []string{p}, nil
	}

	return nil, fmt.Errorf("agy-swarms not found (checked AGY_SWARMS_PATH, sibling directory ../agy-swarms with uv, and PATH)")
}

func runRun(args []string, stdout, stderr io.Writer) int {
	var planPath, gateResultPath, outPath string
	var controlPlaneURL string
	var releasePreviewAuditPath string
	var liveMode bool
	var confirmRelease bool
	var nonInteractive bool
	var noDashboard bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--plan":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge run: --plan requires a value")
				return 2
			}
			planPath = args[i+1]
			i++
		case "--gate-result":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge run: --gate-result requires a value")
				return 2
			}
			gateResultPath = args[i+1]
			i++
		case "--out":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge run: --out requires a value")
				return 2
			}
			outPath = args[i+1]
			i++
		case "--control-plane":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge run: --control-plane requires a value")
				return 2
			}
			controlPlaneURL = args[i+1]
			i++
		case "--release-preview-audit":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge run: --release-preview-audit requires a value")
				return 2
			}
			releasePreviewAuditPath = args[i+1]
			i++
		case "--live":
			liveMode = true
		case "--confirm-release":
			confirmRelease = true
		case "--non-interactive", "--yes", "-y":
			nonInteractive = true
		case "--no-dashboard":
			noDashboard = true
		default:
			fmt.Fprintf(stderr, "forge run: unexpected argument %s\n", args[i])
			return 2
		}
	}

	if planPath == "" {
		fmt.Fprintln(stderr, "forge run: missing required --plan")
		return 2
	}
	if gateResultPath == "" {
		fmt.Fprintln(stderr, "forge run: missing required --gate-result")
		return 2
	}

	plan, err := readPlan(planPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge run: read plan: %v\n", err)
		return 1
	}

	return executePlanRun(plan, planPath, gateResultPath, outPath, controlPlaneURL, releasePreviewAuditPath, liveMode, confirmRelease, nonInteractive, noDashboard, os.Stdin, nil, stdout, stderr)
}

func executePlanRun(
	plan factoryPlan,
	planPath string,
	gateResultPath string,
	outPath string,
	controlPlaneURL string,
	releasePreviewAuditPath string,
	liveMode bool,
	confirmRelease bool,
	nonInteractive bool,
	noDashboard bool,
	stdin io.Reader,
	prevStates map[string]*workcellRunState,
	stdout, stderr io.Writer,
) int {
	var schedulerStates []workcellRunState
	var extraEvidence []struct {
		Label         string `json:"label"`
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Path          string `json:"path"`
		SHA256        string `json:"sha256"`
	}

	// Helper function to write blocked packet when failing closed early
	failClosedWithPacket := func(packetStatus string, workcellStatus string, explanation string, decisionID string, source string, isIndeterminate bool, evidence []struct {
		Label         string `json:"label"`
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Path          string `json:"path"`
		SHA256        string `json:"sha256"`
	}) int {
		packet := factoryPacket{
			SchemaVersion: packetSchemaVersion,
			Status:        packetStatus,
		}
		packet.Objective.Text = plan.Objective.Text
		packet.Objective.Workspace = plan.Objective.Workspace
		packet.Objective.ReleaseMode = plan.Objective.ReleaseMode
		packet.FactoryPlan.PlanID = plan.PlanID
		packet.FactoryPlan.WorkcellCount = len(plan.Workcells)
		
		decisionEnum := "deny"
		if isIndeterminate {
			decisionEnum = "requires_operator_approval"
		}
		packet.PolicyDecisions = []struct {
			DecisionID  string `json:"decision_id"`
			Target      string `json:"target"`
			Decision    string `json:"decision"`
			Explanation string `json:"explanation"`
			Source      string `json:"source"`
		}{
			{
				DecisionID:  decisionID,
				Target:      "factory-plan",
				Decision:    decisionEnum,
				Explanation: explanation,
				Source:      source,
			},
		}

		packet.Workcells = make([]struct {
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
		}, len(plan.Workcells))

		for i, wc := range plan.Workcells {
			packet.Workcells[i].WorkcellID = wc.WorkcellID
			packet.Workcells[i].Kind = wc.Kind
			packet.Workcells[i].Workspace = wc.Workspace
			packet.Workcells[i].Executor = wc.Executor
			packet.Workcells[i].Peers = wc.Peers
			packet.Workcells[i].MaxRepairs = wc.MaxRepairs
			packet.Workcells[i].Task = wc.Task
			packet.Workcells[i].DependsOn = cloneStrings(wc.DependsOn)
			if schedulerStates != nil && i < len(schedulerStates) {
				packet.Workcells[i].Status = schedulerStates[i].Status
				packet.Workcells[i].Summary = schedulerStates[i].Summary
				packet.Workcells[i].RepairsAttempted = schedulerStates[i].RepairsAttempted
				if schedulerStates[i].Status == "passed" {
					if liveMode {
						packet.Workcells[i].AO2Run = "live"
					} else {
						packet.Workcells[i].AO2Run = "dry-run"
					}
				}
			} else {
				packet.Workcells[i].Status = workcellStatus
				packet.Workcells[i].Summary = explanation
			}
		}

		if evidence != nil {
			packet.Evidence = evidence
		} else {
			packet.Evidence = []struct {
				Label         string `json:"label"`
				SchemaVersion string `json:"schema_version"`
				Status        string `json:"status"`
				Path          string `json:"path"`
				SHA256        string `json:"sha256"`
			}{}
			// Attempt to include the plan file in evidence anyway if possible
			if data, err := os.ReadFile(planPath); err == nil {
				h := sha256.Sum256(data)
				packet.Evidence = append(packet.Evidence, struct {
					Label         string `json:"label"`
					SchemaVersion string `json:"schema_version"`
					Status        string `json:"status"`
					Path          string `json:"path"`
					SHA256        string `json:"sha256"`
				}{
					Label:         "factory plan",
					SchemaVersion: planSchemaVersion,
					Status:        "planned",
					Path:          displayPath(planPath),
					SHA256:        hex.EncodeToString(h[:]),
				})
			}
		}

		packet.TrustBoundary.LocalFirst = plan.Constraints.LocalFirst
		packet.TrustBoundary.MutatesReleases = liveMode && plan.Objective.ReleaseMode
		packet.TrustBoundary.StoresCredentials = false
		packet.TrustBoundary.ControlPlaneApprovesWork = false

		actionID := "revise-plan-or-stop"
		if packetStatus == "blocked" {
			actionID = "request-operator-approval"
		}
		packet.NextActions = []nextAction{
			{
				ActionID:    actionID,
				Description: explanation,
				Required:    true,
			},
		}

		encoded, err := marshalIndented(packet)
		if err != nil {
			fmt.Fprintf(stderr, "forge run: encode packet: %v\n", err)
			return 1
		}

		if plan.PlanID != "" {
			archiveRunState(plan.PlanID, planPath, gateResultPath, "", encoded, packet)
		}

		if outPath != "" {
			if err := writeFile(outPath, encoded); err != nil {
				fmt.Fprintf(stderr, "forge run: write packet: %v\n", err)
				return 1
			}
			_ = writeMarkdownPacket(outPath, packet)
			fmt.Fprintf(stdout, "factory_packet=%s\n", displayPath(outPath))
		} else {
			_, _ = stdout.Write(encoded)
		}
		return 1
	}

	// Safety check: Release mode live execution requires confirmation
	if liveMode && (plan.Objective.ReleaseMode || plan.Constraints.AllowReleaseMutation) && !confirmRelease {
		explanation := "forge run: release mode live execution requires explicit operator confirmation (--confirm-release)"
		return failClosedWithPacket("blocked", "blocked", explanation, "release-confirmation-required", "ao-forge", true, nil)
	}

	if liveMode && plan.Objective.ReleaseMode && confirmRelease {
		if strings.TrimSpace(releasePreviewAuditPath) == "" {
			explanation := "forge run: release preview audit is required for confirmed release mutation (--release-preview-audit)"
			fmt.Fprintln(stderr, explanation)
			return failClosedWithPacket("blocked", "blocked", explanation, "release-preview-audit-required", "ao-forge", true, nil)
		}
		evidence, err := validateReleasePreviewAuditForPlan(releasePreviewAuditPath, plan)
		if err != nil {
			explanation := fmt.Sprintf("forge run: release preview audit validation failed: %v", err)
			fmt.Fprintln(stderr, explanation)
			return failClosedWithPacket("blocked", "blocked", explanation, "release-preview-audit-invalid", "ao-forge", true, nil)
		}
		extraEvidence = append(extraEvidence, evidence)
	}

	gateData, err := os.ReadFile(gateResultPath)
	if err != nil {
		explanation := fmt.Sprintf("Gate result is unavailable or missing: %v", err)
		return failClosedWithPacket("blocked", "blocked", explanation, "gate-missing", "ao-forge", true, nil)
	}

	var gate covenantGateResult
	if err := json.Unmarshal(gateData, &gate); err != nil {
		explanation := fmt.Sprintf("Gate result is malformed: %v", err)
		return failClosedWithPacket("blocked", "blocked", explanation, "gate-malformed", "ao-forge", true, nil)
	}

	// Verify target plan ID matches
	if gate.PlanID != plan.PlanID {
		explanation := fmt.Sprintf("Gate result PlanID %q does not match plan PlanID %q", gate.PlanID, plan.PlanID)
		return failClosedWithPacket("blocked", "blocked", explanation, "gate-plan-mismatch", "ao-forge", true, nil)
	}

	// Fail closed if gate result status is not allowed
	if gate.Status != "allowed" {
		if gate.Decision.DecisionID == "indeterminate-release-mutation" && confirmRelease {
			// Operator override accepted, proceed!
		} else if gate.Status == "blocked" && !nonInteractive {
			fmt.Fprintf(stderr, "\nCovenant Gate returned indeterminate/blocked decision.\nDecision ID: %s\nExplanation: %s\nApprove and override execution? [y/N]: ", gate.Decision.DecisionID, gate.Decision.Explanation)
			response, scanErr := readStdinLine(stdin)
			if scanErr == nil && (strings.ToLower(response) == "y" || strings.ToLower(response) == "yes") {
				gate.Status = "allowed"
				overrideEv := map[string]any{
					"schema_version":   "ao2.operator-override-evidence.v1",
					"timestamp":        time.Now().Format(time.RFC3339),
					"gate_decision_id": gate.Decision.DecisionID,
					"explanation":      gate.Decision.Explanation,
					"approved":         true,
				}
				overrideData, err := marshalIndented(overrideEv)
				if err != nil {
					explanation := fmt.Sprintf("Failed to marshal operator override evidence: %v", err)
					return failClosedWithPacket("failed", "failed", explanation, "override-evidence-marshal-failed", "ao-forge", false, nil)
				}
				summaryDir := "."
				if outPath != "" {
					summaryDir = filepath.Dir(outPath)
				}
				overridePath := filepath.Join(summaryDir, "operator-override.json")
				if err := writeFile(overridePath, overrideData); err != nil {
					explanation := fmt.Sprintf("Failed to write operator override evidence: %v", err)
					return failClosedWithPacket("failed", "failed", explanation, "override-evidence-write-failed", "ao-forge", false, nil)
				}
				sum := sha256.Sum256(overrideData)
				extraEvidence = append(extraEvidence, struct {
					Label         string `json:"label"`
					SchemaVersion string `json:"schema_version"`
					Status        string `json:"status"`
					Path          string `json:"path"`
					SHA256        string `json:"sha256"`
				}{
					Label:         "operator override approval evidence",
					SchemaVersion: "ao2.operator-override-evidence.v1",
					Status:        "passed",
					Path:          displayPath(overridePath),
					SHA256:        hex.EncodeToString(sum[:]),
				})
			} else {
				return failClosedWithPacket("blocked", "blocked", gate.Decision.Explanation, gate.Decision.DecisionID, gate.Decision.Source, true, nil)
			}
		} else {
			packetStatus := "blocked"
			workcellStatus := "blocked"
			isIndet := true
			if gate.Status == "denied" {
				packetStatus = "denied"
				workcellStatus = "denied"
				isIndet = false
			}
			return failClosedWithPacket(packetStatus, workcellStatus, gate.Decision.Explanation, gate.Decision.DecisionID, gate.Decision.Source, isIndet, nil)
		}
	}

	// Gate is allowed. Find and verify ao2 binary
	ao2Path, err := resolveAo2Binary()
	if err != nil {
		explanation := fmt.Sprintf("AO2 binary is unavailable: %v", err)
		return failClosedWithPacket("blocked", "blocked", explanation, "ao2-unavailable", "ao-forge", true, nil)
	}

	// Construct the overall Ao2RunSpec to compute its spec_sha256 dynamically
	specTasks := make([]runTask, 0, len(plan.Workcells))
	for _, wc := range plan.Workcells {
		specTasks = append(specTasks, runTask{
			ID:        wc.WorkcellID,
			Kind:      mapWorkcellKind(wc.Kind),
			Deps:      cloneStrings(wc.DependsOn),
			Rationale: "ao-forge workcell " + wc.WorkcellID,
		})
	}

	overallSpec := ao2RunSpec{
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
				RepoPath: plan.Objective.Workspace,
			},
			TrustBoundary: trustBoundary{
				ControlPlaneRole:   "read_only_observer",
				MutatesAoArtifacts: false,
			},
			Tasks: specTasks,
		},
	}

	overallSpecData, err := json.Marshal(overallSpec)
	if err != nil {
		explanation := fmt.Sprintf("Failed to marshal overall ao2 run spec: %v", err)
		return failClosedWithPacket("failed", "failed", explanation, "spec-generation-failed", "ao-forge", false, nil)
	}
	overallSpecSum := sha256.Sum256(overallSpecData)
	overallSpecSHA256 := hex.EncodeToString(overallSpecSum[:])

	summaryDir := "."
	if outPath != "" {
		summaryDir = filepath.Dir(outPath)
	}

	// 1. Run Workcells
	var runErr error
	schedulerStates, runErr = runWorkcellsConcurrent(context.Background(), plan, ao2Path, stdout, stderr, liveMode, nonInteractive, noDashboard, stdin, prevStates)

	// Determine status
	runSummaryStatus := "dry_run_accepted"
	if liveMode {
		runSummaryStatus = "accepted"
	}
	packetStatus := "passed"
	workcellStatus := "passed"
	if runErr != nil {
		runSummaryStatus = "dry_run_failed"
		if liveMode {
			runSummaryStatus = "failed"
		}
		packetStatus = "failed"
		workcellStatus = "failed"
	}

	// Build run summary
	parsedSummary := map[string]any{
		"schema_version":             "ao2.run/v1",
		"status":                     runSummaryStatus,
		"plan_id":                    plan.PlanID,
		"task_count":                 len(plan.Workcells),
		"target_repo":                plan.Objective.Workspace,
		"control_plane_role":         "read_only_observer",
		"mutates_ao_artifacts":       false,
		"factory_v3_drives_workflow": false,
		"spec_sha256":                overallSpecSHA256,
	}

	summaryData, err := marshalIndented(parsedSummary)
	if err != nil {
		explanation := fmt.Sprintf("Failed to marshal ao2 run summary: %v", err)
		return failClosedWithPacket("failed", "failed", explanation, "summary-marshal-failed", "ao-forge", false, nil)
	}

	summaryPath := filepath.Join(summaryDir, "ao2-run-summary.json")
	if err := writeFile(summaryPath, summaryData); err != nil {
		explanation := fmt.Sprintf("Failed to write ao2 run summary: %v", err)
		return failClosedWithPacket("failed", "failed", explanation, "summary-write-failed", "ao-forge", false, nil)
	}

	// Prepare final evidence list
	var evidenceList []struct {
		Label         string `json:"label"`
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Path          string `json:"path"`
		SHA256        string `json:"sha256"`
	}
	evidenceList = append(evidenceList, extraEvidence...)

	// 1. Factory Plan
	if pData, err := os.ReadFile(planPath); err == nil {
		sum := sha256.Sum256(pData)
		evidenceList = append(evidenceList, struct {
			Label         string `json:"label"`
			SchemaVersion string `json:"schema_version"`
			Status        string `json:"status"`
			Path          string `json:"path"`
			SHA256        string `json:"sha256"`
		}{
			Label:         "factory plan",
			SchemaVersion: planSchemaVersion,
			Status:        "planned",
			Path:          displayPath(planPath),
			SHA256:        hex.EncodeToString(sum[:]),
		})
	}

	// 2. Covenant Gate Result
	if gData, err := os.ReadFile(gateResultPath); err == nil {
		sum := sha256.Sum256(gData)
		evidenceList = append(evidenceList, struct {
			Label         string `json:"label"`
			SchemaVersion string `json:"schema_version"`
			Status        string `json:"status"`
			Path          string `json:"path"`
			SHA256        string `json:"sha256"`
		}{
			Label:         "covenant policy decision",
			SchemaVersion: gateResultSchemaVersion,
			Status:        gate.Status,
			Path:          displayPath(gateResultPath),
			SHA256:        hex.EncodeToString(sum[:]),
		})
	}

	// 3. AO2 Run Summary
	if sData, err := os.ReadFile(summaryPath); err == nil {
		sum := sha256.Sum256(sData)
		evidenceList = append(evidenceList, struct {
			Label         string `json:"label"`
			SchemaVersion string `json:"schema_version"`
			Status        string `json:"status"`
			Path          string `json:"path"`
			SHA256        string `json:"sha256"`
		}{
			Label:         "ao2 run summary",
			SchemaVersion: "ao2.run/v1",
			Status:        runSummaryStatus,
			Path:          displayPath(summaryPath),
			SHA256:        hex.EncodeToString(sum[:]),
		})
	}

	// 4. Individual Workcell Evidence
	for _, state := range schedulerStates {
		if state.Status == "passed" || state.Status == "failed" {
			wcEv := map[string]any{
				"schema_version": "ao2.workcell-evidence.v1",
				"workcell_id":    state.ID,
				"status":         state.Status,
				"stdout":         state.Stdout,
				"stderr":         state.Stderr,
				"spec_sha256":    state.SpecSHA256,
			}
			wcEvData, err := marshalIndented(wcEv)
			if err != nil {
				explanation := fmt.Sprintf("Failed to marshal workcell %s evidence: %v", state.ID, err)
				return failClosedWithPacket("failed", "failed", explanation, "wc-evidence-marshal-failed", "ao-forge", false, evidenceList)
			}
			wcEvPath := filepath.Join(summaryDir, fmt.Sprintf("ao2-wc-%s-evidence.json", state.ID))
			if err := writeFile(wcEvPath, wcEvData); err != nil {
				explanation := fmt.Sprintf("Failed to write workcell %s evidence: %v", state.ID, err)
				return failClosedWithPacket("failed", "failed", explanation, "wc-evidence-write-failed", "ao-forge", false, evidenceList)
			}
			sum := sha256.Sum256(wcEvData)
			evidenceList = append(evidenceList, struct {
				Label         string `json:"label"`
				SchemaVersion string `json:"schema_version"`
				Status        string `json:"status"`
				Path          string `json:"path"`
				SHA256        string `json:"sha256"`
			}{
				Label:         fmt.Sprintf("workcell %s evidence", state.ID),
				SchemaVersion: "ao2.workcell-evidence.v1",
				Status:        state.Status,
				Path:          displayPath(wcEvPath),
				SHA256:        hex.EncodeToString(sum[:]),
			})

			if state.Peers > 1 && len(state.PeerStates) > 0 {
				for _, peerState := range state.PeerStates {
					peerEv := map[string]any{
						"schema_version": "ao2.workcell-evidence.v1",
						"workcell_id":    state.ID,
						"status":         peerState.Status,
						"stdout":         peerState.Stdout,
						"stderr":         peerState.Stderr,
						"spec_sha256":    state.SpecSHA256,
					}
					peerEvData, err := marshalIndented(peerEv)
					if err != nil {
						explanation := fmt.Sprintf("Failed to marshal workcell %s peer %d evidence: %v", state.ID, peerState.Index, err)
						return failClosedWithPacket("failed", "failed", explanation, "wc-peer-evidence-marshal-failed", "ao-forge", false, evidenceList)
					}
					peerEvPath := filepath.Join(summaryDir, fmt.Sprintf("ao2-wc-%s-peer-%d-evidence.json", state.ID, peerState.Index))
					if err := writeFile(peerEvPath, peerEvData); err != nil {
						explanation := fmt.Sprintf("Failed to write workcell %s peer %d evidence: %v", state.ID, peerState.Index, err)
						return failClosedWithPacket("failed", "failed", explanation, "wc-peer-evidence-write-failed", "ao-forge", false, evidenceList)
					}
					peerSum := sha256.Sum256(peerEvData)
					evidenceList = append(evidenceList, struct {
						Label         string `json:"label"`
						SchemaVersion string `json:"schema_version"`
						Status        string `json:"status"`
						Path          string `json:"path"`
						SHA256        string `json:"sha256"`
					}{
						Label:         fmt.Sprintf("workcell %s peer %d evidence", state.ID, peerState.Index),
						SchemaVersion: "ao2.workcell-evidence.v1",
						Status:        peerState.Status,
						Path:          displayPath(peerEvPath),
						SHA256:        hex.EncodeToString(peerSum[:]),
					})
				}
			}
		}
	}

	// If scheduler failed, fail closed now with all collected evidence
	if runErr != nil {
		explanation := fmt.Sprintf("Workcell execution failed: %v", runErr)
		return failClosedWithPacket(packetStatus, workcellStatus, explanation, "ao2-execution-failed", "ao-forge", false, evidenceList)
	}

	// Control plane readback if required or if in live mode
	isCPRequired := plan.Constraints.RequireControlPlaneReadback
	if isCPRequired || liveMode {
		cpURL := resolveControlPlaneURL(controlPlaneURL)
		cpToken := resolveControlPlaneToken()
		if cpToken == "" {
			if isCPRequired {
				explanation := "Control plane readback is required, but API token is missing"
				return failClosedWithPacket("blocked", "blocked", explanation, "control-plane-unauthorized", "ao-forge", true, evidenceList)
			}
			fmt.Fprintln(stderr, "Warning: Control plane API token is missing, skipping optional control plane upload")
		} else {
			cpReceiptData, cpErr := performControlPlaneUploadAndReadback(cpURL, cpToken, plan, evidenceList)
			if cpErr != nil {
				if isCPRequired {
					explanation := fmt.Sprintf("Control plane readback failed: %v", cpErr)
					return failClosedWithPacket("blocked", "blocked", explanation, "control-plane-readback-failed", "ao-forge", true, evidenceList)
				}
				fmt.Fprintf(stderr, "Warning: Optional control plane readback failed: %v\n", cpErr)
			} else {
				// Save the receipt as control-plane-receipt.json and append it to the packet's evidence list
				receiptDir := "."
				if outPath != "" {
					receiptDir = filepath.Dir(outPath)
				}
				receiptPath := filepath.Join(receiptDir, "control-plane-receipt.json")
				if err := writeFile(receiptPath, cpReceiptData); err != nil {
					if isCPRequired {
						explanation := fmt.Sprintf("Failed to write control plane receipt: %v", err)
						return failClosedWithPacket("failed", "failed", explanation, "control-plane-receipt-write-failed", "ao-forge", false, evidenceList)
					}
					fmt.Fprintf(stderr, "Warning: Failed to write optional control plane receipt: %v\n", err)
				} else {
					sum := sha256.Sum256(cpReceiptData)
					evidenceList = append(evidenceList, struct {
						Label         string `json:"label"`
						SchemaVersion string `json:"schema_version"`
						Status        string `json:"status"`
						Path          string `json:"path"`
						SHA256        string `json:"sha256"`
					}{
						Label:         "control plane readback receipt",
						SchemaVersion: "ao2.cp-ingest-receipt.v1",
						Status:        "passed",
						Path:          displayPath(receiptPath),
						SHA256:        hex.EncodeToString(sum[:]),
					})
				}
			}
		}
	}

	// Construct and write final passed factory packet
	packet := factoryPacket{
		SchemaVersion: packetSchemaVersion,
		Status:        "passed",
	}
	packet.Objective.Text = plan.Objective.Text
	packet.Objective.Workspace = plan.Objective.Workspace
	packet.Objective.ReleaseMode = plan.Objective.ReleaseMode
	packet.FactoryPlan.PlanID = plan.PlanID
	packet.FactoryPlan.WorkcellCount = len(plan.Workcells)

	packet.PolicyDecisions = []struct {
		DecisionID  string `json:"decision_id"`
		Target      string `json:"target"`
		Decision    string `json:"decision"`
		Explanation string `json:"explanation"`
		Source      string `json:"source"`
	}{
		{
			DecisionID:  gate.Decision.DecisionID,
			Target:      "factory-plan",
			Decision:    "allow",
			Explanation: gate.Decision.Explanation,
			Source:      gate.Decision.Source,
		},
	}

	packet.Workcells = make([]struct {
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
	}, len(plan.Workcells))

	for i, wc := range plan.Workcells {
		packet.Workcells[i].WorkcellID = wc.WorkcellID
		packet.Workcells[i].Kind = wc.Kind
		packet.Workcells[i].Workspace = wc.Workspace
		packet.Workcells[i].Executor = wc.Executor
		packet.Workcells[i].Peers = wc.Peers
		packet.Workcells[i].MaxRepairs = wc.MaxRepairs
		packet.Workcells[i].Task = wc.Task
		packet.Workcells[i].DependsOn = cloneStrings(wc.DependsOn)
		packet.Workcells[i].AO2Run = "none"
		if schedulerStates != nil && i < len(schedulerStates) {
			packet.Workcells[i].Status = schedulerStates[i].Status
			packet.Workcells[i].Summary = schedulerStates[i].Summary
			packet.Workcells[i].RepairsAttempted = schedulerStates[i].RepairsAttempted
			if schedulerStates[i].Status == "passed" {
				if liveMode {
					packet.Workcells[i].AO2Run = "live"
				} else {
					packet.Workcells[i].AO2Run = "dry-run"
				}
			}
		} else {
			packet.Workcells[i].Status = "passed"
			if liveMode {
				packet.Workcells[i].AO2Run = "live"
				packet.Workcells[i].Summary = "Governed run started by ao2"
			} else {
				packet.Workcells[i].AO2Run = "dry-run"
				packet.Workcells[i].Summary = "Dry-run accepted by ao2"
			}
		}
	}

	packet.Evidence = evidenceList

	packet.TrustBoundary.LocalFirst = plan.Constraints.LocalFirst
	packet.TrustBoundary.MutatesReleases = liveMode && plan.Objective.ReleaseMode
	packet.TrustBoundary.StoresCredentials = false
	packet.TrustBoundary.ControlPlaneApprovesWork = false

	packet.NextActions = []nextAction{
		{
			ActionID:    "close-factory-packet",
			Description: func() string {
				if liveMode {
					return "Review the factory packet and live evidence."
				}
				return "Review the factory packet and dry-run evidence."
			}(),
			Required:    false,
		},
	}

	packetData, err := marshalIndented(packet)
	if err != nil {
		fmt.Fprintf(stderr, "forge run: encode final packet: %v\n", err)
		return 1
	}

	archiveRunState(plan.PlanID, planPath, gateResultPath, summaryPath, packetData, packet)

	if liveMode && plan.Objective.ReleaseMode && confirmRelease {
		if err := performReleaseMutation(plan, outPath, evidenceList, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "forge run: release mutation failed: %v\n", err)
			return 1
		}
	}

	if outPath != "" {
		if err := writeFile(outPath, packetData); err != nil {
			fmt.Fprintf(stderr, "forge run: write final packet: %v\n", err)
			return 1
		}
		_ = writeMarkdownPacket(outPath, packet)
		fmt.Fprintf(stdout, "factory_packet=%s\n", displayPath(outPath))
	} else {
		_, _ = stdout.Write(packetData)
	}

	return 0
}

func runResume(args []string, stdout, stderr io.Writer) int {
	var runID, outPath string
	var controlPlaneURL string
	var releasePreviewAuditPath string
	var liveMode bool
	var confirmRelease bool
	var nonInteractive bool
	var noDashboard bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--run":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge resume: --run requires a value")
				return 2
			}
			runID = args[i+1]
			i++
		case "--out":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge resume: --out requires a value")
				return 2
			}
			outPath = args[i+1]
			i++
		case "--control-plane":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge resume: --control-plane requires a value")
				return 2
			}
			controlPlaneURL = args[i+1]
			i++
		case "--release-preview-audit":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge resume: --release-preview-audit requires a value")
				return 2
			}
			releasePreviewAuditPath = args[i+1]
			i++
		case "--live":
			liveMode = true
		case "--confirm-release":
			confirmRelease = true
		case "--non-interactive", "--yes", "-y":
			nonInteractive = true
		case "--no-dashboard":
			noDashboard = true
		default:
			fmt.Fprintf(stderr, "forge resume: unexpected argument %s\n", args[i])
			return 2
		}
	}

	if runID == "" {
		fmt.Fprintln(stderr, "forge resume: missing required --run")
		return 2
	}

	dotForge := ".forge"
	if info, err := os.Stat(dotForge); err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "forge resume: local state directory .forge not found (run forge init first)\n")
		return 1
	}

	runDir := filepath.Join(dotForge, "runs", runID)
	if info, err := os.Stat(runDir); err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "forge resume: run ID %q not found under .forge/runs/\n", runID)
		return 1
	}

	planPath := filepath.Join(runDir, "plan.json")
	gateResultPath := filepath.Join(runDir, "gate_result.json")
	packetPath := filepath.Join(runDir, "factory-packet.json")

	if _, err := os.Stat(planPath); err != nil {
		fmt.Fprintf(stderr, "forge resume: plan.json not found in run directory %q\n", runDir)
		return 1
	}
	if _, err := os.Stat(gateResultPath); err != nil {
		fmt.Fprintf(stderr, "forge resume: gate_result.json not found in run directory %q\n", runDir)
		return 1
	}

	plan, err := readPlan(planPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge resume: read plan: %v\n", err)
		return 1
	}

	prevStates := make(map[string]*workcellRunState)
	if _, err := os.Stat(packetPath); err == nil {
		if packetData, err := os.ReadFile(packetPath); err == nil {
			var prevPacket factoryPacket
			if err := json.Unmarshal(packetData, &prevPacket); err == nil {
				for _, prevWc := range prevPacket.Workcells {
					if prevWc.Status == "passed" {
						wcEvPath := filepath.Join(runDir, fmt.Sprintf("ao2-wc-%s-evidence.json", prevWc.WorkcellID))
						var stdoutText, stderrText, specSHA string
						if evData, err := os.ReadFile(wcEvPath); err == nil {
							var evObj map[string]any
							if err := json.Unmarshal(evData, &evObj); err == nil {
								if st, ok := evObj["stdout"].(string); ok {
									stdoutText = st
								}
								if se, ok := evObj["stderr"].(string); ok {
									stderrText = se
								}
								if sh, ok := evObj["spec_sha256"].(string); ok {
									specSHA = sh
								}
							}
						}
						var peerStates []*peerRunState
						if prevWc.Peers > 1 {
							for idx := 0; idx < prevWc.Peers; idx++ {
								peerEvPath := filepath.Join(runDir, fmt.Sprintf("ao2-wc-%s-peer-%d-evidence.json", prevWc.WorkcellID, idx))
								var pStdout, pStderr, pStatus string
								if pEvData, err := os.ReadFile(peerEvPath); err == nil {
									var pEvObj map[string]any
									if err := json.Unmarshal(pEvData, &pEvObj); err == nil {
										if st, ok := pEvObj["stdout"].(string); ok {
											pStdout = st
										}
										if se, ok := pEvObj["stderr"].(string); ok {
											pStderr = se
										}
										if s, ok := pEvObj["status"].(string); ok {
											pStatus = s
										}
									}
								}
								peerStates = append(peerStates, &peerRunState{
									stateMu: &sync.Mutex{},
									Index:   idx,
									Status:  pStatus,
									Stdout:  pStdout,
									Stderr:  pStderr,
								})
							}
						}

						prevStates[prevWc.WorkcellID] = &workcellRunState{
							ID:               prevWc.WorkcellID,
							Status:           "passed",
							Summary:          prevWc.Summary,
							Stdout:           stdoutText,
							Stderr:           stderrText,
							SpecSHA256:       specSHA,
							Workspace:        prevWc.Workspace,
							Executor:         prevWc.Executor,
							Peers:            prevWc.Peers,
							MaxRepairs:       prevWc.MaxRepairs,
							RepairsAttempted: prevWc.RepairsAttempted,
							Task:             prevWc.Task,
							PeerStates:       peerStates,
						}
					}
				}
			}
		}
	}

	return executePlanRun(plan, planPath, gateResultPath, outPath, controlPlaneURL, releasePreviewAuditPath, liveMode, confirmRelease, nonInteractive, noDashboard, os.Stdin, prevStates, stdout, stderr)
}

func runOnce(args []string, stdout, stderr io.Writer) int {
	var briefPath, covenantPath, outPath string
	var controlPlaneURL string
	var releasePreviewAuditPath string
	var workspacePath string
	var liveMode bool
	var confirmRelease bool
	var nonInteractive bool
	var noDashboard bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--brief":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge once: --brief requires a value")
				return 2
			}
			briefPath = args[i+1]
			i++
		case "--covenant":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge once: --covenant requires a value")
				return 2
			}
			covenantPath = args[i+1]
			i++
		case "--out":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge once: --out requires a value")
				return 2
			}
			outPath = args[i+1]
			i++
		case "--control-plane":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge once: --control-plane requires a value")
				return 2
			}
			controlPlaneURL = args[i+1]
			i++
		case "--release-preview-audit":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge once: --release-preview-audit requires a value")
				return 2
			}
			releasePreviewAuditPath = args[i+1]
			i++
		case "--workspace":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge once: --workspace requires a value")
				return 2
			}
			workspacePath = args[i+1]
			i++
		case "--live":
			liveMode = true
		case "--confirm-release":
			confirmRelease = true
		case "--non-interactive", "--yes", "-y":
			nonInteractive = true
		case "--no-dashboard":
			noDashboard = true
		default:
			fmt.Fprintf(stderr, "forge once: unexpected argument %s\n", args[i])
			return 2
		}
	}

	if briefPath == "" {
		fmt.Fprintln(stderr, "forge once: missing required --brief")
		return 2
	}
	if covenantPath == "" {
		fmt.Fprintln(stderr, "forge once: missing required --covenant")
		return 2
	}

	// 1. Generate plan from brief
	brief, canonical, err := readBrief(briefPath, false)
	if err != nil {
		fmt.Fprintf(stderr, "forge once: %v\n", err)
		return 1
	}

	if workspacePath != "" {
		brief.Objective.Workspace = workspacePath
		rawBytes, err := json.Marshal(brief)
		if err != nil {
			fmt.Fprintf(stderr, "forge once: marshal brief after workspace override: %v\n", err)
			return 1
		}
		var raw any
		if err := json.Unmarshal(rawBytes, &raw); err != nil {
			fmt.Fprintf(stderr, "forge once: canonicalize brief after workspace override: %v\n", err)
			return 1
		}
		canonical, err = json.Marshal(raw)
		if err != nil {
			fmt.Fprintf(stderr, "forge once: marshal canonical brief after workspace override: %v\n", err)
			return 1
		}
	}

	plan := buildPlan(brief, canonical)
	if err := validatePlan(plan); err != nil {
		fmt.Fprintf(stderr, "forge once: generated plan failed contract validation: %v\n", err)
		return 1
	}
	planData, err := marshalIndented(plan)
	if err != nil {
		fmt.Fprintf(stderr, "forge once: encode plan: %v\n", err)
		return 1
	}

	tempPlan, err := os.CreateTemp("", "once-plan-*.json")
	if err != nil {
		fmt.Fprintf(stderr, "forge once: create temporary plan file: %v\n", err)
		return 1
	}
	defer os.Remove(tempPlan.Name())
	if _, err := tempPlan.Write(planData); err != nil {
		tempPlan.Close()
		fmt.Fprintf(stderr, "forge once: write temporary plan file: %v\n", err)
		return 1
	}
	tempPlan.Close()

	// 2. Evaluate policy gate
	var gateStdout, gateStderr bytes.Buffer
	gateCode := runGate([]string{"--plan", tempPlan.Name(), "--covenant", covenantPath}, &gateStdout, &gateStderr)

	// Since we want to fail closed if gate fails, but write a valid packet summary, we write gate output to temp file
	tempGate, err := os.CreateTemp("", "once-gate-*.json")
	if err != nil {
		fmt.Fprintf(stderr, "forge once: create temporary gate file: %v\n", err)
		return 1
	}
	defer os.Remove(tempGate.Name())
	if _, err := tempGate.Write(gateStdout.Bytes()); err != nil {
		tempGate.Close()
		fmt.Fprintf(stderr, "forge once: write temporary gate file: %v\n", err)
		return 1
	}
	tempGate.Close()

	// 3. Execute runRun with our temp plan and gate result files
	runArgs := []string{"--plan", tempPlan.Name(), "--gate-result", tempGate.Name()}
	if outPath != "" {
		runArgs = append(runArgs, "--out", outPath)
	}
	if controlPlaneURL != "" {
		runArgs = append(runArgs, "--control-plane", controlPlaneURL)
	}
	if liveMode {
		runArgs = append(runArgs, "--live")
	}
	if confirmRelease {
		runArgs = append(runArgs, "--confirm-release")
	}
	if releasePreviewAuditPath != "" {
		runArgs = append(runArgs, "--release-preview-audit", releasePreviewAuditPath)
	}
	if nonInteractive {
		runArgs = append(runArgs, "--non-interactive")
	}
	if noDashboard {
		runArgs = append(runArgs, "--no-dashboard")
	}

	runCode := runRun(runArgs, stdout, stderr)
	if gateCode != 0 && runCode == 0 {
		return gateCode
	}
	return runCode
}

func parseAo2DryRunOutput(output string) map[string]any {
	result := make(map[string]any)
	result["schema_version"] = "ao2.run/v1"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		val := parts[1]
		switch key {
		case "task_count":
			var count int
			if _, err := fmt.Sscanf(val, "%d", &count); err == nil {
				result[key] = count
			} else {
				result[key] = val
			}
		case "mutates_ao_artifacts", "factory_v3_drives_workflow":
			result[key] = (val == "true")
		default:
			result[key] = val
		}
	}
	return result
}

type cpOperatorPacket struct {
	SchemaVersion  string               `json:"schema_version"`
	RunID          string               `json:"run_id"`
	Status         string               `json:"status"`
	OperatorID     string               `json:"operator_id"`
	GeneratedAtUTC string               `json:"generated_at_utc"`
	Summary        cpPacketSummary      `json:"summary"`
	Evidence       []cpPacketEvidence   `json:"evidence"`
	TrustBoundary  cpTrustBoundary      `json:"trust_boundary"`
}

type cpPacketSummary struct {
	RecommendedTask string `json:"recommended_task"`
	EvidenceCount   int    `json:"evidence_count"`
}

type cpPacketEvidence struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
}

type cpTrustBoundary struct {
	ControlPlaneRole string `json:"control_plane_role"`
	MutatesAo2       bool   `json:"mutates_ao2"`
}

type cpSignature struct {
	SchemaVersion      string `json:"schema_version"`
	SignatureAlgorithm string `json:"signature_algorithm"`
	SignatureHex       string `json:"signature_hex"`
	PublicKeyPEM       string `json:"public_key_pem"`
	SignerID           string `json:"signer_id"`
	SignatureSHA256    string `json:"signature_sha256,omitempty"`
	PublicKeySHA256    string `json:"public_key_sha256,omitempty"`
}

type cpSignedUpload struct {
	SchemaVersion     string          `json:"schema_version"`
	OperatorPacket    cpOperatorPacket `json:"operator_packet"`
	OperatorPacketB64 string          `json:"operator_packet_b64"`
	Signature         cpSignature     `json:"signature"`
}

func generateTransientRSAKey() (*rsa.PrivateKey, string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", err
	}
	pubASN1, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, "", err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubASN1,
	})
	return priv, string(pubPEM), nil
}

func signPayloadRSA_SHA256(priv *rsa.PrivateKey, payload []byte) ([]byte, error) {
	hashed := sha256.Sum256(payload)
	signature, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func resolveControlPlaneURL(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if env := os.Getenv("AO2_CP_URL"); env != "" {
		return env
	}
	if env := os.Getenv("AO_FORGE_CP_URL"); env != "" {
		return env
	}
	return "http://127.0.0.1:8744"
}

func resolveControlPlaneToken() string {
	if token := os.Getenv("AO2_CP_API_TOKEN"); token != "" {
		return token
	}
	if token := os.Getenv("AO_FORGE_CP_API_TOKEN"); token != "" {
		return token
	}
	if token := os.Getenv("AO2_CP_AUTH_VALUE"); token != "" {
		return token
	}
	return ""
}

func performControlPlaneUploadAndReadback(
	controlPlaneURL string,
	token string,
	plan factoryPlan,
	evidenceList []struct {
		Label         string `json:"label"`
		SchemaVersion string `json:"schema_version"`
		Status        string `json:"status"`
		Path          string `json:"path"`
		SHA256        string `json:"sha256"`
	},
) ([]byte, error) {
	privKey, pubKeyPEM, err := generateTransientRSAKey()
	if err != nil {
		return nil, fmt.Errorf("generate transient RSA key: %w", err)
	}

	cpPacket := cpOperatorPacket{
		SchemaVersion:  "ao2.operator-evidence-packet.v1",
		RunID:          plan.PlanID,
		Status:         "passed",
		OperatorID:     "ao-forge-operator",
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		Summary: cpPacketSummary{
			RecommendedTask: "verify signed operator packet readback",
			EvidenceCount:   len(evidenceList),
		},
		Evidence:      make([]cpPacketEvidence, 0, len(evidenceList)),
		TrustBoundary: cpTrustBoundary{
			ControlPlaneRole: "read_only_observer",
			MutatesAo2:       false,
		},
	}
	for _, ev := range evidenceList {
		cpPacket.Evidence = append(cpPacket.Evidence, cpPacketEvidence{
			Kind:   ev.Label,
			SHA256: ev.SHA256,
		})
	}

	packetData, err := json.MarshalIndent(cpPacket, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal operator packet: %w", err)
	}

	signatureBytes, err := signPayloadRSA_SHA256(privKey, packetData)
	if err != nil {
		return nil, fmt.Errorf("sign operator packet: %w", err)
	}
	signatureHex := hex.EncodeToString(signatureBytes)

	sigHashVal := sha256.Sum256(signatureBytes)
	signatureSHA256 := hex.EncodeToString(sigHashVal[:])

	pubKeyHashVal := sha256.Sum256([]byte(pubKeyPEM))
	pubKeySHA256 := hex.EncodeToString(pubKeyHashVal[:])

	uploadPayload := cpSignedUpload{
		SchemaVersion:     "ao2.cp-operator-packet-signed-upload.v1",
		OperatorPacket:    cpPacket,
		OperatorPacketB64: base64.StdEncoding.EncodeToString(packetData),
		Signature: cpSignature{
			SchemaVersion:      "ao2.cp-operator-packet-signature.v1",
			SignatureAlgorithm: "RSA/SHA-256",
			SignatureHex:       signatureHex,
			PublicKeyPEM:       pubKeyPEM,
			SignerID:           "ao-forge-operator",
			SignatureSHA256:    signatureSHA256,
			PublicKeySHA256:    pubKeySHA256,
		},
	}

	uploadData, err := json.Marshal(uploadPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal upload payload: %w", err)
	}

	uploadURL := strings.TrimSuffix(controlPlaneURL, "/") + "/api/v1/operator-packet/signed"
	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(uploadData))
	if err != nil {
		return nil, fmt.Errorf("create POST request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upload response: %w", err)
	}

	var receipt struct {
		SchemaVersion         string    `json:"schema_version"`
		SHA256                string    `json:"sha256"`
		StoredAt              time.Time `json:"stored_at"`
		IngestedSchemaVersion string    `json:"ingested_schema_version"`
	}
	if err := json.Unmarshal(respData, &receipt); err != nil {
		return nil, fmt.Errorf("unmarshal ingest receipt: %w", err)
	}

	if receipt.SchemaVersion != "ao2.cp-ingest-receipt.v1" {
		return nil, fmt.Errorf("unexpected receipt schema version: %q", receipt.SchemaVersion)
	}
	if receipt.SHA256 == "" {
		return nil, fmt.Errorf("receipt missing sha256")
	}

	readbackURL := strings.TrimSuffix(controlPlaneURL, "/") + "/api/v1/operator-packet/" + receipt.SHA256
	getReq, err := http.NewRequest("GET", readbackURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create GET request: %w", err)
	}
	if token != "" {
		getReq.Header.Set("Authorization", "Bearer "+token)
	}

	getResp, err := client.Do(getReq)
	if err != nil {
		return nil, fmt.Errorf("GET readback request failed: %w", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(getResp.Body)
		return nil, fmt.Errorf("readback failed with status %d: %s", getResp.StatusCode, string(bodyBytes))
	}

	readbackBytes, err := io.ReadAll(getResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read readback response: %w", err)
	}

	var readbackPacket cpOperatorPacket
	if err := json.Unmarshal(readbackBytes, &readbackPacket); err != nil {
		return nil, fmt.Errorf("unmarshal readback packet: %w", err)
	}

	var originalParsed cpOperatorPacket
	if err := json.Unmarshal(packetData, &originalParsed); err != nil {
		return nil, fmt.Errorf("unmarshal original packet: %w", err)
	}

	if !reflect.DeepEqual(readbackPacket, originalParsed) {
		return nil, fmt.Errorf("readback payload mismatch from original packet")
	}

	return respData, nil
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

type peerRunState struct {
	stateMu *sync.Mutex
	Index   int
	Status  string // "pending", "running", "passed", "failed"
	Stdout  string
	Stderr  string
	Summary string
	Cost    float64
	Tokens  float64
}

type workcellRunState struct {
	stateMu    *sync.Mutex
	ID         string
	Kind       string
	Workspace  string
	Executor   string
	Peers      int
	MaxRepairs int
	Task       string
	DependsOn  []string
	Status     string // "pending", "running", "passed", "failed", "skipped"
	Summary    string
	Stdout     string
	Stderr     string
	SpecSHA256 string
	Rubric     *workcellRubric
	PeerStates []*peerRunState
	RepairsAttempted int
}

func (w *workcellRunState) GetStatus() string {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	return w.Status
}

func (w *workcellRunState) SetStatus(status string) {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Status = status
}

func (w *workcellRunState) GetSummary() string {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	return w.Summary
}

func (w *workcellRunState) SetSummary(sum string) {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Summary = sum
}

func (w *workcellRunState) AppendStdout(data string) {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Stdout += data
}

func (w *workcellRunState) AppendStderr(data string) {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Stderr += data
}

func (w *workcellRunState) GetStdout() string {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	return w.Stdout
}

func (w *workcellRunState) GetStderr() string {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	return w.Stderr
}

type realTimeWriter struct {
	appendFunc func(string)
}

func (w *realTimeWriter) Write(p []byte) (n int, err error) {
	w.appendFunc(string(p))
	return len(p), nil
}

func runWorkcellsConcurrent(
	ctx context.Context,
	plan factoryPlan,
	ao2Path string,
	stdout, stderr io.Writer,
	liveMode bool,
	nonInteractive bool,
	noDashboard bool,
	stdin io.Reader,
	prevStates map[string]*workcellRunState,
) ([]workcellRunState, error) {
	// Initialize state
	states := make(map[string]*workcellRunState)
	for _, wc := range plan.Workcells {
		status := "pending"
		var existingSummary, existingStdout, existingStderr, existingSpecSHA256 string
		var existingRepairsAttempted int
		if prevStates != nil {
			if prev, ok := prevStates[wc.WorkcellID]; ok {
				if prev.Status == "passed" {
					status = "passed"
					existingSummary = prev.Summary
					existingStdout = prev.Stdout
					existingStderr = prev.Stderr
					existingSpecSHA256 = prev.SpecSHA256
					existingRepairsAttempted = prev.RepairsAttempted
				}
			}
		}
		var peerStates []*peerRunState
		if prevStates != nil {
			if prev, ok := prevStates[wc.WorkcellID]; ok {
				peerStates = prev.PeerStates
			}
		}
		states[wc.WorkcellID] = &workcellRunState{
			stateMu:          &sync.Mutex{},
			ID:               wc.WorkcellID,
			Kind:             wc.Kind,
			Workspace:        wc.Workspace,
			Executor:         wc.Executor,
			Peers:            wc.Peers,
			MaxRepairs:       wc.MaxRepairs,
			RepairsAttempted: existingRepairsAttempted,
			Task:             wc.Task,
			DependsOn:        wc.DependsOn,
			Status:           status,
			Summary:          existingSummary,
			Stdout:           existingStdout,
			Stderr:           existingStderr,
			SpecSHA256:       existingSpecSHA256,
			Rubric:           wc.Rubric,
			PeerStates:       peerStates,
		}
	}

	var mu sync.Mutex
	// Use a WaitGroup to wait for all running goroutines to complete
	var wg sync.WaitGroup
	var promptMu sync.Mutex

	var tuiFd uintptr
	useDashboard := false
	if f, ok := stderr.(*os.File); ok && !nonInteractive && !noDashboard {
		if isTerminal(f.Fd()) {
			useDashboard = true
			tuiFd = f.Fd()
		}
	}

	if useDashboard {
		enterAlternateScreen(stderr)
		defer exitAlternateScreen(stderr)

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			exitAlternateScreen(stderr)
			os.Exit(130)
		}()
		defer signal.Stop(sigChan)

		d := &dashboard{
			plan:      plan,
			states:    states,
			mu:        &mu,
			startTime: time.Now(),
			writer:    stderr,
		}

		doneChan := make(chan struct{})
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					d.render(tuiFd)
				case <-doneChan:
					return
				}
			}
		}()
		defer func() {
			close(doneChan)
			d.render(tuiFd)
		}()
	}

	// Create a cancellable context to abort pending runs if one fails
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var firstErr error
	var errOnce sync.Once
	setFailure := func(err error) {
		errOnce.Do(func() {
			firstErr = err
			cancel() // cancel all other running/pending tasks
		})
	}

	// We run a loop until all tasks are either finished (passed/failed) or skipped
	for {
		// Select all ready tasks
		mu.Lock()
		var readyTasks []*workcellRunState
		allFinished := true
		for _, state := range states {
			if state.Status == "pending" {
				allFinished = false
				// Check if all dependencies have passed
				depsPassed := true
				for _, dep := range state.DependsOn {
					depState := states[dep]
					if depState == nil || depState.Status != "passed" {
						depsPassed = false
						// If any dependency has failed or is skipped, this task must be skipped
						if depState != nil && (depState.Status == "failed" || depState.Status == "skipped") {
							state.Status = "skipped"
							state.Summary = "Dependency failed or was skipped"
						}
						break
					}
				}
				if depsPassed {
					state.Status = "running"
					readyTasks = append(readyTasks, state)
				}
			} else if state.Status == "running" {
				allFinished = false
			}
		}
		mu.Unlock()

		if allFinished {
			break
		}

		if len(readyTasks) == 0 {
			// If nothing is ready but we aren't finished, check if the context is cancelled
			if ctx.Err() != nil {
				// Cancelled due to a failure, mark remaining pending as skipped
				mu.Lock()
				for _, state := range states {
					if state.Status == "pending" {
						state.Status = "skipped"
						state.Summary = "Run cancelled due to upstream failure"
					}
				}
				mu.Unlock()
				break
			}
			// Otherwise, wait a short duration and check status again
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Launch ready tasks
		for _, task := range readyTasks {
			wg.Add(1)
			go func(t *workcellRunState) {
				defer wg.Done()

				// Run task
				err := executeSingleWorkcell(ctx, plan, t, ao2Path, liveMode)

				if err != nil {
					if !nonInteractive {
						promptMu.Lock()

						// Check if context has been cancelled by another goroutine's abort action
						if ctx.Err() != nil {
							mu.Lock()
							t.Status = "skipped"
							t.Summary = "Cancelled during execution"
							mu.Unlock()
							promptMu.Unlock()
							return
						}

						fmt.Fprintf(stderr, "\nWorkcell [%s] failed.\nError: %v\n", t.ID, err)
						if t.Stdout != "" {
							fmt.Fprintf(stderr, "Stdout: %s\n", t.Stdout)
						}
						if t.Stderr != "" {
							fmt.Fprintf(stderr, "Stderr: %s\n", t.Stderr)
						}
						fmt.Fprintf(stderr, "Choose action: (r)etry, (s)kip and continue, or (a)bort? [r/s/A]: ")

						response, scanErr := readStdinLine(stdin)
						promptMu.Unlock()

						if scanErr == nil {
							respLower := strings.ToLower(strings.TrimSpace(response))
							if respLower == "r" || respLower == "retry" {
								// Loop to retry until success or abort/skip
								retryCount := 0
								for {
									retryCount++
									fmt.Fprintf(stderr, "\nRetrying workcell [%s] (attempt %d)...\n", t.ID, retryCount)
									err = executeSingleWorkcell(ctx, plan, t, ao2Path, liveMode)
									if err == nil {
										mu.Lock()
										t.Status = "passed"
										if t.Summary == "" {
											if liveMode {
												t.Summary = "Governed run started by ao2"
											} else {
												t.Summary = "Dry-run accepted by ao2"
											}
										}
										mu.Unlock()
										return
									}

									// If it fails again, lock promptMu to prompt again
									promptMu.Lock()
									if ctx.Err() != nil {
										mu.Lock()
										t.Status = "skipped"
										t.Summary = "Cancelled during execution"
										mu.Unlock()
										promptMu.Unlock()
										return
									}
									fmt.Fprintf(stderr, "\nWorkcell [%s] failed on retry %d.\nError: %v\n", t.ID, retryCount, err)
									if t.Stdout != "" {
										fmt.Fprintf(stderr, "Stdout: %s\n", t.Stdout)
									}
									if t.Stderr != "" {
										fmt.Fprintf(stderr, "Stderr: %s\n", t.Stderr)
									}
									fmt.Fprintf(stderr, "Choose action: (r)etry, (s)kip and continue, or (a)bort? [r/s/A]: ")
									response, scanErr = readStdinLine(stdin)
									promptMu.Unlock()

									if scanErr != nil {
										break
									}
									respLower = strings.ToLower(strings.TrimSpace(response))
									if respLower != "r" && respLower != "retry" {
										break
									}
								}

								// Process post-retry response (either skip or abort)
								respLower = strings.ToLower(strings.TrimSpace(response))
								if respLower == "s" || respLower == "skip" {
									mu.Lock()
									t.Status = "skipped"
									t.Summary = "Skipped by operator after failure: " + err.Error()
									mu.Unlock()
									return
								}
							} else if respLower == "s" || respLower == "skip" {
								mu.Lock()
								t.Status = "skipped"
								t.Summary = "Skipped by operator after failure: " + err.Error()
								mu.Unlock()
								return
							}
						}
					}

					mu.Lock()
					t.Status = "failed"
					t.Summary = err.Error()
					setFailure(err)
					mu.Unlock()
				} else {
					mu.Lock()
					// Check if context was cancelled while we were running
					if ctx.Err() != nil {
						t.Status = "skipped"
						t.Summary = "Cancelled during execution"
					} else {
						t.Status = "passed"
						if t.Summary == "" {
							if liveMode {
								t.Summary = "Governed run started by ao2"
							} else {
								t.Summary = "Dry-run accepted by ao2"
							}
						}
					}
					mu.Unlock()
				}
			}(task)
		}
	}

	wg.Wait()

	// Convert map back to ordered slice
	orderedStates := make([]workcellRunState, len(plan.Workcells))
	for i, wc := range plan.Workcells {
		orderedStates[i] = *states[wc.WorkcellID]
	}

	return orderedStates, firstErr
}

func executeSingleWorkcell(ctx context.Context, plan factoryPlan, wcState *workcellRunState, ao2Path string, liveMode bool) error {
	if wcState.Peers > 1 && wcState.Executor != "agy-swarms" {
		return fmt.Errorf("parallel peer execution is only supported for executor 'agy-swarms', got %q", wcState.Executor)
	}

	repoPath := plan.Objective.Workspace
	if wcState.Workspace != "" {
		repoPath = wcState.Workspace
	}

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

func (w *workcellRunState) ResetOutputs() {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Stdout = ""
	w.Stderr = ""
	w.Summary = ""
	w.SpecSHA256 = ""
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
			Tasks: []runTask{specTask},
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
		cmd = exec.CommandContext(ctx, ao2Path, "run", "--spec", tempSpec.Name())
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

func archiveRunState(runID string, planPath string, gateResultPath string, summaryPath string, packetData []byte, packet factoryPacket) {
	dotForge := ".forge"
	if info, err := os.Stat(dotForge); err != nil || !info.IsDir() {
		return
	}

	runDir := filepath.Join(dotForge, "runs", runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return
	}

	if planData, err := os.ReadFile(planPath); err == nil {
		_ = os.WriteFile(filepath.Join(runDir, "plan.json"), planData, 0644)
	}

	if gateData, err := os.ReadFile(gateResultPath); err == nil {
		_ = os.WriteFile(filepath.Join(runDir, "gate_result.json"), gateData, 0644)
	}

	if summaryPath != "" {
		if summaryData, err := os.ReadFile(summaryPath); err == nil {
			_ = os.WriteFile(filepath.Join(runDir, "ao2-run-summary.json"), summaryData, 0644)
		}
	}

	for _, wc := range packet.Workcells {
		wcEvName := fmt.Sprintf("ao2-wc-%s-evidence.json", wc.WorkcellID)
		var srcDir string
		if summaryPath != "" {
			srcDir = filepath.Dir(summaryPath)
		} else {
			srcDir = "."
		}
		srcPath := filepath.Join(srcDir, wcEvName)
		if evData, err := os.ReadFile(srcPath); err == nil {
			_ = os.WriteFile(filepath.Join(runDir, wcEvName), evData, 0644)
		}
		if wc.Peers > 1 {
			for idx := 0; idx < wc.Peers; idx++ {
				peerEvName := fmt.Sprintf("ao2-wc-%s-peer-%d-evidence.json", wc.WorkcellID, idx)
				peerSrcPath := filepath.Join(srcDir, peerEvName)
				if peerEvData, err := os.ReadFile(peerSrcPath); err == nil {
					_ = os.WriteFile(filepath.Join(runDir, peerEvName), peerEvData, 0644)
				}
			}
		}
	}

	_ = os.WriteFile(filepath.Join(runDir, "factory-packet.json"), packetData, 0644)
	_ = writeMarkdownPacket(filepath.Join(runDir, "factory-packet.json"), packet)
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

func buildReleasePreviewAudit(flags releasePreviewFlags) releasePreviewAudit {
	audit := releasePreviewAudit{
		SchemaVersion:   "ao.forge.release-preview-audit.v0.1",
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

	data, err := os.ReadFile(path)
	if err != nil {
		return evidence, fmt.Errorf("read audit: %w", err)
	}
	var audit releasePreviewAudit
	if err := decodeJSONStrict(data, &audit); err != nil {
		return evidence, fmt.Errorf("parse audit JSON: %w", err)
	}
	if audit.SchemaVersion != "ao.forge.release-preview-audit.v0.1" {
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
