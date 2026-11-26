/**
 * Tooltip / help text for rigctlWeb controls.
 *
 * Keys for LEVEL/FUNC/PARM/MODE/VFO_OP are the names Hamlib uses (as reported by
 * rigctld, e.g. `\get_level`, `\dump_caps`), taken from hamlib/rig.h (Hamlib 4.7).
 * Behaviour differs between radios; text says so where it matters.
 */

export type HelpKind = 'level' | 'func' | 'parm' | 'mode' | 'vfoop' | 'ui';

/* ------------------------------------------------------------------ */
/* Levels                                                              */
/* ------------------------------------------------------------------ */
export const LEVEL_HELP: Record<string, string> = {
  AF: 'Audio (speaker/headphone) volume. Only changes how loud you hear the radio; it has no effect on the transmitted signal.',
  AGC: 'Automatic gain control speed: how fast the receiver gain recovers after a strong signal. Fast suits CW and weak-signal work; slow suits SSB listening.',
  AGC_TIME: 'AGC decay time in seconds, for radios that let you set it directly. Shorter recovers faster after a strong signal; longer holds gain steadier. Radio dependent.',
  ALC: 'Read-only transmit meter showing automatic level control action. Keep it at or just below the radio ALC limit; over-driving causes distorted, splattering signals.',
  ANTIVOX: 'Anti-VOX level: how much speaker audio is rejected so received audio does not trigger VOX transmit. Raise it if your speakers key the radio.',
  APF: 'Audio peak filter amount. Boosts a narrow band of audio around the CW pitch to help pick a weak CW signal out of noise.',
  ATT: 'Receive attenuator in dB. Reduces all incoming signals to prevent overload from strong nearby stations; available steps depend on the radio.',
  BAND_SELECT: 'Selects the operating band by number or name on radios that support band selection over CAT. Radio dependent.',
  BKINDL: 'CW break-in delay (hang time) before the radio returns to receive after you stop keying, in tenths of a dot length.',
  BKIN_DLYMS: 'CW break-in delay (hang time) before the radio returns to receive after keying stops, in milliseconds.',
  BAL: 'Balance between the main and sub receiver audio when dual watch is on. Radio dependent.',
  COMP: 'Speech compressor level. Raises average SSB talk power for more punch in weak conditions; too much sounds distorted.',
  COMP_METER: 'Read-only meter showing how many dB of speech compression is being applied while transmitting. Typically aim for modest values (a few to ~10 dB).',
  CWPITCH: 'CW sidetone and receive pitch in Hz (commonly 500 to 800 Hz). Set it to a tone comfortable to you; it also sets where signals sit in the passband.',
  ID_METER: 'Read-only meter of the radio final amplifier current draw in amperes while transmitting.',
  IF: 'IF shift in Hz: slides the receive passband up or down to move interference off one side of the signal without changing the tuned frequency.',
  KEYSPD: 'Built-in electronic keyer speed in words per minute (WPM).',
  METER: 'Selects what the radio own front-panel meter displays (such as SWR, compression, ALC or power out) on radios that allow this to be switched over CAT.',
  MICGAIN: 'Microphone gain. Set so voice peaks reach the ALC limit but do not push it past; too high causes distortion and splatter.',
  MONITOR_GAIN: 'Volume of the transmit monitor (hearing your own transmitted audio in the headphones). Only useful when monitor is enabled.',
  NB: 'Noise blanker level. Strips short, sharp impulse noise such as ignition or power-line pulses; too high can distort strong signals.',
  NOTCHF: 'Manual notch filter frequency in Hz. Place the narrow notch on a steady carrier or whistle to remove it from the received audio.',
  NOTCHF_RAW: 'Manual notch filter position as a 0 to 1 fraction of its range instead of Hz. Used by radios that do not report the notch in Hz.',
  NR: 'Noise reduction (DSP) level. Reduces hiss and random noise on weak signals; high levels can make audio sound watery or remove weak signals.',
  PBT_IN: 'Passband tuning, inner edge. With PBT_OUT it narrows or shifts the passband from opposite sides to cut adjacent-channel interference.',
  PBT_OUT: 'Passband tuning, outer edge. With PBT_IN it narrows or shifts the passband from opposite sides to cut adjacent-channel interference.',
  PREAMP: 'Receive preamplifier in dB. Boosts weak signals on quiet bands (10 m and up); leave off on noisy lower bands where it only amplifies noise and can overload.',
  RAWSTR: 'Read-only raw signal strength value from the radio converter. Scale is specific to each radio, so use STRENGTH or the S-meter for meaningful readings.',
  RF: 'Receiver RF gain (not transmit power). Turning it down reduces gain and can tame strong signals and noise; full gain is normal for weak signals.',
  RFPOWER: 'Transmit power setting as a fraction of the radio maximum. Use the lowest power that gets the contact, and keep it low while tuning.',
  RFPOWER_METER: 'Read-only transmit power output as a fraction of the radio maximum power.',
  RFPOWER_METER_WATTS: 'Read-only transmit power output in watts.',
  SLOPE_HIGH: 'Slope tuning, high-frequency cut in Hz: trims the top edge of the receive passband to reduce interference from higher-pitched signals.',
  SLOPE_LOW: 'Slope tuning, low-frequency cut in Hz: trims the bottom edge of the receive passband to reduce interference from lower-pitched signals.',
  SPECTRUM_ATT: 'Attenuator applied to the spectrum scope input in dB. Supported values depend on the radio.',
  SPECTRUM_AVG: 'Averaging mode for the spectrum scope. More averaging smooths the display and makes weak signals easier to see but responds slower. Radio dependent.',
  SPECTRUM_EDGE_HIGH: 'Upper frequency edge in Hz of the spectrum scope when it is in fixed-range mode.',
  SPECTRUM_EDGE_LOW: 'Lower frequency edge in Hz of the spectrum scope when it is in fixed-range mode.',
  SPECTRUM_MODE: 'Spectrum scope mode, such as centered on the tuned frequency or a fixed range. Available modes depend on the radio.',
  SPECTRUM_REF: 'Spectrum scope reference level in dB: moves the displayed signal level up or down on the scope.',
  SPECTRUM_SPAN: 'Spectrum scope width in Hz when centered on the tuned frequency. Narrower shows more detail; wider shows more of the band.',
  SPECTRUM_SPEED: 'How fast the spectrum scope updates. Higher is faster; steps depend on the radio.',
  SQL: 'Squelch threshold. Mutes the receiver until a signal is stronger than the setting; mostly used on FM, normally set just above the point where noise stops.',
  SQLSTAT: 'Deprecated squelch-open status. Hamlib suggests using the DCD (carrier detect) reading instead.',
  STRENGTH: 'Read-only calibrated signal strength in dB relative to S9. Zero is S9, negative is below S9, positive is dB over S9.',
  SWR: 'Read-only standing wave ratio meter while transmitting. Below about 1.5 is excellent and 2.0 acceptable; above 3 reduce power and check the antenna.',
  TEMP_METER: 'Read-only temperature of the radio final amplifier or heat sink in degrees Celsius. Watch it during long, high-duty transmissions.',
  USB_AF: 'Level of the audio sent out over the radio USB or ACC audio connection, for digital-mode software.',
  USB_AF_INPUT: 'Level of the audio received into the radio from the USB or ACC audio connection (your digital-mode software). Keep ALC from moving when transmitting data.',
  VD_METER: 'Read-only supply voltage to the radio (final amplifier) in volts. A large drop while transmitting suggests a weak supply or thin power wiring.',
  VOXDELAY: 'VOX hang time: how long the radio stays in transmit after you stop talking, in tenths of a second.',
  VOXGAIN: 'VOX sensitivity: how loud you must speak into the microphone to trigger transmit. Raise it if VOX does not trigger, lower it if it keys on room noise.',
};

