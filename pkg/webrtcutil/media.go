package webrtcutil

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

func NewVideoTrack() (*webrtc.TrackLocalStaticSample, error) {
	return webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"go-webrtc",
	)
}

func NewAudioTrack() (*webrtc.TrackLocalStaticSample, error) {
	return webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"go-webrtc",
	)
}

func GenerateVideoTestPattern(track *webrtc.TrackLocalStaticSample, stop <-chan struct{}) {
	frameNum := 0
	ticker := time.NewTicker(time.Second / 10)
	defer ticker.Stop()

	img := image.NewRGBA(image.Rect(0, 0, 320, 240))

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			frameNum++
			drawTestPattern(img, frameNum)

			nalu := generateH264KeyFrame(img)
			track.WriteSample(media.Sample{
				Data:     nalu,
				Duration: time.Second / 10,
			})
		}
	}
}

func drawTestPattern(img *image.RGBA, frameNum int) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8((x + frameNum*2) % 256)
			g := uint8((y + frameNum*3) % 256)
			b := uint8((x + y + frameNum) % 256)
			img.SetRGBA(x, y, color.RGBA{r, g, b, 255})
		}
	}

	barY := h / 3
	for y := barY; y < barY+20; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
}

func generateH264KeyFrame(img *image.RGBA) []byte {
	sps := []byte{
		0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x0a,
		0xf8, 0x41, 0xa2, 0x00, 0x00, 0x03, 0x00, 0x02,
		0x00, 0x00, 0x03, 0x00, 0x64, 0x1e, 0x2c, 0x5c,
	}

	pps := []byte{
		0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x38, 0x80,
	}

	rawData := imgToRawH264(img)
	idr := append([]byte{0x00, 0x00, 0x00, 0x01, 0x65}, rawData...)

	result := make([]byte, 0, len(sps)+len(pps)+len(idr))
	result = append(result, sps...)
	result = append(result, pps...)
	result = append(result, idr...)

	return result
}

func imgToRawH264(img *image.RGBA) []byte {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	ySize := w * h
	uSize := (w / 2) * (h / 2)
	vSize := uSize

	data := make([]byte, ySize+uSize+vSize)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
			yVal := uint8((int(r8)*76 + int(g8)*150 + int(b8)*28) >> 8)
			data[y*w+x] = yVal
		}
	}

	uOffset := ySize
	vOffset := ySize + uSize
	for y := 0; y < h/2; y++ {
		for x := 0; x < w/2; x++ {
			data[uOffset+y*(w/2)+x] = 128
			data[vOffset+y*(w/2)+x] = 128
		}
	}

	return data
}

func GenerateAudioTone(track *webrtc.TrackLocalStaticSample, stop <-chan struct{}) {
	sampleRate := 48000
	frameSize := 960
	frequency := 440.0
	phase := 0.0
	ticker := time.NewTicker(time.Duration(frameSize) * time.Second / time.Duration(sampleRate))
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			samples := make([]float64, frameSize)
			for i := 0; i < frameSize; i++ {
				samples[i] = 0.3 * sinApprox(2*3.14159*frequency*phase)
				phase += 1.0 / float64(sampleRate)
			}

			encoded := encodeOpusPacket(samples)
			track.WriteSample(media.Sample{
				Data:     encoded,
				Duration: time.Duration(frameSize) * time.Second / time.Duration(sampleRate),
			})
		}
	}
}

func sinApprox(x float64) float64 {
	for x > 3.14159*2 {
		x -= 3.14159 * 2
	}
	for x < 0 {
		x += 3.14159 * 2
	}
	result := 0.0
	term := x
	n := 1.0
	for i := 0; i < 10; i++ {
		result += term
		n += 2
		term *= -x * x / (n * (n - 1))
	}
	return result
}

func encodeOpusPacket(samples []float64) []byte {
	data := make([]byte, len(samples))
	for i, s := range samples {
		data[i] = byte(int(s*127) + 128)
	}

	if len(data) < 10 {
		return []byte{0xf8, 0xff, 0xfe}
	}

	header := []byte{0xf8}
	return append(header, data[:min(len(data), 160)]...)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ReceiveVideoTrack(track *webrtc.TrackRemote, label string, stop <-chan struct{}) {
	buf := make([]byte, 1500)
	totalBytes := 0
	lastLog := time.Now()

	for {
		select {
		case <-stop:
			return
		default:
			n, _, err := track.Read(buf)
			if err != nil {
				fmt.Printf("[%s] Video track read error: %v\n", label, err)
				return
			}
			totalBytes += n
			if time.Since(lastLog) > time.Second {
				fmt.Printf("[%s] Video received: %.2f KB\n", label, float64(totalBytes)/1024)
				lastLog = time.Now()
			}
		}
	}
}

func ReceiveAudioTrack(track *webrtc.TrackRemote, label string, stop <-chan struct{}) {
	buf := make([]byte, 1500)
	totalBytes := 0
	lastLog := time.Now()

	for {
		select {
		case <-stop:
			return
		default:
			n, _, err := track.Read(buf)
			if err != nil {
				fmt.Printf("[%s] Audio track read error: %v\n", label, err)
				return
			}
			totalBytes += n
			if time.Since(lastLog) > time.Second {
				fmt.Printf("[%s] Audio received: %.2f KB\n", label, float64(totalBytes)/1024)
				lastLog = time.Now()
			}
		}
	}
}

func SetupDataChannel(dc *webrtc.DataChannel, label string, stop <-chan struct{}) {
	dc.OnOpen(func() {
		fmt.Printf("[%s] DataChannel '%s' opened\n", label, dc.Label())
		dc.SendText("Hello from " + label + "!")
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		fmt.Printf("[%s] DataChannel message: %s\n", label, string(msg.Data))
		if msg.IsString {
			reply := "Echo: " + string(msg.Data)
			dc.SendText(reply)
		}
	})

	dc.OnClose(func() {
		fmt.Printf("[%s] DataChannel '%s' closed\n", label, dc.Label())
	})
}
