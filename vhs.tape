# VHS tape for aitop's README demo GIF.
# Install VHS: https://github.com/charmbracelet/vhs
# Record:      go build -o aitop ./cmd/aitop/ && vhs vhs.tape
# Output:      demo.gif (referenced from README.md)

Output demo.gif

Set Shell "zsh"
Set FontSize 14
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set Width 1400
Set Height 1000
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms

Hide
Type "clear"
Enter
Sleep 500ms
Show

Type "./aitop --demo"
Sleep 500ms
Enter
Sleep 3500ms

# Grid: opens with just 3 sessions, then the board fills up on its own
# while we sit in grid — cursor-agent joins first, then a second Claude
# Code session and Cursor join later. One long hold instead of several
# quick toggles, so each stage is actually readable.
Type "v"
Sleep 9500ms

# Pause/resume the refresh loop.
Type " "
Sleep 2000ms
Type " "
Sleep 2000ms

Type "q"
Sleep 1000ms
