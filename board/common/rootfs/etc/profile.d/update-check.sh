if [ -s /run/infix-update ]; then
    printf '\n\033[1;33m *** %s ***\033[0m\n\n' "$(cat /run/infix-update)"
fi
