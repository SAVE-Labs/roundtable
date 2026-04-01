package internal

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"golang.org/x/net/websocket"
)

const (
	audioSampleRate       = 48000
	audioChannels         = 2
	audioFrameSamples     = 960
	audioBytesPerSample   = 2
	audioFrameBytesS16LE  = audioFrameSamples * audioChannels * audioBytesPerSample
	opusMaxPacketBytes    = 4000
	defaultBackendWS      = "ws://127.0.0.1:1323/ws"
	defaultBackendBaseURL = "http://127.0.0.1:1323"
)

type AudioEngine struct {
	ctx            *malgo.AllocatedContext
	captureDevice  *malgo.Device
	playbackDevice *malgo.Device

	mu           sync.Mutex
	playbackBuf  []byte
	onCapturePCM func([]byte)
}

type MicLevelMonitor struct {
	ctx           *malgo.AllocatedContext
	captureDevice *malgo.Device
	deviceName    string
	levelBits     atomic.Uint64
}

func NewAudioEngine() *AudioEngine {
	return &AudioEngine{}
}

func NewMicLevelMonitor(capture malgo.DeviceInfo) (*MicLevelMonitor, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init audio context: %w", err)
	}

	monitor := &MicLevelMonitor{ctx: ctx, deviceName: capture.Name()}
	monitor.levelBits.Store(math.Float64bits(voiceActivationMinThresholdDB))

	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format = malgo.FormatS16
	config.Capture.Channels = audioChannels
	config.SampleRate = audioSampleRate
	config.Alsa.NoMMap = 1

	captureID := capture.ID
	config.Capture.DeviceID = captureID.Pointer()

	callbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, frameCount uint32) {
			bytesNeeded := int(frameCount) * audioChannels * audioBytesPerSample
			if bytesNeeded <= 0 || len(input) == 0 {
				return
			}
			if bytesNeeded > len(input) {
				bytesNeeded = len(input)
			}
			sampleCount := bytesNeeded / audioBytesPerSample
			if sampleCount == 0 {
				return
			}

			var energy float64
			for i := 0; i < sampleCount; i++ {
				off := i * audioBytesPerSample
				s := int16(binary.LittleEndian.Uint16(input[off : off+audioBytesPerSample]))
				v := float64(s) / 32768.0
				energy += v * v
			}

			rms := math.Sqrt(energy / float64(sampleCount))
			levelDB := linearToDB(rms)
			if levelDB < voiceActivationMinThresholdDB {
				levelDB = voiceActivationMinThresholdDB
			}
			if levelDB > 0 {
				levelDB = 0
			}

			monitor.levelBits.Store(math.Float64bits(levelDB))
		},
	}

	dev, err := malgo.InitDevice(ctx.Context, config, callbacks)
	if err != nil {
		monitor.Close()
		return nil, fmt.Errorf("init capture monitor %q: %w", capture.Name(), err)
	}

	if err := dev.Start(); err != nil {
		dev.Uninit()
		monitor.Close()
		return nil, fmt.Errorf("start capture monitor %q: %w", capture.Name(), err)
	}

	monitor.captureDevice = dev
	return monitor, nil
}

func (m *MicLevelMonitor) LevelDB() float64 {
	return math.Float64frombits(m.levelBits.Load())
}

func (m *MicLevelMonitor) DeviceName() string {
	if m == nil {
		return ""
	}
	return m.deviceName
}

func (m *MicLevelMonitor) Close() {
	if m == nil {
		return
	}
	if m.captureDevice != nil {
		m.captureDevice.Stop()
		m.captureDevice.Uninit()
		m.captureDevice = nil
	}
	if m.ctx != nil {
		m.ctx.Uninit()
		m.ctx.Free()
		m.ctx = nil
	}
}

func (a *AudioEngine) Start(capture malgo.DeviceInfo, playback malgo.DeviceInfo, onCapturePCM func([]byte)) error {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("init audio context: %w", err)
	}

	a.ctx = ctx
	a.onCapturePCM = onCapturePCM

	if err := a.startPlayback(playback); err != nil {
		a.Close()
		return err
	}

	if err := a.startCapture(capture); err != nil {
		a.Close()
		return err
	}

	return nil
}

