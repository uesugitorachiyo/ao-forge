package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type dashboard struct {
	plan      factoryPlan
	states    map[string]*workcellRunState
	mu        *sync.Mutex
	startTime time.Time
	writer    io.Writer
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func makeLine(char string, length int) string {
	if length <= 0 {
		return ""
	}
	res := make([]byte, length)
	for i := 0; i < length; i++ {
		res[i] = char[0]
	}
	return string(res)
}

func getTailLines(s string, count int) []string {
	lines := strings.Split(s, "\n")
	var nonEv []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			nonEv = append(nonEv, trimmed)
		}
	}
	if len(nonEv) <= count {
		return nonEv
	}
	return nonEv[len(nonEv)-count:]
}

func (d *dashboard) render(fd uintptr) {
	width, _, err := getTerminalSize(fd)
	if err != nil {
		width = 80
	}

	// Move cursor to top-left
	fmt.Fprintf(d.writer, "\033[H")

	// Print Header
	elapsed := time.Since(d.startTime).Round(time.Second)
	fmt.Fprintf(d.writer, "\033[1;36m[AO Forge Factory Dashboard]\033[0m | Plan: %s | Elapsed: %v\033[K\n", d.plan.PlanID, elapsed)
	fmt.Fprintf(d.writer, "Objective: %s\033[K\n", truncateString(d.plan.Objective.Text, width-12))
	fmt.Fprintf(d.writer, "%s\033[K\n", makeLine("-", width))

	d.mu.Lock()
	defer d.mu.Unlock()

	activeTasks := 0
	accumulatedCost := 0.0
	accumulatedTokens := 0.0

	// Gather metrics
	for _, state := range d.states {
		if state.GetStatus() == "running" {
			activeTasks++
		}
		if state.Peers > 1 && len(state.PeerStates) > 0 {
			for _, pState := range state.PeerStates {
				pState.stateMu.Lock()
				accumulatedCost += pState.Cost
				accumulatedTokens += pState.Tokens
				pState.stateMu.Unlock()
			}
		} else {
			// Try to parse agy-swarms summary token and cost info
			// Swarm execution succeeded (Tokens: 123, Cost: $0.45)
			var tokens, cost float64
			if _, err := fmt.Sscanf(state.Summary, "Swarm execution succeeded (Tokens: %f, Cost: $%f)", &tokens, &cost); err == nil {
				accumulatedCost += cost
				accumulatedTokens += tokens
			} else if _, err := fmt.Sscanf(state.Summary, "Swarm execution failed (Tokens: %f, Cost: $%f)", &tokens, &cost); err == nil {
				accumulatedCost += cost
				accumulatedTokens += tokens
			}
		}
	}

	fmt.Fprintf(d.writer, "Active Tasks: %d | Tokens: %.0f | Est. Cost: $%.2f\033[K\n", activeTasks, accumulatedTokens, accumulatedCost)
	fmt.Fprintf(d.writer, "%s\033[K\n", makeLine("-", width))

	// Print Workcells status list
	for _, wc := range d.plan.Workcells {
		state := d.states[wc.WorkcellID]
		status := state.GetStatus()
		statusColor := "\033[90m" // Gray
		switch status {
		case "running":
			statusColor = "\033[36m" // Cyan
		case "passed":
			statusColor = "\033[32m" // Green
		case "failed":
			statusColor = "\033[31m" // Red
		case "skipped":
			statusColor = "\033[33m" // Yellow
		}

		fmt.Fprintf(d.writer, "- %s (%s) -> %s%s\033[0m", wc.WorkcellID, wc.Kind, statusColor, strings.ToUpper(status))
		if status == "running" {
			if state.RepairsAttempted > 0 {
				fmt.Fprintf(d.writer, " (REPAIRING (Attempt %d/%d))", state.RepairsAttempted, state.MaxRepairs)
			} else if state.Peers > 1 {
				fmt.Fprintf(d.writer, " (running %d peers...)", state.Peers)
			} else {
				fmt.Fprintf(d.writer, " (running...)")
			}
		} else if state.Summary != "" {
			fmt.Fprintf(d.writer, " - %s", state.Summary)
		}
		fmt.Fprintf(d.writer, "\033[K\n")

		// Tail logs for running workcells
		if status == "running" {
			if state.Peers > 1 && len(state.PeerStates) > 0 {
				for _, pState := range state.PeerStates {
					pState.stateMu.Lock()
					pStatus := pState.Status
					pStdout := pState.Stdout
					pStderr := pState.Stderr
					pState.stateMu.Unlock()

					pColor := "\033[90m" // Gray
					switch pStatus {
					case "running":
						pColor = "\033[36m" // Cyan
					case "passed":
						pColor = "\033[32m" // Green
					case "failed":
						pColor = "\033[31m" // Red
					}
					fmt.Fprintf(d.writer, "    Peer %d: %s%s\033[0m\033[K\n", pState.Index, pColor, strings.ToUpper(pStatus))
					if pStatus == "running" {
						stdoutTail := getTailLines(pStdout, 1)
						stderrTail := getTailLines(pStderr, 1)
						for _, line := range stdoutTail {
							fmt.Fprintf(d.writer, "      \033[90m> %s\033[0m\033[K\n", truncateString(line, width-12))
						}
						for _, line := range stderrTail {
							fmt.Fprintf(d.writer, "      \033[31m! %s\033[0m\033[K\n", truncateString(line, width-12))
						}
					}
				}
			} else {
				stdoutTail := getTailLines(state.GetStdout(), 2)
				stderrTail := getTailLines(state.GetStderr(), 2)
				for _, line := range stdoutTail {
					fmt.Fprintf(d.writer, "    \033[90m> %s\033[0m\033[K\n", truncateString(line, width-10))
				}
				for _, line := range stderrTail {
					fmt.Fprintf(d.writer, "    \033[31m! %s\033[0m\033[K\n", truncateString(line, width-10))
				}
			}
		}
	}

	// Footer and clean up leftover screen lines
	fmt.Fprintf(d.writer, "\033[J")
}

func enterAlternateScreen(w io.Writer) {
	fmt.Fprint(w, "\033[?1049h\033[H\033[?25l")
}

func exitAlternateScreen(w io.Writer) {
	fmt.Fprint(w, "\033[?1049l\033[?25h")
}