/* ------------------------------------------------------------------ */
/* Functions (on/off toggles)                                          */
/* ------------------------------------------------------------------ */
export const FUNC_HELP: Record<string, string> = {
  ABM: 'Auto band mode: the radio picks a typical mode (such as LSB or USB) automatically when you change band. Radio dependent.',
  AFC: 'Automatic frequency control. Makes the radio follow a slightly drifting signal, mostly for FM and some receivers.',
  AFLT: 'Audio filter setting on the receiver. Radio dependent.',
  AIP: 'Advanced intercept point / RF preamp setting (called AIP on Kenwood, IPO on Yaesu). Changes the receiver front end to favor strong-signal handling over sensitivity.',
  ANF: 'Automatic notch filter (DSP). Automatically finds and removes steady carriers or tones in the receive audio, useful against heterodynes on SSB.',
  ANL: 'Automatic noise limiter. Clips noise peaks in the audio; a simple way to reduce impulse noise.',
  APF: 'Audio peak filter. Boosts a narrow band of audio around the CW pitch to help a weak CW signal stand out.',
  ARO: 'Automatic repeater offset: the radio applies the standard repeater shift automatically for the band and frequency.',
  BC: 'Beat canceller. Removes a steady interfering beat tone from the receive audio. Radio dependent.',
  BC2: 'Second beat canceller mode. Radio dependent.',
  COMP: 'Speech compression on or off. Increases average SSB power for a stronger-sounding signal; overdoing it causes distortion.',
  CSQL: 'DCS (digital coded squelch) on receive: the radio only opens the squelch when the matching DCS code is received.',
  DIVERSITY: 'Diversity receive: combines two receivers or antennas to reduce fading. Radio dependent.',
  DSQL: 'Digital squelch: opens the squelch only for matching digital-mode signals. Radio dependent.',
  DUAL_WATCH: 'Dual watch or sub receiver: listens on a second frequency at the same time as the main one. Radio dependent.',
  FAGC: 'Fast AGC. Makes receiver gain recover quickly after strong signals, which helps with CW and noisy conditions.',
  FBKIN: 'Full break-in (QSK) for CW: the radio switches to receive between each dit and dah, so you can hear the band while sending.',
  LOCK: 'Locks the tuning dial or controls to avoid accidental frequency changes. Exact scope depends on the radio.',
  MBC: 'Manual beat canceller. Lets you tune out a steady beat tone from the receive audio. Radio dependent.',
  MN: 'Manual notch filter on or off. Use it to remove a steady carrier or whistle from the received audio.',
  MON: 'Transmit monitor: lets you hear your own transmitted audio in the headphones, useful for checking SSB audio quality.',
  MUTE: 'Mutes the receiver audio.',
  NB: 'Noise blanker. Suppresses short, sharp impulse noise such as ignition or power-line pulses.',
  NB2: 'Second noise blanker, usually tuned for a different type of impulse noise than the first one.',
  NR: 'DSP noise reduction. Reduces hiss and random noise on weak signals; too much can make audio sound artificial.',
  OVF_STATUS: 'Read-only receiver overload indicator. When on, the receiver front end is being overloaded; add attenuation or turn the preamp off.',
  REV: 'Reverse: swaps the receive and transmit frequencies, such as listening on a repeater input to check if you can hear the other station directly.',
  RF: 'RTTY filter. Applies a narrow filter suited to RTTY signals. Radio dependent.',
  RESUME: 'Scan auto-resume: scanning continues after stopping on a signal. Radio dependent.',
  RIT: 'Receiver incremental tuning. Shifts only the receive frequency by the RIT offset so you can adjust for a station slightly off frequency without moving your transmit frequency.',
  SATMODE: 'Satellite mode: lets the radio receive and transmit on different bands (or VFOs) at the same time for satellite work. Radio dependent.',
  SBKIN: 'Semi break-in for CW: the radio switches to transmit when you key and returns to receive after a short delay.',
  SCEN: 'Scrambler or encryption on or off. Radio dependent.',
  SCOPE: 'Simple bandscope (band display) on or off. Radio dependent.',
  SEND_MORSE: 'Sends the given text as CW using the radio built-in keyer.',
  SEND_VOICE_MEM: 'Transmits a voice message stored in one of the radio memories. Make sure you are in SSB or voice mode and are allowed to transmit.',
  SLICE: 'Selects a receive slice on slice-based (Flex) radios. Radio dependent.',
  SPECTRUM: 'Turns spectrum scope data output on or off for radios that stream scope data.',
  SPECTRUM_HOLD: 'Pauses spectrum scope updates so you can examine the current display.',
  SQL: 'Squelch monitor: temporarily opens the squelch so you can hear what is on the channel, even when no signal is strong enough to open it.',
  SYNC: 'Synchronizes the two VFOs so tuning one moves the other. Radio dependent (used by some Yaesu radios).',
  TBURST: '1750 Hz tone burst, used in parts of Europe to open some repeaters.',
  TONE: 'Transmits a CTCSS sub-audible tone with your signal. Needed to open repeaters that require a tone (set the tone frequency separately).',
  TRANSCEIVE: 'Radio sends frequency, mode and other changes to the computer automatically when you turn the dial, instead of waiting to be polled.',
  TSQL: 'CTCSS tone squelch on receive: the receiver stays muted until a signal with the matching tone is received. It silences other users on a shared channel.',
  TUNER: 'Puts the antenna tuner in line. This enables the tuner; it does not start a tune cycle (use the Tune button for that).',
  VOX: 'Voice-operated transmit: the radio transmits automatically when it hears your voice, so no push-to-talk is needed. Set gain, delay and anti-VOX first.',
  VSC: 'Voice scan control: scan pauses only on voice signals. Radio dependent.',
  XIT: 'Transmitter incremental tuning. Shifts only the transmit frequency by the XIT offset without changing the receive frequency.',
};

