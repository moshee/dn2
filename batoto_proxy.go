// This file implements a proxy server that will allow a client to upload files
// to a remote server. This is useful if the client does not necessarily have
// the open port 80 required to start an HTTP server locally. No buffering
// happens here; the request body from the client is copied directly into the
// response to the remote server requesting the file.

package main

import (
	"errors"
	"io"
	"strconv"

	"ktkr.us/pkg/gas"
	"ktkr.us/pkg/gas/out"
)

// The actual file contents to be proxied through the server
type proxiedFile struct {
	io.ReadCloser
	Length int64
}

type proxyRequest struct {
	Filename string
	Pipe     proxyPipe
}

type proxyPipe struct {
	File chan chan *proxiedFile
	Errs chan chan error
}

var proxyRequests = make(chan *proxyRequest)

// proxyListener listens for proxy file requests on the channel. It will
// connect a client and a server, who, arriving one by one, either leave a
// channel behind, or recieve a channel with which to send or recieve on. In
// either case, one will be expecting some place to send/recieve to/from and
// the other will be giving.
func proxyListener() {
	// A pool of concurrent proxy requests. Doesn't have to be threadsafe
	// because it's protected by a single channel recieve.
	activeFiles := make(map[string]chan *proxiedFile)

	for {
		req := <-proxyRequests
		if pipe, active := activeFiles[req.Filename]; active {
			// If the one sending the pipe is a client, then the server got
			// here first. Send the pipe that the server left behind so that
			// the client can send the file to the server.
			// If it was a server, then the client got here first. Send the
			// server the pipe so that it can recieve the file and start
			// sending it to the remote end.
			//
			// In either case, this case means the negotiation was successful.
			// Delete the transaction from the pool.
			req.Pipe.File <- pipe
			delete(activeFiles, req.Filename)
		} else {
			// If the one sending the pipe is a client, then we're storing the
			// pipe so that the server can recieve the file from it later. The
			// client was first in this case.
			// If it was a server, that means the server got here first. We're
			// storing the pipe here so that the client can send on it when it
			// arrives.
			pipe = make(chan *proxiedFile)
			activeFiles[req.Filename] = pipe
			req.Pipe.File <- pipe
		}
	}
}

// The client will send the file as the request body, with the filename in the
// headers, for simplicity. To avoid using multipart. The filename is for
// negotiating which channel pipes to send to which handlers in case there are
// multiple proxies running concurrently. Although this is very unlikely in the
// specific use case in mind, what the hell.
func postProxyClient(g *gas.Gas) (int, gas.Outputter) {
	req := &proxyRequest{
		Filename: g.Request.Header.Get("X-Proxied-Filename"),
		Pipe: proxyPipe{
			make(chan chan *proxiedFile),
			make(chan chan error),
		},
	}
	proxyRequests <- req

	// send on the pipe channel recieved from the server
	(<-req.Pipe.File) <- &proxiedFile{
		ReadCloser: g.Body,
		Length:     g.Request.ContentLength,
	}

	errChan := <-req.Pipe.Errs
	err := <-errChan
	if err != nil {
		return 500, out.Error(g, err)
	}

	return 204, nil
}

func getProxyServer(g *gas.Gas) (int, gas.Outputter) {
	errs := make(chan chan error, 1)

	// store the error channel in the error pipe channel's buffer. The client
	// will probably pick it up later.
	errChan := make(chan error)
	errs <- errChan

	req := &proxyRequest{
		Filename: g.Arg("filename"),
		Pipe: proxyPipe{
			make(chan chan *proxiedFile),
			errs,
		},
	}

	proxyRequests <- req

	// recieve from the pipe channel recieved from the client
	file := <-<-req.Pipe.File
	if file.ReadCloser == nil {
		errChan <- errors.New("the file content is somehow nil")
		return 500, nil
	}

	defer file.Close()

	// lol
	g.Header().Set("Content-Type", "application/zip")
	g.Header().Set("Content-Length", strconv.FormatInt(file.Length, 10))

	_, err := io.Copy(g, file)
	if err != nil && err != io.EOF {
		errChan <- err
	}

	return -1, nil
}
