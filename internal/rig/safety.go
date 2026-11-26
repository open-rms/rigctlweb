package rig

import "strings"

// SafeFuncs are the Hamlib functions that other programs (API v1 and openRMS Rigs presets)
// may change: receive-side and setup functions that cannot make the radio transmit. It is an
// allow-list on purpose: a function Hamlib adds later stays refused until it is looked at.
// The openRMS Rigs profile validator uses the same list.
//
// Left out, from Hamlib's rig.h: VOX, SBKIN, FBKIN, TBURST, SEND_MORSE, SEND_VOICE_MEM (they
// transmit on their own); TUNER (on some radios switching it on starts a tune cycle);
// TRANSCEIVE (changes the radio's auto-reporting under rigctld); SCEN (scrambling); SATMODE,
// SLICE, SYNC and OVF_STATUS (not needed yet).
var SafeFuncs = map[string]bool{}

func init() {
	for _, f := range strings.Fields(`FAGC NB NB2 COMP TONE TSQL CSQL DSQL ANF NR AIP APF MON MN RF ARO LOCK
		MUTE VSC REV SQL ABM BC MBC BC2 RIT XIT AFC SCOPE RESUME ANL AFLT DUAL_WATCH DIVERSITY SPECTRUM SPECTRUM_HOLD`) {
		SafeFuncs[f] = true
	}
}

// keyingFuncs can make the radio transmit without a PTT command.
var keyingFuncs = map[string]bool{"VOX": true, "SBKIN": true, "FBKIN": true, "TBURST": true, "SEND_MORSE": true, "SEND_VOICE_MEM": true, "TUNER": true}

// IsKeyingFunc reports whether a function can make the radio transmit by itself.
func IsKeyingFunc(name string) bool {
	n := strings.ToUpper(name)
	return keyingFuncs[n] || strings.HasPrefix(n, "SEND_")
}
