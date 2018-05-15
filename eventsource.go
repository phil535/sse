package sse

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	AllowedContentType = "text/event-stream"
)

var (
	ErrContentType = errors.New("eventsource: the content type of the stream is not allowed")
)

type (
	// EventSource connects and processes events from an SSE stream.
	EventSource struct {
		url         string
		lastEventID string
		d           *Decoder
		resp        *http.Response
		out         chan *MessageEvent

		// Status of the event stream.
		readyState    ReadyState
		readyStateMux sync.RWMutex
	}
)

// NewEventSource constructs returns an EventSource that satisfies the HTML5 EventSource specification.
func NewEventSource(url string) (*EventSource, error) {
	es := &EventSource{
		d:   nil,
		url: url,
		out: make(chan *MessageEvent),
	}
	return es, es.connect()
}

// connect does a connection attempt, if the operation fails, attempt reconnecting
// according to the spec.
func (es *EventSource) connect() (err error) {
	es.setReadyState(Connecting)
	err = es.connectOnce()
	if err != nil {
		es.Close()
	}
	return
}

// reconnect to the stream several until the operation succeeds or the conditions
// to retry no longer hold true.
func (es *EventSource) reconnect() (err error) {
	es.setReadyState(Connecting)
	for es.mustReconnect(err) {
		time.Sleep(time.Duration(es.d.Retry()) * time.Millisecond)
		err = es.connectOnce()
	}
	if err != nil {
		es.Close()
	}
	return
}

// Attempts to connect and updates internal status depending on the outcome.
func (es *EventSource) connectOnce() (err error) {
	es.resp, err = es.doHTTPConnect()
	if err != nil {
		return
	}
	es.setReadyState(Open)
	es.d = NewDecoder(es.resp.Body)
	go es.consume()
	return
}

func (es *EventSource) doHTTPConnect() (*http.Response, error) {
	// Prepare request
	req, err := http.NewRequest("GET", es.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", AllowedContentType)
	req.Header.Set("Cache-Control", "no-store")
	if es.lastEventID != "" {
		req.Header.Set("Last-Event-ID", es.lastEventID)
	}

	// Check response
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return resp, err
	}
	if resp.Header.Get("Content-Type") != AllowedContentType {
		return resp, ErrContentType
	}
	return resp, nil
}

// Method consume() must be called once connect() succeeds.
// It parses the input reader and assigns the event output channel accordingly.
func (es *EventSource) consume() {
	for {
		ev, err := es.d.Decode()
		if err != nil {
			if es.mustReconnect(err) {
				err = es.reconnect()
			}
			es.Close()
			return
		}
		es.lastEventID = ev.LastEventID
		es.out <- ev
	}
}

// Clients will reconnect if the connection is closed;
// a client can be told to stop reconnecting using the HTTP 204 No Content response code.
func (es *EventSource) mustReconnect(err error) bool {
	switch err {
	case ErrContentType:
		return false
	case io.ErrUnexpectedEOF:
		return true
	}
	if es.resp != nil && es.resp.StatusCode == http.StatusNoContent {
		return false
	}
	return true
}

// Returns the event source URL.
func (es *EventSource) URL() string {
	return es.url
}

// Returns the event source connection state, either connecting, open or closed.
func (es *EventSource) ReadyState() ReadyState {
	es.readyStateMux.RLock()
	defer es.readyStateMux.RUnlock()
	return es.readyState
}

func (es *EventSource) setReadyState(newState ReadyState) {
	es.readyStateMux.Lock()
	defer es.readyStateMux.Unlock()

	// Once the EventSource is closed, its ready state cannot change anymore.
	if es.readyState == Closed {
		return
	}
	es.readyState = newState
}

// Returns the channel of events. MessageEvents will be queued in the channel as they
// are received.
func (es *EventSource) MessageEvents() <-chan *MessageEvent {
	return es.out
}

// Closes the event source.
// After closing the event source, it cannot be reused again.
func (es *EventSource) Close() {
	if es.acquireClosingRight() {
		if es.resp != nil {
			es.resp.Body.Close()
		}
		close(es.out)
		es.setReadyState(Closed)
	}
}

// Acquires closing right by setting readyState to Closing if no one else
// is attempting to close the EventSource.
func (es *EventSource) acquireClosingRight() bool {
	es.readyStateMux.Lock()
	defer es.readyStateMux.Unlock()
	if es.readyState == Closed || es.readyState == Closing {
		return false
	}
	es.readyState = Closing
	return true
}
