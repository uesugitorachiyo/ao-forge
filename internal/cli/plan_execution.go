package cli

import (
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
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

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
	ExitCriteria  exitCriteria  `json:"exit_criteria"`
}

type exitCriteria struct {
	Tests  []string `json:"tests"`
	Gates  []string `json:"gates"`
	Manual []string `json:"manual"`
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
			ActionID: "close-factory-packet",
			Description: func() string {
				if liveMode {
					return "Review the factory packet and live evidence."
				}
				return "Review the factory packet and dry-run evidence."
			}(),
			Required: false,
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
	SchemaVersion  string             `json:"schema_version"`
	RunID          string             `json:"run_id"`
	Status         string             `json:"status"`
	OperatorID     string             `json:"operator_id"`
	GeneratedAtUTC string             `json:"generated_at_utc"`
	Summary        cpPacketSummary    `json:"summary"`
	Evidence       []cpPacketEvidence `json:"evidence"`
	TrustBoundary  cpTrustBoundary    `json:"trust_boundary"`
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
	SchemaVersion     string           `json:"schema_version"`
	OperatorPacket    cpOperatorPacket `json:"operator_packet"`
	OperatorPacketB64 string           `json:"operator_packet_b64"`
	Signature         cpSignature      `json:"signature"`
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
		Evidence: make([]cpPacketEvidence, 0, len(evidenceList)),
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
