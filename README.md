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
