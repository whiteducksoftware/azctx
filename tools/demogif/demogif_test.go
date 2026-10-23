package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/whiteducksoftware/azctx/internal/testutil"
	"github.com/whiteducksoftware/azctx/internal/ui"
)

func TestDemoFramesComeFromRealPickerTransitions(t *testing.T) {
	frames, picker, err := captureFrames()
	if err != nil {
		t.Fatal(err)
	}
	assertRichInitialFrame(t, ansi.Strip(frames[0].content))

	production := filteredRows(t, frames, "prod")
	if len(production) < 2 || !allContain(production, "Production") || !containsTenants(production, "Contoso", "Fabrikam") {
		t.Fatalf("prod filter should show production names from several tenants: %q", production)
	}
	fabrikam := filteredRows(t, frames, "fab")
	if len(fabrikam) < 3 || !allContain(fabrikam, "Fabrikam") {
		t.Fatalf("tenant filter should keep only Fabrikam rows: %q", fabrikam)
	}

	index, err := picker.Selection()
	if err != nil {
		t.Fatalf("demo did not finish with a real selection: %v", err)
	}
	sorted := demoSubscriptions()
	ui.SortSubscriptions(sorted)
	if got := sorted[index]; got.Name != "Web Platform Staging" || got.TenantName != "Fabrikam" {
		t.Fatalf("demo selected %q in %q", got.Name, got.TenantName)
	}
	if last := ansi.Strip(frames[len(frames)-1].content); !strings.Contains(last, "▸ Web Platform Staging") {
		t.Fatalf("final frame does not highlight the selected row:\n%s", last)
	}
}

func assertRichInitialFrame(t *testing.T, first string) {
	t.Helper()
	for _, want := range []string{"Name", "Subscription ID", "Tenant"} {
		if !strings.Contains(first, want) {
			t.Fatalf("initial picker lost header %q:\n%s", want, first)
		}
	}
	subscriptions := demoSubscriptions()
	tenants := map[string]bool{}
	environments := map[string]bool{}
	for _, subscription := range subscriptions {
		if !strings.Contains(first, subscription.Id) {
			t.Fatalf("initial picker omits %s:\n%s", subscription.Name, first)
		}
		tenants[subscription.Tenant] = true
		for _, environment := range []string{"Dev", "Staging", "Production"} {
			if strings.Contains(subscription.Name, environment) {
				environments[environment] = true
			}
		}
	}
	if len(subscriptions) < 8 || len(tenants) < 3 || len(environments) != 3 {
		t.Fatalf("demo needs 8+ subscriptions, 3+ tenants and dev/staging/production: %d, %d, %v",
			len(subscriptions), len(tenants), environments)
	}
}

func filteredRows(t *testing.T, frames []frame, query string) []string {
	t.Helper()
	for _, frame := range frames {
		lines := strings.Split(ansi.Strip(frame.content), "\n")
		if strings.TrimSpace(lines[0]) != "Filter: "+query {
			continue
		}
		var rows []string
		for _, line := range lines[1:] {
			if strings.Contains(line, " | ") {
				rows = append(rows, strings.TrimSpace(line))
			}
		}
		return rows
	}
	t.Fatalf("no frame shows the real filter %q", query)
	return nil
}

func allContain(rows []string, text string) bool {
	return !slices.ContainsFunc(rows, func(row string) bool { return !strings.Contains(row, text) })
}

func containsTenants(rows []string, tenants ...string) bool {
	for _, tenant := range tenants {
		if !slices.ContainsFunc(rows, func(row string) bool { return strings.HasSuffix(row, "| "+tenant) }) {
			return false
		}
	}
	return true
}

