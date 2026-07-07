//go:build tirtc_clib

#include "tiRTC.h"
#include "_cgo_export.h"

static void c_whip_accept_on_sdp(const char *sdp, int len, void *user) {
    goWhipAcceptOnSDP((char*)sdp, len, user);
}

static void c_whip_accept_on_connect(int error, tirtc_conn_t hconn, void *user_data) {
    goWhipAcceptOnConnect(error, hconn, user_data);
}

int bridge_whip_accept(const char *offer, int offer_len, const char *candidate, void *context) {
    return TiRtcWhipAccept(
        offer,
        offer_len,
        candidate,
        c_whip_accept_on_sdp,
        c_whip_accept_on_connect,
        context
    );
}

static void c_whip_connect_on_connect(int error, tirtc_conn_t hconn, void *user_data) {
    goWhipConnectOnConnect(error, hconn, user_data);
}

int bridge_whip_connect(const char *service_desc, const char *token, void *context) {
    return TiRtcWhipConnect(service_desc, token, c_whip_connect_on_connect, context);
}
