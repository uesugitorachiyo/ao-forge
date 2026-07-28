package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type goalValidationSummary struct {
	GoalRun         string   `json:"goal_run"`
	Schema          string   `json:"schema"`
	SchemaVersion   string   `json:"schema_version"`
	GoalID          string   `json:"goal_id"`
	CurrentPhase    string   `json:"current_phase"`
	NextActionGuard string   `json:"next_action_guard"`
	Status          string   `json:"status"`
	Errors          []string `json:"errors"`
}

type goalInspectSummary struct {
	InspectSchemaVersion string `json:"inspect_schema_version"`
	GoalRun              string `json:"goal_run"`
	SchemaVersion        string `json:"schema_version"`
	GoalID               string `json:"goal_id"`
	Repo                 string `json:"repo"`
	CurrentPhase         string `json:"current_phase"`
	NextTask             string `json:"next_task"`
	LastVerifiedAt       string `json:"last_verified_at"`
	ContinuationPrompt   string `json:"continuation_prompt"`
	AcceptanceCriteria   int    `json:"acceptance_criteria"`
	AllowedScope         int    `json:"allowed_scope"`
	StopConditions       int    `json:"stop_conditions"`
	LoopOwner            struct {
		StateOwner string `json:"state_owner"`
		Executor   string `json:"executor"`
		Scheduler  string `json:"scheduler"`
	} `json:"loop_owner"`
	NextActionGuard struct {
		Enabled    bool   `json:"enabled"`
		OnMismatch string `json:"on_mismatch"`
	} `json:"next_action_guard"`
	LastIterationStatus  string `json:"last_iteration_status,omitempty"`
	LastIterationSummary string `json:"last_iteration_summary,omitempty"`
}

type goalContextHandoff struct {
	SchemaVersion string `json:"schema_version"`
	GoalID        string `json:"goal_id"`
	GoalRun       string `json:"goal_run"`
	CapturedAt    string `json:"captured_at"`
	ContextBudget struct {
		EstimatedTokensUsed int    `json:"estimated_tokens_used"`
		MaxTokens           int    `json:"max_tokens"`
		CheckpointReason    string `json:"checkpoint_reason"`
	} `json:"context_budget"`
	CurrentTask   string   `json:"current_task"`
	Completed     []string `json:"completed"`
	Decisions     []string `json:"decisions"`
	FilesTouched  []string `json:"files_touched"`
	NextSteps     []string `json:"next_steps"`
	OpenQuestions []string `json:"open_questions"`
	ResumeGuard   struct {
		MustReadLatestGoalRun bool   `json:"must_read_latest_goal_run"`
		MustRunGoalReadiness  bool   `json:"must_run_goal_readiness"`
		OnStaleContext        string `json:"on_stale_context"`
	} `json:"resume_guard"`
}

type goalContextSummary struct {
	ContextSchemaVersion string   `json:"context_schema_version"`
	GoalRun              string   `json:"goal_run"`
	Handoff              string   `json:"handoff"`
	GoalID               string   `json:"goal_id,omitempty"`
	CapturedAt           string   `json:"captured_at,omitempty"`
	Now                  string   `json:"now,omitempty"`
	AgeHours             int      `json:"age_hours"`
	ResumeGuard          string   `json:"resume_guard"`
	Status               string   `json:"status"`
	Errors               []string `json:"errors"`
}

type goalRunVerification struct {
	SchemaVersion string `json:"schema_version"`
	GoalID        string `json:"goal_id"`
	GoalRun       string `json:"goal_run"`
	VerifiedAt    string `json:"verified_at"`
	Phases        []struct {
		Phase    string   `json:"phase"`
		Status   string   `json:"status"`
		Command  string   `json:"command"`
		Evidence []string `json:"evidence"`
		Required bool     `json:"required"`
	} `json:"phases"`
	SecurityReview struct {
		Status             string   `json:"status"`
		ScopesChecked      []string `json:"scopes_checked"`
		Findings           []string `json:"findings"`
		RecommendedActions []string `json:"recommended_actions"`
	} `json:"security_review"`
	MutatesLiveState bool `json:"mutates_live_state"`
}

type goalVerificationSummary struct {
	VerificationSchemaVersion string   `json:"verification_schema_version"`
	Verification              string   `json:"verification"`
	GoalID                    string   `json:"goal_id,omitempty"`
	GoalRun                   string   `json:"goal_run,omitempty"`
	VerifiedAt                string   `json:"verified_at,omitempty"`
	Status                    string   `json:"status"`
	PhasesChecked             int      `json:"phases_checked"`
	Errors                    []string `json:"errors"`
}

func validateAndReadGoalRun(path string) (goalRun, error) {
	if err := validateJSONSchemaDocument(resolveDefaultContractPath(goalRunSchemaPath), path); err != nil {
		return goalRun{}, err
	}
	goal, err := readGoalRun(path)
	if err != nil {
		return goalRun{}, err
	}
	if goal.SchemaVersion != goalRunSchemaVersion {
		return goalRun{}, fmt.Errorf("unsupported goal run schema_version %q", goal.SchemaVersion)
	}
	return goal, nil
}

func validateAndReadGoalContextHandoff(path string) (goalContextHandoff, error) {
	if err := validateJSONSchemaDocument(resolveDefaultContractPath(goalContextHandoffSchemaPath), path); err != nil {
		return goalContextHandoff{}, err
	}
	handoff, err := readGoalContextHandoff(path)
	if err != nil {
		return goalContextHandoff{}, err
	}
	if handoff.SchemaVersion != goalContextHandoffVersion {
		return goalContextHandoff{}, fmt.Errorf("unsupported context handoff schema_version %q", handoff.SchemaVersion)
	}
	return handoff, nil
}

