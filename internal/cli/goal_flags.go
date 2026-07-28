package cli

import (
	"fmt"
	"strings"
	"time"
)

type goalFlags struct {
	goalRunPath string
	json        bool
}

type goalTransitionFlags struct {
	goalRunPath string
	toPhase     string
	json        bool
}

type goalReadinessFlags struct {
	goalRunPath string
	toPhase     string
	now         string
	json        bool
}

type goalContextFlags struct {
	goalRunPath string
	handoffPath string
	now         string
	json        bool
}

type goalVerificationFlags struct {
	verificationPath string
	json             bool
}

type goalUpdateFlags struct {
	goalRunPath          string
	outPath              string
	phase                string
	nextTask             string
	lastVerifiedAt       string
	lastIterationStatus  string
	lastIterationSummary string
	evidencePaths        []string
	json                 bool
}

type goalEvidenceLintFlags struct {
	goalRunPath     string
	updateAuditPath string
	json            bool
}

type goalEvidenceRetentionFlags struct {
	artifactPath string
	now          string
	json         bool
}

type goalEvidenceCleanupFlags struct {
	root   string
	now    string
	dryRun bool
	json   bool
}

func parseGoalFlags(args []string) (goalFlags, error) {
	var flags goalFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--goal-run":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return goalFlags{}, fmt.Errorf("--goal-run requires a value")
			}
			flags.goalRunPath = args[i+1]
			i++
		case "--json":
			flags.json = true
		case "--help", "-h":
			return goalFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return goalFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return goalFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.goalRunPath == "" {
		return goalFlags{}, fmt.Errorf("missing required --goal-run")
	}
	return flags, nil
}

func parseGoalContextFlags(args []string) (goalContextFlags, error) {
	var flags goalContextFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--goal-run":
			value, next, err := readFlagValue(args, i, "--goal-run")
			if err != nil {
				return goalContextFlags{}, err
			}
			flags.goalRunPath = value
			i = next
		case "--handoff":
			value, next, err := readFlagValue(args, i, "--handoff")
			if err != nil {
				return goalContextFlags{}, err
			}
			flags.handoffPath = value
			i = next
		case "--now":
			value, next, err := readFlagValue(args, i, "--now")
			if err != nil {
				return goalContextFlags{}, err
			}
			flags.now = value
			i = next
		case "--json":
			flags.json = true
		case "--help", "-h":
			return goalContextFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return goalContextFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return goalContextFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.goalRunPath == "" {
		return goalContextFlags{}, fmt.Errorf("missing required --goal-run")
	}
	if flags.handoffPath == "" {
		return goalContextFlags{}, fmt.Errorf("missing required --handoff")
	}
	if flags.now != "" {
		if _, err := time.Parse(time.RFC3339, flags.now); err != nil {
			return goalContextFlags{}, fmt.Errorf("--now must be RFC3339: %w", err)
		}
	}
	return flags, nil
}

func parseGoalEvidenceLintFlags(args []string) (goalEvidenceLintFlags, error) {
	var flags goalEvidenceLintFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--goal-run":
			value, next, err := readFlagValue(args, i, "--goal-run")
			if err != nil {
				return goalEvidenceLintFlags{}, err
			}
			flags.goalRunPath = value
			i = next
		case "--update-audit":
			value, next, err := readFlagValue(args, i, "--update-audit")
			if err != nil {
				return goalEvidenceLintFlags{}, err
			}
			flags.updateAuditPath = value
			i = next
		case "--json":
			flags.json = true
		case "--help", "-h":
			return goalEvidenceLintFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return goalEvidenceLintFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return goalEvidenceLintFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.goalRunPath == "" && flags.updateAuditPath == "" {
		return goalEvidenceLintFlags{}, fmt.Errorf("missing required --goal-run or --update-audit")
	}
	if flags.goalRunPath != "" && flags.updateAuditPath != "" {
		return goalEvidenceLintFlags{}, fmt.Errorf("use exactly one of --goal-run or --update-audit")
	}
	return flags, nil
}

