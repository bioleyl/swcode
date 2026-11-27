# swcode 0.17.1

Released 2026-11-27.

Pairing now tells you when it cannot work. Where swcode listens on its computer alone, Settings → Computers says so, with a button to the network settings, instead of showing a QR code and a number field that lead nowhere. A phone or another computer asks the address for an answer before it shows a number, and says what to check when nothing answers.

The QR code no longer stays on localhost when swcode started before the network was up, as it does at logon: it shows the computer's address on the network once there is one.

The rest finishes the review begun in 0.17.0. Stop reaches a command on an SSH host even after ssh's warnings. The daemon's data files are written whole or not at all, so a full disk no longer empties one. A message is put back in the composer when its session is refused. An install from the `.deb` says to install the new package instead of failing its update.
