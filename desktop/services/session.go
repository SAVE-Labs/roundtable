package services

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"strings"
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
	audioSampleRate      = 48000
	audioChannels        = 2
	audioFrameSamples    = 960
	audioBytesPerSample  = 2
	audioFrameBytesS16LE = audioFrameSamples * audioChannels * audioBytesPerSample
	opusMaxPacketBytes   = 4000
)

// SessionService manages a single active voice session: audio capture/playback
// via malgo and WebRTC signaling+transport via pion. Only one session is
// active at a time; call Connect to join and Disconnect to leave.
type SessionService struct {
	mu sync.Mutex

	audioCtx       *malgo.AllocatedContext
	captureDevice  *malgo.Device
	playbackDevice *malgo.Device
	playbackBuf    []byte
	playbackMu     sync.Mutex

	micMonitor     *micLevelMonitor
	voiceActivation *voiceActivation

	webrtcPeer    *webrtc.PeerConnection
	webrtcWS      *websocket.Conn
	webrtcTrack   *webrtc.TrackLocalStaticSample
	opusEncoder   *opus.Encoder
	opusDecoder   *opus.Decoder
	encodeBuf     []byte
	webrtcMu      sync.Mutex
	signalingMu   sync.Mutex

	df        *DFEngine
	nsEnabled atomic.Bool

	muted       atomic.Bool
	loopback    atomic.Bool
	micGainBits atomic.Uint32 // float32 bits
	connected   atomic.Bool
}

func NewSessionService() *SessionService {
	s := &SessionService{}
	s.micGainBits.Store(math.Float32bits(1.0))
	if df, err := NewDFEngine(); err != nil {
		log.Printf("session: deepfilter init failed (noise suppression unavailable): %v", err)
	} else {
		s.df = df
	}
	return s
}

// Connect joins a voice room. serverURL is the HTTP base URL (e.g. http://host:1323),
// roomID is the room to join, peerName is your display name, and
// captureDeviceName / playbackDeviceName are the audio device names from ListDevices
// (empty string picks the system default).
func (s *SessionService) Connect(serverURL, roomID, peerName, captureDeviceName, playbackDeviceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connected.Load() {
		return fmt.Errorf("already connected; call Disconnect first")
	}

	wsURL := deriveWSURL(serverURL, roomID, peerName)

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("init audio context: %w", err)
	}
	s.audioCtx = ctx

	captureInfo, playbackInfo, err := findDevices(ctx, captureDeviceName, playbackDeviceName)
	if err != nil {
		s.closeAudio()
		return err
	}

	encoder, err := opus.NewEncoder(audioSampleRate, audioChannels, opus.AppVoIP)
	if err != nil {
		s.closeAudio()
		return fmt.Errorf("create opus encoder: %w", err)
	}
	decoder, err := opus.NewDecoder(audioSampleRate, audioChannels)
	if err != nil {
		s.closeAudio()
		return fmt.Errorf("create opus decoder: %w", err)
	}
	s.opusEncoder = encoder
	s.opusDecoder = decoder

	peer, track, err := s.setupWebRTC(wsURL)
	if err != nil {
		s.closeAudio()
		return err
	}
	s.webrtcPeer = peer
	s.webrtcTrack = track

	s.voiceActivation = newVoiceActivation(audioSampleRate, audioFrameSamples,
		-42.0, 5.0, 180.0, 120.0)

	if err := s.startPlayback(playbackInfo); err != nil {
		s.closeAll()
		return err
	}
	if err := s.startCapture(captureInfo); err != nil {
		s.closeAll()
		return err
	}

	micMon, err := newMicLevelMonitor(captureInfo)
	if err != nil {
		log.Printf("session: mic monitor init failed (non-fatal): %v", err)
	}
	s.micMonitor = micMon

	s.connected.Store(true)
	return nil
}

func (s *SessionService) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeAll()
	s.connected.Store(false)
	return nil
}

func (s *SessionService) IsConnected() bool {
	return s.connected.Load()
}

func (s *SessionService) SetMuted(muted bool) {
	s.muted.Store(muted)
}

func (s *SessionService) IsMuted() bool {
	return s.muted.Load()
}

func (s *SessionService) SetMicGainDB(db float64) {
	linear := float32(math.Pow(10.0, db/20.0))
	s.micGainBits.Store(math.Float32bits(linear))
}

func (s *SessionService) SetVoiceActivationThresholdDB(db float64) {
	if s.voiceActivation != nil {
		s.voiceActivation.setThresholdDB(db)
	}
}

func (s *SessionService) SetLoopback(enabled bool) {
	s.loopback.Store(enabled)
}

