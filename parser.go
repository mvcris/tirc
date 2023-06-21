package main

import (
	"fmt"
	"strings"
)

func parseCommand(raw string) map[string]any {

	commandParts := strings.Split(raw, " ")
	parsedCommand := make(map[string]any)
	switch commandParts[0] {
	case "JOIN", "PART", "NOTICE", "CLEARCHAT", "HOSTTARGET", "PRIVMSG":
		parsedCommand["command"] = commandParts[0]
		parsedCommand["channel"] = commandParts[1]
	case "PING":
		parsedCommand["command"] = commandParts[0]
	case "CAP":
		capReqEnabled := false
		if commandParts[2] == "ACK" {
			capReqEnabled = true
		}
		parsedCommand["command"] = commandParts[0]
		parsedCommand["isCapRequestEnable"] = capReqEnabled
	case "GLOBALUSERSTATE":
		parsedCommand["command"] = commandParts[0]
	case "USERSTATE", "ROOMSTATE":
		parsedCommand["command"] = commandParts[0]
		parsedCommand["channel"] = commandParts[1]
	}
	return parsedCommand
}

func parseTag(tags string) map[string]any {
	dictParsedTags := make(map[string]any)
	parsedTags := strings.Split(tags, ";")
	for _, tag := range parsedTags {
		parsedTag := strings.Split(tag, "=")
		tagValue := ""

		if parsedTag[1] != "" {
			tagValue = parsedTag[1]
		}

		switch parsedTag[0] {
		case "badges", "badge-info":
			if tagValue != "" {
				dict := make(map[string]any)
				badges := strings.Split(tagValue, ",")
				for _, pair := range badges {
					badgeParts := strings.Split(pair, "/")
					dict[badgeParts[0]] = badgeParts[1]
				}
				dictParsedTags[parsedTag[0]] = dict
			} else {
				dictParsedTags[parsedTag[0]] = nil
			}
		case "emotes":
			if tagValue != "" {
				dictEmotes := make(map[string]any)
				emotes := strings.Split(tagValue, "/")
				for _, emote := range emotes {
					emoteParts := strings.Split(emote, ":")
					positions := strings.Split(emoteParts[1], ",")
					allPosition := make([]map[string]string, len(positions))
					for _, position := range positions {
						positionParts := strings.Split(position, "-")
						textPosition := make(map[string]string)
						textPosition["startPosition"] = positionParts[0]
						textPosition["endPosition"] = positionParts[1]
						allPosition = append(allPosition, textPosition)
					}
					dictEmotes[emoteParts[0]] = allPosition
				}
				dictParsedTags[parsedTag[0]] = dictEmotes
			} else {
				dictParsedTags[parsedTag[0]] = nil
			}
		case "emote-sets":
			emoteSetIds := strings.Split(tagValue, ",")
			dictParsedTags[parsedTag[0]] = emoteSetIds
		default:
			if !Include([]string{"client-nonce", "flags"}, parsedTag[0]) {
				dictParsedTags[parsedTag[0]] = tagValue
			}

		}
	}
	return dictParsedTags
}

func parseSource(raw string) map[string]string {
	nick := ""
	host := ""
	sourceParse := make(map[string]string)
	if raw != "" {
		sourceParts := strings.Split(raw, "!")
		if len(sourceParts) == 2 {
			nick = sourceParts[0]
			host = sourceParts[1]
		} else {
			host = sourceParts[0]
		}
	}
	sourceParse["nick"] = nick
	sourceParse["host"] = host
	return sourceParse
}

func parse(message string) *Message {
	idx := 0
	parsedMessage := &Message{}

	if strings.HasPrefix(message, "@") {
		endIdx := strings.Index(message, " ")
		rawTagsComponent := message[1:endIdx]
		parsedMessage.Tags = parseTag(rawTagsComponent)
		idx = endIdx + 1
	}

	if string(message[idx]) == ":" {
		idx++
		endIdx := strings.Index(message[idx:], " ") + idx
		rawSourceComponent := message[idx:endIdx]
		parsedMessage.Source = parseSource(rawSourceComponent)
		idx = endIdx + 1
	}

	endIdx := strings.Index(message[idx:], ":")
	if endIdx == -1 {
		endIdx = len(message)
	} else {
		endIdx += idx
	}

	rawCommandComponent := message[idx:endIdx]
	parsedMessage.Command = parseCommand(rawCommandComponent)

	if endIdx != len(message) {
		idx = endIdx + 1
		parsedMessage.Parameters = message[idx:]
	}
	return parsedMessage
}
