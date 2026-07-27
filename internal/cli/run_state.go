package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type peerRunState struct {
	stateMu *sync.Mutex
	Index   int
	Status  string // "pending", "running", "passed", "failed"
	Stdout  string
	Stderr  string
	Summary string
	Cost    float64
	Tokens  float64
}

type workcellRunState struct {
	stateMu          *sync.Mutex
	ID               string
	Kind             string
	Workspace        string
	Executor         string
	Peers            int
	MaxRepairs       int
	Task             string
	DependsOn        []string
	Status           string // "pending", "running", "passed", "failed", "skipped"
	Summary          string
	Stdout           string
	Stderr           string
	SpecSHA256       string
	Rubric           *workcellRubric
	PeerStates       []*peerRunState
	RepairsAttempted int
}

func (w *workcellRunState) GetStatus() string {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	return w.Status
}

func (w *workcellRunState) SetStatus(status string) {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Status = status
}

func (w *workcellRunState) GetSummary() string {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	return w.Summary
}

func (w *workcellRunState) SetSummary(sum string) {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Summary = sum
}

func (w *workcellRunState) AppendStdout(data string) {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Stdout += data
}

func (w *workcellRunState) AppendStderr(data string) {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Stderr += data
}

func (w *workcellRunState) GetStdout() string {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	return w.Stdout
}

func (w *workcellRunState) GetStderr() string {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	return w.Stderr
}

func (w *workcellRunState) ResetOutputs() {
	if w.stateMu != nil {
		w.stateMu.Lock()
		defer w.stateMu.Unlock()
	}
	w.Stdout = ""
	w.Stderr = ""
	w.Summary = ""
	w.SpecSHA256 = ""
}

func loadResumeStates(runDir string, packetPath string) map[string]*workcellRunState {
	prevStates := make(map[string]*workcellRunState)
	if _, err := os.Stat(packetPath); err != nil {
		return prevStates
	}

	packetData, err := os.ReadFile(packetPath)
	if err != nil {
		return prevStates
	}
	var prevPacket factoryPacket
	if err := json.Unmarshal(packetData, &prevPacket); err != nil {
		return prevStates
	}

	for _, prevWc := range prevPacket.Workcells {
		if prevWc.Status != "passed" {
			continue
		}
		wcEvPath := filepath.Join(runDir, fmt.Sprintf("ao2-wc-%s-evidence.json", prevWc.WorkcellID))
		var stdoutText, stderrText, specSHA string
		if evData, err := os.ReadFile(wcEvPath); err == nil {
			var evObj map[string]any
			if err := json.Unmarshal(evData, &evObj); err == nil {
				if st, ok := evObj["stdout"].(string); ok {
					stdoutText = st
				}
				if se, ok := evObj["stderr"].(string); ok {
					stderrText = se
				}
				if sh, ok := evObj["spec_sha256"].(string); ok {
					specSHA = sh
				}
			}
		}
		var peerStates []*peerRunState
		if prevWc.Peers > 1 {
			for idx := 0; idx < prevWc.Peers; idx++ {
				peerEvPath := filepath.Join(runDir, fmt.Sprintf("ao2-wc-%s-peer-%d-evidence.json", prevWc.WorkcellID, idx))
				var pStdout, pStderr, pStatus string
				if pEvData, err := os.ReadFile(peerEvPath); err == nil {
					var pEvObj map[string]any
					if err := json.Unmarshal(pEvData, &pEvObj); err == nil {
						if st, ok := pEvObj["stdout"].(string); ok {
							pStdout = st
						}
						if se, ok := pEvObj["stderr"].(string); ok {
							pStderr = se
						}
						if s, ok := pEvObj["status"].(string); ok {
							pStatus = s
						}
					}
				}
				peerStates = append(peerStates, &peerRunState{
					stateMu: &sync.Mutex{},
					Index:   idx,
					Status:  pStatus,
					Stdout:  pStdout,
					Stderr:  pStderr,
				})
			}
		}

		prevStates[prevWc.WorkcellID] = &workcellRunState{
			ID:               prevWc.WorkcellID,
			Status:           "passed",
			Summary:          prevWc.Summary,
			Stdout:           stdoutText,
			Stderr:           stderrText,
			SpecSHA256:       specSHA,
			Workspace:        prevWc.Workspace,
			Executor:         prevWc.Executor,
			Peers:            prevWc.Peers,
			MaxRepairs:       prevWc.MaxRepairs,
			RepairsAttempted: prevWc.RepairsAttempted,
			Task:             prevWc.Task,
			PeerStates:       peerStates,
		}
	}
	return prevStates
}

func archiveRunState(runID string, planPath string, gateResultPath string, summaryPath string, packetData []byte, packet factoryPacket) {
	dotForge := ".forge"
	if info, err := os.Stat(dotForge); err != nil || !info.IsDir() {
		return
	}

	runDir := filepath.Join(dotForge, "runs", runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return
	}

	if planData, err := os.ReadFile(planPath); err == nil {
		_ = os.WriteFile(filepath.Join(runDir, "plan.json"), planData, 0644)
	}

	if gateData, err := os.ReadFile(gateResultPath); err == nil {
		_ = os.WriteFile(filepath.Join(runDir, "gate_result.json"), gateData, 0644)
	}

	if summaryPath != "" {
		if summaryData, err := os.ReadFile(summaryPath); err == nil {
			_ = os.WriteFile(filepath.Join(runDir, "ao2-run-summary.json"), summaryData, 0644)
		}
	}

	for _, wc := range packet.Workcells {
		wcEvName := fmt.Sprintf("ao2-wc-%s-evidence.json", wc.WorkcellID)
		var srcDir string
		if summaryPath != "" {
			srcDir = filepath.Dir(summaryPath)
		} else {
			srcDir = "."
		}
		srcPath := filepath.Join(srcDir, wcEvName)
		if evData, err := os.ReadFile(srcPath); err == nil {
			_ = os.WriteFile(filepath.Join(runDir, wcEvName), evData, 0644)
		}
		if wc.Peers > 1 {
			for idx := 0; idx < wc.Peers; idx++ {
				peerEvName := fmt.Sprintf("ao2-wc-%s-peer-%d-evidence.json", wc.WorkcellID, idx)
				peerSrcPath := filepath.Join(srcDir, peerEvName)
				if peerEvData, err := os.ReadFile(peerSrcPath); err == nil {
					_ = os.WriteFile(filepath.Join(runDir, peerEvName), peerEvData, 0644)
				}
			}
		}
	}

	_ = os.WriteFile(filepath.Join(runDir, "factory-packet.json"), packetData, 0644)
	_ = writeMarkdownPacket(filepath.Join(runDir, "factory-packet.json"), packet)
}
