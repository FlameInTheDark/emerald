package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"

	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/templating"
	lua "github.com/yuin/gopher-lua"
)

type LuaNode struct{}

type luaConfig struct {
	Script string `json:"script"`
}

func (e *LuaNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg luaConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStrings(&cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	L := lua.NewState()
	defer L.Close()

	L.SetGlobal("input", toLuaValue(L, input))
	for key, value := range input {
		L.SetGlobal(key, toLuaValue(L, value))
	}

	if err := L.DoString(cfg.Script); err != nil {
		return nil, fmt.Errorf("lua execution: %w", err)
	}

	outputValue := fromLuaValue(L.Get(-1))
	output := map[string]any{}

	switch typed := outputValue.(type) {
	case nil:
	case map[string]any:
		output = typed
	default:
		output["result"] = typed
	}

	data, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("marshal output: %w", err)
	}

	return &node.NodeResult{Output: data}, nil
}

func (e *LuaNode) Validate(config json.RawMessage) error {
	var cfg luaConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.Script == "" {
		return fmt.Errorf("script is required")
	}
	return nil
}

func toLuaValue(L *lua.LState, value any) lua.LValue {
	if value == nil {
		return lua.LNil
	}

	switch typed := value.(type) {
	case lua.LValue:
		return typed
	case string:
		return lua.LString(typed)
	case bool:
		return lua.LBool(typed)
	case json.Number:
		if i, err := typed.Int64(); err == nil {
			return lua.LNumber(i)
		}
		if f, err := typed.Float64(); err == nil {
			return lua.LNumber(f)
		}
		return lua.LString(typed.String())
	case int:
		return lua.LNumber(typed)
	case int8:
		return lua.LNumber(typed)
	case int16:
		return lua.LNumber(typed)
	case int32:
		return lua.LNumber(typed)
	case int64:
		return lua.LNumber(typed)
	case uint:
		return lua.LNumber(typed)
	case uint8:
		return lua.LNumber(typed)
	case uint16:
		return lua.LNumber(typed)
	case uint32:
		return lua.LNumber(typed)
	case uint64:
		return lua.LNumber(typed)
	case float32:
		return lua.LNumber(typed)
	case float64:
		return lua.LNumber(typed)
	}

	return toLuaReflectValue(L, reflect.ValueOf(value))
}

func toLuaReflectValue(L *lua.LState, value reflect.Value) lua.LValue {
	if !value.IsValid() {
		return lua.LNil
	}

	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			return lua.LNil
		}
		return toLuaReflectValue(L, value.Elem())
	case reflect.String:
		return lua.LString(value.String())
	case reflect.Bool:
		return lua.LBool(value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return lua.LNumber(value.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return lua.LNumber(value.Uint())
	case reflect.Float32, reflect.Float64:
		return lua.LNumber(value.Float())
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8 {
			return lua.LString(string(value.Bytes()))
		}

		table := L.NewTable()
		for i := 0; i < value.Len(); i++ {
			table.RawSetInt(i+1, toLuaReflectValue(L, value.Index(i)))
		}
		return table
	case reflect.Map:
		table := L.NewTable()
		iter := value.MapRange()
		for iter.Next() {
			key := fmt.Sprint(iter.Key().Interface())
			table.RawSetString(key, toLuaReflectValue(L, iter.Value()))
		}
		return table
	case reflect.Struct:
		table := L.NewTable()
		valueType := value.Type()
		for i := 0; i < value.NumField(); i++ {
			fieldType := valueType.Field(i)
			if fieldType.PkgPath != "" {
				continue
			}
			table.RawSetString(fieldType.Name, toLuaReflectValue(L, value.Field(i)))
		}
		return table
	default:
		return lua.LString(fmt.Sprintf("%v", value.Interface()))
	}
}

func fromLuaValue(value lua.LValue) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case lua.LBool:
		return bool(typed)
	case lua.LString:
		return string(typed)
	case lua.LNumber:
		return normalizeLuaNumber(typed)
	case *lua.LTable:
		return fromLuaTable(typed)
	default:
		if value == lua.LNil {
			return nil
		}
		return value.String()
	}
}

func fromLuaTable(table *lua.LTable) any {
	arrayValues := map[int]any{}
	objectValues := map[string]any{}
	hasObjectKeys := false
	maxArrayIndex := 0

	table.ForEach(func(key lua.LValue, value lua.LValue) {
		converted := fromLuaValue(value)

		if index, ok := luaArrayIndex(key); ok {
			arrayValues[index] = converted
			if index > maxArrayIndex {
				maxArrayIndex = index
			}
			return
		}

		hasObjectKeys = true
		objectValues[key.String()] = converted
	})

	if len(arrayValues) == 0 && !hasObjectKeys {
		return map[string]any{}
	}

	if !hasObjectKeys && len(arrayValues) == maxArrayIndex {
		items := make([]any, maxArrayIndex)
		for i := 1; i <= maxArrayIndex; i++ {
			items[i-1] = arrayValues[i]
		}
		return items
	}

	for index, value := range arrayValues {
		objectValues[strconv.Itoa(index)] = value
	}

	return objectValues
}

func luaArrayIndex(value lua.LValue) (int, bool) {
	number, ok := value.(lua.LNumber)
	if !ok {
		return 0, false
	}

	floatValue := float64(number)
	if math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
		return 0, false
	}
	if floatValue < 1 || math.Trunc(floatValue) != floatValue {
		return 0, false
	}

	return int(floatValue), true
}

func normalizeLuaNumber(value lua.LNumber) any {
	floatValue := float64(value)
	if math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
		return floatValue
	}
	if math.Trunc(floatValue) == floatValue {
		return int64(floatValue)
	}
	return floatValue
}
