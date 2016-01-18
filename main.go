package main

import (
	"flag"
	"net/http"
	"log"
	"github.com/gorilla/websocket"
	"time"
	"errors"
)


var addr = flag.String("addr", ":8080", "http service address")

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Room struct {
	Join		chan *Client
	Closed		chan *Client
	RecvFromClient	chan *ClientRecv
	RecvFromGate	chan string
	Purge		chan bool
	Stop		chan bool
	clients		map[*Client]bool
}

type Client struct {
	uuid		string
	closed		chan *Client
	recv		chan *ClientRecv
	send		chan string
	stop		chan bool
	conn		*websocket.Conn
}

type ClientRecv struct {
	client		*Client
	message		string
}

func newRoom() *Room {
	r := &Room{
		Join:		make(chan *Client),
		Closed:		make(chan *Client),
		RecvFromClient:	make(chan *ClientRecv),
		RecvFromGate:	make(chan string),
		Purge:		make(chan bool),
		Stop:		make(chan bool),
		clients:	make(map[*Client]bool),
	}
	go r.run()
	return r
}


func (r *Room) run() {
	defer log.Println("Room closed.")
	for {
		select {
			case c := <-r.Join:
				log.Printf("Client: %v is joined", c)
				r.clients[c] = true
			case cr := <-r.RecvFromClient:
				for c := range r.clients {
					if err := c.Send(cr.message); err != nil {
						log.Println(err)
						c.Stop()
						delete(r.clients, c)
					}
				}
				log.Printf("Room: RecvFromClient %#v", cr)
			case c := <-r.Closed:
				delete(r.clients, c)
			case <-r.Purge:
				r.purge()
			case <-r.Stop:
				r.purge()
				return
		}
	}
}


// Closes client.
func (c *Client) Stop() {
	defer func() {
		if r := recover(); r != nil {
			log.Println(r)
		}
	}()
	close(c.stop)
}


func (r *Room) purge() {
	for c := range r.clients {
		c.Stop()
		delete(r.clients, c)
	}
}

// Send msg to the client.
func (c *Client) Send(message string) error {
	select {
	case c.send <- message:
		return nil
	case <-time.After(time.Millisecond * 10):
		return errors.New("Can't send to client")
	}
}

func (c *Client) sender() {
	defer func() {
		log.Printf("Client %v is closed", c)
		c.closed <- c
	}()
	for {
		select {
			case <-c.stop:
				return
			case msg := <-c.send:
				if err := c.conn.WriteMessage(1, []byte(msg)); err != nil {
					log.Println("sender", err)
					return
				}
		}
	}
}


func (c *Client) receiver() {
	defer func() {
		log.Printf("Client %v is closed", c)
		c.closed <- c
	}()
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Println("receiver", err)
			return
		}
		c.recv <- &ClientRecv{c, string(message)}
	}
}


var room = newRoom();


func handlerWsConn(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("handlerWsConn:", err)
		return
	}
	c := &Client{
		closed:		room.Closed,
		recv:		room.RecvFromClient,
		send:		make(chan string),
		stop:		make(chan bool),
		conn:		conn,
		uuid:		"",
	}
	room.Join <- c
	go c.sender()
	go c.receiver()
	log.Printf("handlerWsConn client %v is created", c)
}


func servertStat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte("Working..."))
}


func main() {
	flag.Parse()
	http.HandleFunc("/stat", servertStat)
	http.HandleFunc("/ws", handlerWsConn)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
