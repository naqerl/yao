package model

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/packages/respjson"
	"github.com/openai/openai-go/shared"
)

const (
	openCodeGoProvider        = "opencode-go"
	openCodeGoBaseURL         = "https://opencode.ai/zen/go/v1"
	openCodeGoDefaultModelEnv = "OPENCODE_GO_MODEL"
	openCodeGoExtraModelsEnv  = "OPENCODE_GO_EXTRA_MODELS"
)

var openCodeGoKnownModels = map[string]ai.ModelOptions{
	"glm-5": {
		Label:    "OpenCode Go GLM-5",
		Supports: &compat_oai.Multimodal,
		Versions: []string{"glm-5"},
	},
	"kimi-k2.5": {
		Label:    "OpenCode Go Kimi K2.5",
		Supports: &compat_oai.Multimodal,
		Versions: []string{"kimi-k2.5"},
	},
}

type OpenCodeGo struct {
	*reasoningCompatibleProvider
}

func NewOpenCodeGo(apiKey string) *OpenCodeGo {
	return &OpenCodeGo{
		reasoningCompatibleProvider: newReasoningCompatibleProvider(
			openCodeGoProvider,
			apiKey,
			openCodeGoBaseURL,
			openCodeGoSupportedModels(),
		),
	}
}

func InitOpenCodeGo(ctx context.Context) (*genkit.Genkit, error) {
	const apiKeyEnv = "OPENCODE_API_KEY"

	apiKey := os.Getenv(apiKeyEnv)
	if apiKey == "" {
		return nil, &CredsNotSetError{Detail: apiKeyEnv}
	}

	provider := NewOpenCodeGo(apiKey)
	modelName := openCodeGoDefaultModel()

	return genkit.Init(ctx,
		genkit.WithPlugins(provider),
		genkit.WithDefaultModel(openCodeGoProvider+"/"+modelName),
	), nil
}

func openCodeGoSupportedModels() map[string]ai.ModelOptions {
	supported := make(map[string]ai.ModelOptions, len(openCodeGoKnownModels))
	for id, opts := range openCodeGoKnownModels {
		supported[id] = opts
	}

	for _, id := range strings.Split(os.Getenv(openCodeGoExtraModelsEnv), ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := supported[id]; ok {
			continue
		}
		supported[id] = ai.ModelOptions{
			Label:    "OpenCode Go " + id,
			Supports: &compat_oai.Multimodal,
			Versions: []string{id},
		}
	}

	return supported
}

func openCodeGoDefaultModel() string {
	model := strings.TrimSpace(os.Getenv(openCodeGoDefaultModelEnv))
	if model == "" {
		return "kimi-k2.5"
	}
	return model
}

type reasoningCompatibleProvider struct {
	mu sync.Mutex

	initted         bool
	client          *openai.Client
	Opts            []option.RequestOption
	Provider        string
	APIKey          string
	BaseURL         string
	supportedModels map[string]ai.ModelOptions
}

func newReasoningCompatibleProvider(provider, apiKey, baseURL string, supportedModels map[string]ai.ModelOptions) *reasoningCompatibleProvider {
	return &reasoningCompatibleProvider{
		Provider:        provider,
		APIKey:          apiKey,
		BaseURL:         baseURL,
		supportedModels: supportedModels,
	}
}

func (p *reasoningCompatibleProvider) Init(ctx context.Context) []api.Action {
	p.mu.Lock()
	if p.initted {
		p.mu.Unlock()
		panic("reasoningCompatibleProvider.Init already called")
	}

	if p.APIKey != "" {
		p.Opts = append([]option.RequestOption{option.WithAPIKey(p.APIKey)}, p.Opts...)
	}
	if p.BaseURL != "" {
		p.Opts = append([]option.RequestOption{option.WithBaseURL(p.BaseURL)}, p.Opts...)
	}

	client := openai.NewClient(p.Opts...)
	p.client = &client
	p.initted = true
	p.mu.Unlock()

	var actions []api.Action
	for model, opts := range p.supportedModels {
		actions = append(actions, p.DefineModel(model, opts).(api.Action))
	}

	return actions
}

func (p *reasoningCompatibleProvider) Name() string {
	return p.Provider
}

