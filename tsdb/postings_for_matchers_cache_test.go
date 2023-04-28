package tsdb

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/index"
)

func TestPostingsForMatchersCache(t *testing.T) {
	const testCacheSize = 5
	// newPostingsForMatchersCache tests the NewPostingsForMatcherCache constructor, but overrides the postingsForMatchers func
	newPostingsForMatchersCache := func(ttl time.Duration, pfm func(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error), timeMock *timeNowMock, force bool) *PostingsForMatchersCache {
		c := NewPostingsForMatchersCache(ttl, testCacheSize, force)
		if c.postingsForMatchers == nil {
			t.Fatalf("NewPostingsForMatchersCache() didn't assign postingsForMatchers func")
		}
		c.postingsForMatchers = pfm
		c.timeNow = timeMock.timeNow
		return c
	}

	t.Run("happy case one call", func(t *testing.T) {
		for _, concurrent := range []bool{true, false} {
			t.Run(fmt.Sprintf("concurrent=%t", concurrent), func(t *testing.T) {
				expectedMatchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")}
				expectedPostingsErr := fmt.Errorf("failed successfully")

				c := newPostingsForMatchersCache(defaultPostingsForMatchersCacheTTL, func(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error) {
					require.IsType(t, indexForPostingsMock{}, ix, "Incorrect IndexPostingsReader was provided to PostingsForMatchers, expected the mock, was given %v (%T)", ix, ix)
					require.Equal(t, expectedMatchers, ms, "Wrong label matchers provided, expected %v, got %v", expectedMatchers, ms)
					return index.ErrPostings(expectedPostingsErr), nil
				}, &timeNowMock{}, false)

				p, err := c.PostingsForMatchers(indexForPostingsMock{}, concurrent, expectedMatchers...)
				require.NoError(t, err)
				require.NotNil(t, p)
				require.Equal(t, p.Err(), expectedPostingsErr, "Expected ErrPostings with err %q, got %T with err %q", expectedPostingsErr, p, p.Err())
			})
		}
	})

	t.Run("err returned", func(t *testing.T) {
		expectedMatchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")}
		expectedErr := fmt.Errorf("failed successfully")

		c := newPostingsForMatchersCache(defaultPostingsForMatchersCacheTTL, func(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error) {
			return nil, expectedErr
		}, &timeNowMock{}, false)

		_, err := c.PostingsForMatchers(indexForPostingsMock{}, true, expectedMatchers...)
		require.Equal(t, expectedErr, err)
	})

	t.Run("happy case multiple concurrent calls: two same one different", func(t *testing.T) {
		for _, cacheEnabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("cacheEnabled=%t", cacheEnabled), func(t *testing.T) {
				for _, forced := range []bool{true, false} {
					concurrent := !forced
					t.Run(fmt.Sprintf("forced=%t", forced), func(t *testing.T) {
						calls := [][]*labels.Matcher{
							{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")},                                                         // 1
							{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")},                                                         // 1 same
							{labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar")},                                                        // 2: different match type
							{labels.MustNewMatcher(labels.MatchEqual, "diff", "bar")},                                                        // 3: different name
							{labels.MustNewMatcher(labels.MatchEqual, "foo", "diff")},                                                        // 4: different value
							{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"), labels.MustNewMatcher(labels.MatchEqual, "boo", "bam")}, // 5
							{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"), labels.MustNewMatcher(labels.MatchEqual, "boo", "bam")}, // 5 same
						}

						// we'll identify results by each call's error, and the error will be the string value of the first matcher
						matchersString := func(ms []*labels.Matcher) string {
							s := strings.Builder{}
							for i, m := range ms {
								if i > 0 {
									s.WriteByte(',')
								}
								s.WriteString(m.String())
							}
							return s.String()
						}
						expectedResults := make([]string, len(calls))
						for i, c := range calls {
							expectedResults[i] = c[0].String()
						}

						expectedPostingsForMatchersCalls := 5
						// we'll block all the calls until we receive the exact amount. if we receive more, WaitGroup will panic
						called := make(chan struct{}, expectedPostingsForMatchersCalls)
						release := make(chan struct{})
						var ttl time.Duration
						if cacheEnabled {
							ttl = defaultPostingsForMatchersCacheTTL
						}
						c := newPostingsForMatchersCache(ttl, func(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error) {
							select {
							case called <- struct{}{}:
							default:
							}
							<-release
							return nil, fmt.Errorf(matchersString(ms))
						}, &timeNowMock{}, forced)

						results := make([]string, len(calls))
						resultsWg := sync.WaitGroup{}
						resultsWg.Add(len(calls))

						// perform all calls
						for i := 0; i < len(calls); i++ {
							go func(i int) {
								_, err := c.PostingsForMatchers(indexForPostingsMock{}, concurrent, calls[i]...)
								results[i] = err.Error()
								resultsWg.Done()
							}(i)
						}

						// wait until all calls arrive to the mocked function
						for i := 0; i < expectedPostingsForMatchersCalls; i++ {
							<-called
						}

						// let them all return
						close(release)

						// wait for the results
						resultsWg.Wait()

						// check that we got correct results
						for i, c := range calls {
							require.Equal(t, matchersString(c), results[i], "Call %d should have returned error %q, but got %q instead", i, matchersString(c), results[i])
						}
					})
				}
			})
		}
	})

	t.Run("with concurrent==false, result is not cached", func(t *testing.T) {
		expectedMatchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")}

		var call int
		c := newPostingsForMatchersCache(defaultPostingsForMatchersCacheTTL, func(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error) {
			call++
			return index.ErrPostings(fmt.Errorf("result from call %d", call)), nil
		}, &timeNowMock{}, false)

		// first call, fills the cache
		p, err := c.PostingsForMatchers(indexForPostingsMock{}, false, expectedMatchers...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 1")

		// second call within the ttl (we didn't advance the time), should call again because concurrent==false
		p, err = c.PostingsForMatchers(indexForPostingsMock{}, false, expectedMatchers...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 2")
	})

	t.Run("with cache disabled, result is not cached", func(t *testing.T) {
		expectedMatchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")}

		var call int
		c := newPostingsForMatchersCache(0, func(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error) {
			call++
			return index.ErrPostings(fmt.Errorf("result from call %d", call)), nil
		}, &timeNowMock{}, false)

		// first call, fills the cache
		p, err := c.PostingsForMatchers(indexForPostingsMock{}, true, expectedMatchers...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 1")

		// second call within the ttl (we didn't advance the time), should call again because concurrent==false
		p, err = c.PostingsForMatchers(indexForPostingsMock{}, true, expectedMatchers...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 2")
	})

	t.Run("cached value is returned, then it expires", func(t *testing.T) {
		timeNow := &timeNowMock{}
		expectedMatchers := []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
		}

		var call int
		c := newPostingsForMatchersCache(defaultPostingsForMatchersCacheTTL, func(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error) {
			call++
			return index.ErrPostings(fmt.Errorf("result from call %d", call)), nil
		}, timeNow, false)

		// first call, fills the cache
		p, err := c.PostingsForMatchers(indexForPostingsMock{}, true, expectedMatchers...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 1")

		timeNow.advance(defaultPostingsForMatchersCacheTTL / 2)

		// second call within the ttl, should use the cache
		p, err = c.PostingsForMatchers(indexForPostingsMock{}, true, expectedMatchers...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 1")

		timeNow.advance(defaultPostingsForMatchersCacheTTL / 2)

		// third call is after ttl (exactly), should call again
		p, err = c.PostingsForMatchers(indexForPostingsMock{}, true, expectedMatchers...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 2")
	})

	t.Run("cached value is evicted because cache exceeds max size", func(t *testing.T) {
		timeNow := &timeNowMock{}
		calls := make([][]*labels.Matcher, testCacheSize)
		for i := range calls {
			calls[i] = []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "matchers", fmt.Sprintf("%d", i))}
		}

		callsPerMatchers := map[string]int{}
		c := newPostingsForMatchersCache(defaultPostingsForMatchersCacheTTL, func(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error) {
			k := matchersKey(ms)
			callsPerMatchers[k]++
			return index.ErrPostings(fmt.Errorf("result from call %d", callsPerMatchers[k])), nil
		}, timeNow, false)

		// each one of the first testCacheSize calls is cached properly
		for _, matchers := range calls {
			// first call
			p, err := c.PostingsForMatchers(indexForPostingsMock{}, true, matchers...)
			require.NoError(t, err)
			require.EqualError(t, p.Err(), "result from call 1")

			// cached value
			p, err = c.PostingsForMatchers(indexForPostingsMock{}, true, matchers...)
			require.NoError(t, err)
			require.EqualError(t, p.Err(), "result from call 1")
		}

		// one extra call is made, which is cached properly, but evicts the first cached value
		someExtraMatchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")}
		// first call
		p, err := c.PostingsForMatchers(indexForPostingsMock{}, true, someExtraMatchers...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 1")

		// cached value
		p, err = c.PostingsForMatchers(indexForPostingsMock{}, true, someExtraMatchers...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 1")

		// make first call again, it's calculated again
		p, err = c.PostingsForMatchers(indexForPostingsMock{}, true, calls[0]...)
		require.NoError(t, err)
		require.EqualError(t, p.Err(), "result from call 2")
	})
}

type indexForPostingsMock struct{}

func (idx indexForPostingsMock) LabelValues(string, ...*labels.Matcher) ([]string, error) {
	panic("implement me")
}

func (idx indexForPostingsMock) Postings(string, ...string) (index.Postings, error) {
	panic("implement me")
}

// timeNowMock offers a mockable time.Now() implementation
// empty value is ready to be used, and it should not be copied (use a reference)
type timeNowMock struct {
	sync.Mutex
	now time.Time
}

// timeNow can be used as a mocked replacement for time.Now()
func (t *timeNowMock) timeNow() time.Time {
	t.Lock()
	defer t.Unlock()
	if t.now.IsZero() {
		t.now = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	}
	return t.now
}

// advance advances the mocked time.Now() value
func (t *timeNowMock) advance(d time.Duration) {
	t.Lock()
	defer t.Unlock()
	if t.now.IsZero() {
		t.now = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	}
	t.now = t.now.Add(d)
}

func BenchmarkMatchersKey(b *testing.B) {
	const totalMatchers = 10
	const matcherSets = 100
	sets := make([][]*labels.Matcher, matcherSets)
	for i := 0; i < matcherSets; i++ {
		for j := 0; j < totalMatchers; j++ {
			sets[i] = append(sets[i], labels.MustNewMatcher(labels.MatchType(j%4), fmt.Sprintf("%d_%d", i*13, j*65537), fmt.Sprintf("%x_%x", i*127, j*2_147_483_647)))
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matchersKey(sets[i%matcherSets])
	}
}
