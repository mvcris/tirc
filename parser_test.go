package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPingMessageParser(t *testing.T) {
	msg := "PING :tmi.twitch.tv"
	parsedMsg := parse(msg)
	assert.Equal(t, parsedMsg.Command["command"], "PING")
	assert.Equal(t, parsedMsg.Parameters, "tmi.twitch.tv")
}

func TestJoinMessage(t *testing.T) {
	msg := ":bar!bar@bar.tmi.twitch.tv JOIN #twitchdev"
	parsedMsg := parse(msg)
	assert.Equal(t, parsedMsg.Command["command"], "JOIN")
}

func TestPrivMessageParser(t *testing.T) {
	msg := "@badges=staff/1,broadcaster/1,turbo/1;color=#FF0000;display-name=PetsgomOO;emote-only=1;emotes=33:0-7;flags=0-7:A.6/P.6,25-36:A.1/I.2;id=c285c9ed-8b1b-4702-ae1c-c64d76cc74ef;mod=0;room-id=81046256;subscriber=0;turbo=0;tmi-sent-ts=1550868292494;user-id=81046256;user-type=staff :petsgomoo!petsgomoo@petsgomoo.tmi.twitch.tv PRIVMSG #petsgomoo :DansGame"
	parsedMsg := parse(msg)
	t.Log(parsedMsg.Source["nick"])
	assert.Equal(t, parsedMsg.Source["nick"], "petsgomoo")
	assert.Equal(t, parsedMsg.Source["host"], "petsgomoo@petsgomoo.tmi.twitch.tv")
	assert.Equal(t, parsedMsg.Command["command"], "PRIVMSG")
	assert.Equal(t, parsedMsg.Command["channel"], "#petsgomoo")
	assert.Equal(t, parsedMsg.Parameters, "DansGame")
	assert.Equal(t, parsedMsg.Tags["badges"].(map[string]any)["staff"], "1")
	assert.Equal(t, parsedMsg.Tags["color"], "#FF0000")
	assert.Equal(t, parsedMsg.Tags["emotes"].(map[string]any)["33"].([]map[string]string)[0]["startPosition"], "0")
}

func TestPrivMessageCommandParser(t *testing.T) {
	msg := ":lovingt3s!lovingt3s@lovingt3s.tmi.twitch.tv PRIVMSG #lovingt3s :!dilly"
	parsedMsg := parse(msg)
	assert.Equal(t, parsedMsg.Source["nick"], "lovingt3s")
	assert.Equal(t, parsedMsg.Parameters, "!dilly")
	assert.Equal(t, parsedMsg.Command["command"], "PRIVMSG")

	msgOnlyHost := ":onlyhostlovingt3s@lovingt3s.tmi.twitch.tv PRIVMSG #lovingt3s :!dilly"
	parsedMsg = parse(msgOnlyHost)
	assert.Equal(t, parsedMsg.Source["nick"], "")
	assert.Equal(t, parsedMsg.Source["host"], "onlyhostlovingt3s@lovingt3s.tmi.twitch.tv")
}

func TestParseTag(t *testing.T) {
	msg := "@badges= :petsgomoo!petsgomoo@petsgomoo.tmi.twitch.tv PRIVMSG #petsgomoo :DansGame"
	parsedMsg := parse(msg)
	assert.Equal(t, parsedMsg.Tags["badges"], nil)
}

func TestParsEmotes(t *testing.T) {
	msg := "@badges=staff/1,broadcaster/1,turbo/1;color=#FF0000;display-name=PetsgomOO;emote-only=1;emotes=;id=c285c9ed-8b1b-4702-ae1c-c64d76cc74ef;mod=0;room-id=81046256;subscriber=0;turbo=0;tmi-sent-ts=1550868292494;user-id=81046256;user-type=staff :petsgomoo!petsgomoo@petsgomoo.tmi.twitch.tv PRIVMSG #petsgomoo :DansGame"
	parsedMsg := parse(msg)
	assert.Nil(t, parsedMsg.Tags["emotes"])
	msg = "@badges=staff/1,broadcaster/1,turbo/1;color=#FF0000;display-name=PetsgomOO;emote-only=1;emote-sets=0,33,50,237;id=c285c9ed-8b1b-4702-ae1c-c64d76cc74ef;mod=0;room-id=81046256;subscriber=0;turbo=0;tmi-sent-ts=1550868292494;user-id=81046256;user-type=staff :petsgomoo!petsgomoo@petsgomoo.tmi.twitch.tv PRIVMSG #petsgomoo :DansGame"
	parsedMsg = parse(msg)
	assert.Equal(t, parsedMsg.Tags["emote-sets"].([]string)[0], "0")
	assert.Equal(t, parsedMsg.Tags["emote-sets"].([]string)[1], "33")

}
