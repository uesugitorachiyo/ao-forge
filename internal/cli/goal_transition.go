package cli

import (
	"fmt"
	"os"
	"strings"
)

type goalTransitionSummary struct {
	TransitionSchemaVersion string   `json:"transition_schema_version"`
	GoalRun                 string   `json:"goal_run"`
	GoalID                  string   `json:"goal_id"`
	CurrentPhase            string   `json:"current_phase"`
	AllowedNextPhases       []string `json:"allowed_next_phases"`
	RequestedPhase          string   `json:"requested_phase,omitempty"`
	Status                  string   `json:"status"`
	Reason                  string   `json:"reason"`
}

type goalUpdateAudit struct {
	AuditSchemaVersion   string            `json:"audit_schema_version"`
	GoalRun              string            `json:"goal_run"`
	Out                  string            `json:"out"`
	GoalID               string            `json:"goal_id"`
	PreviousPhase        string            `json:"previous_phase"`
	CurrentPhase         string            `json:"current_phase"`
	PhaseTransition      string            `json:"phase_transition"`
	UpdatedFields        []string          `json:"updated_fields"`
	LastVerifiedAt       string            `json:"last_verified_at"`
	LastIterationStatus  string            `json:"last_iteration_status,omitempty"`
	LastIterationSummary string            `json:"last_iteration_summary,omitempty"`
	Evidence             []goalRunEvidence `json:"evidence,omitempty"`
	Status               string            `json:"status"`
}

func validateAndReadGoalRunUpdateAudit(path string) (goalUpdateAudit, error) {
	if err := validateJSONSchemaDocument(resolveDefaultContractPath(goalRunUpdateAuditSchemaPath), path); err != nil {
		return goalUpdateAudit{}, err
	}
	audit, err := readGoalRunUpdateAudit(path)
	if err != nil {
		return goalUpdateAudit{}, err
	}
	if audit.AuditSchemaVersion != goalRunUpdateAuditVersion {
		return goalUpdateAudit{}, fmt.Errorf("unsupported goal run update audit schema_version %q", audit.AuditSchemaVersion)
	}
	return audit, nil
}

func applyGoalRunUpdate(goal *goalRun, flags goalUpdateFlags) ([]string, error) {
	var updated []string
	if flags.phase != "" {
		if flags.phase != goal.CurrentPhase {
			transition := buildGoalTransitionSummary(flags.goalRunPath, *goal, flags.phase)
			if transition.Status != "allowed" {
				return nil, fmt.Errorf("phase transition denied: %s", transition.Reason)
			}
			goal.CurrentPhase = flags.phase
			updated = append(updated, "current_phase")
		}
	}
	if flags.nextTask != "" && flags.nextTask != goal.NextTask {
		goal.NextTask = flags.nextTask
		updated = append(updated, "next_task")
	}
	if flags.lastVerifiedAt != "" && flags.lastVerifiedAt != goal.LastVerifiedAt {
		goal.LastVerifiedAt = flags.lastVerifiedAt
		updated = append(updated, "last_verified_at")
	}
	if flags.lastIterationStatus != "" || flags.lastIterationSummary != "" || len(flags.evidencePaths) > 0 {
		if goal.LastIteration == nil {
			if flags.lastIterationStatus == "" || flags.lastIterationSummary == "" {
				return nil, fmt.Errorf("--last-iteration-status and --last-iteration-summary are required when creating last_iteration")
			}
			goal.LastIteration = &goalRunIteration{Evidence: []goalRunEvidence{}}
		}
		if flags.lastIterationStatus != "" && flags.lastIterationStatus != goal.LastIteration.Status {
			goal.LastIteration.Status = flags.lastIterationStatus
			updated = append(updated, "last_iteration.status")
		}
		if flags.lastIterationSummary != "" && flags.lastIterationSummary != goal.LastIteration.Summary {
			goal.LastIteration.Summary = flags.lastIterationSummary
			updated = append(updated, "last_iteration.summary")
		}
		if goal.LastIteration.Evidence == nil {
			goal.LastIteration.Evidence = []goalRunEvidence{}
		}
		if len(flags.evidencePaths) > 0 {
			existingEvidence := map[string]struct{}{}
			for _, evidence := range goal.LastIteration.Evidence {
				existingEvidence[evidence.Path] = struct{}{}
			}
			for _, path := range flags.evidencePaths {
				evidence, err := buildGoalRunEvidence(path)
				if err != nil {
					return nil, err
				}
				if _, ok := existingEvidence[evidence.Path]; ok {
					return nil, fmt.Errorf("--evidence path already attached: %s", evidence.Path)
				}
				goal.LastIteration.Evidence = append(goal.LastIteration.Evidence, evidence)
				existingEvidence[evidence.Path] = struct{}{}
			}
			updated = append(updated, "last_iteration.evidence")
		}
	}
	return updated, nil
}

