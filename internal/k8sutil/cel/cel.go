package cel

import (
	"fmt"

	celgo "github.com/google/cel-go/cel"
	"k8s.io/apimachinery/pkg/api/resource"
	dracel "k8s.io/dynamic-resource-allocation/cel"
)

// ComparisonOperator defines available operators for CEL expression.
type ComparisonOperator string

const (
	OpEqual          ComparisonOperator = "=="
	OpNotEqual       ComparisonOperator = "!="
	OpGreater        ComparisonOperator = ">"
	OpGreaterOrEqual ComparisonOperator = ">="
	OpLess           ComparisonOperator = "<"
	OpLessOrEqual    ComparisonOperator = "<="
)

var boolOperators = map[ComparisonOperator]string{
	OpEqual:    "==",
	OpNotEqual: "!=",
}

var intOperators = map[ComparisonOperator]string{
	OpEqual:          "==",
	OpNotEqual:       "!=",
	OpGreater:        ">",
	OpGreaterOrEqual: ">=",
	OpLess:           "<",
	OpLessOrEqual:    "<=",
}

var stringOperators = map[ComparisonOperator]string{
	OpEqual:    "==",
	OpNotEqual: "!=",
}

var semverOperators = map[ComparisonOperator]string{
	OpEqual:          "== 0",
	OpNotEqual:       "!= 0",
	OpGreater:        "> 0",
	OpGreaterOrEqual: ">= 0",
	OpLess:           "< 0",
	OpLessOrEqual:    "<= 0",
}

var quantityOperators = map[ComparisonOperator]string{
	OpEqual:          "== 0",
	OpNotEqual:       "!= 0",
	OpGreater:        "> 0",
	OpGreaterOrEqual: ">= 0",
	OpLess:           "< 0",
	OpLessOrEqual:    "<= 0",
}

// ValueType defines available value types for CEL.
type ValueType string

const (
	TypeBool     ValueType = "bool"
	TypeString   ValueType = "string"
	TypeInt      ValueType = "int"
	TypeSemver   ValueType = "semver"
	TypeQuantity ValueType = "quantity"
	TypeUnknown  ValueType = "unknown"
)

func (vt ValueType) CELType() *celgo.Type {
	switch vt {
	case TypeBool:
		return celgo.BoolType
	case TypeInt:
		return celgo.IntType
	case TypeString, TypeSemver, TypeQuantity:
		return celgo.StringType
	}
	return nil
}

// BuildExpr returns a CEL expression given key, operator, value, and type.
// Examples:
// BuildExpr("foo", OpEqual, true, TypeBool) => "foo == true"
// BuildExpr("foo", OpEqual, "bar", TypeString) => "foo == \"bar\""
// BuildExpr("count", OpGreater, 5, TypeInt) => "count > 5"
// BuildExpr("ver", OpGreater, "1.2.3", TypeSemver) => "semver(ver).compareTo(semver(\"1.2.3\")) > 0".
func BuildExpr(key string, op ComparisonOperator, value interface{}, vt ValueType) (string, error) {
	if value == nil {
		return "", nil
	}

	var expr string
	switch vt {
	case TypeBool:
		if b, ok := value.(bool); ok {
			comp, ok := boolOperators[op]
			if !ok {
				return "", fmt.Errorf("invalid operator %q for bool type", op)
			}
			expr = fmt.Sprintf("%s %s %t", key, comp, b)
		}
	case TypeInt:
		if num, ok := value.(int); ok {
			comp, ok := intOperators[op]
			if !ok {
				return "", fmt.Errorf("invalid operator %q for int type", op)
			}
			expr = fmt.Sprintf("%s %s %d", key, comp, num)
		}
	case TypeString:
		if str, ok := value.(string); ok {
			comp, ok := stringOperators[op]
			if !ok {
				return "", fmt.Errorf("invalid operator %q for string type", op)
			}
			expr = fmt.Sprintf("%s %s %q", key, comp, str)
		}
	case TypeSemver:
		if str, ok := value.(string); ok {
			comp, ok := semverOperators[op]
			if !ok {
				return "", fmt.Errorf("invalid operator %q for semver type", op)
			}
			expr = fmt.Sprintf("(%s).compareTo(semver(%q)) %s", key, str, comp)
		}
	case TypeQuantity:
		if quantity, ok := value.(*resource.Quantity); ok {
			comp, ok := quantityOperators[op]
			if !ok {
				return "", fmt.Errorf("invalid operator %q for quantity type", op)
			}
			expr = fmt.Sprintf("(%s).compareTo(quantity(%q)) %s", key, quantity.String(), comp)
		}
	}
	if expr == "" {
		return "", fmt.Errorf("invalid value type %q", vt)
	}
	return expr, validateExpr(expr)
}

func validateExpr(expression string) error {
	compiler := dracel.GetCompiler()
	result := compiler.CompileCELExpression(expression, dracel.Options{DisableCostEstimation: true})

	if result.Error != nil {
		return result.Error
	}
	return nil
}
