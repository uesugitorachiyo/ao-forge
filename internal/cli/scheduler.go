package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type realTimeWriter struct {
	appendFunc func(string)
}

func (w *realTimeWriter) Write(p []byte) (n int, err error) {
	w.appendFunc(string(p))
	return len(p), nil
}

func runWorkcellsConcurrent(
	ctx context.Context,
	plan factoryPlan,
	ao2Path string,
	stdout, stderr io.Writer,
	liveMode bool,
	nonInteractive bool,
	noDashboard bool,
	stdin io.Reader,
	prevStates map[string]*workcellRunState,
) ([]workcellRunState, error) {
	// Initialize state
	states := make(map[string]*workcellRunState)
	for _, wc := range plan.Workcells {
		status := "pending"
		var existingSummary, existingStdout, existingStderr, existingSpecSHA256 string
		var existingRepairsAttempted int
		if prevStates != nil {
			if prev, ok := prevStates[wc.WorkcellID]; ok {
				if prev.Status == "passed" {
					status = "passed"
					existingSummary = prev.Summary
					existingStdout = prev.Stdout
					existingStderr = prev.Stderr
					existingSpecSHA256 = prev.SpecSHA256
					existingRepairsAttempted = prev.RepairsAttempted
				}
			}
		}
		var peerStates []*peerRunState
		if prevStates != nil {
			if prev, ok := prevStates[wc.WorkcellID]; ok {
				peerStates = prev.PeerStates
			}
		}
		states[wc.WorkcellID] = &workcellRunState{
			stateMu:          &sync.Mutex{},
			ID:               wc.WorkcellID,
			Kind:             wc.Kind,
			Workspace:        wc.Workspace,
			Executor:         wc.Executor,
			Peers:            wc.Peers,
			MaxRepairs:       wc.MaxRepairs,
			RepairsAttempted: existingRepairsAttempted,
			Task:             wc.Task,
			DependsOn:        wc.DependsOn,
			Status:           status,
			Summary:          existingSummary,
			Stdout:           existingStdout,
			Stderr:           existingStderr,
			SpecSHA256:       existingSpecSHA256,
			Rubric:           wc.Rubric,
			PeerStates:       peerStates,
		}
	}

	var mu sync.Mutex
	// Use a WaitGroup to wait for all running goroutines to complete
	var wg sync.WaitGroup
	var promptMu sync.Mutex

	var tuiFd uintptr
	useDashboard := false
	if f, ok := stderr.(*os.File); ok && !nonInteractive && !noDashboard {
		if isTerminal(f.Fd()) {
			useDashboard = true
			tuiFd = f.Fd()
		}
	}

	if useDashboard {
		enterAlternateScreen(stderr)
		defer exitAlternateScreen(stderr)

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			exitAlternateScreen(stderr)
			os.Exit(130)
		}()
		defer signal.Stop(sigChan)

		d := &dashboard{
			plan:      plan,
			states:    states,
			mu:        &mu,
			startTime: time.Now(),
			writer:    stderr,
		}

		doneChan := make(chan struct{})
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					d.render(tuiFd)
				case <-doneChan:
					return
				}
			}
		}()
		defer func() {
			close(doneChan)
			d.render(tuiFd)
		}()
	}

	// Create a cancellable context to abort pending runs if one fails
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var firstErr error
	var errOnce sync.Once
	setFailure := func(err error) {
		errOnce.Do(func() {
			firstErr = err
			cancel() // cancel all other running/pending tasks
		})
	}

	// We run a loop until all tasks are either finished (passed/failed) or skipped
	for {
		// Select all ready tasks
		mu.Lock()
		var readyTasks []*workcellRunState
		allFinished := true
		for _, state := range states {
			if state.Status == "pending" {
				allFinished = false
				// Check if all dependencies have passed
				depsPassed := true
				for _, dep := range state.DependsOn {
					depState := states[dep]
					if depState == nil || depState.Status != "passed" {
						depsPassed = false
						// If any dependency has failed or is skipped, this task must be skipped
						if depState != nil && (depState.Status == "failed" || depState.Status == "skipped") {
							state.Status = "skipped"
							state.Summary = "Dependency failed or was skipped"
						}
						break
					}
				}
				if depsPassed {
					state.Status = "running"
					readyTasks = append(readyTasks, state)
				}
			} else if state.Status == "running" {
				allFinished = false
			}
		}
		mu.Unlock()

		if allFinished {
			break
		}

		if len(readyTasks) == 0 {
			// If nothing is ready but we aren't finished, check if the context is cancelled
			if ctx.Err() != nil {
				// Cancelled due to a failure, mark remaining pending as skipped
				mu.Lock()
				for _, state := range states {
					if state.Status == "pending" {
						state.Status = "skipped"
						state.Summary = "Run cancelled due to upstream failure"
					}
				}
				mu.Unlock()
				break
			}
			// Otherwise, wait a short duration and check status again
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Launch ready tasks
		for _, task := range readyTasks {
			wg.Add(1)
			go func(t *workcellRunState) {
				defer wg.Done()

				// Run task
				err := executeSingleWorkcell(ctx, plan, t, ao2Path, liveMode)

				if err != nil {
					if !nonInteractive {
						promptMu.Lock()

						// Check if context has been cancelled by another goroutine's abort action
						if ctx.Err() != nil {
							mu.Lock()
							t.Status = "skipped"
							t.Summary = "Cancelled during execution"
							mu.Unlock()
							promptMu.Unlock()
							return
						}

						fmt.Fprintf(stderr, "\nWorkcell [%s] failed.\nError: %v\n", t.ID, err)
						if t.Stdout != "" {
							fmt.Fprintf(stderr, "Stdout: %s\n", t.Stdout)
						}
						if t.Stderr != "" {
							fmt.Fprintf(stderr, "Stderr: %s\n", t.Stderr)
						}
						fmt.Fprintf(stderr, "Choose action: (r)etry, (s)kip and continue, or (a)bort? [r/s/A]: ")

						response, scanErr := readStdinLine(stdin)
						promptMu.Unlock()

						if scanErr == nil {
							respLower := strings.ToLower(strings.TrimSpace(response))
							if respLower == "r" || respLower == "retry" {
								// Loop to retry until success or abort/skip
								retryCount := 0
								for {
									retryCount++
									fmt.Fprintf(stderr, "\nRetrying workcell [%s] (attempt %d)...\n", t.ID, retryCount)
									err = executeSingleWorkcell(ctx, plan, t, ao2Path, liveMode)
									if err == nil {
										mu.Lock()
										t.Status = "passed"
										if t.Summary == "" {
											if liveMode {
												t.Summary = "Governed run started by ao2"
											} else {
												t.Summary = "Dry-run accepted by ao2"
											}
										}
										mu.Unlock()
										return
									}

									// If it fails again, lock promptMu to prompt again
									promptMu.Lock()
									if ctx.Err() != nil {
										mu.Lock()
										t.Status = "skipped"
										t.Summary = "Cancelled during execution"
										mu.Unlock()
										promptMu.Unlock()
										return
									}
									fmt.Fprintf(stderr, "\nWorkcell [%s] failed on retry %d.\nError: %v\n", t.ID, retryCount, err)
									if t.Stdout != "" {
										fmt.Fprintf(stderr, "Stdout: %s\n", t.Stdout)
									}
									if t.Stderr != "" {
										fmt.Fprintf(stderr, "Stderr: %s\n", t.Stderr)
									}
									fmt.Fprintf(stderr, "Choose action: (r)etry, (s)kip and continue, or (a)bort? [r/s/A]: ")
									response, scanErr = readStdinLine(stdin)
									promptMu.Unlock()

									if scanErr != nil {
										break
									}
									respLower = strings.ToLower(strings.TrimSpace(response))
									if respLower != "r" && respLower != "retry" {
										break
									}
								}

								// Process post-retry response (either skip or abort)
								respLower = strings.ToLower(strings.TrimSpace(response))
								if respLower == "s" || respLower == "skip" {
									mu.Lock()
									t.Status = "skipped"
									t.Summary = "Skipped by operator after failure: " + err.Error()
									mu.Unlock()
									return
								}
							} else if respLower == "s" || respLower == "skip" {
								mu.Lock()
								t.Status = "skipped"
								t.Summary = "Skipped by operator after failure: " + err.Error()
								mu.Unlock()
								return
							}
						}
					}

					mu.Lock()
					t.Status = "failed"
					t.Summary = err.Error()
					setFailure(err)
					mu.Unlock()
				} else {
					mu.Lock()
					// Check if context was cancelled while we were running
					if ctx.Err() != nil {
						t.Status = "skipped"
						t.Summary = "Cancelled during execution"
					} else {
						t.Status = "passed"
						if t.Summary == "" {
							if liveMode {
								t.Summary = "Governed run started by ao2"
							} else {
								t.Summary = "Dry-run accepted by ao2"
							}
						}
					}
					mu.Unlock()
				}
			}(task)
		}
	}

	wg.Wait()

	// Convert map back to ordered slice
	orderedStates := make([]workcellRunState, len(plan.Workcells))
	for i, wc := range plan.Workcells {
		orderedStates[i] = *states[wc.WorkcellID]
	}

	return orderedStates, firstErr
}
