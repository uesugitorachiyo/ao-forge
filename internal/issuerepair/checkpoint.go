package issuerepair

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

func checkpointDigest(state State, integrityKey []byte) string {
	payload := struct {
		SchemaVersion        string        `json:"schema_version"`
		RunID                string        `json:"run_id"`
		Repository           string        `json:"repository"`
		SourceSHA            string        `json:"source_sha"`
		BaseSHA              string        `json:"base_sha"`
		WorkspaceRoot        string        `json:"workspace_root"`
		StateRoot            string        `json:"state_root"`
		Stage                string        `json:"stage"`
		NextPage             int           `json:"next_page"`
		CloneComplete        bool          `json:"clone_complete"`
		ReproductionFailed   bool          `json:"reproduction_failed"`
		PatchRecorded        bool          `json:"patch_recorded"`
		LedgerHead           string        `json:"ledger_head"`
		Budget               Budget        `json:"budget"`
		Lease                Lease         `json:"lease"`
		LeaseTTL             time.Duration `json:"lease_ttl"`
		PushAuthorized       bool          `json:"push_authorized"`
		PRAuthorized         bool          `json:"pr_authorized"`
		ReproductionCommand  []string      `json:"reproduction_command"`
		ExpectedExitCode     int           `json:"expected_exit_code"`
		ExpectedOutputSHA256 string        `json:"expected_output_sha256"`
		PolicyDigest         string        `json:"policy_digest"`
	}{
		SchemaVersion:        state.SchemaVersion,
		RunID:                state.RunID,
		Repository:           state.Repository,
		SourceSHA:            state.SourceSHA,
		BaseSHA:              state.BaseSHA,
		WorkspaceRoot:        state.WorkspaceRoot,
		StateRoot:            state.StateRoot,
		Stage:                state.Stage,
		NextPage:             state.NextPageValue,
		CloneComplete:        state.CloneComplete,
		ReproductionFailed:   state.ReproductionFailed,
		PatchRecorded:        state.PatchRecorded,
		LedgerHead:           state.Ledger.Head(),
		Budget:               state.Budget,
		Lease:                state.Lease,
		LeaseTTL:             state.LeaseTTL,
		PushAuthorized:       state.PushAuthorized,
		PRAuthorized:         state.PRAuthorized,
		ReproductionCommand:  state.ReproductionCommand,
		ExpectedExitCode:     state.ExpectedExitCode,
		ExpectedOutputSHA256: state.ExpectedOutputSHA256,
		PolicyDigest:         state.PolicyDigest,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal issue-repair checkpoint: %v", err))
	}
	mac := hmac.New(sha256.New, integrityKey)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func checkpointMatches(state State, integrityKey []byte) bool {
	expected := checkpointDigest(state, integrityKey)
	return hmac.Equal([]byte(state.CheckpointDigest), []byte(expected))
}