func (p *reasoningCompatibleProvider) DefineModel(id string, opts ai.ModelOptions) ai.Model {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initted {
		panic("reasoningCompatibleProvider.Init not called")
	}

	return ai.NewModel(api.NewName(p.Provider, id), &opts, func(
		ctx context.Context,
		input *ai.ModelRequest,
		cb func(context.Context, *ai.ModelResponseChunk) error,
	) (*ai.ModelResponse, error) {
		generator := newReasoningModelGenerator(p.client, id).
			WithMessages(input.Messages).
			WithConfig(input.Config).
			WithTools(input.Tools)

		resp, err := generator.Generate(ctx, input, cb)
		if err != nil {
			return nil, err
		}

		return resp, nil
	})
}

type reasoningModelGenerator struct {
	client    *openai.Client
	modelName string
	request   *openai.ChatCompletionNewParams
	messages  []openai.ChatCompletionMessageParamUnion
	tools     []openai.ChatCompletionToolParam
	err       error
}

func newReasoningModelGenerator(client *openai.Client, modelName string) *reasoningModelGenerator {
	return &reasoningModelGenerator{
		client:    client,
		modelName: modelName,
		request: &openai.ChatCompletionNewParams{
			Model: modelName,
		},
	}
}

func (g *reasoningModelGenerator) WithMessages(messages []*ai.Message) *reasoningModelGenerator {
	if g.err != nil || messages == nil {
		return g
	}

	oaiMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case ai.RoleSystem:
			oaiMessages = append(oaiMessages, openai.SystemMessage(concatenateTextContent(msg.Content)))
		case ai.RoleModel:
			assistant := openai.ChatCompletionAssistantMessageParam{}
			if text := concatenateTextContent(msg.Content); text != "" {
				assistant.Content.OfString = param.NewOpt(text)
			}

			toolCalls, err := convertToolCalls(msg.Content)
			if err != nil {
				g.err = err
				return g
			}
			if len(toolCalls) > 0 {
				assistant.ToolCalls = toolCalls
			}

			reasoning := concatenateReasoningContent(msg.Content)
			if len(toolCalls) > 0 || reasoning != "" {
				assistant.SetExtraFields(map[string]any{
					"reasoning":         reasoning,
					"reasoning_content": reasoning,
				})
			}

			oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &assistant,
			})
		case ai.RoleTool:
			for _, p := range msg.Content {
				if !p.IsToolResponse() {
					continue
				}

				toolCallID := p.ToolResponse.Ref
				if toolCallID == "" {
					toolCallID = p.ToolResponse.Name
				}

				toolOutputBytes, err := json.Marshal(p.ToolResponse.Output)
				if err != nil {
					g.err = fmt.Errorf("failed to marshal tool response output: %#v %w", p.ToolResponse.Output, err)
					return g
				}

				oaiMessages = append(oaiMessages, openai.ToolMessage(string(toolOutputBytes), toolCallID))
			}
		case ai.RoleUser:
			parts := []openai.ChatCompletionContentPartUnionParam{}
			for _, p := range msg.Content {
				if p.IsText() {
					parts = append(parts, openai.TextContentPart(p.Text))
				}
				if p.IsMedia() {
					parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
						URL: p.Text,
					}))
				}
			}
			if len(parts) > 0 {
				oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfArrayOfContentParts: parts,
						},
					},
				})
			}
		}
	}

	g.messages = oaiMessages
	return g
}

func (g *reasoningModelGenerator) WithConfig(config any) *reasoningModelGenerator {
	if g.err != nil || config == nil {
		return g
	}

	switch cfg := config.(type) {
	case openai.ChatCompletionNewParams:
		cfg.Model = g.request.Model
		g.request = &cfg
	case *openai.ChatCompletionNewParams:
		copy := *cfg
		copy.Model = g.request.Model
		g.request = &copy
	case map[string]any:
		body, err := json.Marshal(cfg)
		if err != nil {
			g.err = fmt.Errorf("failed to marshal config: %w", err)
			return g
		}
		var openAIConfig openai.ChatCompletionNewParams
		if err := json.Unmarshal(body, &openAIConfig); err != nil {
			g.err = fmt.Errorf("failed to unmarshal config into openai params: %w", err)
			return g
		}
		openAIConfig.Model = g.request.Model
		g.request = &openAIConfig
	default:
		g.err = fmt.Errorf("unexpected config type: %T", config)
	}

	return g
}

func (g *reasoningModelGenerator) WithTools(tools []*ai.ToolDefinition) *reasoningModelGenerator {
	if g.err != nil || tools == nil {
		return g
	}

	toolParams := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, tool := range tools {
		if tool == nil || tool.Name == "" {
			continue
		}

		toolParams = append(toolParams, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        tool.Name,
				Description: openai.String(tool.Description),
				Parameters:  openai.FunctionParameters(tool.InputSchema),
				Strict:      openai.Bool(false),
			},
		})
	}

	if len(toolParams) > 0 {
		g.tools = toolParams
	}

	return g
}

