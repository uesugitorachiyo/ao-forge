package issuerepair

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const schemaVersion = "ao.forge.issue-repair-execution.v1"
const maxStateBytes = 1 << 20
const maxCommandOutputBytes = 64 << 10
const reproductionTimeout = 5 * time.Minute

var (
	ErrBudgetExhausted      = errors.New("issue-repair budget exhausted")
	ErrCheckpointMismatch   = errors.New("issue-repair checkpoint mismatch")
	ErrDuplicateEvent       = errors.New("duplicate issue-repair event")
	ErrLeaseConflict        = errors.New("issue-repair lease held by a live worker")
	ErrPolicyMismatch       = errors.New("issue-repair policy mismatch")
	ErrPRDenied             = errors.New("pull request boundary denied")
	ErrPushDenied           = errors.New("push boundary denied")
	ErrReproductionRequired = errors.New("failing pre-patch reproduction required")
	ErrStaleBase            = errors.New("issue-repair base is stale")
	ErrUnsafeWorkspace      = errors.New("issue-repair workspace is unsafe")
)

var (
	fullSHA    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	fullDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
	leaseToken = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type Request struct {
	RunID        string
	StateRoot    string
	WorkerID     string
	LeaseTTL     time.Duration
	IntegrityKey []byte
	Policy       Policy
}

type ResumeRequest struct {
	RunID              string
	WorkerID           string
	LeaseTTL           time.Duration
	ExpectedLeaseToken string
	IntegrityKey       []byte
	Policy             Policy
}

type State struct {
	SchemaVersion        string        `json:"schema_version"`
	RunID                string        `json:"run_id"`
	Repository           string        `json:"repository"`
	SourceSHA            string        `json:"source_sha"`
	BaseSHA              string        `json:"base_sha"`
	WorkspaceRoot        string        `json:"workspace_root"`
	StateRoot            string        `json:"state_root"`
	Stage                string        `json:"stage"`
	NextPageValue        int           `json:"next_page"`
	CloneComplete        bool          `json:"clone_complete"`
	ReproductionFailed   bool          `json:"reproduction_failed"`
	PatchRecorded        bool          `json:"patch_recorded"`
	Ledger               Ledger        `json:"ledger"`
	Budget               Budget        `json:"budget"`
	Lease                Lease         `json:"lease"`
	LeaseTTL             time.Duration `json:"lease_ttl"`
	PushAuthorized       bool          `json:"push_authorized"`
	PRAuthorized         bool          `json:"pr_authorized"`
	ReproductionCommand  []string      `json:"reproduction_command"`
	ExpectedExitCode     int           `json:"expected_exit_code"`
	ExpectedOutputSHA256 string        `json:"expected_output_sha256"`
	PolicyDigest         string        `json:"policy_digest"`
	CheckpointDigest     string        `json:"checkpoint_digest"`
}

type stateWriter func(Workspace, State, []byte) (State, error)

type Execution struct {
	mu           sync.Mutex
	workspace    Workspace
	state        State
	policyDigest string
	integrityKey []byte
	clock        func() time.Time
	writeState   stateWriter
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	overflow bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	if buffer.buffer.Len()+len(data) > maxCommandOutputBytes {
		remaining := maxCommandOutputBytes - buffer.buffer.Len()
		if remaining > 0 {
			_, _ = buffer.buffer.Write(data[:remaining])
		}
		buffer.overflow = true
		return len(data), nil
	}
	return buffer.buffer.Write(data)
}

func (buffer *limitedBuffer) String() string {
	return buffer.buffer.String()
}

func Open(request Request, now time.Time) (*Execution, error) {
	if request.RunID == "" || request.WorkerID == "" || request.LeaseTTL <= 0 || len(request.IntegrityKey) < 32 {
		return nil, errors.New("issue-repair run, worker, and positive lease are required")
	}
	policy, err := normalizePolicy(request.Policy)
	if err != nil {
		return nil, err
	}
	workspace, err := openWorkspace(policy.WorkspaceRoot, request.StateRoot)
	if err != nil {
		return nil, err
	}
	lock, err := acquireStateLock(workspace.LockPath)
	if err != nil {
		return nil, fmt.Errorf("lock issue-repair state: %w", err)
	}
	defer lock.Close()
	if _, err := os.Lstat(workspace.StatePath); err == nil {
		return nil, fmt.Errorf("issue-repair state already exists: %w", ErrDuplicateEvent)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect issue-repair state: %w", err)
	}
	lease, err := newLease(request.WorkerID, now, request.LeaseTTL)
	if err != nil {
		return nil, err
	}
	state := State{
		SchemaVersion:        schemaVersion,
		RunID:                request.RunID,
		Repository:           policy.Repository,
		SourceSHA:            policy.SourceSHA,
		BaseSHA:              policy.BaseSHA,
		WorkspaceRoot:        policy.WorkspaceRoot,
		StateRoot:            workspace.StateRoot,
		Stage:                "created",
		NextPageValue:        1,
		Ledger:               Ledger{Events: []Event{}},
		Budget:               Budget{MaxPages: policy.MaxPages, MaxWrites: policy.MaxWrites},
		Lease:                lease,
		LeaseTTL:             request.LeaseTTL,
		PushAuthorized:       policy.PushAuthorized,
		PRAuthorized:         policy.PRAuthorized,
		ReproductionCommand:  append([]string(nil), policy.ReproductionCommand...),
		ExpectedExitCode:     policy.ExpectedExitCode,
		ExpectedOutputSHA256: policy.ExpectedOutputSHA256,
		PolicyDigest:         policyDigest(policy),
	}
	if err := state.Ledger.Append("run:created", "run_created", nil); err != nil {
		return nil, err
	}
	state, err = persistState(workspace, state, request.IntegrityKey)
	if err != nil {
		return nil, err
	}
	return &Execution{
		workspace:    workspace,
		state:        state,
		policyDigest: state.PolicyDigest,
		integrityKey: append([]byte(nil), request.IntegrityKey...),
		clock:        func() time.Time { return time.Now().UTC() },
		writeState:   persistState,
	}, nil
}

func Resume(path string, request ResumeRequest, now time.Time) (*Execution, error) {
	if request.RunID == "" ||
		request.WorkerID == "" ||
		request.LeaseTTL <= 0 ||
		!leaseToken.MatchString(request.ExpectedLeaseToken) ||
		len(request.IntegrityKey) < 32 {
		return nil, ErrPolicyMismatch
	}
	policy, err := normalizePolicy(request.Policy)
	if err != nil {
		return nil, err
	}
	state, err := readState(path)
	if err != nil {
		return nil, err
	}
	if state.RunID != request.RunID {
		return nil, ErrCheckpointMismatch
	}
	if state.BaseSHA != policy.BaseSHA {
		return nil, ErrStaleBase
	}
	if err := validateState(state); err != nil || !checkpointMatches(state, request.IntegrityKey) {
		return nil, ErrCheckpointMismatch
	}
	if state.LeaseTTL != request.LeaseTTL || state.Lease.Token != request.ExpectedLeaseToken {
		return nil, ErrLeaseConflict
	}
	expectedPolicyDigest := policyDigest(policy)
	if state.PolicyDigest != expectedPolicyDigest {
		return nil, ErrPolicyMismatch
	}
	expectedStateRoot, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(path)))
	if err != nil || state.StateRoot != expectedStateRoot {
		return nil, ErrCheckpointMismatch
	}
	workspace, err := openWorkspace(policy.WorkspaceRoot, state.StateRoot)
	if err != nil {
		return nil, err
	}
	if filepath.Clean(path) != filepath.Clean(workspace.StatePath) {
		return nil, ErrCheckpointMismatch
	}
	lock, err := acquireStateLock(workspace.LockPath)
	if err != nil {
		return nil, fmt.Errorf("lock issue-repair state: %w", err)
	}
	defer lock.Close()
	state, err = readState(path)
	if err != nil {
		return nil, err
	}
	if err := validateState(state); err != nil || !checkpointMatches(state, request.IntegrityKey) {
		return nil, ErrCheckpointMismatch
	}
	if state.PolicyDigest != expectedPolicyDigest {
		return nil, ErrPolicyMismatch
	}
	if state.LeaseTTL != request.LeaseTTL || state.Lease.Token != request.ExpectedLeaseToken {
		return nil, ErrLeaseConflict
	}
	lease, err := renewLease(state.Lease, request.WorkerID, now, state.LeaseTTL)
	if err != nil {
		return nil, err
	}
	state.Lease = lease
	state, err = persistState(workspace, state, request.IntegrityKey)
	if err != nil {
		return nil, err
	}
	return &Execution{
		workspace:    workspace,
		state:        state,
		policyDigest: expectedPolicyDigest,
		integrityKey: append([]byte(nil), request.IntegrityKey...),
		clock:        func() time.Time { return time.Now().UTC() },
		writeState:   persistState,
	}, nil
}