func parseGoalEvidenceRetentionFlags(args []string) (goalEvidenceRetentionFlags, error) {
	var flags goalEvidenceRetentionFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--artifact":
			value, next, err := readFlagValue(args, i, "--artifact")
			if err != nil {
				return goalEvidenceRetentionFlags{}, err
			}
			flags.artifactPath = value
			i = next
		case "--now":
			value, next, err := readFlagValue(args, i, "--now")
			if err != nil {
				return goalEvidenceRetentionFlags{}, err
			}
			flags.now = value
			i = next
		case "--json":
			flags.json = true
		case "--help", "-h":
			return goalEvidenceRetentionFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return goalEvidenceRetentionFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return goalEvidenceRetentionFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.artifactPath == "" {
		return goalEvidenceRetentionFlags{}, fmt.Errorf("missing required --artifact")
	}
	if flags.now != "" {
		if _, err := time.Parse(time.RFC3339, flags.now); err != nil {
			return goalEvidenceRetentionFlags{}, fmt.Errorf("--now must be RFC3339: %w", err)
		}
	}
	return flags, nil
}

func parseGoalEvidenceCleanupFlags(args []string) (goalEvidenceCleanupFlags, error) {
	flags := goalEvidenceCleanupFlags{root: "docs/evidence/goals"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			flags.dryRun = true
		case "--root":
			value, next, err := readFlagValue(args, i, "--root")
			if err != nil {
				return goalEvidenceCleanupFlags{}, err
			}
			flags.root = value
			i = next
		case "--now":
			value, next, err := readFlagValue(args, i, "--now")
			if err != nil {
				return goalEvidenceCleanupFlags{}, err
			}
			flags.now = value
			i = next
		case "--json":
			flags.json = true
		case "--help", "-h":
			return goalEvidenceCleanupFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return goalEvidenceCleanupFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return goalEvidenceCleanupFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if !flags.dryRun {
		return goalEvidenceCleanupFlags{}, fmt.Errorf("missing required --dry-run")
	}
	if strings.TrimSpace(flags.root) == "" {
		return goalEvidenceCleanupFlags{}, fmt.Errorf("--root must not be empty")
	}
	if flags.now != "" {
		if _, err := time.Parse(time.RFC3339, flags.now); err != nil {
			return goalEvidenceCleanupFlags{}, fmt.Errorf("--now must be RFC3339: %w", err)
		}
	}
	return flags, nil
}

func parseGoalTransitionFlags(args []string) (goalTransitionFlags, error) {
	var flags goalTransitionFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--goal-run":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return goalTransitionFlags{}, fmt.Errorf("--goal-run requires a value")
			}
			flags.goalRunPath = args[i+1]
			i++
		case "--to":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return goalTransitionFlags{}, fmt.Errorf("--to requires a value")
			}
			flags.toPhase = args[i+1]
			i++
		case "--json":
			flags.json = true
		case "--help", "-h":
			return goalTransitionFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return goalTransitionFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return goalTransitionFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.goalRunPath == "" {
		return goalTransitionFlags{}, fmt.Errorf("missing required --goal-run")
	}
	if flags.toPhase != "" && !isKnownGoalRunPhase(flags.toPhase) {
		return goalTransitionFlags{}, fmt.Errorf("unknown --to phase %q", flags.toPhase)
	}
	return flags, nil
}

func parseGoalReadinessFlags(args []string) (goalReadinessFlags, error) {
	var flags goalReadinessFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--goal-run":
			value, next, err := readFlagValue(args, i, "--goal-run")
			if err != nil {
				return goalReadinessFlags{}, err
			}
			flags.goalRunPath = value
			i = next
		case "--to":
			value, next, err := readFlagValue(args, i, "--to")
			if err != nil {
				return goalReadinessFlags{}, err
			}
			flags.toPhase = value
			i = next
		case "--now":
			value, next, err := readFlagValue(args, i, "--now")
			if err != nil {
				return goalReadinessFlags{}, err
			}
			flags.now = value
			i = next
		case "--json":
			flags.json = true
		case "--help", "-h":
			return goalReadinessFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return goalReadinessFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return goalReadinessFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.goalRunPath == "" {
		return goalReadinessFlags{}, fmt.Errorf("missing required --goal-run")
	}
	if flags.toPhase != "" && !isKnownGoalRunPhase(flags.toPhase) {
		return goalReadinessFlags{}, fmt.Errorf("unknown --to phase %q", flags.toPhase)
	}
	if flags.now != "" {
		if _, err := time.Parse(time.RFC3339, flags.now); err != nil {
			return goalReadinessFlags{}, fmt.Errorf("--now must be RFC3339: %w", err)
		}
	}
	return flags, nil
}

