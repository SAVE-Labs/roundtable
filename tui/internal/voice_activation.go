package internal

import (
	"encoding/binary"
	"math"
	"sync"
)

const (
	defaultVoiceActivationThresholdDB = -42.0
	defaultVoiceActivationAttackMs    = 5.0
	defaultVoiceActivationReleaseMs   = 180.0
	defaultVoiceActivationHoldMs      = 120.0
	voiceActivationMinThresholdDB     = -70.0
	voiceActivationMaxThresholdDB     = -20.0
	voiceActivationThresholdStepDB    = 1.0
)

// VoiceActivation attenuates low-level input between speech to reduce steady room noise.
type VoiceActivation struct {
	mu sync.Mutex

	thresholdDB     float64
	thresholdLinear float64
	attackCoeff     float64
	releaseCoeff    float64
	holdFrames      int

	holdCounter  int
	gain         float64
	inputLevelDB float64
}

func NewVoiceActivation(sampleRate, frameSamples int, thresholdDB, attackMs, releaseMs, holdMs float64) *VoiceActivation {
	frameMs := 10.0
	if sampleRate > 0 && frameSamples > 0 {
		frameMs = (float64(frameSamples) / float64(sampleRate)) * 1000.0
	}

	g := &VoiceActivation{
		thresholdDB:     clampVoiceActivationThresholdDB(thresholdDB),
		thresholdLinear: dbToLinear(thresholdDB),
		attackCoeff:     smoothingCoeff(frameMs, attackMs),
		releaseCoeff:    smoothingCoeff(frameMs, releaseMs),
		holdFrames:      msToFrames(frameMs, holdMs),
		gain:            1.0,
		inputLevelDB:    voiceActivationMinThresholdDB,
	}
	g.thresholdLinear = dbToLinear(g.thresholdDB)

	if g.holdFrames < 0 {
		g.holdFrames = 0
	}

	return g
}

func (g *VoiceActivation) SetThresholdDB(thresholdDB float64) {
	g.mu.Lock()
	g.thresholdDB = clampVoiceActivationThresholdDB(thresholdDB)
	g.thresholdLinear = dbToLinear(g.thresholdDB)
	g.mu.Unlock()
}

func (g *VoiceActivation) ProcessPCM16LE(pcm []byte) {
	if len(pcm) < 2 {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	sampleCount := len(pcm) / audioBytesPerSample
	if sampleCount == 0 {
		return
	}

	var energy float64
	for i := 0; i < sampleCount; i++ {
		off := i * audioBytesPerSample
		s := int16(binary.LittleEndian.Uint16(pcm[off : off+audioBytesPerSample]))
		v := float64(s) / 32768.0
		energy += v * v
	}

	rms := math.Sqrt(energy / float64(sampleCount))
	inputLevelDB := linearToDB(rms)
	if inputLevelDB < voiceActivationMinThresholdDB {
		inputLevelDB = voiceActivationMinThresholdDB
	}
	if inputLevelDB > 0 {
		inputLevelDB = 0
	}
	g.inputLevelDB = inputLevelDB

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

	for i := 0; i < sampleCount; i++ {
		off := i * audioBytesPerSample
		s := int16(binary.LittleEndian.Uint16(pcm[off : off+audioBytesPerSample]))
		scaled := int16(float64(s) * g.gain)
		binary.LittleEndian.PutUint16(pcm[off:off+audioBytesPerSample], uint16(scaled))
	}
}

func (g *VoiceActivation) InputLevelDB() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inputLevelDB
}

// IsSpeaking returns true when the voice gate is currently open (speech
// detected or within the post-speech hold window).
func (g *VoiceActivation) IsSpeaking() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inputLevelDB > g.thresholdDB || g.holdCounter > 0
}

func dbToLinear(db float64) float64 {
	return math.Pow(10.0, db/20.0)
}

func linearToDB(linear float64) float64 {
	if linear <= 0 {
		return -120.0
	}
	return 20.0 * math.Log10(linear)
}

func smoothingCoeff(frameMs, timeMs float64) float64 {
	if timeMs <= 0 {
		return 1.0
	}
	coeff := 1.0 - math.Exp(-frameMs/timeMs)
	if coeff < 0 {
		return 0
	}
	if coeff > 1 {
		return 1
	}
	return coeff
}

func msToFrames(frameMs, holdMs float64) int {
	if frameMs <= 0 || holdMs <= 0 {
		return 0
	}
	return int(math.Ceil(holdMs / frameMs))
}

func clampVoiceActivationThresholdDB(v float64) float64 {
	if v < voiceActivationMinThresholdDB {
		return voiceActivationMinThresholdDB
	}
	if v > voiceActivationMaxThresholdDB {
		return voiceActivationMaxThresholdDB
	}
	return v
}
