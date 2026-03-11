// Copyright The Prometheus Authors
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
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	ConfigurationUnsuccessful = "Configuration reload has failed."
	StartingUp                = "Prometheus is starting and replaying the write-ahead log (WAL)."
	ShuttingDown              = "Prometheus is shutting down and gracefully stopping all operations."
)

// Notification represents an individual notification message.
type Notification struct {
	Text   string    `json:"text"`
	Date   time.Time `json:"date"`
	Active bool      `json:"active"`
}

// Notifications stores a list of Notification objects.
// It also manages live subscribers that receive notifications via channels.
type Notifications struct {
	mu             sync.Mutex
	notifications  []Notification
	subscribers    map[chan Notification]struct{} // Active subscribers.
	maxSubscribers int

	subscriberGauge      prometheus.Gauge
	notificationsSent    prometheus.Counter
	notificationsDropped prometheus.Counter
}

// NewNotifications creates a new Notifications instance.
func NewNotifications(maxSubscribers int, reg prometheus.Registerer) *Notifications {
	n := &Notifications{
		subscribers:    make(map[chan Notification]struct{}),
		maxSubscribers: maxSubscribers,
		subscriberGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "notification_active_subscribers",
			Help:      "The current number of active notification subscribers.",
		}),
		notificationsSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "notification_updates_sent_total",
			Help:      "Total number of notification updates sent.",
		}),
		notificationsDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "api",
			Name:      "notification_updates_dropped_total",
			Help:      "Total number of notification updates dropped.",
		}),
	}

	if reg != nil {
		reg.MustRegister(n.subscriberGauge, n.notificationsSent, n.notificationsDropped)
	}

	return n
}

// AddNotification adds a new notification or updates the timestamp if it already exists.
func (n *Notifications) AddNotification(text string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for i, notification := range n.notifications {
		if notification.Text == text {
			n.notifications[i].Date = time.Now()

			n.notifySubscribers(n.notifications[i])
			return
		}
	}

	newNotification := Notification{
		Text:   text,
		Date:   time.Now(),
		Active: true,
	}
	n.notifications = append(n.notifications, newNotification)

	n.notifySubscribers(newNotification)
}

// notifySubscribers sends a notification to all active subscribers.
func (n *Notifications) notifySubscribers(notification Notification) {
	for sub := range n.subscribers {
		// Non-blocking send to avoid subscriber blocking issues.
		n.notificationsSent.Inc()
		select {
		case sub <- notification:
			// Notification sent to the subscriber.
		default:
			// Drop the notification if the subscriber's channel is full.
			n.notificationsDropped.Inc()
		}
	}
}

// DeleteNotification removes the first notification that matches the provided text.
// The deleted notification is sent to subscribers with Active: false before being removed.
func (n *Notifications) DeleteNotification(text string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Iterate through the notifications to find the matching text.
	for i, notification := range n.notifications {
		if notification.Text == text {
			// Mark the notification as inactive and notify subscribers.
			notification.Active = false
			n.notifySubscribers(notification)

			// Remove the notification from the list.
			n.notifications = append(n.notifications[:i], n.notifications[i+1:]...)
			return
		}
	}
}

// Get returns a copy of the list of notifications for safe access outside the struct.
func (n *Notifications) Get() []Notification {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Return a copy of the notifications slice to avoid modifying the original slice outside.
	notificationsCopy := make([]Notification, len(n.notifications))
	copy(notificationsCopy, n.notifications)
	return notificationsCopy
}

// Sub allows a client to subscribe to live notifications.
// It returns a channel where the subscriber will receive notifications and a function to unsubscribe.
// Each subscriber has its own goroutine to handle notifications and prevent blocking.
func (n *Notifications) Sub() (<-chan Notification, func(), bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(n.subscribers) >= n.maxSubscribers {
		return nil, nil, false
	}

	ch := make(chan Notification, 10) // Buffered channel to prevent blocking.

	// Add the new subscriber to the list.
	n.subscribers[ch] = struct{}{}
	n.subscriberGauge.Set(float64(len(n.subscribers)))

	// Send all current notifications to the new subscriber.
	for _, notification := range n.notifications {
		ch <- notification
	}

	// Unsubscribe function to remove the channel from subscribers.
	unsubscribe := func() {
		n.mu.Lock()
		defer n.mu.Unlock()

		// Close the channel and remove it from the subscribers map.
		close(ch)
		delete(n.subscribers, ch)
		n.subscriberGauge.Set(float64(len(n.subscribers)))
	}

	return ch, unsubscribe, true
}
