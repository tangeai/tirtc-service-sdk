import json
import time

from tirtcx import Conn, ConnEventOptions
from whipecho import CloseReason, HTTPAdapter, Observer
from whipecho.echo import _Session


class FakeBackend:
    def disconnect(self, handle):
        pass

    def send_frame(self, function, handle, info, data):
        pass

    def send_command(self, handle, command, data):
        pass

    def call_stream(self, function, handle, stream_id):
        pass

    def send_buffer_used(self, handle):
        return 0


class FakeSession:
    answer_sdp = b"answer"

    def __init__(self):
        self.conn = Conn(
            FakeBackend(),
            ConnEventOptions(
                audio_buffer=16, video_buffer=16, command_buffer=16
            ),
        )
        self.conn._bind(1)

    def wait_conn(self, timeout):
        return self.conn

    def close(self):
        self.conn._finish(None)


class FakeAcceptor:
    def whip_accept(self, offer, options, timeout):
        assert offer == b"offer"
        return FakeSession()


class RecordingObserver(Observer):
    def __init__(self):
        self.accepted = 0
        self.connected = 0
        self.closed = []
        self.dropped = []

    def session_accepted(self):
        self.accepted += 1

    def session_connected(self, duration_seconds):
        self.connected += 1

    def session_closed(self, reason, duration_seconds):
        self.closed.append(reason)

    def events_dropped(self, event, count):
        self.dropped.append((event, count))


def test_http_adapter_delegates_business_and_manages_resource():
    adapter = HTTPAdapter(FakeAcceptor(), "/whip/echo/resource")
    assert adapter.try_handle_post("", "application/sdp", b"offer") is None
    response = adapter.try_handle_post(
        "_tg_mode=echo", "application/sdp; charset=utf-8", b"offer"
    )
    assert response.status == 201
    assert response.body == b"answer"
    assert response.headers["Location"].startswith("/whip/echo/resource/")
    session_id = response.headers["Location"].rsplit("/", 1)[1]
    assert adapter.serve_delete(session_id).status == 204
    adapter.shutdown()


def test_http_adapter_validates_mode_type_and_size():
    adapter = HTTPAdapter(FakeAcceptor(), "/resource", max_sdp_bytes=3)
    assert adapter.try_handle_post("_tg_mode=bad", "application/sdp", b"x").status == 400
    assert adapter.try_handle_post("_tg_mode=echo", "text/plain", b"x").status == 415
    assert adapter.try_handle_post("_tg_mode=echo", "application/sdp", b"xxxx").status == 413
    assert adapter.try_handle_post("_tg_mode=%zz", "application/sdp", b"x").status == 400
    adapter.shutdown()


def test_time_sync_response_matches_go_schema():
    payload = _Session._time_sync_payload(
        json.dumps(
            {"version": 1, "seq": 7, "client_send_unix_ns": 123}
        ).encode()
    )
    value = json.loads(payload)
    assert value["version"] == 1
    assert value["seq"] == 7
    assert value["client_send_unix_ns"] == 123
    assert value["server_recv_unix_ns"] > 0


def test_observer_stats_and_worker_threads_are_reclaimed():
    observer = RecordingObserver()
    adapter = HTTPAdapter(
        FakeAcceptor(), "/whip/echo/resource", observer=observer
    )
    response = adapter.try_handle_post(
        "_tg_mode=echo", "application/sdp", b"offer"
    )
    session_id = response.headers["Location"].rsplit("/", 1)[1]
    deadline = time.monotonic() + 1
    while observer.connected == 0 and time.monotonic() < deadline:
        time.sleep(0.001)
    assert observer.accepted == 1
    assert observer.connected == 1

    assert adapter.serve_delete(session_id).status == 204
    adapter.shutdown()
    stats = adapter.stats()
    assert stats.active_sessions == 0
    assert stats.accepted_total == 1
    assert stats.deleted_total == 1
    assert stats.closed_total[CloseReason.DELETED] == 1
    assert observer.closed == [CloseReason.DELETED]
    assert not adapter.manager._threads
