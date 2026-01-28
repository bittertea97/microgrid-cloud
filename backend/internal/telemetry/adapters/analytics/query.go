package analytics

import (
	"context"
	"errors"
	"time"

	"microgrid-cloud/internal/analytics/application"
	"microgrid-cloud/internal/analytics/domain/statistic"
	masterdata "microgrid-cloud/internal/masterdata/domain"
	telemetry "microgrid-cloud/internal/telemetry/domain"
)

// QueryAdapter adapts telemetry queries to analytics application queries.
type QueryAdapter struct {
	tenantID string
	query    telemetry.TelemetryQuery
	mappings masterdata.PointMappingRepository
}

// NewQueryAdapter constructs the adapter for a single tenant.
func NewQueryAdapter(tenantID string, query telemetry.TelemetryQuery, mappings masterdata.PointMappingRepository) (*QueryAdapter, error) {
	if tenantID == "" {
		return nil, errors.New("telemetry query adapter: empty tenant id")
	}
	if query == nil {
		return nil, errors.New("telemetry query adapter: nil query")
	}
	if mappings == nil {
		return nil, errors.New("telemetry query adapter: nil mapping repository")
	}
	return &QueryAdapter{tenantID: tenantID, query: query, mappings: mappings}, nil
}

// QueryHour returns analytics telemetry points within [start, end).
func (a *QueryAdapter) QueryHour(ctx context.Context, stationID string, start, end time.Time) ([]application.TelemetryPoint, error) {
	if a == nil {
		return nil, errors.New("telemetry query adapter: nil adapter")
	}
	mappingByPoint, err := a.loadMappings(ctx, stationID)
	if err != nil {
		return nil, err
	}

	points, err := a.query.QueryHour(ctx, a.tenantID, stationID, start, end)
	if err != nil {
		return nil, err
	}

	result := make([]application.TelemetryPoint, 0, len(points))
	for _, point := range points {
		semanticValues := make(map[string]float64)
		for key, value := range point.Values {
			mapping, ok := mappingByPoint[key]
			if !ok {
				continue
			}
			semanticValues[mapping.Semantic] += value * mapping.Factor
		}

		result = append(result, application.TelemetryPoint{
			At:               point.At,
			ChargePowerKW:    semanticValues[string(masterdata.SemanticChargePowerKW)],
			DischargePowerKW: semanticValues[string(masterdata.SemanticDischargePowerKW)],
			Earnings:         semanticValues[string(masterdata.SemanticEarnings)],
			CarbonReduction:  semanticValues[string(masterdata.SemanticCarbonReduction)],
		})
	}
	return result, nil
}

// SumStatisticCalculator sums telemetry values into a statistic fact.
type SumStatisticCalculator struct{}

// CalculateHour sums telemetry points into a statistic fact.
func (SumStatisticCalculator) CalculateHour(ctx context.Context, stationID string, periodStart time.Time, telemetryPoints []application.TelemetryPoint) (statistic.StatisticFact, error) {
	_ = ctx
	_ = stationID
	_ = periodStart

	var fact statistic.StatisticFact
	for _, point := range telemetryPoints {
		fact.ChargeKWh += point.ChargePowerKW
		fact.DischargeKWh += point.DischargePowerKW
		fact.Earnings += point.Earnings
		fact.CarbonReduction += point.CarbonReduction
	}
	return fact, nil
}

type mappedPoint struct {
	Semantic string
	Unit     string
	Factor   float64
	DeviceID string
}

func (a *QueryAdapter) loadMappings(ctx context.Context, stationID string) (map[string]mappedPoint, error) {
	if a.mappings == nil {
		return nil, errors.New("telemetry query adapter: nil mapping repository")
	}
	list, err := a.mappings.ListByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]mappedPoint)
	for _, item := range list {
		if item.PointKey == "" || item.Semantic == "" {
			continue
		}
		if item.DeviceID != "" {
			continue
		}
		result[item.PointKey] = mappedPoint{
			Semantic: item.Semantic,
			Unit:     item.Unit,
			Factor:   item.Factor,
			DeviceID: item.DeviceID,
		}
	}
	if len(result) == 0 {
		return nil, errors.New("telemetry query adapter: no point mappings for station")
	}
	return result, nil
}
