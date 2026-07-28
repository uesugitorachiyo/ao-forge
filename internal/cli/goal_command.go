package cli

import (
	"fmt"
	"io"
	"time"
)

func runGoal(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "forge goal: missing subcommand validate, inspect, transitions, update, or evidence")
		return 2
	}
	switch args[0] {
	case "validate":
		return runGoalValidate(args[1:], stdout, stderr)
	case "inspect":
		return runGoalInspect(args[1:], stdout, stderr)
	case "transitions":
		return runGoalTransitions(args[1:], stdout, stderr)
	case "readiness":
		return runGoalReadiness(args[1:], stdout, stderr)
	case "context":
		return runGoalContext(args[1:], stdout, stderr)
	case "verification":
		return runGoalVerification(args[1:], stdout, stderr)
	case "update":
		return runGoalUpdate(args[1:], stdout, stderr)
	case "evidence":
		return runGoalEvidence(args[1:], stdout, stderr)
	case "--help", "-h":
		fmt.Fprintln(stderr, "forge goal: use `forge goal validate --goal-run <goal-run.json> [--json]`, `forge goal inspect --goal-run <goal-run.json> [--json]`, `forge goal transitions --goal-run <goal-run.json> [--to <phase>] [--json]`, `forge goal readiness --goal-run <goal-run.json> [--to <phase>] [--now <RFC3339>] [--json]`, `forge goal context validate --goal-run <goal-run.json> --handoff <context-handoff.json> [--now <RFC3339>] [--json]`, `forge goal verification validate --verification <goal-run-verification.json> [--json]`, `forge goal update --goal-run <goal-run.json> --out <goal-run.json> [--phase <phase>] [--next-task <text>] [--last-verified-at <RFC3339>] [--last-iteration-status <status>] [--last-iteration-summary <text>] [--evidence <path> ...] [--json]`, `forge goal evidence verify --goal-run <goal-run.json> [--json]`, `forge goal evidence lint --goal-run <goal-run.json> [--json]`, `forge goal evidence retention --artifact <retained-evidence.json> [--now <RFC3339>] [--json]`, or `forge goal evidence cleanup --dry-run [--root <dir>] [--now <RFC3339>] [--json]`")
		return 0
	default:
		fmt.Fprintf(stderr, "forge goal: unknown subcommand %q\n", args[0])
		return 2
	}
}

func runGoalContext(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "forge goal context: missing subcommand validate")
		return 2
	}
	switch args[0] {
	case "validate":
		return runGoalContextValidate(args[1:], stdout, stderr)
	case "--help", "-h":
		fmt.Fprintln(stderr, "forge goal context: use `forge goal context validate --goal-run <goal-run.json> --handoff <context-handoff.json> [--now <RFC3339>] [--json]`")
		return 0
	default:
		fmt.Fprintf(stderr, "forge goal context: unknown subcommand %q\n", args[0])
		return 2
	}
}

func runGoalVerification(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "forge goal verification: missing subcommand validate")
		return 2
	}
	switch args[0] {
	case "validate":
		return runGoalVerificationValidate(args[1:], stdout, stderr)
	case "--help", "-h":
		fmt.Fprintln(stderr, "forge goal verification: use `forge goal verification validate --verification <goal-run-verification.json> [--json]`")
		return 0
	default:
		fmt.Fprintf(stderr, "forge goal verification: unknown subcommand %q\n", args[0])
		return 2
	}
}

func runGoalEvidence(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "forge goal evidence: missing subcommand verify, lint, or retention")
		return 2
	}
	switch args[0] {
	case "verify":
		return runGoalEvidenceVerify(args[1:], stdout, stderr)
	case "lint":
		return runGoalEvidenceLint(args[1:], stdout, stderr)
	case "retention":
		return runGoalEvidenceRetention(args[1:], stdout, stderr)
	case "cleanup":
		return runGoalEvidenceCleanup(args[1:], stdout, stderr)
	case "--help", "-h":
		fmt.Fprintln(stderr, "forge goal evidence: use `forge goal evidence verify --goal-run <goal-run.json> [--json]`, `forge goal evidence lint --goal-run <goal-run.json> [--json]`, `forge goal evidence lint --update-audit <goal-run-update-audit.json> [--json]`, `forge goal evidence retention --artifact <retained-evidence.json> [--now <RFC3339>] [--json]`, or `forge goal evidence cleanup --dry-run [--root <dir>] [--now <RFC3339>] [--json]`")
		return 0
	default:
		fmt.Fprintf(stderr, "forge goal evidence: unknown subcommand %q\n", args[0])
		return 2
	}
}