func (s *SessionService) IsLoopback() bool {
	return s.loopback.Load()
}

func (s *SessionService) SetNoiseSuppression(v bool) {
	if s.df != nil {
		s.nsEnabled.Store(v)
	}
}

func (s *SessionService) IsNoiseSuppressionEnabled() bool {
	return s.df != nil && s.nsEnabled.Load()
}

func (s *SessionService) IsNoiseSuppressionAvailable() bool {
	return s.df != nil
}

func (s *SessionService) GetMicLevelDB() float64 {
	if s.micMonitor != nil {
		return s.micMonitor.levelDB()
	}
	if s.voiceActivation != nil {
		return s.voiceActivation.inputLevelDB()
	}
	return -70.0
}

// ── audio internals ──────────────────────────────────────────────────────────

func (s *SessionService) startCapture(info malgo.DeviceInfo) error {
	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = audioChannels
	cfg.SampleRate = audioSampleRate
	cfg.Alsa.NoMMap = 1
	id := info.ID
	cfg.Capture.DeviceID = id.Pointer()

	callbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, frameCount uint32) {
			if len(input) == 0 {
				return
			}
			need := int(frameCount) * audioChannels * audioBytesPerSample
			if need > len(input) {
				need = len(input)
			}
			buf := make([]byte, need)
			copy(buf, input[:need])
			s.onCapture(buf)
		},
	}

	dev, err := malgo.InitDevice(s.audioCtx.Context, cfg, callbacks)
	if err != nil {
		return fmt.Errorf("init capture device %q: %w", info.Name(), err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start capture device %q: %w", info.Name(), err)
	}
	s.captureDevice = dev
	return nil
}