func TestDemoGIFIsReadableLoopingAnimation(t *testing.T) {
	data := demoGIF(t)
	if len(data) > 700_000 {
		t.Fatalf("demo GIF is too large: %d bytes", len(data))
	}
	animation, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	frames, _, err := captureFrames()
	if err != nil {
		t.Fatal(err)
	}
	width, height := canvasSize()
	if animation.LoopCount != 0 || animation.Config.Width != width || animation.Config.Height != height {
		t.Fatalf("loop=%d size=%dx%d, want endless %dx%d",
			animation.LoopCount, animation.Config.Width, animation.Config.Height, width, height)
	}
	if width < 760 || width > 960 || len(animation.Image) != len(frames) || len(frames) < 15 {
		t.Fatalf("width=%d frames=%d captured=%d", width, len(animation.Image), len(frames))
	}
	assertReadablePauses(t, animation.Delay)
	if animation.Image[0].Bounds() != image.Rect(0, 0, width, height) {
		t.Fatalf("first frame must cover the canvas: %v", animation.Image[0].Bounds())
	}
	assertEveryFrameChanges(t, animation)
	if !hasColor(animation.Image[0], ansiColors[2]) || !hasColor(animation.Image[0], terminalBackground) {
		t.Fatal("first frame lost the terminal background or the highlighted selection color")
	}
}

func assertReadablePauses(t *testing.T, delays []int) {
	t.Helper()
	total := 0
	for index, delay := range delays {
		if delay < 25 {
			t.Fatalf("frame %d is too short to read: %d0 ms", index, delay)
		}
		total += delay
	}
	if delays[0] < 200 || delays[len(delays)-1] < 250 || total < 1200 || total > 3000 {
		t.Fatalf("pauses: first=%d last=%d total=%d centiseconds", delays[0], delays[len(delays)-1], total)
	}
}

func assertEveryFrameChanges(t *testing.T, animation *gif.GIF) {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, animation.Config.Width, animation.Config.Height))
	var previous []byte
	for index, paletted := range animation.Image {
		if !paletted.Bounds().In(canvas.Bounds()) || animation.Disposal[index] != gif.DisposalNone {
			t.Fatalf("frame %d bounds=%v disposal=%d", index, paletted.Bounds(), animation.Disposal[index])
		}
		draw.Draw(canvas, paletted.Bounds(), paletted, paletted.Bounds().Min, draw.Src)
		if bytes.Equal(canvas.Pix, previous) {
			t.Fatalf("frame %d does not visibly change the animation", index)
		}
		previous = slices.Clone(canvas.Pix)
	}
}

func hasColor(img *image.Paletted, want color.RGBA) bool {
	for _, index := range img.Pix {
		if color.RGBAModel.Convert(img.Palette[index]) == want {
			return true
		}
	}
	return false
}

// generatedDemo runs the real generator once for the tests that only inspect its output.
var generatedDemo = sync.OnceValues(generate)

// demoGIF returns a private copy so no test can alter another test's bytes.
func demoGIF(t *testing.T) []byte {
	t.Helper()
	data, err := generatedDemo()
	if err != nil {
		t.Fatal(err)
	}
	return slices.Clone(data)
}

func TestDemoGenerationIsByteForByteRepeatable(t *testing.T) {
	path := filepath.Join(testutil.WorkDir(t), "demo.gif")
	if err := writeOrCheckData(path, demoGIF(t), false); err != nil {
		t.Fatal(err)
	}
	// Check mode runs a second, independent generation and compares the bytes.
	if err := writeOrCheck(path, true); err != nil {
		t.Fatalf("regeneration changed the GIF bytes: %v", err)
	}
}

func TestCommittedDemoMatchesGenerator(t *testing.T) {
	if err := writeOrCheckData(filepath.Join("..", "..", defaultOutput), demoGIF(t), true); err != nil {
		t.Fatalf("%v; run go run ./tools/demogif from the repository root", err)
	}
}

