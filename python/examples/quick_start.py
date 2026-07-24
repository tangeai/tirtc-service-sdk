#!/usr/bin/env python3
import argparse
import base64
import math
import secrets
import signal
import struct
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlsplit

import tirtcx
from tirtcxauth import Ed25519TokenVerifier, verify_bearer
from whipecho import HTTPAdapter

MAX_SDP_BYTES = 1 << 20
TEST_JPEG = base64.b64decode(
    "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAP//////////////////////////////////////////////////////////////////////////////////////"
    "2wBDAf//////////////////////////////////////////////////////////////////////////////////////"
    "wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAX/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/"
    "9oADAMBAAIQAxAAAAH/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/9oACAEBAAEFAqf/xAAUEQEAAAAAAAAAAAAAAAAAAAAA/"
    "9oACAEDAQE/Aaf/xAAUEQEAAAAAAAAAAAAAAAAAAAAA/9oACAECAQE/Aaf/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/"
    "9oACAEBAAY/Aqf/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/9oACAEBAAE/IV//2gAMAwEAAgADAAAAEP/EABQRAQAAAAAAAA"
    "AAAAAAAAAAABD/2gAIAQMBAT8QH//EABQRAQAAAAAAAAAAAAAAAAAAABD/2gAIAQIBAT8QH//EABQQAQAAAAAAAAAAAA"
    "AAAAAAAAABED/2gAIAQEAAT8QH//Z"
)