/* ------------------------------------------------------------------ */
/* Parameters (radio-wide settings, not tied to a VFO)                 */
/* ------------------------------------------------------------------ */
export const PARM_HELP: Record<string, string> = {
  AFIF: 'Selects what audio is sent over the USB connection: normal receive audio (AF) or the radio IF signal (for SDR software). Radio dependent (Icom IC-7300, IC-9700, IC-705).',
  AFIF_ACC: 'Selects AF audio or IF signal output on the ACC connector. Radio dependent (IC-9700).',
  AFIF_LAN: 'Selects AF audio or IF signal output over the network (LAN) connection. Radio dependent (IC-9700).',
  AFIF_WLAN: 'Selects AF audio or IF signal output over the wireless (WLAN) connection. Radio dependent (IC-705).',
  ANN: 'Controls the radio spoken announcements (frequency, mode, S-meter) for use without looking at the display. Radio dependent.',
  APO: 'Auto power off: switches the radio off after this many minutes of no activity.',
  BACKLIGHT: 'Brightness of the radio LCD backlight, from 0 (off or dimmest) to 1 (brightest).',
  BANDSELECT: 'Selects the operating band by name (for example BAND80M or BAND2M) on radios that support it over CAT.',
  BAT: 'Read-only battery level of a portable radio, from 0 (empty) to 1 (full).',
  BEEP: 'Turns the radio key-press beep on or off.',
  KEYERTYPE: 'Selects the built-in CW keyer type: straight key, bug, or paddle.',
  KEYLIGHT: 'Turns the radio button backlight on or off.',
  SCREENSAVER: 'Radio screensaver or display timeout setting. The available values depend on the radio.',
  TIME: 'The radio clock time, set as seconds since midnight.',
};

