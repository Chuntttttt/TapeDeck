// Package sticker handles PDF sticker generation for NFC card mappings.
package sticker

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // Register JPEG decoder
	"image/png"
	"net/http"
	"os"
	"time"

	"github.com/Chuntttttt/tapedeck/internal/models"
	"github.com/jung-kurt/gofpdf"
)

// Sticker dimensions in inches
const (
	stickerWidthPortrait  = 2.13
	stickerHeightPortrait = 3.35
	stickerSizeSquare     = 2.13

	// Page setup
	pageWidth  = 8.5
	pageHeight = 11.0
	margin     = 0.25

	// Grid layout (3 columns x 3 rows for portrait)
	gridCols = 3
	gridRows = 3

	// Spacing
	horizontalGap = 0.6
	verticalGap   = 0.3

	// Registration mark size
	markLength = 0.2
)

// Generator creates PDF stickers from card mappings.
type Generator struct {
	devMode bool
}

// NewGenerator creates a new sticker generator.
func NewGenerator(devMode bool) *Generator {
	return &Generator{
		devMode: devMode,
	}
}

// GeneratePDF generates a PDF with stickers for the given mappings.
// Returns PDF bytes or error.
func (g *Generator) GeneratePDF(mappings []*models.CardMapping, _ string) ([]byte, error) {
	// Create new PDF
	pdf := gofpdf.New("P", "in", "Letter", "")
	pdf.SetMargins(margin, margin, margin)

	// Track position
	col, row := 0, 0

	for _, mapping := range mappings {
		// Start new page if needed
		if col == 0 && row == 0 {
			pdf.AddPage()
		}

		// Calculate position
		x := margin + float64(col)*(stickerWidthPortrait+horizontalGap)
		y := margin + float64(row)*(stickerHeightPortrait+verticalGap)

		// Fetch poster image (or use placeholder)
		var img image.Image
		var dominantColor color.Color

		if mapping.ThumbnailURL != "" {
			fetchedImg, err := g.fetchPosterImage(mapping.ThumbnailURL)
			if err == nil {
				img = fetchedImg
				dominantColor = g.extractDominantColor(img)
			}
		}

		// Determine layout based on media type
		isMusic := mapping.MediaType == "album" || mapping.MediaType == "track" || mapping.MediaType == "artist"

		if isMusic {
			g.addSquareSticker(pdf, img, x, y)
		} else {
			g.addPortraitSticker(pdf, img, dominantColor, x, y)
		}

		// Move to next grid position
		col++
		if col >= gridCols {
			col = 0
			row++
			if row >= gridRows {
				row = 0
			}
		}
	}

	// Output PDF to bytes
	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to output PDF: %w", err)
	}

	return buf.Bytes(), nil
}

// fetchPosterImage downloads an image from the given URL.
func (g *Generator) fetchPosterImage(url string) (image.Image, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Skip TLS verification in dev mode (for local Plex servers)
	if g.devMode {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // Needed for dev mode with self-signed certs
		}
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch image: status %d", resp.StatusCode)
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return img, nil
}

// extractDominantColor analyzes image edges and returns the dominant color, darkened for letterbox effect.
func (g *Generator) extractDominantColor(img image.Image) color.Color {
	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	var rSum, gSum, bSum uint64
	var count uint64

	// Sample pixels from all four edges (25 pixels per edge)
	sampleCount := 25

	// Top edge
	for i := 0; i < sampleCount && i < width; i++ {
		x := i * width / sampleCount
		r, g, b, _ := img.At(x, 0).RGBA()
		r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8) //nolint:gosec // Shifting by 8 ensures values fit in uint8

		// Skip pure white/black pixels
		if (r8 > 235 && g8 > 235 && b8 > 235) || (r8 < 20 && g8 < 20 && b8 < 20) {
			continue
		}

		rSum += uint64(r8)
		gSum += uint64(g8)
		bSum += uint64(b8)
		count++
	}

	// Bottom edge
	for i := 0; i < sampleCount && i < width; i++ {
		x := i * width / sampleCount
		r, g, b, _ := img.At(x, height-1).RGBA()
		r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8) //nolint:gosec // Shifting by 8 ensures values fit in uint8

		if (r8 > 235 && g8 > 235 && b8 > 235) || (r8 < 20 && g8 < 20 && b8 < 20) {
			continue
		}

		rSum += uint64(r8)
		gSum += uint64(g8)
		bSum += uint64(b8)
		count++
	}

	// Left edge
	for i := 0; i < sampleCount && i < height; i++ {
		y := i * height / sampleCount
		r, g, b, _ := img.At(0, y).RGBA()
		r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8) //nolint:gosec // Shifting by 8 ensures values fit in uint8

		if (r8 > 235 && g8 > 235 && b8 > 235) || (r8 < 20 && g8 < 20 && b8 < 20) {
			continue
		}

		rSum += uint64(r8)
		gSum += uint64(g8)
		bSum += uint64(b8)
		count++
	}

	// Right edge
	for i := 0; i < sampleCount && i < height; i++ {
		y := i * height / sampleCount
		r, g, b, _ := img.At(width-1, y).RGBA()
		r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8) //nolint:gosec // Shifting by 8 ensures values fit in uint8

		if (r8 > 235 && g8 > 235 && b8 > 235) || (r8 < 20 && g8 < 20 && b8 < 20) {
			continue
		}

		rSum += uint64(r8)
		gSum += uint64(g8)
		bSum += uint64(b8)
		count++
	}

	// Default to dark gray if no valid samples
	if count == 0 {
		return color.RGBA{40, 50, 60, 255}
	}

	// Calculate average
	avgR := uint8(rSum / count) //nolint:gosec // Average of uint8 values always fits in uint8
	avgG := uint8(gSum / count) //nolint:gosec // Average of uint8 values always fits in uint8
	avgB := uint8(bSum / count) //nolint:gosec // Average of uint8 values always fits in uint8

	// Darken by 20% for better contrast
	avgR = uint8(float64(avgR) * 0.8)
	avgG = uint8(float64(avgG) * 0.8)
	avgB = uint8(float64(avgB) * 0.8)

	return color.RGBA{avgR, avgG, avgB, 255}
}

