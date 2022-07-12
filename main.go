package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"sync"
	"syscall"

	"github.com/google/uuid"

	"github.com/aler9/gortsplib"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/liberrors"
)

type handler struct {
	url     string
	mu      sync.RWMutex
	wg      sync.WaitGroup
	stream  *gortsplib.ServerStream
	session *gortsplib.ServerSession
}

// OnConnOpen called when a connection is opened.
func (h *handler) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	log.Printf("conn opened %v", ctx)
}

// OnConnClose called when a connection is closed.
func (h *handler) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	log.Printf("conn closed %v", ctx)
}

// OnSessionOpen called when a session is opened.
func (h *handler) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	log.Printf("session opened %v", ctx)
}

// OnSessionClose called when a session is closed.
func (h *handler) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	log.Printf("session closed %v", ctx)

	h.mu.Lock()
	defer h.mu.Unlock()

	// close stream-related listeners and disconnect every reader
	if h.stream != nil && ctx.Session == h.session {
		_ = h.stream.Close()
		h.stream = nil
		h.session = nil
	}
}

// OnDescribe called after receiving a DESCRIBE request.
func (h *handler) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	log.Printf("%24v %v %v", ctx.Conn.NetConn().RemoteAddr(), ctx.Request.Method, ctx.Request.URL)

	h.mu.RLock()
	defer h.mu.RUnlock()

	// no one is publishing yet
	if h.stream == nil {
		return &base.Response{
			StatusCode: base.StatusNotFound,
		}, nil, nil
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, h.stream, nil
}

// OnAnnounce called after receiving an ANNOUNCE request.
func (h *handler) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	log.Printf("%24v %v %v", ctx.Conn.NetConn().RemoteAddr(), ctx.Request.Method, ctx.Request.URL)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.stream != nil {
		return &base.Response{
			StatusCode: base.StatusBadRequest,
		}, fmt.Errorf("someone is already publishing")
	}

	h.wg.Add(1)

	from := fmt.Sprintf("rtsp://%v/%v", ctx.Conn.NetConn().LocalAddr(), ctx.Path)
	file := fmt.Sprintf("%v", uuid.New())

	go func(url, file string) {
		defer h.wg.Done()
		arg := []string{"ffmpeg", "-i", url, "-c", "copy", "-y", "-f", FORMAT, file}
		cmd := &exec.Cmd{
			Args: arg,
			Path: "/usr/bin/ffmpeg",
			SysProcAttr: &syscall.SysProcAttr{
				Setpgid: true,
			},
		}
		log.Println("EXEC", arg)
		log.Println("DONE", arg, cmd.Run())
	}(from, path.Join(h.url, file))

	h.stream = gortsplib.NewServerStream(ctx.Tracks)
	h.session = ctx.Session

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnSetup called after receiving a SETUP request.
func (h *handler) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	log.Printf("%24v %v %v", ctx.Conn.NetConn().RemoteAddr(), ctx.Request.Method, ctx.Request.URL)

	h.mu.RLock()
	defer h.mu.RUnlock()

	// no one is publishing yet
	if h.stream == nil {
		return &base.Response{
			StatusCode: base.StatusNotFound,
		}, nil, nil
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, h.stream, nil
}

// OnPlay called after receiving a PLAY request.
func (h *handler) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	log.Printf("%24v %v %v", ctx.Conn.NetConn().RemoteAddr(), ctx.Request.Method, ctx.Request.URL)

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnRecord called after receiving a RECORD request.
func (h *handler) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	log.Printf("%24v %v %v", ctx.Conn.NetConn().RemoteAddr(), ctx.Request.Method, ctx.Request.URL)

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnPause called after receiving a PAUSE request.
func (h *handler) OnPause(ctx *gortsplib.ServerHandlerOnPauseCtx) (*base.Response, error) {
	log.Printf("%24v %v %v", ctx.Conn.NetConn().RemoteAddr(), ctx.Request.Method, ctx.Request.URL)

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// OnPacketRTP called after receiving an RTP packet.
func (h *handler) OnPacketRTP(ctx *gortsplib.ServerHandlerOnPacketRTPCtx) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// if we are the publisher, route the RTP packet to readers
	if ctx.Session == h.session {
		h.stream.WritePacketRTP(ctx.TrackID, ctx.Packet, ctx.PTSEqualsDTS)
	}
}

func (h *handler) Wait() {
	h.wg.Wait()
}

//type Handler interface {
//	gortsplib.ServerHandlerOnAnnounce
//	gortsplib.ServerHandlerOnConnClose
//	gortsplib.ServerHandlerOnConnOpen
//	gortsplib.ServerHandlerOnDescribe
//	gortsplib.ServerHandlerOnPacketRTP
//	gortsplib.ServerHandlerOnPause
//	gortsplib.ServerHandlerOnPlay
//	gortsplib.ServerHandlerOnRecord
//	gortsplib.ServerHandlerOnSessionClose
//	gortsplib.ServerHandlerOnSessionOpen
//	gortsplib.ServerHandlerOnSetup
//}
//
//func NewHandler() Handler {
//	return &handler{}
//}

const FORMAT = "flv" // "matroska"

// ffmpeg -i http://playerservices.streamtheworld.com/api/livestream-redirect/KINK.mp3 -c copy -f rtsp rtsp://127.0.0.1:8554/123
func main() {
	var graceFullStop bool
	var urlPrefix string
	for _, arg := range os.Args[1:] {
		urlPrefix = arg
	}
	h := &handler{
		url: urlPrefix,
	}
	s := &gortsplib.Server{
		Handler:        h,
		RTSPAddress:    ":8554",
		UDPRTPAddress:  ":8000",
		UDPRTCPAddress: ":8001",
	}
	err := s.Start()
	if err != nil {
		log.Fatal(err)
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		if graceFullStop {
			h.Wait()
		}
		_ = s.Close()
	}()

	log.Print("ready")
	defer log.Print("done")

	err = s.Wait()
	if err != nil {
		signal.Stop(c)
		switch err.(type) {
		case liberrors.ErrServerTerminated:
		default:
			log.Fatal(err)
		}
	}
	h.Wait()
}