func (s *SessionService) startPlayback(info malgo.DeviceInfo) error {
	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = audioChannels
	cfg.SampleRate = audioSampleRate
	cfg.Alsa.NoMMap = 1
	id := info.ID
	cfg.Playback.DeviceID = id.Pointer()

	callbacks := malgo.DeviceCallbacks{
		Data: func(output, _ []byte, frameCount uint32) {
			need := int(frameCount) * audioChannels * audioBytesPerSample
			if need > len(output) {
				need = len(output)
			}
			s.playbackMu.Lock()
			n := copy(output[:need], s.playbackBuf)
			if n > 0 {
				s.playbackBuf = s.playbackBuf[n:]
			}
			s.playbackMu.Unlock()
			for i := n; i < need; i++ {
				output[i] = 0
			}
		},
	}

	dev, err := malgo.InitDevice(s.audioCtx.Context, cfg, callbacks)
	if err != nil {
		return fmt.Errorf("init playback device %q: %w", info.Name(), err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start playback device %q: %w", info.Name(), err)
	}
	s.playbackDevice = dev
	return nil
}

func (s *SessionService) onCapture(pcm []byte) {
	if s.df != nil && s.nsEnabled.Load() {
		denoised := s.df.Denoise(pcm)
		if len(denoised) == 0 {
			return // DFEngine buffering; wait for full frame
		}
		pcm = denoised
	}
	s.applyMicGain(pcm)
	if s.voiceActivation != nil {
		s.voiceActivation.processPCM16LE(pcm)
	}
	if s.loopback.Load() {
		s.pushPlaybackPCM(pcm)
	}
	if s.muted.Load() {
		return
	}
	s.webrtcMu.Lock()
	defer s.webrtcMu.Unlock()
	if s.webrtcTrack == nil || s.opusEncoder == nil {
		return
	}

	s.encodeBuf = append(s.encodeBuf, pcm...)
	for len(s.encodeBuf) >= audioFrameBytesS16LE {
		frame := s.encodeBuf[:audioFrameBytesS16LE]
		samples := bytesToInt16LE(frame)
		encoded := make([]byte, opusMaxPacketBytes)
		n, err := s.opusEncoder.Encode(samples, encoded)
		if err == nil && n > 0 {
			_ = s.webrtcTrack.WriteSample(media.Sample{
				Data:     encoded[:n],
				Duration: 20 * time.Millisecond,
			})
		}
		s.encodeBuf = s.encodeBuf[audioFrameBytesS16LE:]
	}
	if len(s.encodeBuf) > audioFrameBytesS16LE*5 {
		s.encodeBuf = s.encodeBuf[len(s.encodeBuf)-audioFrameBytesS16LE*5:]
	}
}

func (s *SessionService) pushPlaybackPCM(pcm []byte) {
	s.playbackMu.Lock()
	s.playbackBuf = append(s.playbackBuf, pcm...)
	if len(s.playbackBuf) > audioFrameBytesS16LE*120 {
		s.playbackBuf = s.playbackBuf[len(s.playbackBuf)-audioFrameBytesS16LE*120:]
	}
	s.playbackMu.Unlock()
}

func (s *SessionService) applyMicGain(pcm []byte) {
	bits := s.micGainBits.Load()
	gain := math.Float32frombits(bits)
	if gain == 1.0 {
		return
	}
	for i := 0; i+1 < len(pcm); i += 2 {
		v := float32(int16(binary.LittleEndian.Uint16(pcm[i:]))) * gain
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		binary.LittleEndian.PutUint16(pcm[i:], uint16(int16(v)))
	}
}

// ── WebRTC internals ─────────────────────────────────────────────────────────

func (s *SessionService) setupWebRTC(wsURL string) (*webrtc.PeerConnection, *webrtc.TrackLocalStaticSample, error) {
	me := &webrtc.MediaEngine{}
	if err := me.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: audioSampleRate,
			Channels:  audioChannels,
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, nil, fmt.Errorf("register opus codec: %w", err)
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me))
	peer, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create peer connection: %w", err)
	}

	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeOpus,
		ClockRate: audioSampleRate,
		Channels:  audioChannels,
	}, "audio", "roundtable-desktop")
	if err != nil {
		peer.Close()
		return nil, nil, fmt.Errorf("create audio track: %w", err)
	}

	rtpSender, err := peer.AddTrack(track)
	if err != nil {
		peer.Close()
		return nil, nil, fmt.Errorf("add audio track: %w", err)
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := rtpSender.Read(buf); err != nil {
				return
			}
		}
	}()

	decodeBuf := make([]int16, audioFrameSamples*audioChannels*6)
	peer.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		for {
			pkt, _, err := remote.ReadRTP()
			if err != nil {
				return
			}
			if s.opusDecoder == nil {
				continue
			}
			n, err := s.opusDecoder.Decode(pkt.Payload, decodeBuf)
			if err != nil || n <= 0 {
				continue
			}
			pcm := int16SliceToBytesLE(decodeBuf[:n*audioChannels])
			s.pushPlaybackPCM(pcm)
		}
	})

	offer, err := peer.CreateOffer(nil)
	if err != nil {
		peer.Close()
		return nil, nil, fmt.Errorf("create offer: %w", err)
	}
	done := webrtc.GatheringCompletePromise(peer)
	if err := peer.SetLocalDescription(offer); err != nil {
		peer.Close()
		return nil, nil, fmt.Errorf("set local description: %w", err)
	}
	<-done

	origin := deriveOrigin(wsURL)
	ws, err := websocket.Dial(wsURL, "", origin)
	if err != nil {
		peer.Close()
		return nil, nil, fmt.Errorf("dial signaling websocket: %w", err)
	}
	s.webrtcWS = ws

	localDesc := peer.LocalDescription()
	offerBytes, err := json.Marshal(localDesc)
	if err != nil {
		ws.Close()
		peer.Close()
		return nil, nil, fmt.Errorf("marshal offer: %w", err)
	}
	if err := websocket.Message.Send(ws, offerBytes); err != nil {
		ws.Close()
		peer.Close()
		return nil, nil, fmt.Errorf("send offer: %w", err)
	}

	var answerBytes []byte
	if err := websocket.Message.Receive(ws, &answerBytes); err != nil {
		ws.Close()
		peer.Close()
		return nil, nil, fmt.Errorf("receive answer: %w", err)
	}
	var answer webrtc.SessionDescription
	if err := json.Unmarshal(answerBytes, &answer); err != nil {
		ws.Close()
		peer.Close()
		return nil, nil, fmt.Errorf("decode answer: %w", err)
	}
	if err := peer.SetRemoteDescription(answer); err != nil {
		ws.Close()
		peer.Close()
		return nil, nil, fmt.Errorf("set remote description: %w", err)
	}

	go s.signalingReadLoop(peer, ws)

	return peer, track, nil
}

func (s *SessionService) signalingReadLoop(peer *webrtc.PeerConnection, ws *websocket.Conn) {
	for {
		var msg []byte
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			return
		}
		var sdp webrtc.SessionDescription
		if err := json.Unmarshal(msg, &sdp); err != nil {
			continue
		}
		s.signalingMu.Lock()
		switch sdp.Type {
		case webrtc.SDPTypeOffer:
			if err := peer.SetRemoteDescription(sdp); err != nil {
				s.signalingMu.Unlock()
				continue
			}
			ans, err := peer.CreateAnswer(nil)
			if err != nil {
				s.signalingMu.Unlock()
				continue
			}
			done := webrtc.GatheringCompletePromise(peer)
			_ = peer.SetLocalDescription(ans)
			<-done
			if b, err := json.Marshal(peer.LocalDescription()); err == nil {
				_ = websocket.Message.Send(ws, b)
			}
		case webrtc.SDPTypeAnswer:
			_ = peer.SetRemoteDescription(sdp)
		}
		s.signalingMu.Unlock()
	}
}

