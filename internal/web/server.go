package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/qzhello/code-review/internal/agent"
	"github.com/qzhello/code-review/internal/model"
	"github.com/qzhello/code-review/internal/review"
)

//go:embed static/*
var staticFS embed.FS

// FindingState holds a finding and its current action state.
type FindingState struct {
	Finding     model.Finding      `json:"finding"`
	Action      string             `json:"action"` // pending, accepted, dismissed, fixed, fixing
	Explanation string             `json:"explanation,omitempty"`
	FixError    string             `json:"fix_error,omitempty"`
	Proposal    *agent.FixProposal `json:"-"`
	ProposalDTO *FixProposalDTO    `json:"proposal,omitempty"`
}

// FixProposalDTO is the JSON-safe version of a fix proposal.
type FixProposalDTO struct {
	FilePath    string `json:"file_path"`
	Original    string `json:"original"`
	Fixed       string `json:"fixed"`
	Explanation string `json:"explanation"`
}

// Server is the web UI server.
type Server struct {
	mu       sync.RWMutex
	findings []FindingState
	diff     *model.DiffResult
	result   *review.Result
	agentCfg *model.AgentConfig

	// Chat sessions per finding index
	chatMu        sync.Mutex
	chatSessions  map[int]*agent.ChatSession
	chatHistories map[int][]ChatMessage
}

// ChatMessage is a chat message for the web API.
type ChatMessage struct {
	Role    string `json:"role"` // user, assistant, system
	Content string `json:"content"`
}

// NewServer creates a new web server.
func NewServer(result *review.Result, diff *model.DiffResult, agentCfg *model.AgentConfig) *Server {
	states := make([]FindingState, len(result.Findings))
	for i, f := range result.Findings {
		states[i] = FindingState{
			Finding: f,
			Action:  "pending",
		}
	}

	return &Server{
		findings:      states,
		diff:          diff,
		result:        result,
		agentCfg:      agentCfg,
		chatSessions:  make(map[int]*agent.ChatSession),
		chatHistories: make(map[int][]ChatMessage),
	}
}

// Start starts the web server and opens the browser.
func (s *Server) Start(port int, open bool) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/findings", s.handleFindings)
	mux.HandleFunc("/api/findings/", s.handleFindingAction)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/chat/stream", s.handleChatStream)
	mux.HandleFunc("/api/summary", s.handleSummary)
	mux.HandleFunc("/api/diff/", s.handleDiff)

	// Static files
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("failed to load static files: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticContent)))

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://localhost:%d", actualPort)
	fmt.Printf("  Code Review Web UI: %s\n", url)
	fmt.Println("  Press Ctrl+C to stop")

	if open {
		go openBrowser(url)
	}

	server := &http.Server{Handler: mux}
	return server.Serve(listener)
}

func openBrowser(url string) {
	time.Sleep(300 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}

// --- API Handlers ---

func (s *Server) handleFindings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	writeJSON(w, s.findings)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	pending, accepted, dismissed, fixed := 0, 0, 0, 0
	for _, f := range s.findings {
		switch f.Action {
		case "accepted":
			accepted++
		case "dismissed":
			dismissed++
		case "fixed":
			fixed++
		default:
			pending++
		}
	}

	writeJSON(w, map[string]int{
		"total":     len(s.findings),
		"pending":   pending,
		"accepted":  accepted,
		"dismissed": dismissed,
		"fixed":     fixed,
	})
}

func (s *Server) handleFindingAction(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/findings/{index}/{action}
	parts := splitPath(r.URL.Path, "/api/findings/")
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	index := parseInt(parts[0])
	action := parts[1]

	s.mu.Lock()
	if index < 0 || index >= len(s.findings) {
		s.mu.Unlock()
		http.Error(w, "invalid finding index", http.StatusBadRequest)
		return
	}

	switch action {
	case "accept":
		s.findings[index].Action = "accepted"
		s.mu.Unlock()
		writeJSON(w, map[string]string{"status": "ok"})

	case "dismiss":
		s.findings[index].Action = "dismissed"
		s.mu.Unlock()
		writeJSON(w, map[string]string{"status": "ok"})

	case "reset":
		s.findings[index].Action = "pending"
		s.findings[index].Explanation = ""
		s.findings[index].FixError = ""
		s.findings[index].Proposal = nil
		s.findings[index].ProposalDTO = nil
		s.mu.Unlock()
		writeJSON(w, map[string]string{"status": "ok"})

	case "fix":
		if r.Method == http.MethodPost {
			s.handleFixPropose(w, r, index)
		} else {
			s.mu.Unlock()
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}

	case "fix-apply":
		s.handleFixApply(w, index)

	case "fix-cancel":
		s.findings[index].Proposal = nil
		s.findings[index].ProposalDTO = nil
		s.findings[index].Action = "pending"
		s.mu.Unlock()
		writeJSON(w, map[string]string{"status": "ok"})

	default:
		s.mu.Unlock()
		http.Error(w, "unknown action: "+action, http.StatusBadRequest)
	}
}

