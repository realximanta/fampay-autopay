package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"net/http"
	"strconv"

	"github.com/skip2/go-qrcode"
)

func parseHexColor(s string) color.RGBA {
	c, _ := hex.DecodeString(s[1:])
	return color.RGBA{c[0], c[1], c[2], 255}
}

func drawGradientQR(upiURI string) ([]byte, error) {
	q, err := qrcode.New(upiURI, qrcode.High)
	if err != nil {
		return nil, err
	}
	q.DisableBorder = false
	bitmap := q.Bitmap()

	pixelSize := 10
	size := len(bitmap)
	img := image.NewRGBA(image.Rect(0, 0, size*pixelSize, size*pixelSize))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	c1 := parseHexColor("#4ECDC4")
	c2 := parseHexColor("#6C2BD9")

	for y, row := range bitmap {
		for x, cell := range row {
			if cell {
				var ratio float64
				if size > 1 {
					ratio = float64(x+y) / float64((size-1)+(size-1))
				}
				r := uint8(float64(c1.R) + (float64(c2.R)-float64(c1.R))*ratio)
				g := uint8(float64(c1.G) + (float64(c2.G)-float64(c1.G))*ratio)
				b := uint8(float64(c1.B) + (float64(c2.B)-float64(c1.B))*ratio)
				c := color.RGBA{r, g, b, 255}

				for dy := 0; dy < pixelSize; dy++ {
					for dx := 0; dx < pixelSize; dx++ {
						img.Set(x*pixelSize+dx, y*pixelSize+dy, c)
					}
				}
			}
		}
	}

	var buf bytes.Buffer
	err = png.Encode(&buf, img)
	return buf.Bytes(), err
}

func HandleQR(w http.ResponseWriter, r *http.Request) {
	upi := r.URL.Query().Get("upi")
	amountStr := r.URL.Query().Get("amount")
	botName := r.URL.Query().Get("bot_name")
	remark := r.URL.Query().Get("remark")

	if upi == "" || amountStr == "" || botName == "" || remark == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Missing required parameters: upi, amount, bot_name, remark"}`))
		return
	}

	baseAmount, err := strconv.Atoi(amountStr)
	if err != nil || baseAmount <= 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Amount must be a positive integer"}`))
		return
	}

	paise := rand.Intn(99) + 1
	finalAmount := float64(baseAmount) + (float64(paise) / 100.0)
	finalAmountStr := fmt.Sprintf("%.2f", finalAmount)

	upiURI := fmt.Sprintf("upi://pay?pa=%s&pn=%s&am=%s&tn=%s", upi, botName, finalAmountStr, remark)

	imgBytes, err := drawGradientQR(upiURI)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Failed to generate QR"}`))
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(imgBytes)))
	w.Header().Set("X-Final-Amount", finalAmountStr)
	w.Header().Set("X-Base-Amount", strconv.Itoa(baseAmount))
	w.Header().Set("X-Paise", strconv.Itoa(paise))
	w.Header().Set("Access-Control-Expose-Headers", "X-Final-Amount, X-Base-Amount, X-Paise")
	w.Header().Set("Cache-Control", "no-store")
	
	w.WriteHeader(http.StatusOK)
	w.Write(imgBytes)
}
