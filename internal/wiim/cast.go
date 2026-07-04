package wiim

import (
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	castNamespaceConnection = "urn:x-cast:com.google.cast.tp.connection"
	castNamespaceReceiver   = "urn:x-cast:com.google.cast.receiver"
	castNamespaceMedia      = "urn:x-cast:com.google.cast.media"
	castMaxMessageLength    = 1 << 20
)

// castMessage represents a Google Cast protocol message with protobuf-encoded fields.
type castMessage struct {
	ProtocolVersion *int32
	SourceID        *string
	DestinationID   *string
	Namespace       *string
	PayloadType     *int32
	PayloadUtf8     *string
}

func castMarshalMessage(msg *castMessage) ([]byte, error) {
	if msg.ProtocolVersion == nil || msg.SourceID == nil || msg.DestinationID == nil || msg.Namespace == nil || msg.PayloadType == nil {
		return nil, errors.New("missing required Cast message field")
	}
	var data []byte
	data = protowire.AppendTag(data, 1, protowire.VarintType)
	if *msg.ProtocolVersion < 0 {
		return nil, errors.New("negative ProtocolVersion")
	}
	data = protowire.AppendVarint(data, uint64(*msg.ProtocolVersion))
	data = protowire.AppendTag(data, 2, protowire.BytesType)
	data = protowire.AppendString(data, *msg.SourceID)
	data = protowire.AppendTag(data, 3, protowire.BytesType)
	data = protowire.AppendString(data, *msg.DestinationID)
	data = protowire.AppendTag(data, 4, protowire.BytesType)
	data = protowire.AppendString(data, *msg.Namespace)
	data = protowire.AppendTag(data, 5, protowire.VarintType)
	if *msg.PayloadType < 0 {
		return nil, errors.New("negative PayloadType")
	}
	data = protowire.AppendVarint(data, uint64(*msg.PayloadType))
	if msg.PayloadUtf8 != nil {
		data = protowire.AppendTag(data, 6, protowire.BytesType)
		data = protowire.AppendString(data, *msg.PayloadUtf8)
	}
	return data, nil
}

func castUnmarshalMessage(data []byte) (castMessage, error) {
	var msg castMessage
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return msg, protowire.ParseError(n)
		}
		data = data[n:]
		switch num {
		case 1:
			if typ != protowire.VarintType {
				return msg, errors.New("invalid Cast protocol_version field type")
			}
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return msg, protowire.ParseError(n)
			}
			if v > 1<<31-1 {
				return msg, errors.New("ProtocolVersion overflows int32")
			}
			value := int32(v)
			msg.ProtocolVersion = &value
			data = data[n:]
		case 2:
			value, rest, err := castConsumeString(data, typ)
			if err != nil {
				return msg, err
			}
			msg.SourceID = &value
			data = rest
		case 3:
			value, rest, err := castConsumeString(data, typ)
			if err != nil {
				return msg, err
			}
			msg.DestinationID = &value
			data = rest
		case 4:
			value, rest, err := castConsumeString(data, typ)
			if err != nil {
				return msg, err
			}
			msg.Namespace = &value
			data = rest
		case 5:
			if typ != protowire.VarintType {
				return msg, errors.New("invalid Cast payload_type field type")
			}
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return msg, protowire.ParseError(n)
			}
			if v > 1<<31-1 {
				return msg, errors.New("PayloadType overflows int32")
			}
			value := int32(v)
			msg.PayloadType = &value
			data = data[n:]
		case 6:
			value, rest, err := castConsumeString(data, typ)
			if err != nil {
				return msg, err
			}
			msg.PayloadUtf8 = &value
			data = rest
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return msg, protowire.ParseError(n)
			}
			data = data[n:]
		}
	}
	return msg, nil
}

func castConsumeString(data []byte, typ protowire.Type) (string, []byte, error) {
	if typ != protowire.BytesType {
		return "", nil, errors.New("invalid Cast string field type")
	}
	value, n := protowire.ConsumeString(data)
	if n < 0 {
		return "", nil, protowire.ParseError(n)
	}
	return value, data[n:], nil
}

// CastMediaInfo holds metadata about media currently playing via Google Cast.
type CastMediaInfo struct {
	App         string         `json:"app,omitempty"`
	Title       string         `json:"title,omitempty"`
	Artist      string         `json:"artist,omitempty"`
	Album       string         `json:"album,omitempty"`
	ImageURL    string         `json:"imageURL,omitempty"`
	PlayerState string         `json:"playerState,omitempty"`
	ContentID   string         `json:"contentID,omitempty"`
	ContentType string         `json:"contentType,omitempty"`
	RawMetadata map[string]any `json:"rawMetadata,omitempty"`
}

