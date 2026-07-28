package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type goalEvidenceVerifySummary struct {
	VerifySchemaVersion string                     `json:"verify_schema_version"`
	GoalRun             string                     `json:"goal_run"`
	GoalID              string                     `json:"goal_id"`
	Status              string                     `json:"status"`
	EvidenceVerified    int                        `json:"evidence_verified"`
	Evidence            []goalEvidenceVerifyResult `json:"evidence"`
	Errors              []string                   `json:"errors"`
}

type goalEvidenceVerifyResult struct {
	Label          string `json:"label"`
	Path           string `json:"path"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	ActualSHA256   string `json:"actual_sha256,omitempty"`
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`
}

type goalEvidenceLintSummary struct {
	LintSchemaVersion string                   `json:"lint_schema_version"`
	Document          string                   `json:"document"`
	DocumentType      string                   `json:"document_type"`
	GoalID            string                   `json:"goal_id,omitempty"`
	Status            string                   `json:"status"`
	EvidenceLinted    int                      `json:"evidence_linted"`
	Evidence          []goalEvidenceLintResult `json:"evidence"`
	Errors            []string                 `json:"errors"`
}

type goalEvidenceLintResult struct {
	Label  string `json:"label"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type goalEvidenceRetentionSummary struct {
	RetentionAuditSchemaVersion            string   `json:"retention_audit_schema_version"`
	Artifact                               string   `json:"artifact"`
	GoalID                                 string   `json:"goal_id"`
	Iteration                              string   `json:"iteration"`
	Phase                                  string   `json:"phase"`
	RetentionClass                         string   `json:"retention_class"`
	RetainedAt                             string   `json:"retained_at"`
	Now                                    string   `json:"now"`
	Status                                 string   `json:"status"`
	RetentionStatus                        string   `json:"retention_status"`
	CleanupReviewStatus                    string   `json:"cleanup_review_status"`
	AgeDays                                int      `json:"age_days"`
	MinimumRetentionDaysAfterTerminalPhase int      `json:"minimum_retention_days_after_terminal_phase"`
	MandatoryRetentionUntil                string   `json:"mandatory_retention_until,omitempty"`
	Errors                                 []string `json:"errors"`
}

type goalEvidenceCleanupSummary struct {
	CleanupSchemaVersion     string                         `json:"cleanup_schema_version"`
	Root                     string                         `json:"root"`
	Now                      string                         `json:"now"`
	Mode                     string                         `json:"mode"`
	Status                   string                         `json:"status"`
	ArtifactsScanned         int                            `json:"artifacts_scanned"`
	EligibleArtifacts        int                            `json:"eligible_artifacts"`
	ProtectedArtifacts       int                            `json:"protected_artifacts"`
	FailedArtifacts          int                            `json:"failed_artifacts"`
	PublicProvenanceExcluded int                            `json:"public_provenance_excluded"`
	ActiveGoalExcluded       int                            `json:"active_goal_excluded"`
	MinimumWindowExcluded    int                            `json:"minimum_window_excluded"`
	RetentionAudits          []goalEvidenceRetentionSummary `json:"retention_audits"`
	Errors                   []string                       `json:"errors"`
}

func validateAndReadGoalRetainedEvidence(path string) (goalRetainedEvidenceArtifact, error) {
	if err := validateJSONSchemaDocument(resolveDefaultContractPath(goalRetainedEvidencePath), path); err != nil {
		return goalRetainedEvidenceArtifact{}, err
	}
	artifact, err := readGoalRetainedEvidence(path)
	if err != nil {
		return goalRetainedEvidenceArtifact{}, err
	}
	if artifact.SchemaVersion != goalRetainedEvidenceVersion {
		return goalRetainedEvidenceArtifact{}, fmt.Errorf("unsupported retained evidence schema_version %q", artifact.SchemaVersion)
	}
	return artifact, nil
}

func applyGoalEvidenceRetentionArtifact(summary *goalEvidenceRetentionSummary, artifact goalRetainedEvidenceArtifact) {
	summary.GoalID = artifact.GoalID
	summary.Iteration = artifact.Iteration
	summary.Phase = artifact.Phase
	summary.RetentionClass = artifact.RetentionMetadata.RetentionClass
	summary.RetainedAt = artifact.RetentionMetadata.RetainedAt
	summary.MinimumRetentionDaysAfterTerminalPhase = artifact.RetentionPolicy.MinimumRetentionDaysAfterTerminalPhase
}

func allGoalRetentionAuditsPassed(audits []goalEvidenceRetentionSummary) bool {
	for _, audit := range audits {
		if audit.Status != "passed" {
			return false
		}
	}
	return true
}

func retainedGoalRunEvidence(goal goalRun) []goalRunEvidence {
	if goal.LastIteration == nil {
		return nil
	}
	var retained []goalRunEvidence
	for _, evidence := range goal.LastIteration.Evidence {
		if strings.HasPrefix(strings.ReplaceAll(evidence.Path, `\`, "/"), "docs/evidence/goals/") {
			retained = append(retained, evidence)
		}
	}
	return retained
}

func emptyGoalEvidenceVerifySummary(path string) goalEvidenceVerifySummary {
	return goalEvidenceVerifySummary{
		VerifySchemaVersion: "ao.forge.goal-run-evidence-verify.v0.1",
		GoalRun:             displayPath(path),
		Status:              "passed",
		Evidence:            []goalEvidenceVerifyResult{},
		Errors:              []string{},
	}
}

func buildGoalEvidenceVerifySummary(path string, goal goalRun) goalEvidenceVerifySummary {
	summary := emptyGoalEvidenceVerifySummary(path)
	summary.GoalID = goal.GoalID
	if goal.LastIteration != nil {
		for _, evidence := range goal.LastIteration.Evidence {
			result := verifyGoalRunEvidence(evidence)
			summary.Evidence = append(summary.Evidence, result)
			if result.Status == "passed" {
				summary.EvidenceVerified++
			} else {
				summary.Status = "failed"
				summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s", result.Path, result.Error))
			}
		}
	}
	return summary
}

func emptyGoalEvidenceLintSummary(path string) goalEvidenceLintSummary {
	return goalEvidenceLintSummary{
		LintSchemaVersion: "ao.forge.goal-run-evidence-lint.v0.1",
		Document:          displayPath(path),
		DocumentType:      "goal_run",
		Status:            "passed",
		Evidence:          []goalEvidenceLintResult{},
		Errors:            []string{},
	}
}

func buildGoalEvidenceLintSummaryForGoal(path string, goal goalRun) goalEvidenceLintSummary {
	summary := emptyGoalEvidenceLintSummary(path)
	summary.GoalID = goal.GoalID
	if goal.LastIteration == nil {
		return summary
	}
	for _, item := range goal.LastIteration.Evidence {
		result := lintGoalRunEvidencePath(item)
		summary.Evidence = append(summary.Evidence, result)
		if result.Status == "passed" {
			summary.EvidenceLinted++
		} else {
			summary.Status = "failed"
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s", result.Path, result.Error))
		}
	}
	return summary
}

func buildGoalEvidenceRetentionSummary(path string, now time.Time) goalEvidenceRetentionSummary {
	summary := goalEvidenceRetentionSummary{
		RetentionAuditSchemaVersion: "ao.forge.goal-run-retained-evidence-audit.v0.1",
		Artifact:                    displayPath(path),
		Now:                         now.UTC().Format(time.RFC3339),
		Status:                      "passed",
		RetentionStatus:             "unknown",
		CleanupReviewStatus:         "unknown",
		Errors:                      []string{},
	}

	artifact, err := validateAndReadGoalRetainedEvidence(resolveGoalRunEvidencePath(path))
	if err != nil {
		summary.Status = "failed"
		summary.Errors = []string{err.Error()}
		return summary
	}

	applyGoalEvidenceRetentionArtifact(&summary, artifact)
	retainedAt, err := time.Parse(time.RFC3339, artifact.RetentionMetadata.RetainedAt)
	if err != nil {
		summary.Status = "failed"
		summary.RetentionStatus = "unknown"
		summary.CleanupReviewStatus = "unknown"
		summary.Errors = []string{fmt.Sprintf("retained_at must be RFC3339: %v", err)}
		return summary
	}

	retainedAt = retainedAt.UTC()
	summary.RetainedAt = retainedAt.Format(time.RFC3339)
	if retainedAt.After(now) {
		summary.Status = "failed"
		summary.RetentionStatus = "unknown"
		summary.CleanupReviewStatus = "unknown"
		summary.Errors = []string{fmt.Sprintf("retained_at %s is after now %s", summary.RetainedAt, summary.Now)}
		return summary
	}

	summary.AgeDays = int(now.Sub(retainedAt).Hours() / 24)
	if isPublicProvenanceRetentionClass(artifact.RetentionMetadata.RetentionClass) {
		summary.RetentionStatus = "mandatory_retention"
		summary.CleanupReviewStatus = "not_eligible_public_provenance"
	} else if isTerminalGoalRunPhase(artifact.Phase) {
		mandatoryUntil := retainedAt.AddDate(0, 0, artifact.RetentionPolicy.MinimumRetentionDaysAfterTerminalPhase)
		summary.MandatoryRetentionUntil = mandatoryUntil.UTC().Format(time.RFC3339)
		if now.Before(mandatoryUntil) {
			summary.RetentionStatus = "mandatory_retention"
			summary.CleanupReviewStatus = "not_eligible_minimum_window"
		} else {
			summary.RetentionStatus = "cleanup_review_eligible"
			summary.CleanupReviewStatus = "eligible_after_review"
		}
	} else {
		summary.RetentionStatus = "active_retention"
		summary.CleanupReviewStatus = "not_eligible_active_goal"
	}
	return summary
}

func buildGoalEvidenceCleanupSummary(root string, now time.Time) goalEvidenceCleanupSummary {
	resolvedRoot := resolveDefaultContractPath(root)
	summary := goalEvidenceCleanupSummary{
		CleanupSchemaVersion: goalEvidenceCleanupVersion,
		Root:                 displayPath(resolvedRoot),
		Now:                  now.UTC().Format(time.RFC3339),
		Mode:                 "dry-run",
		Status:               "passed",
		RetentionAudits:      []goalEvidenceRetentionSummary{},
		Errors:               []string{},
	}

	artifacts, err := discoverRetainedEvidenceArtifacts(resolvedRoot)
	if err != nil {
		summary.Status = "failed"
		summary.Errors = []string{err.Error()}
		return summary
	}

	for _, artifactPath := range artifacts {
		audit := buildGoalEvidenceRetentionSummary(artifactPath, now)
		summary.RetentionAudits = append(summary.RetentionAudits, audit)
		summary.ArtifactsScanned++
		if audit.Status != "passed" {
			summary.FailedArtifacts++
			summary.Status = "failed"
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %s", audit.Artifact, strings.Join(audit.Errors, "; ")))
			continue
		}
		switch audit.CleanupReviewStatus {
		case "eligible_after_review":
			summary.EligibleArtifacts++
		case "not_eligible_public_provenance":
			summary.ProtectedArtifacts++
			summary.PublicProvenanceExcluded++
		case "not_eligible_active_goal":
			summary.ProtectedArtifacts++
			summary.ActiveGoalExcluded++
		case "not_eligible_minimum_window":
			summary.ProtectedArtifacts++
			summary.MinimumWindowExcluded++
		default:
			summary.ProtectedArtifacts++
		}
	}
	return summary
}

func discoverRetainedEvidenceArtifacts(root string) ([]string, error) {
	resolvedRoot := resolveDefaultContractPath(root)
	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return nil, fmt.Errorf("%s is not readable: %v", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	linkInfo, err := os.Lstat(resolvedRoot)
	if err != nil {
		return nil, fmt.Errorf("%s is not readable: %v", root, err)
	}
	if linkInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symlink", root)
	}

	var artifacts []string
	budget := retainedEvidenceCleanupScanBudget{}
	err = filepath.WalkDir(resolvedRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("retained evidence cleanup symlink is not allowed: %s", filepath.ToSlash(path))
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if err := budget.accept(path, info); err != nil {
			return err
		}
		if retainedEvidenceFileHasSchema(path) {
			artifacts = append(artifacts, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan retained evidence root: %w", err)
	}
	sort.Strings(artifacts)
	return artifacts, nil
}

type retainedEvidenceCleanupScanBudget struct {
	files      int
	totalBytes int64
}

func (budget *retainedEvidenceCleanupScanBudget) accept(path string, info fs.FileInfo) error {
	budget.files++
	if budget.files > maxRetainedEvidenceCleanupFiles {
		return fmt.Errorf("retained evidence cleanup file count limit exceeded: max %d", maxRetainedEvidenceCleanupFiles)
	}
	size := info.Size()
	if size > maxRetainedEvidenceCleanupFileBytes {
		return fmt.Errorf("retained evidence cleanup file size limit exceeded for %s: max %d bytes", filepath.ToSlash(path), maxRetainedEvidenceCleanupFileBytes)
	}
	budget.totalBytes += size
	if budget.totalBytes > maxRetainedEvidenceCleanupTotalBytes {
		return fmt.Errorf("retained evidence cleanup total byte limit exceeded: max %d bytes", maxRetainedEvidenceCleanupTotalBytes)
	}
	return nil
}

func retainedEvidenceFileHasSchema(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var header struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return false
	}
	return header.SchemaVersion == goalRetainedEvidenceVersion
}

func isPublicProvenanceRetentionClass(retentionClass string) bool {
	return retentionClass == "release_provenance" || retentionClass == "promotion_provenance"
}

func verifyGoalRunEvidence(evidence goalRunEvidence) goalEvidenceVerifyResult {
	result := goalEvidenceVerifyResult{
		Label:          evidence.Label,
		Path:           evidence.Path,
		ExpectedSHA256: evidence.SHA256,
		Status:         "passed",
	}
	if strings.TrimSpace(evidence.SHA256) == "" {
		result.Status = "failed"
		result.Error = "missing sha256"
		return result
	}
	path := resolveGoalRunEvidencePath(evidence.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("read evidence: %v", err)
		return result
	}
	sum := sha256.Sum256(data)
	result.ActualSHA256 = hex.EncodeToString(sum[:])
	if result.ActualSHA256 != evidence.SHA256 {
		result.Status = "failed"
		result.Error = fmt.Sprintf("sha256 mismatch: expected %s, got %s", evidence.SHA256, result.ActualSHA256)
	}
	return result
}

func lintGoalRunEvidencePath(evidence goalRunEvidence) goalEvidenceLintResult {
	result := goalEvidenceLintResult{
		Label:  evidence.Label,
		Path:   evidence.Path,
		Status: "passed",
	}
	if reason := rejectedGoalRunEvidencePathReason(evidence.Path); reason != "" {
		result.Status = "failed"
		result.Error = reason
	}
	return result
}

func rejectedGoalRunEvidencePathReason(path string) string {
	normalized := strings.ReplaceAll(path, `\`, "/")
	if strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "//") {
		return "uses an absolute path"
	}
	if windowsAbsolutePathPattern.MatchString(normalized) {
		return "uses an absolute path"
	}
	if strings.HasPrefix(normalized, "~/") || strings.HasPrefix(normalized, "$HOME/") || strings.HasPrefix(normalized, "${HOME}/") {
		return "uses a home-directory path"
	}
	parts := strings.Split(normalized, "/")
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		switch part {
		case "..":
			return "uses a parent traversal path"
		case "tmp", ".tmp", "temp":
			return "uses a temporary path"
		}
	}
	return ""
}

func resolveGoalRunEvidencePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if root, ok := findRepoRoot(); ok {
		return filepath.Join(root, filepath.FromSlash(path))
	}
	return path
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func ensureGoalRunEvidenceReadyForUpdate(path string, goal goalRun) error {
	lint := buildGoalEvidenceLintSummaryForGoal(path, goal)
	if lint.Status != "passed" {
		return fmt.Errorf("existing GoalRun evidence lint failed: %s", strings.Join(lint.Errors, "; "))
	}
	verify := buildGoalEvidenceVerifySummary(path, goal)
	if verify.Status != "passed" {
		return fmt.Errorf("existing GoalRun evidence verification failed: %s", strings.Join(verify.Errors, "; "))
	}
	return nil
}

func buildGoalRunEvidence(path string) (goalRunEvidence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return goalRunEvidence{}, fmt.Errorf("read evidence %s: %w", displayPath(path), err)
	}
	sum := sha256.Sum256(data)
	display := displayPath(path)
	return goalRunEvidence{
		Label:  filepath.Base(display),
		Path:   display,
		SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func readGoalRetainedEvidence(path string) (goalRetainedEvidenceArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return goalRetainedEvidenceArtifact{}, err
	}
	var artifact goalRetainedEvidenceArtifact
	if err := decodeJSONStrict(data, &artifact); err != nil {
		return goalRetainedEvidenceArtifact{}, fmt.Errorf("parse retained evidence JSON: %w", err)
	}
	return artifact, nil
}
