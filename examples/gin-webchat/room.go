// Room .
package main

import (
	"fmt"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/log"
)

type Room struct {
	users       map[*arpc.Client]uint64
	chEnterRoom chan *arpc.Client
	chLeaveRoom chan *arpc.Client
	chBroadcast chan *Message
	chStop      chan struct{}
}

// Enter .
func (room *Room) Enter(cli *arpc.Client) {
	room.chEnterRoom <- cli
}

// Leave .
func (room *Room) Leave(cli *arpc.Client) {
	room.chLeaveRoom <- cli
}

// Broadcast .
func (room *Room) Broadcast(msg *Message) {
	room.chBroadcast <- msg
}

// Run .
func (room *Room) Run() *Room {
	go func() {
		for userCnt := uint64(10000); true; userCnt++ {
			select {
			case cli := <-room.chEnterRoom:
				room.users[cli] = userCnt
				cli.Set("user", userCnt)
				userid := fmt.Sprintf("%v", userCnt)
				cli.Notify("/chat/server/userid", userid, 0)
				room.broadcastMsg("/chat/server/userenter", NewMessage(userCnt, ""))
				userCnt++
				log.Info("[user_%v] enter room", userid)
			case cli := <-room.chLeaveRoom:
				delete(room.users, cli)
				user, ok := cli.Get("user")
				if ok {
					userid, ok := user.(uint64)
					if ok {
						room.broadcastMsg("/chat/server/userleave", NewMessage(userid, ""))
						log.Info("[user_%v] leave room", userid)
					}
				}
			case msg := <-room.chBroadcast:
				room.broadcastMsg("/chat/server/broadcast", msg)
			case <-room.chStop:
				room.broadcastMsg("/chat/server/shutdown", nil)
				return
			}
		}
	}()
	return room
}

// Stop .
func (room *Room) Stop() *Room {
	close(room.chStop)
	return room
}

func (room *Room) broadcastMsg(method string, msg *Message) {
	for cli := range room.users {
		cli.Notify(method, msg, 0)
	}
}

// NewRoom .
func NewRoom() *Room {
	return &Room{
		users:       map[*arpc.Client]uint64{},
		chEnterRoom: make(chan *arpc.Client, 1024),
		chLeaveRoom: make(chan *arpc.Client, 1024),
		chBroadcast: make(chan *Message, 1024),
		chStop:      make(chan struct{}),
	}
}
