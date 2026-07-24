from __future__ import annotations

import ctypes
import os
import queue
import threading
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Optional

MEDIA_MESSAGE = 0
AUDIO_PCM = 1
AUDIO_ALAW = 2
AUDIO_AAC = 3
AUDIO_OPUS = 4
AUDIO_AMR = 5
VIDEO_JPEG = 65
VIDEO_H264 = 66
VIDEO_H265 = 67
MIN_EVENT_BUFFER_SIZE = 4


class TiRTCError(RuntimeError):
    def __init__(self, code: int, message: str = "") -> None:
        self.code = code
        super().__init__(message or f"TiRTC error {code}")


@dataclass
class FrameInfo:
    stream_id: int = 0
    media: int = MEDIA_MESSAGE
    flags: int = 0
    timestamp: int = 0


@dataclass(frozen=True)
class AudioFrame:
    info: FrameInfo
    data: bytes


@dataclass(frozen=True)
class VideoFrame:
    info: FrameInfo
    data: bytes


@dataclass(frozen=True)
class MessageFrame:
    info: FrameInfo
    data: bytes


@dataclass(frozen=True)
class Command:
    command: int
    data: bytes


@dataclass
class EventStats:
    audio_dropped: int = 0
    video_dropped: int = 0
    message_dropped: int = 0
    command_dropped: int = 0
    error_dropped: int = 0
    disconnected_dropped: int = 0
    request_key_frame_dropped: int = 0
    unsubscribe_video_dropped: int = 0
    unsubscribe_audio_dropped: int = 0


@dataclass
class ConnEventOptions:
    audio_buffer: int = 0
    video_buffer: int = 0
    message_buffer: int = 0
    command_buffer: int = 0
    error_buffer: int = 0
    disconnected_buffer: int = 0
    request_key_frame_buffer: int = 0
    unsubscribe_video_buffer: int = 0
    unsubscribe_audio_buffer: int = 0
    subscribe_video_handler: Optional[Callable[[int], int]] = None
    subscribe_audio_handler: Optional[Callable[[int], int]] = None


@dataclass(frozen=True)
class ConnEvents:
    audio: Optional[queue.Queue]
    video: Optional[queue.Queue]
    message: Optional[queue.Queue]
    command: Optional[queue.Queue]
    errors: Optional[queue.Queue]
    disconnected: Optional[queue.Queue]
    request_key_frame: Optional[queue.Queue]
    unsubscribe_video: Optional[queue.Queue]
    unsubscribe_audio: Optional[queue.Queue]


def _event_queue(size: int) -> Optional[queue.Queue]:
    return queue.Queue(maxsize=max(size, MIN_EVENT_BUFFER_SIZE)) if size > 0 else None


