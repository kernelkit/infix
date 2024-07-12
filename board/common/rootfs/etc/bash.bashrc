# Hey Emacs, this is -*-sh-*-
# System-wide .bashrc file for interactive bash(1) shells.

# If not running interactively, don't do anything
[ -z "$PS1" ] && return

# Reevaluate for each line, in case hostname changes
function prompt_command
{
    PS1="\u@$(hostname):\w\$ "
}
export PROMPT_COMMAND=prompt_command

# check the window size after each command and, if necessary,
# update the values of LINES and COLUMNS.
shopt -s checkwinsize

log()
{
    local fn="/var/log/syslog"
    [ -n "$1" ] && fn="/var/log/$1"
    less +G "$fn"
}

follow()
{
    local fn="/var/log/syslog"
    [ -n "$1" ] && fn="/var/log/$1"
    tail -F "$fn"
}

_logfile_completions()
{
    local cur=${COMP_WORDS[COMP_CWORD]}
    local files=$(compgen -f -- "/var/log/$cur")
    COMPREPLY=()
    for file in $files; do
        [ -f "$file" ] && COMPREPLY+=("$(basename "$file")")
    done
}

complete -F _logfile_completions log
complete -F _logfile_completions follow
