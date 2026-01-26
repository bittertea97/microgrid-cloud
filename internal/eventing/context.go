package eventing

import "context"

type contextKey string

const (
	contextKeyEnvelope contextKey = "eventing.envelope"
	contextKeyTenant   contextKey = "eventing.tenant_id"
	contextKeyCorr     contextKey = "eventing.correlation_id"
	contextKeyEventID  contextKey = "eventing.event_id"
)

// WithEnvelope attaches envelope metadata to context.
func WithEnvelope(ctx context.Context, env Envelope) context.Context {
	return context.WithValue(ctx, contextKeyEnvelope, env)
}

// EnvelopeFromContext returns envelope metadata if available.
func EnvelopeFromContext(ctx context.Context) (Envelope, bool) {
	value := ctx.Value(contextKeyEnvelope)
	env, ok := value.(Envelope)
	return env, ok
}

// WithTenantID sets tenant id in context.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, contextKeyTenant, tenantID)
}

// WithCorrelationID sets correlation id in context.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, contextKeyCorr, correlationID)
}

// WithEventID sets event id in context.
func WithEventID(ctx context.Context, eventID string) context.Context {
	return context.WithValue(ctx, contextKeyEventID, eventID)
}

// MetaFromContext builds metadata from context with defaults.
func MetaFromContext(ctx context.Context, defaultTenantID string) Meta {
	meta := Meta{}
	if value := ctx.Value(contextKeyTenant); value != nil {
		if tenantID, ok := value.(string); ok {
			meta.TenantID = tenantID
		}
	}
	if meta.TenantID == "" {
		meta.TenantID = defaultTenantID
	}
	if value := ctx.Value(contextKeyCorr); value != nil {
		if corr, ok := value.(string); ok {
			meta.CorrelationID = corr
		}
	}
	if value := ctx.Value(contextKeyEventID); value != nil {
		if id, ok := value.(string); ok {
			meta.EventID = id
		}
	}
	return meta
}
