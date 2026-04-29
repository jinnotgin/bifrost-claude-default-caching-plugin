package main

import (
	"encoding/json"
	"fmt"
	"strings"

	anthropic "github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/schemas"
)

const pluginName = "claude-cache-control-plugin"

type Config struct {
	Enabled               bool                    `json:"enabled"`
	Providers             []schemas.ModelProvider `json:"providers,omitempty"`
	ModelSubstrings       []string                `json:"model_substrings,omitempty"`
	CacheControlType      string                  `json:"cache_control_type,omitempty"`
	TTL                   *string                 `json:"ttl,omitempty"`
	Scope                 *string                 `json:"scope,omitempty"`
	SkipIfRequestHasCache bool                    `json:"skip_if_request_has_cache,omitempty"`
	SkipIfRawRequestBody  bool                    `json:"skip_if_raw_request_body,omitempty"`
	StripScopeOnVertex    bool                    `json:"strip_scope_on_vertex,omitempty"`
	Debug                 bool                    `json:"debug,omitempty"`
}

var cfg = defaultConfig()

func defaultConfig() Config {
	return Config{
		Enabled:               true,
		Providers:             []schemas.ModelProvider{schemas.Anthropic, schemas.Vertex},
		ModelSubstrings:       []string{"claude", "anthropic/claude", "vertex/claude"},
		CacheControlType:      "ephemeral",
		SkipIfRequestHasCache: true,
		SkipIfRawRequestBody:  true,
		StripScopeOnVertex:    true,
		Debug:                 false,
	}
}

func Init(config any) error {
	cfg = defaultConfig()
	if config == nil {
		return nil
	}

	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal plugin config: %w", err)
	}

	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("unmarshal plugin config: %w", err)
	}

	if strings.TrimSpace(cfg.CacheControlType) == "" {
		cfg.CacheControlType = "ephemeral"
	}

	if len(cfg.Providers) == 0 {
		cfg.Providers = []schemas.ModelProvider{schemas.Anthropic, schemas.Vertex}
	}

	if len(cfg.ModelSubstrings) == 0 {
		cfg.ModelSubstrings = []string{"claude", "anthropic/claude", "vertex/claude"}
	}

	return nil
}

func GetName() string {
	return pluginName
}

func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if req == nil || !cfg.Enabled {
		return req, nil, nil
	}

	if req.ResponsesRequest != nil {
		return handleResponsesRequest(ctx, req)
	}

	if req.ChatRequest != nil {
		return handleChatRequest(ctx, req)
	}

	return req, nil, nil
}

func handleResponsesRequest(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	rr := req.ResponsesRequest
	if rr == nil {
		return req, nil, nil
	}

	if !providerAllowed(rr.Provider) || !modelAllowed(rr.Model) {
		return req, nil, nil
	}

	if cfg.SkipIfRawRequestBody && len(rr.RawRequestBody) > 0 {
		debug(ctx, "skipping responses request because RawRequestBody is already set")
		return req, nil, nil
	}

	anthropicReq, err := anthropic.ToAnthropicResponsesRequest(ctx, rr)
	if err != nil {
		return req, nil, fmt.Errorf("convert responses request to anthropic request: %w", err)
	}

	if cfg.SkipIfRequestHasCache && anthropicReq.CacheControl != nil {
		debug(ctx, "skipping responses request because top-level cache_control already exists after conversion")
		return req, nil, nil
	}

	anthropicReq.CacheControl = newCacheControl()

	if rr.Provider == schemas.Vertex && cfg.StripScopeOnVertex {
		anthropicReq.SetStripCacheControlScope(true)
	}

	rawBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return req, nil, fmt.Errorf("marshal anthropic request body: %w", err)
	}

	rr.RawRequestBody = rawBody
	ctx.SetValue(schemas.BifrostContextKeyUseRawRequestBody, true)

	debug(ctx, fmt.Sprintf("injected top-level cache_control for responses provider=%s model=%s", rr.Provider, rr.Model))
	return req, nil, nil
}

func handleChatRequest(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	cr := req.ChatRequest
	if cr == nil {
		return req, nil, nil
	}

	if !providerAllowed(cr.Provider) || !modelAllowed(cr.Model) {
		return req, nil, nil
	}

	if cfg.SkipIfRawRequestBody && len(cr.RawRequestBody) > 0 {
		debug(ctx, "skipping chat request because RawRequestBody is already set")
		return req, nil, nil
	}

	if cr.Params == nil {
		cr.Params = &schemas.ChatParameters{}
	}

	if cfg.SkipIfRequestHasCache && cr.Params.CacheControl != nil {
		debug(ctx, "skipping chat request because params.cache_control already exists")
		return req, nil, nil
	}

	// For /v1/chat/completions, do not set RawRequestBody here.
	// Bifrost's Anthropic-family chat converter already maps
	// ChatParameters.CacheControl onto the final top-level Anthropic
	// cache_control field.
	cr.Params.CacheControl = newCacheControl()

	debug(ctx, fmt.Sprintf("injected top-level cache_control for chat provider=%s model=%s", cr.Provider, cr.Model))
	return req, nil, nil
}

func PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

func Cleanup() error {
	return nil
}

func newCacheControl() *schemas.CacheControl {
	return &schemas.CacheControl{
		Type:  schemas.CacheControlType(cfg.CacheControlType),
		TTL:   cfg.TTL,
		Scope: cfg.Scope,
	}
}

func providerAllowed(provider schemas.ModelProvider) bool {
	for _, allowed := range cfg.Providers {
		if provider == allowed {
			return true
		}
	}
	return false
}

func modelAllowed(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	if lower == "" {
		return false
	}

	if schemas.IsAnthropicModel(lower) {
		return true
	}

	for _, fragment := range cfg.ModelSubstrings {
		if strings.Contains(lower, strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

// For Bifrost 1.5 onwards, you can switch to ctx.Log:
//
// func debug(ctx *schemas.BifrostContext, msg string) {
// 	if !cfg.Debug || ctx == nil {
// 		return
// 	}
// 	ctx.Log(schemas.LogLevelInfo, msg)
// }

func debug(ctx *schemas.BifrostContext, msg string) {
	if !cfg.Debug {
		return
	}
	// BifrostContext.Log exists in Bifrost v1.5.x+, but not v1.4.x.
	// Use stdout logging so this plugin remains compatible with v1.4.x.
	fmt.Printf("[%s] %s\n", pluginName, msg)
}