func runGoalValidate(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal validate: %v\n", err)
		return 2
	}

	summary := goalValidationSummary{
		GoalRun: displayPath(flags.goalRunPath),
		Schema:  displayPath(resolveDefaultContractPath(goalRunSchemaPath)),
		Status:  "passed",
		Errors:  []string{},
	}
	goal, err := validateAndReadGoalRun(flags.goalRunPath)
	if err != nil {
		summary.Status = "failed"
		summary.Errors = []string{err.Error()}
		writeGoalValidationSummary(stdout, summary, flags.json)
		fmt.Fprintf(stderr, "forge goal validate: goal run validation failed: %v\n", err)
		return 1
	}
	summary.SchemaVersion = goal.SchemaVersion
	summary.GoalID = goal.GoalID
	summary.CurrentPhase = goal.CurrentPhase
	summary.NextActionGuard = goalRunGuardStatus(goal)

	writeGoalValidationSummary(stdout, summary, flags.json)
	return 0
}

func runGoalInspect(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal inspect: %v\n", err)
		return 2
	}

	goal, err := validateAndReadGoalRun(flags.goalRunPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal inspect: goal run validation failed: %v\n", err)
		return 1
	}

	summary := buildGoalInspectSummary(flags.goalRunPath, goal)
	writeGoalInspectSummary(stdout, summary, flags.json)
	return 0
}

func runGoalTransitions(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalTransitionFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal transitions: %v\n", err)
		return 2
	}

	goal, err := validateAndReadGoalRun(flags.goalRunPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal transitions: goal run validation failed: %v\n", err)
		return 1
	}

	summary := buildGoalTransitionSummary(flags.goalRunPath, goal, flags.toPhase)
	writeGoalTransitionSummary(stdout, summary, flags.json)
	if flags.toPhase != "" && summary.Status == "denied" {
		return 1
	}
	return 0
}

func runGoalContextValidate(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalContextFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal context validate: %v\n", err)
		return 2
	}

	now := time.Now().UTC()
	if flags.now != "" {
		parsedNow, err := time.Parse(time.RFC3339, flags.now)
		if err != nil {
			fmt.Fprintf(stderr, "forge goal context validate: --now must be RFC3339: %v\n", err)
			return 2
		}
		now = parsedNow.UTC()
	}

	summary := goalContextSummary{
		ContextSchemaVersion: goalContextHandoffVersion,
		GoalRun:              displayPath(flags.goalRunPath),
		Handoff:              displayPath(flags.handoffPath),
		Now:                  now.Format(time.RFC3339),
		Status:               "passed",
		ResumeGuard:          "disabled",
		Errors:               []string{},
	}
	goal, err := validateAndReadGoalRun(flags.goalRunPath)
	if err != nil {
		summary.Status = "failed"
		summary.Errors = append(summary.Errors, fmt.Sprintf("goal run validation failed: %v", err))
		writeGoalContextSummary(stdout, summary, flags.json)
		fmt.Fprintf(stderr, "forge goal context validate: context handoff validation failed for %s\n", summary.Handoff)
		return 1
	}
	handoff, err := validateAndReadGoalContextHandoff(flags.handoffPath)
	if err != nil {
		summary.Status = "failed"
		summary.Errors = append(summary.Errors, fmt.Sprintf("context handoff validation failed: %v", err))
		writeGoalContextSummary(stdout, summary, flags.json)
		fmt.Fprintf(stderr, "forge goal context validate: context handoff validation failed for %s\n", summary.Handoff)
		return 1
	}
	summary.GoalID = handoff.GoalID
	summary.CapturedAt = handoff.CapturedAt
	if handoff.ResumeGuard.MustReadLatestGoalRun &&
		handoff.ResumeGuard.MustRunGoalReadiness &&
		handoff.ResumeGuard.OnStaleContext == "backoff_or_stop" {
		summary.ResumeGuard = "enabled"
	}

	capturedAt, parseErr := time.Parse(time.RFC3339, handoff.CapturedAt)
	if parseErr != nil {
		summary.Status = "failed"
		summary.Errors = append(summary.Errors, fmt.Sprintf("captured_at must be RFC3339: %v", parseErr))
	} else {
		age := now.Sub(capturedAt.UTC())
		summary.AgeHours = int(age.Hours())
		if age < 0 {
			summary.Status = "failed"
			summary.Errors = append(summary.Errors, "captured_at is in the future")
		}
		if age > 24*time.Hour {
			summary.Status = "failed"
			summary.Errors = append(summary.Errors, "context is older than 24h; rerun GoalRun readiness and write a fresh handoff")
		}
	}
	if handoff.GoalID != goal.GoalID {
		summary.Status = "failed"
		summary.Errors = append(summary.Errors, fmt.Sprintf("handoff goal_id %q does not match GoalRun %q", handoff.GoalID, goal.GoalID))
	}
	if handoff.GoalRun != displayPath(flags.goalRunPath) {
		summary.Status = "failed"
		summary.Errors = append(summary.Errors, fmt.Sprintf("handoff goal_run %q does not match %q", handoff.GoalRun, displayPath(flags.goalRunPath)))
	}
	if summary.ResumeGuard != "enabled" {
		summary.Status = "failed"
		summary.Errors = append(summary.Errors, "resume guard must read latest GoalRun, run goal readiness, and backoff_or_stop on stale context")
	}
	if handoff.ContextBudget.EstimatedTokensUsed > handoff.ContextBudget.MaxTokens {
		summary.Status = "failed"
		summary.Errors = append(summary.Errors, "context budget estimated_tokens_used exceeds max_tokens")
	}

	writeGoalContextSummary(stdout, summary, flags.json)
	if summary.Status != "passed" {
		fmt.Fprintf(stderr, "forge goal context validate: context handoff validation failed for %s\n", summary.Handoff)
		return 1
	}
	return 0
}

