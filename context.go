package recontext

import (
	"time"
)

type Context interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any)any
}

// cancelされず、値、deadlineを持たない空のContext
type emptyCtx int

func (*emptyCtx) Deadline() (deadline time.Time,ok bool) {
	return
}

func (*emptyCtx) Done() <-chan struct{} {
	return nil
}

func (*emptyCtx) Err() error {
	return nil
}

func (*emptyCtx) Value(key any) any {
	return nil
}

func (e *emptyCtx) String() string {
	switch e {
	case background:
		return "context.Background"
	case todo:
		return "context.TODO"
	}
	return "unknown empty Context"
}

var (
	background = new(emptyCtx)
	todo = new(emptyCtx)
)

func Background() Context {
	return background
}

func TODO() Context {
	return todo
}

