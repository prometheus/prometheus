package utility

import (
	"fmt"
	"testing"
	"time"
)

func TestGroupSuccess(t *testing.T) {
	uncertaintyGroup := NewUncertaintyGroup(10)

	for i := 0; i < 10; i++ {
		go uncertaintyGroup.Succeed()
	}

	result := make(chan bool)
	go func() {
		result <- uncertaintyGroup.Wait()
	}()
	select {
	case v := <-result:
		if !v {
			t.Fatal("expected success")
		}
	case <-time.After(time.Second):
		t.Fatal("deadline exceeded")
	}
}

func TestGroupFail(t *testing.T) {
	uncertaintyGroup := NewUncertaintyGroup(10)

	for i := 0; i < 10; i++ {
		go uncertaintyGroup.Fail(fmt.Errorf(""))
	}

	result := make(chan bool)
	go func() {
		result <- uncertaintyGroup.Wait()
	}()
	select {
	case v := <-result:
		if v {
			t.Fatal("expected failure")
		}
	case <-time.After(time.Second):
		t.Fatal("deadline exceeded")
	}
}

func TestGroupFailMix(t *testing.T) {
	uncertaintyGroup := NewUncertaintyGroup(10)

	for i := 0; i < 10; i++ {
		go func(i int) {
			switch {
			case i%2 == 0:
				uncertaintyGroup.Fail(fmt.Errorf(""))
			default:
				uncertaintyGroup.Succeed()
			}
		}(i)
	}

	result := make(chan bool)
	go func() {
		result <- uncertaintyGroup.Wait()
	}()
	select {
	case v := <-result:
		if v {
			t.Fatal("expected failure")
		}
	case <-time.After(time.Second):
		t.Fatal("deadline exceeded")
	}
}
