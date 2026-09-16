# Demo for the README. Records against a throwaway config dir so --save
# never touches the recording user's real aliases. Needs bkk on PATH and
# BKK_API_KEY in the environment.
Output ./assets/demo.gif

Set Shell zsh
Set FontSize 16
Set Width 960
Set Height 560
Set TypingSpeed 80ms

Hide
Type "export XDG_CONFIG_HOME=/tmp/bkk-demo && rm -rf $XDG_CONFIG_HOME && clear"
Enter
Show

# A route and a rough stop name: next departure in each direction.
Type "bkk 155 viranyos"
Sleep 0.4s
Enter
Sleep 2.5s

# Accents are optional, more departures, with clock times.
Type "bkk 6 szell kalman -c 3 -t"
Sleep 0.4s
Enter
Sleep 3s

# Remember the lookup you keep typing.
Type "bkk 155 viranyos -c 3 -t --save home"
Sleep 0.4s
Enter
Sleep 3s

Hide
Type "clear"
Enter
Show

# From now on, one word.
Type "bkk home"
Sleep 0.4s
Enter
Sleep 2.5s

# Alias flags can still be overridden, and JSON is there for scripts.
Type "bkk home -c 1 -j | jq -c '.directions[] | {headsign, in: .departures[0].inSeconds}'"
Sleep 0.4s
Enter
Sleep 2.5s

Type "bkk -a"
Sleep 0.4s
Enter
Sleep 4s