func (a *AudioEngine) startCapture(capture malgo.DeviceInfo) error {
	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format = malgo.FormatS16
	config.Capture.Channels = audioChannels
	config.SampleRate = audioSampleRate
	config.Alsa.NoMMap = 1

	captureID := capture.ID
	config.Capture.DeviceID = captureID.Pointer()

	callbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, frameCount uint32) {
			if len(input) == 0 || a.onCapturePCM == nil {
				return
			}
			bytesNeeded := int(frameCount) * audioChannels * audioBytesPerSample
			if bytesNeeded > len(input) {
				bytesNeeded = len(input)
			}
			copyBuf := make([]byte, bytesNeeded)
			copy(copyBuf, input[:bytesNeeded])
			a.onCapturePCM(copyBuf)
		},
	}

	dev, err := malgo.InitDevice(a.ctx.Context, config, callbacks)
	if err != nil {
		return fmt.Errorf("init capture device %q: %w", capture.Name(), err)
	}

	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start capture device %q: %w", capture.Name(), err)
	}

	a.captureDevice = dev
	return nil
}

func (a *AudioEngine) startPlayback(playback malgo.DeviceInfo) error {
	config := malgo.DefaultDeviceConfig(malgo.Playback)
	config.Playback.Format = malgo.FormatS16
	config.Playback.Channels = audioChannels
	config.SampleRate = audioSampleRate
	config.Alsa.NoMMap = 1

	playbackID := playback.ID
	config.Playback.DeviceID = playbackID.Pointer()

	callbacks := malgo.DeviceCallbacks{
		Data: func(output, _ []byte, frameCount uint32) {
			bytesNeeded := int(frameCount) * audioChannels * audioBytesPerSample
			if bytesNeeded > len(output) {
				bytesNeeded = len(output)
			}

			a.mu.Lock()
			defer a.mu.Unlock()

			copied := copy(output[:bytesNeeded], a.playbackBuf)
			if copied > 0 {
				a.playbackBuf = a.playbackBuf[copied:]
			}
			for i := copied; i < bytesNeeded; i++ {
				output[i] = 0
			}
		},
	}

	dev, err := malgo.InitDevice(a.ctx.Context, config, callbacks)
	if err != nil {
		return fmt.Errorf("init playback device %q: %w", playback.Name(), err)
	}

	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start playback device %q: %w", playback.Name(), err)
	}

	a.playbackDevice = dev
	return nil
}

func (a *AudioEngine) PushPCM16LE(payload []byte) {
	if len(payload) == 0 {
		return
	}

	a.mu.Lock()
	a.playbackBuf = append(a.playbackBuf, payload...)
	if len(a.playbackBuf) > audioFrameBytesS16LE*120 {
		a.playbackBuf = a.playbackBuf[len(a.playbackBuf)-audioFrameBytesS16LE*120:]
	}
	a.mu.Unlock()
}

func (a *AudioEngine) Close() {
	if a.captureDevice != nil {
		a.captureDevice.Stop()
		a.captureDevice.Uninit()
		a.captureDevice = nil
	}
	if a.playbackDevice != nil {
		a.playbackDevice.Stop()
		a.playbackDevice.Uninit()
		a.playbackDevice = nil
	}
	if a.ctx != nil {
		a.ctx.Uninit()
		a.ctx.Free()
		a.ctx = nil
	}
}

type WebRTCClient struct {
	peer *webrtc.PeerConnection
	ws   *websocket.Conn

	mu        sync.Mutex
	wsMu      sync.Mutex
	signalMu  sync.Mutex
	track     *webrtc.TrackLocalStaticSample
	encoder   *opus.Encoder
	decoder   *opus.Decoder
	encodeBuf []byte
	muted     atomic.Bool
}

