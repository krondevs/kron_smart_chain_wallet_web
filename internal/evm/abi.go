package evm

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

type ArgInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Indexed bool   `json:"indexed,omitempty"`
}

type MethodInfo struct {
	Name            string    `json:"name"`
	Signature       string    `json:"signature"`
	StateMutability string    `json:"state_mutability"`
	Inputs          []ArgInfo `json:"inputs"`
	Outputs         []ArgInfo `json:"outputs"`
}

type EventInfo struct {
	Name      string    `json:"name"`
	Signature string    `json:"signature"`
	Inputs    []ArgInfo `json:"inputs"`
}

type ABIInfo struct {
	Methods     []MethodInfo `json:"methods"`
	Events      []EventInfo  `json:"events"`
	Constructor []ArgInfo    `json:"constructor"`
}

func Parse(abiJSON string) (*abi.ABI, error) {
	abiJSON = strings.TrimSpace(abiJSON)
	if abiJSON == "" {
		return nil, fmt.Errorf("ABI is required")
	}
	parsed, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, fmt.Errorf("invalid ABI: %w", err)
	}
	return &parsed, nil
}

func Inspect(abiJSON string) (ABIInfo, error) {
	parsed, err := Parse(abiJSON)
	if err != nil {
		return ABIInfo{}, err
	}
	info := ABIInfo{
		Methods:     []MethodInfo{},
		Events:      []EventInfo{},
		Constructor: []ArgInfo{},
	}
	for _, method := range parsed.Methods {
		mi := MethodInfo{
			Name:            method.Name,
			Signature:       method.Sig,
			StateMutability: method.StateMutability,
			Inputs:          []ArgInfo{},
			Outputs:         []ArgInfo{},
		}
		for _, in := range method.Inputs {
			mi.Inputs = append(mi.Inputs, ArgInfo{Name: in.Name, Type: in.Type.String()})
		}
		for _, out := range method.Outputs {
			mi.Outputs = append(mi.Outputs, ArgInfo{Name: out.Name, Type: out.Type.String()})
		}
		info.Methods = append(info.Methods, mi)
	}
	sort.Slice(info.Methods, func(i, j int) bool {
		return info.Methods[i].Name < info.Methods[j].Name
	})
	for _, event := range parsed.Events {
		ei := EventInfo{Name: event.Name, Signature: eventSignature(event), Inputs: []ArgInfo{}}
		for _, in := range event.Inputs {
			ei.Inputs = append(ei.Inputs, ArgInfo{Name: in.Name, Type: in.Type.String(), Indexed: in.Indexed})
		}
		info.Events = append(info.Events, ei)
	}
	sort.Slice(info.Events, func(i, j int) bool {
		return info.Events[i].Name < info.Events[j].Name
	})
	for _, in := range parsed.Constructor.Inputs {
		info.Constructor = append(info.Constructor, ArgInfo{Name: in.Name, Type: in.Type.String()})
	}
	return info, nil
}

func eventSignature(event abi.Event) string {
	types := make([]string, 0, len(event.Inputs))
	for _, in := range event.Inputs {
		types = append(types, in.Type.String())
	}
	return event.Name + "(" + strings.Join(types, ",") + ")"
}

func findMethod(parsed *abi.ABI, nameOrSig string) (*abi.Method, error) {
	nameOrSig = strings.TrimSpace(nameOrSig)
	if nameOrSig == "" {
		return nil, fmt.Errorf("method is required")
	}
	if method, ok := parsed.Methods[nameOrSig]; ok {
		return &method, nil
	}
	var found *abi.Method
	for _, method := range parsed.Methods {
		m := method
		if m.Sig == nameOrSig || m.Name == nameOrSig {
			if found != nil && found.Sig != m.Sig {
				return nil, fmt.Errorf("method %q is overloaded; use the full signature", nameOrSig)
			}
			found = &m
		}
	}
	if found == nil {
		return nil, fmt.Errorf("method %q not found in ABI", nameOrSig)
	}
	return found, nil
}

func EncodeCall(abiJSON, methodName string, args []json.RawMessage) (string, error) {
	parsed, err := Parse(abiJSON)
	if err != nil {
		return "", err
	}
	method, err := findMethod(parsed, methodName)
	if err != nil {
		return "", err
	}
	values, err := convertArgs(args, method.Inputs)
	if err != nil {
		return "", err
	}
	packed, err := method.Inputs.Pack(values...)
	if err != nil {
		return "", fmt.Errorf("encode arguments: %w", err)
	}
	data := append(append([]byte{}, method.ID...), packed...)
	return hex.EncodeToString(data), nil
}

