package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/pion/interceptor"
	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RegisterSupportedCodecs Copied and re-worked version of func (m *MediaEngine) RegisterDefaultCodecs() error
func RegisterSupportedCodecs(m *webrtc.MediaEngine) error {
	{
		audioRTCPFeedback := []webrtc.RTCPFeedback{{"nack", ""}}

		for _, codec := range []webrtc.RTPCodecParameters{
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeOpus, 48000, 2,
					"minptime=10;useinbandfec=1", audioRTCPFeedback},
				PayloadType: 111,
			},
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeG722, 8000, 0,
					"", audioRTCPFeedback},
				PayloadType: 9,
			},
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypePCMU, 8000, 0,
					"", audioRTCPFeedback},
				PayloadType: 0,
			},
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypePCMA, 8000, 0,
					"", audioRTCPFeedback},
				PayloadType: 8,
			},
		} {
			if err := m.RegisterCodec(codec, webrtc.RTPCodecTypeAudio); err != nil {
				return err
			}
		}
	}

	videoRTCPFeedback := []webrtc.RTCPFeedback{{"goog-remb", ""}, {"ccm", "fir"},
		{"nack", ""}, {"nack", "pli"}}

	for _, codec := range []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeVP8, 90000, 0,
				"", videoRTCPFeedback},
			PayloadType: 96,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0,
				"level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f", videoRTCPFeedback},
			PayloadType: 102,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0,
				"level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f",
				videoRTCPFeedback},
			PayloadType: 104,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0,
				"level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f", videoRTCPFeedback},
			PayloadType: 106,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0,
				"level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42e01f", videoRTCPFeedback},
			PayloadType: 108,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0,
				"level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=4d001f", videoRTCPFeedback},
			PayloadType: 127,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0,
				"level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=4d001f", videoRTCPFeedback},
			PayloadType: 39,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeAV1, 90000, 0,
				"", videoRTCPFeedback},
			PayloadType: 45,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeVP9, 90000, 0,
				"profile-id=0", videoRTCPFeedback},
			PayloadType: 98,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeVP9, 90000, 0,
				"profile-id=2", videoRTCPFeedback},
			PayloadType: 100,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{webrtc.MimeTypeH264, 90000, 0,
				"level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=64001f", videoRTCPFeedback},
			PayloadType: 112,
		},
	} {
		if err := m.RegisterCodec(codec, webrtc.RTPCodecTypeVideo); err != nil {
			return err
		}
	}

	return nil
}

// DevNullWriter Ignore all input logs
type DevNullWriter struct {
}

// implements io.Writer interface
func (writer DevNullWriter) Write(p []byte) (n int, err error) { return n, nil }

type NackStatsFactory struct {
	lost atomic.Uint64
}

func (r *NackStatsFactory) updateLostPackets(u uint64) {
	r.lost.Add(u)
}

func (r *NackStatsFactory) getLost() uint64 {
	return r.lost.Swap(0)
}

type NackStatsInterceptor struct {
	factory *NackStatsFactory
	interceptor.NoOp
}

// NewInterceptor constructs a new ResponderInterceptor
func (r *NackStatsFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	i := &NackStatsInterceptor{factory: r}
	return i, nil
}

// BindRTCPWriter lets you modify any outgoing RTCP packets. It is called once per PeerConnection. The returned method
// will be called once per packet batch.
func (n *NackStatsInterceptor) BindRTCPWriter(writer interceptor.RTCPWriter) interceptor.RTCPWriter {
	return interceptor.RTCPWriterFunc(func(pkts []rtcp.Packet, attributes interceptor.Attributes) (int, error) {
		for _, p := range pkts {
			report, ok := p.(*rtcp.TransportLayerNack)
			if report == nil || !ok {
				break
			}
			lostPackets := 0

			for _, nack := range report.Nacks {
				lostPackets += len(nack.PacketList())
			}
			n.factory.updateLostPackets(uint64(lostPackets))
		}
		return writer.Write(pkts, attributes)
	})
}

func CreateWebRtcApi(factory *NackStatsFactory) *webrtc.API {
	m := &webrtc.MediaEngine{}

	if RegisterSupportedCodecs(m) != nil {
		return nil
	}

	i := &interceptor.Registry{}

	i.Add(factory)
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return nil
	}

	if err := RegisterSupportedCodecs(m); err != nil {
		return nil
	}

	s := webrtc.SettingEngine{}

	logFactory := &logging.DefaultLoggerFactory{}
	logFactory.Writer = &DevNullWriter{}
	s.LoggerFactory = logFactory

	_ = s.SetAnsweringDTLSRole(webrtc.DTLSRoleClient)

	s.SetIncludeLoopbackCandidate(true)

	return webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i),
		webrtc.WithSettingEngine(s))
}

