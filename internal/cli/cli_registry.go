package cli

import (
	"fmt"
	"io"
)

// Run executes the AO Forge CLI and returns a process-style exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printHelp(stdout)
		return 0
	}
	if args[0] == "--version" {
		fmt.Fprintf(stdout, "ao-forge version=%s source_sha=%s\n", buildVersion, buildSourceCommit)
		return 0
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "plan":
		return runPlan(args[1:], stdout, stderr)
	case "gate":
		return runGate(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "artifact":
		return runArtifact(args[1:], stdout, stderr)
	case "release-candidate":
		return runReleaseCandidate(args[1:], stdout, stderr)
	case "release-preview":
		return runReleasePreview(args[1:], stdout, stderr)
	case "production-readiness":
		return runProductionReadiness(args[1:], stdout, stderr)
	case "contract":
		return runContract(args[1:], stdout, stderr)
	case "goal":
		return runGoal(args[1:], stdout, stderr)
	case "live-docs":
		return runLiveDocs(args[1:], stdout, stderr)
	case "run":
		return runRun(args[1:], stdout, stderr)
	case "once":
		return runOnce(args[1:], stdout, stderr)
	case "packet":
		return runPacketCommand(args[1:], stdout, stderr)
	case "resume":
		return runResume(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printHelp(stderr)
		return 2
	}
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `AO Forge

Usage:
  forge --help
  forge --version
  forge init
  forge plan --brief <factory-brief.json> [--out <factory-plan.json>] [--dynamic]
  forge gate --plan <factory-plan.json> --covenant <path-to-covenant-or-config> [--out <gate-result.json>]
  forge run --plan <factory-plan.json> --gate-result <gate-result.json> [--out <factory-packet.json>] [--live] [--confirm-release] [--release-preview-audit <release-preview-audit.json>]
  forge once --brief <factory-brief.json> --covenant <path-to-covenant-or-config> [--out <factory-packet.json>] [--live] [--confirm-release] [--release-preview-audit <release-preview-audit.json>]
  forge packet --run <run-id> [--out <factory-packet.json>]
  forge resume --run <run-id> [--out <factory-packet.json>] [--live] [--confirm-release] [--release-preview-audit <release-preview-audit.json>]
  forge inspect --packet <factory-packet.json>
  forge doctor --foundation <foundation-baseline.json> [--json]
  forge artifact checksums --artifact <path> [--artifact <path> ...] [--out <checksums.txt>]
  forge artifact verify-checksums --manifest <checksums.txt>
  forge release-candidate validate --candidate <release-candidate.json>
  forge release-preview --workspace <git-workspace> [--tag <vX.Y.Z>] [--artifact <path> ...] [--out <release-preview-audit.json>]
  forge release-preview inspect --audit <release-preview-audit.json> [--json]
  forge production-readiness audit [--json]
  forge contract validate --schema <schema.json> --document <document.json> [--json]
  forge live-docs guard --plan <dry-run-plan.json> --approval-gate <approval-gate.json> --ticket <ticket.json> --sentinel <sentinel.json> --command-readback <command-readback.json> --out <guard.json>
  forge goal validate --goal-run <goal-run.json> [--json]
  forge goal inspect --goal-run <goal-run.json> [--json]
  forge goal transitions --goal-run <goal-run.json> [--to <phase>] [--json]
  forge goal readiness --goal-run <goal-run.json> [--to <phase>] [--now <RFC3339>] [--json]
  forge goal context validate --goal-run <goal-run.json> --handoff <context-handoff.json> [--now <RFC3339>] [--json]
  forge goal verification validate --verification <goal-run-verification.json> [--json]
  forge goal update --goal-run <goal-run.json> --out <goal-run.json> [--phase <phase>] [--next-task <text>] [--last-verified-at <RFC3339>] [--last-iteration-status <status>] [--last-iteration-summary <text>] [--evidence <path> ...] [--json]
  forge goal evidence verify --goal-run <goal-run.json> [--json]
  forge goal evidence lint --goal-run <goal-run.json> [--json]
  forge goal evidence lint --update-audit <goal-run-update-audit.json> [--json]
  forge goal evidence retention --artifact <retained-evidence.json> [--now <RFC3339>] [--json]
  forge goal evidence cleanup --dry-run [--root <dir>] [--now <RFC3339>] [--json]

Factory terms:
  factory brief   normalized operator objective and constraints
  workcell        bounded unit of factory work with dependencies and evidence
  factory packet  operator-ready JSON summary of plan, gates, evidence, and next actions

Slice 2.8 status:
  durable state persistence, live/dry-run execution orchestration, verification, run resumption, multi-workspace orchestration, worker swarm compatibility, interactive operator overrides, real-time TUI dashboard, parallel swarm peer review, closed-loop repair compatibility, archived agy-swarms dynamic planning behind explicit opt-in, release mutation, GitHub publishing, release preview audits, release preview enforcement, release preview audit inspection, production-readiness scoring, artifact checksums, artifact checksum verification, contract schema validation, GoalRun validation and inspection, GoalRun transition checks, GoalRun context handoff validation, GoalRun verification evidence validation, guarded GoalRun updates, GoalRun update evidence attachments, GoalRun evidence verification, GoalRun evidence path linting, GoalRun retained evidence retention audits, and GoalRun readiness audits are enabled.
  Dynamic planning uses archived agy-swarms compatibility and requires AO_FORGE_ENABLE_ARCHIVED_AGY_SWARMS=1.
`)
}
