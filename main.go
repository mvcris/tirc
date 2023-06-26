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

type HandleFunc func(msg Message)
type HandleCommandFunc func(msg CommandMessage)

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
	commands           []string
	connected          bool
	conn               *tls.Conn
	channels           map[string]bool
	authenticated      bool
	authCh             chan bool
	mw                 *sync.RWMutex
	msgCh              *Channels
	commandHandlers    map[string]HandleCommandFunc
}

type Channels struct {
	privCh *chanx.UnboundedChan[*Message]
	partCh *chanx.UnboundedChan[*Message]
	joinCh *chanx.UnboundedChan[*Message]
	cmdCh  *chanx.UnboundedChan[*CommandMessage]
}

func NewClient() *Client {
	channels := &Channels{
		privCh: chanx.NewUnboundedChan[*Message](10),
		partCh: chanx.NewUnboundedChan[*Message](10),
		joinCh: chanx.NewUnboundedChan[*Message](10),
		cmdCh:  chanx.NewUnboundedChan[*CommandMessage](10),
	}
	client := &Client{
		connected:       false,
		conn:            nil,
		authenticated:   false,
		channels:        make(map[string]bool),
		authCh:          make(chan bool),
		mw:              &sync.RWMutex{},
		commands:        make([]string, 0),
		msgCh:           channels,
		commandHandlers: make(map[string]HandleCommandFunc),
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
	err = c.Send("CAP REQ :twitch.tv/commands twitch.tv/tags")
	err = c.Send(fmt.Sprintf("PASS oauth:%s\r\n", token))
	err = c.Send(fmt.Sprintf("NICK %s\r\n", login))
	if err != nil {
		return err
	}
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

	for _, command := range c.commands {
		if command == prefix {
			return commandMsg
		}
	}

	return nil
}

func (c *Client) OnPrivMsg(maxParallelMessage int, handle HandleFunc) {
	for i := 0; i < maxParallelMessage; i++ {
		go func() {
			for msg := range c.msgCh.privCh.Out {
				handle(*msg)
			}
		}()
	}
}

func (c *Client) AddCommand(command string, handler HandleCommandFunc) {
	c.mw.Lock()
	c.commands = append(c.commands, command)
	c.commandHandlers[command] = handler
	c.mw.Unlock()
	fmt.Println(c.commandHandlers)
	go func() {
		for msg := range c.msgCh.cmdCh.Out {
			if canHandle := c.IsValidCommand(msg); canHandle {
				c.commandHandlers[msg.command](*msg)
			}
		}
	}()
}

func (c *Client) OnPart(maxParallelMessage int, handle HandleFunc) {
	for i := 0; i < maxParallelMessage; i++ {
		go func() {
			for msg := range c.msgCh.partCh.Out {
				handle(*msg)
			}
		}()
	}
}

func (c *Client) OnJoin(maxParallelMessage int, handle HandleFunc) {
	for i := 0; i < maxParallelMessage; i++ {
		go func() {
			for msg := range c.msgCh.joinCh.Out {
				handle(*msg)
			}
		}()
	}

}

func (c *Client) OnNotice(handle HandleFunc) {
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

func (c *Client) IsValidCommand(msg *CommandMessage) bool {
	for _, command := range c.commands {
		if msg.command == command {
			return true
		}
	}
	return false
}

func (c *Client) RegisterCommands(maxParallelMessage int, commands []string, handle HandleCommandFunc) {
	c.mw.Lock()
	c.commands = commands
	c.mw.Unlock()

	for i := 0; i < maxParallelMessage; i++ {
		go func() {
			for msg := range c.msgCh.cmdCh.Out {
				if canHandle := c.IsValidCommand(msg); canHandle {
					handle(*msg)
				}
			}
		}()
	}
}

func CheckMessageCommand(command string, msg *Message) bool {
	if msg.Command["command"] == command {
		return true
	}
	return false
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

func (c *Client) CloseMessageChannels() {
	close(c.msgCh.partCh.In)
	close(c.msgCh.privCh.In)
	close(c.msgCh.joinCh.In)
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
		if isCommand := strings.HasPrefix(msg.Parameters, "!"); isCommand {
			if cmdMsg := c.checkCommand(msg); cmdMsg != nil {
				c.msgCh.cmdCh.In <- cmdMsg
			}
		}
		c.msgCh.privCh.In <- msg
	case "CAP":
		c.connected = true
	case "PING":
		c.onPing(msg)
	case "376":
		fmt.Println(msg)
	case "001":
		c.authenticated = true
		c.authCh <- true
	case "JOIN":
		channel := msg.Command["channel"].(string)
		c.mw.RLock()
		c.channels[channel] = true
		c.mw.RUnlock()
		c.msgCh.joinCh.In <- msg
	case "PART":
		channel := msg.Command["channel"].(string)
		c.mw.RLock()
		delete(c.channels, channel)
		c.mw.RUnlock()
		c.msgCh.partCh.In <- msg
	}
}

func (c *Client) handleMessage() {
	defer c.conn.Close()
	defer c.CloseMessageChannels()
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
	client.OnPrivMsg(50, func(m Message) {
		// fmt.Println(m.Parameters)
	})
	client.OnPart(1, func(m Message) {
		fmt.Println(m)
	})
	client.OnJoin(1, func(m Message) {
		fmt.Println(m)
	})

	client.AddCommand("comando1", func(msg CommandMessage) {
		fmt.Println("comando 1")
	})
	client.AddCommand("comando2", func(msg CommandMessage) {
		fmt.Println("comando 2")
	})

	err := client.Auth("justinfan123456", "justinfan123456")
	if err != nil {
		fmt.Println(err)
		return
	}

	client.Join(
		"nulldemic",
		"kaicenat",
		"hasanabi",
		"fanum",
		"zordhacizco",
		"jonvlogs",
		"universoreality_live13",
		"moistcr1tikal",
		"pauleta_twitch",
		"elzeein",
		"rkdwl12",
		"inecr7024",
		"bakagaijinlive",
		"laagusneta",
		"bananirou",
		"nihmune",
		"thedaarick28",
		"mym_alkapone",
		"truenosurvivor",
		"jinu6734",
		"trielbaenre",
		"mangel",
		"meica05",
		"mira",
		"migi_tw",
		"snuffy",
		"ravshanbtw",
		"holi_nosoysofi",
		"manuuxo",
		"yayahuz",
		"sinder",
	)

	exit := make(chan bool)
	<-exit
}