func (execution *Execution) State() State {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	return cloneState(execution.state)
}

func (execution *Execution) StatePath() string {
	return execution.workspace.StatePath
}

func (execution *Execution) NextPage() int {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	return execution.state.NextPageValue
}

func (execution *Execution) RecordPage(page int, cursor string) error {
	return execution.mutate(func(state *State) (bool, error) {
		if len(cursor) > 4096 {
			return false, ErrCheckpointMismatch
		}
		if page != state.NextPageValue {
			if page < state.NextPageValue {
				return false, ErrDuplicateEvent
			}
			return false, ErrCheckpointMismatch
		}
		if state.Stage != "created" && state.Stage != "discovery" {
			return false, ErrCheckpointMismatch
		}
		if err := state.Budget.consumePage(); err != nil {
			return true, err
		}
		state.NextPageValue++
		state.Stage = "discovery"
		return false, state.Ledger.Append("page:"+strconv.Itoa(page), "page_completed", map[string]string{
			"page":        strconv.Itoa(page),
			"next_cursor": cursor,
		})
	})
}

func (execution *Execution) RecordClone() error {
	return execution.mutate(func(state *State) (bool, error) {
		if state.Ledger.Has("clone:" + state.BaseSHA) {
			return false, ErrDuplicateEvent
		}
		if state.Stage != "created" && state.Stage != "discovery" {
			return false, ErrCheckpointMismatch
		}
		workspaceDigest, err := execution.workspace.verifyClone(state.BaseSHA)
		if err != nil {
			return false, err
		}
		state.CloneComplete = true
		state.Stage = "cloned"
		return false, state.Ledger.Append(
			"clone:"+state.BaseSHA,
			"clone_completed",
			map[string]string{"base_sha": state.BaseSHA, "workspace_sha256": workspaceDigest},
		)
	})
}

