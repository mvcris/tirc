package tirc

import (
	"bufio"
	"context"
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

type ClientConfig struct {
	Nick      string
	Token     string
	Reconnect bool
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
	config             ClientConfig
	ticker             *time.Ticker
	ctx                context.Context
	cancel             context.CancelFunc
}

type Channels struct {
	privCh *chanx.UnboundedChan[*Message]
	partCh *chanx.UnboundedChan[*Message]
	joinCh *chanx.UnboundedChan[*Message]
	cmdCh  *chanx.UnboundedChan[*CommandMessage]
}

func NewClient(config ClientConfig) (*Client, error) {
	channels := &Channels{
		privCh: chanx.NewUnboundedChan[*Message](10),
		partCh: chanx.NewUnboundedChan[*Message](10),
		joinCh: chanx.NewUnboundedChan[*Message](10),
		cmdCh:  chanx.NewUnboundedChan[*CommandMessage](10),
	}

	if len(config.Nick) <= 0 {
		return nil, errors.New("nick not provided")
	}

	if len(config.Token) <= 0 {
		return nil, errors.New("token not provided")
	}
	ctx, cancel := context.WithCancel(context.Background())

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
		config:          config,
		ticker:          time.NewTicker(time.Second * 5),
		ctx:             ctx,
		cancel:          cancel,
	}
	return client, nil
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

func (c *Client) Start() error {
	err := c.Connect()
	if err != nil {
		return err
	}
	c.connected = true
	err = c.Send("CAP REQ :twitch.tv/commands twitch.tv/tags")
	err = c.Send(fmt.Sprintf("PASS oauth:%s\r\n", c.config.Token))
	err = c.Send(fmt.Sprintf("NICK %s\r\n", c.config.Nick))
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) checkConnection() {
Loop:
	for {
		select {
		case <-c.ticker.C:
			err := c.Send("PING :tmi.twitch.tv")
			if err != nil {
				c.ticker.Stop()
				c.cancel()
			}
		case <-c.ctx.Done():
			break Loop
		}
	}
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

func (c *Client) OnPrivMsg(handle HandleFunc) {
	go func() {
		for msg := range c.msgCh.privCh.Out {
			handle(*msg)
		}
	}()
}

func (c *Client) AddCommand(command string, handler HandleCommandFunc) {
	c.mw.Lock()
	c.commands = append(c.commands, command)
	c.commandHandlers[command] = handler
	c.mw.Unlock()
	go func() {
		for msg := range c.msgCh.cmdCh.Out {
			if canHandle := c.IsValidCommand(msg); canHandle {
				c.commandHandlers[msg.command](*msg)
			}
		}
	}()
}

func (c *Client) OnPart(handle HandleFunc) {
	go func() {
		for msg := range c.msgCh.partCh.Out {
			handle(*msg)
		}
	}()
}

func (c *Client) OnJoin(handle HandleFunc) {
	go func() {
		for msg := range c.msgCh.joinCh.Out {
			handle(*msg)
		}
	}()

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
	c.Send(fmt.Sprintf("PONG %s", msg.Parameters))
}

func (c *Client) Part(channel string) error {
	if c.connected {
		c.mw.Lock()
		defer c.mw.Unlock()
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

func (c *Client) Watch() error {
	go c.handleMessage()
	go c.checkConnection()
	select {
	case <-c.authCh:
		close(c.authCh)
		break
	case <-time.After(time.Second * 5):
		close(c.authCh)
		return errors.New("auth error")
	}
	// return nil

	select {
	case <-c.ctx.Done():
		return errors.New("something wrong =(")
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