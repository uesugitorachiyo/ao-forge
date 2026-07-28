package issuerepair

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type testExecution struct {
	execution *Execution
	request   Request
	now       time.Time
}

func TestIssueRepairReproductionProcess(t *testing.T) {
	for _, arg := range os.Args {
		switch arg {
		case "--issue-repair-fail":
			_, _ = fmt.Fprintln(os.Stderr, "authentic defect")
			os.Exit(23)
		case "--issue-repair-unrelated":
			_, _ = fmt.Fprintln(os.Stderr, "unrelated failure")
			os.Exit(23)
		case "--issue-repair-pass":
			return
		case "--issue-repair-sleep":
			time.Sleep(time.Second)
			return
		}
	}
}

func newTestRequest(t *testing.T) Request {
	t.Helper()
	root := t.TempDir()
	repositoryRoot := filepath.Join(root, "repository")
	if err := os.Mkdir(repositoryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, repositoryRoot, "init", "-q")
	runGit(t, repositoryRoot, "config", "user.name", "AO Forge Test")
	runGit(t, repositoryRoot, "config", "user.email", "forge-test@example.invalid")
	if err := os.WriteFile(filepath.Join(repositoryRoot, "seed.txt"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repositoryRoot, "add", "seed.txt")
	runGit(t, repositoryRoot, "commit", "-q", "-m", "seed")
	baseSHA := runGit(t, repositoryRoot, "rev-parse", "HEAD")
	expectedOutput := sha256.Sum256([]byte("authentic defect\n"))
	return Request{
		RunID:        "repair-run-1",
		StateRoot:    filepath.Join(root, "state"),
		WorkerID:     "worker-1",
		LeaseTTL:     10 * time.Minute,
		IntegrityKey: bytes.Repeat([]byte{0x42}, 32),
		Policy: Policy{
			Repository:           "uesugitorachiyo/ao2",
			SourceSHA:            baseSHA,
			BaseSHA:              baseSHA,
			WorkspaceRoot:        repositoryRoot,
			MaxPages:             2,
			MaxWrites:            3,
			ReproductionCommand:  []string{os.Args[0], "-test.run=TestIssueRepairReproductionProcess", "--", "--issue-repair-fail"},
			ExpectedExitCode:     23,
			ExpectedOutputSHA256: hex.EncodeToString(expectedOutput[:]),
		},
	}
}

func openTestExecution(t *testing.T) testExecution {
	t.Helper()
	request := newTestRequest(t)
	now := time.Now().UTC()
	execution, err := Open(request, now)
	if err != nil {
		t.Fatalf("open execution: %v", err)
	}
	return testExecution{execution: execution, request: request, now: now}
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(bytes.TrimSpace(output))
}

func resumeRequest(request Request, state State, workerID string) ResumeRequest {
	return ResumeRequest{
		RunID:              request.RunID,
		WorkerID:           workerID,
		LeaseTTL:           request.LeaseTTL,
		ExpectedLeaseToken: state.Lease.Token,
		IntegrityKey:       append([]byte(nil), request.IntegrityKey...),
		Policy:             request.Policy,
	}
}

func writePatch(t *testing.T, request Request) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(request.Policy.WorkspaceRoot, "repair.txt"), []byte("repair\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func rewriteState(t *testing.T, path string, mutate func(*State)) {
	t.Helper()
	state, err := readState(path)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&state)
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResumeContinuesInterruptedPaginationWithoutRepeatingCompletedPage(t *testing.T) {
	fixture := openTestExecution(t)
	if err := fixture.execution.RecordPage(1, "cursor-1"); err != nil {
		t.Fatalf("record first page: %v", err)
	}
	resumed, err := Resume(
		fixture.execution.StatePath(),
		resumeRequest(fixture.request, fixture.execution.State(), "worker-1"),
		fixture.now.Add(30*time.Second),
	)
	if err != nil {
		t.Fatalf("resume execution: %v", err)
	}
	if got := resumed.NextPage(); got != 2 {
		t.Fatalf("next page = %d, want 2", got)
	}
	if err := resumed.RecordPage(1, "cursor-1"); !errors.Is(err, ErrDuplicateEvent) {
		t.Fatalf("repeated completed page error = %v, want ErrDuplicateEvent", err)
	}
	if err := resumed.RecordPage(2, ""); err != nil {
		t.Fatalf("record second page: %v", err)
	}
}

func TestCloneBoundariesVerifyExactBaseAndCleanWorkspace(t *testing.T) {
	wrongBase := newTestRequest(t)
	wrongBase.Policy.BaseSHA = "3333333333333333333333333333333333333333"
	execution, err := Open(wrongBase, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := execution.RecordClone(); !errors.Is(err, ErrStaleBase) {
		t.Fatalf("wrong clone base error = %v, want ErrStaleBase", err)
	}

	fixture := openTestExecution(t)
	dirtyPath := filepath.Join(fixture.request.Policy.WorkspaceRoot, "dirty.txt")
	if err := os.WriteFile(dirtyPath, []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fixture.execution.RecordClone(); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("dirty clone error = %v, want ErrUnsafeWorkspace", err)
	}
	if err := os.Remove(dirtyPath); err != nil {
		t.Fatal(err)
	}
	if err := fixture.execution.RecordClone(); err != nil {
		t.Fatalf("record exact clean clone: %v", err)
	}
}

func TestPrePatchReproductionAndRealPatchAreMandatory(t *testing.T) {
	fixture := openTestExecution(t)
	if err := fixture.execution.RecordClone(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.execution.RecordPatch(); !errors.Is(err, ErrReproductionRequired) {
		t.Fatalf("patch before reproduction error = %v, want ErrReproductionRequired", err)
	}
	if err := fixture.execution.RecordReproduction(); err != nil {
		t.Fatalf("run failing reproduction: %v", err)
	}
	if err := fixture.execution.RecordPatch(); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("unchanged patch error = %v, want ErrUnsafeWorkspace", err)
	}
	writePatch(t, fixture.request)
	if err := fixture.execution.RecordPatch(); err != nil {
		t.Fatalf("record isolated patch: %v", err)
	}
}

func TestReproductionRejectsWorkspaceDirtiedAfterClone(t *testing.T) {
	fixture := openTestExecution(t)
	if err := fixture.execution.RecordClone(); err != nil {
		t.Fatal(err)
	}
	writePatch(t, fixture.request)
	if err := fixture.execution.RecordReproduction(); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("dirty reproduction error = %v, want ErrUnsafeWorkspace", err)
	}
}

func TestPassingPrePatchCommandIsRejected(t *testing.T) {
	request := newTestRequest(t)
	request.Policy.ReproductionCommand = []string{os.Args[0], "-test.run=TestIssueRepairReproductionProcess", "--", "--issue-repair-pass"}
	execution, err := Open(request, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := execution.RecordClone(); err != nil {
		t.Fatal(err)
	}
	if err := execution.RecordReproduction(); !errors.Is(err, ErrReproductionRequired) {
		t.Fatalf("passing reproduction error = %v, want ErrReproductionRequired", err)
	}
}

func TestUnrelatedFailingCommandIsRejected(t *testing.T) {
	request := newTestRequest(t)
	request.Policy.ReproductionCommand = []string{os.Args[0], "-test.run=TestIssueRepairReproductionProcess", "--", "--issue-repair-unrelated"}
	execution, err := Open(request, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := execution.RecordClone(); err != nil {
		t.Fatal(err)
	}
	if err := execution.RecordReproduction(); !errors.Is(err, ErrReproductionRequired) {
		t.Fatalf("unrelated failure error = %v, want ErrReproductionRequired", err)
	}
}

func TestBoundedCommandTimesOut(t *testing.T) {
	_, err := runBoundedCommand(
		t.TempDir(),
		10*time.Millisecond,
		os.Args[0],
		"-test.run=TestIssueRepairReproductionProcess",
		"--",
		"--issue-repair-sleep",
	)
	if !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("bounded command timeout error = %v, want ErrUnsafeWorkspace", err)
	}
}

func TestPushAndPRBoundariesRemainDeniedWithoutExplicitAuthority(t *testing.T) {
	fixture := openTestExecution(t)
	if err := fixture.execution.RecordPush(); !errors.Is(err, ErrPushDenied) {
		t.Fatalf("push error = %v, want ErrPushDenied", err)
	}
	if err := fixture.execution.RecordPR(); !errors.Is(err, ErrPRDenied) {
		t.Fatalf("PR error = %v, want ErrPRDenied", err)
	}
}

func TestResumeRejectsStalePolicyAndCheckpointMismatch(t *testing.T) {
	fixture := openTestExecution(t)
	stale := resumeRequest(fixture.request, fixture.execution.State(), "worker-1")
	stale.Policy.BaseSHA = "3333333333333333333333333333333333333333"
	if _, err := Resume(fixture.execution.StatePath(), stale, fixture.now.Add(time.Second)); !errors.Is(err, ErrStaleBase) {
		t.Fatalf("stale base error = %v, want ErrStaleBase", err)
	}

	rewriteState(t, fixture.execution.StatePath(), func(state *State) {
		state.CheckpointDigest = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	})
	if _, err := Resume(
		fixture.execution.StatePath(),
		resumeRequest(fixture.request, fixture.execution.State(), "worker-1"),
		fixture.now.Add(time.Second),
	); !errors.Is(err, ErrCheckpointMismatch) {
		t.Fatalf("checkpoint mismatch error = %v, want ErrCheckpointMismatch", err)
	}
}

func TestLedgerRejectsDuplicateWritesAndLiveLeaseConflicts(t *testing.T) {
	fixture := openTestExecution(t)
	if err := fixture.execution.RecordPage(1, "cursor-1"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.execution.RecordPage(1, "cursor-1"); !errors.Is(err, ErrDuplicateEvent) {
		t.Fatalf("duplicate page error = %v, want ErrDuplicateEvent", err)
	}
	if _, err := Resume(
		fixture.execution.StatePath(),
		resumeRequest(fixture.request, fixture.execution.State(), "worker-2"),
		fixture.now.Add(30*time.Second),
	); !errors.Is(err, ErrLeaseConflict) {
		t.Fatalf("live lease conflict error = %v, want ErrLeaseConflict", err)
	}
}

func TestBudgetExhaustionIsPersistedAndFailClosed(t *testing.T) {
	fixture := openTestExecution(t)
	if err := fixture.execution.RecordPage(1, "cursor-1"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.execution.RecordPage(2, "cursor-2"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.execution.RecordPage(3, ""); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("third page error = %v, want ErrBudgetExhausted", err)
	}
	if !fixture.execution.State().Budget.Exhausted {
		t.Fatal("budget exhaustion was not persisted")
	}
}

func TestResumeRebindsAuthorityAndBudgetToTrustedPolicy(t *testing.T) {
	fixture := openTestExecution(t)
	rewriteState(t, fixture.execution.StatePath(), func(state *State) {
		state.PushAuthorized = true
		state.PRAuthorized = true
		state.Budget.MaxPages = 99
		state.Budget.MaxWrites = 99
		state.PolicyDigest = policyDigest(policyFromState(*state))
		state.CheckpointDigest = checkpointDigest(*state, fixture.request.IntegrityKey)
	})
	if _, err := Resume(
		fixture.execution.StatePath(),
		resumeRequest(fixture.request, fixture.execution.State(), "worker-1"),
		fixture.now.Add(time.Second),
	); !errors.Is(err, ErrPolicyMismatch) {
		t.Fatalf("elevated policy error = %v, want ErrPolicyMismatch", err)
	}
}

func TestResumeRequiresExternalIntegrityKeyAndLeaseToken(t *testing.T) {
	fixture := openTestExecution(t)

	wrongKey := resumeRequest(fixture.request, fixture.execution.State(), "worker-1")
	wrongKey.IntegrityKey = bytes.Repeat([]byte{0x99}, 32)
	if _, err := Resume(
		fixture.execution.StatePath(),
		wrongKey,
		fixture.now.Add(time.Second),
	); !errors.Is(err, ErrCheckpointMismatch) {
		t.Fatalf("wrong integrity key error = %v, want ErrCheckpointMismatch", err)
	}

	wrongToken := resumeRequest(fixture.request, fixture.execution.State(), "worker-1")
	wrongToken.ExpectedLeaseToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := Resume(
		fixture.execution.StatePath(),
		wrongToken,
		fixture.now.Add(time.Second),
	); !errors.Is(err, ErrLeaseConflict) {
		t.Fatalf("wrong lease token error = %v, want ErrLeaseConflict", err)
	}
}

func TestExpiredLeaseRecoveryRevokesOldWorker(t *testing.T) {
	fixture := openTestExecution(t)
	expiredAt := fixture.execution.State().Lease.ExpiresAt.Add(time.Second)
	recovered, err := Resume(
		fixture.execution.StatePath(),
		resumeRequest(fixture.request, fixture.execution.State(), "worker-2"),
		expiredAt,
	)
	if err != nil {
		t.Fatalf("recover expired lease: %v", err)
	}
	if recovered.State().Lease.WorkerID != "worker-2" {
		t.Fatalf("recovered lease worker = %q, want worker-2", recovered.State().Lease.WorkerID)
	}
	if err := fixture.execution.RecordPage(1, "cursor"); !errors.Is(err, ErrLeaseConflict) {
		t.Fatalf("old worker mutation error = %v, want ErrLeaseConflict", err)
	}
}

func TestConcurrentExpiredLeaseRecoveryHasOneWinner(t *testing.T) {
	fixture := openTestExecution(t)
	resumeAt := fixture.execution.State().Lease.ExpiresAt.Add(time.Second)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, worker := range []string{"worker-2", "worker-3"} {
		wait.Add(1)
		go func(workerID string) {
			defer wait.Done()
			<-start
			_, err := Resume(
				fixture.execution.StatePath(),
				resumeRequest(fixture.request, fixture.execution.State(), workerID),
				resumeAt,
			)
			results <- err
		}(worker)
	}
	close(start)
	wait.Wait()
	close(results)
	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrLeaseConflict):
			conflicts++
		default:
			t.Fatalf("unexpected recovery error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("recoveries: successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
	}
}

func TestAuthorizedPushAndPRStillRequireCompletedRepair(t *testing.T) {
	request := newTestRequest(t)
	request.Policy.PushAuthorized = true
	request.Policy.PRAuthorized = true
	execution, err := Open(request, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := execution.RecordPush(); !errors.Is(err, ErrReproductionRequired) {
		t.Fatalf("early push error = %v, want ErrReproductionRequired", err)
	}
	if err := execution.RecordClone(); err != nil {
		t.Fatal(err)
	}
	if err := execution.RecordReproduction(); err != nil {
		t.Fatal(err)
	}
	writePatch(t, request)
	if err := execution.RecordPatch(); err != nil {
		t.Fatal(err)
	}
	if err := execution.RecordPR(); !errors.Is(err, ErrPushDenied) {
		t.Fatalf("early PR error = %v, want ErrPushDenied", err)
	}
	if err := execution.RecordPush(); err != nil {
		t.Fatalf("authorized push: %v", err)
	}
	if err := execution.RecordPR(); err != nil {
		t.Fatalf("authorized PR: %v", err)
	}
}

func TestMalformedAndUnsafeResumeFilesAreRejected(t *testing.T) {
	t.Run("trailing JSON", func(t *testing.T) {
		fixture := openTestExecution(t)
		body, err := os.ReadFile(fixture.execution.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		body = append(bytes.TrimSpace(body), []byte("\n{\"trailing\":true}\n")...)
		if err := os.WriteFile(fixture.execution.StatePath(), body, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Resume(
			fixture.execution.StatePath(),
			resumeRequest(fixture.request, fixture.execution.State(), "worker-1"),
			fixture.now.Add(time.Second),
		); !errors.Is(err, ErrCheckpointMismatch) {
			t.Fatalf("malformed resume error = %v, want ErrCheckpointMismatch", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		fixture := openTestExecution(t)
		target := fixture.execution.StatePath() + ".target"
		body, err := os.ReadFile(fixture.execution.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(fixture.execution.StatePath()); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, fixture.execution.StatePath()); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, err := Resume(
			fixture.execution.StatePath(),
			resumeRequest(fixture.request, fixture.execution.State(), "worker-1"),
			fixture.now.Add(time.Second),
		); !errors.Is(err, ErrCheckpointMismatch) {
			t.Fatalf("symlink resume error = %v, want ErrCheckpointMismatch", err)
		}
	})

	t.Run("oversized", func(t *testing.T) {
		fixture := openTestExecution(t)
		if err := os.Truncate(fixture.execution.StatePath(), maxStateBytes+1); err != nil {
			t.Fatal(err)
		}
		if _, err := Resume(
			fixture.execution.StatePath(),
			resumeRequest(fixture.request, fixture.execution.State(), "worker-1"),
			fixture.now.Add(time.Second),
		); !errors.Is(err, ErrCheckpointMismatch) {
			t.Fatalf("oversized resume error = %v, want ErrCheckpointMismatch", err)
		}
	})
}

func TestSemanticallyInvalidResumeStateIsRejectedAfterRehash(t *testing.T) {
	fixture := openTestExecution(t)
	rewriteState(t, fixture.execution.StatePath(), func(state *State) {
		state.NextPageValue = 9
		state.CheckpointDigest = checkpointDigest(*state, fixture.request.IntegrityKey)
	})
	if _, err := Resume(
		fixture.execution.StatePath(),
		resumeRequest(fixture.request, fixture.execution.State(), "worker-1"),
		fixture.now.Add(time.Second),
	); !errors.Is(err, ErrCheckpointMismatch) {
		t.Fatalf("semantic mismatch error = %v, want ErrCheckpointMismatch", err)
	}
}

func TestFailedPersistenceDoesNotAdvanceMemoryOrDurableState(t *testing.T) {
	fixture := openTestExecution(t)
	before := fixture.execution.State()
	realWriter := fixture.execution.writeState
	fixture.execution.writeState = func(Workspace, State, []byte) (State, error) {
		return State{}, errors.New("injected persistence failure")
	}
	if err := fixture.execution.RecordPage(1, "cursor"); err == nil {
		t.Fatal("record page succeeded during injected persistence failure")
	}
	if got := fixture.execution.State(); got.NextPageValue != before.NextPageValue || len(got.Ledger.Events) != len(before.Ledger.Events) {
		t.Fatalf("in-memory state advanced after failed persistence: before=%+v after=%+v", before, got)
	}
	fixture.execution.writeState = realWriter
	if err := fixture.execution.RecordPage(1, "cursor"); err != nil {
		t.Fatalf("retry page after persistence recovery: %v", err)
	}
}

func TestCompletedPhasesCannotBeReplayedOrRunOutOfOrder(t *testing.T) {
	fixture := openTestExecution(t)
	if err := fixture.execution.RecordClone(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.execution.RecordPage(1, "cursor"); !errors.Is(err, ErrCheckpointMismatch) {
		t.Fatalf("late pagination error = %v, want ErrCheckpointMismatch", err)
	}
	if err := fixture.execution.RecordReproduction(); err != nil {
		t.Fatal(err)
	}
	writePatch(t, fixture.request)
	if err := fixture.execution.RecordPatch(); err != nil {
		t.Fatal(err)
	}
	writesBefore := fixture.execution.State().Budget.WritesUsed
	if err := fixture.execution.RecordPatch(); !errors.Is(err, ErrDuplicateEvent) {
		t.Fatalf("duplicate patch error = %v, want ErrDuplicateEvent", err)
	}
	if got := fixture.execution.State().Budget.WritesUsed; got != writesBefore {
		t.Fatalf("duplicate patch consumed budget: got %d, want %d", got, writesBefore)
	}
}

func TestStateReturnsAnIndependentSnapshot(t *testing.T) {
	fixture := openTestExecution(t)
	snapshot := fixture.execution.State()
	snapshot.Ledger.Events[0].ID = "tampered"
	snapshot.ReproductionCommand[0] = "tampered"
	current := fixture.execution.State()
	if current.Ledger.Events[0].ID == "tampered" || current.ReproductionCommand[0] == "tampered" {
		t.Fatal("State exposed mutable execution internals")
	}
}

func ExampleExecution() {
	fmt.Println("issue-repair execution remains policy-bound and resumable")
	// Output: issue-repair execution remains policy-bound and resumable
}