func buildGoalUpdateAudit(flags goalUpdateFlags, goal goalRun, previousPhase string, updatedFields []string) goalUpdateAudit {
	phaseTransition := "unchanged"
	if previousPhase != goal.CurrentPhase {
		phaseTransition = previousPhase + "->" + goal.CurrentPhase
	}
	audit := goalUpdateAudit{
		AuditSchemaVersion: "ao.forge.goal-run-update-audit.v0.1",
		GoalRun:            displayPath(flags.goalRunPath),
		Out:                displayPath(flags.outPath),
		GoalID:             goal.GoalID,
		PreviousPhase:      previousPhase,
		CurrentPhase:       goal.CurrentPhase,
		PhaseTransition:    phaseTransition,
		UpdatedFields:      append([]string(nil), updatedFields...),
		LastVerifiedAt:     goal.LastVerifiedAt,
		Status:             "updated",
	}
	if goal.LastIteration != nil {
		audit.LastIterationStatus = goal.LastIteration.Status
		audit.LastIterationSummary = goal.LastIteration.Summary
		audit.Evidence = append([]goalRunEvidence(nil), goal.LastIteration.Evidence...)
	}
	return audit
}

func buildGoalTransitionSummary(path string, goal goalRun, toPhase string) goalTransitionSummary {
	allowed := allowedGoalRunNextPhases(goal.CurrentPhase)
	summary := goalTransitionSummary{
		TransitionSchemaVersion: "ao.forge.goal-run-transition.v0.1",
		GoalRun:                 displayPath(path),
		GoalID:                  goal.GoalID,
		CurrentPhase:            goal.CurrentPhase,
		AllowedNextPhases:       allowed,
		RequestedPhase:          toPhase,
		Status:                  "listed",
		Reason:                  fmt.Sprintf("phase %s allows: %s", goal.CurrentPhase, formatGoalRunPhaseList(allowed)),
	}
	if toPhase == "" {
		return summary
	}
	if containsString(allowed, toPhase) {
		summary.Status = "allowed"
		summary.Reason = fmt.Sprintf("transition %s -> %s is allowed", goal.CurrentPhase, toPhase)
		return summary
	}
	summary.Status = "denied"
	summary.Reason = fmt.Sprintf("transition %s -> %s is not allowed", goal.CurrentPhase, toPhase)
	return summary
}

func readGoalRunUpdateAudit(path string) (goalUpdateAudit, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return goalUpdateAudit{}, err
	}
	var audit goalUpdateAudit
	if err := decodeJSONStrict(data, &audit); err != nil {
		return goalUpdateAudit{}, fmt.Errorf("parse goal run update audit JSON: %w", err)
	}
	return audit, nil
}

func allowedGoalRunNextPhases(phase string) []string {
	allowed, ok := goalRunPhaseTransitions[phase]
	if !ok {
		return nil
	}
	return append([]string(nil), allowed...)
}

func isKnownGoalRunPhase(phase string) bool {
	_, ok := goalRunPhaseTransitions[phase]
	return ok
}

func isTerminalGoalRunPhase(phase string) bool {
	return phase == "complete" || phase == "stopped"
}

func formatGoalRunPhaseList(phases []string) string {
	if len(phases) == 0 {
		return "none"
	}
	return strings.Join(phases, ",")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func goalRunGuardStatus(goal goalRun) string {
	if goal.NextActionGuard.MustReadLatestGoalRun &&
		goal.NextActionGuard.MustMatchAllowedScope &&
		goal.NextActionGuard.MustSatisfyAcceptanceCriteria &&
		goal.NextActionGuard.OnMismatch == "backoff_or_stop" {
		return "enabled"
	}
	return "disabled"
}