func webrtcSession(api *webrtc.API, whepUrl string, session *atomic.Int64,
	repliedSessions *atomic.Int64, activeWebRTCSessions *atomic.Int64, traffic *atomic.Uint64) {
	defer func() {
		session.Add(-1)
	}()

	// Create new PeerConnection
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{})

	if err != nil {
		return
	}

	// When this frame returns close the PeerConnection
	defer peerConnection.Close() //nolint

	if _, err := peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		return
	}

	if _, err := peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		return
	}

	desc, err := peerConnection.CreateOffer(nil)

	if peerConnection.SetLocalDescription(desc) != nil {
		return
	}

	context := net.Dialer{
		Timeout: 1 * time.Second,
	}

	tripper := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         context.DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        1,
		IdleConnTimeout:     10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{Transport: tripper}
	resp, err := client.Post(whepUrl, "application/sdp", strings.NewReader(desc.SDP))

	if resp == nil {
		fmt.Printf("Was not able to connect to %s", whepUrl)
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		fmt.Printf("Was not able to connect to %s", whepUrl)
		return
	}

	sdpBytes, err := io.ReadAll(resp.Body)

	if err != nil || len(sdpBytes) == 0 {
		fmt.Printf("Was not able read sdp %s", whepUrl)
		return
	}

	repliedSessions.Add(1)

	if peerConnection.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: string(sdpBytes)}) != nil {
		return
	}

	sessionChannel := make(chan bool)

	doneMutex := sync.Mutex{}
	done := false

	// If PeerConnection is closed remove it from global list
	peerConnection.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		switch p {
		case webrtc.PeerConnectionStateDisconnected:
			_ = peerConnection.Close()
		case webrtc.PeerConnectionStateFailed:
			_ = peerConnection.Close()
		case webrtc.PeerConnectionStateClosed:
			doneMutex.Lock()
			done = true
			doneMutex.Unlock()
			sessionChannel <- true
		case webrtc.PeerConnectionStateConnected:
			activeWebRTCSessions.Add(1)
		}
	})

	peerConnection.OnTrack(func(t *webrtc.TrackRemote, r *webrtc.RTPReceiver) {

		for {

			p, _, err := t.ReadRTP()

			if err != nil {
				doneMutex.Lock()
				local := done
				doneMutex.Unlock()

				if local || err == io.EOF {
					return
				} else {
					continue
				}
			} else {
				val := p.MarshalSize()
				traffic.Add(uint64(val))
			}
		}
	})

	<-sessionChannel
}

func main() {
	whepUrl := flag.String("whep-addr", "", "whep live stream url")
	whepSessionsCount := flag.Int("whep-sessions", 1, "whep sessions count")

	flag.Parse()

	if whepUrl == nil || len(*whepUrl) == 0 || whepSessionsCount == nil || *whepSessionsCount <= 0 {
		fmt.Println("wrong params specified")
		return
	}

	var sessions atomic.Int64
	sessions.Store(int64(*whepSessionsCount))

	var repliedSessions atomic.Int64
	repliedSessions.Store(0)

	var activeWebRTCSessions atomic.Int64
	activeWebRTCSessions.Store(0)

	factory := NackStatsFactory{}
	api := CreateWebRtcApi(&factory)

	var traffic atomic.Uint64
	traffic.Store(0)

	for i := 0; i < *whepSessionsCount; i++ {
		go webrtcSession(api, *whepUrl, &sessions, &repliedSessions, &activeWebRTCSessions, &traffic)
	}

	lastBitrateReport := time.Now()

	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if sessions.Load() == 0 {
				return
			}
			now := time.Now()
			interval := now.Sub(lastBitrateReport)
			lastBitrateReport = now
			bandwidthMb := (float64(traffic.Swap(0) * 8 * 1000)) / (float64(interval.Milliseconds()) * 1000 * 1000)
			fmt.Printf("Sessions=%d Confirmed Session=%d Active WebRTC Sessions=%d Bandwidth(Mbit/s)= %.4f Packet Loss=%d\n", sessions.Load(),
				repliedSessions.Load(), activeWebRTCSessions.Load(), bandwidthMb, factory.getLost())
		}
	}
}
