// Integration notes: how other ham radio programs are pointed at a running
// Hamlib rigctld so they can share one radio with rigctlWeb.
//
// Placeholders used in steps and snippets:
//   {HOST}  rigctld host name or address
//   {PORT}  rigctld TCP port (Hamlib default 4532)
//   {RIGID} a rig reference name (used by Pat for its hamlib_rigs key)
//
// Hamlib model 2 is "NET rigctl" (checked with `rigctl -l`, Hamlib 4.7.2).
//
// ptt values:
//   'cat'      the program has its own PTT Method setting (CAT) that is carried
//              to the radio through the rigctld connection
//   'rigctld'  the program has a dedicated rigctld/Hamlib PTT setting
//   'separate' the program keys the radio on its own (serial RTS, VOX, CM108,
//              or through another program), not through rigctld
//   'unknown'  not confirmed

export type IntegrationCategory = 'digital' | 'packet' | 'winlink' | 'logging' | 'modem' | 'other';

export type Integration = {
	id: string;
	name: string;
	category: IntegrationCategory;
	summary: string;
	steps: string[];
	snippet?: string;
	snippetFile?: string;
	notes: string[];
	sources: string[];
	confidence: 'verified' | 'partial' | 'unverified';
	ptt: 'cat' | 'rigctld' | 'separate' | 'unknown';
};

const WSJTX_FAMILY_NOTES = [
	'PTT Method: CAT is normally right with a network rig control server, provided rigctld can key the radio. Use DTR or RTS only if that serial line is not owned by rigctld.',
	'Split Operation: Rig or Fake It. Fake It is the safer choice if split is unreliable through rigctld.',
	'Poll Interval: 1 to 3 seconds is typical. Keep it modest because every client polls the same radio.',
	'Leave the program on a Network Server entry only. Do not also point it at the radio serial port while rigctld owns it.',
	'Use the Test CAT and Test PTT buttons after saving. A red Test CAT usually means rigctld is not reachable.',
];

