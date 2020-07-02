package user

import (
	"bytes"
	"sync"
)

type SnapshotCallback func(snapshot int)

type TextMessageCallback func(message *bytes.Buffer)

type RequestID string

type User struct {
	Name string

	mu sync.Mutex

	snapshotCallbacks []SnapshotCallback

	messageCallbacks []TextMessageCallback

	messageByRequestIDCallbacks map[RequestID][]TextMessageCallback

	patchCallbacks []func(a1 int, b1 int)
}

func (a *User) String() string {
	return a.Name
}
