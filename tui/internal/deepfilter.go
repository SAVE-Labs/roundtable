package internal

/*
#cgo linux   LDFLAGS: -L${SRCDIR} -Wl,-Bstatic -ldf -Wl,-Bdynamic -ldl -lpthread -lm
#cgo darwin  LDFLAGS: -L${SRCDIR} -ldf -ldl -framework CoreFoundation
#cgo windows LDFLAGS: -L${SRCDIR} -ldf -lws2_32 -luserenv -lbcrypt -lntdll
#include <stdlib.h>
#include <stdint.h>

typedef struct DFState DFState;

DFState* df_create(const char* path, float atten_lim, const char* log_level);
void     df_free(DFState* st);
size_t   df_get_frame_length(DFState* st);
float    df_process_frame(DFState* st, float* input, float* output);
*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"log"
	"unsafe"
)

// DFEngine wraps the DeepFilterNet C library for real-time noise suppression.
// Input/output: stereo S16LE at 48 kHz. No resampling needed.
type DFEngine struct {
	state    *C.DFState
	frameLen int // mono samples per DF frame (typically 480 at 48 kHz)
	inBuf    []float32 // pending mono float32 samples
}

// NewDFEngine creates a DeepFilterNet engine using the embedded DFN3 model.
// libdeep_filter.so must have been built with --features capi,tract (default-model
// is on by default, which embeds DeepFilterNet3_onnx.tar.gz).
// An empty path string tells df_create to use the bundled model.
func NewDFEngine() (*DFEngine, error) {
	path := C.CString("")
	defer C.free(unsafe.Pointer(path))

	// 100 dB attenuation limit is effectively unlimited; log_level NULL = default.
	state := C.df_create(path, 100.0, nil)
	if state == nil {
		return nil, fmt.Errorf("deepfilter: df_create returned nil")
	}

	frameLen := int(C.df_get_frame_length(state))
	if frameLen <= 0 {
		C.df_free(state)
		return nil, fmt.Errorf("deepfilter: invalid frame length %d", frameLen)
	}

	log.Printf("deepfilter: engine ready frame_len=%d", frameLen)
	return &DFEngine{state: state, frameLen: frameLen}, nil
}

// Close frees the underlying C state. Safe to call on nil.
func (e *DFEngine) Close() {
	if e == nil || e.state == nil {
		return
	}
	C.df_free(e.state)
	e.state = nil
}

// Denoise accepts stereo S16LE at 48 kHz and returns denoised stereo S16LE.
// Output may be shorter than input on the first call (frame buffering).
func (e *DFEngine) Denoise(stereoS16LE []byte) []byte {
	// --- downmix stereo S16LE → mono float32 ---
	samplePairs := len(stereoS16LE) / 4 // 4 bytes per stereo sample (L+R S16LE)
	for i := 0; i < samplePairs; i++ {
		l := int16(binary.LittleEndian.Uint16(stereoS16LE[i*4:]))
		r := int16(binary.LittleEndian.Uint16(stereoS16LE[i*4+2:]))
		e.inBuf = append(e.inBuf, (float32(l)+float32(r))/(2.0*32768.0))
	}

	// --- process complete frames ---
	outMono := make([]float32, 0, len(e.inBuf))
	frameBuf := make([]float32, e.frameLen) // output buffer reused per frame

	for len(e.inBuf) >= e.frameLen {
		inFrame := e.inBuf[:e.frameLen]
		e.inBuf = e.inBuf[e.frameLen:]

		C.df_process_frame(
			e.state,
			(*C.float)(unsafe.Pointer(&inFrame[0])),
			(*C.float)(unsafe.Pointer(&frameBuf[0])),
		)
		outMono = append(outMono, frameBuf...)
	}

	if len(outMono) == 0 {
		return nil
	}

	// --- upmix mono float32 → stereo S16LE ---
	out := make([]byte, len(outMono)*4)
	for i, s := range outMono {
		// Clamp
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		v := int16(s * 32767.0)
		binary.LittleEndian.PutUint16(out[i*4:], uint16(v))   // L
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(v)) // R
	}
	return out
}
