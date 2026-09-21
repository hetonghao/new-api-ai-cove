package common

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/logger"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

const (
	deepSeekFailureLogTag       = "deepseek_upstream_failure"
	deepSeekFailureChunkSize    = 16 << 10
	deepSeekFailurePayloadCap   = 8 << 20
	deepSeekFailureHourlyCap    = 32 << 20
	deepSeekFailureFingerprints = 64
	deepSeekFailurePreviewLimit = 512
)

// IsDeepSeekResponsesRelay reports whether this relay drives a DeepSeek
// /responses upstream, the only path whose rejected payload is worth dumping.
func IsDeepSeekResponsesRelay(info *RelayInfo) bool {
	if info == nil {
		return false
	}
	switch info.RelayMode {
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
	default:
		return false
	}
	return IsDeepSeekReasoningRelay(info, info.OriginModelName) ||
		IsDeepSeekReasoningRelay(info, deepSeekRelayUpstreamModel(info))
}

// deepSeekRelayUpstreamModel keeps the embedded ChannelMeta access nil-safe:
// embedding a nil *ChannelMeta would otherwise panic on field promotion.
func deepSeekRelayUpstreamModel(info *RelayInfo) string {
	if info == nil || info.ChannelMeta == nil {
		return ""
	}
	return info.ChannelMeta.UpstreamModelName
}

// DeepSeekFailureCapture keeps the exact payload a DeepSeek /responses request
// sent upstream, so a rejection can be reconstructed from the process log
// without turning on DEBUG for the whole process. It is nil for every other
// relay, and all methods tolerate a nil receiver.
type DeepSeekFailureCapture struct {
	ctx           *gin.Context
	info          *RelayInfo
	originalInput json.RawMessage

	outbound     []byte
	sent         DeepSeekToolReasoningDiagnostics
	sentValid    bool
	original     DeepSeekToolReasoningDiagnostics
	originalDone bool
	prep         string
}

func NewDeepSeekFailureCapture(c *gin.Context, info *RelayInfo, originalInput json.RawMessage) *DeepSeekFailureCapture {
	if !IsDeepSeekResponsesRelay(info) {
		return nil
	}
	return &DeepSeekFailureCapture{ctx: c, info: info, originalInput: originalInput}
}

// ObserveOutbound retains the bytes handed to the transport together with their
// item shape, so a rejection can be reported against what actually went out.
func (x *DeepSeekFailureCapture) ObserveOutbound(outbound []byte) {
	if x == nil || len(outbound) == 0 {
		return
	}
	x.outbound = outbound
	sent, ok := DeepSeekToolReasoningDiagnosticsOfBody(outbound)
	x.sent, x.sentValid = sent, ok
}

// NotePreparation records which adaptor prepared this attempt and whether the
// DeepSeek item normalization ran before it. The 400 log carries it so a request
// that skipped normalization can be told apart from one that ran it and still
// failed, without turning on DEBUG.
func (x *DeepSeekFailureCapture) NotePreparation(adaptor string, normalized bool) {
	if x == nil {
		return
	}
	apiType, channelType := 0, 0
	if x.info != nil {
		apiType, channelType = x.info.ApiType, x.info.ChannelType
	}
	x.prep = fmt.Sprintf("api_type=%d channel_type=%d adaptor=%s normalized=%t", apiType, channelType, adaptor, normalized)
}

// ReportFailure dumps the request payload and both shape reports once the
// upstream answered the request with a 400. It reads and restores the response
// body, so the caller's error handling is unaffected.
func (x *DeepSeekFailureCapture) ReportFailure(resp *http.Response) {
	if x == nil || resp == nil {
		return
	}
	upstreamErr := ReadAndRestoreResponseBody(resp)
	if len(x.outbound) == 0 || resp.StatusCode != http.StatusBadRequest || x.ctx == nil {
		return
	}
	fingerprint := deepSeekFailureFingerprint(x.outbound)
	head, tail, omitted := splitDeepSeekFailurePayload(x.outbound)
	allowed, suppressedReason := reserveDeepSeekFailureDump(fingerprint, len(head)+len(tail))

	sent := "unavailable"
	if x.sentValid {
		sent = x.sent.String()
	}
	payload := fmt.Sprintf("fingerprint=%s parts=%d", fingerprint, chunkCount(head)+chunkCount(tail))
	if omitted > 0 {
		payload += fmt.Sprintf(" omitted_bytes=%d", omitted)
	}
	if !allowed {
		payload = fmt.Sprintf("suppressed reason=%s %s", suppressedReason, payload)
	}
	logger.LogError(x.ctx, fmt.Sprintf(
		"%s status=%d channel=%d model=%q upstream_model=%q relay_mode=%d sent[%s] client[%s] prep[%s] payload=%s upstream_error=%q",
		deepSeekFailureLogTag, resp.StatusCode, x.channelID(), x.originModel(), x.upstreamModel(), x.relayMode(),
		sent, x.originalDiagnostics().String(), x.preparation(), payload,
		deepSeekFailurePreview(upstreamErr, deepSeekFailurePreviewLimit),
	))
	if !allowed {
		return
	}
	emitDeepSeekFailureChunks(x.ctx, deepSeekFailureLogTag+" payload", fingerprint, head)
	if omitted > 0 {
		logger.LogError(x.ctx, fmt.Sprintf("%s payload_omitted fingerprint=%s bytes=%d",
			deepSeekFailureLogTag, fingerprint, omitted))
	}
	if len(tail) > 0 {
		emitDeepSeekFailureChunks(x.ctx, deepSeekFailureLogTag+" payload_tail", fingerprint, tail)
	}
	logger.LogError(x.ctx, fmt.Sprintf("%s payload_end fingerprint=%s bytes=%d",
		deepSeekFailureLogTag, fingerprint, len(x.outbound)))
}

