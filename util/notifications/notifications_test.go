// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package notifications

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNotificationLifecycle tests adding, modifying, and deleting notifications.
func TestNotificationLifecycle(t *testing.T) {
	notifs := NewNotifications(10, nil)

	// Add a notification.
	notifs.AddNotification("Test Notification 1")

	// Check if the notification was added.
	notifications := notifs.Get()
	require.Len(t, notifications, 1, "Expected 1 notification after addition.")
	require.Equal(t, "Test Notification 1", notifications[0].Text, "Notification text mismatch.")
	require.True(t, notifications[0].Active, "Expected notification to be active.")

	// Modify the notification.
	notifs.AddNotification("Test Notification 1")
	notifications = notifs.Get()
	require.Len(t, notifications, 1, "Expected 1 notification after modification.")

	// Delete the notification.
	notifs.DeleteNotification("Test Notification 1")
	notifications = notifs.Get()
	require.Empty(t, notifications, "Expected no notifications after deletion.")
}

// TestSubscriberReceivesNotifications tests that a subscriber receives notifications, including modifications and deletions.
func TestSubscriberReceivesNotifications(t *testing.T) {
	notifs := NewNotifications(10, nil)

	// Subscribe to notifications.
	sub, unsubscribe, ok := notifs.Sub()
	require.True(t, ok)

	var wg sync.WaitGroup
	wg.Add(1)

	receivedNotifications := make([]Notification, 0)

	// Goroutine to listen for notifications.
	go func() {
		defer wg.Done()
		for notification := range sub {
			receivedNotifications = append(receivedNotifications, notification)
		}
	}()

	// Add notifications.
	notifs.AddNotification("Test Notification 1")
	notifs.AddNotification("Test Notification 2")

	// Modify a notification.
	notifs.AddNotification("Test Notification 1")

	// Delete a notification.
	notifs.DeleteNotification("Test Notification 2")

	// Wait for notifications to propagate.
	time.Sleep(100 * time.Millisecond)

	unsubscribe()
	wg.Wait() // Wait for the subscriber goroutine to finish.

	// Verify that we received the expected number of notifications.
	require.Len(t, receivedNotifications, 4, "Expected 4 notifications (2 active, 1 modified, 1 deleted).")

	// Check the content and state of received notifications.
	expected := []struct {
		Text   string
		Active bool
	}{
		{"Test Notification 1", true},
		{"Test Notification 2", true},
		{"Test Notification 1", true},
		{"Test Notification 2", false},
	}

	for i, n := range receivedNotifications {
		require.Equal(t, expected[i].Text, n.Text, "Notification text mismatch at index %d.", i)
		require.Equal(t, expected[i].Active, n.Active, "Notification active state mismatch at index %d.", i)
	}
}

// TestMultipleSubscribers tests that multiple subscribers receive notifications independently.
func TestMultipleSubscribers(t *testing.T) {
	notifs := NewNotifications(10, nil)

	// Subscribe two subscribers to notifications.
	sub1, unsubscribe1, ok1 := notifs.Sub()
	require.True(t, ok1)

	sub2, unsubscribe2, ok2 := notifs.Sub()
	require.True(t, ok2)

	var wg sync.WaitGroup
	wg.Add(2)

	receivedSub1 := make([]Notification, 0)
	receivedSub2 := make([]Notification, 0)

	// Goroutine for subscriber 1.
	go func() {
		defer wg.Done()
		for notification := range sub1 {
			receivedSub1 = append(receivedSub1, notification)
		}
	}()

	// Goroutine for subscriber 2.
	go func() {
		defer wg.Done()
		for notification := range sub2 {
			receivedSub2 = append(receivedSub2, notification)
		}
	}()

	// Add and delete notifications.
	notifs.AddNotification("Test Notification 1")
	notifs.DeleteNotification("Test Notification 1")

	// Wait for notifications to propagate.
	time.Sleep(100 * time.Millisecond)

	// Unsubscribe both.
	unsubscribe1()
	unsubscribe2()

	wg.Wait()

	// Both subscribers should have received the same 2 notifications.
	require.Len(t, receivedSub1, 2, "Expected 2 notifications for subscriber 1.")
	require.Len(t, receivedSub2, 2, "Expected 2 notifications for subscriber 2.")

	// Verify that both subscribers received the same notifications.
	for i := range 2 {
		require.Equal(t, receivedSub1[i], receivedSub2[i], "Subscriber notification mismatch at index %d.", i)
	}
}

// TestUnsubscribe tests that unsubscribing prevents further notifications from being received.
func TestUnsubscribe(t *testing.T) {
	notifs := NewNotifications(10, nil)

	// Subscribe to notifications.
	sub, unsubscribe, ok := notifs.Sub()
	require.True(t, ok)

	var wg sync.WaitGroup
	wg.Add(1)

	receivedNotifications := make([]Notification, 0)

	// Goroutine to listen for notifications.
	go func() {
		defer wg.Done()
		for notification := range sub {
			receivedNotifications = append(receivedNotifications, notification)
		}
	}()

	// Add a notification and then unsubscribe.
	notifs.AddNotification("Test Notification 1")
	time.Sleep(100 * time.Millisecond) // Allow time for notification delivery.
	unsubscribe()                      // Unsubscribe.

	// Add another notification after unsubscribing.
	notifs.AddNotification("Test Notification 2")

	// Wait for the subscriber goroutine to finish.
	wg.Wait()

	// Only the first notification should have been received.
	require.Len(t, receivedNotifications, 1, "Expected 1 notification before unsubscribe.")
	require.Equal(t, "Test Notification 1", receivedNotifications[0].Text, "Unexpected notification text.")
}

// TestMaxSubscribers tests that exceeding the max subscribers limit prevents additional subscriptions.
func TestMaxSubscribers(t *testing.T) {
	maxSubscribers := 2
	notifs := NewNotifications(maxSubscribers, nil)

	// Subscribe the maximum number of subscribers.
	_, unsubscribe1, ok1 := notifs.Sub()
	require.True(t, ok1, "Expected first subscription to succeed.")

	_, unsubscribe2, ok2 := notifs.Sub()
	require.True(t, ok2, "Expected second subscription to succeed.")

	// Try to subscribe more than the max allowed.
	_, _, ok3 := notifs.Sub()
	require.False(t, ok3, "Expected third subscription to fail due to max subscriber limit.")

	// Unsubscribe one subscriber and try again.
	unsubscribe1()

	_, unsubscribe4, ok4 := notifs.Sub()
	require.True(t, ok4, "Expected subscription to succeed after unsubscribing a subscriber.")

	// Clean up the subscriptions.
	unsubscribe2()
	unsubscribe4()
}
