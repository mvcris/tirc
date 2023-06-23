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
	onPrivMsg     HandleFunc
	onConnected   HandleFunc
	onPart        HandleFunc
	onCommand     HandleFunc
	onJoin        HandleFunc
	commands      map[string]struct{}
	commandsFuncs map[string]HandleCommandFunc
	connected     bool
	conn          *tls.Conn
	channels      map[string]bool
	authenticated bool
	authCh        chan bool
	mw            *sync.RWMutex
}

func NewClient() *Client {
	client := &Client{
		connected:     false,
		conn:          nil,
		authenticated: false,
		channels:      make(map[string]bool),
		authCh:        make(chan bool),
		mw:            &sync.RWMutex{},
		commands:      make(map[string]struct{}),
		commandsFuncs: make(map[string]HandleCommandFunc),
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
	go c.handleMessage()
	//TODO: check error
	c.Send("CAP REQ :twitch.tv/commands twitch.tv/tags")
	c.Send(fmt.Sprintf("PASS oauth:%s\r\n", token))
	c.Send(fmt.Sprintf("NICK %s\r\n", login))

	//TODO: Improve AUTH check
	select {
	case <-c.authCh:
		break
	case <-time.After(time.Second * 5):
		return errors.New("auth error")
	}
	return nil
}

func (c *Client) checkCommand(msg *Message) {
	prefix := strings.Replace(msg.Parameters, "!", "", 1)
	commandMsg := &CommandMessage{msg, prefix, ""}

	// fmt.Println(prefix)
	if key := strings.Index(msg.Parameters, " "); key != -1 {
		prefix = msg.Parameters[1:key]
		commandMsg.source = strings.Replace(msg.Parameters[key:], " ", "", 1)
		commandMsg.command = prefix
	}

	fmt.Println(prefix)
	if _, ok := c.commands[prefix]; ok {
		fmt.Println(prefix)
		c.commandsFuncs[prefix](commandMsg)
	}
}

func (c *Client) OnPrivMsg(handle HandleFunc) {
	c.onPrivMsg = handle
}

func (c *Client) OnPart(handle HandleFunc) {
	c.onPart = handle
}

func (c *Client) OnJoin(handle HandleFunc) {
	c.onJoin = handle
}

func (c *Client) OnConnected(handle HandleFunc) {
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

func (c *Client) AddCommand(prefix string, handle HandleCommandFunc) {
	c.commands[prefix] = struct{}{}
	c.commandsFuncs[prefix] = handle
}

func (c *Client) OnCommand(prefix string, handle HandleCommandFunc) {
	if fun, ok := c.commandsFuncs[prefix]; ok && fun != nil {
		fun = handle
	}
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
	msg := parse(message)
	switch msg.Command["command"] {
	case "PRIVMSG":
		if c.onPrivMsg != nil {
			c.onPrivMsg(msg)
		}
		if isCommand := strings.HasPrefix(msg.Parameters, "!"); isCommand {
			c.checkCommand(msg)
		}

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
		c.onJoin(msg)
		c.mw.RLock()
		c.channels[channel] = true
		c.mw.RUnlock()
	case "PART":
		channel := msg.Command["channel"].(string)
		c.onPart(msg)
		c.mw.RLock()
		delete(c.channels, channel)
		c.mw.RUnlock()
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
	//Listen messages from IRC
	client.OnPrivMsg(func(m *Message) {

		// fmt.Printf("%+v\n", m.Parameters)
	})
	client.OnConnected(func(m *Message) {
		// bytes, _ := json.Marshal(m)
		// fmt.Println("connect")
		// fmt.Println(string(bytes))
	})
	client.OnPart(func(m *Message) {
		// fmt.Println("PART")
		// fmt.Println(m)
	})
	client.OnJoin(func(m *Message) {
		// fmt.Println("Join")
		// fmt.Println(m)
	})

	client.AddCommand("comando1", func(m *CommandMessage) {
		fmt.Printf("%+v\n", m)
	})
	client.AddCommand("comando2", func(m *CommandMessage) {
		fmt.Printf("comando 2: %s\n", m.source)
	})

	client.AddCommand("comando3", func(m *CommandMessage) {
		fmt.Printf("comando 3: %s\n", m.source)
	})

	err := client.Auth("justinfan123456", "justinfan123456")
	// client.Join("nulldemic", "kaicenat", "hasanabi", "zackrawrr")
	client.Join("nulldemic")
	if err != nil {
		fmt.Println(err)
		return
	}

	time.Sleep(time.Second * 5)
	// client.Part("nulldemic")
	// client.Part("mizkif")
	// msg := ":tmi.twitch.tv 001 justinfan123456 :Welcome, GLHF!"
	// fmt.Println(parse(msg))
	exit := make(chan bool)
	<-exit
}
