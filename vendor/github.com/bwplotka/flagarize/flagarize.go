// Copyright (c) Bartłomiej Płotka @bwplotka
// Licensed under the Apache License 2.0.

package flagarize

import (
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/alecthomas/units"
	"github.com/bwplotka/flagarize/internal/camelcase"
	"github.com/pkg/errors"
	"gopkg.in/alecthomas/kingpin.v2"
)

const flagTagName = "flagarize"

const (
	nameStructTagKey        = "name"
	helpStructTagKey        = "help"
	hiddenStructTagKey      = "hidden"
	requiredStructTagKey    = "required"
	defaultStructTagKey     = "default"
	envvarStructTagKey      = "envvar"
	shortStructTagKey       = "short"
	placeholderStructTagKey = "placeholder"
)

var supportedStuctTagKeys = []string{nameStructTagKey, helpStructTagKey, hiddenStructTagKey, requiredStructTagKey, defaultStructTagKey, envvarStructTagKey, shortStructTagKey, placeholderStructTagKey}

// ValueFlagarizer is the simplest way to extend flagarize to parse your custom type.
// If any field has `flagarize:` struct tag and it implements the ValueFlagarizer, this will be
// used by kingping to parse the flag value.
//
// For an example see: `./timeduration.go` or `./regexp.go`
type ValueFlagarizer interface {
	// FlagarizeSetValue is invoked on kinpgin.Parse with the flag value passed as string.
	// It is expected from this method to parse the string to the underlying type.
	// This method has to be a pointer receiver for the method to take effect.
	// Flagarize will return error otherwise.
	Set(s string) error
}

// Flagarizer is more advanced way to extend flagarize to parse a type. It allows to register
// more than one flag or register them in a custom way. It's ok for a method to register nothing.
// If any field implements `Flagarizer` this method will be invoked even if field does not
// have `flagarize:` struct tag.
//
// If the field implements both ValueFlagarizer and Flagarizer, only Flagarizer will be used.
//
// For an example usagesee: `./pathorcontent.go`
type Flagarizer interface {
	// Flagarize is invoked on Flagarize. If field type does not implement custom Flagarizer
	// default one will be used.
	// Tag argument is nil if no `flagarize` struct tag was specified. Otherwise it has parsed
	//`flagarize` struct tag.
	// The ptr argument is an address of the already allocated type, that can be used
	// by FlagRegisterer kingping *Var methods.
	Flagarize(r FlagRegisterer, tag *Tag, ptr unsafe.Pointer) error
}

// FlagRegisterer allows registering a flag.
type FlagRegisterer interface {
	Flag(name, help string) *kingpin.FlagClause
}

// KingpinRegistry allows registering a flag, getting a flag and registering a command.
// Example implementation is *kingpin.App.
type KingpinRegistry interface {
	FlagRegisterer
	Command(name, help string) *kingpin.CmdClause
	GetFlag(name string) *kingpin.FlagClause
}

type opts struct {
	elemSep string
}

func (o opts) apply(optFuncs ...OptFunc) opts {
	for _, optFunc := range optFuncs {
		optFunc(&o)
	}
	return o
}

// OptFunc sets values in opts structure.
type OptFunc func(opt *opts)

// WithElemSep sets custom divider for elements in flagarize struct tag. It is "|" by default.
func WithElemSep(val string) OptFunc { return func(opt *opts) { opt.elemSep = val } }

