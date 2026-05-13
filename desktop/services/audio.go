package services

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gen2brain/malgo"
)

type DeviceInfo struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

type AudioDevices struct {
	Capture  []DeviceInfo `json:"capture"`
	Playback []DeviceInfo `json:"playback"`
}

type AudioService struct {
	loopbackMu       sync.Mutex
	loopbackCtx      *malgo.AllocatedContext
	loopbackCapture  *malgo.Device
	loopbackPlayback *malgo.Device
	loopbackBuf      []byte
	loopbackBufMu    sync.Mutex
	loopbackActive   atomic.Bool
}

func NewAudioService() *AudioService {
	return &AudioService{}
}

func (s *AudioService) ListDevices() (AudioDevices, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return AudioDevices{}, fmt.Errorf("init audio context: %w", err)
	}
	defer ctx.Free()
	defer ctx.Uninit()

	capture, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return AudioDevices{}, fmt.Errorf("list capture devices: %w", err)
	}

	playback, err := ctx.Devices(malgo.Playback)
	if err != nil {
		return AudioDevices{}, fmt.Errorf("list playback devices: %w", err)
	}

	result := AudioDevices{
		Capture:  make([]DeviceInfo, 0, len(capture)),
		Playback: make([]DeviceInfo, 0, len(playback)),
	}

	for _, d := range capture {
		result.Capture = append(result.Capture, DeviceInfo{
			Name:      d.Name(),
			IsDefault: d.IsDefault != 0,
		})
	}
	for _, d := range playback {
		result.Playback = append(result.Playback, DeviceInfo{
			Name:      d.Name(),
			IsDefault: d.IsDefault != 0,
		})
	}

	return result, nil
}

// StartLoopback opens the given capture and playback devices and routes
// microphone audio directly to the speaker — no server connection required.
// Pass empty strings to use system defaults.
func (s *AudioService) StartLoopback(captureDevice, playbackDevice string) error {
	s.loopbackMu.Lock()
	defer s.loopbackMu.Unlock()

	s.stopLoopbackLocked()

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("init audio context: %w", err)
	}

	captureInfo, playbackInfo, err := findDevices(ctx, captureDevice, playbackDevice)
	if err != nil {
		ctx.Uninit()
		ctx.Free()
		return err
	}
	s.loopbackCtx = ctx

	playbackCfg := malgo.DefaultDeviceConfig(malgo.Playback)
	playbackCfg.Playback.Format = malgo.FormatS16
	playbackCfg.Playback.Channels = audioChannels
	playbackCfg.SampleRate = audioSampleRate
	playbackCfg.Alsa.NoMMap = 1
	pid := playbackInfo.ID
	playbackCfg.Playback.DeviceID = pid.Pointer()

	playDev, err := malgo.InitDevice(ctx.Context, playbackCfg, malgo.DeviceCallbacks{
		Data: func(output, _ []byte, frameCount uint32) {
			need := int(frameCount) * audioChannels * audioBytesPerSample
			if need > len(output) {
				need = len(output)
			}
			s.loopbackBufMu.Lock()
			n := copy(output[:need], s.loopbackBuf)
			if n > 0 {
				s.loopbackBuf = s.loopbackBuf[n:]
			}
			s.loopbackBufMu.Unlock()
			for i := n; i < need; i++ {
				output[i] = 0
			}
		},
	})
	if err != nil {
		ctx.Uninit()
		ctx.Free()
		s.loopbackCtx = nil
		return fmt.Errorf("init loopback playback: %w", err)
	}
	if err := playDev.Start(); err != nil {
		playDev.Uninit()
		ctx.Uninit()
		ctx.Free()
		s.loopbackCtx = nil
		return fmt.Errorf("start loopback playback: %w", err)
	}
	s.loopbackPlayback = playDev

	captureCfg := malgo.DefaultDeviceConfig(malgo.Capture)
	captureCfg.Capture.Format = malgo.FormatS16
	captureCfg.Capture.Channels = audioChannels
	captureCfg.SampleRate = audioSampleRate
	captureCfg.Alsa.NoMMap = 1
	cid := captureInfo.ID
	captureCfg.Capture.DeviceID = cid.Pointer()

	capDev, err := malgo.InitDevice(ctx.Context, captureCfg, malgo.DeviceCallbacks{
		Data: func(_, input []byte, frameCount uint32) {
			need := int(frameCount) * audioChannels * audioBytesPerSample
			if need > len(input) {
				need = len(input)
			}
			buf := make([]byte, need)
			copy(buf, input[:need])
			s.loopbackBufMu.Lock()
			s.loopbackBuf = append(s.loopbackBuf, buf...)
			if len(s.loopbackBuf) > audioFrameBytesS16LE*120 {
				s.loopbackBuf = s.loopbackBuf[len(s.loopbackBuf)-audioFrameBytesS16LE*120:]
			}
			s.loopbackBufMu.Unlock()
		},
	})
	if err != nil {
		s.loopbackPlayback.Stop()
		s.loopbackPlayback.Uninit()
		s.loopbackPlayback = nil
		ctx.Uninit()
		ctx.Free()
		s.loopbackCtx = nil
		return fmt.Errorf("init loopback capture: %w", err)
	}
	if err := capDev.Start(); err != nil {
		capDev.Uninit()
		s.loopbackPlayback.Stop()
		s.loopbackPlayback.Uninit()
		s.loopbackPlayback = nil
		ctx.Uninit()
		ctx.Free()
		s.loopbackCtx = nil
		return fmt.Errorf("start loopback capture: %w", err)
	}
	s.loopbackCapture = capDev
	s.loopbackActive.Store(true)
	return nil
}

func (s *AudioService) StopLoopback() {
	s.loopbackMu.Lock()
	defer s.loopbackMu.Unlock()
	s.stopLoopbackLocked()
}

func (s *AudioService) IsLoopbackActive() bool {
	return s.loopbackActive.Load()
}

func (s *AudioService) stopLoopbackLocked() {
	if s.loopbackCapture != nil {
		s.loopbackCapture.Stop()
		s.loopbackCapture.Uninit()
		s.loopbackCapture = nil
	}
	if s.loopbackPlayback != nil {
		s.loopbackPlayback.Stop()
		s.loopbackPlayback.Uninit()
		s.loopbackPlayback = nil
	}
	if s.loopbackCtx != nil {
		s.loopbackCtx.Uninit()
		s.loopbackCtx.Free()
		s.loopbackCtx = nil
	}
	s.loopbackBufMu.Lock()
	s.loopbackBuf = nil
	s.loopbackBufMu.Unlock()
	s.loopbackActive.Store(false)
}