func (execution *Execution) RecordReproduction() error {
	return execution.mutate(func(state *State) (bool, error) {
		if state.Ledger.Has("reproduction:failed") {
			return false, ErrDuplicateEvent
		}
		if state.Stage != "cloned" || !state.CloneComplete {
			return false, ErrReproductionRequired
		}
		workspaceDigest, err := execution.workspace.verifyClone(state.BaseSHA)
		if err != nil {
			return false, err
		}
		exitCode, outputDigest, err := runReproduction(execution.workspace.Root, state.ReproductionCommand)
		if err != nil {
			return false, err
		}
		if exitCode != state.ExpectedExitCode || outputDigest != state.ExpectedOutputSHA256 {
			return false, ErrReproductionRequired
		}
		afterDigest, err := execution.workspace.verifyClone(state.BaseSHA)
		if err != nil || afterDigest != workspaceDigest {
			return false, ErrUnsafeWorkspace
		}
		state.ReproductionFailed = true
		state.Stage = "reproduced"
		return false, state.Ledger.Append("reproduction:failed", "pre_patch_failure_observed", map[string]string{
			"exit_code":        strconv.Itoa(exitCode),
			"output_sha256":    outputDigest,
			"workspace_sha256": workspaceDigest,
		})
	})
}

func (execution *Execution) RecordPatch() error {
	return execution.mutate(func(state *State) (bool, error) {
		if state.Ledger.Has("patch:recorded") {
			return false, ErrDuplicateEvent
		}
		if state.Stage != "reproduced" || !state.ReproductionFailed {
			return false, ErrReproductionRequired
		}
		statusDigest, err := execution.workspace.patchStatusDigest()
		if err != nil {
			return false, err
		}
		if err := state.Budget.consumeWrite(); err != nil {
			return true, err
		}
		state.PatchRecorded = true
		state.Stage = "patched"
		return false, state.Ledger.Append("patch:recorded", "patch_recorded", map[string]string{
			"status_sha256": statusDigest,
		})
	})
}

func (execution *Execution) RecordPush() error {
	return execution.mutate(func(state *State) (bool, error) {
		if state.Ledger.Has("push:recorded") {
			return false, ErrDuplicateEvent
		}
		if !state.PushAuthorized {
			return false, ErrPushDenied
		}
		if state.Stage != "patched" || !state.PatchRecorded {
			return false, ErrReproductionRequired
		}
		if err := state.Budget.consumeWrite(); err != nil {
			return true, err
		}
		state.Stage = "pushed"
		return false, state.Ledger.Append("push:recorded", "push_recorded", nil)
	})
}

func (execution *Execution) RecordPR() error {
	return execution.mutate(func(state *State) (bool, error) {
		if state.Ledger.Has("pr:recorded") {
			return false, ErrDuplicateEvent
		}
		if !state.PRAuthorized {
			return false, ErrPRDenied
		}
		if state.Stage != "pushed" {
			return false, ErrPushDenied
		}
		if err := state.Budget.consumeWrite(); err != nil {
			return true, err
		}
		state.Stage = "pr_opened"
		return false, state.Ledger.Append("pr:recorded", "pull_request_recorded", nil)
	})
}

