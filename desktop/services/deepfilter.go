package services

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
// Input/output: stereo S16LE at 48 kHz.
type DFEngine struct {
	state    *C.DFState
	frameLen int
	inBuf    []float32
}

func NewDFEngine() (*DFEngine, error) {
	path := C.CString("")
	defer C.free(unsafe.Pointer(path))

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

func (e *DFEngine) Close() {
	if e == nil || e.state == nil {
		return
	}
	C.df_free(e.state)
	e.state = nil
}

// Denoise accepts stereo S16LE at 48 kHz and returns denoised stereo S16LE.
// Returns nil when the internal buffer hasn't accumulated a full frame yet.
func (e *DFEngine) Denoise(stereoS16LE []byte) []byte {
	samplePairs := len(stereoS16LE) / 4
	for i := 0; i < samplePairs; i++ {
		l := int16(binary.LittleEndian.Uint16(stereoS16LE[i*4:]))
		r := int16(binary.LittleEndian.Uint16(stereoS16LE[i*4+2:]))
		e.inBuf = append(e.inBuf, (float32(l)+float32(r))/(2.0*32768.0))
	}

	outMono := make([]float32, 0, len(e.inBuf))
	frameBuf := make([]float32, e.frameLen)

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

	out := make([]byte, len(outMono)*4)
	for i, s := range outMono {
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		v := int16(s * 32767.0)
		binary.LittleEndian.PutUint16(out[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(v))
	}
	return out
}
