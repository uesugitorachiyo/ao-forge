package cli

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type goalReadinessSummary struct {
	ReadinessSchemaVersion string                         `json:"readiness_schema_version"`
	GoalRun                string                         `json:"goal_run"`
	GoalID                 string                         `json:"goal_id,omitempty"`
	CurrentPhase           string                         `json:"current_phase,omitempty"`
	RequestedPhase         string                         `json:"requested_phase,omitempty"`
	Status                 string                         `json:"status"`
	Checks                 []goalReadinessCheck           `json:"checks"`
	EvidenceLint           goalEvidenceLintSummary        `json:"evidence_lint"`
	EvidenceVerify         goalEvidenceVerifySummary      `json:"evidence_verify"`
	Provenance             goalReadinessProvenance        `json:"provenance"`
	RetentionAudits        []goalEvidenceRetentionSummary `json:"retention_audits"`
	Errors                 []string                       `json:"errors"`
}

type goalReadinessProvenance struct {
	GoalRun  goalReadinessProvenanceArtifact   `json:"goal_run"`
	Evidence []goalReadinessProvenanceArtifact `json:"evidence"`
}

type goalReadinessProvenanceArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

type goalReadinessCheck struct {
	CheckID string `json:"check_id"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

func runGoalReadiness(args []string, stdout, stderr io.Writer) int {
	flags, err := parseGoalReadinessFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "forge goal readiness: %v\n", err)
		return 2
	}

	now := time.Now().UTC()
	if flags.now != "" {
		parsedNow, err := time.Parse(time.RFC3339, flags.now)
		if err != nil {
			fmt.Fprintf(stderr, "forge goal readiness: --now must be RFC3339: %v\n", err)
			return 2
		}
		now = parsedNow.UTC()
	}

	summary := emptyGoalReadinessSummary(flags)
	if sha, err := sha256File(flags.goalRunPath); err == nil {
		summary.Provenance.GoalRun.SHA256 = sha
	}
	goal, err := validateAndReadGoalRun(flags.goalRunPath)
	if err != nil {
		markGoalReadinessFailed(&summary, "goal_validate", fmt.Sprintf("goal run validation failed: %v", err))
		writeGoalReadinessSummary(stdout, summary, flags.json)
		fmt.Fprintf(stderr, "forge goal readiness: goal run validation failed: %v\n", err)
		return 1
	}

	summary.GoalID = goal.GoalID
	summary.CurrentPhase = goal.CurrentPhase
	addGoalReadinessCheck(&summary, "goal_validate", "passed", "GoalRun schema validation passed")

	inspect := buildGoalInspectSummary(flags.goalRunPath, goal)
	if !inspect.NextActionGuard.Enabled {
		markGoalReadinessFailed(&summary, "goal_inspect", "next action guard is disabled")
	} else {
		addGoalReadinessCheck(&summary, "goal_inspect", "passed", "next action guard is enabled")
	}

	transition := buildGoalTransitionSummary(flags.goalRunPath, goal, flags.toPhase)
	if flags.toPhase != "" && transition.Status != "allowed" {
		markGoalReadinessFailed(&summary, "goal_transition", transition.Reason)
	} else {
		addGoalReadinessCheck(&summary, "goal_transition", "passed", transition.Reason)
	}

	summary.EvidenceLint = buildGoalEvidenceLintSummaryForGoal(flags.goalRunPath, goal)
	if summary.EvidenceLint.Status != "passed" {
		markGoalReadinessFailed(&summary, "evidence_lint", strings.Join(summary.EvidenceLint.Errors, "; "))
	} else {
		addGoalReadinessCheck(&summary, "evidence_lint", "passed", fmt.Sprintf("linted %d evidence path(s)", summary.EvidenceLint.EvidenceLinted))
	}

	summary.EvidenceVerify = buildGoalEvidenceVerifySummary(flags.goalRunPath, goal)
	summary.Provenance.Evidence = goalReadinessEvidenceProvenance(summary.EvidenceVerify)
	if summary.EvidenceVerify.Status != "passed" {
		markGoalReadinessFailed(&summary, "evidence_verify", strings.Join(summary.EvidenceVerify.Errors, "; "))
	} else {
		addGoalReadinessCheck(&summary, "evidence_verify", "passed", fmt.Sprintf("verified %d evidence artifact(s)", summary.EvidenceVerify.EvidenceVerified))
	}

	retainedEvidence := retainedGoalRunEvidence(goal)
	for _, evidence := range retainedEvidence {
		retention := buildGoalEvidenceRetentionSummary(evidence.Path, now)
		summary.RetentionAudits = append(summary.RetentionAudits, retention)
		if retention.Status != "passed" {
			markGoalReadinessFailed(&summary, "retained_evidence", strings.Join(retention.Errors, "; "))
		}
	}
	if len(retainedEvidence) == 0 {
		addGoalReadinessCheck(&summary, "retained_evidence", "passed", "no retained evidence artifacts to audit")
	} else if allGoalRetentionAuditsPassed(summary.RetentionAudits) {
		addGoalReadinessCheck(&summary, "retained_evidence", "passed", fmt.Sprintf("audited %d retained evidence artifact(s)", len(summary.RetentionAudits)))
	}

	writeGoalReadinessSummary(stdout, summary, flags.json)
	if summary.Status != "passed" {
		fmt.Fprintf(stderr, "forge goal readiness: readiness audit failed for %s\n", summary.GoalRun)
		return 1
	}
	return 0
}

func emptyGoalReadinessSummary(flags goalReadinessFlags) goalReadinessSummary {
	return goalReadinessSummary{
		ReadinessSchemaVersion: "ao.forge.goal-run-readiness-audit.v0.1",
		GoalRun:                displayPath(flags.goalRunPath),
		RequestedPhase:         flags.toPhase,
		Status:                 "passed",
		Checks:                 []goalReadinessCheck{},
		EvidenceLint:           emptyGoalEvidenceLintSummary(flags.goalRunPath),
		EvidenceVerify:         emptyGoalEvidenceVerifySummary(flags.goalRunPath),
		Provenance: goalReadinessProvenance{
			GoalRun:  goalReadinessProvenanceArtifact{Path: displayPath(flags.goalRunPath)},
			Evidence: []goalReadinessProvenanceArtifact{},
		},
		RetentionAudits: []goalEvidenceRetentionSummary{},
		Errors:          []string{},
	}
}

func addGoalReadinessCheck(summary *goalReadinessSummary, checkID, status, checkSummary string) {
	summary.Checks = append(summary.Checks, goalReadinessCheck{
		CheckID: checkID,
		Status:  status,
		Summary: checkSummary,
	})
}

func markGoalReadinessFailed(summary *goalReadinessSummary, checkID, checkSummary string) {
	summary.Status = "failed"
	if strings.TrimSpace(checkSummary) == "" {
		checkSummary = "check failed"
	}
	addGoalReadinessCheck(summary, checkID, "failed", checkSummary)
	summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s", checkID, checkSummary))
}

func goalReadinessEvidenceProvenance(summary goalEvidenceVerifySummary) []goalReadinessProvenanceArtifact {
	provenance := make([]goalReadinessProvenanceArtifact, 0, len(summary.Evidence))
	for _, evidence := range summary.Evidence {
		artifact := goalReadinessProvenanceArtifact{Path: evidence.Path}
		if evidence.ActualSHA256 != "" {
			artifact.SHA256 = evidence.ActualSHA256
		}
		provenance = append(provenance, artifact)
	}
	return provenance
}