// Flagarize registers flags based on `flagarize:"..."` struct tags.
//
// If field is a type that implemented Flagarizer or ValueFlagaizer interface, the custom Flagarizer will be used
// instead of default one.
// IMPORTANT: It is expected that struct fields are filled with values only after kingpin.Application.Parse is invoked for example:
//
//
//	type ComponentAOptions struct {
//		Field1 []string `flagarize:"name=a.flag1|help=..."`
//	}
//
//	func ExampleFlagarize() {
//		// Create new kingpin app as usual.
//		a := kingpin.New(filepath.Base(os.Args[0]), "<Your CLI description>")
//
//		// Define you own config.
//		type ConfigForCLI struct {
//			Field1 string                   `flagarize:"name=flag1|help=...|default=something"`
//			Field2 *url.URL                 `flagarize:"name=flag2|help=...|placeholder=<URL>"`
//			Field3 int                      `flagarize:"name=flag3|help=...|default=2144"`
//			Field4 flagarize.TimeOrDuration `flagarize:"name=flag4|help=...|default=1m|placeholder=<time or duration>"`
//
//			NotFromFlags int
//
//			ComponentA ComponentAOptions
//		}
//
//		// Create new config.
//		cfg := &ConfigForCLI{}
//
//		// Flagarize it! (Register flags from config).
//		if err := flagarize.Flagarize(a, &cfg); err != nil {
//			fmt.Fprintln(os.Stderr, err)
//			os.Exit(2)
//		}
//
//		// You can define some fields as usual as well.
//		var notInConfigField time.Duration
//		a.Flag("some-field10", "...").
//			DurationVar(&notInConfigField)
//
//		// Parse flags as usual.
//		if _, err := a.Parse(os.Args[1:]); err != nil {
//			fmt.Fprintln(os.Stderr, err)
//			os.Exit(2)
//		}
//
//		// Config is filled with flags from value!
//		_ = cfg.Field1
//	}
//
func Flagarize(r KingpinRegistry, s interface{}, o ...OptFunc) error {
	if r == nil {
		return errors.New("flagarize: FlagRegisterer cannot be nil")
	}
	if s == nil {
		return errors.New("flagarize: object cannot be nil")
	}
	v := reflect.ValueOf(s)
	if v.Kind() != reflect.Ptr {
		return errors.New("flagarize: object must be a pointer to struct or interface")
	}
	if v.IsNil() {
		return errors.New("flagarize: object cannot be nil")
	}
	switch e := v.Elem(); e.Kind() {
	case reflect.Struct:
		if err := parseStruct(r, e, opts{
			elemSep: "|",
		}.apply(o...)); err != nil {
			return errors.Wrap(err, "flagarize")
		}
		return nil
	default:
		return errors.Errorf("object must be a pointer to struct or interface, got: %s", e)
	}
}

type dedupFlagRegisterer struct {
	KingpinRegistry
	duplicate string
}

func (d *dedupFlagRegisterer) Flag(name, help string) *kingpin.FlagClause {
	if d.GetFlag(name) != nil {
		d.duplicate = name
	}
	return d.KingpinRegistry.Flag(name, help)
}

