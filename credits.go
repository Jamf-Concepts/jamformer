// Copyright 2026, Jamf Software LLC

package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"
)

// playTransformSound generates a robotic mechanical transformer sound as a WAV
// file, writes it to a temp file, and plays it via afplay (macOS).
// Best-effort: errors are silently ignored (non-macOS, missing afplay, etc.).
func playTransformSound() {
	wav := generateTransformWAV()

	f, err := os.CreateTemp("", "jamformer-credits-*.wav")
	if err != nil {
		return
	}
	path := f.Name()
	if _, err := f.Write(wav); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return
	}
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()

	cmd := exec.Command("afplay", path)
	_ = cmd.Run() // blocks until playback finishes, which is fine in a goroutine
}

// generateTransformWAV synthesises a short robotic mechanical transformation
// sound and returns it as a complete WAV file in memory.
func generateTransformWAV() []byte {
	const (
		sampleRate = 44100
		channels   = 1
		bitsPerSmp = 16
	)

	// Helper: generate samples for a duration
	numSamples := func(d time.Duration) int {
		return int(float64(sampleRate) * d.Seconds())
	}

	// Clamp to int16 range
	clamp := func(v float64) int16 {
		if v > 1.0 {
			v = 1.0
		}
		if v < -1.0 {
			v = -1.0
		}
		return int16(v * 32767)
	}

	// Square wave with controllable duty cycle (harsh, mechanical)
	square := func(phase, duty float64) float64 {
		p := math.Mod(phase, 1.0)
		if p < duty {
			return 1.0
		}
		return -1.0
	}

	// Sawtooth wave (buzzy, gear-like)
	saw := func(phase float64) float64 {
		return 2.0*math.Mod(phase, 1.0) - 1.0
	}

	var samples []int16

	// ── Phase 1: Grinding gear wind-down ─────────────────────────
	// Harsh square+saw sweep downward with grinding texture
	n := numSamples(300 * time.Millisecond)
	for i := range n {
		t := float64(i) / float64(sampleRate)
		progress := float64(i) / float64(n)
		freq := 1800.0 - 1500.0*progress
		phase := freq * t
		// Layered harsh waveforms
		v := 0.35 * square(phase, 0.3+0.2*progress)
		v += 0.25 * saw(phase*1.5)
		v += 0.15 * square(phase*2.0, 0.15) // high harmonic grind
		// Grinding texture: amplitude modulation at gear-tooth rate
		grind := 0.5 + 0.5*square(t*45, 0.4)
		env := 1.0 - 0.4*progress
		samples = append(samples, clamp(v*grind*env*0.55))
	}

	// ── Phase 2: Heavy metallic impacts (clunk-clunk-clunk) ──────
	for hit := range 3 {
		impactDur := numSamples(40 * time.Millisecond)
		for i := range impactDur {
			t := float64(i) / float64(sampleRate)
			progress := float64(i) / float64(impactDur)
			// Layered ring-mod for metallic clank
			metal := math.Sin(2*math.Pi*600*t) * math.Sin(2*math.Pi*2800*t)
			clank := square(900*t, 0.1) * math.Exp(-progress*15)
			body := math.Sin(2*math.Pi*120*t) * math.Exp(-progress*8)
			env := math.Exp(-progress * 10)
			v := (metal*0.5 + clank*0.3 + body*0.4) * env
			samples = append(samples, clamp(v*0.75))
		}
		gap := numSamples(time.Duration(30-hit*8) * time.Millisecond)
		for range gap {
			samples = append(samples, 0)
		}
	}

	// ── Phase 3: Pneumatic hiss + ratchet ────────────────────────
	// Noise burst shaped like compressed air, with clicking ratchet
	n = numSamples(200 * time.Millisecond)
	phase := 0.0
	for i := range n {
		t := float64(i) / float64(sampleRate)
		progress := float64(i) / float64(n)
		// Pseudo-noise via high-freq ring modulation
		hiss := math.Sin(2*math.Pi*7500*t) * math.Sin(2*math.Pi*4300*t) *
			math.Sin(2*math.Pi*11000*t)
		// Ratchet: rapid square clicks accelerating
		ratchetFreq := 20.0 + 80.0*progress
		phase += ratchetFreq / float64(sampleRate)
		ratchet := square(phase, 0.08) * 0.4
		// Envelope: sharp attack, sustain, quick fade
		env := math.Min(progress*20, 1.0) * (1.0 - progress*progress*progress)
		samples = append(samples, clamp((hiss*0.3+ratchet)*env*0.6))
	}

	// ── Phase 4: Servo motor wind-up (ascending grind) ───────────
	n = numSamples(350 * time.Millisecond)
	for i := range n {
		t := float64(i) / float64(sampleRate)
		progress := float64(i) / float64(n)
		freq := 80.0 + 2400.0*progress*progress
		p := freq * t
		// Harsh layered waveforms
		v := 0.3 * saw(p)
		v += 0.3 * square(p, 0.2+0.3*progress)
		v += 0.15 * saw(p*3.01) // detuned harmonic for roughness
		// Gear-tooth amplitude modulation (accelerating)
		toothRate := 30.0 + 120.0*progress
		grind := 0.6 + 0.4*math.Sin(2*math.Pi*toothRate*t)
		env := math.Min(progress*5, 1.0)
		samples = append(samples, clamp(v*grind*env*0.5))
	}

	// ── Phase 5: Final locking impacts + settling ────────────────
	// Two heavy thuds
	for range 2 {
		thudDur := numSamples(50 * time.Millisecond)
		for i := range thudDur {
			t := float64(i) / float64(sampleRate)
			progress := float64(i) / float64(thudDur)
			body := math.Sin(2*math.Pi*80*t) * math.Exp(-progress*6)
			metal := math.Sin(2*math.Pi*1400*t) * math.Sin(2*math.Pi*3100*t) * math.Exp(-progress*12)
			clank := square(2200*t, 0.05) * math.Exp(-progress*18)
			samples = append(samples, clamp((body*0.5+metal*0.35+clank*0.2)*0.8))
		}
		for range numSamples(40 * time.Millisecond) {
			samples = append(samples, 0)
		}
	}

	// Mechanical settling: low buzz winding down
	n = numSamples(300 * time.Millisecond)
	for i := range n {
		t := float64(i) / float64(sampleRate)
		progress := float64(i) / float64(n)
		freq := 150.0 - 90.0*progress
		v := 0.4 * saw(freq*t)
		v += 0.2 * square(freq*t, 0.3)
		// Decelerating gear teeth
		toothRate := 40.0 * (1.0 - progress*0.8)
		grind := 0.7 + 0.3*square(t*toothRate, 0.3)
		env := (1.0 - progress) * (1.0 - progress)
		samples = append(samples, clamp(v*grind*env*0.4))
	}

	// ── Encode as WAV ────────────────────────────────────────────
	dataSize := len(samples) * 2
	fileSize := 36 + dataSize

	buf := make([]byte, 0, 44+dataSize)
	// RIFF header
	buf = append(buf, 'R', 'I', 'F', 'F')
	buf = binary.LittleEndian.AppendUint32(buf, uint32(fileSize))
	buf = append(buf, 'W', 'A', 'V', 'E')
	// fmt sub-chunk
	buf = append(buf, 'f', 'm', 't', ' ')
	buf = binary.LittleEndian.AppendUint32(buf, 16) // sub-chunk size
	buf = binary.LittleEndian.AppendUint16(buf, 1)  // PCM format
	buf = binary.LittleEndian.AppendUint16(buf, channels)
	buf = binary.LittleEndian.AppendUint32(buf, sampleRate)
	buf = binary.LittleEndian.AppendUint32(buf, sampleRate*channels*bitsPerSmp/8) // byte rate
	buf = binary.LittleEndian.AppendUint16(buf, channels*bitsPerSmp/8)            // block align
	buf = binary.LittleEndian.AppendUint16(buf, bitsPerSmp)
	// data sub-chunk
	buf = append(buf, 'd', 'a', 't', 'a')
	buf = binary.LittleEndian.AppendUint32(buf, uint32(dataSize))
	for _, s := range samples {
		buf = binary.LittleEndian.AppendUint16(buf, uint16(s))
	}

	return buf
}

