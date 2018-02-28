package reflect2

import (
	"unsafe"
	"reflect"
	"runtime"
	"strings"
)

// typelinks1 for 1.5 ~ 1.6
//go:linkname typelinks1 reflect.typelinks
func typelinks1() [][]unsafe.Pointer

// typelinks2 for 1.7 ~
//go:linkname typelinks2 reflect.typelinks
func typelinks2() (sections []unsafe.Pointer, offset [][]int32)

var types = map[string]reflect.Type{}

func init() {
	ver := runtime.Version()
	if ver == "go1.5" || strings.HasPrefix(ver, "go1.5.") {
		loadGo15Types()
	} else if ver == "go1.6" || strings.HasPrefix(ver, "go1.6.") {
		loadGo15Types()
	} else {
		loadGo17Types()
	}
}

func loadGo15Types() {
	var obj interface{} = reflect.TypeOf(0)
	typePtrss := typelinks1()
	for _, typePtrs := range typePtrss {
		for _, typePtr := range typePtrs {
			(*emptyInterface)(unsafe.Pointer(&obj)).word = typePtr
			typ := obj.(reflect.Type)
			if typ.Kind() == reflect.Ptr && typ.Elem().Kind() == reflect.Struct {
				types[typ.Elem().String()] = typ.Elem()
			}
			if typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Ptr &&
				typ.Elem().Elem().Kind() == reflect.Struct {
				types[typ.Elem().Elem().String()] = typ.Elem().Elem()
			}
		}
	}
}

func loadGo17Types() {
	var obj interface{} = reflect.TypeOf(0)
	sections, offset := typelinks2()
	for i, offs := range offset {
		rodata := sections[i]
		for _, off := range offs {
			(*emptyInterface)(unsafe.Pointer(&obj)).word = resolveTypeOff(unsafe.Pointer(rodata), off)
			typ := obj.(reflect.Type)
			if typ.Kind() == reflect.Ptr && typ.Elem().Kind() == reflect.Struct {
				types[typ.Elem().String()] = typ.Elem()
			}
		}
	}
}

type emptyInterface struct {
	typ  unsafe.Pointer
	word unsafe.Pointer
}

// TypeByName return the type by its name, just like Class.forName in java
func TypeByName(typeName string) Type {
	return Type2(types[typeName])
}
