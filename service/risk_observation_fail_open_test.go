package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestProcessRiskObservationForRelay_fails_open_when_enabled_policy_has_no_provider(t *testing.T) {
	// Given
	executorCalls := 0
	queuedJobs := 0
	var completed RiskObservationEvent
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.RiskPolicyState{
				Enabled:         true,
				ProviderIDs:     nil,
				EnabledChannels: []int{24},
				ReviewMode:      model.RiskReviewFull,
				ActionMode:      model.RiskActionBlock,
			}, nil
		},
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			executorCalls++
			return RiskModerationOutcome{}, nil
		}),
		enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
			queuedJobs++
			return queuedRiskObservationResult()
		},
		enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
			completed = event
			return queuedRiskObservationResult()
		},
	}
	var decision RiskObservationRelayDecision

	// When
	require.NotPanics(t, func() {
		decision = processRiskObservationForRelay(context.Background(), RiskObservationJob{
			RequestID: "missing-provider", ChannelID: 24, ChannelName: "renamed", Text: "current",
		}, deps)
	})

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Zero(t, executorCalls)
	require.Zero(t, queuedJobs)
	require.Equal(t, RiskObservationError, completed.Result)
	require.Equal(t, riskObservationProviderConfigError, completed.ErrorCode)
	require.Equal(t, RiskObservationSourceLocal, completed.Source)
	require.Zero(t, completed.ProviderID)
	require.False(t, completed.ProviderCalled)
	require.False(t, completed.Blocked)
}

func TestProcessRiskObservationForRelay_skips_tool_continuations_without_current_user_text(t *testing.T) {
	// Given
	responsesInput, err := common.Marshal([]map[string]any{
		{"type": "message", "role": "user", "content": "old responses turn"},
		{"type": "function_call_output", "output": "tool result"},
	})
	require.NoError(t, err)

	requests := []struct {
		name    string
		request dto.Request
	}{
		{name: "openai", request: &dto.GeneralOpenAIRequest{Messages: []dto.Message{
			{Role: "user", Content: "old openai turn"},
			{Role: "tool", Content: "tool result"},
		}}},
		{name: "responses", request: &dto.OpenAIResponsesRequest{Input: responsesInput}},
		{name: "claude", request: &dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "old claude turn"},
			{Role: "assistant", Content: "tool call"},
		}}},
		{name: "gemini", request: &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{
			{Role: "user", Parts: []dto.GeminiPart{{Text: "old gemini turn"}}},
			{Role: "model", Parts: []dto.GeminiPart{{Text: "tool call"}}},
		}}},
	}

	for _, test := range requests {
		t.Run(test.name, func(t *testing.T) {
			policyLoads := 0
			queuedJobs := 0
			queuedEvents := 0
			deps := riskObservationRelayDeps{
				loadPolicy: func() (model.RiskPolicyState, error) {
					policyLoads++
					return model.RiskPolicyState{}, nil
				},
				enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
					queuedJobs++
					return queuedRiskObservationResult()
				},
				enqueueEvent: func(RiskObservationEvent) RiskObservationEnqueueResult {
					queuedEvents++
					return queuedRiskObservationResult()
				},
			}

			// When
			decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
				RequestID: "tool-continuation", ChannelID: 24, Text: ExtractRiskObservationText(test.request),
			}, deps)

			// Then
			require.False(t, decision.Blocked)
			require.Nil(t, decision.DirectRecord)
			require.Zero(t, policyLoads)
			require.Zero(t, queuedJobs)
			require.Zero(t, queuedEvents)
		})
	}
}