func parseGoalVerificationFlags(args []string) (goalVerificationFlags, error) {
	var flags goalVerificationFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--verification":
			value, next, err := readFlagValue(args, i, "--verification")
			if err != nil {
				return goalVerificationFlags{}, err
			}
			flags.verificationPath = value
			i = next
		case "--json":
			flags.json = true
		case "--help", "-h":
			return goalVerificationFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return goalVerificationFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return goalVerificationFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.verificationPath == "" {
		return goalVerificationFlags{}, fmt.Errorf("missing required --verification")
	}
	return flags, nil
}

func parseGoalUpdateFlags(args []string) (goalUpdateFlags, error) {
	var flags goalUpdateFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--goal-run":
			value, next, err := readFlagValue(args, i, "--goal-run")
			if err != nil {
				return goalUpdateFlags{}, err
			}
			flags.goalRunPath = value
			i = next
		case "--out":
			value, next, err := readFlagValue(args, i, "--out")
			if err != nil {
				return goalUpdateFlags{}, err
			}
			flags.outPath = value
			i = next
		case "--phase":
			value, next, err := readFlagValue(args, i, "--phase")
			if err != nil {
				return goalUpdateFlags{}, err
			}
			flags.phase = value
			i = next
		case "--next-task":
			value, next, err := readFlagValue(args, i, "--next-task")
			if err != nil {
				return goalUpdateFlags{}, err
			}
			flags.nextTask = value
			i = next
		case "--last-verified-at":
			value, next, err := readFlagValue(args, i, "--last-verified-at")
			if err != nil {
				return goalUpdateFlags{}, err
			}
			flags.lastVerifiedAt = value
			i = next
		case "--last-iteration-status":
			value, next, err := readFlagValue(args, i, "--last-iteration-status")
			if err != nil {
				return goalUpdateFlags{}, err
			}
			flags.lastIterationStatus = value
			i = next
		case "--last-iteration-summary":
			value, next, err := readFlagValue(args, i, "--last-iteration-summary")
			if err != nil {
				return goalUpdateFlags{}, err
			}
			flags.lastIterationSummary = value
			i = next
		case "--evidence":
			value, next, err := readFlagValue(args, i, "--evidence")
			if err != nil {
				return goalUpdateFlags{}, err
			}
			flags.evidencePaths = append(flags.evidencePaths, value)
			i = next
		case "--json":
			flags.json = true
		case "--help", "-h":
			return goalUpdateFlags{}, fmt.Errorf("help is available with `forge --help`")
		default:
			if strings.HasPrefix(args[i], "--") {
				return goalUpdateFlags{}, fmt.Errorf("unknown flag %s", args[i])
			}
			return goalUpdateFlags{}, fmt.Errorf("unexpected argument %s", args[i])
		}
	}
	if flags.goalRunPath == "" {
		return goalUpdateFlags{}, fmt.Errorf("missing required --goal-run")
	}
	if flags.outPath == "" {
		return goalUpdateFlags{}, fmt.Errorf("missing required --out")
	}
	if samePath(flags.goalRunPath, flags.outPath) {
		return goalUpdateFlags{}, fmt.Errorf("--out must differ from --goal-run")
	}
	if flags.phase != "" && !isKnownGoalRunPhase(flags.phase) {
		return goalUpdateFlags{}, fmt.Errorf("unknown --phase %q", flags.phase)
	}
	if !flags.hasMutation() {
		return goalUpdateFlags{}, fmt.Errorf("at least one update flag is required")
	}
	if flags.lastVerifiedAt != "" {
		if _, err := time.Parse(time.RFC3339, flags.lastVerifiedAt); err != nil {
			return goalUpdateFlags{}, fmt.Errorf("--last-verified-at must be RFC3339: %w", err)
		}
	}
	seenEvidence := map[string]struct{}{}
	for _, path := range flags.evidencePaths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return goalUpdateFlags{}, fmt.Errorf("--evidence requires a non-empty value")
		}
		display := displayPath(trimmed)
		if _, ok := seenEvidence[display]; ok {
			return goalUpdateFlags{}, fmt.Errorf("--evidence path repeated: %s", display)
		}
		seenEvidence[display] = struct{}{}
	}
	return flags, nil
}

func (flags goalUpdateFlags) hasMutation() bool {
	return flags.phase != "" ||
		flags.nextTask != "" ||
		flags.lastVerifiedAt != "" ||
		flags.lastIterationStatus != "" ||
		flags.lastIterationSummary != "" ||
		len(flags.evidencePaths) > 0
}

func readFlagValue(args []string, index int, flag string) (string, int, error) {
	if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
		return "", index, fmt.Errorf("%s requires a value", flag)
	}
	return args[index+1], index + 1, nil
}
