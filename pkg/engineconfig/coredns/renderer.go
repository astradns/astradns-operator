package coredns

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/astradns/astradns-types/engine"
	typesengineconfig "github.com/astradns/astradns-types/engineconfig"
)

// CoreDNSRenderer renders EngineConfig as a CoreDNS Corefile.
type CoreDNSRenderer struct{}

var _ typesengineconfig.ConfigRenderer = (*CoreDNSRenderer)(nil)

func init() {
	typesengineconfig.RegisterRenderer(engine.EngineCoreDNS, func() typesengineconfig.ConfigRenderer {
		return &CoreDNSRenderer{}
	})
}

// Render produces a Corefile string from EngineConfig.
func (r *CoreDNSRenderer) Render(config *engine.EngineConfig) (string, error) {
	if config == nil {
		return "", errors.New("engine config is required")
	}

	data := engine.NewTemplateData(*config)
	tmpl, err := template.New("Corefile").Parse(engine.CorefileTemplate)
	if err != nil {
		return "", fmt.Errorf("parse coredns template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("render coredns template: %w", err)
	}

	return rendered.String(), nil
}

// EngineType returns the CoreDNS engine type.
func (r *CoreDNSRenderer) EngineType() engine.EngineType {
	return engine.EngineCoreDNS
}

// ConfigFileName returns the CoreDNS config filename.
func (r *CoreDNSRenderer) ConfigFileName() string {
	return "Corefile"
}
