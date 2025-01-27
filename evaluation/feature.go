package evaluation

import (
	"encoding/json"

	"github.com/drone/ff-golang-server-sdk.v0/types"

	"reflect"
	"strconv"
)

const (
	segmentMatchOperator   = "segmentMatch"
	inOperator             = "in"
	equalOperator          = "equal"
	gtOperator             = "gt"
	startsWithOperator     = "starts_with"
	endsWithOperator       = "ends_with"
	containsOperator       = "contains"
	equalSensitiveOperator = "equal_sensitive"
)

// Evaluation object is in most cases returned value from evaluation
// methods and contains results of evaluated feature flag for target object
type Evaluation struct {
	Flag  string
	Value interface{}
}

// Clause object
type Clause struct {
	Attribute string
	ID        string
	Negate    bool
	Op        string
	Value     []string
}

// Evaluate clause using target but it can be used also with segments if Op field is segmentMach
func (c *Clause) Evaluate(target *Target, segments Segments, operator types.ValueType) bool {
	switch c.Op {
	case segmentMatchOperator:
		if segments == nil {
			return false
		}
		return segments.Evaluate(target)
	case inOperator:
		return operator.In(c.Value)
	case equalOperator:
		return operator.Equal(c.Value)
	case gtOperator:
		return operator.GreaterThan(c.Value)
	case startsWithOperator:
		return operator.StartsWith(c.Value)
	case endsWithOperator:
		return operator.EndsWith(c.Value)
	case containsOperator:
		return operator.Contains(c.Value)
	case equalSensitiveOperator:
		return operator.EqualSensitive(c.Value)
	}
	return false
}

// Clauses slice
type Clauses []Clause

// Evaluate clauses using target but it can be used also with segments if Op field is segmentMach
func (c Clauses) Evaluate(target *Target, segments Segments) bool {
	// AND operation
	for _, clause := range c {
		// operator should be evaluated based on type of attribute
		op := target.GetOperator(clause.Attribute)
		if !clause.Evaluate(target, segments, op) {
			return false
		}
		// continue on next clause
	}
	// it means that all clauses passed
	return true
}

// Distribution object used for Percentage Rollout evaluations
type Distribution struct {
	BucketBy   string
	Variations []WeightedVariation
}

// GetKeyName returns variation identifier based on target
func (d *Distribution) GetKeyName(target *Target) string {
	variation := ""
	for _, tdVariation := range d.Variations {
		variation = tdVariation.Variation
		if d.isEnabled(target, tdVariation.Weight) {
			return variation
		}
	}
	// distance between last variation and total percentage
	if d.isEnabled(target, OneHundred) {
		return variation
	}
	return ""
}

func (d *Distribution) isEnabled(target *Target, percentage int) bool {
	value := target.GetAttrValue(d.BucketBy)
	identifier := value.String()
	if identifier == "" {
		return false
	}

	bucketID := GetNormalizedNumber(identifier, d.BucketBy)
	return percentage > 0 && bucketID <= percentage
}

// FeatureConfig object is actually where feature flag evaluation
// happens. It contains all data like rules, default values, variations and segments
type FeatureConfig struct {
	DefaultServe         Serve
	Environment          string
	Feature              string
	Kind                 string
	OffVariation         string
	Prerequisites        []Prerequisite
	Project              string
	Rules                ServingRules
	State                FeatureState
	VariationToTargetMap []VariationMap
	Variations           Variations
	Segments             map[string]*Segment `json:"-"`
}

// GetSegmentIdentifiers returns all segments
func (fc FeatureConfig) GetSegmentIdentifiers() StrSlice {
	slice := make(StrSlice, 0)
	for _, rule := range fc.Rules {
		for _, clause := range rule.Clauses {
			if clause.Op == segmentMatchOperator {
				for _, val := range clause.Value {
					slice = append(slice, val)
				}
			}
		}
	}
	return slice
}

// GetKind returns kind of feature flag
func (fc *FeatureConfig) GetKind() reflect.Kind {
	switch fc.Kind {
	case "boolean":
		return reflect.Bool
	case "string":
		return reflect.String
	case "int", "integer":
		return reflect.Int
	case "number":
		return reflect.Float64
	case "json":
		return reflect.Map
	default:
		return reflect.Invalid
	}
}

// Evaluate feature flag and return Evaluation object
func (fc FeatureConfig) Evaluate(target *Target) *Evaluation {
	var value interface{}
	switch fc.GetKind() {
	case reflect.Bool:
		value = fc.BoolVariation(target, false) // need more info
	case reflect.String:
		value = fc.StringVariation(target, "") // need more info
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int8, reflect.Uint, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Uint8:
		value = fc.IntVariation(target, 0)
	case reflect.Float64, reflect.Float32:
		value = fc.NumberVariation(target, 0)
	case reflect.Map:
		value = fc.JSONVariation(target, map[string]interface{}{})
	case reflect.Array, reflect.Chan, reflect.Complex128, reflect.Complex64, reflect.Func, reflect.Interface,
		reflect.Invalid, reflect.Ptr, reflect.Slice, reflect.Struct, reflect.Uintptr, reflect.UnsafePointer:
		return nil
	default:
		value = nil
	}
	return &Evaluation{
		Flag:  fc.Feature,
		Value: value,
	}
}

