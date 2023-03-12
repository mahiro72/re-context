package recontext

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

type Context interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}

// contextがキャンセルされた時のerrメッセージ
var Canceled = errors.New("context canceled")

// emptyCtx はcancelされず、値、deadlineを持たない空のContextです
type emptyCtx int

func (*emptyCtx) Deadline() (deadline time.Time, ok bool) {
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
	todo       = new(emptyCtx)
)

func Background() Context {
	return background
}

func TODO() Context {
	return todo
}

type CancelFunc func()
type CancelCauseFunc func(cause error)

func newCancelCtx(parent Context) *cancelCtx {
	return &cancelCtx{Context: parent}
}

// WithCancel は親contextを受け取り,
// parentを埋め込みかつcancel関数を用意したcontextを返します
func WithCancel(parent Context) (ctx Context, cancel CancelFunc) {
	c := withCancel(parent)
	return c, func() { c.cancel(true, Canceled, nil) }
}

func withCancel(parent Context) *cancelCtx {
	if parent == nil {
		panic("cannnot create context from nil parent")
	}
	c := newCancelCtx(parent)

	// parentがキャンセルされたことを子に伝播するため、
	// キャンセルの受信を待機している
	propagateCancel(parent, c)

	return c
}

var cancelCtxKey int

// Cause は &cancelCtxKeyを用いて &cancelCtxを取得し
// 取得したcancelCtxのcauseを返します
func Cause(c Context) error {
	if cc, ok := c.Value(&cancelCtxKey).(*cancelCtx); ok {
		cc.mu.Lock()
		defer cc.mu.Unlock()
		return cc.cause
	}
	return nil
}

// propagateCancel は親がcancelされたときにそれを子に伝搬します
// selectを用いて常にキャンセルされるかどうかを確認しています
func propagateCancel(parent Context, child canceler) {
	done := parent.Done()
	if done == nil {
		return //parentはキャンセルされない
	}
	
	select {
	// キャンセル待機
	case <-done:
		child.cancel(false, parent.Err(), Cause(parent))
		return
	default:
	}
}

// 親がキャンセルされてない、かつcancelCtxだった場合、
// そのcontextを取得する
func parentCancelCtx(parent Context) (*cancelCtx, bool) {
	done := parent.Done()
	// doneがすでにキャンセルされたか、nilだった場合終了する
	if done == closedchan || done == nil {
		return nil, false
	}

	// &cancelCtxKeyをキーにcancelCtxを取得する
	// もし取得できなかった場合終了する
	p, ok := parent.Value(&cancelCtxKey).(*cancelCtx)
	if !ok {
		return nil, false
	}

	// ???
	pdone, _ := p.done.Load().(chan struct{})
	if pdone != done {
		return nil, false
	}

	return p, true
}

// removeChild はparentからchildを削除します
func removeChild(parent Context,child canceler) {
	// 親がcancelCtxだった場合、親のchildrenから自身を削除する必要がある
	p, ok := parentCancelCtx(parent)
	if !ok {
		return
	}

	p.mu.Lock()
	if p.children != nil {
		// 親のchildrenから子を削除する
		delete(p.children,child)
	}

	p.mu.Unlock()
}

type canceler interface {
	cancel(removeFromParent bool, err, cause error)
	Done() <-chan struct{}
}

// closedchan は再利用可能なすでにcancelされたチャネル
var closedchan = make(chan struct{})

func init() {
	close(closedchan)
}

type cancelCtx struct {
	Context

	mu       sync.Mutex
	done     atomic.Value

	//おそらくchildを削除する時のコスパが良いからmapが使われている?
	// 配列だとループして取得する面倒くささや配列の再定義が必要な可能性あり
	children map[canceler]struct{}
	err      error
	cause    error
}

func (c *cancelCtx) Value(key any) any {
	if key == &cancelCtxKey {
		return c
	}
	return value(c.Context,key)
}

