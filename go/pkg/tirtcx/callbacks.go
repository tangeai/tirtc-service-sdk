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
				if sendToChan(conn.errorCh, tirtcclib.ErrorFromCode(errorCode)) {
					conn.eventStats.errorDropped.Add(1)
				}
			}
		},
		OnDisconnected: func(hconn tirtcclib.ConnHandle) {
			if conn := lookupConn(hconn); conn != nil {
				if sendToChan(conn.disconnectedCh, struct{}{}) {
					conn.eventStats.disconnectedDropped.Add(1)
				}
				connRegistry.Delete(hconn)
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
				result, dropped := sendSubscribeEvent(conn.subscribeVideoCh, streamID)
				if dropped {
					conn.eventStats.subscribeVideoDropped.Add(1)
				}
				return result
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
				result, dropped := sendSubscribeEvent(conn.subscribeAudioCh, streamID)
				if dropped {
					conn.eventStats.subscribeAudioDropped.Add(1)
				}
				return result
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

func sendSubscribeEvent(ch chan SubscribeEvent, streamID uint8) (resultCode int, dropped bool) {
	if ch == nil {
		return 0, false
	}
	result := make(chan int, 1)
	event := SubscribeEvent{StreamID: streamID, Result: result}
	select {
	case ch <- event:
		return <-result, false
	default:
		return 0, true
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

func registerConn(hconn tirtcclib.ConnHandle) *Conn {
	if hconn == nil {
		return nil
	}
	if val, ok := connRegistry.Load(hconn); ok {
		return val.(*Conn)
	}
	conn, _ := bindConnHandle(newConn("", ConnEventOptions{}), hconn)
	return conn
}

func lookupConn(hconn tirtcclib.ConnHandle) *Conn {
	val, ok := connRegistry.Load(hconn)
	if !ok {
		return nil
	}
	return val.(*Conn)
}