func NewWebRTCClient(wsURL string, onRemotePCM16LE func([]byte)) (*WebRTCClient, error) {
	log.Printf("webrtc: init client ws_url=%s", wsURL)
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: audioSampleRate,
			Channels:  audioChannels,
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("register opus codec: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	peer, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		log.Printf("webrtc: create peer connection failed ws_url=%s err=%v", wsURL, err)
		return nil, fmt.Errorf("create peer connection: %w", err)
	}

	encoder, err := opus.NewEncoder(audioSampleRate, audioChannels, opus.AppVoIP)
	if err != nil {
		log.Printf("webrtc: create opus encoder failed ws_url=%s err=%v", wsURL, err)
		peer.Close()
		return nil, fmt.Errorf("create opus encoder: %w", err)
	}

	decoder, err := opus.NewDecoder(audioSampleRate, audioChannels)
	if err != nil {
		log.Printf("webrtc: create opus decoder failed ws_url=%s err=%v", wsURL, err)
		peer.Close()
		return nil, fmt.Errorf("create opus decoder: %w", err)
	}

	client := &WebRTCClient{
		peer:    peer,
		encoder: encoder,
		decoder: decoder,
	}

	peer.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		decodeBuf := make([]int16, audioFrameSamples*audioChannels*6)
		for {
			pkt, _, readErr := track.ReadRTP()
			if readErr != nil {
				return
			}
			if onRemotePCM16LE != nil {
				samplesPerChannel, err := client.decoder.Decode(pkt.Payload, decodeBuf)
				if err != nil || samplesPerChannel <= 0 {
					continue
				}
				pcm := int16SliceToBytesLE(decodeBuf[:samplesPerChannel*audioChannels])
				onRemotePCM16LE(pcm)
			}
		}
	})

	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeOpus,
		ClockRate: audioSampleRate,
		Channels:  audioChannels,
	}, "audio", "roundtable-tui")
	if err != nil {
		log.Printf("webrtc: create local track failed ws_url=%s err=%v", wsURL, err)
		peer.Close()
		return nil, fmt.Errorf("create local audio track: %w", err)
	}

	rtpSender, err := peer.AddTrack(track)
	if err != nil {
		log.Printf("webrtc: add local track failed ws_url=%s err=%v", wsURL, err)
		peer.Close()
		return nil, fmt.Errorf("add local audio track: %w", err)
	}

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, err := rtpSender.Read(rtcpBuf); err != nil {
				return
			}
		}
	}()

	client.track = track

	offer, err := peer.CreateOffer(nil)
	if err != nil {
		log.Printf("webrtc: create offer failed ws_url=%s err=%v", wsURL, err)
		peer.Close()
		return nil, fmt.Errorf("create offer: %w", err)
	}

	gatherComplete := webrtc.GatheringCompletePromise(peer)
	if err := peer.SetLocalDescription(offer); err != nil {
		log.Printf("webrtc: set local description failed ws_url=%s err=%v", wsURL, err)
		peer.Close()
		return nil, fmt.Errorf("set local description: %w", err)
	}
	<-gatherComplete

	serverOrigin := defaultBackendBaseURL
	if parsedWS, err := url.Parse(wsURL); err == nil && parsedWS.Host != "" {
		scheme := "http"
		if parsedWS.Scheme == "wss" {
			scheme = "https"
		}
		serverOrigin = scheme + "://" + parsedWS.Host
	}
	log.Printf("webrtc: dialing websocket ws_url=%s origin=%s", wsURL, serverOrigin)

	ws, err := websocket.Dial(wsURL, "", serverOrigin)
	if err != nil {
		log.Printf("webrtc: dial websocket failed ws_url=%s origin=%s err=%v", wsURL, serverOrigin, err)
		peer.Close()
		return nil, fmt.Errorf("dial websocket: %w", err)
	}
	client.ws = ws
	log.Printf("webrtc: websocket connected ws_url=%s", wsURL)

	localDesc := peer.LocalDescription()
	if localDesc == nil {
		log.Printf("webrtc: missing local description ws_url=%s", wsURL)
		client.Close()
		return nil, fmt.Errorf("missing local description")
	}

	offerBytes, err := json.Marshal(localDesc)
	if err != nil {
		log.Printf("webrtc: marshal offer failed ws_url=%s err=%v", wsURL, err)
		client.Close()
		return nil, fmt.Errorf("marshal offer: %w", err)
	}

	if err := client.sendSignalingBytes(offerBytes); err != nil {
		log.Printf("webrtc: send offer failed ws_url=%s err=%v", wsURL, err)
		client.Close()
		return nil, fmt.Errorf("send offer: %w", err)
	}
	log.Printf("webrtc: offer sent ws_url=%s", wsURL)

	var answerBytes []byte
	if err := websocket.Message.Receive(ws, &answerBytes); err != nil {
		log.Printf("webrtc: receive answer failed ws_url=%s err=%v", wsURL, err)
		client.Close()
		return nil, fmt.Errorf("receive answer: %w", err)
	}
	log.Printf("webrtc: answer received ws_url=%s bytes=%d", wsURL, len(answerBytes))

	var answer webrtc.SessionDescription
	if err := json.Unmarshal(answerBytes, &answer); err != nil {
		log.Printf("webrtc: decode answer failed ws_url=%s err=%v", wsURL, err)
		client.Close()
		return nil, fmt.Errorf("decode answer: %w", err)
	}

	if err := peer.SetRemoteDescription(answer); err != nil {
		log.Printf("webrtc: set remote description failed ws_url=%s err=%v", wsURL, err)
		client.Close()
		return nil, fmt.Errorf("set remote description: %w", err)
	}
	log.Printf("webrtc: signaling established ws_url=%s", wsURL)

	go client.signalingReadLoop()

	return client, nil
}