class Service:
    def __init__(self, service, access_key, public_key, candidate="") -> None:
        self.service = service
        self.verifier = Ed25519TokenVerifier({access_key: public_key})
        self.acceptor = tirtcx.WhipAcceptor(candidate)
        self.echo = HTTPAdapter(self.acceptor, "/whip/echo/resource")
        self.sessions = {}
        self.threads = set()
        self.lock = threading.Lock()

    def post(self, handler: BaseHTTPRequestHandler) -> None:
        parsed = urlsplit(handler.path)
        auth = verify_bearer(
            handler.headers.get("Authorization", ""),
            self.service,
            parsed.query,
            self.verifier,
        )
        if not auth.authenticated:
            handler.send_error(auth.status, auth.error)
            return
        try:
            length = int(handler.headers.get("Content-Length", "0"))
        except ValueError:
            handler.send_error(400, "invalid content-length")
            return
        if length <= 0 or length > MAX_SDP_BYTES:
            handler.send_error(413 if length > MAX_SDP_BYTES else 400)
            return
        offer = handler.rfile.read(length)
        echo = self.echo.try_handle_post(
            parsed.query, handler.headers.get("Content-Type", ""), offer
        )
        if echo is not None:
            self._write(handler, echo.status, echo.headers, echo.body)
            return
        if handler.headers.get_content_type() != "application/sdp":
            handler.send_error(415, "content-type must be application/sdp")
            return
        try:
            session = self.acceptor.whip_accept(
                offer,
                tirtcx.ConnEventOptions(
                    command_buffer=16,
                    error_buffer=4,
                    disconnected_buffer=4,
                ),
            )
        except ValueError as exc:
            handler.send_error(400, str(exc))
            return
        except Exception:
            handler.send_error(500, "internal server error")
            return
        session_id = secrets.token_hex(16)
        stop = threading.Event()
        thread = threading.Thread(
            target=self._run_session,
            args=(session_id, session, stop),
            daemon=True,
        )
        with self.lock:
            self.sessions[session_id] = (session, stop)
            self.threads.add(thread)
        thread.start()
        self._write(
            handler,
            201,
            {
                "Content-Type": "application/sdp",
                "Location": f"/whip/resource/{session_id}",
            },
            session.answer_sdp,
        )

    def delete(self, handler: BaseHTTPRequestHandler) -> None:
        path = urlsplit(handler.path).path
        if path.startswith("/whip/echo/resource/"):
            response = self.echo.serve_delete(path.rsplit("/", 1)[1])
            self._write(handler, response.status, response.headers, response.body)
            return
        if not path.startswith("/whip/resource/"):
            handler.send_error(404)
            return
        session_id = path.rsplit("/", 1)[1]
        with self.lock:
            active = self.sessions.pop(session_id, None)
        if active:
            session, stop = active
            stop.set()
            session.close()
        self._write(handler, 204, {}, b"")

    def _run_session(self, session_id, session, stop) -> None:
        try:
            conn = session.wait_conn(timeout=30)
            next_audio = time.monotonic()
            next_video = next_audio
            audio_frame = 0
            while not stop.is_set() and not conn.done.is_set():
                now = time.monotonic()
                if now >= next_audio:
                    conn.send_audio(
                        tirtcx.FrameInfo(
                            stream_id=0,
                            media=tirtcx.AUDIO_PCM,
                            timestamp=int(time.time() * 1000) & 0xFFFFFFFF,
                        ),
                        self._sine_pcm(audio_frame),
                    )
                    audio_frame += 1
                    next_audio += 0.02
                if now >= next_video:
                    conn.send_video(
                        tirtcx.FrameInfo(
                            stream_id=1,
                            media=tirtcx.VIDEO_JPEG,
                            flags=1,
                            timestamp=int(time.time() * 1000) & 0xFFFFFFFF,
                        ),
                        TEST_JPEG,
                    )
                    next_video += 1.0
                if conn.events.command is not None:
                    while not conn.events.command.empty():
                        command = conn.events.command.get_nowait()
                        print(
                            f"business command: session={session_id} "
                            f"command={command.command} bytes={len(command.data)}"
                        )
                stop.wait(max(0.0, min(next_audio, next_video) - time.monotonic()))
        except Exception as exc:
            if not stop.is_set():
                print(f"business session ended: session={session_id} error={exc}")
        finally:
            session.close()
            with self.lock:
                current = self.sessions.get(session_id)
                if current is not None and current[0] is session:
                    self.sessions.pop(session_id, None)
                self.threads.discard(threading.current_thread())

    @staticmethod
    def _sine_pcm(frame: int) -> bytes:
        sample_rate = 8000
        samples_per_frame = sample_rate // 50
        start = frame * samples_per_frame
        samples = (
            int(math.sin(2 * math.pi * 440 * (start + index) / sample_rate) * 6000)
            for index in range(samples_per_frame)
        )
        return struct.pack(f"<{samples_per_frame}h", *samples)

    @staticmethod
    def _write(handler, status, headers, body) -> None:
        handler.send_response(status)
        for key, value in headers.items():
            handler.send_header(key, value)
        handler.send_header("Content-Length", str(len(body)))
        handler.end_headers()
        if body:
            handler.wfile.write(body)

    def shutdown(self) -> None:
        self.echo.shutdown()
        with self.lock:
            sessions = list(self.sessions.values())
            self.sessions.clear()
        for session, stop in sessions:
            stop.set()
            session.close()
        with self.lock:
            threads = list(self.threads)
        for thread in threads:
            if thread is not threading.current_thread():
                thread.join()


def main() -> None:
    parser = argparse.ArgumentParser(description="TiRTC WHIP Python 快速开始")
    parser.add_argument("--listen", default=":8080")
    parser.add_argument("--service", required=True)
    parser.add_argument("--access-key", required=True)
    parser.add_argument("--public-key", required=True)
    parser.add_argument("--candidate", default="")
    args = parser.parse_args()
    host, port = args.listen.rsplit(":", 1)

    tirtcx.init()
    tirtcx.start()
    service = Service(
        args.service, args.access_key, args.public_key, args.candidate
    )

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            if urlsplit(self.path).path != "/whip":
                self.send_error(404)
                return
            service.post(self)

        def do_DELETE(self):
            service.delete(self)

    server = ThreadingHTTPServer((host, int(port)), Handler)
    signal.signal(
        signal.SIGTERM,
        lambda *_: threading.Thread(target=server.shutdown, daemon=True).start(),
    )
    try:
        server.serve_forever()
    finally:
        service.shutdown()
        tirtcx.stop()
        tirtcx.uninit()


if __name__ == "__main__":
    main()
