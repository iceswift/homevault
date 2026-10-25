# TUI preview provenance

`tui-welcome.png` is a rendering of the actual initial HomeVault terminal output
in [tui-welcome.txt](tui-welcome.txt), captured on Linux on 25 October 2026.

The process ran in a 100-column, 34-row pseudo-terminal with a newly generated
`demo-setup` configuration containing only the `synthetic` project. The helper
waited for the initial view, sent `q`, stripped terminal control sequences and
retained the text from the HomeVault heading through the footer. No backup,
repository initialization, cloud access, notification or production service
operation was performed. The temporary sample configuration/key was removed.

The PNG renders those exact text lines in a dark terminal frame using Consolas;
the frame caption is an image label, not an extra HomeVault UI feature. No text was
added to the captured view and no completed job or successful backup was invented.
It is not a desktop-window screenshot or evidence of a recovery test.

When the TUI changes, recapture the view with generated data, review it for private
identifiers, and update the plain text and image together. The caption and alt text
must continue to distinguish synthetic preview content from operational evidence.
