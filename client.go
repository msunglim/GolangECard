// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 20 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 4096
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub
	// The websocket connection.
	conn *websocket.Conn
	// Buffered channel of outbound messages.
	send1 chan Info
	send2 chan Info
	id    []byte
}

type socketInfo struct {
	Type string
	Key  string
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		message := socketInfo{}
		err := c.conn.ReadJSON(&message) //내가 주는입장.. 나의 정보를 모두에게
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error!!: %v", err)
			}
			break
		}

		switch message.Type {
		case "Join":
			sendObj := Info{
				data: []byte(message.Type),
				id:   c.id,
			}
			c.hub.join <- sendObj
			break
		case "CardSelect":
			keyNumber := message.Key[len(message.Key)-1 : len(message.Key)]
			keyStr := string(keyNumber)
			sendObj := Info{
				data: []byte(message.Type),
				id:   c.id,
				key:  []byte(keyStr),
			}
			c.hub.cardselect <- sendObj
			break
		case "Fight":
			keyNumber := message.Key
			var keyStr string
			if string(keyNumber) != "-1" {
				if string(keyNumber) == "0" {
					keyStr = "Kaiji"
				} else if string(keyNumber) == "1" {
					keyStr = "Tonegawa"
				}
				sendObj := Log{
					data:   []byte(message.Type),
					client: c,
					result: []byte(keyStr + " wins"),
				}
				c.hub.battle <- sendObj
			} else {
				sendObj := Log{
					data:   []byte(message.Type),
					client: nil,
					result: []byte("Draw"),
				}
				c.hub.battle <- sendObj
			}
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case info, ok := <-c.send1: //Join과 같이 나의 행동이 서버에서 나의 데이터를 받아서 내 html에 영향을 미칠때. 행위자의 정보가 행위자의 html에 적용된다..?
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			id := []byte(c.id) //받는 사람 c.id
			message := append(info.data, id...)
			w.Write(message)

			n := len(c.send1)
			for i := 0; i < n; i++ {
				w.Write(newline)
				iinfo := <-c.send1
				id := []byte(c.id)
				message := append(iinfo.data, id...)
				w.Write(message)
			}

			if err := w.Close(); err != nil {
				return
			}
		case info, ok := <-c.send2: //카드선택: 카드 주기와 같이 나의 행동이 상대에게만 영향을 미칠때. 행위자의 정보가 피행위자의 html에 적용된다.
			//싸움: 모두에게 싸움의 정보를 주기위해..
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			message := append(info.data, info.key...)
			w.Write(message)

			n := len(c.send2)
			for i := 0; i < n; i++ {
				w.Write(newline)
				iinfo := <-c.send2
				message := append(iinfo.data, iinfo.key...)
				w.Write(message)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

var count int = 0

// serveWs handles websocket requests from the peer.
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	//id random
	idc := []byte(strconv.Itoa(count % 2))
	client := &Client{hub: hub, conn: conn, send1: make(chan Info), send2: make(chan Info), id: idc}
	count++
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
