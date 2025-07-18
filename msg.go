package netconf

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"golang.org/x/exp/slices"
)

type RawXML []byte

func (x *RawXML) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var inner struct {
		Data []byte `xml:",innerxml"`
	}

	if err := d.DecodeElement(&inner, &start); err != nil {
		return err
	}

	*x = inner.Data
	return nil
}

func (x *RawXML) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	inner := struct {
		Data []byte `xml:",innerxml"`
	}{
		Data: []byte(*x),
	}
	return e.EncodeElement(&inner, start)
}

type RPC struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 rpc"`
	MessageID uint64   `xml:"message-id,attr"`
	Operation any      `xml:",innerxml"`
}

func (msg *RPC) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	if msg.Operation == nil {
		return fmt.Errorf("operation cannot be nil")
	}

	type rpcMsg RPC
	inner := rpcMsg(*msg)
	return e.Encode(&inner)
}

type Hello struct {
	XMLName      xml.Name
	SessionID    uint64   `xml:"session-id,omitempty"`
	Capabilities []string `xml:"capabilities>capability"`
}

type RPCReply struct {
	XMLName   xml.Name
	MessageID uint64    `xml:"message-id,attr"`
	Errors    RPCErrors `xml:"rpc-error,omitempty"`
	rpc       []byte
}

func (r RPCReply) Decode(v any) error {
	return xml.Unmarshal(r.rpc, v)
}

func (r RPCReply) String() string {
	return string(r.rpc)
}

func (r RPCReply) Raw() []byte {
	return r.rpc
}

func (r RPCReply) Err(severity ...ErrSeverity) error {
	if len(r.Errors) == 0 {
		return nil
	}

	errs := r.Errors.Filter(severity...)
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return errs
	}
}

type Notification struct {
	XMLName   xml.Name
	EventTime time.Time `xml:"eventTime"`
	rpc       []byte
}

func (r Notification) Decode(v any) error {
	return xml.Unmarshal(r.rpc, v)
}

func (r Notification) String() string {
	return string(r.rpc)
}

func (r Notification) Raw() []byte {
	return r.rpc
}

type ErrSeverity string

const (
	SevError   ErrSeverity = "error"
	SevWarning ErrSeverity = "warning"
)

type ErrType string

const (
	ErrTypeTransport ErrType = "transport"
	ErrTypeRPC       ErrType = "rpc"
	ErrTypeProtocol  ErrType = "protocol"
	ErrTypeApp       ErrType = "app"
)

type ErrTag string

const (
	ErrInUse                 ErrTag = "in-use"
	ErrInvalidValue          ErrTag = "invalid-value"
	ErrTooBig                ErrTag = "too-big"
	ErrMissingAttribute      ErrTag = "missing-attribute"
	ErrBadAttribute          ErrTag = "bad-attribute"
	ErrUnknownAttribute      ErrTag = "unknown-attribute"
	ErrMissingElement        ErrTag = "missing-element"
	ErrBadElement            ErrTag = "bad-element"
	ErrUnknownElement        ErrTag = "unknown-element"
	ErrUnknownNamespace      ErrTag = "unknown-namespace"
	ErrAccessDenied          ErrTag = "access-denied"
	ErrLockDenied            ErrTag = "lock-denied"
	ErrResourceDenied        ErrTag = "resource-denied"
	ErrRollbackFailed        ErrTag = "rollback-failed"
	ErrDataExists            ErrTag = "data-exists"
	ErrDataMissing           ErrTag = "data-missing"
	ErrOperationNotSupported ErrTag = "operation-not-supported"
	ErrOperationFailed       ErrTag = "operation-failed"
	ErrPartialOperation      ErrTag = "partial-operation"
	ErrMalformedMessage      ErrTag = "malformed-message"
)

type RPCError struct {
	Type     ErrType     `xml:"error-type" json:"error-type"`
	Tag      ErrTag      `xml:"error-tag" json:"error-tag"`
	Severity ErrSeverity `xml:"error-severity" json:"error-severity"`
	AppTag   string      `xml:"error-app-tag,omitempty" json:"error-app-tag,omitempty"`
	Path     string      `xml:"error-path,omitempty" json:"error-path,omitempty"`
	Message  string      `xml:"error-message,omitempty" json:"error-message,omitempty"`
	Info     RawXML      `xml:"error-info,omitempty" json:"error-info,omitempty"`
}

func (e RPCError) Error() string {
	if e.Message != "" {
		return e.Message
	} else {
		return string(e.Info)
	}
}

type RPCErrors []RPCError

func (errs RPCErrors) Filter(severity ...ErrSeverity) RPCErrors {
	if len(errs) == 0 {
		return nil
	}

	if len(severity) == 0 {
		severity = []ErrSeverity{SevError}
	}

	filteredErrs := make(RPCErrors, 0, len(errs))
	for _, err := range errs {
		if slices.Contains(severity, err.Severity) {
			filteredErrs = append(filteredErrs, err)
		}
	}
	return filteredErrs
}

func (errs RPCErrors) Error() string {
	var sb strings.Builder
	for i, err := range errs {
		if i > 0 {
			sb.WriteRune('\n')
		}
		sb.WriteString(err.Error())
	}
	return sb.String()
}

func (errs RPCErrors) Unwrap() []error {
	boxedErrs := make([]error, len(errs))
	for i, err := range errs {
		boxedErrs[i] = err
	}
	return boxedErrs
}
