package powerdns

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/astradns/astradns-types/engine"
	typesengineconfig "github.com/astradns/astradns-types/engineconfig"
)

// PowerDNSRenderer renders EngineConfig as a PowerDNS Recursor configuration file.
type PowerDNSRenderer struct{}

var _ typesengineconfig.ConfigRenderer = (*PowerDNSRenderer)(nil)

func init() {
	typesengineconfig.RegisterRenderer(engine.EnginePowerDNS, func() typesengineconfig.ConfigRenderer {
		return &PowerDNSRenderer{}
	})
}

// Render produces a recursor.conf string from EngineConfig.
func (r *PowerDNSRenderer) Render(config *engine.EngineConfig) (string, error) {
	if config == nil {
		return "", errors.New("engine config is required")
	}

	data := engine.NewTemplateData(*config)
	tmpl, err := template.New("recursor.conf").Parse(engine.RecursorConfTemplate)
	if err != nil {
		return "", fmt.Errorf("parse powerdns template: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("render powerdns template: %w", err)
	}

	return rendered.String(), nil
}

// EngineType returns the PowerDNS engine type.
func (r *PowerDNSRenderer) EngineType() engine.EngineType {
	return engine.EnginePowerDNS
}

// ConfigFileName returns the PowerDNS config filename.
func (r *PowerDNSRenderer) ConfigFileName() string {
	return "recursor.conf"
}
