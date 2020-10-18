var SOCK_STATE_CLOSED = 0;
var SOCK_STATE_CONNECTING = 1;
var SOCK_STATE_CONNECTED = 2;

var CmdNone = 0;
var CmdRequest = 1;
var CmdResponse = 2;
var CmdNotify = 3;

var HeaderIndexBodyLenBegin = 0;
var HeaderIndexBodyLenEnd = 4;
var HeaderIndexReserved = 4;
var HeaderIndexCmd = 5;
var HeaderIndexFlag = 6;
var HeaderIndexMethodLen = 7;
var HeaderIndexSeqBegin = 8;
var HeaderIndexSeqEnd = 16;
var HeaderFlagMaskError = 0x01;
var HeaderFlagMaskAsync = 0x02;

var ErrClosed = "[client stopped]";
var ErrReconnecting = "[error reconnecting]";

function Codec() {
    this.Marshal = function (obj) {
        if (typeof (obj) == 'string') {
            return new TextEncoder("utf-8").encode(obj);
        }
        return new TextEncoder("utf-8").encode(JSON.stringify(obj));
    }
    this.Unmarshal = function (data) {
        try {
            data = JSON.parse(new TextDecoder("utf-8").decode(data));
            return data;
        } catch (e) {
            data = new TextDecoder("utf-8").decode(data);
            return data;
        }
    }
}

function Context(cli, head, body, method, data, msgObj) {
    this.cli = cli;
    this.head = head;
    this.body = body;
    this.method = method;
    this.data = data;
    this.msgObj = msgObj;
}