func (x *DeepSeekFailureCapture) preparation() string {
	if x == nil || x.prep == "" {
		return "unavailable"
	}
	return x.prep
}

func (x *DeepSeekFailureCapture) originalDiagnostics() DeepSeekToolReasoningDiagnostics {
	if x == nil || x.originalDone {
		return x.original
	}
	x.original = DeepSeekToolReasoningDiagnosticsOfInput(x.originalInput)
	x.originalDone = true
	return x.original
}

func (x *DeepSeekFailureCapture) originModel() string {
	if x.info == nil {
		return ""
	}
	return x.info.OriginModelName
}

func (x *DeepSeekFailureCapture) upstreamModel() string {
	if x.info == nil {
		return ""
	}
	return deepSeekRelayUpstreamModel(x.info)
}

func (x *DeepSeekFailureCapture) channelID() int {
	if x.info == nil || x.info.ChannelMeta == nil {
		return 0
	}
	return x.info.ChannelMeta.ChannelId
}

func (x *DeepSeekFailureCapture) relayMode() int {
	if x.info == nil {
		return 0
	}
	return x.info.RelayMode
}

// ReadAndRestoreResponseBody drains a response body and installs a fresh reader
// with the same bytes, so a caller can inspect an upstream rejection without
// taking it away from the regular error path.
func ReadAndRestoreResponseBody(resp *http.Response) []byte {
	if resp == nil || resp.Body == nil {
		return nil
	}
	original := resp.Body
	body, err := io.ReadAll(original)
	_ = original.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil && len(body) == 0 {
		return nil
	}
	return body
}

func deepSeekFailurePreview(body []byte, limit int) string {
	preview := strings.Join(strings.Fields(string(body)), " ")
	if len(preview) > limit {
		preview = preview[:limit] + "..."
	}
	return preview
}

func deepSeekFailureFingerprint(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:4])
}

func splitDeepSeekFailurePayload(body []byte) (head, tail []byte, omitted int) {
	if len(body) <= deepSeekFailurePayloadCap {
		return body, nil, 0
	}
	half := deepSeekFailurePayloadCap / 2
	return body[:half], body[len(body)-half:], len(body) - deepSeekFailurePayloadCap
}

func chunkCount(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	return (len(data) + deepSeekFailureChunkSize - 1) / deepSeekFailureChunkSize
}

// emitDeepSeekFailureChunks writes the payload as base64 parts. The process log
// appends a trailing space to every line and replaces a byte sequence that cuts
// a UTF-8 rune with U+FFFD, so a raw slice cannot be reassembled byte-for-byte.
func emitDeepSeekFailureChunks(c *gin.Context, label, fingerprint string, data []byte) {
	parts := deepSeekFailureDumpParts(data)
	for i, part := range parts {
		logger.LogError(c, fmt.Sprintf("%s fingerprint=%s part=%d/%d b64=%s", label, fingerprint, i+1, len(parts), part))
	}
}

func deepSeekFailureDumpParts(data []byte) []string {
	parts := make([]string, 0, chunkCount(data))
	for start := 0; start < len(data); start += deepSeekFailureChunkSize {
		end := min(start+deepSeekFailureChunkSize, len(data))
		parts = append(parts, base64.StdEncoding.EncodeToString(data[start:end]))
	}
	return parts
}

var (
	deepSeekFailureDumpMu     sync.Mutex
	deepSeekFailureDumpSeen   = map[string]struct{}{}
	deepSeekFailureDumpOrder  []string
	deepSeekFailureDumpWindow time.Time
	deepSeekFailureDumpBytes  int
)

// reserveDeepSeekFailureDump bounds the payload dumps: an hour carries at most
// deepSeekFailureHourlyCap bytes and every distinct payload is dumped once.
func reserveDeepSeekFailureDump(fingerprint string, size int) (bool, string) {
	deepSeekFailureDumpMu.Lock()
	defer deepSeekFailureDumpMu.Unlock()
	if deepSeekFailureDumpWindow.IsZero() || time.Since(deepSeekFailureDumpWindow) >= time.Hour {
		deepSeekFailureDumpWindow = time.Now()
		deepSeekFailureDumpBytes = 0
		deepSeekFailureDumpSeen = map[string]struct{}{}
		deepSeekFailureDumpOrder = nil
	}
	if _, seen := deepSeekFailureDumpSeen[fingerprint]; seen {
		return false, "duplicate_payload"
	}
	if size > deepSeekFailureHourlyCap || deepSeekFailureDumpBytes+size > deepSeekFailureHourlyCap {
		return false, "hourly_cap"
	}
	deepSeekFailureDumpSeen[fingerprint] = struct{}{}
	deepSeekFailureDumpOrder = append(deepSeekFailureDumpOrder, fingerprint)
	if len(deepSeekFailureDumpOrder) > deepSeekFailureFingerprints {
		delete(deepSeekFailureDumpSeen, deepSeekFailureDumpOrder[0])
		deepSeekFailureDumpOrder = deepSeekFailureDumpOrder[1:]
	}
	deepSeekFailureDumpBytes += size
	return true, ""
}

func resetDeepSeekFailureDumpState() {
	deepSeekFailureDumpMu.Lock()
	defer deepSeekFailureDumpMu.Unlock()
	deepSeekFailureDumpSeen = map[string]struct{}{}
	deepSeekFailureDumpOrder = nil
	deepSeekFailureDumpWindow = time.Time{}
	deepSeekFailureDumpBytes = 0
}