func EncodeConstructor(abiJSON string, args []json.RawMessage) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	parsed, err := Parse(abiJSON)
	if err != nil {
		return "", err
	}
	values, err := convertArgs(args, parsed.Constructor.Inputs)
	if err != nil {
		return "", err
	}
	packed, err := parsed.Constructor.Inputs.Pack(values...)
	if err != nil {
		return "", fmt.Errorf("encode constructor: %w", err)
	}
	return hex.EncodeToString(packed), nil
}

func DecodeOutput(abiJSON, methodName, hexData string) ([]any, error) {
	parsed, err := Parse(abiJSON)
	if err != nil {
		return nil, err
	}
	method, err := findMethod(parsed, methodName)
	if err != nil {
		return nil, err
	}
	data, err := decodeHex(hexData)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return []any{}, nil
	}
	values, err := method.Outputs.Unpack(data)
	if err != nil {
		return nil, fmt.Errorf("decode output: %w", err)
	}
	normalized := make([]any, 0, len(values))
	for _, v := range values {
		normalized = append(normalized, normalize(v))
	}
	return normalized, nil
}

func convertArgs(args []json.RawMessage, inputs abi.Arguments) ([]any, error) {
	if len(args) != len(inputs) {
		return nil, fmt.Errorf("expected %d arguments, got %d", len(inputs), len(args))
	}
	values := make([]any, 0, len(inputs))
	for i, input := range inputs {
		var raw any
		dec := json.NewDecoder(strings.NewReader(string(args[i])))
		dec.UseNumber()
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("argument %d is not valid JSON: %w", i+1, err)
		}
		v, err := convertValue(raw, input.Type, input.Name)
		if err != nil {
			return nil, fmt.Errorf("argument %d (%s): %w", i+1, input.Name, err)
		}
		values = append(values, v)
	}
	return values, nil
}