class Conn:
    """已建立连接的 TiRTC 连接对象。"""

    def __init__(self, backend: "_NativeBackend", options: ConnEventOptions) -> None:
        self._backend = backend
        self._handle = 0
        self._handle_lock = threading.Lock()
        self._close_requested = False
        self._closed = False
        self.done = threading.Event()
        self.error: Optional[BaseException] = None
        self.events = ConnEvents(
            audio=_event_queue(options.audio_buffer),
            video=_event_queue(options.video_buffer),
            message=_event_queue(options.message_buffer),
            command=_event_queue(options.command_buffer),
            errors=_event_queue(options.error_buffer),
            disconnected=_event_queue(options.disconnected_buffer),
            request_key_frame=_event_queue(options.request_key_frame_buffer),
            unsubscribe_video=_event_queue(options.unsubscribe_video_buffer),
            unsubscribe_audio=_event_queue(options.unsubscribe_audio_buffer),
        )
        self.subscribe_video_handler = options.subscribe_video_handler
        self.subscribe_audio_handler = options.subscribe_audio_handler
        self._stats = EventStats()

    @property
    def handle(self) -> int:
        return self._handle

    def _bind(self, handle: int) -> None:
        with self._handle_lock:
            self._handle = handle
            close = self._close_requested
        if close:
            self.close()

    def _put(self, name: str, value: object) -> None:
        target = getattr(self.events, name)
        if target is None:
            return
        try:
            target.put_nowait(value)
        except queue.Full:
            stat_name = {
                "errors": "error_dropped",
            }.get(name, f"{name}_dropped")
            setattr(self._stats, stat_name, getattr(self._stats, stat_name) + 1)

    def _finish(self, error: Optional[BaseException]) -> None:
        if not self.done.is_set():
            self.error = error
            self.done.set()

    def event_stats(self) -> EventStats:
        return EventStats(**vars(self._stats))

    def send_message(self, info: FrameInfo, data: bytes) -> None:
        self._backend.send_frame("TiRtcSendMessageStream", self._handle, info, data)

    def send_audio(self, info: FrameInfo, data: bytes) -> None:
        self._backend.send_frame("TiRtcSendAudioStream", self._handle, info, data)

    def send_video(self, info: FrameInfo, data: bytes) -> None:
        self._backend.send_frame("TiRtcSendVideoStream", self._handle, info, data)

    def send_command(self, command: int, data: bytes = b"") -> None:
        self._backend.send_command(self._handle, command, data)

    def request_key_frame(self, stream_id: int) -> None:
        self._backend.call_stream("TiRtcRequestKeyFrame", self._handle, stream_id)

    def subscribe_video(self, stream_id: int) -> None:
        self._backend.call_stream("TiRtcSubscribeVideo", self._handle, stream_id)

    def unsubscribe_video(self, stream_id: int) -> None:
        self._backend.call_stream("TiRtcUnsubscribeVideo", self._handle, stream_id)

    def subscribe_audio(self, stream_id: int) -> None:
        self._backend.call_stream("TiRtcSubscribeAudio", self._handle, stream_id)

    def unsubscribe_audio(self, stream_id: int) -> None:
        self._backend.call_stream("TiRtcUnsubscribeAudio", self._handle, stream_id)

    def send_buffer_used(self) -> int:
        return self._backend.send_buffer_used(self._handle)

    def close(self) -> None:
        with self._handle_lock:
            self._close_requested = True
            handle = self._handle
            if self._closed or not handle:
                return
            self._closed = True
        self._backend.disconnect(handle)


class WhipSession:
    def __init__(self, answer_sdp: bytes, conn: Conn, cancel: Callable[[], None]) -> None:
        self.answer_sdp = bytes(answer_sdp)
        self._conn = conn
        self._cancel = cancel
        self._ready = threading.Event()
        self._result_error: Optional[BaseException] = None
        self._closed = False
        self._lock = threading.Lock()

    def _resolve(self, error: Optional[BaseException]) -> None:
        self._result_error = error
        self._ready.set()

    def wait_conn(self, timeout: Optional[float] = None) -> Conn:
        if not self._ready.wait(timeout):
            raise TimeoutError("waiting for WHIP connection timed out")
        if self._result_error is not None:
            raise self._result_error
        return self._conn

    def close(self) -> None:
        with self._lock:
            if self._closed:
                return
            self._closed = True
        self._cancel()
        self._conn.close()


class WhipAcceptor:
    """不包含认证策略的 WHIP Acceptor。"""

    def __init__(self, candidate: str = "", backend: Optional["_NativeBackend"] = None) -> None:
        self.candidate = candidate.strip()
        self._backend = backend or _backend()

    def whip_accept(
        self,
        offer_sdp: bytes,
        event_options: Optional[ConnEventOptions] = None,
        timeout: Optional[float] = 10.0,
    ) -> WhipSession:
        if not offer_sdp:
            raise ValueError("SDP offer is empty")
        options = event_options or ConnEventOptions()
        conn = Conn(self._backend, options)
        sdp_ready = threading.Event()
        answer: list[bytes] = []
        result: list[WhipSession] = []
        connect_result: list[Optional[BaseException]] = []

        def on_sdp(value: bytes) -> None:
            answer.append(bytes(value))
            sdp_ready.set()

        def on_connect(error: int, handle: int) -> None:
            session = result[0] if result else None
            if error:
                exc = self._backend.error(error)
                conn._put("errors", exc)
                conn._finish(exc)
                if session:
                    session._resolve(exc)
                else:
                    connect_result.append(exc)
                    sdp_ready.set()
                return
            conn._bind(handle)
            self._backend.register_conn(handle, conn)
            if session:
                session._resolve(None)
            else:
                connect_result.append(None)

        cancel = self._backend.whip_accept(
            offer_sdp, self.candidate, on_sdp, on_connect
        )
        if not sdp_ready.wait(timeout):
            cancel()
            conn.close()
            raise TimeoutError("waiting for SDP answer timed out")
        if not answer:
            cancel()
            if connect_result and connect_result[0] is not None:
                raise connect_result[0]
            raise RuntimeError("WHIP accept ended without an SDP answer")
        session = WhipSession(answer[0], conn, cancel)
        result.append(session)
        if connect_result:
            session._resolve(connect_result[0])
        return session


