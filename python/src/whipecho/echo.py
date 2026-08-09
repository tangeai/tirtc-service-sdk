from __future__ import annotations

import json
import logging
import queue
import re
import secrets
import threading
import time
from dataclasses import dataclass, field
from enum import Enum
from typing import Mapping, Optional
from urllib.parse import parse_qs

import tirtcx

CMD_TIME_SYNC_REQUEST = 0x2010
CMD_TIME_SYNC_RESPONSE = 0x2011


class CloseReason(str, Enum):
    DELETED = "deleted"
    DISCONNECTED = "disconnected"
    CONNECTION_ERROR = "connection_error"
    CONNECTION_FAILED = "connection_failed"
    SHUTDOWN = "shutdown"


class AcceptStage(str, Enum):
    VALIDATE = "validate"
    WHIP_ACCEPT = "whip_accept"


class ConnectFailureReason(str, Enum):
    TIMEOUT = "timeout"
    ERROR = "error"


class MediaType(str, Enum):
    AUDIO = "audio"
    VIDEO = "video"
    COMMAND = "command"


class EventType(str, Enum):
    AUDIO = "audio"
    VIDEO = "video"
    MESSAGE = "message"
    COMMAND = "command"
    ERROR = "error"
    DISCONNECTED = "disconnected"
    REQUEST_KEY_FRAME = "request_key_frame"
    UNSUBSCRIBE_VIDEO = "unsubscribe_video"
    UNSUBSCRIBE_AUDIO = "unsubscribe_audio"


class Observer:
    """Echo 生命周期观察器。所有方法都应快速返回并保证线程安全。"""

    def session_accepted(self) -> None:
        pass

    def session_connected(self, duration_seconds: float) -> None:
        pass

    def session_closed(
        self, reason: CloseReason, duration_seconds: float
    ) -> None:
        pass

    def accept_failed(self, stage: AcceptStage) -> None:
        pass

    def connect_failed(self, reason: ConnectFailureReason) -> None:
        pass

    def audio_echoed(self, size: int) -> None:
        pass

    def video_echoed(self, size: int) -> None:
        pass

    def send_failed(self, media: MediaType) -> None:
        pass

    def events_dropped(self, event: EventType, count: int) -> None:
        pass


@dataclass(frozen=True)
class AcceptResult:
    session_id: str
    answer_sdp: bytes
    location: str


@dataclass(frozen=True)
class HTTPResponse:
    status: int
    headers: Mapping[str, str] = field(default_factory=dict)
    body: bytes = b""


@dataclass
class Stats:
    active_sessions: int = 0
    accepted_total: int = 0
    accept_failed_total: int = 0
    connect_failed_total: int = 0
    connect_timeout_total: int = 0
    deleted_total: int = 0
    audio_frames_total: int = 0
    audio_bytes_total: int = 0
    video_frames_total: int = 0
    video_bytes_total: int = 0
    send_failed_total: int = 0
    dropped_events_total: int = 0
    closed_total: dict[CloseReason, int] = field(
        default_factory=lambda: {reason: 0 for reason in CloseReason}
    )