func (execution *Execution) mutate(change func(*State) (persistOnError bool, err error)) error {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	lock, err := acquireStateLock(execution.workspace.LockPath)
	if err != nil {
		return fmt.Errorf("lock issue-repair state: %w", err)
	}
	defer lock.Close()
	durable, err := readState(execution.workspace.StatePath)
	if err != nil {
		return err
	}
	if err := validateState(durable); err != nil || !checkpointMatches(durable, execution.integrityKey) {
		return ErrCheckpointMismatch
	}
	if durable.PolicyDigest != execution.policyDigest {
		return ErrPolicyMismatch
	}
	if durable.Lease.Token != execution.state.Lease.Token ||
		durable.Lease.WorkerID != execution.state.Lease.WorkerID ||
		!execution.clock().Before(durable.Lease.ExpiresAt) {
		return ErrLeaseConflict
	}
	candidate := cloneState(durable)
	persistOnError, changeErr := change(&candidate)
	if changeErr != nil && !persistOnError {
		return changeErr
	}
	if !execution.clock().Before(candidate.Lease.ExpiresAt) {
		return ErrLeaseConflict
	}
	saved, persistErr := execution.writeState(execution.workspace, candidate, execution.integrityKey)
	if persistErr != nil {
		return persistErr
	}
	execution.state = saved
	return changeErr
}