// GetVariationName returns variation identifier for target
func (fc FeatureConfig) GetVariationName(target *Target) string {
	if fc.State == FeatureStateOff {
		return fc.OffVariation
	}
	// TODO: variation to target
	if fc.VariationToTargetMap != nil && len(fc.VariationToTargetMap) > 0 {
		for _, variationMap := range fc.VariationToTargetMap {
			if variationMap.Targets != nil {
				for _, t := range variationMap.Targets {
					if target.Identifier == t {
						return variationMap.Variation
					}
				}
			}
		}
	}

	return fc.Rules.GetVariationName(target, fc.Segments, fc.DefaultServe)

}

// BoolVariation returns boolean evaluation for target
func (fc *FeatureConfig) BoolVariation(target *Target, defaultValue bool) bool {
	if fc.GetKind() != reflect.Bool {
		return defaultValue
	}
	return fc.Variations.FindByIdentifier(fc.GetVariationName(target)).Bool(defaultValue)
}

// StringVariation returns string evaluation for target
func (fc *FeatureConfig) StringVariation(target *Target, defaultValue string) string {
	if fc.GetKind() != reflect.String {
		return defaultValue
	}
	return fc.Variations.FindByIdentifier(fc.GetVariationName(target)).String(defaultValue)
}

// IntVariation returns int evaluation for target
func (fc *FeatureConfig) IntVariation(target *Target, defaultValue int64) int64 {
	if fc.GetKind() != reflect.Int {
		return defaultValue
	}
	return fc.Variations.FindByIdentifier(fc.GetVariationName(target)).Int(defaultValue)
}

// NumberVariation returns number evaluation for target
func (fc *FeatureConfig) NumberVariation(target *Target, defaultValue float64) float64 {
	if fc.GetKind() != reflect.Float64 {
		return defaultValue
	}
	return fc.Variations.FindByIdentifier(fc.GetVariationName(target)).Number(defaultValue)
}

// JSONVariation returns json evaluation for target
func (fc *FeatureConfig) JSONVariation(target *Target, defaultValue types.JSON) types.JSON {
	if fc.GetKind() != reflect.Map {
		return defaultValue
	}
	return fc.Variations.FindByIdentifier(fc.GetVariationName(target)).JSON(defaultValue)
}

// FeatureState represents feature flag ON or OFF state
type FeatureState string

const (
	// FeatureStateOff represents OFF state
	FeatureStateOff FeatureState = "off"
	// FeatureStateOn represents ON state
	FeatureStateOn FeatureState = "on"
)

// Prerequisite object
type Prerequisite struct {
	Feature    string
	Variations []string
}

// Serve object
type Serve struct {
	Distribution *Distribution
	Variation    *string
}

// ServingRule object
type ServingRule struct {
	Clauses  Clauses
	Priority int
	RuleID   string
	Serve    Serve
}

// ServingRules slice of ServingRule
type ServingRules []ServingRule

// GetVariationName returns variation identifier or defaultServe
func (sr ServingRules) GetVariationName(target *Target, segments Segments, defaultServe Serve) string {
RULES:
	for _, rule := range sr {
		// rules are OR operation
		if !rule.Clauses.Evaluate(target, segments) {
			continue RULES
		}
		var name string
		if rule.Serve.Variation != nil {
			name = *rule.Serve.Variation
		} else {
			name = rule.Serve.Distribution.GetKeyName(target)
		}
		return name
	}

	// not found then return defaultServe
	if defaultServe.Variation != nil {
		return *defaultServe.Variation
	}

	if defaultServe.Distribution != nil {
		defaultServe.Distribution.isEnabled(target, OneHundred)
	}
	return "" // need defaultServe
}

// Tag object
type Tag struct {
	Name  string
	Value *string
}

// Variation object
type Variation struct {
	Description *string
	Identifier  string
	Name        *string
	Value       string
}

// Bool returns variation value as bool type
func (v *Variation) Bool(defaultValue bool) bool {
	if v == nil {
		return defaultValue
	}
	boolValue, _ := strconv.ParseBool(v.Value)
	return boolValue
}

// String returns variation value as string type
func (v *Variation) String(defaultValue string) string {
	if v == nil {
		return defaultValue
	}
	return v.Value
}

// Number returns variation value as float
func (v *Variation) Number(defaultValue float64) float64 {
	if v == nil {
		return defaultValue
	}
	number, _ := strconv.ParseFloat(v.Value, 64)
	return number
}

// Int returns variation value as integer value
func (v *Variation) Int(defaultValue int64) int64 {
	if v == nil {
		return defaultValue
	}
	intVal, _ := strconv.ParseInt(v.Value, 10, 64)
	return intVal
}

// JSON returns variation value as JSON value
func (v *Variation) JSON(defaultValue types.JSON) types.JSON {
	if v == nil {
		return defaultValue
	}
	result := make(types.JSON)
	if err := json.Unmarshal([]byte(v.Value), &result); err != nil {
		return defaultValue
	}
	return result
}

// Variations slice of variation
type Variations []Variation

// FindByIdentifier returns Variation with identifier if exist in variations
func (v Variations) FindByIdentifier(identifier string) *Variation {
	for _, val := range v {
		if val.Identifier == identifier {
			return &val
		}
	}
	return nil
}

// VariationMap object is variation which belongs to segments and targets
type VariationMap struct {
	TargetSegments []string
	Targets        []string
	Variation      string
}

// WeightedVariation represents Percentage Rollout data
type WeightedVariation struct {
	Variation string
	Weight    int
}
