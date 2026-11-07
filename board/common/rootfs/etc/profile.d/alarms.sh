# Count active alarms, leaving out the shelved and the closed ones
n=0
for f in /run/alarm/active/*/*; do
    [ -f "$f" ] || continue
    e="/run/alarm/entry/${f#/run/alarm/active/}"
    [ -f "$e" ] && grep -q '"shelf"\|"operator-state":"closed"' "$e" && continue
    n=$((n + 1))
done
if [ $n -gt 0 ]; then
    printf '\n\033[1;33m %d active alarm(s), see "show alarms"\033[0m\n\n' "$n"
fi
