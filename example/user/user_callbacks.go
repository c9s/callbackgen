// Code generated by "callbackgen -type User ./example/user"; DO NOT EDIT.

package user

import (
	"bytes"
	"reflect"
)

func (a *User) OnSnapshot(cb SnapshotCallback) {
	a.snapshotCallbacks = append(a.snapshotCallbacks, cb)
}

func (a *User) EmitSnapshot(snapshot int) {
	for _, cb := range a.snapshotCallbacks {
		cb(snapshot)
	}
}

func (a *User) RemoveOnSnapshot(
	needle SnapshotCallback) (found bool) {

	var newcallbacks []SnapshotCallback
	var fp = reflect.ValueOf(needle).Pointer()
	for _, cb := range a.snapshotCallbacks {
		if fp == reflect.ValueOf(cb).Pointer() {
			found = true
		} else {
			newcallbacks = append(newcallbacks, cb)
		}
	}

	if found {
		a.snapshotCallbacks = newcallbacks
	}

	return found
}

func (a *User) OnMessage(cb TextMessageCallback) {
	a.messageCallbacks = append(a.messageCallbacks, cb)
}

func (a *User) EmitMessage(message *bytes.Buffer) {
	for _, cb := range a.messageCallbacks {
		cb(message)
	}
}

func (a *User) RemoveOnMessage(
	needle TextMessageCallback) (found bool) {

	var newcallbacks []TextMessageCallback
	var fp = reflect.ValueOf(needle).Pointer()
	for _, cb := range a.messageCallbacks {
		if fp == reflect.ValueOf(cb).Pointer() {
			found = true
		} else {
			newcallbacks = append(newcallbacks, cb)
		}
	}

	if found {
		a.messageCallbacks = newcallbacks
	}

	return found
}

func (a *User) OnMessageByRequestID(requestID RequestID, cb TextMessageCallback) {
	a.messageByRequestIDCallbacks[requestID] = append(a.messageByRequestIDCallbacks[requestID], cb)
}

func (a *User) EmitMessageByRequestID(requestID RequestID, message *bytes.Buffer) {
	callbacks, ok := a.messageByRequestIDCallbacks[requestID]
	if !ok {
		return
	}

	for _, cb := range callbacks {
		cb(message)
	}
}

func (a *User) RemoveOnMessageByRequestID(requestID RequestID, needle TextMessageCallback) (found bool) {

	callbacks, ok := a.messageByRequestIDCallbacks[requestID]
	if !ok {
		return
	}

	var newcallbacks []TextMessageCallback
	var fp = reflect.ValueOf(needle).Pointer()
	for _, cb := range callbacks {
		if fp == reflect.ValueOf(cb).Pointer() {
			found = true
		} else {
			newcallbacks = append(newcallbacks, cb)
		}
	}

	if found {
		a.messageByRequestIDCallbacks[requestID] = newcallbacks
	}

	return found
}

func (a *User) OnPatch(cb func(a1 int, b1 int)) {
	a.patchCallbacks = append(a.patchCallbacks, cb)
}

func (a *User) EmitPatch(a1 int, b1 int) {
	for _, cb := range a.patchCallbacks {
		cb(a1, b1)
	}
}

func (a *User) RemoveOnPatch(
	needle func(a1 int, b1 int)) (found bool) {

	var newcallbacks []func(a1 int, b1 int)
	var fp = reflect.ValueOf(needle).Pointer()
	for _, cb := range a.patchCallbacks {
		if fp == reflect.ValueOf(cb).Pointer() {
			found = true
		} else {
			newcallbacks = append(newcallbacks, cb)
		}
	}

	if found {
		a.patchCallbacks = newcallbacks
	}

	return found
}
