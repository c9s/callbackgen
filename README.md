# callbackgen

callbackgen generates callback pattern for your callback fields.

```
//go:generate callbackgen -type Stream
type Stream struct {
	Name string
  
	snapshotCallbacks []SnapshotCallback

	messageCallbacks []TextMessageCallback
}
```

then you will have the following methods:

- OnSnapshot(cb SnapshotCallback)
- RemoveOnSnapshot(cb SnapshotCallback)
- EmitSnapshot(snapshot Snapshot)

- OnMessage(cb TextMessageCallback)
- RemoveOnMessage(cb TextMessageCallback)
- EmitMessage(message TextMessage)
