// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/lesismal/arpc/codec"
	"github.com/lesismal/arpc/log"
	"github.com/lesismal/arpc/util"
)

// DefaultHandler is the Handler cloned by NewServer and NewClient when none
// is given, and configured by the package-level functions.
var DefaultHandler Handler = NewHandler()

// HandlerFunc handles a request or notify, as a method handler or middleware.
type HandlerFunc func(*Context)

// StreamHandlerFunc handles a Stream opened by the peer.
type StreamHandlerFunc func(*Stream)

// AsyncHandlerFunc is the callback of Client.CallAsync. On success, ctx holds
// the response and err is the remote error, if any; on timeout or disconnect,
// ctx is nil and err is ErrTimeout or ErrClientReconnecting.
type AsyncHandlerFunc func(*Context, error)

// asyncHandler is a pending CallAsync: its callback and timeout timer.
type asyncHandler struct {
	timer   *time.Timer
	handler AsyncHandlerFunc
}

var (
	emptyAsyncHandler = asyncHandler{}
	asyncHandlerPool  = sync.Pool{
		New: func() interface{} {
			return &asyncHandler{}
		},
	}
)

func getAsyncHandler(t *time.Timer, h AsyncHandlerFunc) *asyncHandler {
	ah := asyncHandlerPool.Get().(*asyncHandler)
	ah.timer = t
	ah.handler = h
	return ah
}

func putAsyncHandler(ah *asyncHandler) {
	*ah = emptyAsyncHandler
	asyncHandlerPool.Put(ah)
}

// routerHandler is the handler chain of a method: the middlewares and the
// method handler in registration order.
type routerHandler struct {
	async    bool
	handlers []HandlerFunc
}

// streamHandler is the handler of a stream method.
type streamHandler struct {
	async   bool
	handler StreamHandlerFunc
}

