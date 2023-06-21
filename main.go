package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net/textproto"
	"strings"
	"time"
)

type Message struct {
	Tags       map[string]any
	Source     map[string]string
	Command    map[string]any
	Parameters string
}

type Client struct {
	onPrivMsg     func(m *Message)
	connected     bool
	conn          *tls.Conn
	channels      map[string]bool
	authenticated bool
	writer        chan string
}

func NewClient() *Client {
	return &Client{
		connected:     false,
		conn:          nil,
		authenticated: false,
		channels:      make(map[string]bool),
		writer:        make(chan string),
	}
}

func Include(stack []string, find string) bool {
	for _, str := range stack {
		if str == find {
			return true
		}
	}
	return false
}

func (c *Client) Connect() error {
	conn, err := tls.Dial("tcp", "irc.chat.twitch.tv:6697", nil)
	if err != nil {
		return err
	}
	c.conn = conn
	c.connected = true
	go c.handleMessage()
	return nil
}

func (c *Client) Login() error {
	if c.connected {
		c.conn.Write([]byte("CAP REQ :twitch.tv/commands twitch.tv/tags\r\n"))
		c.conn.Write([]byte("PASS oauth:justinfan123456\r\n"))
		c.conn.Write([]byte("NICK justinfan123456\r\n"))
		return nil
	}
	return errors.New("not connected")
}

func (c *Client) OnPrevMsg(handle func(m *Message)) {
	c.onPrivMsg = handle
}

func (c *Client) Send(msg string) {
	if c.connected {
		c.writer <- msg
	}
}

func (c *Client) Join(channel string) error {
	if c.connected {
		c.conn.Write([]byte(fmt.Sprintf("JOIN #%s\r\n", channel)))
		return nil
	}
	return errors.New("not connected")
}

func (c *Client) parseMessage(message string) {
	parsedMessage := parse(message)
	if c.onPrivMsg != nil {
		c.onPrivMsg(parsedMessage)
	}
}

func (c *Client) handleMessage() {
	defer c.conn.Close()
	r := textproto.NewReader(bufio.NewReader(c.conn))
	for {
		raw, err := r.ReadLine()
		if err != nil {
			fmt.Println(err)
			break
		}
		messages := strings.Split(raw, "\r\n")

		for _, msg := range messages {
			c.parseMessage(msg)
		}
	}
}

func main() {
	client := NewClient()
	client.Connect()
	client.Login()
	// client.Join("xqc")
	// client.Join("illojuan")
	client.Join("nulldemic")
	client.OnPrevMsg(func(m *Message) {
		// fmt.Printf("%+v text: %s\n", m.Tags["emotes"], m.Parameters)
	})

	time.Sleep(time.Hour * 1)

}
