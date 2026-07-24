package whipecho

import (
	"sync"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
)

const (
	audioTimestampIndexBits = 12
	audioTimestampIndexMask = uint32((1 << audioTimestampIndexBits) - 1)
	audioTimestampTimeMask  = uint32((1 << 20) - 1)
	audioTimestampUnitUS    = int64(100)
)

type session struct {
	id          string
	whipSession *tirtcx.WhipSession
	conn        *tirtcx.Conn
	opts        Options
	onDone      func(CloseReason)
	createdAt   time.Time

	mu     sync.Mutex
	closed bool
}

func newSession(
	id string,
	whipSession *tirtcx.WhipSession,
	conn *tirtcx.Conn,
	opts Options,
	onDone func(CloseReason),
) *session {
	s := &session{
		id: id, whipSession: whipSession, conn: conn, opts: opts, onDone: onDone,
		createdAt: time.Now(),
	}
	return s
}

func (s *session) bind(conn *tirtcx.Conn) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	s.conn = conn
	s.mu.Unlock()
	return true
}

func (s *session) run(events tirtcx.ConnEvents) {
	for {
		select {
		case frame, ok := <-events.Audio:
			if !ok {
				s.finishFromConn()
				return
			}
			frame.Info.Ts = packAudioTimestamp(time.Now().UnixMicro(), frame.Info.Ts)
			if err := s.conn.SendAudio(frame.Info, frame.Data); err != nil {
				s.opts.stats.sendFailedTotal.Add(1)
				s.opts.Observer.SendFailed(MediaTypeAudio)
				s.opts.Logger.Error("echo audio failed",
					"session_id", s.id,
					"stream_id", frame.Info.StreamID,
					"error", err)
			} else {
				s.opts.stats.audioFramesTotal.Add(1)
				s.opts.stats.audioBytesTotal.Add(uint64(len(frame.Data)))
				s.opts.Observer.AudioEchoed(len(frame.Data))
			}
		case frame, ok := <-events.Video:
			if !ok {
				s.finishFromConn()
				return
			}
			if err := s.conn.SendVideo(frame.Info, frame.Data); err != nil {
				s.opts.stats.sendFailedTotal.Add(1)
				s.opts.Observer.SendFailed(MediaTypeVideo)
				s.opts.Logger.Error("echo video failed",
					"session_id", s.id,
					"stream_id", frame.Info.StreamID,
					"error", err)
			} else {
				s.opts.stats.videoFramesTotal.Add(1)
				s.opts.stats.videoBytesTotal.Add(uint64(len(frame.Data)))
				s.opts.Observer.VideoEchoed(len(frame.Data))
			}
		case command, ok := <-events.Command:
			if !ok {
				s.finishFromConn()
				return
			}
			s.handleCommand(command.Cmd, command.Data)
		case err, ok := <-events.Error:
			if !ok {
				s.finishFromConn()
				return
			}
			s.opts.Logger.Error("echo connection error",
				"session_id", s.id,
				"error", err)
			s.finish(CloseReasonConnectionError)
			return
		case _, ok := <-events.Disconnected:
			if !ok {
				s.finishFromConn()
			} else {
				s.finish(CloseReasonDisconnected)
			}
			return
		case <-s.conn.Done():
			s.finishFromConn()
			return
		}
	}
}

func (s *session) finishFromConn() {
	if s.conn.Err() != nil {
		s.finish(CloseReasonConnectionError)
		return
	}
	s.finish(CloseReasonDisconnected)
}

func (s *session) handleCommand(cmd uint32, data []byte) {
	switch cmd {
	case CmdTimeSyncRequest:
		payload, err := marshalTimeSync(data, time.Now())
		if err != nil {
			s.opts.Logger.Warn("invalid time sync payload",
				"session_id", s.id,
				"error", err)
			return
		}
		if err := s.conn.SendCommand(CmdTimeSyncResponse, payload); err != nil {
			s.opts.stats.sendFailedTotal.Add(1)
			s.opts.Observer.SendFailed(MediaTypeCommand)
			s.opts.Logger.Error("send time sync response failed",
				"session_id", s.id,
				"error", err)
		}
	}
}

func (s *session) close() error {
	return s.shutdown(CloseReasonDeleted)
}

func (s *session) finish(reason CloseReason) {
	if err := s.shutdown(reason); err != nil {
		s.opts.Logger.Error("close echo connection failed",
			"session_id", s.id,
			"error", err)
	}
}

func (s *session) shutdown(reason CloseReason) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	conn := s.conn
	whipSession := s.whipSession
	s.mu.Unlock()
	var err error
	if conn != nil {
		s.opts.stats.recordDropped(s.opts.Observer, conn.EventStats())
	}
	if whipSession != nil {
		err = whipSession.Close()
	} else if conn != nil {
		err = conn.Close()
	}
	s.opts.stats.recordClose(reason)
	s.opts.Observer.SessionClosed(reason, time.Since(s.createdAt))
	if s.onDone != nil {
		s.onDone(reason)
	}
	return err
}

func packAudioTimestamp(unixUS int64, timestamp uint32) uint32 {
	tick := uint32(unixUS/audioTimestampUnitUS) & audioTimestampTimeMask
	return tick<<audioTimestampIndexBits | timestamp&audioTimestampIndexMask
}
