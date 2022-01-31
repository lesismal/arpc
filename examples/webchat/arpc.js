var _SOCK_STATE_CLOSED = 0;
var _SOCK_STATE_CONNECTING = 1;
var _SOCK_STATE_CONNECTED = 2;

var _CmdNone = 0;
var _CmdRequest = 1;
var _CmdResponse = 2;
var _CmdNotify = 3;

var _HeaderIndexBodyLenBegin = 0;
var _HeaderIndexBodyLenEnd = 4;
var _HeaderIndexReserved = 4;
var _HeaderIndexCmd = 5;
var _HeaderIndexFlag = 6;
var _HeaderIndexMethodLen = 7;
var _HeaderIndexSeqBegin = 8;
var _HeaderIndexSeqEnd = 16;
var _HeaderFlagMaskError = 0x01;
var _HeaderFlagMaskAsync = 0x02;

var _ErrClosed = "[client stopped]";
var _ErrDisconnected = "[error disconnected]";
var _ErrReconnecting = "[error reconnecting]";

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


function ArpcClient(url, codec, httpUrl, httpMethod) {
    var client = this;

    this.ws;
    this.url = url;
    this.codec = codec || new Codec();

    this.httpUrl = httpUrl || "/";
    this.httpMethod = httpMethod || "POST";

    this.seqNum = 0;
    this.sessionMap = {};

    this.handlers = {};

    this.state = _SOCK_STATE_CONNECTING;

    this.handle = function (method, h) {
        if (this.handlers[method]) {
            throw ("handler for [${method}] exists");
        }
        this.handlers[method] = { h: h };
    }

    this.callHttp = function (method, request, timeout, cb) {
        this.call(method, request, timeout, cb, true);
    }
    this.call = function (method, request, timeout, cb, isHttp) {
        if (this.state == _SOCK_STATE_CLOSED) {
            return new Promise(function (resolve, reject) {
                resolve({ data: null, err: _ErrClosed });
            });
        }
        if (this.state == _SOCK_STATE_CONNECTING) {
            return new Promise(function (resolve, reject) {
                resolve({ data: null, err: _ErrReconnecting });
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
                delete (client.sessionMap[seq]);
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
        for (var i = _HeaderIndexBodyLenBegin; i < _HeaderIndexBodyLenEnd; i++) {
            buffer[i] = (bodyLen >> ((i - _HeaderIndexBodyLenBegin) * 8)) & 0xFF;
        }

        buffer[_HeaderIndexCmd] = _CmdRequest & 0xFF;
        buffer[_HeaderIndexMethodLen] = method.length & 0xFF;
        for (var i = _HeaderIndexSeqBegin; i < _HeaderIndexSeqBegin + 4; i++) {
            buffer[i] = (seq >> ((i - _HeaderIndexSeqBegin) * 8)) & 0xFF;
        }

        var methodBuffer = new TextEncoder("utf-8").encode(method);
        for (var i = 0; i < methodBuffer.length; i++) {
            buffer[16 + i] = methodBuffer[i];
        }

        if (!isHttp) {
            this.ws.send(buffer);
        } else {
            this.request(buffer, this._onMessage);
        }

        return p;
    }

    this.notifyHttp = function (method, notify) {
        this.notify(method, notify, true);
    }
    this.notify = function (method, notify, isHttp) {
        if (this.state == _SOCK_STATE_CLOSED) {
            return _ErrClosed;
        }
        if (this.state == _SOCK_STATE_CONNECTING) {
            return _ErrReconnecting;
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
        for (var i = _HeaderIndexBodyLenBegin; i < _HeaderIndexBodyLenEnd; i++) {
            buffer[i] = (bodyLen >> ((i - _HeaderIndexBodyLenBegin) * 8)) & 0xFF;
        }
        buffer[_HeaderIndexCmd] = _CmdNotify & 0xFF;
        buffer[_HeaderIndexMethodLen] = method.length & 0xFF;
        for (var i = _HeaderIndexSeqBegin; i < _HeaderIndexSeqBegin + 4; i++) {
            buffer[i] = (this.seqNum >> ((i - _HeaderIndexSeqBegin) * 8)) & 0xFF;
        }

        var methodBuffer = new TextEncoder("utf-8").encode(method);
        for (var i = 0; i < methodBuffer.length; i++) {
            buffer[16 + i] = methodBuffer[i];
        }

        if (!isHttp) {
            this.ws.send(buffer);
        } else {
            this.request(buffer, function () { });
        }
    }

    this.shutdown = function () {
        this.ws.close();
        this.state = _SOCK_STATE_CLOSED;
    }

    this.request = function (data, cb) {
        let resolve;
        let p = new Promise(function (res) {
            resolve = res;
            if (typeof (cb) == 'function') {
                resolve = function (ret) {
                    res(ret);
                    cb(ret);
                }
            }
            let r = new XMLHttpRequest();
            r.open(this.httpMethod, this.httpUrl, true);
            r.onreadystatechange = function () {
                if (r.readyState != 4) {
                    return;
                }
                if (r.status != 200) {
                    resolve({ code: r.status, data: null, err: `request "${this.httpUrl}" failed with status code ${r.status} ` });
                    return;
                }
                // r.responseText
                resolve({ code: r.status, data: r.response, err: null });
            };
            r.send(data);
        });
        return p;
    }

    this._onMessage = function (event) {
        try {
            var offset = 0;
            while (offset < event.data.byteLength) {
                var headArr = new Uint8Array(event.data.slice(offset, offset + 16));
                var bodyLen = 0;
                for (var i = _HeaderIndexBodyLenBegin; i < _HeaderIndexBodyLenEnd; i++) {
                    bodyLen |= (headArr[i] << ((i - _HeaderIndexBodyLenBegin) * 8));
                }
                var cmd = headArr[_HeaderIndexCmd];
                var isError = headArr[_HeaderIndexFlag] & _HeaderFlagMaskError;
                var isAsync = headArr[_HeaderIndexFlag] & _HeaderFlagMaskAsync;
                var methodLen = headArr[_HeaderIndexMethodLen];
                var method = new TextDecoder("utf-8").decode(event.data.slice(offset + 16, offset + 16 + methodLen));
                var bodyArr;
                if (bodyLen > methodLen) {
                    bodyArr = event.data.slice(offset + 16 + methodLen, offset + 16 + methodLen + bodyLen);
                }
                var seq = 0;
                for (var i = offset + _HeaderIndexSeqBegin; i < offset + _HeaderIndexSeqBegin + 4; i++) {
                    seq |= headArr[i] << (i - offset - _HeaderIndexSeqBegin);
                }

                if (methodLen == 0) {
                    console.log("[ArpcClient] onMessage: invalid request message with 0 method length, dropped");
                    return
                }

                switch (cmd) {
                    case _CmdRequest:
                    case _CmdNotify:
                        var handler = client.handlers[method]
                        if (handler) {
                            var data = client.codec.Unmarshal(bodyArr);
                            handler.h(new Context(client, headArr, bodyArr, method, data));
                        } else {
                            console.log("[ArpcClient] onMessage: invalid method: [%s], no handler", method);
                            return
                        }
                        break;
                    case _CmdResponse:
                        var session = client.sessionMap[seq];
                        if (session) {
                            if (session.timer) {
                                clearTimeout(session.timer);
                            }
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

        client.state = _SOCK_STATE_CONNECTING;

        client.ws.onopen = function (event) {
            client.state = _SOCK_STATE_CONNECTED;
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

            for (var k in client.sessionMap) {
                var session = client.sessionMap[k];
                if (session) {
                    if (session.timer) {
                        clearTimeout(session.timer);
                    }
                    session.resolve({ data: null, err: _ErrDisconnected });
                }
            }
            // shutdown
            if (client.state == _SOCK_STATE_CLOSED) {
                return;
            }
            client.state = _SOCK_STATE_CONNECTING;
            client.init();
        };
        client.ws.onerror = function (event) {
            console.log("[ArpcClient] websocket onerror");
            if (client.onError) {
                client.onError(client);
            }
        };
        client.ws.onmessage = client._onMessage;
    }

    try {
        this.init();
    } catch (e) {
        console.log("[ArpcClient] init() failed:", e);
    }
}
