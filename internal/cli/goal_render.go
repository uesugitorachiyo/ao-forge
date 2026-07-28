package cli

import (
	"fmt"
	"io"
	"strings"
)

func writeGoalValidationSummary(stdout io.Writer, summary goalValidationSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"status\":\"failed\",\"errors\":[%q]}\n", err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_run_validation=%s\n", summary.Status)
	fmt.Fprintf(stdout, "goal_run=%s\n", summary.GoalRun)
	fmt.Fprintf(stdout, "schema=%s\n", summary.Schema)
	if summary.SchemaVersion != "" {
		fmt.Fprintf(stdout, "schema_version=%s\n", summary.SchemaVersion)
	}
	if summary.GoalID != "" {
		fmt.Fprintf(stdout, "goal_id=%s\n", summary.GoalID)
	}
	if summary.CurrentPhase != "" {
		fmt.Fprintf(stdout, "current_phase=%s\n", summary.CurrentPhase)
	}
	if summary.NextActionGuard != "" {
		fmt.Fprintf(stdout, "next_action_guard=%s\n", summary.NextActionGuard)
	}
	for _, validationErr := range summary.Errors {
		fmt.Fprintf(stdout, "error=%s\n", validationErr)
	}
}

func writeGoalTransitionSummary(stdout io.Writer, summary goalTransitionSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"transition_schema_version\":\"ao.forge.goal-run-transition.v0.1\",\"status\":\"failed\",\"reason\":%q}\n", err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_run_transition=%s\n", summary.Status)
	fmt.Fprintf(stdout, "goal_run=%s\n", summary.GoalRun)
	fmt.Fprintf(stdout, "goal_id=%s\n", summary.GoalID)
	fmt.Fprintf(stdout, "current_phase=%s\n", summary.CurrentPhase)
	fmt.Fprintf(stdout, "allowed_next_phases=%s\n", formatGoalRunPhaseList(summary.AllowedNextPhases))
	if summary.RequestedPhase != "" {
		fmt.Fprintf(stdout, "requested_phase=%s\n", summary.RequestedPhase)
	}
	fmt.Fprintf(stdout, "reason=%s\n", summary.Reason)
}

