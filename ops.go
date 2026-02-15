package jsonlogic

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	// null
	nullOp = ""

	//  var
	varOp         = "var"
	missingOp     = "missing"
	missingSomeOp = "missing_some"

	// Logic
	ifOp            = "if"
	equalOp         = "==" // TODO - coercion
	equalThreeOp    = "==="
	notEqualOp      = "!="
	notEqualThreeOp = "!=="
	negateOp        = "!"
	doubleNegateOp  = "!!"
	orOp            = "or"
	andOp           = "and"
	ternaryOp       = "?:"

	// Numeric
	greaterOp   = ">"  // TODO - non float
	greaterEqOp = ">=" // TODO
	lessOp      = "<"  // TODO - non float
	lessEqOp    = "<=" // TODO
	maxOp       = "max"
	minOp       = "min"

	plusOp     = "+"
	minusOp    = "-"
	multiplyOp = "*"
	divideOp   = "/"
	moduloOp   = "%"

	// Array operations
	mapOp    = "map"
	reduceOp = "reduce"
	filterOp = "filter"
	allOp    = "all"
	noneOp   = "none"
	someOp   = "some"
	mergeOp  = "merge"

	// String operations
	inOp     = "in"
	catOp    = "cat"
	substrOp = "substr"
)

// ClauseFunc takes input data, returns a result which
// could be any valid json type. JsonLogic seems to
// prefer returning null to returning any specific errors.
// The context argument is not used by any of the standard operations,
// but may be used by custom operations to provide rich functionality.
//
// The data argument must be of a type that might be automatically created by
// json.Unmarshal, specificall map[string]any, []any, string, float64 or bool.
// The existing API cannot enforce this. Rather than calling the functions
// directly, it is best to call ClauseFuncs via SafeCall
type ClauseFunc func(ctx context.Context, data any) any

func identityf(ctx context.Context, data any) any {
	return data
}

func nullf(ctx context.Context, data any) any {
	return nil
}

func emptySlice(ctx context.Context, data any) any {
	return []any{}
}

func truef(ctx context.Context, data any) any {
	return true
}

func falsef(ctx context.Context, data any) any {
	return false
}

// OpsSet operation names to a function that can build an instance of that
// operation.
type OpsSet map[string]func(args Arguments, ops OpsSet) (ClauseFunc, error)

// BuildArgFunc is a utility function for building new operations. It should
// be called once for each argument during compilation, and the resulting function
// should be call at execution time, to retrieve the calculated value of the
// argument.
func BuildArgFunc(arg Argument, ops OpsSet) (ClauseFunc, error) {
	if arg.Clause == nil {
		return func(context.Context, any) any {
			return arg.Value
		}, nil
	}
	return ops.Compile(arg.Clause)
}

func buildNullOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if args[0].Clause == nil {
		return func(ctx context.Context, data any) any {
			return args[0].Value
		}, nil
	}

	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		res := make([]any, len(termArgs))

		for i, ta := range termArgs {
			item := ta(ctx, data)
			res[i] = item
		}
		return res
	}, nil
}

func buildVarOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	var err error
	var indexArg ClauseFunc

	defaultArg := nullf

	switch {
	case len(args) == 0:
		return identityf, nil
	case len(args) >= 2:
		if defaultArg, err = BuildArgFunc(args[1], ops); err != nil {
			return nil, err
		}
		fallthrough
	case len(args) >= 1:
		if indexArg, err = BuildArgFunc(args[0], ops); err != nil {
			return nil, err
		}
	}

	return func(ctx context.Context, data any) any {
		indexVal := indexArg(ctx, data)
		defaultVal := defaultArg(ctx, data)

		// if the index is an empty string, we don't care about
		// the type and return the entire thing.
		indexstr, ok := indexVal.(string)
		if ok && indexstr == "" {
			return data
		}

		// otherwise, we assume this is an indexable type.
		switch data := data.(type) {
		case map[string]any:
			v := DottedRef(data, indexVal)
			if v != nil {
				return v
			}
		case []any:
			v := DottedRef(data, indexVal)
			if v != nil {
				return v
			}
		}
		return defaultVal
	}, nil
}

func buildMissingOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	switch {
	case len(args) == 0:
		return emptySlice, nil
	}

	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		checkItems := make([]any, 0, len(termArgs))

		for _, ta := range termArgs {
			item := ta(ctx, data)
			if sliceitem, ok := item.([]any); ok {
				checkItems = append(checkItems, sliceitem...)
				continue
			}
			checkItems = append(checkItems, item)
		}

		resp := make([]any, len(checkItems))

		n := 0
		for _, lval := range checkItems {
			v := DottedRef(data, lval)
			if v == nil {
				resp[n] = lval
				n++
			}
		}
		resp = resp[:n]

		return resp
	}, nil
}

func buildMissingSomeOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) <= 1 {
		return emptySlice, nil
	}

	requiredArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	termsArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		required := requiredArg(ctx, data)
		requiredfloat, ok := required.(float64)
		if !ok {
			return []any{}
		}

		terms := termsArg(ctx, data)
		termsslice, ok := terms.([]any)
		if !ok {
			return []any{}
		}

		resp := make([]any, len(termsslice))
		found := float64(0)
		n := 0
		for _, ta := range termsslice {
			v := DottedRef(data, ta)
			if v != nil {
				found++
				continue
			}
			resp[n] = ta
			n++
		}
		resp = resp[:n]
		if found >= requiredfloat {
			return []any{}
		}

		return resp
	}, nil
}

func buildIfOp3(args Arguments, ops OpsSet) (ClauseFunc, error) {
	var err error

	termArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	lArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	rArg := nullf
	if len(args) == 3 {
		if rArg, err = BuildArgFunc(args[2], ops); err != nil {
			return nil, err
		}
	}

	return func(ctx context.Context, data any) any {
		termVal := termArg(ctx, data)
		lVal := lArg(ctx, data)
		rVal := rArg(ctx, data)
		if IsTrue(termVal) {
			return lVal
		}
		return rVal
	}, nil
}

func buildIfOpMulti(args Arguments, ops OpsSet) (ClauseFunc, error) {
	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		last := 0
		for i := 0; i < len(termArgs)/2; i++ {
			lval := termArgs[i*2](ctx, data)
			if IsTrue(lval) {
				rval := termArgs[i*2+1](ctx, data)
				return rval
			}
			last += 2
		}

		// got here, if there is a final term, it should
		// be return
		if last != len(termArgs) {
			return termArgs[len(termArgs)-1](ctx, data)
		}
		return nil
	}, nil
}

func buildIfOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	switch {
	case len(args) == 0:
		return nullf, nil
	case len(args) == 1:
		return BuildArgFunc(args[0], ops)
	case len(args) <= 3:
		return buildIfOp3(args, ops)
	default:
		return buildIfOpMulti(args, ops)
	}
}

func buildTernaryOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	var err error

	termArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	lArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	rArg := nullf
	if len(args) == 3 {
		if rArg, err = BuildArgFunc(args[2], ops); err != nil {
			return nil, err
		}
	}

	return func(ctx context.Context, data any) any {
		termVal := termArg(ctx, data)
		lVal := lArg(ctx, data)
		rVal := rArg(ctx, data)
		if IsTrue(termVal) {
			return lVal
		}
		return rVal
	}, nil
}

func buildAndOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) == 0 {
		return nullf, nil
	}

	termArgs := make([]ClauseFunc, len(args))
	for i, ta := range args {
		termArg, err := BuildArgFunc(ta, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		var lastArg any
		for _, t := range termArgs {
			lastArg = t(ctx, data)
			if !IsTrue(lastArg) {
				return lastArg
			}
		}
		return lastArg
	}, nil
}

func buildOrOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) == 0 {
		return nullf, nil
	}

	termArgs := make([]ClauseFunc, len(args))
	for i, ta := range args {
		termArg, err := BuildArgFunc(ta, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		var lastArg any
		for _, t := range termArgs {
			lastArg = t(ctx, data)
			if IsTrue(lastArg) {
				return lastArg
			}
		}
		return lastArg
	}, nil
}

func buildEqualOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	switch {
	case len(args) == 0:
		return truef, nil
	case len(args) == 1:
		return falsef, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := lArg(ctx, data)
		rVal := rArg(ctx, data)

		return IsSoftEqual(lVal, rVal)
	}, nil
}

func buildNotEqualOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	eqf, err := buildEqualOp(args, ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		if eqres, ok := eqf(ctx, data).(bool); ok {
			return !eqres
		}
		return false
	}, nil
}

func buildGreaterOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	switch {
	case len(args) == 0:
		return func(ctx context.Context, data any) any {
			return false
		}, nil
	case len(args) == 1:
		return func(ctx context.Context, data any) any {
			return false
		}, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := toNumber(lArg(ctx, data))
		rVal := toNumber(rArg(ctx, data))

		return lVal > rVal
	}, nil
}

func buildGreaterEqualOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	switch {
	case len(args) == 0:
		return func(ctx context.Context, data any) any {
			return false
		}, nil
	case len(args) == 1:
		return func(ctx context.Context, data any) any {
			return false
		}, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := toNumber(lArg(ctx, data))
		rVal := toNumber(rArg(ctx, data))

		return lVal >= rVal
	}, nil
}

func buildBetweenExOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	mArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[2], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := toNumber(lArg(ctx, data))
		mVal := toNumber(mArg(ctx, data))
		rVal := toNumber(rArg(ctx, data))

		return lVal < mVal && mVal < rVal
	}, nil
}

func buildBetweenIncOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	mArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[2], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := toNumber(lArg(ctx, data))
		mVal := toNumber(mArg(ctx, data))
		rVal := toNumber(rArg(ctx, data))

		return lVal <= mVal && mVal <= rVal
	}, nil
}

func buildLessOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 2 {
		return func(ctx context.Context, data any) any {
			return false
		}, nil
	}
	if len(args) >= 3 {
		return buildBetweenExOp(args, ops)
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := toNumber(lArg(ctx, data))
		rVal := toNumber(rArg(ctx, data))

		return lVal < rVal
	}, nil
}

func buildLessEqualOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 2 {
		return func(ctx context.Context, data any) any {
			return false
		}, nil
	}
	if len(args) >= 3 {
		return buildBetweenIncOp(args, ops)
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := toNumber(lArg(ctx, data))
		rVal := toNumber(rArg(ctx, data))

		return lVal <= rVal
	}, nil
}

func buildMaxOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	switch {
	case len(args) == 0:
		return nullf, nil
	}

	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		resp := math.Inf(-1)
		for _, ta := range termArgs {
			item := toNumber(ta(ctx, data))
			if math.IsNaN(item) {
				return item
			}
			if item > resp {
				resp = item
			}
		}
		return resp
	}, nil
}

func buildMinOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	switch {
	case len(args) == 0:
		return nullf, nil
	}

	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		resp := math.Inf(1)
		for _, ta := range termArgs {
			item := toNumber(ta(ctx, data))
			if math.IsNaN(item) {
				return item
			}
			if item < resp {
				resp = item
			}
		}
		return resp
	}, nil
}

func buildEqualThreeOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	switch {
	case len(args) == 0:
		return truef, nil
	case len(args) == 1:
		return falsef, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := lArg(ctx, data)
		rVal := rArg(ctx, data)

		return IsEqual(lVal, rVal)
	}, nil
}

func buildNotEqualThreeOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	eqf, err := buildEqualThreeOp(args, ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		if eqres, ok := eqf(ctx, data).(bool); ok {
			return !eqres
		}
		return false
	}, nil
}

func buildNegateOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) == 0 {
		return truef, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		return !IsTrue(lArg(ctx, data))
	}, nil
}

func buildDoubleNegateOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) == 0 {
		return falsef, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		return IsTrue(lArg(ctx, data))
	}, nil
}

func buildPlusOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		resp := 0.0
		for _, ta := range termArgs {
			item := toNumber(ta(ctx, data))
			if math.IsNaN(item) {
				return item
			}
			resp += item
		}
		return resp
	}, nil
}

func buildUnaryMinusOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	arg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		item := toNumber(arg(ctx, data))
		if math.IsNaN(item) {
			return item
		}
		return -1.0 * item
	}, nil
}

func buildMinusOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) == 0 {
		return nullf, nil
	}

	if len(args) == 1 {
		return buildUnaryMinusOp(args, ops)
	}

	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		resp := toNumber(termArgs[0](ctx, data))
		if math.IsNaN(resp) {
			return resp
		}

		for _, ta := range termArgs[1:] {
			item := toNumber(ta(ctx, data))
			if math.IsNaN(item) {
				return resp
			}

			resp -= item
		}
		return resp
	}, nil
}

func buildMultiplyOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) == 0 {
		return nullf, nil
	}

	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		resp := 1.0
		for _, ta := range termArgs {
			item := toNumber(ta(ctx, data))
			if math.IsNaN(item) {
				return item
			}
			resp *= item
		}
		return resp
	}, nil
}

func buildDivideOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 2 {
		return nullf, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := toNumber(lArg(ctx, data))
		rVal := toNumber(rArg(ctx, data))

		return lVal / rVal
	}, nil
}

func buildModuloOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 2 {
		return nullf, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lVal := toNumber(lArg(ctx, data))
		rVal := toNumber(rArg(ctx, data))

		return math.Mod(lVal, rVal)
	}, nil
}

func buildMergeOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	switch {
	case len(args) == 0:
		return emptySlice, nil
	}

	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		resp := make([]any, 0, len(termArgs))
		for _, ta := range termArgs {
			item := ta(ctx, data)
			sliceitem, ok := item.([]any)
			if !ok {
				sliceitem = []any{item}
			}
			resp = append(resp, sliceitem...)
		}
		return resp
	}, nil
}

func buildInOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) <= 1 {
		return falsef, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}
	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		res := false
		lval := lArg(ctx, data)
		rval := rArg(ctx, data)

		switch rval := rval.(type) {
		case string:
			lstr := fmt.Sprintf("%v", lval)
			if strings.Contains(rval, lstr) {
				return true
			}
			return false
		case []any:
			for _, r := range rval {
				if IsDeepEqual(lval, r) {
					return true
				}
			}
			return false
		case map[string]any:
			for k := range rval {
				if IsDeepEqual(lval, k) {
					return true
				}
			}
			return false
		default:
		}

		return res
	}, nil
}

func buildCatOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	termArgs := make([]ClauseFunc, len(args))
	for i, a := range args {
		termArg, err := BuildArgFunc(a, ops)
		if err != nil {
			return nil, err
		}
		termArgs[i] = termArg
	}

	return func(ctx context.Context, data any) any {
		var resp strings.Builder
		for _, ta := range termArgs {
			resp.WriteString(fmt.Sprintf("%v", ta(ctx, data)))
		}
		return resp.String()
	}, nil
}

func buildSubstrOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	var err error
	if len(args) == 0 {
		return func(ctx context.Context, data any) any {
			return "undefined"
		}, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	offsetArg := nullf
	if len(args) >= 2 {
		offsetArg, err = BuildArgFunc(args[1], ops)
		if err != nil {
			return nil, err
		}
	}

	lengthArg := nullf
	if len(args) >= 3 {
		lengthArg, err = BuildArgFunc(args[2], ops)
		if err != nil {
			return nil, err
		}
	}

	return func(ctx context.Context, data any) any {
		lVal := lArg(ctx, data)
		offsetVal := offsetArg(ctx, data)
		lengthVal := lengthArg(ctx, data)

		var base string
		var ok bool
		if base, ok = lVal.(string); !ok {
			base = fmt.Sprintf("%v", lVal)
		}

		baseLen := utf8.RuneCountInString(base)
		if baseLen == 0 {
			return base
		}

		offset := 0.0
		offset, _ = offsetVal.(float64)
		offsetint := int(offset)

		length := 0.0
		length, _ = lengthVal.(float64)
		lengthint := int(length)

		start := 0
		end := baseLen

		switch {
		case offsetint > 0:
			if offsetint > len(base) {
				offsetint = len(base)
			}
			start = offsetint
		case offsetint < 0:
			if offsetint < (-1 * len(base)) {
				offsetint = -1 * len(base)
			}

			start = len(base) + offsetint
		}

		switch {
		case lengthint > 0:
			if start+lengthint > baseLen {
				lengthint = baseLen - start
			}
			end = start + lengthint
		case lengthint < 0:
			remaining := baseLen - start
			if lengthint*-1 > remaining {
				lengthint = remaining * -1
			}
			end += lengthint
		}

		var resp strings.Builder
		i := 0
		for _, c := range base {
			if i < start {
				i++
				continue
			}
			if i >= end {
				break
			}

			resp.WriteString(string(c))
			i++
		}

		return resp.String()
	}, nil
}

func buildMapOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 2 {
		return nullf, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lval := lArg(ctx, data)
		lslice, ok := lval.([]any)
		if !ok {
			return []any{}
		}

		resp := make([]any, len(lslice))

		for i, subd := range lslice {
			resp[i] = rArg(ctx, subd)
		}

		return resp
	}, nil
}

func buildFilterOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 2 {
		return nullf, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	rArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lval := lArg(ctx, data)
		lslice, ok := lval.([]any)
		if !ok {
			return []any{}
		}

		resp := make([]any, len(lslice))

		n := 0
		for _, subd := range lslice {
			if IsTrue(rArg(ctx, subd)) {
				resp[n] = subd
				n++
			}
		}
		resp = resp[:n]

		return resp
	}, nil
}

func buildReduceOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 3 {
		return nullf, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	fArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	initialArg, err := BuildArgFunc(args[2], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lval := lArg(ctx, data)
		var acc = initialArg(ctx, data)

		lslice, ok := lval.([]any)
		if !ok {
			return acc
		}

		for _, subd := range lslice {
			acc = fArg(ctx, map[string]any{
				"current":     subd,
				"accumulator": acc,
			})
		}

		return acc
	}, nil
}

func buildAllOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 2 {
		return nullf, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	fArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lval := lArg(ctx, data)
		lslice, ok := lval.([]any)
		if !ok {
			return []any{}
		}
		if len(lslice) == 0 {
			return false
		}

		for _, subd := range lslice {
			if !IsTrue(fArg(ctx, subd)) {
				return false
			}
		}

		return true
	}, nil
}

func buildSomeOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 2 {
		return nullf, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	fArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lval := lArg(ctx, data)
		lslice, ok := lval.([]any)
		if !ok {
			return []any{}
		}
		if len(lslice) == 0 {
			return false
		}

		for _, subd := range lslice {
			if IsTrue(fArg(ctx, subd)) {
				return true
			}
		}

		return false
	}, nil
}

func buildNoneOp(args Arguments, ops OpsSet) (ClauseFunc, error) {
	if len(args) < 2 {
		return nullf, nil
	}

	lArg, err := BuildArgFunc(args[0], ops)
	if err != nil {
		return nil, err
	}

	fArg, err := BuildArgFunc(args[1], ops)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, data any) any {
		lval := lArg(ctx, data)
		lslice, ok := lval.([]any)
		if !ok {
			return []any{}
		}
		if len(lslice) == 0 {
			return true
		}

		for _, subd := range lslice {
			if IsTrue(fArg(ctx, subd)) {
				return false
			}
		}

		return true
	}, nil
}

// Compile compiles a given clause using the operation constructors in this
// OpsSet
func (ops OpsSet) Compile(c *Clause) (ClauseFunc, error) {
	bf, ok := ops[c.Operator.Name]
	if !ok {
		return nil, fmt.Errorf("unrecognized operation %s", c.Operator.Name)
	}
	return bf(c.Arguments, ops)
}

// DefaultOps is the default set of operations as specified on the jsonlogic
// site.
var DefaultOps = OpsSet{
	nullOp:          buildNullOp,
	varOp:           buildVarOp,
	missingOp:       buildMissingOp,
	missingSomeOp:   buildMissingSomeOp,
	ifOp:            buildIfOp,
	ternaryOp:       buildTernaryOp,
	andOp:           buildAndOp,
	orOp:            buildOrOp,
	equalOp:         buildEqualOp,
	equalThreeOp:    buildEqualThreeOp,
	notEqualOp:      buildNotEqualOp,
	notEqualThreeOp: buildNotEqualThreeOp,
	negateOp:        buildNegateOp,
	doubleNegateOp:  buildDoubleNegateOp,
	lessOp:          buildLessOp,
	lessEqOp:        buildLessEqualOp,
	greaterOp:       buildGreaterOp,
	greaterEqOp:     buildGreaterEqualOp,
	minOp:           buildMinOp,
	maxOp:           buildMaxOp,
	plusOp:          buildPlusOp,
	minusOp:         buildMinusOp,
	multiplyOp:      buildMultiplyOp,
	divideOp:        buildDivideOp,
	moduloOp:        buildModuloOp,
	inOp:            buildInOp,
	catOp:           buildCatOp,
	substrOp:        buildSubstrOp,

	mapOp:    buildMapOp,
	filterOp: buildFilterOp,
	reduceOp: buildReduceOp,
	allOp:    buildAllOp,
	someOp:   buildSomeOp,
	noneOp:   buildNoneOp,

	mergeOp: buildMergeOp,
}

// Compile builds a ClauseFunc that will execute
// the provided rule against the data.
func Compile(c *Clause) (ClauseFunc, error) {
	return DefaultOps.Compile(c)
}