class _Session:
    def __init__(self, session_id, whip_session, manager: "SessionManager") -> None:
        self.id = session_id
        self.whip_session = whip_session
        self.manager = manager
        self.closed = threading.Event()
        self.conn: Optional[tirtcx.Conn] = None
        self.created_at = time.monotonic()
        self._close_lock = threading.Lock()

    def run(self) -> None:
        try:
            self.conn = self.whip_session.wait_conn(self.manager.conn_timeout)
        except TimeoutError:
            self.manager._increment("connect_timeout_total")
            self.manager._increment("connect_failed_total")
            self.manager.observer.connect_failed(ConnectFailureReason.TIMEOUT)
            self.close(CloseReason.CONNECTION_FAILED)
            return
        except Exception:
            self.manager._increment("connect_failed_total")
            self.manager.observer.connect_failed(ConnectFailureReason.ERROR)
            self.close(CloseReason.CONNECTION_FAILED)
            return
        self.manager.observer.session_connected(time.monotonic() - self.created_at)

        while not self.closed.is_set() and not self.conn.done.is_set():
            handled = False
            handled |= self._drain(self.conn.events.audio, self._audio)
            handled |= self._drain(self.conn.events.video, self._video)
            handled |= self._drain(self.conn.events.command, self._command)
            if not handled:
                self.conn.done.wait(0.01)
        reason = (
            CloseReason.CONNECTION_ERROR
            if self.conn.error is not None
            else CloseReason.DISCONNECTED
        )
        self.close(reason)

    @staticmethod
    def _drain(source, callback) -> bool:
        if source is None:
            return False
        handled = False
        while True:
            try:
                value = source.get_nowait()
            except queue.Empty:
                return handled
            callback(value)
            handled = True

    def _audio(self, frame: tirtcx.AudioFrame) -> None:
        if self.conn is None:
            return
        now_tick = (int(time.time_ns() / 100_000) & ((1 << 20) - 1)) << 12
        frame.info.timestamp = now_tick | (frame.info.timestamp & ((1 << 12) - 1))
        try:
            self.conn.send_audio(frame.info, frame.data)
            self.manager._add_media("audio", len(frame.data))
            self.manager.observer.audio_echoed(len(frame.data))
        except Exception:
            self.manager._increment("send_failed_total")
            self.manager.observer.send_failed(MediaType.AUDIO)

    def _video(self, frame: tirtcx.VideoFrame) -> None:
        if self.conn is None:
            return
        try:
            self.conn.send_video(frame.info, frame.data)
            self.manager._add_media("video", len(frame.data))
            self.manager.observer.video_echoed(len(frame.data))
        except Exception:
            self.manager._increment("send_failed_total")
            self.manager.observer.send_failed(MediaType.VIDEO)

    def _command(self, command: tirtcx.Command) -> None:
        if self.conn is None or command.command != CMD_TIME_SYNC_REQUEST:
            return
        payload = self._time_sync_payload(command.data)
        if payload is None:
            return
        try:
            self.conn.send_command(CMD_TIME_SYNC_RESPONSE, payload)
        except Exception:
            self.manager._increment("send_failed_total")
            self.manager.observer.send_failed(MediaType.COMMAND)

    @staticmethod
    def _time_sync_payload(data: bytes) -> Optional[bytes]:
        try:
            value = json.loads(data or b"{}")
        except (json.JSONDecodeError, UnicodeDecodeError):
            return None
        if not isinstance(value, dict):
            return None
        seq = value.get("seq", 0)
        client_send = value.get("client_send_unix_ns", 0)
        if (
            not isinstance(seq, int)
            or isinstance(seq, bool)
            or not 0 <= seq <= 0xFFFFFFFF
            or not isinstance(client_send, int)
            or isinstance(client_send, bool)
            or not -(1 << 63) <= client_send < (1 << 63)
        ):
            return None
        response = {
            "version": 1,
            "seq": seq,
            "client_send_unix_ns": client_send,
            "server_recv_unix_ns": time.time_ns(),
        }
        return json.dumps(response, separators=(",", ":")).encode()

    def close(self, reason: CloseReason = CloseReason.DELETED) -> None:
        with self._close_lock:
            if self.closed.is_set():
                return
            self.closed.set()
        if self.conn is not None:
            self.manager._record_dropped(self.conn.event_stats())
        self.whip_session.close()
        self.manager._remove(
            self.id, reason, time.monotonic() - self.created_at
        )