// addPortraitSticker adds a portrait sticker with letterbox bars at top/bottom.
func (g *Generator) addPortraitSticker(pdf *gofpdf.Fpdf, img image.Image, dominantColor color.Color, x, y float64) {
	// Draw registration marks at corners
	g.drawRegistrationMarks(pdf, x, y, stickerWidthPortrait, stickerHeightPortrait)

	// If no dominant color, use default
	if dominantColor == nil {
		dominantColor = color.RGBA{40, 50, 60, 255}
	}

	// Fill entire sticker background with dominant color
	r, gColor, b, _ := dominantColor.RGBA()
	pdf.SetFillColor(int(r>>8), int(gColor>>8), int(b>>8))
	pdf.Rect(x, y, stickerWidthPortrait, stickerHeightPortrait, "F")

	// Calculate poster dimensions (2:3 ratio, centered)
	posterMargin := 0.05
	posterWidth := stickerWidthPortrait - (2 * posterMargin)
	posterHeight := posterWidth * 3.0 / 2.0 // 2:3 ratio portrait

	// Center poster vertically
	posterX := x + posterMargin
	posterY := y + (stickerHeightPortrait-posterHeight)/2.0

	// Draw poster (or placeholder) on top of background
	if img != nil {
		g.drawImage(pdf, img, posterX, posterY, posterWidth, posterHeight)
	} else {
		// Placeholder: gray rectangle
		pdf.SetFillColor(100, 100, 100)
		pdf.Rect(posterX, posterY, posterWidth, posterHeight, "F")
	}
}

// addSquareSticker adds a square sticker for music albums.
func (g *Generator) addSquareSticker(pdf *gofpdf.Fpdf, img image.Image, x, y float64) {
	// Draw registration marks at square corners
	g.drawRegistrationMarks(pdf, x, y, stickerSizeSquare, stickerSizeSquare)

	// Draw album art (or placeholder)
	artMargin := 0.05
	artSize := stickerSizeSquare - (2 * artMargin)
	artX := x + artMargin
	artY := y + artMargin

	if img != nil {
		g.drawImage(pdf, img, artX, artY, artSize, artSize)
	} else {
		// Placeholder: gray square
		pdf.SetFillColor(100, 100, 100)
		pdf.Rect(artX, artY, artSize, artSize, "F")
	}
}

// drawRegistrationMarks draws corner crosshairs for cutting guides.
func (g *Generator) drawRegistrationMarks(pdf *gofpdf.Fpdf, x, y, width, height float64) {
	pdf.SetLineWidth(0.01) // 0.5pt converted to inches
	pdf.SetDrawColor(0, 0, 0)

	// Top-left
	pdf.Line(x-markLength, y, x, y)
	pdf.Line(x, y-markLength, x, y)

	// Top-right
	pdf.Line(x+width, y, x+width+markLength, y)
	pdf.Line(x+width, y-markLength, x+width, y)

	// Bottom-left
	pdf.Line(x-markLength, y+height, x, y+height)
	pdf.Line(x, y+height, x, y+height+markLength)

	// Bottom-right
	pdf.Line(x+width, y+height, x+width+markLength, y+height)
	pdf.Line(x+width, y+height, x+width, y+height+markLength)
}

// drawImage renders an image into the PDF at the specified position and size.
// Saves the image to a temp file, embeds it in the PDF, then cleans up.
func (g *Generator) drawImage(pdf *gofpdf.Fpdf, img image.Image, x, y, width, height float64) {
	// Save image to temp file
	tmpFile, err := os.CreateTemp("", "sticker-*.png")
	if err != nil {
		// Fallback: draw colored rectangle
		g.drawColoredFallback(pdf, img, x, y, width, height)
		return
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	// Convert to 8-bit RGBA (gofpdf doesn't support 16-bit PNGs)
	img8bit := g.convertTo8Bit(img)

	// Encode image as PNG
	if err := png.Encode(tmpFile, img8bit); err != nil {
		_ = tmpFile.Close()
		// Fallback
		g.drawColoredFallback(pdf, img, x, y, width, height)
		return
	}

	// Close file before gofpdf reads it
	if err := tmpFile.Close(); err != nil {
		// Fallback
		g.drawColoredFallback(pdf, img, x, y, width, height)
		return
	}

	// Embed image in PDF
	pdf.Image(tmpPath, x, y, width, height, false, "", 0, "")
}

// drawColoredFallback draws a colored rectangle as a fallback when image embedding fails.
func (g *Generator) drawColoredFallback(pdf *gofpdf.Fpdf, img image.Image, x, y, width, height float64) {
	bounds := img.Bounds()
	centerX := bounds.Max.X / 2
	centerY := bounds.Max.Y / 2

	// Sample center pixel for color
	r, gColor, b, _ := img.At(centerX, centerY).RGBA()
	pdf.SetFillColor(int(r>>8), int(gColor>>8), int(b>>8))
	pdf.Rect(x, y, width, height, "F")
}

// convertTo8Bit converts any image to 8-bit NRGBA format.
// This is necessary because gofpdf doesn't support 16-bit PNG images.
func (g *Generator) convertTo8Bit(src image.Image) *image.NRGBA {
	bounds := src.Bounds()
	dst := image.NewNRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}

	return dst
}
