package arpc

// WebsocketConn defines websocket-conn interface.
type WebsocketConn interface {
	HandleWebsocket(func())
}
