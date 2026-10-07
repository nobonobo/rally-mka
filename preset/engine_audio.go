// engine_audio.go
package preset

import (
	"math/rand/v2"

	"github.com/mokiat/wasmal"
)

// nodeAs は AudioNode を具体的なノード型へ型アサーションします。
func nodeAs[T any](n wasmal.AudioNode) T {
	return n.(T)
}

var (
	audioCtx   wasmal.AudioContext
	oscNode    wasmal.OscillatorNode
	oscNode2   wasmal.OscillatorNode   // 低音用サブトーン
	filterNode wasmal.BiquadFilterNode // ローパスフィルタ
	gainNode   wasmal.GainNode
	audioInit  bool
)

// initAudio creates a persistent AudioContext, oscillator and gain node.
func initAudio() {
	if audioInit {
		return
	}
	audioCtx = wasmal.NewAudioContext()
	// 主オシレータ
	oscNode = audioCtx.CreateOscillator()
	oscNode.SetType(wasmal.OscillatorTypeSawtooth)
	// 低音サブオシレータ
	oscNode2 = audioCtx.CreateOscillator()
	oscNode2.SetType(wasmal.OscillatorTypeSawtooth)
	// ローパスフィルタ（低域を中心に残す）
	filterNode = audioCtx.CreateBiquadFilter()
	filterNode.SetType(wasmal.BiquadFilterTypeLowpass)
	// Gain
	gainNode = audioCtx.CreateGain()
	gainNode.Gain().SetValue(0.0) // ミュートでスタート
	// 接続順序: osc → filter → gain
	oscNode.ConnectToNode(filterNode)
	oscNode2.ConnectToNode(filterNode)
	filterNode.ConnectToNode(gainNode)
	gainNode.ConnectToNode(audioCtx.Destination())
	// 起動（ここでは即時開始しておき、周波数は後から変更）
	oscNode.Start(0)
	oscNode2.Start(0)
	audioInit = true
}

// InitAudio is an exported wrapper to initialise the audio system.
func InitAudio() {
	initAudio()
}

// rpmToFreq maps a RPM value to a frequency (Hz).
func rpmToFreq(rpm float64) float64 {
	if rpm < 0 {
		rpm = 0
	}
	return 10 + (rpm/8000.0)*300
}

// playEngine updates oscillator frequency based on RPM and ramps volume up.
func playEngine(rpm float64) {
	freq := rpmToFreq(rpm)
	// jitter (±2%)
	jitter := 0.98 + rand.Float64()*0.04
	freqJ := freq * jitter
	now := audioCtx.CurrentTime()
	// 主オシレータはそのまま rpm に比例
	oscNode.Frequency().SetValueAtTime(float32(freqJ), now)
	// サブオシレータは半分の周波数で低音を強化
	oscNode2.Frequency().SetValueAtTime(float32(freqJ*0.5), now)
	// ローパスフィルタのカットオフを RPM に合わせて上げる
	// カットオフは 800Hz (idle) から 4000Hz (max) へ線形に
	cutoff := 800.0 + (rpm/8000.0)*(4000.0-800.0)
	filterNode.Frequency().SetValueAtTime(float32(cutoff), now)
	// ボリュームは RPM が上がるほど少しだけ増やす（自然なフェードイン）
	targetGain := 0.05 + (rpm/8000.0)*0.35 // 0.05〜0.4 の範囲
	gainNode.Gain().LinearRampToValueAtTime(float32(targetGain), now+0.05)
}

// stopEngine fades out the sound without stopping the oscillator.
func stopEngine() {
	if !audioInit {
		return
	}
	now := audioCtx.CurrentTime()
	gainNode.Gain().LinearRampToValueAtTime(0.0, now+0.2)
}