/* ------------------------------------------------------------------ */
/* Modes                                                               */
/* ------------------------------------------------------------------ */
export const MODE_HELP: Record<string, string> = {
  AM: 'Amplitude modulation: used for AM broadcast, aircraft and some HF nets. Takes more bandwidth than SSB.',
  AMN: 'Narrow AM: AM with a narrower receive filter, to cut interference at the cost of audio quality.',
  AMS: 'Synchronous AM: locks to the carrier to reduce fading distortion and interference from the other side of a signal.',
  C4FM: 'Yaesu C4FM digital voice (System Fusion) for VHF/UHF. Needs a compatible radio at both ends.',
  CW: 'Morse code using the radio normal CW sideband. The receive tone follows the CW pitch setting.',
  CWN: 'Narrow CW filter mode. Use it to cut interference when signals are close together.',
  CWR: 'Reverse CW: Morse using the opposite sideband to CW. Use it to move an interfering signal to the other side of yours without retuning.',
  DCR: 'Digital voice mode (DCR) offered on some wideband receivers. Radio dependent.',
  DD: 'High-speed digital data mode on some radios (IC-9700 DD mode). Radio dependent.',
  DPMR: 'Digital PMR voice mode on some wideband receivers. Radio dependent.',
  DRM: 'Digital Radio Mondiale, a digital broadcast mode. Appears only on radios or software that support it.',
  'D-STAR': 'D-STAR digital voice (VHF/UHF). Needs a D-STAR capable radio.',
  DSB: 'Double sideband suppressed carrier: both sidebands without the carrier. Radio dependent.',
  ECSSLSB: 'Exalted-carrier SSB, lower sideband: receives with a locally supplied carrier for stable reception of SSB or AM. Radio dependent.',
  ECSSUSB: 'Exalted-carrier SSB, upper sideband: receives with a locally supplied carrier for stable reception of SSB or AM. Radio dependent.',
  FAX: 'Facsimile mode for receiving weather and other fax pictures. Radio dependent.',
  FM: 'Frequency modulation, the standard mode for VHF/UHF voice and repeaters.',
  FMN: 'Narrow FM: FM with a narrower filter and deviation, for closely spaced channels. Radio dependent.',
  IQ: 'I/Q output mode on some kit and SDR radios. Radio dependent.',
  ISBLSB: 'Independent sideband monitoring, lower sideband. Radio dependent.',
  ISBUSB: 'Independent sideband monitoring, upper sideband. Radio dependent.',
  LSB: 'Lower sideband: by convention the voice mode on the bands below 10 MHz (160, 80 and 40 m).',
  LSBD1: 'Lower sideband with data audio path 1 on radios that offer several data modes. Radio dependent.',
  LSBD2: 'Lower sideband with data audio path 2 on radios that offer several data modes. Radio dependent.',
  LSBD3: 'Lower sideband with data audio path 3 on radios that offer several data modes. Radio dependent.',
  'NXDN-N': 'NXDN narrow-band digital voice on some wideband receivers. Radio dependent.',
  'NXDN-VN': 'NXDN very-narrow digital voice on some wideband receivers. Radio dependent.',
  P25: 'APCO P25 digital voice, mainly for receiving public-safety systems on some wideband receivers.',
  PKTAM: 'Packet/data AM: AM through the radio data audio path (for example the USB audio or data input) instead of the microphone.',
  PKTFM: 'Packet/data FM: FM through the radio data audio path, used for packet radio, APRS and other VHF/UHF digital modes.',
  PKTFMN: 'Packet/data narrow FM: narrow FM using the data audio path, as used on some Yaesu radios.',
  PKTLSB: 'Lower sideband with the data audio path (USB audio or data port) instead of the microphone, for digital modes on bands that use LSB.',
  PKTUSB: 'Upper sideband with the data audio path (USB audio or data port) instead of the microphone. Use for FT8, Winlink and other digital modes.',
  PSK: 'PSK data mode on some radios (for example Kenwood). Radio dependent.',
  PSKR: 'Reverse-sideband PSK data mode on some radios (for example Kenwood). Radio dependent.',
  RTTY: 'Radio teletype, using FSK tones. Used for RTTY contesting and data. Radio dependent in how the tones are produced.',
  RTTYR: 'Reverse RTTY: RTTY decoded with the mark and space tones swapped. Use it if RTTY text decodes as garbage.',
  SAH: 'Synchronous AM, upper (higher) sideband only. Locks onto the carrier to reduce fading and interference from the other side.',
  SAL: 'Synchronous AM, lower sideband only. Locks onto the carrier to reduce fading and interference from the other side.',
  SAM: 'Synchronous AM, both sidebands. Locks onto the carrier to reduce fading distortion on AM signals.',
  SPEC: 'Unfiltered spectrum mode as in some SDR software. Radio dependent.',
  USB: 'Upper sideband: by convention the voice mode on bands above 10 MHz (20 m and up); also the digital-mode sideband on all bands.',
  USBD1: 'Upper sideband with data audio path 1 on radios that offer several data modes. Radio dependent.',
  USBD2: 'Upper sideband with data audio path 2 on radios that offer several data modes. Radio dependent.',
  USBD3: 'Upper sideband with data audio path 3 on radios that offer several data modes. Radio dependent.',
  WFM: 'Wide FM, as used by FM broadcast stations. For receiving only.',
  WFMS: 'Wide FM stereo for broadcast FM reception. Radio dependent.',
};

