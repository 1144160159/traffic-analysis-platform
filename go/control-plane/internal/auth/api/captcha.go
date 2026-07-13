package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/big"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

const (
	captchaTTL       = 2 * time.Minute
	captchaCodeLen   = 4
	captchaKeyPrefix = "auth:captcha:"
)

var captchaAlphabet = []rune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")

type captchaResponse struct {
	CaptchaID string `json:"captcha_id"`
	ImageData string `json:"image_data"`
	ExpiresIn int    `json:"expires_in"`
}

func (h *Handler) GetCaptcha(w http.ResponseWriter, r *http.Request) {
	if h.redisClient == nil {
		errors.WriteErrorWithStatus(w, http.StatusServiceUnavailable, errors.ErrCodeServiceUnavailable,
			"captcha storage is not available", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	captchaID, err := randomHex(16)
	if err != nil {
		errors.WriteError(w, errors.Wrap(err, errors.ErrCodeInternal, "failed to generate captcha id"), httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	code, err := randomCaptchaCode(captchaCodeLen)
	if err != nil {
		errors.WriteError(w, errors.Wrap(err, errors.ErrCodeInternal, "failed to generate captcha code"), httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if err := h.redisClient.Set(r.Context(), captchaKey(captchaID), code, captchaTTL); err != nil {
		h.logger.Warn("Failed to persist captcha", zap.Error(err))
		errors.WriteErrorWithStatus(w, http.StatusServiceUnavailable, errors.ErrCodeServiceUnavailable,
			"captcha storage is not available", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	imageData, err := renderCaptchaPNGDataURL(code)
	if err != nil {
		_ = h.redisClient.Delete(r.Context(), captchaKey(captchaID))
		errors.WriteError(w, errors.Wrap(err, errors.ErrCodeInternal, "failed to render captcha"), httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(captchaResponse{
		CaptchaID: captchaID,
		ImageData: imageData,
		ExpiresIn: int(captchaTTL.Seconds()),
	})
}

func (h *Handler) verifyCaptcha(ctx context.Context, captchaID, captchaCode string) error {
	if h.redisClient == nil {
		return nil
	}

	captchaID = strings.TrimSpace(captchaID)
	captchaCode = strings.ToUpper(strings.TrimSpace(captchaCode))
	if captchaID == "" || captchaCode == "" {
		return errors.New(errors.ErrCodeMissingParameter, "captcha is required")
	}

	key := captchaKey(captchaID)
	expected, err := h.redisClient.Get(ctx, key)
	if err != nil {
		h.logger.Warn("Failed to load captcha", zap.Error(err))
		return errors.New(errors.ErrCodeServiceUnavailable, "captcha storage is not available")
	}
	_ = h.redisClient.Delete(ctx, key)

	if expected == "" || !strings.EqualFold(expected, captchaCode) {
		return errors.New(errors.ErrCodeInvalidParameter, "captcha is invalid or expired")
	}
	return nil
}

func captchaKey(captchaID string) string {
	return captchaKeyPrefix + captchaID
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomCaptchaCode(length int) (string, error) {
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(captchaAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteRune(captchaAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func renderCaptchaPNGDataURL(code string) (string, error) {
	const (
		width  = 136
		height = 46
		scale  = 4
	)

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 245, G: 249, B: 255, A: 255}}, image.Point{}, draw.Src)

	for i := 0; i < 90; i++ {
		x, _ := randomInt(width)
		y, _ := randomInt(height)
		c := randomCaptchaColor(110, 220)
		img.Set(x, y, c)
	}
	for i := 0; i < 5; i++ {
		x1, _ := randomInt(width)
		y1, _ := randomInt(height)
		x2, _ := randomInt(width)
		y2, _ := randomInt(height)
		drawLine(img, x1, y1, x2, y2, randomCaptchaColor(120, 210))
	}

	for i, r := range code {
		yOffset, _ := randomInt(8)
		x := 14 + i*30
		y := 7 + yOffset/2
		drawGlyph(img, r, x, y, scale, randomCaptchaColor(35, 115))
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func randomInt(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func randomCaptchaColor(min, max uint8) color.RGBA {
	span := int(max-min) + 1
	r, _ := randomInt(span)
	g, _ := randomInt(span)
	b, _ := randomInt(span)
	return color.RGBA{R: min + uint8(r), G: min + uint8(g), B: min + uint8(b), A: 255}
}

func drawLine(img *image.RGBA, x1, y1, x2, y2 int, c color.Color) {
	dx := abs(x2 - x1)
	dy := -abs(y2 - y1)
	sx := -1
	if x1 < x2 {
		sx = 1
	}
	sy := -1
	if y1 < y2 {
		sy = 1
	}
	err := dx + dy
	for {
		if image.Pt(x1, y1).In(img.Bounds()) {
			img.Set(x1, y1, c)
		}
		if x1 == x2 && y1 == y2 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x1 += sx
		}
		if e2 <= dx {
			err += dx
			y1 += sy
		}
	}
}

func drawGlyph(img *image.RGBA, r rune, x, y, scale int, c color.Color) {
	rows, ok := captchaFont[r]
	if !ok {
		return
	}
	for row, pattern := range rows {
		for col, bit := range pattern {
			if bit != '1' {
				continue
			}
			rect := image.Rect(x+col*scale, y+row*scale, x+(col+1)*scale-1, y+(row+1)*scale-1)
			draw.Draw(img, rect, &image.Uniform{C: c}, image.Point{}, draw.Src)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

var captchaFont = map[rune][]string{
	'2': {"11110", "00001", "00001", "11110", "10000", "10000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"10010", "10010", "10010", "11111", "00010", "00010", "00010"},
	'5': {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6': {"01111", "10000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00001", "11110"},
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01111", "10000", "10000", "10111", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'J': {"00111", "00010", "00010", "00010", "10010", "10010", "01100"},
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W': {"10001", "10001", "10001", "10101", "10101", "10101", "01010"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
}
