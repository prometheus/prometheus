package api

import (
	"fmt"
	"time"
)

type pollingHandler func() (exit bool, state interface{}, err error)

func poll(handler pollingHandler, timeout time.Duration) (chan (interface{}), chan (interface{}), chan (error)) {

	compChan := make(chan interface{})
	progChan := make(chan interface{})
	errChan := make(chan error)

	tick := time.Tick(5 * time.Second)
	bomb := time.After(timeout)

	go func() {
		for {
			select {
			case <-tick:
				exit, state, err := handler()
				if err != nil {
					errChan <- fmt.Errorf("Failed: poll: %s", err)
					return
				}
				if state != nil {
					progChan <- state
					if exit {
						compChan <- state
						return
					}
				}
			case <-bomb:
				errChan <- fmt.Errorf("Timeout")
				return
			}
		}
	}()
	return compChan, progChan, errChan
}

func blockingPoll(handler pollingHandler, timeout time.Duration) error {
	c, p, e := poll(handler, timeout)
	for {
		select {
		case <-c:
			return nil
		case <-p:
			// noop
		case err := <-e:
			return err
		}
	}
}

type hasAvailable interface {
	IsAvailable() bool
}
type hasFailed interface {
	IsFailed() bool
}

func waitingForAvailableFunc(readFunc func() (hasAvailable, error), maxRetry int) func() (bool, interface{}, error) {
	counter := 0
	return func() (bool, interface{}, error) {
		counter++
		v, err := readFunc()
		if err != nil {
			if maxRetry > 0 && counter < maxRetry {
				return false, nil, nil
			}
			return false, nil, err
		}
		if v == nil {
			return false, nil, fmt.Errorf("readFunc returns nil")
		}

		if v.IsAvailable() {
			return true, v, nil
		}
		if f, ok := v.(hasFailed); ok && f.IsFailed() {
			return false, v, fmt.Errorf("InstanceState is failed: %#v", v)
		}

		return false, v, nil
	}
}

type hasUpDown interface {
	IsUp() bool
	IsDown() bool
}

func waitingForUpFunc(readFunc func() (hasUpDown, error), maxRetry int) func() (bool, interface{}, error) {
	counter := 0
	return func() (bool, interface{}, error) {
		counter++
		v, err := readFunc()
		if err != nil {
			if maxRetry > 0 && counter < maxRetry {
				return false, nil, nil
			}
			return false, nil, err
		}
		if v == nil {
			return false, nil, fmt.Errorf("readFunc returns nil")
		}

		if v.IsUp() {
			return true, v, nil
		}
		return false, v, nil
	}
}

func waitingForDownFunc(readFunc func() (hasUpDown, error), maxRetry int) func() (bool, interface{}, error) {
	counter := 0
	return func() (bool, interface{}, error) {
		counter++
		v, err := readFunc()
		if err != nil {
			if maxRetry > 0 && counter < maxRetry {
				return false, nil, nil
			}
			return false, nil, err
		}
		if v == nil {
			return false, nil, fmt.Errorf("readFunc returns nil")
		}

		if v.IsDown() {
			return true, v, nil
		}
		return false, v, nil
	}
}

func waitingForReadFunc(readFunc func() (interface{}, error), maxRetry int) func() (bool, interface{}, error) {
	counter := 0
	return func() (bool, interface{}, error) {
		counter++
		v, err := readFunc()
		if err != nil {
			if maxRetry > 0 && counter < maxRetry {
				return false, nil, nil
			}
			return false, nil, err
		}
		if v != nil {
			return true, v, nil
		}
		return false, v, nil
	}
}