func TestCheckModeRejectsStaleOrMissingOutputWithoutRewriting(t *testing.T) {
	dir := testutil.WorkDir(t)
	data := demoGIF(t)
	missing := filepath.Join(dir, "missing.gif")
	if err := writeOrCheckData(missing, data, true); err == nil {
		t.Fatal("check accepted a missing GIF")
	}
	if _, err := os.Stat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("check created the missing GIF: %v", err)
	}
	path := filepath.Join(dir, "demo.gif")
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeOrCheckData(path, data, true); err == nil {
		t.Fatal("check accepted a stale GIF")
	}
	if current, err := os.ReadFile(path); err != nil || string(current) != "stale" {
		t.Fatalf("check rewrote the file: %q, %v", current, err)
	}
	if err := writeOrCheckData(path, data, false); err != nil {
		t.Fatal(err)
	}
	if err := writeOrCheckData(path, data, true); err != nil {
		t.Fatalf("freshly written GIF failed its check: %v", err)
	}
}

func TestTerminalParsesStyledCells(t *testing.T) {
	screen, err := parseScreen("\x1b[1;32m▸ a\x1b[m b\x1b[38;2;1;2;3mc\x1b[7m \x1b[m\nx", 7, 2)
	if err != nil {
		t.Fatal(err)
	}
	if screen[0][0].text != "▸" || !screen[0][0].style.bold || screen[0][0].style.fg != ansiColors[2] {
		t.Fatalf("styled selection marker: %+v", screen[0][0])
	}
	if screen[0][3].text != " " || screen[0][3].style != (cellStyle{}) {
		t.Fatalf("reset did not clear style: %+v", screen[0][3])
	}
	truecolor := color.RGBA{1, 2, 3, 255}
	if screen[0][5].style != (cellStyle{fg: truecolor}) || screen[0][6].style != (cellStyle{fg: truecolor, reverse: true}) {
		t.Fatalf("truecolor/reverse: %+v %+v", screen[0][5], screen[0][6])
	}
	if screen[1][0].text != "x" || screen[1][1].text != " " {
		t.Fatalf("second line: %+v", screen[1][:2])
	}
}

func TestTerminalRejectsUnsupportedOutput(t *testing.T) {
	for name, content := range map[string]string{
		"cursor movement": "\x1b[2J",
		"blink":           "\x1b[5mx",
		"too wide":        "abcdefg",
		"too tall":        "a\nb\nc",
		"OSC":             "\x1b]11;?\x1b\\",
	} {
		if _, err := parseScreen(content, 6, 2); err == nil {
			t.Errorf("%s was rendered instead of rejected", name)
		}
	}
}

func TestRendererRejectsMissingGlyphs(t *testing.T) {
	screen, err := parseScreen("❯", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := newRenderer()
	if err != nil {
		t.Fatal(err)
	}
	target := image.NewPaletted(image.Rect(0, 0, 40, 40), renderer.palette)
	if err := renderer.drawCells(target, screen, image.Point{}); err == nil {
		t.Fatal("renderer silently drew a missing glyph")
	}
}

func TestRasterizerUsesExactIntegerCoverage(t *testing.T) {
	// A 2×1 pixel rectangle from x=0.5 to x=2.5 (26.6 units), wound clockwise.
	square := []edge{
		{point{32, 0}, point{160, 0}},
		{point{160, 0}, point{160, 64}},
		{point{160, 64}, point{32, 64}},
		{point{32, 64}, point{32, 0}},
	}
	mask, err := rasterize(square)
	if err != nil {
		t.Fatal(err)
	}
	if mask.bounds != image.Rect(0, 0, 3, 1) {
		t.Fatalf("bounds = %v", mask.bounds)
	}
	if got := []int{mask.at(0, 0), mask.at(1, 0), mask.at(2, 0)}; !slices.Equal(got, []int{32, 64, 32}) {
		t.Fatalf("coverage = %v, want half, full, half", got)
	}
	if got := quadPoint(point{0, 0}, point{64, 128}, point{128, 0}, curveSteps/2); got != (point{64, 64}) {
		t.Fatalf("quadratic midpoint = %v", got)
	}
	if _, err := rasterize(nil); err == nil {
		t.Fatal("empty outline was accepted")
	}
}