// CastMediaStatus connects to the WiiM's Cast endpoint (port 8009) over TLS,
// negotiates a Cast protocol session, and retrieves the current media status.
// The timeout specifies both the connection and read deadlines.
func CastMediaStatus(host string, timeoutSeconds float64) (CastMediaInfo, error) {
	dialer := &net.Dialer{Timeout: time.Duration(timeoutSeconds * float64(time.Second))}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, "8009"), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return CastMediaInfo{}, runtimef("could not connect to Cast endpoint %s:8009 within %.1fs: %v", host, timeoutSeconds, err)
		}
		return CastMediaInfo{}, runtimef("could not connect to Cast endpoint %s:8009: %v", host, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Duration(timeoutSeconds * float64(time.Second))))

	if err := castSend(conn, "sender-0", "receiver-0", castNamespaceConnection, map[string]any{"type": "CONNECT"}); err != nil {
		return CastMediaInfo{}, err
	}
	if err := castSend(conn, "sender-0", "receiver-0", castNamespaceReceiver, map[string]any{"type": "GET_STATUS", "requestId": 1}); err != nil {
		return CastMediaInfo{}, err
	}
	receiver, err := castReadType(conn, "RECEIVER_STATUS")
	if err != nil {
		return CastMediaInfo{}, err
	}
	appName, transportID := castReceiverApp(receiver)
	if transportID == "" {
		return CastMediaInfo{}, runtimef("no active Cast media session")
	}
	if err := castSend(conn, "sender-0", transportID, castNamespaceConnection, map[string]any{"type": "CONNECT"}); err != nil {
		return CastMediaInfo{}, err
	}
	if err := castSend(conn, "sender-0", transportID, castNamespaceMedia, map[string]any{"type": "GET_STATUS", "requestId": 2}); err != nil {
		return CastMediaInfo{}, err
	}
	media, err := castReadType(conn, "MEDIA_STATUS")
	if err != nil {
		return CastMediaInfo{}, err
	}
	info := castMediaInfo(media)
	info.App = appName
	return info, nil
}

func castSend(w io.Writer, source, dest, namespace string, payload map[string]any) error {
	version := int32(0)
	payloadType := int32(0)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	payloadString := string(payloadBytes)
	msg := &castMessage{ProtocolVersion: &version, SourceID: &source, DestinationID: &dest, Namespace: &namespace, PayloadType: &payloadType, PayloadUtf8: &payloadString}
	data, err := castMarshalMessage(msg)
	if err != nil {
		return err
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(data))) // #nosec G115 -- message data is bounded by castMaxMessageLength
	if _, err := w.Write(size[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func castReadType(r io.Reader, want string) (map[string]any, error) {
	for i := 0; i < 20; i++ {
		var size [4]byte
		if _, err := io.ReadFull(r, size[:]); err != nil {
			return nil, runtimef("could not read Cast response: %v", err)
		}
		messageLength := binary.BigEndian.Uint32(size[:])
		if messageLength > castMaxMessageLength {
			return nil, runtimef("Cast response length %d exceeds maximum %d", messageLength, castMaxMessageLength)
		}
		data := make([]byte, messageLength)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, runtimef("could not read Cast payload: %v", err)
		}
		msg, err := castUnmarshalMessage(data)
		if err != nil || msg.PayloadUtf8 == nil {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(*msg.PayloadUtf8), &payload); err != nil {
			continue
		}
		if payload["type"] == want {
			return payload, nil
		}
	}
	return nil, runtimef("Cast endpoint did not return %s", want)
}

func castReceiverApp(payload map[string]any) (string, string) {
	status, _ := payload["status"].(map[string]any)
	apps, _ := status["applications"].([]any)
	if len(apps) == 0 {
		return "", ""
	}
	app, _ := apps[0].(map[string]any)
	return stringValue(app["displayName"]), stringValue(app["transportId"])
}

func castMediaInfo(payload map[string]any) CastMediaInfo {
	statuses, _ := payload["status"].([]any)
	if len(statuses) == 0 {
		return CastMediaInfo{}
	}
	status, _ := statuses[0].(map[string]any)
	media, _ := status["media"].(map[string]any)
	metadata, _ := media["metadata"].(map[string]any)
	info := CastMediaInfo{
		Title:       cleanMetadataText(firstString(metadata, "title", "")),
		Artist:      cleanMetadataText(firstString(metadata, "artist", "")),
		Album:       cleanMetadataText(firstString(metadata, "albumName", firstString(metadata, "album", ""))),
		PlayerState: stringValue(status["playerState"]),
		ContentID:   stringValue(media["contentId"]),
		ContentType: stringValue(media["contentType"]),
		RawMetadata: metadata,
	}
	if images, ok := metadata["images"].([]any); ok && len(images) > 0 {
		if image, ok := images[0].(map[string]any); ok {
			info.ImageURL = stringValue(image["url"])
		}
	}
	return info
}
