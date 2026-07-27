package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPhase4RootCLIParity(t *testing.T) {
	var helpStdout, helpStderr bytes.Buffer
	if code := Run(nil, &helpStdout, &helpStderr); code != 0 {
		t.Fatalf("root help exit code = %d, want 0", code)
	}
	if helpStderr.Len() != 0 {
		t.Fatalf("root help stderr = %q, want empty", helpStderr.String())
	}
	helpSum := sha256.Sum256(helpStdout.Bytes())
	if got, want := hex.EncodeToString(helpSum[:]), "d3fa90a0c176fc4b04a169dffd6afd5781ffedff47a38952bceeab440f8a63f2"; got != want {
		t.Fatalf("root help bytes drifted: sha256 = %s, want %s", got, want)
	}

	oldVersion, oldSource := buildVersion, buildSourceCommit
	buildVersion, buildSourceCommit = "phase4-test", strings.Repeat("a", 40)
	t.Cleanup(func() {
		buildVersion, buildSourceCommit = oldVersion, oldSource
	})

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "version",
			args:       []string{"--version"},
			wantCode:   0,
			wantStdout: "ao-forge version=phase4-test source_sha=" + strings.Repeat("a", 40) + "\n",
		},
		{
			name:       "unknown command",
			args:       []string{"not-a-command"},
			wantCode:   2,
			wantStderr: "unknown command \"not-a-command\"\n\n" + helpStdout.String(),
		},
		{
			name:       "run missing arguments",
			args:       []string{"run"},
			wantCode:   2,
			wantStderr: "forge run: missing required --plan\n",
		},
		{
			name:       "once missing arguments",
			args:       []string{"once"},
			wantCode:   2,
			wantStderr: "forge once: missing required --brief\n",
		},
		{
			name:       "resume missing arguments",
			args:       []string{"resume"},
			wantCode:   2,
			wantStderr: "forge resume: missing required --run\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(test.args, &stdout, &stderr)
			if code != test.wantCode {
				t.Fatalf("exit code = %d, want %d", code, test.wantCode)
			}
			if stdout.String() != test.wantStdout {
				t.Fatalf("stdout drifted\nwant: %q\ngot:  %q", test.wantStdout, stdout.String())
			}
			if stderr.String() != test.wantStderr {
				t.Fatalf("stderr drifted\nwant: %q\ngot:  %q", test.wantStderr, stderr.String())
			}
		})
	}
}

func TestPhase4ResumeStateWorkspaceAndPlanOrderParity(t *testing.T) {
	tmpDir := t.TempDir()
	defaultWorkspace := filepath.Join(tmpDir, "default")
	overrideWorkspace := filepath.Join(tmpDir, "override")
	for _, dir := range []string{defaultWorkspace, overrideWorkspace} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create workspace %s: %v", dir, err)
		}
	}

	tracePath := filepath.Join(tmpDir, "trace.log")
	ao2Path := compileTestAo2(t, tmpDir, tracePath)
	plan := factoryPlan{
		PlanID: "forge-plan-phase4state",
		Objective: factoryObjective{
			Text:      "characterize resumed orchestration",
			Workspace: defaultWorkspace,
		},
		Workcells: []planWorkcell{
			{
				WorkcellID: "already-passed",
				Kind:       "prepare",
				Executor:   "agy-swarms",
				Peers:      2,
				MaxRepairs: 3,
				Task:       "retain state",
				Status:     "planned",
			},
			{
				WorkcellID: "override",
				Kind:       "execute",
				Workspace:  overrideWorkspace,
				Status:     "planned",
				DependsOn:  []string{"already-passed"},
			},
			{
				WorkcellID: "fallback",
				Kind:       "verify",
				Status:     "planned",
				DependsOn:  []string{"override"},
			},
		},
	}

	retainedPeers := []*peerRunState{
		{
			stateMu: &sync.Mutex{},
			Index:   0,
			Status:  "passed",
			Stdout:  "peer zero stdout",
			Stderr:  "peer zero stderr",
			Summary: "peer zero summary",
			Cost:    1.25,
			Tokens:  17,
		},
		{
			stateMu: &sync.Mutex{},
			Index:   1,
			Status:  "passed",
			Stdout:  "peer one stdout",
			Stderr:  "peer one stderr",
			Summary: "peer one summary",
			Cost:    2.5,
			Tokens:  23,
		},
	}
	prevStates := map[string]*workcellRunState{
		"already-passed": {
			ID:               "already-passed",
			Status:           "passed",
			Summary:          "retained summary",
			Stdout:           "retained stdout",
			Stderr:           "retained stderr",
			SpecSHA256:       strings.Repeat("b", 64),
			RepairsAttempted: 2,
			PeerStates:       retainedPeers,
		},
	}

	var stdout, stderr bytes.Buffer
	states, err := runWorkcellsConcurrent(
		context.Background(),
		plan,
		ao2Path,
		&stdout,
		&stderr,
		false,
		true,
		true,
		strings.NewReader(""),
		prevStates,
	)
	if err != nil {
		t.Fatalf("run resumed workcells: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if len(states) != len(plan.Workcells) {
		t.Fatalf("state count = %d, want %d", len(states), len(plan.Workcells))
	}
	for index, wantID := range []string{"already-passed", "override", "fallback"} {
		if states[index].ID != wantID {
			t.Fatalf("state %d ID = %q, want %q", index, states[index].ID, wantID)
		}
		if states[index].Status != "passed" {
			t.Fatalf("state %s status = %q, want passed", wantID, states[index].Status)
		}
	}

	retained := states[0]
	if retained.Summary != "retained summary" ||
		retained.Stdout != "retained stdout" ||
		retained.Stderr != "retained stderr" ||
		retained.SpecSHA256 != strings.Repeat("b", 64) ||
		retained.RepairsAttempted != 2 {
		t.Fatalf("resumed state drifted: %+v", retained)
	}
	if len(retained.PeerStates) != 2 {
		t.Fatalf("peer state count = %d, want 2", len(retained.PeerStates))
	}
	for index, want := range retainedPeers {
		got := retained.PeerStates[index]
		if got.Index != want.Index ||
			got.Status != want.Status ||
			got.Stdout != want.Stdout ||
			got.Stderr != want.Stderr ||
			got.Summary != want.Summary ||
			got.Cost != want.Cost ||
			got.Tokens != want.Tokens {
			t.Fatalf("peer state %d drifted: got %+v, want %+v", index, got, want)
		}
	}

	traceData, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read execution trace: %v", err)
	}
	wantTrace := fmt.Sprintf("override:%s\nfallback:%s", overrideWorkspace, defaultWorkspace)
	if got := strings.TrimSpace(string(traceData)); got != wantTrace {
		t.Fatalf("unfinished-only/workspace trace drifted\nwant:\n%s\ngot:\n%s", wantTrace, got)
	}
}

