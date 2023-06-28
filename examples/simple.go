package main

import (
	"fmt"
	"github.com/mvcris/tirc"
)

func main() {
	//anonymous login
	client, err := tirc.NewClient(tirc.ClientConfig{Nick: "justinfan123456", Token: "justinfan123456", Reconnect: true})
	if err != nil {
		panic(err)
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
	client.OnPrivMsg(func(m tirc.Message) {
		fmt.Println(m.Parameters)
	})
	client.OnPart(func(m tirc.Message) {
		fmt.Println(m)
	})
	client.OnJoin(func(m tirc.Message) {
		fmt.Println(m)
	})

	err = client.Run()

}