func (g *reasoningModelGenerator) Generate(ctx context.Context, req *ai.ModelRequest, handleChunk func(context.Context, *ai.ModelResponseChunk) error) (*ai.ModelResponse, error) {
	if g.err != nil {
		return nil, g.err
	}
	if len(g.messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	g.request.Messages = g.messages
	if len(g.tools) > 0 {
		g.request.Tools = g.tools
	}
	if req.Output != nil {
		g.request.ResponseFormat = getResponseFormat(req.Output)
	}

	if handleChunk != nil {
		return g.generateStream(ctx, handleChunk)
	}
	return g.generateComplete(ctx, req)
}

func (g *reasoningModelGenerator) generateStream(ctx context.Context, handleChunk func(context.Context, *ai.ModelResponseChunk) error) (*ai.ModelResponse, error) {
	stream := g.client.Chat.Completions.NewStreaming(ctx, *g.request)
	defer stream.Close()

	acc := &openai.ChatCompletionAccumulator{}
	var reasoningBuilder string
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if len(chunk.Choices) == 0 {
			continue
		}

		reasoning, err := firstStringField(
			chunk.Choices[0].Delta.JSON.ExtraFields["reasoning_content"],
			chunk.Choices[0].Delta.JSON.ExtraFields["reasoning"],
		)
		if err != nil {
			return nil, fmt.Errorf("could not parse reasoning chunk: %w", err)
		}
		if reasoning != "" {
			reasoningBuilder += reasoning
		}

		modelChunk := &ai.ModelResponseChunk{}
		if chunk.Choices[0].Delta.Content != "" {
			modelChunk.Content = append(modelChunk.Content, ai.NewTextPart(chunk.Choices[0].Delta.Content))
		}
		for _, toolCall := range chunk.Choices[0].Delta.ToolCalls {
			if toolCall.Function.Name == "" && toolCall.Function.Arguments == "" {
				continue
			}
			modelChunk.Content = append(modelChunk.Content, ai.NewToolRequestPart(&ai.ToolRequest{
				Name:  toolCall.Function.Name,
				Input: toolCall.Function.Arguments,
				Ref:   toolCall.ID,
			}))
		}

		if len(modelChunk.Content) > 0 {
			if err := handleChunk(ctx, modelChunk); err != nil {
				return nil, fmt.Errorf("callback error: %w", err)
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	resp, err := convertChatCompletionToModelResponse(&acc.ChatCompletion)
	if err != nil {
		return nil, err
	}

	if reasoningBuilder != "" && resp.Message != nil && resp.Reasoning() == "" {
		resp.Message.Content = append([]*ai.Part{ai.NewReasoningPart(reasoningBuilder, nil)}, resp.Message.Content...)
	}

	return resp, nil
}

func (g *reasoningModelGenerator) generateComplete(ctx context.Context, req *ai.ModelRequest) (*ai.ModelResponse, error) {
	completion, err := g.client.Chat.Completions.New(ctx, *g.request)
	if err != nil {
		return nil, fmt.Errorf("failed to create completion: %w", err)
	}

	resp, err := convertChatCompletionToModelResponse(completion)
	if err != nil {
		return nil, err
	}

	resp.Request = req
	return resp, nil
}

func getResponseFormat(output *ai.ModelOutputConfig) openai.ChatCompletionNewParamsResponseFormatUnion {
	var format openai.ChatCompletionNewParamsResponseFormatUnion
	if output == nil {
		return format
	}

	switch output.Format {
	case "json":
		if output.Schema != nil {
			jsonSchemaParam := shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "output",
					Schema: output.Schema,
					Strict: openai.Bool(true),
				},
			}
			format.OfJSONSchema = &jsonSchemaParam
		} else {
			jsonObjectParam := shared.NewResponseFormatJSONObjectParam()
			format.OfJSONObject = &jsonObjectParam
		}
	case "text":
		textParam := shared.NewResponseFormatTextParam()
		format.OfText = &textParam
	}

	return format
}

func convertChatCompletionToModelResponse(completion *openai.ChatCompletion) (*ai.ModelResponse, error) {
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no choices in completion")
	}

	choice := completion.Choices[0]
	usage := &ai.GenerationUsage{
		InputTokens:  int(completion.Usage.PromptTokens),
		OutputTokens: int(completion.Usage.CompletionTokens),
		TotalTokens:  int(completion.Usage.TotalTokens),
	}
	if completion.Usage.CompletionTokensDetails.ReasoningTokens > 0 {
		usage.ThoughtsTokens = int(completion.Usage.CompletionTokensDetails.ReasoningTokens)
	}
	if completion.Usage.PromptTokensDetails.CachedTokens > 0 {
		usage.CachedContentTokens = int(completion.Usage.PromptTokensDetails.CachedTokens)
	}

	resp := &ai.ModelResponse{
		Request: &ai.ModelRequest{},
		Usage:   usage,
		Message: &ai.Message{
			Role:    ai.RoleModel,
			Content: make([]*ai.Part, 0),
		},
	}

	switch choice.FinishReason {
	case "stop", "tool_calls":
		resp.FinishReason = ai.FinishReasonStop
	case "length":
		resp.FinishReason = ai.FinishReasonLength
	case "content_filter":
		resp.FinishReason = ai.FinishReasonBlocked
	case "function_call":
		resp.FinishReason = ai.FinishReasonOther
	default:
		resp.FinishReason = ai.FinishReasonUnknown
	}

	if choice.Message.Refusal != "" {
		resp.FinishMessage = choice.Message.Refusal
		resp.FinishReason = ai.FinishReasonBlocked
	}

	reasoning, err := firstStringField(
		choice.Message.JSON.ExtraFields["reasoning_content"],
		choice.Message.JSON.ExtraFields["reasoning"],
	)
	if err != nil {
		return nil, fmt.Errorf("could not parse reasoning field: %w", err)
	}
	if reasoning != "" {
		resp.Message.Content = append(resp.Message.Content, ai.NewReasoningPart(reasoning, nil))
	}

	if choice.Message.Content != "" {
		resp.Message.Content = append(resp.Message.Content, ai.NewTextPart(choice.Message.Content))
	}

	for _, toolCall := range choice.Message.ToolCalls {
		var args map[string]any
		err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
		if err != nil {
			return nil, fmt.Errorf("could not parse tool args: %w", err)
		}
		resp.Message.Content = append(resp.Message.Content, ai.NewToolRequestPart(&ai.ToolRequest{
			Ref:   toolCall.ID,
			Name:  toolCall.Function.Name,
			Input: args,
		}))
	}

	if completion.SystemFingerprint != "" {
		resp.Custom = map[string]any{
			"systemFingerprint": completion.SystemFingerprint,
			"model":             completion.Model,
			"id":                completion.ID,
		}
	}

	return resp, nil
}

