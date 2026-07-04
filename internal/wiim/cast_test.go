package wiim

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

const castGoldenMessageHex = "0800120873656e6465722d301a0a72656365697665722d30222075726e3a782d636173743a636f6d2e676f6f676c652e636173742e6d65646961280032237b2274797065223a224745545f535441545553222c22726571756573744964223a327d"

func TestCastMessageGoldenWireFormat(t *testing.T) {
	version := int32(0)
	source := "sender-0"
	dest := "receiver-0"
	namespace := castNamespaceMedia
	payloadType := int32(0)
	payload := `{"type":"GET_STATUS","requestId":2}`

	got, err := castMarshalMessage(&castMessage{
		ProtocolVersion: &version,
		SourceID:        &source,
		DestinationID:   &dest,
		Namespace:       &namespace,
		PayloadType:     &payloadType,
		PayloadUtf8:     &payload,
	})
	if err != nil {
		t.Fatalf("castMarshalMessage returned error: %v", err)
	}
	want, err := hex.DecodeString(castGoldenMessageHex)
	if err != nil {
		t.Fatalf("bad golden hex: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded message mismatch\ngot  %x\nwant %x", got, want)
	}

	decoded, err := castUnmarshalMessage(want)
	if err != nil {
		t.Fatalf("castUnmarshalMessage returned error: %v", err)
	}
	if decoded.ProtocolVersion == nil || *decoded.ProtocolVersion != version {
		t.Fatalf("protocol_version = %v, want %d", decoded.ProtocolVersion, version)
	}
	if decoded.SourceID == nil || *decoded.SourceID != source {
		t.Fatalf("source_id = %v, want %q", decoded.SourceID, source)
	}
	if decoded.DestinationID == nil || *decoded.DestinationID != dest {
		t.Fatalf("destination_id = %v, want %q", decoded.DestinationID, dest)
	}
	if decoded.Namespace == nil || *decoded.Namespace != namespace {
		t.Fatalf("namespace = %v, want %q", decoded.Namespace, namespace)
	}
	if decoded.PayloadType == nil || *decoded.PayloadType != payloadType {
		t.Fatalf("payload_type = %v, want %d", decoded.PayloadType, payloadType)
	}
	if decoded.PayloadUtf8 == nil || *decoded.PayloadUtf8 != payload {
		t.Fatalf("payload_utf8 = %v, want %q", decoded.PayloadUtf8, payload)
	}
}

func TestCastFramingRoundTripOverPipe(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	writeErr := make(chan error, 1)
	go func() {
		defer client.Close()
		writeErr <- castSend(client, "sender-0", "receiver-0", castNamespaceReceiver, map[string]any{"type": "GET_STATUS", "requestId": 1})
	}()

	_ = server.SetReadDeadline(time.Now().Add(time.Second))
	var size [4]byte
	if _, err := io.ReadFull(server, size[:]); err != nil {
		t.Fatalf("ReadFull(size) returned error: %v", err)
	}
	messageLength := binary.BigEndian.Uint32(size[:])
	if messageLength == 0 || messageLength > castMaxMessageLength {
		t.Fatalf("message length = %d, want 1..%d", messageLength, castMaxMessageLength)
	}
	data := make([]byte, messageLength)
	if _, err := io.ReadFull(server, data); err != nil {
		t.Fatalf("ReadFull(payload) returned error: %v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("castSend returned error: %v", err)
	}
	msg, err := castUnmarshalMessage(data)
	if err != nil {
		t.Fatalf("castUnmarshalMessage returned error: %v", err)
	}
	if msg.PayloadUtf8 == nil || *msg.PayloadUtf8 != `{"requestId":1,"type":"GET_STATUS"}` {
		t.Fatalf("payload_utf8 = %v", msg.PayloadUtf8)
	}
}

func TestCastOversizedLengthRejected(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	writeErr := make(chan error, 1)
	go func() {
		defer client.Close()
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], castMaxMessageLength+1)
		_, err := client.Write(size[:])
		writeErr <- err
	}()

	_ = server.SetReadDeadline(time.Now().Add(time.Second))
	_, err := castReadType(server, "MEDIA_STATUS")
	if err == nil {
		t.Fatal("castReadType returned nil error for oversized frame")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("error = %q, want exceeds maximum", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("writing oversized length returned error: %v", err)
	}
}

func TestCastMediaInfo(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    CastMediaInfo
	}{
		{
			name:    "idle state",
			payload: map[string]any{"status": []any{}},
			want:    CastMediaInfo{},
		},
		{
			name: "populated media",
			payload: map[string]any{"status": []any{map[string]any{
				"playerState": "PLAYING",
				"media": map[string]any{
					"contentId":   "track-1",
					"contentType": "audio/flac",
					"metadata": map[string]any{
						"title":     " Song ",
						"artist":    "Artist",
						"albumName": "Album",
						"images":    []any{map[string]any{"url": "http://example.invalid/image.jpg"}},
					},
				},
			}}},
			want: CastMediaInfo{
				Title:       "Song",
				Artist:      "Artist",
				Album:       "Album",
				ImageURL:    "http://example.invalid/image.jpg",
				PlayerState: "PLAYING",
				ContentID:   "track-1",
				ContentType: "audio/flac",
				RawMetadata: map[string]any{
					"title":     " Song ",
					"artist":    "Artist",
					"albumName": "Album",
					"images":    []any{map[string]any{"url": "http://example.invalid/image.jpg"}},
				},
			},
		},
		{
			name: "missing fields",
			payload: map[string]any{"status": []any{map[string]any{
				"playerState": "IDLE",
			}}},
			want: CastMediaInfo{PlayerState: "IDLE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := castMediaInfo(tt.payload)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("castMediaInfo() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCastReceiverApp(t *testing.T) {
	tests := []struct {
		name          string
		payload       map[string]any
		wantName      string
		wantTransport string
	}{
		{name: "idle state", payload: map[string]any{"status": map[string]any{}}, wantName: "", wantTransport: ""},
		{name: "populated app", payload: map[string]any{"status": map[string]any{"applications": []any{map[string]any{"displayName": "WiiM", "transportId": "transport-1"}}}}, wantName: "WiiM", wantTransport: "transport-1"},
		{name: "missing fields", payload: map[string]any{"status": map[string]any{"applications": []any{map[string]any{}}}}, wantName: "", wantTransport: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotTransport := castReceiverApp(tt.payload)
			if gotName != tt.wantName || gotTransport != tt.wantTransport {
				t.Fatalf("castReceiverApp() = %q, %q; want %q, %q", gotName, gotTransport, tt.wantName, tt.wantTransport)
			}
		})
	}
}

func TestCastReadTypeIterationCap(t *testing.T) {
	var buf bytes.Buffer
	for i := 0; i < 20; i++ {
		if err := writeCastTestFrame(&buf, map[string]any{"type": "IGNORED", "requestId": i}); err != nil {
			t.Fatalf("writeCastTestFrame(%d) returned error: %v", i, err)
		}
	}
	if _, err := castReadType(&buf, "MEDIA_STATUS"); err == nil || !strings.Contains(err.Error(), "did not return MEDIA_STATUS") {
		t.Fatalf("castReadType error = %v, want iteration cap error", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("buffer has %d unread bytes after 20 iterations", buf.Len())
	}
}

func writeCastTestFrame(w io.Writer, payload map[string]any) error {
	return castSend(w, "sender-0", "receiver-0", castNamespaceMedia, payload)
}
