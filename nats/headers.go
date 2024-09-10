package nats

import "github.com/nats-io/nats.go"

// Default headers which published in messages from API backend
const (
	RegionHdr     = "region"
	RequestIdHdr  = "x-request-id"
	CompanyIdHdr  = "x-company-id"
	UserIdHdr     = "x-user-id"
	LocationIdHdr = "x-location-id"
	SessionIdHdr  = "x-session-id"
)

// Default nats headers
const (
	ContentEncodingHdr = "content-encoding"
)

// getters

func getRegionFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, RegionHdr)
}

func getRequestIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, RequestIdHdr)
}

func getCompanyIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, CompanyIdHdr)
}

func getLocationIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, LocationIdHdr)
}

func getUserIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, UserIdHdr)
}

func getSessionIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, SessionIdHdr)
}

func getMsgIdFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, nats.MsgIdHdr)
}

func getContentEncodingFromHeader(m *nats.Msg) string {
	return getHeaderValue(m, ContentEncodingHdr)
}

func getHeaderValue(m *nats.Msg, header string) string {
	return m.Header.Get(header)
}

// setters

func setRegionHeader(m *nats.Msg, value string) {
	setHeader(m, RegionHdr, value)
}

func setRequestIdHeader(m *nats.Msg, value string) {
	setHeader(m, RequestIdHdr, value)
}

func setCompanyIdHeader(m *nats.Msg, value string) {
	setHeader(m, CompanyIdHdr, value)
}

func setLocationIdHeader(m *nats.Msg, value string) {
	setHeader(m, LocationIdHdr, value)
}

func setUserIdHeader(m *nats.Msg, value string) {
	setHeader(m, UserIdHdr, value)
}

func setSessionIdHeader(m *nats.Msg, value string) {
	setHeader(m, SessionIdHdr, value)
}

func setMsgIdHeader(m *nats.Msg, value string) {
	setHeader(m, nats.MsgIdHdr, value)
}

func setContentEncodingHeader(m *nats.Msg, value string) {
	setHeader(m, ContentEncodingHdr, value)
}

func setHeader(m *nats.Msg, header string, value string) {
	m.Header.Set(header, value)
}