// Handler holds the configuration, callbacks and routes shared by Clients,
// and implements how messages are read, written and dispatched.
//
// A Handler is not safe for concurrent modification: configure it before it
// is used by any Client or Server.
type Handler interface {
	// Clone returns a copy of the Handler with its own middlewares, coders,
	// routes and context. Callbacks are shared.
	Clone() Handler

	// LogTag returns the prefix of the Handler's log lines.
	LogTag() string
	// SetLogTag sets the prefix of the Handler's log lines.
	SetLogTag(tag string)

	// HandleConnected registers the callback called when a connection is
	// established: after accept for a server-role Client, and after the first
	// connect and each successful reconnect for a client-role Client. It
	// replaces the default callback, which disables TCP_NODELAY.
	HandleConnected(onConnected func(*Client))
	// OnConnected calls the connected callback.
	OnConnected(c *Client)

	// HandleDisconnected registers the callback called when a Client stops:
	// when the conn of a server-role Client breaks, or when a client-role
	// Client gives up reconnecting or is stopped. It is not called for each
	// disconnection that is followed by a reconnect.
	HandleDisconnected(onDisConnected func(*Client))
	// OnDisconnected calls the disconnected callback.
	OnDisconnected(c *Client)

	// HandleReconnect registers the callback called after every reconnect
	// attempt of a client-role Client, whether it succeeds or not. It runs on
	// the Client's read goroutine; on success, before OnConnected.
	HandleReconnect(onReconnect func(c *Client, info *ReconnectInfo))
	// OnReconnect calls the reconnect callback.
	OnReconnect(c *Client, info *ReconnectInfo)

	// MaxReconnectTimes returns the max reconnect attempts after a client-role
	// Client loses its conn; <= 0 means unlimited.
	MaxReconnectTimes() int
	// SetMaxReconnectTimes sets the max reconnect attempts; <= 0 (default)
	// means unlimited. Attempts are 1 second apart.
	SetMaxReconnectTimes(n int)

	// HandleOverstock registers the callback called when a message cannot be
	// pushed because the send queue is full. OnMessageDone is called on the
	// message after it.
	HandleOverstock(onOverstock func(c *Client, m *Message))
	// OnOverstock calls the overstock callback. Without one registered, it
	// does nothing, not even OnMessageDone.
	OnOverstock(c *Client, m *Message)

	// HandleMessageDone registers the callback called when arpc is done with a
	// message: sent, dropped, or received and consumed. EnablePool(true) sets
	// it to release the message.
	HandleMessageDone(onMessageDone func(c *Client, m *Message))
	// OnMessageDone calls the message done callback if m is not nil.
	OnMessageDone(c *Client, m *Message)

	// HandleMessageDropped registers the callback called when a message is
	// dropped instead of being sent, e.g. while reconnecting or after the
	// Client stops. OnMessageDone is called on the message after it.
	HandleMessageDropped(onMessageDropped func(c *Client, m *Message))
	// OnMessageDropped calls the message dropped callback. Without one
	// registered, it does nothing, not even OnMessageDone.
	OnMessageDropped(c *Client, m *Message)

	// HandleSessionMiss registers the callback called when a response arrives
	// but its call is gone, e.g. timed out. OnMessageDone is called on the
	// message after it.
	HandleSessionMiss(onSessionMiss func(c *Client, m *Message))
	// OnSessionMiss calls the session miss callback. Without one registered,
	// it does nothing, not even OnMessageDone.
	OnSessionMiss(c *Client, m *Message)

	// HandleContextDone registers the callback called after the handler chain
	// of a request or notify returns, or after a CallAsync handler returns.
	// EnablePool(true) sets it to release the Context.
	HandleContextDone(onContextDone func(ctx *Context))
	// OnContextDone calls the context done callback.
	OnContextDone(ctx *Context)

	// BeforeRecv registers a hook called on the conn before reading each
	// message. A non-nil error fails the read and breaks the conn.
	BeforeRecv(h func(net.Conn) error)
	// BeforeSend registers a hook called on the conn before each write. A
	// non-nil error fails the write.
	BeforeSend(h func(net.Conn) error)

	// BatchRecv reports whether reads go through WrapReader, a buffered
	// reader by default. Default true.
	BatchRecv() bool
	// SetBatchRecv sets whether reads go through WrapReader.
	SetBatchRecv(batch bool)
	// BatchSend reports whether the send loop merges queued messages into one
	// write, up to SendBufferSize bytes. Default true, but it has no effect
	// while SendBufferSize is 0. Only used with AsyncWrite.
	BatchSend() bool
	// SetBatchSend sets whether the send loop merges queued messages.
	SetBatchSend(batch bool)

	// AsyncWrite reports whether messages are queued and written by a send
	// goroutine per Client, instead of written on the calling goroutine.
	// Default true.
	AsyncWrite() bool
	// SetAsyncWrite sets whether messages are written asynchronously.
	SetAsyncWrite(async bool)

	// AsyncWritev reports whether messages are queued and written with writev
	// (net.Buffers) by an on-demand writer goroutine, one at most per Client.
	// It takes precedence over AsyncWrite, and pushing never blocks since the
	// queue is unbounded. Default false.
	AsyncWritev() bool
	// SetAsyncWritev sets whether the writev path is used.
	SetAsyncWritev(async bool)

	// AsyncResponse reports whether handlers registered by Handle and
	// HandleStream run via AsyncExecute by default, instead of on the Client's
	// read goroutine. Default true.
	AsyncResponse() bool
	// SetAsyncResponse sets the default of AsyncResponse for handlers
	// registered after it.
	SetAsyncResponse(async bool)

	// WrapReader returns the reader used to read from conn when BatchRecv is
	// enabled. By default it is a bufio.Reader of RecvBufferSize.
	WrapReader(conn net.Conn) io.Reader
	// SetReaderWrapper replaces the WrapReader function.
	SetReaderWrapper(wrapper func(conn net.Conn) io.Reader)

	// Recv reads a message from the Client's Reader, applying BeforeRecv and
	// ReadTimeout.
	Recv(c *Client) (*Message, error)
	// Send writes buffer to conn, applying BeforeSend and WriteTimeout.
	Send(c net.Conn, buffer []byte) (int, error)
	// SendN writes buffers to conn in one writev, applying BeforeSend and
	// WriteTimeout.
	SendN(conn net.Conn, buffers net.Buffers) (int, error)

	// RecvBufferSize returns the buffer size of the default WrapReader.
	RecvBufferSize() int
	// SetRecvBufferSize sets the buffer size of the default WrapReader.
	// Default 8192.
	SetRecvBufferSize(size int)

	// SendBufferSize returns the max bytes BatchSend merges into one write.
	SendBufferSize() int
	// SetSendBufferSize sets the max bytes BatchSend merges into one write.
	// Default 0, which disables merging.
	SetSendBufferSize(size int)

	// ReadTimeout returns the read deadline set before reading each message;
	// 0 means none.
	ReadTimeout() time.Duration
	// SetReadTimeout sets the read deadline set before reading each message.
	SetReadTimeout(timeout time.Duration)

	// WriteTimeout returns the write deadline set before each write; 0 means
	// none.
	WriteTimeout() time.Duration
	// SetWriteTimeout sets the write deadline set before each write.
	SetWriteTimeout(timeout time.Duration)

	// SendQueueSize returns the capacity of each Client's send queue used by
	// AsyncWrite.
	SendQueueSize() int
	// SetSendQueueSize sets the capacity of the send queue of Clients created
	// or restarted after it. Default 4096.
	SetSendQueueSize(size int)

	// StreamQueueSize returns the capacity of each Stream's receive queue.
	StreamQueueSize() int
	// SetStreamQueueSize sets the capacity of each Stream's receive queue.
	// Default 4.
	SetStreamQueueSize(size int)

	// MaxBodyLen returns the max body length of a received message.
	MaxBodyLen() int
	// SetMaxBodyLen sets the max body length of a received message; a longer
	// one breaks the conn. Default DefaultMaxBodyLen.
	SetMaxBodyLen(l int)

	// Use appends a middleware to the handler chain of all methods, including
	// those already registered; for them it runs after their handler. The
	// chain continues after the middleware returns, unless it calls
	// Context.Abort. Stream handlers are not affected.
	Use(h HandlerFunc)

	// UseCoder appends a MessageCoder. Encode is called in registration order
	// before a message is sent, Decode in reverse order after one is received.
	UseCoder(coder MessageCoder)

	// Coders returns the registered MessageCoders.
	Coders() []MessageCoder

	// Handle registers the handler of a method, after the middlewares
	// registered so far. It panics if method is empty (use HandleNotFound),
	// too long, or already registered.
	//
	// An optional bool arg sets whether h runs via AsyncExecute (true) or on
	// the Client's read goroutine (false), overriding AsyncResponse.
	Handle(m string, h HandlerFunc, args ...interface{})

	// Singleflight enables singleflight de-duplication of Client.Call,
	// CallContext and CallAsync for method: concurrent calls with the same key
	// send only one request and share its response.
	//
	// keyFunc is optional and computes the key from the call's req. If it is
	// omitted or nil, the key is req.String() when req implements
	// fmt.Stringer, otherwise fmt.Sprintf("%v", req).
	Singleflight(method string, keyFunc ...func(req interface{}) string)

	// SingleflightKey reports whether method has singleflight enabled and, if
	// so, returns the key computed from req.
	SingleflightKey(method string, req interface{}) (string, bool)

	// Register registers the eligible methods of h as handlers, with route
	// names in the "Service.Method" form: m + "." + the method name, or just
	// the method name when m is empty.
	//
	// Eligible methods are exported and are either:
	//   - A method with the signature
	//       func(ctx context.Context, req *Request, rsp *Response)
	//     where Request and Response are structs. If h also has a method named
	//     after it plus "Binding" of HandlerFunc type, that method is
	//     registered for the route, and is expected to create req/rsp and call
	//     the first one. Otherwise an auto-generated handler is registered: it
	//     creates req/rsp, binds the request into req, calls the method with
	//     the Context, and writes rsp as the response.
	//   - A method of HandlerFunc type (func(*arpc.Context)) that is not the
	//     "Binding" method of the above.
	//
	// It panics if no method is eligible, or on any error of Handle; the
	// returned error is only non-nil for a nil h.
	Register(m string, h interface{}) error

	// HandleNotFound registers the handler for methods that have no handler.
	// By default, such requests get an ErrMethodNotFound error response.
	HandleNotFound(h HandlerFunc)

	// HandleStream registers the handler of a stream method. It is called
	// when the peer sends the first message of a new Stream for the method.
	// An optional bool arg overrides AsyncResponse, as for Handle.
	HandleStream(m string, h StreamHandlerFunc, args ...interface{})

	// OnMessage dispatches a received message: it answers pings, decodes the
	// message with the coders, then runs the handler chain for requests and
	// notifies, completes the matching call for responses, or feeds the
	// matching Stream for stream messages.
	OnMessage(c *Client, m *Message)

	// Malloc allocates a message buffer, with make by default.
	Malloc(size int) []byte
	// HandleMalloc replaces the Malloc function.
	HandleMalloc(f func(size int) []byte)

	// Append appends more to b, with the builtin append by default.
	Append(b []byte, more ...byte) []byte
	// HandleAppend replaces the Append function.
	HandleAppend(f func(b []byte, more ...byte) []byte)

	// Free recycles a buffer from Malloc; it does nothing by default.
	Free([]byte)
	// HandleFree replaces the Free function.
	HandleFree(f func(buf []byte))

	// EnablePool(true) allocates buffers from DefaultAllocator, and releases
	// Contexts and Messages when done (via HandleContextDone and
	// HandleMessageDone), so they must not be used after their handler
	// returns; use Message.Retain to keep a Message longer. EnablePool(false)
	// restores plain allocation and replaces those callbacks with no-ops.
	EnablePool(enable bool)

	// Context returns the Handler's context and its cancel func. It is not
	// used by arpc itself; Clone creates a new one.
	Context() (context.Context, context.CancelFunc)
	// SetContext replaces the Handler's context and cancel func.
	SetContext(ctx context.Context, cancel context.CancelFunc)
	// Cancel cancels the Handler's context.
	Cancel()

	// NewMessage creates a Message with this Handler, like the package-level
	// NewMessage: isError and isAsync are ignored.
	NewMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, seq uint64, codec codec.Codec, values map[interface{}]interface{}) *Message

	// NewMessageWithBuffer wraps buffer, a complete encoded message, in a
	// pooled Message. With EnablePool(true), buffer should come from Malloc
	// since it is freed when the Message is released.
	NewMessageWithBuffer(buffer []byte) *Message

	// SetAsyncExecutor sets the function AsyncExecute runs tasks with, e.g. a
	// goroutine pool.
	SetAsyncExecutor(executor func(f func()))
	// AsyncExecute runs f with the executor, or in a new goroutine with panic
	// recovery if none is set.
	AsyncExecute(f func())
}

