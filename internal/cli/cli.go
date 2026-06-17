package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

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

type briefWorkcell struct {
	WorkcellID string   `json:"workcell_id"`
	Kind       string   `json:"kind"`
	DependsOn  []string `json:"depends_on"`
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
	WorkcellID string   `json:"workcell_id"`
	Kind       string   `json:"kind"`
	Status     string   `json:"status"`
	DependsOn  []string `json:"depends_on"`
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
		WorkcellID string   `json:"workcell_id"`
		Kind       string   `json:"kind"`
		Status     string   `json:"status"`
		DependsOn  []string `json:"depends_on"`
		AO2Run     string   `json:"ao2_run"`
		Summary    string   `json:"summary"`
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
	case "plan":
		return runPlan(args[1:], stdout, stderr)
	case "gate":
		return runGate(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "run", "once":
		fmt.Fprintf(stderr, "forge %s: execution is disabled until the AO2 adapter slice; run `forge gate --plan <file> --covenant <covenant>` first\n", args[0])
		return 2
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
  forge plan --brief <factory-brief.json> [--out <factory-plan.json>]
  forge gate --plan <factory-plan.json> --covenant <path-to-covenant-or-config> [--out <gate-result.json>]
  forge inspect --packet <factory-packet.json>
  forge doctor --foundation <foundation-baseline.json> [--json]

Factory terms:
  factory brief   normalized operator objective and constraints
  workcell        bounded unit of factory work with dependencies and evidence
  factory packet  operator-ready JSON summary of plan, gates, evidence, and next actions

Slice 0.3 status:
  planning, live Covenant gate adapter, and packet inspection are enabled.
  execution remains disabled until the AO2 adapter and evidence router slices
  are implemented.
`)
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	briefPath, outPath, err := parsePathFlags(args, "--brief", "--out")
	if err != nil {
		fmt.Fprintf(stderr, "forge plan: %v\n", err)
		return 2
	}
	if briefPath == "" {
		fmt.Fprintln(stderr, "forge plan: missing required --brief")
		return 2
	}

	brief, canonical, err := readBrief(briefPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge plan: %v\n", err)
		return 1
	}
	plan := buildPlan(brief, canonical)
	if err := validatePlan(plan); err != nil {
		fmt.Fprintf(stderr, "forge plan: generated plan failed contract validation: %v\n", err)
		return 1
	}
	encoded, err := marshalIndented(plan)
	if err != nil {
		fmt.Fprintf(stderr, "forge plan: encode plan: %v\n", err)
		return 1
	}

	if outPath != "" {
		if err := writeFile(outPath, encoded); err != nil {
			fmt.Fprintf(stderr, "forge plan: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "factory_plan=%s\n", outPath)
		return 0
	}

	_, _ = stdout.Write(encoded)
	return 0
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

		if plan.Constraints.AllowReleaseMutation {
			decision.DecisionID = "deny-release-mutation"
			decision.Decision = "deny"
			decision.Explanation = "The plan requests release mutation, which is denied by policy."
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

func readBrief(path string) (factoryBrief, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return factoryBrief{}, nil, fmt.Errorf("read brief: %w", err)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return factoryBrief{}, nil, fmt.Errorf("parse brief JSON: %w", err)
	}
	canonical, err := json.Marshal(raw)
	if err != nil {
		return factoryBrief{}, nil, fmt.Errorf("canonicalize brief JSON: %w", err)
	}
	if err := validateBriefRequiredFields(data); err != nil {
		return factoryBrief{}, nil, err
	}

	var brief factoryBrief
	if err := decodeJSONStrict(data, &brief); err != nil {
		return factoryBrief{}, nil, fmt.Errorf("decode brief: %w", err)
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
	if len(brief.ExpectedWorkcells) == 0 {
		return factoryBrief{}, nil, fmt.Errorf("brief expected_workcells must not be empty")
	}
	return brief, canonical, nil
}

func validateBriefRequiredFields(data []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse brief JSON: %w", err)
	}

	var missing []string
	requireFields(&missing, root, "", "schema_version", "objective", "constraints", "expected_workcells", "expected_evidence")

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
			Status:     "planned",
			DependsOn:  cloneStrings(cell.DependsOn),
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
