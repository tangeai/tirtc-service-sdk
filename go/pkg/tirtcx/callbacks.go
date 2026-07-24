package tirtcx

import (
	"sync"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/internal/tirtcclib"
)

var (
	connRegistry sync.Map // tirtcclib.ConnHandle -> *Conn
)

func callbacks() *tirtcclib.Callbacks {
	return &tirtcclib.Callbacks{
		OnConnError: func(hconn tirtcclib.ConnHandle, errorCode int) {
			if conn := lookupConn(hconn); conn != nil {
				err := tirtcclib.ErrorFromCode(errorCode)
				if sendToChan(conn.errorCh, err) {
					conn.eventStats.errorDropped.Add(1)
				}
				conn.finish(err)
			}
		},
		OnDisconnected: func(hconn tirtcclib.ConnHandle) {
			if conn := lookupConn(hconn); conn != nil {
				if sendToChan(conn.disconnectedCh, struct{}{}) {
					conn.eventStats.disconnectedDropped.Add(1)
				}
				connRegistry.Delete(hconn)
				conn.finish(nil)
				// TiRTC guarantees callbacks for the same connection are
				// serialized, so no media callback can race these closes.
				conn.closeEventChannels()
				return
			}
			connRegistry.Delete(hconn)
		},
		OnAudio: func(hconn tirtcclib.ConnHandle, info tirtcclib.FrameInfo, data []byte) {
			if conn := lookupConn(hconn); conn != nil {
				if sendToChan(conn.audioCh, AudioFrame{Info: info, Data: data}) {
					conn.eventStats.audioDropped.Add(1)
				}
			}
		},
		OnVideo: func(hconn tirtcclib.ConnHandle, info tirtcclib.FrameInfo, data []byte) {
			if conn := lookupConn(hconn); conn != nil {
				if sendToChan(conn.videoCh, VideoFrame{Info: info, Data: data}) {
					conn.eventStats.videoDropped.Add(1)
				}
			}
		},
		OnMessage: func(hconn tirtcclib.ConnHandle, info tirtcclib.FrameInfo, data []byte) {
			if conn := lookupConn(hconn); conn != nil {
				if sendToChan(conn.messageCh, MessageFrame{Info: info, Data: data}) {
					conn.eventStats.messageDropped.Add(1)
				}
			}
		},
		OnCommand: func(hconn tirtcclib.ConnHandle, cmd uint32, data []byte) {
			if conn := lookupConn(hconn); conn != nil {
				if sendToChan(conn.commandCh, Command{Cmd: cmd, Data: data}) {
					conn.eventStats.commandDropped.Add(1)
				}
			}
		},
		OnRequestKeyFrame: func(hconn tirtcclib.ConnHandle, streamID uint8) {
			if conn := lookupConn(hconn); conn != nil {
				if sendToChan(conn.requestKeyFrameCh, streamID) {
					conn.eventStats.requestKeyFrameDropped.Add(1)
				}
			}
		},
		OnSubscribeVideo: func(hconn tirtcclib.ConnHandle, streamID uint8) int {
			if conn := lookupConn(hconn); conn != nil {
				return callSubscribeHandler(conn.subscribeVideo, streamID)
			}
			return 0
		},
		OnUnsubscribeVideo: func(hconn tirtcclib.ConnHandle, streamID uint8) {
			if conn := lookupConn(hconn); conn != nil {
				if sendToChan(conn.unsubscribeVideoCh, streamID) {
					conn.eventStats.unsubscribeVideoDropped.Add(1)
				}
			}
		},
		OnSubscribeAudio: func(hconn tirtcclib.ConnHandle, streamID uint8) int {
			if conn := lookupConn(hconn); conn != nil {
				return callSubscribeHandler(conn.subscribeAudio, streamID)
			}
			return 0
		},
		OnUnsubscribeAudio: func(hconn tirtcclib.ConnHandle, streamID uint8) {
			if conn := lookupConn(hconn); conn != nil {
				if sendToChan(conn.unsubscribeAudioCh, streamID) {
					conn.eventStats.unsubscribeAudioDropped.Add(1)
				}
			}
		},
	}
}

func callSubscribeHandler(handler func(uint8) int, streamID uint8) (result int) {
	if handler == nil {
		return 0
	}
	defer func() {
		if recover() != nil {
			result = 0
		}
	}()
	return handler(streamID)
}

func sendToChan[T any](ch chan T, event T) (dropped bool) {
	if ch == nil {
		return false
	}
	select {
	case ch <- event:
		return false
	default:
		return true
	}
}

func bindConnHandle(conn *Conn, hconn tirtcclib.ConnHandle) (*Conn, bool) {
	if conn == nil || hconn == nil {
		return nil, false
	}
	conn.bindHandle(hconn)
	actual, loaded := connRegistry.LoadOrStore(hconn, conn)
	if loaded && actual != conn {
		return actual.(*Conn), false
	}
	return conn, true
}

func lookupConn(hconn tirtcclib.ConnHandle) *Conn {
	val, ok := connRegistry.Load(hconn)
	if !ok {
		return nil
	}
	return val.(*Conn)
}
