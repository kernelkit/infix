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

# Disble built-ins
enable -n help
