package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"time"
)

const (
	modelsPath = "/v1/models"
	propsPath  = "/props"
	modelParam = "model"
	// The status is shown while people work: a server that does not answer quickly counts as unreachable.
	statusTimeout = 3 * time.Second
	loaded        = "loaded"
	imageModality = "image"
)

// servedModels is /v1/models; in router mode, each model also says whether it is loaded, and what it takes in,
// loaded or not (pictures when it has a vision projector, --mmproj). vLLM says how long a conversation can be.
type servedModels struct {
	Data []struct {
		ID          string `json:"id"`
		MaxModelLen int    `json:"max_model_len"`
		Status      *struct {
			Value string `json:"value"`
		} `json:"status"`
		Architecture *struct {
			InputModalities []string `json:"input_modalities"`
		} `json:"architecture"`
	} `json:"data"`
}

// lengthOf is the longest conversation a model takes, as vLLM lists it: the only model's when none is named.
func (m servedModels) lengthOf(model string) int {
	for _, served := range m.Data {
		if served.ID == model || (model == "" && len(m.Data) == 1) {
			return served.MaxModelLen
		}
	}
	return 0
}

// visionOf tells whether a router's model takes pictures; known is false when the list does not say, as a
// single-model llm-server's, whose /props does.
func (m servedModels) visionOf(model string) (sees, known bool) {
	for _, served := range m.Data {
		if served.ID == model && served.Architecture != nil {
			return slices.Contains(served.Architecture.InputModalities, imageModality), true
		}
	}
	return false, false
}

// props is /props: a slot's context, the slots, and what else than text the model takes (pictures once
// llm-server loaded a vision projector, --mmproj, for it).
type props struct {
	DefaultGenerationSettings *struct {
		NCtx int `json:"n_ctx"`
	} `json:"default_generation_settings"`
	Modalities *struct {
		Vision bool `json:"vision"`
	} `json:"modalities"`
	NCtx       int `json:"n_ctx"`
	TotalSlots int `json:"total_slots"`
}

// context is a slot's context; a router asked without a model reports 0: no slot to measure, so none known.
func (p *props) context() int {
	if p == nil {
		return 0
	}
	if p.DefaultGenerationSettings != nil && p.DefaultGenerationSettings.NCtx > 0 {
		return p.DefaultGenerationSettings.NCtx
	}
	return max(0, p.NCtx)
}

func (p *props) vision() bool {
	return p != nil && p.Modalities != nil && p.Modalities.Vision
}

// firstLoaded is a router's first loaded model, which wants requests to name one and loads an unloaded one when
// asked: preferably not the one to avoid, another agent's (the list is in alphabetical order, not by size).
func firstLoaded(models servedModels, avoid string) string {
	first := ""
	for _, model := range models.Data {
		if model.Status == nil || model.Status.Value != loaded {
			continue
		}
		if model.ID != avoid {
			return model.ID
		}
		first = firstNonEmpty(first, model.ID)
	}
	return first
}

// StatusReader asks llm-server which models it serves and how long a slot's context is.
type StatusReader struct {
	options Options
}

func NewStatusReader(options Options) *StatusReader {
	return &StatusReader{options: options}
}

// Models is the names of the models llm-server serves, and whether it answers.
func (s *StatusReader) Models(ctx context.Context) ([]string, bool) {
	models, ok := s.models(ctx)
	if !ok {
		return []string{}, false
	}
	ids := []string{}
	for _, model := range models.Data {
		ids = append(ids, model.ID)
	}
	return ids, true
}

// FirstLoaded is the model to ask when none is named: the configured one, else in router mode the first loaded one
// other than avoid. A single-model llm-server needs none.
func (s *StatusReader) FirstLoaded(ctx context.Context, avoid string) string {
	if s.options.Model != "" {
		return s.options.Model
	}
	models, ok := s.models(ctx)
	if !ok {
		return ""
	}
	return firstLoaded(models, avoid)
}

// SlotsOf is how many requests the model answers at once, when llm-server says (0 when not).
func (s *StatusReader) SlotsOf(ctx context.Context, model string) int {
	found := s.props(ctx, model)
	if found == nil {
		return 0
	}
	return found.TotalSlots
}

// ContextOf is how many tokens a conversation of the model holds, when the server says (0 when not): llama.cpp in
// /props, a slot's; vLLM in /v1/models.
func (s *StatusReader) ContextOf(ctx context.Context, model string) int {
	if found := s.props(ctx, model).context(); found > 0 {
		return found
	}
	models, _ := s.models(ctx)
	return models.lengthOf(firstNonEmpty(model, s.options.Model))
}

// VisionOf tells whether the model sees pictures, as a router lists it whether it is loaded or not (unloaded,
// loading or asleep, as at the start of a turn that will load it), else as llm-server's /props says once it loaded
// a vision projector.
func (s *StatusReader) VisionOf(ctx context.Context, model string) bool {
	models, _ := s.models(ctx)
	if sees, known := models.visionOf(firstNonEmpty(model, s.options.Model)); known {
		return sees
	}
	return s.props(ctx, model).vision()
}

// props is a model's settings, as /props gives them. Never for a router's model that is not loaded: a router loads
// the model a request names, unloading another, so a status check naming a model it does not serve right now (an
// old setting, say) would keep swapping models in and out.
func (s *StatusReader) props(ctx context.Context, model string) *props {
	name := firstNonEmpty(model, s.options.Model)
	if name != "" && !s.isLoaded(ctx, name) {
		return nil
	}
	path := propsPath
	if name != "" {
		path += "?" + url.Values{modelParam: {name}}.Encode()
	}
	var found props
	if !s.getJSON(ctx, path, &found) {
		return nil
	}
	return &found
}

// isLoaded tells whether a model is loaded: a single-model server's, or one it does not list, counts as loaded.
func (s *StatusReader) isLoaded(ctx context.Context, model string) bool {
	models, _ := s.models(ctx)
	for _, served := range models.Data {
		if served.ID == model {
			return served.Status == nil || served.Status.Value == loaded
		}
	}
	return true
}

func (s *StatusReader) models(ctx context.Context) (servedModels, bool) {
	var models servedModels
	return models, s.getJSON(ctx, modelsPath, &models)
}

func (s *StatusReader) getJSON(ctx context.Context, path string, target any) bool {
	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	request, err := s.options.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return false
	}
	response, err := s.options.client().Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode < http.StatusBadRequest && json.NewDecoder(response.Body).Decode(target) == nil
}