func printCredits() {
	const (
		reset    = "\033[0m"
		bold     = "\033[1m"
		dim      = "\033[2m"
		clearScr = "\033[2J\033[H"
		hideCur  = "\033[?25l"
		showCur  = "\033[?25h"
	)

	type rgb struct{ r, g, b int }
	blueC := rgb{29, 108, 232}
	purpleC := rgb{99, 56, 228}
	goldC := rgb{255, 200, 60}
	whiteC := rgb{220, 220, 230}
	grayC := rgb{120, 120, 130}

	colorStr := func(c rgb) string {
		return fmt.Sprintf("\033[38;2;%d;%d;%dm", c.r, c.g, c.b)
	}
	lerpColor := func(a, b rgb, t float64) rgb {
		return rgb{
			r: a.r + int(float64(b.r-a.r)*t),
			g: a.g + int(float64(b.g-a.g)*t),
			b: a.b + int(float64(b.b-a.b)*t),
		}
	}

	// Credit entries: {text, color, pause}
	type credit struct {
		text  string
		color rgb
		pause time.Duration
	}

	credits := []credit{
		{"", whiteC, 200 * time.Millisecond},
		{"J A M F O R M E R", blueC, 600 * time.Millisecond},
		{"", whiteC, 300 * time.Millisecond},
		{"Jamf → Terraform", purpleC, 400 * time.Millisecond},
		{"", whiteC, 600 * time.Millisecond},

		{"─── Maintainers ───", grayC, 300 * time.Millisecond},
		{"", whiteC, 100 * time.Millisecond},
		{"Neil Martin", whiteC, 300 * time.Millisecond},
		{"Dan Cuddeford", whiteC, 300 * time.Millisecond},
		{"Jamf Concepts", blueC, 400 * time.Millisecond},
		{"", whiteC, 500 * time.Millisecond},

		{"─── Terraform Providers ───", grayC, 300 * time.Millisecond},
		{"", whiteC, 100 * time.Millisecond},
		{"Jamf Pro Provider", blueC, 200 * time.Millisecond},
		{"Dafydd Watkins / Joseph Little / Bobby Williams (Deployment Theory)", grayC, 300 * time.Millisecond},
		{"", whiteC, 200 * time.Millisecond},
		{"Jamf Protect Provider", purpleC, 200 * time.Millisecond},
		{"James Smith / Jamf Concepts", grayC, 300 * time.Millisecond},
		{"", whiteC, 200 * time.Millisecond},
		{"Jamf Platform Provider", rgb{71, 76, 230}, 200 * time.Millisecond},
		{"Jamf Concepts", grayC, 300 * time.Millisecond},
		{"", whiteC, 200 * time.Millisecond},
		{"JSC Provider", goldC, 200 * time.Millisecond},
		{"Jamf Concepts", grayC, 400 * time.Millisecond},
		{"", whiteC, 500 * time.Millisecond},

		{"─── Built with ───", grayC, 300 * time.Millisecond},
		{"", whiteC, 100 * time.Millisecond},
		{"Go", whiteC, 150 * time.Millisecond},
		{"HashiCorp Terraform", purpleC, 150 * time.Millisecond},
		{"hclwrite", grayC, 150 * time.Millisecond},
		{"tfexec", grayC, 150 * time.Millisecond},
		{"go-api-sdk-jamfpro", grayC, 400 * time.Millisecond},
		{"", whiteC, 500 * time.Millisecond},

		{"─── Special thanks to the Beta Testers ───", grayC, 300 * time.Millisecond},
		{"", whiteC, 100 * time.Millisecond},
		{"Aaron Bonsall", whiteC, 150 * time.Millisecond},
		{"Aaron Polley", whiteC, 150 * time.Millisecond},
		{"Adam Codega", whiteC, 150 * time.Millisecond},
		{"Aiden", whiteC, 150 * time.Millisecond},
		{"Aleyna Arslan", whiteC, 150 * time.Millisecond},
		{"Andrew Barnett", whiteC, 150 * time.Millisecond},
		{"Arnold", whiteC, 150 * time.Millisecond},
		{"cander", whiteC, 150 * time.Millisecond},
		{"Dafydd Watkins", whiteC, 150 * time.Millisecond},
		{"Eric Holtam", whiteC, 150 * time.Millisecond},
		{"Eric N", whiteC, 150 * time.Millisecond},
		{"Girish Ganesh", whiteC, 150 * time.Millisecond},
		{"Harish M", whiteC, 150 * time.Millisecond},
		{"Hector Alcala", whiteC, 150 * time.Millisecond},
		{"Ian Thompson", whiteC, 150 * time.Millisecond},
		{"Jacob Burley", whiteC, 150 * time.Millisecond},
		{"James Smith", whiteC, 150 * time.Millisecond},
		{"Jarred Wheeler", whiteC, 150 * time.Millisecond},
		{"Jason Phegley", whiteC, 150 * time.Millisecond},
		{"Jerry Morrison", whiteC, 150 * time.Millisecond},
		{"Jonathan Porter", whiteC, 150 * time.Millisecond},
		{"Jordy Thery", whiteC, 150 * time.Millisecond},
		{"Jose", whiteC, 150 * time.Millisecond},
		{"Joseph Little", whiteC, 150 * time.Millisecond},
		{"Ju", whiteC, 150 * time.Millisecond},
		{"Kevin Matan", whiteC, 150 * time.Millisecond},
		{"Kyle Hoare", whiteC, 150 * time.Millisecond},
		{"Luke Charters", whiteC, 150 * time.Millisecond},
		{"Matt Brown", whiteC, 150 * time.Millisecond},
		{"Matt Foust", whiteC, 150 * time.Millisecond},
		{"Matt Jerome", whiteC, 150 * time.Millisecond},
		{"Matthew Ward", whiteC, 150 * time.Millisecond},
		{"Mauricio Pellizzon", whiteC, 150 * time.Millisecond},
		{"Melwin Moeskops", whiteC, 150 * time.Millisecond},
		{"Nick F", whiteC, 150 * time.Millisecond},
		{"Niels Vermeulen", whiteC, 150 * time.Millisecond},
		{"Oscar Reyes", whiteC, 150 * time.Millisecond},
		{"Q. Bajeena", whiteC, 150 * time.Millisecond},
		{"Rob Lee", whiteC, 150 * time.Millisecond},
		{"Robbie Trencheny", whiteC, 150 * time.Millisecond},
		{"Robert Schroeder", whiteC, 150 * time.Millisecond},
		{"Ryan Legg", whiteC, 150 * time.Millisecond},
		{"Ryon Riley", whiteC, 150 * time.Millisecond},
		{"Viktor Glemme", whiteC, 150 * time.Millisecond},
		{"", whiteC, 500 * time.Millisecond},

		{"── concepts.jamf.com ──", grayC, 600 * time.Millisecond},
		{"", whiteC, 400 * time.Millisecond},
		{"© Jamf Software LLC 2026", grayC, 600 * time.Millisecond},
		{"", whiteC, 200 * time.Millisecond},
		{"Made with ❤ and too much coffee.", goldC, 0},
	}

	enableColors()
	fmt.Print(clearScr)
	fmt.Print(hideCur)

	// Play synthesised transformer sound in the background (best-effort)
	go playTransformSound()

	// Typing effect for a single line, centered to 78 cols
	typeLine := func(text string, c rgb, charDelay time.Duration) {
		pad := max((78-len([]rune(text)))/2, 0)
		fmt.Print(strings.Repeat(" ", pad))
		fmt.Print(colorStr(c) + bold)
		for _, ch := range text {
			fmt.Print(string(ch))
			if charDelay > 0 {
				time.Sleep(charDelay)
			}
		}
		fmt.Print(reset)
		fmt.Println()
	}

	// Roll through credits with typing effect
	for i, c := range credits {
		if c.text == "" {
			fmt.Println()
		} else if strings.HasPrefix(c.text, "───") || strings.HasPrefix(c.text, "──") {
			// Section headers: appear instantly, dimmed
			pad := max((78-len([]rune(c.text)))/2, 0)
			fmt.Println(dim + colorStr(c.color) + strings.Repeat(" ", pad) + c.text + reset)
		} else if i == 1 {
			// Title line: slow typing with gradient
			pad := max((78-len([]rune(c.text)))/2, 0)
			fmt.Print(strings.Repeat(" ", pad))
			runes := []rune(c.text)
			for j, ch := range runes {
				t := float64(j) / float64(max(len(runes)-1, 1))
				col := lerpColor(blueC, purpleC, t)
				fmt.Print(bold + colorStr(col) + string(ch) + reset)
				time.Sleep(60 * time.Millisecond)
			}
			fmt.Println()
		} else {
			typeLine(c.text, c.color, 20*time.Millisecond)
		}
		if c.pause > 0 {
			time.Sleep(c.pause)
		}
	}

	fmt.Println()
	fmt.Print(showCur)
}
