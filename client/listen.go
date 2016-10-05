package client

import (
	"encoding/json"

	"github.com/CyCoreSystems/ari"
	"github.com/CyCoreSystems/ari-proxy/session"
	"github.com/nats-io/nats"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Handler defines a function which is called when a new dialog is created
type Handler func(cl *ari.Client, dialog *session.Dialog)

// Listen listens for an AppStart event and calls the handler when an event comes in
func Listen(ctx context.Context, conn *nats.Conn, appName string, h Handler) error {
	ch := make(chan *nats.Msg, 2)
	sub, err := conn.QueueSubscribeSyncWithChan("ari.app."+appName, appName+"_app_listener", ch)
	if err != nil {
		return errors.Wrap(err, "Unable to subscribe to ARI application start queue")
	}

	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			//TODO: log error
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			var appStart session.AppStart
			err := json.Unmarshal(msg.Data, &appStart)
			if err != nil {
				//TODO: log error
				go sendErrorReply(conn, msg.Reply, err)
				continue
			}

			go handler(conn, msg.Reply, appStart, h)
		}
	}
}

func sendErrorReply(conn *nats.Conn, reply string, err error) {
	// we got an error in the AppStart, reply with the error
	data := []byte(err.Error())
	if err := conn.Publish(reply, data); err != nil {
		//TODO: log error
	}
}

func handler(conn *nats.Conn, reply string, appStart session.AppStart, h Handler) {
	data := []byte("ok")
	if err := conn.Publish(reply, data); err != nil {
		//TODO: log error
		return
	}

	d := session.NewDialog(appStart.DialogID, nil)
	d.ChannelID = appStart.ChannelID

	cl, err := New(conn, appStart.Application, d, Options{})
	if err != nil {
		//TODO: log error
		return
	}

	go func() {
		conn.Subscribe("events.dialog."+d.ID, func(msg *nats.Msg) {
			var ariMessage ari.Message
			ariMessage.SetRaw(&msg.Data)
			cl.Bus.Send(&ariMessage)
		})
	}()

	h(cl, d)
}