func convertValue(raw any, typ abi.Type, name string) (any, error) {
	switch typ.T {
	case abi.UintTy, abi.IntTy:
		s, err := numericString(raw)
		if err != nil {
			return nil, err
		}
		n, ok := new(big.Int).SetString(s, 10)
		if !ok {
			return nil, fmt.Errorf("invalid integer %q", s)
		}
		if typ.T == abi.UintTy && n.Sign() < 0 {
			return nil, fmt.Errorf("unsigned integer cannot be negative")
		}
		return n, nil
	case abi.BoolTy:
		switch v := raw.(type) {
		case bool:
			return v, nil
		case string:
			return strings.EqualFold(v, "true"), nil
		default:
			return nil, fmt.Errorf("expected boolean")
		}
	case abi.AddressTy:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected address string")
		}
		clean := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "0x"), "0X")
		if len(clean) != 40 {
			return nil, fmt.Errorf("address must be 20 bytes (40 hex chars)")
		}
		if !common.IsHexAddress("0x" + clean) {
			return nil, fmt.Errorf("invalid address")
		}
		return common.HexToAddress("0x" + clean), nil
	case abi.StringTy:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected string")
		}
		return s, nil
	case abi.BytesTy:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected hex string")
		}
		b, err := decodeHex(s)
		if err != nil {
			return nil, err
		}
		return b, nil
	case abi.FixedBytesTy:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected hex string")
		}
		b, err := decodeHex(s)
		if err != nil {
			return nil, err
		}
		if len(b) != typ.Size {
			return nil, fmt.Errorf("expected %d bytes, got %d", typ.Size, len(b))
		}
		arr := reflect.New(reflect.ArrayOf(typ.Size, reflect.TypeOf(byte(0)))).Elem()
		reflect.Copy(arr, reflect.ValueOf(b))
		return arr.Interface(), nil
	case abi.ArrayTy, abi.SliceTy:
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("expected JSON array")
		}
		if typ.T == abi.ArrayTy && len(list) != typ.Size {
			return nil, fmt.Errorf("expected %d elements, got %d", typ.Size, len(list))
		}
		values := make([]any, 0, len(list))
		for i, item := range list {
			v, err := convertValue(item, *typ.Elem, fmt.Sprintf("%s[%d]", name, i))
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
		// Reflect into the exact slice/array type expected by go-ethereum.
		sliceType := reflect.SliceOf(typ.Elem.GetType())
		slice := reflect.MakeSlice(sliceType, len(values), len(values))
		for i, v := range values {
			rv := reflect.ValueOf(v)
			if !rv.Type().AssignableTo(typ.Elem.GetType()) {
				if rv.Type().ConvertibleTo(typ.Elem.GetType()) {
					rv = rv.Convert(typ.Elem.GetType())
				} else {
					return nil, fmt.Errorf("cannot convert element %d to %s", i, typ.Elem.GetType())
				}
			}
			slice.Index(i).Set(rv)
		}
		if typ.T == abi.ArrayTy {
			arrayType := reflect.ArrayOf(typ.Size, typ.Elem.GetType())
			arr := reflect.New(arrayType).Elem()
			reflect.Copy(arr, slice)
			return arr.Interface(), nil
		}
		return slice.Interface(), nil
	case abi.TupleTy:
		elems := typ.TupleElems
		var list []any
		switch v := raw.(type) {
		case []any:
			list = v
		case map[string]any:
			list = make([]any, len(elems))
			for i := range elems {
				key := ""
				if i < len(typ.TupleRawNames) {
					key = typ.TupleRawNames[i]
				}
				item, ok := v[key]
				if !ok {
					item, ok = v[fmt.Sprintf("%d", i)]
				}
				if !ok {
					return nil, fmt.Errorf("missing tuple field %q", key)
				}
				list[i] = item
			}
		default:
			return nil, fmt.Errorf("expected JSON array or object for tuple")
		}
		if len(list) != len(elems) {
			return nil, fmt.Errorf("expected %d tuple fields, got %d", len(elems), len(list))
		}
		values := make([]any, 0, len(list))
		for i, item := range list {
			fieldName := fmt.Sprintf("%s.%d", name, i)
			if i < len(typ.TupleRawNames) {
				fieldName = name + "." + typ.TupleRawNames[i]
			}
			v, err := convertValue(item, *elems[i], fieldName)
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
		tupleType := typ.GetType()
		if tupleType.Kind() == reflect.Struct {
			structVal := reflect.New(tupleType).Elem()
			for i, v := range values {
				if i >= structVal.NumField() {
					break
				}
				field := structVal.Field(i)
				rv := reflect.ValueOf(v)
				if !rv.Type().AssignableTo(field.Type()) {
					if rv.Type().ConvertibleTo(field.Type()) {
						rv = rv.Convert(field.Type())
					} else {
						return nil, fmt.Errorf("cannot convert tuple field %d", i)
					}
				}
				field.Set(rv)
			}
			return structVal.Interface(), nil
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported ABI type %s", typ.String())
	}
}

func numericString(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v), nil
	case json.Number:
		return v.String(), nil
	case float64:
		return fmt.Sprintf("%.0f", v), nil
	default:
		return "", fmt.Errorf("expected integer string or number")
	}
}

func decodeHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return []byte{}, nil
	}
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("hex string has odd length")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}
	return b, nil
}

func normalize(v any) any {
	switch value := v.(type) {
	case nil:
		return nil
	case *big.Int:
		return value.String()
	case common.Address:
		return strings.TrimPrefix(strings.ToLower(value.Hex()), "0x")
	case []byte:
		return "0x" + hex.EncodeToString(value)
	case string, bool:
		return value
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = normalize(rv.Index(i).Interface())
		}
		return out
	case reflect.Struct:
		out := make(map[string]any, rv.NumField())
		for i := 0; i < rv.NumField(); i++ {
			field := rv.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			out[field.Name] = normalize(rv.Field(i).Interface())
		}
		return out
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", rv.Uint())
	default:
		return v
	}
}

// DecodeEventData decodes non-indexed event data. Indexed topics are returned
// as raw hex because event-specific topic decoding adds little for a generic
// contract console.
func DecodeEventData(abiJSON, eventName string, dataHex string) ([]any, error) {
	parsed, err := Parse(abiJSON)
	if err != nil {
		return nil, err
	}
	var event *abi.Event
	for _, e := range parsed.Events {
		if e.Name == eventName || eventSignature(e) == eventName {
			ev := e
			event = &ev
			break
		}
	}
	if event == nil {
		return nil, fmt.Errorf("event %q not found in ABI", eventName)
	}
	data, err := decodeHex(dataHex)
	if err != nil {
		return nil, err
	}
	values, err := event.Inputs.NonIndexed().Unpack(data)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, normalize(v))
	}
	return out, nil
}
