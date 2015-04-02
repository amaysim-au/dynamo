package dynamo

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/awslabs/aws-sdk-go/service/dynamodb"
)

/*
types
B 	[]byte
BOOL bool
BS 	[][]byte
L 	[]any
M 	map[string]any
N 	int64 or float64
NULL bool?
S 	string
SS 	[]string
*/

type Unmarshaler interface {
	UnmarshalDynamo(av *dynamodb.AttributeValue) error
}

func unmarshal(av *dynamodb.AttributeValue, out interface{}) error {
	return nil
}

// unmarshals one value
func unmarshalReflect(av *dynamodb.AttributeValue, rv reflect.Value) error {
	// TODO: fix fix fix

	// first try interface unmarshal stuff
	if rv.CanInterface() {
		var iface = rv.Interface()
		// TODO: try non-pointer types too, for compatibility
		if rv.CanAddr() {
			iface = rv.Addr().Interface()
		}
		if u, ok := iface.(Unmarshaler); ok {
			return u.UnmarshalDynamo(av)
		}
		if u, ok := iface.(encoding.TextUnmarshaler); ok {
			if av.S != nil {
				return u.UnmarshalText([]byte(*av.S))
			}
		}
	}

	switch rv.Kind() {
	case reflect.Ptr:
		pt := reflect.New(rv.Type().Elem())
		rv.Set(pt)
		if av.NULL == nil || !(*av.NULL) {
			return unmarshalReflect(av, rv.Elem())
		}
	case reflect.Bool:
		if av.BOOL == nil {
			return errors.New("expected BOOL to be non-nil")
		}
		rv.SetBool(*av.BOOL)
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		if av.N == nil {
			return errors.New("expected N to be non-nil")
		}
		n, err := strconv.ParseInt(*av.N, 10, 64)
		if err != nil {
			return err
		}
		rv.SetInt(n)
	case reflect.String:
		// TODO: unmarshal len=1 lists to single string etc
		if av.S == nil {
			return errors.New("expected S to be non-nil")
		}
		rv.SetString(*av.S)
	default:
		var iface = rv.Interface()
		if rv.CanAddr() {
			iface = rv.Addr().Interface()
		}

		if u, ok := iface.(Unmarshaler); ok {
			return u.UnmarshalDynamo(av)
		}

		return fmt.Errorf("can't unmarshal to type: %T (%+v)", iface, iface)
	}
	return nil
}

// unmarshals a struct
// TODO: unmarshal to map[string]interface{} too
func unmarshalItem(itemptr *map[string]*dynamodb.AttributeValue, out interface{}) error {
	item := *itemptr
	rv := reflect.ValueOf(out)

	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("not a pointer: %T", out)
	}

	// fmt.Printf("unmarshal item %T %v %v\n", out, rv.Type().Kind(), rv.Kind())

	var err error
	switch rv.Elem().Kind() {
	case reflect.Struct:
		for i := 0; i < rv.Elem().Type().NumField(); i++ {
			field := rv.Elem().Type().Field(i)

			name, _ := fieldName(field)
			if name == "-" {
				// skip
				continue
			}

			if av, ok := item[name]; ok {
				if innerErr := unmarshalReflect(av, rv.Elem().Field(i)); innerErr != nil {
					err = innerErr
				}
			}
		}
	default:
		return fmt.Errorf("unknown type: %T", out)
	}

	return err
}

// unmarshals to a slice
func unmarshalAll(items []*map[string]*dynamodb.AttributeValue, out interface{}) error {
	// cribbed from mgo
	resultv := reflect.ValueOf(out)
	if resultv.Kind() != reflect.Ptr || resultv.Elem().Kind() != reflect.Slice {
		panic("result argument must be a slice address")
	}
	slicev := resultv.Elem()
	slicev = slicev.Slice(0, slicev.Cap())
	elemt := slicev.Type().Elem()
	for i, item := range items {
		if slicev.Len() == i {
			elemp := reflect.New(elemt)
			if err := unmarshalItem(item, elemp.Interface()); err != nil {
				return err
			}
			slicev = reflect.Append(slicev, elemp.Elem())
			slicev = slicev.Slice(0, slicev.Cap())
		} else {
			if err := unmarshalItem(item, slicev.Index(i).Addr().Interface()); err != nil {
				return err
			}
		}
	}
	resultv.Elem().Set(slicev.Slice(0, len(items)))
	return nil
}