import threading

import pytest

from tirtcx import Conn, ConnEventOptions, FrameInfo, WhipAcceptor


class FakeBackend:
    def __init__(self):
        self.connections = {}
        self.disconnected = []
        self.frames = []
        self.commands = []

    def whip_accept(self, offer, candidate, on_sdp, on_connect):
        assert offer == b"offer"
        assert candidate == "203.0.113.10"
        on_sdp(b"answer")
        threading.Timer(0.01, lambda: on_connect(0, 42)).start()
        return lambda: None

    def register_conn(self, handle, conn):
        self.connections[handle] = conn

    def disconnect(self, handle):
        self.disconnected.append(handle)

    def send_frame(self, function, handle, info, data):
        self.frames.append((function, handle, info, data))

    def send_command(self, handle, command, data):
        self.commands.append((handle, command, data))

    def call_stream(self, function, handle, stream_id):
        pass

    def send_buffer_used(self, handle):
        return 7

    def error(self, code):
        return RuntimeError(f"native error {code}")


def test_accept_wait_and_connection_operations():
    backend = FakeBackend()
    session = WhipAcceptor("203.0.113.10", backend).whip_accept(
        b"offer", ConnEventOptions(command_buffer=1)
    )
    assert session.answer_sdp == b"answer"
    conn = session.wait_conn(1)
    assert conn.handle == 42
    conn.send_audio(FrameInfo(media=1), b"pcm")
    conn.send_command(0x10000, b"hello")
    assert conn.send_buffer_used() == 7
    session.close()
    assert backend.disconnected == [42]


def test_empty_offer_is_rejected():
    with pytest.raises(ValueError):
        WhipAcceptor(backend=FakeBackend()).whip_accept(b"")


def test_connect_error_before_sdp_is_returned_immediately():
    backend = FakeBackend()

    def fail(offer, candidate, on_sdp, on_connect):
        on_connect(-40012, 0)
        return lambda: None

    backend.whip_accept = fail
    with pytest.raises(RuntimeError, match="-40012"):
        WhipAcceptor(backend=backend).whip_accept(b"offer", timeout=1)


def test_connect_before_sdp_registers_connection_immediately():
    backend = FakeBackend()

    def connect_first(offer, candidate, on_sdp, on_connect):
        on_connect(0, 42)
        assert backend.connections[42].handle == 42
        on_sdp(b"answer")
        return lambda: None

    backend.whip_accept = connect_first
    session = WhipAcceptor(backend=backend).whip_accept(b"offer")
    assert session.wait_conn(1).handle == 42


def test_full_error_queue_updates_error_dropped_counter():
    conn = Conn(FakeBackend(), ConnEventOptions(error_buffer=1))
    for index in range(5):
        conn._put("errors", RuntimeError(str(index)))
    assert conn.event_stats().error_dropped == 1
