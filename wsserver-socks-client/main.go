package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
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
	r.ParseForm()
	log.Infoln("Form is,", r.Form)
	portF := r.Form.Get("port")
	port, err := strconv.Atoi(portF)
	if err != nil {
		port = 0
		log.Errorln("Err for converting value:", portF)
	}
	l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
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
		MaxFrameSize:      32768 / 2,
		MaxReceiveBuffer:  4194304,
		MaxStreamBuffer:   32768,
	})
	if err != nil {
		nc.Close()
		cfun()
		return
	}
	var ctr int
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
				ctr += 1
				go func(pconn net.Conn, ct int) {
					tcon, err := sess.OpenStream()
					if err != nil {
						log.Errorf("Error occured: cannot open conn over smux", err)
						cfun()
						return
					}
					var wg sync.WaitGroup
					copyIO := func(dst, src net.Conn, wg sync.WaitGroup, grp string) {
						log.Infoln("Copying between", grp, dst, src)
						n, err := io.Copy(dst, src)
						log.Infoln("Copied", n)
						log.Errorln("err in copy", grp, err)
						wg.Done()
					}
					wg.Add(1)
					go copyIO(pconn, tcon, wg, fmt.Sprintf("pc->tc %d", ct))
					wg.Add(1)
					go copyIO(tcon, pconn, wg, fmt.Sprintf("tc->pc %d", ct))
					wg.Wait()
				}(conn, ctr)
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