func (c *cancelCtx) Done() <-chan struct{} {
	d := c.done.Load()
	if d != nil {
		return d.(chan struct{})
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// ???
	// なぜもう一度
	d = c.done.Load()

	if d == nil {
		d = make(chan struct{})
		c.done.Store(d)
	}
	return d.(chan struct{})
}

// Err は自身のerrを取得し、そのエラーを第三者が安全に利用できる形で返す
func (c *cancelCtx) Err() error {
	c.mu.Lock()
	err := c.err
	c.mu.Unlock()
	return err
}

type stringer interface {
	String() string
}

func contextName(c Context) string {
	// stringer interfaceを満たすか確認
	if s,ok := c.(stringer); ok{
		return s.String()
	}
	// ???
	// もし自前のstringer interfaceを満たさなかった場合、
	// reflectのType型がもつStringレシーバを利用して返す
	// FIXME: もともとはreflectlite,internalパッケージのため一旦reflectで代用
	return reflect.TypeOf(c).String()
}


// cancel はdoneのcloseやcの子のをcancelを行います
// またremoveFromParentがtrueの時は、cの親の子から削除します
// さらにcancelはcが最初にcancelされたcontextだった場合、causeにその原因を設定します
func (c *cancelCtx) cancel(removeFromParent bool, err, cause error) {
	if err == nil {
		panic("context: internal error: missing cancel error")
	}
	// causeがnilだった場合、causeにerrを設定し、cancelされた最初の原因を設定します
	if cause == nil {
		cause = err
	}
	c.mu.Lock()
	
	// contextのもつerrがすでに設定されていた場合、
	// そのcontextはすでにcancelされているため終了する
	if c.err != nil {
		c.mu.Unlock()
		return
	}

	c.err = err
	c.cause = cause

	// ???
	// 何してるかわかってない
	d, _ := c.done.Load().(chan struct{})
	if d == nil {
		c.done.Store(closedchan)
	} else {
		close(d)
	}

	// cancelが伝搬される順番は先に親、その後に子
	for child := range c.children {
		// 親のロックを保持したまま子を取得しロックをかけながらcancelする
		// この処理の終了後 親のchildrenはnilに設定されるのでremoveFromParentはfalseでok
		child.cancel(false, err, cause)
	}

	c.children = nil
	c.mu.Unlock()

	if removeFromParent {
		//
		removeChild(c.Context, c)
	}
}

// WithValue はparentとなるContextを埋め込んだvalueCtxを返す
func WithValue(parent Context,key ,val any) Context {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	if key == nil {
		panic("key is nil")
	}
	// keyが比較可能な型か確認する
	if !reflect.TypeOf(key).Comparable() {
		panic("key is not comparable")
	}

	return &valueCtx{parent,key ,val}
}

// valueCtx はkeyとvalueの伝搬の役割をもつContextです
type valueCtx struct {
	Context
	key, val any
}

// stringifyはfmtパッケージを使わずに引数vの文字列化をします
func stringify(v any) string {
	switch s := v.(type) {
	case stringer:
		return s.String()
	case string:
		return s
	}
	return "<not Stringer>"
}

func (c *valueCtx) String() string {
	return contextName(c.Context) + ".WithValue(type " +
		reflect.TypeOf(c.key).String() +
		", val " + stringify(c.val)+")"
}

func (c *valueCtx) Value(key any) any {
	if c.key == key {
		return c.val
	}
	return value(c.Context,key)
}

func value(c Context,key any) any {
	for {
		switch ctx := c.(type) {
		case *valueCtx:
			if key == ctx.key {
				return ctx.val
			}
			c = ctx.Context
		case *cancelCtx :
			if key == &cancelCtxKey {
				return c
			}
			c = ctx.Context
		case *emptyCtx:
			// 全てのcontextの親はemptyCtxである。
			// 逆にここまでたどり着いてしまった場合,
			// keyに対するvalueが見つからなかったということなので
			// nilを返却する
			return nil
		default:
			// ???
			// 書いてある意味はわかるが、defaultに用意する必要があるのかわからない
			// 結局各caseにかいてあることと、Valueメソッドの実装内容はほとんど同じなので
			// Note: もしかしたら上記が違う実装箇所があるのかもしれない
			return c.Value(key)
		}
	}
}
