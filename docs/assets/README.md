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

## Banner

`homevault-banner.svg` is a repository-native vector wordmark/header. It contains
only static shapes and text, with no scripts, external assets or embedded fonts.
It is presentation artwork, separate from the captured TUI and verification evidence.

## Design prototype captures

`tui-prototype-wide.png` and `tui-prototype-compact.png` render the actual initial
`tui-demo --project notes` PTY frames at 120x32 and 60x26. The corresponding
[tui-prototype-wide.txt](tui-prototype-wide.txt) and
[tui-prototype-compact.txt](tui-prototype-compact.txt) preserve the uncolored text with invisible trailing padding omitted.
The renderer reproduces emitted ANSI foreground colors in a dark frame using
Consolas; the frame caption is a label outside the captured application view.

Captured on Linux on 25 October 2026. The sample clock, project names, result and
recovery states are deliberately invented fixtures. Persistent DESIGN PREVIEW /
DEMO ONLY labels distinguish them from operational evidence. No application
configuration, backup tools, cloud credentials or production data were used.
The isolated process was started and quit; no services or timers were installed.
These images show the prototype, not the existing live `tui` command.
