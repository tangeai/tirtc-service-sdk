//go:build tirtc_clib

#include "tiRTC.h"
#include "_cgo_export.h"

static void c_rtc_on_event(int event, const void* data, int len) {
    goRtcOnEvent(event, (void*)data, len);
}

static void c_rtc_on_conn_accepted(tirtc_conn_t hconn) {
    goRtcOnConnAccepted(hconn);
}

static void c_rtc_on_conn_error(tirtc_conn_t hconn, int error) {
    goRtcOnConnError(hconn, error);
}

static void c_rtc_on_disconnected(tirtc_conn_t hconn) {
    goRtcOnDisconnected(hconn);
}

static void c_rtc_on_audio(tirtc_conn_t hconn, const TIRTCFRAMEINFO* pFi, void* data) {
    goRtcOnAudio(hconn, (void*)pFi, data);
}

static void c_rtc_on_video(tirtc_conn_t hconn, const TIRTCFRAMEINFO* pFi, void* data) {
    goRtcOnVideo(hconn, (void*)pFi, data);
}

static void c_rtc_on_message(tirtc_conn_t hconn, const TIRTCFRAMEINFO* pFi, void* data) {
    goRtcOnMessage(hconn, (void*)pFi, data);
}

static void c_rtc_on_command(tirtc_conn_t hconn, uint32_t cmd, const void* data, uint32_t len) {
    goRtcOnCommand(hconn, cmd, (void*)data, len);
}

static void c_rtc_on_request_key_frame(tirtc_conn_t hconn, uint8_t stream_id) {
    goRtcOnRequestKeyFrame(hconn, stream_id);
}

static int c_rtc_on_subscribe_video(tirtc_conn_t hconn, uint8_t stream_id) {
    return goRtcOnSubscribeVideo(hconn, stream_id);
}

static void c_rtc_on_unsubscribe_video(tirtc_conn_t hconn, uint8_t stream_id) {
    goRtcOnUnsubscribeVideo(hconn, stream_id);
}

static int c_rtc_on_subscribe_audio(tirtc_conn_t hconn, uint8_t stream_id) {
    return goRtcOnSubscribeAudio(hconn, stream_id);
}

static void c_rtc_on_unsubscribe_audio(tirtc_conn_t hconn, uint8_t stream_id) {
    goRtcOnUnsubscribeAudio(hconn, stream_id);
}

static TIRTCCALLBACKS g_rtc_callbacks;

TIRTCCALLBACKS* bridge_get_rtc_callbacks() {
    g_rtc_callbacks.on_event = c_rtc_on_event;
    g_rtc_callbacks.on_conn_accepted = c_rtc_on_conn_accepted;
    g_rtc_callbacks.on_conn_error = c_rtc_on_conn_error;
    g_rtc_callbacks.on_disconnected = c_rtc_on_disconnected;
    g_rtc_callbacks.on_audio = c_rtc_on_audio;
    g_rtc_callbacks.on_video = c_rtc_on_video;
    g_rtc_callbacks.on_message = c_rtc_on_message;
    g_rtc_callbacks.on_command = c_rtc_on_command;
    g_rtc_callbacks.on_request_key_frame = c_rtc_on_request_key_frame;
    g_rtc_callbacks.on_subscribe_video = c_rtc_on_subscribe_video;
    g_rtc_callbacks.on_unsubscribe_video = c_rtc_on_unsubscribe_video;
    g_rtc_callbacks.on_subscribe_audio = c_rtc_on_subscribe_audio;
    g_rtc_callbacks.on_unsubscribe_audio = c_rtc_on_unsubscribe_audio;
    return &g_rtc_callbacks;
}
