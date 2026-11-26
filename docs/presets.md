---
title: Radio presets
description: Browse settings other operators shared on openRMS Rigs, check them against your radio, apply them safely and restore your old values.
order: 4
---

# Radio presets

Setting a radio up for FT8, Winlink or SSB over USB means a dozen menu items, and the right values
are scattered across manuals, forum posts and videos. [openRMS Rigs](https://rigs.openrms.dev)
collects them: each **preset** is one radio model and one operation, with every setting, its
value, why, where it came from and how sure the author is. Presets are crowd-sourced and shared
under CC-BY-SA-4.0.

The **Presets** page in rigctlWeb shows them like a manual, compares them with your radio, and
applies the settings it can set safely.

## Get the library

rigctlWeb doesn't contact openRMS Rigs on its own. Press **Update library** to download the
published presets. They're kept on your computer and work offline after that. Press it again
whenever you want newer presets.

The list shows presets for the selected radio's model; switch it to all radios to read the
others. You can filter by operation. You can read any preset, but only apply one made for your
radio.

## Read a preset

Open a preset to see each setting with its value, the reason, its sources and a confidence level,
plus the menu path on the radio, notes for your software (WSJT-X, Pat and so on), and the author
and licence. Settings fall into groups:

- **Settings rigctlWeb can set**: through Hamlib, or through a raw CAT command for a radio menu
  item (see below).
- **Set these on the radio**: menu items rigctlWeb can't change on your radio. Each says why, and
  gives the menu path.
- **Checked only**: values rigctlWeb compares with the radio but never sets.

## Check against your radio

**Check against my radio** reads each setting from the radio and marks it as matching, different
or unknown. Checking changes nothing on the radio and sends nothing anywhere: the comparison runs
on your computer.

## Apply

Tick the settings you want and press **Apply**. Settings that change how the radio transmits
(power, mic gain, compression, the data audio source and so on) are marked **transmit** and start
unticked: tick them yourself after checking them against your station. Settings that already match
are unticked after a check.

Before it changes anything, rigctlWeb reads and saves the current value of each setting, so it can
put it back. It applies the settings in the preset's order and stops at the first one that fails.
Nothing is applied while the radio is transmitting, and nothing at all on a read-only radio.

## Restore

**Restore previous settings** puts back the values from before the first preset you applied, newest
change first. The saved values stay on disk until you restore, so Restore works after a restart
too.

## Raw CAT menu settings

Many data-mode settings, such as the USB audio source or the rear input gain, are radio menu items
Hamlib can't reach. Presets can give them as raw CAT commands, and rigctlWeb sends them only on a
narrow path:

- **Only commands on the openRMS allow-list.** Every command is checked against the shared
  [allow-list](https://github.com/open-rms/openRMS-Spec/blob/main/rigs/cat-allowlist.json), which
  is built into rigctlWeb. Commands that transmit, switch to transmit, toggle VOX or break-in,
  start a tuner, send CW or voice messages, write memories, change the frequency or turn the radio
  off are always refused. A preset with a command off the list is refused as a whole.
- **Kenwood, Yaesu and Elecraft only.** Icom CI-V commands are checked but not sent yet.
- **The read can't be a write.** A setting is read only when its read command is exactly its set
  command without the value, and the radio's reply must be exactly the read command followed by the
  value. A step that doesn't follow these rules stays a "set it on the radio" item, and nothing is
  read.
- **Read, set, read back.** The current value is read first (for Restore), then the new value is
  set, then read back. If the radio doesn't take it, the step fails.
- **You confirm the exact commands.** The Apply dialog lists, for each setting, the read, the set
  and the read-back, exactly as they'll be sent, and Apply stays off until you tick that you've
  checked them against your radio's manual. The server refuses if the commands differ from what you
  confirmed.
- **Never while transmitting**, and never on a read-only radio.
- **Everything is logged.** Every raw command, reads included, is appended to
  `presets/cat-log.jsonl` in rigctlWeb's state folder, with the time, radio, preset and result.

Raw CAT can't be sent from the public API or any other page. Only Apply and Restore on the Presets
page use it.

## What presets never do

Presets never key the transmitter. A preset that names PTT, the tuner, VOX, break-in or any function
outside rigctlWeb's list of safe functions is refused as a whole. Hamlib settings are applied only
through rigctlWeb's own checked commands, using Hamlib's standard names.

## Contribute a preset

Presets are written and reviewed on [openRMS Rigs](https://rigs.openrms.dev). If a preset worked
(or didn't) on your radio, say so there: that feedback is what makes them trustworthy.