class _CFrameInfo(ctypes.Structure):
    _pack_ = 1
    _fields_ = [
        ("stream_id", ctypes.c_uint8),
        ("media", ctypes.c_uint8),
        ("flags", ctypes.c_uint8),
        ("reserved", ctypes.c_uint8),
        ("ts", ctypes.c_uint32),
        ("length", ctypes.c_uint32),
    ]


class _NativeBackend:
    def __init__(self, library: Optional[str] = None) -> None:
        path = library or os.getenv("TIRTC_PYTHON_LIBRARY") or self._default_library()
        if not path:
            raise RuntimeError(
                "TiRTC native library not found; set TIRTC_PYTHON_LIBRARY"
            )
        self.lib = ctypes.CDLL(path)
        self._connections: dict[int, Conn] = {}
        self._accepts: dict[int, tuple] = {}
        self._next_context = 1
        self._lock = threading.RLock()
        self._configure()

    @staticmethod
    def _default_library() -> str:
        root = Path(__file__).resolve().parents[3] / "clib" / "lib"
        for name in ("libTiRTC.so", "libTiRTC.dylib"):
            candidate = root / name
            if candidate.exists():
                return str(candidate)
        return ""

    def _configure(self) -> None:
        self._event_cb = ctypes.CFUNCTYPE(None, ctypes.c_int, ctypes.c_void_p, ctypes.c_int)(self._on_event)
        self._conn_cb = ctypes.CFUNCTYPE(None, ctypes.c_void_p)(self._on_conn_accepted)
        self._conn_error_cb = ctypes.CFUNCTYPE(None, ctypes.c_void_p, ctypes.c_int)(self._on_conn_error)
        self._disconnected_cb = ctypes.CFUNCTYPE(None, ctypes.c_void_p)(self._on_disconnected)
        self._frame_cb_type = ctypes.CFUNCTYPE(None, ctypes.c_void_p, ctypes.POINTER(_CFrameInfo), ctypes.c_void_p)
        self._audio_cb = self._frame_cb_type(lambda h, i, d: self._on_frame("audio", AudioFrame, h, i, d))
        self._video_cb = self._frame_cb_type(lambda h, i, d: self._on_frame("video", VideoFrame, h, i, d))
        self._message_cb = self._frame_cb_type(lambda h, i, d: self._on_frame("message", MessageFrame, h, i, d))
        self._command_cb = ctypes.CFUNCTYPE(None, ctypes.c_void_p, ctypes.c_uint32, ctypes.c_void_p, ctypes.c_uint32)(self._on_command)
        self._stream_cb_type = ctypes.CFUNCTYPE(None, ctypes.c_void_p, ctypes.c_uint8)
        self._request_key_cb = self._stream_cb_type(lambda h, s: self._stream_event("request_key_frame", h, s))
        self._unsubscribe_video_cb = self._stream_cb_type(lambda h, s: self._stream_event("unsubscribe_video", h, s))
        self._unsubscribe_audio_cb = self._stream_cb_type(lambda h, s: self._stream_event("unsubscribe_audio", h, s))
        self._subscribe_cb_type = ctypes.CFUNCTYPE(ctypes.c_int, ctypes.c_void_p, ctypes.c_uint8)
        self._subscribe_video_cb = self._subscribe_cb_type(lambda h, s: self._subscribe("video", h, s))
        self._subscribe_audio_cb = self._subscribe_cb_type(lambda h, s: self._subscribe("audio", h, s))

        class Callbacks(ctypes.Structure):
            _fields_ = [
                ("on_event", type(self._event_cb)),
                ("on_conn_accepted", type(self._conn_cb)),
                ("on_conn_error", type(self._conn_error_cb)),
                ("on_disconnected", type(self._disconnected_cb)),
                ("on_audio", self._frame_cb_type),
                ("on_video", self._frame_cb_type),
                ("on_message", self._frame_cb_type),
                ("on_command", type(self._command_cb)),
                ("on_request_key_frame", self._stream_cb_type),
                ("on_subscribe_video", self._subscribe_cb_type),
                ("on_unsubscribe_video", self._stream_cb_type),
                ("on_subscribe_audio", self._subscribe_cb_type),
                ("on_unsubscribe_audio", self._stream_cb_type),
            ]

        self.callbacks = Callbacks(
            self._event_cb, self._conn_cb, self._conn_error_cb, self._disconnected_cb,
            self._audio_cb, self._video_cb, self._message_cb, self._command_cb,
            self._request_key_cb, self._subscribe_video_cb, self._unsubscribe_video_cb,
            self._subscribe_audio_cb, self._unsubscribe_audio_cb,
        )
        self._sdp_cb_type = ctypes.CFUNCTYPE(None, ctypes.c_void_p, ctypes.c_int, ctypes.c_void_p)
        self._connect_cb_type = ctypes.CFUNCTYPE(None, ctypes.c_int, ctypes.c_void_p, ctypes.c_void_p)
        self._sdp_cb = self._sdp_cb_type(self._on_sdp)
        self._connect_cb = self._connect_cb_type(self._on_accept_connect)
        self.lib.TiRtcGetVersion.restype = ctypes.c_char_p
        self.lib.TiRtcGetErrorStr.argtypes = [ctypes.c_int]
        self.lib.TiRtcGetErrorStr.restype = ctypes.c_char_p
        self.lib.TiRtcStart.argtypes = [ctypes.c_char_p, ctypes.POINTER(Callbacks)]
        self.lib.TiRtcWhipAccept.argtypes = [
            ctypes.c_void_p, ctypes.c_int, ctypes.c_char_p,
            self._sdp_cb_type, self._connect_cb_type, ctypes.c_void_p,
        ]
        self.lib.TiRtcGetSendBufferUsed.restype = ctypes.c_size_t

    def error(self, code: int) -> TiRTCError:
        raw = self.lib.TiRtcGetErrorStr(code)
        return TiRTCError(code, raw.decode(errors="replace") if raw else "")

    def _check(self, code: int) -> int:
        if code < 0:
            raise self.error(code)
        return code

    def init(self) -> None:
        self._check(self.lib.TiRtcInit())

    def start(self) -> None:
        self._check(self.lib.TiRtcStart(None, ctypes.byref(self.callbacks)))

    def stop(self) -> None:
        self._check(self.lib.TiRtcStop())

    def uninit(self) -> None:
        self.lib.TiRtcUninit()

    def set_log_level(self, level: int) -> None:
        self.lib.TiRtcLogSetLevel(level)

    def version(self) -> str:
        value = self.lib.TiRtcGetVersion()
        return value.decode() if value else ""

    def whip_accept(self, offer: bytes, candidate: str, on_sdp, on_connect) -> Callable[[], None]:
        with self._lock:
            context = self._next_context
            self._next_context += 1
            self._accepts[context] = (on_sdp, on_connect, False)
        offer_buf = ctypes.create_string_buffer(offer)
        candidate_raw = candidate.encode() if candidate else None
        code = self.lib.TiRtcWhipAccept(
            offer_buf, len(offer), candidate_raw,
            self._sdp_cb, self._connect_cb, ctypes.c_void_p(context),
        )
        try:
            self._check(code)
        except Exception:
            with self._lock:
                self._accepts.pop(context, None)
            raise

        def cancel() -> None:
            with self._lock:
                entry = self._accepts.get(context)
                if entry:
                    self._accepts[context] = (entry[0], entry[1], True)

        return cancel

    def register_conn(self, handle: int, conn: Conn) -> None:
        with self._lock:
            self._connections[handle] = conn

    def _on_sdp(self, data, length, user) -> None:
        context = int(user or 0)
        with self._lock:
            entry = self._accepts.get(context)
        if entry and not entry[2]:
            entry[0](ctypes.string_at(data, length))

    def _on_accept_connect(self, error, handle, user) -> None:
        context = int(user or 0)
        with self._lock:
            entry = self._accepts.pop(context, None)
        if entry and not entry[2]:
            entry[1](error, int(handle or 0))
        elif not error and handle:
            self.disconnect(int(handle))

    def _conn(self, handle) -> Optional[Conn]:
        with self._lock:
            return self._connections.get(int(handle or 0))

    def _on_event(self, _event, _data, _length) -> None:
        return

    def _on_conn_accepted(self, _handle) -> None:
        return

    def _on_conn_error(self, handle, code) -> None:
        conn = self._conn(handle)
        if conn:
            error = self.error(code)
            conn._put("errors", error)
            conn._finish(error)

    def _on_disconnected(self, handle) -> None:
        key = int(handle or 0)
        with self._lock:
            conn = self._connections.pop(key, None)
        if conn:
            conn._put("disconnected", None)
            conn._finish(None)

    def _on_frame(self, target, frame_type, handle, info_ptr, data) -> None:
        conn = self._conn(handle)
        if not conn or not info_ptr:
            return
        raw = info_ptr.contents
        info = FrameInfo(raw.stream_id, raw.media, raw.flags, raw.ts)
        conn._put(target, frame_type(info, ctypes.string_at(data, raw.length)))

    def _on_command(self, handle, command, data, length) -> None:
        conn = self._conn(handle)
        if conn:
            conn._put("command", Command(command, ctypes.string_at(data, length)))

    def _stream_event(self, target, handle, stream_id) -> None:
        conn = self._conn(handle)
        if conn:
            conn._put(target, int(stream_id))

    def _subscribe(self, media, handle, stream_id) -> int:
        conn = self._conn(handle)
        if not conn:
            return 0
        callback = (
            conn.subscribe_video_handler if media == "video"
            else conn.subscribe_audio_handler
        )
        if callback is None:
            return 0
        try:
            return int(callback(int(stream_id)))
        except Exception:
            return 0

    def send_frame(self, function: str, handle: int, info: FrameInfo, data: bytes) -> None:
        raw = _CFrameInfo(info.stream_id, info.media, info.flags, 0, info.timestamp, len(data))
        payload = ctypes.create_string_buffer(data) if data else None
        self._check(getattr(self.lib, function)(ctypes.c_void_p(handle), ctypes.byref(raw), payload))

    def send_command(self, handle: int, command: int, data: bytes) -> None:
        payload = ctypes.create_string_buffer(data) if data else None
        self._check(self.lib.TiRtcSendCommand(ctypes.c_void_p(handle), command, payload, len(data)))

    def call_stream(self, function: str, handle: int, stream_id: int) -> None:
        self._check(getattr(self.lib, function)(ctypes.c_void_p(handle), stream_id))

    def send_buffer_used(self, handle: int) -> int:
        return int(self.lib.TiRtcGetSendBufferUsed(ctypes.c_void_p(handle)))

    def disconnect(self, handle: int) -> None:
        self._check(self.lib.TiRtcDisconnect(ctypes.c_void_p(handle)))


_backend_instance: Optional[_NativeBackend] = None
_backend_lock = threading.Lock()


def _backend() -> _NativeBackend:
    global _backend_instance
    with _backend_lock:
        if _backend_instance is None:
            _backend_instance = _NativeBackend()
        return _backend_instance


def init() -> None:
    _backend().init()


def start() -> None:
    _backend().start()


def stop() -> None:
    _backend().stop()


def uninit() -> None:
    _backend().uninit()


def set_log_level(level: int) -> None:
    _backend().set_log_level(level)


def get_version() -> str:
    return _backend().version()
