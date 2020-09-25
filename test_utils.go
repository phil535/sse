package sse

import "fmt"

// retryEvent is used to represent a connection retry event
type retryEvent struct {
	delayInMs int
}

func newMessageEvent(lastEventID, name string, dataSize int) *MessageEvent {
	data := make([]byte, dataSize)
	for i := range data {
		data[i] = 'e'
	}
	return &MessageEvent{LastEventID: lastEventID, Name: name, Data: string(data)}
}

func newRetryEvent(delayInMs int) *retryEvent {
	return &retryEvent{delayInMs}
}

func messageEventToString(ev *MessageEvent) string {
	msg := ""
	if ev.LastEventID != "" {
		msg = buildString("id: ", ev.LastEventID, "\n")
	}
	if ev.Name != "" {
		msg = buildString(msg, "event: ", ev.Name, "\n")
	}
	return buildString(msg, "data: ", ev.Data, "\n\n")
}

func retryEventToString(ev *retryEvent) string {
	return buildString("retry: ", fmt.Sprintf("%d", ev.delayInMs), "\n")
}

func buildString(fields ...string) string {
	data := []byte{}
	for _, field := range fields {
		data = append(data, field...)
	}
	return string(data)
}
