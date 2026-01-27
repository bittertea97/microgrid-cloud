package notify

import (
	"bytes"
	"errors"
	"text/template"
)

const DefaultTemplate = `[Alarm {{.EventLabel}}]
Station: {{.Station}}
Rule: {{.Rule}}
Trigger Value: {{.TriggerValue}}
Threshold: {{.Threshold}}
Start Time: {{.StartTime}}
Current Status: {{.Status}}
Severity: {{.Severity}}
Suggestion: {{.Suggestion}}
{{ if .ReportURL }}
Report: {{.ReportURL}}
{{ end }}`

// TemplateData provides fields for rendering notification content.
type TemplateData struct {
	Station      string
	StationID    string
	Rule         string
	RuleID       string
	TriggerValue string
	Threshold    string
	StartTime    string
	Status       string
	StatusCode   string
	Severity     string
	Suggestion   string
	ReportURL    string
	Event        string
	EventLabel   string
}

// Template renders notification content.
type Template struct {
	tpl *template.Template
}

// NewTemplate parses a notification template, falling back to DefaultTemplate.
func NewTemplate(tpl string) (*Template, error) {
	if tpl == "" {
		tpl = DefaultTemplate
	}
	parsed, err := template.New("alarm-notification").Parse(tpl)
	if err != nil {
		return nil, err
	}
	return &Template{tpl: parsed}, nil
}

// Render applies the template to data.
func (t *Template) Render(data TemplateData) (string, error) {
	if t == nil || t.tpl == nil {
		return "", errors.New("alarm template: nil")
	}
	var buf bytes.Buffer
	if err := t.tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