func (s *Server) handleFixPropose(w http.ResponseWriter, r *http.Request, index int) {
	if s.agentCfg == nil || s.agentCfg.APIKey == "" {
		s.mu.Unlock()
		writeJSON(w, map[string]string{"error": "agent not configured"})
		return
	}

	finding := s.findings[index].Finding
	s.findings[index].Action = "fixing"
	cfg := *s.agentCfg
	s.mu.Unlock()

	// Run AI fix in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		fixer, err := agent.NewFixer(cfg)
		if err != nil {
			s.mu.Lock()
			s.findings[index].Action = "pending"
			s.findings[index].FixError = err.Error()
			s.mu.Unlock()
			return
		}

		proposal, err := fixer.Propose(ctx, finding)
		s.mu.Lock()
		if err != nil {
			s.findings[index].Action = "pending"
			s.findings[index].FixError = err.Error()
		} else {
			s.findings[index].Action = "pending"
			s.findings[index].FixError = ""
			s.findings[index].Proposal = proposal
			s.findings[index].ProposalDTO = &FixProposalDTO{
				FilePath:    proposal.FilePath,
				Original:    proposal.Original,
				Fixed:       proposal.FixedContent,
				Explanation: proposal.Explanation,
			}
		}
		s.mu.Unlock()
	}()

	writeJSON(w, map[string]string{"status": "fixing"})
}

