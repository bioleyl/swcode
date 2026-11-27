# swcode 0.17.0

Released 2026-11-27.

The session's header now says where its folder stands in git: the branch, how many files are uncommitted, and how many commits it is ahead of and behind the branch it follows. Outside a repository it says "no git". It is read again after each turn and after a commit.

The Android app asks for the microphone before it dictates: Android never showed its question, so dictation failed. It also stops reopening the "install unknown apps" screen at every return, and a tapped notification opens its own session.

This release follows a review of the whole project, and most of it is fixes. Restoring a turn no longer deletes files git ignored when the turn started, such as a `.env`, after the turn changed `.gitignore`. A sandboxed command can no longer swap the repository's `.git` folder for one of its own. A program you allowed no longer covers shell loops and conditions or commands hidden behind unusual quotes. Pairing numbers can no longer be guessed over many connections. A workflow no longer starts a step twice when the step before it used subagents. The Windows service starts when the profile folder's name has a space.

The Android signing keystore was tracked in the repository by mistake and no longer is.
