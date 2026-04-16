package eventhub

import (
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	"api/external/trans"
	"context"
	"encoding/json"
	"errors"
	"file/internal/data"
	"file/internal/data/rpc"
	"fmt"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/gofrs/uuid"
	"github.com/samber/lo"
)

type Subscriber interface {
	ID() string
	Ch() chan *Event
	Publish(evt Event)
	Stop()
	Buffer() []*Event
	// Owner returns the owner of the subscriber.
	Owner() (*userpb.User, error)
	// Online returns whether the subscriber is online.
	Online() bool
	// OfflineSince returns when the subscriber went offline.
	// Returns zero time if the subscriber is online.
	OfflineSince() time.Time
}

const (
	userCacheTTL = 1 * time.Hour
)

type subscriber struct {
	mu            sync.Mutex
	userClient    pbuser.UserClient
	fsEventClient data.FsEventClient
	l             *log.Helper

	id  string
	uid int
	ch  chan *Event

	// Online status
	online       bool
	offlineSince time.Time

	// Debounce buffer for pending events
	buffer        []*Event
	timer         *time.Timer
	offlineMaxAge time.Duration
	debounceDelay time.Duration

	// Owner info
	ownerCached *userpb.User
	cachedAt    time.Time

	// Close signal
	closed   bool
	closedCh chan struct{}
}

func newSubscriber(ctx context.Context, id string, userClient pbuser.UserClient, fsEventClient data.FsEventClient, maxAge, debounceDelay time.Duration, l *log.Helper) (*subscriber, error) {
	user := trans.FromContext(ctx)
	if user == nil || data.IsAnonymousUser(user) {
		return nil, errors.New("user not found")
	}

	return &subscriber{
		id:            id,
		ch:            make(chan *Event, bufSize),
		userClient:    userClient,
		fsEventClient: fsEventClient,

		ownerCached:   user,
		uid:           int(user.Id),
		cachedAt:      time.Now(),
		online:        true,
		closedCh:      make(chan struct{}),
		offlineMaxAge: maxAge,
		debounceDelay: debounceDelay,
		l:             l,
	}, nil
}

func (s *subscriber) ID() string {
	return s.id
}

func (s *subscriber) Ch() chan *Event {
	return s.ch
}

func (s *subscriber) Online() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.online
}

func (s *subscriber) OfflineSince() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offlineSince
}

func (s *subscriber) Owner() (*userpb.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.ownerLocked()
}

func (s *subscriber) ownerLocked() (*userpb.User, error) {
	if time.Since(s.cachedAt) > userCacheTTL || s.ownerCached == nil {
		user, err := rpc.GetUserInfo(context.Background(), s.uid, s.userClient)
		if err != nil {
			return nil, fmt.Errorf("failed to get login user: %w", err)
		}

		s.ownerCached = user
		s.cachedAt = time.Now()
	}

	return s.ownerCached, nil
}

// Publish adds an event to the buffer and starts/resets the debounce timer.
// Events will be flushed to the channel after the debounce delay.
// If the subscriber is offline, events are kept in the buffer only.
func (s *subscriber) Publish(evt Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.publishLocked(evt)
}

// publishLocked adds an event to the buffer and manages the debounce timer.
// Caller must hold s.mu.
func (s *subscriber) publishLocked(evt Event) {
	// Add event to buffer
	s.buffer = append(s.buffer, &evt)

	// Reset or start the debounce timer
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.debounceDelay, s.flush)
}

// flush sends all buffered events to the channel.
// Called by the debounce timer.
func (s *subscriber) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.flushLocked(context.Background())
}

// flushLocked sends all buffered events to the channel.
// Caller must hold s.mu.
func (s *subscriber) flushLocked(ctx context.Context) {
	if len(s.buffer) == 0 || s.closed {
		return
	}

	if !s.online {
		owner, err := s.ownerLocked()
		if err != nil {
			return
		}
		_ = s.fsEventClient.Create(ctx, int(owner.Id), uuid.FromStringOrNil(s.id), lo.Map(s.buffer, func(item *Event, index int) string {
			res, _ := json.Marshal(item)
			return string(res)
		})...)
	} else {
		// TODO: implement event merging logic here
		// For now, send all buffered events individually
		debouncedEvents := DebounceEvents(s.buffer)
		for _, evt := range debouncedEvents {
			select {
			case s.ch <- evt:
			default:
				// Non-blocking send; drop if subscriber is slow
			}
		}
	}

	// Clear the buffer
	s.buffer = nil
	s.timer = nil
}

// Stop cancels any pending debounce timer and flushes remaining events.
// Should be called before closing the subscriber.
func (s *subscriber) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	// Flush any remaining events before stopping
	s.flushLocked(context.Background())
}

// setOnline marks the subscriber as online and flushes any buffered events.
func (s *subscriber) setOnline(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.online = true
	s.ownerCached = nil
	s.offlineSince = time.Time{}

	// Retrieve events from inventory
	events, err := s.fsEventClient.TakeBySubscriber(ctx, uuid.FromStringOrNil(s.id), s.uid)
	if err != nil {
		s.l.Errorf("Failed to get events from inventory: %s", err)
		return
	}

	// Append events to buffer
	for _, event := range events {
		var eventParsed Event
		err := json.Unmarshal([]byte(event.Event), &eventParsed)
		if err != nil {
			s.l.Errorf("Failed to unmarshal event: %s", err)
			continue
		}
		s.buffer = append(s.buffer, &eventParsed)
	}

	// Flush buffered events if any
	if len(s.buffer) > 0 {
		if s.timer != nil {
			s.timer.Stop()
		}
		s.timer = time.AfterFunc(s.debounceDelay, s.flush)
	}
}

// setOffline marks the subscriber as offline.
func (s *subscriber) setOffline() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.online = false
	s.offlineSince = time.Now()

	// Stop the timer, events will be kept in buffer
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	// flush the buffer
	s.flushLocked(context.Background())
}

// close permanently closes the subscriber.
func (s *subscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	// Delete the FsEvent
	s.fsEventClient.DeleteBySubscriber(context.Background(), uuid.FromStringOrNil(s.id))

	// Signal close and close the channel
	close(s.closedCh)
	close(s.ch)
	s.buffer = nil
}

// isClosed returns whether the subscriber is closed.
func (s *subscriber) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// shouldExpire returns whether the subscriber should be expired (offline for too long).
func (s *subscriber) shouldExpire() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.online && !s.offlineSince.IsZero() && time.Since(s.offlineSince) > s.offlineMaxAge
}

// Buffer returns a copy of the current buffered events.
// Useful for debugging or implementing custom merging logic.
func (s *subscriber) Buffer() []*Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.buffer) == 0 {
		return nil
	}

	// Return a copy to avoid data races
	buf := make([]*Event, len(s.buffer))
	copy(buf, s.buffer)
	return buf
}
