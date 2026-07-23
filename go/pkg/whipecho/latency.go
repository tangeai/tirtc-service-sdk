package whipecho

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"time"

	"gitlab.tange-ai.com/tirtc-nexus/whip-sdk/go/pkg/tirtcx"
)

const (
	probeVersion  = 1
	probeHeader   = 32
	maxRepeat     = 1000
	maxIntervalMS = 60000
	maxTimeoutMS  = 60000
	maxTotalMS    = 5 * 60 * 1000
	maxStreamID   = 15
	maxProbeBytes = 64 * 1024
)

var probeMagic = [8]byte{'T', 'I', 'R', 'T', 'C', 'E', 'C', 'H'}

type LatencyStartRequest struct {
	Version    int   `json:"version"`
	Repeat     int   `json:"repeat,omitempty"`
	IntervalMS int   `json:"interval_ms,omitempty"`
	TimeoutMS  int   `json:"timeout_ms,omitempty"`
	StreamID   uint8 `json:"stream_id,omitempty"`
	ProbeBytes int   `json:"probe_bytes,omitempty"`
	TotalMS    int   `json:"total_ms,omitempty"`
}

type LatencyResult struct {
	Version      int             `json:"version"`
	Repeat       int             `json:"repeat"`
	Received     int             `json:"received"`
	Lost         int             `json:"lost"`
	MinRTTUS     int64           `json:"min_rtt_us"`
	MaxRTTUS     int64           `json:"max_rtt_us"`
	AvgRTTUS     int64           `json:"avg_rtt_us"`
	StartedAtNS  int64           `json:"started_at_unix_ns"`
	FinishedAtNS int64           `json:"finished_at_unix_ns"`
	Samples      []LatencySample `json:"samples"`
}

type LatencySample struct {
	Seq          uint32 `json:"seq"`
	RTTUS        int64  `json:"rtt_us"`
	SentAtNS     int64  `json:"sent_at_unix_ns"`
	ReceivedAtNS int64  `json:"received_at_unix_ns"`
}

type LatencyError struct {
	Version int    `json:"version"`
	Error   string `json:"error"`
}

type probeInfo struct {
	seq      uint32
	sentAtNS int64
}

type latencyRunner struct {
	session *session
	mu      sync.Mutex
	running bool
	nextSeq uint32
	pending map[uint32]chan LatencySample
	done    chan struct{}
}

func newLatencyRunner(session *session) *latencyRunner {
	return &latencyRunner{session: session, pending: make(map[uint32]chan LatencySample), done: make(chan struct{})}
}

func (r *latencyRunner) start(request LatencyStartRequest) error {
	request = r.normalize(request)
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("latency test already running")
	}
	r.running = true
	r.mu.Unlock()
	go r.run(request)
	return nil
}

func (r *latencyRunner) stop() {
	r.mu.Lock()
	select {
	case <-r.done:
	default:
		close(r.done)
	}
	r.pending = make(map[uint32]chan LatencySample)
	r.running = false
	r.mu.Unlock()
}

func (r *latencyRunner) handleAudio(frame tirtcx.AudioFrame) bool {
	info, ok := parseProbePayload(frame.Data)
	if !ok {
		return false
	}
	now := time.Now()
	sample := LatencySample{Seq: info.seq, RTTUS: now.Sub(time.Unix(0, info.sentAtNS)).Microseconds(), SentAtNS: info.sentAtNS, ReceivedAtNS: now.UnixNano()}
	r.mu.Lock()
	ch := r.pending[info.seq]
	r.mu.Unlock()
	if ch != nil {
		select {
		case ch <- sample:
		default:
		}
	}
	return true
}

func (r *latencyRunner) run(request LatencyStartRequest) {
	started := time.Now()
	deadline := started.Add(time.Duration(request.TotalMS) * time.Millisecond)
	samples := make([]LatencySample, 0, request.Repeat)
	for index := 0; index < request.Repeat && time.Now().Before(deadline); index++ {
		sample, ok := r.sendAndWait(request, deadline)
		if ok {
			samples = append(samples, sample)
		}
		if index+1 < request.Repeat {
			select {
			case <-time.After(time.Duration(request.IntervalMS) * time.Millisecond):
			case <-r.done:
				r.finish()
				return
			}
		}
	}
	payload, err := json.Marshal(buildLatencyResult(request.Repeat, started, time.Now(), samples))
	if err == nil {
		_ = r.session.conn.SendCommand(CmdLatencyResult, payload)
	}
	r.finish()
}