function ArpcClient(url, codec) {
    var client = this;

    this.ws;
    this.url = url;
    this.codec = codec || new Codec();

    this.seqNum = 0;
    this.sessionMap = {};

    this.handlers = {};

    this.state = SOCK_STATE_CONNECTING;

    this.Handle = function (method, h) {
        if (this.handlers[method]) {
            throw ("handler for [${method}] exists");
        }
        this.handlers[method] = { h: h };
    }

    this.Call = function (method, request, timeout, cb) {
        if (this.state == SOCK_STATE_CLOSED) {
            return new Promise(function (resolve, reject) {
                resolve({ data: null, err: ErrClosed });
            });
        }
        if (this.state == SOCK_STATE_CONNECTING) {
            return new Promise(function (resolve, reject) {
                resolve({ data: null, err: ErrReconnecting });
            });
        }
        this.seqNum++;
        var seq = this.seqNum;
        var session = {};
        var p = new Promise(function (resolve, reject) {
            session.resolve = resolve;
        });
        if (typeof (cb) == 'function') {
            session.resolve = cb;
        }
        this.sessionMap[seq] = session;

        if (timeout > 0) {
            session.timer = setTimeout(function () {
                var isErr = 1;
                delete (this.sessionMap[seq]);
                session.resolve({ data: null, err: "timeout" });
            }, timeout);
        }

        var buffer;
        if (request) {
            var data = this.codec.Marshal(request);
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
            buffer[i] = (bodyLen >> ((i - HeaderIndexBodyLenBegin) * 8)) & 0xFF;
        }

        buffer[HeaderIndexCmd] = CmdRequest & 0xFF;
        buffer[HeaderIndexMethodLen] = method.length & 0xFF;
        for (var i = HeaderIndexSeqBegin; i < HeaderIndexSeqBegin + 4; i++) {
            buffer[i] = (seq >> ((i - HeaderIndexSeqBegin) * 8)) & 0xFF;
        }

        var methodBuffer = new TextEncoder("utf-8").encode(method);
        for (var i = 0; i < methodBuffer.length; i++) {
            buffer[16 + i] = methodBuffer[i];
        }

        this.ws.send(buffer);

        return p;
    }

    this.Notify = function (method, notify) {
        if (this.state == SOCK_STATE_CLOSED) {
            return ErrClosed;
        }
        if (this.state == SOCK_STATE_CONNECTING) {
            return ErrReconnecting;
        }
        this.seqNum++;
        var buffer;
        if (notify) {
            var data = this.codec.Marshal(notify);
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
            buffer[i] = (bodyLen >> ((i - HeaderIndexBodyLenBegin) * 8)) & 0xFF;
        }

        buffer[HeaderIndexCmd] = CmdNotify & 0xFF;
        buffer[HeaderIndexMethodLen] = method.length & 0xFF;
        for (var i = HeaderIndexSeqBegin; i < HeaderIndexSeqBegin + 4; i++) {
            buffer[i] = (this.seqNum >> ((i - HeaderIndexSeqBegin) * 8)) & 0xFF;
        }

        var methodBuffer = new TextEncoder("utf-8").encode(method);
        for (var i = 0; i < methodBuffer.length; i++) {
            buffer[16 + i] = methodBuffer[i];
        }

        this.ws.send(buffer);
    }

    this.Shutdown = function () {
        this.ws.close();
        this.state = SOCK_STATE_CLOSED;
    }

    this.onMessage = function (event) {
        try {
            var offset = 0;
            while (offset < event.data.byteLength) {
                var headArr = new Uint8Array(event.data.slice(offset, offset + 16));
                var bodyLen = 0;
                for (var i = HeaderIndexBodyLenBegin; i < HeaderIndexBodyLenEnd; i++) {
                    bodyLen |= (headArr[i] << ((i - HeaderIndexBodyLenBegin) * 8)) & 0xFF;
                }
                var cmd = headArr[HeaderIndexCmd];
                var isError = headArr[HeaderIndexFlag] & HeaderFlagMaskError;
                var isAsync = headArr[HeaderIndexFlag] & HeaderFlagMaskAsync;
                var methodLen = headArr[HeaderIndexMethodLen];
                var method = new TextDecoder("utf-8").decode(event.data.slice(offset + 16, offset + 16 + methodLen));
                var bodyArr;
                if (bodyLen > methodLen) {
                    bodyArr = event.data.slice(offset + 16 + methodLen, offset + 16 + methodLen + bodyLen);
                }
                var seq = 0;
                for (var i = offset + HeaderIndexSeqBegin; i < offset + HeaderIndexSeqBegin + 4; i++) {
                    seq |= headArr[i] << (i - offset - HeaderIndexSeqBegin);
                }

                if (methodLen == 0) {
                    console.log("[ArpcClient] onMessage: invalid request message with 0 method length, dropped");
                    return
                }

                switch (cmd) {
                    case CmdRequest:
                    case CmdNotify:
                        var handler = client.handlers[method]
                        if (handler) {
                            var data = client.codec.Unmarshal(bodyArr);
                            handler.h(new Context(client, headArr, bodyArr, method, data));
                        } else {
                            console.log("[ArpcClient] onMessage: invalid method: [%s], no handler", method);
                            return
                        }
                        break;
                    case CmdResponse:
                        var session = client.sessionMap[seq];
                        if (session) {
                            clearTimeout(session.timer);
                            delete (client.sessionMap[seq]);
                            var data = client.codec.Unmarshal(bodyArr);
                            if (isError) {
                                session.resolve({ data: null, err: data });
                                return;
                            }
                            session.resolve({ data: data, err: null });
                        } else {
                            console.log("[ArpcClient] onMessage: session [%d] missing", seq);
                            return;
                        }
                        break;
                    default:
                        break;
                }
                offset += 16 + bodyLen;
            }
        } catch (e) {
            console.log("[ArpcClient] onMessage: panic:", e);
        }
    }

    this.init = function () {
        console.log("[ArpcClient] init...");
        if ('WebSocket' in window) {
            client.ws = new WebSocket(this.url);
        } else if ('MozWebSocket' in window) {
            client.ws = new MozWebSocket(this.url);
        } else {
            client.ws = new SockJS(this.url);
        }

        // 消息类型,不设置则默认为'text'
        client.ws.binaryType = 'arraybuffer';

        client.state = SOCK_STATE_CONNECTING;

        client.ws.onopen = function (event) {
            client.state = SOCK_STATE_CONNECTED;
            console.log("[ArpcClient] websocket onopen");
            if (client.onOpen) {
                client.onOpen(client);
            }
        };
        client.ws.onclose = function (event) {
            console.log("[ArpcClient] websocket onclose");
            if (client.onClose) {
                client.onClose(client);
            }
            client.ws.close();

            // shutdown
            if (client.state == SOCK_STATE_CLOSED) {
                return;
            }
            client.state = SOCK_STATE_CONNECTING;
            client.init();
        };
        client.ws.onerror = function (event) {
            console.log("[ArpcClient] websocket onerror");
            if (client.onError) {
                client.onError(client);
            }
        };
        client.ws.onmessage = client.onMessage;
    }

    try {
        this.init();
    } catch (e) {
        console.log("[ArpcClient] init() failed:", e);
    }
}
