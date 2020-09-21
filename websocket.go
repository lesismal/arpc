package arpc

// WebsocketConn .
type WebsocketConn interface {
	HandleWebsocket(func())
}