func writeGoalUpdateAudit(stdout io.Writer, audit goalUpdateAudit, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(audit)
		if err != nil {
			fmt.Fprintf(stdout, "{\"audit_schema_version\":\"ao.forge.goal-run-update-audit.v0.1\",\"status\":\"failed\"}\n")
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_run_update=%s\n", audit.Status)
	fmt.Fprintf(stdout, "goal_run=%s\n", audit.GoalRun)
	fmt.Fprintf(stdout, "out=%s\n", audit.Out)
	fmt.Fprintf(stdout, "goal_id=%s\n", audit.GoalID)
	fmt.Fprintf(stdout, "previous_phase=%s\n", audit.PreviousPhase)
	fmt.Fprintf(stdout, "current_phase=%s\n", audit.CurrentPhase)
	fmt.Fprintf(stdout, "phase_transition=%s\n", audit.PhaseTransition)
	fmt.Fprintf(stdout, "updated_fields=%s\n", strings.Join(audit.UpdatedFields, ","))
	fmt.Fprintf(stdout, "last_verified_at=%s\n", audit.LastVerifiedAt)
	if audit.LastIterationStatus != "" {
		fmt.Fprintf(stdout, "last_iteration_status=%s\n", audit.LastIterationStatus)
	}
	if len(audit.Evidence) > 0 {
		fmt.Fprintf(stdout, "evidence=%d\n", len(audit.Evidence))
		for _, evidence := range audit.Evidence {
			fmt.Fprintf(stdout, "evidence_path=%s sha256=%s\n", evidence.Path, evidence.SHA256)
		}
	}
}

func writeGoalContextSummary(stdout io.Writer, summary goalContextSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"context_schema_version\":\"%s\",\"status\":\"failed\",\"errors\":[%q]}\n", goalContextHandoffVersion, err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_context_handoff=%s\n", summary.Status)
	fmt.Fprintf(stdout, "goal_run=%s\n", summary.GoalRun)
	fmt.Fprintf(stdout, "handoff=%s\n", summary.Handoff)
	fmt.Fprintf(stdout, "schema=%s\n", displayPath(resolveDefaultContractPath(goalContextHandoffSchemaPath)))
	if summary.GoalID != "" {
		fmt.Fprintf(stdout, "goal_id=%s\n", summary.GoalID)
	}
	if summary.CapturedAt != "" {
		fmt.Fprintf(stdout, "captured_at=%s\n", summary.CapturedAt)
	}
	if summary.Now != "" {
		fmt.Fprintf(stdout, "now=%s\n", summary.Now)
	}
	fmt.Fprintf(stdout, "age_hours=%d\n", summary.AgeHours)
	fmt.Fprintf(stdout, "resume_guard=%s\n", summary.ResumeGuard)
	for _, validationErr := range summary.Errors {
		fmt.Fprintf(stdout, "error=%s\n", validationErr)
	}
}

func writeGoalVerificationSummary(stdout io.Writer, summary goalVerificationSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"verification_schema_version\":\"%s\",\"status\":\"failed\",\"errors\":[%q]}\n", goalVerificationVersion, err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_verification=%s\n", summary.Status)
	fmt.Fprintf(stdout, "verification=%s\n", summary.Verification)
	fmt.Fprintf(stdout, "schema=%s\n", displayPath(resolveDefaultContractPath(goalVerificationSchemaPath)))
	if summary.GoalID != "" {
		fmt.Fprintf(stdout, "goal_id=%s\n", summary.GoalID)
	}
	if summary.GoalRun != "" {
		fmt.Fprintf(stdout, "goal_run=%s\n", summary.GoalRun)
	}
	if summary.VerifiedAt != "" {
		fmt.Fprintf(stdout, "verified_at=%s\n", summary.VerifiedAt)
	}
	fmt.Fprintf(stdout, "phases_checked=%d\n", summary.PhasesChecked)
	for _, validationErr := range summary.Errors {
		fmt.Fprintf(stdout, "error=%s\n", validationErr)
	}
}

func writeGoalEvidenceVerifySummary(stdout io.Writer, summary goalEvidenceVerifySummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"verify_schema_version\":\"ao.forge.goal-run-evidence-verify.v0.1\",\"status\":\"failed\",\"errors\":[%q]}\n", err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_evidence_verify=%s\n", summary.Status)
	fmt.Fprintf(stdout, "goal_run=%s\n", summary.GoalRun)
	if summary.GoalID != "" {
		fmt.Fprintf(stdout, "goal_id=%s\n", summary.GoalID)
	}
	fmt.Fprintf(stdout, "evidence_verified=%d\n", summary.EvidenceVerified)
	for _, evidence := range summary.Evidence {
		fmt.Fprintf(stdout, "evidence_path=%s status=%s", evidence.Path, evidence.Status)
		if evidence.ActualSHA256 != "" {
			fmt.Fprintf(stdout, " sha256=%s", evidence.ActualSHA256)
		}
		if evidence.Error != "" {
			fmt.Fprintf(stdout, " error=%s", evidence.Error)
		}
		fmt.Fprintln(stdout)
	}
	for _, verifyErr := range summary.Errors {
		fmt.Fprintf(stdout, "error=%s\n", verifyErr)
	}
}

func writeGoalEvidenceLintSummary(stdout io.Writer, summary goalEvidenceLintSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"lint_schema_version\":\"ao.forge.goal-run-evidence-lint.v0.1\",\"status\":\"failed\",\"errors\":[%q]}\n", err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_evidence_lint=%s\n", summary.Status)
	fmt.Fprintf(stdout, "document=%s\n", summary.Document)
	fmt.Fprintf(stdout, "document_type=%s\n", summary.DocumentType)
	if summary.GoalID != "" {
		fmt.Fprintf(stdout, "goal_id=%s\n", summary.GoalID)
	}
	fmt.Fprintf(stdout, "evidence_linted=%d\n", summary.EvidenceLinted)
	for _, evidence := range summary.Evidence {
		fmt.Fprintf(stdout, "evidence_path=%s status=%s", evidence.Path, evidence.Status)
		if evidence.Error != "" {
			fmt.Fprintf(stdout, " error=%s", evidence.Error)
		}
		fmt.Fprintln(stdout)
	}
	for _, lintErr := range summary.Errors {
		fmt.Fprintf(stdout, "error=%s\n", lintErr)
	}
}

func writeGoalEvidenceRetentionSummary(stdout io.Writer, summary goalEvidenceRetentionSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"retention_audit_schema_version\":\"ao.forge.goal-run-retained-evidence-audit.v0.1\",\"status\":\"failed\",\"retention_status\":\"unknown\",\"cleanup_review_status\":\"unknown\",\"errors\":[%q]}\n", err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_evidence_retention=%s\n", summary.Status)
	fmt.Fprintf(stdout, "artifact=%s\n", summary.Artifact)
	if summary.GoalID != "" {
		fmt.Fprintf(stdout, "goal_id=%s\n", summary.GoalID)
	}
	if summary.Iteration != "" {
		fmt.Fprintf(stdout, "iteration=%s\n", summary.Iteration)
	}
	if summary.Phase != "" {
		fmt.Fprintf(stdout, "phase=%s\n", summary.Phase)
	}
	if summary.RetentionClass != "" {
		fmt.Fprintf(stdout, "retention_class=%s\n", summary.RetentionClass)
	}
	if summary.RetainedAt != "" {
		fmt.Fprintf(stdout, "retained_at=%s\n", summary.RetainedAt)
	}
	fmt.Fprintf(stdout, "now=%s\n", summary.Now)
	fmt.Fprintf(stdout, "retention_status=%s\n", summary.RetentionStatus)
	fmt.Fprintf(stdout, "cleanup_review_status=%s\n", summary.CleanupReviewStatus)
	fmt.Fprintf(stdout, "age_days=%d\n", summary.AgeDays)
	fmt.Fprintf(stdout, "minimum_retention_days_after_terminal_phase=%d\n", summary.MinimumRetentionDaysAfterTerminalPhase)
	if summary.MandatoryRetentionUntil != "" {
		fmt.Fprintf(stdout, "mandatory_retention_until=%s\n", summary.MandatoryRetentionUntil)
	}
	for _, auditErr := range summary.Errors {
		fmt.Fprintf(stdout, "error=%s\n", auditErr)
	}
}

func writeGoalEvidenceCleanupSummary(stdout io.Writer, summary goalEvidenceCleanupSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"cleanup_schema_version\":\"%s\",\"mode\":\"dry-run\",\"status\":\"failed\",\"errors\":[%q]}\n", goalEvidenceCleanupVersion, err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_evidence_cleanup=%s\n", summary.Status)
	fmt.Fprintf(stdout, "mode=%s\n", summary.Mode)
	fmt.Fprintf(stdout, "root=%s\n", summary.Root)
	fmt.Fprintf(stdout, "now=%s\n", summary.Now)
	fmt.Fprintf(stdout, "artifacts_scanned=%d\n", summary.ArtifactsScanned)
	fmt.Fprintf(stdout, "eligible_artifacts=%d\n", summary.EligibleArtifacts)
	fmt.Fprintf(stdout, "protected_artifacts=%d\n", summary.ProtectedArtifacts)
	fmt.Fprintf(stdout, "failed_artifacts=%d\n", summary.FailedArtifacts)
	fmt.Fprintf(stdout, "public_provenance_excluded=%d\n", summary.PublicProvenanceExcluded)
	fmt.Fprintf(stdout, "active_goal_excluded=%d\n", summary.ActiveGoalExcluded)
	fmt.Fprintf(stdout, "minimum_window_excluded=%d\n", summary.MinimumWindowExcluded)
	for _, audit := range summary.RetentionAudits {
		fmt.Fprintf(stdout, "artifact=%s retention_status=%s cleanup_review_status=%s retention_class=%s\n", audit.Artifact, audit.RetentionStatus, audit.CleanupReviewStatus, audit.RetentionClass)
	}
	for _, cleanupErr := range summary.Errors {
		fmt.Fprintf(stdout, "error=%s\n", cleanupErr)
	}
}

func writeGoalReadinessSummary(stdout io.Writer, summary goalReadinessSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"readiness_schema_version\":\"ao.forge.goal-run-readiness-audit.v0.1\",\"status\":\"failed\",\"errors\":[%q]}\n", err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_readiness=%s\n", summary.Status)
	fmt.Fprintf(stdout, "goal_run=%s\n", summary.GoalRun)
	if summary.GoalID != "" {
		fmt.Fprintf(stdout, "goal_id=%s\n", summary.GoalID)
	}
	if summary.CurrentPhase != "" {
		fmt.Fprintf(stdout, "current_phase=%s\n", summary.CurrentPhase)
	}
	if summary.RequestedPhase != "" {
		fmt.Fprintf(stdout, "requested_phase=%s\n", summary.RequestedPhase)
	}
	for _, check := range summary.Checks {
		fmt.Fprintf(stdout, "check=%s status=%s summary=%s\n", check.CheckID, check.Status, check.Summary)
	}
	fmt.Fprintf(stdout, "evidence_linted=%d\n", summary.EvidenceLint.EvidenceLinted)
	fmt.Fprintf(stdout, "evidence_verified=%d\n", summary.EvidenceVerify.EvidenceVerified)
	fmt.Fprintf(stdout, "retention_audits=%d\n", len(summary.RetentionAudits))
	for _, readinessErr := range summary.Errors {
		fmt.Fprintf(stdout, "error=%s\n", readinessErr)
	}
}

func writeGoalInspectSummary(stdout io.Writer, summary goalInspectSummary, asJSON bool) {
	if asJSON {
		data, err := marshalIndented(summary)
		if err != nil {
			fmt.Fprintf(stdout, "{\"inspect_schema_version\":\"ao.forge.goal-run-inspect.v0.1\",\"error\":%q}\n", err.Error())
			return
		}
		_, _ = stdout.Write(data)
		return
	}
	fmt.Fprintf(stdout, "goal_run=%s\n", summary.GoalRun)
	fmt.Fprintf(stdout, "schema_version=%s\n", summary.SchemaVersion)
	fmt.Fprintf(stdout, "goal_id=%s\n", summary.GoalID)
	fmt.Fprintf(stdout, "repo=%s\n", summary.Repo)
	fmt.Fprintf(stdout, "current_phase=%s\n", summary.CurrentPhase)
	fmt.Fprintf(stdout, "acceptance_criteria=%d\n", summary.AcceptanceCriteria)
	fmt.Fprintf(stdout, "allowed_scope=%d\n", summary.AllowedScope)
	fmt.Fprintf(stdout, "stop_conditions=%d\n", summary.StopConditions)
	fmt.Fprintf(stdout, "loop_owner=%s/%s/%s\n", summary.LoopOwner.StateOwner, summary.LoopOwner.Executor, summary.LoopOwner.Scheduler)
	fmt.Fprintf(stdout, "next_task=%s\n", summary.NextTask)
	fmt.Fprintf(stdout, "continuation_prompt=%s\n", summary.ContinuationPrompt)
	fmt.Fprintf(stdout, "next_action_guard_enabled=%t\n", summary.NextActionGuard.Enabled)
	fmt.Fprintf(stdout, "next_action_guard_on_mismatch=%s\n", summary.NextActionGuard.OnMismatch)
	if summary.LastIterationStatus != "" {
		fmt.Fprintf(stdout, "last_iteration_status=%s\n", summary.LastIterationStatus)
	}
}
