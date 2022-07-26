// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
)

type Info struct {
	data []byte
	id   []byte //보통 보낸사람의 아이디이다..
	key  []byte //html로 부터 받은 정보.
}
type Log struct {
	data   []byte
	client *Client
	result []byte
}

var history []Log

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	join       chan Info
	cardselect chan Info
	// Register requests from the clients.
	register chan *Client
	// Unregister requests from clients.
	unregister chan *Client
	battle     chan Log
	bet        chan Log
}

func newHub() *Hub {

	return &Hub{
		join:       make(chan Info),
		cardselect: make(chan Info),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		battle:     make(chan Log),
		bet:        make(chan Log),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send1)
				close(client.send2)
			}
		case info := <-h.join: //모두를 업데이트하지만, 자기 사정에 맞게.
			for client := range h.clients {
				select {
				case client.send1 <- info:
				default:
					close(client.send1)
					delete(h.clients, client)
				}
			}
		case info := <-h.cardselect: //발신자를 제외한 나머지 클라이언트 업데이트
			strId := string(info.id[:])
			var senderId string = string(strId)

			for client := range h.clients {
				cid := string(client.id)
				if cid != senderId {
					select {
					case client.send2 <- info:
					default:
						close(client.send2)
						delete(h.clients, client)
					}
				}
			}
		case matchLog := <-h.battle:
			history = append(history, matchLog)
			var info Info
			if matchLog.client != nil {
				info = Info{
					data: matchLog.data,
					id:   matchLog.client.id,
					key:  matchLog.client.id,
				}
			} else {
				info = Info{
					data: matchLog.data,
					id:   []byte{},
					key:  []byte("-1"),
				}
			}
			for client := range h.clients {
				select {
				case client.send2 <- info:
				default:
					close(client.send2)
					delete(h.clients, client)
				}
			}
		case betLog := <-h.bet:
			history = append(history, betLog)
			var info Info

			info = Info{
				data: betLog.data,
				id:   betLog.client.id,
				key:  betLog.result,
			}
			for client := range h.clients {
				if bytes.Compare(client.id, betLog.client.id) != 0 {
					select {
					case client.send2 <- info:
					default:
						close(client.send2)
						delete(h.clients, client)
					}
				}
			}
		}
	}
}
