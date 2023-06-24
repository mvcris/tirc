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

	"github.com/smallnest/chanx"
)

type HandleFunc func(msg *Message)
type HandleCommandFunc func(msg *CommandMessage)

//TODO: REFACTOR STRUCTS Message | CommandMessage

type CommandMessage struct {
	msg     *Message
	command string
	source  string
}

type Message struct {
	Tags       map[string]any    `json:"tags,omitempty"`
	Source     map[string]string `json:"source,omitempty"`
	Command    map[string]any    `json:"command,omitempty"`
	Parameters string            `json:"parameters,omitempty"`
}

type Client struct {
	MaxParallelMessage int32
	commands           map[string]struct{}
	connected          bool
	conn               *tls.Conn
	channels           map[string]bool
	authenticated      bool
	authCh             chan bool
	mw                 *sync.RWMutex
	privCh             chan struct{}
	ch                 *chanx.UnboundedChan[*Message]
}

type ClientConfig struct {
	MaxParallelMessage int32
}

func NewClient(config *ClientConfig) *Client {
	var maxParallel int32 = 5

	if config != nil {
		maxParallel = config.MaxParallelMessage
	}

	client := &Client{
		connected:          false,
		conn:               nil,
		authenticated:      false,
		channels:           make(map[string]bool),
		authCh:             make(chan bool),
		mw:                 &sync.RWMutex{},
		commands:           make(map[string]struct{}),
		ch:                 chanx.NewUnboundedChan[*Message](10),
		MaxParallelMessage: maxParallel,
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
	//TODO: check error
	c.Send("CAP REQ :twitch.tv/commands twitch.tv/tags")
	c.Send(fmt.Sprintf("PASS oauth:%s\r\n", token))
	c.Send(fmt.Sprintf("NICK %s\r\n", login))
	go c.handleMessage()
	//TODO: Improve AUTH check
	select {
	case <-c.authCh:
		close(c.authCh)
		break
	case <-time.After(time.Second * 5):
		close(c.authCh)
		return errors.New("auth error")
	}
	return nil
}

func (c *Client) checkCommand(msg *Message) *CommandMessage {
	prefix := strings.Replace(msg.Parameters, "!", "", 1)
	commandMsg := &CommandMessage{msg, prefix, ""}

	if key := strings.Index(msg.Parameters, " "); key != -1 {
		prefix = msg.Parameters[1:key]
		commandMsg.source = strings.Replace(msg.Parameters[key:], " ", "", 1)
		commandMsg.command = prefix
	}

	if _, ok := c.commands[prefix]; ok {
		return commandMsg
	}

	return nil
}

func (c *Client) OnPrivMsg(handle HandleFunc) {
	for i := 0; i < int(c.MaxParallelMessage); i++ {
		go func() {
			for msg := range c.ch.Out {
				handle(msg)
			}
		}()
	}
}

func (c *Client) OnPart(handle HandleFunc) {
}

func (c *Client) OnJoin(handle HandleFunc) {
}

func (c *Client) OnConnected(handle HandleFunc) {
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

func (c *Client) AddCommand(prefix string, handle HandleCommandFunc) {
	c.commands[prefix] = struct{}{}
	go func() {
		for msg := range c.ch.Out {
			if command := c.checkCommand(msg); command != nil {
				handle(command)
			}
		}
	}()
}

func (c *Client) Join(channels ...string) error {
	if c.connected {
		c.mw.Lock()
		defer c.mw.Unlock()
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
	//TODO: PING RESPONSE ERROR??? MB RETRY CNNCT
	c.Send(fmt.Sprintf("PONG %s", msg.Parameters))
	fmt.Println("PONG WORKS")
}

func (c *Client) Part(channel string) error {
	if c.connected {
		c.mw.Lock()
		defer c.mw.Unlock()
		//TODO: check error
		c.Send(fmt.Sprintf("PART #%s", channel))
		delete(c.channels, fmt.Sprintf("#%s", channel))
	}
	return errors.New("not connected")
}

func (c *Client) parseMessage(message string) {

	//TODO: add msgs to a queue
	msg := parse(message)
	switch msg.Command["command"] {
	case "PRIVMSG":
		c.ch.In <- msg
		if isCommand := strings.HasPrefix(msg.Parameters, "!"); isCommand {
			c.checkCommand(msg)
		}

	case "CAP":
		c.connected = true
	case "PING":
		c.onPing(msg)
	case "001":
		c.authenticated = true
		c.authCh <- true
		fmt.Println("conectado")
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

func (c *Client) handleMessage() {
	defer c.conn.Close()
	defer close(c.ch.In)
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
	client := NewClient(&ClientConfig{MaxParallelMessage: 1})
	//Listen messages from IRC
	client.OnPrivMsg(func(m *Message) {
		time.Sleep(time.Second * 5)
		fmt.Printf("%+v\n", m.Parameters)
	})
	// client.OnConnected(func(m *Message) {
	// 	fmt.Println("finish connection")
	// 	// bytes, _ := json.Marshal(m)
	// 	// fmt.Println("connect")
	// 	// fmt.Println(string(bytes))
	// })
	// client.OnPart(func(m *Message) {
	// 	// fmt.Println("PART")
	// 	// fmt.Println(m)
	// })
	// client.OnJoin(func(m *Message) {
	// 	// fmt.Println("Join")
	// 	// fmt.Println(m)
	// })

	client.AddCommand("comando1", func(m *CommandMessage) {
		fmt.Printf("%+v\n", m)
	})
	client.AddCommand("comando2", func(m *CommandMessage) {
		fmt.Printf("%+v\n", m)
	})

	err := client.Auth("justinfan123456", "justinfan123456")
	client.Join("sodapoppin", "jesusavgn")
	client.Join("nulldemic")
	if err != nil {
		fmt.Println(err)
		return
	}

	// client.Part("mizkif")
	// msg := ":tmi.twitch.tv 001 justinfan123456 :Welcome, GLHF!"
	// fmt.Println(parse(msg))
	// time.Sleep(time.Second * 10)
	exit := make(chan bool)
	<-exit
}
