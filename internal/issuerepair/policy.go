package issuerepair

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
)

type Policy struct {
	Repository           string   `json:"repository"`
	SourceSHA            string   `json:"source_sha"`
	BaseSHA              string   `json:"base_sha"`
	WorkspaceRoot        string   `json:"workspace_root"`
	MaxPages             int      `json:"max_pages"`
	MaxWrites            int      `json:"max_writes"`
	PushAuthorized       bool     `json:"push_authorized"`
	PRAuthorized         bool     `json:"pr_authorized"`
	ReproductionCommand  []string `json:"reproduction_command"`
	ExpectedExitCode     int      `json:"expected_exit_code"`
	ExpectedOutputSHA256 string   `json:"expected_output_sha256"`
}

func validatePolicy(policy Policy) error {
	if policy.Repository == "" ||
		!fullSHA.MatchString(policy.SourceSHA) ||
		!fullSHA.MatchString(policy.BaseSHA) ||
		policy.MaxPages <= 0 ||
		policy.MaxPages > 10000 ||
		policy.MaxWrites <= 0 ||
		policy.MaxWrites > 1000 ||
		len(policy.ReproductionCommand) == 0 ||
		len(policy.ReproductionCommand) > 32 ||
		!filepath.IsAbs(policy.ReproductionCommand[0]) ||
		policy.ExpectedExitCode <= 0 ||
		!fullDigest.MatchString(policy.ExpectedOutputSHA256) {
		return errors.New("invalid issue-repair policy")
	}
	totalCommandBytes := 0
	for _, argument := range policy.ReproductionCommand {
		if argument == "" || len(argument) > 4096 {
			return errors.New("invalid issue-repair reproduction command")
		}
		totalCommandBytes += len(argument)
	}
	if totalCommandBytes > 8192 {
		return errors.New("issue-repair reproduction command is too large")
	}
	return nil
}

func policyDigest(policy Policy) string {
	body, err := json.Marshal(policy)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func policyFromState(state State) Policy {
	return Policy{
		Repository:           state.Repository,
		SourceSHA:            state.SourceSHA,
		BaseSHA:              state.BaseSHA,
		WorkspaceRoot:        state.WorkspaceRoot,
		MaxPages:             state.Budget.MaxPages,
		MaxWrites:            state.Budget.MaxWrites,
		PushAuthorized:       state.PushAuthorized,
		PRAuthorized:         state.PRAuthorized,
		ReproductionCommand:  append([]string(nil), state.ReproductionCommand...),
		ExpectedExitCode:     state.ExpectedExitCode,
		ExpectedOutputSHA256: state.ExpectedOutputSHA256,
	}
}