func runGoalVerificationValidate(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalVerificationFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal verification validate: %v\n", err)
		return 2
	}

	summary := goalVerificationSummary{
		VerificationSchemaVersion: goalVerificationVersion,
		Verification:              displayPath(flags.verificationPath),
		Status:                    "passed",
		Errors:                    []string{},
	}
	verification, err := validateAndReadGoalVerification(flags.verificationPath)
	if err != nil {
		summary.Status = "failed"
		summary.Errors = append(summary.Errors, err.Error())
		writeGoalVerificationSummary(stdout, summary, flags.json)
		fmt.Fprintf(stderr, "forge goal verification validate: verification validation failed for %s\n", summary.Verification)
		return 1
	}
	summary.GoalID = verification.GoalID
	summary.GoalRun = verification.GoalRun
	summary.VerifiedAt = verification.VerifiedAt
	summary.PhasesChecked = len(verification.Phases)

	for _, validationErr := range validateGoalVerificationSemantics(verification) {
		summary.Status = "failed"
		summary.Errors = append(summary.Errors, validationErr)
	}
	writeGoalVerificationSummary(stdout, summary, flags.json)
	if summary.Status != "passed" {
		fmt.Fprintf(stderr, "forge goal verification validate: verification validation failed for %s\n", summary.Verification)
		return 1
	}
	return 0
}

func runGoalUpdate(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalUpdateFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal update: %v\n", err)
		return 2
	}

	goal, err := validateAndReadGoalRun(flags.goalRunPath)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal update: goal run validation failed: %v\n", err)
		return 1
	}
	if err := ensureGoalRunEvidenceReadyForUpdate(flags.goalRunPath, goal); err != nil {
		fmt.Fprintf(stderr, "forge goal update: %v\n", err)
		return 1
	}

	previousPhase := goal.CurrentPhase
	updatedFields, err := applyGoalRunUpdate(&goal, flags)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal update: %v\n", err)
		return 1
	}
	if len(updatedFields) == 0 {
		fmt.Fprintln(stderr, "forge goal update: update produced no changes")
		return 1
	}
	if err := validateGoalRunValue(goal); err != nil {
		fmt.Fprintf(stderr, "forge goal update: updated goal run validation failed: %v\n", err)
		return 1
	}

	data, err := marshalIndented(goal)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal update: marshal updated goal run: %v\n", err)
		return 1
	}
	if err := writeFile(flags.outPath, data); err != nil {
		fmt.Fprintf(stderr, "forge goal update: write updated goal run: %v\n", err)
		return 1
	}

	audit := buildGoalUpdateAudit(flags, goal, previousPhase, updatedFields)
	writeGoalUpdateAudit(stdout, audit, flags.json)
	return 0
}