func validateAndReadGoalVerification(path string) (goalRunVerification, error) {
	if err := validateJSONSchemaDocument(resolveDefaultContractPath(goalVerificationSchemaPath), path); err != nil {
		return goalRunVerification{}, err
	}
	verification, err := readGoalVerification(path)
	if err != nil {
		return goalRunVerification{}, err
	}
	if verification.SchemaVersion != goalVerificationVersion {
		return goalRunVerification{}, fmt.Errorf("unsupported verification schema_version %q", verification.SchemaVersion)
	}
	return verification, nil
}

func validateGoalVerificationSemantics(verification goalRunVerification) []string {
	var validationErrors []string
	if verification.MutatesLiveState {
		validationErrors = append(validationErrors, "verification evidence must be non-mutating")
	}
	if _, err := time.Parse(time.RFC3339, verification.VerifiedAt); err != nil {
		validationErrors = append(validationErrors, fmt.Sprintf("verified_at must be RFC3339: %v", err))
	}
	required := map[string]bool{
		"build":            false,
		"contract_schema":  false,
		"lint":             false,
		"public_readiness": false,
		"security_scan":    false,
		"tests":            false,
		"type_or_vet":      false,
	}
	for _, phase := range verification.Phases {
		if _, ok := required[phase.Phase]; ok {
			required[phase.Phase] = true
		}
		if phase.Required && phase.Status != "passed" {
			validationErrors = append(validationErrors, fmt.Sprintf("required verification phase %s status=%s", phase.Phase, phase.Status))
		}
		if phase.Required && strings.TrimSpace(phase.Command) == "" {
			validationErrors = append(validationErrors, fmt.Sprintf("required verification phase %s missing command", phase.Phase))
		}
		if phase.Required && len(phase.Evidence) == 0 {
			validationErrors = append(validationErrors, fmt.Sprintf("required verification phase %s missing evidence", phase.Phase))
		}
	}
	keys := make([]string, 0, len(required))
	for phase := range required {
		keys = append(keys, phase)
	}
	sort.Strings(keys)
	for _, phase := range keys {
		if !required[phase] {
			validationErrors = append(validationErrors, fmt.Sprintf("missing required verification phase %s", phase))
		}
	}
	if verification.SecurityReview.Status != "passed" {
		validationErrors = append(validationErrors, fmt.Sprintf("security review status=%s", verification.SecurityReview.Status))
	}
	if len(verification.SecurityReview.ScopesChecked) == 0 {
		validationErrors = append(validationErrors, "security review scopes_checked is required")
	}
	return validationErrors
}

func validateGoalRunValue(goal goalRun) error {
	var document any
	data, err := json.Marshal(goal)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	return validateJSONSchemaValue(resolveDefaultContractPath(goalRunSchemaPath), document)
}

func readGoalRun(path string) (goalRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return goalRun{}, err
	}
	var goal goalRun
	if err := decodeJSONStrict(data, &goal); err != nil {
		return goalRun{}, fmt.Errorf("parse goal run JSON: %w", err)
	}
	return goal, nil
}

func readGoalContextHandoff(path string) (goalContextHandoff, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return goalContextHandoff{}, err
	}
	var handoff goalContextHandoff
	if err := decodeJSONStrict(data, &handoff); err != nil {
		return goalContextHandoff{}, fmt.Errorf("parse context handoff JSON: %w", err)
	}
	return handoff, nil
}

func readGoalVerification(path string) (goalRunVerification, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return goalRunVerification{}, err
	}
	var verification goalRunVerification
	if err := decodeJSONStrict(data, &verification); err != nil {
		return goalRunVerification{}, fmt.Errorf("parse goal verification JSON: %w", err)
	}
	return verification, nil
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return left == right
	}
	return leftAbs == rightAbs
}

func resolveDefaultContractPath(relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	if _, err := os.Stat(relativePath); err == nil {
		return relativePath
	}
	wd, err := os.Getwd()
	if err != nil {
		return relativePath
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, relativePath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
	}
	return relativePath
}

func buildGoalInspectSummary(path string, goal goalRun) goalInspectSummary {
	var summary goalInspectSummary
	summary.InspectSchemaVersion = "ao.forge.goal-run-inspect.v0.1"
	summary.GoalRun = displayPath(path)
	summary.SchemaVersion = goal.SchemaVersion
	summary.GoalID = goal.GoalID
	summary.Repo = goal.Repo
	summary.CurrentPhase = goal.CurrentPhase
	summary.NextTask = goal.NextTask
	summary.LastVerifiedAt = goal.LastVerifiedAt
	summary.ContinuationPrompt = goal.ContinuationPrompt
	summary.AcceptanceCriteria = len(goal.AcceptanceCriteria)
	summary.AllowedScope = len(goal.AllowedScope)
	summary.StopConditions = len(goal.StopConditions)
	summary.LoopOwner.StateOwner = goal.LoopOwner.StateOwner
	summary.LoopOwner.Executor = goal.LoopOwner.Executor
	summary.LoopOwner.Scheduler = goal.LoopOwner.Scheduler
	summary.NextActionGuard.Enabled = goalRunGuardStatus(goal) == "enabled"
	summary.NextActionGuard.OnMismatch = goal.NextActionGuard.OnMismatch
	if goal.LastIteration != nil {
		summary.LastIterationStatus = goal.LastIteration.Status
		summary.LastIterationSummary = goal.LastIteration.Summary
	}
	return summary
}