export const INTEGRATIONS: Integration[] = [
	{
		id: 'pat',
		name: 'Pat (Winlink)',
		category: 'winlink',
		summary:
			'Pat reads frequency and optionally keys PTT through a named rig defined under hamlib_rigs in config.json.',
		steps: [
			'Run rigctld for your radio and note its host and port.',
			'Edit Pat config.json (default ~/.wl2k/config.json, or run pat help to find it) or use the web GUI configuration page.',
			'Under "hamlib_rigs", add a rig named {RIGID} with "network": "tcp" and "address": "{HOST}:{PORT}".',
			'In each transport section you use ("ardop", "varahf", "varafm", "ax25", "pactor"), set "rig": "{RIGID}".',
			'Set "ptt_ctrl": true in that section only if Pat should key the radio through rigctld. Leave it false if the modem keys the radio itself.',
			'Start rigctld first, then restart Pat. Verify with: pat interactive, then freq ardop.',
		],
		snippet: `{
  "hamlib_rigs": {
    "{RIGID}": { "network": "tcp", "address": "{HOST}:{PORT}" }
  },
  "ardop": {
    "addr": "localhost:8515",
    "rig": "{RIGID}",
    "ptt_ctrl": true
  }
}`,
		snippetFile: 'config.json',
		notes: [
			'ptt_ctrl is per transport. SignaLink-style interfaces with their own VOX need it false; most rig-keyed interfaces need it true.',
			'Pat queries the frequency of every rig in hamlib_rigs at startup and reports an error if rigctld is down, even if no transport uses that rig.',
			'Pat reads the dial frequency when connecting. It is not a continuous poller, but rigctlWeb and Pat both talk to the same radio, so avoid changing frequency while a connection is in progress.',
			'The optional "VFO" field ("A" or "B") selects which VFO Pat controls; empty means the active VFO.',
			'The default Pat build talks only to rigctld over TCP. Direct serial Hamlib needs a build with -tags libhamlib.',
			'If ptt_ctrl is true but rig is empty or undefined, Pat fails with "unable to set PTT rig".',
		],
		sources: [
			'https://github.com/la5nta/pat/wiki/Rig-control',
			'https://github.com/la5nta/pat/blob/master/cfg/config.go',
			'https://github.com/la5nta/pat/blob/master/app/connect.go',
			'https://github.com/pflarue/ardop/blob/master/docs/Pat_windows.md',
		],
		confidence: 'verified',
		ptt: 'rigctld',
	},
	{
		id: 'wsjtx',
		name: 'WSJT-X',
		category: 'digital',
		summary:
			'WSJT-X acts as a client of rigctld when the Rig is set to Hamlib NET rigctl and a Network Server address is given.',
		steps: [
			'Run rigctld for your radio.',
			'Open File | Settings | Radio.',
			'In the Rig drop-down choose "Hamlib NET rigctl". The field under CAT Control is relabeled "Network Server".',
			'Enter {HOST}:{PORT} in Network Server (formats: hostname:port, IPv4-address:port, [IPv6-address]:port).',
			'Set Poll Interval, PTT Method (CAT) and Split Operation as needed.',
			'Click Test CAT. The button turns green when communication works.',
		],
		notes: [
			...WSJTX_FAMILY_NOTES,
			'The manual says a blank Network Server field means the default instance on the same computer. Enter the address explicitly to avoid ambiguity.',
			'The WSJT-X FAQ states that sharing one radio reliably needs a rig control server such as rigctld, and that dumb serial port splitters are not supported.',
			'WSJT-X also ships rigctld-wsjtx, a rigctld built with the same Hamlib drivers as WSJT-X itself.',
		],
		sources: [
			'https://wsjt.sourceforge.io/wsjtx-doc/wsjtx-main-2.7.0.html',
		],
		confidence: 'verified',
		ptt: 'cat',
	},
	{
		id: 'wsjtx-improved',
		name: 'WSJT-X Improved (WS)',
		category: 'digital',
		summary:
			'This fork of WSJT-X keeps the same Radio settings tab, so the WSJT-X NET rigctl procedure should apply.',
		steps: [
			'Run rigctld for your radio.',
			'Open File | Settings | Radio.',
			'In the Rig drop-down choose "Hamlib NET rigctl".',
			'Enter {HOST}:{PORT} in the Network Server field.',
			'Set PTT Method to CAT, then click Test CAT.',
		],
		notes: [
			'Not confirmed from fork-specific documentation. The project page (now named WS - Digital Mode Suite) does not describe radio setup, so these steps are taken from WSJT-X, from which it derives.',
			...WSJTX_FAMILY_NOTES,
			'The fork bundles its own Hamlib and can update it from the program on Windows. A Hamlib version mismatch between the program and rigctld is usually harmless for the network protocol but is worth checking if frequency readback looks wrong.',
		],
		sources: [
			'https://sourceforge.net/projects/wsjt-x-improved/',
			'https://wsjt.sourceforge.io/wsjtx-doc/wsjtx-main-2.7.0.html',
		],
		confidence: 'partial',
		ptt: 'cat',
	},
	{
		id: 'jtdx',
		name: 'JTDX',
		category: 'digital',
		summary:
			'JTDX uses the same Radio tab as WSJT-X: pick Hamlib NET rigctl and enter the rigctld address as Network Server.',
		steps: [
			'Run rigctld for your radio.',
			'Open File | Settings | Radio.',
			'In the Rig drop-down choose "Hamlib NET rigctl". The CAT Control field is relabeled "Network Server".',
			'Enter {HOST}:{PORT} in Network Server.',
			'Set Poll Interval, PTT Method (CAT) and Split Operation, then click Test CAT.',
		],
		notes: [
			...WSJTX_FAMILY_NOTES,
			'JTDX ships a heavily modified Hamlib build (rigctl-jtdx, rigctld-jtdx). A standard system rigctld is still the server to run when sharing with other programs.',
			'The JTDX user guide section is a copy of the WSJT-X text, and the JTDX source passes the Network Server value to Hamlib as rig_pathname for network-type rigs.',
		],
		sources: [
			'https://sources.debian.org/src/jtdx/2.2.159%2Bimproved-3/doc/user_guide/en/settings-radio.adoc',
			'https://github.com/jtdx-project/jtdx/blob/master/Configuration.cpp',
			'https://github.com/jtdx-project/jtdx/blob/master/HamlibTransceiver.cpp',
		],
		confidence: 'verified',
		ptt: 'cat',
	},
	{
		id: 'js8call',
		name: 'JS8Call',
		category: 'digital',
		summary:
			'JS8Call is a WSJT-X derivative with the same Radio settings, so it can use Hamlib NET rigctl with a Network Server address.',
		steps: [
			'Run rigctld for your radio.',
			'Open File | Settings, then the Radio tab.',
			'Choose the Hamlib NET rigctl entry in the Rig list.',
			'Enter {HOST}:{PORT} in the Network Server field.',
			'Set PTT Method to CAT and click Test CAT.',
		],
		notes: [
			'No official JS8Call page describing NET rigctl was found. The JS8Call source has the same "Network Server:" label and passes the value to Hamlib as rig_pathname, and its README says it is a WSJT-X derivative, so the procedure is inferred.',
			'Menu wording may differ slightly between JS8Call versions. Check the Radio tab of your installed version.',
			...WSJTX_FAMILY_NOTES,
		],
		sources: [
			'https://github.com/js8call/js8call/blob/master/README.md',
			'https://github.com/js8call/js8call/blob/master/Configuration.cpp',
			'https://github.com/js8call/js8call/blob/master/Transceiver/HamlibTransceiver.cpp',
			'https://wiki.oarc.uk/js8',
		],
		confidence: 'partial',
		ptt: 'cat',
	},
	{
		id: 'fldigi',
		name: 'fldigi',
		category: 'digital',
		summary:
			'fldigi has built-in Hamlib support; selecting the NET rigctl rig and giving host:port as the device makes it a rigctld client.',
		steps: [
			'Run rigctld for your radio.',
			'Open Configure | Rig Control | Hamlib (menu names may vary by version).',
			'Tick Use Hamlib.',
			'Choose the rig entry "Hamlib NET rigctl" (model 2).',
			'Enter {HOST}:{PORT} as the device.',
			'Apply the initialization (Initialize/Open button) and check that the frequency display follows the radio.',
			'Set PTT to use Hamlib if fldigi should key through rigctld; leave other rig-control methods (flrig, rigcat) unticked.',
		],
		notes: [
			'Confirmed from the fldigi source: the model number selected in the list is passed to Hamlib, and the device string is set as rig_pathname, which for NET rigctl is the host:port of rigctld. The exact menu labels above were not confirmed from current official docs (the official help site was unreachable) and may differ.',
			'Only one rig-control method should be active at a time in fldigi (Hamlib, flrig, or rigcat).',
			'Hamlib timing fields in fldigi (write delay, post write delay, timeout, retries) are passed to Hamlib. The defaults are usually fine for a network backend.',
			'fldigi keeps polling the radio while open. Keep the poll interval modest when sharing.',
			'Sound card PTT or a separate PTT line may be used instead of rigctld PTT; make sure only one program drives PTT.',
		],
		sources: [
			'https://github.com/w1hkj/fldigi/blob/master/src/rigcontrol/hamlib.cxx',
		],
		confidence: 'partial',
		ptt: 'rigctld',
	},
	{
		id: 'direwolf',
		name: 'Dire Wolf',
		category: 'packet',
		summary:
			'Dire Wolf can key PTT through rigctld with the PTT RIG 2 host:port line in direwolf.conf.',
		steps: [
			'Run rigctld for your radio.',
			'Edit direwolf.conf (or the file passed with -c).',
			'Under the CHANNEL section, add: PTT RIG 2 {HOST}:{PORT}',
			'Model 2 means NET rigctl. The port is optional and defaults to 4532.',
			'Restart Dire Wolf. At startup its banner lists the optional features built in; it must include hamlib.',
		],
		snippet: `ADEVICE plughw:1,0
CHANNEL 0
MYCALL N0CALL-1
MODEM 1200
PTT RIG 2 {HOST}:{PORT}`,
		snippetFile: 'direwolf.conf',
		notes: [
			'Hamlib PTT is optional and the guide describes it as Linux only and possibly problematic. Many binary packages and Windows builds lack it; check the startup banner for "hamlib".',
			'Dire Wolf uses rigctld for PTT only. It does not set or read frequency.',
			'Other PTT methods in direwolf.conf: PTT <device> [-]RTS|DTR [ [-]RTS|DTR ] (serial control line, COMx on Windows), PTT GPIO / PTT GPIOD (Linux GPIO), PTT LPT (parallel port, Linux), and CM108/CM119 GPIO on USB audio adapters. VOX needs no PTT line.',
			'PTT RIG with a rig number other than 2 and a serial port makes Dire Wolf open the serial port itself. Do not use that if rigctld owns the port; use model 2.',
			'The ADEVICE, MYCALL and MODEM lines in the snippet are placeholders; keep your own values.',
		],
		sources: [
			'https://github.com/wb2osz/direwolf/blob/master/doc/User-Guide.pdf',
		],
		confidence: 'verified',
		ptt: 'rigctld',
	},
	{
		id: 'xastir',
		name: 'Xastir',
		category: 'packet',
		summary:
			'Xastir has no rigctld or Hamlib support; it talks to TNCs and APRS-IS, so radio keying happens in the TNC or in Dire Wolf.',
		steps: [
			'Xastir cannot be pointed at rigctld.',
			'To use a radio with rigctld, run Dire Wolf with PTT RIG 2 {HOST}:{PORT} (see the Dire Wolf entry).',
			'In Xastir, add an AGWPE network interface (Interface menu, Interface Control) and enter the host and port of the Dire Wolf AGW server. See the Dire Wolf guide for its AGW port setting.',
		],
		notes: [
			'Verified negative: a search of the Xastir source tree found no reference to hamlib or rigctl. Its interface types include serial KISS TNC, multi-port KISS, and network AGWPE.',
			'Menu wording was not checked against a running copy; treat the menu path as approximate. The AGWPE interface type exists in the source (DEVICE_NET_AGWPE).',
			'rigctlWeb and Xastir therefore only meet indirectly, through the program that keys the radio.',
		],
		sources: [
			'https://github.com/Xastir/Xastir',
			'https://github.com/Xastir/Xastir/blob/master/src/interface.h',
		],
		confidence: 'partial',
		ptt: 'separate',
	},
	{
		id: 'ardopcf',
		name: 'ardopcf (ARDOP)',
		category: 'modem',
		summary:
			'ardopcf does not use rigctld; to share a radio through rigctld let Pat do CAT and PTT, or let ardopcf key PTT itself on a different serial port.',
		steps: [
			'Option A (rigctld through Pat): do not pass -p, --ptt, -c, --cat, -g, -k or -u to ardopcf.',
			'In Pat config.json, define the rig under "hamlib_rigs" with "address": "{HOST}:{PORT}", then in the "ardop" section set "rig": "{RIGID}" and "ptt_ctrl": true.',
			'Option B (ardopcf keys PTT): start ardopcf with -p <serial-port> (RTS) or -c <serial-port>[:baud] with -k and -u hex CAT strings, and set "ptt_ctrl": false in Pat.',
		],
		snippet: `"ardop": {
  "addr": "localhost:8515",
  "rig": "{RIGID}",
  "ptt_ctrl": true
}`,
		snippetFile: 'config.json',
		notes: [
			'ardopcf provides no dial frequency control; frequency comes from Pat via rigctld, or you tune the radio by hand.',
			'Do not let ardopcf do PTT on the same USB or serial device that rigctld or any other program uses for CAT.',
			'The ardopcf docs say a partial attempt at rigctld PTT exists in the original ardopc code but is not currently usable.',
			'Pat talks to the ardopcf host interface on port 8515 by default; the WebGUI is on 8514 and is a different port.',
		],
		sources: [
			'https://github.com/pflarue/ardop/blob/master/docs/USAGE_linux.md',
			'https://github.com/pflarue/ardop/blob/master/docs/Commandline_options.md',
			'https://github.com/pflarue/ardop/blob/master/docs/Pat_windows.md',
			'https://github.com/pflarue/ardop/blob/master/README.md',
		],
		confidence: 'verified',
		ptt: 'separate',
	},
	{
		id: 'mercury',
		name: 'Mercury modem (Rhizomatica)',
		category: 'modem',
		summary:
			'Mercury can key PTT through Hamlib; with model 2 and a host:port device it talks to rigctld.',
		steps: [
			'Run rigctld for your radio.',
			'Start mercury with: -R 2 -A {HOST}:{PORT} (-R sets the Hamlib model and selects the Hamlib PTT method, -A sets the ip:port endpoint).',
			'Or set the same in mercury.ini under [ptt]: method = hamlib, hamlib_model = 2, device = {HOST}:{PORT}.',
			'Test with -Q, which keys PTT for one second and exits without starting the modem or audio.',
		],
		snippet: `[ptt]
method = hamlib
hamlib_model = 2
device = {HOST}:{PORT}`,
		snippetFile: 'mercury.ini',
		notes: [
			'The man page gives localhost:4532 as an example Hamlib ip:port endpoint, and the source special-cases the NET rigctl model. That model 2 is the value to use follows from Hamlib itself.',
			'The ini key name hamlib_model is taken from the man page configuration table (ptt:hamlib_model). Check mercury.ini.example for your version.',
			'The Hamlib method and the HERMES shared-memory method (-S) are mutually exclusive.',
			'Mercury also polls frequency through Hamlib in some builds; the source calls rig_get_freq. Whether this is user-visible was not confirmed.',
			'Other PTT methods: serial (RTS/DTR, e.g. DigiRig), cm108 GPIO, hermes_shm, none.',
		],
		sources: [
			'https://github.com/Rhizomatica/mercury/blob/master/mercury.1',
			'https://github.com/Rhizomatica/mercury/blob/master/mercury.ini.example',
			'https://github.com/Rhizomatica/mercury/blob/master/radio_io/radio_io.c',
		],
		confidence: 'verified',
		ptt: 'rigctld',
	},
	{
		id: 'vara',
		name: 'VARA HF / VARA FM',
		category: 'modem',
		summary:
			'VARA is a closed Windows modem with no known rigctld client; to use rigctld, let Pat do CAT and PTT for it.',
		steps: [
			'Run VARA (usually under Wine on Linux) and note its command port (default 8300).',
			'Run rigctld for your radio and add it to Pat config.json under "hamlib_rigs" with "address": "{HOST}:{PORT}".',
			'In the "varahf" or "varafm" section set "rig": "{RIGID}" and "ptt_ctrl": true so Pat keys the radio through rigctld.',
			'In VARA, set PTT so it does not also try to key the radio (or leave it unused).',
		],
		snippet: `"varahf": {
  "addr": "localhost:8300",
  "bandwidth": 2300,
  "rig": "{RIGID}",
  "ptt_ctrl": true
}`,
		snippetFile: 'config.json',
		notes: [
			'Verified from the Pat source and wiki: Pat supports rig and ptt_ctrl for VARA sections and does the keying through its Hamlib rig. VARA settings screens and manual were not read (the manuals are hosted on file-sharing links), so the VARA-side menu path is not documented here.',
			'Forum reports say VARA under Wine often cannot drive some interface PTT lines (for example DRA-style or Digirig Lite); keying through Pat and rigctld worked for those users. These are anecdotal.',
			'Running VARA inside Wine does not affect rigctld: Pat runs natively and talks to rigctld, VARA only needs the audio and its command port.',
			'Do not let VARA and rigctld both open the same serial port.',
		],
		sources: [
			'https://github.com/la5nta/pat/wiki/VARA',
			'https://github.com/la5nta/pat/blob/master/cfg/config.go',
			'https://github.com/la5nta/pat/blob/master/app/connect.go',
			'https://rosmodem.wordpress.com/',
		],
		confidence: 'partial',
		ptt: 'separate',
	},
];

function substitute(text: string, vars: {host: string; port: number; rigId?: string}): string {
	const rigId = vars.rigId && vars.rigId.length > 0 ? vars.rigId : 'my_rig';
	return text
		.split('{HOST}')
		.join(vars.host)
		.split('{PORT}')
		.join(String(vars.port))
		.split('{RIGID}')
		.join(rigId);
}

/**
 * Fill {HOST}, {PORT} and {RIGID} in an integration's steps and snippet.
 * rigId defaults to "my_rig" when not given.
 */
export function render(
	i: Integration,
	vars: {host: string; port: number; rigId?: string},
): {steps: string[]; snippet?: string} {
	return {
		steps: i.steps.map((s) => substitute(s, vars)),
		snippet: i.snippet === undefined ? undefined : substitute(i.snippet, vars),
	};
}