func TestPhase4SchedulerCancelsInflightSiblingParity(t *testing.T) {
	tmpDir := t.TempDir()
	markerPath := filepath.Join(tmpDir, "slow-started")
	ao2Path := compileCancellationAo2(t, tmpDir, markerPath)
	plan := factoryPlan{
		PlanID: "forge-plan-phase4cancel",
		Objective: factoryObjective{
			Text:      "characterize sibling cancellation",
			Workspace: tmpDir,
		},
		Workcells: []planWorkcell{
			{WorkcellID: "fail", Kind: "prepare", Status: "planned"},
			{WorkcellID: "slow", Kind: "execute", Status: "planned"},
		},
	}

	startedAt := time.Now()
	states, err := runWorkcellsConcurrent(
		context.Background(),
		plan,
		ao2Path,
		&bytes.Buffer{},
		&bytes.Buffer{},
		false,
		true,
		true,
		strings.NewReader(""),
		nil,
	)
	elapsed := time.Since(startedAt)
	if err == nil {
		t.Fatal("scheduler error = nil, want first workcell failure")
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("in-flight sibling was not cancelled promptly: elapsed %s", elapsed)
	}
	if _, statErr := os.Stat(markerPath); statErr != nil {
		t.Fatalf("slow sibling did not start before cancellation: %v", statErr)
	}
	if len(states) != 2 || states[0].ID != "fail" || states[1].ID != "slow" {
		t.Fatalf("scheduler result order drifted: %+v", states)
	}
	if states[0].Status != "failed" || states[1].Status != "failed" {
		t.Fatalf("cancellation statuses drifted: fail=%q slow=%q", states[0].Status, states[1].Status)
	}
}

func compileCancellationAo2(t *testing.T, tmpDir string, markerPath string) string {
	t.Helper()
	sourcePath := filepath.Join(tmpDir, "cancellation_ao2.go")
	binaryPath := filepath.Join(tmpDir, "cancellation_ao2")
	if os.PathSeparator == '\\' {
		binaryPath += ".exe"
	}
	source := fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type runSpec struct {
	Spec struct {
		Tasks []struct {
			ID string `+"`json:\"id\"`"+`
		} `+"`json:\"tasks\"`"+`
	} `+"`json:\"spec\"`"+`
}

func main() {
	var specPath string
	for index, arg := range os.Args {
		if arg == "--spec" && index+1 < len(os.Args) {
			specPath = os.Args[index+1]
		}
	}
	data, _ := os.ReadFile(specPath)
	var spec runSpec
	_ = json.Unmarshal(data, &spec)
	if len(spec.Spec.Tasks) == 0 {
		os.Exit(2)
	}
	switch spec.Spec.Tasks[0].ID {
	case "slow":
		_ = os.WriteFile(%q, []byte("started"), 0o644)
		time.Sleep(10 * time.Second)
	case "fail":
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(%q); err == nil {
				fmt.Fprintln(os.Stderr, "intentional failure")
				os.Exit(1)
			}
			time.Sleep(10 * time.Millisecond)
		}
		os.Exit(3)
	}
	fmt.Println("status=dry_run_accepted")
	fmt.Println("schema_version=ao2.run/v1")
	fmt.Println("plan_id=forge-plan-phase4cancel")
}
`, markerPath, markerPath)
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write cancellation ao2 source: %v", err)
	}
	command := exec.Command("go", "build", "-o", binaryPath, sourcePath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build cancellation ao2: %v\n%s", err, output)
	}
	return binaryPath
}