func runGoalEvidenceVerify(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal evidence verify: %v\n", err)
		return 2
	}

	goal, err := validateAndReadGoalRun(flags.goalRunPath)
	summary := emptyGoalEvidenceVerifySummary(flags.goalRunPath)
	if err != nil {
		summary.Status = "failed"
		summary.Errors = []string{err.Error()}
		writeGoalEvidenceVerifySummary(stdout, summary, flags.json)
		fmt.Fprintf(stderr, "forge goal evidence verify: goal run validation failed: %v\n", err)
		return 1
	}
	summary = buildGoalEvidenceVerifySummary(flags.goalRunPath, goal)

	writeGoalEvidenceVerifySummary(stdout, summary, flags.json)
	if summary.Status != "passed" {
		fmt.Fprintf(stderr, "forge goal evidence verify: evidence verification failed for %s\n", summary.GoalRun)
		return 1
	}
	return 0
}

func runGoalEvidenceLint(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalEvidenceLintFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal evidence lint: %v\n", err)
		return 2
	}

	summary := goalEvidenceLintSummary{
		LintSchemaVersion: "ao.forge.goal-run-evidence-lint.v0.1",
		Status:            "passed",
		Evidence:          []goalEvidenceLintResult{},
		Errors:            []string{},
	}
	var evidence []goalRunEvidence
	if flags.goalRunPath != "" {
		goal, err := validateAndReadGoalRun(flags.goalRunPath)
		if err != nil {
			summary.Document = displayPath(flags.goalRunPath)
			summary.DocumentType = "goal_run"
			summary.Status = "failed"
			summary.Errors = []string{err.Error()}
			writeGoalEvidenceLintSummary(stdout, summary, flags.json)
			fmt.Fprintf(stderr, "forge goal evidence lint: goal run validation failed: %v\n", err)
			return 1
		}
		summary = buildGoalEvidenceLintSummaryForGoal(flags.goalRunPath, goal)
	} else {
		summary.Document = displayPath(flags.updateAuditPath)
		summary.DocumentType = "goal_run_update_audit"
		audit, err := validateAndReadGoalRunUpdateAudit(flags.updateAuditPath)
		if err != nil {
			summary.Status = "failed"
			summary.Errors = []string{err.Error()}
			writeGoalEvidenceLintSummary(stdout, summary, flags.json)
			fmt.Fprintf(stderr, "forge goal evidence lint: update audit validation failed: %v\n", err)
			return 1
		}
		summary.GoalID = audit.GoalID
		evidence = audit.Evidence

		for _, item := range evidence {
			result := lintGoalRunEvidencePath(item)
			summary.Evidence = append(summary.Evidence, result)
			if result.Status == "passed" {
				summary.EvidenceLinted++
			} else {
				summary.Status = "failed"
				summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s", result.Path, result.Error))
			}
		}
	}

	writeGoalEvidenceLintSummary(stdout, summary, flags.json)
	if summary.Status != "passed" {
		fmt.Fprintf(stderr, "forge goal evidence lint: evidence path lint failed for %s\n", summary.Document)
		return 1
	}
	return 0
}

func runGoalEvidenceRetention(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalEvidenceRetentionFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal evidence retention: %v\n", err)
		return 2
	}

	now := time.Now().UTC()
	if flags.now != "" {
		parsedNow, err := time.Parse(time.RFC3339, flags.now)
		if err != nil {
			fmt.Fprintf(stderr, "forge goal evidence retention: --now must be RFC3339: %v\n", err)
			return 2
		}
		now = parsedNow.UTC()
	}

	summary := buildGoalEvidenceRetentionSummary(flags.artifactPath, now)
	writeGoalEvidenceRetentionSummary(stdout, summary, flags.json)
	if summary.Status != "passed" {
		if summary.GoalID == "" && len(summary.Errors) > 0 {
			fmt.Fprintf(stderr, "forge goal evidence retention: retained evidence validation failed: %s\n", summary.Errors[0])
			return 1
		}
		fmt.Fprintf(stderr, "forge goal evidence retention: retained evidence audit failed for %s\n", summary.Artifact)
		return 1
	}
	return 0
}

func runGoalEvidenceCleanup(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalEvidenceCleanupFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal evidence cleanup: %v\n", err)
		return 2
	}

	now := time.Now().UTC()
	if flags.now != "" {
		parsedNow, err := time.Parse(time.RFC3339, flags.now)
		if err != nil {
			fmt.Fprintf(stderr, "forge goal evidence cleanup: --now must be RFC3339: %v\n", err)
			return 2
		}
		now = parsedNow.UTC()
	}

	summary := buildGoalEvidenceCleanupSummary(flags.root, now)
	writeGoalEvidenceCleanupSummary(stdout, summary, flags.json)
	if summary.Status != "passed" {
		fmt.Fprintf(stderr, "forge goal evidence cleanup: dry-run failed for %s\n", summary.Root)
		return 1
	}
	return 0
}
