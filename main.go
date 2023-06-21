package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net/textproto"
	"strings"
	"sync"
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
	onConnected   func(m *Message)
	connected     bool
	conn          *tls.Conn
	channels      map[string]bool
	authenticated bool
	actions       chan *Message
	capCh         chan bool
	authCh        chan bool
	nickCh        chan bool
	mw            *sync.RWMutex
}

func NewClient() *Client {
	client := &Client{
		connected:     false,
		conn:          nil,
		authenticated: false,
		channels:      make(map[string]bool),
		actions:       make(chan *Message),
		authCh:        make(chan bool),
		mw:            &sync.RWMutex{},
	}
	return client
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
	return nil
}

func (c *Client) Auth(login string, token string) error {
	err := c.Connect()
	if err != nil {
		return err
	}
	c.connected = true
	go c.listenActions()
	go c.handleMessage()
	c.conn.Write([]byte("CAP REQ :twitch.tv/commands twitch.tv/tags\r\n"))
	c.conn.Write([]byte(fmt.Sprintf("PASS oauth:%s\r\n", token)))
	c.conn.Write([]byte(fmt.Sprintf("NICK %s\r\n", login)))
	//TODO: Improve AUTH check
	select {
	case <-c.authCh:
		break
	case <-time.After(time.Second * 5):
		return errors.New("auth error")
	}
	return nil
}

func (c *Client) OnPrevMsg(handle func(m *Message)) {
	c.mw.RLock()
	defer c.mw.RUnlock()
	c.onPrivMsg = handle
}

func (c *Client) OnConnected(handle func(m *Message)) {
	c.onConnected = handle
}

func (c *Client) Send(msg string) error {
	if c.connected {
		_, err := c.conn.Write([]byte(fmt.Sprintf("%s\r\n", msg)))
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Join(channels ...string) error {
	if c.connected {
		for _, channel := range channels {
			err := c.Send(fmt.Sprintf("JOIN #%s", channel))
			if err != nil {
				return err
			}
			c.channels[fmt.Sprintf("#%s", channel)] = false
		}
		return nil
	}
	return errors.New("not connected")
}

func (c *Client) onSuccessJoin(message *Message) {

}

func (c *Client) onPing(msg *Message) {
	//TODO: ON PING RESPONSE ERROR??? MB RETRY CNNCT
	c.Send(fmt.Sprintf("PONG %s", msg.Parameters))
}

func (c *Client) Part(channel string) error {
	defer c.mw.RUnlock()
	if c.connected {
		c.mw.RLock()
		c.conn.Write([]byte(fmt.Sprintf("PART #%s\r\n", channel)))
		delete(c.channels, fmt.Sprintf("#%s", channel))
	}
	return errors.New("not connected")
}

func (c *Client) listenActions() {
	for {
		select {
		case msg := <-c.actions:
			switch msg.Command["command"] {
			case "CAP":
				c.connected = true
				if c.onConnected != nil {
					c.onConnected(msg)
				}
			case "PING":
				c.onPing(msg)
			case "001":
				c.authenticated = true
				c.authCh <- true
			case "JOIN":

				channel := msg.Command["channel"].(string)
				c.mw.RLock()
				c.channels[channel] = true
				c.mw.RUnlock()
			case "PART":
				channel := msg.Command["channel"].(string)
				c.mw.RLock()
				delete(c.channels, channel)
				c.mw.RUnlock()
			}
		}
	}
}

func (c *Client) parseMessage(message string) {
	parsedMessage := parse(message)
	c.actions <- parsedMessage
	c.mw.Lock()
	defer c.mw.Unlock()
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
	err := client.Auth("justinfan123456", "justinfan123456")
	if err != nil {
		fmt.Println(err)
		return
	}
	client.Join("juansguarnizo", "mizkif")
	client.OnPrevMsg(func(m *Message) {
		fmt.Printf("%+v\n", m.Parameters)
	})
	client.OnConnected(func(m *Message) {
		fmt.Println(m)
	})
	// client.Part("mizkif")
	// msg := ":tmi.twitch.tv 001 justinfan123456 :Welcome, GLHF!"
	// fmt.Println(parse(msg))
	exit := make(chan bool)
	<-exit
}
