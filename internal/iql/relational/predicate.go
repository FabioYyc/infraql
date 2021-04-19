package relational

import (
	"fmt"
	"infraql/internal/iql/iqlmodel"
	"regexp"

	"vitess.io/vitess/go/sqltypes"
	"vitess.io/vitess/go/vt/sqlparser"
	"vitess.io/vitess/go/vt/vtgate/evalengine"
)

func AndTableFilters(lhs, rhs func(iqlmodel.ITable) (iqlmodel.ITable, error)) func(iqlmodel.ITable) (iqlmodel.ITable, error) {
	if lhs == nil {
		return rhs
	}
	return func(t iqlmodel.ITable) (iqlmodel.ITable, error) {
		lResult, lErr := lhs(t)
		rResult, rErr := rhs(t)
		if lErr != nil {
			return nil, lErr
		}
		if rErr != nil {
			return nil, rErr
		}
		if lResult != nil && rResult != nil {
			return lResult, nil
		}
		return nil, nil
	}
}

func OrTableFilters(lhs, rhs func(iqlmodel.ITable) (iqlmodel.ITable, error)) func(iqlmodel.ITable) (iqlmodel.ITable, error) {
	if lhs == nil {
		return rhs
	}
	return func(t iqlmodel.ITable) (iqlmodel.ITable, error) {
		lResult, lErr := lhs(t)
		rResult, rErr := rhs(t)
		if lErr != nil {
			return nil, lErr
		}
		if rErr != nil {
			return nil, rErr
		}
		if lResult != nil {
			return lResult, nil
		}
		if rResult != nil {
			return rResult, nil
		}
		return nil, nil
	}
}

func ConstructTablePredicateFilter(colName string, rhs sqltypes.Value, operatorPredicate func(int) bool) func(iqlmodel.ITable) (iqlmodel.ITable, error) {
	return func(row iqlmodel.ITable) (iqlmodel.ITable, error) {
		v, e := row.GetKeyAsSqlVal(colName)
		if e != nil {
			return nil, e
		}
		result, err := evalengine.NullsafeCompare(v, rhs)
		if err == nil && operatorPredicate(result) {
			return row, nil
		}
		return nil, err
	}
}

func ConstructLikePredicateFilter(colName string, rhs *regexp.Regexp, isNegating bool) func(iqlmodel.ITable) (iqlmodel.ITable, error) {
	return func(row iqlmodel.ITable) (iqlmodel.ITable, error) {
		v, vErr := row.GetKey(colName)
		if vErr != nil {
			return nil, vErr
		}
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("cannot compare non-string type '%T' with regex", v)
		}
		if rhs.MatchString(s) != isNegating {
			return row, nil
		}
		return nil, nil
	}
}

func GetOperatorPredicate(operator string) (func(int) bool, error) {
	switch operator {
	case sqlparser.EqualStr:
		return func(result int) bool {
			return result == 0
		}, nil
	case sqlparser.NotEqualStr:
		return func(result int) bool {
			return result != 0
		}, nil
	case sqlparser.GreaterEqualStr:
		return func(result int) bool {
			return result >= 0
		}, nil
	case sqlparser.GreaterThanStr:
		return func(result int) bool {
			return result > 0
		}, nil
	case sqlparser.LessEqualStr:
		return func(result int) bool {
			return result <= 0
		}, nil
	case sqlparser.LessThanStr:
		return func(result int) bool {
			return result < 0
		}, nil
	}
	return nil, fmt.Errorf("cannot determine predicate")
}