func concatenateTextContent(parts []*ai.Part) string {
	content := ""
	for _, part := range parts {
		if part.IsText() {
			content += part.Text
		}
	}
	return content
}

func concatenateReasoningContent(parts []*ai.Part) string {
	content := ""
	for _, part := range parts {
		if part.IsReasoning() {
			content += part.Text
		}
	}
	return content
}

func convertToolCalls(content []*ai.Part) ([]openai.ChatCompletionMessageToolCallParam, error) {
	var toolCalls []openai.ChatCompletionMessageToolCallParam
	for _, p := range content {
		if !p.IsToolRequest() {
			continue
		}
		toolCall, err := convertToolCall(p)
		if err != nil {
			return nil, err
		}
		toolCalls = append(toolCalls, *toolCall)
	}
	return toolCalls, nil
}

func convertToolCall(part *ai.Part) (*openai.ChatCompletionMessageToolCallParam, error) {
	toolCallID := part.ToolRequest.Ref
	if toolCallID == "" {
		toolCallID = part.ToolRequest.Name
	}

	param := &openai.ChatCompletionMessageToolCallParam{
		ID: toolCallID,
		Function: openai.ChatCompletionMessageToolCallFunctionParam{
			Name: part.ToolRequest.Name,
		},
	}

	if part.ToolRequest.Input != nil {
		args, err := json.Marshal(part.ToolRequest.Input)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool request input: %#v %w", part.ToolRequest.Input, err)
		}
		param.Function.Arguments = string(args)
	}

	return param, nil
}

func stringField(field respjson.Field) (string, error) {
	if field.Raw() == "" || field.Raw() == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal([]byte(field.Raw()), &value); err != nil {
		return "", err
	}
	return value, nil
}

func firstStringField(fields ...respjson.Field) (string, error) {
	for _, field := range fields {
		value, err := stringField(field)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
	}
	return "", nil
}
