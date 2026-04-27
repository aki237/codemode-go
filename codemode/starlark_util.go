package codemode

import (
	"fmt"

	"go.starlark.net/starlark"
)

// UnpackDict converts a Starlark Dict into a Go map[string]any
func UnpackDict(d *starlark.Dict) (map[string]any, error) {
	out := make(map[string]any)

	// Iterate over the keys in the Starlark dictionary
	for _, k := range d.Keys() {
		// Ensure the key is a string
		strKey, ok := starlark.AsString(k)
		if !ok {
			return nil, fmt.Errorf("dict key %s is not a string", k.Type())
		}

		// Fetch the value for the key
		v, found, err := d.Get(k)
		if err != nil || !found {
			continue
		}

		// Convert the value to a native Go interface
		val, err := starlarkValueToAny(v)
		if err != nil {
			return nil, fmt.Errorf("failed to parse key '%s': %w", strKey, err)
		}

		out[strKey] = val
	}
	return out, nil
}

// starlarkValueToAny maps Starlark types to native Go types
func starlarkValueToAny(v starlark.Value) (any, error) {
	switch val := v.(type) {
	case starlark.String:
		return string(val), nil
	case starlark.Int:
		i, ok := val.Int64()
		if !ok {
			return nil, fmt.Errorf("starlark integer too large for int64")
		}
		return i, nil
	case starlark.Float:
		return float64(val), nil
	case starlark.Bool:
		return bool(val), nil
	case starlark.NoneType:
		return nil, nil
	case *starlark.List:
		var arr []any
		iter := val.Iterate()
		defer iter.Done()

		var item starlark.Value
		for iter.Next(&item) {
			parsed, err := starlarkValueToAny(item)
			if err != nil {
				return nil, err
			}
			arr = append(arr, parsed)
		}
		return arr, nil
	case *starlark.Dict:
		// Recurse for nested dictionaries
		return UnpackDict(val)
	default:
		return nil, fmt.Errorf("unsupported starlark type: %s", val.Type())
	}
}

// ToStarlarkValue converts native Go types to starlark.Value
func ToStarlarkValue(v any) (starlark.Value, error) {
	if v == nil {
		return starlark.None, nil
	}

	switch val := v.(type) {
	case string:
		return starlark.String(val), nil
	case int:
		return starlark.MakeInt(val), nil
	case int64:
		return starlark.MakeInt64(val), nil
	case float64:
		return starlark.Float(val), nil
	case bool:
		return starlark.Bool(val), nil
	case []any:
		// Handle nested lists
		var list []starlark.Value
		for _, item := range val {
			parsed, err := ToStarlarkValue(item)
			if err != nil {
				return nil, err
			}
			list = append(list, parsed)
		}
		return starlark.NewList(list), nil
	case map[string]any:
		// Handle nested dictionaries
		dict := &starlark.Dict{}
		for k, item := range val {
			parsed, err := ToStarlarkValue(item)
			if err != nil {
				return nil, fmt.Errorf("error parsing key '%s': %w", k, err)
			}
			if err := dict.SetKey(starlark.String(k), parsed); err != nil {
				return nil, err
			}
		}
		return dict, nil
	default:
		return nil, fmt.Errorf("unsupported Go type: %T", val)
	}
}

// MapToDict is a convenience wrapper for maps
func MapToDict(m map[string]any) (*starlark.Dict, error) {
	val, err := ToStarlarkValue(m)
	if err != nil {
		return nil, err
	}
	return val.(*starlark.Dict), nil
}