func (s *Server) handleFixApply(w http.ResponseWriter, index int) {
	proposal := s.findings[index].Proposal
	if proposal == nil {
		s.mu.Unlock()
		writeJSON(w, map[string]string{"error": "no pending fix"})
		return
	}

	err := proposal.Apply()
	if err != nil {
		s.findings[index].FixError = err.Error()
		s.mu.Unlock()
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}

	s.findings[index].Action = "fixed"
	s.findings[index].Explanation = proposal.Explanation
	s.findings[index].Proposal = nil
	s.findings[index].ProposalDTO = nil
	s.mu.Unlock()
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Index   int    `json:"index"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	if req.Index < 0 || req.Index >= len(s.findings) {
		s.mu.RUnlock()
		http.Error(w, "invalid finding index", http.StatusBadRequest)
		return
	}
	finding := s.findings[req.Index].Finding
	s.mu.RUnlock()

	if s.agentCfg == nil || s.agentCfg.APIKey == "" {
		writeJSON(w, map[string]interface{}{
			"message": "Agent not configured (no API key)",
			"action":  "none",
		})
		return
	}

	// Get or create chat session
	s.chatMu.Lock()
	session, ok := s.chatSessions[req.Index]
	if !ok {
		var err error
		session, err = agent.NewChatSession(*s.agentCfg)
		if err != nil {
			s.chatMu.Unlock()
			writeJSON(w, map[string]interface{}{
				"message": "Failed to start chat: " + err.Error(),
				"action":  "none",
			})
			return
		}
		if err := session.SetFindingContext(finding); err != nil {
			s.chatMu.Unlock()
			writeJSON(w, map[string]interface{}{
				"message": "Failed to load file: " + err.Error(),
				"action":  "none",
			})
			return
		}
		s.chatSessions[req.Index] = session
	}

	// Save user message to history
	s.chatHistories[req.Index] = append(s.chatHistories[req.Index], ChatMessage{
		Role:    "user",
		Content: req.Message,
	})
	s.chatMu.Unlock()

	// Send to AI
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	resp, err := session.Send(ctx, req.Message)
	if err != nil {
		s.chatMu.Lock()
		s.chatHistories[req.Index] = append(s.chatHistories[req.Index], ChatMessage{
			Role:    "system",
			Content: "Error: " + err.Error(),
		})
		s.chatMu.Unlock()
		writeJSON(w, map[string]interface{}{
			"message": "Error: " + err.Error(),
			"action":  "none",
		})
		return
	}

	// Save assistant message
	s.chatMu.Lock()
	s.chatHistories[req.Index] = append(s.chatHistories[req.Index], ChatMessage{
		Role:    "assistant",
		Content: resp.Message,
	})
	s.chatMu.Unlock()

	// Handle actions
	result := map[string]interface{}{
		"message": resp.Message,
		"action":  resp.Action,
	}

	s.mu.Lock()
	switch resp.Action {
	case "fix":
		if resp.FixedContent != "" {
			original, readErr := os.ReadFile(finding.FilePath)
			if readErr == nil {
				s.findings[req.Index].Proposal = &agent.FixProposal{
					FilePath:     finding.FilePath,
					Original:     string(original),
					FixedContent: resp.FixedContent,
					Explanation:  resp.Message,
				}
				s.findings[req.Index].ProposalDTO = &FixProposalDTO{
					FilePath:    finding.FilePath,
					Original:    string(original),
					Fixed:       resp.FixedContent,
					Explanation: resp.Message,
				}
				result["proposal"] = s.findings[req.Index].ProposalDTO
			}
		}
	case "dismiss":
		s.findings[req.Index].Action = "dismissed"
	case "accept":
		s.findings[req.Index].Action = "accepted"
	}
	s.mu.Unlock()

	writeJSON(w, result)
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Index   int    `json:"index"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	if req.Index < 0 || req.Index >= len(s.findings) {
		s.mu.RUnlock()
		http.Error(w, "invalid finding index", http.StatusBadRequest)
		return
	}
	finding := s.findings[req.Index].Finding
	s.mu.RUnlock()

	if s.agentCfg == nil || s.agentCfg.APIKey == "" {
		http.Error(w, "agent not configured", http.StatusBadRequest)
		return
	}

	// Get or create plain-mode chat session (markdown output, no JSON)
	s.chatMu.Lock()
	session, ok := s.chatSessions[req.Index]
	if !ok {
		var err error
		session, err = agent.NewPlainChatSession(*s.agentCfg)
		if err != nil {
			s.chatMu.Unlock()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := session.SetFindingContext(finding); err != nil {
			s.chatMu.Unlock()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.chatSessions[req.Index] = session
	}
	s.chatHistories[req.Index] = append(s.chatHistories[req.Index], ChatMessage{
		Role: "user", Content: req.Message,
	})
	s.chatMu.Unlock()

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// Real streaming — plain markdown, no JSON parsing needed.
	// Tokens go directly from LLM → SSE → browser.
	fullText, err := session.SendPlainStream(ctx, req.Message, func(token string) {
		tokenData, _ := json.Marshal(map[string]string{"token": token})
		fmt.Fprintf(w, "data: %s\n\n", tokenData)
		flusher.Flush()
	})

	if err != nil {
		errData, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintf(w, "data: %s\n\n", errData)
		flusher.Flush()
		return
	}

	// Save to history
	s.chatMu.Lock()
	s.chatHistories[req.Index] = append(s.chatHistories[req.Index], ChatMessage{
		Role: "assistant", Content: fullText,
	})
	s.chatMu.Unlock()

	// Send done event
	doneData, _ := json.Marshal(map[string]interface{}{
		"done":    true,
		"message": fullText,
	})
	fmt.Fprintf(w, "data: %s\n\n", doneData)
	flusher.Flush()
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	// /api/diff/{index}
	parts := splitPath(r.URL.Path, "/api/diff/")
	if len(parts) < 1 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	index := parseInt(parts[0])

	s.mu.RLock()
	defer s.mu.RUnlock()

	if index < 0 || index >= len(s.findings) {
		http.Error(w, "invalid index", http.StatusBadRequest)
		return
	}

	finding := s.findings[index].Finding
	if s.diff == nil {
		writeJSON(w, map[string]string{"diff": ""})
		return
	}

	for _, file := range s.diff.Files {
		if file.Path() != finding.FilePath {
			continue
		}
		var lines []map[string]interface{}
		for _, hunk := range file.Hunks {
			hunkEnd := hunk.NewStart + hunk.NewCount
			if finding.Line > 0 && (finding.Line < hunk.NewStart || finding.Line > hunkEnd) {
				continue
			}
			lines = append(lines, map[string]interface{}{
				"type":    "header",
				"content": fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount, hunk.Header),
			})
			for _, line := range hunk.Lines {
				lineType := "context"
				switch line.Type {
				case model.LineAdded:
					lineType = "added"
				case model.LineRemoved:
					lineType = "removed"
				}
				highlight := lineType == "added" && line.NewNum == finding.Line
				lines = append(lines, map[string]interface{}{
					"type":      lineType,
					"content":   line.Content,
					"highlight": highlight,
					"num":       line.NewNum,
				})
			}
		}
		writeJSON(w, map[string]interface{}{"lines": lines})
		return
	}

	writeJSON(w, map[string]interface{}{"lines": []interface{}{}})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func splitPath(path, prefix string) []string {
	rest := path[len(prefix):]
	var parts []string
	for _, p := range splitString(rest, '/') {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitString(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := range len(s) {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}

// Silence unused import
var _ = log.Println
