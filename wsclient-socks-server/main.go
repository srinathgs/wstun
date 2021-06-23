package main

import (
	"context"
	"net"
	"time"

	"github.com/armon/go-socks5"
	log "github.com/sirupsen/logrus"
	"github.com/xtaci/smux/v2"
	"nhooyr.io/websocket"
)

func main() {
	ctx := context.Background()
	c, resp, err := websocket.Dial(ctx, "http://localhost:8080/ws", nil)
	log.SetLevel(log.DebugLevel)
	log.Infoln("resp", resp)
	log.Infoln(err)
	nc := websocket.NetConn(ctx, c, websocket.MessageBinary)
	sess, err := smux.Server(nc, &smux.Config{
		KeepAliveInterval: time.Duration(20 * time.Second),
		KeepAliveTimeout:  time.Duration(200 * time.Second),
		MaxFrameSize:      32768,
		MaxReceiveBuffer:  4194304,
		MaxStreamBuffer:   65536,
	})
	log.Infoln("Error getting session", err)
	ss, err := socks5.New(&socks5.Config{
		Credentials: socks5.StaticCredentials{
			"admin": "admin",
		},
	})
	log.Infoln(err)
	log.Error(err)
	for {
		stream, err := sess.AcceptStream()
		log.Infoln("Stream opened")
		log.Infoln(err)
		go func(str net.Conn) {
			log.Infoln("servinc new conn")
			ss.ServeConn(str)
			log.Infoln("Done Serving")
		}(stream)
	}
}