func (r *latencyRunner) sendAndWait(request LatencyStartRequest, deadline time.Time) (LatencySample, bool) {
	seq, ch := r.newPending()
	defer r.deletePending(seq)
	sentAt := time.Now()
	info := tirtcx.FrameInfo{StreamID: request.StreamID, Media: tirtcx.AudioPCM, Ts: uint32(sentAt.UnixMilli())}
	if err := r.session.conn.SendAudio(info, makeProbePayload(seq, sentAt, request.ProbeBytes)); err != nil {
		return LatencySample{}, false
	}
	timeout := time.Duration(request.TimeoutMS) * time.Millisecond
	if remaining := time.Until(deadline); timeout > remaining {
		timeout = remaining
	}
	select {
	case sample := <-ch:
		return sample, true
	case <-time.After(timeout):
		return LatencySample{}, false
	case <-r.done:
		return LatencySample{}, false
	}
}

func (r *latencyRunner) newPending() (uint32, chan LatencySample) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextSeq++
	ch := make(chan LatencySample, 1)
	r.pending[r.nextSeq] = ch
	return r.nextSeq, ch
}

func (r *latencyRunner) deletePending(seq uint32) {
	r.mu.Lock()
	delete(r.pending, seq)
	r.mu.Unlock()
}

func (r *latencyRunner) finish() {
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
}

func (r *latencyRunner) normalize(request LatencyStartRequest) LatencyStartRequest {
	if request.Repeat <= 0 {
		request.Repeat = r.session.opts.LatencyRepeat
	}
	if request.Repeat > maxRepeat {
		request.Repeat = maxRepeat
	}
	if request.IntervalMS <= 0 {
		request.IntervalMS = int(r.session.opts.LatencyInterval / time.Millisecond)
	}
	if request.IntervalMS > maxIntervalMS {
		request.IntervalMS = maxIntervalMS
	}
	if request.TimeoutMS <= 0 {
		request.TimeoutMS = int(r.session.opts.LatencyTimeout / time.Millisecond)
	}
	if request.TimeoutMS > maxTimeoutMS {
		request.TimeoutMS = maxTimeoutMS
	}
	if request.ProbeBytes <= 0 {
		request.ProbeBytes = r.session.opts.LatencyProbeBytes
	}
	if request.ProbeBytes < probeHeader {
		request.ProbeBytes = probeHeader
	}
	if request.ProbeBytes > maxProbeBytes {
		request.ProbeBytes = maxProbeBytes
	}
	if request.StreamID == 0 {
		request.StreamID = r.session.opts.LatencyAudioStream
	}
	if request.StreamID > maxStreamID {
		request.StreamID = maxStreamID
	}
	if request.TotalMS <= 0 || request.TotalMS > maxTotalMS {
		request.TotalMS = maxTotalMS
	}
	return request
}

func makeProbePayload(seq uint32, sentAt time.Time, size int) []byte {
	if size < probeHeader {
		size = probeHeader
	}
	payload := make([]byte, size)
	copy(payload[:8], probeMagic[:])
	payload[8] = probeVersion
	binary.BigEndian.PutUint32(payload[12:16], seq)
	binary.BigEndian.PutUint64(payload[16:24], uint64(sentAt.UnixNano()))
	for index := probeHeader; index < len(payload); index++ {
		payload[index] = byte(seq + uint32(index))
	}
	return payload
}

func parseProbePayload(payload []byte) (probeInfo, bool) {
	if len(payload) < probeHeader || string(payload[:8]) != string(probeMagic[:]) || payload[8] != probeVersion {
		return probeInfo{}, false
	}
	sentAtNS := int64(binary.BigEndian.Uint64(payload[16:24]))
	if sentAtNS <= 0 {
		return probeInfo{}, false
	}
	return probeInfo{seq: binary.BigEndian.Uint32(payload[12:16]), sentAtNS: sentAtNS}, true
}

func buildLatencyResult(repeat int, started, finished time.Time, samples []LatencySample) LatencyResult {
	result := LatencyResult{Version: 1, Repeat: repeat, Received: len(samples), Lost: repeat - len(samples), StartedAtNS: started.UnixNano(), FinishedAtNS: finished.UnixNano(), Samples: samples}
	if len(samples) == 0 {
		return result
	}
	minimum := int64(math.MaxInt64)
	var maximum, sum int64
	for _, sample := range samples {
		if sample.RTTUS < minimum {
			minimum = sample.RTTUS
		}
		if sample.RTTUS > maximum {
			maximum = sample.RTTUS
		}
		sum += sample.RTTUS
	}
	result.MinRTTUS, result.MaxRTTUS, result.AvgRTTUS = minimum, maximum, sum/int64(len(samples))
	return result
}