// ── cleanup ──────────────────────────────────────────────────────────────────

func (s *SessionService) closeAudio() {
	if s.captureDevice != nil {
		s.captureDevice.Stop()
		s.captureDevice.Uninit()
		s.captureDevice = nil
	}
	if s.playbackDevice != nil {
		s.playbackDevice.Stop()
		s.playbackDevice.Uninit()
		s.playbackDevice = nil
	}
	if s.audioCtx != nil {
		s.audioCtx.Uninit()
		s.audioCtx.Free()
		s.audioCtx = nil
	}
	if s.micMonitor != nil {
		s.micMonitor.close()
		s.micMonitor = nil
	}
}

func (s *SessionService) closeAll() {
	if s.webrtcWS != nil {
		s.webrtcWS.Close()
		s.webrtcWS = nil
	}
	if s.webrtcPeer != nil {
		s.webrtcPeer.Close()
		s.webrtcPeer = nil
	}
	s.webrtcTrack = nil
	s.opusEncoder = nil
	s.opusDecoder = nil
	s.encodeBuf = nil
	s.closeAudio()
	s.playbackBuf = nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func deriveWSURL(serverURL, roomID, peerName string) string {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	wsBase := strings.Replace(base, "https://", "wss://", 1)
	wsBase = strings.Replace(wsBase, "http://", "ws://", 1)
	return wsBase + "/ws?room=" + url.QueryEscape(roomID) + "&peer_name=" + url.QueryEscape(peerName)
}

func deriveOrigin(wsURL string) string {
	parsed, err := url.Parse(wsURL)
	if err != nil || parsed.Host == "" {
		return "http://localhost"
	}
	scheme := "http"
	if parsed.Scheme == "wss" {
		scheme = "https"
	}
	return scheme + "://" + parsed.Host
}

func findDevices(ctx *malgo.AllocatedContext, captureName, playbackName string) (malgo.DeviceInfo, malgo.DeviceInfo, error) {
	captures, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return malgo.DeviceInfo{}, malgo.DeviceInfo{}, fmt.Errorf("list capture devices: %w", err)
	}
	playbacks, err := ctx.Devices(malgo.Playback)
	if err != nil {
		return malgo.DeviceInfo{}, malgo.DeviceInfo{}, fmt.Errorf("list playback devices: %w", err)
	}

	capture, err := pickDevice(captures, captureName)
	if err != nil {
		return malgo.DeviceInfo{}, malgo.DeviceInfo{}, fmt.Errorf("capture device: %w", err)
	}
	playback, err := pickDevice(playbacks, playbackName)
	if err != nil {
		return malgo.DeviceInfo{}, malgo.DeviceInfo{}, fmt.Errorf("playback device: %w", err)
	}
	return capture, playback, nil
}

func pickDevice(devices []malgo.DeviceInfo, name string) (malgo.DeviceInfo, error) {
	if name == "" {
		for _, d := range devices {
			if d.IsDefault != 0 {
				return d, nil
			}
		}
		if len(devices) > 0 {
			return devices[0], nil
		}
		return malgo.DeviceInfo{}, fmt.Errorf("no devices available")
	}
	for _, d := range devices {
		if d.Name() == name {
			return d, nil
		}
	}
	return malgo.DeviceInfo{}, fmt.Errorf("device %q not found", name)
}

func bytesToInt16LE(input []byte) []int16 {
	out := make([]int16, len(input)/audioBytesPerSample)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(input[i*audioBytesPerSample:]))
	}
	return out
}

func int16SliceToBytesLE(input []int16) []byte {
	out := make([]byte, len(input)*audioBytesPerSample)
	for i, s := range input {
		binary.LittleEndian.PutUint16(out[i*audioBytesPerSample:], uint16(s))
	}
	return out
}

// ── mic level monitor ────────────────────────────────────────────────────────

type micLevelMonitor struct {
	ctx    *malgo.AllocatedContext
	device *malgo.Device
	bits   atomic.Uint64
}

