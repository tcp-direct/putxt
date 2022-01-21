package main

import (
	"github.com/rs/zerolog/log"

	termbin "git.tcp.direct/kayos/putxt"
)

func init() {
	termbin.UseChannel = true
}

func incoming() {
	var (
		msg      termbin.Message
		deflated []byte
		err      error
	)

	select {
	case msg = <-termbin.Msg:
		switch msg.Type {
		case termbin.Error:
			log.Error().
				Str("RemoteAddr", msg.RAddr).
				Int("Size", msg.Size).
				Msg(msg.Content)
		case termbin.IncomingData:
			log.Debug().
				Str("RemoteAddr", msg.RAddr).
				Int("Size", msg.Size).
				Msg("INCOMING_DATA")
		case termbin.Debug:
			log.Debug().
				Str("RemoteAddr", msg.RAddr).
				Int("Size", msg.Size).
				Msg(msg.Content)

		case termbin.Final:
			log.Info().
				Str("RemoteAddr", msg.RAddr).
				Int("Size", msg.Size).
				Msg(msg.Content)
			if termbin.Gzip {
				if deflated, err = termbin.Deflate(msg.Bytes); err != nil {
					log.Error().Err(err).Msg("DEFLATE_ERROR")
				}
				println(string(deflated))
			} else {
				println(string(msg.Bytes))
			}
		case termbin.Finish:
			break
		default:
			log.Fatal().Msg("invalid message")
		}
	}
}

func main() {
	if termbin.UseChannel {
		go func() {
			for {
				incoming()
			}
		}()
	}

	err := termbin.Listen("127.0.0.1", "8888")
	if err != nil {
		println(err.Error())
		return
	}
}
