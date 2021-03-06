package main

import (
	"flag"
	"fmt"
	"io"
	"net"

	"github.com/couchbase/eventing/dcp/transport"
	"github.com/couchbase/eventing/dcp/transport/server"
	"github.com/couchbase/eventing/logging"
)

var port = flag.Int("port", 11212, "Port on which to listen")

type chanReq struct {
	req *transport.MCRequest
	res chan *transport.MCResponse
}

type reqHandler struct {
	ch chan chanReq
}

func (rh *reqHandler) HandleMessage(w io.Writer, req *transport.MCRequest) *transport.MCResponse {
	cr := chanReq{
		req,
		make(chan *transport.MCResponse),
	}

	rh.ch <- cr
	return <-cr.res
}

func connectionHandler(s net.Conn, h memcached.RequestHandler) {
	// Explicitly ignoring errors since they all result in the
	// client getting hung up on and many are common.
	_ = memcached.HandleIO(s, h)
}

func waitForConnections(ls net.Listener) {
	reqChannel := make(chan chanReq)

	go RunServer(reqChannel)
	handler := &reqHandler{reqChannel}

	logging.Warnf("Listening on port %d", *port)
	for {
		s, e := ls.Accept()
		if e == nil {
			logging.Warnf("Got a connection from %v", s.RemoteAddr())
			go connectionHandler(s, handler)
		} else {
			logging.Warnf("Error accepting from %s", ls)
		}
	}
}

func main() {
	flag.Parse()
	ls, e := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if e != nil {
		logging.Fatalf("Got an error:  %s", e)
	}

	waitForConnections(ls)
}
