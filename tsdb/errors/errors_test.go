// Copyright 2016 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package errors_test

import (
	stderrors "errors"
	"testing"

	pkgerrors "github.com/pkg/errors"
	"github.com/prometheus/prometheus/tsdb/errors"
	"github.com/stretchr/testify/require"
)

func TestNilMultiError(t *testing.T) {
	require.NoError(t, errors.NewMulti().Err())
	require.NoError(t, errors.NewMulti(nil, nil, nil).Err())

	e := errors.NewMulti()
	e.Add()
	require.NoError(t, e.Err())

	e = errors.NewMulti(nil, nil, nil)
	e.Add()
	require.NoError(t, e.Err())

	e = errors.NewMulti()
	e.Add(nil, nil, nil)
	require.NoError(t, e.Err())

	e = errors.NewMulti(nil, nil, nil)
	e.Add(nil, nil, nil)
	require.NoError(t, e.Err())
}

func TestMultiError(t *testing.T) {
	err := stderrors.New("test1")
	require.Error(t, errors.NewMulti(err).Err())
	require.Error(t, errors.NewMulti(nil, err, nil).Err())

	e := errors.NewMulti(err)
	e.Add()
	require.Error(t, e.Err())

	e = errors.NewMulti(nil, nil, nil)
	e.Add(err)
	require.Error(t, e.Err())

	e = errors.NewMulti(err)
	e.Add(nil, nil, nil)
	require.Error(t, e.Err())

	e = errors.NewMulti(nil, nil, nil)
	e.Add(nil, err, nil)
	require.Error(t, e.Err())

	require.Error(t, func() error {
		return e.Err()
	}())

	require.NoError(t, func() error {
		return errors.NewMulti(nil, nil, nil).Err()
	}())
}

func TestMultiError_Error(t *testing.T) {
	err := stderrors.New("test1")

	require.Equal(t, "test1", errors.NewMulti(err).Err().Error())
	require.Equal(t, "test1", errors.NewMulti(err, nil).Err().Error())
	require.Equal(t, "4 errors: test1; test1; test2; test3", errors.NewMulti(err, err, stderrors.New("test2"), nil, stderrors.New("test3")).Err().Error())
}

type customErr struct{ error }

type customErr2 struct{ error }

type customErr3 struct{ error }

func TestMultiError_As(t *testing.T) {
	err := customErr{error: stderrors.New("err1")}

	require.True(t, stderrors.As(err, &err))
	require.True(t, stderrors.As(err, &customErr{}))

	require.False(t, stderrors.As(err, &customErr2{}))
	require.False(t, stderrors.As(err, &customErr3{}))

	// This is just to show limitation of std As.
	require.False(t, stderrors.As(&err, &err))
	require.False(t, stderrors.As(&err, &customErr{}))
	require.False(t, stderrors.As(&err, &customErr2{}))
	require.False(t, stderrors.As(&err, &customErr3{}))

	e := errors.NewMulti(err).Err()
	require.True(t, stderrors.As(e, &customErr{}))
	same := errors.NewMulti(err).Err()
	require.True(t, stderrors.As(e, &same))
	require.False(t, stderrors.As(e, &customErr2{}))
	require.False(t, stderrors.As(e, &customErr3{}))

	e2 := errors.NewMulti(err, customErr3{error: stderrors.New("some")}).Err()
	require.True(t, stderrors.As(e2, &customErr{}))
	require.True(t, stderrors.As(e2, &customErr3{}))
	require.False(t, stderrors.As(e2, &customErr2{}))

	// Wrapped.
	e3 := pkgerrors.Wrap(errors.NewMulti(err, customErr3{}).Err(), "wrap")
	require.True(t, stderrors.As(e3, &customErr{}))
	require.True(t, stderrors.As(e3, &customErr3{}))
	require.False(t, stderrors.As(e3, &customErr2{}))

	// This is just to show limitation of std As.
	e4 := errors.NewMulti(err, &customErr3{}).Err()
	require.False(t, stderrors.As(e4, &customErr2{}))
	require.False(t, stderrors.As(e4, &customErr3{}))
}

