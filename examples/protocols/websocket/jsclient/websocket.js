var SOCK_STATE_CLOSED = 0;
var SOCK_STATE_CONNECTING = 1;
var SOCK_STATE_CONNECTED = 2;

var CmdNone = 0;
var CmdRequest = 1;
var CmdResponse = 2;
var CmdNotify = 3;

var HeaderIndexBodyLenBegin = 0
var HeaderIndexBodyLenEnd = 4
var HeaderIndexReserved = 4
var HeaderIndexCmd = 5
var HeaderIndexFlag = 6
var HeaderIndexMethodLen = 7
var HeaderIndexSeqBegin = 8
var HeaderIndexSeqEnd = 16
var HeaderFlagMaskError = 0x01
var HeaderFlagMaskAsync = 0x02

var ErrClosed = "[client stopped]";
var ErrReconnecting = "[error reconnecting]";

function Codec() {
    this.Marshal = function (obj) {
        if (typeof (obj) == 'string') {
            return new TextEncoder("utf-8").encode(obj);
        }
        return new TextEncoder("utf-8").encode(JSON.stringify(obj));
    }
    this.Unmarshal = function (data, reply) {
        if (typeof (reply) == 'string') {
            return [new TextDecoder("utf-8").decode(data), null];
        }
        var data;
        try {
            data = JSON.parse(data);
        } catch (e) {
            return [null, e]
        }
        return [data, null];
    }
}

function Context(cli, head, body, method, data, msgObj) {
    this.cli = cli;
    this.head = head;
    this.body = body;
    this.method = method;
    this.data = data;
    this.msgObj = msgObj;

    // TODO
    // Write = function (data) {
    //     return JSON.parse(data);
    // }
}


