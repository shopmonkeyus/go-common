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

func GetRegionFromHeader(m *nats.Msg) string {
	return GetHeaderValue(m, RegionHdr)
}

func GetRequestIdFromHeader(m *nats.Msg) string {
	return GetHeaderValue(m, RequestIdHdr)
}

func GetCompanyIdFromHeader(m *nats.Msg) string {
	return GetHeaderValue(m, CompanyIdHdr)
}

func GetLocationIdFromHeader(m *nats.Msg) string {
	return GetHeaderValue(m, LocationIdHdr)
}

func GetUserIdFromHeader(m *nats.Msg) string {
	return GetHeaderValue(m, UserIdHdr)
}

func GetSessionIdFromHeader(m *nats.Msg) string {
	return GetHeaderValue(m, SessionIdHdr)
}

func GetMsgIdFromHeader(m *nats.Msg) string {
	return GetHeaderValue(m, nats.MsgIdHdr)
}

func GetContentEncodingFromHeader(m *nats.Msg) string {
	return GetHeaderValue(m, ContentEncodingHdr)
}

func GetHeaderValue(m *nats.Msg, header string) string {
	return m.Header.Get(header)
}

// setters

func SetRegionHeader(m *nats.Msg, value string) {
	SetHeader(m, RegionHdr, value)
}

func SetRequestIdHeader(m *nats.Msg, value string) {
	SetHeader(m, RequestIdHdr, value)
}

func SetCompanyIdHeader(m *nats.Msg, value string) {
	SetHeader(m, CompanyIdHdr, value)
}

func SetLocationIdHeader(m *nats.Msg, value string) {
	SetHeader(m, LocationIdHdr, value)
}

func SetUserIdHeader(m *nats.Msg, value string) {
	SetHeader(m, UserIdHdr, value)
}

func SetSessionIdHeader(m *nats.Msg, value string) {
	SetHeader(m, SessionIdHdr, value)
}

func SetMsgIdHeader(m *nats.Msg, value string) {
	SetHeader(m, nats.MsgIdHdr, value)
}

func SetContentEncodingHeader(m *nats.Msg, value string) {
	SetHeader(m, ContentEncodingHdr, value)
}

func SetHeader(m *nats.Msg, header string, value string) {
	m.Header.Set(header, value)
}
