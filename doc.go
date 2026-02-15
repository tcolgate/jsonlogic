// Package jsonlogic is a Go implementation of JsonLogic as described
// at http://jsonlogic.com.  All example content for all operations, as given on
// the web site, work as expected. The test suite provided at
// https://jsonlogic.com/tests.json passes unaltered.
//
// var data lookup assumes the passed-in data structure is akin to that
// provided  by a raw json.Unmarshal. Only the following types are
// supported:
//
//	primitives of type string, float64 or bool
//	map[string]interface{} // where interface{} is a compatible type
//	[]interface{} // where interface{} is a compatible type
//
// A compiled ClauseFunc can be called directly, but it is easy to
// pass an unsupported type. The SafeCall generic function can be used
// to constrain the acceptable types, and remind us to cast the data to a type
// that the internals can support.
//
//	type myData map[string]interface{}
//	var data myData
//
//	cf(ctx, data))                                   // Compiles but will not work
//	cf(ctx, (map[string]interface{})(data))          // is correct but the cast is easily forgotten
//	SafeCall(ctx, cf, data)                          // Will not compile
//	SafeCall(ctx, cf, map[string]interface{})(data)) // Forces us to cast
//
// We cannot currently query Go native structs, or maps/slices of other native Go
// types.
//
// Incompatibilities may exist in support for JavaScript type coercion. Any
// incompatibilities found should be reported as bugs.
package jsonlogic
