package masterdata

// Semantic defines a normalized metric meaning for telemetry points.
type Semantic string

const (
	SemanticChargePowerKW    Semantic = "charge_power_kw"
	SemanticDischargePowerKW Semantic = "discharge_power_kw"
	SemanticEarnings         Semantic = "earnings"
	SemanticCarbonReduction  Semantic = "carbon_reduction"
	SemanticGridExportKW     Semantic = "grid_export_kw"
)