func parseStruct(r KingpinRegistry, value reflect.Value, o opts) error {
	helpVars := parseHelpVars(value)
	for i := 0; i < value.NumField(); i++ {
		field := value.Type().Field(i)
		fieldValue := value.Field(i)

		tag, err := parseTag(field, helpVars[field.Name], o.elemSep)
		if err != nil {
			return errors.Wrap(err, "parse flagarize tags")
		}

		if tag == nil {
			if fieldValue.Kind() == reflect.Struct && (field.PkgPath == "" || field.Anonymous) {
				if err := parseStruct(r, fieldValue, o); err != nil {
					return err
				}
			}
			continue
		}

		if field.PkgPath != "" {
			return errors.Errorf("flagarize struct Tag found on private field %q; it has to be exported", field.Name)
		}

		if r.GetFlag(tag.Name) != nil {
			return errors.Errorf("flagarize field %s was already registered", field.Name)
		}

		if !fieldValue.CanAddr() {
			return errors.Errorf("flagarize struct Tag found on non-addressable field %q", field.Name)
		}

		// Favor custom Flagarizers if specified.
		d := &dedupFlagRegisterer{KingpinRegistry: r}
		ok, err := invokeFlagarizersIfImplements(d, tag, fieldValue, field.Name)
		if err != nil {
			return err
		}
		if ok {
			if d.duplicate != "" {
				return errors.Errorf("flagarize field %s was already registered", d.duplicate)
			}
			continue
		}

		clause := tag.Flag(r)
		switch fieldValue.Interface().(type) {
		// TODO(bwplotka): Support Enums and maybe hex?
		case string:
			clause.StringVar((*string)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case bool:
			clause.BoolVar((*bool)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case uint:
			clause.UintVar((*uint)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case uint8:
			clause.Uint8Var((*uint8)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case uint16:
			clause.Uint16Var((*uint16)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case uint32:
			clause.Uint32Var((*uint32)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case uint64:
			clause.Uint64Var((*uint64)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case int:
			clause.IntVar((*int)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case int8:
			clause.Int8Var((*int8)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case int16:
			clause.Int16Var((*int16)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case int32:
			clause.Int32Var((*int32)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case int64:
			clause.Int64Var((*int64)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case float32:
			clause.Float32Var((*float32)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case float64:
			clause.Float64Var((*float64)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case time.Duration:
			clause.DurationVar((*time.Duration)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case net.IP:
			clause.IPVar((*net.IP)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case units.Base2Bytes:
			clause.BytesVar((*units.Base2Bytes)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case *net.TCPAddr:
			clause.TCPVar((**net.TCPAddr)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case *url.URL:
			clause.URLVar((**url.URL)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case *os.File:
			clause.FileVar((**os.File)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []bool:
			clause.BoolListVar((*[]bool)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []string:
			clause.StringsVar((*[]string)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []int:
			clause.IntsVar((*[]int)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []int8:
			clause.Int8ListVar((*[]int8)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []int16:
			clause.Int16ListVar((*[]int16)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []int32:
			clause.Int32ListVar((*[]int32)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []int64:
			clause.Int64ListVar((*[]int64)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []uint:
			clause.UintsVar((*[]uint)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []uint8:
			clause.Uint8ListVar((*[]uint8)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []uint16:
			clause.Uint16ListVar((*[]uint16)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []uint32:
			clause.Uint32ListVar((*[]uint32)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []uint64:
			clause.Uint64ListVar((*[]uint64)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []float32:
			clause.Float32ListVar((*[]float32)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []float64:
			clause.Float64ListVar((*[]float64)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []time.Duration:
			clause.DurationListVar((*[]time.Duration)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []net.IP:
			clause.IPListVar((*[]net.IP)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []*net.TCPAddr:
			clause.TCPListVar((*[]*net.TCPAddr)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case []*url.URL:
			clause.URLListVar((*[]*url.URL)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		case map[string]string:
			if fieldValue.IsNil() {
				fieldValue.Set(reflect.MakeMap(fieldValue.Type()))
			}
			clause.StringMapVar((*map[string]string)(unsafe.Pointer(fieldValue.Addr().Pointer())))
		default:
			return errors.Errorf("flagarize struct Tag found on not supported type %s %T for field %q", fieldValue.Kind().String(), fieldValue.Interface(), field.Name)

		}
	}
	return nil
}

func allocPtrIfNil(fieldValue reflect.Value) {
	if fieldValue.Kind() == reflect.Ptr {
		if fieldValue.IsNil() {
			fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
		}
	}
}
func invokeFlagarizersIfImplements(r KingpinRegistry, tag *Tag, fieldValue reflect.Value, name string) (impl bool, err error) {
	if _, ok := fieldValue.Interface().(Flagarizer); ok {
		allocPtrIfNil(fieldValue)
		// Do fieldValue.Interface() once more as after alloc the copied value is not changed.
		if err := invokeCustomFlagarizer(r, fieldValue.Interface().(Flagarizer), tag, fieldValue, name); err != nil {
			return true, err
		}
		return true, nil
	}

	if _, ok := fieldValue.Addr().Interface().(Flagarizer); ok {
		allocPtrIfNil(fieldValue)
		// Do fieldValue.Interface() once more as after alloc the copied value is not changed.
		if err := invokeCustomFlagarizer(r, fieldValue.Addr().Interface().(Flagarizer), tag, fieldValue, name); err != nil {
			return true, err
		}
		return true, nil
	}

	if _, ok := fieldValue.Interface().(ValueFlagarizer); ok {
		allocPtrIfNil(fieldValue)
		// Do fieldValue.Interface() once more as after alloc the copied value is not changed.
		if err := invokeCustomValueFlagarizer(r, fieldValue.Interface().(ValueFlagarizer), tag, fieldValue, name); err != nil {
			return true, err
		}
		return true, nil
	}

	if _, ok := fieldValue.Addr().Interface().(ValueFlagarizer); ok {
		allocPtrIfNil(fieldValue)
		// Do fieldValue.Interface() once more as after alloc the copied value is not changed.
		if err := invokeCustomValueFlagarizer(r, fieldValue.Addr().Interface().(ValueFlagarizer), tag, fieldValue, name); err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

func invokeCustomFlagarizer(r KingpinRegistry, f Flagarizer, tag *Tag, fieldValue reflect.Value, name string) error {
	if fieldValue.Kind() != reflect.Ptr {
		fieldValue = fieldValue.Addr()
	}
	if fieldValue.IsNil() {
		v := reflect.New(fieldValue.Type().Elem())
		fieldValue.Set(v)
	}

	if fieldValue.Elem().MethodByName("Flagarize").IsValid() {
		return errors.Errorf("flagarize field %q custom Flagarizer is non receiver pointer", name)
	}

	if err := f.Flagarize(r, tag, unsafe.Pointer(fieldValue.Pointer())); err != nil {
		return errors.Wrapf(err, "custom Flagarizer for field %s", name)
	}
	return nil
}

type flagarizeValue struct {
	ValueFlagarizer
	def string
}

func (f *flagarizeValue) String() string {
	return f.def
}

func invokeCustomValueFlagarizer(r KingpinRegistry, vf ValueFlagarizer, tag *Tag, fieldValue reflect.Value, name string) error {
	if fieldValue.Kind() != reflect.Ptr {
		fieldValue = fieldValue.Addr()
	}
	if fieldValue.IsNil() {
		v := reflect.New(fieldValue.Type().Elem())
		fieldValue.Set(v)
	}

	if fieldValue.Elem().MethodByName("Set").IsValid() {
		return errors.Errorf("flagarize field %q custom ValueFlagarizer is non receiver pointer", name)
	}

	tag.Flag(r).SetValue(&flagarizeValue{ValueFlagarizer: vf, def: tag.DefaultValue})
	return nil
}

type Tag struct {
	Name         string
	Short        rune
	EnvName      string
	Help         string
	DefaultValue string
	PlaceHolder  string
	Hidden       bool
	Required     bool
}

func (t *Tag) Flag(r FlagRegisterer) *kingpin.FlagClause {
	c := r.Flag(t.Name, t.Help).Short(t.Short)
	if t.Hidden {
		c.Hidden()
	}
	if t.Required {
		c.Required()
	}
	if t.DefaultValue != "" {
		c.Default(t.DefaultValue)
	}
	if t.EnvName != "" {
		c.Envar(t.EnvName)
	}
	if t.PlaceHolder != "" {
		c.PlaceHolder(t.PlaceHolder)
	}
	return c
}

func parseHelpVars(structVal reflect.Value) map[string]*string {
	helpVars := map[string]*string{}
	for i := 0; i < structVal.NumField(); i++ {
		name := structVal.Type().Field(i).Name

		if !strings.HasSuffix(name, "FlagarizeHelp") || structVal.Field(i).Kind() != reflect.String || structVal.Field(i).String() == "" {
			continue
		}
		v := structVal.Field(i).String()
		helpVars[name[:len(name)-len("FlagarizeHelp")]] = &v
	}
	return helpVars
}

func parseTag(field reflect.StructField, helpVar *string, elemSep string) (*Tag, error) {
	val, ok := field.Tag.Lookup(flagTagName)
	if !ok {
		return nil, nil
	}

	f := &Tag{}
	if val != "" {
		for _, t := range strings.Split(val, elemSep) {
			kv := strings.Split(t, "=")
			if len(kv) == 1 || t == "" {
				return nil, errors.Errorf("flagarize: expected map-like Tag elements (e.g hidden=true), found non"+
					" supported format %q for field %q", t, field.Name)
			}
			switch kv[0] {
			case nameStructTagKey:
				f.Name = kv[1]
			case helpStructTagKey:
				f.Help = kv[1]
			case hiddenStructTagKey:
				f.Hidden = isTrue(kv[1])
			case requiredStructTagKey:
				f.Required = isTrue(kv[1])
			case defaultStructTagKey:
				f.DefaultValue = kv[1]
			case envvarStructTagKey:
				if kv[1] != strings.ToUpper(kv[1]) {
					return nil, errors.Errorf("flagarize: environment variable name has to be upper case, but it's not %q for field %q", kv[1], field.Name)
				}
				f.EnvName = kv[1]
			case shortStructTagKey:
				if len(kv[1]) > 1 {
					return nil, errors.Errorf("flagarize: short cannot be longer than one character got %q for field %q", kv[1], field.Name)
				}
				f.Short = rune(kv[1][0])
			case placeholderStructTagKey:
				f.PlaceHolder = kv[1]
			default:
				return nil, errors.Errorf("flagarize: expected map-like Tag elements (e.g hidden=true) separated with %s, found but"+
					" no supported key found %q for field %q; only %v are supported", elemSep, kv[0], field.Name, supportedStuctTagKeys)
			}
		}
	}
	if f.Name == "" || f.Name == "-" {
		f.Name = strings.ToLower(strings.Join(camelcase.Split(field.Name), "_"))
	}
	if f.Help == "" {
		if helpVar == nil {
			return nil, errors.Errorf("flagarize: no help=<help> in struct Tag for field %q and no help"+
				" var; help=<help> in struct Tag or \"%s_\" is required for help/usage of the flag; be helpful! :)", field.Name, field.Name)
		}
		f.Help = *helpVar
	}
	return f, nil
}

func isTrue(v string) bool {
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}
