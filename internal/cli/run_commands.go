package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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

	prevStates := loadResumeStates(runDir, packetPath)

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