class SessionManager:
    def __init__(
        self,
        acceptor: tirtcx.WhipAcceptor,
        location_prefix: str,
        conn_timeout: float = 30.0,
        logger: Optional[logging.Logger] = None,
        observer: Optional[Observer] = None,
    ) -> None:
        if acceptor is None:
            raise ValueError("acceptor is required")
        if (
            not location_prefix.startswith("/")
            or location_prefix.endswith("/")
            or "?" in location_prefix
            or "#" in location_prefix
        ):
            raise ValueError("location_prefix must be an absolute path without trailing slash")
        self.acceptor = acceptor
        self.location_prefix = location_prefix
        self.conn_timeout = conn_timeout if conn_timeout > 0 else 30.0
        self.logger = logger or logging.getLogger(__name__)
        self.observer = observer or Observer()
        self._sessions: dict[str, _Session] = {}
        self._stats = Stats()
        self._lock = threading.RLock()
        self._closed = False
        self._threads: set[threading.Thread] = set()

    def accept(self, offer_sdp: bytes, timeout: float = 10.0) -> AcceptResult:
        if not offer_sdp:
            self._increment("accept_failed_total")
            self.observer.accept_failed(AcceptStage.VALIDATE)
            raise ValueError("SDP offer is empty")
        with self._lock:
            if self._closed:
                raise RuntimeError("session manager is closed")
        session_id = secrets.token_hex(16)
        try:
            whip_session = self.acceptor.whip_accept(
                offer_sdp,
                tirtcx.ConnEventOptions(
                    audio_buffer=16,
                    video_buffer=16,
                    command_buffer=16,
                    error_buffer=4,
                    disconnected_buffer=4,
                ),
                timeout,
            )
        except Exception:
            self._increment("accept_failed_total")
            self.observer.accept_failed(AcceptStage.WHIP_ACCEPT)
            raise
        session = _Session(session_id, whip_session, self)
        with self._lock:
            if self._closed:
                whip_session.close()
                raise RuntimeError("session manager is closed")
            self._sessions[session_id] = session
            self._stats.active_sessions = len(self._sessions)
            self._stats.accepted_total += 1
        self.observer.session_accepted()
        thread = threading.Thread(
            target=self._run_session, args=(session,), daemon=True
        )
        with self._lock:
            self._threads.add(thread)
            thread.start()
        return AcceptResult(
            session_id,
            whip_session.answer_sdp,
            f"{self.location_prefix}/{session_id}",
        )

    def delete(self, session_id: str) -> None:
        value = session_id.strip()
        if not value:
            raise ValueError("session_id is required")
        with self._lock:
            session = self._sessions.get(value)
            self._stats.deleted_total += 1
        if session:
            session.close(CloseReason.DELETED)

    def shutdown(self) -> None:
        with self._lock:
            self._closed = True
            sessions = list(self._sessions.values())
        for session in sessions:
            session.close(CloseReason.SHUTDOWN)
        current = threading.current_thread()
        with self._lock:
            threads = list(self._threads)
        for thread in threads:
            if thread is not current:
                thread.join()

    def stats(self) -> Stats:
        with self._lock:
            values = vars(self._stats).copy()
            values["closed_total"] = self._stats.closed_total.copy()
            return Stats(**values)

    def _run_session(self, session: _Session) -> None:
        try:
            session.run()
        finally:
            with self._lock:
                self._threads.discard(threading.current_thread())

    def _remove(
        self, session_id: str, reason: CloseReason, duration_seconds: float
    ) -> None:
        with self._lock:
            self._sessions.pop(session_id, None)
            self._stats.active_sessions = len(self._sessions)
            self._stats.closed_total[reason] += 1
        self.observer.session_closed(reason, duration_seconds)

    def _increment(self, name: str) -> None:
        with self._lock:
            setattr(self._stats, name, getattr(self._stats, name) + 1)

    def _add_media(self, media: str, size: int) -> None:
        with self._lock:
            setattr(
                self._stats,
                f"{media}_frames_total",
                getattr(self._stats, f"{media}_frames_total") + 1,
            )
            setattr(
                self._stats,
                f"{media}_bytes_total",
                getattr(self._stats, f"{media}_bytes_total") + size,
            )

    def _record_dropped(self, stats: tirtcx.EventStats) -> None:
        mapping = {
            EventType.AUDIO: stats.audio_dropped,
            EventType.VIDEO: stats.video_dropped,
            EventType.MESSAGE: stats.message_dropped,
            EventType.COMMAND: stats.command_dropped,
            EventType.ERROR: stats.error_dropped,
            EventType.DISCONNECTED: stats.disconnected_dropped,
            EventType.REQUEST_KEY_FRAME: stats.request_key_frame_dropped,
            EventType.UNSUBSCRIBE_VIDEO: stats.unsubscribe_video_dropped,
            EventType.UNSUBSCRIBE_AUDIO: stats.unsubscribe_audio_dropped,
        }
        for event, count in mapping.items():
            if not count:
                continue
            with self._lock:
                self._stats.dropped_events_total += count
            self.observer.events_dropped(event, count)


class HTTPAdapter:
    """不绑定 Web 框架的 Echo HTTP 适配器。"""

    def __init__(
        self,
        acceptor: tirtcx.WhipAcceptor,
        location_prefix: str,
        max_sdp_bytes: int = 1 << 20,
        conn_timeout: float = 30.0,
        observer: Optional[Observer] = None,
    ) -> None:
        self.manager = SessionManager(
            acceptor,
            location_prefix,
            conn_timeout,
            observer=observer,
        )
        self.max_sdp_bytes = max_sdp_bytes if max_sdp_bytes > 0 else 1 << 20

    def try_handle_post(
        self, raw_query: str, content_type: str, body: bytes
    ) -> Optional[HTTPResponse]:
        values = parse_qs(raw_query, keep_blank_values=True)
        if re.search(r"%(?![0-9A-Fa-f]{2})", raw_query) or ";" in raw_query:
            return HTTPResponse(400, body=b"invalid query")
        modes = values.get("_tg_mode")
        if modes is None:
            return None
        if modes != ["echo"]:
            return HTTPResponse(400, body=b"invalid internal mode")
        if content_type.split(";", 1)[0].strip().lower() != "application/sdp":
            return HTTPResponse(415, body=b"content-type must be application/sdp")
        if len(body) > self.max_sdp_bytes:
            return HTTPResponse(413, body=b"SDP offer is too large")
        try:
            result = self.manager.accept(body)
        except ValueError as exc:
            return HTTPResponse(400, body=str(exc).encode())
        except RuntimeError:
            return HTTPResponse(503, body=b"service unavailable")
        except Exception:
            return HTTPResponse(500, body=b"internal server error")
        return HTTPResponse(
            201,
            {"Content-Type": "application/sdp", "Location": result.location},
            result.answer_sdp,
        )

    def serve_delete(self, session_id: str) -> HTTPResponse:
        try:
            self.manager.delete(session_id)
        except ValueError as exc:
            return HTTPResponse(400, body=str(exc).encode())
        return HTTPResponse(204)

    def stats(self) -> Stats:
        return self.manager.stats()

    def shutdown(self) -> None:
        self.manager.shutdown()
