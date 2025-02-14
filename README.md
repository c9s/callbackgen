# callbackgen

callbackgen generates callback pattern for your callback fields.

## Installation

```
go install github.com/c9s/callbackgen@latest
```

## Usage

`callbackgen` scans all the fields of the target struct, and generate methods for the fields that end with `Callbacks`. In the following example,
methods of `snapshotCallbacks` and `messageCallbacks` will be generated in `stream_callbacks.go`.

```
//go:generate callbackgen -type Stream
type Stream struct {
	Name string
  
	snapshotCallbacks []func(ctx context.Context)

	messageCallbacks []TextMessageCallback
}
```

Run `go generate <target path>` and `stream_callbacks.go` will be created with the following context:

```go
import (
	"context"
)

func (S *Stream) OnSnapshot(cb func(ctx context.Context)) {
	S.snapshotCallbacks = append(S.snapshotCallbacks, cb)
}

func (S *Stream) EmitSnapshot(ctx context.Context) {
	for _, cb := range S.snapshotCallbacks {
		cb(ctx)
	}
}

func (S *Stream) OnMessage(cb TextMessageCallback) {
	S.messageCallbacks = append(S.messageCallbacks, cb)
}

func (S *Stream) EmitMessage() {
	for _, cb := range S.messageCallbacks {
		cb()
	}
}
```

You could register the callback using `On*` methods, and trigger callbacks using `Emit*` methods.


# See Also

- requestgen <https://github.com/c9s/requestgen>

# License

MIT License
