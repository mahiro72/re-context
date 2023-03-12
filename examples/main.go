package main

import (
	"context"
	"fmt"
	"time"

)

type emptyCtx int
func (*emptyCtx) Deadline() (deadline time.Time, ok bool) {return}
func (*emptyCtx) Done() <-chan struct{} {return nil}
func (*emptyCtx) Err() error {return nil}
func (*emptyCtx) Value(key any) any {return nil}

var (
	background = new(emptyCtx)
)

func Background() context.Context {
	return background
}

func main() {
	fmt.Printf("%p, %p\n",context.Background(),context.Background())

	x := 1
	fmt.Printf("%p\n",&x)
}
