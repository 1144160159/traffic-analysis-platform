// Command extract_fusion_ui_icons extracts page-specific icon assets from the
// canonical Fusion UI reference. It uses only the Go image standard library so
// extraction remains deterministic even when the Windows Chrome tunnel is down;
// the resulting assets are still validated in Windows Chrome with the page.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

type iconSpec struct {
	Name string
	Box  image.Rectangle
	Mode string
}

type iconRecord struct {
	Name      string         `json:"name"`
	BBox      map[string]int `json:"bbox"`
	AlphaMode string         `json:"alpha_mode"`
	Output    string         `json:"output"`
	SHA256    string         `json:"sha256"`
}

func main() {
	root, err := os.Getwd()
	must(err)
	candidateID := os.Getenv("FUSION_ICON_CANDIDATE_ID")
	if candidateID == "" {
		candidateID = "fusion-r599-source-candidate"
	}
	if !regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(candidateID) {
		panic("FUSION_ICON_CANDIDATE_ID must contain only lowercase letters, digits, and hyphens")
	}
	sourceRel := "doc/04_assets/ui_suite_gpt_v1/screens/pages/fusion.png"
	sourcePath := filepath.Join(root, sourceRel)
	candidateRel := filepath.ToSlash(filepath.Join("evidence/ui-image-breakdowns/pages/fusion/icon-candidates", candidateID))
	assetRel := filepath.ToSlash(filepath.Join(candidateRel, "assets"))
	assetDir := filepath.Join(root, assetRel)
	evidenceRel := filepath.ToSlash(filepath.Join(candidateRel, "review-sheet.png"))
	evidencePath := filepath.Join(root, evidenceRel)

	file, err := os.Open(sourcePath)
	must(err)
	defer file.Close()
	source, err := png.Decode(file)
	must(err)
	if source.Bounds().Dx() != 1920 || source.Bounds().Dy() != 1080 {
		panic(fmt.Sprintf("unexpected Fusion source dimensions: %v", source.Bounds()))
	}
	sourceBytes, err := os.ReadFile(sourcePath)
	must(err)
	sourceDigest := fmt.Sprintf("%x", sha256.Sum256(sourceBytes))

	specs := []iconSpec{
		{"source-flow", image.Rect(201, 159, 241, 199), "circle"},
		{"source-asset", image.Rect(425, 159, 465, 199), "circle"},
		{"source-device-log", image.Rect(641, 159, 681, 199), "circle"},
		{"source-user-event", image.Rect(861, 159, 901, 199), "circle"},
		{"source-threat-intel", image.Rect(1047, 159, 1087, 199), "circle"},
		{"source-vulnerability", image.Rect(1229, 159, 1269, 199), "circle"},
	}

	must(os.MkdirAll(assetDir, 0o755))
	must(os.MkdirAll(filepath.Dir(evidencePath), 0o755))
	sheet := image.NewNRGBA(image.Rect(0, 0, len(specs)*168, 120))
	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{C: color.NRGBA{R: 6, G: 28, B: 43, A: 255}}, image.Point{}, draw.Src)
	records := make([]iconRecord, 0, len(specs))

	for index, spec := range specs {
		icon := extract(source, spec)
		outputName := "fusion-" + spec.Name + ".png"
		outputPath := filepath.Join(assetDir, outputName)
		out, err := os.Create(outputPath)
		must(err)
		must(png.Encode(out, icon))
		must(out.Close())
		outputBytes, err := os.ReadFile(outputPath)
		must(err)
		outputDigest := fmt.Sprintf("%x", sha256.Sum256(outputBytes))

		left := index * 168
		contextBox := spec.Box.Inset(-10)
		contextImage := image.NewNRGBA(image.Rect(0, 0, contextBox.Dx(), contextBox.Dy()))
		draw.Draw(contextImage, contextImage.Bounds(), source, contextBox.Min, draw.Src)
		draw.Draw(sheet, image.Rect(left+4, 8, left+64, 68), contextImage, image.Point{}, draw.Src)
		draw.Draw(sheet, image.Rect(left+72, 8, left+152, 88), scaleNearest(icon, 2), image.Point{}, draw.Over)
		records = append(records, iconRecord{
			Name:      spec.Name,
			BBox:      map[string]int{"x": spec.Box.Min.X, "y": spec.Box.Min.Y, "width": spec.Box.Dx(), "height": spec.Box.Dy()},
			AlphaMode: spec.Mode,
			Output:    filepath.ToSlash(filepath.Join(assetRel, outputName)),
			SHA256:    outputDigest,
		})
	}

	sheetFile, err := os.Create(evidencePath)
	must(err)
	must(png.Encode(sheetFile, sheet))
	must(sheetFile.Close())
	provenance := map[string]any{
		"status":            "candidate",
		"candidate_id":      candidateID,
		"source":            sourceRel,
		"source_sha256":     sourceDigest,
		"source_dimensions": map[string]int{"width": 1920, "height": 1080},
		"extraction_path":   "Go image/png deterministic crop and alpha mask",
		"icons":             records,
		"evidence_sheet":    evidenceRel,
		"generated_at":      time.Now().UTC().Format(time.RFC3339),
	}
	encoded, err := json.MarshalIndent(provenance, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(assetDir, "fusion-icons.source.json"), append(encoded, '\n'), 0o644))
	fmt.Printf("extracted %d Fusion icon candidates to %s; no production assets were changed\n", len(records), candidateRel)
}

func scaleNearest(source image.Image, factor int) *image.NRGBA {
	bounds := source.Bounds()
	result := image.NewNRGBA(image.Rect(0, 0, bounds.Dx()*factor, bounds.Dy()*factor))
	for y := 0; y < result.Bounds().Dy(); y++ {
		for x := 0; x < result.Bounds().Dx(); x++ {
			result.Set(x, y, source.At(bounds.Min.X+x/factor, bounds.Min.Y+y/factor))
		}
	}
	return result
}

func extract(source image.Image, spec iconSpec) *image.NRGBA {
	width, height := spec.Box.Dx(), spec.Box.Dy()
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(result, result.Bounds(), source, spec.Box.Min, draw.Src)
	if spec.Mode == "circle" {
		cx, cy := float64(width-1)/2, float64(height-1)/2
		radius := float64(min(width, height))/2 - 0.75
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				offset := result.PixOffset(x, y)
				distance := math.Hypot(float64(x)-cx, float64(y)-cy)
				mask := math.Max(0, math.Min(1, radius+0.75-distance))
				result.Pix[offset+3] = uint8(float64(result.Pix[offset+3]) * mask)
			}
		}
		return result
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := result.PixOffset(x, y)
			red := int(result.Pix[offset])
			green := int(result.Pix[offset+1])
			blue := int(result.Pix[offset+2])
			maximum := max(red, green, blue)
			minimum := min(red, green, blue)
			// The reference glyphs are bright and chromatic while the panel is a
			// low-luminance navy. This threshold removes panel gradients without
			// cutting the antialiased cyan/orange/green strokes.
			signal := max((maximum-46)*5, (maximum-minimum-16)*6)
			alpha := max(0, min(255, signal))
			result.Pix[offset+3] = uint8(min(int(result.Pix[offset+3]), alpha))
		}
	}
	return result
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