/* ------------------------------------------------------------------ */
/* VFO operations                                                      */
/* ------------------------------------------------------------------ */
export const VFO_OP_HELP: Record<string, string> = {
  BAND_DOWN: 'Jumps to the next lower band.',
  BAND_UP: 'Jumps to the next higher band.',
  CPY: 'Copies VFO A to VFO B so both are on the same frequency and mode. Useful before setting up split.',
  DOWN: 'Lowers the VFO frequency by one tuning step.',
  FROM_VFO: 'Stores the current VFO frequency and mode into a memory channel.',
  LEFT: 'Moves left, such as to the previous item. Exact action depends on the radio.',
  MCL: 'Clears a memory channel.',
  RIGHT: 'Moves right, such as to the next item. Exact action depends on the radio.',
  TOGGLE: 'Switches the active VFO between A and B.',
  TO_VFO: 'Recalls a memory channel into the current VFO.',
  TUNE: 'Starts an antenna tuner cycle. The radio transmits a carrier while the tuner matches the antenna.',
  UP: 'Raises the VFO frequency by one tuning step.',
  XCHG: 'Swaps the frequencies and modes of VFO A and VFO B.',
};

/* ------------------------------------------------------------------ */
/* UI controls                                                         */
/* ------------------------------------------------------------------ */
export const UI_HELP: Record<string, string> = {
  agc: 'Receiver gain recovery speed. Slow is best for SSB listening; fast suits CW and weak signals with impulse noise. Medium is a general compromise.',
  attenuator: 'Reduces all incoming signal strength by a fixed number of dB. Use it when strong nearby stations overload the receiver or the S-meter pegs.',
  bands: 'Jump to a ham band. The radio goes to a frequency in that band, usually where it last was.',
  'fm.offset': 'Repeater offset: the gap between receive and transmit frequency. Typically 600 kHz on 2 m and 5 MHz on 70 cm, but check local band plans.',
  'fm.shift': 'Repeater shift direction: plus transmits above the receive frequency, minus below it, and simplex uses the same frequency for both.',
  'fm.sql': 'Tone squelch (CTCSS): the receiver stays muted until a signal with the matching tone is received. Useful to ignore other users on the channel.',
  'fm.tone': 'CTCSS encode tone: a sub-audible tone sent with your transmission. Many repeaters need the correct tone to open; typical values are 67 to 254.1 Hz.',
  freq: 'Current frequency. Click and use the mouse wheel or arrow keys to tune, or type a frequency and press Enter. Press F to type a new frequency.',
  mode: 'Modulation mode. Common choices are LSB or USB for voice on HF, CW for Morse, FM for VHF/UHF, and PKTUSB for digital modes.',
  passband: 'Receive filter width in Hz. Narrower cuts interference and noise but too narrow loses part of the signal; typical widths are about 2.4 kHz for SSB and a few hundred Hz for CW.',
  ptt: 'Push to talk: starts transmitting while on and returns to receive when off. Make sure the right antenna and mode are selected before you transmit.',
  readonly: 'Read-only mode: you can view the radio but not change settings or transmit.',
  rit: 'Receiver incremental tuning: shifts only the receive frequency so you can fine-tune a station slightly off frequency without moving your transmit frequency.',
  smeter: 'Received signal strength. S9 is -73 dBm on HF, each S unit is 6 dB, and readings above S9 are shown as dB over S9.',
  split: 'Receive on one VFO and transmit on another. Typical in DX pile-ups where the DX station listens on a different frequency from where it transmits.',
  step: 'Tuning step: how far the frequency moves for each wheel click or arrow press, such as 10 Hz for fine tuning or 1 kHz for browsing.',
  tune: 'Starts a tuner cycle: the radio transmits a carrier while the tuner matches the antenna. Judge the result by the final SWR and stop if it stays high.',
  tuner: 'Antenna tuner. In-line puts the tuner in the signal path; a tune cycle (Tune button) transmits a carrier while it finds a match.',
  vfo: 'VFO A and B are two independent frequency and mode settings. Swap them to jump between two stations, or copy A to B to start from the same frequency.',
  views: 'Interface views are different layouts of the controls, such as a compact view for operating and fuller views with all the radio settings. Pick the one that suits what you are doing.',
  xit: 'Transmitter incremental tuning: shifts only your transmit frequency by the XIT offset without changing the receive frequency.',
  preamp: 'Receive preamplifier, adding gain for weak signals. Helpful on quiet upper HF bands and VHF/UHF; leave it off on noisy lower bands where it just raises the noise.',
  'meter.alc': 'ALC level while transmitting. Keep it at or just below the ALC limit; over-driving the radio with too much audio causes distortion and splatter.',
  'meter.comp': 'How much speech compression is being applied while transmitting, in dB. A few dB is typical; too much makes audio sound distorted.',
  'meter.power': 'Transmit output power. Shows the power your radio is putting out while transmitting.',
  'meter.swr': 'Standing wave ratio while transmitting. Below about 1.5 is excellent and 2.0 acceptable; above 3 reduce power and check the antenna and feedline.',
  'meter.temp': 'Temperature of the radio final amplifier or heat sink. Watch it during long transmissions and let the radio cool if it runs hot.',
};

/* ------------------------------------------------------------------ */
/* Lookup                                                              */
/* ------------------------------------------------------------------ */
const MAPS: Record<HelpKind, Record<string, string>> = {
  level: LEVEL_HELP,
  func: FUNC_HELP,
  parm: PARM_HELP,
  mode: MODE_HELP,
  vfoop: VFO_OP_HELP,
  ui: UI_HELP,
};

export function helpFor(kind: HelpKind, name: string): string | undefined {
  const map = MAPS[kind];
  if (!map) return undefined;
  return Object.prototype.hasOwnProperty.call(map, name) ? map[name] : undefined;
}