function ArpcClient(url, codec) {
    var ws;
    var url = url;
    var codec = codec || new Codec();

    var seqNum = 0;
    var sessionMap = {};

    var handlers = {};

    var init;
    var onMessage;

    // export method
    var Handle;
    var Call;
    var Notify;
    var Shutdown;

    var state = SOCK_STATE_CONNECTING;

    Handle = function (method, h, obj) {
        if (handlers[method]) {
            throw ("handler for [${method}] exists")
        }
        handlers[method] = { h: h, obj: obj };
    }

    Call = function (method, request, reply, timeout) {
        if (state == SOCK_STATE_CLOSED) {
            return new Promise(function (resolve, reject) {
                resolve({ data: null, err: ErrClosed });
            });
        }
        if (state == SOCK_STATE_CONNECTING) {
            return new Promise(function (resolve, reject) {
                resolve({ data: null, err: ErrReconnecting });
            });
        }
        seqNum++;
        var seq = seqNum
        var session = {};
        var p = new Promise(function (resolve, reject) {
            session.resolve = resolve;
            session.reject = reject;
            session.reply = reply;
        });
        sessionMap[seq] = session;

        if (timeout > 0) {
            session.timer = setTimeout(function () {
                var isErr = 1;
                delete (sessionMap[seq]);
                session.resolve(null, "timeout");
            }, timeout)
        }

        var buffer;
        if (request) {
            var data = codec.Marshal(request);
            if (data) {
                buffer = new Uint8Array(16 + method.length + data.length);
                for (var i = 0; i < data.length; i++) {
                    buffer[16 + method.length + i] = data[i];
                }
            }
        } else {
            buffer = new Uint8Array(16 + method.length);
        }
        var bodyLen = buffer.length - 16;
        for (var i = HeaderIndexBodyLenBegin; i < HeaderIndexBodyLenEnd; i++) {
            buffer[i] = (bodyLen >> ((i - HeaderIndexBodyLenBegin) * 8)) % 0xFF;
        }

        buffer[HeaderIndexCmd] = CmdRequest % 0xFF;
        buffer[HeaderIndexMethodLen] = method.length % 0xFF;
        for (var i = HeaderIndexSeqBegin; i < HeaderIndexSeqBegin + 4; i++) {
            buffer[i] = (seq >> ((i - HeaderIndexSeqBegin) * 8)) % 0xFF;
        }

        var methodBuffer = new TextEncoder("utf-8").encode(method);
        for (var i = 0; i < methodBuffer.length; i++) {
            buffer[16 + i] = methodBuffer[i];
        }

        ws.send(buffer);

        return p;
    }

    Notify = function (method, notify) {
        if (state == SOCK_STATE_CLOSED) {
            return ErrClosed;
        }
        if (state == SOCK_STATE_CONNECTING) {
            return ErrReconnecting;
        }
        seqNum++;
        var buffer;
        if (notify) {
            var data = codec.Marshal(notify);
            if (data) {
                buffer = new Uint8Array(16 + method.length + data.length);
                for (var i = 0; i < data.length; i++) {
                    buffer[16 + method.length + i] = data[i];
                }
            }
        } else {
            buffer = new Uint8Array(16 + method.length);
        }
        var bodyLen = buffer.length - 16;
        for (var i = HeaderIndexBodyLenBegin; i < HeaderIndexBodyLenEnd; i++) {
            buffer[i] = (bodyLen >> ((i - HeaderIndexBodyLenBegin) * 8)) % 0xFF;
        }

        buffer[HeaderIndexCmd] = CmdNotify % 0xFF;
        buffer[HeaderIndexMethodLen] = method.length % 0xFF;
        for (var i = HeaderIndexSeqBegin; i < HeaderIndexSeqBegin + 4; i++) {
            buffer[i] = (seqNum >> ((i - HeaderIndexSeqBegin) * 8)) % 0xFF;
        }

        var methodBuffer = new TextEncoder("utf-8").encode(method);
        for (var i = 0; i < methodBuffer.length; i++) {
            buffer[16 + i] = methodBuffer[i];
        }

        ws.send(buffer);
    }

    Shutdown = function () {
        ws.close();
        state = SOCK_STATE_CLOSED;
    }

    onMessage = function (event) {
        try {
            var offset = 0;
            while (offset < event.data.byteLength) {
                var headArr = new Uint8Array(event.data.slice(offset, offset + 16));
                var bodyLen = 0;// headArr.readUint32LE(offset + HeaderIndexBodyLenBegin);
                for (var i = HeaderIndexBodyLenBegin; i < HeaderIndexBodyLenEnd; i++) {
                    bodyLen |= (headArr[i] << ((i - HeaderIndexBodyLenBegin) * 8)) % 0xFF;
                }
                // var bodyLen = headArr[4] | headArr[5] << 8 | headArr[6] << 16 | headArr[7] << 24;
                var cmd = headArr[HeaderIndexCmd];
                var isError = headArr[HeaderIndexFlag] & HeaderFlagMaskError;
                var isAsync = headArr[HeaderIndexFlag] & HeaderFlagMaskAsync;
                var methodLen = headArr[HeaderIndexMethodLen];
                var method = new TextDecoder("utf-8").decode(event.data.slice(offset + 16, offset + 16 + methodLen));
                var bodyArr;
                if (bodyLen > methodLen) {
                    bodyArr = new Uint8Array(event.data.slice(offset + 16 + methodLen, offset + 16 + methodLen + bodyLen));
                }
                var seq = 0;
                for (var i = offset + HeaderIndexSeqBegin; i < offset + HeaderIndexSeqBegin + 4; i++) {
                    seq |= headArr[i] << (i - offset - HeaderIndexSeqBegin);
                }

                if (methodLen == 0) {
                    console.log("%v OnMessage: invalid request message with 0 method length, dropped", h.LogTag())
                    return
                }

                switch (cmd) {
                    case CmdRequest:
                    case CmdNotify:
                        var handler = handlers[method]
                        if (handler) {
                            var ret = codec.Unmarshal(bodyArr, handler.obj);
                            var data = ret[0];
                            var err = ret[1];
                            if (err) {
                                console.log(`handle [${method}] codec.Unmarshal failed: ${err}`);
                                return;
                            }
                            handler.h(new Context(this, headArr, bodyArr, method, data));
                        } else {
                            console.log("invalid method: [%s], no handler", method);
                            return
                        }
                        break;
                    case CmdResponse:
                        var session = sessionMap[seq];
                        if (session) {
                            clearTimeout(session.timer);
                            delete (sessionMap[seq]);
                            if (isError) {
                                var err = new TextDecoder("utf-8").decode(event.data.slice(offset + 16 + methodLen, offset + 16 + bodyLen));
                                session.resolve({ data: null, err: err });
                                return;
                            }
                            var ret = codec.Unmarshal(bodyArr, session.reply);
                            var data = ret[0];
                            var err = ret[1];
                            session.resolve({ data: data, err: err });
                        } else {
                            console.log("session [%d] missing:", seqNum);
                            return;
                        }
                        break;
                    default:
                        break;
                }
                offset += 16 + bodyLen;
            }
        } catch (e) {
            console.log("Websocket onMessage panic:", e);
        }
    }

    init = function () {
        if ('WebSocket' in window) {
            ws = new WebSocket(url);
        } else if ('MozWebSocket' in window) {
            ws = new MozWebSocket(url);
        } else {
            ws = new SockJS(url);
        }

        // 消息类型,不设置则默认为'text'
        ws.binaryType = 'arraybuffer';

        state = SOCK_STATE_CONNECTING;

        ws.onopen = function (event) {
            state = SOCK_STATE_CONNECTED;
            if (this.onOpen) {
                this.onOpen(this);
            }
        };
        ws.onclose = function (event) {
            if (this.onClose) {
                this.onClose(this);
            }
            ws.close();

            // shutdown
            if (state == SOCK_STATE_CLOSED) {
                return;
            }
            state = SOCK_STATE_CONNECTING;
            init();
        };
        ws.onerror = function (event) {
            if (this.onError) {
                this.onError(this);
            }
        };
        ws.onmessage = onMessage;
    }

    try {
        init();
    } catch (e) {
        console.log("Websocket constructor failed:", e);
    }

    this.Handle = Handle;
    this.Call = Call;
    this.Notify = Notify;
    this.Shutdown = Shutdown;
}


