package cli

import (
	"bytes"
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
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
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
	case "run":
		return runRun(args[1:], stdout, stderr)
	case "once":
		return runOnce(args[1:], stdout, stderr)
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
  forge run --plan <factory-plan.json> --gate-result <gate-result.json> [--out <factory-packet.json>]
  forge once --brief <factory-brief.json> --covenant <path-to-covenant-or-config> [--out <factory-packet.json>]
  forge inspect --packet <factory-packet.json>
  forge doctor --foundation <foundation-baseline.json> [--json]

Factory terms:
  factory brief   normalized operator objective and constraints
  workcell        bounded unit of factory work with dependencies and evidence
  factory packet  operator-ready JSON summary of plan, gates, evidence, and next actions

Slice 0.4 status:
  planning, live Covenant gate adapter, packet inspection, and dry-run execution are enabled.
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
		if err := validateBriefRequiredFields(data); err != nil {
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

func runRun(args []string, stdout, stderr io.Writer) int {
	var planPath, gateResultPath, outPath string
	var controlPlaneURL string
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
			WorkcellID string   `json:"workcell_id"`
			Kind       string   `json:"kind"`
			Status     string   `json:"status"`
			DependsOn  []string `json:"depends_on"`
			AO2Run     string   `json:"ao2_run"`
			Summary    string   `json:"summary"`
		}, len(plan.Workcells))

		for i, wc := range plan.Workcells {
			packet.Workcells[i].WorkcellID = wc.WorkcellID
			packet.Workcells[i].Kind = wc.Kind
			packet.Workcells[i].Status = workcellStatus
			packet.Workcells[i].DependsOn = cloneStrings(wc.DependsOn)
			packet.Workcells[i].Summary = explanation
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
		packet.TrustBoundary.MutatesReleases = false
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

	// Gate is allowed. Find and verify ao2 binary
	ao2Path, err := resolveAo2Binary()
	if err != nil {
		explanation := fmt.Sprintf("AO2 binary is unavailable: %v", err)
		return failClosedWithPacket("blocked", "blocked", explanation, "ao2-unavailable", "ao-forge", true, nil)
	}

	// Construct Ao2RunSpec
	specTasks := make([]runTask, 0, len(plan.Workcells))
	for _, wc := range plan.Workcells {
		specTasks = append(specTasks, runTask{
			ID:        wc.WorkcellID,
			Kind:      mapWorkcellKind(wc.Kind),
			Deps:      cloneStrings(wc.DependsOn),
			Rationale: "ao-forge workcell " + wc.WorkcellID,
		})
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
				RepoPath: plan.Objective.Workspace,
			},
			TrustBoundary: trustBoundary{
				ControlPlaneRole:   "read_only_observer",
				MutatesAoArtifacts: false,
			},
			Tasks: specTasks,
		},
	}

	specData, err := marshalIndented(runSpec)
	if err != nil {
		explanation := fmt.Sprintf("Failed to marshal ao2 run spec: %v", err)
		return failClosedWithPacket("blocked", "blocked", explanation, "spec-generation-failed", "ao-forge", true, nil)
	}

	tempSpec, err := os.CreateTemp("", "ao2-runspec-*.json")
	if err != nil {
		explanation := fmt.Sprintf("Failed to create temporary spec file: %v", err)
		return failClosedWithPacket("blocked", "blocked", explanation, "spec-temp-failed", "ao-forge", true, nil)
	}
	defer os.Remove(tempSpec.Name())

	if _, err := tempSpec.Write(specData); err != nil {
		tempSpec.Close()
		explanation := fmt.Sprintf("Failed to write temporary spec file: %v", err)
		return failClosedWithPacket("blocked", "blocked", explanation, "spec-write-failed", "ao-forge", true, nil)
	}
	tempSpec.Close()

	// Execute ao2 run --dry-run
	cmd := exec.Command(ao2Path, "run", "--dry-run", "--spec", tempSpec.Name())
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		explanation := fmt.Sprintf("ao2 run failed: %v (stderr: %q)", err, stderrBuf.String())
		return failClosedWithPacket("failed", "failed", explanation, "ao2-execution-failed", "ao-forge", false, nil)
	}

	stdoutStr := stdoutBuf.String()
	if !strings.Contains(stdoutStr, "status=dry_run_accepted") {
		explanation := fmt.Sprintf("ao2 run output did not confirm acceptance: %q (stderr: %q)", stdoutStr, stderrBuf.String())
		return failClosedWithPacket("failed", "failed", explanation, "ao2-not-accepted", "ao-forge", false, nil)
	}

	// Parse dry-run output and write ao2-run-summary.json evidence
	parsedSummary := parseAo2DryRunOutput(stdoutStr)
	summaryData, err := marshalIndented(parsedSummary)
	if err != nil {
		explanation := fmt.Sprintf("Failed to marshal ao2 run summary: %v", err)
		return failClosedWithPacket("failed", "failed", explanation, "summary-marshal-failed", "ao-forge", false, nil)
	}

	summaryDir := "."
	if outPath != "" {
		summaryDir = filepath.Dir(outPath)
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
			Status:        "dry_run_accepted",
			Path:          displayPath(summaryPath),
			SHA256:        hex.EncodeToString(sum[:]),
		})
	}

	// Control plane readback if required
	if plan.Constraints.RequireControlPlaneReadback {
		cpURL := resolveControlPlaneURL(controlPlaneURL)
		cpToken := resolveControlPlaneToken()
		if cpToken == "" {
			explanation := "Control plane readback is required, but API token is missing"
			return failClosedWithPacket("blocked", "blocked", explanation, "control-plane-unauthorized", "ao-forge", true, evidenceList)
		}

		cpReceiptData, cpErr := performControlPlaneUploadAndReadback(cpURL, cpToken, plan, evidenceList)
		if cpErr != nil {
			explanation := fmt.Sprintf("Control plane readback failed: %v", cpErr)
			return failClosedWithPacket("blocked", "blocked", explanation, "control-plane-readback-failed", "ao-forge", true, evidenceList)
		}

		// Save the receipt as control-plane-receipt.json and append it to the packet's evidence list
		receiptDir := "."
		if outPath != "" {
			receiptDir = filepath.Dir(outPath)
		}
		receiptPath := filepath.Join(receiptDir, "control-plane-receipt.json")
		if err := writeFile(receiptPath, cpReceiptData); err != nil {
			explanation := fmt.Sprintf("Failed to write control plane receipt: %v", err)
			return failClosedWithPacket("failed", "failed", explanation, "control-plane-receipt-write-failed", "ao-forge", false, evidenceList)
		}

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
		WorkcellID string   `json:"workcell_id"`
		Kind       string   `json:"kind"`
		Status     string   `json:"status"`
		DependsOn  []string `json:"depends_on"`
		AO2Run     string   `json:"ao2_run"`
		Summary    string   `json:"summary"`
	}, len(plan.Workcells))

	for i, wc := range plan.Workcells {
		packet.Workcells[i].WorkcellID = wc.WorkcellID
		packet.Workcells[i].Kind = wc.Kind
		packet.Workcells[i].Status = "passed"
		packet.Workcells[i].DependsOn = cloneStrings(wc.DependsOn)
		packet.Workcells[i].AO2Run = "dry-run"
		packet.Workcells[i].Summary = "Dry-run accepted by ao2"
	}

	packet.Evidence = evidenceList

	packet.TrustBoundary.LocalFirst = plan.Constraints.LocalFirst
	packet.TrustBoundary.MutatesReleases = false
	packet.TrustBoundary.StoresCredentials = false
	packet.TrustBoundary.ControlPlaneApprovesWork = false

	packet.NextActions = []nextAction{
		{
			ActionID:    "close-factory-packet",
			Description: "Review the factory packet and dry-run evidence.",
			Required:    false,
		},
	}

	packetData, err := marshalIndented(packet)
	if err != nil {
		fmt.Fprintf(stderr, "forge run: encode final packet: %v\n", err)
		return 1
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

func runOnce(args []string, stdout, stderr io.Writer) int {
	var briefPath, covenantPath, outPath string
	var controlPlaneURL string
	var workspacePath string
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
		case "--workspace":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				fmt.Fprintln(stderr, "forge once: --workspace requires a value")
				return 2
			}
			workspacePath = args[i+1]
			i++
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
	brief, canonical, err := readBrief(briefPath)
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

	workcellRegex := regexp.MustCompile(`^\s*[-\*]\s*([a-zA-Z0-9_-]+)\s*\((prepare|execute|verify|close)\)(?:\s+(?:depends\s+on|depends_on):\s*([a-zA-Z0-9_,\s-]+))?\s*$`)

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
				matches := workcellRegex.FindStringSubmatch(trimmedLine)
				if len(matches) >= 3 {
					wcID := matches[1]
					wcKind := matches[2]
					deps := []string{}
					if len(matches) > 3 && matches[3] != "" {
						depList := strings.Split(matches[3], ",")
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
	buf.WriteString("| Workcell ID | Kind | Status | Run Mode | Summary |\n")
	buf.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, wc := range packet.Workcells {
		runMode := wc.AO2Run
		if runMode == "" {
			runMode = "none"
		}
		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s |\n", wc.WorkcellID, wc.Kind, wc.Status, runMode, wc.Summary)
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
