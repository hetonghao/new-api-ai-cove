package common

import (
	"encoding/json"
	"fmt"
	"strings"

	napicommon "github.com/QuantumNous/new-api/common"
)

// deepSeekDiagnosticSampleLimit caps how many offending call sites a single
// report renders. The counters stay exact; only the sample is truncated.
const deepSeekDiagnosticSampleLimit = 4

// DeepSeekToolReasoningDiagnostics summarizes the tool-run/reasoning shape of a
// /responses input payload. DeepSeek rejects a tool run whose reasoning_text is
// missing or empty with a 400 that never names the offending item, so this
// report is what keeps such a failure reconstructable from the process log.
type DeepSeekToolReasoningDiagnostics struct {
	Items                 int
	Bytes                 int
	Runs                  int
	Calls                 int
	Outputs               int
	Reasoning             int
	EmptyReasoning        int
	UnpairedCalls         int
	OrphanOutputs         int
	Interleaved           int
	MissingReasoningTotal int
	MissingReasoning      []string
}

// NeedsReasoning reports whether the payload carries a tool run that the
// upstream rejects for a missing reasoning_text item.
func (d DeepSeekToolReasoningDiagnostics) NeedsReasoning() bool {
	return d.MissingReasoningTotal > 0
}

func (d DeepSeekToolReasoningDiagnostics) String() string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"items=%d runs=%d calls=%d outputs=%d reasoning=%d reasoning_empty=%d missing_reasoning=%d",
		d.Items, d.Runs, d.Calls, d.Outputs, d.Reasoning, d.EmptyReasoning, d.MissingReasoningTotal)
	if len(d.MissingReasoning) > 0 {
		fmt.Fprintf(&b, "[%s]", strings.Join(d.MissingReasoning, " "))
	}
	if d.UnpairedCalls > 0 {
		fmt.Fprintf(&b, " unpaired_calls=%d", d.UnpairedCalls)
	}
	if d.OrphanOutputs > 0 {
		fmt.Fprintf(&b, " orphan_outputs=%d", d.OrphanOutputs)
	}
	if d.Interleaved > 0 {
		fmt.Fprintf(&b, " interleaved=%d", d.Interleaved)
	}
	if d.Bytes > 0 {
		fmt.Fprintf(&b, " bytes=%d", d.Bytes)
	}
	return b.String()
}

// DeepSeekToolReasoningDiagnosticsOfBody reports the input shape of a marshalled
// /responses request body and records its byte size.
func DeepSeekToolReasoningDiagnosticsOfBody(body []byte) (DeepSeekToolReasoningDiagnostics, bool) {
	if len(body) == 0 {
		return DeepSeekToolReasoningDiagnostics{}, false
	}
	// One decode of the whole body keeps this affordable on the hot path: the
	// payload reaches 1-2 MB, and every extra pass costs milliseconds.
	var envelope struct {
		Input []deepSeekDiagnosticItem `json:"input"`
	}
	if napicommon.Unmarshal(body, &envelope) != nil {
		return DeepSeekToolReasoningDiagnostics{}, false
	}
	diagnostics := summarizeDeepSeekToolReasoning(envelope.Input)
	diagnostics.Bytes = len(body)
	return diagnostics, true
}

func DeepSeekToolReasoningDiagnosticsOfInput(input json.RawMessage) DeepSeekToolReasoningDiagnostics {
	var items []deepSeekDiagnosticItem
	if len(input) == 0 || napicommon.Unmarshal(input, &items) != nil {
		return DeepSeekToolReasoningDiagnostics{}
	}
	return summarizeDeepSeekToolReasoning(items)
}

func summarizeDeepSeekToolReasoning(items []deepSeekDiagnosticItem) DeepSeekToolReasoningDiagnostics {
	diagnostics := DeepSeekToolReasoningDiagnostics{Items: len(items)}
	diagnostics.summarizeToolRuns(items)
	diagnostics.summarizeToolPairing(items)
	for _, item := range items {
		if item.Type != "reasoning" {
			continue
		}
		diagnostics.Reasoning++
		if !item.hasReasoningText() {
			diagnostics.EmptyReasoning++
		}
	}
	return diagnostics
}

