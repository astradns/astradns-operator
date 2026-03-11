package unbound

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/astradns/astradns-types/engine"
	typesengineconfig "github.com/astradns/astradns-types/engineconfig"
)

// UnboundRenderer renders EngineConfig as an Unbound configuration file.
type UnboundRenderer struct{}

var _ typesengineconfig.ConfigRenderer = (*UnboundRenderer)(nil)

func init() {
	typesengineconfig.RegisterRenderer(engine.EngineUnbound, func() typesengineconfig.ConfigRenderer {
		return &UnboundRenderer{}
	})
}

// Render produces an unbound.conf string from EngineConfig.
func (r *UnboundRenderer) Render(config *engine.EngineConfig) (string, error) {
	if config == nil {
		return "", errors.New("engine config is required")
	}

	normalized := normalizeConfig(*config)
	data := engine.NewTemplateData(normalized)
	tmpl, err := template.New("unbound.conf").Parse(engine.UnboundConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("parse unbound template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("render unbound template: %w", err)
	}

	return rendered.String(), nil
}

// EngineType returns the Unbound engine type.
func (r *UnboundRenderer) EngineType() engine.EngineType {
	return engine.EngineUnbound
}

// ConfigFileName returns the Unbound config filename.
func (r *UnboundRenderer) ConfigFileName() string {
	return "unbound.conf"
}

func normalizeConfig(config engine.EngineConfig) engine.EngineConfig {
	normalized := config
	normalized.Upstreams = make([]engine.UpstreamConfig, len(config.Upstreams))
	copy(normalized.Upstreams, config.Upstreams)

	for i := range normalized.Upstreams {
		if normalized.Upstreams[i].Port == 0 {
			normalized.Upstreams[i].Port = 53
		}
	}

	return normalized
}