func (c *WebRTCClient) signalingReadLoop() {
	for {
		var msgBytes []byte
		if err := websocket.Message.Receive(c.ws, &msgBytes); err != nil {
			log.Printf("webrtc: signaling loop receive failed err=%v", err)
			return
		}

		var sdp webrtc.SessionDescription
		if err := json.Unmarshal(msgBytes, &sdp); err != nil {
			log.Printf("webrtc: signaling loop decode failed err=%v", err)
			continue
		}

		if err := c.handleRemoteSDP(sdp); err != nil {
			log.Printf("webrtc: handle remote sdp failed type=%s err=%v", sdp.Type.String(), err)
		}
	}
}

func (c *WebRTCClient) handleRemoteSDP(sdp webrtc.SessionDescription) error {
	c.signalMu.Lock()
	defer c.signalMu.Unlock()

	if c.peer == nil {
		return fmt.Errorf("peer connection is closed")
	}

	switch sdp.Type {
	case webrtc.SDPTypeOffer:
		if err := c.peer.SetRemoteDescription(sdp); err != nil {
			return fmt.Errorf("set remote offer: %w", err)
		}

		answer, err := c.peer.CreateAnswer(nil)
		if err != nil {
			return fmt.Errorf("create answer: %w", err)
		}

		gatherComplete := webrtc.GatheringCompletePromise(c.peer)
		if err := c.peer.SetLocalDescription(answer); err != nil {
			return fmt.Errorf("set local answer: %w", err)
		}
		<-gatherComplete

		localDesc := c.peer.LocalDescription()
		if localDesc == nil {
			return fmt.Errorf("missing local answer")
		}

		answerBytes, err := json.Marshal(localDesc)
		if err != nil {
			return fmt.Errorf("marshal answer: %w", err)
		}

		if err := c.sendSignalingBytes(answerBytes); err != nil {
			return fmt.Errorf("send answer: %w", err)
		}

	case webrtc.SDPTypeAnswer:
		if err := c.peer.SetRemoteDescription(sdp); err != nil {
			return fmt.Errorf("set remote answer: %w", err)
		}
	}

	return nil
}

func (c *WebRTCClient) sendSignalingBytes(payload []byte) error {
	c.wsMu.Lock()
	defer c.wsMu.Unlock()
	if c.ws == nil {
		return fmt.Errorf("websocket is closed")
	}
	return websocket.Message.Send(c.ws, payload)
}

func (c *WebRTCClient) SendPCM16LE(pcmBytes []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.track == nil {
		return nil
	}
	if c.encoder == nil {
		return fmt.Errorf("opus encoder is not initialized")
	}
	if c.muted.Load() {
		return nil
	}

	c.encodeBuf = append(c.encodeBuf, pcmBytes...)
	for len(c.encodeBuf) >= audioFrameBytesS16LE {
		frame := c.encodeBuf[:audioFrameBytesS16LE]
		samples := bytesToInt16LE(frame)
		encoded := make([]byte, opusMaxPacketBytes)
		packetLen, err := c.encoder.Encode(samples, encoded)
		if err != nil {
			return fmt.Errorf("encode opus frame: %w", err)
		}

		if err := c.track.WriteSample(media.Sample{Data: encoded[:packetLen], Duration: 20 * time.Millisecond}); err != nil {
			return err
		}

		c.encodeBuf = c.encodeBuf[audioFrameBytesS16LE:]
	}

	if len(c.encodeBuf) > audioFrameBytesS16LE*5 {
		c.encodeBuf = c.encodeBuf[len(c.encodeBuf)-audioFrameBytesS16LE*5:]
	}

	return nil
}

func (c *WebRTCClient) SetMuted(muted bool) {
	c.muted.Store(muted)
}

func (c *WebRTCClient) IsMuted() bool {
	return c.muted.Load()
}

func (c *WebRTCClient) Close() {
	if c.ws != nil {
		log.Printf("webrtc: closing websocket")
		c.ws.Close()
		c.ws = nil
	}
	if c.peer != nil {
		log.Printf("webrtc: closing peer connection")
		c.peer.Close()
		c.peer = nil
	}
}

func bytesToInt16LE(input []byte) []int16 {
	sampleCount := len(input) / audioBytesPerSample
	out := make([]int16, sampleCount)
	for i := 0; i < sampleCount; i++ {
		out[i] = int16(binary.LittleEndian.Uint16(input[i*audioBytesPerSample:]))
	}
	return out
}

func int16SliceToBytesLE(input []int16) []byte {
	out := make([]byte, len(input)*audioBytesPerSample)
	for i, sample := range input {
		binary.LittleEndian.PutUint16(out[i*audioBytesPerSample:], uint16(sample))
	}
	return out
}