// summarizeToolRuns walks maximal runs of contiguous tool items and records the
// runs that start without a preceding non-empty reasoning item.
func (d *DeepSeekToolReasoningDiagnostics) summarizeToolRuns(items []deepSeekDiagnosticItem) {
	runStart := -1
	for i := range len(items) + 1 {
		insideRun := i < len(items) && items[i].isToolItem()
		if insideRun && runStart < 0 {
			runStart = i
			d.Runs++
			if !d.hasReasoningBefore(items, runStart) {
				d.recordMissingReasoning(items, runStart)
			}
			continue
		}
		if !insideRun {
			runStart = -1
		}
	}
}

func (d *DeepSeekToolReasoningDiagnostics) hasReasoningBefore(items []deepSeekDiagnosticItem, runStart int) bool {
	if runStart == 0 {
		return false
	}
	previous := items[runStart-1]
	return previous.Type == "reasoning" && previous.hasReasoningText()
}

func (d *DeepSeekToolReasoningDiagnostics) recordMissingReasoning(items []deepSeekDiagnosticItem, runStart int) {
	for i := runStart; i < len(items) && items[i].isToolItem(); i++ {
		if !items[i].isToolCall() {
			continue
		}
		d.MissingReasoningTotal++
		if len(d.MissingReasoning) < deepSeekDiagnosticSampleLimit {
			label := fmt.Sprintf("%s@%d", items[i].CallID, i)
			if items[i].Name != "" {
				label += ":" + items[i].Name
			}
			d.MissingReasoning = append(d.MissingReasoning, label)
		}
	}
}

// summarizeToolPairing pairs every call with the next unconsumed output that
// carries the same call_id, then counts what the upstream would refuse:
// outputs without a call, calls without an output, and non-tool items sitting
// between a call and its own output.
func (d *DeepSeekToolReasoningDiagnostics) summarizeToolPairing(items []deepSeekDiagnosticItem) {
	nonToolBefore := make([]int, len(items)+1)
	for i, item := range items {
		nonToolBefore[i+1] = nonToolBefore[i]
		if !item.isToolItem() {
			nonToolBefore[i+1]++
		}
	}
	pendingCalls := make(map[string][]int)
	pairedCalls := make(map[int]struct{})
	for i, item := range items {
		switch {
		case item.isToolCall():
			d.Calls++
			if item.CallID == "" {
				continue
			}
			pendingCalls[item.CallID] = append(pendingCalls[item.CallID], i)
		case item.isToolOutput():
			d.Outputs++
			if item.CallID == "" {
				continue
			}
			open := pendingCalls[item.CallID]
			if len(open) == 0 {
				d.OrphanOutputs++
				continue
			}
			callIndex := open[0]
			pendingCalls[item.CallID] = open[1:]
			pairedCalls[callIndex] = struct{}{}
			d.Interleaved += nonToolBefore[i] - nonToolBefore[callIndex+1]
		}
	}
	d.UnpairedCalls = d.Calls - len(pairedCalls)
}

type deepSeekDiagnosticItem struct {
	Type    string          `json:"type"`
	CallID  string          `json:"call_id"`
	Name    string          `json:"name"`
	Content json.RawMessage `json:"content"`
	Summary json.RawMessage `json:"summary"`
}

type deepSeekTextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// hasReasoningText decodes the content lazily: message items carry content in
// shapes this report must not choke on (plain strings, other part types), and
// only reasoning items need to be inspected at all.
func (item deepSeekDiagnosticItem) hasReasoningText() bool {
	var parts []deepSeekTextPart
	if len(item.Content) > 0 && napicommon.Unmarshal(item.Content, &parts) == nil {
		for _, part := range parts {
			if part.Type == "reasoning_text" && strings.TrimSpace(part.Text) != "" {
				return true
			}
		}
	}
	parts = nil
	if len(item.Summary) > 0 && napicommon.Unmarshal(item.Summary, &parts) == nil {
		for _, part := range parts {
			if strings.TrimSpace(part.Text) != "" {
				return true
			}
		}
	}
	return false
}

func (item deepSeekDiagnosticItem) isToolCall() bool {
	return item.Type == "function_call" || item.Type == "custom_tool_call"
}

func (item deepSeekDiagnosticItem) isToolOutput() bool {
	return item.Type == "function_call_output" || item.Type == "custom_tool_call_output"
}

func (item deepSeekDiagnosticItem) isToolItem() bool {
	return item.isToolCall() || item.isToolOutput()
}
