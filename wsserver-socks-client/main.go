package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
	"github.com/xtaci/smux/v2"
	"nhooyr.io/websocket"
)

func wsHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:       nil,
		InsecureSkipVerify: true,
	})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		c.Close(websocket.CloseStatus(err), "Unable to listen for a proxy port")
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "Going down")
	defer l.Close()
	log.Info("listener started", l.Addr())
	ctx := r.Context()
	ctx, cfun := context.WithCancel(ctx)
	nc := websocket.NetConn(ctx, c, websocket.MessageBinary)
	sess, err := smux.Client(nc, &smux.Config{
		KeepAliveInterval: time.Duration(20 * time.Second),
		KeepAliveTimeout:  time.Duration(200 * time.Second),
		MaxFrameSize:      32768,
		MaxReceiveBuffer:  4194304,
		MaxStreamBuffer:   65536,
	})
	if err != nil {
		nc.Close()
		cfun()
		return
	}
	for {
		select {
		case <-ctx.Done():
			{
				break
			}
		default:
			{
				conn, err := l.Accept()
				log.Errorln("Accept err", err)
				if err != nil {
					continue
				}
				go func(conn net.Conn) {
					tcon, err := sess.OpenStream()
					if err != nil {
						log.Errorf("Error occured: cannot open conn over smux", err)
						cfun()
						return
					}
					var wg sync.WaitGroup
					copyIO := func(dst, src net.Conn, wg sync.WaitGroup) {
						log.Infoln("Copying between", dst, src)
						n, err := io.Copy(dst, src)
						log.Infoln("Copied", n)
						log.Errorln("err in copy", err)
						wg.Done()
					}
					wg.Add(1)
					go copyIO(conn, tcon, wg)
					wg.Add(1)
					go copyIO(tcon, conn, wg)
					wg.Wait()
				}(conn)
			}
		}
	}
}

func main() {
	r := httprouter.New()
	r.GET("/ws", wsHandler)
	log.SetLevel(log.DebugLevel)
	http.ListenAndServe("0.0.0.0:8080", r)
}