func TestMultiError_Is(t *testing.T) {
	err := customErr{error: stderrors.New("err1")}

	require.True(t, stderrors.Is(err, err))
	require.True(t, stderrors.Is(err, customErr{error: err.error}))
	require.False(t, stderrors.Is(err, &err))
	require.False(t, stderrors.Is(err, customErr{}))
	require.False(t, stderrors.Is(err, customErr{error: stderrors.New("err1")}))
	require.False(t, stderrors.Is(err, customErr2{}))
	require.False(t, stderrors.Is(err, customErr3{}))

	require.True(t, stderrors.Is(&err, &err))
	require.False(t, stderrors.Is(&err, &customErr{error: err.error}))
	require.False(t, stderrors.Is(&err, &customErr2{}))
	require.False(t, stderrors.Is(&err, &customErr3{}))

	e := errors.NewMulti(err).Err()
	require.True(t, stderrors.Is(e, err))
	require.True(t, stderrors.Is(err, customErr{error: err.error}))
	require.True(t, stderrors.Is(e, e))
	require.True(t, stderrors.Is(e, errors.NewMulti(err).Err()))
	require.False(t, stderrors.Is(e, &err))
	require.False(t, stderrors.Is(err, customErr{}))
	require.False(t, stderrors.Is(e, customErr2{}))
	require.False(t, stderrors.Is(e, customErr3{}))

	e2 := errors.NewMulti(err, customErr3{}).Err()
	require.True(t, stderrors.Is(e2, err))
	require.True(t, stderrors.Is(e2, customErr3{}))
	require.True(t, stderrors.Is(e2, errors.NewMulti(err, customErr3{}).Err()))
	require.False(t, stderrors.Is(e2, errors.NewMulti(customErr3{}, err).Err()))
	require.False(t, stderrors.Is(e2, customErr{}))
	require.False(t, stderrors.Is(e2, customErr2{}))

	// Wrapped.
	e3 := pkgerrors.Wrap(errors.NewMulti(err, customErr3{}).Err(), "wrap")
	require.True(t, stderrors.Is(e3, err))
	require.True(t, stderrors.Is(e3, customErr3{}))
	require.False(t, stderrors.Is(e3, customErr{}))
	require.False(t, stderrors.Is(e3, customErr2{}))

	exact := &customErr3{}
	e4 := errors.NewMulti(err, exact).Err()
	require.True(t, stderrors.Is(e4, err))
	require.True(t, stderrors.Is(e4, exact))
	require.True(t, stderrors.Is(e4, errors.NewMulti(err, exact).Err()))
	require.False(t, stderrors.Is(e4, customErr{}))
	require.False(t, stderrors.Is(e4, customErr2{}))
	require.False(t, stderrors.Is(e4, &customErr3{}))
}

func TestMultiError_Count(t *testing.T) {
	err := customErr{error: stderrors.New("err1")}
	merr := errors.NewMulti()
	merr.Add(customErr3{})

	m, ok := errors.AsMulti(merr.Err())
	require.True(t, ok)
	require.Equal(t, 0, m.Count(err))
	require.Equal(t, 1, m.Count(customErr3{}))

	merr.Add(customErr3{})
	merr.Add(customErr3{})

	m, ok = errors.AsMulti(merr.Err())
	require.True(t, ok)
	require.Equal(t, 0, m.Count(err))
	require.Equal(t, 3, m.Count(customErr3{}))

	// Nest multi errors with wraps.
	merr2 := errors.NewMulti()
	merr2.Add(customErr3{})
	merr2.Add(customErr3{})
	merr2.Add(customErr3{})

	merr3 := errors.NewMulti()
	merr3.Add(customErr3{})
	merr3.Add(customErr3{})

	// Wrap it so Add cannot add inner errors in.
	merr2.Add(pkgerrors.Wrap(merr3.Err(), "wrap"))
	merr.Add(pkgerrors.Wrap(merr2.Err(), "wrap"))

	m, ok = errors.AsMulti(merr.Err())
	require.True(t, ok)
	require.Equal(t, 0, m.Count(err))
	require.Equal(t, 8, m.Count(customErr3{}))
}

func TestAsMulti(t *testing.T) {
	err := customErr{error: stderrors.New("err1")}
	merr := errors.NewMulti(err, customErr3{}).Err()
	wrapped := pkgerrors.Wrap(merr, "wrap")

	_, ok := errors.AsMulti(err)
	require.False(t, ok)

	m, ok := errors.AsMulti(merr)
	require.True(t, ok)
	require.True(t, stderrors.Is(m, merr))

	m, ok = errors.AsMulti(wrapped)
	require.True(t, ok)
	require.True(t, stderrors.Is(m, merr))
}