// handler is the default Handler implementation.
type handler struct {
	logtag            string
	batchRecv         bool
	batchSend         bool
	asyncWrite        bool
	asyncWritev       bool
	asyncResponse     bool
	recvBufferSize    int
	sendBufferSize    int
	readTimeout       time.Duration
	writeTimeout      time.Duration
	sendQueueSize     int
	streamQueueSize   int
	maxBodyLen        int
	maxReconnectTimes int

	onConnected      func(*Client)
	onDisConnected   func(*Client)
	onReconnect      func(c *Client, info *ReconnectInfo)
	onOverstock      func(c *Client, m *Message)
	onMessageDone    func(c *Client, m *Message)
	onMessageDropped func(c *Client, m *Message)
	onSessionMiss    func(c *Client, m *Message)
	onContextDone    func(ctx *Context)

	beforeRecv func(net.Conn) error
	beforeSend func(net.Conn) error
	malloc     func(int) []byte
	append     func([]byte, ...byte) []byte
	free       func([]byte)

	wrapReader func(conn net.Conn) io.Reader

	routes  map[string]*routerHandler
	streams map[string]*streamHandler

	// singleflights maps each method with singleflight enabled to the func
	// computing the key from req.
	singleflights map[string]func(req interface{}) string

	middles   []HandlerFunc
	msgCoders []MessageCoder

	ctx    context.Context
	cancel context.CancelFunc

	executor func(f func())
}

