# VHS tape for a short, punchy "social / PR" clip of aitop.
# Tighter and smaller than the README's demo.gif — made to loop and to embed
# in a PR, a tweet, or a Show HN.
#
# Prereqs (local machine — needs a real TTY; won't run headless in CI):
#   brew install vhs           # pulls in ttyd + ffmpeg
# Record:
#   go build -o aitop ./cmd/aitop/ && vhs tapes/social.tape
# Output:
#   docs/assets/aitop-social.gif
#
# Drop the result into the PR template's "show it off" slot, or anywhere you
# want a lively glance at the board. The FontFamily must be an installed
# monospace/Nerd Font or box-drawing/bars silently misalign — see CONTRIBUTING.

Output docs/assets/aitop-social.gif

Set Shell "zsh"
Set FontSize 15
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set Width 1200
Set Height 760
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 55ms

Hide
Type "clear"
Enter
Sleep 400ms
Show

Type "./aitop --demo"
Sleep 400ms
Enter
Sleep 3000ms

# Flip to the grid so the multi-agent board reads at a glance.
Type "v"
Sleep 6000ms

# Cycle the sort column once so it visibly reacts to input.
Type "o"
Sleep 2500ms

Type "q"
Sleep 600ms
