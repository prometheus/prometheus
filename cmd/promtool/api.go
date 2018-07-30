package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
)

type prometheusAPIQuerier struct {
	api            v1.API
	requestTimeout time.Duration
	queryFunc      func() (string, error)
}

func newPrometheusAPIQuerier(serverURL string) (*prometheusAPIQuerier, error) {
	c, err := api.NewClient(api.Config{Address: serverURL})
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %s", err)
	}
	api := v1.NewAPI(c)
	return &prometheusAPIQuerier{
		api:            api,
		requestTimeout: defaultTimeout,
	}, nil
}

func (q *prometheusAPIQuerier) buildQueryFunc(fn interface{}, args ...interface{}) func() (string, error) {
	funcValue := reflect.ValueOf(fn)
	funcType := funcValue.Type()

	if funcType.Kind() != reflect.Func {
		panic(fmt.Sprintf("expect reflect.Func but got %T", fn))
	}

	// It has one less paramter because the context.Contex paramter is created below
	if len(args) != funcType.NumIn()-1 {
		panic(fmt.Sprintf("number of parameters: expect %d but got %d", funcType.NumIn()-1, len(args)))
	}

	return func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), q.requestTimeout)
		inputParams := []reflect.Value{reflect.ValueOf(ctx)}
		for _, arg := range args {
			inputParams = append(inputParams, reflect.ValueOf(arg))
		}

		returnedValues := funcValue.Call(inputParams)
		cancel()

		var buf bytes.Buffer
		var err error
		for i, retVal := range returnedValues {
			valType := funcType.Out(i)
			if valType == reflect.TypeOf(new(error)).Elem() {
				if !retVal.IsNil() {
					err = retVal.Interface().(error)
				}
			} else if valType.Kind() == reflect.Slice {
				// It's a workaround for slice types. Types in commom/model should implement String()
				len := retVal.Len()
				for i := 0; i < len; i++ {
					buf.WriteString(fmt.Sprintln(retVal.Index(i)))
				}
			} else {
				buf.WriteString(fmt.Sprintln(retVal))
			}
		}
		return buf.String(), err
	}
}

func (q *prometheusAPIQuerier) setQueryFunc(fn interface{}, args ...interface{}) {
	q.queryFunc = q.buildQueryFunc(fn, args...)
}

func (q *prometheusAPIQuerier) exec() int {
	if q.queryFunc == nil {
		panic("expect queryFunc to be set in advance")
	}

	val, err := q.queryFunc()
	if err != nil {
		fmt.Fprintln(os.Stderr, "query error:", err)
		return 1
	}

	fmt.Print(val)
	return 0
}