func (h *handler) Clone() Handler {
	cp := *h
	cp.middles = make([]HandlerFunc, len(h.middles))
	copy(cp.middles, h.middles)

	cp.msgCoders = make([]MessageCoder, len(h.msgCoders))
	copy(cp.msgCoders, h.msgCoders)

	cp.routes = map[string]*routerHandler{}
	for k, v := range h.routes {
		rh := &routerHandler{
			async:    v.async,
			handlers: make([]HandlerFunc, len(v.handlers)),
		}
		copy(rh.handlers, v.handlers)
		cp.routes[k] = rh
	}

	cp.streams = map[string]*streamHandler{}
	for k, v := range h.streams {
		sh := &streamHandler{
			async:   v.async,
			handler: v.handler,
		}
		cp.streams[k] = sh
	}

	if h.singleflights != nil {
		cp.singleflights = make(map[string]func(req interface{}) string, len(h.singleflights))
		for k, v := range h.singleflights {
			cp.singleflights[k] = v
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cp.ctx = ctx
	cp.cancel = cancel

	return &cp
}

func (h *handler) LogTag() string {
	return h.logtag
}

func (h *handler) SetLogTag(tag string) {
	h.logtag = tag
}

func (h *handler) HandleConnected(onConnected func(*Client)) {
	h.onConnected = onConnected
}

func (h *handler) OnConnected(c *Client) {
	if h.onConnected != nil {
		h.onConnected(c)
	}
}

func (h *handler) HandleDisconnected(onDisConnected func(*Client)) {
	h.onDisConnected = onDisConnected
}

func (h *handler) OnDisconnected(c *Client) {
	if h.onDisConnected != nil {
		h.onDisConnected(c)
	}
}

func (h *handler) HandleReconnect(onReconnect func(c *Client, info *ReconnectInfo)) {
	h.onReconnect = onReconnect
}

func (h *handler) OnReconnect(c *Client, info *ReconnectInfo) {
	if h.onReconnect != nil {
		h.onReconnect(c, info)
	}
}

func (h *handler) MaxReconnectTimes() int {
	return h.maxReconnectTimes
}

func (h *handler) SetMaxReconnectTimes(n int) {
	h.maxReconnectTimes = n
}

func (h *handler) HandleOverstock(onOverstock func(c *Client, m *Message)) {
	h.onOverstock = func(c *Client, m *Message) {
		if onOverstock != nil {
			onOverstock(c, m)
		}
		h.OnMessageDone(c, m)
	}
}

func (h *handler) OnOverstock(c *Client, m *Message) {
	if h.onOverstock != nil {
		h.onOverstock(c, m)
	}
}

func (h *handler) HandleMessageDropped(onMessageDropped func(c *Client, m *Message)) {
	h.onMessageDropped = func(c *Client, m *Message) {
		if onMessageDropped != nil {
			onMessageDropped(c, m)
		}
		h.OnMessageDone(c, m)
	}
}

func (h *handler) OnMessageDropped(c *Client, m *Message) {
	if h.onMessageDropped != nil {
		h.onMessageDropped(c, m)
	}
}

func (h *handler) HandleMessageDone(onMessageDone func(c *Client, m *Message)) {
	h.onMessageDone = onMessageDone
}

func (h *handler) OnMessageDone(c *Client, m *Message) {
	if h.onMessageDone != nil && m != nil {
		h.onMessageDone(c, m)
	}
}

func (h *handler) HandleSessionMiss(onSessionMiss func(c *Client, m *Message)) {
	h.onSessionMiss = onSessionMiss
}

func (h *handler) OnSessionMiss(c *Client, m *Message) {
	if h.onSessionMiss != nil {
		h.onSessionMiss(c, m)
		h.OnMessageDone(c, m)
	}
}

func (h *handler) HandleContextDone(onContextDone func(ctx *Context)) {
	h.onContextDone = onContextDone
}

func (h *handler) OnContextDone(ctx *Context) {
	if h.onContextDone != nil {
		h.onContextDone(ctx)
	}
}

func (h *handler) BeforeRecv(hb func(net.Conn) error) {
	h.beforeRecv = hb
}

func (h *handler) BeforeSend(hs func(net.Conn) error) {
	h.beforeSend = hs
}

func (h *handler) BatchRecv() bool {
	return h.batchRecv
}

func (h *handler) SetBatchRecv(batch bool) {
	h.batchRecv = batch
}

func (h *handler) BatchSend() bool {
	return h.batchSend
}

func (h *handler) SetBatchSend(batch bool) {
	h.batchSend = batch
}

func (h *handler) AsyncWrite() bool {
	return h.asyncWrite
}

func (h *handler) SetAsyncWrite(async bool) {
	h.asyncWrite = async
}

func (h *handler) AsyncWritev() bool {
	return h.asyncWritev
}

func (h *handler) SetAsyncWritev(async bool) {
	h.asyncWritev = async
}

func (h *handler) AsyncResponse() bool {
	return h.asyncResponse
}

func (h *handler) SetAsyncResponse(async bool) {
	h.asyncResponse = async
}

func (h *handler) WrapReader(conn net.Conn) io.Reader {
	if h.wrapReader != nil {
		return h.wrapReader(conn)
	}
	return conn
}

func (h *handler) SetReaderWrapper(wrapper func(conn net.Conn) io.Reader) {
	h.wrapReader = wrapper
}

func (h *handler) RecvBufferSize() int {
	return h.recvBufferSize
}

func (h *handler) SetRecvBufferSize(size int) {
	h.recvBufferSize = size
}

func (h *handler) SendBufferSize() int {
	return h.sendBufferSize
}

func (h *handler) SetSendBufferSize(size int) {
	h.sendBufferSize = size
}

func (h *handler) ReadTimeout() time.Duration {
	return h.readTimeout
}

func (h *handler) SetReadTimeout(timeout time.Duration) {
	h.readTimeout = timeout
}

func (h *handler) WriteTimeout() time.Duration {
	return h.writeTimeout
}

func (h *handler) SetWriteTimeout(timeout time.Duration) {
	h.writeTimeout = timeout
}

func (h *handler) SendQueueSize() int {
	return h.sendQueueSize
}

func (h *handler) SetSendQueueSize(size int) {
	h.sendQueueSize = size
}

func (h *handler) StreamQueueSize() int {
	return h.streamQueueSize
}

func (h *handler) SetStreamQueueSize(size int) {
	h.streamQueueSize = size
}

func (h *handler) MaxBodyLen() int {
	return h.maxBodyLen
}

func (h *handler) SetMaxBodyLen(l int) {
	h.maxBodyLen = l
}

func (h *handler) Use(cb HandlerFunc) {
	if cb == nil {
		return
	}
	cbWithNext := func(ctx *Context) {
		cb(ctx)
		ctx.Next()
	}
	h.middles = append(h.middles, cbWithNext)
	for k, v := range h.routes {
		rh := &routerHandler{
			async:    v.async,
			handlers: make([]HandlerFunc, len(v.handlers)+1),
		}
		copy(rh.handlers, v.handlers)
		rh.handlers[len(v.handlers)] = cbWithNext
		h.routes[k] = rh
	}
}

func (h *handler) UseCoder(coder MessageCoder) {
	if coder != nil {
		h.msgCoders = append(h.msgCoders, coder)
	}
}

func (h *handler) Coders() []MessageCoder {
	return h.msgCoders
}

func (h *handler) Handle(method string, cb HandlerFunc, args ...interface{}) {
	if method == "" {
		panic(fmt.Errorf("empty('') method is reserved for [method not found], should use HandleNotFound to register '' handler"))
	}
	h.handle(method, cb, args...)
}

var (
	typeContext     = reflect.TypeOf((*context.Context)(nil)).Elem()
	typeHandlerFunc = reflect.TypeOf(HandlerFunc(nil))
)

// bindingSuffix is the name suffix of the "Binding" method, see Register.
const bindingSuffix = "Binding"

func (h *handler) Register(m string, h2 interface{}) error {
	if h2 == nil {
		return fmt.Errorf("arpc: Register: nil handler")
	}

	hv := reflect.ValueOf(h2)
	ht := hv.Type()

	// route builds the "Service.Method" route name, or just the method name
	// when the service name m is empty.
	route := func(name string) string {
		if m == "" {
			return name
		}
		return m + "." + name
	}

	// First pass: find eligible first/Binding method pairs. The Binding method
	// of each pair is registered under the first method's name, and is recorded
	// in paired so that it is not also registered standalone in the second pass.
	paired := map[string]bool{}
	registered := 0
	for i := 0; i < ht.NumMethod(); i++ {
		method := ht.Method(i)

		// Only consider exported methods, and skip the "Binding" methods
		// themselves so that they are not treated as a first method.
		if method.PkgPath != "" || strings.HasSuffix(method.Name, bindingSuffix) {
			continue
		}

		// Check the first method's signature:
		//   func (receiver) Name(ctx context.Context, req *struct, rsp *struct)
		// mt.In(0) is the receiver, so there are 4 input params and no output.
		mt := method.Type
		if mt.NumIn() != 4 || mt.NumOut() != 0 {
			continue
		}
		if mt.In(1) != typeContext {
			continue
		}
		if !isStructPtr(mt.In(2)) || !isStructPtr(mt.In(3)) {
			continue
		}

		// Find the paired second method: Name + "Binding".
		bindingName := method.Name + bindingSuffix
		bindingVal := hv.MethodByName(bindingName)
		if bindingVal.IsValid() && bindingVal.Type().ConvertibleTo(typeHandlerFunc) {
			// Eligible pair: register the Binding method under the first
			// method's name, and record it so it is not registered standalone.
			cb := bindingVal.Convert(typeHandlerFunc).Interface().(HandlerFunc)
			h.Handle(route(method.Name), cb)
			paired[bindingName] = true
			registered++
			continue
		}

		// No valid Binding method: register an auto-generated handler, see
		// newStructHandler.
		h.Handle(route(method.Name), newStructHandler(hv.Method(i), mt.In(2), mt.In(3)))
		registered++
	}

	// Second pass: register the remaining arpc.HandlerFunc methods standalone,
	// under their own name. A HandlerFunc method that was already consumed as
	// the Binding of a pair in the first pass is skipped.
	for i := 0; i < ht.NumMethod(); i++ {
		method := ht.Method(i)
		if method.PkgPath != "" || paired[method.Name] {
			continue
		}

		// The bound method value must be convertible to HandlerFunc, i.e.
		// func(*arpc.Context).
		mv := hv.Method(i)
		if !mv.Type().ConvertibleTo(typeHandlerFunc) {
			continue
		}

		cb := mv.Convert(typeHandlerFunc).Interface().(HandlerFunc)
		h.Handle(route(method.Name), cb)
		registered++
	}

	if registered == 0 {
		panic(fmt.Errorf("arpc: Register: no eligible method found on %v", ht))
	}

	return nil
}

// defaultSingleflightKey is the key func used when Singleflight gets none:
// req.String() if req is a fmt.Stringer, otherwise fmt.Sprintf("%v", req).
func defaultSingleflightKey(req interface{}) string {
	if s, ok := req.(fmt.Stringer); ok {
		return s.String()
	}
	return fmt.Sprintf("%v", req)
}

func (h *handler) Singleflight(method string, keyFunc ...func(req interface{}) string) {
	if method == "" {
		panic(fmt.Errorf("empty('') method is not allowed for Singleflight"))
	}
	if h.singleflights == nil {
		h.singleflights = map[string]func(req interface{}) string{}
	}
	kf := defaultSingleflightKey
	if len(keyFunc) > 0 && keyFunc[0] != nil {
		kf = keyFunc[0]
	}
	h.singleflights[method] = kf
}

func (h *handler) SingleflightKey(method string, req interface{}) (string, bool) {
	if h.singleflights == nil {
		return "", false
	}
	kf, ok := h.singleflights[method]
	if !ok {
		return "", false
	}
	return kf(req), true
}

// isStructPtr reports whether t is a pointer to a struct.
func isStructPtr(t reflect.Type) bool {
	return t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct
}

// newStructHandler builds the handler for a Register method without a
// "Binding" method. fn is the bound method, of type
// func(context.Context, reqType, rspType), where both types are struct
// pointers. The handler creates req/rsp, binds the request into req (writing
// an error response on failure), calls fn with the Context and writes rsp.
func newStructHandler(fn reflect.Value, reqType, rspType reflect.Type) HandlerFunc {
	return func(ctx *Context) {
		req := reflect.New(reqType.Elem())
		if err := ctx.Bind(req.Interface()); err != nil {
			ctx.Error(err)
			return
		}
		rsp := reflect.New(rspType.Elem())
		fn.Call([]reflect.Value{reflect.ValueOf(ctx), req, rsp})
		ctx.Write(rsp.Interface())
	}
}

func (h *handler) HandleNotFound(cb HandlerFunc) {
	h.handle("", cb)
}

// handle registers the chain of method. On first use it also registers the
// default "" route, which responds ErrMethodNotFound.
func (h *handler) handle(method string, cb HandlerFunc, args ...interface{}) {
	if h.routes == nil {
		h.routes = map[string]*routerHandler{}
	}
	if len(method) > MaxMethodLen {
		panic(fmt.Errorf("invalid method length %v(> MaxMethodLen %v)", len(method), MaxMethodLen))
	}

	if _, ok := h.routes[""]; !ok {
		rh := &routerHandler{
			async:    false,
			handlers: make([]HandlerFunc, len(h.middles)+1),
		}
		copy(rh.handlers, h.middles)
		rh.handlers[len(h.middles)] = func(ctx *Context) {
			ctx.Error(ErrMethodNotFound)
			ctx.Next()
		}
		h.routes[""] = rh
	}

	if _, ok := h.routes[method]; ok && method != "" {
		panic(fmt.Errorf("handler exist for method %v ", method))
	}

	async := h.AsyncResponse()
	if len(args) > 0 {
		if bv, ok := args[0].(bool); ok {
			async = bv
		}
	}
	rh := &routerHandler{
		async:    async,
		handlers: make([]HandlerFunc, len(h.middles)+1),
	}
	copy(rh.handlers, h.middles)
	rh.handlers[len(h.middles)] = func(ctx *Context) {
		cb(ctx)
		ctx.Next()
	}
	h.routes[method] = rh
}

func (h *handler) HandleStream(method string, cb StreamHandlerFunc, args ...interface{}) {
	if h.streams == nil {
		h.streams = map[string]*streamHandler{}
	}
	if len(method) > MaxMethodLen {
		panic(fmt.Errorf("invalid method length %v(> MaxMethodLen %v)", len(method), MaxMethodLen))
	}

	if _, ok := h.streams[method]; ok && method != "" {
		panic(fmt.Errorf("stream handler exist for method %v ", method))
	}

	async := h.AsyncResponse()
	if len(args) > 0 {
		if bv, ok := args[0].(bool); ok {
			async = bv
		}
	}
	rh := &streamHandler{
		async:   async,
		handler: cb,
	}
	h.streams[method] = rh
}

func (h *handler) Recv(c *Client) (*Message, error) {
	var (
		err     error
		message *Message
	)

	if h.beforeRecv != nil {
		if err = h.beforeRecv(c.Conn); err != nil {
			return nil, err
		}
	}
	if h.readTimeout > 0 {
		c.Conn.SetReadDeadline(time.Now().Add(h.readTimeout))
	}

	_, err = io.ReadFull(c.Reader, c.Head[:])
	if err != nil {
		return nil, err
	}

	message, err = c.Head.message(h)
	if err != nil {
		return nil, err
	}

	if message.Len() >= HeadLen {
		_, err = io.ReadFull(c.Reader, message.Buffer[HeaderIndexBodyLenEnd:])
	}

	return message, err
}

func (h *handler) Send(conn net.Conn, buffer []byte) (int, error) {
	if h.beforeSend != nil {
		if err := h.beforeSend(conn); err != nil {
			return -1, err
		}
	}
	if h.writeTimeout > 0 {
		conn.SetWriteDeadline(time.Now().Add(h.writeTimeout))
	}

	n, err := conn.Write(buffer)
	return n, err
}

func (h *handler) SendN(conn net.Conn, buffers net.Buffers) (int, error) {
	if h.beforeSend != nil {
		if err := h.beforeSend(conn); err != nil {
			return -1, err
		}
	}
	if h.writeTimeout > 0 {
		conn.SetWriteDeadline(time.Now().Add(h.writeTimeout))
	}

	n64, err := buffers.WriteTo(conn)
	return int(n64), err
}

func (h *handler) OnMessage(c *Client, msg *Message) {
	defer util.Recover()

	switch msg.Cmd() {
	case CmdPing:
		c.Pong()
		return
	case CmdPong:
		return
	}

	for i := len(h.msgCoders) - 1; i >= 0; i-- {
		msg = h.msgCoders[i].Decode(c, msg)
	}

	ml := msg.MethodLen()
	if ml <= 0 || ml > MaxMethodLen || ml > (msg.Len()-HeadLen) {
		log.Warn("%v OnMessage: invalid request method length %v, dropped", h.LogTag(), ml)
		return
	}

	cmd := msg.Cmd()
	switch cmd {
	case CmdRequest, CmdNotify:
		method := msg.method()
		if rh, ok := h.routes[method]; ok {
			ctx := newContext(c, msg, rh.handlers)
			if !rh.async {
				ctx.Next()
				h.OnContextDone(ctx)
			} else {
				h.AsyncExecute(func() {
					ctx.Next()
					h.OnContextDone(ctx)
				})
			}
		} else {
			if rh, ok = h.routes[""]; ok {
				ctx := newContext(c, msg, rh.handlers)
				ctx.Next()
				h.OnContextDone(ctx)
			}

			if cmd == CmdRequest {
				log.Warn("%v OnMessage: invalid Call with method: [%v], no handler", h.LogTag(), method)
			} else {
				log.Warn("%v OnMessage: invalid Notify with method: [%v], no handler", h.LogTag(), method)
			}
		}
	case CmdResponse:
		if !msg.IsAsync() {
			seq := msg.Seq()
			session, ok := c.getSession(seq)
			if ok {
				session.done <- msg
			} else {
				h.OnSessionMiss(c, msg)
				log.Warn("%v OnMessage: session not exist or expired", h.LogTag())
			}
		} else {
			ah, ok := c.getAndDeleteAsyncHandler(msg.Seq())
			if ok {
				if ah.timer != nil {
					ah.timer.Stop()
				}
				ctx := newContext(c, msg, nil)
				ah.handler(ctx, msg.Error())
				putAsyncHandler(ah)
				h.OnContextDone(ctx)
			} else {
				h.OnSessionMiss(c, msg)
				log.Warn("%v OnMessage: async handler not exist or expired", h.LogTag())
			}
		}
	case CmdStream:
		id := msg.Seq()
		local := !msg.IsStreamLocal()
		eof := msg.IsStreamEOF()
		method := msg.method()
		stream, ok := c.getStreamAndPushMsg(id, local, eof)
		if !ok {
			sh, ok := h.streams[method]
			if ok && !local {
				stream = c.newStream(msg.method(), id, false)
				stream.onMessage(msg)
				if eof {
					stream.CloseRecv()
				}
				if !sh.async {
					sh.handler(stream)
				} else {
					h.AsyncExecute(func() { sh.handler(stream) })
				}
			} else {
				h.onMessageDone(c, msg)
				log.Warn("%v OnMessage: invalid Stream with method: [%v], no handler", h.LogTag(), method)
			}
		} else {
			stream.onMessage(msg)
			if eof {
				stream.CloseRecv()
			}
		}
	default:
		log.Warn("%v OnMessage: invalid cmd [%v]", h.LogTag(), msg.Cmd())
		go c.Stop()
	}
}

func (h *handler) Malloc(size int) []byte {
	if h.malloc != nil {
		return h.malloc(size)
	}
	return make([]byte, size)
}

func (h *handler) HandleMalloc(f func(int) []byte) {
	h.malloc = f
}

func (h *handler) Append(b []byte, more ...byte) []byte {
	if h.append != nil {
		return h.append(b, more...)
	}
	return append(b, more...)
}

func (h *handler) HandleAppend(f func(b []byte, more ...byte) []byte) {
	h.append = f
}

func (h *handler) Free(b []byte) {
	if h.free != nil {
		h.free(b)
	}
}

func (h *handler) HandleFree(f func([]byte)) {
	h.free = f
}

func (h *handler) EnablePool(enable bool) {
	if enable {
		h.HandleMalloc(DefaultAllocator.Malloc)
		h.HandleAppend(DefaultAllocator.Append)
		h.HandleFree(DefaultAllocator.Free)
		h.HandleContextDone(func(ctx *Context) {
			ctx.Release()
		})
		h.HandleMessageDone(func(c *Client, m *Message) {
			m.Release()
		})
	} else {
		h.HandleMalloc(func(size int) []byte {
			return make([]byte, size)
		})
		h.HandleAppend(func(b []byte, more ...byte) []byte {
			return append(b, more...)
		})
		h.HandleFree(func(buf []byte) {})
		h.HandleContextDone(func(ctx *Context) {})
		h.HandleMessageDone(func(c *Client, m *Message) {})
	}
}

func (h *handler) Context() (context.Context, context.CancelFunc) {
	return h.ctx, h.cancel
}

func (h *handler) SetContext(ctx context.Context, cancel context.CancelFunc) {
	h.ctx = ctx
	h.cancel = cancel
}

func (h *handler) Cancel() {
	if h.cancel != nil {
		h.cancel()
	}
}

func (h *handler) NewMessage(cmd byte, method string, v interface{}, isError bool, isAsync bool, seq uint64, codec codec.Codec, values map[interface{}]interface{}) *Message {
	return newMessage(cmd, method, v, false, false, seq, h, codec, values)
}

func (h *handler) NewMessageWithBuffer(buffer []byte) *Message {
	msg := messagePool.Get().(*Message)
	msg.Buffer = buffer
	msg.handler = h
	return msg
}

// SetAsyncExecutor sets the executor of AsyncExecute.
func (h *handler) SetAsyncExecutor(executor func(f func())) {
	h.executor = executor
}

// AsyncExecute runs f with the executor, or in a new goroutine if none is set.
func (h *handler) AsyncExecute(f func()) {
	if h.executor != nil {
		h.executor(f)
	} else {
		go util.Safe(f)
	}
}

// NewHandler returns a Handler with the default settings: BatchRecv,
// BatchSend, AsyncWrite and AsyncResponse enabled, an 8KB recv buffer, a send
// queue of 4096, a stream queue of 4, and a connected callback disabling
// TCP_NODELAY.
func NewHandler() Handler {
	h := &handler{
		logtag:          "[ARPC CLI]",
		batchRecv:       true,
		batchSend:       true,
		asyncWrite:      true,
		asyncResponse:   true,
		recvBufferSize:  8192,
		sendQueueSize:   4096,
		streamQueueSize: 4,
		maxBodyLen:      DefaultMaxBodyLen,
	}
	h.wrapReader = func(conn net.Conn) io.Reader {
		return bufio.NewReaderSize(conn, h.recvBufferSize)
	}
	h.HandleConnected(func(cli *Client) {
		if tcpConn, ok := cli.Conn.(*net.TCPConn); ok {
			tcpConn.SetNoDelay(false)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	h.ctx = ctx
	h.cancel = cancel
	return h
}

// SetHandler replaces DefaultHandler.
func SetHandler(h Handler) {
	DefaultHandler = h
}

// SetLogTag sets DefaultHandler's log tag.
func SetLogTag(tag string) {
	DefaultHandler.SetLogTag(tag)
}

// HandleConnected calls DefaultHandler.HandleConnected.
func HandleConnected(onConnected func(*Client)) {
	DefaultHandler.HandleConnected(onConnected)
}

// HandleDisconnected calls DefaultHandler.HandleDisconnected.
func HandleDisconnected(onDisConnected func(*Client)) {
	DefaultHandler.HandleDisconnected(onDisConnected)
}

// HandleReconnect calls DefaultHandler.HandleReconnect.
func HandleReconnect(onReconnect func(c *Client, info *ReconnectInfo)) {
	DefaultHandler.HandleReconnect(onReconnect)
}

// HandleOverstock calls DefaultHandler.HandleOverstock.
func HandleOverstock(onOverstock func(c *Client, m *Message)) {
	DefaultHandler.HandleOverstock(onOverstock)
}

// HandleMessageDropped calls DefaultHandler.HandleMessageDropped.
func HandleMessageDropped(onOverstock func(c *Client, m *Message)) {
	DefaultHandler.HandleMessageDropped(onOverstock)
}

// HandleSessionMiss calls DefaultHandler.HandleSessionMiss.
func HandleSessionMiss(onSessionMiss func(c *Client, m *Message)) {
	DefaultHandler.HandleSessionMiss(onSessionMiss)
}

// BeforeRecv calls DefaultHandler.BeforeRecv.
func BeforeRecv(h func(net.Conn) error) {
	DefaultHandler.BeforeRecv(h)
}

// BeforeSend calls DefaultHandler.BeforeSend.
func BeforeSend(h func(net.Conn) error) {
	DefaultHandler.BeforeSend(h)
}

// BatchRecv calls DefaultHandler.BatchRecv.
func BatchRecv() bool {
	return DefaultHandler.BatchRecv()
}

// SetBatchRecv calls DefaultHandler.SetBatchRecv.
func SetBatchRecv(batch bool) {
	DefaultHandler.SetBatchRecv(batch)
}

// BatchSend calls DefaultHandler.BatchSend.
func BatchSend() bool {
	return DefaultHandler.BatchSend()
}

// SetBatchSend calls DefaultHandler.SetBatchSend.
func SetBatchSend(batch bool) {
	DefaultHandler.SetBatchSend(batch)
}

// AsyncResponse calls DefaultHandler.AsyncResponse.
func AsyncResponse() bool {
	return DefaultHandler.AsyncResponse()
}

// SetAsyncResponse calls DefaultHandler.SetAsyncResponse.
func SetAsyncResponse(async bool) {
	DefaultHandler.SetAsyncResponse(async)
}

// SetReaderWrapper calls DefaultHandler.SetReaderWrapper.
func SetReaderWrapper(wrapper func(conn net.Conn) io.Reader) {
	DefaultHandler.SetReaderWrapper(wrapper)
}

// RecvBufferSize calls DefaultHandler.RecvBufferSize.
func RecvBufferSize() int {
	return DefaultHandler.RecvBufferSize()
}

// SetRecvBufferSize calls DefaultHandler.SetRecvBufferSize.
func SetRecvBufferSize(size int) {
	DefaultHandler.SetRecvBufferSize(size)
}

// SendBufferSize calls DefaultHandler.SendBufferSize.
func SendBufferSize() int {
	return DefaultHandler.SendBufferSize()
}

// SetSendBufferSize calls DefaultHandler.SetSendBufferSize.
func SetSendBufferSize(size int) {
	DefaultHandler.SetSendBufferSize(size)
}

// ReadTimeout calls DefaultHandler.ReadTimeout.
func ReadTimeout() time.Duration {
	return DefaultHandler.ReadTimeout()
}

// SetReadTimeout calls DefaultHandler.SetReadTimeout.
func SetReadTimeout(timeout time.Duration) {
	DefaultHandler.SetReadTimeout(timeout)
}

// WriteTimeout calls DefaultHandler.WriteTimeout.
func WriteTimeout() time.Duration {
	return DefaultHandler.WriteTimeout()
}

// SetWriteTimeout calls DefaultHandler.SetWriteTimeout.
func SetWriteTimeout(timeout time.Duration) {
	DefaultHandler.SetWriteTimeout(timeout)
}

// SendQueueSize calls DefaultHandler.SendQueueSize.
func SendQueueSize() int {
	return DefaultHandler.SendQueueSize()
}

// SetSendQueueSize calls DefaultHandler.SetSendQueueSize.
func SetSendQueueSize(size int) {
	DefaultHandler.SetSendQueueSize(size)
}

// StreamQueueSize calls DefaultHandler.StreamQueueSize.
func StreamQueueSize() int {
	return DefaultHandler.StreamQueueSize()
}

// SetStreamQueueSize calls DefaultHandler.SetStreamQueueSize.
func SetStreamQueueSize(size int) {
	DefaultHandler.SetStreamQueueSize(size)
}

// MaxBodyLen calls DefaultHandler.MaxBodyLen.
func MaxBodyLen() int {
	return DefaultHandler.MaxBodyLen()
}

// SetMaxBodyLen calls DefaultHandler.SetMaxBodyLen.
func SetMaxBodyLen(l int) {
	DefaultHandler.SetMaxBodyLen(l)
}

// Use calls DefaultHandler.Use.
func Use(h HandlerFunc) {
	DefaultHandler.Use(h)
}

// UseCoder calls DefaultHandler.UseCoder.
func UseCoder(coder MessageCoder) {
	DefaultHandler.UseCoder(coder)
}

// Handle calls DefaultHandler.Handle.
func Handle(m string, h HandlerFunc, args ...interface{}) {
	DefaultHandler.Handle(m, h, args...)
}

// Register calls DefaultHandler.Register.
func Register(m string, h interface{}) error {
	return DefaultHandler.Register(m, h)
}

// Singleflight calls DefaultHandler.Singleflight.
func Singleflight(method string, keyFunc ...func(req interface{}) string) {
	DefaultHandler.Singleflight(method, keyFunc...)
}

// HandleNotFound calls DefaultHandler.HandleNotFound.
func HandleNotFound(h HandlerFunc) {
	DefaultHandler.HandleNotFound(h)
}

// HandleMalloc calls DefaultHandler.HandleMalloc.
func HandleMalloc(f func(int) []byte) {
	DefaultHandler.HandleMalloc(f)
}

// HandleFree calls DefaultHandler.HandleFree.
func HandleFree(f func([]byte)) {
	DefaultHandler.HandleFree(f)
}

// EnablePool calls DefaultHandler.EnablePool.
func EnablePool(enable bool) {
	DefaultHandler.EnablePool(enable)
}

// SetAsyncExecutor calls DefaultHandler.SetAsyncExecutor.
func SetAsyncExecutor(executor func(f func())) {
	DefaultHandler.SetAsyncExecutor(executor)
}

// AsyncExecute calls DefaultHandler.AsyncExecute.
func AsyncExecute(f func()) {
	DefaultHandler.AsyncExecute(f)
}