func persistState(workspace Workspace, state State, integrityKey []byte) (State, error) {
	state.CheckpointDigest = checkpointDigest(state, integrityKey)
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return State{}, fmt.Errorf("marshal issue-repair state: %w", err)
	}
	body = append(body, '\n')
	if len(body) > maxStateBytes {
		return State{}, ErrBudgetExhausted
	}
	temporary, err := os.CreateTemp(filepath.Dir(workspace.StatePath), ".state-*.tmp")
	if err != nil {
		return State{}, fmt.Errorf("create issue-repair state temporary: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return State{}, fmt.Errorf("protect issue-repair state temporary: %w", err)
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return State{}, fmt.Errorf("write issue-repair state temporary: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return State{}, fmt.Errorf("sync issue-repair state temporary: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return State{}, fmt.Errorf("close issue-repair state temporary: %w", err)
	}
	if err := replaceStateFile(temporaryName, workspace.StatePath); err != nil {
		return State{}, fmt.Errorf("replace issue-repair state: %w", err)
	}
	return state, nil
}

func readState(path string) (State, error) {
	file, err := openStateFile(path)
	if err != nil {
		return State{}, ErrCheckpointMismatch
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxStateBytes {
		return State{}, ErrCheckpointMismatch
	}
	body, err := io.ReadAll(io.LimitReader(file, maxStateBytes+1))
	if err != nil || len(body) > maxStateBytes {
		return State{}, ErrCheckpointMismatch
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var state State
	if err := decoder.Decode(&state); err != nil {
		return State{}, ErrCheckpointMismatch
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return State{}, ErrCheckpointMismatch
	}
	return state, nil
}

func validateState(state State) error {
	if state.SchemaVersion != schemaVersion ||
		state.RunID == "" ||
		state.Repository == "" ||
		!fullSHA.MatchString(state.SourceSHA) ||
		!fullSHA.MatchString(state.BaseSHA) ||
		!filepath.IsAbs(state.WorkspaceRoot) ||
		!filepath.IsAbs(state.StateRoot) ||
		state.LeaseTTL <= 0 ||
		state.Budget.MaxPages <= 0 ||
		state.Budget.MaxWrites <= 0 ||
		state.Budget.PagesUsed < 0 ||
		state.Budget.PagesUsed > state.Budget.MaxPages ||
		state.Budget.WritesUsed < 0 ||
		state.Budget.WritesUsed > state.Budget.MaxWrites ||
		state.Lease.WorkerID == "" ||
		!leaseToken.MatchString(state.Lease.Token) ||
		!state.Lease.IssuedAt.Before(state.Lease.ExpiresAt) ||
		state.Lease.ExpiresAt.Sub(state.Lease.IssuedAt) != state.LeaseTTL ||
		!fullDigest.MatchString(state.PolicyDigest) ||
		state.PolicyDigest != policyDigest(policyFromState(state)) {
		return ErrCheckpointMismatch
	}
	if err := validatePolicy(policyFromState(state)); err != nil {
		return ErrCheckpointMismatch
	}
	if err := state.Ledger.Validate(); err != nil || len(state.Ledger.Events) == 0 {
		return ErrCheckpointMismatch
	}

	var pages, writes int
	var cloneComplete, reproductionFailed, patchRecorded, pushed bool
	expectedStage := ""
	for index, event := range state.Ledger.Events {
		switch event.Kind {
		case "run_created":
			if index != 0 || event.ID != "run:created" || len(event.Fields) != 0 {
				return ErrCheckpointMismatch
			}
			expectedStage = "created"
		case "page_completed":
			if cloneComplete {
				return ErrCheckpointMismatch
			}
			pages++
			if event.ID != "page:"+strconv.Itoa(pages) ||
				len(event.Fields) != 2 ||
				event.Fields["page"] != strconv.Itoa(pages) {
				return ErrCheckpointMismatch
			}
			expectedStage = "discovery"
		case "clone_completed":
			if cloneComplete ||
				event.ID != "clone:"+state.BaseSHA ||
				len(event.Fields) != 2 ||
				event.Fields["base_sha"] != state.BaseSHA ||
				!fullDigest.MatchString(event.Fields["workspace_sha256"]) {
				return ErrCheckpointMismatch
			}
			cloneComplete = true
			expectedStage = "cloned"
		case "pre_patch_failure_observed":
			if !cloneComplete ||
				reproductionFailed ||
				event.ID != "reproduction:failed" ||
				len(event.Fields) != 3 ||
				event.Fields["exit_code"] != strconv.Itoa(state.ExpectedExitCode) ||
				event.Fields["output_sha256"] != state.ExpectedOutputSHA256 ||
				!fullDigest.MatchString(event.Fields["workspace_sha256"]) {
				return ErrCheckpointMismatch
			}
			reproductionFailed = true
			expectedStage = "reproduced"
		case "patch_recorded":
			if !reproductionFailed ||
				patchRecorded ||
				event.ID != "patch:recorded" ||
				len(event.Fields) != 1 ||
				!fullDigest.MatchString(event.Fields["status_sha256"]) {
				return ErrCheckpointMismatch
			}
			patchRecorded = true
			writes++
			expectedStage = "patched"
		case "push_recorded":
			if !patchRecorded || pushed || !state.PushAuthorized || event.ID != "push:recorded" || len(event.Fields) != 0 {
				return ErrCheckpointMismatch
			}
			pushed = true
			writes++
			expectedStage = "pushed"
		case "pull_request_recorded":
			if !pushed || !state.PRAuthorized || event.ID != "pr:recorded" || len(event.Fields) != 0 {
				return ErrCheckpointMismatch
			}
			writes++
			expectedStage = "pr_opened"
		default:
			return ErrCheckpointMismatch
		}
	}
	if pages != state.Budget.PagesUsed ||
		state.NextPageValue != pages+1 ||
		writes != state.Budget.WritesUsed ||
		cloneComplete != state.CloneComplete ||
		reproductionFailed != state.ReproductionFailed ||
		patchRecorded != state.PatchRecorded ||
		expectedStage != state.Stage {
		return ErrCheckpointMismatch
	}
	return nil
}

func normalizePolicy(policy Policy) (Policy, error) {
	if err := validatePolicy(policy); err != nil {
		return Policy{}, err
	}
	root, err := existingCanonicalDirectory(policy.WorkspaceRoot)
	if err != nil {
		return Policy{}, err
	}
	policy.WorkspaceRoot = root
	return policy, nil
}

func cloneState(state State) State {
	state.ReproductionCommand = append([]string(nil), state.ReproductionCommand...)
	state.Ledger.Events = append([]Event(nil), state.Ledger.Events...)
	for index := range state.Ledger.Events {
		if state.Ledger.Events[index].Fields != nil {
			fields := make(map[string]string, len(state.Ledger.Events[index].Fields))
			for key, value := range state.Ledger.Events[index].Fields {
				fields[key] = value
			}
			state.Ledger.Events[index].Fields = fields
		}
	}
	return state
}

func runReproduction(root string, command []string) (int, string, error) {
	context, cancel := context.WithTimeout(context.Background(), reproductionTimeout)
	defer cancel()
	process := exec.CommandContext(context, command[0], command[1:]...)
	process.Dir = root
	var output limitedBuffer
	process.Stdout = &output
	process.Stderr = &output
	err := process.Run()
	if context.Err() != nil || output.overflow {
		return 0, "", ErrReproductionRequired
	}
	if err == nil {
		return 0, "", ErrReproductionRequired
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() <= 0 {
		return 0, "", ErrReproductionRequired
	}
	sum := sha256.Sum256([]byte(output.String()))
	return exitError.ExitCode(), hex.EncodeToString(sum[:]), nil
}