func newMicLevelMonitor(capture malgo.DeviceInfo) (*micLevelMonitor, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}
	m := &micLevelMonitor{ctx: ctx}
	m.bits.Store(math.Float64bits(-70.0))

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = audioChannels
	cfg.SampleRate = audioSampleRate
	cfg.Alsa.NoMMap = 1
	id := capture.ID
	cfg.Capture.DeviceID = id.Pointer()

	callbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, frameCount uint32) {
			n := int(frameCount) * audioChannels * audioBytesPerSample
			if n > len(input) {
				n = len(input)
			}
			samples := n / audioBytesPerSample
			if samples == 0 {
				return
			}
			var energy float64
			for i := 0; i < samples; i++ {
				s := int16(binary.LittleEndian.Uint16(input[i*audioBytesPerSample:]))
				v := float64(s) / 32768.0
				energy += v * v
			}
			rms := math.Sqrt(energy / float64(samples))
			db := -70.0
			if rms > 0 {
				db = 20.0 * math.Log10(rms)
			}
			if db < -70 {
				db = -70
			}
			if db > 0 {
				db = 0
			}
			m.bits.Store(math.Float64bits(db))
		},
	}

	dev, err := malgo.InitDevice(ctx.Context, cfg, callbacks)
	if err != nil {
		ctx.Uninit()
		ctx.Free()
		return nil, err
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		ctx.Uninit()
		ctx.Free()
		return nil, err
	}
	m.device = dev
	return m, nil
}

func (m *micLevelMonitor) levelDB() float64 {
	return math.Float64frombits(m.bits.Load())
}

func (m *micLevelMonitor) close() {
	if m.device != nil {
		m.device.Stop()
		m.device.Uninit()
		m.device = nil
	}
	if m.ctx != nil {
		m.ctx.Uninit()
		m.ctx.Free()
		m.ctx = nil
	}
}

// ── voice activation ─────────────────────────────────────────────────────────

type voiceActivation struct {
	mu              sync.Mutex
	thresholdLinear float64
	thresholdDB     float64
	attackCoeff     float64
	releaseCoeff    float64
	holdFrames      int
	holdCounter     int
	gain            float64
	inputLevelDBVal float64
}

func newVoiceActivation(sampleRate, frameSamples int, thresholdDB, attackMs, releaseMs, holdMs float64) *voiceActivation {
	frameMs := (float64(frameSamples) / float64(sampleRate)) * 1000.0
	smoothCoeff := func(fm, tm float64) float64 {
		if tm <= 0 {
			return 1.0
		}
		c := 1.0 - math.Exp(-fm/tm)
		if c < 0 {
			return 0
		}
		if c > 1 {
			return 1
		}
		return c
	}
	holdF := 0
	if frameMs > 0 && holdMs > 0 {
		holdF = int(math.Ceil(holdMs / frameMs))
	}
	return &voiceActivation{
		thresholdDB:     thresholdDB,
		thresholdLinear: math.Pow(10.0, thresholdDB/20.0),
		attackCoeff:     smoothCoeff(frameMs, attackMs),
		releaseCoeff:    smoothCoeff(frameMs, releaseMs),
		holdFrames:      holdF,
		gain:            1.0,
		inputLevelDBVal: -70.0,
	}
}

func (g *voiceActivation) setThresholdDB(db float64) {
	g.mu.Lock()
	g.thresholdDB = db
	g.thresholdLinear = math.Pow(10.0, db/20.0)
	g.mu.Unlock()
}

func (g *voiceActivation) inputLevelDB() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inputLevelDBVal
}

func (g *voiceActivation) processPCM16LE(pcm []byte) {
	if len(pcm) < 2 {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	samples := len(pcm) / audioBytesPerSample
	var energy float64
	for i := 0; i < samples; i++ {
		s := int16(binary.LittleEndian.Uint16(pcm[i*audioBytesPerSample:]))
		v := float64(s) / 32768.0
		energy += v * v
	}
	rms := math.Sqrt(energy / float64(samples))
	db := -70.0
	if rms > 0 {
		db = 20.0 * math.Log10(rms)
	}
	if db < -70 {
		db = -70
	}
	if db > 0 {
		db = 0
	}
	g.inputLevelDBVal = db

	target := 0.0
	if rms >= g.thresholdLinear {
		target = 1.0
		g.holdCounter = g.holdFrames
	} else if g.holdCounter > 0 {
		target = 1.0
		g.holdCounter--
	}

	if target > g.gain {
		g.gain += g.attackCoeff * (target - g.gain)
	} else {
		g.gain += g.releaseCoeff * (target - g.gain)
	}

	if g.gain >= 0.999 {
		return
	}
	for i := 0; i < samples; i++ {
		off := i * audioBytesPerSample
		s := int16(binary.LittleEndian.Uint16(pcm[off:]))
		binary.LittleEndian.PutUint16(pcm[off:], uint16(int16(float64(s)*g.gain)))
	}
}
