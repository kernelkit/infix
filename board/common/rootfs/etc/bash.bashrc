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

# don't put duplicate lines or lines starting with space in the history.
export HISTCONTROL=ignoreboth

# append to the history file, don't overwrite it
shopt -s histappend

# for setting history length see HISTSIZE and HISTFILESIZE in bash(1)
export HISTSIZE=1000
export HISTFILESIZE=2000

# case-insensitive filename completion
bind "set completion-ignore-case on"

# show all completions immediately instead of ringing bell
bind "set show-all-if-ambiguous on"

export LANG=C.UTF-8

log()
{
    local fn="/var/log/syslog"
    [ -n "$1" ] && fn="/var/log/$1"
    less +G -r "$fn"
}

follow ()
{
    local fn="/var/log/syslog"
    [ -n "$1" ] && fn="/var/log/$1"

    tail -F -n +1 "$fn"
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
