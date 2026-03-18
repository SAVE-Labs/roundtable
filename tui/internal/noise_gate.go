package internal

import (
	"encoding/binary"
	"math"
	"sync"
)

const (
	defaultNoiseGateThresholdDB = -42.0
	defaultNoiseGateAttackMs    = 5.0
	defaultNoiseGateReleaseMs   = 180.0
	defaultNoiseGateHoldMs      = 120.0
	noiseGateMinThresholdDB     = -70.0
	noiseGateMaxThresholdDB     = -20.0
	noiseGateThresholdStepDB    = 1.0
)

// NoiseGate attenuates low-level input between speech to reduce steady room noise.
type NoiseGate struct {
	mu sync.Mutex

	enabled         bool
	thresholdDB     float64
	thresholdLinear float64
	attackCoeff     float64
	releaseCoeff    float64
	holdFrames      int

	holdCounter int
	gain        float64
}

func NewNoiseGate(sampleRate, frameSamples int, thresholdDB, attackMs, releaseMs, holdMs float64) *NoiseGate {
	frameMs := 10.0
	if sampleRate > 0 && frameSamples > 0 {
		frameMs = (float64(frameSamples) / float64(sampleRate)) * 1000.0
	}

	g := &NoiseGate{
		enabled:         true,
		thresholdDB:     clampNoiseGateThresholdDB(thresholdDB),
		thresholdLinear: dbToLinear(thresholdDB),
		attackCoeff:     smoothingCoeff(frameMs, attackMs),
		releaseCoeff:    smoothingCoeff(frameMs, releaseMs),
		holdFrames:      msToFrames(frameMs, holdMs),
		gain:            1.0,
	}
	g.thresholdLinear = dbToLinear(g.thresholdDB)

	if g.holdFrames < 0 {
		g.holdFrames = 0
	}

	return g
}

func (g *NoiseGate) SetEnabled(enabled bool) {
	g.mu.Lock()
	g.enabled = enabled
	g.mu.Unlock()
}

func (g *NoiseGate) SetThresholdDB(thresholdDB float64) {
	g.mu.Lock()
	g.thresholdDB = clampNoiseGateThresholdDB(thresholdDB)
	g.thresholdLinear = dbToLinear(g.thresholdDB)
	g.mu.Unlock()
}

func (g *NoiseGate) ProcessPCM16LE(pcm []byte) {
	if len(pcm) < 2 {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.enabled {
		return
	}

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

func dbToLinear(db float64) float64 {
	return math.Pow(10.0, db/20.0)
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

func clampNoiseGateThresholdDB(v float64) float64 {
	if v < noiseGateMinThresholdDB {
		return noiseGateMinThresholdDB
	}
	if v > noiseGateMaxThresholdDB {
		return noiseGateMaxThresholdDB
	}
	return v
}
