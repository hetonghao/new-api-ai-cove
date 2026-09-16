package common

import (
	"encoding/json"
	"fmt"
	"strings"

	napicommon "github.com/QuantumNous/new-api/common"
)

// DeepSeekToolReasoningDiagnostics summarizes the item shape of a /responses
// input payload against what the DeepSeek upstream actually validates. The 400
// it answers with never names the offending item, so this report is what keeps a
// rejection reconstructable from the process log.
//
// A tool run without a reasoning item is accepted by the upstream. What it
// rejects is a reasoning item directly after an assistant message:
//
//	message(assistant) -> reasoning
//
// which it answers with "The `reasoning_text` in the thinking mode must be
// passed back to the API." even though the reasoning is present. That adjacency
// is counted in ReasoningAfterMessage and must stay zero.
type DeepSeekToolReasoningDiagnostics struct {
	Items                 int
	Bytes                 int
	Runs                  int
	Calls                 int
	Outputs               int
	Reasoning             int
	EmptyReasoning        int
	ReasoningAfterMessage int
	UnpairedCalls         int
	OrphanOutputs         int
	Interleaved           int
}

func (d DeepSeekToolReasoningDiagnostics) String() string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"items=%d runs=%d calls=%d outputs=%d reasoning=%d reasoning_empty=%d reasoning_after_message=%d",
		d.Items, d.Runs, d.Calls, d.Outputs, d.Reasoning, d.EmptyReasoning, d.ReasoningAfterMessage)
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
	for i, item := range items {
		if item.Type != "reasoning" {
			continue
		}
		diagnostics.Reasoning++
		if !item.hasReasoningText() {
			diagnostics.EmptyReasoning++
		}
		if i > 0 && items[i-1].Role == "assistant" {
			diagnostics.ReasoningAfterMessage++
		}
	}
	return diagnostics
}

// summarizeToolRuns counts maximal runs of contiguous tool items.
func (d *DeepSeekToolReasoningDiagnostics) summarizeToolRuns(items []deepSeekDiagnosticItem) {
	runStart := -1
	for i := range len(items) + 1 {
		insideRun := i < len(items) && items[i].isToolItem()
		if insideRun && runStart < 0 {
			runStart = i
			d.Runs++
			continue
		}
		if !insideRun {
			runStart = -1
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
	Role    string          `json:"role"`
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
